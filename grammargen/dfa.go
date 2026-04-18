package grammargen

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// dfaState is a state in the deterministic finite automaton.
type dfaState struct {
	transitions    []dfaTransition
	accept         int  // symbol ID if accepting, 0 if not
	acceptPriority int  // lower = higher priority (from NFA)
	skip           bool // true for whitespace/extra tokens
}

// dfaTransition maps a character range to a next state.
type dfaTransition struct {
	lo, hi    rune
	nextState int
}

// buildLexDFA constructs a DFA from the terminal patterns and produces
// LexState tables compatible with the gotreesitter runtime.
// It builds per-lex-mode DFAs based on which terminals are valid in each mode.
// skipExtras contains only the extras that should be silently consumed (e.g.,
// whitespace). Visible extras like `comment` should NOT be in skipExtras — they
// produce tree nodes via shift-extra parse actions.
// Returns the concatenated LexStates and per-mode start offsets.
func buildLexDFA(ctx context.Context, patterns []TerminalPattern, extraSymbols []int, skipExtras map[int]bool, lexModes []lexModeSpec) ([]gotreesitter.LexState, []int, error) {
	extraSet := make(map[int]bool)
	for _, e := range extraSymbols {
		extraSet[e] = true
	}

	var allStates []gotreesitter.LexState
	modeOffsets := make([]int, len(lexModes))

	for mi, mode := range lexModes {
		modeOffsets[mi] = len(allStates)
		// Filter patterns to only those valid in this mode.
		var modePatterns []TerminalPattern
		for _, p := range patterns {
			if mode.validSymbols[p.SymbolID] || extraSet[p.SymbolID] {
				modePatterns = append(modePatterns, p)
			}
		}

		// Build combined NFA for this mode's terminals.
		combined, err := buildCombinedNFA(modePatterns)
		if err != nil {
			return nil, nil, err
		}

		// Build immediate symbol set for this mode.
		immediateSyms := make(map[int]bool)
		for _, p := range modePatterns {
			if p.Immediate {
				immediateSyms[p.SymbolID] = true
			}
		}

		// Convert NFA to DFA via subset construction. Pass immediateSyms
		// so the accept logic prefers non-immediate tokens over immediate
		// ones when both accept in the same DFA state.
		dfa, err := subsetConstruction(ctx, combined, immediateSyms)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"lex mode %d (patterns=%d valid_symbols=%d nfa_states=%d skip_ws=%v): %w",
				mi, len(modePatterns), len(mode.validSymbols), len(combined.states), mode.skipWhitespace, err,
			)
		}

		if len(immediateSyms) > 0 {
			pruneImmediateTransitions(dfa, immediateSyms)
		}

		// Mark skip states only for invisible extra symbols (like whitespace).
		// Named/visible extras (like `comment`) must NOT be skipped — they
		// produce tree nodes via shift-extra parse actions.
		for i := range dfa {
			if dfa[i].accept > 0 && skipExtras[dfa[i].accept] {
				dfa[i].skip = true
			}
		}

		// Fix extras-override conflicts in merged lex modes.
		//
		// When LALR merging combines character-class and non-character-class
		// parser states, the lex mode's DFA may contain both a catch-all
		// pattern (like class_character = [^\\\]\-]) and the extras pattern
		// (like _whitespace = \r?\n). The catch-all matches extras characters
		// (\n, \r) with better priority than extras (prio 0 vs 2000), so the
		// DFA accepts the catch-all token instead of skipping whitespace.
		//
		// This fix walks the DFA from the start state following extras-pattern
		// character paths. If the DFA would accept a non-skip token where the
		// extras pattern should produce a skip, AND that token can also be
		// reached via non-extras characters (meaning it's not exclusively
		// reachable through extras chars), we change the accept to skip.
		if mode.skipWhitespace && len(dfa) > 0 {
			dfa = fixExtrasOverrideConflicts(dfa, modePatterns, extraSet, skipExtras)
		}

		// Convert to LexState format.
		lexStates := convertDFAToLexStates(dfa, mode.skipWhitespace)

		// Offset all transition targets and skip-loop targets to account
		// for concatenation with previous modes' states.
		offset := len(allStates)
		if offset > 0 {
			for i := range lexStates {
				for j := range lexStates[i].Transitions {
					if lexStates[i].Transitions[j].NextState >= 0 {
						lexStates[i].Transitions[j].NextState += offset
					}
				}
				if lexStates[i].Default >= 0 {
					lexStates[i].Default += offset
				}
				if lexStates[i].EOF >= 0 {
					lexStates[i].EOF += offset
				}
			}
		}

		allStates = append(allStates, lexStates...)
	}

	return allStates, modeOffsets, nil
}

// lexModeSpec describes what a lex mode should recognize.
type lexModeSpec struct {
	validSymbols   map[int]bool // terminal symbol IDs valid in this mode
	skipWhitespace bool         // whether to add skip transitions for whitespace
}

// fixExtrasOverrideConflicts fixes DFA states where a non-extras terminal
// overrides the extras (skip) pattern for the same characters. This happens
// when LALR merging creates lex modes containing both broad catch-all patterns
// (like class_character = [^\\\]\-]) and extras patterns (like \r?\n).
//
// The fix builds a mini-DFA for the extras pattern to determine which first
// characters the extras pattern would match. Then, from the main DFA's start
// state, for each transition on those characters that leads to a non-skip
// accept: if that accept symbol is a SECONDARY pattern (receives fewer than
// half of the start state's character transitions), redirect those transitions
// to a new skip state. In pure character-class modes, the catch-all pattern
// is dominant and left unchanged.
func fixExtrasOverrideConflicts(dfa []dfaState, modePatterns []TerminalPattern, extraSet map[int]bool, skipExtras map[int]bool) []dfaState {
	if len(dfa) == 0 {
		return dfa
	}

	// Build set of symbol IDs that must never be overridden to skip:
	// 1. Immediate tokens (e.g. IMMTOKEN(\r?\n) at end of C #define)
	// 2. String tokens (e.g. STRING("\n") in C preproc_if after condition)
	// These are explicit grammar-authored tokens that share characters with
	// extras patterns but must be preserved for the parser to shift them.
	protectedSyms := make(map[int]bool)
	for _, p := range modePatterns {
		if p.Immediate {
			protectedSyms[p.SymbolID] = true
		}
		if p.Rule != nil && p.Rule.Kind == RuleString {
			protectedSyms[p.SymbolID] = true
		}
	}

	// Find the skip extras pattern(s) and compute their first-char set.
	skipSymID := 0
	extrasFirstChars := make(map[rune]bool)
	for _, p := range modePatterns {
		if !skipExtras[p.SymbolID] || p.Rule == nil {
			continue
		}
		if skipSymID == 0 {
			skipSymID = p.SymbolID
		}
		// Build a mini NFA for the extras pattern and determine the set
		// of characters that can start a match (first-char set).
		miniNFA, err := buildCombinedNFA([]TerminalPattern{p})
		if err != nil || miniNFA == nil {
			continue
		}
		// Find characters reachable from the NFA's start state through
		// epsilon closures and first real transitions.
		startClosure := epsilonClosure(miniNFA, []int{miniNFA.start})
		for _, s := range startClosure {
			for _, tr := range miniNFA.states[s].transitions {
				if !tr.epsilon {
					for r := tr.lo; r <= tr.hi; r++ {
						extrasFirstChars[r] = true
					}
				}
			}
		}
	}
	if skipSymID == 0 || len(extrasFirstChars) == 0 {
		return dfa
	}

	// Count how many character transitions from the start state lead to
	// each non-skip accept symbol.
	startState := &dfa[0]
	symTransCount := make(map[int]int)
	totalNonSkipTrans := 0
	for _, tr := range startState.transitions {
		target := &dfa[tr.nextState]
		if target.accept > 0 && !target.skip {
			charCount := int(tr.hi - tr.lo + 1)
			symTransCount[target.accept] += charCount
			totalNonSkipTrans += charCount
		}
	}
	if totalNonSkipTrans == 0 {
		return dfa
	}

	// Fix: for each transition from start on extras-first characters,
	// if the target accepts a secondary (non-dominant) symbol, redirect
	// to a new skip state.
	for ti, tr := range startState.transitions {
		// Check that ALL characters in this transition range are extras
		// first characters.
		allExtras := true
		for r := tr.lo; r <= tr.hi; r++ {
			if !extrasFirstChars[r] {
				allExtras = false
				break
			}
		}
		if !allExtras {
			continue
		}

		target := &dfa[tr.nextState]
		if target.accept <= 0 || target.skip {
			continue
		}

		// Never override protected tokens (immediate or string) to skip.
		// These are significant grammar tokens: immediate tokens like the
		// terminating \r?\n in C preprocessor directives, or string tokens
		// like "\n" in C preproc_if. They share characters with extras
		// (\s matches \n) but must be preserved for the parser to shift.
		if protectedSyms[target.accept] {
			continue
		}

		// Check if this symbol is secondary (fewer than half of total
		// non-skip transitions). Dominant symbols are left unchanged.
		count := symTransCount[target.accept]
		if count*2 >= totalNonSkipTrans {
			continue
		}

		// Create a new DFA state with skip accept.
		newStateIdx := len(dfa)
		dfa = append(dfa, dfaState{
			transitions:    target.transitions,
			accept:         skipSymID,
			acceptPriority: 2000,
			skip:           true,
		})
		startState.transitions[ti].nextState = newStateIdx
		startState = &dfa[0] // re-anchor after potential realloc
	}
	return dfa
}

// computeStringPrefixExtensions returns, for each string literal symbol that
// is a strict prefix of another string literal, the set of longer-literal
// symbols. When a shorter literal is valid in a lex mode, the lexer must also
// consider the longer literals so it can produce the longest possible match
// (e.g., "---" is valid → "----" must also be in the lex mode).
func computeStringPrefixExtensions(patterns []TerminalPattern) map[int][]int {
	bySymbol := make(map[int]string)
	for _, pat := range patterns {
		if pat.Rule == nil || pat.Rule.Kind != RuleString {
			continue
		}
		if _, ok := bySymbol[pat.SymbolID]; !ok {
			bySymbol[pat.SymbolID] = pat.Rule.Value
		}
	}
	if len(bySymbol) == 0 {
		return nil
	}

	out := make(map[int][]int)
	for shortSym, shortLit := range bySymbol {
		for longSym, longLit := range bySymbol {
			if shortSym == longSym || len(longLit) <= len(shortLit) {
				continue
			}
			if strings.HasPrefix(longLit, shortLit) {
				out[shortSym] = append(out[shortSym], longSym)
			}
		}
		sort.Ints(out[shortSym])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type dfaStateWorkItem struct {
	id     int
	states []int
}

type dfaStateHashEntry struct {
	stateIdx int
	next     *dfaStateHashEntry
}

type closureCacheEntry struct {
	targets []int
	closure []int
	next    *closureCacheEntry
}

type nfaRangeMove struct {
	lo, hi  rune
	targets []int
}

type nfaSweepEvent struct {
	point     rune
	nextState int
	delta     int
}

// dfaSubsetScratch owns reusable buffers for one NFA→DFA subset construction.
// It lets us probe candidate closures without allocating fresh maps/slices on
// every range transition; new backing storage is only retained for novel DFA
// states that survive addState.
type dfaSubsetScratch struct {
	seenGen uint32
	seen    []uint32
	stack   []int
	closure []int
}

func newDFASubsetScratch(stateCount int) dfaSubsetScratch {
	return dfaSubsetScratch{seenGen: 1, seen: make([]uint32, stateCount)}
}

func (s *dfaSubsetScratch) nextSeenGen() uint32 {
	s.seenGen++
	if s.seenGen == 0 {
		for i := range s.seen {
			s.seen[i] = 0
		}
		s.seenGen = 1
	}
	return s.seenGen
}

func (s *dfaSubsetScratch) epsilonClosure(n *nfa, seeds []int) []int {
	gen := s.nextSeenGen()
	stack := s.stack[:0]
	closure := s.closure[:0]
	for _, state := range seeds {
		if s.seen[state] == gen {
			continue
		}
		s.seen[state] = gen
		closure = append(closure, state)
		stack = append(stack, state)
	}
	for len(stack) > 0 {
		state := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, t := range n.states[state].transitions {
			if !t.epsilon || s.seen[t.nextState] == gen {
				continue
			}
			s.seen[t.nextState] = gen
			closure = append(closure, t.nextState)
			stack = append(stack, t.nextState)
		}
	}
	sort.Ints(closure)
	s.stack = stack[:0]
	s.closure = closure
	return closure
}

func hashIntSlice(vals []int) uint64 {
	h := uint64(0xcbf29ce484222325)
	for _, v := range vals {
		h ^= uint64(uint32(v))
		h *= 0x100000001b3
	}
	return h
}

// subsetConstruction converts an NFA to a DFA using the subset construction algorithm.
func subsetConstruction(ctx context.Context, n *nfa, _ ...map[int]bool) ([]dfaState, error) {
	scratch := newDFASubsetScratch(len(n.states))

	// Compute epsilon closure of start state.
	startClosure := scratch.epsilonClosure(n, []int{n.start})

	stateMap := make(map[uint64]*dfaStateHashEntry) // closure hash → DFA state index chain
	closureCache := make(map[uint64]*closureCacheEntry)
	var stateSets [][]int
	var dfaStates []dfaState
	var worklist []dfaStateWorkItem

	addState := func(states []int) int {
		hash := hashIntSlice(states)
		for entry := stateMap[hash]; entry != nil; entry = entry.next {
			if sameIntSlice(stateSets[entry.stateIdx], states) {
				return entry.stateIdx
			}
		}
		id := len(dfaStates)
		stored := append([]int(nil), states...)
		stateMap[hash] = &dfaStateHashEntry{stateIdx: id, next: stateMap[hash]}
		stateSets = append(stateSets, stored)

		// Determine accept symbol (highest priority = lowest priority number).
		// On tie, prefer the lower symbol ID — this matches tree-sitter C's
		// implicit ordering where string terminals (keywords) get lower IDs
		// than pattern terminals (identifiers).
		accept := 0
		bestPriority := int(^uint(0) >> 1) // max int
		for _, s := range stored {
			if n.states[s].accept > 0 {
				p := n.states[s].priority
				sym := n.states[s].accept
				if p < bestPriority || (p == bestPriority && sym < accept) {
					bestPriority = p
					accept = sym
				}
			}
		}

		dfaStates = append(dfaStates, dfaState{accept: accept, acceptPriority: bestPriority})
		worklist = append(worklist, dfaStateWorkItem{id: id, states: stored})
		return id
	}

	closureForTargets := func(targets []int) []int {
		hash := hashIntSlice(targets)
		for entry := closureCache[hash]; entry != nil; entry = entry.next {
			if sameIntSlice(entry.targets, targets) {
				return entry.closure
			}
		}
		closure := append([]int(nil), scratch.epsilonClosure(n, targets)...)
		closureCache[hash] = &closureCacheEntry{
			targets: append([]int(nil), targets...),
			closure: closure,
			next:    closureCache[hash],
		}
		return closure
	}

	addState(startClosure)

	worklistIter := 0
	for len(worklist) > 0 {
		current := worklist[0]
		worklist = worklist[1:]
		curID := current.id

		// Check for cancellation every 64 iterations.
		worklistIter++
		if worklistIter&63 == 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("subset construction cancelled after %d DFA states: %w", len(dfaStates), ctx.Err())
			default:
			}
		}

		// Partition character space once and carry direct target sets forward so
		// subset construction does not rescan all NFA transitions for every range.
		moves := collectTransitionMoves(n, current.states)
		if len(moves) > 0 {
			dfaStates[curID].transitions = make([]dfaTransition, 0, len(moves))
		}

		// For each character range, epsilon-close the direct target set.
		for _, move := range moves {
			targetStates := closureForTargets(move.targets)
			if len(targetStates) == 0 {
				continue
			}
			targetID := addState(targetStates)
			transitions := dfaStates[curID].transitions
			if n := len(transitions); n > 0 {
				last := &transitions[n-1]
				if last.nextState == targetID && last.hi+1 == move.lo {
					last.hi = move.hi
					dfaStates[curID].transitions = transitions
					continue
				}
			}
			dfaStates[curID].transitions = append(transitions,
				dfaTransition{lo: move.lo, hi: move.hi, nextState: targetID})
		}
	}

	return dfaStates, nil
}

// epsilonClosure computes the epsilon closure of a set of NFA states.
func epsilonClosure(n *nfa, states []int) []int {
	seen := make(map[int]bool)
	var stack []int
	for _, s := range states {
		if !seen[s] {
			seen[s] = true
			stack = append(stack, s)
		}
	}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, t := range n.states[s].transitions {
			if t.epsilon && !seen[t.nextState] {
				seen[t.nextState] = true
				stack = append(stack, t.nextState)
			}
		}
	}
	result := make([]int, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	sort.Ints(result)
	return result
}

// collectTransitionRanges collects all non-epsilon character transition ranges
// from the given NFA states and partitions them into non-overlapping ranges.
func collectTransitionRanges(n *nfa, states []int) []runeRange {
	transitionCount := 0
	for _, s := range states {
		for _, t := range n.states[s].transitions {
			if !t.epsilon {
				transitionCount++
			}
		}
	}
	if transitionCount == 0 {
		return nil
	}

	// Collect boundary points.
	points := make([]rune, 0, transitionCount*2)
	if transitionCount >= 128 {
		for _, s := range states {
			for _, t := range n.states[s].transitions {
				if t.epsilon {
					continue
				}
				points = append(points, t.lo, t.hi+1) // exclusive upper bound
			}
		}
		sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
		write := 1
		for read := 1; read < len(points); read++ {
			if points[read] != points[write-1] {
				points[write] = points[read]
				write++
			}
		}
		points = points[:write]
	} else {
		pointSet := make(map[rune]bool, transitionCount*2)
		addPoint := func(r rune) {
			if !pointSet[r] {
				pointSet[r] = true
				points = append(points, r)
			}
		}
		for _, s := range states {
			for _, t := range n.states[s].transitions {
				if t.epsilon {
					continue
				}
				addPoint(t.lo)
				addPoint(t.hi + 1) // exclusive upper bound
			}
		}
		sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	}

	// Create non-overlapping ranges from boundary points.
	ranges := make([]runeRange, 0, len(points))
	for i := 0; i < len(points); i++ {
		lo := points[i]
		var hi rune
		if i+1 < len(points) {
			hi = points[i+1] - 1
		} else {
			hi = lo
		}
		if lo > hi {
			continue
		}
		// Check if any NFA transition covers this range.
		hasTransition := false
		for _, s := range states {
			for _, t := range n.states[s].transitions {
				if !t.epsilon && t.lo <= lo && t.hi >= hi {
					hasTransition = true
					break
				}
			}
			if hasTransition {
				break
			}
		}
		if hasTransition {
			ranges = append(ranges, runeRange{lo, hi})
		}
	}

	return ranges
}

// collectTransitionMoves partitions the current NFA frontier into
// non-overlapping character ranges and records the direct move targets for each
// range in a single sweep. This avoids rescanning all NFA transitions first to
// test coverage and then again to recover the same target set.
func collectTransitionMoves(n *nfa, states []int) []nfaRangeMove {
	transitionCount := 0
	for _, s := range states {
		for _, t := range n.states[s].transitions {
			if !t.epsilon {
				transitionCount++
			}
		}
	}
	if transitionCount == 0 {
		return nil
	}

	events := make([]nfaSweepEvent, 0, transitionCount*2)
	for _, s := range states {
		for _, t := range n.states[s].transitions {
			if t.epsilon {
				continue
			}
			events = append(events,
				nfaSweepEvent{point: t.lo, nextState: t.nextState, delta: 1},
				nfaSweepEvent{point: t.hi + 1, nextState: t.nextState, delta: -1},
			)
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].point != events[j].point {
			return events[i].point < events[j].point
		}
		if events[i].nextState != events[j].nextState {
			return events[i].nextState < events[j].nextState
		}
		return events[i].delta < events[j].delta
	})

	active := make(map[int]int)
	moves := make([]nfaRangeMove, 0, len(events))
	for i := 0; i < len(events); {
		point := events[i].point
		for i < len(events) && events[i].point == point {
			ev := events[i]
			active[ev.nextState] += ev.delta
			if active[ev.nextState] == 0 {
				delete(active, ev.nextState)
			}
			i++
		}
		if len(active) == 0 || i >= len(events) {
			continue
		}
		hi := events[i].point - 1
		if point > hi {
			continue
		}
		targets := make([]int, 0, len(active))
		for nextState := range active {
			targets = append(targets, nextState)
		}
		sort.Ints(targets)
		if n := len(moves); n > 0 {
			last := &moves[n-1]
			if last.hi+1 == point && sameIntSlice(last.targets, targets) {
				last.hi = hi
				continue
			}
		}
		moves = append(moves, nfaRangeMove{lo: point, hi: hi, targets: targets})
	}

	return moves
}

func moveTargets(n *nfa, states []int, lo, hi rune) []int {
	var targets []int
	seen := make(map[int]bool)
	for _, s := range states {
		for _, t := range n.states[s].transitions {
			if !t.epsilon && t.lo <= lo && t.hi >= hi && !seen[t.nextState] {
				seen[t.nextState] = true
				targets = append(targets, t.nextState)
			}
		}
	}
	sort.Ints(targets)
	return targets
}

func sameIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// convertDFAToLexStates converts internal DFA states to gotreesitter LexState format.
func convertDFAToLexStates(dfa []dfaState, addSkipTransitions bool) []gotreesitter.LexState {
	states := make([]gotreesitter.LexState, len(dfa))
	totalTransitions := 0
	for i := range dfa {
		totalTransitions += len(dfa[i].transitions)
	}
	// Reserve room for up to three whitespace skip transitions on the mode's
	// start state so addWhitespaceSkip can grow in place.
	if addSkipTransitions && len(dfa) > 0 {
		totalTransitions += 3
	}
	transitionBuf := make([]gotreesitter.LexTransition, totalTransitions)
	bufPos := 0
	for i := range dfa {
		ds := &dfa[i]
		prio := int16(0)
		if ds.accept > 0 && ds.acceptPriority < int(^uint(0)>>1) {
			// Clamp to int16 range.
			if ds.acceptPriority > 32767 {
				prio = 32767
			} else if ds.acceptPriority < -32768 {
				prio = -32768
			} else {
				prio = int16(ds.acceptPriority)
			}
		}
		ls := gotreesitter.LexState{
			AcceptToken:    gotreesitter.Symbol(ds.accept),
			AcceptPriority: prio,
			Skip:           ds.skip,
			Default:        -1,
			EOF:            -1,
		}

		if len(ds.transitions) > 0 {
			extraCap := 0
			if addSkipTransitions && i == 0 {
				extraCap = 3
			}
			ls.Transitions = transitionBuf[bufPos : bufPos+len(ds.transitions) : bufPos+len(ds.transitions)+extraCap]
			for j, t := range ds.transitions {
				ls.Transitions[j] = gotreesitter.LexTransition{
					Lo:        t.lo,
					Hi:        t.hi,
					NextState: t.nextState,
				}
			}
			bufPos += len(ds.transitions) + extraCap
			// Release the source edge slice once it has been copied so the DFA
			// graph does not stay fully duplicated through the rest of conversion.
			ds.transitions = nil
		} else if addSkipTransitions && i == 0 {
			ls.Transitions = transitionBuf[bufPos : bufPos : bufPos+3]
			bufPos += 3
		}

		states[i] = ls
	}

	// For the start state (index 0, local), add skip transitions for whitespace
	// characters if requested. The skip transitions loop back to state 0 (local).
	// The offset adjustment happens later during concatenation.
	if addSkipTransitions && len(states) > 0 {
		addWhitespaceSkip(&states[0])
	}

	return states
}

// addWhitespaceSkip modifies the start state to have skip transitions for
// whitespace characters (\t, \n, \r, space). These transitions loop back
// to the start state with Skip=true.
//
// IMPORTANT: We must NOT mark existing DFA transitions as Skip. Existing
// transitions were created by real terminal patterns (e.g., \r?\n) and must
// remain non-skip so the lexer can match them as real tokens. We only add
// NEW skip transitions for whitespace characters that have no existing
// transition. The DFA already handles whitespace via the extra symbol's
// accepting states (LexState.Skip = true).
func addWhitespaceSkip(state *gotreesitter.LexState) {
	wsRanges := []runeRange{
		{'\t', '\n'}, // \t and \n
		{'\r', '\r'}, // \r
		{' ', ' '},   // space
	}

	for _, ws := range wsRanges {
		// Check if ANY existing transition overlaps with this whitespace range.
		// If so, leave it alone — a real terminal needs that character range.
		// We only add skip transitions for characters that have no existing
		// DFA path, because the DFA already handles extras via accept-state
		// Skip flags.
		overlaps := false
		for i := range state.Transitions {
			t := &state.Transitions[i]
			// Check if the ranges overlap at all.
			if t.Lo <= ws.hi && t.Hi >= ws.lo {
				overlaps = true
				break
			}
		}
		if !overlaps {
			state.Transitions = append(state.Transitions, gotreesitter.LexTransition{
				Lo:        ws.lo,
				Hi:        ws.hi,
				NextState: 0, // loops back to start state (local index)
				Skip:      true,
			})
		}
	}

	// Sort transitions by Lo for deterministic behavior.
	sort.Slice(state.Transitions, func(i, j int) bool {
		return state.Transitions[i].Lo < state.Transitions[j].Lo
	})
}

// pruneImmediateTransitions removes transitions from DFA states that accept
// an immediate token when those transitions can only lead to non-immediate
// (catch-all) accepts. This prevents greedy patterns like [^\r\n]+ from
// defeating shorter immediate tokens like "-" or "---".
//
// In tree-sitter's C lexer, immediate token paths are "dead-end" — once the
// lexer matches an immediate token like "-", it can only continue to other
// immediate tokens like "--" or "---", but never fall through to a catch-all
// like "context". This function replicates that behavior in our combined DFA.
//
// Exception: transitions leading to non-immediate tokens with BETTER priority
// than the current immediate accept are kept. This handles cases like C's
// char_literal where the character IMMTOKEN [^\n'] (prio -500) accepts at '\'
// but escape_sequence TOKEN(PREC(1,...)) (prio -1000) accepts at '\0'. The
// escape_sequence has better priority and should be reachable.
func pruneImmediateTransitions(dfa []dfaState, immediateSyms map[int]bool) {
	n := len(dfa)
	if n == 0 {
		return
	}

	// Step 1: Compute canReachImmediate[i] = true if state i (or any
	// reachable descendant) accepts an immediate token.
	canReachImmediate := make([]bool, n)
	for i, s := range dfa {
		if s.accept > 0 && immediateSyms[s.accept] {
			canReachImmediate[i] = true
		}
	}

	// Propagate backwards: if any successor can reach an immediate accept,
	// so can the current state. Iterate until stable.
	for changed := true; changed; {
		changed = false
		for i, s := range dfa {
			if canReachImmediate[i] {
				continue
			}
			for _, t := range s.transitions {
				if canReachImmediate[t.nextState] {
					canReachImmediate[i] = true
					changed = true
					break
				}
			}
		}
	}

	// Step 1b: Compute bestReachablePriority[i] = lowest (best) accept
	// priority reachable from state i. Used to preserve transitions to
	// non-immediate tokens that have better priority than the current accept.
	const maxPrio = int(^uint(0) >> 1) // max int
	bestReachablePriority := make([]int, n)
	for i := range bestReachablePriority {
		bestReachablePriority[i] = maxPrio
	}
	for i, s := range dfa {
		if s.accept > 0 && s.acceptPriority < bestReachablePriority[i] {
			bestReachablePriority[i] = s.acceptPriority
		}
	}
	// Propagate backwards.
	for changed := true; changed; {
		changed = false
		for i, s := range dfa {
			for _, t := range s.transitions {
				if bestReachablePriority[t.nextState] < bestReachablePriority[i] {
					bestReachablePriority[i] = bestReachablePriority[t.nextState]
					changed = true
				}
			}
		}
	}

	// Step 2: For each state that accepts an immediate token, keep only
	// transitions whose targets can reach another immediate token accept
	// OR can reach a non-immediate token with better priority.
	for i := range dfa {
		if dfa[i].accept == 0 || !immediateSyms[dfa[i].accept] {
			continue
		}
		curPrio := dfa[i].acceptPriority
		var kept []dfaTransition
		for _, t := range dfa[i].transitions {
			if canReachImmediate[t.nextState] {
				kept = append(kept, t)
			} else if bestReachablePriority[t.nextState] < curPrio {
				// Keep transition to non-immediate token with better priority.
				// This preserves paths like '\' (character IMMTOKEN prio -500)
				// continuing to '\0' (escape_sequence TOKEN prio -1000).
				kept = append(kept, t)
			}
		}
		dfa[i].transitions = kept
	}
}

// computeLexModes determines the lex modes needed for the parse table.
// Each unique set of valid terminal symbols gets its own lex mode.
// Returns the lex mode specs and a mapping from parser state to lex mode index.
// terminalPatternSymSet returns the set of symbol IDs that have DFA terminal
// patterns. Used to distinguish dual-role external tokens (which have both a
// scanner entry and a DFA pattern) from pure-external tokens.
// patternImmediateTokenSet returns symbol IDs of immediate tokens that are
// PATTERN-based (catch-all regex patterns like [^@:\s\$]+). String-based
// IMMTOKENs like ":", "=", "mount" are excluded — they're legitimate tokens
// even after whitespace.
func patternImmediateTokenSet(ng *NormalizedGrammar) map[int]bool {
	m := make(map[int]bool)
	for _, t := range ng.Terminals {
		if !t.Immediate {
			continue
		}
		// A terminal is pattern-based if its rule is a RulePattern or contains
		// patterns (via RuleSeq/RuleChoice wrapping patterns).
		if ruleContainsPattern(t.Rule) && !isStringOnlyRule(t.Rule) {
			m[t.SymbolID] = true
		}
	}
	return m
}

func ruleContainsPattern(r *Rule) bool {
	if r == nil {
		return false
	}
	if r.Kind == RulePattern {
		return true
	}
	for _, c := range r.Children {
		if ruleContainsPattern(c) {
			return true
		}
	}
	return false
}

func isStringOnlyRule(r *Rule) bool {
	if r == nil {
		return false
	}
	switch r.Kind {
	case RuleString:
		return true
	case RuleSeq, RuleChoice:
		for _, c := range r.Children {
			if !isStringOnlyRule(c) {
				return false
			}
		}
		return len(r.Children) > 0
	default:
		return false
	}
}

func terminalPatternSymSet(ng *NormalizedGrammar) map[int]bool {
	m := make(map[int]bool, len(ng.Terminals))
	for _, t := range ng.Terminals {
		m[t.SymbolID] = true
	}
	return m
}

func computeLexModes(
	stateCount int,
	tokenCount int,
	actionLookup func(state, sym int) bool,
	stringPrefixExtensions map[int][]int,
	extraSymbols []int,
	extraChainStateStart int,
	immediateTokens map[int]bool,
	externalSymbols []int,
	wordSymbolID int,
	keywordSymbols map[int]bool,
	terminalPatternSyms map[int]bool, // symbols that have DFA terminal patterns
	followTokens func(state int) []int, // additional tokens from reduce-follow expansion (may be nil)
	patternImmediateTokens map[int]bool, // immediate tokens that are PATTERN-based (catch-all)
) ([]lexModeSpec, []int, []afterWSModeEntry) {
	extraSet := make(map[int]bool)
	hasTerminalExtras := false
	for _, e := range extraSymbols {
		extraSet[e] = true
		if e > 0 && e < tokenCount {
			hasTerminalExtras = true
		}
	}

	// External tokens are handled by the external scanner, not the DFA.
	// Exclude pure-external tokens from lex mode computation. Only keep
	// external tokens that ALSO have a corresponding terminal pattern in
	// the DFA (like Python's ")", "]", "}", "except"). Checking for a
	// terminal pattern is more precise than checking action-table presence,
	// because most external tokens appear in action tables but only
	// dual-role tokens have actual DFA patterns.
	extSet := make(map[int]bool)
	for _, e := range externalSymbols {
		if !terminalPatternSyms[e] {
			extSet[e] = true
		}
	}

	modeMap := make(map[string]int) // key → mode index
	var modes []lexModeSpec
	stateToMode := make([]int, stateCount)
	var afterWSModeMap []afterWSModeEntry

	for state := 0; state < stateCount; state++ {
		isExtraChainState := extraChainStateStart >= 0 && state >= extraChainStateStart
		// Collect valid terminal symbols for this state.
		validSyms := make(map[int]bool)
		hasImmediate := false
		hasKeyword := false
		// Collect all directly-valid or follow-valid symbols first, then
		// add prefix extensions only for longer tokens that are themselves
		// valid. Without this gate the DFA greedily matches a longer prefix
		// (e.g. "?:" for "?") that the parser has no action for, consuming
		// too many characters and causing a parse error.
		directValid := make(map[int]bool) // symbols valid by action or follow
		for sym := 1; sym < tokenCount; sym++ {
			if extSet[sym] {
				continue // skip external tokens
			}
			if actionLookup(state, sym) {
				directValid[sym] = true
			}
		}
		if followTokens != nil {
			for _, sym := range followTokens(state) {
				// Reduce-follow expansion exists to admit the word token in
				// states where a keyword becomes valid only after reducing a
				// preceding nonterminal. Widening lex modes with every follow
				// terminal is both unnecessary and expensive for large grammars.
				if sym > 0 && sym < tokenCount && !extSet[sym] && keywordSymbols[sym] {
					directValid[sym] = true
				}
			}
		}

		for sym := range directValid {
			validSyms[sym] = true
			// Only add prefix extensions when the longer symbol is also
			// directly valid. This prevents the DFA from greedily matching
			// e.g. "?:" when only "?" has a parse action.
			for _, longerSym := range stringPrefixExtensions[sym] {
				if !extSet[longerSym] && directValid[longerSym] {
					validSyms[longerSym] = true
				}
			}
			if immediateTokens[sym] {
				hasImmediate = true
			}
			if keywordSymbols[sym] {
				hasKeyword = true
			}
		}

		// Extra terminal symbols (e.g., whitespace pattern) must be valid
		// in every lex mode so they can always be recognized. Only include
		// terminal extras (ID < tokenCount); nonterminal extras are handled
		// by the parser, not the lexer. But also include the first-set
		// terminals of nonterminal extras so the lexer can recognize the
		// start of nonterminal extra rules (like comment → [;#]...).
		stateHasTerminalExtras := hasTerminalExtras && !isExtraChainState
		for _, e := range extraSymbols {
			if stateHasTerminalExtras && e > 0 && e < tokenCount {
				validSyms[e] = true
			}
		}
		// Note: nonterminal extra first-set terminals are NOT unconditionally
		// added here. They're already in main states' action tables via
		// addNonterminalExtraChains chain shifts, so actionLookup picks them
		// up naturally. Forcing them into every lex mode (including chain
		// states) creates DFA conflicts — e.g., \r?\n from _blank competes
		// with [^\r\n]* in comment chain states, and the longer match wins,
		// producing the wrong token.

		// When any keyword symbol is valid in this state, include the word
		// token in the lex mode. Keywords are excluded from the main DFA
		// and recognized via the word token + keyword promotion DFA.
		if hasKeyword && wordSymbolID > 0 {
			validSyms[wordSymbolID] = true
		}

		// Determine if whitespace should be skipped in this mode.
		// When the grammar has no terminal extras (extras=[]), whitespace
		// must never be skipped — the grammar handles all whitespace explicitly.
		// Otherwise, skip whitespace unless ALL valid tokens are immediate.
		skipWS := stateHasTerminalExtras && (!hasImmediate || len(validSyms) > countImmediate(validSyms, immediateTokens))

		key := buildModeKey(validSyms, skipWS)

		if modeIdx, ok := modeMap[key]; ok {
			stateToMode[state] = modeIdx
		} else {
			modeIdx := len(modes)
			modeMap[key] = modeIdx
			modes = append(modes, lexModeSpec{
				validSymbols:   validSyms,
				skipWhitespace: skipWS,
			})
			stateToMode[state] = modeIdx
		}

		// For states with both immediate tokens and non-immediate STRING tokens
		// that overlap (same first character), create an after-whitespace variant
		// that excludes immediate tokens. This lets STRING keywords win after
		// whitespace where immediate continuation tokens would otherwise dominate.
		if hasImmediate && !isExtraChainState {
			hasNonImmString := false
			for sym := range validSyms {
				if !immediateTokens[sym] && sym > 0 && sym < tokenCount {
					hasNonImmString = true
					break
				}
			}
			if hasNonImmString {
				awsSyms := make(map[int]bool)
				for sym := range validSyms {
					// Only exclude pattern-based immediate tokens (catch-alls).
					// String-based IMMTOKENs like ":", "=", "}" are kept
					// because they're legitimate after whitespace.
					if !patternImmediateTokens[sym] {
						awsSyms[sym] = true
					}
				}
				if len(awsSyms) > 0 && len(awsSyms) < len(validSyms) {
					awsKey := buildModeKey(awsSyms, skipWS)
					if awsModeIdx, ok := modeMap[awsKey]; ok {
						afterWSModeMap = append(afterWSModeMap, afterWSModeEntry{state, awsModeIdx})
					} else {
						awsModeIdx := len(modes)
						modeMap[awsKey] = awsModeIdx
						modes = append(modes, lexModeSpec{
							validSymbols:   awsSyms,
							skipWhitespace: skipWS,
						})
						afterWSModeMap = append(afterWSModeMap, afterWSModeEntry{state, awsModeIdx})
					}
				}
			}
		}
	}

	return modes, stateToMode, afterWSModeMap
}

// afterWSModeEntry maps a parser state to its after-whitespace lex mode index.
// Only populated for states that have both immediate and non-immediate STRING tokens.
type afterWSModeEntry struct {
	stateIdx int
	modeIdx  int
}

func countImmediate(syms map[int]bool, imm map[int]bool) int {
	n := 0
	for s := range syms {
		if imm[s] {
			n++
		}
	}
	return n
}

func buildModeKey(syms map[int]bool, skip bool) string {
	sorted := make([]int, 0, len(syms))
	for s := range syms {
		sorted = append(sorted, s)
	}
	sort.Ints(sorted)
	buf := make([]byte, len(sorted)*4+1)
	for i, s := range sorted {
		buf[i*4] = byte(s >> 24)
		buf[i*4+1] = byte(s >> 16)
		buf[i*4+2] = byte(s >> 8)
		buf[i*4+3] = byte(s)
	}
	if skip {
		buf[len(buf)-1] = 1
	}
	return string(buf)
}

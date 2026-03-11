package grammargen

import (
	"sort"

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
func buildLexDFA(patterns []TerminalPattern, extraSymbols []int, skipExtras map[int]bool, lexModes []lexModeSpec) ([]gotreesitter.LexState, []int, error) {
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

		// Convert NFA to DFA via subset construction.
		dfa := subsetConstruction(combined)

		// Prune transitions from immediate-accepting DFA states that can
		// only reach non-immediate (catch-all) patterns. This prevents
		// greedy patterns like [^\r\n]+ from defeating immediate tokens
		// like "-", "---", "+", "+++" in diff-style grammars.
		immediateSyms := make(map[int]bool)
		for _, p := range modePatterns {
			if p.Immediate {
				immediateSyms[p.SymbolID] = true
			}
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


type dfaStateWorkItem struct {
	id     int
	states []int
}

type dfaStateHashEntry struct {
	stateIdx int
	next     *dfaStateHashEntry
}

// dfaSubsetScratch owns reusable buffers for one NFA→DFA subset construction.
// It lets us probe candidate closures without allocating fresh maps/slices on
// every range transition; new backing storage is only retained for novel DFA
// states that survive addState.
type dfaSubsetScratch struct {
	seenGen uint32
	seen    []uint32
	stack   []int
	move    []int
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

func (s *dfaSubsetScratch) collectMoveTargets(n *nfa, states []int, lo, hi rune) []int {
	gen := s.nextSeenGen()
	targets := s.move[:0]
	for _, state := range states {
		for _, t := range n.states[state].transitions {
			if t.epsilon || t.lo > lo || t.hi < hi || s.seen[t.nextState] == gen {
				continue
			}
			s.seen[t.nextState] = gen
			targets = append(targets, t.nextState)
		}
	}
	sort.Ints(targets)
	s.move = targets
	return targets
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
func subsetConstruction(n *nfa) []dfaState {
	scratch := newDFASubsetScratch(len(n.states))

	// Compute epsilon closure of start state.
	startClosure := scratch.epsilonClosure(n, []int{n.start})

	stateMap := make(map[uint64]*dfaStateHashEntry) // closure hash → DFA state index chain
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
		accept := 0
		bestPriority := int(^uint(0) >> 1) // max int
		for _, s := range stored {
			if n.states[s].accept > 0 {
				if n.states[s].priority < bestPriority {
					bestPriority = n.states[s].priority
					accept = n.states[s].accept
				}
			}
		}

		dfaStates = append(dfaStates, dfaState{accept: accept, acceptPriority: bestPriority})
		worklist = append(worklist, dfaStateWorkItem{id: id, states: stored})
		return id
	}

	addState(startClosure)

	for len(worklist) > 0 {
		current := worklist[0]
		worklist = worklist[1:]
		curID := current.id

		// Collect all character ranges from transitions of current NFA states.
		ranges := collectTransitionRanges(n, current.states)
		if len(ranges) > 0 {
			dfaStates[curID].transitions = make([]dfaTransition, 0, len(ranges))
		}

		// For each character range, compute the target NFA state set.
		for _, r := range ranges {
			targetStates := scratch.collectMoveTargets(n, current.states, r.lo, r.hi)
			if len(targetStates) == 0 {
				continue
			}
			targetStates = scratch.epsilonClosure(n, targetStates)
			if len(targetStates) == 0 {
				continue
			}
			targetID := addState(targetStates)
			transitions := dfaStates[curID].transitions
			if n := len(transitions); n > 0 {
				last := &transitions[n-1]
				if last.nextState == targetID && last.hi+1 == r.lo {
					last.hi = r.hi
					dfaStates[curID].transitions = transitions
					continue
				}
			}
			dfaStates[curID].transitions = append(transitions,
				dfaTransition{lo: r.lo, hi: r.hi, nextState: targetID})
		}
	}

	return dfaStates
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

// mergeAdjacentRanges merges adjacent ranges that lead to the same target state set.
func mergeAdjacentRanges(ranges []runeRange, n *nfa, states []int) []runeRange {
	if len(ranges) <= 1 {
		return ranges
	}
	merged := make([]runeRange, 0, len(ranges))
	cur := ranges[0]
	curTarget := moveTargets(n, states, cur.lo, cur.hi)

	for i := 1; i < len(ranges); i++ {
		next := ranges[i]
		nextTarget := moveTargets(n, states, next.lo, next.hi)
		canMerge := next.lo == cur.hi+1 && sameIntSlice(curTarget, nextTarget)
		if canMerge {
			// Merge only when one or more direct NFA transitions cover the
			// entire combined range. Otherwise subsetConstruction(moveAndClose)
			// can later drop the merged range as unreachable.
			combinedTarget := moveTargets(n, states, cur.lo, next.hi)
			canMerge = sameIntSlice(curTarget, combinedTarget)
		}
		if canMerge {
			cur.hi = next.hi
			continue
		}
		merged = append(merged, cur)
		cur = next
		curTarget = nextTarget
	}
	merged = append(merged, cur)
	return merged
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

// moveAndClose computes move(states, [lo,hi]) followed by epsilon closure.
func moveAndClose(n *nfa, states []int, lo, hi rune) []int {
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
	if len(targets) == 0 {
		return nil
	}
	return epsilonClosure(n, targets)
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

	// Step 2: For each state that accepts an immediate token, keep only
	// transitions whose targets can reach another immediate token accept.
	for i := range dfa {
		if dfa[i].accept == 0 || !immediateSyms[dfa[i].accept] {
			continue
		}
		var kept []dfaTransition
		for _, t := range dfa[i].transitions {
			if canReachImmediate[t.nextState] {
				kept = append(kept, t)
			}
		}
		dfa[i].transitions = kept
	}
}

// computeLexModes determines the lex modes needed for the parse table.
// Each unique set of valid terminal symbols gets its own lex mode.
// Returns the lex mode specs and a mapping from parser state to lex mode index.
func computeLexModes(
	stateCount int,
	tokenCount int,
	actionLookup func(state, sym int) bool,
	extraSymbols []int,
	immediateTokens map[int]bool,
	externalSymbols []int,
	wordSymbolID int,
	keywordSymbols map[int]bool,
) ([]lexModeSpec, []int) {
	extraSet := make(map[int]bool)
	hasTerminalExtras := false
	for _, e := range extraSymbols {
		extraSet[e] = true
		if e > 0 && e < tokenCount {
			hasTerminalExtras = true
		}
	}

	// External tokens are handled by the external scanner, not the DFA.
	// Exclude them from lex mode computation to avoid creating spurious
	// lex modes based on external token validity differences.
	extSet := make(map[int]bool)
	for _, e := range externalSymbols {
		extSet[e] = true
	}

	modeMap := make(map[string]int) // key → mode index
	var modes []lexModeSpec
	stateToMode := make([]int, stateCount)

	for state := 0; state < stateCount; state++ {
		// Collect valid terminal symbols for this state.
		validSyms := make(map[int]bool)
		hasImmediate := false
		hasKeyword := false
		for sym := 1; sym < tokenCount; sym++ {
			if extSet[sym] {
				continue // skip external tokens
			}
			if actionLookup(state, sym) {
				validSyms[sym] = true
				if immediateTokens[sym] {
					hasImmediate = true
				}
				if keywordSymbols[sym] {
					hasKeyword = true
				}
			}
		}

		// Extra terminal symbols (e.g., whitespace pattern) must be valid
		// in every lex mode so they can always be recognized. Only include
		// terminal extras (ID < tokenCount); nonterminal extras are handled
		// by the parser, not the lexer. But also include the first-set
		// terminals of nonterminal extras so the lexer can recognize the
		// start of nonterminal extra rules (like comment → [;#]...).
		for _, e := range extraSymbols {
			if e > 0 && e < tokenCount {
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
		skipWS := hasTerminalExtras && (!hasImmediate || len(validSyms) > countImmediate(validSyms, immediateTokens))

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
	}

	return modes, stateToMode
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

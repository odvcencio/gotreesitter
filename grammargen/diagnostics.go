package grammargen

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// buildFollowTokensFunc returns a function that, given a parser state,
// returns the terminal symbols valid in states reachable after a reduce.
// This expands lex modes so that keywords like "AS" in dockerfile can be
// recognized even when parsing inside a production like image_name where
// "AS" isn't directly valid but becomes valid after reducing.

func buildLexModeFollowTokensFunc(tables *LRTables, tokenCount int, ng *NormalizedGrammar, keywordSymbols map[int]bool) func(int) []int {
	follow := buildFollowTokensFunc(tables, tokenCount)
	if follow == nil {
		return nil
	}
	allowed := lexModeFollowTokenSet(ng, tokenCount, keywordSymbols)
	if len(allowed) == 0 {
		return nil
	}
	cache := make(map[int][]int)
	return func(state int) []int {
		if cached, ok := cache[state]; ok {
			return cached
		}
		raw := follow(state)
		if len(raw) == 0 {
			cache[state] = nil
			return nil
		}
		out := make([]int, 0, len(raw))
		for _, sym := range raw {
			if allowed[sym] {
				out = append(out, sym)
			}
		}
		cache[state] = out
		return out
	}
}

func lexModeFollowTokenSet(ng *NormalizedGrammar, tokenCount int, keywordSymbols map[int]bool) map[int]bool {
	allowed := make(map[int]bool, len(keywordSymbols)+8)
	for sym := range keywordSymbols {
		if sym > 0 && sym < tokenCount {
			allowed[sym] = true
		}
	}
	if ng == nil {
		return allowed
	}
	limit := tokenCount
	if limit > len(ng.Symbols) {
		limit = len(ng.Symbols)
	}
	for sym := 1; sym < limit; sym++ {
		if lexModeFollowTokenName(ng.Symbols[sym].Name) {
			allowed[sym] = true
		}
	}
	return allowed
}

func lexModeFollowTokenName(name string) bool {
	switch name {
	case ")", "]", "}", ";", ",":
		return true
	default:
		return false
	}
}

// buildLexModeMissingRecoveryTokensFunc wraps
// buildMissingRecoveryTokensFuncWithContext's raw (unrestricted) recovery
// lookaheads with the same narrow "safe to widen with" allowlist already
// used for reduce-follow widening (see buildLexModeFollowTokensFunc):
// closing/terminator punctuation and keywords. Missing-token recovery's own
// BFS over reachable reduce-chains is, by construction, close to unique per
// LR state (every state's reachable-reduce graph differs), so admitting its
// FULL raw result into every state's lex mode multiplies the number of
// distinct (validSyms) combinations — and therefore the compiled DFA state
// count — roughly in proportion to parser state count on large grammars.
// The overwhelming majority of real "missing token" typo/incomplete-code
// scenarios are exactly the punctuation/keyword categories already deemed
// safe for follow-token widening, so restricting recovery widening to the
// same allowlist keeps the common, valuable cases (the lexer can still see
// a stray ")"/"]"/"}"/ ";"/","/keyword that would let recovery insert the
// missing token) while dropping the long tail of rarely-useful, highly
// state-specific pattern/identifier-shaped lookaheads that were driving
// lex-mode (and DFA state) proliferation without a matching recovery
// benefit.
//
// KNOWN GAP: because the allowlist admits only closing/terminator
// punctuation and keywords, missing-token recovery can no longer see
// expression-start lookaheads (identifiers, literals, prefix operators) that
// the raw BFS would otherwise have surfaced. An incomplete-expression tail
// with NO trailing closing punctuation at all is therefore not flagged as an
// error: `let x = ` (EOF right after the assignment), `1 + ` followed by
// EOF, or a `[1: ]`-style container literal missing its value all parse
// clean today, because inserting the missing identifier/literal requires an
// identifier/literal lookahead that this allowlist drops. Incomplete
// expressions that DO retain trailing closing punctuation are unaffected and
// still flag correctly — e.g. `(1 + )` and `obj.` immediately followed by a
// `)`/`}`/`;` recover via the missing-token insertion exactly as before,
// since that punctuation lookahead is in the allowlist.
//
// Follow-up (not done here): widen the allowlist with a bounded
// expression-start first-set (a small, capped set of identifier/literal-
// shaped lookaheads, NOT the full raw BFS result) to close this gap without
// reintroducing the per-state lex-mode proliferation this narrowing fixed.
// Evaluate this as a dedicated experiment in the blob-consolidation round —
// both swift and go consume this function, so any change here re-triggers a
// full regen+reverify of both blobs.
func buildLexModeMissingRecoveryTokensFunc(ctx context.Context, tables *LRTables, tokenCount int, terminalPatterns []TerminalPattern, skipExtras map[int]bool, ng *NormalizedGrammar, keywordSymbols map[int]bool) func(int) []int {
	raw := buildMissingRecoveryTokensFuncWithContext(ctx, tables, tokenCount, terminalPatterns, skipExtras)
	if raw == nil {
		return nil
	}
	allowed := lexModeFollowTokenSet(ng, tokenCount, keywordSymbols)
	if len(allowed) == 0 {
		return raw
	}
	cache := make(map[int][]int)
	return func(state int) []int {
		if cached, ok := cache[state]; ok {
			return cached
		}
		rawTokens := raw(state)
		if len(rawTokens) == 0 {
			cache[state] = nil
			return nil
		}
		out := make([]int, 0, len(rawTokens))
		for _, sym := range rawTokens {
			if allowed[sym] {
				out = append(out, sym)
			}
		}
		cache[state] = out
		return out
	}
}

func buildFollowTokensFunc(tables *LRTables, tokenCount int) func(int) []int {
	if tables == nil {
		return nil
	}
	// Pre-build reverse GOTO index: lhsSym → list of GOTO target states.
	// This avoids the O(stateCount) scan per reduce action that made
	// computeLexModes unusable for large grammars (C# 121K states, TS 42K).
	type gotoTarget struct{ targetState int }
	gotoIndex := make(map[int][]gotoTarget) // lhsSym → targets
	for state := 0; state < tables.StateCount; state++ {
		acts, ok := tables.ActionTable[state]
		if !ok {
			continue
		}
		for sym, actions := range acts {
			for _, act := range actions {
				if act.kind == lrShift && sym >= tokenCount {
					// This is a GOTO entry (nonterminal shift)
					gotoIndex[sym] = append(gotoIndex[sym], gotoTarget{act.state})
				}
			}
		}
	}

	// Pre-build terminal sets per state for fast lookup
	stateTerminals := make(map[int][]int) // state → terminal syms
	for state := 0; state < tables.StateCount; state++ {
		acts, ok := tables.ActionTable[state]
		if !ok {
			continue
		}
		var terms []int
		for sym := range acts {
			if sym > 0 && sym < tokenCount {
				terms = append(terms, sym)
			}
		}
		if len(terms) > 0 {
			stateTerminals[state] = terms
		}
	}

	cache := make(map[int][]int)
	return func(state int) []int {
		if cached, ok := cache[state]; ok {
			return cached
		}
		seen := make(map[int]bool)
		acts, ok := tables.ActionTable[state]
		if !ok {
			cache[state] = nil
			return nil
		}
		for _, actions := range acts {
			for _, act := range actions {
				if act.kind != lrReduce {
					continue
				}
				lhsSym := act.lhsSym
				if lhsSym <= 0 {
					continue
				}
				// Use pre-built GOTO index instead of scanning all states
				for _, gt := range gotoIndex[lhsSym] {
					for _, sym := range stateTerminals[gt.targetState] {
						seen[sym] = true
					}
				}
			}
		}
		result := make([]int, 0, len(seen))
		for sym := range seen {
			result = append(result, sym)
		}
		cache[state] = result
		return result
	}
}

// buildMissingRecoveryTokensFunc returns lookahead terminals that should be
// lexable in a state solely so missing-token recovery can see them.
// Example: after Dart's `library`, `;` is not a direct action. It is the real
// lookahead that permits inserting a missing `identifier`, reducing
// `library_name`, and then shifting `;`. If the lex mode excludes `;`, the
// parser only sees EOF and cannot perform the same recovery.
func buildMissingRecoveryTokensFunc(tables *LRTables, tokenCount int, terminalPatterns []TerminalPattern, skipExtras map[int]bool) func(int) []int {
	return buildMissingRecoveryTokensFuncWithContext(context.Background(), tables, tokenCount, terminalPatterns, skipExtras)
}

func buildMissingRecoveryTokensFuncWithContext(ctx context.Context, tables *LRTables, tokenCount int, terminalPatterns []TerminalPattern, skipExtras map[int]bool) func(int) []int {
	if ctx == nil {
		ctx = context.Background()
	}
	if tables == nil {
		return nil
	}

	stateShiftTerminals := make(map[int]map[int]bool)
	for state := 0; state < tables.StateCount; state++ {
		acts, ok := tables.ActionTable[state]
		if !ok {
			continue
		}
		for sym, actions := range acts {
			if sym <= 0 || sym >= tokenCount {
				continue
			}
			for _, act := range actions {
				if act.kind == lrShift && !act.isExtra {
					if stateShiftTerminals[state] == nil {
						stateShiftTerminals[state] = make(map[int]bool)
					}
					stateShiftTerminals[state][sym] = true
					break
				}
			}
		}
	}

	gotoTarget := func(state, sym int) (int, bool) {
		if sym < tokenCount {
			return 0, false
		}
		if gotos, ok := tables.GotoTable[state]; ok {
			if target, ok := gotos[sym]; ok {
				return target, true
			}
		}
		if acts, ok := tables.ActionTable[state]; ok {
			for _, act := range acts[sym] {
				if act.kind == lrShift {
					return act.state, true
				}
			}
		}
		return 0, false
	}

	patternsBySymbol := terminalPatternsBySymbol(terminalPatterns)
	type preemptionKey struct {
		direct    int
		lookahead int
	}
	preemptionCache := make(map[preemptionKey]bool)
	preemptsDirectTerminalShift := func(state, lookahead int) bool {
		if len(patternsBySymbol) == 0 {
			return false
		}
		lookaheadPatterns, ok := patternsBySymbol[lookahead]
		if !ok {
			return false
		}
		for direct := range stateShiftTerminals[state] {
			if direct == lookahead {
				return true
			}
			directPatterns, ok := patternsBySymbol[direct]
			if !ok {
				continue
			}
			key := preemptionKey{direct: direct, lookahead: lookahead}
			preempts, ok := preemptionCache[key]
			if !ok {
				preempts = recoveryLookaheadPreemptsDirectTerminal(ctx, directPatterns, lookaheadPatterns, direct, lookahead)
				preemptionCache[key] = preempts
			}
			if preempts {
				return true
			}
		}
		return false
	}
	// preemptsSkippedTerminalExtra depends only on `lookahead` (patternsBySymbol
	// and skipExtras are fixed for the lifetime of this closure), so its result
	// is identical for every state that queries the same lookahead. Without
	// this cache, addSeen re-invokes preemptsSkippedTerminalExtra (and the NFA
	// + lex DFA rebuild inside recoveryLookaheadPreemptsDirectTerminal it
	// performs per skipExtras entry) once per state per lookahead, which
	// dominates compute_lex_modes on large grammars (e.g. c_sharp: 19564
	// states x up to tokenCount-1 lookaheads). Memoizing by lookahead alone
	// mirrors preemptionCache above and is safe here for the same reason:
	// callers of the returned closure run sequentially within a single
	// buildMissingRecoveryTokensFuncWithContext invocation (computeLexModes
	// iterates states in a plain for-loop, no goroutines), and this map is
	// local to that invocation, so there is no cross-invocation or concurrent
	// access to guard against.
	skipExtraPreemptionCache := make(map[int]bool)
	preemptsSkippedTerminalExtraCached := func(lookahead int) bool {
		if preempts, ok := skipExtraPreemptionCache[lookahead]; ok {
			return preempts
		}
		preempts := preemptsSkippedTerminalExtra(ctx, patternsBySymbol, skipExtras, lookahead)
		skipExtraPreemptionCache[lookahead] = preempts
		return preempts
	}
	addSeen := func(state int, seen map[int]bool, lookahead int) {
		if preemptsDirectTerminalShift(state, lookahead) {
			return
		}
		if preemptsSkippedTerminalExtraCached(lookahead) {
			return
		}
		seen[lookahead] = true
	}

	cache := make(map[int][]int)
	return func(state int) []int {
		if ctx.Err() != nil {
			return nil
		}
		if cached, ok := cache[state]; ok {
			return cached
		}
		acts, ok := tables.ActionTable[state]
		if !ok {
			cache[state] = nil
			return nil
		}

		seen := make(map[int]bool)
		for missingSym, missingActions := range acts {
			if missingSym <= 0 || missingSym >= tokenCount {
				continue
			}
			for _, shift := range missingActions {
				if shift.kind != lrShift || shift.isExtra || shift.state == 0 || shift.state == state {
					continue
				}

				queue := []int{shift.state}
				visited := map[int]bool{shift.state: true}
				steps := 0
				for len(queue) > 0 {
					steps++
					if steps&255 == 0 && ctx.Err() != nil {
						return nil
					}
					top := queue[0]
					queue = queue[1:]
					nextActs, ok := tables.ActionTable[top]
					if !ok {
						continue
					}
					for lookahead, lookaheadActions := range nextActs {
						if lookahead <= 0 || lookahead >= tokenCount || len(lookaheadActions) == 0 {
							continue
						}
						if stateShiftTerminals[top][lookahead] {
							addSeen(state, seen, lookahead)
						}
						for _, reduce := range lookaheadActions {
							if reduce.kind != lrReduce || reduce.lhsSym <= 0 {
								continue
							}
							target, ok := gotoTarget(state, reduce.lhsSym)
							if !ok {
								continue
							}
							if stateShiftTerminals[target][lookahead] {
								addSeen(state, seen, lookahead)
							}
							if !visited[target] {
								visited[target] = true
								queue = append(queue, target)
							}
						}
					}
				}
			}
		}

		result := make([]int, 0, len(seen))
		for sym := range seen {
			result = append(result, sym)
		}
		sort.Ints(result)
		cache[state] = result
		return result
	}
}

func preemptsSkippedTerminalExtra(ctx context.Context, patternsBySymbol map[int][]TerminalPattern, skipExtras map[int]bool, lookahead int) bool {
	if len(patternsBySymbol) == 0 || len(skipExtras) == 0 {
		return false
	}
	lookaheadPatterns, ok := patternsBySymbol[lookahead]
	if !ok {
		return false
	}
	for extra := range skipExtras {
		if extra == lookahead {
			continue
		}
		extraPatterns, ok := patternsBySymbol[extra]
		if !ok {
			continue
		}
		if recoveryLookaheadPreemptsDirectTerminal(ctx, extraPatterns, lookaheadPatterns, extra, lookahead) {
			return true
		}
	}
	return false
}

func terminalPatternsBySymbol(patterns []TerminalPattern) map[int][]TerminalPattern {
	if len(patterns) == 0 {
		return nil
	}
	bySym := make(map[int][]TerminalPattern)
	for _, pat := range patterns {
		bySym[pat.SymbolID] = append(bySym[pat.SymbolID], pat)
	}
	return bySym
}

func recoveryLookaheadPreemptsDirectTerminal(ctx context.Context, directPatterns, lookaheadPatterns []TerminalPattern, direct, lookahead int) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	witnesses := directTerminalLexingWitnesses(directPatterns, direct)
	if len(witnesses) == 0 {
		return false
	}

	patterns := make([]TerminalPattern, 0, len(directPatterns)+len(lookaheadPatterns))
	patterns = append(patterns, directPatterns...)
	patterns = append(patterns, lookaheadPatterns...)
	valid := map[int]bool{direct: true, lookahead: true}
	lexStates, offsets, err := buildLexDFA(ctx, patterns, nil, nil, []lexModeSpec{{
		validSymbols: valid,
	}})
	if err != nil || len(offsets) == 0 {
		return false
	}

	// Bounded generalized preemption check: for each short string that a
	// direct terminal can lex, try the exact string plus a small continuation
	// alphabet. This catches broad recovery tokens like /.+/ stealing "#!..."
	// while allowing same-first-rune non-preempting pairs like "ab" vs "ac".
	for _, witness := range witnesses {
		for _, suffix := range missingRecoveryPreemptionSuffixes() {
			lexer := gotreesitter.NewLexer(lexStates, []byte(witness+suffix))
			tok := lexer.Next(uint32(offsets[0]))
			if int(tok.Symbol) == lookahead && tok.StartByte == 0 {
				return true
			}
		}
	}
	return false
}

func directTerminalLexingWitnesses(patterns []TerminalPattern, sym int) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, pat := range patterns {
		if pat.SymbolID != sym || pat.Rule == nil {
			continue
		}
		if value, ok := stringRuleValue(pat.Rule); ok {
			add(value)
			continue
		}
		for _, witness := range terminalPatternWitnesses(pat) {
			add(witness)
		}
	}
	sort.Strings(out)
	return out
}

func terminalPatternWitnesses(pat TerminalPattern) []string {
	nfa, err := buildCombinedNFA([]TerminalPattern{pat})
	if err != nil || nfa == nil {
		return nil
	}
	type item struct {
		states []int
		text   string
	}
	start := epsilonClosure(nfa, []int{nfa.start})
	queue := []item{{states: start}}
	seen := map[string]bool{intSliceKey(start): true}
	witnessSeen := make(map[string]bool)
	var witnesses []string
	const (
		maxDepth     = 8
		maxWitnesses = 32
		maxSeen      = 4096
	)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, state := range cur.states {
			if nfa.states[state].accept == pat.SymbolID && cur.text != "" && !witnessSeen[cur.text] {
				witnessSeen[cur.text] = true
				witnesses = append(witnesses, cur.text)
				if len(witnesses) >= maxWitnesses {
					sort.Strings(witnesses)
					return witnesses
				}
				break
			}
		}
		if len([]rune(cur.text)) >= maxDepth {
			continue
		}
		for _, tr := range representativeTransitions(nfa, cur.states) {
			for _, r := range representativeRunesForRange(tr.lo, tr.hi) {
				next := epsilonClosure(nfa, []int{tr.nextState})
				nextText := cur.text + string(r)
				key := intSliceKey(next) + ":" + nextText
				if seen[key] {
					continue
				}
				seen[key] = true
				queue = append(queue, item{states: next, text: nextText})
				if len(seen) >= maxSeen {
					sort.Strings(witnesses)
					return witnesses
				}
			}
		}
	}
	sort.Strings(witnesses)
	return witnesses
}

func representativeTransitions(nfa *nfa, states []int) []nfaTransition {
	var out []nfaTransition
	for _, state := range states {
		for _, tr := range nfa.states[state].transitions {
			if tr.epsilon || tr.hi < tr.lo {
				continue
			}
			out = append(out, tr)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].lo != out[j].lo {
			return out[i].lo < out[j].lo
		}
		if out[i].hi != out[j].hi {
			return out[i].hi < out[j].hi
		}
		return out[i].nextState < out[j].nextState
	})
	return out
}

func representativeRunesForRange(lo, hi rune) []rune {
	if hi < lo {
		return nil
	}
	seen := make(map[rune]bool)
	var out []rune
	add := func(r rune) {
		if r >= lo && r <= hi {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	for _, r := range []rune{'\n', '\r', '\t', ' ', '\f', '\v', 'a', 'x', '0', '_', '#', '!', ';'} {
		add(r)
	}
	if lo <= hi && lo >= 0 && lo <= maxSupportedRune {
		add(lo)
	}
	return out
}

func missingRecoveryPreemptionSuffixes() []string {
	return []string{
		"", "x", "a", "0", "_", "#", "!", ";", " ",
		"|", "&", "=", ".", ":", "-", "+", "*", "/", "%", "<", ">",
	}
}

func intSliceKey(values []int) string {
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	var b strings.Builder
	for i, value := range sorted {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d", value)
	}
	return b.String()
}

func useForcedBroadLexFallback() bool {
	return os.Getenv("GTS_GRAMMARGEN_FORCE_BROAD_LEX") == "1"
}

func suppressAfterWhitespaceSymbols(g *Grammar, ng *NormalizedGrammar) map[int]bool {
	if g == nil || g.Name != "elixir" || ng == nil {
		return nil
	}
	out := make(map[int]bool)
	for i, sym := range ng.Symbols {
		if sym.Name == "#{" {
			out[i] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ConflictKind describes the type of LR conflict.
type ConflictKind int

const (
	ShiftReduce ConflictKind = iota
	ReduceReduce
)

// ConflictDiag describes a conflict encountered during LR table construction.
type ConflictDiag struct {
	Kind          ConflictKind
	State         int
	LookaheadSym  int
	Actions       []lrAction // the conflicting actions
	Resolution    string     // how it was resolved (or "GLR" if kept)
	IsMergedState bool       // was this state produced by LALR merging?
	MergeCount    int        // how many merge origins this state has
}

func (d *ConflictDiag) String(ng *NormalizedGrammar) string {
	var b strings.Builder
	symName := func(id int) string {
		if id >= 0 && id < len(ng.Symbols) {
			return ng.Symbols[id].Name
		}
		return fmt.Sprintf("sym_%d", id)
	}
	prodStr := func(prodIdx int) string {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			return fmt.Sprintf("prod_%d", prodIdx)
		}
		p := &ng.Productions[prodIdx]
		var rhs []string
		for _, s := range p.RHS {
			rhs = append(rhs, symName(s))
		}
		return fmt.Sprintf("%s → %s", symName(p.LHS), strings.Join(rhs, " "))
	}

	switch d.Kind {
	case ShiftReduce:
		fmt.Fprintf(&b, "Shift/reduce conflict in state %d on %q:\n",
			d.State, symName(d.LookaheadSym))
		for _, a := range d.Actions {
			switch a.kind {
			case lrShift:
				fmt.Fprintf(&b, "  Shift → state %d (prec %d)\n", a.state, a.prec)
			case lrReduce:
				p := &ng.Productions[a.prodIdx]
				assocStr := ""
				switch p.Assoc {
				case AssocLeft:
					assocStr = ", left-associative"
				case AssocRight:
					assocStr = ", right-associative"
				}
				fmt.Fprintf(&b, "  Reduce: %s (prec %d%s)\n", prodStr(a.prodIdx), p.Prec, assocStr)
			}
		}
	case ReduceReduce:
		fmt.Fprintf(&b, "Reduce/reduce conflict in state %d on %q:\n",
			d.State, symName(d.LookaheadSym))
		for _, a := range d.Actions {
			p := &ng.Productions[a.prodIdx]
			fmt.Fprintf(&b, "  Reduce: %s (prec %d)\n", prodStr(a.prodIdx), p.Prec)
		}
	}
	fmt.Fprintf(&b, "  Resolution: %s", d.Resolution)
	return b.String()
}

// GenerateReport holds the result of grammar generation with diagnostics.
type GenerateReport struct {
	Language        *gotreesitter.Language
	Blob            []byte
	Conflicts       []ConflictDiag
	SplitCandidates []splitCandidate
	SplitResult     *splitReport
	Warnings        []string
	SymbolCount     int
	StateCount      int
	TokenCount      int
}

// resolveConflictsWithDiag is like resolveConflicts but collects diagnostics.
func resolveConflictsWithDiag(ctx context.Context, tables *LRTables, ng *NormalizedGrammar, prov *mergeProvenance) ([]ConflictDiag, conflictResolutionStats, error) {
	return resolveConflictsWithDiagAndTrace(ctx, tables, ng, prov, phaseTrace{})
}

func resolveConflictsWithDiagAndTrace(ctx context.Context, tables *LRTables, ng *NormalizedGrammar, prov *mergeProvenance, trace phaseTrace) ([]ConflictDiag, conflictResolutionStats, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var stats conflictResolutionStats

	endAugment := trace.start("resolve_conflicts_augment", nil)
	augmentStats, err := augmentAdjacentRepeatElementReduceLookaheadsWithTrace(ctx, tables, ng, trace)
	stats.add(augmentStats)
	if err != nil {
		endAugment(stats.augmentTraceFields())
		return nil, stats, err
	}
	endAugment(augmentStats.augmentTraceFields())

	cache := getConflictResolutionCache(ng)
	if cache != nil {
		cache.resetStructuralStats()
	}
	endActions := trace.start("resolve_conflicts_actions", nil)
	actionFields := func() map[string]any {
		current := stats
		current.add(cache.snapshotStructuralStats())
		return current.actionTraceFields()
	}
	finishActions := func() {
		stats.add(cache.snapshotStructuralStats())
		endActions(stats.actionTraceFields())
	}
	if err := checkConflictResolutionContext(ctx, "before diagnostic action resolution"); err != nil {
		finishActions()
		return nil, stats, err
	}

	var diags []ConflictDiag

	// Sort states and syms for deterministic conflict resolution order.
	states := make([]int, 0, len(tables.ActionTable))
	for state := range tables.ActionTable {
		states = append(states, state)
	}
	sort.Ints(states)

	for _, state := range states {
		if err := checkConflictResolutionContext(ctx, "scanning diagnostic states"); err != nil {
			finishActions()
			return diags, stats, err
		}
		stats.StatesScanned++
		actions := tables.ActionTable[state]
		syms := make([]int, 0, len(actions))
		for sym := range actions {
			syms = append(syms, sym)
		}
		sort.Ints(syms)
		for _, sym := range syms {
			stats.ActionEntriesScanned++
			if stats.ActionEntriesScanned&1023 == 0 {
				if trace.enabled {
					trace.log("resolve_conflicts_actions", "progress", 0, actionFields())
				}
				if err := checkConflictResolutionContext(ctx, "scanning diagnostic action entries"); err != nil {
					finishActions()
					return diags, stats, err
				}
			}
			acts := actions[sym]
			if len(acts) <= 1 {
				continue
			}
			stats.ConflictsResolved++
			if len(acts) > stats.MaxActionsPerConflict {
				stats.MaxActionsPerConflict = len(acts)
			}

			diag := ConflictDiag{
				State:        state,
				LookaheadSym: sym,
				Actions:      append([]lrAction{}, acts...),
			}

			if prov != nil {
				diag.IsMergedState = prov.isMerged(state)
				diag.MergeCount = len(prov.origins(state))
			}

			// Classify conflict.
			hasShift, hasReduce := false, false
			for _, a := range acts {
				if a.kind == lrShift {
					hasShift = true
				}
				if a.kind == lrReduce {
					hasReduce = true
				}
			}
			if hasShift && hasReduce {
				diag.Kind = ShiftReduce
			} else {
				diag.Kind = ReduceReduce
			}

			resolved, err := resolveActionConflict(sym, acts, ng)
			if err != nil {
				finishActions()
				return diags, stats, fmt.Errorf("state %d, symbol %d: %w", state, sym, err)
			}
			tables.ActionTable[state][sym] = resolved

			// Determine resolution description.
			switch {
			case len(resolved) > 1:
				diag.Resolution = "GLR (multiple actions kept)"
			case len(resolved) == 1 && resolved[0].kind == lrShift:
				diag.Resolution = "shift wins"
				if hasReduce {
					for _, a := range acts {
						if a.kind == lrReduce {
							p := &ng.Productions[a.prodIdx]
							if p.Prec > 0 || resolved[0].prec > 0 {
								diag.Resolution = fmt.Sprintf("shift wins (prec %d > %d)", resolved[0].prec, p.Prec)
							} else if p.Assoc == AssocRight {
								diag.Resolution = "shift wins (right-associative)"
							} else {
								diag.Resolution = "shift wins (default yacc behavior)"
							}
							break
						}
					}
				}
			case len(resolved) == 1 && resolved[0].kind == lrReduce:
				prod := &ng.Productions[resolved[0].prodIdx]
				if prod.Assoc == AssocLeft {
					diag.Resolution = "reduce wins (left-associative)"
				} else {
					diag.Resolution = fmt.Sprintf("reduce wins (prec %d)", prod.Prec)
				}
			case len(resolved) == 0:
				diag.Resolution = "error (non-associative)"
			}

			diags = append(diags, diag)
		}
	}
	if err := checkConflictResolutionContext(ctx, "after diagnostic action resolution"); err != nil {
		finishActions()
		return diags, stats, err
	}
	finishActions()
	return diags, stats, nil
}

// Validate checks the grammar for common issues and returns warnings.
func Validate(g *Grammar) []string {
	var warnings []string

	if len(g.RuleOrder) == 0 {
		warnings = append(warnings, "grammar has no rules defined")
		return warnings
	}

	// Check for undefined symbol references.
	defined := make(map[string]bool)
	for _, name := range g.RuleOrder {
		defined[name] = true
	}
	// External symbols are also valid references.
	for _, ext := range g.Externals {
		if ext.Kind == RuleSymbol && ext.Value != "" {
			defined[ext.Value] = true
		}
	}
	for _, name := range g.RuleOrder {
		refs := collectSymbolRefs(g.Rules[name])
		for _, ref := range refs {
			if !defined[ref] {
				warnings = append(warnings, fmt.Sprintf("rule %q references undefined symbol %q", name, ref))
			}
		}
	}

	// Check for unreachable rules (not reachable from start symbol).
	reachable := make(map[string]bool)
	var walk func(name string)
	walk = func(name string) {
		if reachable[name] {
			return
		}
		reachable[name] = true
		if rule, ok := g.Rules[name]; ok {
			for _, ref := range collectSymbolRefs(rule) {
				walk(ref)
			}
		}
	}
	walk(g.RuleOrder[0]) // start from start symbol
	// Extras and externals can reference rules too.
	for _, extra := range g.Extras {
		for _, ref := range collectSymbolRefs(extra) {
			walk(ref)
		}
	}
	for _, ext := range g.Externals {
		for _, ref := range collectSymbolRefs(ext) {
			walk(ref)
		}
	}
	for _, name := range g.RuleOrder {
		if !reachable[name] {
			warnings = append(warnings, fmt.Sprintf("rule %q is unreachable from start symbol %q", name, g.RuleOrder[0]))
		}
	}

	// Check for empty choice alternatives.
	for _, name := range g.RuleOrder {
		checkEmptyChoice(g.Rules[name], name, &warnings)
	}

	// Check conflicts reference existing rules.
	for i, group := range g.Conflicts {
		for _, sym := range group {
			if !defined[sym] {
				warnings = append(warnings, fmt.Sprintf("conflict group %d references undefined rule %q", i, sym))
			}
		}
	}

	// Check supertypes reference existing rules.
	for _, st := range g.Supertypes {
		if !defined[st] {
			warnings = append(warnings, fmt.Sprintf("supertype %q is not a defined rule", st))
		}
	}

	// Check word token is defined.
	if g.Word != "" && !defined[g.Word] {
		warnings = append(warnings, fmt.Sprintf("word token %q is not a defined rule", g.Word))
	}

	return warnings
}

// collectSymbolRefs returns all symbol references in a rule tree.
func collectSymbolRefs(r *Rule) []string {
	if r == nil {
		return nil
	}
	var refs []string
	if r.Kind == RuleSymbol {
		refs = append(refs, r.Value)
	}
	for _, child := range r.Children {
		refs = append(refs, collectSymbolRefs(child)...)
	}
	return refs
}

// checkEmptyChoice warns about choice rules with blank alternatives
// that might indicate a mistake (usually Optional should be used instead).
func checkEmptyChoice(r *Rule, ruleName string, warnings *[]string) {
	if r == nil {
		return
	}
	for _, child := range r.Children {
		checkEmptyChoice(child, ruleName, warnings)
	}
}

// RunTests generates the grammar and runs all embedded test cases.
// Returns nil if all tests pass, or an error describing failures.
func RunTests(g *Grammar) error {
	if len(g.Tests) == 0 {
		return nil
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		return fmt.Errorf("generate failed: %w", err)
	}

	var failures []string
	for _, tc := range g.Tests {
		parser := gotreesitter.NewParser(lang)
		tree, err := parser.Parse([]byte(tc.Input))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: parse error: %v", tc.Name, err))
			continue
		}

		sexp := tree.RootNode().SExpr(lang)
		hasError := strings.Contains(sexp, "ERROR")

		if tc.ExpectError {
			if !hasError {
				failures = append(failures, fmt.Sprintf("%s: expected ERROR nodes but got: %s", tc.Name, sexp))
			}
			continue
		}

		if hasError {
			failures = append(failures, fmt.Sprintf("%s: unexpected ERROR in tree: %s", tc.Name, sexp))
			continue
		}

		if tc.Expected != "" && sexp != tc.Expected {
			failures = append(failures, fmt.Sprintf("%s: tree mismatch\n  got:      %s\n  expected: %s", tc.Name, sexp, tc.Expected))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d test(s) failed:\n%s", len(failures), strings.Join(failures, "\n"))
	}
	return nil
}

type reportBuildOptions struct {
	includeDiagnostics bool
	includeLanguage    bool
	includeBlob        bool
}

func generateWithReport(g *Grammar, opts reportBuildOptions) (*GenerateReport, error) {
	return generateWithReportCtx(context.Background(), g, opts)
}

// GenerateWithReport compiles a grammar and returns a full diagnostic report.
func GenerateWithReport(g *Grammar) (*GenerateReport, error) {
	return GenerateWithReportContext(context.Background(), g)
}

// GenerateWithReportContext is like GenerateWithReport but accepts a context
// for cancellation and performs a SINGLE LR generation pass that produces
// both the diagnostic report and the compiled *gotreesitter.Language.
//
// The returned report's Language field already holds the compiled Language
// (same as GenerateLanguage(g) would return) — callers that need both the
// diagnostics (Conflicts, Warnings, SplitCandidates, ...) and the compiled
// Language should read report.Language here instead of calling
// GenerateLanguage separately, which would otherwise trigger a second,
// redundant full LR generation pass (state construction, conflict
// resolution, lex DFA construction — all done twice for no benefit). This
// matters most for heavy grammars (e.g. imported TypeScript/JavaScript
// grammars), where a second pass can double wall-clock generation time and,
// without a cancellable context, cannot be aborted by a caller such as a Web
// Worker enforcing a generation deadline.
//
// When ctx is cancelled, LR table construction, lex DFA construction, and
// conflict resolution abort promptly at their periodic cancellation checks
// and this function returns a non-nil error wrapping ctx.Err() (checkable
// with errors.Is(err, context.Canceled) / errors.Is(err,
// context.DeadlineExceeded)).
func GenerateWithReportContext(ctx context.Context, g *Grammar) (*GenerateReport, error) {
	return generateWithReportCtx(ctx, g, reportBuildOptions{
		includeDiagnostics: true,
		includeLanguage:    true,
		includeBlob:        true,
	})
}

// generateWithReportCtx is like generateWithReport but threads a context
// through LR table construction for cancellation support. When the context
// is cancelled, the LR builder aborts promptly and returns an error.
func generateWithReportCtx(bgCtx context.Context, g *Grammar, opts reportBuildOptions) (*GenerateReport, error) {
	report := &GenerateReport{}
	trace := newPhaseTrace(g)

	endPhase := trace.start("validate_normalize", trace.grammarCounters(g))
	report.Warnings = Validate(g)

	ng, err := Normalize(g)
	if err != nil {
		endPhase(map[string]any{"error": true, "warnings": len(report.Warnings)})
		return nil, fmt.Errorf("normalize: %w", err)
	}
	normalizeCounters := trace.normalizedCounters(ng)
	if trace.enabled {
		normalizeCounters["warnings"] = len(report.Warnings)
	}
	endPhase(normalizeCounters)

	needDiagnostics := opts.includeDiagnostics || g.EnableLRSplitting
	endPhase = trace.start("build_lr", map[string]any{"diagnostics": needDiagnostics})
	tables, lrCtx, err := buildLRTablesInternal(bgCtx, ng, needDiagnostics)
	if err != nil {
		endPhase(map[string]any{"error": true})
		return nil, fmt.Errorf("build LR tables: %w", err)
	}
	endPhase(trace.lrCounters(tables))
	prov := lrCtx.provenance

	endPhase = trace.start("resolve_conflicts", map[string]any{"diagnostics": needDiagnostics})
	if needDiagnostics {
		diags, conflictStats, err := resolveConflictsWithDiagAndTrace(bgCtx, tables, ng, prov, trace)
		if err != nil {
			endFields := conflictStats.traceFields()
			endFields["error"] = true
			endPhase(endFields)
			return nil, fmt.Errorf("resolve conflicts: %w", err)
		}
		if opts.includeDiagnostics {
			report.Conflicts = diags
		}
		resolveFields := trace.lrCounters(tables)
		if trace.enabled {
			resolveFields["conflicts"] = len(diags)
			for key, value := range conflictStats.traceFields() {
				resolveFields[key] = value
			}
		}
		endPhase(resolveFields)

		var splitCandidates []splitCandidate
		endPhase = trace.start("split_candidate_rebuild", map[string]any{"enabled": g.EnableLRSplitting})
		if opts.includeDiagnostics || g.EnableLRSplitting {
			splitCandidates = newSplitOracle(diags, prov, tables, ng).candidates()
			if opts.includeDiagnostics {
				report.SplitCandidates = splitCandidates
			}
		}

		if len(splitCandidates) > 0 && g.EnableLRSplitting {
			glrBefore := 0
			for _, d := range diags {
				if d.Resolution == "GLR (multiple actions kept)" {
					glrBefore++
				}
			}

			extTokenCandidates := 0
			dirBCandidates := 0
			for _, c := range splitCandidates {
				if c.reason == "hidden external token in merged LALR state" {
					extTokenCandidates++
				}
				if c.reason == "heavily merged LALR state with hidden external reduce" {
					dirBCandidates++
				}
			}

			if os.Getenv("GOT_DEBUG_SPLIT") == "1" {
				fmt.Fprintf(os.Stderr, "[LRSPLIT] candidates=%d ext=%d dirB=%d\n",
					len(splitCandidates), extTokenCandidates, dirBCandidates)
			}

			sr := &splitReport{CandidatesFound: len(splitCandidates)}
			sr.ConflictsBefore = len(diags)
			statesBefore := tables.StateCount
			splitCount, splitErr := localLR1Rebuild(tables, ng, lrCtx, splitCandidates, 200)
			sr.StatesSplit = splitCount
			sr.NewStatesAdded = tables.StateCount - statesBefore
			sr.Error = splitErr

			if os.Getenv("GOT_DEBUG_SPLIT") == "1" {
				fmt.Fprintf(os.Stderr, "[LRSPLIT] split=%d new_states=%d err=%v\n",
					splitCount, sr.NewStatesAdded, splitErr)
			}

			diagsAfter, _, _ := resolveConflictsWithDiag(bgCtx, tables, ng, prov)
			sr.ConflictsAfter = len(diagsAfter)

			glrAfter := 0
			for _, d := range diagsAfter {
				if d.Resolution == "GLR (multiple actions kept)" {
					glrAfter++
				}
			}
			sr.GLRBefore = glrBefore
			sr.GLRAfter = glrAfter

			keepSplit := glrAfter < glrBefore || len(diagsAfter) < len(diags) ||
				(extTokenCandidates > 0 && splitCount > 0)

			if !keepSplit {
				tables, err = buildLRTables(ng)
				if err != nil {
					endPhase(map[string]any{"enabled": g.EnableLRSplitting, "error": true})
					return nil, fmt.Errorf("rebuild LR tables after split rollback: %w", err)
				}
				if _, err := resolveConflicts(bgCtx, tables, ng); err != nil {
					endPhase(map[string]any{"enabled": g.EnableLRSplitting, "error": true})
					return nil, fmt.Errorf("resolve conflicts after split rollback: %w", err)
				}
				sr.StatesSplit = 0
				sr.NewStatesAdded = 0
				sr.ConflictsAfter = sr.ConflictsBefore
				sr.Error = fmt.Errorf("rollback: conflicts %d -> %d, GLR conflicts %d -> %d (not reduced)",
					len(diags), len(diagsAfter), glrBefore, glrAfter)
			} else if opts.includeDiagnostics {
				report.Conflicts = diagsAfter
				report.SplitCandidates = newSplitOracle(diagsAfter, prov, tables, ng).candidates()
			}
			if opts.includeDiagnostics {
				report.SplitResult = sr
			}
		}
		endFields := trace.lrCounters(tables)
		if trace.enabled {
			endFields["split_candidates"] = len(splitCandidates)
			endFields["enabled"] = g.EnableLRSplitting
		}
		endPhase(endFields)
	} else {
		conflictStats, err := resolveConflictsWithTrace(bgCtx, tables, ng, trace)
		if err != nil {
			endFields := conflictStats.traceFields()
			endFields["error"] = true
			endPhase(endFields)
			return nil, fmt.Errorf("resolve conflicts: %w", err)
		}
		endFields := trace.lrCounters(tables)
		if trace.enabled {
			for key, value := range conflictStats.traceFields() {
				endFields[key] = value
			}
		}
		endPhase(endFields)
	}

	endPhase = trace.start("add_nonterminal_extra_chains", trace.lrCounters(tables))
	if err := addNonterminalExtraChainsGuarded(tables, ng, lrCtx); err != nil {
		endPhase(map[string]any{"error": true})
		return nil, fmt.Errorf("add nonterminal extra chains: %w", err)
	}
	endFields := trace.lrCounters(tables)
	if trace.enabled {
		endFields["extra_chain_start"] = tables.ExtraChainStateStart
	}
	endPhase(endFields)

	lrCtx.releaseScratch()
	prov = nil
	lrCtx = nil

	report.SymbolCount = len(ng.Symbols)
	report.StateCount = tables.StateCount + 1
	report.TokenCount = ng.TokenCount()

	if !opts.includeLanguage {
		return report, nil
	}

	tokenCount := ng.TokenCount()
	immediateTokens := make(map[int]bool)
	for _, t := range ng.Terminals {
		if t.Immediate {
			immediateTokens[t.SymbolID] = true
		}
	}

	keywordSet := make(map[int]bool, len(ng.KeywordSymbols))
	for _, ks := range ng.KeywordSymbols {
		keywordSet[ks] = true
	}
	stringPrefixExtensions := computeStringPrefixExtensions(ng.Terminals)
	termPatSyms := terminalPatternSymSet(ng)
	patternSyms := patternTerminalSymSet(ng)
	zeroWidthSyms := zeroWidthTerminalSymSet(ng)
	skipExtras := computeSkipExtras(ng)

	var lexModes []lexModeSpec
	var stateToMode []int
	var afterWSModes []afterWSModeEntry
	endPhase = trace.start("compute_lex_modes", map[string]any{"states": tables.StateCount, "tokens": tokenCount})
	if useForcedBroadLexFallback() {
		// Escape hatch only. The broad DFA is much faster to build for huge
		// grammars, but it is not parser-correct for languages that rely on
		// stateful contextual lexing such as C# and COBOL.
		allSyms := make(map[int]bool)
		for _, t := range ng.Terminals {
			allSyms[t.SymbolID] = true
		}
		for _, e := range ng.ExtraSymbols {
			if e > 0 && e < tokenCount {
				allSyms[e] = true
			}
		}
		lexModes = []lexModeSpec{{validSymbols: allSyms, skipWhitespace: true}}
		stateToMode = make([]int, tables.StateCount)
	} else {
		lexModes, stateToMode, afterWSModes, err = computeLexModesWithContext(bgCtx,
			tables.StateCount,
			tokenCount,
			func(state, sym int) bool {
				if acts, ok := tables.ActionTable[state]; ok {
					if entry, ok := acts[sym]; ok && len(entry) > 0 {
						return true
					}
				}
				return false
			},
			stringPrefixExtensions,
			ng.ExtraSymbols,
			tables.ExtraChainStateStart,
			immediateTokens,
			ng.ExternalSymbols,
			ng.WordSymbolID,
			keywordSet,
			termPatSyms,
			buildLexModeFollowTokensFunc(tables, tokenCount, ng, keywordSet),
			buildLexModeMissingRecoveryTokensFunc(bgCtx, tables, tokenCount, ng.Terminals, skipExtras, ng, keywordSet),
			suppressAfterWhitespaceSymbols(g, ng),
			patternSyms,
			zeroWidthSyms,
		)
		if err != nil {
			endPhase(map[string]any{"error": true})
			return nil, fmt.Errorf("compute lex modes: %w", err)
		}
	}
	endPhase(map[string]any{
		"lex_modes":       len(lexModes),
		"state_modes":     len(stateToMode),
		"after_ws_modes":  len(afterWSModes),
		"forced_fallback": useForcedBroadLexFallback(),
	})

	endPhase = trace.start("build_lex_dfa", map[string]any{"lex_modes": len(lexModes), "terminals": len(ng.Terminals), "extras": len(ng.ExtraSymbols)})
	lexStates, lexModeOffsets, err := buildLexDFA(bgCtx, ng.Terminals, ng.ExtraSymbols, skipExtras, lexModes)
	if err != nil {
		endPhase(map[string]any{"error": true})
		return nil, fmt.Errorf("build lex DFA: %w", err)
	}
	endPhase(map[string]any{"lex_states": len(lexStates), "lex_modes": len(lexModeOffsets)})

	var keywordLexStates []gotreesitter.LexState
	endPhase = trace.start("build_keyword_dfa", map[string]any{"keywords": len(ng.KeywordEntries)})
	if len(ng.KeywordEntries) > 0 {
		kls, _, err := buildLexDFA(bgCtx, ng.KeywordEntries, nil, nil, []lexModeSpec{{
			validSymbols:   allSymbolsSet(ng.KeywordEntries),
			skipWhitespace: false,
		}})
		if err != nil {
			endPhase(map[string]any{"error": true})
			return nil, fmt.Errorf("build keyword DFA: %w", err)
		}
		keywordLexStates = kls
	}
	endPhase(map[string]any{"keyword_lex_states": len(keywordLexStates)})

	endPhase = trace.start("assemble", map[string]any{"states": tables.StateCount, "lex_states": len(lexStates)})
	lang, err := assemble(ng, tables, lexStates, stateToMode, lexModeOffsets, afterWSModes)
	if err != nil {
		endPhase(map[string]any{"error": true})
		return nil, fmt.Errorf("assemble: %w", err)
	}
	lang.Name = g.Name
	lang.WantsForest = g.WantsForest

	repairGeneratedCompatibilitySymbols(lang)

	if len(keywordLexStates) > 0 {
		lang.KeywordLexStates = keywordLexStates
		lang.KeywordCaptureToken = gotreesitter.Symbol(ng.WordSymbolID)
	}
	endPhase(map[string]any{
		"symbols":       lang.SymbolCount,
		"states":        lang.StateCount,
		"tokens":        lang.TokenCount,
		"parse_actions": len(lang.ParseActions),
		"lex_states":    len(lang.LexStates),
	})

	report.Language = lang
	report.SymbolCount = int(lang.SymbolCount)
	report.StateCount = int(lang.StateCount)
	report.TokenCount = int(lang.TokenCount)

	if !opts.includeBlob {
		return report, nil
	}

	endPhase = trace.start("encode_blob", map[string]any{
		"symbols": lang.SymbolCount,
		"states":  lang.StateCount,
		"tokens":  lang.TokenCount,
	})
	blob, err := encodeLanguageBlob(lang)
	if err != nil {
		endPhase(map[string]any{"error": true})
		return nil, fmt.Errorf("encode: %w", err)
	}
	report.Blob = blob
	endPhase(map[string]any{"blob_bytes": len(blob)})

	return report, nil
}

// generateDiagnosticsReport runs the report pipeline but skips lex/assemble/blob
// work. It is intended for large-grammar diagnostic/perf tests that only need
// conflicts, split metadata, warnings, and final table counts.
func generateDiagnosticsReport(g *Grammar) (*GenerateReport, error) {
	return generateWithReport(g, reportBuildOptions{includeDiagnostics: true})
}

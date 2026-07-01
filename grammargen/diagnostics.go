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

func useForcedBroadLexFallback() bool {
	return os.Getenv("GTS_GRAMMARGEN_FORCE_BROAD_LEX") == "1"
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
func resolveConflictsWithDiag(tables *LRTables, ng *NormalizedGrammar, prov *mergeProvenance) ([]ConflictDiag, error) {
	var diags []ConflictDiag

	// Sort states and syms for deterministic conflict resolution order.
	states := make([]int, 0, len(tables.ActionTable))
	for state := range tables.ActionTable {
		states = append(states, state)
	}
	sort.Ints(states)

	for _, state := range states {
		actions := tables.ActionTable[state]
		syms := make([]int, 0, len(actions))
		for sym := range actions {
			syms = append(syms, sym)
		}
		sort.Ints(syms)
		for _, sym := range syms {
			acts := actions[sym]
			if len(acts) <= 1 {
				continue
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
				return diags, fmt.Errorf("state %d, symbol %d: %w", state, sym, err)
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
	return diags, nil
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
	return generateWithReport(g, reportBuildOptions{
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

	report.Warnings = Validate(g)

	ng, err := Normalize(g)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	needDiagnostics := opts.includeDiagnostics || g.EnableLRSplitting
	tables, lrCtx, err := buildLRTablesInternal(bgCtx, ng, needDiagnostics)
	if err != nil {
		return nil, fmt.Errorf("build LR tables: %w", err)
	}
	prov := lrCtx.provenance

	if needDiagnostics {
		diags, err := resolveConflictsWithDiag(tables, ng, prov)
		if err != nil {
			return nil, fmt.Errorf("resolve conflicts: %w", err)
		}
		if opts.includeDiagnostics {
			report.Conflicts = diags
		}

		var splitCandidates []splitCandidate
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

			diagsAfter, _ := resolveConflictsWithDiag(tables, ng, prov)
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
					return nil, fmt.Errorf("rebuild LR tables after split rollback: %w", err)
				}
				if err := resolveConflicts(tables, ng); err != nil {
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
	} else {
		if err := resolveConflicts(tables, ng); err != nil {
			return nil, fmt.Errorf("resolve conflicts: %w", err)
		}
	}

	addNonterminalExtraChains(tables, ng, lrCtx)

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

	var lexModes []lexModeSpec
	var stateToMode []int
	var afterWSModes []afterWSModeEntry
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
		lexModes, stateToMode, afterWSModes = computeLexModes(
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
			buildFollowTokensFunc(tables, tokenCount),
			patternImmediateTokenSet(ng),
		)
	}

	skipExtras := computeSkipExtras(ng)
	lexStates, lexModeOffsets, err := buildLexDFA(bgCtx, ng.Terminals, ng.ExtraSymbols, skipExtras, lexModes)
	if err != nil {
		return nil, fmt.Errorf("build lex DFA: %w", err)
	}

	var keywordLexStates []gotreesitter.LexState
	if len(ng.KeywordEntries) > 0 {
		kls, _, err := buildLexDFA(bgCtx, ng.KeywordEntries, nil, nil, []lexModeSpec{{
			validSymbols:   allSymbolsSet(ng.KeywordEntries),
			skipWhitespace: false,
		}})
		if err != nil {
			return nil, fmt.Errorf("build keyword DFA: %w", err)
		}
		keywordLexStates = kls
	}

	lang, err := assemble(ng, tables, lexStates, stateToMode, lexModeOffsets)
	if err != nil {
		return nil, fmt.Errorf("assemble: %w", err)
	}
	lang.Name = g.Name
	lang.WantsForest = g.WantsForest

	// Set after-whitespace lex states for states that need IMMTOKEN exclusion.
	for _, entry := range afterWSModes {
		if entry.stateIdx < len(lang.LexModes) && entry.modeIdx < len(lexModeOffsets) {
			lang.LexModes[entry.stateIdx].SetAfterWhitespaceLexStateIndex(uint32(lexModeOffsets[entry.modeIdx]))
		}
	}

	if len(keywordLexStates) > 0 {
		lang.KeywordLexStates = keywordLexStates
		lang.KeywordCaptureToken = gotreesitter.Symbol(ng.WordSymbolID)
	}

	report.Language = lang
	report.SymbolCount = int(lang.SymbolCount)
	report.StateCount = int(lang.StateCount)
	report.TokenCount = int(lang.TokenCount)

	if !opts.includeBlob {
		return report, nil
	}

	blob, err := encodeLanguageBlob(lang)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	report.Blob = blob

	return report, nil
}

// generateDiagnosticsReport runs the report pipeline but skips lex/assemble/blob
// work. It is intended for large-grammar diagnostic/perf tests that only need
// conflicts, split metadata, warnings, and final table counts.
func generateDiagnosticsReport(g *Grammar) (*GenerateReport, error) {
	return generateWithReport(g, reportBuildOptions{includeDiagnostics: true})
}

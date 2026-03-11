package grammargen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

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
	includeLanguage bool
	includeBlob     bool
}

func generateWithReport(g *Grammar, opts reportBuildOptions) (*GenerateReport, error) {
	report := &GenerateReport{}

	// Validate first.
	report.Warnings = Validate(g)

	ng, err := Normalize(g)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		return nil, fmt.Errorf("build LR tables: %w", err)
	}
	prov := ctx.provenance

	// Resolve conflicts with diagnostics.
	diags, err := resolveConflictsWithDiag(tables, ng, prov)
	if err != nil {
		return nil, fmt.Errorf("resolve conflicts: %w", err)
	}
	report.Conflicts = diags

	// Run split oracle.
	oracle := newSplitOracle(diags, prov)
	report.SplitCandidates = oracle.candidates()

	// Apply local LR(1) rebuild only for grammars with declared conflict groups.
	// Grammars without declared conflicts are supposed to be unambiguous under
	// LALR; splitting them toward LR(1) diverges from tree-sitter's LALR tables.
	// The rollback guard further reverts if splitting does not reduce GLR conflicts.
	if len(report.SplitCandidates) > 0 && (len(ng.Conflicts) > 0 || g.EnableLRSplitting) {
		// Count pre-split GLR conflicts.
		glrBefore := 0
		for _, d := range diags {
			if d.Resolution == "GLR (multiple actions kept)" {
				glrBefore++
			}
		}

		sr := &splitReport{CandidatesFound: len(report.SplitCandidates)}
		sr.ConflictsBefore = len(diags)
		statesBefore := tables.StateCount
		splitCount, splitErr := localLR1Rebuild(tables, ng, ctx, report.SplitCandidates, 200)
		sr.StatesSplit = splitCount
		sr.NewStatesAdded = tables.StateCount - statesBefore
		sr.Error = splitErr

		// Re-resolve conflicts after splitting.
		diagsAfter, _ := resolveConflictsWithDiag(tables, ng, prov)
		sr.ConflictsAfter = len(diagsAfter)

		// Count post-split GLR conflicts.
		glrAfter := 0
		for _, d := range diagsAfter {
			if d.Resolution == "GLR (multiple actions kept)" {
				glrAfter++
			}
		}
		sr.GLRBefore = glrBefore
		sr.GLRAfter = glrAfter

		if glrAfter >= glrBefore {
			// Global rollback: splitting didn't reduce GLR conflicts.
			// Rebuild the original resolved tables instead of retaining a
			// whole-table snapshot on the success path.
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
			sr.Error = fmt.Errorf("rollback: GLR conflicts %d → %d (not reduced)", glrBefore, glrAfter)
		} else {
			report.Conflicts = diagsAfter
			// Re-run oracle on new conflicts.
			oracleAfter := newSplitOracle(diagsAfter, prov)
			report.SplitCandidates = oracleAfter.candidates()
		}
		report.SplitResult = sr
	}

	// Add nonterminal extra parse chains (must be after conflict resolution
	// and optional splitting, since both modify the tables).
	addNonterminalExtraChains(tables, ng, ctx)

	// The LR construction context is only needed through conflict diagnostics,
	// optional split evaluation, and extra-chain synthesis. Drop its scratch
	// data before building lex tables and encoding so those phases do not
	// overlap with the full LR build heap for large grammars.
	ctx.releaseScratch()
	prov = nil
	ctx = nil

	report.SymbolCount = len(ng.Symbols)
	report.StateCount = tables.StateCount + 1 // buildParseTables inserts error state 0
	report.TokenCount = ng.TokenCount()

	if !opts.includeLanguage {
		return report, nil
	}

	// Build lex DFA.
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
	lexModes, stateToMode := computeLexModes(
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
		ng.ExtraSymbols,
		immediateTokens,
		ng.ExternalSymbols,
		ng.WordSymbolID,
		keywordSet,
	)

	skipExtras := computeSkipExtras(ng)
	lexStates, lexModeOffsets, err := buildLexDFA(ng.Terminals, ng.ExtraSymbols, skipExtras, lexModes)
	if err != nil {
		return nil, fmt.Errorf("build lex DFA: %w", err)
	}

	var keywordLexStates []gotreesitter.LexState
	if len(ng.KeywordEntries) > 0 {
		kls, _, err := buildLexDFA(ng.KeywordEntries, nil, nil, []lexModeSpec{{
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

// GenerateWithReport compiles a grammar and returns a full diagnostic report.
func GenerateWithReport(g *Grammar) (*GenerateReport, error) {
	return generateWithReport(g, reportBuildOptions{
		includeLanguage: true,
		includeBlob:     true,
	})
}

// generateDiagnosticsReport runs the report pipeline but skips lex/assemble/blob
// work. It is intended for large-grammar diagnostic/perf tests that only need
// conflicts, split metadata, warnings, and final table counts.
func generateDiagnosticsReport(g *Grammar) (*GenerateReport, error) {
	return generateWithReport(g, reportBuildOptions{})
}

package grammargen

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestDiagYAMLImplicitMapping(t *testing.T) {
	if os.Getenv("DIAG_YAML_IMPLICIT") != "1" {
		t.Skip("set DIAG_YAML_IMPLICIT=1 to run YAML implicit mapping diagnostics")
	}

	var pg importParityGrammar
	for _, g := range importParityGrammars {
		if g.name == "yaml" {
			pg = g
			break
		}
	}
	if pg.name == "" {
		t.Fatal("yaml grammar not found")
	}

	gram, err := importParityGrammarSource(pg)
	if err != nil {
		t.Fatalf("import yaml grammar: %v", err)
	}
	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize yaml grammar: %v", err)
	}
	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build lr tables: %v", err)
	}

	report, err := GenerateWithReport(gram)
	if err != nil {
		t.Fatalf("GenerateWithReport: %v", err)
	}
	genLang := report.Language
	refLang := pg.blobFunc()
	adaptExternalScanner(refLang, genLang)

	src := []byte("A null: null\n")
	genTree, err := gotreesitter.NewParser(genLang).Parse(src)
	if err != nil {
		t.Fatalf("gen parse: %v", err)
	}
	refTree, err := gotreesitter.NewParser(refLang).Parse(src)
	if err != nil {
		t.Fatalf("ref parse: %v", err)
	}
	t.Logf("gen-root: error=%v range=[%d:%d] sexpr=%s",
		genTree.RootNode().HasError(),
		genTree.RootNode().StartByte(), genTree.RootNode().EndByte(),
		genTree.RootNode().SExpr(genLang))
	t.Logf("gen-runtime: stop=%s truncated=%v max_stacks=%d token_eof_early=%v",
		genTree.ParseRuntime().StopReason,
		genTree.ParseRuntime().Truncated,
		genTree.ParseRuntime().MaxStacksSeen,
		genTree.ParseRuntime().TokenSourceEOFEarly)
	t.Logf("ref-root: error=%v range=[%d:%d] sexpr=%s",
		refTree.RootNode().HasError(),
		refTree.RootNode().StartByte(), refTree.RootNode().EndByte(),
		refTree.RootNode().SExpr(refLang))
	t.Logf("ref-runtime: stop=%s truncated=%v max_stacks=%d token_eof_early=%v",
		refTree.ParseRuntime().StopReason,
		refTree.ParseRuntime().Truncated,
		refTree.ParseRuntime().MaxStacksSeen,
		refTree.ParseRuntime().TokenSourceEOFEarly)

	colonSyms := diagFindAllSymbols(ng, ":")
	if len(colonSyms) == 0 {
		t.Fatal("no ':' symbols found")
	}
	interestingNames := []string{
		"block_mapping_pair",
		"_blk_imp_itm_tal",
		"flow_node",
		"plain_scalar",
		"string_scalar",
	}
	for _, colonSym := range colonSyms {
		sym := ng.Symbols[colonSym]
		t.Logf("colon symbol=%d kind=%d visible=%v named=%v immediate=%v",
			colonSym, sym.Kind, sym.Visible, sym.Named, sym.Immediate)
	}
	for _, name := range interestingNames {
		ids := diagFindAllSymbols(ng, name)
		t.Logf("symbol %q ids=%v", name, ids)
	}

	t.Log("--- Interesting productions ---")
	for i, prod := range ng.Productions {
		if diagProductionMentionsNames(ng, &prod, interestingNames) {
			t.Logf("prod[%d] %s", i, diagFormatProd(ng, i, -1))
		}
	}

	t.Log("--- States with ':' actions touching implicit mapping ---")
	for _, colonSym := range colonSyms {
		t.Logf("lookahead ':' symbol=%d", colonSym)
		for state := 0; state < len(ctx.itemSets); state++ {
			acts := tables.ActionTable[state][colonSym]
			if len(acts) == 0 {
				continue
			}
			if !diagStateMentionsNames(ng, &ctx.itemSets[state], interestingNames) {
				continue
			}

			t.Logf("state=%d merged=%v merges=%d actions=%s",
				state,
				ctx.provenance != nil && ctx.provenance.isMerged(state),
				diagMergeCount(ctx, state),
				diagFormatActions(ng, acts))
			resolved, err := resolveActionConflict(colonSym, slices.Clone(acts), ng)
			if err != nil {
				t.Fatalf("resolve state=%d ':' actions for sym=%d: %v", state, colonSym, err)
			}
			t.Logf("  resolved=%s", diagFormatActions(ng, resolved))
			for _, ce := range ctx.itemSets[state].cores {
				prod := &ng.Productions[ce.prodIdx]
				if !diagProductionMentionsNames(ng, prod, interestingNames) {
					continue
				}
				la := ""
				if ce.lookaheads.contains(colonSym) {
					la = " LA(:)"
				}
				t.Logf("  item%s %s", la, diagFormatProd(ng, ce.prodIdx, ce.dot))
			}
			for _, act := range acts {
				if act.kind != lrShift {
					continue
				}
				target := act.state
				if target < 0 || target >= len(ctx.itemSets) {
					continue
				}
				if !diagStateMentionsNames(ng, &ctx.itemSets[target], interestingNames) {
					continue
				}
				t.Logf("  shift-target=%d", target)
				for _, ce := range ctx.itemSets[target].cores {
					prod := &ng.Productions[ce.prodIdx]
					if diagProductionMentionsNames(ng, prod, interestingNames) {
						t.Logf("    %s", diagFormatProd(ng, ce.prodIdx, ce.dot))
					}
				}
			}
		}
	}

	t.Log("--- Conflicts on ':' touching implicit mapping ---")
	tablesForDiag, provCtx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build lr tables for diag: %v", err)
	}
	diags, err := resolveConflictsWithDiag(tablesForDiag, ng, provCtx.provenance)
	if err != nil {
		t.Fatalf("resolve conflicts with diag: %v", err)
	}
	for _, d := range diags {
		if !slices.Contains(colonSyms, d.LookaheadSym) {
			continue
		}
		if !diagStateMentionsNames(ng, &provCtx.itemSets[d.State], interestingNames) {
			continue
		}
		t.Logf("conflict state=%d kind=%v resolution=%s actions=%s",
			d.State, d.Kind, d.Resolution, diagFormatActions(ng, d.Actions))
	}
}

func diagFindAllSymbols(ng *NormalizedGrammar, name string) []int {
	var ids []int
	for i, sym := range ng.Symbols {
		if sym.Name == name {
			ids = append(ids, i)
		}
	}
	return ids
}

func diagProductionMentionsNames(ng *NormalizedGrammar, prod *Production, names []string) bool {
	nameSet := make(map[string]bool, len(names))
	for _, name := range names {
		nameSet[name] = true
	}
	if prod.LHS >= 0 && prod.LHS < len(ng.Symbols) && nameSet[ng.Symbols[prod.LHS].Name] {
		return true
	}
	for _, sym := range prod.RHS {
		if sym >= 0 && sym < len(ng.Symbols) && nameSet[ng.Symbols[sym].Name] {
			return true
		}
	}
	return false
}

func diagStateMentionsNames(ng *NormalizedGrammar, set *lrItemSet, names []string) bool {
	for _, ce := range set.cores {
		if diagProductionMentionsNames(ng, &ng.Productions[ce.prodIdx], names) {
			return true
		}
	}
	return false
}

func diagFormatProd(ng *NormalizedGrammar, prodIdx, dot int) string {
	prod := &ng.Productions[prodIdx]
	var rhs []string
	for i, sym := range prod.RHS {
		if i == dot {
			rhs = append(rhs, "•")
		}
		rhs = append(rhs, diagSymbolName(ng, sym))
	}
	if dot == len(prod.RHS) {
		rhs = append(rhs, "•")
	}
	return fmt.Sprintf("%s -> %s", diagSymbolName(ng, prod.LHS), strings.Join(rhs, " "))
}

func diagFormatActions(ng *NormalizedGrammar, acts []lrAction) string {
	if len(acts) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(acts))
	for _, act := range acts {
		switch act.kind {
		case lrShift:
			parts = append(parts, fmt.Sprintf("shift(state=%d,lhs=%s)", act.state, diagSymbolName(ng, act.lhsSym)))
		case lrReduce:
			parts = append(parts, fmt.Sprintf("reduce(prod=%d,%s)", act.prodIdx, diagFormatProd(ng, act.prodIdx, len(ng.Productions[act.prodIdx].RHS))))
		case lrAccept:
			parts = append(parts, "accept")
		default:
			parts = append(parts, fmt.Sprintf("kind=%d", act.kind))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func diagSymbolName(ng *NormalizedGrammar, sym int) string {
	if sym < 0 || sym >= len(ng.Symbols) {
		return fmt.Sprintf("sym%d", sym)
	}
	return fmt.Sprintf("%s(%d)", ng.Symbols[sym].Name, sym)
}

func diagMergeCount(ctx *lrContext, state int) int {
	if ctx == nil || ctx.provenance == nil {
		return 0
	}
	return len(ctx.provenance.origins(state))
}

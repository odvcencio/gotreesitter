package grammargen

import (
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestDiagYAMLRootEOFReduction(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic test")
	}
	if getenvOr("DIAG_YAML_ROOT_EOF", "") != "1" {
		t.Skip("set DIAG_YAML_ROOT_EOF=1 to run YAML EOF/root diagnostics")
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
	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()

	t.Logf("gen-root: sym=%d type=%q err=%v cc=%d stop=%s",
		genRoot.Symbol(), genRoot.Type(genLang), genRoot.HasError(), genRoot.ChildCount(), genTree.ParseRuntime().StopReason)
	for i, child := range genRoot.Children() {
		t.Logf("  gen-child[%d]: sym=%d type=%q err=%v cc=%d span=[%d:%d]",
			i, child.Symbol(), child.Type(genLang), child.HasError(), child.ChildCount(), child.StartByte(), child.EndByte())
	}
	t.Logf("ref-root: sym=%d type=%q err=%v cc=%d stop=%s",
		refRoot.Symbol(), refRoot.Type(refLang), refRoot.HasError(), refRoot.ChildCount(), refTree.ParseRuntime().StopReason)
	diagLogFirstNamedPath(t, "ref", refRoot, refLang)

	if genRoot.ChildCount() == 0 {
		t.Fatal("generated root has no children")
	}
	pairSym := int(genRoot.Child(0).Symbol())
	blSym := -1
	if genRoot.ChildCount() > 1 {
		blSym = int(genRoot.Child(1).Symbol())
	}
	t.Logf("pairSym=%d name=%s blSym=%d name=%s",
		pairSym, diagSymbolName(ng, pairSym), blSym, diagSymbolName(ng, blSym))

	wrapperNames := []string{"stream", "document", "_imp_doc", "_doc_wo_bgn_w_end", "_doc_w_bgn_w_end", "block_node", "block_mapping", "block_mapping_pair", "_bl"}
	t.Log("--- Wrapper productions using pair/_bl ---")
	for i, prod := range ng.Productions {
		if prod.LHS == pairSym || prod.LHS == blSym || containsSym(prod.RHS, pairSym) || containsSym(prod.RHS, blSym) {
			if diagProductionMentionsNames(ng, &prod, wrapperNames) {
				t.Logf("prod[%d] %s", i, diagFormatProd(ng, i, -1))
			}
		}
	}

	t.Log("--- Wrapper productions by name ---")
	for i, prod := range ng.Productions {
		if diagProductionMentionsNames(ng, &prod, wrapperNames) {
			t.Logf("prod[%d] %s", i, diagFormatProd(ng, i, -1))
		}
	}

	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build lr tables: %v", err)
	}
	prevDisableMerge, hadDisableMerge := os.LookupEnv("GOT_LR_DISABLE_STATE_MERGE")
	if err := os.Setenv("GOT_LR_DISABLE_STATE_MERGE", "1"); err != nil {
		t.Fatalf("enable no-merge diag build: %v", err)
	}
	noMergeTables, noMergeCtx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatalf("build no-merge lr tables: %v", err)
	}
	if hadDisableMerge {
		if err := os.Setenv("GOT_LR_DISABLE_STATE_MERGE", prevDisableMerge); err != nil {
			t.Fatalf("restore merge env: %v", err)
		}
	} else if err := os.Unsetenv("GOT_LR_DISABLE_STATE_MERGE"); err != nil {
		t.Fatalf("restore merge env: %v", err)
	}
	t.Log("--- EOF actions in wrapper states ---")
	for state := 0; state < len(ctx.itemSets); state++ {
		acts := tables.ActionTable[state][0]
		if len(acts) == 0 {
			continue
		}
		if !diagStateMentionsNames(ng, &ctx.itemSets[state], wrapperNames) &&
			!diagStateMentionsSymbols(ng, &ctx.itemSets[state], []int{pairSym, blSym}) {
			continue
		}
		t.Logf("state=%d eof-actions=%s", state, diagFormatActions(ng, acts))
		for _, ce := range ctx.itemSets[state].cores {
			prod := &ng.Productions[ce.prodIdx]
			if diagProductionMentionsNames(ng, prod, wrapperNames) || containsSym(prod.RHS, pairSym) || prod.LHS == pairSym {
				la := ""
				if ce.lookaheads.contains(0) {
					la = " LA($)"
				}
				t.Logf("  item%s %s", la, diagFormatProd(ng, ce.prodIdx, ce.dot))
			}
		}
	}

	blockMappingSym := diagFindAllSymbols(ng, "block_mapping")
	blockNodeSym := diagFindAllSymbols(ng, "block_node")
	impDocSym := diagFindAllSymbols(ng, "_imp_doc")
	rBlkMapItemSym := diagFindAllSymbols(ng, "_r_blk_map_itm")
	targetProdIDs := []int{
		diagFindUnaryProduction(ng, rBlkMapItemSym, []int{pairSym}),
		diagFindBinaryProduction(ng, blockMappingSym, rBlkMapItemSym, []int{blSym}),
		diagFindBinaryProduction(ng, blockMappingSym, []int{pairSym}, []int{blSym}),
		diagFindUnaryProduction(ng, blockNodeSym, blockMappingSym),
		diagFindUnaryProduction(ng, impDocSym, blockNodeSym),
	}
	t.Log("--- Completed wrapper item lookaheads ---")
	for _, prodIdx := range targetProdIDs {
		if prodIdx < 0 {
			continue
		}
		t.Logf("prod[%d] %s", prodIdx, diagFormatProd(ng, prodIdx, -1))
		diagLogProdDots(t, "merged", ng, ctx, prodIdx)
		diagLogProdDots(t, "no-merge", ng, noMergeCtx, prodIdx)
	}

	_ = noMergeTables
}

func getenvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func containsSym(syms []int, want int) bool {
	for _, sym := range syms {
		if sym == want {
			return true
		}
	}
	return false
}

func diagStateMentionsSymbols(ng *NormalizedGrammar, set *lrItemSet, syms []int) bool {
	for _, ce := range set.cores {
		prod := &ng.Productions[ce.prodIdx]
		if containsSym(syms, prod.LHS) {
			return true
		}
		for _, rhs := range prod.RHS {
			if containsSym(syms, rhs) {
				return true
			}
		}
	}
	return false
}

func diagFindUnaryProduction(ng *NormalizedGrammar, lhsChoices []int, rhsChoices []int) int {
	for i, prod := range ng.Productions {
		if !containsSym(lhsChoices, prod.LHS) {
			continue
		}
		if len(prod.RHS) != 1 {
			continue
		}
		if containsSym(rhsChoices, prod.RHS[0]) {
			return i
		}
	}
	return -1
}

func diagFindBinaryProduction(ng *NormalizedGrammar, lhsChoices, rhs0Choices, rhs1Choices []int) int {
	for i, prod := range ng.Productions {
		if !containsSym(lhsChoices, prod.LHS) {
			continue
		}
		if len(prod.RHS) != 2 {
			continue
		}
		if containsSym(rhs0Choices, prod.RHS[0]) && containsSym(rhs1Choices, prod.RHS[1]) {
			return i
		}
	}
	return -1
}

func diagFormatLookaheads(ng *NormalizedGrammar, set *bitset) string {
	var names []string
	set.forEach(func(sym int) {
		names = append(names, diagSymbolName(ng, sym))
	})
	return "[" + strings.Join(names, ", ") + "]"
}

func diagLogFirstNamedPath(t *testing.T, label string, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	cur := root
	depth := 0
	for cur != nil && depth < 8 {
		t.Logf("  %s-path[%d]: sym=%d type=%q err=%v cc=%d",
			label, depth, cur.Symbol(), cur.Type(lang), cur.HasError(), cur.ChildCount())
		var next *gotreesitter.Node
		for _, child := range cur.Children() {
			if child.IsNamed() {
				next = child
				break
			}
		}
		cur = next
		depth++
	}
}

func diagLogProdDots(t *testing.T, label string, ng *NormalizedGrammar, ctx *lrContext, prodIdx int) {
	t.Helper()
	if ctx == nil || prodIdx < 0 || prodIdx >= len(ng.Productions) {
		return
	}
	prod := &ng.Productions[prodIdx]
	for dot := 0; dot <= len(prod.RHS); dot++ {
		found := false
		for state := 0; state < len(ctx.itemSets); state++ {
			itemSet := &ctx.itemSets[state]
			idx, ok := itemSet.coreLookup(prodIdx, dot)
			if !ok {
				continue
			}
			found = true
			merged := false
			mergeCount := 0
			if ctx.provenance != nil {
				merged = ctx.provenance.isMerged(state)
				mergeCount = len(ctx.provenance.origins(state))
			}
			t.Logf("  %s dot=%d state=%d merged=%v mergeCount=%d lookaheads=%s",
				label, dot, state, merged, mergeCount, diagFormatLookaheads(ng, &itemSet.cores[idx].lookaheads))
		}
		if !found {
			t.Logf("  %s dot=%d no items found", label, dot)
		}
	}
}

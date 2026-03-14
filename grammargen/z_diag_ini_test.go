package grammargen

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestDiagGrammar(t *testing.T) {
	if os.Getenv("DIAG_GRAMMAR") == "" {
		t.Skip("set DIAG_GRAMMAR to run grammar diagnostics")
	}
	grammarName := "ini"
	if v := os.Getenv("DIAG_GRAMMAR"); v != "" {
		grammarName = v
	}

	var g importParityGrammar
	for _, pg := range importParityGrammars {
		if pg.name == grammarName {
			g = pg
			break
		}
	}
	if g.name == "" {
		t.Fatalf("%s not found in importParityGrammars", grammarName)
	}

	gram, err := importParityGrammarSource(g)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	report, err := GenerateWithReport(gram)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	genLang := report.Language
	t.Logf("GenerateReport: states=%d symbols=%d tokens=%d conflicts=%d warnings=%v",
		report.StateCount, report.SymbolCount, report.TokenCount, len(report.Conflicts), report.Warnings)
	t.Logf("  conflicts: %d", len(report.Conflicts))
	refLang := g.blobFunc()
	adaptExternalScanner(refLang, genLang)

	// Collect corpus samples.
	root := "/tmp/grammar_parity"
	repoRoot := parityGrammarRepoRoot(g, root)
	if repoRoot == "" {
		t.Skipf("no repo for %s in %s", grammarName, root)
	}
	cfg := realCorpusCollectConfig{
		TargetEligible:      10,
		MaxSampleBytes:      65536,
		CandidateMultiplier: 6,
		Profile:             realCorpusProfileSmoke,
	}
	candidates := collectGrammarCorpusCandidates(t, repoRoot, cfg)
	if len(candidates) == 0 {
		t.Skipf("no corpus candidates for %s", grammarName)
	}
	samples := make([]string, len(candidates))
	for i, c := range candidates {
		samples[i] = c.Text
	}

	// Compare language metadata.
	t.Logf("=== Language Comparison ===")
	t.Logf("GEN: symbols=%d tokens=%d parseTable=%d smallParseTable=%d parseActions=%d",
		genLang.SymbolCount, genLang.TokenCount, len(genLang.ParseTable), len(genLang.SmallParseTable), len(genLang.ParseActions))
	t.Logf("REF: symbols=%d tokens=%d parseTable=%d smallParseTable=%d parseActions=%d",
		refLang.SymbolCount, refLang.TokenCount, len(refLang.ParseTable), len(refLang.SmallParseTable), len(refLang.ParseActions))

	t.Logf("--- GEN symbol names ---")
	for i, name := range genLang.SymbolNames {
		vis := ""
		if i < len(genLang.SymbolMetadata) && genLang.SymbolMetadata[i].Visible {
			vis = " [visible]"
		}
		t.Logf("  sym%d: %q%s", i, name, vis)
	}
	t.Logf("--- REF symbol names ---")
	for i, name := range refLang.SymbolNames {
		vis := ""
		if i < len(refLang.SymbolMetadata) && refLang.SymbolMetadata[i].Visible {
			vis = " [visible]"
		}
		t.Logf("  sym%d: %q%s", i, name, vis)
	}

	// Dump alias sequences.
	t.Logf("--- GEN AliasSequences (%d) ---", len(genLang.AliasSequences))
	for i, seq := range genLang.AliasSequences {
		if seq == nil {
			continue
		}
		hasNonZero := false
		for _, s := range seq {
			if s != 0 {
				hasNonZero = true
				break
			}
		}
		if hasNonZero {
			names := make([]string, len(seq))
			for j, s := range seq {
				if s == 0 {
					names[j] = "-"
				} else if int(s) < len(genLang.SymbolNames) {
					names[j] = fmt.Sprintf("%d(%s)", s, genLang.SymbolNames[s])
				} else {
					names[j] = fmt.Sprintf("%d", s)
				}
			}
			t.Logf("  prodID=%d: %v", i, names)
		}
	}
	// Dump parse actions for reduce to see which prodIDs are used.
	t.Logf("--- GEN Reduce Actions ---")
	for i, entry := range genLang.ParseActions {
		for _, a := range entry.Actions {
			if a.Type == gotreesitter.ParseActionReduce {
				symName := fmt.Sprintf("sym%d", a.Symbol)
				if int(a.Symbol) < len(genLang.SymbolNames) {
					symName = genLang.SymbolNames[a.Symbol]
				}
				t.Logf("  actions[%d]: REDUCE sym=%s cc=%d prodID=%d", i, symName, a.ChildCount, a.ProductionID)
			}
		}
	}

	genParser := gotreesitter.NewParser(genLang)
	refParser := gotreesitter.NewParser(refLang)

	for i, src := range samples {
		genTree, _ := genParser.Parse([]byte(src))
		refTree, _ := refParser.Parse([]byte(src))

		genRoot := genTree.RootNode()
		refRoot := refTree.RootNode()

		genSExpr := genRoot.SExpr(genLang)
		refSExpr := refRoot.SExpr(refLang)

		t.Logf("=== Sample %d (len=%d) ===", i, len(src))
		t.Logf("Source: %q", src)
		t.Logf("GEN range=[%d:%d] children=%d", genRoot.StartByte(), genRoot.EndByte(), genRoot.ChildCount())
		t.Logf("REF range=[%d:%d] children=%d", refRoot.StartByte(), refRoot.EndByte(), refRoot.ChildCount())
		t.Logf("GEN sexpr: %s", genSExpr)
		t.Logf("REF sexpr: %s", refSExpr)

		if genSExpr != refSExpr {
			t.Logf("DIVERGENCE in sample %d", i)
			// Print children details.
			printNodeChildren(t, "GEN", genRoot, genLang, 0)
			printNodeChildren(t, "REF", refRoot, refLang, 0)
		}

		// Also dump token-level differences.
		t.Logf("--- Token walk GEN ---")
		walkTokens(t, genRoot, genLang, 0)
		t.Logf("--- Token walk REF ---")
		walkTokens(t, refRoot, refLang, 0)

		fmt.Println(strings.Repeat("-", 60))
	}
}

func printNodeChildren(t *testing.T, label string, n *gotreesitter.Node, lang *gotreesitter.Language, depth int) {
	if depth > 10 || n == nil {
		return
	}
	indent := strings.Repeat("  ", depth)
	t.Logf("%s%s %s [%d:%d] named=%v children=%d type=%q",
		indent, label, n.Type(lang), n.StartByte(), n.EndByte(), n.IsNamed(), n.ChildCount(), n.Type(lang))
	for i := 0; i < int(n.ChildCount()); i++ {
		child := n.Child(i)
		if child != nil {
			printNodeChildren(t, label, child, lang, depth+1)
		}
	}
}

func walkTokens(t *testing.T, n *gotreesitter.Node, lang *gotreesitter.Language, depth int) {
	if depth > 20 || n == nil {
		return
	}
	if n.ChildCount() == 0 {
		t.Logf("  token: %q [%d:%d] type=%q named=%v",
			"...", n.StartByte(), n.EndByte(), n.Type(lang), n.IsNamed())
		return
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		walkTokens(t, n.Child(i), lang, depth+1)
	}
}

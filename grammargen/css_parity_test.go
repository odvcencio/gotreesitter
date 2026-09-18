package grammargen

import (
	"os"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCSSFunctionValueParity(t *testing.T) {
	jsonPath := "/tmp/grammar_parity/css/src/grammar.json"
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("CSS grammar.json not available: %v", err)
	}

	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import CSS grammar.json: %v", err)
	}

	genLang, err := generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate CSS language: %v", err)
	}
	refLang := grammars.CssLanguage()
	adaptExternalScanner(refLang, genLang)

	samples := []string{
		"body { color: rgba(255, 255, 255, 0.9); }",
		"body { text-shadow: 0 1px 0 rgba(255, 255, 255, 0.9); }",
		"body { box-shadow: inset 1px 0 0 rgba(255, 255, 255, 0.2); }",
		"body { color: hsla(0, 0%, 100%, 0.5); }",
		`body { padding-right: 8px\9; }`,
	}

	genParser := gotreesitter.NewParser(genLang)
	refParser := gotreesitter.NewParser(refLang)

	for _, sample := range samples {
		genTree, _ := genParser.Parse([]byte(sample))
		refTree, _ := refParser.Parse([]byte(sample))
		genRoot := genTree.RootNode()
		refRoot := refTree.RootNode()

		genSexp := genRoot.SExpr(genLang)
		refSexp := refRoot.SExpr(refLang)
		if genSexp != refSexp {
			if os.Getenv("GTS_CSS_DEBUG_DFA") == "1" {
				gotreesitter.DebugDFA.Store(true)
				debugTree, _ := gotreesitter.NewParser(genLang).Parse([]byte(sample))
				gotreesitter.DebugDFA.Store(false)
				if debugTree != nil {
					t.Logf("debug gen sexpr: %s", debugTree.RootNode().SExpr(genLang))
				}
			}
			t.Fatalf("SExpr mismatch for %q\nGEN: %s\nREF: %s", sample, genSexp, refSexp)
		}

		if divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10); len(divs) > 0 {
			t.Fatalf("deep mismatch for %q: %s\nGEN: %s\nREF: %s", sample, divs[0].String(), genSexp, refSexp)
		}
	}
}

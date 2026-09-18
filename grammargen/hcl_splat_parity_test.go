package grammargen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHCLSplatContinuationParity(t *testing.T) {
	source, err := os.ReadFile(hclGrammarJSONPathForTest())
	if err != nil {
		t.Skipf("HCL grammar.json not available: %v", err)
	}
	grammar, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import HCL grammar.json: %v", err)
	}
	generated, err := generateWithTimeout(grammar, 60*time.Second)
	if err != nil {
		t.Fatalf("generate HCL language: %v", err)
	}
	reference := grammars.HclLanguage()
	adaptExternalScanner(reference, generated)

	for _, testCase := range []struct {
		name  string
		input string
	}{
		{name: "attribute splat", input: "splat1 = foo.*.bar.baz[0]\n"},
		{name: "full splat", input: "splat2 = foo[*].bar.baz[0]\n"},
		{name: "nested resource", input: "resource \"example\" \"splat_expressions\" {\n  splat1 = foo.*.bar.baz[0]\n  splat2 = foo[*].bar.baz[0]\n}\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			generatedTree, err := gotreesitter.NewParser(generated).Parse([]byte(testCase.input))
			if err != nil {
				t.Fatalf("parse with generated language: %v", err)
			}
			defer generatedTree.Release()
			referenceTree, err := gotreesitter.NewParser(reference).Parse([]byte(testCase.input))
			if err != nil {
				t.Fatalf("parse with reference language: %v", err)
			}
			defer referenceTree.Release()

			generatedRoot := generatedTree.RootNode()
			referenceRoot := referenceTree.RootNode()
			generatedSExpr := generatedRoot.SExpr(generated)
			referenceSExpr := referenceRoot.SExpr(reference)
			if generatedSExpr != referenceSExpr {
				t.Fatalf("S-expression mismatch\ngenerated: %s\nreference: %s", generatedSExpr, referenceSExpr)
			}
			if differences := compareTreesDeep(generatedRoot, generated, referenceRoot, reference, "root", 10); len(differences) > 0 {
				t.Errorf("deep mismatch: %s", differences[0].String())
			}
		})
	}
}

func hclGrammarJSONPathForTest() string {
	root := os.Getenv("GTS_GRAMMARGEN_REAL_CORPUS_ROOT")
	if root == "" {
		root = "/tmp/grammar_parity"
	}
	return filepath.Join(root, "hcl", "src", "grammar.json")
}

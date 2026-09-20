//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestLiquidCommentExternalSymbolCOracle pins the liquid scanner's
// comment-content fix against the real C tree-sitter oracle. The scanner
// used to hardcode absolute symbols [96 97 98 99 100 101] for its six
// tokens, but the shipped liquid.bin blob's ExternalSymbols is
// [98 99 100 101 102 103]. liquidSymInlineCommentContent (96) and
// liquidSymPairedCommentContent (97) fell outside ExternalSymbols entirely;
// the other four hardcoded values landed one external position too low
// each while still passing a naive membership check.
func TestLiquidCommentExternalSymbolCOracle(t *testing.T) {
	cLanguage, err := ParityCLanguage("liquid")
	if err != nil {
		t.Skipf("C liquid oracle unavailable: %v", err)
	}
	goLanguage := grammars.LiquidLanguage()

	cases := []struct {
		name     string
		source   string
		wantSExp string
	}{
		{name: "paired-comment", source: "{% comment %}hello{% endcomment %}", wantSExp: "(program (comment))"},
		{name: "inline-comment", source: "{% # inline %}\n", wantSExp: "(program (comment) (template_content))"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)

			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot == nil {
				t.Fatal("liquid C oracle returned no root")
			}
			if cRoot.HasError() {
				t.Fatalf("liquid C oracle root has error:\n%s", dumpCTree(cRoot, 0))
			}

			goParser := gotreesitter.NewParser(goLanguage)
			goTree, err := goParser.Parse(source)
			if err != nil {
				t.Fatalf("liquid Go parse: %v", err)
			}
			defer goTree.Release()
			goRoot := goTree.RootNode()
			if goRoot == nil {
				t.Fatal("liquid Go parse returned no root")
			}
			if goRoot.HasError() {
				t.Fatalf("liquid Go root has error:\n%s", dumpGoTree(goRoot, goLanguage, 0))
			}

			var errs []string
			compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
			if len(errs) != 0 {
				t.Fatalf(
					"liquid Go/C comment tree diverged for %q:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
					tc.source,
					joinTopErrors(errs),
					goRoot.SExpr(goLanguage),
					dumpGoTree(goRoot, goLanguage, 0),
					cRoot.ToSexp(),
					dumpCTree(cRoot, 0),
				)
			}

			if got := goRoot.SExpr(goLanguage); got != tc.wantSExp {
				t.Fatalf("liquid Go S-expression = %s, want %s", got, tc.wantSExp)
			}
		})
	}
}

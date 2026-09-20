package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestLiquidCommentExternalSymbolRegression regresses a real bug: the liquid
// scanner's six comment/raw/front-matter tokens hardcoded absolute symbols
// [96 97 98 99 100 101], but the shipped liquid.bin blob's ExternalSymbols
// is [98 99 100 101 102 103]. liquidSymInlineCommentContent (96) and
// liquidSymPairedCommentContent (97) fell outside ExternalSymbols entirely
// and broke the parse; the other four constants (98, 99, 100, 101) landed
// one external position too low each, silently emitting the wrong token
// while still passing a naive ExternalSymbols-membership check. cgo_harness
// carries C-oracle twins of both cases below.
func TestLiquidCommentExternalSymbolRegression(t *testing.T) {
	lang := grammars.LiquidLanguage()
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{name: "paired-comment", source: "{% comment %}hello{% endcomment %}", want: "(program (comment))"},
		{name: "inline-comment", source: "{% # inline %}\n", want: "(program (comment) (template_content))"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("liquid %q produced an error tree:\n%s", tc.source, root.SExpr(lang))
			}
			if got := root.SExpr(lang); got != tc.want {
				t.Fatalf("liquid %q SExpr = %s, want %s", tc.source, got, tc.want)
			}
		})
	}
}

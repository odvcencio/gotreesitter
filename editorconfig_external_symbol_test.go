package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestEditorconfigIntegerRangeExternalSymbolRegression regresses a real bug:
// the editorconfig scanner's integer-range-start token used to hardcode
// gotreesitter.Symbol(32), but the shipped editorconfig.bin blob's
// ExternalSymbols is [23 24]. Symbol 32 in that blob names brace_expansion,
// so the mislabeled token broke the parse table lookup and the range
// production failed. cgo_harness carries a C-oracle twin of this case.
func TestEditorconfigIntegerRangeExternalSymbolRegression(t *testing.T) {
	lang := grammars.EditorconfigLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte("[{1..3}]\n")
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("editorconfig integer range produced an error tree:\n%s", root.SExpr(lang))
	}
	want := "(editorconfig (section (header (glob (integer_range (integer) (integer))))))"
	if got := root.SExpr(lang); got != want {
		t.Fatalf("editorconfig integer range SExpr = %s, want %s", got, want)
	}
}

// TestEditorconfigEndOfFileExternalSymbolRegression pins the end-of-file
// token to a clean, error-free parse for a header and a bare property, both
// at end of input with no trailing newline. The pre-fix hardcoded symbol did
// not make these specific cases visibly diverge (the token is zero-width
// and hidden), but the test still exercises the scanner's end-of-file
// branch end to end against the now correctly bound symbol.
func TestEditorconfigEndOfFileExternalSymbolRegression(t *testing.T) {
	lang := grammars.EditorconfigLanguage()
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{name: "header-no-trailing-newline", source: "[foo]", want: "(editorconfig (section (header (glob))))"},
		{name: "property-no-trailing-newline", source: "key=1", want: "(editorconfig (preamble (pair (property) (string))))"},
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
				t.Fatalf("editorconfig %q produced an error tree:\n%s", tc.source, root.SExpr(lang))
			}
			if got := root.SExpr(lang); got != tc.want {
				t.Fatalf("editorconfig %q SExpr = %s, want %s", tc.source, got, tc.want)
			}
		})
	}
}

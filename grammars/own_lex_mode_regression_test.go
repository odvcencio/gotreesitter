package grammars

import (
	"os"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// A GLR stack that re-reads the shared lookahead with its own lex mode must
// start where C lexes that version: at the stack's position, before the
// whitespace the shared lexer skipped. Read from the token's own start, these
// lex modes take readings that only exist without leading whitespace (Julia's
// `:` after `?`, F#'s `<` and `+`, Earthfile's immediate string fragments).
// The expected values are the C runtime's for the same grammar revisions.
func TestOwnLexModeReadAfterWhitespaceMatchesC(t *testing.T) {
	src := []byte("h = (UInt === UInt64) ? 0x94cb2bb20a28ce96 : 0x1f60a087\n")
	const want = "(source_file (assignment (identifier) (operator) (ternary_expression (parenthesized_expression (binary_expression (identifier) (operator) (identifier))) (integer_literal) (integer_literal))))"
	tree, err := ts.NewParser(JuliaLanguage()).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.RootNode().SExpr(JuliaLanguage()); got != want {
		t.Errorf("julia ternary:\n got  %s\n want %s\n runtime=%s", got, want, tree.ParseRuntime().Summary())
	}

	for _, tc := range []struct {
		name     string
		lang     func() *ts.Language
		hasError bool
		children int
	}{
		{"julia", JuliaLanguage, false, 22},
		{"earthfile", EarthfileLanguage, false, 11},
		{"fsharp", FsharpLanguage, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile("../internal/benchfixtures/testdata/real/" + tc.name)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := ts.NewParser(tc.lang()).Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			root := tree.RootNode()
			if root.EndByte() != uint32(len(src)) || root.HasError() != tc.hasError || int(root.ChildCount()) != tc.children {
				t.Errorf("root ends at %d of %d, HasError=%v, %d children; want the whole input, HasError=%v, %d children; runtime=%s",
					root.EndByte(), len(src), root.HasError(), root.ChildCount(), tc.hasError, tc.children, tree.ParseRuntime().Summary())
			}
		})
	}
}

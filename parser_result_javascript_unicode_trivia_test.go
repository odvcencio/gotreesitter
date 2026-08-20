package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestJavaScriptUnicodeLineSeparatorAfterExtraParsesClean(t *testing.T) {
	source := []byte("let x = { a: //\u2028 3\n}")
	lang := grammars.JavascriptLanguage()
	want := "(program (lexical_declaration (variable_declarator (identifier) (object (pair (property_identifier) (comment) (number))))))"

	for _, candidate := range []bool{true, false} {
		name := "production"
		if candidate {
			name = "candidate"
		}
		t.Run(name, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(candidate)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()

			root := tree.RootNode()
			if root == nil {
				t.Fatal("parser returned a nil root")
			}
			if root.HasError() {
				t.Fatalf("unicode line separator produced an error tree: %s", root.SExpr(lang))
			}
			if got := root.SExpr(lang); got != want {
				t.Fatalf("tree = %s, want %s", got, want)
			}
		})
	}
}

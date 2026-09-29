package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGdscriptValidLayoutMatchesC(t *testing.T) {
	entry := grammars.DetectLanguageByName("gdscript")
	if entry == nil {
		t.Fatal("gdscript grammar is not registered")
	}
	lang := entry.Language()
	for _, tc := range []struct {
		name, source, want string
	}{
		{
			name:   "nested getter dedents before variable",
			source: "var h:\n\tget:\n\t\tif 0:\n\t\t\te\nvar m:=2()",
			want:   "(source (variable_statement (name) (setget (get_body (body (if_statement (integer) (body (expression_statement (identifier)))))))) (variable_statement (name) (inferred_type) (call (integer) (arguments))))",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if got := root.SExpr(lang); got != tc.want {
				t.Errorf("SExpr = %s, want %s", got, tc.want)
			}
			if root.HasError() {
				t.Errorf("unexpected parse error: %s", root.SExpr(lang))
			}
		})
	}
}

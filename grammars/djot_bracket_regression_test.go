package grammars_test

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"testing"
)

func TestDjotBracketReferenceParity(t *testing.T) {
	lang := grammars.DjotLanguage()
	for _, tc := range []struct{ source, want string }{
		{" [ m", "(document (paragraph) (paragraph))"},
		{"[e][]\n", "(document (paragraph (collapsed_reference_link (link_text))))"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if got := tree.RootNode().SExpr(lang); got != tc.want {
				t.Fatalf("tree = %s, want %s", got, tc.want)
			}
			if tree.RootNode().HasError() {
				t.Fatal("valid input has an error")
			}
		})
	}
}

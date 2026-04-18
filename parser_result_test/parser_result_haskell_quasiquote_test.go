package parserresult_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestHaskellQuasiquoteIncludesOpeningBracket(t *testing.T) {
	const src = "instance Witch.From BackendType NonEmptyText where\n  from (Postgres Vanilla) = [nonEmptyTextQQ|postgres|]\n"
	tree, lang := parseByLanguageName(t, "haskell", src)
	root := tree.RootNode()
	quasi := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "quasiquote"
	})
	if quasi == nil {
		t.Fatalf("missing quasiquote: %s", root.SExpr(lang))
	}
	if got, want := quasi.Text([]byte(src)), " [nonEmptyTextQQ|postgres|]"; got != want {
		t.Fatalf("quasiquote text = %q, want %q", got, want)
	}
}

package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCSharpRecoveredTopLevelChunksKeepsChildErrorOnRoot is the regression
// test for task #97: normalizeCSharpRecoveredTopLevelChunks replaced the
// root's children with re-parsed top-level chunks and then cleared the
// root's hasError bit unconditionally. When a recovered chunk still carries
// an error, the root claimed no error while a descendant reported one, which
// breaks the parent/child HasError invariant every traversal API relies on.
// The pass must derive the root's flag from its new children instead.
func TestCSharpRecoveredTopLevelChunksKeepsChildErrorOnRoot(t *testing.T) {
	lang := grammars.CSharpLanguage()
	src := []byte("var x = c is < '0' o >= 'A' and <= 'Z';")
	p := gotreesitter.NewParser(lang)
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root == nil || !root.HasError() {
		t.Fatalf("expected an erroring root after Parse:\n%s", root.SExpr(lang))
	}
	if !nodeHasErrorDescendant(root) {
		t.Fatalf("witness no longer carries a descendant error:\n%s", root.SExpr(lang))
	}

	// A second application on the already-normalized root re-parses the same
	// chunks and must leave the root's flag consistent with its children.
	gotreesitter.NormalizeCSharpRecoveredTopLevelChunksForTest(p, tree, src)
	root = tree.RootNode()
	if nodeHasErrorDescendant(root) && !root.HasError() {
		for i := 0; i < root.ChildCount(); i++ {
			c := root.Child(i)
			t.Logf("child %d: type=%s isError=%v isMissing=%v hasError=%v span=[%d-%d]", i, c.Type(lang), c.IsError(), c.IsMissing(), c.HasError(), c.StartByte(), c.EndByte())
		}
		t.Fatalf("root HasError()=false while a descendant reports an error after a second application:\n%s", root.SExpr(lang))
	}
	assertParentChildHasErrorInvariant(t, lang, root)
}

func nodeHasErrorDescendant(n *gotreesitter.Node) bool {
	if n == nil {
		return false
	}
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.IsError() || child.IsMissing() || child.HasError() || nodeHasErrorDescendant(child) {
			return true
		}
	}
	return false
}

func assertParentChildHasErrorInvariant(t *testing.T, lang *gotreesitter.Language, n *gotreesitter.Node) {
	t.Helper()
	if n == nil {
		return
	}
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.HasError() && !n.HasError() {
			t.Fatalf("node %q has HasError()=false but child %q reports HasError()=true", n.Type(lang), child.Type(lang))
		}
		assertParentChildHasErrorInvariant(t, lang, child)
	}
}

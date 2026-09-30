//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestTreeViewNavigationLockedC keeps both versions alive and compares every
// view edge, payload property, and field with the canonical fresh C tree.
func TestTreeViewNavigationLockedC(t *testing.T) {
	for _, name := range []string{"go", "c_sharp"} {
		t.Run(name, func(t *testing.T) {
			source, _, err := benchfixtures.GeneratedSource(name, 32*1024)
			if err != nil {
				t.Fatal(err)
			}
			at := bytes.LastIndex(source, []byte("a + b")) + 2
			if at < 2 {
				t.Fatal("missing edit marker")
			}
			edited := append([]byte(nil), source...)
			edited[at] = '-'
			var lang *gts.Language
			if name == "go" {
				lang = grammars.GoLanguage()
			} else {
				lang = grammars.CSharpLanguage()
			}
			cLang, err := COracleLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := COracleIdentity(name)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("runtime=%s@%s grammar=%s artifact_sha256=%s source_sha256=%x edited_sha256=%x", identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarCommit, identity.GrammarArtifactSHA256, sha256.Sum256(source), sha256.Sum256(edited))
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cOld := cParser.Parse(source, nil)
			defer cOld.Close()
			cNew := cParser.Parse(edited, nil)
			defer cNew.Close()
			if cOld == nil || cNew == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			parser := gts.NewParser(lang)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			oldView := old.RootNodeView()
			assertTreeViewLockedC(t, oldView, cOld.RootNode(), lang)
			old.Edit(gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1), StartPoint: pointAtOffset(source, at), OldEndPoint: pointAtOffset(source, at+1), NewEndPoint: pointAtOffset(source, at+1)})
			next, err := parser.ParseIncremental(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			assertTreeViewLockedC(t, next.RootNodeView(), cNew.RootNode(), lang)
			assertTreeViewLockedC(t, oldView, cOld.RootNode(), lang)
			next.Release()
			assertTreeViewLockedC(t, oldView, cOld.RootNode(), lang)
		})
	}
}

func assertTreeViewLockedC(t *testing.T, view *gts.NodeView, cNode *sitter.Node, lang *gts.Language) {
	t.Helper()
	if view == nil || cNode == nil {
		t.Fatal("nil navigation witness node")
	}
	if view.Type(lang) != cNode.Kind() || uint(view.StartByte()) != cNode.StartByte() || uint(view.EndByte()) != cNode.EndByte() || view.ChildCount() != int(cNode.ChildCount()) || view.NamedChildCount() != int(cNode.NamedChildCount()) || view.IsNamed() != cNode.IsNamed() || view.IsExtra() != cNode.IsExtra() || view.IsMissing() != cNode.IsMissing() || view.IsError() != cNode.IsError() || view.HasError() != cNode.HasError() {
		t.Fatalf("view/C node differs: Go=%s[%d,%d] C=%s[%d,%d]", view.Type(lang), view.StartByte(), view.EndByte(), cNode.Kind(), cNode.StartByte(), cNode.EndByte())
	}
	start, end := cNode.StartPosition(), cNode.EndPosition()
	if view.StartPoint() != (gts.Point{Row: uint32(start.Row), Column: uint32(start.Column)}) || view.EndPoint() != (gts.Point{Row: uint32(end.Row), Column: uint32(end.Column)}) {
		t.Fatal("view/C point differs")
	}
	for i := 0; i < view.ChildCount(); i++ {
		child, cChild := view.Child(i), cNode.Child(uint(i))
		if child.Parent() != view || cChild.Parent().Id() != cNode.Id() || view.FieldNameForChild(i, lang) != cNode.FieldNameForChild(uint32(i)) {
			t.Fatal("view/C parent or field differs")
		}
		if i > 0 && child.PrevSibling() != view.Child(i-1) || i+1 < view.ChildCount() && child.NextSibling() != view.Child(i+1) {
			t.Fatal("view sibling differs from C child order")
		}
		assertTreeViewLockedC(t, child, cChild, lang)
	}
}

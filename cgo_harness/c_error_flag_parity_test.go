//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestIssue454CErrorFlagFreshAndIncrementalDeepParity(t *testing.T) {
	source := []byte("int f0(void) { int x0 = 0; return x0; }\n")
	if len(source) != 40 {
		t.Fatalf("minimized C witness length = %d, want 40", len(source))
	}
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C edit marker is absent")
	}
	edited := append(append([]byte(nil), source[:site]...), source[site+1:]...)
	point := issue454CErrorFlagPointAt(source, site)
	lang := grammars.CLanguage()
	oldTree, err := gts.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	oldTree.Edit(gts.InputEdit{
		StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
		StartPoint: point, OldEndPoint: gts.Point{Row: point.Row, Column: point.Column + 1}, NewEndPoint: point,
	})
	incremental, _, err := gts.NewParser(lang).ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	fresh, err := gts.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	goFresh, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	goIncremental, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	cLang, err := COracleLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C oracle returned no tree")
	}
	defer cTree.Close()
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	if goFresh.SHA256 != cDigest || goIncremental.SHA256 != cDigest {
		t.Fatalf("deep digest mismatch: fresh=%s incremental=%s C=%s", goFresh.SHA256, goIncremental.SHA256, cDigest)
	}
	assertIssue454ErrorChild(t, fresh.RootNode(), lang, cTree.RootNode())
	assertIssue454ErrorChild(t, incremental.RootNode(), lang, cTree.RootNode())
}

func TestIssue454CErrorFlagNonCControls(t *testing.T) {
	cases := []struct {
		name   string
		lang   *gts.Language
		source []byte
	}{
		{name: "go", lang: grammars.GoLanguage(), source: []byte("package p\nfunc f() { f(1 2) }\n")},
		{name: "javascript", lang: grammars.JavascriptLanguage(), source: []byte("function f(){return 1;}@\n")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			tree, err := gts.NewParser(test.lang).Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if !tree.RootNode().HasErrorOrMissing() {
				t.Fatalf("root HasErrorOrMissing = false, want true: %s", tree.RootNode().SExpr(test.lang))
			}
			goDigest, err := benchfixtures.InspectGoTree(tree.RootNode(), test.lang)
			if err != nil {
				t.Fatal(err)
			}
			cLang, err := COracleLanguage(test.name)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned no tree")
			}
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if goDigest.SHA256 != cDigest {
				t.Fatalf("deep digest mismatch: Go=%s C=%s", goDigest.SHA256, cDigest)
			}
			assertIssue454ErrorChild(t, tree.RootNode(), test.lang, cTree.RootNode())
		})
	}
}

func assertIssue454ErrorChild(t *testing.T, goRoot *gts.Node, lang *gts.Language, cRoot *sitter.Node) {
	t.Helper()
	goError, goChild := issue454GoErrorWithOrdinaryChild(goRoot)
	if goError == nil || goChild == nil {
		t.Fatalf("no ERROR node with an ordinary child: %s", goRoot.SExpr(nil))
	}
	if !goError.HasError() {
		t.Fatal("ERROR parent HasError = false, want true")
	}
	if goChild.HasError() {
		t.Fatalf("ordinary ERROR child %q HasError = true, want false", goChild.Type(lang))
	}
	cError, cChild := issue454CErrorWithOrdinaryChild(cRoot)
	if cError == nil || cChild == nil {
		t.Fatal("C oracle has no matching ERROR node with an ordinary child")
	}
	if cError.Kind() != goError.Type(lang) || cChild.Kind() != goChild.Type(lang) ||
		cError.StartByte() != uint(goError.StartByte()) || cError.EndByte() != uint(goError.EndByte()) ||
		cChild.StartByte() != uint(goChild.StartByte()) || cChild.EndByte() != uint(goChild.EndByte()) {
		t.Fatalf("ERROR child shape differs: Go=%s/%s [%d:%d]/[%d:%d] C=%s/%s [%d:%d]/[%d:%d]",
			goError.Type(lang), goChild.Type(lang), goError.StartByte(), goError.EndByte(), goChild.StartByte(), goChild.EndByte(),
			cError.Kind(), cChild.Kind(), cError.StartByte(), cError.EndByte(), cChild.StartByte(), cChild.EndByte())
	}
}

func issue454GoErrorWithOrdinaryChild(root *gts.Node) (*gts.Node, *gts.Node) {
	if root == nil {
		return nil, nil
	}
	if root.IsError() {
		for i := 0; i < root.ChildCount(); i++ {
			child := root.Child(i)
			if child != nil && !child.IsError() && !child.IsMissing() {
				return root, child
			}
		}
	}
	for i := 0; i < root.ChildCount(); i++ {
		if parent, child := issue454GoErrorWithOrdinaryChild(root.Child(i)); parent != nil {
			return parent, child
		}
	}
	return nil, nil
}

func issue454CErrorWithOrdinaryChild(root *sitter.Node) (*sitter.Node, *sitter.Node) {
	if root == nil {
		return nil, nil
	}
	if root.IsError() {
		for i := uint(0); i < root.ChildCount(); i++ {
			child := root.Child(i)
			if child != nil && !child.IsError() && !child.IsMissing() {
				return root, child
			}
		}
	}
	for i := uint(0); i < root.ChildCount(); i++ {
		if parent, child := issue454CErrorWithOrdinaryChild(root.Child(i)); parent != nil {
			return parent, child
		}
	}
	return nil, nil
}

func issue454CErrorFlagPointAt(source []byte, offset int) gts.Point {
	var row, column uint32
	for _, b := range source[:offset] {
		if b == '\n' {
			row++
			column = 0
		} else {
			column++
		}
	}
	return gts.Point{Row: row, Column: column}
}

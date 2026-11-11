//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"math"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestRootNodeWithOffsetPointsMatchC walks every node of Tree.RootNodeWithOffset
// and of the C ts_tree_root_node_with_offset for the same source and offset,
// and compares start byte, end byte, start point and end point. C applies
// point_add: only a point on row zero of the tree takes the offset column.
func TestRootNodeWithOffsetPointsMatchC(t *testing.T) {
	type input struct {
		lang   string
		goLang func() *gotreesitter.Language
		src    string
	}
	inputs := []input{
		{"json", grammars.JsonLanguage, "1"},
		{"json", grammars.JsonLanguage, "\n1"},
		{"json", grammars.JsonLanguage, "{\n  \"a\": [1, 2,\n    3],\n  \"b\": {\"c\": null}\n}\n"},
		{"json", grammars.JsonLanguage, "\n\n  [\n    true,\n  false ]\n"},
		{"go", grammars.GoLanguage, "package p\n\nfunc f(a int) int {\n\tx := a + 1\n\treturn x\n}\n"},
		{"go", grammars.GoLanguage, "\n\n\tvar x = 1\n\nfunc g() {\n}\n"},
		{"python", grammars.PythonLanguage, "def f(a):\n    if a:\n        return 1\n    return 2\n"},
		{"python", grammars.PythonLanguage, "\n\n    x = 1\ny = [1,\n     2]\n"},
	}
	type offset struct {
		bytes uint32
		point gotreesitter.Point
	}
	offsets := []offset{
		{3, gotreesitter.Point{Row: 0, Column: 4}},
		{10, gotreesitter.Point{Row: 2, Column: 5}},
		{10, gotreesitter.Point{Row: 2, Column: 0}},
		{1, gotreesitter.Point{Row: 1, Column: 3}},
		{7, gotreesitter.Point{Row: 0, Column: 0}},
		// Unsigned wrap-around, as in C: column, byte and row additions overflow.
		{0, gotreesitter.Point{Row: 0, Column: math.MaxUint32}},
		{math.MaxUint32, gotreesitter.Point{}},
		{0, gotreesitter.Point{Row: math.MaxUint32, Column: 0}},
		{1, gotreesitter.Point{Row: math.MaxUint32, Column: 5}},
	}
	nodes, diffs := 0, 0
	perLang := map[string]int{}
	for _, in := range inputs {
		cLang, err := ParityCLanguage(in.lang)
		if err != nil {
			t.Fatalf("C parser %s unavailable: %v", in.lang, err)
		}
		cParser := sitter.NewParser()
		if err := cParser.SetLanguage(cLang); err != nil {
			t.Fatalf("C SetLanguage %s: %v", in.lang, err)
		}
		src := []byte(in.src)
		cTree := cParser.Parse(src, nil)
		if cTree == nil {
			t.Fatalf("C parse %s failed", in.lang)
		}
		goTree, err := gotreesitter.NewParser(in.goLang()).Parse(src)
		if err != nil || goTree == nil {
			t.Fatalf("go parse %s: %v", in.lang, err)
		}
		for _, off := range offsets {
			goRoot := goTree.RootNodeWithOffset(off.bytes, off.point)
			cRoot := cTree.RootNodeWithOffset(int(off.bytes), sitter.Point{Row: uint(off.point.Row), Column: uint(off.point.Column)})
			var walk func(g *gotreesitter.Node, c *sitter.Node, path string)
			walk = func(g *gotreesitter.Node, c *sitter.Node, path string) {
				nodes++
				perLang[in.lang]++
				gs, ge := g.StartPoint(), g.EndPoint()
				cs, ce := c.StartPosition(), c.EndPosition()
				if g.StartByte() != uint32(c.StartByte()) || g.EndByte() != uint32(c.EndByte()) ||
					uint(gs.Row) != cs.Row || uint(gs.Column) != cs.Column ||
					uint(ge.Row) != ce.Row || uint(ge.Column) != ce.Column {
					diffs++
					if diffs <= 20 {
						t.Errorf("%s offset=%d/%+v %s: go bytes %d-%d %+v-%+v, C bytes %d-%d %+v-%+v",
							in.lang, off.bytes, off.point, path, g.StartByte(), g.EndByte(), gs, ge,
							c.StartByte(), c.EndByte(), cs, ce)
					}
				}
				if gc, cc := g.ChildCount(), int(c.ChildCount()); gc != cc {
					diffs++
					t.Errorf("%s %s: child count go %d, C %d", in.lang, path, gc, cc)
					return
				}
				for i := 0; i < g.ChildCount(); i++ {
					walk(g.Child(i), c.Child(uint(i)), fmt.Sprintf("%s/%d", path, i))
				}
			}
			walk(goRoot, cRoot, "root")
		}
		cTree.Close()
		cParser.Close()
	}
	t.Logf("compared %d nodes, differences %d", nodes, diffs)
	for _, name := range []string{"json", "go", "python"} {
		if perLang[name] == 0 {
			t.Fatalf("no nodes compared for %s", name)
		}
	}
	t.Logf("nodes per grammar: %v", perLang)
}

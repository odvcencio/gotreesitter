//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreJSONNodeNextState(t *testing.T) {
	gl, language := ccoreLanguages(t, "json")

	cp := sitter.NewParser()
	defer cp.Close()
	if e := cp.SetLanguage(language); e != nil {
		t.Fatal(e)
	}
	core := ccoreNew(gl)
	checked, comparable, preDiff, skipped := 0, 0, 0, 0
	for _, src := range []string{`{"a":1}`, `[true,false,null,{"x":[1,2]}]`, `{"a":`, `[]`} {
		ct := cp.Parse([]byte(src), nil)
		if ct == nil {
			t.Fatal("C nil tree")
		}
		ch, cn, rc := corecords.Hash(ct, "")
		if rc != 0 {
			t.Fatal("C records")
		}
		if stop := core.Parse([]byte(src), nil); stop != "" {
			t.Fatal(stop)
		}
		gh, gn := core.RecordsHash("")
		if ch != gh || cn != gn {
			skipped++
			t.Errorf("incomparable records input=%q C=%x/%d Go=%x/%d", src, ch, cn, gh, gn)
			core.ReleaseLast()
			ct.Close()
			continue
		}
		comparable++
		node := core.Tree()
		rd, root := core.NodeReader(node)
		var walk func(ccore.Node, *sitter.Node)
		walk = func(g ccore.Node, c *sitter.Node) {
			if g.Null() || c == nil {
				t.Fatal("nil node")
			}
			checked++
			if rd.ParseState(g) != c.NextParseState() {
				preDiff++
			}
			if a, b := rd.ParseState(g), c.ParseState(); a != b {
				t.Errorf("parse state C=%d Go=%d", b, a)
			}
			if a, b := rd.NextParseState(g), c.NextParseState(); a != b {
				t.Errorf("next state C=%d Go=%d", b, a)
			}
			if int(rd.ChildCount(g)) != int(c.ChildCount()) {
				t.Fatal("child count")
			}
			for i := uint32(0); i < rd.ChildCount(g); i++ {
				child, ok := rd.Child(g, i)
				if !ok {
					t.Fatal("missing child")
				}
				walk(child, c.Child(uint(i)))
			}
		}
		walk(rd.RootNode(root), ct.RootNode())
		core.ReleaseLast()
		ct.Close()
	}
	fmt.Printf("NEXTSTATE comparable_inputs=%d skipped_inputs=%d checked_nodes=%d pre_state_differs_from_next=%d\n", comparable, skipped, checked, preDiff)
	fmt.Printf("CCCOUNT test=TestCcoreJSONNodeNextState set=json checked=%d\n", checked)
	if comparable == 0 || checked == 0 || preDiff == 0 {
		t.Fatal("no comparable inputs")
	}
}

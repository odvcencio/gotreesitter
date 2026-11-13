//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestFieldAnswersAgainstC(t *testing.T) {
	var name, source string
	switch os.Getenv("CCORE_FIELD_GRAMMAR") {
	case "lua":
		if os.Getenv("CCORE_FIELD_LUA_UNCHECKED_SCANNER") != "1" {
			t.Skip("Lua requires the diagnostic scanner opt-in")
		}
		name = "lua"
		source = "local M = {}\nlocal function f(a) return a + 1 end\n"
	case "", "json":
		name = "json"
		source = `{"a": [1, true, null], "b": {"c": "x"}}`
	default:
		t.Skip("set CCORE_FIELD_GRAMMAR to lua or json")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatal("missing Go grammar")
	}
	lang := entry.Language()
	cfg, e := bind.Config(lang)
	if e != nil {
		t.Fatal(e)
	}
	p, e := ccore.NewParser(cfg)
	if e != nil {
		t.Fatal(e)
	}
	src := []byte(source)
	tr, stop := p.Parse(src)
	if stop != "" || tr == nil {
		t.Fatalf("core stop=%q", stop)
	}
	defer tr.Release()
	if cfg.Scanner != nil {
		defer cfg.Scanner.Close()
	}
	// This log makes the existing pinned-source cache identity reviewable.
	lockPath, e := findParityLockPath()
	if e != nil {
		t.Fatal(e)
	}
	lock, e := loadParityLock(lockPath)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("oracle cache=%s", parityCachedSOPath(parityDefaultCBuildCacheDir(lockPath), lock[name]))
	cl, e := COracleLanguage(name)
	if e != nil {
		t.Fatal(e)
	}
	ct := receiptCTree(t, cl, src)
	defer ct.Close()
	if int(cl.FieldCount()) != len(lang.FieldNames)-1 {
		t.Fatalf("field count C=%d Go=%d", cl.FieldCount(), len(lang.FieldNames)-1)
	}
	r := ccore.NewNodeReader(tr.Reader(), cfg.Nodes)
	root := r.RootNode(tr.Root())
	parents, fields, hits, aliases, hidden := 0, 0, 0, 0, 0
	cursorZero, cursorNonzero := 0, 0
	var locate func(*sitter.Node, *sitter.Node, []uint32) ([]uint32, bool)
	locate = func(parent, target *sitter.Node, path []uint32) ([]uint32, bool) {
		for i := uint(0); i < parent.ChildCount(); i++ {
			ch := parent.Child(i)
			next := append(path, uint32(i))
			if ch.Equals(*target) {
				return next, true
			}
			if p, ok := locate(ch, target, next); ok {
				return p, true
			}
		}
		return nil, false
	}
	var walk func(ccore.Node, *sitter.Node, string)
	walk = func(n ccore.Node, c *sitter.Node, path string) {
		parents++
		if n.Alias != 0 {
			aliases++
		}
		_, h := r.ChildrenHidden(n, nil)
		if h {
			hidden++
		}
		if c == nil || uint(r.ChildCount(n)) != c.ChildCount() || r.StartByte(n) != uint32(c.StartByte()) || r.EndByte(n) != uint32(c.EndByte()) {
			t.Fatalf("tree mismatch %s", path)
		}
		var edgeBuf [128]ccore.FieldEdgeAnswer
		edges, _, _, ok := r.FieldAnswers(n, nil, 100000, edgeBuf[:0], nil, nil)
		if !ok {
			t.Fatalf("edge snapshot declined %s", path)
		}
		cursor := c.Walk()
		if len(edges) > 0 && !cursor.GotoFirstChild() {
			t.Fatalf("cursor missing child %s", path)
		}
		for j, edge := range edges {
			want := cursor.FieldId()
			if edge.CursorField != want {
				t.Fatalf("cursor %s.%d core=%d C=%d", path, j, edge.CursorField, want)
			}
			if want == 0 {
				cursorZero++
			} else {
				cursorNonzero++
			}
			if j+1 < len(edges) && !cursor.GotoNextSibling() {
				t.Fatalf("cursor missing sibling %s.%d", path, j)
			}
		}
		cursor.Close()
		for f := 1; f < len(lang.FieldNames); f++ {
			fields++
			var dst [64]uint32
			var edges [128]ccore.FieldEdgeAnswer
			var answer [1]ccore.FieldLookupAnswer
			ev, av, ps, ok := r.FieldAnswers(n, []uint16{uint16(f)}, 100000, edges[:0], answer[:0], dst[:0])
			if ok {
				for j := range ev {
					child, exists := r.Child(n, uint32(j))
					if !exists || ev[j] != ccore.FieldEdgeOf(child) {
						t.Fatalf("edge mismatch %s.%d", path, j)
					}
				}
			}
			found := ok && av[0].Found
			got := ps
			if !ok {
				t.Fatalf("declined %s field %d", path, f)
			}
			want := c.ChildByFieldId(uint16(f))
			if (want != nil) != found {
				t.Fatalf("presence %s field %s: Go=%t C=%t", path, lang.FieldNames[f], found, want != nil)
			}
			if found {
				hits++
				if hits == 1 {
					if _, _, ok := r.FieldLookupPath(n, uint16(f), 1, dst[:0]); ok {
						t.Fatal("work exhaustion did not decline")
					}
					if _, _, ok := r.FieldLookupPath(n, uint16(f), 100000, nil); ok {
						t.Fatal("destination exhaustion did not decline")
					}
				}
				p, ok := locate(c, want, nil)
				if !ok || fmt.Sprint(p) != fmt.Sprint(got) {
					t.Fatalf("path %s field %s Go=%v C=%v", path, lang.FieldNames[f], got, p)
				}
			}
		}
		for i := uint32(0); i < r.ChildCount(n); i++ {
			ch, ok := r.Child(n, i)
			if !ok {
				t.Fatal("missing child")
			}
			walk(ch, c.Child(uint(i)), fmt.Sprintf("%s.%d", path, i))
		}
	}
	walk(root, ct.RootNode(), "r")
	if hits == 0 {
		t.Fatal("fixture did not exercise a field hit")
	}
	t.Logf("grammar=%s parents=%d fields=%d hits=%d aliases=%d hidden_field_paths=%d", name, parents, fields, hits, aliases, hidden)
	fmt.Printf("CCCOUNT test=TestFieldAnswersAgainstC set=%s checked=%d cursor_zero=%d cursor_nonzero=%d\n", name, fields, cursorZero, cursorNonzero)
}

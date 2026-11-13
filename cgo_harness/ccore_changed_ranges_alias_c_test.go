//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestCcoreChangedRangesTypeScriptAliasFixture checks a nonempty range inside
// TypeScript's aliased interface body against the pinned C runtime.
func TestCcoreChangedRangesTypeScriptAliasFixture(t *testing.T) {
	lang := os.Getenv("CCORE_ALIAS_LANG")
	if lang == "" {
		t.Skip("set CCORE_ALIAS_LANG=typescript")
	}
	if lang != "typescript" {
		t.Fatalf("unsupported CCORE_ALIAS_LANG %q", lang)
	}
	gl, cl := ccoreLanguages(t, "typescript")
	cfg, err := bind.Config(gl)
	if err != nil {
		t.Fatal(err)
	}
	core, err := ccore.NewParser(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	src := []byte("interface A { p: string; }\n")
	at := strings.Index(string(src), "string")
	if at < 0 {
		t.Fatal("edit marker missing")
	}
	at += len("string")
	nextSrc := append(append(append([]byte(nil), src[:at]...), '[', ']'), src[at:]...)
	old, stop := core.ParseWith(src, nil)
	co := cp.Parse(src, nil)
	if stop != "" || old == nil || co == nil {
		t.Fatalf("old parse: %s", stop)
	}
	defer old.Release()
	defer co.Close()
	if cfg.Nodes == nil {
		t.Fatal("missing value-node metadata")
	}
	reader := ccore.NewNodeReader(old.Reader(), cfg.Nodes)
	var target ccore.Node
	count := 0
	var find func(ccore.Node)
	find = func(n ccore.Node) {
		if n.Alias != 0 && int(n.Alias) < len(gl.SymbolNames) && gl.SymbolNames[n.Alias] == "interface_body" && reader.StartByte(n) <= uint32(at) && reader.EndByte(n) >= uint32(at) {
			target = n
			count++
		}
		for i := uint32(0); i < reader.ChildCount(n); i++ {
			child, ok := reader.Child(n, i)
			if !ok {
				t.Fatal("core child missing")
			}
			find(child)
		}
	}
	find(reader.RootNode(old.Root()))
	if count != 1 {
		t.Fatalf("want exactly one aliased interface_body containing byte %d, got %d", at, count)
	}
	t.Logf("CCALIASFIXTURE target_alias=%d span=%d-%d edit=%d", target.Alias, reader.StartByte(target), reader.EndByte(target), at)
	edit := ccore.Edit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 2), StartCol: uint32(at), OldEndCol: uint32(at), NewEndCol: uint32(at + 2)}
	old.Edit(edit)
	co.Edit(&sitter.InputEdit{StartByte: uint(at), OldEndByte: uint(at), NewEndByte: uint(at + 2), StartPosition: sitter.Point{Column: uint(at)}, OldEndPosition: sitter.Point{Column: uint(at)}, NewEndPosition: sitter.Point{Column: uint(at + 2)}})
	next, stop := core.ParseWith(nextSrc, old)
	cn := cp.Parse(nextSrc, co)
	if stop != "" || next == nil || cn == nil {
		t.Fatalf("new parse: %s", stop)
	}
	defer next.Release()
	defer cn.Close()
	got, ok := ccore.ChangedRanges(old, next, cfg.Tables, nil, nil)
	if !ok {
		t.Fatal("changed ranges declined")
	}
	want := co.ChangedRanges(cn)
	formatCore := func(rs []ccore.Range) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = fmt.Sprintf("%d-%d(%d,%d)-(%d,%d)", r.StartByte, r.EndByte, r.StartRow, r.StartCol, r.EndRow, r.EndCol)
		}
		return out
	}
	formatC := func(rs []sitter.Range) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = fmt.Sprintf("%d-%d(%d,%d)-(%d,%d)", r.StartByte, r.EndByte, r.StartPoint.Row, r.StartPoint.Column, r.EndPoint.Row, r.EndPoint.Column)
		}
		return out
	}
	if len(got) == 0 {
		t.Fatal("fixture reported no changed range")
	}
	if !reflect.DeepEqual(formatCore(got), formatC(want)) {
		t.Fatalf("core %v != C %v", formatCore(got), formatC(want))
	}
	fmt.Printf("CCALIASCOMPARE core=%v c=%v\n", formatCore(got), formatC(want))
	ccoreCount("TestCcoreChangedRangesTypeScriptAliasFixture", "typescript", 1)
}

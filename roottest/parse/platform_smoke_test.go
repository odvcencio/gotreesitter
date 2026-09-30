package parse_test

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPlatformRuntime is the small, dependency-free D-D3 execution witness.
// Keep it runnable under WASI as well as each native target.
func TestPlatformRuntime(t *testing.T) {
	t.Logf("executing on %s/%s with %s", runtime.GOOS, runtime.GOARCH, runtime.Version())
	lang := grammars.GoLanguage()
	p := gts.NewParser(lang)
	source := []byte("package p\nvar answer = 1\n")
	parse := func(source []byte) *gts.Tree {
		t.Helper()
		tree, err := p.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if tree == nil || tree.RootNode() == nil {
			t.Fatal("parse returned no root")
		}
		return tree
	}
	check := func(tree *gts.Tree, source []byte) {
		t.Helper()
		root := tree.RootNode()
		if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
			t.Fatalf("root span = [%d,%d), input length = %d", root.StartByte(), root.EndByte(), len(source))
		}
		if tree.ParseStopReason() != gts.ParseStopAccepted || root.HasError() {
			t.Fatalf("clean parse: stop=%s error=%v", tree.ParseStopReason(), root.HasError())
		}
	}
	tree := parse(source)
	defer tree.Release()
	check(tree, source)

	t.Run("no-edit", func(t *testing.T) {
		allocs := testing.AllocsPerRun(10, func() {
			unchanged, err := p.ParseIncremental(source, tree)
			if err != nil || unchanged != tree {
				t.Fatalf("unchanged reparse: tree identity=%v err=%v", unchanged == tree, err)
			}
		})
		if allocs != 0 {
			t.Fatalf("unchanged reparse allocated %g times, want 0", allocs)
		}
	})

	t.Run("edit-matches-fresh", func(t *testing.T) {
		offset := uint32(strings.Index(string(source), "1"))
		tree.Edit(gts.InputEdit{
			StartByte: offset, OldEndByte: offset + 1, NewEndByte: offset + 1,
			StartPoint:  gts.Point{Row: 1, Column: 13},
			OldEndPoint: gts.Point{Row: 1, Column: 14},
			NewEndPoint: gts.Point{Row: 1, Column: 14},
		})
		edited := []byte(strings.Replace(string(source), "1", "2", 1))
		incremental, err := p.ParseIncremental(edited, tree)
		if err != nil {
			t.Fatal(err)
		}
		defer incremental.Release()
		fresh := parse(edited)
		defer fresh.Release()
		check(incremental, edited)
		check(fresh, edited)
		var snapshot func(*gts.Node) string
		snapshot = func(n *gts.Node) string {
			text := fmt.Sprintf("%s:%d:%d:%v:%v:%v:%v:%v", n.Type(lang), n.StartByte(), n.EndByte(), n.StartPoint(), n.EndPoint(), n.IsNamed(), n.IsMissing(), n.HasError())
			for i := 0; i < int(n.ChildCount()); i++ {
				text += "[" + n.FieldNameForChild(i, lang) + ":" + snapshot(n.Child(i)) + "]"
			}
			return text
		}
		if got, want := snapshot(incremental.RootNode()), snapshot(fresh.RootNode()); got != want {
			t.Fatalf("incremental tree differs from fresh:\n%s\n%s", got, want)
		}
	})

	t.Run("recovery", func(t *testing.T) {
		broken := parse([]byte("package p\nvar =\n"))
		defer broken.Release()
		if !broken.RootNode().HasError() {
			t.Fatal("malformed source did not report an error")
		}
		if broken.RootNode().EndByte() != uint32(len(broken.Source())) {
			t.Fatal("recovery dropped input")
		}
	})
}

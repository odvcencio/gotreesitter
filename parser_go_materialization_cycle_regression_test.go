package gotreesitter_test

import (
	"os"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGoRepeatedNestedSubtestsReturnedTreeAcyclic(t *testing.T) {
	fixture, err := os.ReadFile("internal/parsercorephase0/core_test.go")
	if err != nil {
		t.Fatal(err)
	}
	const start, end = 60255, 68583
	if len(fixture) < end {
		t.Fatalf("fixture size = %d, want at least %d", len(fixture), end)
	}
	source := append([]byte("package p\n"), fixture[start:end]...)
	function := append([]byte(nil), source[len("package p\n"):]...)
	for range 7 {
		source = append(source, function...)
	}

	type parseResult struct {
		tree *gotreesitter.Tree
		err  error
	}
	done := make(chan parseResult, 1)
	go func() {
		tree, err := gotreesitter.NewParser(grammars.GoLanguage()).Parse(source)
		done <- parseResult{tree: tree, err: err}
	}()

	var tree *gotreesitter.Tree
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		tree = result.tree
	case <-time.After(30 * time.Second):
		t.Fatal("parse did not return")
	}
	defer tree.Release()

	root := tree.RootNode()
	type frame struct {
		node  *gotreesitter.Node
		child int
	}
	active := map[*gotreesitter.Node]struct{}{root: {}}
	stack := []frame{{node: root}}
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.child >= top.node.ChildCount() {
			delete(active, top.node)
			stack = stack[:len(stack)-1]
			continue
		}
		child := top.node.Child(top.child)
		top.child++
		if child == nil {
			continue
		}
		if _, ok := active[child]; ok {
			t.Fatalf(
				"returned tree contains a back-edge to %s [%d,%d)",
				child.Type(grammars.GoLanguage()),
				child.StartByte(),
				child.EndByte(),
			)
		}
		active[child] = struct{}{}
		stack = append(stack, frame{node: child})
	}
}

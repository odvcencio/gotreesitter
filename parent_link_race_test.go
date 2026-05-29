package gotreesitter_test

import (
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestParentLinkConcurrentAccessNoRace reproduces the HEAD-only data race in
// lazy parent-link wiring: on a fresh full-parse tree, parent links are
// deferred (parser.go deferParentLinks gate), and Node.Parent() wires them on
// first access via wireDeferredParentPathToNode -> setNodeParentLink, which
// writes child.parent OUTSIDE parentLinkMu. Two goroutines calling Parent() on
// the same returned tree therefore race on n.parent (concurrent writes to
// shared ancestors, plus unsynchronized reads).
//
// Run with the race detector — it must be clean:
//
//	go test . -race -run TestParentLinkConcurrentAccessNoRace
//
// Before the fix this fails under -race; after, it passes.
func TestParentLinkConcurrentAccessNoRace(t *testing.T) {
	// Python is one of the languages (java/python/typescript/tsx) that take the
	// DEFERRED parent-link path by default (shouldDeferResultParentLinks); other
	// languages wire eagerly and are race-free. Deeply nested so parent chains
	// overlap (shared ancestors maximize the write/write + read/write window).
	src := []byte(`def outer():
    if a:
        for i in xs:
            with ctx:
                try:
                    return deep(value(here(inner(x))))
                except Exception:
                    while cond:
                        nested(again(more(stuff)))
`)
	lang := grammars.PythonLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Release()

	// Each goroutine independently descends the SAME fresh tree and calls
	// Parent() on every node — so the first deferred parent-link wiring happens
	// CONCURRENTLY (no single-threaded pre-walk that would wire links first).
	// This is the realistic "parse, then fan out goroutines over the tree"
	// consumer pattern. Overlapping chains write shared ancestors' .parent.
	const goroutines = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		_ = n.Parent()
		for i := 0; i < n.NamedChildCount(); i++ {
			walk(n.NamedChild(i))
		}
	}
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			walk(tree.RootNode())
		}()
	}
	close(start)
	wg.Wait()
}

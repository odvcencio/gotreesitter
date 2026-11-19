//go:build !grammar_subset

package grammars

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// maxNestedErrorDepth returns the length of the longest chain of ERROR nodes
// with children that are direct children of each other (1 = no such ERROR
// inside another). Childless ERROR leaves (skipped bytes) do not count; C
// keeps those as children of the recovery ERROR too.
func maxNestedErrorDepth(n *gotreesitter.Node, lang *gotreesitter.Language) int {
	best := 0
	var walk func(n *gotreesitter.Node, run int)
	walk = func(n *gotreesitter.Node, run int) {
		if n.Type(lang) == "ERROR" && n.ChildCount() > 0 {
			run++
			if run > best {
				best = run
			}
		} else {
			run = 0
		}
		for i := 0; i < n.ChildCount(); i++ {
			if c := n.Child(i); c != nil {
				walk(c, run)
			}
		}
	}
	walk(n, 0)
	return best
}

// TestWolframResyncRetryKeepsOneFlatError checks that a top-level resync
// splices a leading recovery ERROR's children into the new ERROR, as C's
// ts_parser__recover_to_state does (C/parser.c, ts_stack_pop_error splice),
// instead of nesting one ERROR per retry.
func TestWolframResyncRetryKeepsOneFlatError(t *testing.T) {
	lang := WolframLanguage()
	read := func(name string) []byte {
		src, err := os.ReadFile(filepath.Join("../testdata/retired_dispatch_arms/wolfram", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return src
	}
	large := read("large__EvaluationUtilities.wl")
	medium := read("medium__OutputHandlingUtilities.wl")
	cases := []struct {
		name string
		src  []byte
	}{
		{"medium", medium},
		{"medium-no-final-newline", bytes.TrimRight(medium, "\r\n")},
		{"large", large},
		{"large-x4", bytes.Repeat(large, 4)},
		{"empty", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan int, 1)
			go func() {
				p := gotreesitter.NewParser(lang)
				tree, err := p.Parse(tc.src)
				if err != nil {
					t.Errorf("parse: %v", err)
					done <- -1
					return
				}
				done <- maxNestedErrorDepth(tree.RootNode(), lang)
			}()
			select {
			case depth := <-done:
				if depth > 1 {
					t.Fatalf("deepest ERROR-inside-ERROR chain = %d, want <= 1", depth)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("parse timed out")
			}
		})
	}
}

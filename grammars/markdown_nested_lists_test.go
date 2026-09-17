package grammars_test

import (
	"fmt"
	"strings"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestMarkdownNestedListClosures(t *testing.T) {
	for _, depth := range []int{4, 5, 8, 32} {
		for _, indent := range []int{2, 4} {
			for _, tail := range []string{"", "\nFollowing paragraph.\n", "* Following item.\n"} {
				t.Run(fmt.Sprintf("depth%d/indent%d/tail%q", depth, indent, tail), func(t *testing.T) {
					// A leading paragraph keeps this test independent of the
					// grammar's existing first-block wrapper normalization.
					var input strings.Builder
					input.WriteString("Preamble.\n\n")
					for level := range depth {
						fmt.Fprintf(&input, "%s* Level %d: The client checks scopes.\n", strings.Repeat(" ", level*indent), level+1)
					}
					input.WriteString(tail)
					source := []byte(input.String())
					lang := grammars.MarkdownLanguage()
					parser := ts.NewParser(lang)
					parser.SetTimeoutMicros(5000000)
					tree, err := parser.ParseStrict(source)
					if err != nil {
						t.Fatalf("ParseStrict: %v", err)
					}
					if tree == nil {
						t.Fatal("nil tree")
					}
					defer tree.Release()
					root := tree.RootNode()
					if root == nil || root.HasErrorOrMissing() || tree.ParseStoppedEarly() {
						t.Fatalf("incomplete list parse: %s", tree.ParseRuntime().Summary())
					}
					if root.EndByte() != uint32(len(source)) {
						t.Fatalf("root ends at %d, want %d", root.EndByte(), len(source))
					}
					count, maxDepth := 0, 0
					ts.Walk(root, func(node *ts.Node, _ int) ts.WalkAction {
						if node.Type(lang) != "list_item" {
							return ts.WalkContinue
						}
						count++
						nesting := 1
						for parent := node.Parent(); parent != nil; parent = parent.Parent() {
							if parent.Type(lang) == "list_item" {
								nesting++
							}
						}
						if nesting > maxDepth {
							maxDepth = nesting
						}
						return ts.WalkContinue
					})
					want := depth
					if strings.HasPrefix(tail, "*") {
						want++
					}
					if count != want || maxDepth != depth {
						t.Fatalf("items=%d depth=%d; want items=%d depth=%d", count, maxDepth, want, depth)
					}
				})
			}
		}
	}
}

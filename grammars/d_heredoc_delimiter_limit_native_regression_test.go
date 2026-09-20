//go:build !grammar_subset

package grammars

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestDHeredocDelimiterLimit covers tree-sitter-d@7c8c31c: a heredoc string
// (q"IDENT ... IDENT") with a 256-character delimiter still parses cleanly,
// and a 257-character delimiter is rejected as a heredoc string, matching
// upstream's fixed scanner instead of the pre-fix unbounded C buffer write.
func TestDHeredocDelimiterLimit(t *testing.T) {
	tests := []struct {
		name      string
		delimLen  int
		wantError bool
	}{
		{name: "short delimiter", delimLen: 1, wantError: false},
		{name: "at limit", delimLen: 256, wantError: false},
		{name: "one over limit", delimLen: 257, wantError: true},
	}

	language := DLanguage()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ident := strings.Repeat("a", test.delimLen)
			source := []byte(fmt.Sprintf("void f() { auto s = q\"%s\nhello\n%s\"; }\n", ident, ident))

			tree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			if got := root.HasError(); got != test.wantError {
				t.Fatalf("delimLen=%d root.HasError() = %v, want %v: %s", test.delimLen, got, test.wantError, root.SExpr(language))
			}
		})
	}
}

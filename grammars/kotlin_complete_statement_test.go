//go:build !grammar_subset || grammar_subset_kotlin

package grammars

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestKotlinCompleteStatementsParseWithoutErrors preserves complete statement
// alternatives across generic-call and indexed-assignment ambiguities.
func TestKotlinCompleteStatementsParseWithoutErrors(t *testing.T) {
	entry := DetectLanguageByName("kotlin")
	language := entry.Language()
	files, readError := os.ReadDir("testdata/kotlin_complete_statements")
	if readError != nil {
		t.Fatal(readError)
	}
	for _, file := range files {
		t.Run(file.Name(), func(t *testing.T) {
			source, readError := os.ReadFile(filepath.Join("testdata/kotlin_complete_statements", file.Name()))
			if readError != nil {
				t.Fatal(readError)
			}
			parser := gotreesitter.NewParser(language)
			tree, parseError := parser.ParseStrict(source)
			if parseError != nil {
				t.Fatal(parseError)
			}
			t.Cleanup(tree.Release)
			root := tree.RootNode()
			runtime := tree.ParseRuntime()
			t.Logf("tokens=%d nodes=%d max_stacks=%d", runtime.TokensConsumed, runtime.NodesAllocated, runtime.MaxStacksSeen)
			if root.HasErrorOrMissing() {
				t.Fatalf("valid Kotlin contains an error or missing node:\n%s", root.SExpr(language))
			}
			if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
				t.Fatalf("root covers %d..%d, want 0..%d", root.StartByte(), root.EndByte(), len(source))
			}
		})
	}
}

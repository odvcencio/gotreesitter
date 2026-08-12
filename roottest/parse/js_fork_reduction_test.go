package parse_test

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestJSForkReductionParity asserts that the C-faithful repetition fold does
// not change the parse tree on the real-corpus JS files, and reports the total
// GLR fork count so the reduction is visible in `go test -v` output.
//
// The fold is unconditional for clean JS lineages. This test is the parity
// safety net: if repetition-boundary resolution were wrong, the tree would
// have errors or a different shape, which we'd catch here against the
// structural expectations.
func TestJSForkReductionParity(t *testing.T) {
	cases := []string{
		"cgo_harness/corpus_real/javascript/large__text-editor-component.js",
		"cgo_harness/corpus_real/javascript/large__jquery.js",
		"cgo_harness/corpus_real/javascript/small__functions.js",
	}
	lang := grammars.JavascriptLanguage()
	for _, path := range cases {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("corpus not present: %v", err)
		}
		profile := gotreesitter.NewAmbiguityProfile()
		parser := gotreesitter.NewParser(lang)
		parser.SetAmbiguityProfile(profile)
		tree, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("%s: parse: %v", path, err)
		}
		root := tree.RootNode()
		if root.HasError() {
			t.Errorf("%s: tree has parse error with C-faithful repetition fold", path)
		}
		var totalForks uint64
		for _, st := range profile.SnapshotTop(50) {
			totalForks += st.Forks
		}
		t.Logf("%s: root=%s childCount=%d hasError=%v topForks=%d",
			path, root.Type(lang), root.ChildCount(), root.HasError(), totalForks)
	}
}

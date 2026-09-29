package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAgdaValidLayoutMatchesC pins valid function heads whose repetition
// conflict must take the grammar-table reduction instead of the generated
// repetition-shift optimization. Expected trees come from the pinned C oracle;
// they are written in Go SExpr form, which prints no field names.
func TestAgdaValidLayoutMatchesC(t *testing.T) {
	entry := grammars.DetectLanguageByName("agda")
	if entry == nil || entry.Language() == nil {
		t.Skip("agda unavailable in this build")
	}
	lang := entry.Language()
	const want = "(source_file (function (lhs (atom (qid)) (atom (qid)) (atom (qid)))))"
	for _, src := range []string{"o o N", "= t r", "ψ A u", "ᴹ U ⊥"} {
		tree, err := gotreesitter.NewParser(lang).Parse([]byte(src))
		if err != nil {
			t.Fatalf("%q: parse: %v", src, err)
		}
		root := tree.RootNode()
		if got := root.SExpr(lang); got != want || root.HasError() {
			t.Errorf("%q: got %s (has error %v), want %s", src, got, root.HasError(), want)
		}
	}
}

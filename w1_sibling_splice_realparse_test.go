package gotreesitter_test

// Campaign O(edit) workstream W1 (spec.campaign.oedit) real-parse companion
// to w1_sibling_splice_test.go's fixture-level proofs. See that file's
// package doc comment for the full picture.

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestRealParseFragileGateCounterFiresOnAmbiguousGoTopLevelDecls is the
// real-parse, end-to-end proof a prior Lane-1 review flagged as missing: Go's
// own grammar has a genuine local ambiguity in untyped-vs-typed parameter
// lists (`func f(a, b int)`), which the GLR engine resolves via table
// conflict / multi-version dispatch on every occurrence -- markReduceFragility
// (parser_reduce.go) marks the resulting function_declaration fragile on a
// real parse, not just in a synthetic fixture. This drives
// IncrementalParseProfile.ReuseRejectFragileNonLeaf > 0 on an ordinary
// incremental edit, proving the counter is live end to end (not just at the
// tryReuseSubtree-unit level, see w1_sibling_splice_test.go) and therefore
// cannot be silently neutered by whatever reduce/merge path a real GLR
// dispatch takes.
func TestRealParseFragileGateCounterFiresOnAmbiguousGoTopLevelDecls(t *testing.T) {
	lang := grammars.GoLanguage()
	var src bytes.Buffer
	src.WriteString("package p\n\n")
	for i := 0; i < 40; i++ {
		src.WriteString("func f")
		src.WriteString(string(rune('A' + i%26)))
		src.WriteString(string(rune('0' + i%10)))
		src.WriteString("(a, b int) int {\n\treturn a + b\n}\n\n")
	}
	oldSrc := src.Bytes()

	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(oldSrc)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	if oldTree.RootNode().HasError() {
		t.Fatal("fixture invariant: base parse must be clean")
	}
	defer oldTree.Release()

	// Single-char insert inside an identifier well past the first
	// declaration, so trailing siblings exist for the splice gate to
	// consider.
	const insertOffset = 30
	newSrc := make([]byte, 0, len(oldSrc)+1)
	newSrc = append(newSrc, oldSrc[:insertOffset]...)
	newSrc = append(newSrc, 'x')
	newSrc = append(newSrc, oldSrc[insertOffset:]...)

	prefix := oldSrc[:insertOffset]
	row := uint32(bytes.Count(prefix, []byte{'\n'}))
	col := uint32(insertOffset)
	if idx := bytes.LastIndexByte(prefix, '\n'); idx >= 0 {
		col = uint32(insertOffset - idx - 1)
	}
	startPoint := gotreesitter.Point{Row: row, Column: col}
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte: insertOffset, OldEndByte: insertOffset, NewEndByte: insertOffset + 1,
		StartPoint:  startPoint,
		OldEndPoint: startPoint,
		NewEndPoint: gotreesitter.Point{Row: row, Column: col + 1},
	})

	newTree, profile, err := parser.ParseIncrementalProfiled(newSrc, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()

	if profile.ReuseRejectFragileNonLeaf == 0 {
		t.Fatal("expected ReuseRejectFragileNonLeaf > 0 on a real parse of Go's genuinely ambiguous " +
			"untyped-parameter-list construct -- the fragility gate did not fire on any real candidate")
	}
}

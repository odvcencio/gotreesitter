package highlight_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestIssue23_IncrementalHighlightAfterSequentialDeletions reproduces the
// original bug report: HighlightIncremental returned fewer ranges than
// Highlight after two sequential single-character deletions.
//
// Root cause: the incremental reuse cursor offered leaf nodes from under
// dirty ancestors with stale parser-state metadata, causing the parser to
// produce a partial/overlapping tree.
func TestIssue23_IncrementalHighlightAfterSequentialDeletions(t *testing.T) {
	lang := grammars.GoLanguage()
	entry := grammars.DetectLanguage("test.go")
	opts := []gotreesitter.HighlighterOption{}
	if entry.TokenSourceFactory != nil {
		// Prefer the registered token source factory when one exists (ts2go
		// Go blob carries a ts2go-calibrated factory). The grammargen blob
		// shipped in 0.14.0 does not register a custom factory and uses the
		// baked DFA; leave opts empty in that case.
		opts = append(opts, gotreesitter.WithTokenSourceFactory(func(src []byte) gotreesitter.TokenSource {
			return entry.TokenSourceFactory(src, lang)
		}))
	}
	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
	if err != nil {
		t.Fatal(err)
	}

	src := []byte("package main\n\nfunc a() {\n\tx := ab.C()\n}\n\nfunc b() {}\n")

	_, tree := hl.HighlightIncremental(src, nil)

	del := func(s []byte, i int) []byte { return append(append([]byte{}, s[:i]...), s[i+1:]...) }

	// Delete byte at index 31 ("a" from "ab")
	src = del(src, 31)
	tree.Edit(gotreesitter.InputEdit{
		StartByte: 31, OldEndByte: 32, NewEndByte: 31,
		StartPoint:  gotreesitter.Point{Row: 3, Column: 7},
		OldEndPoint: gotreesitter.Point{Row: 3, Column: 8},
		NewEndPoint: gotreesitter.Point{Row: 3, Column: 7},
	})
	_, tree = hl.HighlightIncremental(src, tree)

	// Delete byte at index 30 (space after :=)
	src = del(src, 30)
	tree.Edit(gotreesitter.InputEdit{
		StartByte: 30, OldEndByte: 31, NewEndByte: 30,
		StartPoint:  gotreesitter.Point{Row: 3, Column: 6},
		OldEndPoint: gotreesitter.Point{Row: 3, Column: 7},
		NewEndPoint: gotreesitter.Point{Row: 3, Column: 6},
	})
	incRanges, _ := hl.HighlightIncremental(src, tree)
	fullRanges := hl.Highlight(src)

	if len(incRanges) != len(fullRanges) {
		t.Errorf("incremental=%d full=%d — want equal", len(incRanges), len(fullRanges))
	}
}

// TestIssue23_IncrementalTreeMatchesFullParse verifies that the incremental
// parse tree structure matches a fresh full parse after a deletion that
// removes whitespace between tokens (e.g., ":= b" → ":=b").
func TestIssue23_IncrementalTreeMatchesFullParse(t *testing.T) {
	lang := grammars.GoLanguage()
	p := gotreesitter.NewParser(lang)
	del := func(s []byte, i int) []byte { return append(append([]byte{}, s[:i]...), s[i+1:]...) }

	src := []byte("package main\n\nfunc a() {\n\tx := b.C()\n}\n\nfunc b() {}\n")

	tree, err := p.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	// Delete space after := (byte 30)
	src = del(src, 30)
	tree.Edit(gotreesitter.InputEdit{
		StartByte: 30, OldEndByte: 31, NewEndByte: 30,
		StartPoint:  gotreesitter.Point{Row: 3, Column: 5},
		OldEndPoint: gotreesitter.Point{Row: 3, Column: 6},
		NewEndPoint: gotreesitter.Point{Row: 3, Column: 5},
	})
	incTree, err := p.ParseIncremental(src, tree)
	if err != nil {
		t.Fatal(err)
	}

	p2 := gotreesitter.NewParser(lang)
	fullTree, _ := p2.Parse(src)

	incSExpr := incTree.RootNode().SExpr(lang)
	fullSExpr := fullTree.RootNode().SExpr(lang)

	if incSExpr != fullSExpr {
		t.Errorf("incremental and full parse trees differ\n  inc:  %s\n  full: %s", incSExpr, fullSExpr)
	}
}

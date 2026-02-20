package gotreesitter

import "testing"

func TestTreeEditShiftsNodes(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Parse "1+2"
	tree := parser.Parse([]byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}

	// Simulate inserting "0" before "1": "01+2"
	// Edit: at byte 0, old end 0, new end 1 (inserted 1 byte)
	tree.Edit(InputEdit{
		StartByte:   0,
		OldEndByte:  0,
		NewEndByte:  1,
		StartPoint:  Point{0, 0},
		OldEndPoint: Point{0, 0},
		NewEndPoint: Point{0, 1},
	})

	// After edit, the root's end should shift by 1.
	if root.EndByte() != 4 {
		t.Errorf("root EndByte after edit = %d, want 4", root.EndByte())
	}

	// The edit should be recorded.
	if len(tree.Edits()) != 1 {
		t.Fatalf("expected 1 edit recorded, got %d", len(tree.Edits()))
	}
}

func TestParseIncremental(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Parse "1+2"
	tree := parser.Parse([]byte("1+2"))

	// Edit: change to "1+3"
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	// Incremental re-parse with new source.
	newTree := parser.ParseIncremental([]byte("1+3"), tree)
	root := newTree.RootNode()
	if root == nil {
		t.Fatal("incremental parse returned nil root")
	}

	// Should have the same structure: expression(expression(NUMBER), +, NUMBER)
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	num := root.Child(2)
	if num.Text(newTree.Source()) != "3" {
		t.Errorf("changed NUMBER text = %q, want %q", num.Text(newTree.Source()), "3")
	}
}

func TestHighlightIncremental(t *testing.T) {
	lang := buildArithmeticLanguage()

	// Simple highlight query: capture NUMBER nodes.
	h, err := NewHighlighter(lang, `(NUMBER) @number`)
	if err != nil {
		t.Fatal(err)
	}

	// Initial highlight.
	source1 := []byte("1+2")
	ranges1 := h.Highlight(source1)
	if len(ranges1) < 2 {
		t.Fatalf("expected at least 2 highlight ranges, got %d", len(ranges1))
	}

	// Parse for incremental use.
	parser := NewParser(lang)
	tree := parser.Parse(source1)

	// Edit: "1+2" -> "1+3"
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	source2 := []byte("1+3")
	ranges2, newTree := h.HighlightIncremental(source2, tree)
	if newTree == nil {
		t.Fatal("HighlightIncremental returned nil tree")
	}

	// Should still have at least 2 number ranges.
	if len(ranges2) < 2 {
		t.Fatalf("expected at least 2 incremental highlight ranges, got %d", len(ranges2))
	}

	// Verify the captures are "number".
	for _, r := range ranges2 {
		if r.Capture != "number" {
			t.Errorf("unexpected capture %q, want %q", r.Capture, "number")
		}
	}
}

func TestParseIncrementalReusesUnchangedLeaf(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	oldSource := []byte("1+2+3")
	tree := parser.Parse(oldSource)
	root := tree.RootNode()
	if root == nil {
		t.Fatal("initial parse returned nil root")
	}
	oldRight := root.Child(2)
	if oldRight == nil {
		t.Fatal("missing right child in initial tree")
	}

	// Edit the middle number: "1+2+3" -> "1+4+3"
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{0, 2},
		OldEndPoint: Point{0, 3},
		NewEndPoint: Point{0, 3},
	})

	newSource := []byte("1+4+3")
	newTree := parser.ParseIncremental(newSource, tree)
	newRoot := newTree.RootNode()
	if newRoot == nil {
		t.Fatal("incremental parse returned nil root")
	}
	newRight := newRoot.Child(2)
	if newRight == nil {
		t.Fatal("missing right child in incremental tree")
	}

	if newRight != oldRight {
		t.Fatal("expected unchanged right leaf node to be reused")
	}
	if got := newRight.Text(newTree.Source()); got != "3" {
		t.Fatalf("reused leaf text = %q, want %q", got, "3")
	}
}

func TestParseIncrementalReusesRootWhenUnchanged(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	source := []byte("1+2")
	tree := parser.Parse(source)
	if tree.RootNode() == nil {
		t.Fatal("initial parse returned nil root")
	}

	// No edits: incremental parse should be able to reuse the whole root subtree.
	newTree := parser.ParseIncremental(source, tree)
	if newTree.RootNode() == nil {
		t.Fatal("incremental parse returned nil root")
	}

	if newTree.RootNode() != tree.RootNode() {
		t.Fatal("expected root node to be reused when there are no edits")
	}
}

func TestTreeEditNodesAfterEdit(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := parser.Parse([]byte("1+2+3"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}

	origEnd := root.EndByte()

	// Delete the "+3" at end: "1+2+3" -> "1+2"
	// Edit: start=3, oldEnd=5, newEnd=3
	tree.Edit(InputEdit{
		StartByte:   3,
		OldEndByte:  5,
		NewEndByte:  3,
		StartPoint:  Point{0, 3},
		OldEndPoint: Point{0, 5},
		NewEndPoint: Point{0, 3},
	})

	// Root should shrink.
	if root.EndByte() != 3 {
		t.Errorf("root EndByte after deletion = %d, want 3 (was %d)", root.EndByte(), origEnd)
	}
}

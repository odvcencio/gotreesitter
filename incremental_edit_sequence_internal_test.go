package gotreesitter

import "testing"

func TestIncrementalEditsRestoreNodeSpans(t *testing.T) {
	insert := func(start uint32, text string) InputEdit {
		end := start + uint32(len(text))
		return InputEdit{StartByte: start, OldEndByte: start, NewEndByte: end,
			StartPoint: Point{Column: start}, OldEndPoint: Point{Column: start}, NewEndPoint: Point{Column: end}}
	}
	remove := func(start, end uint32) InputEdit {
		return InputEdit{StartByte: start, OldEndByte: end, NewEndByte: start,
			StartPoint: Point{Column: start}, OldEndPoint: Point{Column: end}, NewEndPoint: Point{Column: start}}
	}
	for _, tc := range []struct {
		name  string
		edits []InputEdit
		want  bool
	}{
		{"no edits", nil, true},
		{"insertions only", []InputEdit{insert(3, "a"), insert(7, "bc")}, true},
		{"type then delete", []InputEdit{insert(3, "a"), remove(3, 4)}, true},
		{"nested undo", []InputEdit{insert(3, "a"), insert(9, "b"), remove(9, 10), remove(3, 4)}, true},
		{"delete then reinsert", []InputEdit{remove(5, 7), insert(5, "20")}, false},
		{"delete other range", []InputEdit{insert(3, "a"), remove(2, 3)}, false},
		{"wide replacement", []InputEdit{{StartByte: 5, OldEndByte: 7, NewEndByte: 7}}, false},
		{"one-byte replacement and inverse", []InputEdit{
			{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartPoint: Point{Column: 2}, OldEndPoint: Point{Column: 3}, NewEndPoint: Point{Column: 3}},
			{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartPoint: Point{Column: 2}, OldEndPoint: Point{Column: 3}, NewEndPoint: Point{Column: 3}},
		}, true},
		{"one-byte replacement that adds a line", []InputEdit{
			{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartPoint: Point{Column: 2}, OldEndPoint: Point{Column: 3}, NewEndPoint: Point{Row: 1}},
		}, false},
	} {
		if got := incrementalEditsRestoreNodeSpans(tc.edits); got != tc.want {
			t.Errorf("%s: incrementalEditsRestoreNodeSpans = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIncludedRangesMatchTree(t *testing.T) {
	r := func(start, end uint32) Range { return Range{StartByte: start, EndByte: end} }
	tree := &Tree{includedRanges: []Range{r(0, 7), r(9, 9)}}
	if !includedRangesMatchTree(tree, []Range{r(0, 7)}) {
		t.Fatal("empty old range must not count as a difference")
	}
	if includedRangesMatchTree(tree, []Range{r(0, 14)}) {
		t.Fatal("a wider parser range must count as a difference")
	}
	if includedRangesMatchTree(&Tree{}, []Range{r(0, 7)}) {
		t.Fatal("new parser ranges on a tree without ranges must count as a difference")
	}
	if !includedRangesMatchTree(nil, nil) {
		t.Fatal("no ranges on either side must match")
	}
}

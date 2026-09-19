package gotreesitter

import "testing"

// TestOffsetDirectiveShrinksRangeExecutePath exercises #offset! through the
// reader/non-streaming matcher path (Query.Execute -> executeQueryWithReader
// in query_reader.go / query_matcher_generic.go). It shrinks the identifier
// "main" (bytes [5,9)) by one column at each end, expecting the reported
// capture range to become bytes [6,8) ("ai") while the underlying node keeps
// its own, unmodified range.
func TestOffsetDirectiveShrinksRangeExecutePath(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`((identifier) @ident (#offset! @ident 0 1 0 -1))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if len(matches[0].Captures) != 1 {
		t.Fatalf("captures: got %d, want 1", len(matches[0].Captures))
	}
	capture := matches[0].Captures[0]

	// The node itself is immutable and still reports its original range.
	if capture.Node.StartByte() != 5 || capture.Node.EndByte() != 9 {
		t.Fatalf("node range changed unexpectedly: got [%d,%d)", capture.Node.StartByte(), capture.Node.EndByte())
	}

	start, end := capture.ByteRange()
	if start != 6 || end != 8 {
		t.Fatalf("ByteRange: got [%d,%d), want [6,8)", start, end)
	}
	if got, want := capture.Text(tree.Source()), "ai"; got != want {
		t.Fatalf("Text: got %q, want %q", got, want)
	}

	startPoint, endPoint := capture.PointRange()
	if startPoint != (Point{Row: 0, Column: 6}) || endPoint != (Point{Row: 0, Column: 8}) {
		t.Fatalf("PointRange: got [%v,%v)", startPoint, endPoint)
	}
}

// TestOffsetDirectiveShrinksRangeCursorPath exercises the same #offset!
// directive through the streaming cursor path (Query.Exec / NextMatch in
// query_matcher.go).
func TestOffsetDirectiveShrinksRangeCursorPath(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`((identifier) @ident (#offset! @ident 0 1 0 -1))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), lang, tree.Source())
	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("NextMatch: expected a match")
	}
	if len(match.Captures) != 1 {
		t.Fatalf("captures: got %d, want 1", len(match.Captures))
	}
	capture := match.Captures[0]

	start, end := capture.ByteRange()
	if start != 6 || end != 8 {
		t.Fatalf("ByteRange: got [%d,%d), want [6,8)", start, end)
	}
	if got, want := capture.Text(tree.Source()), "ai"; got != want {
		t.Fatalf("Text: got %q, want %q", got, want)
	}

	if _, ok := cursor.NextMatch(); ok {
		t.Fatal("expected exactly one match")
	}
}

// TestOffsetDirectiveShrinksRangeExecuteIntoPath exercises ExecuteInto,
// which also runs through the streaming cursor path via executeNodeInto.
func TestOffsetDirectiveShrinksRangeExecuteIntoPath(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`((identifier) @ident (#offset! @ident 0 1 0 -1))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	var dst []QueryMatch
	dst = q.ExecuteInto(tree, dst)
	if len(dst) != 1 || len(dst[0].Captures) != 1 {
		t.Fatalf("ExecuteInto matches: got %+v", dst)
	}
	start, end := dst[0].Captures[0].ByteRange()
	if start != 6 || end != 8 {
		t.Fatalf("ByteRange via ExecuteInto: got [%d,%d), want [6,8)", start, end)
	}
}

// TestOffsetDirectiveNoOffsetLeavesRangeUnchanged is a control: a capture
// with no #offset! directive reports the node's own range from ByteRange
// and PointRange.
func TestOffsetDirectiveNoOffsetLeavesRangeUnchanged(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @ident`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 || len(matches[0].Captures) != 1 {
		t.Fatalf("matches: got %+v", matches)
	}
	capture := matches[0].Captures[0]
	start, end := capture.ByteRange()
	if start != 5 || end != 9 {
		t.Fatalf("ByteRange: got [%d,%d), want [5,9)", start, end)
	}
}

// TestHighlighterOffsetDirectiveShrinksHighlightRange shows that
// Highlighter.Highlight reports the #offset!-adjusted range, applied
// independently to each match, rather than each NUMBER node's own range.
func TestHighlighterOffsetDirectiveShrinksHighlightRange(t *testing.T) {
	lang := buildArithmeticLanguage()
	h, err := NewHighlighter(lang, `((NUMBER) @number (#offset! @number 0 0 0 -1))`)
	if err != nil {
		t.Fatalf("NewHighlighter: %v", err)
	}

	source := []byte("42+77")
	ranges := h.Highlight(source)
	if len(ranges) != 2 {
		t.Fatalf("ranges: got %d, want 2: %+v", len(ranges), ranges)
	}

	if ranges[0].StartByte != 0 || ranges[0].EndByte != 1 {
		t.Fatalf("range[0]: got [%d,%d), want [0,1)", ranges[0].StartByte, ranges[0].EndByte)
	}
	if got, want := string(source[ranges[0].StartByte:ranges[0].EndByte]), "4"; got != want {
		t.Fatalf("range[0] text: got %q, want %q", got, want)
	}

	if ranges[1].StartByte != 3 || ranges[1].EndByte != 4 {
		t.Fatalf("range[1]: got [%d,%d), want [3,4)", ranges[1].StartByte, ranges[1].EndByte)
	}
	if got, want := string(source[ranges[1].StartByte:ranges[1].EndByte]), "7"; got != want {
		t.Fatalf("range[1] text: got %q, want %q", got, want)
	}
}

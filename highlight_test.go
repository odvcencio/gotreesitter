package gotreesitter

import (
	"testing"
)

// --------------------------------------------------------------------------
// Unit tests with hand-built trees
// --------------------------------------------------------------------------

func TestHighlightBasic(t *testing.T) {
	lang := queryTestLanguage()
	// Tree: program > [func(kw), identifier("main"), number("42")]
	funcKw := leaf(Symbol(8), false, 0, 4) // "func"
	ident := leaf(Symbol(1), true, 5, 9)   // "main"
	num := leaf(Symbol(2), true, 10, 12)   // "42"
	program := parent(Symbol(7), true,
		[]*Node{funcKw, ident, num},
		[]FieldID{0, 0, 0})
	source := []byte("func main 42")
	_ = NewTree(program, source, lang)

	h, err := NewHighlighter(lang, `
"func" @keyword
(identifier) @variable
(number) @number
`)
	if err != nil {
		t.Fatalf("NewHighlighter error: %v", err)
	}

	// We need to parse from source, but since we're using queryTestLanguage
	// with no parse tables, the parser will produce an error tree. Instead,
	// test the Highlight method indirectly by testing resolveOverlaps and
	// the query engine separately for hand-built trees.
	//
	// For direct unit testing of the Highlight pipeline with hand-built trees,
	// we test the helper functions. Full integration uses the Go grammar.
	_ = h
}

func TestHighlightRangeSorting(t *testing.T) {
	// Test that resolveOverlaps produces ranges sorted by StartByte.
	ranges := []HighlightRange{
		{StartByte: 10, EndByte: 15, Capture: "number"},
		{StartByte: 0, EndByte: 4, Capture: "keyword"},
		{StartByte: 5, EndByte: 9, Capture: "variable"},
	}
	result := resolveOverlaps(ranges)

	// Since these don't overlap, all should be preserved, sorted by StartByte.
	if len(result) != 3 {
		t.Fatalf("expected 3 ranges, got %d", len(result))
	}
	for i := 1; i < len(result); i++ {
		if result[i].StartByte < result[i-1].StartByte {
			t.Errorf("ranges not sorted: [%d].StartByte=%d < [%d].StartByte=%d",
				i, result[i].StartByte, i-1, result[i-1].StartByte)
		}
	}
	if result[0].Capture != "keyword" {
		t.Errorf("result[0].Capture = %q, want %q", result[0].Capture, "keyword")
	}
	if result[1].Capture != "variable" {
		t.Errorf("result[1].Capture = %q, want %q", result[1].Capture, "variable")
	}
	if result[2].Capture != "number" {
		t.Errorf("result[2].Capture = %q, want %q", result[2].Capture, "number")
	}
}

func TestHighlightOverlappingRangesInnerWins(t *testing.T) {
	// Outer range: bytes 0-20, capture "function"
	// Inner range: bytes 5-9, capture "keyword"
	// Expected: [0-5: function], [5-9: keyword], [9-20: function]
	ranges := []HighlightRange{
		{StartByte: 0, EndByte: 20, Capture: "function"},
		{StartByte: 5, EndByte: 9, Capture: "keyword"},
	}
	result := resolveOverlaps(ranges)

	if len(result) != 3 {
		t.Fatalf("expected 3 ranges, got %d: %+v", len(result), result)
	}

	// First segment: 0-5 function
	if result[0].StartByte != 0 || result[0].EndByte != 5 || result[0].Capture != "function" {
		t.Errorf("result[0] = %+v, want {0, 5, function}", result[0])
	}
	// Second segment: 5-9 keyword (inner wins)
	if result[1].StartByte != 5 || result[1].EndByte != 9 || result[1].Capture != "keyword" {
		t.Errorf("result[1] = %+v, want {5, 9, keyword}", result[1])
	}
	// Third segment: 9-20 function (outer resumes)
	if result[2].StartByte != 9 || result[2].EndByte != 20 || result[2].Capture != "function" {
		t.Errorf("result[2] = %+v, want {9, 20, function}", result[2])
	}
}

func TestHighlightMultipleOverlaps(t *testing.T) {
	// Outer: 0-30 "function"
	// Mid:   5-25 "type"
	// Inner: 10-15 "keyword"
	// Expected: [0-5: function], [5-10: type], [10-15: keyword], [15-25: type], [25-30: function]
	ranges := []HighlightRange{
		{StartByte: 0, EndByte: 30, Capture: "function"},
		{StartByte: 5, EndByte: 25, Capture: "type"},
		{StartByte: 10, EndByte: 15, Capture: "keyword"},
	}
	result := resolveOverlaps(ranges)

	expected := []HighlightRange{
		{StartByte: 0, EndByte: 5, Capture: "function"},
		{StartByte: 5, EndByte: 10, Capture: "type"},
		{StartByte: 10, EndByte: 15, Capture: "keyword"},
		{StartByte: 15, EndByte: 25, Capture: "type"},
		{StartByte: 25, EndByte: 30, Capture: "function"},
	}

	if len(result) != len(expected) {
		t.Fatalf("expected %d ranges, got %d: %+v", len(expected), len(result), result)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Errorf("result[%d] = %+v, want %+v", i, result[i], want)
		}
	}
}

func TestHighlightAdjacentNonOverlapping(t *testing.T) {
	// Adjacent ranges that don't overlap should pass through unchanged.
	ranges := []HighlightRange{
		{StartByte: 0, EndByte: 5, Capture: "keyword"},
		{StartByte: 5, EndByte: 10, Capture: "variable"},
		{StartByte: 10, EndByte: 15, Capture: "number"},
	}
	result := resolveOverlaps(ranges)

	if len(result) != 3 {
		t.Fatalf("expected 3 ranges, got %d: %+v", len(result), result)
	}
	for i, want := range ranges {
		if result[i] != want {
			t.Errorf("result[%d] = %+v, want %+v", i, result[i], want)
		}
	}
}

func TestHighlightSameStartInnerWins(t *testing.T) {
	// Two ranges starting at the same byte: the inner (narrower) should win.
	// Outer: 0-20, "function"
	// Inner: 0-5, "keyword"
	// Expected: [0-5: keyword], [5-20: function]
	ranges := []HighlightRange{
		{StartByte: 0, EndByte: 20, Capture: "function"},
		{StartByte: 0, EndByte: 5, Capture: "keyword"},
	}
	result := resolveOverlaps(ranges)

	if len(result) != 2 {
		t.Fatalf("expected 2 ranges, got %d: %+v", len(result), result)
	}
	if result[0].StartByte != 0 || result[0].EndByte != 5 || result[0].Capture != "keyword" {
		t.Errorf("result[0] = %+v, want {0, 5, keyword}", result[0])
	}
	if result[1].StartByte != 5 || result[1].EndByte != 20 || result[1].Capture != "function" {
		t.Errorf("result[1] = %+v, want {5, 20, function}", result[1])
	}
}

func TestHighlightEmptyRanges(t *testing.T) {
	result := resolveOverlaps(nil)
	if result != nil {
		t.Fatalf("expected nil for empty input, got %+v", result)
	}

	result = resolveOverlaps([]HighlightRange{})
	if result != nil {
		t.Fatalf("expected nil for empty slice, got %+v", result)
	}
}

func TestHighlighterEmptySource(t *testing.T) {
	lang := queryTestLanguage()
	h, err := NewHighlighter(lang, `(identifier) @ident`)
	if err != nil {
		t.Fatalf("NewHighlighter error: %v", err)
	}

	result := h.Highlight(nil)
	if result != nil {
		t.Fatalf("expected nil for nil source, got %+v", result)
	}

	result = h.Highlight([]byte{})
	if result != nil {
		t.Fatalf("expected nil for empty source, got %+v", result)
	}
}

func TestHighlighterInvalidQuery(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewHighlighter(lang, `(nonexistent_type) @x`)
	if err == nil {
		t.Fatal("expected error for invalid query")
	}
}

func TestHighlighterWithTokenSourceFactory(t *testing.T) {
	lang := queryTestLanguage()
	factoryCalled := false
	factory := func(source []byte) TokenSource {
		factoryCalled = true
		// Return a simple token source that produces EOF immediately.
		return &eofTokenSource{pos: uint32(len(source))}
	}

	h, err := NewHighlighter(lang, `(identifier) @ident`, WithTokenSourceFactory(factory))
	if err != nil {
		t.Fatalf("NewHighlighter error: %v", err)
	}

	// Highlight should call the factory.
	h.Highlight([]byte("test"))
	if !factoryCalled {
		t.Error("expected token source factory to be called")
	}
}

// eofTokenSource is a minimal TokenSource that returns EOF immediately.
type eofTokenSource struct {
	pos uint32
}

func (e *eofTokenSource) Next() Token {
	return Token{Symbol: 0, StartByte: e.pos, EndByte: e.pos}
}

func TestHighlightSingleRange(t *testing.T) {
	// Single range, no overlaps.
	ranges := []HighlightRange{
		{StartByte: 3, EndByte: 7, Capture: "keyword"},
	}
	result := resolveOverlaps(ranges)
	if len(result) != 1 {
		t.Fatalf("expected 1 range, got %d: %+v", len(result), result)
	}
	if result[0] != ranges[0] {
		t.Errorf("result[0] = %+v, want %+v", result[0], ranges[0])
	}
}

func TestHighlightIdenticalRanges(t *testing.T) {
	// Two identical ranges: the second (inner/more specific) should win.
	ranges := []HighlightRange{
		{StartByte: 0, EndByte: 10, Capture: "outer"},
		{StartByte: 0, EndByte: 10, Capture: "inner"},
	}
	result := resolveOverlaps(ranges)
	// When ranges are identical, "inner" (the last one pushed) should win.
	if len(result) != 1 {
		t.Fatalf("expected 1 range, got %d: %+v", len(result), result)
	}
	if result[0].Capture != "inner" {
		t.Errorf("result[0].Capture = %q, want %q", result[0].Capture, "inner")
	}
}

// --------------------------------------------------------------------------
// Test with query execution on hand-built tree (via buildSimpleTree)
// --------------------------------------------------------------------------

func TestHighlightFromQueryMatches(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// Execute a realistic highlight query.
	q, err := NewQuery(`
"func" @keyword
(identifier) @variable
(number) @number
`, lang)
	if err != nil {
		t.Fatalf("NewQuery error: %v", err)
	}

	matches := q.Execute(tree)

	// Convert matches to HighlightRanges manually (simulating what Highlight does).
	var ranges []HighlightRange
	for _, m := range matches {
		for _, c := range m.Captures {
			node := c.Node
			if node.StartByte() == node.EndByte() {
				continue
			}
			ranges = append(ranges, HighlightRange{
				StartByte: node.StartByte(),
				EndByte:   node.EndByte(),
				Capture:   c.Name,
			})
		}
	}

	if len(ranges) == 0 {
		t.Fatal("expected highlight ranges from query execution")
	}

	// Verify we got captures for keyword, variable, and number.
	captureSet := make(map[string]bool)
	for _, r := range ranges {
		captureSet[r.Capture] = true
	}
	for _, want := range []string{"keyword", "variable", "number"} {
		if !captureSet[want] {
			t.Errorf("missing capture %q in ranges: %+v", want, ranges)
		}
	}
}

// --------------------------------------------------------------------------
// Test overlapping ranges from query execution
// --------------------------------------------------------------------------

func TestHighlightOverlapsFromQuery(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// This query captures function_declaration (wide) and identifier (narrow inside it).
	// The identifier is inside function_declaration, so it should overlap.
	q, err := NewQuery(`
(function_declaration) @function
(identifier) @variable
`, lang)
	if err != nil {
		t.Fatalf("NewQuery error: %v", err)
	}

	matches := q.Execute(tree)

	var ranges []HighlightRange
	for _, m := range matches {
		for _, c := range m.Captures {
			node := c.Node
			if node.StartByte() == node.EndByte() {
				continue
			}
			ranges = append(ranges, HighlightRange{
				StartByte: node.StartByte(),
				EndByte:   node.EndByte(),
				Capture:   c.Name,
			})
		}
	}

	// Sort like Highlight does: by StartByte asc, width desc.
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			swap := false
			if ranges[i].StartByte > ranges[j].StartByte {
				swap = true
			} else if ranges[i].StartByte == ranges[j].StartByte {
				wi := ranges[i].EndByte - ranges[i].StartByte
				wj := ranges[j].EndByte - ranges[j].StartByte
				if wi < wj {
					swap = true
				}
			}
			if swap {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}

	result := resolveOverlaps(ranges)

	// The identifier "main" (bytes 5-9) should be captured as "variable",
	// not "function", even though function_declaration covers bytes 0-19.
	for _, r := range result {
		if r.StartByte <= 5 && r.EndByte > 5 && r.EndByte <= 9 {
			// This range covers part of the identifier.
			if r.Capture == "function" && r.StartByte == 5 {
				t.Errorf("identifier range should be 'variable', not 'function': %+v", r)
			}
		}
		if r.StartByte == 5 && r.EndByte == 9 {
			if r.Capture != "variable" {
				t.Errorf("identifier at [5,9) should be 'variable', got %q", r.Capture)
			}
		}
	}
}

package gotreesitter

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestHighlighterTimeoutOptionConfiguresParser(t *testing.T) {
	highlighter, err := NewHighlighter(queryTestLanguage(), "", WithHighlighterTimeoutMicros(12_345))
	if err != nil {
		t.Fatal(err)
	}
	if got := highlighter.parser.TimeoutMicros(); got != 12_345 {
		t.Fatalf("timeout = %d, want 12345", got)
	}
}

func TestHighlightIncrementalStrictReportsTimeout(t *testing.T) {
	highlighter, err := NewHighlighter(buildArithmeticLanguage(), "",
		WithHighlighterTimeoutMicros(100),
		WithTokenSourceFactory(func(source []byte) TokenSource {
			return &slowArithmeticTokenSource{
				delay: 2 * time.Millisecond,
				tokens: []Token{
					{Symbol: 1, StartByte: 0, EndByte: 1},
					{Symbol: 0, StartByte: 1, EndByte: 1},
				},
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ranges, tree, err := highlighter.HighlightIncrementalStrict([]byte("1"), nil)
	if tree == nil {
		t.Fatal("strict highlight returned nil partial tree")
	}
	defer tree.Release()
	if !errors.Is(err, ErrParseStoppedEarly) {
		t.Fatalf("error = %v, want ErrParseStoppedEarly", err)
	}
	if got := tree.ParseStopReason(); got != ParseStopTimeout {
		t.Fatalf("stop reason = %q, want %q", got, ParseStopTimeout)
	}
	if ranges != nil {
		t.Fatalf("ranges = %#v, want nil", ranges)
	}
}

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

func TestHighlightSamePatternPrefersColorOverSpell(t *testing.T) {
	lang := queryTestLanguage()
	source := []byte("abc")
	tree := NewTree(parent(Symbol(7), true, []*Node{leaf(Symbol(1), true, 0, 3)}, []FieldID{0}), source, lang)
	for _, query := range []string{"(identifier) @comment @spell", "(identifier) @spell @comment"} {
		h, err := NewHighlighter(lang, query)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			ranges := h.highlightTree(tree, source)
			if len(ranges) != 1 || ranges[0].Capture != "comment" {
				t.Fatalf("%q: ranges = %#v", query, ranges)
			}
		}
	}
}

func TestHighlightSamePatternCaptureOrder(t *testing.T) {
	ranges := resolveOverlaps([]HighlightRange{
		{StartByte: 0, EndByte: 3, Capture: "first"},
		{StartByte: 0, EndByte: 3, Capture: "last"},
	})
	if len(ranges) != 1 || ranges[0].Capture != "last" {
		t.Fatalf("ranges = %#v", ranges)
	}
}

func BenchmarkHighlightSamePatternTies(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ranges := make([]HighlightRange, 755*2)
		for j := 0; j < 755; j++ {
			ranges[2*j] = HighlightRange{StartByte: uint32(j * 10), EndByte: uint32(j*10 + 9), Capture: "comment"}
			ranges[2*j+1] = HighlightRange{StartByte: uint32(j * 10), EndByte: uint32(j*10 + 9), Capture: "spell"}
		}
		resolveOverlaps(ranges)
	}
}

func TestHighlightSpellDoesNotOverrideOtherPattern(t *testing.T) {
	for _, ranges := range [][]HighlightRange{
		{{StartByte: 0, EndByte: 3, Capture: "comment", PatternIndex: 0}, {StartByte: 0, EndByte: 3, Capture: "spell", PatternIndex: 1}},
		{{StartByte: 0, EndByte: 3, Capture: "spell", PatternIndex: 0}, {StartByte: 0, EndByte: 3, Capture: "comment", PatternIndex: 1}},
	} {
		got := resolveOverlaps(ranges)
		if len(got) != 1 || got[0].Capture != "comment" {
			t.Fatalf("ranges = %#v", got)
		}
	}
}

func TestHighlighterAdmissionRouteForInjectedParser(t *testing.T) {
	lang := queryTestLanguage()
	plain, err := NewHighlighter(lang, "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.newInjectedParser(lang).admissionCandidateRoute != admissionRouteProductionForced {
		t.Fatal("injected parser without an option must remain on production")
	}
	for _, enabled := range []bool{false, true} {
		h, err := NewHighlighter(lang, "", WithHighlighterAdmissionCandidateRoute(enabled))
		if err != nil {
			t.Fatal(err)
		}
		if got := h.parser.admissionCandidateRouteEnabled(); got != enabled {
			t.Fatalf("document route=%v, want %v", got, enabled)
		}
		// The injected parser follows the highlighter's explicit pin, not the process default.
		if got := h.newInjectedParser(lang).admissionCandidateRouteEnabled(); got != enabled {
			t.Fatalf("injected route=%v, want %v", got, enabled)
		}
	}
}

// The capture policy tests below are exactly five top-level tests with one name
// prefix. The root race shards deal the sorted test names round-robin across five
// shards (.github/scripts/root_race_shard.sh), so five adjacent names leave every
// other test in its shard. Adding any other number of names moves heavy tests
// into one shard and can push it past its time limit. Keep the count a multiple
// of five, and group new cases as subtests.

// formatHighlightRanges renders ranges as "start-end:capture" for readable
// comparisons in test failures.
func formatHighlightRanges(ranges []HighlightRange) string {
	if len(ranges) == 0 {
		return "(empty)"
	}
	var b strings.Builder
	for i, r := range ranges {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%d-%d:%s", r.StartByte, r.EndByte, r.Capture)
	}
	return b.String()
}

// TestHighlighterCapturePolicy covers what the highlighter does with captures
// that do not name a color. The arithmetic language parses "42+77" as
// (expression (expression (NUMBER)) "+" (NUMBER)), and "42" as
// (expression (NUMBER)), where the wrapper and its child share one span.
func TestHighlighterCapturePolicy(t *testing.T) {
	lang := buildArithmeticLanguage()
	cases := []struct {
		name   string
		source string
		query  string
		want   string
	}{
		{
			name:   "drops private, spell, nospell, conceal and locals captures",
			source: "42+77",
			query: `
(NUMBER) @number
(NUMBER) @spell
(NUMBER) @nospell
(NUMBER) @conceal
(NUMBER) @local.reference
((NUMBER) @_private (#eq? @_private "42"))
`,
			want: "0-2:number 3-5:number",
		},
		{
			name:   "private captures still feed predicates",
			source: "42+77",
			query:  `((NUMBER) @number @_n (#eq? @_n "42"))`,
			want:   "0-2:number",
		},
		{
			name:   "only the locals vocabulary is dropped",
			source: "42+77",
			query: `
(NUMBER) @local.function.elm
(NUMBER) @local.scope
(NUMBER) @local.definition.var
(NUMBER) @local.definition-value
`,
			want: "0-2:local.function.elm 3-5:local.function.elm",
		},
		{
			name:   "none resets the enclosing capture and is not returned",
			source: "42+77",
			query:  "(expression) @outer\n(NUMBER) @none",
			want:   "2-3:outer",
		},
		{
			name:   "none after a capture on the same node clears it",
			source: "42+77",
			query:  "(NUMBER) @number @none",
			want:   "(empty)",
		},
		{
			name:   "a capture after none on the same node wins",
			source: "42+77",
			query:  "(NUMBER) @none @number",
			want:   "0-2:number 3-5:number",
		},
		{
			name:   "none alone returns nothing",
			source: "42+77",
			query:  "(NUMBER) @none",
			want:   "(empty)",
		},
		{
			name:   "set! priority above the default beats nesting",
			source: "42+77",
			query:  "((expression) @outer (#set! priority 105))\n(NUMBER) @number",
			want:   "0-5:outer",
		},
		{
			name:   "set! priority accepts a quoted key",
			source: "42+77",
			query:  "((expression) @outer (#set! \"priority\" 105))\n(NUMBER) @number",
			want:   "0-5:outer",
		},
		{
			name:   "set! priority below the default loses to an outer capture",
			source: "42+77",
			query:  "((NUMBER) @number (#set! priority 90))\n(expression) @outer",
			want:   "0-5:outer",
		},
		{
			name:   "equal priorities keep inner-wins nesting",
			source: "42+77",
			query:  "((expression) @outer (#set! priority 105))\n((NUMBER) @number (#set! priority 105))",
			want:   "0-2:number 2-3:outer 3-5:number",
		},
		{
			name:   "an invalid priority is ignored",
			source: "42+77",
			query:  "((expression) @outer (#set! priority high))\n(NUMBER) @number",
			want:   "0-2:number 2-3:outer 3-5:number",
		},
		{
			name:   "none honors set! priority",
			source: "42+77",
			query:  "((expression) @outer (#set! priority 105))\n((NUMBER) @none (#set! priority 110))",
			want:   "2-3:outer",
		},
		{
			name:   "a low priority none does not reset a default capture",
			source: "42+77",
			query:  "((NUMBER) @none (#set! priority 90))\n(expression) @outer",
			want:   "0-5:outer",
		},
		{
			// teal and SQL write (identifier) @variable first and a wrapper such as
			// (simple_type) @type later; the later pattern must keep the span.
			name:   "a later pattern wins on a span shared by a wrapper and its child",
			source: "42",
			query:  "(NUMBER) @inner\n(expression) @outer",
			want:   "0-2:outer",
		},
		{
			name:   "an earlier pattern loses on a span shared by a wrapper and its child",
			source: "42",
			query:  "(expression) @outer\n(NUMBER) @inner",
			want:   "0-2:inner",
		},
		{
			name:   "later pattern still wins on the same node",
			source: "42+77",
			query:  "(NUMBER) @a\n(NUMBER) @b",
			want:   "0-2:b 3-5:b",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewHighlighter(lang, tc.query)
			if err != nil {
				t.Fatalf("NewHighlighter: %v", err)
			}
			// Repeat so buffer reuse cannot change the result.
			for i := 0; i < 3; i++ {
				if got := formatHighlightRanges(h.Highlight([]byte(tc.source))); got != tc.want {
					t.Fatalf("call %d, query:\n%s\ngot  %s\nwant %s", i+1, tc.query, got, tc.want)
				}
			}
			ranges, tree := h.HighlightIncremental([]byte(tc.source), nil)
			defer tree.Release()
			if got := formatHighlightRanges(ranges); got != tc.want {
				t.Fatalf("HighlightIncremental, query:\n%s\ngot  %s\nwant %s", tc.query, got, tc.want)
			}
		})
	}
}

// TestHighlighterCapturePolicyInjected checks that an injected
// child tree obeys the same capture policy as the host document. The host
// "[42+77]" holds the child document "42+77" at bytes 1-6.
func TestHighlighterCapturePolicyInjected(t *testing.T) {
	childLang := buildArithmeticLanguage()
	cases := []struct {
		name       string
		hostQuery  string
		childQuery string
		want       string
	}{
		{
			name:      "drops non-highlight captures",
			hostQuery: "(document) @doc",
			childQuery: `
(expression) @expr
(NUMBER) @number
(NUMBER) @spell
(NUMBER) @nospell
(NUMBER) @conceal
(NUMBER) @local.reference
((NUMBER) @_private)
`,
			want: "0-1:doc 1-3:number 3-4:expr 4-6:number 6-7:doc",
		},
		{
			name:       "none resets the host capture beneath it",
			hostQuery:  "(document) @doc",
			childQuery: "(expression) @expr\n(NUMBER) @none",
			want:       "0-1:doc 3-4:expr 6-7:doc",
		},
		{
			name:       "child priority is read from the child query",
			hostQuery:  "(document) @doc",
			childQuery: "((NUMBER) @number (#set! priority 90))\n(expression) @expr",
			want:       "0-1:doc 1-6:expr 6-7:doc",
		},
		{
			name:       "host priority beats the child layer",
			hostQuery:  "((content) @content (#set! priority 105))",
			childQuery: "(expression) @expr",
			want:       "1-6:content",
		},
		{
			name:       "child layer beats the host on an identical span",
			hostQuery:  "(document) @doc\n(content) @content",
			childQuery: "(expression) @expr",
			want:       "0-1:doc 1-6:expr 6-7:doc",
		},
		{
			// The second pattern matches at the wrapper, before the first pattern's match
			// at the number. The layer used to lose its pattern indexes, so the first
			// pattern won by match order.
			name:       "child layer keeps its own pattern order on identical spans",
			hostQuery:  "(document) @doc",
			childQuery: "(NUMBER) @first\n(expression (NUMBER) @second)",
			want:       "0-1:doc 1-3:second 3-4:doc 4-6:second 6-7:doc",
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parentLang := buildContainerLanguage()
			parentLang.Name = fmt.Sprintf("gts_capture_policy_container_%d", i)
			childQuery := tc.childQuery
			RegisterHighlighterInjection(parentLang.Name, HighlighterInjectionSpec{
				Query: `(content) @injection.content (#set! injection.language "child")`,
				ResolveLanguage: func(hint string) (*Language, string, func([]byte) TokenSource, bool) {
					if hint != "child" {
						return nil, "", nil, false
					}
					return childLang, childQuery, nil, true
				},
			})
			h, err := NewHighlighter(parentLang, tc.hostQuery)
			if err != nil {
				t.Fatalf("NewHighlighter: %v", err)
			}
			for call := 0; call < 3; call++ {
				if got := formatHighlightRanges(h.Highlight([]byte("[42+77]"))); got != tc.want {
					t.Fatalf("call %d, host query:\n%s\nchild query:\n%s\ngot  %s\nwant %s", call+1, tc.hostQuery, tc.childQuery, got, tc.want)
				}
			}
		})
	}
}

// TestHighlighterCapturePolicyNames covers the two tables that drive the policy:
// which capture names are dropped, and which priority each pattern sets.
func TestHighlighterCapturePolicyNames(t *testing.T) {
	t.Run("capture names", func(t *testing.T) {
		for name, want := range map[string]bool{
			"_":                       true,
			"_private":                true,
			"__name__":                true,
			"_heading.marker":         true,
			"spell":                   true,
			"nospell":                 true,
			"conceal":                 true,
			"local.scope":             true,
			"local.reference":         true,
			"local.definition":        true,
			"local.definition-value":  true,
			"local.definition.var":    true,
			"local.definition.import": true,
			"keyword":                 false,
			"comment.documentation":   false,
			"none":                    false, // a reset, which resolution handles
			"spellcheck":              false,
			"nospell.line":            false,
			"concealed":               false,
			"Spell":                   false,
			"local":                   false,
			"local.function.elm":      false, // a real highlight name in the elm query
			"variable.local":          false,
			"locals.scope":            false,
			"":                        false,
		} {
			if got := isNonHighlightCapture(name); got != want {
				t.Errorf("isNonHighlightCapture(%q) = %v, want %v", name, got, want)
			}
		}
	})

	t.Run("pattern priorities", func(t *testing.T) {
		lang := queryTestLanguage()
		plain, err := NewQuery("(identifier) @a\n(number) @b", lang)
		if err != nil {
			t.Fatalf("NewQuery: %v", err)
		}
		if got := highlightPatternPriorities(plain); got != nil {
			t.Fatalf("priorities = %v for a query that sets none, want nil", got)
		}

		q, err := NewQuery(`
(identifier) @plain
((identifier) @raised (#set! priority 105))
((identifier) @lowered (#set! "priority" 90))
((identifier) @negative (#set! priority -5))
((identifier) @invalid (#set! priority high))
((identifier) @empty (#set! priority))
((identifier) @overflow (#set! priority 99999999999))
((identifier) @other-key (#set! injection.language "go"))
((identifier) @last-wins (#set! priority 120) (#set! priority 95))
((identifier) @invalid-last (#set! priority 120) (#set! priority high))
((identifier) @with-predicate (#set! priority 110) (#eq? @with-predicate "x"))
`, lang)
		if err != nil {
			t.Fatalf("NewQuery: %v", err)
		}
		want := []int32{100, 105, 90, -5, 100, 100, 100, 100, 95, 100, 110}
		got := highlightPatternPriorities(q)
		if len(got) != len(want) {
			t.Fatalf("priorities = %v, want %v", got, want)
		}
		for pattern := range want {
			if got[pattern] != want[pattern] {
				t.Errorf("pattern %d priority = %d, want %d", pattern, got[pattern], want[pattern])
			}
		}
	})
}

// genLaminarRanges emits intervals that nest or stay disjoint, as the nodes of
// one syntax tree do. Intervals may repeat.
func genLaminarRanges(r *rand.Rand, lo, hi uint32, depth int, emit func(lo, hi uint32)) {
	if hi <= lo {
		return
	}
	for n := r.Intn(3); n > 0; n-- {
		emit(lo, hi)
	}
	if depth == 0 || hi-lo < 2 {
		return
	}
	for cur := lo; cur < hi; {
		end := cur + 1 + uint32(r.Intn(int(hi-cur)))
		if r.Intn(3) != 0 {
			genLaminarRanges(r, cur, end, depth-1, emit)
		}
		cur = end
	}
}

type pricedHighlightRange struct {
	HighlightRange
	priority int32
}

// modelRangeBeats reports whether range a, the aIndex-th range added, takes a
// byte from range b, the bIndex-th: the higher priority wins, then the narrower
// range, then the later pattern, then the range added last.
func modelRangeBeats(a pricedHighlightRange, aIndex int, b pricedHighlightRange, bIndex int) bool {
	if a.priority != b.priority {
		return a.priority > b.priority
	}
	if aw, bw := a.EndByte-a.StartByte, b.EndByte-b.StartByte; aw != bw {
		return aw < bw
	}
	if a.PatternIndex != b.PatternIndex {
		return a.PatternIndex > b.PatternIndex
	}
	return aIndex > bIndex
}

// TestHighlighterCapturePolicyOverlay covers laying a higher priority over a lower
// one, the shifts that injected layers go through, and a result of resets only.
func TestHighlighterCapturePolicyOverlay(t *testing.T) {
	t.Run("overlay", func(t *testing.T) {
		r := func(start, end uint32, capture string) HighlightRange {
			return HighlightRange{StartByte: start, EndByte: end, Capture: capture}
		}
		for _, tc := range []struct {
			name         string
			lower, upper []HighlightRange
			want         string
		}{
			{"no upper", []HighlightRange{r(0, 5, "a")}, nil, "0-5:a"},
			{"no lower", nil, []HighlightRange{r(2, 4, "u")}, "2-4:u"},
			{"upper inside lower", []HighlightRange{r(0, 10, "a")}, []HighlightRange{r(3, 6, "u")}, "0-3:a 3-6:u 6-10:a"},
			{"upper covers lower", []HighlightRange{r(2, 4, "a")}, []HighlightRange{r(0, 10, "u")}, "0-10:u"},
			{"upper clips the start", []HighlightRange{r(5, 10, "a")}, []HighlightRange{r(0, 7, "u")}, "0-7:u 7-10:a"},
			{"upper clips the end", []HighlightRange{r(0, 5, "a")}, []HighlightRange{r(3, 9, "u")}, "0-3:a 3-9:u"},
			{
				"upper spans several lower ranges",
				[]HighlightRange{r(0, 10, "a"), r(10, 20, "b")},
				[]HighlightRange{r(5, 15, "u")},
				"0-5:a 5-15:u 15-20:b",
			},
			{
				"upper spans a gap between lower ranges",
				[]HighlightRange{r(0, 4, "a"), r(8, 12, "b")},
				[]HighlightRange{r(2, 10, "u")},
				"0-2:a 2-10:u 10-12:b",
			},
			{
				"upper in a gap",
				[]HighlightRange{r(0, 4, "a"), r(8, 12, "b")},
				[]HighlightRange{r(5, 7, "u")},
				"0-4:a 5-7:u 8-12:b",
			},
			{
				"several upper ranges in one lower range",
				[]HighlightRange{r(0, 20, "a")},
				[]HighlightRange{r(2, 4, "u"), r(6, 8, "v"), r(8, 10, "w")},
				"0-2:a 2-4:u 4-6:a 6-8:v 8-10:w 10-20:a",
			},
			{
				"equal captures join",
				[]HighlightRange{r(0, 10, "a")},
				[]HighlightRange{r(3, 6, "a")},
				"0-10:a",
			},
			{
				"upper before and after all lower ranges",
				[]HighlightRange{r(5, 8, "a")},
				[]HighlightRange{r(0, 2, "u"), r(9, 12, "v")},
				"0-2:u 5-8:a 9-12:v",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := formatHighlightRanges(overlayHighlightRanges(tc.lower, tc.upper)); got != tc.want {
					t.Fatalf("got  %s\nwant %s", got, tc.want)
				}
			})
		}
	})

	t.Run("only resets resolve to nil", func(t *testing.T) {
		var tiers highlightTiers
		if !tiers.empty() {
			t.Fatal("zero tiers are not empty")
		}
		tiers.add(defaultHighlightPriority, HighlightRange{StartByte: 0, EndByte: 4, Capture: highlightResetCapture})
		tiers.add(110, HighlightRange{StartByte: 2, EndByte: 3, Capture: highlightResetCapture})
		if tiers.empty() {
			t.Fatal("tiers with ranges are empty")
		}
		if got := withoutHighlightResets(tiers.resolve(nil)); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("merge and clip", func(t *testing.T) {
		var host highlightTiers
		host.add(defaultHighlightPriority, HighlightRange{StartByte: 1, EndByte: 6, Capture: "content", PatternIndex: 3})

		var layer highlightTiers
		layer.add(defaultHighlightPriority, HighlightRange{StartByte: 0, EndByte: 5, Capture: "expr", PatternIndex: 0})
		layer.add(105, HighlightRange{StartByte: 0, EndByte: 2, Capture: "raised", PatternIndex: 1})
		layer.add(105, HighlightRange{StartByte: 8, EndByte: 9, Capture: "outside", PatternIndex: 1})
		layer.clip(0, 5) // the wrapped-source path keeps what lies inside the fence
		host.merge(&layer, 1, 10)

		// The layer's expr shares the host's span and has the later pattern, so it wins;
		// raised has a higher priority and keeps the start of the span.
		if got, want := formatHighlightRanges(host.resolve(nil)), "1-3:raised 3-6:expr"; got != want {
			t.Fatalf("got %s, want %s", got, want)
		}

		var shifted highlightTiers
		shifted.add(defaultHighlightPriority, HighlightRange{StartByte: 3, EndByte: 5, Capture: "in"})
		shifted.add(defaultHighlightPriority, HighlightRange{StartByte: 0, EndByte: 3, Capture: "before"})
		shifted.add(defaultHighlightPriority, HighlightRange{StartByte: 4, EndByte: 9, Capture: "after"})
		shifted.clip(3, 8)
		if got, want := formatHighlightRanges(shifted.base), "0-2:in"; got != want {
			t.Fatalf("clip kept %s, want %s", got, want)
		}
	})
}

// TestHighlighterCapturePolicyPainter checks the resolved ranges of 3,000 random
// nested range sets against a byte-by-byte model that picks each byte's winner
// with modelRangeBeats. A reset capture wins like any other and leaves no range.
func TestHighlighterCapturePolicyPainter(t *testing.T) {
	r := rand.New(rand.NewSource(20261026))
	captures := []string{"a", "b", "c", "d", highlightResetCapture}
	priorities := []int32{90, defaultHighlightPriority, defaultHighlightPriority, 105, 110}
	const size = 48
	for round := 0; round < 3000; round++ {
		var added []pricedHighlightRange
		genLaminarRanges(r, 0, size, 4, func(lo, hi uint32) {
			added = append(added, pricedHighlightRange{
				HighlightRange: HighlightRange{
					StartByte:    lo,
					EndByte:      hi,
					Capture:      captures[r.Intn(len(captures))],
					PatternIndex: r.Intn(6),
				},
				priority: priorities[r.Intn(len(priorities))],
			})
		})
		r.Shuffle(len(added), func(i, j int) { added[i], added[j] = added[j], added[i] })

		// Model: pick the winner of every byte, then join equal neighbors.
		var want []HighlightRange
		for b := uint32(0); b < size; b++ {
			best := -1
			for i, a := range added {
				if b < a.StartByte || b >= a.EndByte {
					continue
				}
				if best < 0 || modelRangeBeats(a, i, added[best], best) {
					best = i
				}
			}
			if best < 0 || added[best].Capture == highlightResetCapture {
				continue
			}
			want = appendHighlightSegment(want, HighlightRange{StartByte: b, EndByte: b + 1, Capture: added[best].Capture})
		}

		var tiers highlightTiers
		for _, a := range added {
			tiers.add(a.priority, a.HighlightRange)
		}
		got := withoutHighlightResets(tiers.resolve(nil))
		if gotText, wantText := formatHighlightRanges(got), formatHighlightRanges(want); gotText != wantText {
			t.Fatalf("round %d\nadded: %+v\ngot  %s\nwant %s", round, added, gotText, wantText)
		}
	}
}

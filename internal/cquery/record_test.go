package cquery

import (
	"strings"
	"testing"
)

func TestRecordTextForms(t *testing.T) {
	depth := uint32(1)
	cases := []struct {
		record Record
		want   string
	}{
		{Capture{Name: "k", Type: "string", Named: true, StartByte: 1, EndByte: 4}, `@k "string" named 1-4`},
		{Capture{Name: "close", Type: "}", StartByte: 30, EndByte: 31}, `@close "}" anon 30-31`},
		{Capture{Name: "x", Type: "\"", StartByte: 2, EndByte: 3}, `@x "\"" anon 2-3`},
		{Match{Pattern: 2}, `p2`},
		{Match{Pattern: 0, Captures: []Capture{
			{Name: "k", Type: "string", Named: true, StartByte: 1, EndByte: 4},
			{Name: "type.builtin", Type: "identifier", Named: true, StartByte: 5, EndByte: 9},
		}}, `p0 @k "string" named 1-4 @type.builtin "identifier" named 5-9`},
		{StreamCapture{Pattern: 1, Index: 2, Capture: Capture{Name: "n", Type: "number", Named: true, StartByte: 1, EndByte: 2}}, `p1 #2 @n "number" named 1-2`},
		{CaptureName{ID: 3, Name: "definition.function"}, `capture 3 definition.function`},
		{CaptureQuantifier{Pattern: 0, Capture: 1, Quantifier: QuantifierOneOrMore}, `quantifier p0 1 one_or_more`},
		{Pattern{Index: 1, StepOffset: 6, StepCount: 4, StartByte: 31, EndByte: 60}, `pattern 1 steps=6+4 bytes=31-60`},
		{PatternEntry{Index: 0, Step: 1, Pattern: 0, Rooted: true}, `entry 0 step=1 pattern=0 rooted`},
		{PatternEntry{Index: 2, Step: 9}, `entry 2 step=9 pattern=0 unrooted`},
		{Step{Index: 5, Depth: DoneDepth, AlternativeIndex: NoStep}, `s5 depth=done symbol=_`},
		{Step{
			Index:             1,
			Depth:             1,
			Symbol:            Symbol{ID: 12, Name: "pair"},
			Supertype:         Symbol{ID: 40, Name: "value"},
			Field:             Symbol{ID: 1, Name: "key"},
			CaptureIDs:        []uint16{0, 2},
			AlternativeIndex:  4,
			NegatedFields:     []Symbol{{ID: 2, Name: "value"}, {ID: 3, Name: "name"}},
			Named:             true,
			Immediate:         true,
			LastChild:         true,
			PassThrough:       true,
			DeadEnd:           true,
			InsideAlternation: true,
			Missing:           true,
			AlternativeIsSkip: true,
		}, `s1 depth=1 symbol=12:"pair" supertype=40:"value" field=1:"key" captures=0,2 alternative=4 negated=2:"value",3:"name" named immediate last_child pass_through dead_end inside_alternation missing alternative_is_skip`},
		{StepOffset{ByteOffset: 12, Step: 3}, `offset 12 step=3`},
		{Verdict{Accepted: true}, `accept`},
		{Verdict{ErrorKind: "structure", ErrorOffset: 12}, `reject kind=structure offset=12`},
		{PatternAnalysis{Pattern: 0, Rooted: true}, `pattern 0 rooted local`},
		{PatternAnalysis{Pattern: 1, NonLocal: true}, `pattern 1 unrooted non_local`},
		{StepAnalysis{Step: 3}, `s3 -`},
		{StepAnalysis{Step: 3, ContainsCaptures: true, RootPatternGuaranteed: true, ParentPatternGuaranteed: true}, `s3 contains_captures root_guaranteed parent_guaranteed`},
		{RepeatSymbol{Symbol: Symbol{ID: 40, Name: "block_repeat1"}}, `repeat 40:"block_repeat1"`},
		{LimitRun{}, `limits`},
		{LimitRun{ByteRange: &[2]uint32{10, 20}, PointRange: &[2]Point{{0, 1}, {2, 3}}, MaxStartDepth: &depth, MatchLimit: 4, Exceeded: true},
			`limits bytes=10-20 points=0:1-2:3 max_start_depth=1 match_limit=4 exceeded`},
	}
	for _, tc := range cases {
		if got := tc.record.String(); got != tc.want {
			t.Errorf("%T text form\n got %s\nwant %s", tc.record, got, tc.want)
		}
	}
}

func TestFirstDifference(t *testing.T) {
	a := []string{"p0 x", "p0 y", "p1 z"}
	if d, ok := FirstDifference(LayerMatches, a, append([]string(nil), a...)); ok {
		t.Fatalf("equal lists reported %s", d)
	}
	if _, ok := FirstDifference(LayerMatches, nil, []string{}); ok {
		t.Fatal("nil and empty lists differ")
	}
	cases := []struct {
		got  []string
		want Difference
		text string
	}{
		{[]string{"p0 x", "p0 q", "p1 z"}, Difference{Layer: LayerMatches, Index: 1, Want: "p0 y", Got: "p0 q", WantCount: 3, GotCount: 3},
			`L3 record 1 of 3 (got 3): want "p0 y" got "p0 q"`},
		{[]string{"p0 x"}, Difference{Layer: LayerMatches, Index: 1, Want: "p0 y", WantCount: 3, GotCount: 1},
			`L3 record 1 of 3 (got 1): want "p0 y" got <end>`},
		{[]string{"p0 x", "p0 y", "p1 z", "p2 w"}, Difference{Layer: LayerMatches, Index: 3, Got: "p2 w", WantCount: 3, GotCount: 4},
			`L3 record 3 of 3 (got 4): want <end> got "p2 w"`},
	}
	for _, tc := range cases {
		d, ok := FirstDifference(LayerMatches, a, tc.got)
		if !ok || d != tc.want {
			t.Errorf("FirstDifference(%q) = %+v, %v; want %+v", tc.got, d, ok, tc.want)
		}
		if d.String() != tc.text {
			t.Errorf("Difference text\n got %s\nwant %s", d.String(), tc.text)
		}
	}
}

func TestTexts(t *testing.T) {
	got := Texts([]Match{{Pattern: 0}, {Pattern: 1}})
	if strings.Join(got, "|") != "p0|p1" {
		t.Fatalf("Texts = %q", got)
	}
}

func TestDigest(t *testing.T) {
	if got, want := Digest(nil), "n=0 sha256=e3b0c44298fc1c14"; got != want {
		t.Fatalf("Digest(nil) = %s, want %s", got, want)
	}
	// The record boundary is part of the digest.
	if Digest([]string{"ab", "c"}) == Digest([]string{"a", "bc"}) {
		t.Fatal("digest ignores record boundaries")
	}
	if Digest([]string{"a", "b"}) == Digest([]string{"b", "a"}) {
		t.Fatal("digest ignores record order")
	}
}

func TestTreeHasher(t *testing.T) {
	sum := func(nodes ...TreeNode) string {
		h := NewTreeHasher()
		for _, n := range nodes {
			h.Add(n)
		}
		return h.Sum()
	}
	root := TreeNode{Type: "document", Named: true, EndByte: 3, ChildCount: 1}
	child := TreeNode{Depth: 1, Type: "array", Named: true, EndByte: 3}
	base := sum(root, child)
	if !strings.HasPrefix(base, "sha256:") || len(base) != len("sha256:")+16 {
		t.Fatalf("Sum = %q", base)
	}
	if base != sum(root, child) {
		t.Fatal("digest is not stable")
	}
	for name, changed := range map[string]TreeNode{
		"field":   {Depth: 1, Field: "value", Type: "array", Named: true, EndByte: 3},
		"type":    {Depth: 1, Type: "object", Named: true, EndByte: 3},
		"named":   {Depth: 1, Type: "array", EndByte: 3},
		"missing": {Depth: 1, Type: "array", Named: true, Missing: true, EndByte: 3},
		"range":   {Depth: 1, Type: "array", Named: true, StartByte: 1, EndByte: 3},
		"depth":   {Depth: 2, Type: "array", Named: true, EndByte: 3},
	} {
		if sum(root, changed) == base {
			t.Errorf("digest ignores the %s", name)
		}
	}
}

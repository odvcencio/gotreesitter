package gotreesitter

import "testing"

func extendParentSpanToWindowForTest(parent *Node, entries []stackEntry, start, reducedEnd int, symbolMeta []SymbolMetadata, symbolNames []string) {
	extendParentSpanToWindowForSourceTest(parent, entries, start, reducedEnd, symbolMeta, symbolNames, nil)
}

func extendParentSpanToWindowForSourceTest(parent *Node, entries []stackEntry, start, reducedEnd int, symbolMeta []SymbolMetadata, symbolNames []string, source []byte) {
	spanExtending, nonSpanExtending := buildInvisibleSpanSymbolTables(symbolNames)
	extendParentSpanToWindow(parent, entries, start, reducedEnd, symbolMeta, spanExtending, nonSpanExtending, source)
}

func TestRewriteRefreshPreservesProducedSpan(t *testing.T) {
	visible := NewLeafNode(2, true, 10, 20, Point{Column: 10}, Point{Column: 20})
	parent := NewParentNode(3, true, []*Node{visible}, nil, 7)
	parent.endByte = 24
	parent.endPoint = Point{Row: 1, Column: 2}

	replacement := NewLeafNode(2, true, 10, 18, Point{Column: 10}, Point{Column: 18})
	replacement.setHasError(true)
	parent.children = []*Node{replacement}
	refreshRewrittenParentPreservingProducedSpan(parent, parent.children)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent start byte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(24); got != want {
		t.Fatalf("parent end byte = %d, want %d", got, want)
	}
	if got, want := parent.endPoint, (Point{Row: 1, Column: 2}); got != want {
		t.Fatalf("parent end point = %#v, want %#v", got, want)
	}
	if replacement.parent != parent || replacement.childIndex != 0 {
		t.Fatal("rewrite refresh did not restore the child link")
	}
	if !parent.hasError() {
		t.Fatal("rewrite refresh did not update the parent error state")
	}
}

func TestRewriteRefreshCanWidenProducedSpan(t *testing.T) {
	visible := NewLeafNode(2, true, 10, 20, Point{Column: 10}, Point{Column: 20})
	parent := NewParentNode(3, true, []*Node{visible}, nil, 7)
	parent.endByte = 24
	parent.endPoint = Point{Row: 1, Column: 2}

	replacement := NewLeafNode(2, true, 8, 30, Point{Column: 8}, Point{Row: 1, Column: 8})
	parent.children = []*Node{replacement}
	refreshRewrittenParentPreservingProducedSpan(parent, parent.children)

	if got, want := parent.startByte, uint32(8); got != want {
		t.Fatalf("parent start byte = %d, want %d", got, want)
	}
	if got, want := parent.startPoint, (Point{Column: 8}); got != want {
		t.Fatalf("parent start point = %#v, want %#v", got, want)
	}
	if got, want := parent.endByte, uint32(30); got != want {
		t.Fatalf("parent end byte = %d, want %d", got, want)
	}
	if got, want := parent.endPoint, (Point{Row: 1, Column: 8}); got != want {
		t.Fatalf("parent end point = %#v, want %#v", got, want)
	}
}

func TestRewriteRefreshPreservesProducedSpanAcrossMultipleChildren(t *testing.T) {
	original := NewLeafNode(2, true, 10, 24, Point{Column: 10}, Point{Row: 1, Column: 2})
	parent := NewParentNode(3, true, []*Node{original}, nil, 7)
	parent.endByte = 30
	parent.endPoint = Point{Row: 1, Column: 8}

	first := NewLeafNode(2, true, 12, 16, Point{Column: 12}, Point{Column: 16})
	second := NewLeafNode(2, true, 18, 22, Point{Column: 18}, Point{Column: 22})
	second.setHasError(true)
	third := NewLeafNode(2, true, 24, 26, Point{Row: 1, Column: 2}, Point{Row: 1, Column: 4})
	parent.children = []*Node{first, second, third}
	refreshRewrittenParentPreservingProducedSpan(parent, parent.children)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent start byte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(30); got != want {
		t.Fatalf("parent end byte = %d, want %d", got, want)
	}
	if got, want := parent.endPoint, (Point{Row: 1, Column: 8}); got != want {
		t.Fatalf("parent end point = %#v, want %#v", got, want)
	}
	for index, child := range parent.children {
		if child.parent != parent || child.childIndex != int32(index) {
			t.Fatalf("child %d link = (%p, %d), want (%p, %d)", index, child.parent, child.childIndex, parent, index)
		}
	}
	if !parent.hasError() {
		t.Fatal("rewrite refresh did not collect the error state from all children")
	}

	second.setHasError(false)
	refreshRewrittenParentPreservingProducedSpan(parent, parent.children)
	if parent.hasError() {
		t.Fatal("rewrite refresh retained a stale child error state")
	}
}

func TestExtendParentSpanCoversInvisibleLeafChild(t *testing.T) {
	// Invisible non-extra leaf child [20-22] dropped by buildReduceChildren
	// should extend parent endByte from 20 to 22 (contiguous), but leading
	// extras should not pull the visible parent start before the core child.
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 10
	parent.endByte = 20
	parent.startPoint = Point{Row: 1, Column: 10}
	parent.endPoint = Point{Row: 1, Column: 20}

	leadingExtra := NewLeafNode(1, false, 8, 9, Point{Row: 1, Column: 8}, Point{Row: 1, Column: 9})
	leadingExtra.setExtra(true)
	core := NewLeafNode(2, true, 10, 20, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 20})
	invisible := NewLeafNode(4, false, 20, 22, Point{Row: 1, Column: 20}, Point{Row: 1, Column: 22})

	entries := []stackEntry{
		newStackEntryNode(0, leadingExtra),
		newStackEntryNode(0, core),
		newStackEntryNode(0, invisible),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, nil)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(22); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanSkipsLeadingExtraBeforeVisibleChild(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 10
	parent.endByte = 20
	parent.startPoint = Point{Row: 1, Column: 0}
	parent.endPoint = Point{Row: 1, Column: 10}

	leadingExtra := NewLeafNode(1, false, 0, 10, Point{Row: 0, Column: 0}, Point{Row: 1, Column: 0})
	leadingExtra.setExtra(true)
	core := NewLeafNode(2, true, 10, 20, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 10})
	invisibleTail := NewLeafNode(4, false, 20, 22, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 12})

	entries := []stackEntry{
		newStackEntryNode(0, leadingExtra),
		newStackEntryNode(0, core),
		newStackEntryNode(0, invisibleTail),
	}
	meta := []SymbolMetadata{
		{}, {Visible: false}, {Visible: true}, {Visible: true}, {Visible: false},
	}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, nil)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(22); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanChainsInvisiblePrefixLeaves(t *testing.T) {
	parent := NewParentNode(5, true, nil, nil, 0)
	parent.startByte = 25
	parent.endByte = 30
	parent.startPoint = Point{Row: 1, Column: 25}
	parent.endPoint = Point{Row: 1, Column: 30}

	prefix1 := NewLeafNode(1, false, 10, 15, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 15})
	prefix2 := NewLeafNode(2, false, 15, 20, Point{Row: 1, Column: 15}, Point{Row: 1, Column: 20})
	prefix3 := NewLeafNode(3, false, 20, 25, Point{Row: 1, Column: 20}, Point{Row: 1, Column: 25})
	core := NewLeafNode(4, true, 25, 30, Point{Row: 1, Column: 25}, Point{Row: 1, Column: 30})

	entries := []stackEntry{
		newStackEntryNode(0, prefix1),
		newStackEntryNode(0, prefix2),
		newStackEntryNode(0, prefix3),
		newStackEntryNode(0, core),
	}
	meta := []SymbolMetadata{
		{},
		{Visible: false},
		{Visible: false},
		{Visible: false},
		{Visible: true},
	}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, nil)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(30); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanCoversHiddenKeywordPrefixAcrossWhitespace(t *testing.T) {
	// Dart reductions like `type_alias` and `declaration` can start with hidden
	// non-extra keywords (`typedef ` / `static `) followed by a visible child.
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 8
	parent.endByte = 47
	parent.startPoint = Point{Row: 0, Column: 8}
	parent.endPoint = Point{Row: 0, Column: 47}

	hiddenKeyword := NewLeafNode(1, false, 0, 7, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 7})
	visibleTail := NewLeafNode(2, true, 8, 47, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 47})

	entries := []stackEntry{
		newStackEntryNode(0, hiddenKeyword),
		newStackEntryNode(0, visibleTail),
	}
	meta := []SymbolMetadata{
		{}, {Visible: false}, {Visible: true},
	}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, []string{"", "_typedef", "visible"})

	if got, want := parent.startByte, uint32(0); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(47); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanSkipsDiscontiguousPhantom(t *testing.T) {
	// A zero-width invisible entry AFTER the parent span (like javascript
	// _automatic_semicolon at [27-27] after statement_block [13-26])
	// must NOT extend the parent span.
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 13
	parent.endByte = 26
	parent.startPoint = Point{Row: 1, Column: 13}
	parent.endPoint = Point{Row: 1, Column: 26}

	core := NewLeafNode(2, true, 13, 26, Point{Row: 1, Column: 13}, Point{Row: 1, Column: 26})
	phantom := NewLeafNode(4, false, 27, 27, Point{Row: 1, Column: 27}, Point{Row: 1, Column: 27})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, phantom),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, []string{"", "", "visible", "", "_automatic_semicolon"})

	if got, want := parent.endByte, uint32(26); got != want {
		t.Fatalf("parent.endByte = %d, want %d (phantom should not extend)", got, want)
	}
}

func TestExtendParentSpanSkipsNonWhitelistedInvisibleGap(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 10
	parent.endByte = 20
	parent.startPoint = Point{Row: 1, Column: 10}
	parent.endPoint = Point{Row: 1, Column: 20}

	core := NewLeafNode(2, true, 10, 20, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 20})
	invisibleTail := NewLeafNode(4, false, 22, 25, Point{Row: 1, Column: 22}, Point{Row: 1, Column: 25})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, invisibleTail),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_ordinary_hidden_tail"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.endByte, uint32(20); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanCoversInvisibleTailAcrossWhitespace(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 100
	parent.endByte = 102
	parent.startPoint = Point{Row: 1, Column: 0}
	parent.endPoint = Point{Row: 1, Column: 2}

	core := NewLeafNode(2, true, 100, 102, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 2})
	invisibleTail := NewLeafNode(4, false, 103, 109, Point{Row: 1, Column: 3}, Point{Row: 1, Column: 9})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, invisibleTail),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	source := []byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxas String,")
	extendParentSpanToWindowForSourceTest(parent, entries, 0, len(entries), meta, nil, source)

	if got, want := parent.endByte, uint32(109); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanSkipsInvisibleTailAcrossRealText(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 100
	parent.endByte = 102
	parent.startPoint = Point{Row: 1, Column: 0}
	parent.endPoint = Point{Row: 1, Column: 2}

	core := NewLeafNode(2, true, 100, 102, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 2})
	invisibleTail := NewLeafNode(4, false, 104, 110, Point{Row: 1, Column: 4}, Point{Row: 1, Column: 10})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, invisibleTail),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	source := []byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxas/*String,")
	extendParentSpanToWindowForSourceTest(parent, entries, 0, len(entries), meta, nil, source)

	if got, want := parent.endByte, uint32(102); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanCoversInvisibleWithChildren(t *testing.T) {
	// An invisible node WITH children whose span exceeds its children's span
	// (due to nested invisible leaf extension) should still extend the parent.
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 5
	parent.endByte = 14
	parent.startPoint = Point{Row: 1, Column: 5}
	parent.endPoint = Point{Row: 1, Column: 14}

	invisibleWithKids := NewParentNode(4, false, []*Node{
		NewLeafNode(5, true, 5, 14, Point{Row: 1, Column: 5}, Point{Row: 1, Column: 14}),
	}, nil, 0)
	invisibleWithKids.startByte = 5
	invisibleWithKids.endByte = 15
	invisibleWithKids.startPoint = Point{Row: 1, Column: 5}
	invisibleWithKids.endPoint = Point{Row: 1, Column: 15}

	entries := []stackEntry{
		newStackEntryNode(0, invisibleWithKids),
	}
	meta := []SymbolMetadata{
		{}, {}, {}, {}, {Visible: false}, {Visible: true},
	}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, nil)

	if got, want := parent.endByte, uint32(15); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanNoOp(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 10
	parent.endByte = 20
	parent.startPoint = Point{Row: 2, Column: 10}
	parent.endPoint = Point{Row: 2, Column: 20}

	core := NewLeafNode(2, true, 10, 20, Point{Row: 2, Column: 10}, Point{Row: 2, Column: 20})
	entries := []stackEntry{newStackEntryNode(0, core)}
	meta := []SymbolMetadata{{}, {}, {Visible: true}}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, nil)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(20); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanAllowsImplicitEndTagGap(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 10
	parent.endByte = 20
	parent.startPoint = Point{Row: 1, Column: 10}
	parent.endPoint = Point{Row: 1, Column: 20}

	core := NewLeafNode(2, true, 10, 20, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 20})
	implicitEnd := NewLeafNode(4, false, 21, 21, Point{Row: 1, Column: 21}, Point{Row: 1, Column: 21})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, implicitEnd),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_implicit_end_tag"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(21); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanAllowsOutdentGap(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 3209
	parent.endByte = 3242
	parent.startPoint = Point{Row: 98, Column: 8}
	parent.endPoint = Point{Row: 98, Column: 41}

	core := NewLeafNode(2, true, 3209, 3242, Point{Row: 98, Column: 8}, Point{Row: 98, Column: 41})
	outdent := NewLeafNode(4, false, 3250, 3250, Point{Row: 100, Column: 6}, Point{Row: 100, Column: 6})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, outdent),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_outdent"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.endByte, uint32(3250); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanAllowsMultilineStringEndGap(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 2409
	parent.endByte = 2747
	parent.startPoint = Point{Row: 68, Column: 28}
	parent.endPoint = Point{Row: 74, Column: 51}

	core := NewLeafNode(2, true, 2409, 2747, Point{Row: 68, Column: 28}, Point{Row: 74, Column: 51})
	stringEnd := NewLeafNode(4, false, 2756, 2759, Point{Row: 75, Column: 8}, Point{Row: 75, Column: 11})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, stringEnd),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_multiline_string_end"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.endByte, uint32(2759); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanChainsInterpolatedMultilineStringTail(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 2409
	parent.endByte = 2747
	parent.startPoint = Point{Row: 68, Column: 28}
	parent.endPoint = Point{Row: 74, Column: 51}

	core := NewLeafNode(2, true, 2409, 2747, Point{Row: 68, Column: 28}, Point{Row: 74, Column: 51})
	middle := NewLeafNode(4, false, 2747, 2756, Point{Row: 75, Column: 0}, Point{Row: 75, Column: 9})
	stringEnd := NewLeafNode(5, false, 2756, 2759, Point{Row: 75, Column: 9}, Point{Row: 75, Column: 12})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, middle),
		newStackEntryNode(0, stringEnd),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_interpolated_multiline_string_middle", "_multiline_string_end"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.endByte, uint32(2759); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanSkipsInvisibleLineEnding(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 10
	parent.endByte = 20
	parent.startPoint = Point{Row: 1, Column: 10}
	parent.endPoint = Point{Row: 1, Column: 20}

	core := NewLeafNode(2, true, 10, 20, Point{Row: 1, Column: 10}, Point{Row: 1, Column: 20})
	lineEnd := NewLeafNode(4, false, 20, 21, Point{Row: 1, Column: 20}, Point{Row: 2, Column: 0})

	entries := []stackEntry{
		newStackEntryNode(0, core),
		newStackEntryNode(0, lineEnd),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_line_ending_or_eof"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.startByte, uint32(10); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(20); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanIncludesInvisiblePrefixAcrossWhitespace(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 7
	parent.endByte = 39
	parent.startPoint = Point{Row: 0, Column: 7}
	parent.endPoint = Point{Row: 0, Column: 39}

	hiddenPrefix := NewLeafNode(4, false, 0, 6, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 6})
	visibleTail := NewLeafNode(2, true, 7, 39, Point{Row: 0, Column: 7}, Point{Row: 0, Column: 39})

	entries := []stackEntry{
		newStackEntryNode(0, hiddenPrefix),
		newStackEntryNode(0, visibleTail),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_hidden_prefix"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.startByte, uint32(0); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(39); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestExtendParentSpanSkipsNonSpanExtendingInvisiblePrefix(t *testing.T) {
	parent := NewParentNode(3, true, nil, nil, 0)
	parent.startByte = 17
	parent.endByte = 27
	parent.startPoint = Point{Row: 4, Column: 4}
	parent.endPoint = Point{Row: 4, Column: 14}

	layoutPrefix := NewLeafNode(4, false, 9, 17, Point{Row: 1, Column: 0}, Point{Row: 4, Column: 4})
	visibleTail := NewLeafNode(2, true, 17, 27, Point{Row: 4, Column: 4}, Point{Row: 4, Column: 14})

	entries := []stackEntry{
		newStackEntryNode(0, layoutPrefix),
		newStackEntryNode(0, visibleTail),
	}
	meta := []SymbolMetadata{
		{}, {}, {Visible: true}, {}, {Visible: false},
	}
	names := []string{"", "", "visible", "", "_line_ending_or_eof"}
	extendParentSpanToWindowForTest(parent, entries, 0, len(entries), meta, names)

	if got, want := parent.startByte, uint32(17); got != want {
		t.Fatalf("parent.startByte = %d, want %d", got, want)
	}
	if got, want := parent.endByte, uint32(27); got != want {
		t.Fatalf("parent.endByte = %d, want %d", got, want)
	}
}

func TestShouldUseRawSpanForInvisibleReduction(t *testing.T) {
	meta := []SymbolMetadata{
		{},
		{Visible: true},
		{Visible: false},
	}
	children := []*Node{
		NewLeafNode(1, true, 38, 45, Point{Row: 0, Column: 38}, Point{Row: 0, Column: 45}),
	}

	if !shouldUseRawSpanForReduction(2, children, meta, false, nil) {
		t.Fatalf("expected invisible reduction to preserve raw span")
	}
	if shouldUseRawSpanForReduction(1, children, meta, false, nil) {
		t.Fatalf("expected visible reduction with visible children to keep child-derived span")
	}
}

func TestComputeReduceRawSpanKeepsDroppedInvisiblePrefix(t *testing.T) {
	visibleTail := NewLeafNode(1, true, 38, 45, Point{Row: 0, Column: 38}, Point{Row: 0, Column: 45})
	invisibleReduced := NewParentNode(2, false, []*Node{visibleTail}, nil, 0)
	invisibleReduced.startByte = 16
	invisibleReduced.endByte = 45
	invisibleReduced.startPoint = Point{Row: 0, Column: 16}
	invisibleReduced.endPoint = Point{Row: 0, Column: 45}

	entries := []stackEntry{newStackEntryNode(0, invisibleReduced)}
	span := computeReduceRawSpan(entries, 0, len(entries))
	if got, want := span.startByte, uint32(16); got != want {
		t.Fatalf("span.startByte = %d, want %d", got, want)
	}
	if got, want := span.endByte, uint32(45); got != want {
		t.Fatalf("span.endByte = %d, want %d", got, want)
	}
}

func TestFlattenedHiddenWrapperDoesNotWidenVisibleDescendantStart(t *testing.T) {
	visible := NewLeafNode(1, true, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	hidden := NewParentNode(2, false, []*Node{visible}, nil, 0)
	hidden.startByte = 3
	hidden.endByte = 5
	hidden.startPoint = Point{Row: 0, Column: 3}
	hidden.endPoint = Point{Row: 0, Column: 5}
	symbolMeta := []SymbolMetadata{
		{},
		{Name: "visible", Visible: true, Named: true},
		{Name: "_hidden", Visible: false},
	}

	scratch := &reduceBuildScratch{}
	appendFlattenedHiddenChildrenToScratch(scratch, hidden, symbolMeta, nil)
	if got, want := len(scratch.nodes), 1; got != want {
		t.Fatalf("flattened child count = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].startByte, uint32(4); got != want {
		t.Fatalf("flattened visible child startByte = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].endByte, uint32(5); got != want {
		t.Fatalf("flattened visible child endByte = %d, want %d", got, want)
	}
}

func TestDroppedHiddenSiblingPaddingDoesNotWidenFollowingVisibleChild(t *testing.T) {
	hidden := NewLeafNode(2, false, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	visible := NewLeafNode(1, true, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	entries := []stackEntry{
		newStackEntryNode(0, hidden),
		newStackEntryNode(0, visible),
	}
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "visible", Visible: true, Named: true},
			{Name: "_hidden", Visible: false},
			{Name: "parent", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang}

	children, _, _, _, ok := parser.buildReduceChildrenNoAliasNoFieldsPlanned(entries, 0, len(entries), 3, lang.SymbolMetadata, &nodeArena{})
	if !ok {
		t.Fatal("buildReduceChildrenNoAliasNoFieldsPlanned returned ok=false")
	}
	if got, want := len(children), 1; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	if got, want := children[0].startByte, uint32(4); got != want {
		t.Fatalf("visible child startByte = %d, want %d", got, want)
	}
}

func TestDroppedHiddenSiblingPaddingDoesNotWidenFollowingExternalAnonymousLeaf(t *testing.T) {
	hidden := NewLeafNode(2, false, 5, 6, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 6})
	visibleAnon := NewLeafNode(1, false, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})
	visibleAnon.setExternalScannerToken(true)
	entries := []stackEntry{
		newStackEntryNode(0, hidden),
		newStackEntryNode(0, visibleAnon),
	}
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "=", Visible: true, Named: false},
			{Name: "_hidden", Visible: false},
			{Name: "parent", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang}

	children, _, _, _, ok := parser.buildReduceChildrenNoAliasNoFieldsPlanned(entries, 0, len(entries), 3, lang.SymbolMetadata, &nodeArena{})
	if !ok {
		t.Fatal("buildReduceChildrenNoAliasNoFieldsPlanned returned ok=false")
	}
	if got, want := len(children), 1; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	if got, want := children[0].startByte, uint32(6); got != want {
		t.Fatalf("anonymous leaf startByte = %d, want %d", got, want)
	}
	if got, want := children[0].endByte, uint32(7); got != want {
		t.Fatalf("anonymous leaf endByte = %d, want %d", got, want)
	}
}

func TestFlattenedHiddenWrapperDoesNotWidenExternalAnonymousLeaf(t *testing.T) {
	visibleAnon := NewLeafNode(1, false, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})
	visibleAnon.setExternalScannerToken(true)
	hidden := NewParentNode(2, false, []*Node{visibleAnon}, nil, 0)
	hidden.startByte = 5
	hidden.endByte = 7
	hidden.startPoint = Point{Row: 0, Column: 5}
	hidden.endPoint = Point{Row: 0, Column: 7}
	symbolMeta := []SymbolMetadata{
		{},
		{Name: "=", Visible: true, Named: false},
		{Name: "_hidden", Visible: false},
	}

	scratch := &reduceBuildScratch{}
	appendFlattenedHiddenChildrenToScratch(scratch, hidden, symbolMeta, nil)
	if got, want := len(scratch.nodes), 1; got != want {
		t.Fatalf("flattened child count = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].startByte, uint32(6); got != want {
		t.Fatalf("external anonymous leaf startByte = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].endByte, uint32(7); got != want {
		t.Fatalf("external anonymous leaf endByte = %d, want %d", got, want)
	}
}

func TestFlattenedHiddenWrapperDoesNotWidenOrdinaryAnonymousLeaf(t *testing.T) {
	visibleAnon := NewLeafNode(1, false, 8, 11, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 11})
	hidden := NewParentNode(2, false, []*Node{visibleAnon}, nil, 0)
	hidden.startByte = 7
	hidden.endByte = 11
	hidden.startPoint = Point{Row: 0, Column: 7}
	hidden.endPoint = Point{Row: 0, Column: 11}
	symbolMeta := []SymbolMetadata{
		{},
		{Name: "nil", Visible: true, Named: false},
		{Name: "_hidden", Visible: false},
	}

	scratch := &reduceBuildScratch{}
	appendFlattenedHiddenChildrenToScratch(scratch, hidden, symbolMeta, nil)
	if got, want := len(scratch.nodes), 1; got != want {
		t.Fatalf("flattened child count = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].startByte, uint32(8); got != want {
		t.Fatalf("ordinary anonymous leaf startByte = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].endByte, uint32(11); got != want {
		t.Fatalf("ordinary anonymous leaf endByte = %d, want %d", got, want)
	}
}

func TestFlattenedGeneratedRepeatPaddingDoesNotWidenAnonymousLeaf(t *testing.T) {
	visibleAnon := NewLeafNode(1, false, 3, 5, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 5})
	hiddenRepeat := NewParentNode(2, false, []*Node{visibleAnon}, nil, 0)
	hiddenRepeat.startByte = 2
	hiddenRepeat.endByte = 5
	hiddenRepeat.startPoint = Point{Row: 0, Column: 2}
	hiddenRepeat.endPoint = Point{Row: 0, Column: 5}
	symbolMeta := []SymbolMetadata{
		{},
		{Name: "//", Visible: true, Named: false},
		{Name: "block_comment_repeat1", Visible: false, GeneratedRepeatAux: true},
	}

	scratch := &reduceBuildScratch{}
	appendFlattenedHiddenChildrenToScratch(scratch, hiddenRepeat, symbolMeta, nil)
	if got, want := len(scratch.nodes), 1; got != want {
		t.Fatalf("flattened child count = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].startByte, uint32(3); got != want {
		t.Fatalf("generated-repeat anonymous leaf startByte = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].endByte, uint32(5); got != want {
		t.Fatalf("generated-repeat anonymous leaf endByte = %d, want %d", got, want)
	}
}

func TestFlattenedGeneratedRepeatPaddingWidensAnonymousWrapper(t *testing.T) {
	inner := NewLeafNode(3, true, 3, 5, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 5})
	visibleAnon := NewParentNode(1, false, []*Node{inner}, nil, 0)
	visibleAnon.startByte = 3
	visibleAnon.endByte = 5
	visibleAnon.startPoint = Point{Row: 0, Column: 3}
	visibleAnon.endPoint = Point{Row: 0, Column: 5}
	hiddenRepeat := NewParentNode(2, false, []*Node{visibleAnon}, nil, 0)
	hiddenRepeat.startByte = 2
	hiddenRepeat.endByte = 5
	hiddenRepeat.startPoint = Point{Row: 0, Column: 2}
	hiddenRepeat.endPoint = Point{Row: 0, Column: 5}
	symbolMeta := []SymbolMetadata{
		{},
		{Name: "_wrapper", Visible: true, Named: false},
		{Name: "block_comment_repeat1", Visible: false, GeneratedRepeatAux: true},
		{Name: "content", Visible: true, Named: true},
	}

	scratch := &reduceBuildScratch{}
	appendFlattenedHiddenChildrenToScratch(scratch, hiddenRepeat, symbolMeta, nil)
	if got, want := len(scratch.nodes), 1; got != want {
		t.Fatalf("flattened child count = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].startByte, uint32(2); got != want {
		t.Fatalf("generated-repeat anonymous wrapper startByte = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].endByte, uint32(5); got != want {
		t.Fatalf("generated-repeat anonymous wrapper endByte = %d, want %d", got, want)
	}
}

// TestFlattenedSiblingGapDoesNotWidenAliasedWrapper is the family-S
// regression fixture (spore.2026-08-02.maple-e.shared-divergence-census;
// elm/aliased-span-fix): a minimal, hand-built shape of python's
// `1 in 1 not in 1` -- an aliased multi-token wrapper (visible, unnamed, has
// children -- the same shape TestFlattenedGeneratedRepeatPaddingWidensAnonymousWrapper
// legitimately widens) sits as the FIRST flattened node of the SECOND
// element of an operator repeat, with a one-byte unclaimed gap (the
// separating space) between it and the first element's already-finished,
// disjoint sibling subtree.
//
// The two cases share flattenedHiddenPaddingTarget's exact "visible,
// unnamed, has children" shape check and must be told apart by whether the
// padding source's span actually reaches the candidate's start
// (flattenedHiddenPaddingSourceReachesTarget): the legitimate case's source
// is the wrapper's own enclosing hidden node, which by construction always
// spans at least up to its descendant; here the source is a finished,
// disjoint PRECEDING SIBLING two levels up (elem1), whose span ends before
// the wrapper begins. Before the fix, elem1's trailing edge leaked across
// that sibling boundary and pulled the wrapper's start back over the gap
// (matching the measured Go span 10..17 vs C's 11..17 on
// `a = 1 in 1 not in 1`).
func TestFlattenedSiblingGapDoesNotWidenAliasedWrapper(t *testing.T) {
	op1 := NewLeafNode(3, false, 0, 2, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 2})
	elem1 := NewParentNode(4, false, []*Node{op1}, nil, 0)
	elem1.startByte, elem1.endByte = 0, 2
	elem1.startPoint, elem1.endPoint = Point{Row: 0, Column: 0}, Point{Row: 0, Column: 2}

	// Byte 2 is an unclaimed gap (the separating space); the wrapper's own
	// content does not start until byte 3.
	content := NewLeafNode(2, true, 3, 5, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 5})
	wrapper := NewParentNode(1, false, []*Node{content}, nil, 0)
	wrapper.startByte, wrapper.endByte = 3, 5
	wrapper.startPoint, wrapper.endPoint = Point{Row: 0, Column: 3}, Point{Row: 0, Column: 5}
	elem2 := NewParentNode(4, false, []*Node{wrapper}, nil, 0)
	elem2.startByte, elem2.endByte = 3, 5
	elem2.startPoint, elem2.endPoint = Point{Row: 0, Column: 3}, Point{Row: 0, Column: 5}

	repeatList := NewParentNode(5, false, []*Node{elem1, elem2}, nil, 0)
	repeatList.startByte, repeatList.endByte = 0, 5
	repeatList.startPoint, repeatList.endPoint = Point{Row: 0, Column: 0}, Point{Row: 0, Column: 5}

	symbolMeta := []SymbolMetadata{
		{},
		{Name: "not_in", Visible: true, Named: false},
		{Name: "content", Visible: true, Named: true},
		{Name: "op", Visible: true, Named: false},
		{Name: "_repeat_elem", Visible: false},
		{Name: "_repeat_list", Visible: false},
	}

	scratch := &reduceBuildScratch{}
	appendFlattenedHiddenChildrenToScratch(scratch, repeatList, symbolMeta, nil)
	if got, want := len(scratch.nodes), 2; got != want {
		t.Fatalf("flattened child count = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[0].startByte, uint32(0); got != want {
		t.Fatalf("first element startByte = %d, want %d", got, want)
	}
	if got, want := scratch.nodes[1].startByte, uint32(3); got != want {
		t.Fatalf("aliased wrapper startByte = %d, want %d (must not widen across the preceding sibling's gap)", got, want)
	}
	if got, want := scratch.nodes[1].endByte, uint32(5); got != want {
		t.Fatalf("aliased wrapper endByte = %d, want %d", got, want)
	}
}

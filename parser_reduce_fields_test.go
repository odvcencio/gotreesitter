package gotreesitter

import "testing"

func TestParserFieldMapFieldNames(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.FieldCount = 1
	lang.FieldNames = []string{"", "value"}

	// Production 0 (expr -> NUMBER) has one child; map it to field ID 1.
	lang.FieldMapSlices = [][2]uint16{
		{0, 1},
	}
	lang.FieldMapEntries = []FieldMapEntry{
		{FieldID: 1, ChildIndex: 0, Inherited: false},
	}

	lang.ParseActions[2].Actions[0].ProductionID = 0
	lang.ParseActions[7].Actions[0].ProductionID = 1
	lang.ProductionIDCount = 2

	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("42"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}

	fieldChild := root.ChildByFieldName("value", lang)
	if fieldChild == nil {
		t.Fatal("expected field-mapped child by name \"value\"")
	}
	if fieldChild.Symbol() != 1 {
		t.Errorf("field child symbol = %d, want 1 (NUMBER)", fieldChild.Symbol())
	}
	if fieldChild.Text(tree.Source()) != "42" {
		t.Errorf("field child text = %q, want %q", fieldChild.Text(tree.Source()), "42")
	}
}

func TestBuildResultFoldExtrasPreservesFieldMappings(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.FieldCount = 1
	lang.FieldNames = []string{"", "value"}
	parser := NewParser(lang)

	source := []byte(" 42 ")

	leadingExtra := NewLeafNode(2, false, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	leadingExtra.setExtra(true)

	valueChild := NewLeafNode(1, true, 1, 3, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 3})
	realRoot := NewParentNode(3, true, []*Node{valueChild}, []FieldID{1}, 0)

	trailingExtra := NewLeafNode(2, false, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	trailingExtra.setExtra(true)

	stack := []stackEntry{
		newStackEntryNode(0, leadingExtra),
		newStackEntryNode(0, realRoot),
		newStackEntryNode(0, trailingExtra),
	}

	tree := parser.buildResult(stack, source, nil, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResult returned nil tree/root")
	}
	root := tree.RootNode()
	if root != realRoot {
		t.Fatal("expected folded result to reuse real root node")
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}
	if root.Child(0) != leadingExtra || root.Child(1) != valueChild || root.Child(2) != trailingExtra {
		t.Fatalf("unexpected child order after folding extras")
	}

	fieldChild := root.ChildByFieldName("value", lang)
	if fieldChild == nil {
		t.Fatal("expected field-mapped child by name \"value\"")
	}
	if fieldChild != valueChild {
		t.Fatal("field mapping shifted after folding extras")
	}
	if len(root.fieldIDs()) != 3 || root.fieldIDs()[1] != 1 {
		t.Fatalf("fieldIDs not re-aligned after folding extras: %#v", root.fieldIDs())
	}
	if leadingExtra.Parent() != root || trailingExtra.Parent() != root {
		t.Fatal("extra child parent pointers were not updated during fold")
	}
}

func TestBuildReduceChildrenHiddenChildDoesNotDuplicateExistingField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "!=", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "!=", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "operators"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	operator := newLeafNodeInArena(arena, 2, false, 0, 2, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 2})
	rhs := newLeafNodeInArena(arena, 3, true, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{operator, rhs}, []FieldID{1, 0}, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 3, 0, arena)
	if got, want := len(children), 2; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 2; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenDefersDirectHiddenFieldUntilVisibleBoundary(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_inner", "_outer", ".", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false},
			{Name: "_inner", Visible: false},
			{Name: "_outer", Visible: false},
			{Name: ".", Visible: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "path"},
		FieldMapSlices: [][2]uint16{{0, 1}, {1, 0}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	dot0 := newLeafNodeInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1})
	a := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	dot1 := newLeafNodeInArena(arena, 3, false, 2, 3, Point{Column: 2}, Point{Column: 3})
	b := newLeafNodeInArena(arena, 4, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	inner := newParentNodeInArena(arena, 1, false, []*Node{dot0, a, dot1, b}, nil, 0)

	children, fieldIDs, fieldSources := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, inner)}, 0, 1, 1, 2, 0, arena,
	)
	if got, want := len(children), 1; got != want {
		t.Fatalf("deferred len(children) = %d, want %d", got, want)
	}
	if children[0] != inner {
		t.Fatal("deferred child does not retain the hidden subtree")
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("deferred field ID = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 0), uint8(fieldSourceDeferredDirect); got != want {
		t.Fatalf("deferred field source = %d, want %d", got, want)
	}

	// _outer's entry for _inner's position is direct (non-inherited), not
	// deferred-to-inherited: it is C's carried fallback for every descendant
	// _inner flattens to, punctuation included, exactly like the scala
	// `import foo.bar.Baz` case field-maps "path" directly onto
	// _namespace_expression's whole sep1(".", identifier) slot
	// (tree-sitter-scala grammar.js:229-232) and the "." separators inside
	// the generated repeat helper inherit "path" from that level. See
	// ts_node_field_name_for_child (tree-sitter lib/src/node.c:689-729):
	// a !inherited entry at the position being descended through sets
	// inherited_field_name for everything reached beneath it.
	outer := newParentNodeInArenaWithFieldSources(arena, 2, false, children, fieldIDs, fieldSources, 0)
	children, fieldIDs, fieldSources = parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, outer)}, 0, 1, 1, 5, 1, arena,
	)
	if got, want := len(children), 4; got != want {
		t.Fatalf("visible len(children) = %d, want %d", got, want)
	}
	wantChildren := []*Node{dot0, a, dot1, b}
	wantFields := []FieldID{1, 1, 1, 1}
	wantSources := []uint8{fieldSourceDirect, fieldSourceDirect, fieldSourceDirect, fieldSourceDirect}
	for i := range wantChildren {
		if children[i] != wantChildren[i] {
			t.Fatalf("children[%d] = %p, want %p", i, children[i], wantChildren[i])
		}
		if fieldIDs[i] != wantFields[i] {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, fieldIDs[i], wantFields[i])
		}
		if got := fieldSourceAt(fieldSources, i); got != wantSources[i] {
			t.Fatalf("fieldSources[%d] = %d, want %d", i, got, wantSources[i])
		}
	}
}

// TestBuildReduceChildrenDeferredInheritedFieldStaysEmptyAlongsideSiblingDirectField
// covers a two-level hidden chain: _outer wraps [_inner (no field map of
// its own), tail]. _outer's own production carries an inherited entry over
// the hidden position and an unrelated direct entry over the sibling
// visible position. Both entries name the same field id.
//
// The test used to also assert which deferred-source marker the inherited
// edge received: fieldSourceDeferredInherited or the since-removed
// fieldSourceDeferredInheritedLater. That distinction never changed the
// resolved field. applyParentFieldToFlattenedHiddenSpan ignored it, and the
// O(remaining) fieldIDAppearsLater scan that computed it was dead work on
// the reduce hot path. PR #638 removed both.
//
// This test still verifies the part that matters. The inherited edge
// resolves to nothing, because no deeper non-inherited entry claims it. The
// sibling's own direct entry resolves independently at its own position.
func TestBuildReduceChildrenDeferredInheritedFieldStaysEmptyAlongsideSiblingDirectField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_inner", "_outer", "operator", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false},
			{Name: "_inner", Visible: false},
			{Name: "_outer", Visible: false},
			{Name: "operator", Visible: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "value"},
		FieldMapSlices: [][2]uint16{{0, 2}, {2, 0}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 1, ChildIndex: 1},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	op := newLeafNodeInArena(arena, 3, false, 0, 1, Point{}, Point{Column: 1})
	value := newLeafNodeInArena(arena, 4, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	inner := newParentNodeInArena(arena, 1, false, []*Node{op, value}, nil, 0)
	tail := newLeafNodeInArena(arena, 4, true, 2, 3, Point{Column: 2}, Point{Column: 3})

	children, fieldIDs, fieldSources := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, inner), newStackEntryNode(0, tail)}, 0, 2, 2, 2, 0, arena,
	)
	if got, want := len(children), 2; got != want {
		t.Fatalf("deferred len(children) = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 0), uint8(fieldSourceDeferredInherited); got != want {
		t.Fatalf("first deferred field source = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 1), uint8(fieldSourceDirect); got != want {
		t.Fatalf("later field source = %d, want %d", got, want)
	}

	outer := newParentNodeInArenaWithFieldSources(arena, 2, false, children, fieldIDs, fieldSources, 0)
	children, fieldIDs, fieldSources = parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, outer)}, 0, 1, 1, 5, 1, arena,
	)
	wantChildren := []*Node{op, value, tail}
	wantFields := []FieldID{0, 0, 1}
	wantSources := []uint8{fieldSourceNone, fieldSourceNone, fieldSourceDirect}
	for i := range wantChildren {
		if children[i] != wantChildren[i] {
			t.Fatalf("children[%d] = %p, want %p", i, children[i], wantChildren[i])
		}
		if fieldIDs[i] != wantFields[i] {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, fieldIDs[i], wantFields[i])
		}
		if got := fieldSourceAt(fieldSources, i); got != wantSources[i] {
			t.Fatalf("fieldSources[%d] = %d, want %d", i, got, wantSources[i])
		}
	}
}

func TestBuildReduceChildrenDeferredDirectCountsAsRepeatedField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_inner", "_outer", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false},
			{Name: "_inner", Visible: false},
			{Name: "_outer", Visible: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "path", "other"},
		FieldMapSlices: [][2]uint16{{0, 2}, {2, 0}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0},
			{FieldID: 1, ChildIndex: 2},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	a := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
	b := newLeafNodeInArena(arena, 3, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	c := newLeafNodeInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	d := newLeafNodeInArena(arena, 3, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	middle := newParentNodeInArenaWithFieldSources(
		arena, 1, false, []*Node{b, c}, []FieldID{2, 2}, []uint8{fieldSourceInherited, fieldSourceInherited}, 0,
	)
	tail := newParentNodeInArena(arena, 1, false, []*Node{d}, nil, 0)

	compactChildren, compactIDs, compactSources := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, a), newStackEntryNode(0, middle), newStackEntryNode(0, tail)},
		0, 3, 3, 2, 0, arena,
	)
	if got, want := len(compactChildren), 3; got != want {
		t.Fatalf("compact len(children) = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(compactSources, 2), uint8(fieldSourceDeferredDirect); got != want {
		t.Fatalf("tail field source = %d, want %d", got, want)
	}
	outer := newParentNodeInArenaWithFieldSources(arena, 2, false, compactChildren, compactIDs, compactSources, 0)

	assertFlattened := func(label string, children []*Node, fieldIDs []FieldID, fieldSources []uint8) {
		t.Helper()
		wantChildren := []*Node{a, b, c, d}
		if got, want := len(children), len(wantChildren); got != want {
			t.Fatalf("%s len(children) = %d, want %d", label, got, want)
		}
		for i := range wantChildren {
			if children[i] != wantChildren[i] {
				t.Fatalf("%s children[%d] = %p, want %p", label, i, children[i], wantChildren[i])
			}
			if got, want := fieldIDs[i], FieldID(1); got != want {
				t.Fatalf("%s fieldIDs[%d] = %d, want %d", label, i, got, want)
			}
			if got, want := fieldSourceAt(fieldSources, i), uint8(fieldSourceDirect); got != want {
				t.Fatalf("%s fieldSources[%d] = %d, want %d", label, i, got, want)
			}
		}
	}

	children, fieldIDs, fieldSources := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, outer)}, 0, 1, 1, 4, 1, arena,
	)
	assertFlattened("scratch", children, fieldIDs, fieldSources)

	arrayChildren := make([]*Node, 4)
	arrayIDs := make([]FieldID, 4)
	arraySources := make([]uint8, 4)
	if got, want := appendFlattenedHiddenChildrenWithFields(arrayChildren, arrayIDs, arraySources, 0, outer, lang.SymbolMetadata, nil), 4; got != want {
		t.Fatalf("array flattened count = %d, want %d", got, want)
	}
	assertFlattened("array", arrayChildren, arrayIDs, arraySources)
}

// TestBuildReduceChildrenInheritedFieldDoesNotOverrideAlreadyResolvedInnerField
// covers a hidden child whose own children already carry field data (of a
// different field id) when the enclosing production also marks that hidden
// child's position inherited. C never uses an inherited field-map entry to
// name a child directly (ts_node__field_name_from_language, tree-sitter
// lib/src/node.c:673-687, filters !field_map->inherited unconditionally), so
// an inherited entry at this outer level can never override what a deeper
// level already resolved -- it can only confirm it or add nothing. This test
// used to assert the opposite (the outer inherited field replacing the
// already-resolved inner one); that was the fleet-wide field over-projection
// bug (finding.production-divergence-census-2026-08-02).
func TestBuildReduceChildrenInheritedFieldDoesNotOverrideAlreadyResolvedInnerField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "type_identifier", "with", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "with", Visible: true, Named: false},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "type", "arguments"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 2, true, 0, 12, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 12})
	withTok := newLeafNodeInArena(arena, 3, false, 13, 17, Point{Row: 0, Column: 13}, Point{Row: 0, Column: 17})
	right := newLeafNodeInArena(arena, 2, true, 18, 25, Point{Row: 0, Column: 18}, Point{Row: 0, Column: 25})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{left, withTok, right}, []FieldID{2, 2, 2}, 0)
	// fieldSourceDirect models a deeper level that already resolved this
	// field. The outer reduction must leave the assignment alone.
	hidden.setFieldSources([]uint8{fieldSourceDirect, fieldSourceDirect, fieldSourceDirect})

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	for i, fid := range fieldIDs {
		if got, want := fid, FieldID(2); got != want {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, got, want)
		}
	}
}

// TestBuildReduceChildrenLeavesMatchedInheritedFieldConflictUnprojected is a
// regression witness against a return of type-name-matching field
// projection. The child symbol names ("imports", "declarations") equal the
// conflicting field names. C still assigns no field here: its
// ts_node__field_name_from_language (node.c:673-687) filters every
// inherited entry and stops. C never matches a child's type name against
// the field map. projectConflictedInheritedFields did that match. PR #638
// removed the function, after a review disabled it and measured identical
// output on a 52-language sweep. This test pairs with
// TestBuildReduceChildrenLeavesUnmatchedInheritedFieldConflictUnprojected,
// which covers the same conflict shape with names that do not match.
func TestBuildReduceChildrenLeavesMatchedInheritedFieldConflictUnprojected(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_sections", "imports", "declarations", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false},
			{Name: "_sections", Visible: false, Named: true},
			{Name: "imports", Visible: true, Named: true},
			{Name: "declarations", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "imports", "declarations"},
		FieldMapSlices: [][2]uint16{{0, 2}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: true},
		},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 0}}},
		},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	imports := newLeafNodeInArena(arena, 2, true, 0, 7, Point{}, Point{Column: 7})
	declarations := newLeafNodeInArena(arena, 3, true, 8, 20, Point{Column: 8}, Point{Column: 20})
	sections := newParentNodeInArena(arena, 1, true, []*Node{imports, declarations}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, sections)},
		0,
		1,
		1,
		4,
		0,
		arena,
	)
	if got, want := len(children), 2; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	if fieldIDSliceHasAny(fieldIDs) {
		t.Fatalf("matched conflict fields = %v, want none", fieldIDs)
	}
}

func TestBuildReduceChildrenLeavesUnmatchedInheritedFieldConflictUnprojected(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_sections", "header", "body", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false},
			{Name: "_sections", Visible: false, Named: true},
			{Name: "header", Visible: true, Named: true},
			{Name: "body", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "imports", "declarations"},
		FieldMapSlices: [][2]uint16{{0, 2}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: true},
		},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	header := newLeafNodeInArena(arena, 2, true, 0, 6, Point{}, Point{Column: 6})
	body := newLeafNodeInArena(arena, 3, true, 7, 11, Point{Column: 7}, Point{Column: 11})
	sections := newParentNodeInArena(arena, 1, true, []*Node{header, body}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, sections)},
		0,
		1,
		1,
		4,
		0,
		arena,
	)
	if got, want := len(children), 2; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	if fieldIDSliceHasAny(fieldIDs) {
		t.Fatalf("unmatched conflict fields = %v, want none", fieldIDs)
	}
}

func TestBuildReduceChildrenDirectFieldOverridesSingleIndirectNamedChild(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "type_identifier", "arguments", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "arguments", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "type", "arguments"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	typ := newLeafNodeInArena(arena, 2, true, 0, 9, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 9})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{typ}, []FieldID{2}, 0)
	hidden.setFieldSources([]uint8{fieldSourceInherited})

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 1; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenDirectFieldKeepsInnerHiddenProjection(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_inner", "type_identifier", "user_type", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_inner", Visible: false, Named: false},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "user_type", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "name", "type"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 2, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	typeIdentifier := newLeafNodeInArena(arena, 2, true, 0, 3, Point{}, Point{Column: 3})
	userType := newParentNodeInArena(arena, 3, true, []*Node{typeIdentifier}, nil, 0)
	inner := newParentNodeInArena(arena, 1, false, []*Node{userType}, []FieldID{1}, 0)
	inner.setFieldSources([]uint8{fieldSourceDeferredDirect})

	children, fieldIDs, fieldSources := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, inner)}, 0, 1, 1, 4, 0, arena,
	)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 0), uint8(fieldSourceDirect); got != want {
		t.Fatalf("fieldSources[0] = %d, want %d", got, want)
	}
}

// TestBuildReduceChildrenInheritedFieldWithNoDirectEntryAnywhereProjectsNothing
// covers a hidden child whose own production declares no fields at all, spliced
// under a parent whose only field-map entry for that position is inherited. C
// only ever answers a field query with a genuine !inherited entry found
// somewhere along the descent (ts_node_field_name_for_child, tree-sitter
// lib/src/node.c:689-729); with no such entry at any level here, C's answer is
// empty for every descendant, not just the punctuation. This test used to
// assert that the first (named, non-leaf-adjacent) descendant received the
// field anyway -- picking a target from shape (position, leaf-ness) rather
// than from a real grammar declaration was the fleet-wide field
// over-projection bug (finding.production-divergence-census-2026-08-02).
func TestBuildReduceChildrenInheritedFieldWithNoDirectEntryAnywhereProjectsNothing(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "identifier", ".", "namespace_wildcard", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "namespace_wildcard", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "path"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	head := newLeafNodeInArena(arena, 2, true, 0, 5, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 5})
	dot := newLeafNodeInArena(arena, 3, false, 5, 6, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 6})
	tail := newLeafNodeInArena(arena, 4, true, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{head, dot, tail}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 5, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
	if got := fieldIDs[2]; got != 0 {
		t.Fatalf("fieldIDs[2] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenInheritedFieldSkipsNamedHiddenSpanWithMultipleNamedTargets(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_join_header", "identifier", "in", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_join_header", Visible: false, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "in", Visible: true, Named: false},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "type"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 2, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	inTok := newLeafNodeInArena(arena, 3, false, 2, 4, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 4})
	right := newLeafNodeInArena(arena, 2, true, 5, 6, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 6})
	hidden := newParentNodeInArena(arena, 1, true, []*Node{left, inTok, right}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	for i, fid := range fieldIDs {
		if fid != 0 {
			t.Fatalf("fieldIDs[%d] = %d, want 0", i, fid)
		}
	}
}

func TestBuildReduceChildrenDirectFieldPrefersNamedTargetsOnFlattenedSpan(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", ".", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "path"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	dot0 := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	net := newLeafNodeInArena(arena, 3, true, 5, 8, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 8})
	dot1 := newLeafNodeInArena(arena, 2, false, 8, 9, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 9})
	url := newLeafNodeInArena(arena, 3, true, 9, 12, Point{Row: 0, Column: 9}, Point{Row: 0, Column: 12})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{dot0, net, dot1, url}, []FieldID{0, 1, 0, 1}, 0)
	hidden.setFieldSources([]uint8{fieldSourceNone, fieldSourceDirect, fieldSourceNone, fieldSourceDirect})

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 4; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 4; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got, want := fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("fieldIDs[1] = %d, want %d", got, want)
	}
	if got := fieldIDs[2]; got != 0 {
		t.Fatalf("fieldIDs[2] = %d, want 0", got)
	}
	if got, want := fieldIDs[3], FieldID(1); got != want {
		t.Fatalf("fieldIDs[3] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenRepeatedDirectFieldOnHiddenPathLeavesAnonymousGapUnfielded(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", ".", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "path"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	java := newLeafNodeInArena(arena, 3, true, 0, 4, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 4})
	dot0 := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	net := newLeafNodeInArena(arena, 3, true, 5, 8, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 8})
	dot1 := newLeafNodeInArena(arena, 2, false, 8, 9, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 9})
	url := newLeafNodeInArena(arena, 3, true, 9, 12, Point{Row: 0, Column: 9}, Point{Row: 0, Column: 12})

	tail := newParentNodeInArena(arena, 1, false, []*Node{net, dot1, url}, []FieldID{1, 0, 1}, 0)
	tail.setFieldSources([]uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect})
	outer := newParentNodeInArena(arena, 1, false, []*Node{java, dot0, tail}, []FieldID{1, 0, 1}, 0)
	outer.setFieldSources([]uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect})

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, outer)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 5; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 5; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
	if got, want := fieldIDs[2], FieldID(1); got != want {
		t.Fatalf("fieldIDs[2] = %d, want %d", got, want)
	}
	if got := fieldIDs[3]; got != 0 {
		t.Fatalf("fieldIDs[3] = %d, want 0", got)
	}
	if got, want := fieldIDs[4], FieldID(1); got != want {
		t.Fatalf("fieldIDs[4] = %d, want %d", got, want)
	}
}

// TestBuildReduceChildrenInheritedFieldLeavesRepeatedDirectAnonymousGapsUnfielded
// covers a hidden child whose own production wraps the field around each
// repeat element independently (java DOT net DOT url, each identifier's own
// direct entry, like python's commaSep1(field('name', ...)) shape
// (tree-sitter-python grammar.js:175-181) rather than around the whole
// repeat as one unit). C resolves the field for each child with its own
// independent descent (ts_node_field_name_for_child, tree-sitter
// lib/src/node.c:689-729): the separators have no field-map entry of their
// own at any level, so C never assigns them one just because same-fielded
// siblings surround them. Verified against the C oracle: `from a import b,
// c` in python leaves the comma fieldless
// (finding.production-divergence-census-2026-08-02). This test used to
// assert the opposite -- that the gaps inherit the surrounding field -- which
// was exactly that over-projection bug.
func TestBuildReduceChildrenInheritedFieldLeavesRepeatedDirectAnonymousGapsUnfielded(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", ".", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "path"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	java := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	dot0 := newLeafNodeInArena(arena, 2, false, 4, 5, Point{Column: 4}, Point{Column: 5})
	net := newLeafNodeInArena(arena, 3, true, 5, 8, Point{Column: 5}, Point{Column: 8})
	dot1 := newLeafNodeInArena(arena, 2, false, 8, 9, Point{Column: 8}, Point{Column: 9})
	url := newLeafNodeInArena(arena, 3, true, 9, 12, Point{Column: 9}, Point{Column: 12})
	hidden := newParentNodeInArena(
		arena,
		1,
		false,
		[]*Node{java, dot0, net, dot1, url},
		[]FieldID{1, 0, 1, 0, 1},
		0,
	)
	hidden.setFieldSources([]uint8{
		fieldSourceDirect,
		fieldSourceNone,
		fieldSourceDirect,
		fieldSourceNone,
		fieldSourceDirect,
	})

	children, fieldIDs, fieldSources := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, hidden)},
		0,
		1,
		1,
		4,
		0,
		arena,
	)
	if got, want := len(children), 5; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	wantFieldIDs := []FieldID{1, 0, 1, 0, 1}
	for index, want := range wantFieldIDs {
		if got := fieldIDs[index]; got != want {
			t.Fatalf("field %d = %d, want %d", index, got, want)
		}
		if want == 0 {
			continue
		}
		if got, want := fieldSourceAt(fieldSources, index), uint8(fieldSourceDirect); got != want {
			t.Fatalf("field source %d = %d, want %d", index, got, want)
		}
	}
}

func TestBuildReduceChildrenInheritedFieldDoesNotCrossLeadingSeparator(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", ",", "identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: ",", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "into"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	comma := newLeafNodeInArena(arena, 2, false, 0, 1, Point{}, Point{Column: 1})
	identifier := newLeafNodeInArena(arena, 3, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{comma, identifier}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren(
		[]stackEntry{newStackEntryNode(0, hidden)},
		0,
		1,
		1,
		4,
		0,
		arena,
	)
	if got, want := len(children), 2; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	if fieldIDSliceHasAny(fieldIDs) {
		t.Fatalf("fields = %v, want none", fieldIDs)
	}
}

func TestBuildReduceChildrenInheritedFieldYieldsToDirectTargetOnHiddenSpan(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "modifiers", "def", "identifier", ":", "type_identifier", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "modifiers", Visible: true, Named: true},
			{Name: "def", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ":", Visible: true, Named: false},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "return_type", "name"},
	}

	arena := newNodeArena(arenaClassFull)
	modifiers := newLeafNodeInArena(arena, 2, true, 0, 7, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 7})
	defTok := newLeafNodeInArena(arena, 3, false, 8, 11, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 11})
	name := newLeafNodeInArena(arena, 4, true, 12, 18, Point{Row: 0, Column: 12}, Point{Row: 0, Column: 18})
	colon := newLeafNodeInArena(arena, 5, false, 18, 19, Point{Row: 0, Column: 18}, Point{Row: 0, Column: 19})
	retType := newLeafNodeInArena(arena, 6, true, 20, 23, Point{Row: 0, Column: 20}, Point{Row: 0, Column: 23})

	hidden := newParentNodeInArena(arena, 1, false, []*Node{modifiers, defTok, name, colon, retType}, []FieldID{1, 0, 2, 0, 1}, 0)
	hidden.setFieldSources([]uint8{fieldSourceInherited, fieldSourceNone, fieldSourceDirect, fieldSourceNone, fieldSourceDirect})

	children := arena.allocNodeSlice(5)
	fieldIDs := arena.allocFieldIDSlice(5)
	fieldSources := make([]uint8, 5)
	if got, want := appendFlattenedHiddenChildrenWithFields(children, fieldIDs, fieldSources, 0, hidden, lang.SymbolMetadata, nil), 5; got != want {
		t.Fatalf("appendFlattenedHiddenChildrenWithFields() = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got, want := fieldIDs[2], FieldID(2); got != want {
		t.Fatalf("fieldIDs[2] = %d, want %d", got, want)
	}
	if got := fieldIDs[3]; got != 0 {
		t.Fatalf("fieldIDs[3] = %d, want 0", got)
	}
	if got, want := fieldIDs[4], FieldID(1); got != want {
		t.Fatalf("fieldIDs[4] = %d, want %d", got, want)
	}
	if got, want := fieldSources[4], uint8(fieldSourceDirect); got != want {
		t.Fatalf("fieldSources[4] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenDirectFieldDoesNotSpreadToLeadingExtraComment(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "comment", "binding", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "comment", Visible: true, Named: true},
			{Name: "binding", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "binding"},
		FieldMapSlices: [][2]uint16{{0, 1}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	comment := newLeafNodeInArena(arena, 2, true, 0, 9, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 9})
	comment.setExtra(true)
	binding := newLeafNodeInArena(arena, 3, true, 10, 16, Point{Row: 0, Column: 10}, Point{Row: 0, Column: 16})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{comment, binding}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 2; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got, want := fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("fieldIDs[1] = %d, want %d", got, want)
	}
}

func TestAppendFlattenedHiddenChildrenRepeatedDirectFieldSkipsCommaSeparator(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "identifier", ",", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ",", Visible: true, Named: false},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "name"},
	}

	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 2, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	comma := newLeafNodeInArena(arena, 3, false, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2})
	right := newLeafNodeInArena(arena, 2, true, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{left, comma, right}, []FieldID{1, 0, 1}, 0)
	hidden.setFieldSources([]uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect})

	children := arena.allocNodeSlice(3)
	fieldIDs := arena.allocFieldIDSlice(3)
	fieldSources := make([]uint8, 3)
	if got, want := appendFlattenedHiddenChildrenWithFields(children, fieldIDs, fieldSources, 0, hidden, lang.SymbolMetadata, nil), 3; got != want {
		t.Fatalf("appendFlattenedHiddenChildrenWithFields() = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
	if got, want := fieldIDs[2], FieldID(1); got != want {
		t.Fatalf("fieldIDs[2] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenDirectFieldFillsSingleNamedHiddenSpanDelimiters(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "(", "list_expression", ")", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "(", Visible: true, Named: false},
			{Name: "list_expression", Visible: true, Named: true},
			{Name: ")", Visible: true, Named: false},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "right"},
		FieldMapSlices: [][2]uint16{{0, 1}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	open := newLeafNodeInArena(arena, 2, false, 10, 11, Point{Row: 0, Column: 10}, Point{Row: 0, Column: 11})
	list := newLeafNodeInArena(arena, 3, true, 11, 20, Point{Row: 0, Column: 11}, Point{Row: 0, Column: 20})
	close := newLeafNodeInArena(arena, 4, false, 20, 21, Point{Row: 0, Column: 20}, Point{Row: 0, Column: 21})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{open, list, close}, nil, 0)

	_, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 5, 0, arena)
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	for i, fid := range fieldIDs {
		if got, want := fid, FieldID(1); got != want {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestBuildReduceChildrenDirectFieldAssignsSingleAnonymousHiddenTarget(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "expression", "this", "member_access_expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "expression", Visible: false, Named: true},
			{Name: "this", Visible: true, Named: false},
			{Name: "member_access_expression", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "expression"},
		FieldMapSlices: [][2]uint16{{0, 1}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	thisTok := newLeafNodeInArena(arena, 2, false, 0, 4, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 4})
	hidden := newParentNodeInArena(arena, 1, true, []*Node{thisTok}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 3, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 1; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenInheritedFieldSkipsProjectionWhenFlattenedSpanHasDirectFields(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "modifier", "predefined_type", "identifier", "parameter_list", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "modifier", Visible: true, Named: true},
			{Name: "predefined_type", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "parameter_list", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "type", "name", "parameters", "type_parameters"},
		FieldMapSlices: [][2]uint16{{0, 1}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 4, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	modifier := newLeafNodeInArena(arena, 2, true, 0, 8, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 8})
	typ := newLeafNodeInArena(arena, 3, true, 9, 13, Point{Row: 0, Column: 9}, Point{Row: 0, Column: 13})
	name := newLeafNodeInArena(arena, 4, true, 14, 15, Point{Row: 0, Column: 14}, Point{Row: 0, Column: 15})
	params := newLeafNodeInArena(arena, 5, true, 15, 21, Point{Row: 0, Column: 15}, Point{Row: 0, Column: 21})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{modifier, typ, name, params}, []FieldID{0, 1, 2, 3}, 0)
	hidden.setFieldSources([]uint8{fieldSourceNone, fieldSourceDirect, fieldSourceDirect, fieldSourceDirect})

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 6, 0, arena)
	if got, want := len(children), 4; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got, want := fieldIDs[1], FieldID(1); got != want {
		t.Fatalf("fieldIDs[1] = %d, want %d", got, want)
	}
	if got, want := fieldIDs[2], FieldID(2); got != want {
		t.Fatalf("fieldIDs[2] = %d, want %d", got, want)
	}
	if got, want := fieldIDs[3], FieldID(3); got != want {
		t.Fatalf("fieldIDs[3] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenInheritedFieldSkipsProjectionWhenDescendantHasDirectFields(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "join", "identifier", ".", "member_access_expression", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "join", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "member_access_expression", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames:     []string{"", "type", "expression", "name"},
		FieldMapSlices: [][2]uint16{{0, 1}},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	joinTok := newLeafNodeInArena(arena, 2, false, 0, 4, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 4})
	ident := newLeafNodeInArena(arena, 3, true, 5, 6, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 6})
	exprBase := newLeafNodeInArena(arena, 3, true, 7, 8, Point{Row: 0, Column: 7}, Point{Row: 0, Column: 8})
	dot := newLeafNodeInArena(arena, 4, false, 8, 9, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 9})
	exprName := newLeafNodeInArena(arena, 3, true, 9, 11, Point{Row: 0, Column: 9}, Point{Row: 0, Column: 11})
	access := newParentNodeInArena(arena, 5, true, []*Node{exprBase, dot, exprName}, []FieldID{2, 0, 3}, 0)
	access.setFieldSources([]uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{joinTok, ident, access}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 6, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
	if got := fieldIDs[2]; got != 0 {
		t.Fatalf("fieldIDs[2] = %d, want 0", got)
	}
}

// TestBuildReduceChildrenInheritedFieldDoesNotProjectFromUnrelatedVisibleSiblingField
// covers a hidden child whose own production declares no fields, where one of
// its visible (opaque, not-further-flattened) children happens to carry a
// same-numbered field id on ITS OWN production for an unrelated reason. C
// only walks a hidden node's own field map when deciding what a child of the
// enclosing production inherits (ts_node_field_name_for_child, tree-sitter
// lib/src/node.c:689-729); once the target is itself a relevant (visible)
// node, C never looks inside that node's own children to justify a field on
// the node as a whole. This test used to assert that the coincidental match
// (matching purely because it was the sole descendant carrying any direct
// field id) was projected onto call's outer position -- that
// "single-descendant" heuristic was the fleet-wide field over-projection bug
// (finding.production-divergence-census-2026-08-02).
func TestBuildReduceChildrenInheritedFieldDoesNotProjectFromUnrelatedVisibleSiblingField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "call", "identifier", "arguments", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "call", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "arguments", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "target"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	ident := newLeafNodeInArena(arena, 3, true, 0, 7, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 7})
	callArgs := newLeafNodeInArena(arena, 4, true, 7, 10, Point{Row: 0, Column: 7}, Point{Row: 0, Column: 10})
	call := newParentNodeInArena(arena, 2, true, []*Node{ident, callArgs}, []FieldID{1, 0}, 0)
	call.setFieldSources([]uint8{fieldSourceDirect, fieldSourceNone})
	outerArgs := newLeafNodeInArena(arena, 4, true, 10, 13, Point{Row: 0, Column: 10}, Point{Row: 0, Column: 13})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{call, outerArgs}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 5, 0, arena)
	if got, want := len(children), 2; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got := fieldIDs[1]; got != 0 {
		t.Fatalf("fieldIDs[1] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenInheritedFieldSkipsSingleLeafHiddenProjectionWithoutDirectField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "variable_name", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "variable_name", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "operator"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	name := newLeafNodeInArena(arena, 2, true, 2, 14, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 14})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{name}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 3, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
}

// TestBuildReduceChildrenInheritedFieldSkipsSingleNonLeafHiddenProjectionWithoutDirectField
// mirrors the sibling leaf case above for a non-leaf single descendant: the
// hidden child's sole child (decl) has children of its own but declares no
// field anywhere along its own production. Whether the lone descendant is a
// leaf or not is irrelevant to C -- ts_node_field_name_for_child (tree-sitter
// lib/src/node.c:689-729) only ever answers from a genuine !inherited
// field-map entry found along the descent, never from "this position's
// child count." Verified against the C oracle: ada's `A : Integer_Array
// (1..3) := (others => 0)` leaves the aggregate fieldless even though it is
// the single, non-leaf child of term's inherited "name" position
// (finding.production-divergence-census-2026-08-02, ada grammar.js:561-566).
// This test used to assert the opposite -- that a single non-leaf descendant
// without its own field still received the parent's inherited field -- which
// was exactly that "has children" over-projection heuristic.
func TestBuildReduceChildrenInheritedFieldSkipsSingleNonLeafHiddenProjectionWithoutDirectField(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "local", "function_declaration", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "local", Visible: true, Named: false},
			{Name: "function_declaration", Visible: true, Named: true},
			{Name: "visible_parent", Visible: true, Named: true},
		},
		FieldNames: []string{"", "local_declaration"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	localTok := newLeafNodeInArena(arena, 2, false, 0, 5, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 5})
	decl := newParentNodeInArena(arena, 3, true, []*Node{localTok}, nil, 0)
	hidden := newParentNodeInArena(arena, 1, false, []*Node{decl}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenCarriesHiddenChildFieldsThroughFieldlessParent(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_inner", "_hidden_outer", "function_declaration", "chunk"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_inner", Visible: false, Named: false},
			{Name: "_hidden_outer", Visible: false, Named: false},
			{Name: "function_declaration", Visible: true, Named: true},
			{Name: "chunk", Visible: true, Named: true},
		},
		FieldNames: []string{"", "local_declaration"},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	fn := newLeafNodeInArena(arena, 3, true, 0, 8, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 8})
	inner := newParentNodeInArena(arena, 1, false, []*Node{fn}, []FieldID{1}, 0)
	inner.setFieldSources([]uint8{fieldSourceDirect})
	outer := newParentNodeInArena(arena, 2, false, []*Node{inner}, nil, 0)

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, outer)}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 0), uint8(fieldSourceDirect); got != want {
		t.Fatalf("fieldSources[0] = %d, want %d", got, want)
	}
}

func TestBuildFieldIDsSkipsConflictingInheritedEntriesOnSameChild(t *testing.T) {
	lang := &Language{
		FieldNames: []string{"", "name", "type"},
		FieldMapSlices: [][2]uint16{
			{0, 2},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	fieldIDs, inherited, _ := parser.buildFieldIDs(1, 0, arena)
	if got, want := len(fieldIDs), 1; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got := inherited[0]; got {
		t.Fatal("inherited[0] = true, want false")
	}
}

func TestBuildReduceChildrenDirectFieldWinsOverInheritedEntriesOnSameChild(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "function_declaration", "identifier", "parameters", "block", "declaration"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "function_declaration", Visible: true, Named: true},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "parameters", Visible: true, Named: true},
			{Name: "block", Visible: true, Named: true},
			{Name: "declaration", Visible: true, Named: true},
		},
		FieldNames: []string{"", "body", "local_declaration", "name", "parameters"},
		FieldMapSlices: [][2]uint16{
			{0, 4},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: true},
			{FieldID: 2, ChildIndex: 0, Inherited: false},
			{FieldID: 3, ChildIndex: 0, Inherited: true},
			{FieldID: 4, ChildIndex: 0, Inherited: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	name := newLeafNodeInArena(arena, 2, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	params := newLeafNodeInArena(arena, 3, true, 1, 3, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 3})
	body := newLeafNodeInArena(arena, 4, true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	decl := newParentNodeInArena(arena, 1, true, []*Node{name, params, body}, []FieldID{3, 4, 1}, 0)
	decl.setFieldSources([]uint8{fieldSourceDirect, fieldSourceDirect, fieldSourceDirect})

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, decl)}, 0, 1, 1, 5, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(2); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 0), uint8(fieldSourceDirect); got != want {
		t.Fatalf("fieldSources[0] = %d, want %d", got, want)
	}
}

func TestBuildReduceChildrenDartConstructorParamDoesNotReceiveDirectNameField(t *testing.T) {
	lang := &Language{
		Name:        "dart",
		SymbolNames: []string{"EOF", "formal_parameter", "constructor_param", "this", ".", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "formal_parameter", Visible: true, Named: true},
			{Name: "constructor_param", Visible: true, Named: true},
			{Name: "this", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "name"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	thisLeaf := newLeafNodeInArena(arena, 3, true, 0, 4, Point{}, Point{Column: 4})
	dot := newLeafNodeInArena(arena, 4, false, 4, 5, Point{Column: 4}, Point{Column: 5})
	name := newLeafNodeInArena(arena, 5, true, 5, 6, Point{Column: 5}, Point{Column: 6})
	constructorParam := newParentNodeInArena(arena, 2, true, []*Node{thisLeaf, dot, name}, nil, 0)

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, constructorParam)}, 0, 1, 1, 1, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got := fieldSourceAt(fieldSources, 0); got != 0 {
		t.Fatalf("fieldSources[0] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenDartHiddenConstructorParamDoesNotReceiveNameField(t *testing.T) {
	lang := &Language{
		Name:        "dart",
		SymbolNames: []string{"EOF", "formal_parameter", "_hidden", "constructor_param", "this", ".", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "formal_parameter", Visible: true, Named: true},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "constructor_param", Visible: true, Named: true},
			{Name: "this", Visible: true, Named: true},
			{Name: ".", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		FieldNames: []string{"", "name"},
		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	thisLeaf := newLeafNodeInArena(arena, 4, true, 0, 4, Point{}, Point{Column: 4})
	dot := newLeafNodeInArena(arena, 5, false, 4, 5, Point{Column: 4}, Point{Column: 5})
	name := newLeafNodeInArena(arena, 6, true, 5, 6, Point{Column: 5}, Point{Column: 6})
	constructorParam := newParentNodeInArena(arena, 3, true, []*Node{thisLeaf, dot, name}, nil, 0)
	hidden := newParentNodeInArena(arena, 2, false, []*Node{constructorParam}, []FieldID{1}, 0)
	hidden.setFieldSources([]uint8{fieldSourceDirect})

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden)}, 0, 1, 1, 1, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := children[0].Type(lang), "constructor_param"; got != want {
		t.Fatalf("children[0].Type() = %q, want %q", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
	if got := fieldSourceAt(fieldSources, 0); got != 0 {
		t.Fatalf("fieldSources[0] = %d, want 0", got)
	}
}
func TestBuildReduceChildrenNoAliasNoFieldsInlinesHiddenChildren(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "identifier", "operator"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "operator", Visible: true, Named: false},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 2, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	op := newLeafNodeInArena(arena, 3, false, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{left, op}, nil, 0)
	right := newLeafNodeInArena(arena, 2, true, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hidden), newStackEntryNode(0, right)}, 0, 2, 2, 2, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if fieldIDs != nil {
		t.Fatalf("fieldIDs = %#v, want nil", fieldIDs)
	}
	if children[0] != left || children[1] != op || children[2] != right {
		t.Fatalf("children order = %#v, want hidden children then right leaf", children)
	}
}

func TestPendingNoFieldChildCountRejectsHiddenFields(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
	}

	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{child}, []FieldID{1}, 0)
	entry := newStackEntryNode(0, hidden)

	if _, _, _, ok := pendingNoFieldChildCount(entry, arena, true, lang.SymbolMetadata, nil); ok {
		t.Fatal("pendingNoFieldChildCount accepted hidden field-bearing child; want reject")
	}
	if count, _, _, ok := pendingNoFieldChildCount(entry, arena, false, lang.SymbolMetadata, nil); !ok || count != 1 {
		t.Fatalf("hidden child under hidden parent count/ok = %d/%t, want 1/true", count, ok)
	}
}

func TestBuildReduceChildrenHiddenParentDefersFlattenUntilVisibleBoundary(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"EOF", "_hidden_a", "_hidden_b", "identifier", "operator", "visible_parent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_hidden_a", Visible: false, Named: false},
			{Name: "_hidden_b", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "operator", Visible: true, Named: false},
			{Name: "visible_parent", Visible: true, Named: true},
		},
	}

	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	left := newLeafNodeInArena(arena, 3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	op := newLeafNodeInArena(arena, 4, false, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})
	right := newLeafNodeInArena(arena, 3, true, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	tail := newLeafNodeInArena(arena, 3, true, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})

	hiddenInner := newParentNodeInArena(arena, 2, false, []*Node{left, op}, nil, 0)
	hiddenOuterChildren, _, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hiddenInner), newStackEntryNode(0, right)}, 0, 2, 2, 1, 0, arena)
	if got, want := len(hiddenOuterChildren), 2; got != want {
		t.Fatalf("len(hiddenOuterChildren) = %d, want %d", got, want)
	}
	if hiddenOuterChildren[0] != hiddenInner || hiddenOuterChildren[1] != right {
		t.Fatalf("hidden outer children = %#v, want compact hidden child then right", hiddenOuterChildren)
	}

	hiddenOuter := newParentNodeInArena(arena, 1, false, hiddenOuterChildren, nil, 0)
	visibleChildren, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{newStackEntryNode(0, hiddenOuter), newStackEntryNode(0, tail)}, 0, 2, 2, 5, 0, arena)
	if fieldIDs != nil {
		t.Fatalf("fieldIDs = %#v, want nil", fieldIDs)
	}
	if got, want := len(visibleChildren), 4; got != want {
		t.Fatalf("len(visibleChildren) = %d, want %d", got, want)
	}
	if visibleChildren[0] != left || visibleChildren[1] != op || visibleChildren[2] != right || visibleChildren[3] != tail {
		t.Fatalf("visible children order = %#v, want fully flattened hidden chain plus tail", visibleChildren)
	}
}
func TestNodesFromGSSFiltersNilAndPreservesOrder(t *testing.T) {
	var scratch gssScratch
	n1 := NewLeafNode(1, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	n2 := NewLeafNode(1, true, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})

	var s gssStack
	s.push(1, nil, &scratch)
	s.push(2, n1, &scratch)
	s.push(3, nil, &scratch)
	s.push(4, n2, &scratch)

	nodes := nodesFromGSS(s)
	if len(nodes) != 2 {
		t.Fatalf("nodesFromGSS len = %d, want 2", len(nodes))
	}
	if nodes[0] != n1 || nodes[1] != n2 {
		t.Fatalf("nodesFromGSS order mismatch: got [%p %p], want [%p %p]", nodes[0], nodes[1], n1, n2)
	}
}

func TestBuildResultFromGLRWithGSSOnlyStack(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	source := []byte("1")
	arena := acquireNodeArena(arenaClassFull)

	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	leaf.parseState = 1
	expr := newParentNodeInArena(arena, 3, true, []*Node{leaf}, nil, 0)
	expr.parseState = 2

	var gScratch gssScratch
	gss := newGSSStack(lang.InitialState, &gScratch)
	gss.push(expr.parseState, expr, &gScratch)
	stack := glrStack{gss: gss}

	tree := parser.buildResultFromGLR([]glrStack{stack}, source, arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	if tree.RootNode() != expr {
		t.Fatal("expected GSS-only stack result to reuse the GSS node as root")
	}
	if got := tree.RootNode().Text(tree.Source()); got != "1" {
		t.Fatalf("root text = %q, want %q", got, "1")
	}
	tree.Release()
}

func TestBuildResultFromGLRExpandsPackedGSSAlternateForFinalChoice(t *testing.T) {
	lang := &Language{
		Name:        "packed-result",
		SymbolNames: []string{"EOF", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "root", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}
	source := []byte("x")
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	inlineErr := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	good := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	head := scratch.allocNode(newStackEntryNode(2, inlineErr), base, 2)
	head.appendExtraLink(gssMainLink{
		prev:  base,
		entry: newStackEntryNode(2, good),
	})
	stack := glrStack{accepted: true, gss: gssStack{head: head}, byteOffset: 1}

	tree := parser.buildResultFromGLR([]glrStack{stack}, source, arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	if tree.RootNode() != good {
		t.Fatalf("root = %p (%s), want packed alternate %p", tree.RootNode(), tree.RootNode().Type(lang), good)
	}
	tree.Release()
}

func TestBuildResultFromGLRExpandsNestedPackedGSSAlternate(t *testing.T) {
	lang := &Language{
		Name:        "packed-result",
		SymbolNames: []string{"EOF", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "root", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}
	source := []byte("x")
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	inlineErr := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	good := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})

	var scratch gssScratch
	base := scratch.allocNode(stackEntry{state: 1}, nil, 1)
	packedBelowHead := scratch.allocNode(newStackEntryNode(2, inlineErr), base, 2)
	packedBelowHead.appendExtraLink(gssMainLink{
		prev:  base,
		entry: newStackEntryNode(2, good),
	})
	commonHead := scratch.allocNode(stackEntry{state: 3}, packedBelowHead, 3)
	stack := glrStack{accepted: true, gss: gssStack{head: commonHead}, byteOffset: 1}

	tree := parser.buildResultFromGLR([]glrStack{stack}, source, arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	if tree.RootNode() != good {
		t.Fatalf("root = %p (%s), want nested packed alternate %p", tree.RootNode(), tree.RootNode().Type(lang), good)
	}
	tree.Release()
}

func TestBuildResultFromNodesUsesErrorRootForMultipleFragments(t *testing.T) {
	lang := &Language{
		SymbolNames:    []string{"number", "expression"},
		SymbolMetadata: []SymbolMetadata{{Visible: true, Named: true}, {Visible: true, Named: true}},
		Name:           "test",
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("12")

	left := newLeafNodeInArena(arena, 0, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 0, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{left, right}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromNodes returned nil tree/root")
	}
	if got := tree.RootNode().Type(lang); got != "ERROR" {
		t.Fatalf("root type = %q, want %q", got, "ERROR")
	}
	if !tree.RootNode().HasError() {
		t.Fatal("expected recovered multi-fragment root to have HasError=true")
	}
	tree.Release()
}

func TestBuildResultFromNodesFlattensLeadingRootFragment(t *testing.T) {
	lang := &Language{
		SymbolNames:    []string{"number", "expression"},
		SymbolMetadata: []SymbolMetadata{{Visible: true, Named: true}, {Visible: true, Named: true}},
		Name:           "test",
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("123")

	left := newLeafNodeInArena(arena, 0, true, 0, 1, Point{}, Point{Column: 1})
	middle := newLeafNodeInArena(arena, 0, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	right := newLeafNodeInArena(arena, 0, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	right.setHasError(true)
	fragment := newParentNodeInArena(arena, 1, true, []*Node{left, middle}, nil, 0)
	fragment.setHasError(true)

	tree := parser.buildResultFromNodes([]*Node{fragment, right}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromNodes returned nil tree/root")
	}
	root := tree.RootNode()
	if got := root.Type(lang); got != "ERROR" {
		t.Fatalf("root type = %q, want %q", got, "ERROR")
	}
	if got, want := root.ChildCount(), 3; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	if first := root.Child(0); first == nil || first == fragment {
		t.Fatalf("expected flattened first child, got %v", first)
	}
	tree.Release()
}

func TestBuildResultFromNodesKeepsExpectedRootForValidMultipleFragments(t *testing.T) {
	lang := &Language{
		SymbolNames:    []string{"number", "expression"},
		SymbolMetadata: []SymbolMetadata{{Visible: true, Named: true}, {Visible: true, Named: true}},
		Name:           "test",
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("12")

	left := newLeafNodeInArena(arena, 0, true, 0, 1, Point{}, Point{Column: 1})
	right := newLeafNodeInArena(arena, 0, true, 1, 2, Point{Column: 1}, Point{Column: 2})

	tree := parser.buildResultFromNodes([]*Node{left, right}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromNodes returned nil tree/root")
	}
	root := tree.RootNode()
	if got := root.Type(lang); got != "expression" {
		t.Fatalf("root type = %q, want %q", got, "expression")
	}
	if root.HasError() {
		t.Fatal("expected valid multi-fragment root to stay error-free")
	}
	tree.Release()
}

func TestBuildResultFromNodesKeepsDartProgramRootWhenOnlyChildNodesHaveErrors(t *testing.T) {
	lang := &Language{
		Name:        "dart",
		SymbolNames: []string{"library_name", "class_definition", "program"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "library_name", Visible: true, Named: true},
			{Name: "class_definition", Visible: true, Named: true},
			{Name: "program", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 2}
	arena := acquireNodeArena(arenaClassFull)
	source := []byte("library;\nclass A {}\n")

	library := newLeafNodeInArena(arena, 0, true, 0, 8, Point{}, Point{Column: 8})
	library.setHasError(true)
	classDef := newLeafNodeInArena(arena, 1, true, 9, 19, Point{Row: 1}, Point{Row: 1, Column: 10})

	tree := parser.buildResultFromNodes([]*Node{library, classDef}, source, arena, nil, nil, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromNodes returned nil tree/root")
	}
	root := tree.RootNode()
	if got := root.Type(lang); got != "program" {
		t.Fatalf("root type = %q, want %q", got, "program")
	}
	if !root.HasError() {
		t.Fatal("expected program root to retain HasError=true when a child has error")
	}
	tree.Release()
}

func TestCompactAcceptedStacksPreservesAllAcceptedForFinalChoice(t *testing.T) {
	lang := buildAmbiguousLanguage()
	parser := NewParser(lang)
	source := []byte("x")
	arena := acquireNodeArena(arenaClassFull)

	low := newLeafNodeInArena(arena, 2, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	low.parseState = 2
	high := newLeafNodeInArena(arena, 3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	high.parseState = 2

	stacks := []glrStack{
		{accepted: false, score: 99, entries: []stackEntry{{state: 1}}},
		{accepted: true, score: 0, entries: []stackEntry{newStackEntryNode(2, low)}},
		{accepted: true, score: 5, entries: []stackEntry{newStackEntryNode(2, high)}},
	}

	accepted := compactAcceptedStacks(stacks)
	if got, want := len(accepted), 2; got != want {
		t.Fatalf("len(accepted) = %d, want %d", got, want)
	}
	if !accepted[0].accepted || !accepted[1].accepted {
		t.Fatal("expected only accepted stacks after compaction")
	}
	if accepted[0].score != 0 || accepted[1].score != 5 {
		t.Fatalf("accepted scores = [%d %d], want [0 5]", accepted[0].score, accepted[1].score)
	}

	tree := parser.buildResultFromGLR(accepted, source, arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	if got, want := tree.RootNode().Symbol(), Symbol(3); got != want {
		t.Fatalf("root symbol = %d, want %d", got, want)
	}
	tree.Release()
}

func TestBuildResultFromGLRPrefersAliasTargetTreeOnFinalTie(t *testing.T) {
	lang := &Language{
		SymbolCount: 4,
		TokenCount:  1,
		SymbolNames: []string{"EOF", "identifier", "type_identifier", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
		},
		AliasSequences: [][]Symbol{
			{0, 2},
		},
	}
	parser := &Parser{
		language:          lang,
		aliasTargetSymbol: buildAliasTargetSymbols(lang),
	}
	source := []byte("sudog")
	arena := acquireNodeArena(arenaClassFull)

	plainLeaf := newLeafNodeInArena(arena, 1, true, 0, 5, Point{}, Point{Column: 5})
	aliasLeaf := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})
	plainRoot := newParentNodeInArena(arena, 3, true, []*Node{plainLeaf}, nil, 0)
	aliasRoot := newParentNodeInArena(arena, 3, true, []*Node{aliasLeaf}, nil, 0)

	stacks := []glrStack{
		{
			accepted:    true,
			byteOffset:  5,
			score:       0,
			branchOrder: 0,
			entries:     []stackEntry{newStackEntryNode(1, plainRoot)},
		},
		{
			accepted:    true,
			byteOffset:  5,
			score:       0,
			branchOrder: 1,
			entries:     []stackEntry{newStackEntryNode(1, aliasRoot)},
		},
	}

	tree := parser.buildResultFromGLR(stacks, source, arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	root := tree.RootNode()
	if got, want := root.Type(lang), "root"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if got, want := root.Child(0).Type(lang), "type_identifier"; got != want {
		t.Fatalf("child type = %q, want %q", got, want)
	}
	tree.Release()
}

func TestBuildResultFromGLRScoreBeatsAliasTargetPreference(t *testing.T) {
	lang := &Language{
		SymbolCount: 4,
		TokenCount:  1,
		SymbolNames: []string{"EOF", "identifier", "type_identifier", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "type_identifier", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
		},
		AliasSequences: [][]Symbol{
			{0, 2},
		},
	}
	parser := &Parser{
		language:          lang,
		aliasTargetSymbol: buildAliasTargetSymbols(lang),
	}
	source := []byte("sudog")
	arena := acquireNodeArena(arenaClassFull)

	plainLeaf := newLeafNodeInArena(arena, 1, true, 0, 5, Point{}, Point{Column: 5})
	aliasLeaf := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})
	plainRoot := newParentNodeInArena(arena, 3, true, []*Node{plainLeaf}, nil, 0)
	aliasRoot := newParentNodeInArena(arena, 3, true, []*Node{aliasLeaf}, nil, 0)

	stacks := []glrStack{
		{
			accepted:    true,
			byteOffset:  5,
			score:       1,
			branchOrder: 0,
			entries:     []stackEntry{newStackEntryNode(1, plainRoot)},
		},
		{
			accepted:    true,
			byteOffset:  5,
			score:       0,
			branchOrder: 1,
			entries:     []stackEntry{newStackEntryNode(1, aliasRoot)},
		},
	}

	tree := parser.buildResultFromGLR(stacks, source, arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	root := tree.RootNode()
	if got, want := root.Type(lang), "root"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if got, want := root.Child(0).Type(lang), "identifier"; got != want {
		t.Fatalf("child type = %q, want %q", got, want)
	}
	tree.Release()
}

func TestBuildResultFromGLRDoesNotSpliceVisibleNamedPrefixWrapper(t *testing.T) {
	lang := &Language{
		SymbolCount: 6,
		TokenCount:  1,
		SymbolNames: []string{"EOF", "identifier", "selector", "operator", "semantic_group", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "selector", Visible: true, Named: true},
			{Name: "operator", Visible: true, Named: false},
			{Name: "semantic_group", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
		},
	}
	parser := NewParser(lang)
	source := []byte("receiver.call")
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	groupID := newLeafNodeInArena(arena, 1, true, 0, 8, Point{}, Point{Column: 8})
	groupQualifier := newLeafNodeInArena(arena, 2, true, 8, 10, Point{Column: 8}, Point{Column: 10})
	groupOp := newLeafNodeInArena(arena, 3, false, 10, 11, Point{Column: 10}, Point{Column: 11})
	groupArg := newLeafNodeInArena(arena, 1, true, 11, 13, Point{Column: 11}, Point{Column: 13})
	group := newParentNodeInArena(arena, 4, true, []*Node{groupID, groupQualifier, groupOp, groupArg}, nil, 0)
	groupRoot := newParentNodeInArena(arena, 5, true, []*Node{group}, nil, 0)

	flatID := newLeafNodeInArena(arena, 1, true, 0, 8, Point{}, Point{Column: 8})
	flatQualifier := newLeafNodeInArena(arena, 2, true, 8, 10, Point{Column: 8}, Point{Column: 10})
	flatCall := newLeafNodeInArena(arena, 2, true, 10, 13, Point{Column: 10}, Point{Column: 13})
	flatRoot := newParentNodeInArena(arena, 5, true, []*Node{flatID, flatQualifier, flatCall}, nil, 0)

	stacks := []glrStack{
		{
			accepted:    true,
			byteOffset:  13,
			branchOrder: 0,
			entries:     []stackEntry{newStackEntryNode(1, groupRoot)},
		},
		{
			accepted:    true,
			byteOffset:  13,
			branchOrder: 1,
			entries:     []stackEntry{newStackEntryNode(1, flatRoot)},
		},
	}

	tree := parser.buildResultFromGLR(stacks, source, arena, nil, nil, nil, nil, nil, false, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("buildResultFromGLR returned nil tree/root")
	}
	if tree.RootNode() != groupRoot {
		t.Fatalf("root = %p, want visible-wrapper-preserving winner %p", tree.RootNode(), groupRoot)
	}
	tree.Release()
}

func TestFieldIDsAlignAfterExtrasFold(t *testing.T) {
	lang := queryTestLanguage()

	// Construct a parent with fielded children:
	//   children:  [ident,        paramList,       block]
	//   fieldIDs:  [name(1),      parameters(5),   body(2)]
	ident := NewLeafNode(Symbol(1), true, 5, 9, Point{}, Point{})
	paramList := NewLeafNode(Symbol(13), true, 9, 11, Point{}, Point{})
	block := NewLeafNode(Symbol(14), true, 12, 20, Point{}, Point{})
	root := NewParentNode(Symbol(5), true,
		[]*Node{ident, paramList, block},
		[]FieldID{1, 5, 2}, 0)

	// Sanity: field lookups work before modification.
	if got := root.ChildByFieldName("name", lang); got != ident {
		t.Fatal("pre-check: name field should return ident")
	}
	if got := root.ChildByFieldName("body", lang); got != block {
		t.Fatal("pre-check: body field should return block")
	}

	// Simulate what buildResult's extras fold does: prepend a leading extra.
	extra := NewLeafNode(Symbol(0), false, 0, 3, Point{}, Point{})
	extra.setExtra(true)

	leadingCount := 1
	merged := make([]*Node, 0, 1+len(root.children))
	merged = append(merged, extra)
	merged = append(merged, root.children...)
	root.children = merged

	// Pad fieldIDs to match: extras get 0.
	if len(root.fieldIDs()) > 0 {
		padded := make([]FieldID, leadingCount+len(root.fieldIDs()))
		copy(padded[leadingCount:], root.fieldIDs())
		root.setFieldIDs(padded)
	}

	// Verify field lookups still return correct nodes.
	if got := root.ChildByFieldName("name", lang); got != ident {
		t.Fatalf("after fold: name field should return ident (sym 1), got sym %d", got.Symbol())
	}
	if got := root.ChildByFieldName("body", lang); got != block {
		t.Fatalf("after fold: body field should return block (sym 14), got sym %d", got.Symbol())
	}
	if got := root.ChildByFieldName("parameters", lang); got != paramList {
		t.Fatalf("after fold: parameters field should return paramList (sym 13), got sym %d", got.Symbol())
	}
}

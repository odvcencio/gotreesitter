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
	leadingExtra.isExtra = true

	valueChild := NewLeafNode(1, true, 1, 3, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 3})
	realRoot := NewParentNode(3, true, []*Node{valueChild}, []FieldID{1}, 0)

	trailingExtra := NewLeafNode(2, false, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	trailingExtra.isExtra = true

	stack := []stackEntry{
		{state: 0, node: leadingExtra},
		{state: 0, node: realRoot},
		{state: 0, node: trailingExtra},
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
	if len(root.fieldIDs) != 3 || root.fieldIDs[1] != 1 {
		t.Fatalf("fieldIDs not re-aligned after folding extras: %#v", root.fieldIDs)
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

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 0, 0, arena)
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

func TestBuildReduceChildrenInheritedFieldOverridesInheritedInnerFieldOnFlattenedSpan(t *testing.T) {
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
	hidden.fieldSources = []uint8{fieldSourceInherited, fieldSourceInherited, fieldSourceInherited}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	for i, fid := range fieldIDs {
		if got, want := fid, FieldID(1); got != want {
			t.Fatalf("fieldIDs[%d] = %d, want %d", i, got, want)
		}
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
	hidden.fieldSources = []uint8{fieldSourceInherited}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
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

func TestBuildReduceChildrenInheritedFieldDoesNotBlanketSpanWithoutConflict(t *testing.T) {
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

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 5, 0, arena)
	if got, want := len(children), 3; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := len(fieldIDs), 3; got != want {
		t.Fatalf("len(fieldIDs) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
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

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
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
	hidden.fieldSources = []uint8{fieldSourceNone, fieldSourceDirect, fieldSourceNone, fieldSourceDirect}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
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
	tail.fieldSources = []uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect}
	outer := newParentNodeInArena(arena, 1, false, []*Node{java, dot0, tail}, []FieldID{1, 0, 1}, 0)
	outer.fieldSources = []uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: outer}}, 0, 1, 1, 4, 0, arena)
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
	hidden.fieldSources = []uint8{fieldSourceInherited, fieldSourceNone, fieldSourceDirect, fieldSourceNone, fieldSourceDirect}

	children := arena.allocNodeSlice(5)
	fieldIDs := arena.allocFieldIDSlice(5)
	fieldSources := make([]uint8, 5)
	if got, want := appendFlattenedHiddenChildrenWithFields(children, fieldIDs, fieldSources, 0, hidden, lang.SymbolMetadata), 5; got != want {
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
	comment.isExtra = true
	binding := newLeafNodeInArena(arena, 3, true, 10, 16, Point{Row: 0, Column: 10}, Point{Row: 0, Column: 16})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{comment, binding}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
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
	hidden.fieldSources = []uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect}

	children := arena.allocNodeSlice(3)
	fieldIDs := arena.allocFieldIDSlice(3)
	fieldSources := make([]uint8, 3)
	if got, want := appendFlattenedHiddenChildrenWithFields(children, fieldIDs, fieldSources, 0, hidden, lang.SymbolMetadata), 3; got != want {
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

	_, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 5, 0, arena)
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

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 3, 0, arena)
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
	hidden.fieldSources = []uint8{fieldSourceNone, fieldSourceDirect, fieldSourceDirect, fieldSourceDirect}

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 6, 0, arena)
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
	access.fieldSources = []uint8{fieldSourceDirect, fieldSourceNone, fieldSourceDirect}
	hidden := newParentNodeInArena(arena, 1, false, []*Node{joinTok, ident, access}, nil, 0)

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 6, 0, arena)
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

func TestBuildReduceChildrenInheritedFieldProjectsSingleHiddenChildWhenDescendantHasDirectField(t *testing.T) {
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
	call.fieldSources = []uint8{fieldSourceDirect, fieldSourceNone}
	outerArgs := newLeafNodeInArena(arena, 4, true, 10, 13, Point{Row: 0, Column: 10}, Point{Row: 0, Column: 13})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{call, outerArgs}, nil, 0)

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 5, 0, arena)
	if got, want := len(children), 2; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 0), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[0] = %d, want %d", got, want)
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

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 3, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got := fieldIDs[0]; got != 0 {
		t.Fatalf("fieldIDs[0] = %d, want 0", got)
	}
}

func TestBuildReduceChildrenInheritedFieldProjectsSingleNonLeafHiddenChildWithoutDirectField(t *testing.T) {
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

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 4, 0, arena)
	if got, want := len(children), 1; got != want {
		t.Fatalf("len(children) = %d, want %d", got, want)
	}
	if got, want := fieldIDs[0], FieldID(1); got != want {
		t.Fatalf("fieldIDs[0] = %d, want %d", got, want)
	}
	if got, want := fieldSourceAt(fieldSources, 0), uint8(fieldSourceInherited); got != want {
		t.Fatalf("fieldSources[0] = %d, want %d", got, want)
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
	inner.fieldSources = []uint8{fieldSourceDirect}
	outer := newParentNodeInArena(arena, 2, false, []*Node{inner}, nil, 0)

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{{node: outer}}, 0, 1, 1, 4, 0, arena)
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
	fieldIDs, inherited := parser.buildFieldIDs(1, 0, arena)
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
	decl.fieldSources = []uint8{fieldSourceDirect, fieldSourceDirect, fieldSourceDirect}

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{{node: decl}}, 0, 1, 1, 5, 0, arena)
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

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{{node: constructorParam}}, 0, 1, 1, 1, 0, arena)
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
	hidden.fieldSources = []uint8{fieldSourceDirect}

	children, fieldIDs, fieldSources := parser.buildReduceChildren([]stackEntry{{node: hidden}}, 0, 1, 1, 1, 0, arena)
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

	children, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hidden}, {node: right}}, 0, 2, 2, 2, 0, arena)
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
	hiddenOuterChildren, _, _ := parser.buildReduceChildren([]stackEntry{{node: hiddenInner}, {node: right}}, 0, 2, 2, 1, 0, arena)
	if got, want := len(hiddenOuterChildren), 2; got != want {
		t.Fatalf("len(hiddenOuterChildren) = %d, want %d", got, want)
	}
	if hiddenOuterChildren[0] != hiddenInner || hiddenOuterChildren[1] != right {
		t.Fatalf("hidden outer children = %#v, want compact hidden child then right", hiddenOuterChildren)
	}

	hiddenOuter := newParentNodeInArena(arena, 1, false, hiddenOuterChildren, nil, 0)
	visibleChildren, fieldIDs, _ := parser.buildReduceChildren([]stackEntry{{node: hiddenOuter}, {node: tail}}, 0, 2, 2, 5, 0, arena)
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

	tree := parser.buildResultFromGLR([]glrStack{stack}, source, arena, nil, nil, nil)
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
	right.hasError = true

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
	right.hasError = true
	fragment := newParentNodeInArena(arena, 1, true, []*Node{left, middle}, nil, 0)
	fragment.hasError = true

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
	library.hasError = true
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
		{accepted: true, score: 0, entries: []stackEntry{{state: 2, node: low}}},
		{accepted: true, score: 5, entries: []stackEntry{{state: 2, node: high}}},
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

	tree := parser.buildResultFromGLR(accepted, source, arena, nil, nil, nil)
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
			entries:     []stackEntry{{state: 1, node: plainRoot}},
		},
		{
			accepted:    true,
			byteOffset:  5,
			score:       -1,
			branchOrder: 1,
			entries:     []stackEntry{{state: 1, node: aliasRoot}},
		},
	}

	tree := parser.buildResultFromGLR(stacks, source, arena, nil, nil, nil)
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
	extra.isExtra = true

	leadingCount := 1
	merged := make([]*Node, 0, 1+len(root.children))
	merged = append(merged, extra)
	merged = append(merged, root.children...)
	root.children = merged

	// Pad fieldIDs to match: extras get 0.
	if len(root.fieldIDs) > 0 {
		padded := make([]FieldID, leadingCount+len(root.fieldIDs))
		copy(padded[leadingCount:], root.fieldIDs)
		root.fieldIDs = padded
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

package gotreesitter

import (
	"strings"
	"testing"
	"unsafe"
)

// testLanguage returns a minimal Language for use in tree tests.
func testLanguage() *Language {
	return &Language{
		Name:        "test",
		SymbolNames: []string{"", "identifier", "number", "expression", "program", "ERROR"},
		FieldNames:  []string{"", "left", "right", "operator"},
		FieldCount:  3,
	}
}

func TestNodeLayoutSizeBudget(t *testing.T) {
	var n Node
	got := unsafe.Sizeof(n)
	t.Logf(
		"Node size=%d align=%d children=%d parent=%d ownerArena=%d startPoint=%d startByte=%d parseState=%d childIndex=%d symbol=%d flags=%d dirtyFlag=%d",
		got,
		unsafe.Alignof(n),
		unsafe.Offsetof(n.children),
		unsafe.Offsetof(n.parent),
		unsafe.Offsetof(n.ownerArena),
		unsafe.Offsetof(n.startPoint),
		unsafe.Offsetof(n.startByte),
		unsafe.Offsetof(n.parseState),
		unsafe.Offsetof(n.childIndex),
		unsafe.Offsetof(n.symbol),
		unsafe.Offsetof(n.flags),
		unsafe.Offsetof(n.dirtyFlag),
	)
	const budget = 136
	if got > budget {
		t.Fatalf("Node size = %d, want <= %d", got, budget)
	}
}

func TestLeafNode(t *testing.T) {
	lang := testLanguage()

	n := NewLeafNode(
		Symbol(1), // identifier
		true,      // named
		5, 10,
		Point{Row: 0, Column: 5},
		Point{Row: 0, Column: 10},
	)

	if n.Symbol() != Symbol(1) {
		t.Errorf("Symbol: got %d, want 1", n.Symbol())
	}
	if got := n.Type(lang); got != "identifier" {
		t.Errorf("Type: got %q, want %q", got, "identifier")
	}
	if !n.IsNamed() {
		t.Error("IsNamed: got false, want true")
	}
	if n.IsMissing() {
		t.Error("IsMissing: got true, want false")
	}
	if n.HasError() {
		t.Error("HasError: got true, want false")
	}
	if n.IsExtra() {
		t.Error("IsExtra: got true, want false")
	}
	if n.IsError() {
		t.Error("IsError: got true, want false")
	}
	if n.HasChanges() {
		t.Error("HasChanges: got true, want false")
	}
	if n.StartByte() != 5 {
		t.Errorf("StartByte: got %d, want 5", n.StartByte())
	}
	if n.EndByte() != 10 {
		t.Errorf("EndByte: got %d, want 10", n.EndByte())
	}
	if n.StartPoint() != (Point{Row: 0, Column: 5}) {
		t.Errorf("StartPoint: got %v, want {0,5}", n.StartPoint())
	}
	if n.EndPoint() != (Point{Row: 0, Column: 10}) {
		t.Errorf("EndPoint: got %v, want {0,10}", n.EndPoint())
	}
	if n.ChildCount() != 0 {
		t.Errorf("ChildCount: got %d, want 0", n.ChildCount())
	}
	if n.Parent() != nil {
		t.Error("Parent: got non-nil, want nil")
	}

	r := n.Range()
	if r.StartByte != 5 || r.EndByte != 10 {
		t.Errorf("Range bytes: got %d-%d, want 5-10", r.StartByte, r.EndByte)
	}
	if r.StartPoint != (Point{Row: 0, Column: 5}) || r.EndPoint != (Point{Row: 0, Column: 10}) {
		t.Errorf("Range points: got %v-%v", r.StartPoint, r.EndPoint)
	}
}

func TestNodeFlagAccessors(t *testing.T) {
	n := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	n.setExtra(true)
	n.setDirty(true)
	if !n.IsExtra() {
		t.Fatal("IsExtra should be true")
	}
	if !n.HasChanges() {
		t.Fatal("HasChanges should be true")
	}

	errNode := NewLeafNode(errorSymbol, false, 0, 1, Point{}, Point{Row: 0, Column: 1})
	if !errNode.IsError() {
		t.Fatal("IsError should be true for errorSymbol node")
	}
}

func TestLeafNodeTypeOutOfRange(t *testing.T) {
	lang := testLanguage()
	n := NewLeafNode(Symbol(999), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	if got := n.Type(lang); got != "" {
		t.Errorf("Type out of range: got %q, want empty", got)
	}
}

func TestLeafNodeTypeUnescapesPunctuationSymbols(t *testing.T) {
	lang := &Language{
		Name:        "test",
		SymbolNames: []string{"", "\\?", "\\?.", "\\?:", "identifier", "defined\\?", "\\u2200", "$\\?", "\\\\"},
	}

	tests := []struct {
		sym  Symbol
		want string
	}{
		{sym: 1, want: "?"},
		{sym: 2, want: "?."},
		{sym: 3, want: "?:"},
		{sym: 4, want: "identifier"},
		{sym: 5, want: "defined?"},
		{sym: 6, want: "\\u2200"},
		{sym: 7, want: "$?"},
		{sym: 8, want: "\\\\"},
	}

	for _, tc := range tests {
		n := NewLeafNode(tc.sym, true, 0, 1, Point{}, Point{Row: 0, Column: 1})
		if got := n.Type(lang); got != tc.want {
			t.Fatalf("Type(%d) = %q, want %q", tc.sym, got, tc.want)
		}
	}
}

func TestParentNode(t *testing.T) {
	child0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	child1 := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})

	parent := NewParentNode(
		Symbol(3), true,
		[]*Node{child0, child1},
		[]FieldID{FieldID(1), FieldID(2)}, // left, right
		42,
	)

	if parent.ChildCount() != 2 {
		t.Errorf("ChildCount: got %d, want 2", parent.ChildCount())
	}
	if parent.Child(0) != child0 {
		t.Error("Child(0): not the expected child")
	}
	if parent.Child(1) != child1 {
		t.Error("Child(1): not the expected child")
	}

	// Parent pointers set.
	if child0.Parent() != parent {
		t.Error("child0.Parent: not set to parent")
	}
	if child1.Parent() != parent {
		t.Error("child1.Parent: not set to parent")
	}
	if child0.childIndex != 0 {
		t.Errorf("child0.childIndex = %d, want 0", child0.childIndex)
	}
	if child1.childIndex != 1 {
		t.Errorf("child1.childIndex = %d, want 1", child1.childIndex)
	}

	// Span computed from children.
	if parent.StartByte() != 0 {
		t.Errorf("Parent StartByte: got %d, want 0", parent.StartByte())
	}
	if parent.EndByte() != 7 {
		t.Errorf("Parent EndByte: got %d, want 7", parent.EndByte())
	}
	if parent.StartPoint() != (Point{Row: 0, Column: 0}) {
		t.Errorf("Parent StartPoint: got %v, want {0,0}", parent.StartPoint())
	}
	if parent.EndPoint() != (Point{Row: 0, Column: 7}) {
		t.Errorf("Parent EndPoint: got %v, want {0,7}", parent.EndPoint())
	}

	// Children slice.
	kids := parent.Children()
	if len(kids) != 2 {
		t.Errorf("Children len: got %d, want 2", len(kids))
	}
}

func TestParentNodeEmptyChildren(t *testing.T) {
	parent := NewParentNode(Symbol(3), true, nil, nil, 0)
	if parent.StartByte() != 0 || parent.EndByte() != 0 {
		t.Errorf("Empty parent bytes: got %d-%d, want 0-0", parent.StartByte(), parent.EndByte())
	}
	if parent.ChildCount() != 0 {
		t.Errorf("Empty parent ChildCount: got %d, want 0", parent.ChildCount())
	}
}

func TestNamedChild(t *testing.T) {
	named0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	unnamed := NewLeafNode(Symbol(2), false, 3, 4, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 4})
	named1 := NewLeafNode(Symbol(1), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})

	parent := NewParentNode(
		Symbol(3), true,
		[]*Node{named0, unnamed, named1},
		[]FieldID{0, 0, 0},
		0,
	)

	if parent.NamedChildCount() != 2 {
		t.Errorf("NamedChildCount: got %d, want 2", parent.NamedChildCount())
	}
	if parent.NamedChild(0) != named0 {
		t.Error("NamedChild(0): not the expected node")
	}
	if parent.NamedChild(1) != named1 {
		t.Error("NamedChild(1): not the expected node")
	}
}

func TestSiblingNavigation(t *testing.T) {
	first := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	second := NewLeafNode(Symbol(2), true, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})
	third := NewLeafNode(Symbol(1), true, 4, 5, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 5})
	parent := NewParentNode(Symbol(3), true, []*Node{first, second, third}, nil, 0)

	if first.PrevSibling() != nil {
		t.Fatal("first.PrevSibling should be nil")
	}
	if first.NextSibling() != second {
		t.Fatal("first.NextSibling should return second")
	}
	if second.PrevSibling() != first {
		t.Fatal("second.PrevSibling should return first")
	}
	if second.NextSibling() != third {
		t.Fatal("second.NextSibling should return third")
	}
	if third.NextSibling() != nil {
		t.Fatal("third.NextSibling should be nil")
	}
	if third.PrevSibling() != second {
		t.Fatal("third.PrevSibling should return second")
	}

	leafWithoutParent := NewLeafNode(Symbol(1), true, 6, 7, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 7})
	if leafWithoutParent.NextSibling() != nil {
		t.Fatal("leaf without parent should have nil NextSibling")
	}
	if leafWithoutParent.PrevSibling() != nil {
		t.Fatal("leaf without parent should have nil PrevSibling")
	}

	if parent.NextSibling() != nil {
		t.Fatal("root parent node should have nil NextSibling")
	}
}

func TestDeferredParentLinksWireOnAccess(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	first := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	second := newLeafNodeInArena(arena, Symbol(2), true, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})
	children := arena.allocNodeSliceNoClear(2)
	children[0] = first
	children[1] = second
	parent := newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(3), true, children, nil, nil, 0, false)
	arena.deferParentLinks(parent)

	if first.parent != nil || second.parent != nil {
		t.Fatal("expected deferred links to leave children unwired before access")
	}
	if got := first.Parent(); got != parent {
		t.Fatalf("first.Parent() = %p, want %p", got, parent)
	}
	// First parent access now wires ALL deferred links once, under parentLinkMu
	// (so the sibling is wired too). This replaced the previous per-path lazy
	// wiring, which wrote parent pointers outside the lock and raced with
	// concurrent Parent()/sibling reads on a freshly parsed java/python/ts/tsx
	// tree (issue #93 parser-core sweep). The defer-until-first-access property
	// is preserved (links stay unwired until the first access above).
	if second.parent != parent {
		t.Fatalf("expected first access to wire all links once; second.parent = %p, want %p", second.parent, parent)
	}
	if got := first.NextSibling(); got != second {
		t.Fatalf("first.NextSibling() = %p, want %p", got, second)
	}
	if got := second.PrevSibling(); got != first {
		t.Fatalf("second.PrevSibling() = %p, want %p", got, first)
	}
	if parent.Parent() != nil {
		t.Fatal("root parent should remain nil after deferred wiring")
	}
}

func TestFinalizeResultRootDefersSelectedLanguageParentLinks(t *testing.T) {
	for _, name := range []string{"java", "python", "typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()

			child := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
			children := arena.allocNodeSliceNoClear(1)
			children[0] = child
			root := newParentNodeInArenaNoLinksWithFieldSources(arena, Symbol(2), true, children, nil, nil, 0, false)
			parser := NewParser(&Language{Name: name})

			parser.finalizeResultRoot(root, []byte("x"), nil, true, false)

			if !arena.parentLinksDeferred.Load() {
				t.Fatalf("expected %s finalization to defer parent links", name)
			}
			if child.parent != nil {
				t.Fatal("expected child parent link to stay unwired until access")
			}
			if got := child.Parent(); got != root {
				t.Fatalf("child.Parent() = %p, want %p", got, root)
			}
			// First parent access wires ALL deferred links once (under
			// parentLinkMu) and clears the flag — this replaced the previous
			// lazy per-path wiring, which wrote parent pointers outside the lock
			// and raced with concurrent Parent()/sibling reads (issue #93 sweep).
			if arena.parentLinksDeferred.Load() {
				t.Fatal("expected first parent access to wire all links once and clear the deferred flag")
			}
			if child.parent != root {
				t.Fatalf("expected child parent link wired to root after access; got %p", child.parent)
			}
		})
	}
}

func TestChildByFieldName(t *testing.T) {
	lang := testLanguage()

	leftChild := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	opChild := NewLeafNode(Symbol(2), false, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2})
	rightChild := NewLeafNode(Symbol(1), true, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})

	parent := NewParentNode(
		Symbol(3), true,
		[]*Node{leftChild, opChild, rightChild},
		[]FieldID{FieldID(1), FieldID(3), FieldID(2)}, // left, operator, right
		0,
	)

	if got := parent.ChildByFieldName("left", lang); got != leftChild {
		t.Error("ChildByFieldName(left): wrong node")
	}
	if got := parent.ChildByFieldName("right", lang); got != rightChild {
		t.Error("ChildByFieldName(right): wrong node")
	}
	if got := parent.ChildByFieldName("operator", lang); got != opChild {
		t.Error("ChildByFieldName(operator): wrong node")
	}
	if got := parent.ChildByFieldName("nonexistent", lang); got != nil {
		t.Error("ChildByFieldName(nonexistent): expected nil")
	}
}

func TestText(t *testing.T) {
	source := []byte("hello world")
	n := NewLeafNode(Symbol(1), true, 6, 11, Point{Row: 0, Column: 6}, Point{Row: 0, Column: 11})

	if got := n.Text(source); got != "world" {
		t.Errorf("Text: got %q, want %q", got, "world")
	}
}

func TestTextReturnsEmptyForNilNode(t *testing.T) {
	var n *Node
	if got := n.Text([]byte("hello")); got != "" {
		t.Fatalf("Text(nil node) = %q, want empty", got)
	}
}

func TestTextReturnsEmptyForOutOfBoundsRange(t *testing.T) {
	tests := []struct {
		name   string
		node   *Node
		source []byte
	}{
		{
			name:   "end beyond source",
			node:   NewLeafNode(Symbol(1), true, 2, 8, Point{}, Point{}),
			source: []byte("hello"),
		},
		{
			name:   "start beyond source",
			node:   NewLeafNode(Symbol(1), true, 8, 8, Point{}, Point{}),
			source: []byte("hello"),
		},
		{
			name:   "end before start",
			node:   NewLeafNode(Symbol(1), true, 4, 2, Point{}, Point{}),
			source: []byte("hello"),
		},
		{
			name:   "nil source with non-empty range",
			node:   NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{}),
			source: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.node.Text(tc.source); got != "" {
				t.Fatalf("Text() = %q, want empty", got)
			}
		})
	}
}

func TestTreeReleaseClearsRoot(t *testing.T) {
	root := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	tree := NewTree(root, []byte("x"), testLanguage())
	if tree.RootNode() == nil {
		t.Fatal("precondition: root should be non-nil")
	}
	tree.Release()
	if tree.RootNode() != nil {
		t.Fatal("root should be nil after Release")
	}
	// Release remains idempotent.
	tree.Release()
}

func TestNodeSExpr(t *testing.T) {
	lang := testLanguage()
	left := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	op := NewLeafNode(Symbol(2), false, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2})
	right := NewLeafNode(Symbol(1), true, 2, 3, Point{Row: 0, Column: 2}, Point{Row: 0, Column: 3})
	root := NewParentNode(Symbol(3), true, []*Node{left, op, right}, nil, 0)

	if got, want := root.SExpr(lang), "(expression (identifier) (identifier))"; got != want {
		t.Fatalf("SExpr: got %q, want %q", got, want)
	}
}

func TestTree(t *testing.T) {
	lang := testLanguage()
	source := []byte("x + y")

	leaf := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	root := NewParentNode(Symbol(4), true, []*Node{leaf}, nil, 0)

	tree := NewTree(root, source, lang)

	if tree.RootNode() != root {
		t.Error("RootNode: wrong")
	}
	if string(tree.Source()) != "x + y" {
		t.Errorf("Source: got %q", tree.Source())
	}
	if tree.Language() != lang {
		t.Error("Language: wrong")
	}
}

func TestReusableTreeEditScratch(t *testing.T) {
	small := reusableTreeEditScratch(make([]InputEdit, 1, maxRetainedTreeEditCap))
	if len(small) != 0 || cap(small) != maxRetainedTreeEditCap {
		t.Fatalf("small edit scratch len/cap = %d/%d, want 0/%d", len(small), cap(small), maxRetainedTreeEditCap)
	}
	if large := reusableTreeEditScratch(make([]InputEdit, 1, maxRetainedTreeEditCap+1)); large != nil {
		t.Fatalf("large edit scratch retained with cap %d, want nil", cap(large))
	}
	if none := reusableTreeEditScratch(nil); none != nil {
		t.Fatalf("nil edit scratch retained as %v, want nil", none)
	}
}

func TestTreeCopyIndependentNodes(t *testing.T) {
	lang := testLanguage()
	left := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	right := NewLeafNode(Symbol(2), true, 3, 6, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 6})
	root := NewParentNode(Symbol(3), true, []*Node{left, right}, nil, 0)
	tree := NewTree(root, []byte("abcdef"), lang)
	tree.Edit(InputEdit{
		StartByte:   3,
		OldEndByte:  4,
		NewEndByte:  5,
		StartPoint:  Point{Row: 0, Column: 3},
		OldEndPoint: Point{Row: 0, Column: 4},
		NewEndPoint: Point{Row: 0, Column: 5},
	})

	cp := tree.Copy()
	if cp == nil {
		t.Fatal("Copy() returned nil")
	}
	if cp == tree {
		t.Fatal("Copy() returned same tree pointer")
	}
	if cp.RootNode() == tree.RootNode() {
		t.Fatal("copy root should be a distinct node pointer")
	}
	if cp.RootNode().Child(0) == tree.RootNode().Child(0) {
		t.Fatal("copy child should be a distinct node pointer")
	}
	if cp.Language() != tree.Language() {
		t.Fatal("copy language mismatch")
	}
	if got, want := len(cp.Edits()), len(tree.Edits()); got != want {
		t.Fatalf("copy edits len: got %d want %d", got, want)
	}

	// Mutating the copy must not mutate the original.
	beforeOrigEnd := tree.RootNode().EndByte()
	cp.Edit(InputEdit{
		StartByte:   0,
		OldEndByte:  0,
		NewEndByte:  2,
		StartPoint:  Point{Row: 0, Column: 0},
		OldEndPoint: Point{Row: 0, Column: 0},
		NewEndPoint: Point{Row: 0, Column: 2},
	})
	if tree.RootNode().EndByte() != beforeOrigEnd {
		t.Fatalf("original tree root mutated by copy edit: got %d want %d", tree.RootNode().EndByte(), beforeOrigEnd)
	}
}

func TestTreeCopySurvivesOriginalRelease(t *testing.T) {
	lang := testLanguage()
	root := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	tree := NewTree(root, []byte("x"), lang)
	cp := tree.Copy()
	if cp == nil || cp.RootNode() == nil {
		t.Fatal("copy/root should be non-nil")
	}

	tree.Release()
	if cp.RootNode() == nil {
		t.Fatal("copy root should remain valid after releasing original")
	}
	if got, want := cp.RootNode().Text(cp.Source()), "x"; got != want {
		t.Fatalf("copy text: got %q want %q", got, want)
	}
}

func TestTreeRootNodeWithOffsetShiftsDescendants(t *testing.T) {
	lang := testLanguage()
	a := NewLeafNode(Symbol(1), true, 1, 3, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 3})
	b := NewLeafNode(Symbol(2), true, 4, 8, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 4})
	root := NewParentNode(Symbol(3), true, []*Node{a, b}, nil, 0)
	tree := NewTree(root, []byte("abcdefgh"), lang)

	offset := tree.RootNodeWithOffset(10, Point{Row: 3, Column: 7})
	if offset == nil {
		t.Fatal("RootNodeWithOffset returned nil")
	}
	if offset == tree.RootNode() {
		t.Fatal("RootNodeWithOffset should return a distinct node for non-zero offset")
	}

	if got, want := offset.StartByte(), uint32(11); got != want {
		t.Fatalf("root start byte: got %d want %d", got, want)
	}
	if got, want := offset.EndByte(), uint32(18); got != want {
		t.Fatalf("root end byte: got %d want %d", got, want)
	}
	if got, want := offset.StartPoint(), (Point{Row: 3, Column: 8}); got != want {
		t.Fatalf("root start point: got %+v want %+v", got, want)
	}

	first := offset.Child(0)
	if first == nil {
		t.Fatal("offset first child nil")
	}
	if got, want := first.StartPoint(), (Point{Row: 3, Column: 8}); got != want {
		t.Fatalf("first child start point: got %+v want %+v", got, want)
	}
	if got, want := first.EndPoint(), (Point{Row: 3, Column: 10}); got != want {
		t.Fatalf("first child end point: got %+v want %+v", got, want)
	}

	second := offset.Child(1)
	if second == nil {
		t.Fatal("offset second child nil")
	}
	if got, want := second.StartPoint(), (Point{Row: 4, Column: 0}); got != want {
		t.Fatalf("second child start point: got %+v want %+v", got, want)
	}
	if got, want := second.EndPoint(), (Point{Row: 4, Column: 4}); got != want {
		t.Fatalf("second child end point: got %+v want %+v", got, want)
	}

	// Original tree must remain unchanged.
	if got, want := tree.RootNode().StartByte(), uint32(1); got != want {
		t.Fatalf("original root start byte mutated: got %d want %d", got, want)
	}
	if got, want := tree.RootNode().StartPoint(), (Point{Row: 0, Column: 1}); got != want {
		t.Fatalf("original root start point mutated: got %+v want %+v", got, want)
	}
}

func TestTreeWriteDOT(t *testing.T) {
	lang := testLanguage()
	left := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	right := NewLeafNode(Symbol(2), true, 3, 6, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 6})
	root := NewParentNode(Symbol(3), true, []*Node{left, right}, nil, 0)
	tree := NewTree(root, []byte("abcdef"), lang)

	dot := tree.DOT(lang)
	if !strings.Contains(dot, "digraph gotreesitter") {
		t.Fatalf("DOT missing graph header: %q", dot)
	}
	if !strings.Contains(dot, "expression [0,6)") {
		t.Fatalf("DOT missing root label: %q", dot)
	}
	if !strings.Contains(dot, "n0 -> n") {
		t.Fatalf("DOT missing edge: %q", dot)
	}
}

func TestDescendantForByteRange(t *testing.T) {
	lang := testLanguage()
	left := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	inner := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 3})
	right := NewParentNode(Symbol(3), true, []*Node{inner}, nil, 0)
	root := NewParentNode(Symbol(4), true, []*Node{left, right}, nil, 0)
	tree := NewTree(root, []byte("abc\ndef"), lang)

	got := tree.RootNode().DescendantForByteRange(4, 6)
	if got != inner {
		t.Fatal("DescendantForByteRange should return deepest matching descendant")
	}
	named := tree.RootNode().NamedDescendantForByteRange(4, 6)
	if named != inner {
		t.Fatal("NamedDescendantForByteRange should return deepest named descendant")
	}
}

func TestDescendantForPointRange(t *testing.T) {
	left := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	inner := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 3})
	right := NewParentNode(Symbol(3), true, []*Node{inner}, nil, 0)
	root := NewParentNode(Symbol(4), true, []*Node{left, right}, nil, 0)

	got := root.DescendantForPointRange(Point{Row: 1, Column: 0}, Point{Row: 1, Column: 2})
	if got != inner {
		t.Fatal("DescendantForPointRange should return deepest matching descendant")
	}
}

func TestHasErrorPropagation(t *testing.T) {
	// Create a child with an error.
	errChild := NewLeafNode(Symbol(5), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	errChild.setHasError(true)

	normalChild := NewLeafNode(Symbol(1), true, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2})

	parent := NewParentNode(Symbol(3), true, []*Node{errChild, normalChild}, nil, 0)
	if !parent.HasError() {
		t.Error("Parent HasError: got false, want true (child has error)")
	}

	// Normal case: no error children → parent has no error.
	clean0 := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	clean1 := NewLeafNode(Symbol(2), true, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2})
	cleanParent := NewParentNode(Symbol(3), true, []*Node{clean0, clean1}, nil, 0)
	if cleanParent.HasError() {
		t.Error("Clean parent HasError: got true, want false")
	}
}

func TestOutOfRange(t *testing.T) {
	child := NewLeafNode(Symbol(1), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	parent := NewParentNode(Symbol(3), true, []*Node{child}, nil, 0)

	if parent.Child(-1) != nil {
		t.Error("Child(-1): expected nil")
	}
	if parent.Child(100) != nil {
		t.Error("Child(100): expected nil")
	}
	if parent.NamedChild(100) != nil {
		t.Error("NamedChild(100): expected nil")
	}

	// Also test on a leaf node.
	if child.Child(0) != nil {
		t.Error("Leaf Child(0): expected nil")
	}
	if child.NamedChild(0) != nil {
		t.Error("Leaf NamedChild(0): expected nil")
	}
}

func TestDiffChangedRangesIdenticalTrees(t *testing.T) {
	lang := testLanguage()
	// Build two identical trees: program -> [identifier, number]
	oldLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	oldLeaf1 := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	oldRoot := NewParentNode(Symbol(4), true, []*Node{oldLeaf0, oldLeaf1}, nil, 0)
	oldTree := NewTree(oldRoot, []byte("abc def"), lang)

	newLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	newLeaf1 := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	newRoot := NewParentNode(Symbol(4), true, []*Node{newLeaf0, newLeaf1}, nil, 0)
	newTree := NewTree(newRoot, []byte("abc def"), lang)

	ranges := DiffChangedRanges(oldTree, newTree)
	if len(ranges) != 0 {
		t.Fatalf("identical trees: got %d ranges, want 0", len(ranges))
	}
}

func TestDiffChangedRangesLeafChanged(t *testing.T) {
	lang := testLanguage()

	// Old tree: program -> [identifier(0-3), number(4-7)]
	oldLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	oldLeaf1 := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	oldRoot := NewParentNode(Symbol(4), true, []*Node{oldLeaf0, oldLeaf1}, nil, 0)
	oldTree := NewTree(oldRoot, []byte("abc 123"), lang)

	// Simulate an edit in the second leaf: "123" -> "4567" (bytes 4-7 -> 4-8)
	oldTree.Edit(InputEdit{
		StartByte:   4,
		OldEndByte:  7,
		NewEndByte:  8,
		StartPoint:  Point{Row: 0, Column: 4},
		OldEndPoint: Point{Row: 0, Column: 7},
		NewEndPoint: Point{Row: 0, Column: 8},
	})

	// New tree: program -> [identifier(0-3), number(4-8)]
	newLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	newLeaf1 := NewLeafNode(Symbol(2), true, 4, 8, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 8})
	newRoot := NewParentNode(Symbol(4), true, []*Node{newLeaf0, newLeaf1}, nil, 0)
	newTree := NewTree(newRoot, []byte("abc 4567"), lang)

	ranges := DiffChangedRanges(oldTree, newTree)
	if len(ranges) != 1 {
		t.Fatalf("leaf change: got %d ranges, want 1", len(ranges))
	}
	// The changed range should cover bytes 4-8 (the union of old 4-8 edited + new 4-8)
	if ranges[0].StartByte != 4 || ranges[0].EndByte != 8 {
		t.Fatalf("leaf change range: got %d-%d, want 4-8", ranges[0].StartByte, ranges[0].EndByte)
	}
}

func TestDiffChangedRangesStructureChanged(t *testing.T) {
	lang := testLanguage()

	// Old tree: program -> [identifier(0-3), number(4-7)]
	oldLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	oldLeaf1 := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	oldRoot := NewParentNode(Symbol(4), true, []*Node{oldLeaf0, oldLeaf1}, nil, 0)
	oldTree := NewTree(oldRoot, []byte("abc 123"), lang)

	// Simulate an edit — mark the root as dirty
	oldTree.Edit(InputEdit{
		StartByte:   0,
		OldEndByte:  7,
		NewEndByte:  11,
		StartPoint:  Point{Row: 0, Column: 0},
		OldEndPoint: Point{Row: 0, Column: 7},
		NewEndPoint: Point{Row: 0, Column: 11},
	})

	// New tree: program -> [identifier(0-3), expression(4-7), number(8-11)]
	// Different child count (3 vs 2) -> entire root range reported
	newLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	newLeaf1 := NewLeafNode(Symbol(3), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	newLeaf2 := NewLeafNode(Symbol(2), true, 8, 11, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 11})
	newRoot := NewParentNode(Symbol(4), true, []*Node{newLeaf0, newLeaf1, newLeaf2}, nil, 0)
	newTree := NewTree(newRoot, []byte("abc expr 123"), lang)

	ranges := DiffChangedRanges(oldTree, newTree)
	if len(ranges) != 1 {
		t.Fatalf("structure change: got %d ranges, want 1", len(ranges))
	}
	// The changed range should cover the union of old (0-11 after edit) and new (0-11)
	if ranges[0].StartByte != 0 || ranges[0].EndByte != 11 {
		t.Fatalf("structure change range: got %d-%d, want 0-11", ranges[0].StartByte, ranges[0].EndByte)
	}
}

func TestDiffChangedRangesSymbolChanged(t *testing.T) {
	lang := testLanguage()

	// Old tree: program -> [identifier(0-3)]
	oldLeaf := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	oldLeaf.setDirty(true) // simulate edit marking
	oldRoot := NewParentNode(Symbol(4), true, []*Node{oldLeaf}, nil, 0)
	oldRoot.setDirty(true)
	oldTree := NewTree(oldRoot, []byte("abc"), lang)

	// New tree: program -> [number(0-3)] — same position, different symbol
	newLeaf := NewLeafNode(Symbol(2), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	newRoot := NewParentNode(Symbol(4), true, []*Node{newLeaf}, nil, 0)
	newTree := NewTree(newRoot, []byte("123"), lang)

	ranges := DiffChangedRanges(oldTree, newTree)
	if len(ranges) != 1 {
		t.Fatalf("symbol change: got %d ranges, want 1", len(ranges))
	}
	if ranges[0].StartByte != 0 || ranges[0].EndByte != 3 {
		t.Fatalf("symbol change range: got %d-%d, want 0-3", ranges[0].StartByte, ranges[0].EndByte)
	}
}

func TestDiffChangedRangesNilTrees(t *testing.T) {
	lang := testLanguage()
	leaf := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	root := NewParentNode(Symbol(4), true, []*Node{leaf}, nil, 0)
	tree := NewTree(root, []byte("abc"), lang)

	if ranges := DiffChangedRanges(nil, tree); ranges != nil {
		t.Fatal("nil oldTree: expected nil result")
	}
	if ranges := DiffChangedRanges(tree, nil); ranges != nil {
		t.Fatal("nil newTree: expected nil result")
	}
	if ranges := DiffChangedRanges(nil, nil); ranges != nil {
		t.Fatal("both nil: expected nil result")
	}

	// Tree with nil root
	emptyTree := NewTree(nil, nil, lang)
	if ranges := DiffChangedRanges(emptyTree, tree); ranges != nil {
		t.Fatal("nil root in oldTree: expected nil result")
	}
	if ranges := DiffChangedRanges(tree, emptyTree); ranges != nil {
		t.Fatal("nil root in newTree: expected nil result")
	}
}

func TestDiffChangedRangesMultipleChanges(t *testing.T) {
	lang := testLanguage()

	// Old tree: program -> [identifier(0-3), number(4-7), identifier(8-11)]
	oldLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	oldLeaf1 := NewLeafNode(Symbol(2), true, 4, 7, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 7})
	oldLeaf2 := NewLeafNode(Symbol(1), true, 8, 11, Point{Row: 0, Column: 8}, Point{Row: 0, Column: 11})
	oldRoot := NewParentNode(Symbol(4), true, []*Node{oldLeaf0, oldLeaf1, oldLeaf2}, nil, 0)
	oldTree := NewTree(oldRoot, []byte("abc 123 def"), lang)

	// Mark first and third children as dirty (simulating edits to non-adjacent ranges)
	oldLeaf0.setDirty(true)
	oldLeaf2.setDirty(true)
	oldRoot.setDirty(true)

	// New tree: first and third leaves have different byte ranges
	newLeaf0 := NewLeafNode(Symbol(1), true, 0, 4, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 4})
	newLeaf1 := NewLeafNode(Symbol(2), true, 5, 8, Point{Row: 0, Column: 5}, Point{Row: 0, Column: 8})
	newLeaf2 := NewLeafNode(Symbol(1), true, 9, 13, Point{Row: 0, Column: 9}, Point{Row: 0, Column: 13})
	newRoot := NewParentNode(Symbol(4), true, []*Node{newLeaf0, newLeaf1, newLeaf2}, nil, 0)
	newTree := NewTree(newRoot, []byte("abcd 123 defg"), lang)

	ranges := DiffChangedRanges(oldTree, newTree)
	// Expect changes for leaf0 (byte ranges differ) and leaf2 (byte ranges differ)
	// leaf1 also differs (4-7 vs 5-8) since dirty propagates
	// All three leaves differ in byte range, so we should get ranges for all three
	// But coalescing may merge them. Let's verify we get at least the right coverage.
	if len(ranges) == 0 {
		t.Fatal("multiple changes: expected non-empty ranges")
	}
	// The first changed range should start at 0
	if ranges[0].StartByte != 0 {
		t.Fatalf("first range start: got %d, want 0", ranges[0].StartByte)
	}
	// The last changed range should end at 13
	lastRange := ranges[len(ranges)-1]
	if lastRange.EndByte != 13 {
		t.Fatalf("last range end: got %d, want 13", lastRange.EndByte)
	}
}

func TestDiffChangedRangesCoalescing(t *testing.T) {
	lang := testLanguage()

	// Two adjacent changed children should be coalesced into one range.
	oldLeaf0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	oldLeaf1 := NewLeafNode(Symbol(2), true, 3, 6, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 6})
	oldRoot := NewParentNode(Symbol(4), true, []*Node{oldLeaf0, oldLeaf1}, nil, 0)
	oldLeaf0.setDirty(true)
	oldLeaf1.setDirty(true)
	oldRoot.setDirty(true)
	oldTree := NewTree(oldRoot, []byte("abcdef"), lang)

	// New tree: same structure but different byte ranges
	newLeaf0 := NewLeafNode(Symbol(1), true, 0, 4, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 4})
	newLeaf1 := NewLeafNode(Symbol(2), true, 4, 8, Point{Row: 0, Column: 4}, Point{Row: 0, Column: 8})
	newRoot := NewParentNode(Symbol(4), true, []*Node{newLeaf0, newLeaf1}, nil, 0)
	newTree := NewTree(newRoot, []byte("abcdEFGH"), lang)

	ranges := DiffChangedRanges(oldTree, newTree)
	// Both leaf ranges touch/overlap (0-3/0-4 and 3-6/4-8), should coalesce to 1
	if len(ranges) != 1 {
		t.Fatalf("coalescing: got %d ranges, want 1", len(ranges))
	}
	if ranges[0].StartByte != 0 || ranges[0].EndByte != 8 {
		t.Fatalf("coalesced range: got %d-%d, want 0-8", ranges[0].StartByte, ranges[0].EndByte)
	}
}

func TestTreeChangedRanges(t *testing.T) {
	lang := testLanguage()
	root := NewLeafNode(Symbol(1), true, 0, 6, Point{}, Point{Row: 0, Column: 6})
	tree := NewTree(root, []byte("abcdef"), lang)

	tree.Edit(InputEdit{
		StartByte:   1,
		OldEndByte:  2,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 1},
		OldEndPoint: Point{Row: 0, Column: 2},
		NewEndPoint: Point{Row: 0, Column: 3},
	})
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  4,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 4},
	})

	ranges := tree.ChangedRanges()
	if len(ranges) != 1 {
		t.Fatalf("ChangedRanges len: got %d, want 1", len(ranges))
	}
	if ranges[0].StartByte != 1 || ranges[0].EndByte != 4 {
		t.Fatalf("ChangedRanges bytes: got %d-%d, want 1-4", ranges[0].StartByte, ranges[0].EndByte)
	}
}

func TestNodeEditFromSubnodeMutatesContainingRoot(t *testing.T) {
	left := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	right := NewLeafNode(Symbol(2), true, 3, 6, Point{Row: 0, Column: 3}, Point{Row: 0, Column: 6})
	root := NewParentNode(Symbol(4), true, []*Node{left, right}, nil, 0)
	tree := NewTree(root, []byte("abcdef"), testLanguage())

	right.Edit(InputEdit{
		StartByte:   3,
		OldEndByte:  4,
		NewEndByte:  6, // +2 bytes
		StartPoint:  Point{Row: 0, Column: 3},
		OldEndPoint: Point{Row: 0, Column: 4},
		NewEndPoint: Point{Row: 0, Column: 6},
	})

	// Root and edited subtree should move together.
	if got, want := tree.RootNode().EndByte(), uint32(8); got != want {
		t.Fatalf("root.EndByte: got %d, want %d", got, want)
	}
	if got, want := right.EndByte(), uint32(8); got != want {
		t.Fatalf("right.EndByte: got %d, want %d", got, want)
	}
	// Unaffected left sibling should remain unchanged.
	if got, want := left.EndByte(), uint32(3); got != want {
		t.Fatalf("left.EndByte: got %d, want %d", got, want)
	}
	// Node-level edit does not append tree edit history.
	if got := len(tree.Edits()); got != 0 {
		t.Fatalf("tree.Edits len: got %d, want 0", got)
	}
}

func TestNodeEditDetachedNode(t *testing.T) {
	n := NewLeafNode(Symbol(1), true, 5, 7, Point{Row: 1, Column: 5}, Point{Row: 1, Column: 7})
	n.Edit(InputEdit{
		StartByte:   0,
		OldEndByte:  0,
		NewEndByte:  2, // insertion before node => shift by +2
		StartPoint:  Point{Row: 0, Column: 0},
		OldEndPoint: Point{Row: 0, Column: 0},
		NewEndPoint: Point{Row: 0, Column: 2},
	})

	if got, want := n.StartByte(), uint32(7); got != want {
		t.Fatalf("StartByte: got %d, want %d", got, want)
	}
	if got, want := n.EndByte(), uint32(9); got != want {
		t.Fatalf("EndByte: got %d, want %d", got, want)
	}
}

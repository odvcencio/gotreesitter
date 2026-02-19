package gotreesitter

import "testing"

// testLanguage returns a minimal Language for use in tree tests.
func testLanguage() *Language {
	return &Language{
		Name:        "test",
		SymbolNames: []string{"", "identifier", "number", "expression", "program", "ERROR"},
		FieldNames:  []string{"", "left", "right", "operator"},
		FieldCount:  3,
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

func TestLeafNodeTypeOutOfRange(t *testing.T) {
	lang := testLanguage()
	n := NewLeafNode(Symbol(999), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	if got := n.Type(lang); got != "" {
		t.Errorf("Type out of range: got %q, want empty", got)
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

func TestHasErrorPropagation(t *testing.T) {
	// Create a child with an error.
	errChild := NewLeafNode(Symbol(5), true, 0, 1, Point{}, Point{Row: 0, Column: 1})
	errChild.hasError = true

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

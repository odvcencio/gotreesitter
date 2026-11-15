package gotreesitter

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

func TestTreeCursorNew(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	root := tree.RootNode()

	c := NewTreeCursor(root, tree)
	if c.CurrentNode() != root {
		t.Fatal("CurrentNode should be root")
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth at root should be 0, got %d", c.Depth())
	}
}

func TestTreeCursorCoreInheritedFieldUnderExtra(t *testing.T) {
	// The first visible child is below an extra hidden node. Its node field is
	// inherited, but the cursor field stops at the extra node.
	a := newNodeArena(arenaClassIncremental)
	a.ccore = &ccoreArenaState{}
	view := &ccoreFakeView{
		kids:    map[ccore.Handle][]ccore.Handle{1: {2, 3, 4}, 3: {5}},
		visible: map[ccore.Handle]bool{4: true, 5: true},
		extra:   map[ccore.Handle]bool{3: true},
		prod:    map[ccore.Handle]uint16{1: 1},
	}
	tables := &ccoreConvTables{fieldSlices: [][2]uint16{{}, {0, 2}}, fieldEntries: []FieldMapEntry{{FieldID: 1, ChildIndex: 0}, {FieldID: 1, ChildIndex: 1}}}
	conv := &ccoreConv{fs: view, conv: tables, arena: a}
	inherited := conv.hiddenInheritedField(1, 1, view.Extra(3), 0, 0)
	secondField := conv.ownField(1, 1)
	first := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	second := NewLeafNode(1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	root := newParentNode(a, 2, true, []*Node{first, second}, []FieldID{inherited, secondField}, 0)
	conv.fids = []FieldID{inherited, secondField}
	conv.cfids = []FieldID{0, secondField}
	conv.recordFieldAnswers(root, 1, 0, 2)
	tree := &Tree{root: root, arena: a, language: queryTestLanguage()}
	c := NewTreeCursorFromTree(tree)
	if !c.GotoChildByFieldID(1) || c.CurrentNode() != second || c.CurrentFieldID() != 1 {
		t.Fatalf("cursor landed on %p with field %d, want second child %p with field 1", c.CurrentNode(), c.CurrentFieldID(), second)
	}
	c = NewTreeCursorFromTree(tree)
	if !c.GotoChildByFieldName("name") || c.CurrentNode() != second || c.CurrentFieldID() != 1 {
		t.Fatalf("field-name lookup landed on %p with field %d", c.CurrentNode(), c.CurrentFieldID())
	}
}

func TestTreeCursorNewWithNilNodeIsSafe(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	c := NewTreeCursor(nil, tree)
	if c.CurrentNode() != nil {
		t.Fatal("CurrentNode should be nil")
	}
	if c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should fail on nil node cursor")
	}
	if c.GotoNextSibling() {
		t.Fatal("GotoNextSibling should fail on nil node cursor")
	}
	if c.GotoChildByFieldID(1) {
		t.Fatal("GotoChildByFieldID should fail on nil node cursor")
	}
}

func TestTreeCursorFromNilTreeIsSafe(t *testing.T) {
	c := NewTreeCursorFromTree(nil)
	if c.CurrentNode() != nil {
		t.Fatal("CurrentNode should be nil")
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth should be 0, got %d", c.Depth())
	}
	if c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should fail for nil tree cursor")
	}
}

func TestTreeCursorGotoFirstChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)

	if !c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should succeed on program node")
	}
	// First child of program is function_declaration
	if c.CurrentNode().Symbol() != Symbol(5) {
		t.Fatalf("expected function_declaration (5), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 1 {
		t.Fatalf("Depth should be 1, got %d", c.Depth())
	}

	// Descend again: first child of function_declaration is "func" keyword
	if !c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should succeed on function_declaration")
	}
	if c.CurrentNode().Symbol() != Symbol(8) {
		t.Fatalf("expected func keyword (8), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 2 {
		t.Fatalf("Depth should be 2, got %d", c.Depth())
	}
}

func TestTreeCursorGotoNextSibling(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	// Traverse siblings: func -> identifier -> parameter_list -> block
	expected := []Symbol{1, 13, 14}
	for i, sym := range expected {
		if !c.GotoNextSibling() {
			t.Fatalf("GotoNextSibling should succeed at step %d", i)
		}
		if c.CurrentNode().Symbol() != sym {
			t.Fatalf("step %d: expected symbol %d, got %d", i, sym, c.CurrentNode().Symbol())
		}
	}

	// No more siblings
	if c.GotoNextSibling() {
		t.Fatal("GotoNextSibling should return false at last sibling")
	}
}

func TestTreeCursorGotoPrevSibling(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	// Move to last sibling
	for c.GotoNextSibling() {
	}
	// Now at block (14)
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}

	// Go back: block -> parameter_list -> identifier -> func
	expected := []Symbol{13, 1, 8}
	for i, sym := range expected {
		if !c.GotoPrevSibling() {
			t.Fatalf("GotoPrevSibling should succeed at step %d", i)
		}
		if c.CurrentNode().Symbol() != sym {
			t.Fatalf("step %d: expected symbol %d, got %d", i, sym, c.CurrentNode().Symbol())
		}
	}

	// No more prev siblings
	if c.GotoPrevSibling() {
		t.Fatal("GotoPrevSibling should return false at first sibling")
	}
}

func TestTreeCursorGotoParent(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	if !c.GotoParent() {
		t.Fatal("GotoParent should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(5) {
		t.Fatalf("expected function_declaration (5), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 1 {
		t.Fatalf("Depth should be 1, got %d", c.Depth())
	}

	if !c.GotoParent() {
		t.Fatal("GotoParent should succeed to root")
	}
	if c.CurrentNode().Symbol() != Symbol(7) {
		t.Fatalf("expected program (7), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth should be 0, got %d", c.Depth())
	}
}

func TestTreeCursorGotoLastChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	if !c.GotoLastChild() {
		t.Fatal("GotoLastChild should succeed")
	}
	// Last child of function_declaration is block (14)
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}
}

func TestTreeCursorAtLeaf(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword (leaf)

	if c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should return false on leaf node")
	}
	if c.GotoLastChild() {
		t.Fatal("GotoLastChild should return false on leaf node")
	}
}

func TestTreeCursorBoundary(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)

	// At root: parent should fail
	if c.GotoParent() {
		t.Fatal("GotoParent at root should return false")
	}

	// At root: siblings should fail
	if c.GotoNextSibling() {
		t.Fatal("GotoNextSibling at root should return false")
	}
	if c.GotoPrevSibling() {
		t.Fatal("GotoPrevSibling at root should return false")
	}
}

func TestTreeCursorFromTree(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursorFromTree(tree)
	if c.CurrentNode() != tree.RootNode() {
		t.Fatal("NewTreeCursorFromTree should start at root")
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth should be 0, got %d", c.Depth())
	}
}

func TestTreeCursorCurrentFieldID(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// buildSimpleTree: function_declaration children have fields:
	// [0: func(no field), 1: identifier(name=1), 2: parameter_list(parameters=5), 3: block(body=2)]
	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword — no field

	if fid := c.CurrentFieldID(); fid != 0 {
		t.Fatalf("func keyword should have field ID 0, got %d", fid)
	}

	c.GotoNextSibling() // identifier — field "name" (1)
	if fid := c.CurrentFieldID(); fid != FieldID(1) {
		t.Fatalf("identifier should have field ID 1 (name), got %d", fid)
	}

	c.GotoNextSibling() // parameter_list — field "parameters" (5)
	if fid := c.CurrentFieldID(); fid != FieldID(5) {
		t.Fatalf("parameter_list should have field ID 5 (parameters), got %d", fid)
	}

	c.GotoNextSibling() // block — field "body" (2)
	if fid := c.CurrentFieldID(); fid != FieldID(2) {
		t.Fatalf("block should have field ID 2 (body), got %d", fid)
	}
}

func TestTreeCursorCurrentFieldName(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	if name := c.CurrentFieldName(); name != "" {
		t.Fatalf("func keyword should have empty field name, got %q", name)
	}

	c.GotoNextSibling() // identifier
	if name := c.CurrentFieldName(); name != "name" {
		t.Fatalf("identifier field name should be 'name', got %q", name)
	}

	c.GotoNextSibling() // parameter_list
	if name := c.CurrentFieldName(); name != "parameters" {
		t.Fatalf("parameter_list field name should be 'parameters', got %q", name)
	}

	c.GotoNextSibling() // block
	if name := c.CurrentFieldName(); name != "body" {
		t.Fatalf("block field name should be 'body', got %q", name)
	}
}

func TestTreeCursorGotoChildByFieldName(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	if !c.GotoChildByFieldName("body") {
		t.Fatal("GotoChildByFieldName('body') should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}

	// Go back to function_declaration and try "name"
	c.GotoParent()
	if !c.GotoChildByFieldName("name") {
		t.Fatal("GotoChildByFieldName('name') should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(1) {
		t.Fatalf("expected identifier (1), got %d", c.CurrentNode().Symbol())
	}

	// Non-existent field
	c.GotoParent()
	if c.GotoChildByFieldName("nonexistent") {
		t.Fatal("GotoChildByFieldName('nonexistent') should return false")
	}
}

func TestTreeCursorGotoChildByFieldIDZeroSentinel(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	if c.GotoChildByFieldID(0) {
		t.Fatal("GotoChildByFieldID(0) should fail; 0 is the no-field sentinel")
	}
}

func TestTreeCursorFieldIDAtRoot(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	if fid := c.CurrentFieldID(); fid != 0 {
		t.Fatalf("field ID at root should be 0, got %d", fid)
	}
	if name := c.CurrentFieldName(); name != "" {
		t.Fatalf("field name at root should be empty, got %q", name)
	}
}

func TestTreeCursorGotoFirstNamedChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	// function_declaration children: func(anon), identifier(named), parameter_list(named), block(named)
	// GotoFirstNamedChild should skip "func" keyword and land on identifier
	if !c.GotoFirstNamedChild() {
		t.Fatal("GotoFirstNamedChild should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(1) {
		t.Fatalf("expected identifier (1), got %d", c.CurrentNode().Symbol())
	}
}

func TestTreeCursorGotoNextNamedSibling(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild()      // function_declaration
	c.GotoFirstNamedChild() // identifier

	// Next named sibling: parameter_list (13)
	if !c.GotoNextNamedSibling() {
		t.Fatal("GotoNextNamedSibling should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(13) {
		t.Fatalf("expected parameter_list (13), got %d", c.CurrentNode().Symbol())
	}

	// Next named sibling: block (14)
	if !c.GotoNextNamedSibling() {
		t.Fatal("GotoNextNamedSibling should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}

	// No more named siblings
	if c.GotoNextNamedSibling() {
		t.Fatal("GotoNextNamedSibling should return false at end")
	}
}

func TestTreeCursorGotoPrevNamedSibling(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoLastChild()  // block (14, named)

	// Prev named sibling: parameter_list (13)
	if !c.GotoPrevNamedSibling() {
		t.Fatal("GotoPrevNamedSibling should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(13) {
		t.Fatalf("expected parameter_list (13), got %d", c.CurrentNode().Symbol())
	}

	// Prev named sibling: identifier (1) — skips "func" keyword
	if !c.GotoPrevNamedSibling() {
		t.Fatal("GotoPrevNamedSibling should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(1) {
		t.Fatalf("expected identifier (1), got %d", c.CurrentNode().Symbol())
	}

	// No more prev named siblings (func keyword is anonymous)
	if c.GotoPrevNamedSibling() {
		t.Fatal("GotoPrevNamedSibling should return false — func keyword is anonymous")
	}
}

func TestTreeCursorGotoLastNamedChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	// Last named child: block (14)
	if !c.GotoLastNamedChild() {
		t.Fatal("GotoLastNamedChild should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}

	// Test on parameter_list which has only anonymous children: "(" and ")"
	c.GotoParent()                       // back to function_declaration
	c.GotoChildByFieldName("parameters") // parameter_list
	if c.GotoFirstNamedChild() {
		t.Fatal("GotoFirstNamedChild should return false on parameter_list with only anonymous children")
	}
	if c.GotoLastNamedChild() {
		t.Fatal("GotoLastNamedChild should return false on parameter_list with only anonymous children")
	}
}

func TestTreeCursorGotoFirstChildForByte(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	// Tree: func main() { 42 }
	// function_declaration children:
	//   func(0-4), identifier(5-9), parameter_list(9-11), block(14-16? endByte from tree)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	// Byte 6 is inside "main" (5-9), so first child where endByte > 6 is identifier
	if idx := c.GotoFirstChildForByte(6); idx != 1 {
		t.Fatalf("GotoFirstChildForByte(6) index: got %d, want 1", idx)
	}
	if c.CurrentNode().Symbol() != Symbol(1) {
		t.Fatalf("expected identifier (1), got %d", c.CurrentNode().Symbol())
	}

	// Go back and try byte 0, should land on "func" keyword
	c.GotoParent()
	if idx := c.GotoFirstChildForByte(0); idx != 0 {
		t.Fatalf("GotoFirstChildForByte(0) index: got %d, want 0", idx)
	}
	if c.CurrentNode().Symbol() != Symbol(8) {
		t.Fatalf("expected func keyword (8), got %d", c.CurrentNode().Symbol())
	}

	// Try byte 15, should land on block (which contains number at 14-16)
	c.GotoParent()
	if idx := c.GotoFirstChildForByte(15); idx != 3 {
		t.Fatalf("GotoFirstChildForByte(15) index: got %d, want 3", idx)
	}
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}
}

func TestTreeCursorGotoFirstChildForByteOutOfRange(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	// Byte way past end of all children
	if idx := c.GotoFirstChildForByte(9999); idx != -1 {
		t.Fatalf("GotoFirstChildForByte with out-of-range byte should return -1, got %d", idx)
	}

	// Leaf node
	c.GotoFirstChild() // "func" keyword
	if idx := c.GotoFirstChildForByte(0); idx != -1 {
		t.Fatalf("GotoFirstChildForByte on leaf should return -1, got %d", idx)
	}
}

func TestTreeCursorGotoFirstChildForPoint(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	// All nodes in buildSimpleTree are on row 0 with column == byte offset

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	// Point at column 6 (inside "main") should land on identifier
	if idx := c.GotoFirstChildForPoint(Point{Row: 0, Column: 6}); idx != 1 {
		t.Fatalf("GotoFirstChildForPoint index: got %d, want 1", idx)
	}
	if c.CurrentNode().Symbol() != Symbol(1) {
		t.Fatalf("expected identifier (1), got %d", c.CurrentNode().Symbol())
	}

	// Point at column 0 should land on "func" keyword
	c.GotoParent()
	if idx := c.GotoFirstChildForPoint(Point{Row: 0, Column: 0}); idx != 0 {
		t.Fatalf("GotoFirstChildForPoint(0,0) index: got %d, want 0", idx)
	}
	if c.CurrentNode().Symbol() != Symbol(8) {
		t.Fatalf("expected func keyword (8), got %d", c.CurrentNode().Symbol())
	}
}

func TestTreeCursorGotoFirstChildForByteExactBoundary(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	// targetByte == endByte of "func" (4) should pick the next child (identifier).
	if idx := c.GotoFirstChildForByte(4); idx != 1 {
		t.Fatalf("GotoFirstChildForByte(4) index: got %d, want 1", idx)
	}
	if c.CurrentNode().Symbol() != Symbol(1) {
		t.Fatalf("expected identifier (1), got %d", c.CurrentNode().Symbol())
	}
}

func buildMultiRowCursorTree() *Tree {
	c0 := NewLeafNode(Symbol(1), true, 0, 3, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 3})
	c1 := NewLeafNode(Symbol(2), true, 4, 8, Point{Row: 1, Column: 0}, Point{Row: 1, Column: 4})
	c2 := NewLeafNode(Symbol(3), true, 9, 14, Point{Row: 2, Column: 0}, Point{Row: 2, Column: 5})
	root := NewParentNode(Symbol(4), true, []*Node{c0, c1, c2}, []FieldID{0, 0, 0}, 0)
	return NewTree(root, []byte("aaa\nbbbb\nccccc"), nil)
}

func TestTreeCursorGotoFirstChildForPointMultiRow(t *testing.T) {
	tree := buildMultiRowCursorTree()
	c := NewTreeCursorFromTree(tree)

	if idx := c.GotoFirstChildForPoint(Point{Row: 1, Column: 1}); idx != 1 {
		t.Fatalf("GotoFirstChildForPoint(Row1) index: got %d, want 1", idx)
	}
	if c.CurrentNode().Symbol() != Symbol(2) {
		t.Fatalf("expected symbol 2, got %d", c.CurrentNode().Symbol())
	}

	c.GotoParent()
	if idx := c.GotoFirstChildForPoint(Point{Row: 0, Column: 3}); idx != 1 {
		t.Fatalf("GotoFirstChildForPoint(boundary) index: got %d, want 1", idx)
	}
}

func TestTreeCursorGotoParentThenGotoNextSibling(t *testing.T) {
	leafA := NewLeafNode(Symbol(1), true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1})
	leafB := NewLeafNode(Symbol(2), true, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2})
	child0 := NewParentNode(Symbol(3), true, []*Node{leafA}, []FieldID{0}, 0)
	child1 := NewParentNode(Symbol(4), true, []*Node{leafB}, []FieldID{0}, 0)
	root := NewParentNode(Symbol(5), true, []*Node{child0, child1}, []FieldID{0, 0}, 0)
	tree := NewTree(root, []byte("ab"), nil)
	c := NewTreeCursorFromTree(tree)

	if !c.GotoFirstChild() { // child0
		t.Fatal("GotoFirstChild should succeed")
	}
	if !c.GotoFirstChild() { // leafA
		t.Fatal("GotoFirstChild on child0 should succeed")
	}
	if !c.GotoParent() { // back to child0
		t.Fatal("GotoParent should succeed")
	}
	if !c.GotoNextSibling() { // child1
		t.Fatal("GotoNextSibling after GotoParent should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(4) {
		t.Fatalf("expected symbol 4, got %d", c.CurrentNode().Symbol())
	}
}

func TestTreeCursorReset(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursorFromTree(tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	if c.Depth() != 2 {
		t.Fatalf("expected depth 2, got %d", c.Depth())
	}

	// Reset to root
	c.Reset(tree.RootNode())
	if c.Depth() != 0 {
		t.Fatalf("after Reset, depth should be 0, got %d", c.Depth())
	}
	if c.CurrentNode() != tree.RootNode() {
		t.Fatal("after Reset, CurrentNode should be the new root")
	}

	// Verify navigation still works after reset
	if !c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should work after Reset")
	}
}

func TestTreeCursorResetZeroValue(t *testing.T) {
	var c TreeCursor
	c.Reset(nil)
	if c.CurrentNode() != nil {
		t.Fatal("CurrentNode should be nil after Reset(nil)")
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth should be 0, got %d", c.Depth())
	}
	if c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should fail after Reset(nil)")
	}
}

func TestTreeCursorResetTree(t *testing.T) {
	lang := queryTestLanguage()
	tree1 := buildSimpleTree(lang)
	tree2 := buildSimpleTree(lang)

	c := NewTreeCursorFromTree(tree1)
	c.GotoFirstChild()
	c.GotoFirstChild()

	c.ResetTree(tree2)
	if c.CurrentNode() != tree2.RootNode() {
		t.Fatal("ResetTree should place cursor at new tree root")
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth should be 0 after ResetTree, got %d", c.Depth())
	}
	if !c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should work after ResetTree")
	}
}

func TestTreeCursorResetTreeNil(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	c := NewTreeCursorFromTree(tree)
	c.GotoFirstChild()

	c.ResetTree(nil)
	if c.CurrentNode() != nil {
		t.Fatal("CurrentNode should be nil after ResetTree(nil)")
	}
	if c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should fail after ResetTree(nil)")
	}
}

func TestTreeCursorCopy(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursorFromTree(tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	cp := c.Copy()

	// Both should be at the same node
	if cp.CurrentNode() != c.CurrentNode() {
		t.Fatal("Copy should point to same node")
	}
	if cp.Depth() != c.Depth() {
		t.Fatal("Copy should have same depth")
	}

	// Moving the copy should not affect the original
	cp.GotoNextSibling() // identifier
	if cp.CurrentNode().Symbol() == c.CurrentNode().Symbol() {
		t.Fatal("moving copy should not affect original")
	}
	if c.CurrentNode().Symbol() != Symbol(8) {
		t.Fatalf("original should still be at func (8), got %d", c.CurrentNode().Symbol())
	}
}

func TestTreeCursorDFSTraversal(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// Collect nodes via Walk
	var walkSymbols []Symbol
	Walk(tree.RootNode(), func(n *Node, depth int) WalkAction {
		walkSymbols = append(walkSymbols, n.Symbol())
		return WalkContinue
	})

	// Collect nodes via cursor DFS
	var cursorSymbols []Symbol
	c := NewTreeCursorFromTree(tree)
	// Iterative DFS using cursor
	reachedRoot := false
	for !reachedRoot {
		cursorSymbols = append(cursorSymbols, c.CurrentNode().Symbol())

		// Try to go deeper
		if c.GotoFirstChild() {
			continue
		}
		// Try next sibling
		if c.GotoNextSibling() {
			continue
		}
		// Go up until we can go to a sibling or reach root
		for {
			if !c.GotoParent() {
				reachedRoot = true
				break
			}
			if c.GotoNextSibling() {
				break
			}
		}
	}

	if len(cursorSymbols) != len(walkSymbols) {
		t.Fatalf("cursor DFS found %d nodes, Walk found %d", len(cursorSymbols), len(walkSymbols))
	}
	for i := range walkSymbols {
		if cursorSymbols[i] != walkSymbols[i] {
			t.Fatalf("node %d: cursor got symbol %d, Walk got %d", i, cursorSymbols[i], walkSymbols[i])
		}
	}
}

func TestTreeCursorConvenienceAccessors(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursorFromTree(tree)
	c.GotoFirstChild() // function_declaration

	if typ := c.CurrentNodeType(); typ != "function_declaration" {
		t.Fatalf("expected 'function_declaration', got %q", typ)
	}
	if !c.CurrentNodeIsNamed() {
		t.Fatal("function_declaration should be named")
	}

	c.GotoFirstChild() // "func" keyword
	if typ := c.CurrentNodeType(); typ != "func" {
		t.Fatalf("expected 'func', got %q", typ)
	}
	if c.CurrentNodeIsNamed() {
		t.Fatal("func keyword should not be named")
	}
	if text := c.CurrentNodeText(); text != "func" {
		t.Fatalf("expected text 'func', got %q", text)
	}

	c.GotoNextSibling() // identifier "main"
	if text := c.CurrentNodeText(); text != "main" {
		t.Fatalf("expected text 'main', got %q", text)
	}
}

func TestTreeCursorBoundTree(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	bt := Bind(tree)

	c := bt.TreeCursor()
	if c.CurrentNode() != tree.RootNode() {
		t.Fatal("BoundTree.TreeCursor should start at root")
	}

	c.GotoFirstChild() // function_declaration
	if typ := c.CurrentNodeType(); typ != "function_declaration" {
		t.Fatalf("expected 'function_declaration', got %q", typ)
	}
}

func BenchmarkTreeCursorDFS(b *testing.B) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	c := NewTreeCursorFromTree(tree)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Reset(tree.RootNode())
		reachedRoot := false
		for !reachedRoot {
			_ = c.CurrentNode()
			if c.GotoFirstChild() {
				continue
			}
			if c.GotoNextSibling() {
				continue
			}
			for {
				if !c.GotoParent() {
					reachedRoot = true
					break
				}
				if c.GotoNextSibling() {
					break
				}
			}
		}
	}
}

func BenchmarkWalkDFS(b *testing.B) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Walk(tree.RootNode(), func(n *Node, depth int) WalkAction {
			_ = n
			return WalkContinue
		})
	}
}

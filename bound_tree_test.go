package gotreesitter

import "testing"

func TestBoundTreeNodeType(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	bt := Bind(tree)

	root := bt.RootNode()
	if got := bt.NodeType(root); got != "program" {
		t.Errorf("NodeType(root) = %q, want %q", got, "program")
	}
}

func TestBoundTreeNodeText(t *testing.T) {
	lang := queryTestLanguage()
	source := []byte("func main 42")
	funcKw := leaf(Symbol(8), false, 0, 4)
	ident := leaf(Symbol(1), true, 5, 9)
	num := leaf(Symbol(2), true, 10, 12)
	program := parent(Symbol(7), true,
		[]*Node{funcKw, ident, num},
		[]FieldID{0, 0, 0})
	tree := NewTree(program, source, lang)
	bt := Bind(tree)

	if got := bt.NodeText(ident); got != "main" {
		t.Errorf("NodeText(ident) = %q, want %q", got, "main")
	}
}

func TestBoundTreeChildByField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildFieldedTree(lang)
	bt := Bind(tree)

	root := bt.RootNode()
	funcDecl := root.Child(0)
	nameNode := bt.ChildByField(funcDecl, "name")
	if nameNode == nil {
		t.Fatal("ChildByField(name) returned nil")
	}
	if got := bt.NodeType(nameNode); got != "identifier" {
		t.Errorf("ChildByField(name) type = %q, want %q", got, "identifier")
	}
}

func TestBoundTreeRelease(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	bt := Bind(tree)
	bt.Release()
	bt.Release() // double release should not panic
}

func TestBoundTreeLanguage(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	bt := Bind(tree)

	if bt.Language() != lang {
		t.Error("Language() returned wrong language")
	}
}

func TestBoundTreeSource(t *testing.T) {
	lang := queryTestLanguage()
	source := []byte("func main 42")
	funcKw := leaf(Symbol(8), false, 0, 4)
	ident := leaf(Symbol(1), true, 5, 9)
	num := leaf(Symbol(2), true, 10, 12)
	program := parent(Symbol(7), true,
		[]*Node{funcKw, ident, num},
		[]FieldID{0, 0, 0})
	tree := NewTree(program, source, lang)
	bt := Bind(tree)

	if string(bt.Source()) != "func main 42" {
		t.Errorf("Source() = %q, want %q", string(bt.Source()), "func main 42")
	}
}

func TestBoundTreeParseReceipt(t *testing.T) {
	tree := buildSimpleTree(queryTestLanguage())
	tree.setParseStopReason(ParseStopTimeout)
	bt := Bind(tree)

	if got := bt.ParseStopReason(); got != ParseStopTimeout {
		t.Fatalf("ParseStopReason() = %q, want %q", got, ParseStopTimeout)
	}
	if !bt.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = false, want true")
	}
	if got := bt.ParseRuntime().StopReason; got != ParseStopTimeout {
		t.Fatalf("ParseRuntime().StopReason = %q, want %q", got, ParseStopTimeout)
	}
}

func TestNilBoundTreeParseReceipt(t *testing.T) {
	var bt *BoundTree
	if got := bt.ParseStopReason(); got != ParseStopNone {
		t.Fatalf("ParseStopReason() = %q, want %q", got, ParseStopNone)
	}
	if bt.ParseStoppedEarly() {
		t.Fatal("ParseStoppedEarly() = true, want false")
	}
	if got := bt.ParseRuntime().StopReason; got != ParseStopNone {
		t.Fatalf("ParseRuntime().StopReason = %q, want %q", got, ParseStopNone)
	}
}

func TestBindNil(t *testing.T) {
	bt := Bind(nil)
	if bt.RootNode() != nil {
		t.Error("Bind(nil).RootNode() should be nil")
	}
	bt.Release()
}

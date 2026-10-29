package gotreesitter

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// makeArithmeticExpr builds a long arithmetic expression "1+2+3+...+n".
func makeArithmeticExpr(n int) []byte {
	var sb strings.Builder
	sb.Grow(n * 4) // rough estimate
	for i := 1; i <= n; i++ {
		if i > 1 {
			sb.WriteByte('+')
		}
		fmt.Fprintf(&sb, "%d", i)
	}
	return []byte(sb.String())
}

func TestParseLargeFile(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	src := makeArithmeticExpr(2000)
	tree := mustParse(t, parser, src)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("expected non-nil root for large file")
	}
	if root.ChildCount() == 0 {
		t.Fatal("expected root to have children")
	}

	// The root expression should span the entire input.
	if root.StartByte() != 0 {
		t.Errorf("root StartByte = %d, want 0", root.StartByte())
	}
	if root.EndByte() != uint32(len(src)) {
		t.Errorf("root EndByte = %d, want %d", root.EndByte(), len(src))
	}
}

func TestParseDeeplyNested(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Generate "1+1+1+...+1" with 500 terms. This creates a left-recursive
	// tree with 499 levels of nesting.
	src := makeArithmeticExpr(500)
	tree := mustParse(t, parser, src)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("expected non-nil root for deeply nested expression")
	}

	// Walk down the left spine and count depth.
	depth := 0
	node := root
	for node.ChildCount() == 3 {
		node = node.Child(0)
		depth++
	}
	// 500 terms means 499 additions, so 499 levels of nesting.
	if depth != 499 {
		t.Errorf("left-nesting depth = %d, want 499", depth)
	}

	// The innermost expression should have 1 child (NUMBER).
	if node.ChildCount() != 1 {
		t.Errorf("innermost expression child count = %d, want 1", node.ChildCount())
	}
}

func TestParseIncrementalLargeFile(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	src := makeArithmeticExpr(500)
	tree := mustParse(t, parser, src)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("initial parse returned nil root")
	}

	// Edit a byte in the middle: change one digit to another.
	editAt := len(src) / 2
	// Find the nearest digit to edit.
	for editAt < len(src) && (src[editAt] < '0' || src[editAt] > '9') {
		editAt++
	}
	if editAt >= len(src) {
		t.Fatal("could not find a digit to edit in the middle of the source")
	}

	// Toggle the digit.
	if src[editAt] == '9' {
		src[editAt] = '1'
	} else {
		src[editAt]++
	}

	// Apply the edit to the old tree.
	edit := InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  Point{Row: 0, Column: uint32(editAt)},
		OldEndPoint: Point{Row: 0, Column: uint32(editAt + 1)},
		NewEndPoint: Point{Row: 0, Column: uint32(editAt + 1)},
	}
	tree.Edit(edit)

	newTree := mustParseIncremental(t, parser, src, tree)

	newRoot := newTree.RootNode()
	if newRoot == nil {
		t.Fatal("incremental parse returned nil root")
	}
	if newRoot.ChildCount() == 0 {
		t.Fatal("incremental parse root has no children")
	}

	// Verify the tree spans the entire source.
	if newRoot.StartByte() != 0 {
		t.Errorf("newRoot StartByte = %d, want 0", newRoot.StartByte())
	}
	if newRoot.EndByte() != uint32(len(src)) {
		t.Errorf("newRoot EndByte = %d, want %d", newRoot.EndByte(), len(src))
	}
}

func TestParseBinaryContent(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Generate 1MB of random bytes. The parser should not panic even
	// though this is not valid arithmetic source.
	rng := rand.New(rand.NewSource(42))
	src := make([]byte, 1<<20) // 1 MB
	rng.Read(src)

	tree := mustParse(t, parser, src)

	// We just need a valid tree (no panic). The tree may have errors
	// since random bytes are not valid arithmetic expressions.
	if tree == nil {
		t.Fatal("Parse returned nil tree for binary content")
	}
}

func TestParseNilSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, nil)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("expected a root for nil source, got nil")
	}
	if !root.IsError() || root.ChildCount() != 0 || root.EndByte() != 0 {
		t.Errorf("expected empty ERROR root for nil source, got symbol %d with %d children, end %d",
			root.Symbol(), root.ChildCount(), root.EndByte())
	}
}

func TestParseEmptySource(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte{})

	root := tree.RootNode()
	if root == nil {
		t.Fatal("expected a root for empty source, got nil")
	}
	if !root.IsError() || root.ChildCount() != 0 || root.EndByte() != 0 {
		t.Errorf("expected empty ERROR root for empty source, got symbol %d with %d children, end %d",
			root.Symbol(), root.ChildCount(), root.EndByte())
	}
}

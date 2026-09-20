package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestScalaProducedSpanBeforeCompatibility(t *testing.T) {
	const sourceText = `object SpanReceipt {
  def first =
    val value = 1
    value

  def second = 2

  def choose(n: Int) =
    n match {
      case 0 =>
        "zero"
      case _ =>
        "other"
    }
}
`
	source := []byte(sourceText)
	lang := grammars.ScalaLanguage()
	tree, err := gotreesitter.NewParser(lang).ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("compatibility-free Scala parse returned an invalid tree: %v", root)
	}

	// The C parser pinned in grammars/languages.lock produced these ranges.
	receipt := []struct {
		name       string
		kind       string
		startByte  uint32
		startPoint gotreesitter.Point
		endByte    uint32
		endPoint   gotreesitter.Point
	}{
		{
			name:       "first function",
			kind:       "function_definition",
			startByte:  23,
			startPoint: gotreesitter.Point{Row: 1, Column: 2},
			endByte:    66,
			endPoint:   gotreesitter.Point{Row: 5, Column: 2},
		},
		{
			name:       "first indented block",
			kind:       "indented_block",
			startByte:  39,
			startPoint: gotreesitter.Point{Row: 2, Column: 4},
			endByte:    66,
			endPoint:   gotreesitter.Point{Row: 5, Column: 2},
		},
		{
			// tree-sitter-scala's db390f312a54 grammar refresh tightens
			// case_clause so it ends at its body's last real token instead
			// of reaching into the next clause's leading whitespace.
			// Verified against the locked C oracle in Docker
			// (cgo_harness.TestScalaSpanOwnershipCOracleParity): compareNodes
			// finds no Go/C divergence, so both trees agree on this
			// narrower span.
			name:       "first case clause",
			kind:       "case_clause",
			startByte:  125,
			startPoint: gotreesitter.Point{Row: 9, Column: 6},
			endByte:    149,
			endPoint:   gotreesitter.Point{Row: 10, Column: 14},
		},
		{
			name:       "second case clause",
			kind:       "case_clause",
			startByte:  156,
			startPoint: gotreesitter.Point{Row: 11, Column: 6},
			endByte:    181,
			endPoint:   gotreesitter.Point{Row: 12, Column: 15},
		},
	}
	for _, want := range receipt {
		node := findScalaProducedSpanNode(root, lang, want.kind, want.startByte)
		if node == nil {
			t.Fatalf("%s is missing at byte %d: %s", want.name, want.startByte, root.SExpr(lang))
		}
		if got := node.StartByte(); got != want.startByte {
			t.Fatalf("%s start byte = %d, want %d", want.name, got, want.startByte)
		}
		if got := node.StartPoint(); got != want.startPoint {
			t.Fatalf("%s start point = %#v, want %#v", want.name, got, want.startPoint)
		}
		if got := node.EndByte(); got != want.endByte {
			t.Fatalf("%s end byte = %d, want %d", want.name, got, want.endByte)
		}
		if got := node.EndPoint(); got != want.endPoint {
			t.Fatalf("%s end point = %#v, want %#v", want.name, got, want.endPoint)
		}
	}
}

func findScalaProducedSpanNode(
	node *gotreesitter.Node,
	lang *gotreesitter.Language,
	kind string,
	startByte uint32,
) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if node.Type(lang) == kind && node.StartByte() == startByte {
		return node
	}
	for index := 0; index < node.ChildCount(); index++ {
		if found := findScalaProducedSpanNode(node.Child(index), lang, kind, startByte); found != nil {
			return found
		}
	}
	return nil
}

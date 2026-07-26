//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestScalaSpanOwnershipCOracleParity(t *testing.T) {
	scheduleParityMemoryScavenge(t)

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
	test := parityCase{name: "scala", source: sourceText}
	goTree, goLanguage, err := parseWithGo(test, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(goTree)

	cLanguage, err := ParityCLanguage(test.name)
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C reference parser returned a nil tree")
	}
	defer cTree.Close()

	var divergences []string
	compareNodes(goTree.RootNode(), goLanguage, cTree.RootNode(), "root", &divergences)
	if len(divergences) != 0 {
		t.Fatalf("C-oracle divergences = %v", divergences)
	}

	spans := []struct {
		name       string
		kind       string
		startByte  uint32
		startPoint gotreesitter.Point
		endByte    uint32
		endPoint   gotreesitter.Point
	}{
		{
			name:       "compilation root",
			kind:       "compilation_unit",
			startPoint: gotreesitter.Point{},
			endByte:    190,
			endPoint:   gotreesitter.Point{Row: 15},
		},
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
			name:       "first case clause",
			kind:       "case_clause",
			startByte:  125,
			startPoint: gotreesitter.Point{Row: 9, Column: 6},
			endByte:    156,
			endPoint:   gotreesitter.Point{Row: 11, Column: 6},
		},
		{
			name:       "second case clause",
			kind:       "case_clause",
			startByte:  156,
			startPoint: gotreesitter.Point{Row: 11, Column: 6},
			endByte:    186,
			endPoint:   gotreesitter.Point{Row: 13, Column: 4},
		},
	}
	for _, want := range spans {
		goNode := findGoNodeByTypeAndStart(goTree.RootNode(), goLanguage, want.kind, want.startByte)
		if goNode == nil {
			t.Fatalf("Go tree is missing %s at byte %d", want.name, want.startByte)
		}
		cNode := findCNodeByKindAndStart(cTree.RootNode(), want.kind, uint(want.startByte))
		if cNode == nil {
			t.Fatalf("C tree is missing %s at byte %d", want.name, want.startByte)
		}
		if goNode.StartByte() != want.startByte || goNode.StartPoint() != want.startPoint ||
			goNode.EndByte() != want.endByte || goNode.EndPoint() != want.endPoint {
			t.Fatalf(
				"Go %s range = %d:%#v-%d:%#v, want %d:%#v-%d:%#v",
				want.name,
				goNode.StartByte(),
				goNode.StartPoint(),
				goNode.EndByte(),
				goNode.EndPoint(),
				want.startByte,
				want.startPoint,
				want.endByte,
				want.endPoint,
			)
		}
		cStart := cNode.StartPosition()
		cEnd := cNode.EndPosition()
		if cNode.StartByte() != uint(want.startByte) ||
			uint32(cStart.Row) != want.startPoint.Row ||
			uint32(cStart.Column) != want.startPoint.Column ||
			cNode.EndByte() != uint(want.endByte) ||
			uint32(cEnd.Row) != want.endPoint.Row ||
			uint32(cEnd.Column) != want.endPoint.Column {
			t.Fatalf(
				"C %s range = %d:%#v-%d:%#v, want %d:%#v-%d:%#v",
				want.name,
				cNode.StartByte(),
				cStart,
				cNode.EndByte(),
				cEnd,
				want.startByte,
				want.startPoint,
				want.endByte,
				want.endPoint,
			)
		}
	}
}

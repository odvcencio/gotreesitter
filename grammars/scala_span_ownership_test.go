package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestScalaSpanOwnershipRoutes(t *testing.T) {
	const baseSourceText = `object SpanReceipt {
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
}`
	lang := ScalaLanguage()
	receipts := retiredDispatchRouteReceipts(t, lang, []byte(baseSourceText))

	source := append([]byte(baseSourceText), '\n')
	fresh, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("fresh parse failed: %v", err)
	}
	t.Cleanup(fresh.Release)
	receipts = append(receipts, retiredDispatchRouteReceipt{name: "fresh", tree: fresh})

	productionInspection, err := benchfixtures.InspectGoTree(receipts[0].tree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	freshInspection, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if freshInspection.SHA256 != productionInspection.SHA256 {
		t.Fatalf(
			"fresh digest = %s, want production %s",
			freshInspection.SHA256,
			productionInspection.SHA256,
		)
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
	for _, receipt := range receipts {
		root := receipt.tree.RootNode()
		for _, want := range spans {
			node := scalaSpanOwnershipNode(root, lang, want.kind, want.startByte)
			if node == nil {
				t.Fatalf(
					"%s route is missing %s at byte %d: %s",
					receipt.name,
					want.name,
					want.startByte,
					root.SExpr(lang),
				)
			}
			if node.StartByte() != want.startByte || node.StartPoint() != want.startPoint ||
				node.EndByte() != want.endByte || node.EndPoint() != want.endPoint {
				t.Fatalf(
					"%s %s range = %d:%#v-%d:%#v, want %d:%#v-%d:%#v",
					receipt.name,
					want.name,
					node.StartByte(),
					node.StartPoint(),
					node.EndByte(),
					node.EndPoint(),
					want.startByte,
					want.startPoint,
					want.endByte,
					want.endPoint,
				)
			}
		}
		if receipt.name != "incremental" {
			continue
		}
		profile := receipt.incrementalProfile
		if !profile.ReuseUnsupported ||
			profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
			profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
			t.Fatalf("incremental Scala reuse receipt = %+v", profile)
		}
	}
}

func scalaSpanOwnershipNode(
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
		found := scalaSpanOwnershipNode(node.Child(index), lang, kind, startByte)
		if found != nil {
			return found
		}
	}
	return nil
}

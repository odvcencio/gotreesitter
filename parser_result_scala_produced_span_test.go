package gotreesitter_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
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

// TestScalaSnippetRootMemoMatchesDirectSnippetParse pins the snippet memo
// that removed the FuzzScanner_Scala allocation finding (seeds
// 17b32d7658155455 and 3a3aee1efac4b114). Scala template-member recovery
// parses each member as a snippet on every pass of the parent's retry ladder.
// The parent now keeps one zero-offset copy of the snippet root per parse
// operation and clones it on later calls. Both calls must return the same
// tree, field for field, as an independent snippet parse shifted with
// Tree.RootNodeWithOffset, and the memo must be empty once the operation ends.
func TestScalaSnippetRootMemoMatchesDirectSnippetParse(t *testing.T) {
	lang := grammars.ScalaLanguage()
	cases := []struct {
		name    string
		source  string
		snippet string
	}{
		{"fuzz_member", "object A { def A(A:A),A000 0} ", "def A(A:A),A000 0"},
		{"second_row", "object A {\n  val x = 1\n  def f(x: Int) = x +\n}\n", "def f(x: Int) = x +"},
		{"clean_member", "object A {\n  def f(x: Int): Int = x + 1\n}\n", "def f(x: Int): Int = x + 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source := []byte(c.source)
			start := strings.Index(c.source, c.snippet)
			if start < 0 {
				t.Fatalf("snippet %q is not in source", c.snippet)
			}
			end := start + len(c.snippet)
			probe := gotreesitter.SnippetRootMemoProbeForTest(gotreesitter.NewParser(lang), source, uint32(start), uint32(end))
			defer probe.Release()
			if !probe.Memoized {
				t.Fatal("the first snippet parse did not store its root")
			}
			if !probe.ClearedAfterOperation {
				t.Fatal("the snippet memo outlived its parse operation")
			}
			if probe.First == nil || probe.Second == nil || probe.Direct == nil {
				t.Fatalf("missing snippet root: first=%v second=%v direct=%v", probe.First != nil, probe.Second != nil, probe.Direct != nil)
			}
			want, err := benchfixtures.InspectGoTree(probe.Direct, lang)
			if err != nil {
				t.Fatal(err)
			}
			for name, got := range map[string]*gotreesitter.Node{"first": probe.First, "memoized": probe.Second} {
				digest, err := benchfixtures.InspectGoTree(got, lang)
				if err != nil {
					t.Fatal(err)
				}
				if digest.SHA256 != want.SHA256 {
					t.Fatalf("%s snippet root %s differs from the direct parse %s", name, got.SExpr(lang), probe.Direct.SExpr(lang))
				}
				if got.StartByte() != uint32(start) || got.StartPoint() != probe.Direct.StartPoint() {
					t.Fatalf("%s snippet root starts at %d %v, want %d %v", name, got.StartByte(), got.StartPoint(), start, probe.Direct.StartPoint())
				}
			}
		})
	}
}

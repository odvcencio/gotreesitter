package gotreesitter_test

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestArenaSliceAliasJSWhileStatement guards the arena.allocNodeSlice fix
// that caps returned slices at their length. Without `:slab.used:slab.used`
// the slab's spare capacity leaks beyond the parent's children, and an
// `append` from a downstream pass (e.g. the trailing-continue-comment
// normalizer that ran on the while-body statement_block) can overwrite the
// next parent's children in the shared backing array. The visible symptom in
// text-editor-component.js was the JS while_statement at bytes [81360..82557]
// reporting Child(0) as the body's closing `}` instead of the `while` keyword,
// which broke the JS leg of the real-corpus parity bench matrix.
func TestArenaSliceAliasJSWhileStatement(t *testing.T) {
	const path = "cgo_harness/corpus_real/javascript/large__text-editor-component.js"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("corpus not present: %v", err)
	}
	lang := grammars.JavascriptLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	target := findNodeAtBytes(tree.RootNode(), lang, "while_statement", 81360, 82557)
	if target == nil {
		t.Fatalf("no while_statement at bytes 81360..82557")
	}
	first := target.Child(0)
	if first == nil || first.Type(lang) != "while" {
		gotType := "<nil>"
		if first != nil {
			gotType = first.Type(lang)
		}
		t.Fatalf("while_statement.Child(0).Type = %q, want %q (arena slice aliasing regression)",
			gotType, "while")
	}
}

// TestArenaSliceAliasNoChildOrderAnomalies parses the same file and walks the
// whole tree looking for any parent whose children are not in non-decreasing
// byte order. A single corrupted parent (caused by slab aliasing) would show
// up here even if the specific while_statement above weren't affected.
func TestArenaSliceAliasNoChildOrderAnomalies(t *testing.T) {
	const path = "cgo_harness/corpus_real/javascript/large__text-editor-component.js"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("corpus not present: %v", err)
	}
	lang := grammars.JavascriptLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	type anomaly struct {
		parentType  string
		parentStart uint32
		idx         int
		childType   string
		childStart  uint32
		prevType    string
		prevStart   uint32
	}
	var anomalies []anomaly
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		var prevStart uint32
		var prevType string
		for i := 0; i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c == nil {
				continue
			}
			cs := c.StartByte()
			if i > 0 && cs < prevStart {
				anomalies = append(anomalies, anomaly{
					parentType: n.Type(lang), parentStart: n.StartByte(),
					idx: i, childType: c.Type(lang), childStart: cs,
					prevType: prevType, prevStart: prevStart,
				})
			}
			prevStart, prevType = cs, c.Type(lang)
			walk(c)
		}
	}
	walk(tree.RootNode())

	if len(anomalies) > 0 {
		for i, a := range anomalies {
			if i >= 5 {
				t.Logf("  ... (%d more anomalies suppressed)", len(anomalies)-5)
				break
			}
			t.Logf("anomaly: parent=%s@%d child[%d]=%s@%d (prev %s@%d)",
				a.parentType, a.parentStart, a.idx, a.childType, a.childStart,
				a.prevType, a.prevStart)
		}
		t.Fatalf("%d child-order anomalies", len(anomalies))
	}
}

func findNodeAtBytes(n *gotreesitter.Node, lang *gotreesitter.Language, typ string, start, end uint32) *gotreesitter.Node {
	if n == nil {
		return nil
	}
	if n.Type(lang) == typ && n.StartByte() == start && n.EndByte() == end {
		return n
	}
	for i := 0; i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c == nil {
			continue
		}
		if found := findNodeAtBytes(c, lang, typ, start, end); found != nil {
			return found
		}
	}
	return nil
}

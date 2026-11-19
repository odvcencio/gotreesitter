//go:build cgo && treesitter_c_parity

package cgoharness

// Systematic comparisons of the public tree with C's, node by node, for the
// field APIs, the edited flags and the changed ranges. The trees are walked in
// step by child index, so they are compared only when the route's tree equals C's.

import (
	"fmt"

	g "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ccoreFieldCounts are the counts of one language: the field lookups compared (one
// per node and per field of the language), the cursor steps compared, the
// differences, and the first difference of each kind.
type ccoreFieldCounts struct {
	lookups, lookupDiffs, steps, stepDiffs int
	firstLookup, firstCursor               string
}

func ccoreSameNode(gn *g.Node, lang *g.Language, cn *sitter.Node) bool {
	if gn == nil || cn == nil {
		return gn == nil && cn == nil
	}
	return uint(gn.StartByte()) == cn.StartByte() && uint(gn.EndByte()) == cn.EndByte() && gn.Type(lang) == cn.Kind()
}

// ccoreCompareFields compares, for every node of the tree and every field of the
// language, Node.ChildByFieldName with ts_node_child_by_field_name, and walks both
// trees with a cursor comparing the current field at every step.
func ccoreCompareFields(lang *g.Language, root *g.Node, tree *g.Tree, cRoot *sitter.Node) ccoreFieldCounts {
	var out ccoreFieldCounts
	var walk func(gn *g.Node, cn *sitter.Node)
	walk = func(gn *g.Node, cn *sitter.Node) {
		for fid := 1; fid < len(lang.FieldNames); fid++ {
			name := lang.FieldNames[fid]
			if name == "" {
				continue
			}
			out.lookups++
			gc, cc := gn.ChildByFieldName(name, lang), cn.ChildByFieldName(name)
			if !ccoreSameNode(gc, lang, cc) {
				out.lookupDiffs++
				if out.firstLookup == "" {
					d := func(a *g.Node, b *sitter.Node) string {
						if a != nil {
							return fmt.Sprintf("go %s[%d,%d)", a.Type(lang), a.StartByte(), a.EndByte())
						}
						return fmt.Sprintf("c %s[%d,%d)", b.Kind(), b.StartByte(), b.EndByte())
					}
					gs, cs := "go nil", "c nil"
					if gc != nil {
						gs = d(gc, nil)
					}
					if cc != nil {
						cs = d(nil, cc)
					}
					out.firstLookup = fmt.Sprintf("%s[%d,%d) field %q: %s, %s", gn.Type(lang), gn.StartByte(), gn.EndByte(), name, gs, cs)
				}
			}
		}
		n := gn.ChildCount()
		if n != int(cn.ChildCount()) {
			return
		}
		for i := 0; i < n; i++ {
			walk(gn.Child(i), cn.Child(uint(i)))
		}
	}
	walk(root, cRoot)
	gc, cc := g.NewTreeCursor(root, tree), cRoot.Walk()
	defer cc.Close()
	compare := func() {
		out.steps++
		gf, cf := gc.CurrentFieldName(), cc.FieldName()
		if gf != cf {
			out.stepDiffs++
			if out.firstCursor == "" {
				n := gc.CurrentNode()
				out.firstCursor = fmt.Sprintf("%s[%d,%d) go %q c %q", n.Type(lang), n.StartByte(), n.EndByte(), gf, cf)
			}
		}
	}
	for done := false; !done; {
		compare()
		switch {
		case gc.GotoFirstChild() && cc.GotoFirstChild():
		case gc.GotoNextSibling() && cc.GotoNextSibling():
		default:
			for {
				if !gc.GotoParent() || !cc.GotoParent() {
					done = true
					break
				}
				if gc.GotoNextSibling() && cc.GotoNextSibling() {
					break
				}
			}
		}
	}
	return out
}

// ccoreCompareChanges counts the nodes whose HasChanges equals C's ts_node_has_changes.
func ccoreCompareChanges(root *g.Node, cRoot *sitter.Node) (nodes, diffs int) {
	var walk func(gn *g.Node, cn *sitter.Node)
	walk = func(gn *g.Node, cn *sitter.Node) {
		nodes++
		if gn.HasChanges() != cn.HasChanges() {
			diffs++
		}
		n := gn.ChildCount()
		if n != int(cn.ChildCount()) {
			return
		}
		for i := 0; i < n; i++ {
			walk(gn.Child(i), cn.Child(uint(i)))
		}
	}
	walk(root, cRoot)
	return nodes, diffs
}

// ccoreSameRanges compares the changed ranges of the public tree with C's.
func ccoreSameRanges(a []g.Range, b []sitter.Range) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if uint(a[i].StartByte) != b[i].StartByte || uint(a[i].EndByte) != b[i].EndByte ||
			uint(a[i].StartPoint.Row) != b[i].StartPoint.Row || uint(a[i].StartPoint.Column) != b[i].StartPoint.Column ||
			uint(a[i].EndPoint.Row) != b[i].EndPoint.Row || uint(a[i].EndPoint.Column) != b[i].EndPoint.Column {
			return false
		}
	}
	return true
}

package corpuscheck

import gotreesitter "github.com/odvcencio/gotreesitter"

// RenderTree converts a live gotreesitter parse tree into the same
// canonical SNode shape ParseExpected produces from a corpus fixture's
// text, so the two can be compared structurally without caring about
// whitespace or indentation.
//
// A node is included if it is named, or if it is a MISSING node
// (including an anonymous MISSING token, which tree-sitter's own
// s-expression dump shows even though the corresponding present token
// would be invisible). Every other unnamed node (ordinary anonymous
// punctuation/keyword tokens) is skipped entirely, matching upstream
// tree-sitter's own S-expression dump.
func RenderTree(root *gotreesitter.Node, lang *gotreesitter.Language) *SNode {
	if root == nil || lang == nil {
		return nil
	}
	return renderNode(root, lang, "")
}

func renderNode(n *gotreesitter.Node, lang *gotreesitter.Language, field string) *SNode {
	if n == nil {
		return nil
	}
	if n.IsMissing() {
		typ := n.Type(lang)
		return &SNode{
			Field:         field,
			Missing:       true,
			Type:          typ,
			MissingIsAnon: !n.IsNamed(),
		}
	}
	node := &SNode{Field: field, Type: n.Type(lang)}
	count := n.ChildCount()
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if !child.IsNamed() && !child.IsMissing() {
			continue
		}
		childField := n.FieldNameForChild(i, lang)
		node.Children = append(node.Children, renderNode(child, lang, childField))
	}
	return node
}

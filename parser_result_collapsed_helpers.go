package gotreesitter

// normalizeCollapsedNamedLeafChildren restores collapsed single-anonymous-child
// nodes. When a named node (parentName) wraps a single anonymous token
// (childName) and the collapse logic strips the child, this function
// reconstructs the child so the tree matches C tree-sitter output.
func normalizeCollapsedNamedLeafChildren(root *Node, lang *Language, parentName, childName string) {
	if root == nil || lang == nil {
		return
	}
	parentSym, ok := symbolByName(lang, parentName)
	if !ok {
		return
	}
	childSym, childOk := symbolByName(lang, childName)
	if !childOk {
		return
	}
	childNamed := false
	if int(childSym) < len(lang.SymbolMetadata) {
		childNamed = lang.SymbolMetadata[childSym].Named
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.symbol == parentSym && len(n.children) == 0 {
			child := newLeafNodeInArena(n.ownerArena, childSym, childNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

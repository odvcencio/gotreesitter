package gotreesitter

// BoundTree pairs a Tree with its Language and source, eliminating the need
// to pass *Language and []byte to every node method call.
type BoundTree struct {
	tree *Tree
}

// Bind creates a BoundTree from a Tree. The Tree must have been created with
// a Language (via NewTree or a Parser). Returns a BoundTree that delegates to
// the underlying Tree's Language and Source.
func Bind(tree *Tree) *BoundTree {
	return &BoundTree{tree: tree}
}

// RootNode returns the tree's root node.
func (bt *BoundTree) RootNode() *Node {
	if bt == nil || bt.tree == nil {
		return nil
	}
	return bt.tree.RootNode()
}

// Language returns the tree's language.
func (bt *BoundTree) Language() *Language {
	if bt == nil || bt.tree == nil {
		return nil
	}
	return bt.tree.Language()
}

// Source returns the tree's source bytes.
func (bt *BoundTree) Source() []byte {
	if bt == nil || bt.tree == nil {
		return nil
	}
	return bt.tree.Source()
}

// ParseStopReason reports why parsing ended.
func (bt *BoundTree) ParseStopReason() ParseStopReason {
	if bt == nil || bt.tree == nil {
		return ParseStopNone
	}
	return bt.tree.ParseStopReason()
}

// ParseStoppedEarly reports whether parsing hit an early-stop condition.
func (bt *BoundTree) ParseStoppedEarly() bool {
	return bt != nil && bt.tree != nil && bt.tree.ParseStoppedEarly()
}

// ParseRuntime returns diagnostics from the parse that built the tree.
func (bt *BoundTree) ParseRuntime() ParseRuntime {
	if bt == nil || bt.tree == nil {
		return ParseRuntime{StopReason: ParseStopNone}
	}
	return bt.tree.ParseRuntime()
}

// NodeType returns the node's type name, resolved via the bound language.
func (bt *BoundTree) NodeType(n *Node) string {
	if bt == nil || bt.tree == nil || n == nil {
		return ""
	}
	return n.Type(bt.tree.Language())
}

// NodeText returns the source text covered by the node.
func (bt *BoundTree) NodeText(n *Node) string {
	if bt == nil || bt.tree == nil || n == nil {
		return ""
	}
	return n.Text(bt.tree.Source())
}

// ChildByField returns the first child assigned to the given field name.
func (bt *BoundTree) ChildByField(n *Node, fieldName string) *Node {
	if bt == nil || bt.tree == nil || n == nil {
		return nil
	}
	return n.ChildByFieldName(fieldName, bt.tree.Language())
}

// TreeCursor returns a new TreeCursor starting at the tree's root node.
func (bt *BoundTree) TreeCursor() *TreeCursor {
	if bt == nil || bt.tree == nil {
		return nil
	}
	return NewTreeCursorFromTree(bt.tree)
}

// Release releases the underlying tree's arena memory.
func (bt *BoundTree) Release() {
	if bt == nil || bt.tree == nil {
		return
	}
	bt.tree.Release()
}

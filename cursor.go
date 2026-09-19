package gotreesitter

import "sort"

// cursorFrame tracks a node and the child index within its parent.
// childIndex is -1 for the cursor root (no parent context).
type cursorFrame struct {
	node       *Node
	childIndex int
}

// TreeCursor provides stateful, O(1) tree navigation.
// It maintains a stack of (node, childIndex) frames enabling efficient
// parent, child, and sibling movement without scanning.
//
// The cursor holds pointers to Nodes. If the underlying Tree is released,
// edited, or replaced via incremental reparse, the cursor should be recreated.
type TreeCursor struct {
	stack []cursorFrame
	tree  *Tree
}

// NewTreeCursor creates a cursor starting at the given node.
// The optional tree reference enables field name resolution and text extraction.
func NewTreeCursor(node *Node, tree *Tree) *TreeCursor {
	return &TreeCursor{
		stack: []cursorFrame{{node: node, childIndex: -1}},
		tree:  tree,
	}
}

// NewTreeCursorFromTree creates a cursor starting at the tree's root node.
func NewTreeCursorFromTree(tree *Tree) *TreeCursor {
	if tree == nil {
		return NewTreeCursor(nil, nil)
	}
	return NewTreeCursor(tree.RootNode(), tree)
}

func (c *TreeCursor) ensureStack() {
	if len(c.stack) == 0 {
		c.stack = []cursorFrame{{node: nil, childIndex: -1}}
	}
}

// CurrentNode returns the node the cursor is currently pointing to.
// Returns nil for a nil cursor.
func (c *TreeCursor) CurrentNode() *Node {
	if c == nil || len(c.stack) == 0 {
		return nil
	}
	return c.stack[len(c.stack)-1].node
}

// Depth returns the cursor's current depth (0 at the root). Returns 0 for
// a nil cursor.
func (c *TreeCursor) Depth() int {
	if c == nil || len(c.stack) == 0 {
		return 0
	}
	return len(c.stack) - 1
}

// GotoFirstChild moves the cursor to the first child of the current node.
// Returns false if the current node has no children.
func (c *TreeCursor) GotoFirstChild() bool {
	node := c.CurrentNode()
	if node == nil || nodeChildCountNoMaterialize(node) == 0 {
		return false
	}
	c.stack = append(c.stack, cursorFrame{node: nodeChildAtForReason(node, 0, materializeForCursor), childIndex: 0})
	return true
}

// GotoLastChild moves the cursor to the last child of the current node.
// Returns false if the current node has no children.
func (c *TreeCursor) GotoLastChild() bool {
	node := c.CurrentNode()
	if node == nil {
		return false
	}
	n := nodeChildCountNoMaterialize(node)
	if n == 0 {
		return false
	}
	c.stack = append(c.stack, cursorFrame{node: nodeChildAtForReason(node, n-1, materializeForCursor), childIndex: n - 1})
	return true
}

// GotoNextSibling moves the cursor to the next sibling.
// Returns false if the cursor is at the root or the last sibling, or the
// cursor is nil.
func (c *TreeCursor) GotoNextSibling() bool {
	if c == nil || len(c.stack) < 2 {
		return false
	}
	frame := &c.stack[len(c.stack)-1]
	parentNode := c.stack[len(c.stack)-2].node
	if parentNode == nil {
		return false
	}
	next := frame.childIndex + 1
	if next >= nodeChildCountNoMaterialize(parentNode) {
		return false
	}
	frame.childIndex = next
	frame.node = nodeChildAtForReason(parentNode, next, materializeForCursor)
	return true
}

// GotoPrevSibling moves the cursor to the previous sibling.
// Returns false if the cursor is at the root or the first sibling, or the
// cursor is nil.
func (c *TreeCursor) GotoPrevSibling() bool {
	if c == nil || len(c.stack) < 2 {
		return false
	}
	frame := &c.stack[len(c.stack)-1]
	parentNode := c.stack[len(c.stack)-2].node
	if parentNode == nil {
		return false
	}
	prev := frame.childIndex - 1
	if prev < 0 {
		return false
	}
	frame.childIndex = prev
	frame.node = nodeChildAtForReason(parentNode, prev, materializeForCursor)
	return true
}

// GotoParent moves the cursor to the parent of the current node.
// Returns false if the cursor is at the root, or the cursor is nil.
func (c *TreeCursor) GotoParent() bool {
	if c == nil || len(c.stack) < 2 {
		return false
	}
	c.stack = c.stack[:len(c.stack)-1]
	return true
}

// CurrentFieldID returns the field ID of the current node within its parent.
// Returns 0 if the cursor is nil, is at the root, or the node has no field
// assignment.
func (c *TreeCursor) CurrentFieldID() FieldID {
	if c == nil || len(c.stack) < 2 {
		return 0
	}
	frame := c.stack[len(c.stack)-1]
	parentNode := c.stack[len(c.stack)-2].node
	if parentNode == nil {
		return 0
	}
	return nodeFieldIDAt(parentNode, frame.childIndex)
}

// CurrentFieldName returns the field name of the current node within its parent.
// Returns "" if the cursor is nil, no tree is associated, the cursor is at
// the root, or the node has no field assignment.
func (c *TreeCursor) CurrentFieldName() string {
	fid := c.CurrentFieldID()
	if fid == 0 || c.tree == nil {
		return ""
	}
	lang := c.tree.Language()
	if lang == nil || int(fid) >= len(lang.FieldNames) {
		return ""
	}
	return lang.FieldNames[fid]
}

// GotoChildByFieldID moves the cursor to the first child with the given field ID.
// Returns false if no child has that field.
func (c *TreeCursor) GotoChildByFieldID(fid FieldID) bool {
	if fid == 0 {
		// Field ID 0 is a sentinel meaning "no field assignment".
		return false
	}
	node := c.CurrentNode()
	if node == nil {
		return false
	}
	count := nodeChildCountNoMaterialize(node)
	for i := 0; i < count; i++ {
		if nodeFieldIDAt(node, i) == fid {
			c.stack = append(c.stack, cursorFrame{node: nodeChildAtForReason(node, i, materializeForCursor), childIndex: i})
			return true
		}
	}
	return false
}

// GotoChildByFieldName moves the cursor to the first child with the given field name.
// Returns false if the cursor is nil, the tree has no language, the field
// name is unknown, or no child has that field.
func (c *TreeCursor) GotoChildByFieldName(name string) bool {
	if c == nil || c.tree == nil {
		return false
	}
	lang := c.tree.Language()
	if lang == nil {
		return false
	}
	fid, ok := lang.FieldByName(name)
	if !ok || fid == 0 {
		return false
	}
	return c.GotoChildByFieldID(fid)
}

// GotoFirstNamedChild moves the cursor to the first named child of the
// current node, skipping anonymous nodes. Returns false if no named child exists.
func (c *TreeCursor) GotoFirstNamedChild() bool {
	node := c.CurrentNode()
	if node == nil {
		return false
	}
	count := nodeChildCountNoMaterialize(node)
	for i := 0; i < count; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(node, i)
		if !ok || !stackEntryNodeIsNamed(entry) {
			continue
		}
		child := nodeChildAtForReason(node, i, materializeForCursor)
		if child != nil {
			c.stack = append(c.stack, cursorFrame{node: child, childIndex: i})
			return true
		}
	}
	return false
}

// GotoLastNamedChild moves the cursor to the last named child of the
// current node, skipping anonymous nodes. Returns false if no named child exists.
func (c *TreeCursor) GotoLastNamedChild() bool {
	node := c.CurrentNode()
	if node == nil {
		return false
	}
	for i := nodeChildCountNoMaterialize(node) - 1; i >= 0; i-- {
		entry, ok := nodeChildEntryAtNoMaterialize(node, i)
		if !ok || !stackEntryNodeIsNamed(entry) {
			continue
		}
		child := nodeChildAtForReason(node, i, materializeForCursor)
		if child != nil {
			c.stack = append(c.stack, cursorFrame{node: child, childIndex: i})
			return true
		}
	}
	return false
}

// GotoNextNamedSibling moves the cursor to the next named sibling,
// skipping anonymous nodes. Returns false if no named sibling follows, or
// the cursor is nil.
func (c *TreeCursor) GotoNextNamedSibling() bool {
	if c == nil || len(c.stack) < 2 {
		return false
	}
	frame := &c.stack[len(c.stack)-1]
	parentNode := c.stack[len(c.stack)-2].node
	if parentNode == nil {
		return false
	}
	count := nodeChildCountNoMaterialize(parentNode)
	for i := frame.childIndex + 1; i < count; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(parentNode, i)
		if !ok || !stackEntryNodeIsNamed(entry) {
			continue
		}
		child := nodeChildAtForReason(parentNode, i, materializeForCursor)
		if child != nil {
			frame.childIndex = i
			frame.node = child
			return true
		}
	}
	return false
}

// GotoPrevNamedSibling moves the cursor to the previous named sibling,
// skipping anonymous nodes. Returns false if no named sibling precedes, or
// the cursor is nil.
func (c *TreeCursor) GotoPrevNamedSibling() bool {
	if c == nil || len(c.stack) < 2 {
		return false
	}
	frame := &c.stack[len(c.stack)-1]
	parentNode := c.stack[len(c.stack)-2].node
	if parentNode == nil {
		return false
	}
	for i := frame.childIndex - 1; i >= 0; i-- {
		entry, ok := nodeChildEntryAtNoMaterialize(parentNode, i)
		if !ok || !stackEntryNodeIsNamed(entry) {
			continue
		}
		child := nodeChildAtForReason(parentNode, i, materializeForCursor)
		if child != nil {
			frame.childIndex = i
			frame.node = child
			return true
		}
	}
	return false
}

func pointGreaterThan(a, b Point) bool {
	if a.Row != b.Row {
		return a.Row > b.Row
	}
	return a.Column > b.Column
}

// GotoFirstChildForByte moves the cursor to the first child whose byte range
// contains targetByte (i.e., first child where endByte > targetByte).
// Returns the child index, or -1 when no child contains the byte.
func (c *TreeCursor) GotoFirstChildForByte(targetByte uint32) int64 {
	node := c.CurrentNode()
	childCount := nodeChildCountNoMaterialize(node)
	if node == nil || childCount == 0 {
		return -1
	}
	i := sort.Search(childCount, func(i int) bool {
		entry, ok := nodeChildEntryAtNoMaterialize(node, i)
		return ok && stackEntryNodeEndByte(entry) > targetByte
	})
	if i >= childCount {
		return -1
	}
	c.stack = append(c.stack, cursorFrame{node: nodeChildAtForReason(node, i, materializeForCursor), childIndex: i})
	return int64(i)
}

// GotoFirstChildForPoint moves the cursor to the first child whose point range
// contains targetPoint (i.e., first child where endPoint > targetPoint).
// Returns the child index, or -1 when no child contains the point.
func (c *TreeCursor) GotoFirstChildForPoint(targetPoint Point) int64 {
	node := c.CurrentNode()
	childCount := nodeChildCountNoMaterialize(node)
	if node == nil || childCount == 0 {
		return -1
	}
	i := sort.Search(childCount, func(i int) bool {
		entry, ok := nodeChildEntryAtNoMaterialize(node, i)
		return ok && pointGreaterThan(stackEntryNodeEndPoint(entry), targetPoint)
	})
	if i >= childCount {
		return -1
	}
	c.stack = append(c.stack, cursorFrame{node: nodeChildAtForReason(node, i, materializeForCursor), childIndex: i})
	return int64(i)
}

// Reset resets the cursor to a new root node, clearing the navigation
// stack. Does nothing for a nil cursor.
func (c *TreeCursor) Reset(node *Node) {
	if c == nil {
		return
	}
	c.ensureStack()
	c.stack = c.stack[:1]
	c.stack[0] = cursorFrame{node: node, childIndex: -1}
}

// ResetTree resets the cursor to the root of a new tree. Does nothing for
// a nil cursor.
func (c *TreeCursor) ResetTree(tree *Tree) {
	if c == nil {
		return
	}
	c.tree = tree
	if tree == nil {
		c.Reset(nil)
		return
	}
	c.Reset(tree.RootNode())
}

// Copy returns an independent copy of the cursor. The copy shares the same
// tree reference but has its own navigation stack. Returns nil for a nil
// cursor.
func (c *TreeCursor) Copy() *TreeCursor {
	if c == nil {
		return nil
	}
	newStack := make([]cursorFrame, len(c.stack))
	copy(newStack, c.stack)
	return &TreeCursor{
		stack: newStack,
		tree:  c.tree,
	}
}

// CurrentNodeType returns the type name of the current node.
// Requires a tree with a language to be associated. Returns "" for a nil
// cursor.
func (c *TreeCursor) CurrentNodeType() string {
	if c == nil || c.tree == nil {
		return ""
	}
	node := c.CurrentNode()
	if node == nil {
		return ""
	}
	lang := c.tree.Language()
	if lang == nil {
		return ""
	}
	return node.Type(lang)
}

// CurrentNodeText returns the source text of the current node.
// Requires a tree with source to be associated. Returns "" for a nil
// cursor.
func (c *TreeCursor) CurrentNodeText() string {
	if c == nil || c.tree == nil {
		return ""
	}
	node := c.CurrentNode()
	if node == nil {
		return ""
	}
	return node.Text(c.tree.Source())
}

// CurrentNodeIsNamed returns whether the current node is a named node.
func (c *TreeCursor) CurrentNodeIsNamed() bool {
	node := c.CurrentNode()
	if node == nil {
		return false
	}
	return node.IsNamed()
}

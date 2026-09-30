package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/treeview"

// NodeView is an experimental, tree-bound occurrence of a syntax payload.
// Repeated navigation to an occurrence returns the same *NodeView. Different
// trees have different views even when incremental parsing shares their Node
// payloads. Parent and sibling relations always belong to the view's tree.
//
// A view borrows its tree's lifetime: keep a returned tree handle alive until
// navigation finishes. Views do not retain an extra handle. Tree.Edit changes
// payload coordinates; edit, parse, and release must not overlap navigation.
// See docs/tree-views.md for the identity, concurrency, and adapter contracts.
type NodeView struct {
	state *nodeViewState
	id    treeview.ID
}

type nodeViewState struct {
	tree  *Tree
	index treeview.Index[*Node, *NodeView]
}

func (s *nodeViewState) create(id treeview.ID) *NodeView {
	return &NodeView{state: s, id: id}
}

// RootNodeView returns the experimental tree-scoped root view. Navigation
// storage is created on first use, never during parsing. Nil and released
// trees return nil. An unchanged incremental parse preserves view identity.
func (t *Tree) RootNodeView() *NodeView {
	if t == nil || t.released {
		return nil
	}
	root := t.RootNode()
	if root == nil {
		return nil
	}
	sidecar := t.resultCompatibilityFinalizer.Load()
	if sidecar == nil {
		candidate := &treeResultCompatibilityFinalizer{viewOnly: true}
		if t.resultCompatibilityFinalizer.CompareAndSwap(nil, candidate) {
			sidecar = candidate
		} else {
			sidecar = t.resultCompatibilityFinalizer.Load()
		}
	}
	sidecar.viewOnce.Do(func() { sidecar.views = &nodeViewState{tree: t} })
	return sidecar.views.index.Root(root, sidecar.views.create)
}

// Node returns the shared payload for compatibility with APIs accepting
// *Node. The pointer does not carry tree context: use the view for parent and
// sibling navigation. After the tree's final Release, Node returns nil.
func (n *NodeView) Node() *Node {
	if n == nil || n.state == nil || n.state.tree.released {
		return nil
	}
	return n.state.index.Payload(n.id)
}

// Tree returns the tree to which the view belongs, or nil after final release.
func (n *NodeView) Tree() *Tree {
	if n == nil || n.state == nil || n.state.tree.released {
		return nil
	}
	return n.state.tree
}

// Parent returns this occurrence's tree-local parent, or nil at the root.
func (n *NodeView) Parent() *NodeView {
	if n.Tree() == nil {
		return nil
	}
	parent, _ := n.state.index.Parent(n.id)
	return parent
}

// nodeViewChild serializes legacy lazy-payload materialization across views
// from different trees borrowing the same arena. View navigation never reads
// the resulting shared parent link.
func nodeViewChild(parent *Node, i int) (*Node, int, bool) {
	if parent.ownerArena != nil {
		parent.ownerArena.parentLinkMu.Lock()
		defer parent.ownerArena.parentLinkMu.Unlock()
	}
	count := nodeChildCountNoMaterialize(parent)
	if i < 0 || i >= count {
		return nil, count, false
	}
	// Reused materialized children already have payload identity. Reading them
	// through the legacy materializer would overwrite a shared parent link.
	if entry, ok := nodeChildEntryAtNoMaterialize(parent, i); ok {
		if child := stackEntryNode(entry); child != nil {
			return child, count, true
		}
	}
	child := nodeChildAtForReason(parent, i, materializeForParentAPI)
	return child, count, child != nil
}

// Child returns the i-th tree-local child, creating its view lazily.
func (n *NodeView) Child(i int) *NodeView {
	if n.Tree() == nil {
		return nil
	}
	return n.state.index.Child(n.id, i, nodeViewChild, n.state.create)
}

// ChildCount returns the number of named and anonymous children.
func (n *NodeView) ChildCount() int { return n.Node().ChildCount() }

// Children returns the tree-local child views in source order.
func (n *NodeView) Children() []*NodeView {
	count := n.ChildCount()
	if count == 0 {
		return nil
	}
	children := make([]*NodeView, count)
	for i := range children {
		children[i] = n.Child(i)
	}
	return children
}

// NextSibling returns the next occurrence in this view's tree.
func (n *NodeView) NextSibling() *NodeView { return n.sibling(1, false) }

// PrevSibling returns the previous occurrence in this view's tree.
func (n *NodeView) PrevSibling() *NodeView { return n.sibling(-1, false) }

// NextNamedSibling returns the next named occurrence in this view's tree.
func (n *NodeView) NextNamedSibling() *NodeView { return n.sibling(1, true) }

// PrevNamedSibling returns the previous named occurrence in this view's tree.
func (n *NodeView) PrevNamedSibling() *NodeView { return n.sibling(-1, true) }

func (n *NodeView) sibling(step int, named bool) *NodeView {
	if n.Tree() == nil {
		return nil
	}
	parent, index := n.state.index.Parent(n.id)
	if parent == nil {
		return nil
	}
	count := parent.ChildCount()
	for i := index + step; i >= 0 && i < count; i += step {
		if !named || nodeViewChildNamed(parent.Node(), i) {
			return parent.Child(i)
		}
	}
	return nil
}

func nodeViewChildNamed(parent *Node, i int) bool {
	if parent.ownerArena != nil {
		parent.ownerArena.parentLinkMu.Lock()
		defer parent.ownerArena.parentLinkMu.Unlock()
	}
	entry, ok := nodeChildEntryAtNoMaterialize(parent, i)
	return ok && stackEntryNodeIsNamed(entry)
}

// NamedChildCount returns the number of named children without creating views.
func (n *NodeView) NamedChildCount() int {
	parent := n.Node()
	if parent == nil {
		return 0
	}
	if parent.ownerArena != nil {
		parent.ownerArena.parentLinkMu.Lock()
		defer parent.ownerArena.parentLinkMu.Unlock()
	}
	return parent.NamedChildCount()
}

// NamedChild returns the i-th named tree-local child.
func (n *NodeView) NamedChild(i int) *NodeView {
	if i < 0 || n.Tree() == nil {
		return nil
	}
	parent := n.Node()
	for child := 0; child < n.ChildCount(); child++ {
		if !nodeViewChildNamed(parent, child) {
			continue
		}
		if i == 0 {
			return n.Child(child)
		}
		i--
	}
	return nil
}

// ChildByFieldName returns the first tree-local child with the named field.
func (n *NodeView) ChildByFieldName(name string, lang *Language) *NodeView {
	if lang == nil {
		return nil
	}
	fid, ok := lang.FieldByName(name)
	if !ok || fid == 0 {
		return nil
	}
	for i := 0; i < n.ChildCount(); i++ {
		if nodeFieldIDAt(n.Node(), i) == fid {
			return n.Child(i)
		}
	}
	return nil
}

// FieldNameForChild returns the field name of the i-th child.
func (n *NodeView) FieldNameForChild(i int, lang *Language) string {
	return n.Node().FieldNameForChild(i, lang)
}

// Payload accessors preserve the existing Node API's values and signatures.
func (n *NodeView) Symbol() Symbol             { return n.Node().Symbol() }
func (n *NodeView) ParseState() StateID        { return n.Node().ParseState() }
func (n *NodeView) PreGotoState() StateID      { return n.Node().PreGotoState() }
func (n *NodeView) IsNamed() bool              { return n.Node().IsNamed() }
func (n *NodeView) IsExtra() bool              { return n.Node().IsExtra() }
func (n *NodeView) IsMissing() bool            { return n.Node().IsMissing() }
func (n *NodeView) IsError() bool              { return n.Node().IsError() }
func (n *NodeView) HasError() bool             { return n.Node().HasError() }
func (n *NodeView) HasChanges() bool           { return n.Node().HasChanges() }
func (n *NodeView) StartByte() uint32          { return n.Node().StartByte() }
func (n *NodeView) EndByte() uint32            { return n.Node().EndByte() }
func (n *NodeView) StartPoint() Point          { return n.Node().StartPoint() }
func (n *NodeView) EndPoint() Point            { return n.Node().EndPoint() }
func (n *NodeView) Range() Range               { return n.Node().Range() }
func (n *NodeView) Text(source []byte) string  { return n.Node().Text(source) }
func (n *NodeView) Type(lang *Language) string { return n.Node().Type(lang) }

// NodeViewForNode adapts a legacy payload, including a query result, to this
// tree's experimental view. It searches child edges, never parent links, and
// returns nil if the payload is absent. If a hand-built tree uses the same
// payload at multiple positions, it selects the first preorder occurrence.
// Prefer Child navigation or TreeCursor.CurrentNodeView when a path is known.
func (t *Tree) NodeViewForNode(node *Node) *NodeView {
	root := t.RootNodeView()
	if root == nil || node == nil {
		return nil
	}
	return root.viewForNode(node)
}

func (n *NodeView) viewForNode(target *Node) *NodeView {
	type frame struct {
		node  *Node
		next  int
		index int
	}
	stack := []frame{{node: n.Node(), index: -1}}
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		if top.node == target {
			view := n
			for _, f := range stack[1:] {
				view = view.Child(f.index)
			}
			return view
		}
		if top.next >= top.node.ChildCount() {
			stack = stack[:len(stack)-1]
			continue
		}
		i := top.next
		top.next++
		child, _, ok := nodeViewChild(top.node, i)
		if ok {
			stack = append(stack, frame{node: child, index: i})
		}
	}
	return nil
}

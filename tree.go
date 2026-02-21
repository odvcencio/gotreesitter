package gotreesitter

// Range is a span of source text.
type Range struct {
	StartByte  uint32
	EndByte    uint32
	StartPoint Point
	EndPoint   Point
}

// Node is a syntax tree node.
type Node struct {
	symbol       Symbol
	parseState   StateID // parser state after this node was pushed
	startByte    uint32
	endByte      uint32
	startPoint   Point
	endPoint     Point
	children     []*Node
	fieldIDs     []FieldID // parallel to children, 0 = no field
	isNamed      bool
	isExtra      bool
	isMissing    bool
	hasError     bool
	dirty        bool // set by Tree.Edit for nodes touched by edits
	productionID uint16
	parent       *Node
	ownerArena   *nodeArena
}

// Symbol returns the node's grammar symbol.
func (n *Node) Symbol() Symbol { return n.symbol }

// IsNamed reports whether this is a named node (as opposed to anonymous syntax like punctuation).
func (n *Node) IsNamed() bool { return n.isNamed }

// IsMissing reports whether this node was inserted by error recovery.
func (n *Node) IsMissing() bool { return n.isMissing }

// HasError reports whether this node or any descendant contains a parse error.
func (n *Node) HasError() bool { return n.hasError }

// StartByte returns the byte offset where this node begins.
func (n *Node) StartByte() uint32 { return n.startByte }

// EndByte returns the byte offset where this node ends (exclusive).
func (n *Node) EndByte() uint32 { return n.endByte }

// StartPoint returns the row/column position where this node begins.
func (n *Node) StartPoint() Point { return n.startPoint }

// EndPoint returns the row/column position where this node ends.
func (n *Node) EndPoint() Point { return n.endPoint }

// Range returns the full span of this node as a Range.
func (n *Node) Range() Range {
	return Range{
		StartByte:  n.startByte,
		EndByte:    n.endByte,
		StartPoint: n.startPoint,
		EndPoint:   n.endPoint,
	}
}

// Parent returns this node's parent, or nil if it is the root.
func (n *Node) Parent() *Node { return n.parent }

// ChildCount returns the number of children (both named and anonymous).
func (n *Node) ChildCount() int { return len(n.children) }

// Child returns the i-th child, or nil if i is out of range.
func (n *Node) Child(i int) *Node {
	if i < 0 || i >= len(n.children) {
		return nil
	}
	return n.children[i]
}

// NextSibling returns the next sibling node, or nil when this is the last child
// or has no parent.
func (n *Node) NextSibling() *Node {
	if n == nil || n.parent == nil {
		return nil
	}
	siblings := n.parent.children
	for i := range siblings {
		if siblings[i] != n {
			continue
		}
		if i+1 < len(siblings) {
			return siblings[i+1]
		}
		return nil
	}
	return nil
}

// PrevSibling returns the previous sibling node, or nil when this is the first
// child or has no parent.
func (n *Node) PrevSibling() *Node {
	if n == nil || n.parent == nil {
		return nil
	}
	siblings := n.parent.children
	for i := range siblings {
		if siblings[i] != n {
			continue
		}
		if i > 0 {
			return siblings[i-1]
		}
		return nil
	}
	return nil
}

// NamedChildCount returns the number of named children.
func (n *Node) NamedChildCount() int {
	count := 0
	for _, c := range n.children {
		if c.isNamed {
			count++
		}
	}
	return count
}

// NamedChild returns the i-th named child (skipping anonymous children),
// or nil if i is out of range.
func (n *Node) NamedChild(i int) *Node {
	count := 0
	for _, c := range n.children {
		if c.isNamed {
			if count == i {
				return c
			}
			count++
		}
	}
	return nil
}

// ChildByFieldName returns the first child assigned to the given field name,
// or nil if no child has that field. The Language is needed to resolve field
// names to IDs. Uses Language.FieldByName for O(1) lookup.
func (n *Node) ChildByFieldName(name string, lang *Language) *Node {
	fid, ok := lang.FieldByName(name)
	if !ok || fid == 0 {
		return nil
	}

	for i, id := range n.fieldIDs {
		if id == fid && i < len(n.children) {
			return n.children[i]
		}
	}
	return nil
}

// Children returns a slice of all children.
func (n *Node) Children() []*Node { return n.children }

// Text returns the source text covered by this node.
func (n *Node) Text(source []byte) string {
	return string(source[n.startByte:n.endByte])
}

// Type returns the node's type name from the language.
func (n *Node) Type(lang *Language) string {
	if int(n.symbol) < len(lang.SymbolNames) {
		return lang.SymbolNames[n.symbol]
	}
	return ""
}

// NewLeafNode creates a terminal/leaf node.
func NewLeafNode(sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *Node {
	return &Node{
		symbol:     sym,
		isNamed:    named,
		startByte:  startByte,
		endByte:    endByte,
		startPoint: startPoint,
		endPoint:   endPoint,
	}
}

// NewParentNode creates a non-terminal node with children.
// It sets parent pointers on all children and computes byte/point spans
// from the first and last children. If any child has an error, the parent
// is marked as having an error too.
func NewParentNode(sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	n := &Node{
		symbol:       sym,
		isNamed:      named,
		children:     children,
		fieldIDs:     fieldIDs,
		productionID: productionID,
	}

	if len(children) > 0 {
		first := children[0]
		last := children[len(children)-1]
		n.startByte = first.startByte
		n.endByte = last.endByte
		n.startPoint = first.startPoint
		n.endPoint = last.endPoint

		for _, c := range children {
			c.parent = n
			if c.hasError {
				n.hasError = true
			}
		}
	}

	return n
}

func symbolVisible(lang *Language, sym Symbol) bool {
	if lang == nil || int(sym) >= len(lang.SymbolMetadata) {
		return true
	}
	return lang.SymbolMetadata[sym].Visible
}

func newLeafNodeInArena(arena *nodeArena, sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *Node {
	if arena == nil {
		return NewLeafNode(sym, named, startByte, endByte, startPoint, endPoint)
	}
	n := arena.allocNode()
	n.symbol = sym
	n.isNamed = named
	n.startByte = startByte
	n.endByte = endByte
	n.startPoint = startPoint
	n.endPoint = endPoint
	n.ownerArena = arena
	return n
}

func newParentNodeInArena(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	if arena == nil {
		return NewParentNode(sym, named, children, fieldIDs, productionID)
	}
	n := arena.allocNode()
	n.symbol = sym
	n.isNamed = named
	n.children = children
	n.fieldIDs = fieldIDs
	n.productionID = productionID
	n.ownerArena = arena

	if len(children) > 0 {
		first := children[0]
		last := children[len(children)-1]
		n.startByte = first.startByte
		n.endByte = last.endByte
		n.startPoint = first.startPoint
		n.endPoint = last.endPoint

		for _, c := range children {
			c.parent = n
			if c.hasError {
				n.hasError = true
			}
		}
	}

	return n
}

// Tree holds a complete syntax tree along with its source text and language.
type Tree struct {
	root          *Node
	source        []byte
	language      *Language
	edits         []InputEdit  // pending edits applied to this tree
	arena         *nodeArena   // primary arena that owns newly-built nodes
	borrowedArena []*nodeArena // arenas borrowed via subtree reuse
	released      bool
}

// NewTree creates a new Tree.
func NewTree(root *Node, source []byte, lang *Language) *Tree {
	return &Tree{
		root:     root,
		source:   source,
		language: lang,
	}
}

func newTreeWithArenas(root *Node, source []byte, lang *Language, arena *nodeArena, borrowed []*nodeArena) *Tree {
	return &Tree{
		root:          root,
		source:        source,
		language:      lang,
		arena:         arena,
		borrowedArena: uniqueArenas(borrowed, arena),
	}
}

func uniqueArenas(arenas []*nodeArena, exclude *nodeArena) []*nodeArena {
	if len(arenas) == 0 {
		return nil
	}
	seen := make(map[*nodeArena]struct{}, len(arenas))
	out := make([]*nodeArena, 0, len(arenas))
	if exclude != nil {
		seen[exclude] = struct{}{}
	}
	for _, a := range arenas {
		if a == nil {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (t *Tree) referencedArenas() []*nodeArena {
	if t == nil {
		return nil
	}
	refs := make([]*nodeArena, 0, 1+len(t.borrowedArena))
	if t.arena != nil {
		refs = append(refs, t.arena)
	}
	refs = append(refs, t.borrowedArena...)
	return uniqueArenas(refs, nil)
}

// Release decrements arena references held by this tree.
// After Release, the tree should be treated as invalid and not reused.
func (t *Tree) Release() {
	if t == nil || t.released {
		return
	}
	t.released = true
	for _, a := range t.borrowedArena {
		a.Release()
	}
	t.borrowedArena = nil
	if t.arena != nil {
		t.arena.Release()
		t.arena = nil
	}
}

// RootNode returns the tree's root node.
func (t *Tree) RootNode() *Node { return t.root }

// Source returns the original source text.
func (t *Tree) Source() []byte { return t.source }

// Language returns the language used to parse this tree.
func (t *Tree) Language() *Language { return t.language }

// InputEdit describes a single edit to the source text. It tells the parser
// what byte range was replaced and what the new range looks like, so the
// incremental parser can skip unchanged subtrees.
type InputEdit struct {
	StartByte   uint32
	OldEndByte  uint32
	NewEndByte  uint32
	StartPoint  Point
	OldEndPoint Point
	NewEndPoint Point
}

// Edit records an edit on this tree. Call this before ParseIncremental to
// inform the parser which regions changed. The edit adjusts byte offsets
// and marks overlapping nodes as dirty so the incremental parser knows
// what to re-parse.
func (t *Tree) Edit(edit InputEdit) {
	t.edits = append(t.edits, edit)
	if t.root != nil {
		editNode(t.root, edit)
	}
}

// Edits returns the pending edits recorded on this tree.
func (t *Tree) Edits() []InputEdit { return t.edits }

// editNode recursively adjusts a node's byte/point spans for an edit and
// marks nodes that overlap the edited region as dirty.
func editNode(n *Node, edit InputEdit) {
	byteDelta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
	hasTailShift := byteDelta != 0 || edit.NewEndPoint != edit.OldEndPoint
	editNodeWithDelta(n, edit, byteDelta, hasTailShift)
}

func addUint32Delta(value uint32, delta int64) uint32 {
	next := int64(value) + delta
	if next < 0 {
		return 0
	}
	if next > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(next)
}

func editNodeWithDelta(n *Node, edit InputEdit, byteDelta int64, hasTailShift bool) {
	// If the node ends before the edit starts, it's completely unaffected.
	if n.endByte <= edit.StartByte {
		return
	}

	// If the node starts after the old edit end, shift its offsets.
	if n.startByte >= edit.OldEndByte {
		if !hasTailShift {
			return
		}
		n.startByte = addUint32Delta(n.startByte, byteDelta)
		n.endByte = addUint32Delta(n.endByte, byteDelta)
		// Shift points approximately (row stays, col shifts if same row).
		if n.startPoint.Row == edit.OldEndPoint.Row {
			rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
			n.startPoint.Row = addUint32Delta(n.startPoint.Row, rowDelta)
			if rowDelta == 0 {
				colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
				n.startPoint.Column = addUint32Delta(n.startPoint.Column, colDelta)
			}
		}
		if n.endPoint.Row == edit.OldEndPoint.Row {
			rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
			n.endPoint.Row = addUint32Delta(n.endPoint.Row, rowDelta)
			if rowDelta == 0 {
				colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
				n.endPoint.Column = addUint32Delta(n.endPoint.Column, colDelta)
			}
		}
		shiftSubtreeAfterEdit(n.children, edit, byteDelta)
		return
	}

	// The node overlaps the edit — mark it dirty and adjust its end.
	n.dirty = true
	if n.endByte <= edit.OldEndByte {
		// Node is fully within the edited region.
		n.endByte = edit.NewEndByte
		n.endPoint = edit.NewEndPoint
	} else {
		// Node extends past the edit — adjust end.
		n.endByte = addUint32Delta(n.endByte, byteDelta)
	}

	// Recurse only into children that can be affected.
	for _, c := range n.children {
		if c.endByte <= edit.StartByte {
			continue
		}
		if c.startByte >= edit.OldEndByte {
			if !hasTailShift {
				continue
			}
			shiftSubtreeAfterEdit([]*Node{c}, edit, byteDelta)
			continue
		}
		editNodeWithDelta(c, edit, byteDelta, hasTailShift)
	}
}

func shiftSubtreeAfterEdit(roots []*Node, edit InputEdit, byteDelta int64) {
	if len(roots) == 0 {
		return
	}

	stack := make([]*Node, 0, len(roots)*2)
	stack = append(stack, roots...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		n.startByte = addUint32Delta(n.startByte, byteDelta)
		n.endByte = addUint32Delta(n.endByte, byteDelta)

		if n.startPoint.Row == edit.OldEndPoint.Row {
			rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
			n.startPoint.Row = addUint32Delta(n.startPoint.Row, rowDelta)
			if rowDelta == 0 {
				colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
				n.startPoint.Column = addUint32Delta(n.startPoint.Column, colDelta)
			}
		}
		if n.endPoint.Row == edit.OldEndPoint.Row {
			rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
			n.endPoint.Row = addUint32Delta(n.endPoint.Row, rowDelta)
			if rowDelta == 0 {
				colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
				n.endPoint.Column = addUint32Delta(n.endPoint.Column, colDelta)
			}
		}

		for _, c := range n.children {
			stack = append(stack, c)
		}
	}
}

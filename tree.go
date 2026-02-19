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
	startByte    uint32
	endByte      uint32
	startPoint   Point
	endPoint     Point
	children     []*Node
	fieldIDs     []FieldID // parallel to children, 0 = no field
	isNamed      bool
	isMissing    bool
	hasError     bool
	productionID uint16
	parent       *Node
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
// names to IDs.
func (n *Node) ChildByFieldName(name string, lang *Language) *Node {
	// Find the field ID for this name.
	fid := FieldID(0)
	for i, fn := range lang.FieldNames {
		if fn == name {
			fid = FieldID(i)
			break
		}
	}
	if fid == 0 {
		return nil
	}

	// Search children for the first one with this field ID.
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

// Tree holds a complete syntax tree along with its source text and language.
type Tree struct {
	root     *Node
	source   []byte
	language *Language
}

// NewTree creates a new Tree.
func NewTree(root *Node, source []byte, lang *Language) *Tree {
	return &Tree{
		root:     root,
		source:   source,
		language: lang,
	}
}

// RootNode returns the tree's root node.
func (t *Tree) RootNode() *Node { return t.root }

// Source returns the original source text.
func (t *Tree) Source() []byte { return t.source }

// Language returns the language used to parse this tree.
func (t *Tree) Language() *Language { return t.language }

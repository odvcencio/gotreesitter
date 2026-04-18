package gotreesitter

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

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
	preGotoState StateID // parser state before goto (state exposed after popping children)
	startByte    uint32
	endByte      uint32
	startPoint   Point
	endPoint     Point
	children     []*Node
	fieldIDs     []FieldID // parallel to children, 0 = no field
	fieldSources []uint8   // parallel to children, 0 = none, 1 = direct, 2 = inherited
	isNamed      bool
	isExtra      bool
	isMissing    bool
	hasError     bool
	dirty        bool // set by Tree.Edit for nodes touched by edits
	productionID uint16
	equivVersion uint32
	parent       *Node
	childIndex   int
	ownerArena   *nodeArena
}

func nodeInitEquivVersion(n *Node) {
	if n == nil {
		return
	}
	n.equivVersion = 1
}

func nodeBumpEquivVersion(n *Node) {
	if n == nil {
		return
	}
	n.equivVersion++
	if n.equivVersion == 0 {
		n.equivVersion = 1
	}
}

func defaultFieldSourcesInArena(arena *nodeArena, fieldIDs []FieldID) []uint8 {
	if len(fieldIDs) == 0 {
		return nil
	}
	var out []uint8
	if arena != nil {
		out = arena.allocFieldSourceSlice(len(fieldIDs))
	} else {
		out = make([]uint8, len(fieldIDs))
	}
	for i, fid := range fieldIDs {
		if fid != 0 {
			out[i] = fieldSourceDirect
		}
	}
	return out
}

// ParseStopReason reports why parseInternal terminated.
type ParseStopReason string

const (
	ParseStopNone            ParseStopReason = "none"
	ParseStopAccepted        ParseStopReason = "accepted"
	ParseStopNoStacksAlive   ParseStopReason = "no_stacks_alive"
	ParseStopTokenSourceEOF  ParseStopReason = "token_source_eof"
	ParseStopTimeout         ParseStopReason = "timeout"
	ParseStopCancelled       ParseStopReason = "cancelled"
	ParseStopIterationLimit  ParseStopReason = "iteration_limit"
	ParseStopStackDepthLimit ParseStopReason = "stack_depth_limit"
	ParseStopNodeLimit       ParseStopReason = "node_limit"
	ParseStopMemoryBudget    ParseStopReason = "memory_budget"
)

// ParseRuntime captures parser-loop diagnostics for a completed tree.
type ParseRuntime struct {
	StopReason                  ParseStopReason
	SourceLen                   uint32
	ExpectedEOFByte             uint32
	RootEndByte                 uint32
	Truncated                   bool
	TokenSourceEOFEarly         bool
	TokensConsumed              uint64
	LastTokenEndByte            uint32
	LastTokenSymbol             Symbol
	LastTokenWasEOF             bool
	IterationLimit              int
	StackDepthLimit             int
	NodeLimit                   int
	MemoryBudgetBytes           int64
	Iterations                  int
	NodesAllocated              int
	ArenaBytesAllocated         int64
	ScratchBytesAllocated       int64
	EntryScratchBytesAllocated  int64
	GSSBytesAllocated           int64
	PeakStackDepth              int
	MaxStacksSeen               int
	SingleStackIterations       int
	MultiStackIterations        int
	SingleStackTokens           uint64
	MultiStackTokens            uint64
	SingleStackGSSNodes         uint64
	MultiStackGSSNodes          uint64
	GSSNodesAllocated           uint64
	GSSNodesRetained            uint64
	GSSNodesDroppedSameToken    uint64
	ParentNodesAllocated        uint64
	ParentNodesRetained         uint64
	ParentNodesDroppedSameToken uint64
	LeafNodesAllocated          uint64
	LeafNodesRetained           uint64
	LeafNodesDroppedSameToken   uint64
	MergeStacksIn               uint64
	MergeStacksOut              uint64
	MergeSlotsUsed              uint64
	GlobalCullStacksIn          uint64
	GlobalCullStacksOut         uint64
}

// Summary returns a stable one-line diagnostic string for parse-runtime stats.
func (rt ParseRuntime) Summary() string {
	stopReason := rt.StopReason
	if stopReason == "" {
		stopReason = ParseStopNone
	}
	return fmt.Sprintf(
		"truncated=%v stopReason=%s tokenEOFEarly=%v tokens=%d lastTokenEnd=%d expectedEOF=%d lastTokenSymbol=%d lastTokenEOF=%v iterations=%d/%d nodes=%d/%d arena=%d/%d scratch=%d(entry=%d gss=%d)/%d peakDepth=%d/%d maxStacks=%d",
		rt.Truncated, stopReason, rt.TokenSourceEOFEarly, rt.TokensConsumed,
		rt.LastTokenEndByte, rt.ExpectedEOFByte, rt.LastTokenSymbol, rt.LastTokenWasEOF,
		rt.Iterations, rt.IterationLimit, rt.NodesAllocated, rt.NodeLimit,
		rt.ArenaBytesAllocated, rt.MemoryBudgetBytes,
		rt.ScratchBytesAllocated, rt.EntryScratchBytesAllocated, rt.GSSBytesAllocated, rt.MemoryBudgetBytes,
		rt.PeakStackDepth, rt.StackDepthLimit, rt.MaxStacksSeen,
	)
}

// Symbol returns the node's grammar symbol.
func (n *Node) Symbol() Symbol { return n.symbol }

// ParseState returns the parser state associated with this node.
func (n *Node) ParseState() StateID { return n.parseState }

// PreGotoState returns the parser state that was on top of the stack before
// this node was pushed (i.e., the state exposed after popping children during
// reduce). For non-leaf nodes: lookupGoto(PreGotoState, Symbol) == ParseState.
func (n *Node) PreGotoState() StateID { return n.preGotoState }

// IsNamed reports whether this is a named node (as opposed to anonymous syntax like punctuation).
func (n *Node) IsNamed() bool { return n.isNamed }

// IsExtra reports whether this node was marked as extra syntax
// (e.g. whitespace/comments outside the core parse structure).
func (n *Node) IsExtra() bool { return n.isExtra }

// IsMissing reports whether this node was inserted by error recovery.
func (n *Node) IsMissing() bool { return n.isMissing }

// IsError reports whether this node is an explicit error node.
func (n *Node) IsError() bool { return n.symbol == errorSymbol }

// HasError reports whether this node or any descendant contains a parse error.
func (n *Node) HasError() bool { return n.hasError }

// HasChanges reports whether this node was marked dirty by Tree.Edit.
func (n *Node) HasChanges() bool { return n.dirty }

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
	if i := n.childIndex; i >= 0 && i < len(siblings) && siblings[i] == n {
		if i+1 < len(siblings) {
			return siblings[i+1]
		}
		return nil
	}
	for i, s := range siblings {
		if s != n {
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
	if i := n.childIndex; i >= 0 && i < len(siblings) && siblings[i] == n {
		if i > 0 {
			return siblings[i-1]
		}
		return nil
	}
	for i, s := range siblings {
		if s != n {
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

// FieldNameForChild returns the field name assigned to the i-th child,
// or an empty string when no field is assigned.
func (n *Node) FieldNameForChild(i int, lang *Language) string {
	if n == nil || lang == nil || i < 0 || i >= len(n.children) || i >= len(n.fieldIDs) {
		return ""
	}
	fid := n.fieldIDs[i]
	if fid == 0 || int(fid) >= len(lang.FieldNames) {
		return ""
	}
	return lang.FieldNames[fid]
}

// Children returns a slice of all children.
func (n *Node) Children() []*Node { return n.children }

// SExpr returns a tree-sitter-style S-expression for this node.
// It includes only named nodes for stable debug snapshots.
func (n *Node) SExpr(lang *Language) string {
	if n == nil || lang == nil {
		return ""
	}
	if !n.IsNamed() {
		return ""
	}
	var b strings.Builder
	// S-expressions are typically ~5x the source byte count for named nodes.
	// Pre-growing the builder avoids intermediate reallocations.
	b.Grow((int(n.endByte-n.startByte) * 5) + 32)
	sexprWrite(n, lang, &b)
	return b.String()
}

// sexprWrite writes the S-expression for n into b, returning true if anything
// was written. Using a shared builder avoids the per-node string allocation and
// the intermediate []string slice that the previous implementation required.
func sexprWrite(n *Node, lang *Language, b *strings.Builder) {
	if n == nil || !n.IsNamed() {
		return
	}
	name := n.Type(lang)
	b.WriteByte('(')
	b.WriteString(name)

	// Walk children, writing only named ones. Because a named child always
	// produces at least "(type)", we can write a space before each one eagerly.
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child != nil && child.IsNamed() {
			b.WriteByte(' ')
			sexprWrite(child, lang, b)
		}
	}

	b.WriteByte(')')
}

// Text returns the source text covered by this node.
// Returns an empty string for nil nodes or invalid byte ranges.
func (n *Node) Text(source []byte) string {
	if n == nil {
		return ""
	}
	start := int(n.startByte)
	end := int(n.endByte)
	if end < start || start > len(source) || end > len(source) {
		return ""
	}
	return string(source[start:end])
}

// Type returns the node's type name from the language.
func (n *Node) Type(lang *Language) string {
	if n != nil && n.symbol == errorSymbol {
		return "ERROR"
	}
	if int(n.symbol) < len(lang.SymbolNames) {
		name := lang.SymbolNames[n.symbol]
		name = unescapePunctuationSymbolName(name)
		return name
	}
	return ""
}

func unescapePunctuationSymbolName(name string) string {
	if !strings.Contains(name, "\\") {
		return name
	}
	var b strings.Builder
	b.Grow(len(name))
	changed := false
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r != '\\' {
			b.WriteRune(r)
			i += size
			continue
		}
		if i+size >= len(name) {
			b.WriteRune(r)
			i += size
			continue
		}
		next, nextSize := utf8.DecodeRuneInString(name[i+size:])
		if next == '\\' || unicode.IsLetter(next) || unicode.IsDigit(next) {
			b.WriteRune(r)
			i += size
			continue
		}
		changed = true
		b.WriteRune(next)
		i += size + nextSize
	}
	if !changed {
		return name
	}
	return b.String()
}

func pointLessThan(a, b Point) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Column < b.Column
}

func pointLessOrEqual(a, b Point) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Column <= b.Column
}

func (n *Node) containsByteRange(startByte, endByte uint32) bool {
	return startByte >= n.startByte && endByte <= n.endByte
}

func (n *Node) containsPointRange(startPoint, endPoint Point) bool {
	return pointLessOrEqual(n.startPoint, startPoint) && pointLessOrEqual(endPoint, n.endPoint)
}

func (n *Node) descendantForByteRange(startByte, endByte uint32, namedOnly bool) *Node {
	if n == nil || endByte < startByte || !n.containsByteRange(startByte, endByte) {
		return nil
	}

	var deepest *Node
	if !namedOnly || n.isNamed {
		deepest = n
	}
	for _, child := range n.children {
		if !child.containsByteRange(startByte, endByte) {
			continue
		}
		if d := child.descendantForByteRange(startByte, endByte, namedOnly); d != nil {
			deepest = d
		}
	}
	return deepest
}

func (n *Node) descendantForPointRange(startPoint, endPoint Point, namedOnly bool) *Node {
	if n == nil || pointLessThan(endPoint, startPoint) || !n.containsPointRange(startPoint, endPoint) {
		return nil
	}

	var deepest *Node
	if !namedOnly || n.isNamed {
		deepest = n
	}
	for _, child := range n.children {
		if !child.containsPointRange(startPoint, endPoint) {
			continue
		}
		if d := child.descendantForPointRange(startPoint, endPoint, namedOnly); d != nil {
			deepest = d
		}
	}
	return deepest
}

// DescendantForByteRange returns the smallest descendant that fully contains
// the given byte range, or nil when no such descendant exists.
func (n *Node) DescendantForByteRange(startByte, endByte uint32) *Node {
	return n.descendantForByteRange(startByte, endByte, false)
}

// NamedDescendantForByteRange returns the smallest named descendant that fully
// contains the given byte range, or nil when no such descendant exists.
func (n *Node) NamedDescendantForByteRange(startByte, endByte uint32) *Node {
	return n.descendantForByteRange(startByte, endByte, true)
}

// DescendantForPointRange returns the smallest descendant that fully contains
// the given point range, or nil when no such descendant exists.
func (n *Node) DescendantForPointRange(startPoint, endPoint Point) *Node {
	return n.descendantForPointRange(startPoint, endPoint, false)
}

// NamedDescendantForPointRange returns the smallest named descendant that
// fully contains the given point range, or nil when no such descendant exists.
func (n *Node) NamedDescendantForPointRange(startPoint, endPoint Point) *Node {
	return n.descendantForPointRange(startPoint, endPoint, true)
}

// NewLeafNode creates a terminal/leaf node.
func NewLeafNode(sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *Node {
	n := &Node{
		symbol:     sym,
		isNamed:    named,
		startByte:  startByte,
		endByte:    endByte,
		startPoint: startPoint,
		endPoint:   endPoint,
		childIndex: -1,
	}
	nodeInitEquivVersion(n)
	return n
}

func populateParentNode(n *Node, children []*Node) {
	switch len(children) {
	case 0:
		return
	case 1:
		c0 := children[0]
		n.startByte = c0.startByte
		n.endByte = c0.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c0.endPoint
		c0.parent = n
		c0.childIndex = 0
		n.hasError = c0.hasError
		return
	case 2:
		c0 := children[0]
		c1 := children[1]
		n.startByte = c0.startByte
		n.endByte = c1.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c1.endPoint
		c0.parent = n
		c0.childIndex = 0
		c1.parent = n
		c1.childIndex = 1
		n.hasError = c0.hasError || c1.hasError
		return
	default:
		first := children[0]
		last := children[len(children)-1]
		n.startByte = first.startByte
		n.endByte = last.endByte
		n.startPoint = first.startPoint
		n.endPoint = last.endPoint

		for i, c := range children {
			c.parent = n
			c.childIndex = i
			if c.hasError {
				n.hasError = true
				break
			}
		}
	}
}

// populateParentNodeNoLinks computes parent span/error metadata from children
// without wiring child.parent/childIndex links. Used on deferred-link paths.
func populateParentNodeNoLinks(n *Node, children []*Node, trackChildErrors bool) {
	switch len(children) {
	case 0:
		return
	case 1:
		c0 := children[0]
		n.startByte = c0.startByte
		n.endByte = c0.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c0.endPoint
		if trackChildErrors {
			n.hasError = c0.hasError
		}
		return
	case 2:
		c0 := children[0]
		c1 := children[1]
		n.startByte = c0.startByte
		n.endByte = c1.endByte
		n.startPoint = c0.startPoint
		n.endPoint = c1.endPoint
		if trackChildErrors {
			n.hasError = c0.hasError || c1.hasError
		}
		return
	default:
		first := children[0]
		last := children[len(children)-1]
		n.startByte = first.startByte
		n.endByte = last.endByte
		n.startPoint = first.startPoint
		n.endPoint = last.endPoint
		if trackChildErrors {
			for i := range children {
				if children[i].hasError {
					n.hasError = true
					break
				}
			}
		}
	}
}

func wireParentLinksWithScratch(root *Node, scratch *[]*Node) {
	if root == nil {
		return
	}
	root.parent = nil
	root.childIndex = -1

	var stack []*Node
	if scratch != nil {
		stack = (*scratch)[:0]
	} else {
		var local [64]*Node
		stack = local[:0]
	}
	stack = append(stack, root)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for i := range n.children {
			c := n.children[i]
			if c == nil {
				continue
			}
			c.parent = n
			c.childIndex = i
			stack = append(stack, c)
		}
	}
	if scratch != nil {
		*scratch = stack[:0]
	}
}

func newParentNode(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	var n *Node
	if arena == nil {
		n = &Node{}
	} else {
		n = arena.allocNode()
		n.ownerArena = arena
	}
	n.symbol = sym
	n.isNamed = named
	n.children = children
	n.fieldIDs = fieldIDs
	n.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	n.productionID = productionID
	n.childIndex = -1
	populateParentNode(n, children)
	nodeInitEquivVersion(n)
	return n
}

// NewParentNode creates a non-terminal node with children.
// It sets parent pointers on all children and computes byte/point spans
// from the first and last children. If any child has an error, the parent
// is marked as having an error too.
func NewParentNode(sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	return newParentNode(nil, sym, named, children, fieldIDs, productionID)
}

func newLeafNodeInArena(arena *nodeArena, sym Symbol, named bool, startByte, endByte uint32, startPoint, endPoint Point) *Node {
	if arena == nil {
		return &Node{
			symbol:     sym,
			isNamed:    named,
			startByte:  startByte,
			endByte:    endByte,
			startPoint: startPoint,
			endPoint:   endPoint,
			childIndex: -1,
		}
	}
	n := arena.allocNodeFast()
	n.symbol = sym
	n.isNamed = named
	n.startByte = startByte
	n.endByte = endByte
	n.startPoint = startPoint
	n.endPoint = endPoint
	n.childIndex = -1
	n.ownerArena = arena
	nodeInitEquivVersion(n)
	if arena.audit != nil {
		arena.audit.recordNodeAlloc(n, runtimeAuditNodeKindLeaf)
	}
	return n
}

func newParentNodeInArena(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, productionID uint16) *Node {
	return newParentNodeInArenaWithFieldSources(arena, sym, named, children, fieldIDs, nil, productionID)
}

func newParentNodeInArenaWithFieldSources(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, fieldSources []uint8, productionID uint16) *Node {
	if arena == nil {
		return newParentNode(nil, sym, named, children, fieldIDs, productionID)
	}
	if perfCountersEnabled {
		perfRecordParentChildren(len(children))
	}
	n := arena.allocNodeFast()
	n.ownerArena = arena
	n.symbol = sym
	n.isNamed = named
	n.children = children
	n.fieldIDs = fieldIDs
	if fieldSources != nil {
		n.fieldSources = fieldSources
	} else {
		n.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	}
	n.productionID = productionID
	n.childIndex = -1
	populateParentNode(n, children)
	nodeInitEquivVersion(n)
	if arena.audit != nil {
		arena.audit.recordNodeAlloc(n, runtimeAuditNodeKindParent)
	}
	return n
}

func newParentNodeInArenaNoLinksWithFieldSources(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID, fieldSources []uint8, productionID uint16, trackChildErrors bool) *Node {
	if arena == nil {
		return newParentNode(nil, sym, named, children, fieldIDs, productionID)
	}
	if perfCountersEnabled {
		perfRecordParentChildren(len(children))
	}
	n := arena.allocNodeFast()
	n.ownerArena = arena
	n.symbol = sym
	n.isNamed = named
	n.children = children
	n.fieldIDs = fieldIDs
	if fieldSources != nil {
		n.fieldSources = fieldSources
	} else {
		n.fieldSources = defaultFieldSourcesInArena(arena, fieldIDs)
	}
	n.productionID = productionID
	n.childIndex = -1
	populateParentNodeNoLinks(n, children, trackChildErrors)
	nodeInitEquivVersion(n)
	if arena.audit != nil {
		arena.audit.recordNodeAlloc(n, runtimeAuditNodeKindParent)
	}
	return n
}

// Tree holds a complete syntax tree along with its source text and language.
// Tree is safe for concurrent reads after construction. Edit and Release are
// not safe for concurrent use.
type Tree struct {
	root           *Node
	source         []byte
	language       *Language
	edits          []InputEdit  // pending edits applied to this tree
	lastEditedLeaf *Node        // deepest leaf overlapped by the most recent edit, when tracked
	arena          *nodeArena   // primary arena that owns newly-built nodes
	borrowedArena  []*nodeArena // arenas borrowed via subtree reuse
	parseRuntime   ParseRuntime
	released       bool
}

var treePool = sync.Pool{
	New: func() any {
		return &Tree{}
	},
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
	tree := treePool.Get().(*Tree)
	*tree = Tree{
		root:          root,
		source:        source,
		language:      lang,
		arena:         arena,
		borrowedArena: uniqueArenas(borrowed, arena),
	}
	rebuildExternalScannerCheckpoints(root, lang)
	return tree
}

func uniqueArenas(arenas []*nodeArena, exclude *nodeArena) []*nodeArena {
	if len(arenas) == 0 {
		return nil
	}
	out := make([]*nodeArena, 0, len(arenas))
	for _, a := range arenas {
		if a == nil {
			continue
		}
		if a == exclude {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == a {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Release decrements arena references held by this tree.
// After Release, the tree should be treated as invalid and not reused.
func (t *Tree) Release() {
	if t == nil || t.released {
		return
	}
	t.released = true
	t.lastEditedLeaf = nil
	for _, a := range t.borrowedArena {
		a.Release()
	}
	if len(t.borrowedArena) > 0 {
		clear(t.borrowedArena)
		t.borrowedArena = t.borrowedArena[:0]
	}
	if t.arena != nil {
		t.arena.Release()
		t.arena = nil
	}
	t.root = nil
	t.source = nil
	t.language = nil
	t.edits = nil
	t.parseRuntime = ParseRuntime{}
	treePool.Put(t)
}

// RootNode returns the tree's root node.
func (t *Tree) RootNode() *Node { return t.root }

// RootNodeWithOffset returns a copy of the root node with all spans shifted by
// the provided byte and point offsets.
//
// This mirrors tree-sitter C's root-node-with-offset behavior for callers that
// need to embed a parsed tree at a larger document offset.
func (t *Tree) RootNodeWithOffset(offsetBytes uint32, offsetExtent Point) *Node {
	if t == nil || t.root == nil {
		return nil
	}
	if offsetBytes == 0 && offsetExtent == (Point{}) {
		return t.root
	}
	return cloneTreeNodesWithOffset(t.root, offsetBytes, offsetExtent)
}

// Source returns the original source text.
func (t *Tree) Source() []byte { return t.source }

// Language returns the language used to parse this tree.
func (t *Tree) Language() *Language { return t.language }

// WriteDOT writes a DOT graph representation of this tree to w.
func (t *Tree) WriteDOT(w io.Writer, lang *Language) error {
	if w == nil {
		return fmt.Errorf("tree: nil writer")
	}
	if t == nil || t.root == nil {
		_, err := io.WriteString(w, "digraph gotreesitter {\n}\n")
		return err
	}

	type dotItem struct {
		node *Node
		id   int
	}

	if _, err := io.WriteString(w, "digraph gotreesitter {\n"); err != nil {
		return err
	}

	nextID := 1
	stack := []dotItem{{node: t.root, id: 0}}
	for len(stack) > 0 {
		last := len(stack) - 1
		item := stack[last]
		stack = stack[:last]
		n := item.node
		if n == nil {
			continue
		}

		label := fmt.Sprintf("%s [%d,%d)", n.Type(lang), n.StartByte(), n.EndByte())
		if _, err := fmt.Fprintf(w, "  n%d [label=%q];\n", item.id, label); err != nil {
			return err
		}

		for _, child := range n.children {
			if child == nil {
				continue
			}
			childID := nextID
			nextID++
			if _, err := fmt.Fprintf(w, "  n%d -> n%d;\n", item.id, childID); err != nil {
				return err
			}
			stack = append(stack, dotItem{node: child, id: childID})
		}
	}

	_, err := io.WriteString(w, "}\n")
	return err
}

// DOT returns a DOT graph representation of this tree.
func (t *Tree) DOT(lang *Language) string {
	var b strings.Builder
	_ = t.WriteDOT(&b, lang)
	return b.String()
}

// Copy returns an independent copy of this tree.
//
// The copied tree has distinct node objects, so subsequent Tree.Edit calls on
// either tree do not mutate the other's spans/dirty bits. Source bytes and
// language pointer are shared (read-only).
func (t *Tree) Copy() *Tree {
	if t == nil {
		return nil
	}

	out := &Tree{
		source:       t.source,
		language:     t.language,
		parseRuntime: t.parseRuntime,
	}
	if len(t.edits) > 0 {
		out.edits = make([]InputEdit, len(t.edits))
		copy(out.edits, t.edits)
	}
	if t.root == nil {
		return out
	}

	class := arenaClassIncremental
	if t.arena != nil {
		class = t.arena.class
	}
	arena := acquireNodeArena(class)
	out.root = cloneTreeNodesIntoArena(t.root, arena)
	out.arena = arena
	return out
}

func cloneTreeNodesIntoArena(root *Node, arena *nodeArena) *Node {
	if root == nil {
		return nil
	}

	type clonePair struct {
		old *Node
		new *Node
	}

	cloneNode := func(src *Node) *Node {
		dst := arena.allocNodeFast()
		*dst = *src
		dst.children = nil
		dst.fieldIDs = nil
		dst.fieldSources = nil
		dst.parent = nil
		dst.childIndex = -1
		dst.ownerArena = arena
		return dst
	}

	newRoot := cloneNode(root)
	stack := []clonePair{{old: root, new: newRoot}}
	for len(stack) > 0 {
		last := len(stack) - 1
		pair := stack[last]
		stack = stack[:last]

		oldNode := pair.old
		newNode := pair.new

		if n := len(oldNode.fieldIDs); n > 0 {
			fieldIDs := arena.allocFieldIDSlice(n)
			copy(fieldIDs, oldNode.fieldIDs)
			newNode.fieldIDs = fieldIDs
		}
		if n := len(oldNode.fieldSources); n > 0 {
			fieldSources := arena.allocFieldSourceSlice(n)
			copy(fieldSources, oldNode.fieldSources)
			newNode.fieldSources = fieldSources
		}

		if n := len(oldNode.children); n > 0 {
			children := arena.allocNodeSlice(n)
			newNode.children = children
			for i, oldChild := range oldNode.children {
				if oldChild == nil {
					continue
				}
				newChild := cloneNode(oldChild)
				newChild.parent = newNode
				newChild.childIndex = i
				children[i] = newChild
				stack = append(stack, clonePair{old: oldChild, new: newChild})
			}
		}
	}

	return newRoot
}

func cloneTreeNodesWithOffset(root *Node, offsetBytes uint32, offsetExtent Point) *Node {
	if root == nil {
		return nil
	}

	type clonePair struct {
		old *Node
		new *Node
	}

	baseRow := root.startPoint.Row
	offsetPoint := func(p Point) Point {
		originalRow := p.Row
		p.Row = addUint32Delta(p.Row, int64(offsetExtent.Row))
		// When adding a multi-line prefix, only nodes on the original first row
		// of this tree receive the column offset. Rows after that keep columns.
		if offsetExtent.Row == 0 || originalRow == baseRow {
			p.Column = addUint32Delta(p.Column, int64(offsetExtent.Column))
		}
		return p
	}

	cloneNode := func(src *Node) *Node {
		dst := &Node{}
		*dst = *src
		dst.startByte = addUint32Delta(src.startByte, int64(offsetBytes))
		dst.endByte = addUint32Delta(src.endByte, int64(offsetBytes))
		dst.startPoint = offsetPoint(src.startPoint)
		dst.endPoint = offsetPoint(src.endPoint)
		dst.children = nil
		dst.fieldIDs = nil
		dst.fieldSources = nil
		dst.parent = nil
		dst.childIndex = -1
		dst.ownerArena = nil
		return dst
	}

	newRoot := cloneNode(root)
	stack := []clonePair{{old: root, new: newRoot}}
	for len(stack) > 0 {
		last := len(stack) - 1
		pair := stack[last]
		stack = stack[:last]

		oldNode := pair.old
		newNode := pair.new

		if n := len(oldNode.fieldIDs); n > 0 {
			newNode.fieldIDs = make([]FieldID, n)
			copy(newNode.fieldIDs, oldNode.fieldIDs)
		}
		if n := len(oldNode.fieldSources); n > 0 {
			newNode.fieldSources = make([]uint8, n)
			copy(newNode.fieldSources, oldNode.fieldSources)
		}

		if n := len(oldNode.children); n > 0 {
			newNode.children = make([]*Node, n)
			for i, oldChild := range oldNode.children {
				if oldChild == nil {
					continue
				}
				newChild := cloneNode(oldChild)
				newChild.parent = newNode
				newChild.childIndex = i
				newNode.children[i] = newChild
				stack = append(stack, clonePair{old: oldChild, new: newChild})
			}
		}
	}

	return newRoot
}

// ParseStopReason reports why parsing terminated.
func (t *Tree) ParseStopReason() ParseStopReason {
	if t == nil {
		return ParseStopNone
	}
	if t.parseRuntime.StopReason == "" {
		return ParseStopNone
	}
	return t.parseRuntime.StopReason
}

// ParseStoppedEarly reports whether parsing hit an early-stop condition.
func (t *Tree) ParseStoppedEarly() bool {
	switch t.ParseStopReason() {
	case ParseStopIterationLimit, ParseStopStackDepthLimit, ParseStopNodeLimit, ParseStopMemoryBudget, ParseStopTokenSourceEOF, ParseStopTimeout, ParseStopCancelled:
		return true
	default:
		return false
	}
}

// ParseRuntime returns parser-loop diagnostics captured when this tree was built.
func (t *Tree) ParseRuntime() ParseRuntime {
	if t == nil {
		return ParseRuntime{StopReason: ParseStopNone}
	}
	out := t.parseRuntime
	if out.StopReason == "" {
		out.StopReason = ParseStopNone
	}
	return out
}

func (t *Tree) setParseRuntime(rt ParseRuntime) {
	if t == nil {
		return
	}
	if rt.StopReason == "" {
		rt.StopReason = ParseStopNone
	}
	t.parseRuntime = rt
}

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

// Edit adjusts this node's byte/point span for a source edit.
//
// If the node belongs to a larger tree, the edit is applied from the
// containing root so sibling and ancestor spans remain consistent.
// Unlike Tree.Edit, this method does not record edit history on a Tree.
func (n *Node) Edit(edit InputEdit) {
	if n == nil {
		return
	}
	root := n
	for root.parent != nil {
		root = root.parent
	}
	editNode(root, edit)
}

// Edit records an edit on this tree. Call this before ParseIncremental to
// inform the parser which regions changed. The edit adjusts byte offsets
// and marks overlapping nodes as dirty so the incremental parser knows
// what to re-parse.
func (t *Tree) Edit(edit InputEdit) {
	t.edits = append(t.edits, edit)
	t.lastEditedLeaf = nil
	if t.root != nil {
		byteDelta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
		rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
		colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
		hasTailShift := byteDelta != 0 || edit.NewEndPoint != edit.OldEndPoint
		var shiftScratch []*Node
		editNodeWithDelta(t.root, edit, byteDelta, rowDelta, colDelta, hasTailShift, &shiftScratch, &t.lastEditedLeaf)
	}
}

// Edits returns the pending edits recorded on this tree.
func (t *Tree) Edits() []InputEdit { return t.edits }

// ChangedRanges converts this tree's recorded edits into changed source ranges.
// Overlapping ranges are coalesced.
func (t *Tree) ChangedRanges() []Range {
	if t == nil || len(t.edits) == 0 {
		return nil
	}
	ranges := make([]Range, 0, len(t.edits))
	for _, e := range t.edits {
		ranges = append(ranges, Range{
			StartByte:  e.StartByte,
			EndByte:    e.NewEndByte,
			StartPoint: e.StartPoint,
			EndPoint:   e.NewEndPoint,
		})
	}
	return coalesceRanges(ranges)
}

func rangesOverlapOrTouch(a, b Range) bool {
	return !(a.EndByte < b.StartByte || b.EndByte < a.StartByte)
}

func coalesceRanges(in []Range) []Range {
	if len(in) <= 1 {
		return in
	}
	sort.Slice(in, func(i, j int) bool {
		if in[i].StartByte != in[j].StartByte {
			return in[i].StartByte < in[j].StartByte
		}
		return in[i].EndByte < in[j].EndByte
	})
	out := make([]Range, 0, len(in))
	current := in[0]
	for i := 1; i < len(in); i++ {
		r := in[i]
		if rangesOverlapOrTouch(current, r) {
			if r.StartByte < current.StartByte {
				current.StartByte = r.StartByte
				current.StartPoint = r.StartPoint
			}
			if r.EndByte > current.EndByte {
				current.EndByte = r.EndByte
				current.EndPoint = r.EndPoint
			}
			continue
		}
		out = append(out, current)
		current = r
	}
	out = append(out, current)
	return out
}

// editNode recursively adjusts a node's byte/point spans for an edit and
// marks nodes that overlap the edited region as dirty.
func editNode(n *Node, edit InputEdit) {
	byteDelta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
	rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
	colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
	hasTailShift := byteDelta != 0 || edit.NewEndPoint != edit.OldEndPoint
	var shiftScratch []*Node
	editNodeWithDelta(n, edit, byteDelta, rowDelta, colDelta, hasTailShift, &shiftScratch, nil)
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

func editNodeWithDelta(n *Node, edit InputEdit, byteDelta, rowDelta, colDelta int64, hasTailShift bool, shiftScratch *[]*Node, leafHint **Node) {
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
			n.startPoint.Row = addUint32Delta(n.startPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.startPoint.Column = addUint32Delta(n.startPoint.Column, colDelta)
			}
		}
		if n.endPoint.Row == edit.OldEndPoint.Row {
			n.endPoint.Row = addUint32Delta(n.endPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.endPoint.Column = addUint32Delta(n.endPoint.Column, colDelta)
			}
		}
		shiftSubtreeAfterEdit(n.children, edit, byteDelta, rowDelta, colDelta, shiftScratch)
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
	descended := false
	for _, c := range n.children {
		if c.endByte <= edit.StartByte {
			continue
		}
		if c.startByte >= edit.OldEndByte {
			if !hasTailShift {
				break
			}
			shiftSubtreeNodeAfterEdit(c, edit, byteDelta, rowDelta, colDelta, shiftScratch)
			continue
		}
		descended = true
		editNodeWithDelta(c, edit, byteDelta, rowDelta, colDelta, hasTailShift, shiftScratch, leafHint)
	}
	if leafHint != nil && !descended && len(n.children) == 0 {
		*leafHint = n
	}
}

func shiftSubtreeAfterEdit(roots []*Node, edit InputEdit, byteDelta, rowDelta, colDelta int64, shiftScratch *[]*Node) {
	if len(roots) == 0 {
		return
	}

	var stack [](*Node)
	if shiftScratch != nil {
		stack = (*shiftScratch)[:0]
	}
	stack = append(stack, roots...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		n.startByte = addUint32Delta(n.startByte, byteDelta)
		n.endByte = addUint32Delta(n.endByte, byteDelta)

		if n.startPoint.Row == edit.OldEndPoint.Row {
			n.startPoint.Row = addUint32Delta(n.startPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.startPoint.Column = addUint32Delta(n.startPoint.Column, colDelta)
			}
		}
		if n.endPoint.Row == edit.OldEndPoint.Row {
			n.endPoint.Row = addUint32Delta(n.endPoint.Row, rowDelta)
			if rowDelta == 0 {
				n.endPoint.Column = addUint32Delta(n.endPoint.Column, colDelta)
			}
		}

		for _, c := range n.children {
			stack = append(stack, c)
		}
	}
	if shiftScratch != nil {
		*shiftScratch = stack[:0]
	}
}

func shiftSubtreeNodeAfterEdit(root *Node, edit InputEdit, byteDelta, rowDelta, colDelta int64, shiftScratch *[]*Node) {
	if root == nil {
		return
	}
	var roots [1]*Node
	roots[0] = root
	shiftSubtreeAfterEdit(roots[:], edit, byteDelta, rowDelta, colDelta, shiftScratch)
}

// DiffChangedRanges compares two syntax trees and returns the minimal
// ranges where syntactic structure differs. The old tree should have been
// edited (via Tree.Edit) to match the new tree's source positions before
// reparsing.
//
// This is equivalent to C tree-sitter's ts_tree_get_changed_ranges().
func DiffChangedRanges(oldTree, newTree *Tree) []Range {
	if oldTree == nil || newTree == nil {
		return nil
	}
	oldRoot := oldTree.RootNode()
	newRoot := newTree.RootNode()
	if oldRoot == nil || newRoot == nil {
		return nil
	}

	var ranges []Range
	diffNodes(oldRoot, newRoot, &ranges)
	return coalesceRanges(ranges)
}

// diffNodes recursively compares old and new tree nodes, appending changed
// ranges when structural differences are found.
func diffNodes(oldNode, newNode *Node, ranges *[]Range) {
	// If both nodes are structurally identical, nothing changed.
	if nodesStructurallyEqual(oldNode, newNode) {
		return
	}

	// If they differ at the symbol level or child count, the entire range is changed.
	if oldNode.Symbol() != newNode.Symbol() ||
		oldNode.ChildCount() != newNode.ChildCount() {
		addChangedRange(oldNode, newNode, ranges)
		return
	}

	// Leaf nodes (no children) that are not structurally equal: they differ in
	// byte range or one of them has been marked dirty. Report the range.
	if oldNode.ChildCount() == 0 {
		addChangedRange(oldNode, newNode, ranges)
		return
	}

	// Same symbol and child count — recurse into children.
	for i := 0; i < oldNode.ChildCount(); i++ {
		oldChild := oldNode.Child(i)
		newChild := newNode.Child(i)
		diffNodes(oldChild, newChild, ranges)
	}
}

// nodesStructurallyEqual reports whether two nodes are structurally identical
// and can be skipped during diff. Two nodes are equal if they have the same
// symbol, the same byte range, the same child count, and neither has been
// marked as changed by Tree.Edit.
func nodesStructurallyEqual(a, b *Node) bool {
	if a.Symbol() != b.Symbol() {
		return false
	}
	if a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() {
		return false
	}
	if a.ChildCount() != b.ChildCount() {
		return false
	}
	// Fast path: if neither node has changes, they're equal.
	if !a.HasChanges() && !b.HasChanges() {
		return true
	}
	return false
}

// addChangedRange records a changed range covering both the old and new node spans.
func addChangedRange(oldNode, newNode *Node, ranges *[]Range) {
	startByte := min(oldNode.StartByte(), newNode.StartByte())
	endByte := max(oldNode.EndByte(), newNode.EndByte())
	startPoint := oldNode.StartPoint()
	endPoint := newNode.EndPoint()
	if newNode.StartByte() < oldNode.StartByte() {
		startPoint = newNode.StartPoint()
	}
	if oldNode.EndByte() > newNode.EndByte() {
		endPoint = oldNode.EndPoint()
	}
	*ranges = append(*ranges, Range{
		StartByte:  startByte,
		EndByte:    endByte,
		StartPoint: startPoint,
		EndPoint:   endPoint,
	})
}

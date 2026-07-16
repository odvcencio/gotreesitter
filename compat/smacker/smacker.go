// Package smacker is a compatibility shim that lets code written against
// github.com/smacker/go-tree-sitter run on the pure-Go gotreesitter runtime
// with an import swap and no per-node rewrites.
//
// smacker/go-tree-sitter (a cgo binding to the C tree-sitter runtime) has been
// unmaintained since August 2024, yet ~560 modules still depend on it. Its
// public Node methods take no language argument because the language is baked
// into the C node; gotreesitter threads the *Language explicitly for its
// pure-Go, arena-based nodes. This package bridges that difference: a Node here
// carries its owning language (and source) so the smacker-shaped, argument-free
// methods keep working.
//
// Coverage is the subset of the smacker surface real consumers use: package
// ParseCtx, the stateful Parser/Tree, Node navigation, and Query/QueryCursor.
// Per-grammar language handles live in subpackages (smacker/golang,
// smacker/python, ...) exposing GetLanguage(), mirroring smacker's layout.
package smacker

import (
	"context"
	"sync/atomic"
	"time"

	gts "github.com/odvcencio/gotreesitter"
)

// Language is the opaque grammar handle. It is identical to gotreesitter's, so
// values from the per-grammar subpackages flow straight into Parser and Query.
type Language = gts.Language

// Point is a zero-based (row, column) source position. Field-identical to
// smacker's and gotreesitter's Point.
type Point = gts.Point

// Range is a node's byte and point span. Field-identical to smacker's Range.
type Range struct {
	StartByte  uint32
	EndByte    uint32
	StartPoint Point
	EndPoint   Point
}

// EditInput describes a source edit for incremental reparsing. It mirrors
// smacker's EditInput; gotreesitter's equivalent type is named InputEdit with
// byte-suffixed field names.
type EditInput struct {
	StartIndex  uint32
	OldEndIndex uint32
	NewEndIndex uint32
	StartPoint  Point
	OldEndPoint Point
	NewEndPoint Point
}

func (e EditInput) toInputEdit() gts.InputEdit {
	return gts.InputEdit{
		StartByte:   e.StartIndex,
		OldEndByte:  e.OldEndIndex,
		NewEndByte:  e.NewEndIndex,
		StartPoint:  e.StartPoint,
		OldEndPoint: e.OldEndPoint,
		NewEndPoint: e.NewEndPoint,
	}
}

// Node wraps a gotreesitter node and exposes the argument-free smacker Node API
// by threading the owning language and source that gotreesitter takes
// explicitly. The zero value and a nil *Node both report IsNull() == true.
type Node struct {
	inner  *gts.Node
	lang   *gts.Language
	source []byte
}

func wrapNode(n *gts.Node, lang *gts.Language, source []byte) *Node {
	if n == nil {
		return nil
	}
	return &Node{inner: n, lang: lang, source: source}
}

// IsNull reports whether the node is absent. smacker returns a non-nil node
// wrapping a null C node; here both nil and an empty wrapper are null.
func (n *Node) IsNull() bool { return n == nil || n.inner == nil }

// Type returns the node's grammar symbol name (e.g. "identifier").
func (n *Node) Type() string {
	if n.IsNull() {
		return ""
	}
	return n.inner.Type(n.lang)
}

// Content returns the node's source text. smacker requires the caller to pass
// the source buffer; that buffer is used verbatim.
func (n *Node) Content(input []byte) string {
	if n.IsNull() {
		return ""
	}
	return n.inner.Text(input)
}

// String renders the node as an S-expression, matching smacker's Node.String().
func (n *Node) String() string {
	if n.IsNull() {
		return ""
	}
	return n.inner.SExpr(n.lang)
}

// ChildByFieldName returns the child under the named field, or a null node.
func (n *Node) ChildByFieldName(field string) *Node {
	if n.IsNull() {
		return nil
	}
	return wrapNode(n.inner.ChildByFieldName(field, n.lang), n.lang, n.source)
}

// FieldNameForChild returns the field name of the i-th child, or "".
func (n *Node) FieldNameForChild(i int) string {
	if n.IsNull() {
		return ""
	}
	return n.inner.FieldNameForChild(i, n.lang)
}

// Child returns the i-th child (including anonymous nodes), or a null node.
func (n *Node) Child(i int) *Node {
	if n.IsNull() {
		return nil
	}
	return wrapNode(n.inner.Child(i), n.lang, n.source)
}

// NamedChild returns the i-th named child, or a null node.
func (n *Node) NamedChild(i int) *Node {
	if n.IsNull() {
		return nil
	}
	return wrapNode(n.inner.NamedChild(i), n.lang, n.source)
}

// ChildCount returns the number of children, matching smacker's uint32 return.
func (n *Node) ChildCount() uint32 {
	if n.IsNull() {
		return 0
	}
	return uint32(n.inner.ChildCount())
}

// NamedChildCount returns the number of named children.
func (n *Node) NamedChildCount() uint32 {
	if n.IsNull() {
		return 0
	}
	return uint32(n.inner.NamedChildCount())
}

// Parent returns the node's parent, or a null node at the root.
func (n *Node) Parent() *Node {
	if n.IsNull() {
		return nil
	}
	return wrapNode(n.inner.Parent(), n.lang, n.source)
}

// IsNamed reports whether the node is a named (non-anonymous) node.
func (n *Node) IsNamed() bool {
	if n.IsNull() {
		return false
	}
	return n.inner.IsNamed()
}

// PrevSibling returns the node's previous sibling, or a null node.
func (n *Node) PrevSibling() *Node {
	if n.IsNull() {
		return nil
	}
	return wrapNode(n.inner.PrevSibling(), n.lang, n.source)
}

// NextSibling returns the node's next sibling, or a null node.
func (n *Node) NextSibling() *Node {
	if n.IsNull() {
		return nil
	}
	return wrapNode(n.inner.NextSibling(), n.lang, n.source)
}

// StartByte returns the node's start byte offset.
func (n *Node) StartByte() uint32 {
	if n.IsNull() {
		return 0
	}
	return n.inner.StartByte()
}

// EndByte returns the node's end byte offset.
func (n *Node) EndByte() uint32 {
	if n.IsNull() {
		return 0
	}
	return n.inner.EndByte()
}

// StartPoint returns the node's start (row, column).
func (n *Node) StartPoint() Point {
	if n.IsNull() {
		return Point{}
	}
	return n.inner.StartPoint()
}

// EndPoint returns the node's end (row, column).
func (n *Node) EndPoint() Point {
	if n.IsNull() {
		return Point{}
	}
	return n.inner.EndPoint()
}

// Range returns the node's byte and point span.
func (n *Node) Range() Range {
	if n.IsNull() {
		return Range{}
	}
	return Range{
		StartByte:  n.inner.StartByte(),
		EndByte:    n.inner.EndByte(),
		StartPoint: n.inner.StartPoint(),
		EndPoint:   n.inner.EndPoint(),
	}
}

// Equal reports whether two wrapped nodes refer to the same syntax node,
// matching smacker's C-node identity semantics. gotreesitter returns a stable
// *Node pointer for the same logical node across all navigation, so inner-
// pointer identity is exact. A span+type comparison is deliberately NOT used as
// a fallback: distinct nodes can share a span and type (e.g. nested ERROR nodes
// on malformed input), and a node from Copy() has a fresh pointer, so such a
// fallback would report false positives on exactly the error-laden input that
// downstream scanners process.
func (n *Node) Equal(other *Node) bool {
	if n.IsNull() || other == nil || other.inner == nil {
		return n.IsNull() && (other == nil || other.inner == nil)
	}
	return n.inner == other.inner
}

// Tree is a parsed syntax tree. It retains the language and source so RootNode
// can hand out fully-threaded Nodes.
type Tree struct {
	inner  *gts.Tree
	lang   *gts.Language
	source []byte
}

// RootNode returns the tree's root node.
func (t *Tree) RootNode() *Node {
	if t == nil || t.inner == nil {
		return nil
	}
	return wrapNode(t.inner.RootNode(), t.lang, t.source)
}

// Edit applies a source edit in preparation for incremental reparsing.
func (t *Tree) Edit(edit EditInput) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.Edit(edit.toInputEdit())
}

// Copy returns an independent copy of the tree. gotreesitter deep-clones the
// nodes into a fresh arena, matching smacker's ts_tree_copy.
func (t *Tree) Copy() *Tree {
	if t == nil || t.inner == nil {
		return nil
	}
	return &Tree{inner: t.inner.Copy(), lang: t.lang, source: t.source}
}

// Close is a no-op; gotreesitter trees are garbage-collected.
func (t *Tree) Close() {}

// Parser is a reusable, single-language parser matching smacker's stateful API.
type Parser struct {
	lang *gts.Language
}

// NewParser returns a new parser. Call SetLanguage before parsing.
func NewParser() *Parser { return &Parser{} }

// SetLanguage selects the grammar the parser will use.
func (p *Parser) SetLanguage(lang *Language) { p.lang = lang }

// ParseCtx parses content into a tree, honoring the context deadline (mapped to
// gotreesitter's timeout). The oldTree argument enables incremental reparsing.
func (p *Parser) ParseCtx(ctx context.Context, oldTree *Tree, content []byte) (*Tree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parser := gts.NewParser(p.lang)
	stop := applyContext(ctx, parser)
	defer stop()
	var (
		tree *gts.Tree
		err  error
	)
	if oldTree != nil && oldTree.inner != nil {
		tree, err = parser.ParseIncremental(content, oldTree.inner)
	} else {
		tree, err = parser.Parse(content)
	}
	if err != nil {
		return nil, err
	}
	return &Tree{inner: tree, lang: p.lang, source: content}, nil
}

// Parse parses content into a tree (non-incremental unless oldTree is given).
func (p *Parser) Parse(oldTree *Tree, content []byte) (*Tree, error) {
	return p.ParseCtx(context.Background(), oldTree, content)
}

// Close is a no-op; gotreesitter parsers hold no C resources.
func (p *Parser) Close() {}

// ParseCtx parses content with the given language and returns the root node,
// mirroring smacker's package-level helper.
func ParseCtx(ctx context.Context, content []byte, lang *Language) (*Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parser := gts.NewParser(lang)
	stop := applyContext(ctx, parser)
	defer stop()
	tree, err := parser.Parse(content)
	if err != nil {
		return nil, err
	}
	return wrapNode(tree.RootNode(), lang, content), nil
}

// applyContext maps a context onto the parser the way smacker's ParseCtx does:
// a deadline becomes gotreesitter's timeout, and cancellation is wired to the
// parser's cancellation flag via a watcher goroutine so a context.WithCancel
// context aborts an in-flight parse even without a deadline. The returned
// function stops the watcher and must be called after parsing.
func applyContext(ctx context.Context, parser *gts.Parser) func() {
	if dl, ok := ctx.Deadline(); ok {
		if micros := time.Until(dl).Microseconds(); micros > 0 {
			parser.SetTimeoutMicros(uint64(micros))
		}
	}
	if ctx.Done() == nil {
		return func() {}
	}
	var flag uint32
	parser.SetCancellationFlag(&flag)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			atomic.StoreUint32(&flag, 1)
		case <-done:
		}
	}()
	return func() { close(done) }
}

// Query is a compiled tree-sitter query.
type Query struct {
	inner    *gts.Query
	names    []string
	nameToID map[string]uint32
}

// NewQuery compiles a query pattern for the given language. The pattern is
// smacker's []byte; gotreesitter takes a string, so it is converted verbatim.
func NewQuery(pattern []byte, lang *Language) (*Query, error) {
	inner, err := gts.NewQuery(string(pattern), lang)
	if err != nil {
		return nil, err
	}
	names := inner.CaptureNames()
	nameToID := make(map[string]uint32, len(names))
	for i, name := range names {
		nameToID[name] = uint32(i)
	}
	return &Query{inner: inner, names: names, nameToID: nameToID}, nil
}

// CaptureNameForId returns the capture name for a capture index, matching the
// smacker method consumers call as query.CaptureNameForId(capture.Index).
func (q *Query) CaptureNameForId(id uint32) string {
	if int(id) < len(q.names) {
		return q.names[id]
	}
	return ""
}

// PatternCount returns the number of patterns in the query.
func (q *Query) PatternCount() uint32 { return uint32(q.inner.PatternCount()) }

// Close is a no-op; gotreesitter queries hold no C resources.
func (q *Query) Close() {}

// QueryCapture is a single captured node within a match. Index identifies the
// capture; pass it to Query.CaptureNameForId to recover the capture name.
type QueryCapture struct {
	Index uint32
	Node  *Node
}

// QueryMatch is a single query match.
type QueryMatch struct {
	PatternIndex int
	Captures     []QueryCapture
}

// QueryCursor iterates the matches of a query over a node's subtree.
type QueryCursor struct {
	inner  *gts.QueryCursor
	query  *Query
	lang   *gts.Language
	source []byte
}

// NewQueryCursor returns a new, unbound cursor. Call Exec to bind it.
func NewQueryCursor() *QueryCursor { return &QueryCursor{} }

// Exec binds the cursor to a query and a node, ready for NextMatch iteration.
func (c *QueryCursor) Exec(q *Query, n *Node) {
	c.query = q
	if n == nil || n.IsNull() {
		c.inner = nil
		return
	}
	c.lang = n.lang
	c.source = n.source
	c.inner = q.inner.Exec(n.inner, n.lang, n.source)
}

// NextMatch returns the next match, or (nil, false) when iteration is done.
func (c *QueryCursor) NextMatch() (*QueryMatch, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	m, ok := c.inner.NextMatch()
	if !ok {
		return nil, false
	}
	captures := make([]QueryCapture, len(m.Captures))
	for i, cap := range m.Captures {
		captures[i] = QueryCapture{
			Index: c.query.nameToID[cap.Name],
			Node:  wrapNode(cap.Node, c.lang, c.source),
		}
	}
	return &QueryMatch{PatternIndex: m.PatternIndex, Captures: captures}, true
}

// FilterPredicates returns the match after applying the query's text
// predicates (#eq?, #match?, #any-of?, ...). gotreesitter evaluates predicates
// during matching — Exec is given the source for exactly that — so matches
// reaching this call already satisfy their predicates and the match is returned
// unchanged. It exists so smacker consumers that call it explicitly compile and
// behave identically.
func (c *QueryCursor) FilterPredicates(m *QueryMatch, input []byte) *QueryMatch {
	return m
}

// Close is a no-op; gotreesitter cursors hold no C resources.
func (c *QueryCursor) Close() {}

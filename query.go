package gotreesitter

import (
	"fmt"
	"regexp"
	"slices"
)

// Query holds compiled patterns parsed from a tree-sitter .scm query file.
// It can be executed against a syntax tree to find matching nodes and
// return captured names.
//
// Query is safe for concurrent calls to Execute, ExecuteInto, ExecuteNode, and
// Exec after construction. Each goroutine must use its own QueryCursor and any
// ExecuteInto destination slice must remain caller-owned. The mutating methods
// DisableCapture and DisablePattern are NOT safe to call concurrently with
// execution (or with each other); call them before sharing the Query.
type Query struct {
	patterns []Pattern
	captures []string // capture name by index
	strings  []string // string literals by index

	rootCandidatesBySymbol     map[Symbol][]int
	rootCandidatesDense        [][]int
	rootFallbackCandidates     []int
	postCandidatesBySymbol     map[Symbol][]int
	postCandidatesDense        [][]int
	postFallbackCandidates     []int
	rootZeroOrMorePatterns     []int
	rootRepetitionPostPatterns []int
	canSkipExactRootLeaves     bool

	disabledPatternIdx  map[int]struct{}
	disabledCaptureName map[string]struct{}
}

// Pattern is a single top-level S-expression pattern in a query.
type Pattern struct {
	startByte  uint32
	endByte    uint32
	steps      []QueryStep
	predicates []QueryPredicate
}

// QueryStep is one matching instruction within a pattern.
type QueryStep struct {
	symbol       Symbol          // node type to match, or 0 for wildcard
	field        FieldID         // required field on parent, or 0
	absentFields []FieldID       // fields that must be absent on this node
	captureIDs   []int           // all captures in declaration order
	isNamed      bool            // whether we expect a named node
	depth        int             // nesting depth (0 = top-level node in pattern)
	quantifier   queryQuantifier // ?, *, + (default: exactly one)
	anchorBefore bool            // '.' before this step (first child / immediate sibling)
	anchorAfter  bool            // '.' after this step (last child)
	synthetic    bool            // true for grouping roots that do not correspond to a query node
	// For alternation steps, alternatives lists the alternative symbols
	// that can match at this position. If non-nil, symbol is ignored.
	alternatives []alternativeSymbol
	// altIndex accelerates alternation branch selection while preserving
	// declaration order. It is built once at query compile time.
	altIndex *queryAlternationIndex
	// textMatch is for string literal matching ("func", "return", etc.).
	// When non-empty, we match anonymous nodes whose symbol name equals this.
	textMatch string
	// isMissing marks a step compiled from the special "MISSING" node
	// pattern ((MISSING), (MISSING kind), or (MISSING "token")). Such a
	// step only matches nodes for which Node.IsMissing() is true, in
	// addition to any symbol/textMatch constraint carried above.
	isMissing bool
}

type queryQuantifier uint8

const (
	queryQuantifierOne queryQuantifier = iota
	queryQuantifierZeroOrOne
	queryQuantifierZeroOrMore
	queryQuantifierOneOrMore
)

type queryPredicateType uint8

const (
	predicateEq queryPredicateType = iota
	predicateNotEq
	predicateMatch
	predicateNotMatch
	predicateAnyOf
	predicateNotAnyOf
	predicateLuaMatch
	predicateHasAncestor
	predicateNotHasAncestor
	predicateHasParent
	predicateNotHasParent
	predicateIs
	predicateIsNot
	predicateSet
	predicateOffset
	predicateAnyEq
	predicateAnyNotEq
	predicateAnyMatch
	predicateAnyNotMatch
	predicateSelectAdjacent
	predicateStrip
	predicateCount      // #count? @cap op value
	predicateIsExported // #is-exported? @cap
)

// QueryPredicate is a post-match constraint attached to a pattern.
// Supported forms:
//   - (#eq? @a @b)
//   - (#eq? @a "literal")
//   - (#not-eq? @a @b)
//   - (#not-eq? @a "literal")
//   - (#match? @a "regex")
//   - (#not-match? @a "regex")
//   - (#lua-match? @a "lua-pattern")
//   - (#any-of? @a "v1" "v2" ...)
//   - (#not-any-of? @a "v1" "v2" ...)
//   - (#any-eq? @a "literal"), (#any-eq? @a @b)
//   - (#any-not-eq? @a "literal"), (#any-not-eq? @a @b)
//   - (#any-match? @a "regex")
//   - (#any-not-match? @a "regex")
//   - (#has-ancestor? @a type ...)
//   - (#not-has-ancestor? @a type ...)
//   - (#has-parent? @a type ...)
//   - (#not-has-parent? @a type ...)
//   - (#is? ...), (#is-not? ...)
//   - (#set! key value), (#offset! @cap ...)
//   - (#count? @a op value)       -- op: >, <, >=, <=, ==, !=
//   - (#is-exported? @a)
type QueryPredicate struct {
	kind queryPredicateType

	leftCapture  string
	rightCapture string // optional for #eq? / #not-eq?
	// optional property/name token for #is? / #is-not?.
	property   string
	literal    string // literal or regex source
	values     []string
	regex      *regexp.Regexp
	offset     [4]int // #offset! start_row start_col end_row end_col
	countOp    string // for #count?: ">", "<", ">=", "<=", "==", "!="
	countValue int    // for #count?

	allowMissing bool // true when scoped under a child pattern that may match zero times
}

// QueryPropertyPredicate is host-consumable metadata for an inert #is? or
// #is-not? predicate. Property is the queried property name, Capture is the
// optional capture name without its leading '@', and Positive distinguishes
// #is? from #is-not?. Property predicates do not filter query matches.
type QueryPropertyPredicate struct {
	Property string
	Capture  string
	Positive bool
}

// PropertyPredicate exposes p when it represents #is? or #is-not? metadata.
// Other predicate kinds return ok=false.
func (p QueryPredicate) PropertyPredicate() (property QueryPropertyPredicate, ok bool) {
	switch p.kind {
	case predicateIs:
		return QueryPropertyPredicate{Property: p.property, Capture: p.leftCapture, Positive: true}, true
	case predicateIsNot:
		return QueryPropertyPredicate{Property: p.property, Capture: p.leftCapture, Positive: false}, true
	default:
		return QueryPropertyPredicate{}, false
	}
}

// alternativeSymbol is one branch of an alternation like [(true) (false)].
type alternativeSymbol struct {
	symbol    Symbol
	isNamed   bool
	isMissing bool
	// field constrains this branch to a child with the given parent field ID.
	// It is only evaluated when the alternation step is matched as a child.
	field FieldID
	// textMatch for string alternatives like "func"
	textMatch  string
	captureIDs []int
	// steps/predicates represent a complex branch like
	// [(function_declaration name: (identifier) @name) ...].
	steps      []QueryStep
	predicates []QueryPredicate
}

// QueryMatch represents a successful pattern match with its captures.
type QueryMatch struct {
	PatternIndex int
	Captures     []QueryCapture
}

// QueryCapture is a single captured node within a match.
type QueryCapture struct {
	Name string
	Node *Node
	// TextOverride, when non-empty, replaces the node's source text for
	// downstream consumers. It is set by the #strip! directive.
	TextOverride string
}

// Text returns the effective text for this capture. If TextOverride is set
// (e.g. by the #strip! directive), it is returned. Otherwise the node's
// source text is returned.
func (c QueryCapture) Text(source []byte) string {
	if c.TextOverride != "" {
		return c.TextOverride
	}
	if c.Node == nil {
		return ""
	}
	return c.Node.Text(source)
}

// UTF16Range returns this capture's node range in UTF-16 code-unit
// coordinates for trees produced by UTF-16 parse APIs.
func (c QueryCapture) UTF16Range(tree *Tree) (UTF16Range, bool) {
	if c.Node == nil {
		return UTF16Range{}, false
	}
	return tree.UTF16RangeForNode(c.Node)
}

type queryUnknownNodeTypeError struct {
	name string
}

func (e queryUnknownNodeTypeError) Error() string {
	return fmt.Sprintf("query: unknown node type %q", e.name)
}

// QueryCursor incrementally walks a node subtree and yields matches one by one.
// It is the streaming counterpart to Query.Execute and avoids materializing all
// matches up front.
// QueryCursor is not safe for concurrent use.
type QueryCursor struct {
	query  *Query
	lang   *Language
	source []byte

	worklist []queryCursorWorkItem

	hasByteRange bool
	startByte    uint32
	endByte      uint32

	hasPointRange bool
	startPoint    Point
	endPoint      Point

	currentNode       *Node
	currentParent     *Node
	currentChildIdx   int
	currentNodeDepth  uint32
	currentNodePost   bool
	currentCandidates []int
	candidateIdx      int

	// Pending captures from the last match returned by NextMatch.
	pendingCaptures   []QueryCapture
	pendingCaptureIdx int

	pendingMatches  []QueryMatch
	pendingMatchIdx int

	matchLimit        uint32
	matchCount        uint32
	limitProbePending bool
	didExceedMatchLim bool

	workBudget int

	hasMaxStartDepth bool
	maxStartDepth    uint32

	done bool
}

type queryCursorWorkItem struct {
	node     *Node
	parent   *Node
	childIdx int
	depth    uint32
	post     bool
}

type queryExecBuffer struct {
	matches        []QueryMatch
	readerMatches  []queryReaderMatch[QueryCapture]
	readerWorklist []queryReaderWorkItem[*Node]
}

// NewQuery compiles query source (tree-sitter .scm format) against a language.
// It returns an error if the query syntax is invalid or references unknown
// node types or field names.
func NewQuery(source string, lang *Language) (*Query, error) {
	p := &queryParser{
		input: source,
		lang:  lang,
		q: &Query{
			captures: []string{},
		},
	}
	if err := p.parse(); err != nil {
		return nil, err
	}
	p.q.buildAlternationIndices()
	p.q.buildRootPatternIndex()
	return p.q, nil
}

// Execute runs the query against a syntax tree and returns all matches.
func (q *Query) Execute(tree *Tree) []QueryMatch {
	if tree == nil {
		return nil
	}
	return q.executeNode(tree.RootNode(), tree.Language(), tree.Source())
}

// ExecuteInto runs the query against a syntax tree, appending matches into
// dst and returning the updated slice. Callers can pre-allocate or reuse dst
// across calls to eliminate the per-call slice allocation from Execute.
//
// Example:
//
//	var buf []QueryMatch
//	for _, tree := range trees {
//	    buf = q.ExecuteInto(tree, buf[:0])
//	    process(buf)
//	}
func (q *Query) ExecuteInto(tree *Tree, dst []QueryMatch) []QueryMatch {
	if tree == nil {
		return dst
	}
	return q.executeNodeInto(tree.RootNode(), tree.Language(), tree.Source(), dst)
}

// ExecuteNode runs the query starting from a specific node.
//
// source is required for text predicates (like #eq? / #match?); pass the
// originating source bytes for correct predicate evaluation.
func (q *Query) ExecuteNode(node *Node, lang *Language, source []byte) []QueryMatch {
	return q.executeNode(node, lang, source)
}

// Exec creates a streaming cursor over matches rooted at node.
func (q *Query) Exec(node *Node, lang *Language, source []byte) *QueryCursor {
	c := &QueryCursor{
		query:      q,
		lang:       lang,
		source:     source,
		workBudget: defaultQueryMatchWorkBudget,
	}
	if node != nil {
		// Pre-size the worklist for typical tree depth (avoids early growths).
		c.worklist = make([]queryCursorWorkItem, 1, 32)
		c.worklist[0] = queryCursorWorkItem{node: node, childIdx: -1, depth: 0}
	}
	return c
}

// SetByteRange restricts matches to nodes that intersect [startByte, endByte).
// As in tree-sitter C, endByte == 0 is the unbounded-end sentinel.
func (c *QueryCursor) SetByteRange(startByte, endByte uint32) {
	if endByte == 0 {
		endByte = ^uint32(0)
	}
	c.setByteRangeExact(startByte, endByte)
}

// setByteRangeExact stores an exact half-open byte range. Unlike the public C-
// compatible setter, a zero end remains zero; the UTF-16 adapter needs this to
// represent an actual empty range at the beginning of a document.
func (c *QueryCursor) setByteRangeExact(startByte, endByte uint32) bool {
	if c == nil || endByte < startByte {
		return false
	}
	c.hasByteRange = true
	c.startByte = startByte
	c.endByte = endByte
	return true
}

func pointRangeEndSentinel(endPoint Point) Point {
	if endPoint == (Point{}) {
		return Point{Row: ^uint32(0), Column: ^uint32(0)}
	}
	return endPoint
}

func (c *QueryCursor) setPointRangeExact(startPoint, endPoint Point) bool {
	if c == nil || pointLessThan(endPoint, startPoint) {
		return false
	}
	c.hasPointRange = true
	c.startPoint = startPoint
	c.endPoint = endPoint
	return true
}

// SetUTF16Range restricts matches to nodes that intersect the given UTF-16
// code-unit range. tree must have been produced by a UTF-16 parse API.
func (c *QueryCursor) SetUTF16Range(tree *Tree, startCodeUnit, endCodeUnit uint32) bool {
	if c == nil || tree == nil || endCodeUnit < startCodeUnit {
		return false
	}
	startByte, ok := tree.UTF8ByteForUTF16Offset(startCodeUnit)
	if !ok {
		return false
	}
	endByte, ok := tree.UTF8ByteForUTF16Offset(endCodeUnit)
	if !ok {
		return false
	}
	return c.setByteRangeExact(startByte, endByte)
}

// SetPointRange restricts matches to nodes that intersect [startPoint, endPoint).
// As in tree-sitter C, an all-zero end point is the unbounded-end sentinel.
func (c *QueryCursor) SetPointRange(startPoint, endPoint Point) {
	c.setPointRangeExact(startPoint, pointRangeEndSentinel(endPoint))
}

// SetMatchLimit sets the maximum number of matches this cursor can return.
// A limit of 0 means unlimited.
func (c *QueryCursor) SetMatchLimit(limit uint32) {
	if c == nil {
		return
	}
	c.matchLimit = limit
	c.didExceedMatchLim = false
	c.limitProbePending = limit > 0 && c.matchCount >= limit
}

// DidExceedMatchLimit reports whether query execution had additional matches
// beyond the configured match limit.
func (c *QueryCursor) DidExceedMatchLimit() bool {
	if c == nil {
		return false
	}
	return c.didExceedMatchLim
}

// SetMatchWorkBudget bounds the number of enumeration steps the matcher may
// take per (pattern,node) attempt, guarding against pathological O(2^n)
// queries. A limit of 0 means unlimited. The default is defaultQueryMatchWorkBudget.
// On exhaustion the cursor returns bounded partial results and DidExceedMatchLimit
// reports true, mirroring C tree-sitter's over-limit behavior.
func (c *QueryCursor) SetMatchWorkBudget(limit int) {
	if c == nil {
		return
	}
	c.workBudget = limit
}

// SetMaxStartDepth limits the depth at which new matches can begin.
// Depth 0 means only the starting node passed to Exec.
func (c *QueryCursor) SetMaxStartDepth(depth uint32) {
	if c == nil {
		return
	}
	c.hasMaxStartDepth = true
	c.maxStartDepth = depth
}

func byteRangeIntersects(start, end, rangeStart, rangeEnd uint32) bool {
	if rangeEnd < rangeStart {
		return false
	}
	isEmpty := start == end
	if end < rangeStart || (!isEmpty && end == rangeStart) {
		return false
	}
	return start < rangeEnd
}

func pointRangeIntersects(start, end, rangeStart, rangeEnd Point, isEmpty bool) bool {
	if pointLessThan(rangeEnd, rangeStart) {
		return false
	}
	if pointLessThan(end, rangeStart) || (!isEmpty && end == rangeStart) {
		return false
	}
	return pointLessThan(start, rangeEnd)
}

func (c *QueryCursor) nodeIntersectsRanges(n *Node) bool {
	if n == nil {
		return false
	}
	if c.hasByteRange {
		if !byteRangeIntersects(n.startByte, n.endByte, c.startByte, c.endByte) {
			return false
		}
	}
	if c.hasPointRange {
		// Locked tree-sitter treats a zero-width node at range start as
		// intersecting, while ordinary nodes that merely end there do not. A
		// node beginning at range end is always outside the half-open range.
		if !pointRangeIntersects(n.startPoint, n.endPoint, c.startPoint, c.endPoint, n.startByte == n.endByte) {
			return false
		}
	}
	return true
}

func (c *QueryCursor) stackEntryIntersectsRanges(e stackEntry) bool {
	if c == nil {
		return false
	}
	if c.hasByteRange {
		if !byteRangeIntersects(stackEntryNodeStartByte(e), stackEntryNodeEndByte(e), c.startByte, c.endByte) {
			return false
		}
	}
	if c.hasPointRange {
		if e.kind == stackEntryKindNoTreeNode {
			return true
		}
		startPoint := stackEntryNodeStartPoint(e)
		endPoint := stackEntryNodeEndPoint(e)
		isEmpty := stackEntryNodeStartByte(e) == stackEntryNodeEndByte(e)
		if !pointRangeIntersects(startPoint, endPoint, c.startPoint, c.endPoint, isEmpty) {
			return false
		}
	}
	return true
}

func (q *Query) executeNode(root *Node, lang *Language, source []byte) []QueryMatch {
	if root == nil || lang == nil {
		return nil
	}
	var buf queryExecBuffer
	return q.executeNodeIntoBuffer(root, lang, source, &buf)
}

func (q *Query) executeNodeInto(root *Node, lang *Language, source []byte, dst []QueryMatch) []QueryMatch {
	if root == nil || lang == nil {
		return dst
	}

	cursor := q.Exec(root, lang, source)
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		dst = append(dst, m)
	}
	return dst
}

func (q *Query) executeNodeIntoBuffer(root *Node, lang *Language, source []byte, buf *queryExecBuffer) []QueryMatch {
	if buf == nil {
		return q.executeNode(root, lang, source)
	}
	buf.matches = buf.matches[:0]
	buf.readerMatches, buf.readerWorklist = executeQueryWithReader(
		q, root, lang, source, publicQueryReader{}, buf.readerMatches, buf.readerWorklist,
	)
	buf.matches = slices.Grow(buf.matches, len(buf.readerMatches))
	for _, match := range buf.readerMatches {
		buf.matches = append(buf.matches, QueryMatch{PatternIndex: match.PatternIndex, Captures: match.Captures})
	}
	return buf.matches
}

func (q *Query) rootPatternCandidates(sym Symbol) []int {
	if int(sym) < len(q.rootCandidatesDense) {
		if cands := q.rootCandidatesDense[sym]; cands != nil {
			return cands
		}
	}
	if cands, ok := q.rootCandidatesBySymbol[sym]; ok {
		return cands
	}
	return q.rootFallbackCandidates
}

func (q *Query) postorderPatternCandidates(sym Symbol) []int {
	if int(sym) < len(q.postCandidatesDense) {
		if cands := q.postCandidatesDense[sym]; cands != nil {
			return cands
		}
	}
	if cands, ok := q.postCandidatesBySymbol[sym]; ok {
		return cands
	}
	return q.postFallbackCandidates
}

func (q *Query) hasPostorderPatterns() bool {
	return len(q.rootRepetitionPostPatterns) > 0 ||
		len(q.postFallbackCandidates) > 0 ||
		len(q.postCandidatesBySymbol) > 0
}

func mergePatternIndexLists(a, b []int) []int {
	if len(a) == 0 {
		out := make([]int, len(b))
		copy(out, b)
		return out
	}
	if len(b) == 0 {
		out := make([]int, len(a))
		copy(out, a)
		return out
	}

	out := make([]int, 0, len(a)+len(b))
	i, j := 0, 0
	last := -1
	hasLast := false

	appendUnique := func(v int) {
		if hasLast && v == last {
			return
		}
		out = append(out, v)
		last = v
		hasLast = true
	}

	for i < len(a) && j < len(b) {
		if a[i] < b[j] {
			appendUnique(a[i])
			i++
			continue
		}
		if b[j] < a[i] {
			appendUnique(b[j])
			j++
			continue
		}
		appendUnique(a[i])
		i++
		j++
	}
	for ; i < len(a); i++ {
		appendUnique(a[i])
	}
	for ; j < len(b); j++ {
		appendUnique(b[j])
	}
	return out
}

func (q *Query) buildRootPatternIndex() {
	bySymbolExact := make(map[Symbol][]int)
	postBySymbolExact := make(map[Symbol][]int)
	var wildcard []int
	var complex []int
	var postWildcard []int
	var postComplex []int
	q.rootZeroOrMorePatterns = q.rootZeroOrMorePatterns[:0]
	q.rootRepetitionPostPatterns = q.rootRepetitionPostPatterns[:0]

	for pi, pat := range q.patterns {
		if len(pat.steps) == 0 {
			continue
		}
		step := pat.steps[0]

		if step.quantifier == queryQuantifierZeroOrMore {
			complex = append(complex, pi)
			q.rootZeroOrMorePatterns = append(q.rootZeroOrMorePatterns, pi)
			q.rootRepetitionPostPatterns = append(q.rootRepetitionPostPatterns, pi)
			continue
		}
		if step.quantifier == queryQuantifierOneOrMore {
			q.rootRepetitionPostPatterns = append(q.rootRepetitionPostPatterns, pi)
		}

		if patternHasPostorderChildRepetition(pat) {
			addRootPatternCandidate(pi, step, postBySymbolExact, &postWildcard, &postComplex)
			continue
		}

		addRootPatternCandidate(pi, step, bySymbolExact, &wildcard, &complex)
	}

	fallback := mergePatternIndexLists(wildcard, complex)
	q.rootFallbackCandidates = fallback
	q.rootCandidatesBySymbol = make(map[Symbol][]int, len(bySymbolExact))
	maxSymbol := Symbol(0)
	for sym, exact := range bySymbolExact {
		if sym > maxSymbol {
			maxSymbol = sym
		}
		q.rootCandidatesBySymbol[sym] = mergePatternIndexLists(exact, fallback)
	}
	q.rootCandidatesDense = make([][]int, int(maxSymbol)+1)
	for sym, candidates := range q.rootCandidatesBySymbol {
		q.rootCandidatesDense[sym] = candidates
	}

	postFallback := mergePatternIndexLists(postWildcard, postComplex)
	q.postFallbackCandidates = postFallback
	q.postCandidatesBySymbol = make(map[Symbol][]int, len(postBySymbolExact))
	maxPostSymbol := Symbol(0)
	for sym, exact := range postBySymbolExact {
		if sym > maxPostSymbol {
			maxPostSymbol = sym
		}
		q.postCandidatesBySymbol[sym] = mergePatternIndexLists(exact, postFallback)
	}
	q.postCandidatesDense = make([][]int, int(maxPostSymbol)+1)
	for sym, candidates := range q.postCandidatesBySymbol {
		q.postCandidatesDense[sym] = candidates
	}
	q.canSkipExactRootLeaves = len(q.rootFallbackCandidates) == 0 &&
		len(q.rootRepetitionPostPatterns) == 0 &&
		len(q.postFallbackCandidates) == 0 &&
		len(q.postCandidatesBySymbol) == 0
}

func addRootPatternCandidate(pi int, step QueryStep, bySymbolExact map[Symbol][]int, wildcard *[]int, complex *[]int) {
	if len(step.alternatives) > 0 {
		complexAlt := false
		for _, alt := range step.alternatives {
			if alt.textMatch != "" || alt.symbol == 0 {
				complexAlt = true
				break
			}
		}
		if complexAlt {
			*complex = append(*complex, pi)
			return
		}

		seen := make(map[Symbol]struct{}, len(step.alternatives))
		for _, alt := range step.alternatives {
			if _, ok := seen[alt.symbol]; ok {
				continue
			}
			seen[alt.symbol] = struct{}{}
			bySymbolExact[alt.symbol] = append(bySymbolExact[alt.symbol], pi)
		}
		return
	}

	if step.textMatch != "" {
		*complex = append(*complex, pi)
		return
	}
	if step.symbol == 0 {
		*wildcard = append(*wildcard, pi)
		return
	}

	bySymbolExact[step.symbol] = append(bySymbolExact[step.symbol], pi)
}

func patternHasPostorderChildRepetition(pat Pattern) bool {
	for i := 1; i < len(pat.steps); i++ {
		step := pat.steps[i]
		if step.depth == 0 {
			continue
		}
		if step.quantifier == queryQuantifierZeroOrMore || step.quantifier == queryQuantifierOneOrMore {
			return true
		}
	}
	return false
}

// NextMatch yields the next query match from the cursor.
func (c *QueryCursor) NextMatch() (QueryMatch, bool) {
	if c == nil || c.done || c.query == nil || c.lang == nil {
		return QueryMatch{}, false
	}

	// If callers mix NextCapture and NextMatch, NextMatch advances at match
	// granularity and discards any partially-consumed capture buffer.
	c.pendingCaptures = nil
	c.pendingCaptureIdx = 0

	if c.matchLimit == 0 {
		return c.nextMatchRaw()
	}

	if c.matchCount < c.matchLimit {
		m, ok := c.nextMatchRaw()
		if !ok {
			return QueryMatch{}, false
		}
		c.matchCount++
		if c.matchCount == c.matchLimit {
			c.limitProbePending = true
		}
		return m, true
	}

	if c.limitProbePending {
		_, ok := c.nextMatchRaw()
		c.didExceedMatchLim = ok
		c.limitProbePending = false
	}
	c.done = true
	return QueryMatch{}, false
}

func (c *QueryCursor) nextMatchRaw() (QueryMatch, bool) {
	if c == nil || c.done || c.query == nil || c.lang == nil {
		return QueryMatch{}, false
	}
	if c.pendingMatchIdx < len(c.pendingMatches) {
		m := c.pendingMatches[c.pendingMatchIdx]
		c.pendingMatchIdx++
		if c.pendingMatchIdx >= len(c.pendingMatches) {
			c.pendingMatches = nil
			c.pendingMatchIdx = 0
		}
		return m, true
	}
	q := c.query
	if q.rootCandidatesBySymbol == nil && q.rootFallbackCandidates == nil {
		q.buildRootPatternIndex()
	}

	for {
		if c.currentNode == nil {
			if len(c.worklist) == 0 {
				c.done = true
				return QueryMatch{}, false
			}

			// Pop next node in DFS order.
			item := c.worklist[len(c.worklist)-1]
			c.worklist = c.worklist[:len(c.worklist)-1]
			n := item.node
			depth := item.depth
			if (c.hasByteRange || c.hasPointRange) && !c.nodeIntersectsRanges(n) {
				continue
			}
			if item.post {
				c.currentNode = n
				c.currentParent = item.parent
				c.currentChildIdx = item.childIdx
				c.currentNodeDepth = depth
				c.currentNodePost = true
				postCandidates := q.postorderPatternCandidates(c.lang.PublicSymbolForNamedness(n.Symbol(), n.IsNamed()))
				c.currentCandidates = mergePatternIndexLists(q.rootRepetitionPostPatterns, postCandidates)
				c.candidateIdx = 0
				continue
			}

			if c.hasMaxStartDepth && depth > c.maxStartDepth {
				continue
			}

			c.currentNode = n
			c.currentParent = item.parent
			c.currentChildIdx = item.childIdx
			c.currentNodeDepth = depth
			c.currentNodePost = false
			c.currentCandidates = q.rootPatternCandidates(c.lang.PublicSymbolForNamedness(n.Symbol(), n.IsNamed()))
			c.candidateIdx = 0
		}

		for c.candidateIdx < len(c.currentCandidates) {
			pi := c.currentCandidates[c.candidateIdx]
			c.candidateIdx++
			if q.isPatternDisabled(pi) {
				continue
			}
			pat := q.patterns[pi]
			if !c.currentNodePost {
				if match, ok := q.singleStepQueryMatch(&pat, pi, c.currentNode, c.lang); ok {
					return match, true
				}
			}
			budget := newQueryMatchBudget(c.workBudget)
			var captureSets [][]QueryCapture
			if c.currentNodePost {
				if pat.steps[0].quantifier == queryQuantifierZeroOrMore || pat.steps[0].quantifier == queryQuantifierOneOrMore {
					captureSets = q.matchPatternPostorderAll(&pat, c.currentNode, c.currentParent, c.currentChildIdx, c.lang, c.source, budget)
				} else {
					captureSets = q.matchPatternAll(&pat, c.currentNode, c.lang, c.source, budget)
				}
			} else {
				captureSets = q.matchPatternAll(&pat, c.currentNode, c.lang, c.source, budget)
			}
			if budget.tripped() {
				c.didExceedMatchLim = true
			}
			if len(captureSets) == 0 {
				continue
			}
			first := QueryMatch{
				PatternIndex: pi,
				Captures:     captureSets[0],
			}
			if len(captureSets) > 1 {
				c.pendingMatches = make([]QueryMatch, len(captureSets)-1)
				for i := 1; i < len(captureSets); i++ {
					c.pendingMatches[i-1] = QueryMatch{
						PatternIndex: pi,
						Captures:     captureSets[i],
					}
				}
			}
			return first, true
		}

		// Exhausted candidates for this node; advance to the next node.
		c.pushCurrentNodeChildren()
		c.currentNode = nil
		c.currentParent = nil
		c.currentChildIdx = -1
		c.currentNodeDepth = 0
		c.currentNodePost = false
		c.currentCandidates = nil
		c.candidateIdx = 0
	}
}

func (q *Query) singleStepQueryMatch(pat *Pattern, patternIndex int, node *Node, lang *Language) (QueryMatch, bool) {
	if q == nil || pat == nil || node == nil || len(pat.predicates) != 0 || len(pat.steps) != 1 {
		return QueryMatch{}, false
	}
	step := &pat.steps[0]
	if step.field != 0 ||
		len(step.absentFields) != 0 ||
		len(step.alternatives) != 0 ||
		step.textMatch != "" ||
		step.depth != 0 ||
		step.quantifier != queryQuantifierOne ||
		step.anchorBefore ||
		step.anchorAfter ||
		!q.nodeMatchesStep(step, node, lang) {
		return QueryMatch{}, false
	}
	var captures []QueryCapture
	q.appendCaptureIDs(step.captureIDs, node, &captures)
	return QueryMatch{
		PatternIndex: patternIndex,
		Captures:     captures,
	}, true
}

func (q *Query) canSkipQueryLeafEntry(entry stackEntry, lang *Language) bool {
	if q == nil || lang == nil || !q.canSkipExactRootLeaves {
		return false
	}
	if stackEntryNodeChildCount(entry) != 0 {
		return false
	}
	nodeNamed := stackEntryNodeIsNamed(entry)
	sym := lang.PublicSymbolForNamedness(stackEntryNodeSymbol(entry), nodeNamed)
	return len(q.rootPatternCandidates(sym)) == 0
}

func (c *QueryCursor) pushCurrentNodeChildren() {
	n := c.currentNode
	if n == nil {
		return
	}
	if c.currentNodePost {
		return
	}
	if c.query != nil && c.query.hasPostorderPatterns() {
		c.worklist = append(c.worklist, queryCursorWorkItem{
			node:     n,
			parent:   c.currentParent,
			childIdx: c.currentChildIdx,
			depth:    c.currentNodeDepth,
			post:     true,
		})
	}
	if c.hasMaxStartDepth && c.currentNodeDepth >= c.maxStartDepth {
		return
	}
	nextDepth := c.currentNodeDepth + 1
	rangeLimited := c.hasByteRange || c.hasPointRange
	// Push children in reverse order so leftmost is visited first.
	for i := nodeChildCountNoMaterialize(n) - 1; i >= 0; i-- {
		entry, ok := nodeChildEntryAtNoMaterialize(n, i)
		if ok {
			if rangeLimited && !c.stackEntryIntersectsRanges(entry) {
				continue
			}
			if c.query.canSkipQueryLeafEntry(entry, c.lang) {
				continue
			}
		}
		child := nodeChildAtForReason(n, i, materializeForQuery)
		if child != nil && (!rangeLimited || c.nodeIntersectsRanges(child)) {
			c.worklist = append(c.worklist, queryCursorWorkItem{
				node:     child,
				parent:   n,
				childIdx: i,
				depth:    nextDepth,
			})
		}
	}
}

// NextCapture yields captures in match order by draining NextMatch results.
// This is a practical first-pass ordering: captures are returned in each
// match's capture order, then by subsequent matches in DFS match order.
func (c *QueryCursor) NextCapture() (QueryCapture, bool) {
	if c == nil || c.done || c.query == nil || c.lang == nil {
		return QueryCapture{}, false
	}

	for {
		if c.pendingCaptureIdx < len(c.pendingCaptures) {
			cap := c.pendingCaptures[c.pendingCaptureIdx]
			c.pendingCaptureIdx++
			return cap, true
		}

		m, ok := c.NextMatch()
		if !ok {
			return QueryCapture{}, false
		}
		c.pendingCaptures = m.Captures
		c.pendingCaptureIdx = 0
	}
}

func (q *Query) matchStepWithRollbackPredicates(steps []QueryStep, stepIdx int, node *Node, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	return q.matchStepWithRollbackAtParentPredicates(steps, stepIdx, node, nil, -1, lang, source, predicates, captures)
}

func (q *Query) matchStepWithRollbackAtParentPredicates(steps []QueryStep, stepIdx int, node *Node, parent *Node, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	checkpoint := len(*captures)
	if q.matchStepsWithParentPredicates(steps, stepIdx, node, parent, childIdx, lang, source, predicates, captures) {
		return true
	}
	*captures = (*captures)[:checkpoint]
	return false
}

// PatternCount returns the number of patterns in the query.
func (q *Query) PatternCount() int {
	return len(q.patterns)
}

// CaptureCount returns the number of unique capture names in this query.
func (q *Query) CaptureCount() uint32 {
	if q == nil {
		return 0
	}
	return uint32(len(q.captures))
}

// CaptureNames returns the list of unique capture names used in the query.
func (q *Query) CaptureNames() []string {
	return q.captures
}

// CaptureNameForID returns the capture name for the given capture id.
func (q *Query) CaptureNameForID(id uint32) (string, bool) {
	if q == nil || int(id) >= len(q.captures) {
		return "", false
	}
	return q.captures[id], true
}

// StringCount returns the number of unique string literals in this query.
func (q *Query) StringCount() uint32 {
	if q == nil {
		return 0
	}
	return uint32(len(q.strings))
}

// StringValueForID returns the string literal for the given string id.
func (q *Query) StringValueForID(id uint32) (string, bool) {
	if q == nil || int(id) >= len(q.strings) {
		return "", false
	}
	return q.strings[id], true
}

// StartByteForPattern returns the query-source start byte for patternIndex.
func (q *Query) StartByteForPattern(patternIndex uint32) (uint32, bool) {
	if q == nil {
		return 0, false
	}
	idx := int(patternIndex)
	if idx < 0 || idx >= len(q.patterns) {
		return 0, false
	}
	return q.patterns[idx].startByte, true
}

// EndByteForPattern returns the query-source end byte for patternIndex.
func (q *Query) EndByteForPattern(patternIndex uint32) (uint32, bool) {
	if q == nil {
		return 0, false
	}
	idx := int(patternIndex)
	if idx < 0 || idx >= len(q.patterns) {
		return 0, false
	}
	return q.patterns[idx].endByte, true
}

// PredicatesForPattern returns a copy of predicates attached to patternIndex.
func (q *Query) PredicatesForPattern(patternIndex uint32) ([]QueryPredicate, bool) {
	if q == nil {
		return nil, false
	}
	idx := int(patternIndex)
	if idx < 0 || idx >= len(q.patterns) {
		return nil, false
	}
	preds := q.patterns[idx].predicates
	if len(preds) == 0 {
		return nil, true
	}
	out := make([]QueryPredicate, len(preds))
	copy(out, preds)
	return out, true
}

// PropertyPredicatesForPattern returns the #is? and #is-not? metadata attached
// to patternIndex in source order. A valid pattern with no property predicates
// returns an empty slice and ok=true.
func (q *Query) PropertyPredicatesForPattern(patternIndex uint32) ([]QueryPropertyPredicate, bool) {
	predicates, ok := q.PredicatesForPattern(patternIndex)
	if !ok {
		return nil, false
	}
	var properties []QueryPropertyPredicate
	for _, predicate := range predicates {
		if property, isProperty := predicate.PropertyPredicate(); isProperty {
			properties = append(properties, property)
		}
	}
	return properties, true
}

// IsPatternRooted reports whether the pattern has exactly one root step at
// depth 0. Rooted patterns start matching from a single concrete root.
func (q *Query) IsPatternRooted(patternIndex uint32) bool {
	if q == nil {
		return false
	}
	idx := int(patternIndex)
	if idx < 0 || idx >= len(q.patterns) {
		return false
	}
	steps := q.patterns[idx].steps
	if len(steps) == 0 {
		return false
	}
	rootCount := 0
	for _, step := range steps {
		if step.depth == 0 {
			rootCount++
		}
	}
	return rootCount == 1
}

// IsPatternNonLocal reports whether the pattern can begin at multiple roots.
func (q *Query) IsPatternNonLocal(patternIndex uint32) bool {
	return !q.IsPatternRooted(patternIndex)
}

// StepIsDefinite reports whether a pattern step matches a definite symbol
// (i.e. not wildcard).
func (q *Query) StepIsDefinite(patternIndex uint32, stepIndex uint32) bool {
	if q == nil {
		return false
	}
	pi := int(patternIndex)
	if pi < 0 || pi >= len(q.patterns) {
		return false
	}
	si := int(stepIndex)
	steps := q.patterns[pi].steps
	if si < 0 || si >= len(steps) {
		return false
	}
	step := steps[si]
	if step.symbol == 0 {
		return false
	}
	if len(step.alternatives) > 0 {
		for _, alt := range step.alternatives {
			if alt.symbol == 0 || alt.textMatch != "" {
				return false
			}
		}
	}
	return true
}

// IsPatternGuaranteedAtStep reports whether all steps through stepIndex are
// definite and non-quantified.
func (q *Query) IsPatternGuaranteedAtStep(patternIndex uint32, stepIndex uint32) bool {
	if q == nil {
		return false
	}
	pi := int(patternIndex)
	if pi < 0 || pi >= len(q.patterns) {
		return false
	}
	si := int(stepIndex)
	steps := q.patterns[pi].steps
	if si < 0 || si >= len(steps) {
		return false
	}
	for i := 0; i <= si; i++ {
		step := steps[i]
		if step.quantifier != queryQuantifierOne {
			return false
		}
		if !q.StepIsDefinite(patternIndex, uint32(i)) {
			return false
		}
	}
	return true
}

// DisableCapture removes captures with the given name from future query
// results. Matching behavior is unchanged; only returned captures are filtered.
func (q *Query) DisableCapture(name string) {
	if q == nil || name == "" {
		return
	}
	if q.disabledCaptureName == nil {
		q.disabledCaptureName = make(map[string]struct{})
	}
	q.disabledCaptureName[name] = struct{}{}
}

// DisablePattern disables a pattern by index.
func (q *Query) DisablePattern(patternIndex uint32) {
	if q == nil {
		return
	}
	idx := int(patternIndex)
	if idx < 0 || idx >= len(q.patterns) {
		return
	}
	if q.disabledPatternIdx == nil {
		q.disabledPatternIdx = make(map[int]struct{})
	}
	q.disabledPatternIdx[idx] = struct{}{}
}

func (q *Query) isCaptureDisabled(name string) bool {
	if q == nil || q.disabledCaptureName == nil {
		return false
	}
	_, disabled := q.disabledCaptureName[name]
	return disabled
}

func (q *Query) isPatternDisabled(patternIndex int) bool {
	if q == nil || q.disabledPatternIdx == nil {
		return false
	}
	_, disabled := q.disabledPatternIdx[patternIndex]
	return disabled
}

// SetValues returns the values of a #set! directive with the given key
// for a match's pattern, or nil if not present. This is used by
// InjectionParser to read injection.language metadata.
func (m QueryMatch) SetValues(q *Query, key string) []string {
	if q == nil || m.PatternIndex < 0 || m.PatternIndex >= len(q.patterns) {
		return nil
	}
	for _, pred := range q.patterns[m.PatternIndex].predicates {
		if pred.kind == predicateSet && pred.literal == key {
			return pred.values
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// S-expression parser
// --------------------------------------------------------------------------

// queryParser parses tree-sitter .scm query files into a Query.

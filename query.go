package gotreesitter

import (
	"fmt"
	"regexp"
)

// Query holds compiled patterns parsed from a tree-sitter .scm query file.
// It can be executed against a syntax tree to find matching nodes and
// return captured names.
// Query is safe for concurrent use after construction.
type Query struct {
	patterns []Pattern
	captures []string // capture name by index
	strings  []string // string literals by index

	rootCandidatesBySymbol map[Symbol][]int
	rootCandidatesDense    [][]int
	rootFallbackCandidates []int

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
	captureID    int             // first capture index into Query.captures, or -1
	captureIDs   []int           // all captures in declaration order
	isNamed      bool            // whether we expect a named node
	depth        int             // nesting depth (0 = top-level node in pattern)
	quantifier   queryQuantifier // ?, *, + (default: exactly one)
	anchorBefore bool            // '.' before this step (first child / immediate sibling)
	anchorAfter  bool            // '.' after this step (last child)
	// For alternation steps, alternatives lists the alternative symbols
	// that can match at this position. If non-nil, symbol is ignored.
	alternatives []alternativeSymbol
	// altIndex accelerates alternation branch selection while preserving
	// declaration order. It is built once at query compile time.
	altIndex *queryAlternationIndex
	// textMatch is for string literal matching ("func", "return", etc.).
	// When non-empty, we match anonymous nodes whose symbol name equals this.
	textMatch string
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
}

// alternativeSymbol is one branch of an alternation like [(true) (false)].
type alternativeSymbol struct {
	symbol  Symbol
	isNamed bool
	// field constrains this branch to a child with the given parent field ID.
	// It is only evaluated when the alternation step is matched as a child.
	field FieldID
	// textMatch for string alternatives like "func"
	textMatch string
	// captureID is the first capture on this branch. captureIDs contains all.
	captureID  int
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
	currentNodeDepth  uint32
	currentCandidates []int
	candidateIdx      int

	// Pending captures from the last match returned by NextMatch.
	pendingCaptures   []QueryCapture
	pendingCaptureIdx int

	matchLimit        uint32
	matchCount        uint32
	limitProbePending bool
	didExceedMatchLim bool

	hasMaxStartDepth bool
	maxStartDepth    uint32

	done bool
}

type queryCursorWorkItem struct {
	node  *Node
	depth uint32
}

type queryExecBuffer struct {
	matches  []QueryMatch
	captures []QueryCapture
	worklist []queryCursorWorkItem
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
		query:  q,
		lang:   lang,
		source: source,
	}
	if node != nil {
		// Pre-size the worklist for typical tree depth (avoids early growths).
		c.worklist = make([]queryCursorWorkItem, 1, 32)
		c.worklist[0] = queryCursorWorkItem{node: node, depth: 0}
	}
	return c
}

// SetByteRange restricts matches to nodes that intersect [startByte, endByte).
func (c *QueryCursor) SetByteRange(startByte, endByte uint32) {
	if c == nil {
		return
	}
	c.hasByteRange = true
	c.startByte = startByte
	c.endByte = endByte
}

// SetPointRange restricts matches to nodes that intersect [startPoint, endPoint).
func (c *QueryCursor) SetPointRange(startPoint, endPoint Point) {
	if c == nil {
		return
	}
	c.hasPointRange = true
	c.startPoint = startPoint
	c.endPoint = endPoint
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

// SetMaxStartDepth limits the depth at which new matches can begin.
// Depth 0 means only the starting node passed to Exec.
func (c *QueryCursor) SetMaxStartDepth(depth uint32) {
	if c == nil {
		return
	}
	c.hasMaxStartDepth = true
	c.maxStartDepth = depth
}

func (c *QueryCursor) nodeIntersectsRanges(n *Node) bool {
	if n == nil {
		return false
	}
	if c.hasByteRange {
		if c.endByte <= c.startByte {
			return false
		}
		if n.endByte <= c.startByte || n.startByte >= c.endByte {
			return false
		}
	}
	if c.hasPointRange {
		if !pointLessThan(c.startPoint, c.endPoint) && c.startPoint != c.endPoint {
			return false
		}
		if !pointLessThan(n.startPoint, c.endPoint) && n.startPoint != c.endPoint {
			return false
		}
		if !pointLessThan(c.startPoint, n.endPoint) && c.startPoint != n.endPoint {
			return false
		}
	}
	return true
}

func (q *Query) executeNode(root *Node, lang *Language, source []byte) []QueryMatch {
	if root == nil || lang == nil {
		return nil
	}

	cursor := q.Exec(root, lang, source)
	// Pre-size based on source length: empirically ~1 match per 40 bytes for
	// typical highlight queries. Underestimating is fine; we just grow once more.
	initCap := len(source)/40 + 16
	matches := make([]QueryMatch, 0, initCap)
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		matches = append(matches, m)
	}
	return matches
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
	if root == nil || lang == nil {
		if buf == nil {
			return nil
		}
		buf.matches = buf.matches[:0]
		buf.captures = buf.captures[:0]
		buf.worklist = buf.worklist[:0]
		return buf.matches
	}
	if buf == nil {
		return q.executeNode(root, lang, source)
	}
	if q.rootCandidatesBySymbol == nil && q.rootFallbackCandidates == nil {
		q.buildRootPatternIndex()
	}

	buf.matches = buf.matches[:0]
	buf.captures = buf.captures[:0]
	buf.worklist = append(buf.worklist[:0], queryCursorWorkItem{node: root, depth: 0})

	for len(buf.worklist) > 0 {
		last := len(buf.worklist) - 1
		item := buf.worklist[last]
		buf.worklist = buf.worklist[:last]

		n := item.node
		if n == nil {
			continue
		}

		for i := n.ChildCount() - 1; i >= 0; i-- {
			child := n.Child(i)
			if child == nil {
				continue
			}
			buf.worklist = append(buf.worklist, queryCursorWorkItem{
				node:  child,
				depth: item.depth + 1,
			})
		}

		candidates := q.rootPatternCandidates(lang.PublicSymbol(n.Symbol()))
		for _, pi := range candidates {
			if q.isPatternDisabled(pi) {
				continue
			}
			pat := q.patterns[pi]
			nextCaptures, ok := q.matchPatternIntoBuffer(&pat, n, lang, source, buf.captures)
			if !ok {
				continue
			}
			start := len(buf.captures)
			buf.captures = nextCaptures
			buf.matches = append(buf.matches, QueryMatch{
				PatternIndex: pi,
				Captures:     buf.captures[start:len(buf.captures):len(buf.captures)],
			})
		}
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
	var wildcard []int
	var complex []int

	for pi, pat := range q.patterns {
		if len(pat.steps) == 0 {
			continue
		}
		step := pat.steps[0]

		if len(step.alternatives) > 0 {
			complexAlt := false
			for _, alt := range step.alternatives {
				if alt.textMatch != "" || alt.symbol == 0 {
					complexAlt = true
					break
				}
			}
			if complexAlt {
				complex = append(complex, pi)
				continue
			}

			seen := make(map[Symbol]struct{}, len(step.alternatives))
			for _, alt := range step.alternatives {
				if _, ok := seen[alt.symbol]; ok {
					continue
				}
				seen[alt.symbol] = struct{}{}
				bySymbolExact[alt.symbol] = append(bySymbolExact[alt.symbol], pi)
			}
			continue
		}

		if step.textMatch != "" {
			complex = append(complex, pi)
			continue
		}
		if step.symbol == 0 {
			wildcard = append(wildcard, pi)
			continue
		}

		bySymbolExact[step.symbol] = append(bySymbolExact[step.symbol], pi)
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
			if !c.nodeIntersectsRanges(n) {
				continue
			}

			if c.hasMaxStartDepth && depth > c.maxStartDepth {
				continue
			}

			// Push children in reverse order so leftmost is visited first.
			if !c.hasMaxStartDepth || depth < c.maxStartDepth {
				children := n.Children()
				for i := len(children) - 1; i >= 0; i-- {
					if c.nodeIntersectsRanges(children[i]) {
						c.worklist = append(c.worklist, queryCursorWorkItem{
							node:  children[i],
							depth: depth + 1,
						})
					}
				}
			}

			c.currentNode = n
			c.currentNodeDepth = depth
			c.currentCandidates = q.rootPatternCandidates(c.lang.PublicSymbol(n.Symbol()))
			c.candidateIdx = 0
		}

		for c.candidateIdx < len(c.currentCandidates) {
			pi := c.currentCandidates[c.candidateIdx]
			c.candidateIdx++
			if q.isPatternDisabled(pi) {
				continue
			}
			pat := q.patterns[pi]
			if caps, ok := q.matchPattern(&pat, c.currentNode, c.lang, c.source); ok {
				return QueryMatch{
					PatternIndex: pi,
					Captures:     caps,
				}, true
			}
		}

		// Exhausted candidates for this node; advance to the next node.
		c.currentNode = nil
		c.currentNodeDepth = 0
		c.currentCandidates = nil
		c.candidateIdx = 0
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

// matchPattern tries to match a pattern against the given node.
// The pattern's steps describe a nested structure; step depth 0 matches
// the given node, depth 1 matches its children, etc.
func (q *Query) matchPattern(pat *Pattern, node *Node, lang *Language, source []byte) ([]QueryCapture, bool) {
	if len(pat.steps) == 0 {
		return nil, false
	}

	var captures []QueryCapture
	ok := q.matchStepsWithPredicates(pat.steps, 0, node, lang, source, pat.predicates, &captures)
	if !ok {
		return nil, false
	}
	if !q.matchesPredicates(pat.predicates, captures, lang, source) {
		return nil, false
	}
	captures = q.applyDirectives(pat.predicates, captures, source)
	return captures, true
}

func (q *Query) matchPatternIntoBuffer(pat *Pattern, node *Node, lang *Language, source []byte, captures []QueryCapture) ([]QueryCapture, bool) {
	if len(pat.steps) == 0 {
		return captures, false
	}

	start := len(captures)
	if !q.matchStepsWithPredicates(pat.steps, 0, node, lang, source, pat.predicates, &captures) {
		return captures[:start], false
	}

	matchCaptures := captures[start:]
	if !q.matchesPredicates(pat.predicates, matchCaptures, lang, source) {
		return captures[:start], false
	}

	matchCaptures = q.applyDirectives(pat.predicates, matchCaptures, source)
	return captures[:start+len(matchCaptures)], true
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

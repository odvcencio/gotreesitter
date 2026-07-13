package gotreesitter

import (
	"cmp"
	"fmt"
	"slices"
)

// HighlightRange represents a styled range of source code, mapping a byte span
// to a capture name from a highlight query. The editor maps capture names
// (e.g., "keyword", "string", "function") to FSS style classes.
type HighlightRange struct {
	StartByte    uint32
	EndByte      uint32
	Capture      string // "keyword", "string", "function", etc.
	PatternIndex int    // query pattern index; later patterns override earlier for identical ranges
}

// UTF16HighlightRange is a styled source range in UTF-16 code-unit
// coordinates.
type UTF16HighlightRange struct {
	StartCodeUnit uint32
	EndCodeUnit   uint32
	StartPoint    Point
	EndPoint      Point
	Capture       string
	PatternIndex  int
}

// Highlighter is a high-level API that takes source code and returns styled
// ranges. It combines a Parser, a compiled Query, and a Language to provide
// a single Highlight() call for the editor.
type Highlighter struct {
	parser             *Parser
	query              *Query
	lang               *Language
	tokenSourceFactory func(source []byte) TokenSource
	injectionQuery     *Query
	injectionResolver  HighlighterInjectionResolver
	childQueries       map[string]*Query
	execBuffer         queryExecBuffer
	rangeBuffer        []HighlightRange
	resolvedBuffer     []HighlightRange
}

// HighlighterOption configures a Highlighter.
type HighlighterOption func(*Highlighter)

// WithTokenSourceFactory sets a factory function that creates a TokenSource
// for each Highlight call. This is needed for languages that use a custom
// lexer bridge (like Go, which uses go/scanner instead of a DFA lexer).
//
// When set, Highlight() calls ParseWithTokenSource instead of Parse.
func WithTokenSourceFactory(factory func(source []byte) TokenSource) HighlighterOption {
	return func(h *Highlighter) {
		h.tokenSourceFactory = factory
	}
}

// NewHighlighter creates a Highlighter for the given language and highlight
// query (in tree-sitter .scm format). Returns an error if the query fails
// to compile.
func NewHighlighter(lang *Language, highlightQuery string, opts ...HighlighterOption) (*Highlighter, error) {
	q, err := NewQuery(highlightQuery, lang)
	if err != nil {
		return nil, err
	}

	h := &Highlighter{
		parser: NewParser(lang),
		query:  q,
		lang:   lang,
	}
	for _, opt := range opts {
		opt(h)
	}
	if lang != nil {
		if spec, ok := lookupHighlighterInjection(lang.Name); ok {
			injQ, injErr := NewQuery(spec.Query, lang)
			if injErr != nil {
				return nil, fmt.Errorf("highlighter injection query for %q: %w", lang.Name, injErr)
			}
			h.injectionQuery = injQ
			h.injectionResolver = spec.ResolveLanguage
			h.childQueries = make(map[string]*Query)
		}
	}
	return h, nil
}

// HighlightIncremental re-highlights source after edits were applied to oldTree.
// Returns the new highlight ranges and the new parse tree (for use in subsequent
// incremental calls). Call oldTree.Edit() before calling this.
func (h *Highlighter) HighlightIncremental(source []byte, oldTree *Tree) ([]HighlightRange, *Tree) {
	if len(source) == 0 {
		return nil, NewTree(nil, source, h.lang)
	}

	tree := h.parse(source, oldTree)

	if tree.RootNode() == nil {
		return nil, tree
	}

	return h.highlightTree(tree, source), tree
}

// HighlightIncrementalUTF16 re-highlights UTF-16 source after edits were
// applied to oldTree with Tree.EditUTF16.
func (h *Highlighter) HighlightIncrementalUTF16(source []uint16, oldTree *Tree) ([]UTF16HighlightRange, *Tree) {
	if len(source) == 0 {
		tree := dispatchParseUTF16(h.parser, source, nil, h.tokenSourceFactory, h.lang)
		return nil, tree
	}

	tree := h.parseUTF16(source, oldTree)
	if tree.RootNode() == nil {
		return nil, tree
	}

	return h.highlightTreeUTF16(tree), tree
}

// HighlightIncrementalUTF16Bytes is like HighlightIncrementalUTF16 for
// endian-specific UTF-16 bytes.
func (h *Highlighter) HighlightIncrementalUTF16Bytes(source []byte, oldTree *Tree, order UTF16ByteOrder) ([]UTF16HighlightRange, *Tree, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, nil, err
	}
	ranges, tree := h.HighlightIncrementalUTF16(units, oldTree)
	return ranges, tree, nil
}

// Highlight parses the source code and executes the highlight query, returning
// a slice of HighlightRange sorted by StartByte. When ranges overlap, inner
// (more specific) captures take priority over outer ones.
func (h *Highlighter) Highlight(source []byte) []HighlightRange {
	if len(source) == 0 {
		return nil
	}

	tree := h.parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil
	}
	defer tree.Release()

	return h.highlightTree(tree, source)
}

// HighlightUTF16 parses UTF-16 source and returns highlight ranges in UTF-16
// code-unit coordinates.
func (h *Highlighter) HighlightUTF16(source []uint16) []UTF16HighlightRange {
	if len(source) == 0 {
		return nil
	}

	tree := h.parseUTF16(source, nil)
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil
	}
	defer tree.Release()

	return h.highlightTreeUTF16(tree)
}

// HighlightTreeUTF16 executes the highlighter query against an already parsed
// UTF-16 tree. It lets editor runtimes share one persistent parse tree across
// syntax highlighting, tags, and ad-hoc queries.
func (h *Highlighter) HighlightTreeUTF16(tree *Tree) []UTF16HighlightRange {
	if tree == nil || tree.RootNode() == nil {
		return nil
	}
	return h.highlightTreeUTF16(tree)
}

// HighlightUTF16Bytes is like HighlightUTF16 for endian-specific UTF-16 bytes.
func (h *Highlighter) HighlightUTF16Bytes(source []byte, order UTF16ByteOrder) ([]UTF16HighlightRange, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return h.HighlightUTF16(units), nil
}

func (h *Highlighter) parse(source []byte, oldTree *Tree) *Tree {
	return dispatchParse(h.parser, source, oldTree, h.tokenSourceFactory, h.lang)
}

func (h *Highlighter) parseUTF16(source []uint16, oldTree *Tree) *Tree {
	return dispatchParseUTF16(h.parser, source, oldTree, h.tokenSourceFactory, h.lang)
}

func (h *Highlighter) highlightTreeUTF16(tree *Tree) []UTF16HighlightRange {
	return highlightRangesToUTF16(tree, h.highlightTree(tree, tree.Source()))
}

func highlightRangesToUTF16(tree *Tree, ranges []HighlightRange) []UTF16HighlightRange {
	if tree == nil || len(ranges) == 0 {
		return nil
	}
	out := make([]UTF16HighlightRange, 0, len(ranges))
	for _, r := range ranges {
		utf16Range, ok := tree.UTF16RangeForByteRange(r.StartByte, r.EndByte)
		if !ok {
			continue
		}
		out = append(out, UTF16HighlightRange{
			StartCodeUnit: utf16Range.StartCodeUnit,
			EndCodeUnit:   utf16Range.EndCodeUnit,
			StartPoint:    utf16Range.StartPoint,
			EndPoint:      utf16Range.EndPoint,
			Capture:       r.Capture,
			PatternIndex:  r.PatternIndex,
		})
	}
	return out
}

func (h *Highlighter) highlightTree(tree *Tree, source []byte) []HighlightRange {
	matches := h.query.executeNodeIntoBuffer(tree.RootNode(), tree.Language(), source, &h.execBuffer)
	if len(matches) == 0 && h.injectionQuery == nil {
		return nil
	}

	ranges := h.rangeBuffer[:0]
	// Most highlight patterns have exactly one capture per match, so pre-sizing
	// to len(matches) avoids early growth when reusing a smaller prior buffer.
	if cap(ranges) < len(matches) {
		ranges = make([]HighlightRange, 0, len(matches))
	}
	for _, m := range matches {
		for _, c := range m.Captures {
			node := c.Node
			if node.StartByte() == node.EndByte() {
				continue
			}
			ranges = append(ranges, HighlightRange{
				StartByte:    node.StartByte(),
				EndByte:      node.EndByte(),
				Capture:      c.Name,
				PatternIndex: m.PatternIndex,
			})
		}
	}

	ranges = h.appendInjectedRanges(tree, source, ranges)
	h.rangeBuffer = ranges[:0]

	if len(ranges) == 0 {
		return nil
	}

	resolved := resolveOverlapsInto(ranges, h.resolvedBuffer[:0])
	h.resolvedBuffer = resolved[:0]

	out := make([]HighlightRange, len(resolved))
	copy(out, resolved)
	return out
}

// resolveOverlaps takes a range list (in any order) and returns a sorted,
// non-overlapping slice where inner (narrower) captures take priority over
// outer (wider) ones.
//
// Algorithm:
//  1. Sort ranges by start asc, width desc.
//  2. Sweep boundaries with a stack of active nested ranges.
//     The top of the stack is the currently active innermost capture.
//
// This avoids the previous second O(n log n) event sort.
func resolveOverlaps(ranges []HighlightRange) []HighlightRange {
	return resolveOverlapsInto(ranges, nil)
}

func resolveOverlapsInto(ranges []HighlightRange, dst []HighlightRange) []HighlightRange {
	if len(ranges) == 0 {
		return dst[:0]
	}

	sorted := compactNonEmptyHighlightRanges(ranges)
	if len(sorted) == 0 {
		return nil
	}
	sortHighlightRanges(sorted)

	resolver := highlightOverlapResolver{
		ranges: sorted,
		stack:  make([]HighlightRange, 0, 8),
		result: prepareHighlightResult(dst, len(sorted)),
		curPos: sorted[0].StartByte,
	}
	return resolver.resolve()
}

func compactNonEmptyHighlightRanges(ranges []HighlightRange) []HighlightRange {
	n := 0
	for i := range ranges {
		if ranges[i].EndByte > ranges[i].StartByte {
			ranges[n] = ranges[i]
			n++
		}
	}
	return ranges[:n]
}

func sortHighlightRanges(ranges []HighlightRange) {
	slices.SortFunc(ranges, func(a, b HighlightRange) int {
		if c := cmp.Compare(a.StartByte, b.StartByte); c != 0 {
			return c
		}
		wa := a.EndByte - a.StartByte
		wb := b.EndByte - b.StartByte
		if c := cmp.Compare(wb, wa); c != 0 { // wider first
			return c
		}
		return cmp.Compare(a.PatternIndex, b.PatternIndex)
	})
}

func prepareHighlightResult(dst []HighlightRange, capacity int) []HighlightRange {
	result := dst[:0]
	if cap(result) < capacity {
		return make([]HighlightRange, 0, capacity)
	}
	return result
}

type highlightOverlapResolver struct {
	ranges       []HighlightRange
	stack        []HighlightRange
	result       []HighlightRange
	curPos       uint32
	nextStartIdx int
}

func (r *highlightOverlapResolver) resolve() []HighlightRange {
	for r.hasPendingEvents() {
		nextPos := r.nextBoundary()
		r.emitActiveUntil(nextPos)
		r.popEndedRanges()
		r.pushStartingRanges()
		r.skipInactiveGap()
		r.clampToActiveRange()
	}
	return r.result
}

func (r *highlightOverlapResolver) hasPendingEvents() bool {
	return r.nextStartIdx < len(r.ranges) || len(r.stack) > 0
}

func (r *highlightOverlapResolver) nextBoundary() uint32 {
	nextStart := noHighlightBoundary
	if r.nextStartIdx < len(r.ranges) {
		nextStart = r.ranges[r.nextStartIdx].StartByte
	}
	nextEnd := noHighlightBoundary
	if len(r.stack) > 0 {
		nextEnd = r.stack[len(r.stack)-1].EndByte
	}
	return minHighlightBoundary(nextStart, nextEnd)
}

const noHighlightBoundary = ^uint32(0)

func minHighlightBoundary(a, b uint32) uint32 {
	if b < a {
		return b
	}
	return a
}

func (r *highlightOverlapResolver) emitActiveUntil(nextPos uint32) {
	if len(r.stack) > 0 && nextPos > r.curPos {
		r.emit(r.curPos, nextPos, r.stack[len(r.stack)-1].Capture)
		r.curPos = nextPos
		return
	}
	if r.curPos < nextPos {
		r.curPos = nextPos
	}
}

func (r *highlightOverlapResolver) emit(start, end uint32, capture string) {
	if capture == "" || end <= start {
		return
	}
	if n := len(r.result); n > 0 && r.result[n-1].Capture == capture && r.result[n-1].EndByte == start {
		r.result[n-1].EndByte = end
		return
	}
	r.result = append(r.result, HighlightRange{StartByte: start, EndByte: end, Capture: capture})
}

func (r *highlightOverlapResolver) popEndedRanges() {
	for len(r.stack) > 0 && r.stack[len(r.stack)-1].EndByte <= r.curPos {
		r.stack = r.stack[:len(r.stack)-1]
	}
}

func (r *highlightOverlapResolver) pushStartingRanges() {
	for r.nextStartIdx < len(r.ranges) && r.ranges[r.nextStartIdx].StartByte == r.curPos {
		r.stack = append(r.stack, r.ranges[r.nextStartIdx])
		r.nextStartIdx++
	}
}

func (r *highlightOverlapResolver) skipInactiveGap() {
	if len(r.stack) == 0 && r.nextStartIdx < len(r.ranges) && r.curPos < r.ranges[r.nextStartIdx].StartByte {
		r.curPos = r.ranges[r.nextStartIdx].StartByte
	}
}

func (r *highlightOverlapResolver) clampToActiveRange() {
	if len(r.stack) == 0 {
		return
	}
	if r.curPos < r.stack[len(r.stack)-1].StartByte {
		r.curPos = r.stack[len(r.stack)-1].StartByte
	}
	if r.curPos > r.stack[len(r.stack)-1].EndByte {
		r.popStaleActiveRanges()
	}
}

func (r *highlightOverlapResolver) popStaleActiveRanges() {
	for len(r.stack) > 0 && r.curPos >= r.stack[len(r.stack)-1].EndByte {
		r.stack = r.stack[:len(r.stack)-1]
	}
}

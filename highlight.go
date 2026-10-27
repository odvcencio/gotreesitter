package gotreesitter

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
	admissionRoute     *bool
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

// WithHighlighterTimeoutMicros bounds every full and incremental parse
// performed by the highlighter. A value of zero disables timeout checks.
func WithHighlighterTimeoutMicros(timeoutMicros uint64) HighlighterOption {
	return func(h *Highlighter) {
		h.parser.SetTimeoutMicros(timeoutMicros)
	}
}

// WithHighlighterAdmissionCandidateRoute pins the route for the document parser
// and injected-language parsers. The compact route still obeys eligibility checks.
func WithHighlighterAdmissionCandidateRoute(enabled bool) HighlighterOption {
	return func(h *Highlighter) {
		h.admissionRoute = &enabled
		h.parser.SetAdmissionCandidateRoute(enabled)
	}
}

// NewHighlighter creates a Highlighter for the given language and highlight
// query (in tree-sitter .scm format). Returns an error if the query fails
// to compile.
//
// Only captures that name a color become ranges. The highlighter does not
// return private captures (a leading underscore), @spell, @nospell, @conceal, or
// the directives of a locals query (@local.scope, @local.reference,
// @local.definition, @local.definition-value and @local.definition.*).
// Predicates and directives still read them. @none is a reset: it clears the
// colors beneath its span and is not returned either. (#set! priority N) sets the
// priority of a pattern's captures, 100 by default, and a higher priority wins
// over a lower one even when the lower capture is the inner one. These follow the
// conventions of Neovim's treesitter highlighter, which many shipped queries use.
func NewHighlighter(lang *Language, highlightQuery string, opts ...HighlighterOption) (*Highlighter, error) {
	q, err := NewQuery(highlightQuery, lang)
	if err != nil {
		return nil, err
	}
	disableNonHighlightCaptures(q)

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
// incremental calls). Call oldTree.Edit() before calling this: see
// ParseIncremental's doc comment for what happens to a length-changing
// source when that call is skipped.
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

// HighlightIncrementalStrict is like HighlightIncremental, but reports
// ErrParseStoppedEarly and skips query execution when parsing is stopped by a
// timeout, cancellation, token-source EOF, or parser safety limit. The partial
// tree is returned so callers can release it or use it for diagnostics.
func (h *Highlighter) HighlightIncrementalStrict(source []byte, oldTree *Tree) ([]HighlightRange, *Tree, error) {
	if len(source) == 0 {
		return nil, NewTree(nil, source, h.lang), nil
	}

	tree := h.parse(source, oldTree)
	if err := parseStoppedEarlyError(tree); err != nil {
		return nil, tree, err
	}
	if tree.RootNode() == nil {
		return nil, tree, nil
	}

	return h.highlightTree(tree, source), tree, nil
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
// a slice of HighlightRange sorted by StartByte. When ranges overlap, the
// higher (#set! priority N) wins; at equal priority the inner (narrower) capture
// wins. Of two captures on one span, the injected layer wins over its host, then
// the later pattern. See NewHighlighter for the captures that are not returned.
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

	tiers := highlightTiers{base: h.rangeBuffer[:0]}
	// Most highlight patterns have exactly one capture per match, so pre-sizing
	// to len(matches) avoids early growth when reusing a smaller prior buffer.
	if cap(tiers.base) < len(matches) {
		tiers.base = make([]HighlightRange, 0, len(matches))
	}
	tiers.addMatches(h.query, matches, 0)
	h.appendInjectedRanges(tree, source, &tiers)
	h.rangeBuffer = tiers.base[:0]

	if tiers.empty() {
		return nil
	}
	resolved := tiers.resolve(h.resolvedBuffer[:0])
	h.resolvedBuffer = resolved[:0]

	return withoutHighlightResets(resolved)
}

// withoutHighlightResets copies ranges and leaves out the reset captures, which
// clear the colors beneath them but are not colors. It returns nil when no range
// is left.
func withoutHighlightResets(ranges []HighlightRange) []HighlightRange {
	out := make([]HighlightRange, 0, len(ranges))
	for _, r := range ranges {
		if r.Capture != highlightResetCapture {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// defaultHighlightPriority is the priority of a pattern that sets none. It is
// the default of Neovim, whose (#set! priority N) directive the query files use.
const defaultHighlightPriority = 100

// highlightResetCapture clears the colors beneath its span. It takes part in
// resolution like any capture, and the result leaves it out.
const highlightResetCapture = "none"

// isNonHighlightCapture reports whether a capture name never colors text:
// a private helper (leading underscore), a spell-check or conceal marker, or a
// directive of a locals query. Only the locals vocabulary is dropped, because
// other dotted names that start with "local." are real highlight names; the Elm
// query captures @local.function.elm, for one.
func isNonHighlightCapture(name string) bool {
	switch name {
	case "spell", "nospell", "conceal",
		"local.scope", "local.reference", "local.definition", "local.definition-value":
		return true
	}
	return strings.HasPrefix(name, "_") || strings.HasPrefix(name, "local.definition.")
}

// disableNonHighlightCaptures drops every capture of q that never colors text.
// DisableCapture filters only a finished match, so predicates and directives
// still read the dropped captures.
func disableNonHighlightCaptures(q *Query) {
	for _, name := range q.captures {
		if isNonHighlightCapture(name) {
			q.DisableCapture(name)
		}
	}
}

// highlightPatternPriorities returns the priority of each pattern of q, or nil
// when no pattern sets one. The last (#set! priority N) directive of a pattern
// decides; a value that is not an integer leaves the default.
func highlightPatternPriorities(q *Query) []int32 {
	var priorities []int32
	for pattern := range q.patterns {
		priority, set := int32(defaultHighlightPriority), false
		for i := range q.patterns[pattern].predicates {
			pred := &q.patterns[pattern].predicates[i]
			if pred.kind != predicateSet || pred.literal != "priority" || len(pred.values) == 0 {
				continue
			}
			set = true
			priority = defaultHighlightPriority
			if n, err := strconv.ParseInt(pred.values[0], 10, 32); err == nil {
				priority = int32(n)
			}
		}
		if !set || priority == defaultHighlightPriority {
			continue
		}
		if priorities == nil {
			priorities = make([]int32, len(q.patterns))
			for j := range priorities {
				priorities[j] = defaultHighlightPriority
			}
		}
		priorities[pattern] = priority
	}
	return priorities
}

// highlightTier holds the ranges of the patterns that share one priority.
type highlightTier struct {
	priority int32
	ranges   []HighlightRange
}

// highlightTiers collects ranges by the priority of their pattern. The ranges of
// patterns that set no priority go to base, which keeps the highlighter's reusable
// buffer; the few other priorities allocate.
type highlightTiers struct {
	base   []HighlightRange
	others []highlightTier
}

func (t *highlightTiers) empty() bool {
	if len(t.base) > 0 {
		return false
	}
	for i := range t.others {
		if len(t.others[i].ranges) > 0 {
			return false
		}
	}
	return true
}

func (t *highlightTiers) add(priority int32, r HighlightRange) {
	if priority == defaultHighlightPriority {
		t.base = append(t.base, r)
		return
	}
	for i := range t.others {
		if t.others[i].priority == priority {
			t.others[i].ranges = append(t.others[i].ranges, r)
			return
		}
	}
	t.others = append(t.others, highlightTier{priority: priority, ranges: []HighlightRange{r}})
}

// addMatches adds one range for each non-empty capture of matches, which q
// produced. patternBase is added to each pattern index.
func (t *highlightTiers) addMatches(q *Query, matches []QueryMatch, patternBase int) {
	priorities := highlightPatternPriorities(q)
	for i := range matches {
		match := &matches[i]
		priority := int32(defaultHighlightPriority)
		if priorities != nil {
			priority = priorities[match.PatternIndex]
		}
		for _, c := range match.Captures {
			startByte, endByte := c.ByteRange()
			if endByte <= startByte {
				continue
			}
			t.add(priority, HighlightRange{
				StartByte:    startByte,
				EndByte:      endByte,
				Capture:      c.Name,
				PatternIndex: patternBase + match.PatternIndex,
			})
		}
	}
}

// merge adds the ranges of an injected layer, moved up by shift bytes. Its
// pattern indexes start at patternBase, so on a span that host and layer both
// capture, the layer's capture is the later one and wins.
func (t *highlightTiers) merge(layer *highlightTiers, shift uint32, patternBase int) {
	move := func(r HighlightRange) HighlightRange {
		r.StartByte += shift
		r.EndByte += shift
		r.PatternIndex += patternBase
		return r
	}
	for _, r := range layer.base {
		t.add(defaultHighlightPriority, move(r))
	}
	for _, tier := range layer.others {
		for _, r := range tier.ranges {
			t.add(tier.priority, move(r))
		}
	}
}

// clip keeps the ranges that lie inside [lo, hi) and moves them down by lo.
func (t *highlightTiers) clip(lo, hi uint32) {
	t.base = clipHighlightRanges(t.base, lo, hi)
	for i := range t.others {
		t.others[i].ranges = clipHighlightRanges(t.others[i].ranges, lo, hi)
	}
}

func clipHighlightRanges(ranges []HighlightRange, lo, hi uint32) []HighlightRange {
	kept := ranges[:0]
	for _, r := range ranges {
		if r.StartByte < lo || r.EndByte > hi {
			continue
		}
		r.StartByte -= lo
		r.EndByte -= lo
		kept = append(kept, r)
	}
	return kept
}

// resolve turns the ranges into sorted, non-overlapping ranges. The innermost
// capture wins within one priority; a higher priority replaces the lower ones
// wherever it applies. scratch is reusable space for the common case of a single
// priority.
func (t *highlightTiers) resolve(scratch []HighlightRange) []HighlightRange {
	if len(t.others) == 0 {
		return resolveOverlapsInto(t.base, scratch)
	}
	tiers := make([]highlightTier, 0, len(t.others)+1)
	tiers = append(tiers, highlightTier{priority: defaultHighlightPriority, ranges: t.base})
	tiers = append(tiers, t.others...)
	slices.SortFunc(tiers, func(a, b highlightTier) int { return cmp.Compare(a.priority, b.priority) })

	var resolved []HighlightRange
	for i, tier := range tiers {
		layer := resolveOverlapsInto(tier.ranges, nil)
		if i == 0 {
			resolved = layer
		} else {
			resolved = overlayHighlightRanges(resolved, layer)
		}
	}
	return resolved
}

// overlayHighlightRanges returns lower with every byte that upper covers given
// to upper. Both inputs are sorted and non-overlapping, and so is the result.
func overlayHighlightRanges(lower, upper []HighlightRange) []HighlightRange {
	out := make([]HighlightRange, 0, len(lower)+len(upper))
	next := 0          // the first upper range not yet written
	var covered uint32 // the end of the last upper range written
	for _, l := range lower {
		start := max(l.StartByte, covered)
		for start < l.EndByte {
			for next < len(upper) && upper[next].EndByte <= start {
				out = appendHighlightSegment(out, upper[next])
				next++
			}
			if next == len(upper) || upper[next].StartByte >= l.EndByte {
				out = appendHighlightSegment(out, HighlightRange{StartByte: start, EndByte: l.EndByte, Capture: l.Capture})
				break
			}
			if upper[next].StartByte > start {
				out = appendHighlightSegment(out, HighlightRange{StartByte: start, EndByte: upper[next].StartByte, Capture: l.Capture})
			}
			out = appendHighlightSegment(out, upper[next])
			start = upper[next].EndByte
			covered = start
			next++
		}
	}
	for ; next < len(upper); next++ {
		out = appendHighlightSegment(out, upper[next])
	}
	return out
}

// appendHighlightSegment appends r, joining it to the last range when they touch
// and share a capture.
func appendHighlightSegment(dst []HighlightRange, r HighlightRange) []HighlightRange {
	if r.EndByte <= r.StartByte {
		return dst
	}
	if n := len(dst); n > 0 && dst[n-1].Capture == r.Capture && dst[n-1].EndByte == r.StartByte {
		dst[n-1].EndByte = r.EndByte
		return dst
	}
	return append(dst, HighlightRange{StartByte: r.StartByte, EndByte: r.EndByte, Capture: r.Capture})
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
	slices.SortStableFunc(ranges, func(a, b HighlightRange) int {
		if c := cmp.Compare(a.StartByte, b.StartByte); c != 0 {
			return c
		}
		wa := a.EndByte - a.StartByte
		wb := b.EndByte - b.StartByte
		if c := cmp.Compare(wb, wa); c != 0 { // wider first
			return c
		}
		// Spell captures do not color text. Give a real highlight the
		// last position in an identical-span stack, across all patterns.
		if a.Capture == "spell" && b.Capture != "spell" {
			return -1
		}
		if b.Capture == "spell" && a.Capture != "spell" {
			return 1
		}
		// Later patterns override earlier ones. A stable sort preserves
		// source capture order when the pattern and span are identical.
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

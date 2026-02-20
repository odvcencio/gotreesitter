package gotreesitter

import "sort"

// HighlightRange represents a styled range of source code, mapping a byte span
// to a capture name from a highlight query. The editor maps capture names
// (e.g., "keyword", "string", "function") to FSS style classes.
type HighlightRange struct {
	StartByte uint32
	EndByte   uint32
	Capture   string // "keyword", "string", "function", etc.
}

// Highlighter is a high-level API that takes source code and returns styled
// ranges. It combines a Parser, a compiled Query, and a Language to provide
// a single Highlight() call for the editor.
type Highlighter struct {
	parser             *Parser
	query              *Query
	lang               *Language
	tokenSourceFactory func(source []byte) TokenSource
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

	return h.highlightTree(tree), tree
}

// Highlight parses the source code and executes the highlight query, returning
// a slice of HighlightRange sorted by StartByte. When ranges overlap, inner
// (more specific) captures take priority over outer ones.
func (h *Highlighter) Highlight(source []byte) []HighlightRange {
	if len(source) == 0 {
		return nil
	}

	tree := h.parse(source, nil)

	if tree.RootNode() == nil {
		return nil
	}

	return h.highlightTree(tree)
}

func (h *Highlighter) parse(source []byte, oldTree *Tree) *Tree {
	if h.tokenSourceFactory != nil {
		ts := h.tokenSourceFactory(source)
		if oldTree != nil {
			return h.parser.ParseIncrementalWithTokenSource(source, oldTree, ts)
		}
		return h.parser.ParseWithTokenSource(source, ts)
	}
	if oldTree != nil {
		return h.parser.ParseIncremental(source, oldTree)
	}
	return h.parser.Parse(source)
}

func (h *Highlighter) highlightTree(tree *Tree) []HighlightRange {
	matches := h.query.Execute(tree)
	if len(matches) == 0 {
		return nil
	}

	var ranges []HighlightRange
	for _, m := range matches {
		for _, c := range m.Captures {
			node := c.Node
			if node.StartByte() == node.EndByte() {
				continue
			}
			ranges = append(ranges, HighlightRange{
				StartByte: node.StartByte(),
				EndByte:   node.EndByte(),
				Capture:   c.Name,
			})
		}
	}

	if len(ranges) == 0 {
		return nil
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].StartByte != ranges[j].StartByte {
			return ranges[i].StartByte < ranges[j].StartByte
		}
		wi := ranges[i].EndByte - ranges[i].StartByte
		wj := ranges[j].EndByte - ranges[j].StartByte
		return wi > wj
	})

	return resolveOverlaps(ranges)
}

// resolveOverlaps takes a sorted slice of ranges (sorted by StartByte asc,
// span width desc) and returns a non-overlapping slice where inner (narrower)
// captures take priority over outer (wider) ones.
//
// Algorithm: sweep through byte positions, maintaining a stack of active
// outer ranges. When an inner range is encountered, it replaces the outer
// range for its span, and the outer range continues after the inner one ends.
func resolveOverlaps(ranges []HighlightRange) []HighlightRange {
	if len(ranges) == 0 {
		return nil
	}

	// Use an event-based approach: for each range, create start/end events,
	// then sweep through them. The most recently started (innermost) range
	// is the active one at any point.

	type event struct {
		pos     uint32
		isStart bool
		idx     int // index into ranges
	}

	events := make([]event, 0, len(ranges)*2)
	for i := range ranges {
		events = append(events,
			event{pos: ranges[i].StartByte, isStart: true, idx: i},
			event{pos: ranges[i].EndByte, isStart: false, idx: i},
		)
	}

	// Sort events: by position, then ends before starts at same position,
	// then for starts at the same position, narrower ranges (higher index
	// since ranges are sorted wider-first) come after wider ones.
	sort.Slice(events, func(i, j int) bool {
		if events[i].pos != events[j].pos {
			return events[i].pos < events[j].pos
		}
		// At same position: ends before starts.
		if events[i].isStart != events[j].isStart {
			return !events[i].isStart // end events first
		}
		if events[i].isStart {
			// Both starts: wider (lower index) first so it's pushed onto
			// the stack first, and narrower is on top (takes priority).
			return events[i].idx < events[j].idx
		}
		// Both ends: narrower (higher index) ends first.
		return events[i].idx > events[j].idx
	})

	// Sweep through events maintaining a stack of active range indices.
	// The top of the stack is the currently active (innermost) capture.
	type stackEntry struct {
		idx int
	}
	var stack []stackEntry
	active := make([]bool, len(ranges)) // which range indices are currently active

	var result []HighlightRange
	var lastPos uint32
	var lastCapture string
	hasLast := false

	flushSegment := func(endPos uint32) {
		if hasLast && endPos > lastPos && lastCapture != "" {
			result = append(result, HighlightRange{
				StartByte: lastPos,
				EndByte:   endPos,
				Capture:   lastCapture,
			})
		}
	}

	for _, ev := range events {
		if ev.pos > lastPos && hasLast {
			flushSegment(ev.pos)
		}

		if ev.isStart {
			stack = append(stack, stackEntry{idx: ev.idx})
			active[ev.idx] = true
		} else {
			active[ev.idx] = false
			// Pop inactive entries from top of stack.
			for len(stack) > 0 && !active[stack[len(stack)-1].idx] {
				stack = stack[:len(stack)-1]
			}
		}

		// Determine current capture from top of active stack.
		lastPos = ev.pos
		lastCapture = ""
		hasLast = true
		for i := len(stack) - 1; i >= 0; i-- {
			if active[stack[i].idx] {
				lastCapture = ranges[stack[i].idx].Capture
				break
			}
		}
	}

	return result
}

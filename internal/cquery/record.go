package cquery

import (
	"strconv"
	"strings"
)

// Layer names one level at which the port is compared with pinned C.
type Layer string

// The five layers of comparison, in the order the port is built.
const (
	LayerSteps    Layer = "L1"
	LayerAnalysis Layer = "L2"
	LayerMatches  Layer = "L3"
	LayerCaptures Layer = "L4"
	LayerLimits   Layer = "L5"
)

// Record is one record of a layer. String returns its stable text form, which
// the corpus stores. Two records are equal when their text forms are equal.
type Record interface {
	String() string
}

// NoStep is query.c's NONE (UINT16_MAX): no alternative step, no capture id.
const NoStep uint16 = 0xFFFF

// DoneDepth is query.c's PATTERN_DONE_MARKER: the depth of the step that ends
// a pattern.
const DoneDepth uint16 = 0xFFFF

// Capture is one captured node: the capture name, the node's type and named
// flag, and its byte range. It is the per-capture record of L3 and L4, with
// the fields that the harness's exactQueryMatch compares apart from text. The
// text form writes the name bare, because a capture name is a query
// identifier (letters, digits and _ - . ? !), and quotes the type, because
// an anonymous node's type can be any text.
type Capture struct {
	Name      string
	Type      string
	Named     bool
	StartByte uint32
	EndByte   uint32
}

// String returns the text form, for example `@k "string" named 1-4`.
func (c Capture) String() string {
	var b strings.Builder
	writeCapture(&b, c)
	return b.String()
}

func writeCapture(b *strings.Builder, c Capture) {
	b.WriteByte('@')
	b.WriteString(c.Name)
	b.WriteByte(' ')
	b.WriteString(strconv.Quote(c.Type))
	if c.Named {
		b.WriteString(" named ")
	} else {
		b.WriteString(" anon ")
	}
	b.WriteString(strconv.FormatUint(uint64(c.StartByte), 10))
	b.WriteByte('-')
	b.WriteString(strconv.FormatUint(uint64(c.EndByte), 10))
}

// Match is one L3 record: a finished match, with the index of its pattern and
// its captures in the order the engine returns them.
type Match struct {
	Pattern  int
	Captures []Capture
}

// String returns the text form, for example
// `p0 @k "string" named 1-4 @v "number" named 5-6`.
func (m Match) String() string {
	var b strings.Builder
	b.WriteByte('p')
	b.WriteString(strconv.Itoa(m.Pattern))
	for _, c := range m.Captures {
		b.WriteByte(' ')
		writeCapture(&b, c)
	}
	return b.String()
}

// StreamCapture is one L4 record: one element of the capture stream
// (ts_query_cursor_next_capture), with the pattern of the match it belongs to
// and the capture's position within that match.
type StreamCapture struct {
	Pattern int
	Index   int
	Capture Capture
}

// String returns the text form, for example `p0 #1 @k "string" named 1-4`.
func (s StreamCapture) String() string {
	var b strings.Builder
	b.WriteByte('p')
	b.WriteString(strconv.Itoa(s.Pattern))
	b.WriteString(" #")
	b.WriteString(strconv.Itoa(s.Index))
	b.WriteByte(' ')
	writeCapture(&b, s.Capture)
	return b.String()
}

// Symbol is a grammar symbol or field as a compiled query refers to it: its
// id in the language and the language's name for that id.
type Symbol struct {
	ID   uint16
	Name string
}

// String returns the text form, for example `12:"pair"`.
func (s Symbol) String() string {
	return strconv.Itoa(int(s.ID)) + ":" + strconv.Quote(s.Name)
}

// CaptureName is an L1 record: the name of one capture id.
type CaptureName struct {
	ID   int
	Name string
}

// String returns the text form, for example `capture 0 k`.
func (c CaptureName) String() string {
	var b strings.Builder
	b.WriteString("capture ")
	b.WriteString(strconv.Itoa(c.ID))
	b.WriteByte(' ')
	b.WriteString(c.Name)
	return b.String()
}

// Quantifier names a capture quantifier as ts_query_capture_quantifier_for_id
// reports it.
type Quantifier string

// The capture quantifiers of the C API.
const (
	QuantifierZero       Quantifier = "zero"
	QuantifierZeroOrOne  Quantifier = "zero_or_one"
	QuantifierZeroOrMore Quantifier = "zero_or_more"
	QuantifierOne        Quantifier = "one"
	QuantifierOneOrMore  Quantifier = "one_or_more"
)

// CaptureQuantifier is an L1 record: the quantifier of one capture in one
// pattern. The corpus lists only quantifiers other than zero.
type CaptureQuantifier struct {
	Pattern    int
	Capture    int
	Quantifier Quantifier
}

// String returns the text form, for example `quantifier p0 1 one_or_more`.
func (q CaptureQuantifier) String() string {
	return "quantifier p" + strconv.Itoa(q.Pattern) + " " + strconv.Itoa(q.Capture) + " " + string(q.Quantifier)
}

// Pattern is an L1 record: one QueryPattern, with its slice of the step list
// and its byte range in the query source.
type Pattern struct {
	Index      int
	StepOffset uint32
	StepCount  uint32
	StartByte  uint32
	EndByte    uint32
}

// String returns the text form, for example `pattern 0 steps=0+5 bytes=0-31`.
func (p Pattern) String() string {
	return "pattern " + strconv.Itoa(p.Index) +
		" steps=" + strconv.FormatUint(uint64(p.StepOffset), 10) + "+" + strconv.FormatUint(uint64(p.StepCount), 10) +
		" bytes=" + strconv.FormatUint(uint64(p.StartByte), 10) + "-" + strconv.FormatUint(uint64(p.EndByte), 10)
}

// PatternEntry is an L1 record: one entry of the pattern map, which says
// where a pattern starts and whether it has a single root node.
type PatternEntry struct {
	Index   int
	Step    int
	Pattern int
	Rooted  bool
}

// String returns the text form, for example `entry 0 step=1 pattern=0 rooted`.
func (e PatternEntry) String() string {
	s := "entry " + strconv.Itoa(e.Index) + " step=" + strconv.Itoa(e.Step) + " pattern=" + strconv.Itoa(e.Pattern)
	if e.Rooted {
		return s + " rooted"
	}
	return s + " unrooted"
}

// Step is an L1 record: one compiled QueryStep with its parse-time fields.
// The three analysis flags are in StepAnalysis (L2). A Symbol ID of 0 is the
// wildcard; a Supertype or Field ID of 0 means none.
type Step struct {
	Index             int
	Depth             uint16
	Symbol            Symbol
	Supertype         Symbol
	Field             Symbol
	CaptureIDs        []uint16
	AlternativeIndex  uint16
	NegatedFields     []Symbol
	Named             bool
	Immediate         bool
	LastChild         bool
	PassThrough       bool
	DeadEnd           bool
	InsideAlternation bool
	Missing           bool
	AlternativeIsSkip bool
}

// String returns the text form, for example
// `s1 depth=1 symbol=12:"pair" field=1:"key" captures=0 alternative=4 named`.
// Empty fields and false flags are left out.
func (s Step) String() string {
	var b strings.Builder
	b.WriteByte('s')
	b.WriteString(strconv.Itoa(s.Index))
	b.WriteString(" depth=")
	if s.Depth == DoneDepth {
		b.WriteString("done")
	} else {
		b.WriteString(strconv.Itoa(int(s.Depth)))
	}
	b.WriteString(" symbol=")
	if s.Symbol.ID == 0 {
		b.WriteByte('_')
	} else {
		b.WriteString(s.Symbol.String())
	}
	if s.Supertype.ID != 0 {
		b.WriteString(" supertype=")
		b.WriteString(s.Supertype.String())
	}
	if s.Field.ID != 0 {
		b.WriteString(" field=")
		b.WriteString(s.Field.String())
	}
	if len(s.CaptureIDs) > 0 {
		b.WriteString(" captures=")
		for i, id := range s.CaptureIDs {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Itoa(int(id)))
		}
	}
	if s.AlternativeIndex != NoStep {
		b.WriteString(" alternative=")
		b.WriteString(strconv.Itoa(int(s.AlternativeIndex)))
	}
	if len(s.NegatedFields) > 0 {
		b.WriteString(" negated=")
		for i, f := range s.NegatedFields {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(f.String())
		}
	}
	for _, flag := range []struct {
		on   bool
		name string
	}{
		{s.Named, "named"},
		{s.Immediate, "immediate"},
		{s.LastChild, "last_child"},
		{s.PassThrough, "pass_through"},
		{s.DeadEnd, "dead_end"},
		{s.InsideAlternation, "inside_alternation"},
		{s.Missing, "missing"},
		{s.AlternativeIsSkip, "alternative_is_skip"},
	} {
		if flag.on {
			b.WriteByte(' ')
			b.WriteString(flag.name)
		}
	}
	return b.String()
}

// StepOffset is an L1 record: the byte offset in the query source where a
// step starts, as the step_offsets table holds it.
type StepOffset struct {
	ByteOffset uint32
	Step       int
}

// String returns the text form, for example `offset 12 step=3`.
func (o StepOffset) String() string {
	return "offset " + strconv.FormatUint(uint64(o.ByteOffset), 10) + " step=" + strconv.Itoa(o.Step)
}

// Verdict is the first L2 record: whether C accepts the query and, if not,
// the error kind (the binding's QueryErrorKind name) and byte offset.
type Verdict struct {
	Accepted    bool
	ErrorKind   string
	ErrorOffset uint32
}

// String returns the text form: `accept`, or for example
// `reject kind=structure offset=12`.
func (v Verdict) String() string {
	if v.Accepted {
		return "accept"
	}
	return "reject kind=" + v.ErrorKind + " offset=" + strconv.FormatUint(uint64(v.ErrorOffset), 10)
}

// PatternAnalysis is an L2 record: the rooted and non-local flags of one
// pattern, as ts_query_is_pattern_rooted and ts_query_is_pattern_non_local
// report them.
type PatternAnalysis struct {
	Pattern  int
	Rooted   bool
	NonLocal bool
}

// String returns the text form, for example `pattern 0 rooted local`.
func (p PatternAnalysis) String() string {
	s := "pattern " + strconv.Itoa(p.Pattern)
	if p.Rooted {
		s += " rooted"
	} else {
		s += " unrooted"
	}
	if p.NonLocal {
		return s + " non_local"
	}
	return s + " local"
}

// StepAnalysis is an L2 record: the three flags that ts_query__analyze_patterns
// sets on one step.
type StepAnalysis struct {
	Step                    int
	ContainsCaptures        bool
	RootPatternGuaranteed   bool
	ParentPatternGuaranteed bool
}

// String returns the text form, for example
// `s3 contains_captures root_guaranteed`, or `s3 -` when no flag is set.
func (s StepAnalysis) String() string {
	var b strings.Builder
	b.WriteByte('s')
	b.WriteString(strconv.Itoa(s.Step))
	n := b.Len()
	if s.ContainsCaptures {
		b.WriteString(" contains_captures")
	}
	if s.RootPatternGuaranteed {
		b.WriteString(" root_guaranteed")
	}
	if s.ParentPatternGuaranteed {
		b.WriteString(" parent_guaranteed")
	}
	if b.Len() == n {
		b.WriteString(" -")
	}
	return b.String()
}

// RepeatSymbol is an L2 record: one symbol of
// repeat_symbols_with_rootless_patterns, which the cursor uses to decide
// whether to descend into a node.
type RepeatSymbol struct {
	Symbol Symbol
}

// String returns the text form, for example `repeat 40:"block_repeat1"`.
func (r RepeatSymbol) String() string { return "repeat " + r.Symbol.String() }

// Point is a row and column in the input, as tree-sitter counts them.
type Point struct {
	Row    uint32
	Column uint32
}

// LimitRun is the first record of an L5 run: the cursor settings and whether
// C reported that the run exceeded its capture-list limit. The run's match
// records follow it. A nil range or depth, and a MatchLimit of 0, mean that
// the setting is off.
type LimitRun struct {
	ByteRange     *[2]uint32
	PointRange    *[2]Point
	MaxStartDepth *uint32
	MatchLimit    uint32
	Exceeded      bool
}

// String returns the text form, for example
// `limits bytes=10-20 max_start_depth=1 match_limit=4 exceeded`.
func (l LimitRun) String() string {
	var b strings.Builder
	b.WriteString("limits")
	if l.ByteRange != nil {
		b.WriteString(" bytes=")
		b.WriteString(strconv.FormatUint(uint64(l.ByteRange[0]), 10))
		b.WriteByte('-')
		b.WriteString(strconv.FormatUint(uint64(l.ByteRange[1]), 10))
	}
	if l.PointRange != nil {
		b.WriteString(" points=")
		for i, p := range l.PointRange {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteString(strconv.FormatUint(uint64(p.Row), 10))
			b.WriteByte(':')
			b.WriteString(strconv.FormatUint(uint64(p.Column), 10))
		}
	}
	if l.MaxStartDepth != nil {
		b.WriteString(" max_start_depth=")
		b.WriteString(strconv.FormatUint(uint64(*l.MaxStartDepth), 10))
	}
	if l.MatchLimit != 0 {
		b.WriteString(" match_limit=")
		b.WriteString(strconv.FormatUint(uint64(l.MatchLimit), 10))
	}
	if l.Exceeded {
		b.WriteString(" exceeded")
	}
	return b.String()
}

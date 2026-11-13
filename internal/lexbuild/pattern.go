package lexbuild

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// hirKind is the discriminant of regex_syntax::hir::HirKind.
type hirKind uint8

const (
	hirEmpty hirKind = iota
	hirLiteral
	hirClass
	hirRepetition
	hirCapture
	hirConcat
	hirAlternation
	hirLook // an assertion; the generator rejects those that survive simplification (expand_tokens.rs)
)

// hir is regex_syntax::hir::Hir reduced to the kinds that expand_tokens.rs
// reads. Look (assertions) is refused at parse time.
type hir struct {
	kind     hirKind
	lit      []rune     // hirLiteral
	class    []classRng // hirClass: sorted, disjoint, non-adjacent, inclusive; none for Hir::fail
	min, max uint32     // hirRepetition; hasMax tells whether max is set
	hasMax   bool
	lazy     bool   // hirRepetition: not greedy (after the U flag is applied)
	sub      *hir   // hirRepetition, hirCapture
	subs     []*hir // hirConcat, hirAlternation
	capIdx   int    // hirCapture: index in order of the opening parenthesis
	capName  string // hirCapture: name, if any
	zl       int8   // cached hirZeroLen: 0 unknown, 1 yes, 2 no
}

type classRng struct{ lo, hi uint32 } // inclusive

// nestLimit is the default nest limit of regex_syntax (ast::parse: ParserBuilder).
const nestLimit = 250

// pattern.rs: fn parse. The generator parses a pattern with the Rust crate
// regex_syntax 0.8.11 (AST parser, then the Translator with Unicode on, UTF-8
// off and case folding off), after an Expander pass that folds case. This
// parser follows the AST parser (ast/parse.rs) and the Translator
// (hir/translate.rs) for the constructs it accepts: literals and escapes,
// classes with ranges, nesting, negation and the set operations, \p and \P for
// general categories and binary properties, groups (capturing, named,
// non-capturing, with flags), the flags i s m U u R, repetitions, alternation,
// and the case folding of the Expander.
//
// It returns an error for what the generator rejects (assertions, look-around,
// a repetition without an expression, unclosed constructs, a syntax tree
// nested deeper than 250) and a NotPortedError for what it does not read yet:
// the flags x and -u, ASCII classes ([[:alpha:]]), \p for scripts,
// Script_Extensions, Age and the segmentation properties.
//
// The depth of the syntax tree is computed as the Rust NestLimiter counts it
// (ast/parse.rs: struct NestLimiter): every group, repetition, alternation or
// concatenation of two or more items, bracketed class, union of two or more
// class items and class set operation is one level.
//
// Differences from the Rust syntax tree that cannot change a result: the Rust
// Hir carries properties (look sets, the UTF-8 and literal flags) that this
// code does not keep. The only property that a simplification reads and that
// reaches the output is the maximum match length being zero (Hir::repetition),
// which is reproduced.
func parsePattern(pattern string, caseInsensitive bool) (*hir, error) {
	h, _, err := parsePatternWith(pattern, caseInsensitive)
	return h, err
}

// parsePatternCounting is parsePattern with a count of class ranges shared by
// all the patterns of one grammar (bPatternRanges).
func parsePatternCounting(pattern string, caseInsensitive bool, total *int) (*hir, error) {
	h, _, err := parsePatternShared(pattern, caseInsensitive, total)
	return h, err
}

// parsePatternWith is parsePattern; it also returns the parser, so that a test
// can read how far the parser got (pos) and how many class set operations it
// evaluated (classOps).
func parsePatternWith(pattern string, caseInsensitive bool) (*hir, *patParser, error) {
	return parsePatternShared(pattern, caseInsensitive, new(int))
}

func parsePatternShared(pattern string, caseInsensitive bool, total *int) (*hir, *patParser, error) {
	p := &patParser{src: []rune(pattern), orig: pattern, capNames: map[string]bool{}, total: total}
	p.flags.i = caseInsensitive
	h, ht, err := p.parseAlternation()
	if err != nil {
		return nil, p, err
	}
	if p.pos < len(p.src) {
		return nil, p, fmt.Errorf("pattern parse error: unmatched closing parenthesis in %q", pattern)
	}
	if ht > nestLimit {
		return nil, p, p.nestErr()
	}
	return h, p, nil
}

// patFlags are the flags in force at a point of the pattern: i (case
// insensitive, applied by the Expander of pattern.rs), s (dot matches a line
// feed), R (CRLF mode for dot) and U (swap greed).
type patFlags struct{ i, s, crlf, swapGreed bool }

type patParser struct {
	flags    patFlags
	src      []rune
	pos      int
	depth    int // recursion depth of groups and classes (stack guard; deeper than the limit is an error anyway)
	orig     string
	caps     int
	capNames map[string]bool
	classOps int  // class set operations evaluated (a test reads it)
	total    *int // character ranges read into classes and escapes, over the patterns of the grammar (bPatternRanges)
}

func (p *patParser) more() bool { return p.pos < len(p.src) }
func (p *patParser) peek() rune { return p.src[p.pos] }

// noteRanges counts n character ranges against bPatternRanges before they are kept.
func (p *patParser) noteRanges(n int) {
	*p.total += n
	bPatternRanges.check(*p.total)
}

func (p *patParser) nestErr() error {
	ops.errBytes.add(len(p.orig))
	return fmt.Errorf("pattern parse error: exceeded the nest limit (%d) in %q", nestLimit, p.orig)
}

func (p *patParser) notPorted(what string) error {
	ops.errBytes.add(len(p.orig))
	return &NotPortedError{Construct: what, Where: "pattern " + strconv.Quote(p.orig)}
}

func (p *patParser) errf(format string, a ...any) error {
	ops.errBytes.add(len(p.orig))
	return fmt.Errorf("pattern parse error: "+format+" in %q", append(a, p.orig)...)
}

func max1(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// parseAlternation parses branches separated by |, then builds the result with
// the rules of regex_syntax::hir::Hir::alternation. The second result is the
// height of the syntax tree node.
func (p *patParser) parseAlternation() (*hir, int, error) {
	var branches []*hir
	h := 0
	for {
		b, bh, err := p.parseConcat()
		if err != nil {
			return nil, 0, err
		}
		branches = append(branches, b)
		h = max1(h, bh)
		if p.more() && p.peek() == '|' {
			p.pos++
			continue
		}
		break
	}
	if len(branches) == 1 {
		return branches[0], h, nil
	}
	if h+1 > nestLimit {
		return nil, 0, p.nestErr()
	}
	r, err := hirAlternationOf(branches, p)
	return r, h + 1, err
}

// parseConcat parses repetitions until | or ) and builds with the rules of
// regex_syntax: Hir::concat.
func (p *patParser) parseConcat() (*hir, int, error) {
	var items []*hir
	h := 0
	for p.more() && p.peek() != '|' && p.peek() != ')' {
		atom, ah, err := p.parseRepetition()
		if err != nil {
			return nil, 0, err
		}
		items = append(items, atom)
		h = max1(h, ah)
	}
	if len(items) >= 2 {
		h++
	}
	if h > nestLimit {
		return nil, 0, p.nestErr()
	}
	return hirConcatOf(items), h, nil
}

// parseRepetition parses an atom and its repetition operators (ast/parse.rs:
// parse_uncounted_repetition, parse_counted_repetition).
func (p *patParser) parseRepetition() (*hir, int, error) {
	atom, h, isFlags, err := p.parseAtom()
	if err != nil {
		return nil, 0, err
	}
	for p.more() {
		var min, max uint32
		hasMax := false
		switch p.peek() {
		case '*', '+', '?':
			if isFlags {
				return nil, 0, p.errf("repetition operator missing expression")
			}
			switch p.peek() {
			case '*':
				min = 0
			case '+':
				min = 1
			default:
				min, max, hasMax = 0, 1, true
			}
			p.pos++
		case '{':
			if isFlags {
				return nil, 0, p.errf("repetition operator missing expression")
			}
			m, mx, hm, err := p.parseCounted()
			if err != nil {
				return nil, 0, err
			}
			min, max, hasMax = m, mx, hm
		default:
			return atom, h, nil
		}
		lazy := false
		if p.more() && p.peek() == '?' { // lazy: no effect on the NFA, but part of Hir equality
			p.pos++
			lazy = true
		}
		if p.flags.swapGreed {
			lazy = !lazy
		}
		h++
		if h > nestLimit {
			return nil, 0, p.nestErr()
		}
		atom = hirRepetitionOf(atom, min, max, hasMax, lazy)
		isFlags = false
	}
	return atom, h, nil
}

// hirZeroLen reports whether the Rust Properties of the expression give a
// maximum match length of Some(0): the empty expression, and captures,
// concatenations, alternations and repetitions of such expressions. A class
// (even the empty one, Hir::fail) and a literal do not.
func hirZeroLen(h *hir) bool {
	if h.zl != 0 {
		return h.zl == 1
	}
	z := false
	switch h.kind {
	case hirEmpty, hirLook:
		z = true
	case hirCapture, hirRepetition:
		z = hirZeroLen(h.sub)
	case hirConcat, hirAlternation:
		z = true
		for _, s := range h.subs {
			if !hirZeroLen(s) {
				z = false
				break
			}
		}
	}
	if z {
		h.zl = 1
	} else {
		h.zl = 2
	}
	return z
}

// hirRepetitionOf is regex_syntax: Hir::repetition (hir/mod.rs).
func hirRepetitionOf(sub *hir, min, max uint32, hasMax, lazy bool) *hir {
	if hirZeroLen(sub) {
		if min > 1 {
			min = 1
		}
		if hasMax && max > 1 {
			max = 1
		} else if !hasMax {
			max, hasMax = 1, true
		}
	}
	if min == 0 && hasMax && max == 0 {
		return &hir{kind: hirEmpty}
	}
	if min == 1 && hasMax && max == 1 {
		return sub
	}
	return &hir{kind: hirRepetition, min: min, max: max, hasMax: hasMax, lazy: lazy, sub: sub}
}

// skipWS is the whitespace skip of parse_decimal: char::is_whitespace.
func isRustWhitespace(c rune) bool {
	r := rxPerlSpace
	for i := 0; i+1 < len(r); i += 2 {
		if uint32(c) >= r[i] && uint32(c) <= r[i+1] {
			return true
		}
	}
	return false
}

// parseDecimal is ast/parse.rs: fn parse_decimal. ok is false when there are no
// digits (DecimalEmpty); a value above u32 is an error.
func (p *patParser) parseDecimal() (v uint32, ok bool, err error) {
	for p.more() && isRustWhitespace(p.peek()) {
		p.pos++
	}
	st := p.pos
	for p.more() && p.peek() >= '0' && p.peek() <= '9' {
		p.pos++
	}
	digits := string(p.src[st:p.pos])
	for p.more() && isRustWhitespace(p.peek()) {
		p.pos++
	}
	if digits == "" {
		return 0, false, nil
	}
	n, e := strconv.ParseUint(digits, 10, 32)
	if e != nil {
		return 0, false, p.errf("invalid decimal")
	}
	return uint32(n), true, nil
}

// parseCounted parses {n}, {n,} and {n,m} at p.pos (ast/parse.rs: fn
// parse_counted_repetition, with empty_min_range off).
func (p *patParser) parseCounted() (min, max uint32, hasMax bool, err error) {
	defer func() {
		if err == nil {
			big := max
			if min > big {
				big = min
			}
			if big > math.MaxInt32 {
				big = math.MaxInt32
			}
			bRepeatCount.check(int(big))
		}
	}()
	// The message quotes the whole pattern: make it only when it is returned,
	// not for each counted repetition (a pattern of 65,000 of them would quote
	// 262,000 bytes 65,000 times).
	unclosed := func() error { return p.errf("unclosed counted repetition") }
	p.pos++ // {
	if !p.more() {
		return 0, 0, false, unclosed()
	}
	n, nok, err := p.parseDecimal()
	if err != nil {
		return 0, 0, false, err
	}
	if !p.more() {
		return 0, 0, false, unclosed()
	}
	if p.peek() == ',' {
		p.pos++
		if !p.more() {
			return 0, 0, false, unclosed()
		}
		if p.peek() != '}' {
			if !nok {
				return 0, 0, false, p.errf("repetition quantifier expects a valid decimal")
			}
			e, eok, err := p.parseDecimal()
			if err != nil {
				return 0, 0, false, err
			}
			if !eok {
				return 0, 0, false, p.errf("repetition quantifier expects a valid decimal")
			}
			min, max, hasMax = n, e, true
			if e < n {
				return 0, 0, false, p.errf("invalid repetition count range")
			}
		} else {
			if !nok {
				return 0, 0, false, p.errf("repetition quantifier expects a valid decimal")
			}
			min = n
		}
	} else {
		if !nok {
			return 0, 0, false, p.errf("repetition quantifier expects a valid decimal")
		}
		min, max, hasMax = n, n, true
	}
	if !p.more() || p.peek() != '}' {
		return 0, 0, false, unclosed()
	}
	p.pos++
	return min, max, hasMax, nil
}

// parseAtom parses one primitive, class or group. The results are the Hir, the
// height of its syntax tree node, and whether it was a flag directive such as
// (?i), which is not an expression a repetition can apply to.
func (p *patParser) parseAtom() (*hir, int, bool, error) {
	c := p.peek()
	switch c {
	case '(':
		p.pos++
		return p.parseGroup()
	case '[':
		cls, h, err := p.parseClass()
		if err != nil {
			return nil, 0, false, err
		}
		return hirClassOf(cls), h, false, nil
	case '.':
		p.pos++
		// Hir::dot: any character; except line feed; except CR and LF (flags s, R).
		switch {
		case p.flags.s:
			return hirClassOf([]classRng{{0, 0x10FFFF}}), 0, false, nil
		case p.flags.crlf:
			return hirClassOf([]classRng{{0, 0x09}, {0x0B, 0x0C}, {0x0E, 0x10FFFF}}), 0, false, nil
		}
		return hirClassOf([]classRng{{0, 0x09}, {0x0B, 0x10FFFF}}), 0, false, nil
	case '^', '$':
		p.pos++
		return &hir{kind: hirLook}, 0, false, nil
	case '*', '+', '?', '{':
		return nil, 0, false, p.errf("repetition operator missing expression")
	case '\\':
		e, err := p.parseEscape()
		if err != nil {
			return nil, 0, false, err
		}
		if e.isLook {
			return &hir{kind: hirLook}, 0, false, nil
		}
		if e.isClass {
			p.noteRanges(len(e.cls))
			return hirClassOf(e.cls), 0, false, nil
		}
		return p.literalHir(e.r), 0, false, nil
	}
	p.pos++
	return p.literalHir(c), 0, false, nil
}

// escRes is the result of an escape: a literal character or a class. An empty
// class (\P{Any}) is a class, not the literal U+0000.
type escRes struct {
	isLook  bool // an assertion (\A \z \b \B \< \>)
	isClass bool
	r       rune
	cls     []classRng
}

// parseEscape parses a backslash escape at p.pos (ast/parse.rs: fn parse_escape).
// The generator rewrites \d \s \w \D \S \W in the pattern text before parsing,
// so those letters are not escapes here.
func (p *patParser) parseEscape() (escRes, error) {
	p.pos++ // backslash
	if !p.more() {
		return escRes{}, p.errf("incomplete escape sequence")
	}
	c := p.peek()
	p.pos++
	switch c {
	case 'n':
		return escRes{r: '\n'}, nil
	case 'r':
		return escRes{r: '\r'}, nil
	case 't':
		return escRes{r: '\t'}, nil
	case 'f':
		return escRes{r: '\f'}, nil
	case 'v':
		return escRes{r: '\v'}, nil
	case 'a':
		return escRes{r: 7}, nil
	case 'x', 'u', 'U':
		r, err := p.parseHex(c)
		return escRes{r: r}, err
	case 'p', 'P':
		cls, err := p.parseUnicodeEscape(c)
		return escRes{isClass: true, cls: cls}, err
	case 'A', 'z', 'B', '<', '>':
		return escRes{isLook: true}, nil
	case 'b':
		if p.more() && p.peek() == '{' {
			if err := p.maybeSpecialWordBoundary(); err != nil {
				return escRes{}, err
			}
		}
		return escRes{isLook: true}, nil
	}
	if c < 0x80 && !(c >= '0' && c <= '9') && !(c >= 'A' && c <= 'Z') && !(c >= 'a' && c <= 'z') && c != '<' && c != '>' {
		return escRes{r: c}, nil // is_escapeable_character
	}
	return escRes{}, p.errf("unrecognized escape sequence \\%c", c)
}

// hirAlternationOf is regex_syntax: Hir::alternation. It flattens nested
// alternations; an alternation of single-character literals, or of classes,
// becomes one class; otherwise a common prefix of concatenations is lifted
// (lift_common_prefix).
func hirAlternationOf(subs []*hir, p *patParser) (*hir, error) {
	var flat []*hir
	for _, s := range subs {
		if s.kind == hirAlternation {
			flat = append(flat, s.subs...)
		} else {
			flat = append(flat, s)
		}
	}
	if len(flat) == 0 {
		return &hir{kind: hirClass}, nil // Hir::fail()
	}
	if len(flat) == 1 {
		return flat[0], nil
	}
	allLit := true
	for _, s := range flat {
		if !(s.kind == hirLiteral && len(s.lit) == 1) {
			allLit = false
		}
	}
	if allLit {
		var rs []classRng
		for _, s := range flat {
			rs = append(rs, classRng{uint32(s.lit[0]), uint32(s.lit[0])})
		}
		return hirClassOf(canonClass(rs)), nil
	}
	allCls := true
	for _, s := range flat {
		if s.kind != hirClass {
			allCls = false
		}
	}
	if allCls {
		var rs []classRng
		for _, s := range flat {
			rs = append(rs, s.class...)
		}
		return hirClassOf(canonClass(rs)), nil
	}
	if h := liftCommonPrefix(flat); h != nil {
		return h, nil
	}
	return &hir{kind: hirAlternation, subs: flat}, nil
}

// hirClassOf is regex_syntax: Hir::class. An empty class is Hir::fail (kept
// as an empty class); a class of one character is a literal.
func hirClassOf(cls []classRng) *hir {
	if len(cls) == 1 && cls[0].lo == cls[0].hi {
		return &hir{kind: hirLiteral, lit: []rune{rune(cls[0].lo)}}
	}
	return &hir{kind: hirClass, class: cls}
}

// hirEqual is the structural equality of regex_syntax::hir::Hir.
func hirEqual(a, b *hir) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case hirLiteral:
		return string(a.lit) == string(b.lit)
	case hirClass:
		if len(a.class) != len(b.class) {
			return false
		}
		for i := range a.class {
			if a.class[i] != b.class[i] {
				return false
			}
		}
		return true
	case hirRepetition:
		return a.min == b.min && a.max == b.max && a.hasMax == b.hasMax && a.lazy == b.lazy && hirEqual(a.sub, b.sub)
	case hirCapture:
		return a.capIdx == b.capIdx && a.capName == b.capName && hirEqual(a.sub, b.sub)
	case hirConcat, hirAlternation:
		if len(a.subs) != len(b.subs) {
			return false
		}
		for i := range a.subs {
			if !hirEqual(a.subs[i], b.subs[i]) {
				return false
			}
		}
	}
	return true
}

// liftCommonPrefix is regex_syntax: fn lift_common_prefix. It returns nil
// when the branches do not all start with one shared prefix of a concatenation.
func liftCommonPrefix(hirs []*hir) *hir {
	if len(hirs) <= 1 || hirs[0].kind != hirConcat || len(hirs[0].subs) == 0 {
		return nil
	}
	prefix := hirs[0].subs
	for _, h := range hirs[1:] {
		if h.kind != hirConcat {
			return nil
		}
		n := 0
		for n < len(prefix) && n < len(h.subs) && hirEqual(prefix[n], h.subs[n]) {
			n++
		}
		prefix = prefix[:n]
		if len(prefix) == 0 {
			return nil
		}
	}
	l := len(prefix)
	var prefixConcat []*hir
	var suffixAlts []*hir
	for _, h := range hirs {
		suffixAlts = append(suffixAlts, hirConcatOf(append([]*hir(nil), h.subs[l:]...)))
		if prefixConcat == nil {
			prefixConcat = append([]*hir(nil), h.subs[:l]...)
		}
	}
	alt, _ := hirAlternationOf(suffixAlts, nil)
	return hirConcatOf(append(prefixConcat, alt))
}

func hirConcatOf(items []*hir) *hir {
	var flat []*hir
	copies := 0 // runes and nodes copied (ops.concatRunes)
	for _, it := range items {
		switch it.kind {
		case hirEmpty:
		case hirConcat:
			flat = append(flat, it.subs...)
			copies += len(it.subs)
		default:
			flat = append(flat, it)
			copies++
		}
	}
	// Runs of literals are joined into one literal. The runes of a run are
	// collected in one buffer and the node is made when the run ends: joining
	// two nodes at a time copies the run so far for each literal.
	var merged []*hir
	var run []rune
	var first *hir // the node of the run when it has one literal
	n := 0         // literals in the run
	flush := func() {
		switch n {
		case 0:
		case 1:
			merged = append(merged, first)
		default:
			merged = append(merged, &hir{kind: hirLiteral, lit: run})
		}
		run, first, n = nil, nil, 0
	}
	for _, it := range flat {
		if it.kind == hirLiteral {
			switch n {
			case 0:
				first = it
			case 1:
				run = append(append([]rune(nil), first.lit...), it.lit...)
				copies += len(first.lit) + len(it.lit)
			default:
				run = append(run, it.lit...)
				copies += len(it.lit)
			}
			n++
			continue
		}
		flush()
		merged = append(merged, it)
		copies++
	}
	flush()
	ops.concatRunes.add(copies)
	switch len(merged) {
	case 0:
		return &hir{kind: hirEmpty}
	case 1:
		return merged[0]
	}
	return &hir{kind: hirConcat, subs: merged}
}

// canonClass sorts and merges ranges (regex_syntax: IntervalSet::canonicalize:
// ranges that overlap or touch are merged).
func canonClass(rs []classRng) []classRng {
	out := append([]classRng(nil), rs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].lo != out[j].lo {
			return out[i].lo < out[j].lo
		}
		return out[i].hi < out[j].hi
	})
	var res []classRng
	for _, r := range out {
		if n := len(res); n > 0 && r.lo <= res[n-1].hi+1 {
			if r.hi > res[n-1].hi {
				res[n-1].hi = r.hi
			}
			continue
		}
		res = append(res, r)
	}
	return res
}

func charDecrement(c uint32) uint32 {
	if c == 0xE000 {
		return 0xD7FF
	}
	return c - 1
}

func charIncrement(c uint32) uint32 {
	if c == 0xD7FF {
		return 0xE000
	}
	return c + 1
}

// negateClass is regex_syntax: IntervalSet::negate over chars (the
// increment and decrement of a char skip the surrogate gap).
func negateClass(cls []classRng) []classRng {
	const max = 0x10FFFF
	if len(cls) == 0 {
		return []classRng{{0, max}}
	}
	var out []classRng
	if cls[0].lo > 0 {
		out = append(out, classRng{0, charDecrement(cls[0].lo)})
	}
	for i := 1; i < len(cls); i++ {
		lo := charIncrement(cls[i-1].hi)
		hi := charDecrement(cls[i].lo)
		if lo > hi {
			lo, hi = hi, lo
		}
		out = append(out, classRng{lo, hi})
	}
	if cls[len(cls)-1].hi < max {
		out = append(out, classRng{charIncrement(cls[len(cls)-1].hi), max})
	}
	return out
}

// perlClassRewrite is the text rewrite at the top of expand_tokens.rs: fn
// expand_rule (Rule::Pattern). The Rust code applies six replace calls in this
// order, each over the whole text produced by the previous one.
func perlClassRewrite(s string) string {
	s = strings.ReplaceAll(s, `\w`, `[0-9A-Za-z_]`)
	s = strings.ReplaceAll(s, `\s`, `[\t-\r ]`)
	s = strings.ReplaceAll(s, `\d`, `[0-9]`)
	s = strings.ReplaceAll(s, `\W`, `[^0-9A-Za-z_]`)
	s = strings.ReplaceAll(s, `\S`, `[^\t-\r ]`)
	s = strings.ReplaceAll(s, `\D`, `[^0-9]`)
	return s
}

// parseClass parses a bracketed class at p.pos ('[') as ast/parse.rs does
// (parse_set_class_open, parse_set_class, parse_set_class_range) and
// evaluates it as hir/translate.rs does. The second result is the height of
// the class in the syntax tree.
func (p *patParser) parseClass() ([]classRng, int, error) {
	p.pos++ // [
	negated := false
	if p.more() && p.peek() == '^' {
		negated = true
		p.pos++
	}
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > nestLimit {
		return nil, 0, p.nestErr()
	}
	var items []classRng // the union read so far
	nItems, itemH := 0, 0
	addItem := func(rs []classRng, h int) {
		p.noteRanges(len(rs))
		items = append(items, rs...)
		nItems++
		itemH = max1(itemH, h)
	}
	unionHeight := func() int {
		if nItems >= 2 {
			return itemH + 1
		}
		return itemH
	}
	// A set operation closes the union read so far; operations are
	// left-associative and share one precedence. An operand can be empty.
	var result []classRng
	haveResult := false
	resultH := 0
	var pending rune
	// tooTall is set when the operation tree already stands taller than the nest
	// limit allows. Rust (ast/parse.rs: NestLimiter, run on the finished syntax
	// tree before hir/translate.rs lowers it) never evaluates the set operations
	// of such a class. The parser reads on, so that a syntax error later in the
	// pattern is still reported first, but it evaluates no further operation: the
	// class is rejected by the height check of the caller, and its value is unused.
	tooTall := false
	applyOp := func(op rune, a, b []classRng, newH int) []classRng {
		if newH+1 > nestLimit { // +1: the enclosing bracketed class
			tooTall = true
		}
		if tooTall {
			return nil
		}
		p.classOps++
		return applyClassOp(op, a, b)
	}
	finish := func() {
		cur := canonClass(items)
		uh := unionHeight()
		if !haveResult {
			result, haveResult, resultH = cur, true, uh
			return
		}
		resultH = 1 + max1(resultH, uh)
		result = applyOp(pending, result, cur, resultH)
	}
	isOp := func() bool {
		return p.pos+1 < len(p.src) && (p.peek() == '&' || p.peek() == '-' || p.peek() == '~') && p.src[p.pos+1] == p.peek()
	}
	// parse_set_class_open: any number of - right after [ or [^ are literals;
	// if there were none, a ] there is a literal too.
	for p.more() && p.peek() == '-' {
		p.pos++
		addItem(p.foldLeafIfI([]classRng{{'-', '-'}}), 0)
	}
	if nItems == 0 && p.more() && p.peek() == ']' {
		p.pos++
		addItem(p.foldLeafIfI([]classRng{{']', ']'}}), 0)
	}
	for {
		if !p.more() {
			return nil, 0, p.errf("unclosed character class")
		}
		c := p.peek()
		if c == ']' {
			p.pos++
			break
		}
		if isOp() {
			finish()
			pending = p.peek()
			p.pos += 2
			items, nItems, itemH = nil, 0, 0
			continue
		}
		if c == '[' {
			if cls, ok := p.maybeAsciiClass(); ok {
				addItem(cls, 0)
				continue
			}
			nested, nh, err := p.parseClass()
			if err != nil {
				return nil, 0, err
			}
			addItem(nested, nh)
			continue
		}
		// parse_set_class_range: a primitive, then maybe a range.
		prim1, err := p.classPrimitive()
		if err != nil {
			return nil, 0, err
		}
		if !p.more() {
			return nil, 0, p.errf("unclosed character class")
		}
		isRange := p.peek() == '-' && !(p.pos+1 < len(p.src) && (p.src[p.pos+1] == ']' || p.src[p.pos+1] == '-'))
		if !isRange {
			if prim1.isClass {
				addItem(prim1.cls, 0)
			} else {
				addItem(p.foldLeafIfI([]classRng{{uint32(prim1.r), uint32(prim1.r)}}), 0)
			}
			continue
		}
		p.pos++ // -
		if !p.more() {
			return nil, 0, p.errf("unclosed character class")
		}
		prim2, err := p.classPrimitive()
		if err != nil {
			return nil, 0, err
		}
		if prim1.isClass || prim2.isClass {
			return nil, 0, p.errf("invalid character class range: an endpoint is a class")
		}
		if prim2.r < prim1.r {
			return nil, 0, p.errf("invalid character class range")
		}
		addItem(p.foldLeafIfI([]classRng{{uint32(prim1.r), uint32(prim2.r)}}), 0)
	}
	cls := canonClass(items)
	h := unionHeight()
	if haveResult {
		h = 1 + max1(resultH, h)
		cls = applyOp(pending, result, cls, h)
	}
	if negated {
		cls = negateClass(cls)
	}
	return cls, h + 1, nil
}

// classPrimitive is ast/parse.rs: fn parse_set_class_item: an escape or one
// literal character.
func (p *patParser) classPrimitive() (escRes, error) {
	c := p.peek()
	if c == '\\' {
		e, err := p.parseEscape()
		if err == nil && e.isLook {
			return escRes{}, p.errf("an assertion is not a class item")
		}
		return e, err
	}
	p.pos++
	return escRes{r: c}, nil
}

// applyClassOp is the evaluation of a class set operation: && is intersection,
// -- is difference and ~~ is symmetric difference (IntervalSet::intersect,
// difference, symmetric_difference).
func applyClassOp(op rune, a, b []classRng) []classRng {
	switch op {
	case '&':
		return classIntersect(a, b)
	case '-':
		return classDifference(a, b)
	}
	// symmetric_difference: intersection = self & other; self |= other; self -= intersection
	return classDifference(classUnion(a, b), classIntersect(a, b))
}

// parseHex is regex_syntax ast::parse: fn parse_hex (\x, \u, \U): a fixed
// number of hex digits (2, 4, 8), or a braced run of any number of digits whose
// value must be a Unicode scalar value (parse_hex_digits, parse_hex_brace).
func (p *patParser) parseHex(kind rune) (rune, error) {
	width := map[rune]int{'x': 2, 'u': 4, 'U': 8}[kind]
	bad := func(what string) (rune, error) {
		return 0, p.errf("%s in escape \\%c", what, kind)
	}
	var digits []rune
	if p.more() && p.peek() == '{' {
		p.pos++
		for {
			if !p.more() {
				return bad("unclosed brace")
			}
			c := p.peek()
			p.pos++
			if c == '}' {
				break
			}
			if !isHexDigit(c) {
				return bad("invalid hexadecimal digit")
			}
			digits = append(digits, c)
		}
		if len(digits) == 0 {
			return bad("empty hexadecimal number")
		}
	} else {
		for i := 0; i < width; i++ {
			if !p.more() || !isHexDigit(p.peek()) {
				return bad("invalid hexadecimal digit")
			}
			digits = append(digits, p.peek())
			p.pos++
		}
	}
	v, err := strconv.ParseUint(string(digits), 16, 32)
	if err != nil || v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) {
		return bad("invalid Unicode scalar value")
	}
	return rune(v), nil
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// foldLeafIfI folds a positive leaf when the i flag is in force.
func (p *patParser) foldLeafIfI(rs []classRng) []classRng {
	if p.flags.i {
		return foldLeaf(rs, false)
	}
	return rs
}

// literalHir builds the Hir of one literal character. With the i flag the
// Expander of pattern.rs replaces it by its folded class.
func (p *patParser) literalHir(c rune) *hir {
	if p.flags.i {
		return hirClassOf(foldASCIISafe([]classRng{{uint32(c), uint32(c)}}))
	}
	return &hir{kind: hirLiteral, lit: []rune{c}}
}

// parseUnicodeEscape parses what follows \p or \P (ast/parse.rs: fn
// parse_unicode_class): one character, or a braced name that holds name=value,
// name:value or name!=value. A ^ is part of the name, not a negation. The
// returned class already holds the negation and the folding.
func (p *patParser) parseUnicodeEscape(kind rune) ([]classRng, error) {
	negated := kind == 'P'
	if !p.more() {
		return nil, p.errf("incomplete Unicode class escape")
	}
	var name, value string
	hasValue := false
	if p.peek() == '{' {
		p.pos++
		end := -1
		for i := p.pos; i < len(p.src); i++ {
			if p.src[i] == '}' {
				end = i
				break
			}
		}
		if end < 0 {
			return nil, p.errf("unclosed Unicode class")
		}
		text := string(p.src[p.pos:end])
		p.pos = end + 1
		if i := strings.Index(text, "!="); i >= 0 {
			name, value, hasValue = text[:i], text[i+2:], true
			negated = !negated
		} else if i := strings.Index(text, ":"); i >= 0 {
			name, value, hasValue = text[:i], text[i+1:], true
		} else if i := strings.Index(text, "="); i >= 0 {
			name, value, hasValue = text[:i], text[i+1:], true
		} else {
			name = text
		}
	} else {
		if p.peek() == '\\' {
			return nil, p.errf("invalid Unicode class")
		}
		name = string(p.peek())
		p.pos++
	}
	pos, err := p.unicodeClass(name, value, hasValue)
	if err != nil {
		return nil, err
	}
	if p.flags.i {
		return foldLeaf(pos, negated), nil
	}
	if negated {
		return negateClass(pos), nil
	}
	return pos, nil
}

func isCaptureChar(c rune, first bool) bool {
	if first {
		return c == '_' || isRustAlphabetic(uint32(c))
	}
	return c == '_' || c == '.' || c == '[' || c == ']' || isRustAlphabetic(uint32(c)) || isRustNumeric(uint32(c))
}

// groupHead is the text that parseGroup tests after "(": the next three runes,
// which is as many as the longest prefix it looks for ("?<!", "?<=", "?P<").
// Converting the whole remainder of the pattern for each group costs the
// length of the pattern for each group.
func (p *patParser) groupHead() string {
	end := p.pos + 3
	if end > len(p.src) {
		end = len(p.src)
	}
	ops.groupRunes.add(end - p.pos)
	return string(p.src[p.pos:end])
}

// groupHeadOf is groupHead; a variable so that a test can put the earlier
// version (the whole remainder) in its place and compare the parses.
var groupHeadOf = (*patParser).groupHead

// parseGroup parses what follows "(" (ast/parse.rs: fn parse_group): a capture
// group, (?:...), (?P<name>...), (?<name>...), (?flags) and (?flags:...). Flags
// set by (?flags) last to the end of the enclosing group. The results are the
// Hir, the tree height and whether the group was a flag directive.
func (p *patParser) parseGroup() (*hir, int, bool, error) {
	saved := p.flags
	capture := false
	capName := ""
	rest := groupHeadOf(p)
	switch {
	case strings.HasPrefix(rest, "?=") || strings.HasPrefix(rest, "?!") || strings.HasPrefix(rest, "?<=") || strings.HasPrefix(rest, "?<!"):
		return nil, 0, false, p.errf("look-around is not supported")
	case strings.HasPrefix(rest, "?P<") || strings.HasPrefix(rest, "?<"):
		if rest[1] == 'P' {
			p.pos++
		}
		p.pos += 2
		p.caps++
		name, err := p.parseCaptureName()
		if err != nil {
			return nil, 0, false, err
		}
		capture, capName = true, name
	case strings.HasPrefix(rest, "?"):
		p.pos++
		if !p.more() {
			return nil, 0, false, p.errf("unclosed group")
		}
		nf, term, empty, err := p.parseFlags()
		if err != nil {
			return nil, 0, false, err
		}
		if term == ')' {
			if empty {
				return nil, 0, false, p.errf("repetition operator missing expression")
			}
			p.flags = nf
			return &hir{kind: hirEmpty}, 0, true, nil
		}
		p.flags = nf
	default:
		p.caps++
		capture = true
	}
	idx := p.caps
	p.depth++
	if p.depth > nestLimit {
		return nil, 0, false, p.nestErr()
	}
	inner, ih, err := p.parseAlternation()
	if err != nil {
		return nil, 0, false, err
	}
	p.depth--
	if !p.more() || p.peek() != ')' {
		return nil, 0, false, p.errf("unclosed group")
	}
	p.pos++
	p.flags = saved
	if capture {
		return &hir{kind: hirCapture, sub: inner, capIdx: idx, capName: capName}, ih + 1, false, nil
	}
	return inner, ih + 1, false, nil
}

// parseCaptureName is ast/parse.rs: fn parse_capture_name and add_capture_name.
func (p *patParser) parseCaptureName() (string, error) {
	if !p.more() {
		return "", p.errf("unexpected end of a group name")
	}
	start := p.pos
	for {
		if p.peek() == '>' {
			break
		}
		if !isCaptureChar(p.peek(), p.pos == start) {
			return "", p.errf("invalid capture group name")
		}
		p.pos++
		if !p.more() {
			break
		}
	}
	end := p.pos
	if !p.more() {
		return "", p.errf("unexpected end of a group name")
	}
	p.pos++ // >
	name := string(p.src[start:end])
	if name == "" {
		return "", p.errf("empty capture group name")
	}
	if p.capNames[name] {
		return "", p.errf("duplicate capture group name %q", name)
	}
	p.capNames[name] = true
	return name, nil
}

// parseFlags is ast/parse.rs: fn parse_flags, with the effect of the flags. It
// reads up to ) or : (consumed). A flag may appear once, a negation once, and
// a negation cannot end the list.
func (p *patParser) parseFlags() (nf patFlags, term rune, empty bool, err error) {
	nf = p.flags
	negated, lastNeg := false, false
	seen := map[rune]bool{}
	count := 0
	for {
		if !p.more() {
			return nf, 0, false, p.errf("unclosed flag group")
		}
		c := p.peek()
		if c == ':' || c == ')' {
			break
		}
		if c == '-' {
			lastNeg = true
			if seen['-'] {
				return nf, 0, false, p.errf("repeated flag negation")
			}
			seen['-'] = true
			negated = true
		} else {
			lastNeg = false
			switch c {
			case 'i', 'm', 's', 'U', 'u', 'R', 'x':
			default:
				return nf, 0, false, p.errf("unrecognized flag %q", c)
			}
			if seen[c] {
				return nf, 0, false, p.errf("duplicate flag %q", c)
			}
			seen[c] = true
			switch c {
			case 'i':
				nf.i = !negated
			case 's':
				nf.s = !negated
			case 'R':
				nf.crlf = !negated
			case 'U':
				nf.swapGreed = !negated
			case 'u':
				if negated {
					return nf, 0, false, p.notPorted("flag -u (non-Unicode mode)")
				}
			case 'x':
				return nf, 0, false, p.notPorted("flag x (verbose mode)")
			}
		}
		count++
		p.pos++
	}
	if lastNeg {
		return nf, 0, false, p.errf("dangling flag negation")
	}
	term = p.peek()
	p.pos++
	return nf, term, count == 0, nil
}

// maybeSpecialWordBoundary is ast/parse.rs: fn maybe_parse_special_word_boundary,
// at "{" after \b. If the brace does not start a name the position is kept
// and \b stands alone; a name must be start, end, start-half or end-half.
func (p *patParser) maybeSpecialWordBoundary() error {
	save := p.pos
	p.pos++ // {
	if !p.more() {
		return p.errf("unexpected end of a special word boundary")
	}
	valid := func(c rune) bool { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '-' }
	if !valid(p.peek()) {
		p.pos = save
		return nil
	}
	st := p.pos
	for p.more() && valid(p.peek()) {
		p.pos++
	}
	if !p.more() || p.peek() != '}' {
		return p.errf("unclosed special word boundary")
	}
	name := string(p.src[st:p.pos])
	p.pos++
	switch name {
	case "start", "end", "start-half", "end-half":
		return nil
	}
	return p.errf("unrecognized special word boundary %q", name)
}

// maybeAsciiClass is ast/parse.rs: fn maybe_parse_ascii_class at "[" inside a
// class. A complete, valid [:name:] or [:^name:] is an ASCII class (a leaf, folded
// and negated like a property class); anything else restores the position and is
// read as an ordinary nested class.
func (p *patParser) maybeAsciiClass() ([]classRng, bool) {
	save := p.pos
	fail := func() ([]classRng, bool) { p.pos = save; return nil, false }
	p.pos++ // [
	if !p.more() || p.peek() != ':' {
		return fail()
	}
	p.pos++
	if !p.more() {
		return fail()
	}
	negated := false
	if p.peek() == '^' {
		negated = true
		p.pos++
		if !p.more() {
			return fail()
		}
	}
	st := p.pos
	for p.more() && p.peek() != ':' {
		p.pos++
	}
	if !p.more() {
		return fail()
	}
	name := string(p.src[st:p.pos])
	if !(p.pos+1 < len(p.src) && p.src[p.pos+1] == ']') {
		return fail()
	}
	pos, ok := asciiClassRanges[name]
	if !ok {
		return fail()
	}
	p.pos += 2
	cls := canonClass(append([]classRng(nil), pos...))
	if p.flags.i {
		return foldLeaf(cls, negated), true
	}
	if negated {
		return negateClass(cls), true
	}
	return cls, true
}

// asciiClassRanges is ast/mod.rs and hir/translate.rs: fn ascii_class_as_chars.
var asciiClassRanges = map[string][]classRng{
	"alnum":  {{'0', '9'}, {'A', 'Z'}, {'a', 'z'}},
	"alpha":  {{'A', 'Z'}, {'a', 'z'}},
	"ascii":  {{0, 0x7F}},
	"blank":  {{'\t', '\t'}, {' ', ' '}},
	"cntrl":  {{0, 0x1F}, {0x7F, 0x7F}},
	"digit":  {{'0', '9'}},
	"graph":  {{'!', '~'}},
	"lower":  {{'a', 'z'}},
	"print":  {{' ', '~'}},
	"punct":  {{'!', '/'}, {':', '@'}, {'[', '`'}, {'{', '~'}},
	"space":  {{'\t', '\r'}, {' ', ' '}},
	"upper":  {{'A', 'Z'}},
	"word":   {{'0', '9'}, {'A', 'Z'}, {'a', 'z'}, {'_', '_'}},
	"xdigit": {{'0', '9'}, {'A', 'F'}, {'a', 'f'}},
}

package lexexact

import (
	"fmt"
	"sort"
)

// Source is the lexer of one generated parser.c, read as data: the ordered
// statements of each lex state, with the guards kept as expression trees.
type Source struct {
	// Main is the ts_lex function.
	Main *Lexer
	// Keyword is the ts_lex_keywords function, or nil when the file has none.
	Keyword *Lexer

	sets   map[string][]charRange
	starts map[string][]uint32
	// funcOrder lists the helper functions in definition order.
	funcOrder []string

	funcs map[string]*charFunc
	hdr   header
}

// Lexer is one lex function (ts_lex or ts_lex_keywords).
type Lexer struct {
	src    *Source
	states []lexState // index = state number; holes have defined == false
	// own is true for a lexer written by EmitC from an own-range record: it has
	// a start array (ts_lex_start or ts_lex_keywords_start) and every step for a
	// valid code point is a range in the emitter's form. trans and start then
	// hold the ranges per state and the start states.
	own   bool
	trans [][]Range
	start []uint32
	// eofSet reports whether the C code sets `eof` from lexer->eof() at each
	// step. If not, `eof` is always false in that function.
	eofSet bool
}

type charRange struct{ start, end int64 }

type charFunc struct {
	param string
	body  *expr
}

type stmtKind uint8

const (
	stmtAdvance stmtKind = iota // ADVANCE(target)
	stmtSkip                    // SKIP(target)
	stmtMap                     // ADVANCE_MAP(k, t, ...)
)

type mapPair struct{ key, target int64 }

// stmt is one step statement. cond is nil for an unconditional step.
type stmt struct {
	kind   stmtKind
	cond   *expr
	target int64
	pairs  []mapPair
}

type lexState struct {
	defined   bool
	hasAccept bool
	accept    int64 // symbol number
	stmts     []stmt
}

// NumStates is the number of lex states, one more than the largest case label.
func (l *Lexer) NumStates() int { return len(l.states) }

type exprOp uint8

const (
	opConst exprOp = iota
	opLook
	opEOF
	opNot
	opOr
	opAnd
	opEq
	opNe
	opLt
	opLe
	opGt
	opGe
	opTern
	opSet  // set_contains(name, n, arg)
	opCall // name(arg)
)

type expr struct {
	op      exprOp
	val     int64
	name    string
	a, b, c *expr
	// list holds the terms of a run of || or && at one level (opOr, opAnd).
	list []*expr
	h    int // height of the tree: parentheses, !, and ternaries add one; a run of || or && does not
}

const (
	maxState  = 0xFFFF // TSStateId is uint16_t
	maxSymbol = 0xFFFF // TSSymbol is uint16_t
)

func (sc *scanner) number() (int64, error) {
	t, err := sc.next()
	if err != nil {
		return 0, err
	}
	if t.kind != tokNumber && t.kind != tokChar {
		return 0, fmt.Errorf("expected a number, got %v", t)
	}
	return t.num, nil
}

func (sc *scanner) expect(p string) error {
	t, err := sc.next()
	if err != nil {
		return err
	}
	if t.kind != tokPunct || t.text != p {
		return fmt.Errorf("expected %q, got %v", p, t)
	}
	return nil
}

func (sc *scanner) expectIdent(id string) error {
	t, err := sc.next()
	if err != nil {
		return err
	}
	if t.kind != tokIdent || t.text != id {
		return fmt.Errorf("expected %q, got %v", id, t)
	}
	return nil
}

// readLexer reads one lex function. It returns nil, nil when the function is
// not in the file.
func (s *Source) readLexer(sc *scanner, fname string, enum map[string]int64) (*Lexer, error) {
	p := &exprParser{sc: sc, src: s, look: "lookahead", enum: enum, allowEOF: true, allowSet: s.hdr.hasSet}
	l := &Lexer{src: s}
	fail := func(err error) (*Lexer, error) { return nil, fmt.Errorf("%s: %v", fname, err) }

	if err := sc.expectIdent("START_LEXER"); err != nil {
		return fail(err)
	}
	if err := sc.expect("("); err != nil {
		return fail(err)
	}
	if err := sc.expect(")"); err != nil {
		return fail(err)
	}
	if err := sc.expect(";"); err != nil {
		return fail(err)
	}
	l.eofSet = s.hdr.eofSet
	if err := p.advance(); err != nil {
		return fail(err)
	}
	if p.tok.kind == tokIdent && p.tok.text == "eof" {
		// eof = lexer->eof(lexer);
		for _, want := range []string{"eof", "=", "lexer", "->", "eof", "(", "lexer", ")", ";"} {
			if p.tok.text != want {
				return fail(fmt.Errorf("expected %q in the eof assignment, got %v", want, p.tok))
			}
			if err := p.advance(); err != nil {
				return fail(err)
			}
		}
		if !s.hdr.eofDeclared {
			return fail(fmt.Errorf("eof is set but the header does not declare it"))
		}
		l.eofSet = true
	}
	p.allowEOF = s.hdr.eofDeclared
	if !(p.tok.kind == tokIdent && p.tok.text == "switch") {
		return fail(fmt.Errorf("expected switch, got %v", p.tok))
	}
	for _, w := range []string{"switch", "(", "state", ")", "{"} {
		if p.tok.text != w {
			return fail(fmt.Errorf("expected %q, got %v", w, p.tok))
		}
		if err := p.advance(); err != nil {
			return fail(err)
		}
	}
	for {
		if p.tok.kind == tokIdent && p.tok.text == "default" {
			break
		}
		if !(p.tok.kind == tokIdent && p.tok.text == "case") {
			return fail(fmt.Errorf("expected case, got %v", p.tok))
		}
		if err := p.advance(); err != nil {
			return fail(err)
		}
		if p.tok.kind != tokNumber {
			return fail(fmt.Errorf("expected a case number, got %v", p.tok))
		}
		id := int(p.tok.num)
		if err := p.advance(); err != nil {
			return fail(err)
		}
		if err := p.want(":"); err != nil {
			return fail(err)
		}
		if id > maxState {
			return fail(fmt.Errorf("case %d is above TSStateId (uint16_t)", id))
		}
		st, err := s.readState(p, enum)
		if err != nil {
			return fail(fmt.Errorf("case %d: %v", id, err))
		}
		for len(l.states) <= id {
			l.states = append(l.states, lexState{})
		}
		if l.states[id].defined {
			return fail(fmt.Errorf("case %d defined twice", id))
		}
		l.states[id] = st
	}
	// default: return false; } }
	for _, w := range []string{"default", ":", "return", "false", ";", "}", "}"} {
		if p.tok.text != w {
			return fail(fmt.Errorf("expected %q at the end of the function, got %v", w, p.tok))
		}
		if err := p.advance(); err != nil {
			return fail(err)
		}
	}
	for i := range l.states {
		for _, st := range l.states[i].stmts {
			var targets []int64
			if st.kind == stmtMap {
				targets = pairTargets(st.pairs)
			} else {
				targets = []int64{st.target}
			}
			for _, t := range targets {
				if t < 0 || t >= int64(len(l.states)) || !l.states[t].defined {
					return fail(fmt.Errorf("case %d steps to state %d, which has no case", i, t))
				}
			}
		}
	}
	if start, ok := s.starts[fname+"_start"]; ok {
		if err := l.readOwn(start); err != nil {
			return fail(err)
		}
	}
	return l, nil
}

// ownRangeOf recognises the guard of an own-range step, as EmitC writes it:
// (!eof && lookahead == N) or (!eof && (lo <= lookahead && lookahead <= hi)).
func ownRangeOf(e *expr) (lo, hi int64, ok bool) {
	if e == nil || e.op != opAnd || len(e.list) != 2 || e.list[0].op != opNot || e.list[0].a.op != opEOF {
		return 0, 0, false
	}
	x := e.list[1]
	switch {
	case x.op == opEq && x.a.op == opLook && x.b.op == opConst:
		return x.b.val, x.b.val, true
	case x.op == opEq && x.a.op == opConst && x.b.op == opLook:
		return x.a.val, x.a.val, true
	case x.op == opAnd && len(x.list) == 2 && x.list[0].op == opLe && x.list[1].op == opLe &&
		x.list[0].a.op == opConst && x.list[0].b.op == opLook && x.list[1].a.op == opLook && x.list[1].b.op == opConst:
		return x.list[0].a.val, x.list[1].b.val, true
	}
	return 0, 0, false
}

// readOwn switches the lexer to own-table mode: it collects the ranges and
// the start states. Any statement that is not one of the emitter's forms is
// an error, because the ranges would then be incomplete.
func (l *Lexer) readOwn(start []uint32) error {
	l.own = true
	for _, s := range start {
		if int(s) >= len(l.states) || !l.states[s].defined {
			return fmt.Errorf("start state %d has no case", s)
		}
	}
	l.start = start
	l.trans = make([][]Range, len(l.states))
	for i := range l.states {
		for _, st := range l.states[i].stmts {
			switch {
			case st.kind == stmtMap || st.cond == nil:
				return fmt.Errorf("case %d: own table with a map or an unguarded step", i)
			case st.cond.op == opEOF, st.cond.op == opLt && st.cond.a.op == opLook && st.cond.b.op == opConst && st.cond.b.val == 0:
				// end-of-input and ill-formed-byte steps are in OnEOF and OnInvalid
			default:
				lo, hi, ok := ownRangeOf(st.cond)
				if !ok || hi > MaxCodePoint || lo < 0 || lo > hi {
					return fmt.Errorf("case %d: own table with a guard that is not a range", i)
				}
				l.trans[i] = append(l.trans[i], Range{Lo: uint32(lo), Hi: uint32(hi), Target: uint32(st.target), Skip: st.kind == stmtSkip})
			}
		}
	}
	return nil
}

func pairTargets(ps []mapPair) []int64 {
	out := make([]int64, len(ps))
	for i, p := range ps {
		out[i] = p.target
	}
	return out
}

// readState reads the statements of one case, up to and including END_STATE.
func (s *Source) readState(p *exprParser, enum map[string]int64) (lexState, error) {
	st := lexState{defined: true}
	first := true
	for {
		t := p.tok
		if t.kind != tokIdent {
			return st, fmt.Errorf("unexpected %v", t)
		}
		switch t.text {
		case "END_STATE":
			for _, w := range []string{"END_STATE", "(", ")"} {
				if p.tok.text != w {
					return st, fmt.Errorf("expected %q, got %v", w, p.tok)
				}
				if err := p.advance(); err != nil {
					return st, err
				}
			}
			return st, p.optSemi()
		case "ACCEPT_TOKEN":
			if !first {
				return st, fmt.Errorf("ACCEPT_TOKEN is not the first statement")
			}
			if err := p.advance(); err != nil {
				return st, err
			}
			if err := p.want("("); err != nil {
				return st, err
			}
			if p.tok.kind == tokIdent {
				v, ok := enum[p.tok.text]
				if !ok {
					return st, fmt.Errorf("unknown accept symbol %q", p.tok.text)
				}
				st.accept = v
			} else if p.tok.kind == tokNumber {
				st.accept = p.tok.num
			} else {
				return st, fmt.Errorf("bad ACCEPT_TOKEN argument %v", p.tok)
			}
			if st.accept < 0 || st.accept > maxSymbol {
				return st, fmt.Errorf("accept symbol %d does not fit TSSymbol (uint16_t)", st.accept)
			}
			st.hasAccept = true
			if err := p.advance(); err != nil {
				return st, err
			}
			if err := p.want(")"); err != nil {
				return st, err
			}
			if err := p.want(";"); err != nil {
				return st, err
			}
		case "ADVANCE_MAP":
			if !s.hdr.hasMap {
				return st, fmt.Errorf("ADVANCE_MAP is used but the header does not define it")
			}
			if err := p.advance(); err != nil {
				return st, err
			}
			if err := p.want("("); err != nil {
				return st, err
			}
			var pairs []mapPair
			for !(p.tok.kind == tokPunct && p.tok.text == ")") {
				if p.tok.kind != tokNumber && p.tok.kind != tokChar {
					return st, fmt.Errorf("bad ADVANCE_MAP key %v", p.tok)
				}
				k := p.tok.num
				if err := p.advance(); err != nil {
					return st, err
				}
				if err := p.want(","); err != nil {
					return st, err
				}
				if p.tok.kind != tokNumber {
					return st, fmt.Errorf("bad ADVANCE_MAP target %v", p.tok)
				}
				tg := p.tok.num
				if err := p.advance(); err != nil {
					return st, err
				}
				if k > 0xFFFF || tg > 0xFFFF {
					return st, fmt.Errorf("ADVANCE_MAP value above uint16_t")
				}
				pairs = append(pairs, mapPair{k, tg})
				if p.tok.kind == tokPunct && p.tok.text == "," {
					if err := p.advance(); err != nil {
						return st, err
					}
				}
			}
			if err := p.want(")"); err != nil {
				return st, err
			}
			if err := p.optSemi(); err != nil {
				return st, err
			}
			st.stmts = append(st.stmts, stmt{kind: stmtMap, pairs: pairs})
		case "if", "ADVANCE", "SKIP":
			var cond *expr
			if t.text == "if" {
				if err := p.advance(); err != nil {
					return st, err
				}
				if err := p.want("("); err != nil {
					return st, err
				}
				e, err := p.ternary()
				if err != nil {
					return st, err
				}
				cond = e
				if err := p.want(")"); err != nil {
					return st, err
				}
			}
			kind := stmtAdvance
			switch p.tok.text {
			case "ADVANCE":
			case "SKIP":
				kind = stmtSkip
			default:
				return st, fmt.Errorf("expected ADVANCE or SKIP, got %v", p.tok)
			}
			if p.tok.kind != tokIdent {
				return st, fmt.Errorf("expected ADVANCE or SKIP, got %v", p.tok)
			}
			if err := p.advance(); err != nil {
				return st, err
			}
			if err := p.want("("); err != nil {
				return st, err
			}
			if p.tok.kind != tokNumber {
				return st, fmt.Errorf("bad step target %v", p.tok)
			}
			tg := p.tok.num
			if tg > maxState {
				return st, fmt.Errorf("step target %d does not fit TSStateId (uint16_t)", tg)
			}
			if err := p.advance(); err != nil {
				return st, err
			}
			if err := p.want(")"); err != nil {
				return st, err
			}
			if err := p.optSemi(); err != nil {
				return st, err
			}
			st.stmts = append(st.stmts, stmt{kind: kind, cond: cond, target: tg})
		default:
			return st, fmt.Errorf("unknown statement %v", t)
		}
		first = false
	}
}

// checkCalls verifies that every called function and set exists.
func (s *Source) checkCalls() error {
	var walk func(e *expr) error
	walk = func(e *expr) error {
		if e == nil {
			return nil
		}
		switch e.op {
		case opSet:
			if _, ok := s.sets[e.name]; !ok {
				return fmt.Errorf("unknown character set %q", e.name)
			}
			if int(e.val) > len(s.sets[e.name]) || e.val < 1 {
				return fmt.Errorf("set_contains(%s, %d): the array has %d entries", e.name, e.val, len(s.sets[e.name]))
			}
		case opCall:
			if _, ok := s.funcs[e.name]; !ok {
				return fmt.Errorf("unknown function %q", e.name)
			}
		}
		for _, c := range e.kids() {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	for _, f := range s.funcs {
		if err := walk(f.body); err != nil {
			return err
		}
	}
	if err := s.checkCallGraph(); err != nil {
		return err
	}
	for _, l := range []*Lexer{s.Main, s.Keyword} {
		if l == nil {
			continue
		}
		for i := range l.states {
			for _, st := range l.states[i].stmts {
				if err := walk(st.cond); err != nil {
					return fmt.Errorf("case %d: %v", i, err)
				}
			}
		}
	}
	return nil
}

// ---- expression parser ----

type exprParser struct {
	sc       *scanner
	tok      token
	look     string // name of the lookahead variable
	enum     map[string]int64
	allowEOF bool // the identifier `eof` is declared
	allowSet bool // the header holds a verified set_contains
	src      *Source
	depth    int
	deepErr  error
}

func (p *exprParser) advance() error {
	if p.deepErr != nil {
		return p.deepErr
	}
	t, err := p.sc.next()
	if err != nil {
		return err
	}
	p.tok = t
	return nil
}

// want checks that the current token is the punctuation q and moves on.
func (p *exprParser) want(q string) error {
	if p.tok.kind != tokPunct || p.tok.text != q {
		return fmt.Errorf("expected %q, got %v", q, p.tok)
	}
	return p.advance()
}

// optSemi skips a ';' if there is one. ADVANCE, SKIP, ADVANCE_MAP and
// END_STATE expand to a block or a return statement, so C accepts them with or
// without the semicolon, and older generators leave it out.
func (p *exprParser) optSemi() error {
	if p.isPunct(";") {
		return p.advance()
	}
	return nil
}

func (p *exprParser) isPunct(q string) bool {
	return p.tok.kind == tokPunct && p.tok.text == q
}

// kids returns the children of e: the fixed operands and the terms of a run.
func (e *expr) kids() []*expr {
	out := make([]*expr, 0, 3+len(e.list))
	for _, c := range []*expr{e.a, e.b, e.c} {
		if c != nil {
			out = append(out, c)
		}
	}
	return append(out, e.list...)
}

// mk sets the height of e and records an error when the tree is deeper than maxNesting.
func (p *exprParser) mk(e *expr) *expr {
	h := 0
	for _, c := range e.kids() {
		if c.h > h {
			h = c.h
		}
	}
	e.h = h
	if e.op != opOr && e.op != opAnd {
		e.h++
	}
	if e.h > maxNesting && p.deepErr == nil {
		p.deepErr = fmt.Errorf("expression nests deeper than %d", maxNesting)
	}
	return e
}

func (p *exprParser) enter() error {
	p.depth++
	if p.depth > maxNesting {
		return fmt.Errorf("expression nests deeper than %d", maxNesting)
	}
	return nil
}

func (p *exprParser) ternary() (*expr, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	c, err := p.or()
	if err != nil {
		return nil, err
	}
	if !p.isPunct("?") {
		return c, nil
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	a, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if err := p.want(":"); err != nil {
		return nil, err
	}
	b, err := p.ternary()
	if err != nil {
		return nil, err
	}
	return p.mk(&expr{op: opTern, a: c, b: a, c: b}), nil
}

func (p *exprParser) or() (*expr, error) {
	a, err := p.and()
	if err != nil {
		return nil, err
	}
	if !p.isPunct("||") {
		return a, nil
	}
	terms := []*expr{a}
	for p.isPunct("||") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		b, err := p.and()
		if err != nil {
			return nil, err
		}
		terms = append(terms, b)
	}
	return p.mk(&expr{op: opOr, list: terms}), nil
}

func (p *exprParser) and() (*expr, error) {
	a, err := p.equality()
	if err != nil {
		return nil, err
	}
	if !p.isPunct("&&") {
		return a, nil
	}
	terms := []*expr{a}
	for p.isPunct("&&") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		b, err := p.equality()
		if err != nil {
			return nil, err
		}
		terms = append(terms, b)
	}
	return p.mk(&expr{op: opAnd, list: terms}), nil
}

func (p *exprParser) equality() (*expr, error) {
	a, err := p.relational()
	if err != nil {
		return nil, err
	}
	for p.isPunct("==") || p.isPunct("!=") {
		op := opEq
		if p.tok.text == "!=" {
			op = opNe
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		b, err := p.relational()
		if err != nil {
			return nil, err
		}
		if err := checkCompare(a, b); err != nil {
			return nil, err
		}
		a = p.mk(&expr{op: op, a: a, b: b})
	}
	return a, nil
}

func (p *exprParser) relational() (*expr, error) {
	a, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.isPunct("<") || p.isPunct("<=") || p.isPunct(">") || p.isPunct(">=") {
		op := map[string]exprOp{"<": opLt, "<=": opLe, ">": opGt, ">=": opGe}[p.tok.text]
		if err := p.advance(); err != nil {
			return nil, err
		}
		b, err := p.unary()
		if err != nil {
			return nil, err
		}
		if err := checkCompare(a, b); err != nil {
			return nil, err
		}
		a = p.mk(&expr{op: op, a: a, b: b})
	}
	return a, nil
}

// checkCompare accepts only the generator's comparison: the lookahead against
// a constant, in either order. A Boolean-valued operand would put a boundary
// at a code point that no literal names.
func checkCompare(a, b *expr) error {
	if (a.op == opLook && b.op == opConst) || (a.op == opConst && b.op == opLook) {
		return nil
	}
	return fmt.Errorf("comparison of something other than the lookahead and a constant")
}

func (p *exprParser) unary() (*expr, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	if p.isPunct("!") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		a, err := p.unary()
		if err != nil {
			return nil, err
		}
		return p.mk(&expr{op: opNot, a: a}), nil
	}
	return p.primary()
}

func (p *exprParser) primary() (*expr, error) {
	t := p.tok
	switch {
	case p.isPunct("("):
		if err := p.advance(); err != nil {
			return nil, err
		}
		e, err := p.ternary()
		if err != nil {
			return nil, err
		}
		return e, p.want(")")
	case t.kind == tokNumber || t.kind == tokChar:
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &expr{op: opConst, val: t.num}, nil
	case t.kind == tokIdent:
		if err := p.advance(); err != nil {
			return nil, err
		}
		switch t.text {
		case p.look:
			return &expr{op: opLook}, nil
		case "eof":
			if !p.allowEOF {
				return nil, fmt.Errorf("line %d: eof is used but not declared", t.line)
			}
			return &expr{op: opEOF}, nil
		case "true":
			return &expr{op: opConst, val: 1}, nil
		case "false":
			return &expr{op: opConst, val: 0}, nil
		case "set_contains":
			if !p.allowSet {
				return nil, fmt.Errorf("line %d: set_contains is used but the header holds no verified implementation", t.line)
			}
			if err := p.want("("); err != nil {
				return nil, err
			}
			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("line %d: bad set_contains set name %v", t.line, p.tok)
			}
			name := p.tok.text
			if err := p.advance(); err != nil {
				return nil, err
			}
			if err := p.want(","); err != nil {
				return nil, err
			}
			if p.tok.kind != tokNumber {
				return nil, fmt.Errorf("line %d: bad set_contains length %v", t.line, p.tok)
			}
			n := p.tok.num
			if err := p.advance(); err != nil {
				return nil, err
			}
			if err := p.want(","); err != nil {
				return nil, err
			}
			if !(p.tok.kind == tokIdent && p.tok.text == p.look) {
				return nil, fmt.Errorf("line %d: set_contains must test %s", t.line, p.look)
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			if name == p.look {
				return nil, fmt.Errorf("line %d: set_contains uses %s, which is also the parameter name", t.line, name)
			}
			if _, ok := p.src.sets[name]; !ok {
				return nil, fmt.Errorf("line %d: set_contains uses %s, which is not defined before this point", t.line, name)
			}
			return &expr{op: opSet, name: name, val: n}, p.want(")")
		}
		if p.isPunct("(") { // name(arg)
			if _, ok := p.src.funcs[t.text]; !ok {
				return nil, fmt.Errorf("line %d: call of %s, which is not defined before this point", t.line, t.text)
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			arg, err := p.ternary()
			if err != nil {
				return nil, err
			}
			if arg.op != opLook && arg.op != opConst {
				return nil, fmt.Errorf("line %d: call of %s with an argument other than the lookahead", t.line, t.text)
			}
			return p.mk(&expr{op: opCall, name: t.text, a: arg}), p.want(")")
		}
		return nil, fmt.Errorf("line %d: unknown identifier %q", t.line, t.text)
	}
	return nil, fmt.Errorf("unexpected %v in expression", t)
}

// literals appends every integer that the expression compares with.
func (s *Source) literals(e *expr, seen map[*expr]bool, out map[int64]bool) {
	if e == nil || seen[e] {
		return
	}
	seen[e] = true
	switch e.op {
	case opConst:
		out[e.val] = true
	case opSet:
		for _, r := range s.sets[e.name][:e.val] {
			out[r.start], out[r.end] = true, true
		}
	case opCall:
		s.literals(s.funcs[e.name].body, seen, out)
	}
	for _, c := range e.kids() {
		s.literals(c, seen, out)
	}
}

// stateLiterals returns, sorted, the integers that state i tests.
func (l *Lexer) stateLiterals(i int) []int64 {
	out := map[int64]bool{}
	seen := map[*expr]bool{}
	for _, st := range l.states[i].stmts {
		l.src.literals(st.cond, seen, out)
		for _, p := range st.pairs {
			out[p.key] = true
		}
	}
	vs := make([]int64, 0, len(out))
	for v := range out {
		vs = append(vs, v)
	}
	sort.Slice(vs, func(a, b int) bool { return vs[a] < vs[b] })
	return vs
}

// maxCallDepth is the longest chain of helper calls that the evaluator follows.
const maxCallDepth = 16

// checkCallGraph rejects helper chains longer than maxCallDepth, so that
// evaluation never has to cut a call off. A helper may call only helpers defined
// before it (the parser enforces that), so the helpers in definition order are
// already ordered by their calls and one pass computes every depth without
// recursion over the call graph.
func (s *Source) checkCallGraph() error {
	depth := map[string]int{}
	var calls func(e *expr, out map[string]bool)
	calls = func(e *expr, out map[string]bool) { // bounded by the expression height
		if e == nil {
			return
		}
		if e.op == opCall {
			out[e.name] = true
		}
		for _, c := range e.kids() {
			calls(c, out)
		}
	}
	for _, name := range s.funcOrder {
		out := map[string]bool{}
		calls(s.funcs[name].body, out)
		best := 0
		for callee := range out {
			d, ok := depth[callee]
			if !ok {
				return fmt.Errorf("function %s calls %s, which is not defined before it", name, callee)
			}
			if d+1 > best {
				best = d + 1
			}
		}
		if best > maxCallDepth {
			return fmt.Errorf("function %s: helper calls nest deeper than %d", name, maxCallDepth)
		}
		depth[name] = best
	}
	return nil
}

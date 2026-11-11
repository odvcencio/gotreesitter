package lexexact

// Step is the step that C takes in one lex state for one input pair.
type Step struct {
	// Target is the next lex state.
	Target int
	// Skip is true for SKIP: the token start moves past the character.
	Skip bool
}

// memo caches the results of helper calls within one evaluation, by helper and
// argument. A helper is a pure function of its argument, so a guard that calls
// helpers in a tree of calls costs one evaluation per distinct (helper, argument).
type memo struct {
	cache map[memoKey]int64
}

type memoKey struct {
	name string
	arg  int64
}

// eval computes the C value of e with the integer rules of C: every test is an
// int, a true test is 1 and a false test is 0.
func (s *Source) eval(e *expr, look int64, eof bool, depth int, m *memo) int64 {
	b2i := func(b bool) int64 {
		if b {
			return 1
		}
		return 0
	}
	switch e.op {
	case opConst:
		return e.val
	case opLook:
		return look
	case opEOF:
		return b2i(eof)
	case opNot:
		return b2i(s.eval(e.a, look, eof, depth, m) == 0)
	case opOr:
		for _, t := range e.list {
			if s.eval(t, look, eof, depth, m) != 0 {
				return 1
			}
		}
		return 0
	case opAnd:
		for _, t := range e.list {
			if s.eval(t, look, eof, depth, m) == 0 {
				return 0
			}
		}
		return 1
	case opEq:
		return b2i(s.eval(e.a, look, eof, depth, m) == s.eval(e.b, look, eof, depth, m))
	case opNe:
		return b2i(s.eval(e.a, look, eof, depth, m) != s.eval(e.b, look, eof, depth, m))
	case opLt:
		return b2i(s.eval(e.a, look, eof, depth, m) < s.eval(e.b, look, eof, depth, m))
	case opLe:
		return b2i(s.eval(e.a, look, eof, depth, m) <= s.eval(e.b, look, eof, depth, m))
	case opGt:
		return b2i(s.eval(e.a, look, eof, depth, m) > s.eval(e.b, look, eof, depth, m))
	case opGe:
		return b2i(s.eval(e.a, look, eof, depth, m) >= s.eval(e.b, look, eof, depth, m))
	case opTern:
		if s.eval(e.a, look, eof, depth, m) != 0 {
			return s.eval(e.b, look, eof, depth, m)
		}
		return s.eval(e.c, look, eof, depth, m)
	case opSet:
		return b2i(setContains(s.sets[e.name], int(e.val), look))
	case opCall:
		// Parse rejects recursion and chains deeper than maxCallDepth, so depth stays small.
		arg := s.eval(e.a, look, eof, depth, m)
		k := memoKey{e.name, arg}
		if v, ok := m.cache[k]; ok {
			return v
		}
		v := s.eval(s.funcs[e.name].body, arg, eof, depth+1, m)
		if m.cache == nil {
			m.cache = map[memoKey]int64{}
		}
		m.cache[k] = v
		return v
	}
	return 0
}

// setContains is set_contains of tree_sitter/parser.h: a binary search over
// the first n ranges, written as in C.
func setContains(ranges []charRange, n int, look int64) bool {
	index := 0
	size := n - index
	for size > 1 {
		half := size / 2
		mid := index + half
		r := ranges[mid]
		if look >= r.start && look <= r.end {
			return true
		} else if look > r.end {
			index = mid
		}
		size -= half
	}
	r := ranges[index]
	return look >= r.start && look <= r.end
}

// Step returns the step of lex state `state` for the pair (eof, lookahead): the
// first statement whose guard holds. ok is false when C reaches END_STATE
// without a step. eof is ignored when the C function never sets `eof`.
// lookahead is -1 for an ill-formed byte and 0 at the end of input.
func (l *Lexer) Step(state int, eof bool, lookahead int32) (st Step, ok bool) {
	if state < 0 || state >= len(l.states) || !l.states[state].defined {
		return Step{}, false
	}
	eof = eof && l.eofSet
	look := int64(lookahead)
	m := &memo{}
	for _, s := range l.states[state].stmts {
		switch s.kind {
		case stmtMap:
			// map[i] is a uint16_t; it is promoted to int before the compare.
			for _, p := range s.pairs {
				if p.key == look {
					return Step{Target: int(p.target)}, true
				}
			}
			continue
		}
		if s.cond == nil || l.src.eval(s.cond, look, eof, 0, m) != 0 {
			return Step{Target: int(s.target), Skip: s.kind == stmtSkip}, true
		}
	}
	return Step{}, false
}

// Accept returns the symbol of ACCEPT_TOKEN in the state, if it has one.
func (l *Lexer) Accept(state int) (symbol int, ok bool) {
	if state < 0 || state >= len(l.states) || !l.states[state].hasAccept {
		return 0, false
	}
	return int(l.states[state].accept), true
}

// Defined reports whether the function has a case for the state.
func (l *Lexer) Defined(state int) bool {
	return state >= 0 && state < len(l.states) && l.states[state].defined
}

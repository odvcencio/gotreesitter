package lexbuild

import (
	"fmt"
	"sort"

	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// Prepared is the result of the grammar front end: what prepare_grammar
// (prepare_grammar.rs: fn prepare_grammar) hands to the table builder, reduced
// to what the lexer reads.
type Prepared struct {
	Syntax *SyntaxGrammar
	Lex    *LexicalGrammar

	large []largeSet
	haveL bool
}

// Prepare reads a grammar.json and runs the front end in the order of
// prepare_grammar.rs: fn prepare_grammar: intern symbols, extract tokens,
// expand repeats, flatten, expand tokens, with the checks of validate_precedences,
// validate_indirect_recursion and process_inlines. Not run: extract_default_aliases
// and the inline expansion itself (they change no token, FIRST set or LAST set).
func Prepare(grammarJSON []byte) (*Prepared, error) {
	g, err := ParseGrammar(grammarJSON)
	if err != nil {
		return nil, err
	}
	if err := validatePrecedences(g); err != nil {
		return nil, err
	}
	in, err := internSymbols(g)
	if err != nil {
		return nil, err
	}
	m, err := extractTokens(g, in)
	if err != nil {
		return nil, err
	}
	if err := expandRepeats(g, m); err != nil {
		return nil, err
	}
	syn, err := flattenGrammar(g, m)
	if err != nil {
		return nil, err
	}
	if err := validateIndirectRecursion(g, syn); err != nil {
		return nil, err
	}
	if err := checkInlines(g, m, syn); err != nil {
		return nil, err
	}
	lex, err := expandTokens(g.Pool, m.lexicalVariables, m.separatorRoots)
	if err != nil {
		return nil, err
	}
	return &Prepared{Syntax: syn, Lex: lex}, nil
}

func (p *Prepared) largeSets() []largeSet {
	if !p.haveL {
		p.large = collectLargeCharacterSets(newLexTableBuilder(p.Lex), p.Lex)
		p.haveL = true
	}
	return p.large
}

// ParseState is the input the lexer builder takes from a parse state: the
// tokens that are valid lookaheads in it (terminals, externals, and the end
// token). The tokens are an input: they come from the parse table.
type ParseState struct {
	Tokens TokenSet
}

// Table is a lex table with the rendered form of each state.
type Table struct {
	t     lexTable
	plans []statePlan
}

func (p *Prepared) newTable(t lexTable) *Table {
	tb := &Table{t: t}
	large := p.largeSets()
	for i := range t.states {
		tb.plans = append(tb.plans, planState(&t.states[i], large))
	}
	return tb
}

// BuildUnmerged builds one lex entry state for each token set, with no
// merging of token sets and no minimization: the construction of
// build_lex_table.rs: fn LexTableBuilder::add_state_for_tokens. It returns the
// table and the entry state of each set.
func (p *Prepared) BuildUnmerged(sets []TokenSet) (*Table, []uint32) {
	b := newLexTableBuilder(p.Lex)
	starts := make([]uint32, len(sets))
	for i := range sets {
		starts[i] = b.addStateForTokens(&sets[i])
	}
	return p.newTable(b.table), starts
}

// NumStates is the number of lex states.
func (t *Table) NumStates() int { return len(t.t.states) }

// Accept returns the token accepted in the state, if any.
func (t *Table) Accept(state int) (Symbol, bool) {
	s := &t.t.states[state]
	return s.accept, s.hasAccept
}

// Step is the step of the state at (eof, lookahead): lookahead is -1 for an
// ill-formed byte and 0 at the end of input. ok is false if no statement holds.
func (t *Table) Step(state int, eof bool, lookahead int32) (target int, skip bool, ok bool) {
	tg, sk, ok := t.t.states[state].stepAt(&t.plans[state], eof, int64(lookahead))
	return int(tg), sk, ok
}

// Ranges returns the ranges of the state for valid code points (lookahead
// with eof false), sorted by start; Hi is inclusive. They are the steps of the
// rendered state, not the character sets of its advance actions: the
// generator writes a negated set with an explicit test against 0, so the code
// point 0 can have no step although the set holds it. Ranges are found by
// evaluating the state between the points where a condition can change.
func (t *Table) Ranges(state int) []lexexact.Range {
	pts := map[uint32]bool{0: true, 1: true, 0xD800: true, 0xE000: true}
	for _, ca := range t.t.states[state].advances {
		for _, r := range ca.chars.r {
			pts[r.start] = true
			if r.end <= 0x10FFFF {
				pts[r.end] = true
			}
		}
	}
	list := make([]uint32, 0, len(pts))
	for p := range pts {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	var out []lexexact.Range
	st := &t.t.states[state]
	for k, lo := range list {
		hi := uint32(0x10FFFF)
		if k+1 < len(list) {
			hi = list[k+1] - 1
		}
		tg, sk, ok := st.stepAt(&t.plans[state], false, int64(lo))
		if !ok {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Hi+1 == lo && out[n-1].Target == tg && out[n-1].Skip == sk {
			out[n-1].Hi = hi
			continue
		}
		out = append(out, lexexact.Range{Lo: lo, Hi: hi, Target: tg, Skip: sk})
	}
	return out
}

// Record writes the table as a lexexact.Func. symbolID gives the C symbol id
// of a token (the end token is 0). start holds the lex state of each parse
// state (it may be nil).
func (t *Table) Record(symbolID func(Symbol) (uint32, bool), start []uint32) (lexexact.Func, error) {
	n := len(t.t.states)
	f := lexexact.Func{
		Accept: make([]uint32, n), OnEOF: make([]uint32, n), OnInvalid: make([]uint32, n),
		Trans: make([][]lexexact.Range, n), Start: start,
	}
	for i := 0; i < n; i++ {
		if a, ok := t.Accept(i); ok {
			id := uint32(0)
			if !a.IsEOF() {
				var found bool
				if id, found = symbolID(a); !found {
					return f, fmt.Errorf("lexbuild: state %d accepts a token with no symbol id: %v", i, a)
				}
			}
			f.Accept[i] = id + 1
		}
		tg, sk, ok := t.Step(i, true, 0)
		f.OnEOF[i] = lexexact.StepCode(lexexact.Step{Target: tg, Skip: sk}, ok)
		tg, sk, ok = t.Step(i, false, -1)
		f.OnInvalid[i] = lexexact.StepCode(lexexact.Step{Target: tg, Skip: sk}, ok)
		f.Trans[i] = t.Ranges(i)
	}
	return f, nil
}

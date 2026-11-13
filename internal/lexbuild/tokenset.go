package lexbuild

import "math/bits"

// TokenSet is rules.rs: struct TokenSet: sets of terminals and externals, the
// end-of-input token and the end-of-nonterminal-extra token. The Rust set is a
// pair of bit vectors; here each is a growable []uint64.
type TokenSet struct {
	terminals []uint64
	externals []uint64
	eof       bool
	eonte     bool
}

func bitSet(b *[]uint64, i int) {
	for len(*b) <= i/64 {
		*b = append(*b, 0)
	}
	(*b)[i/64] |= 1 << (uint(i) % 64)
}

func bitHas(b []uint64, i int) bool {
	return i/64 < len(b) && b[i/64]&(1<<(uint(i)%64)) != 0
}

// Insert adds a symbol (rules.rs: fn TokenSet::insert).
func (s *TokenSet) Insert(sym Symbol) {
	switch sym.Kind {
	case Terminal:
		bitSet(&s.terminals, int(sym.Index))
	case External:
		bitSet(&s.externals, int(sym.Index))
	case End:
		s.eof = true
	case EndOfNonTerminalExtra:
		s.eonte = true
	default:
		panic("lexbuild: TokenSet.Insert: non-terminal symbol")
	}
}

// Contains is rules.rs: fn TokenSet::contains.
func (s *TokenSet) Contains(sym Symbol) bool {
	switch sym.Kind {
	case Terminal:
		return bitHas(s.terminals, int(sym.Index))
	case External:
		return bitHas(s.externals, int(sym.Index))
	case End:
		return s.eof
	case EndOfNonTerminalExtra:
		return s.eonte
	}
	return false
}

func (s *TokenSet) containsTerminal(i int) bool { return bitHas(s.terminals, i) }

// insertAll is rules.rs: fn TokenSet::insert_all.
func (s *TokenSet) insertAll(o *TokenSet) {
	for i, w := range o.terminals {
		for len(s.terminals) <= i {
			s.terminals = append(s.terminals, 0)
		}
		s.terminals[i] |= w
	}
	for i, w := range o.externals {
		for len(s.externals) <= i {
			s.externals = append(s.externals, 0)
		}
		s.externals[i] |= w
	}
	s.eof = s.eof || o.eof
	s.eonte = s.eonte || o.eonte
}

// insertAllTerminals is rules.rs: fn TokenSet::insert_all_terminals.
func (s *TokenSet) insertAllTerminals(o *TokenSet) {
	for i, w := range o.terminals {
		for len(s.terminals) <= i {
			s.terminals = append(s.terminals, 0)
		}
		s.terminals[i] |= w
	}
}

func (s *TokenSet) clone() TokenSet {
	return TokenSet{append([]uint64(nil), s.terminals...), append([]uint64(nil), s.externals...), s.eof, s.eonte}
}

// terminalIndices lists the terminals in ascending order.
//
// Go shape: a pass over the words that skips to the next set bit, O(words +
// members); a test of all 64 bits of each word costs 64 steps for each word
// however few members it has.
func (s *TokenSet) terminalIndices() []int {
	var out []int
	for i, w := range s.terminals {
		for w != 0 {
			b := bits.TrailingZeros64(w)
			out = append(out, i*64+b)
			w &= w - 1
		}
	}
	return out
}

// symbols lists the members: terminals, then externals, then end, then
// end-of-nonterminal-extra. The Rust iteration order is not read by any
// function ported here in a way that reaches the output.
func (s *TokenSet) symbols() []Symbol {
	var out []Symbol
	for _, i := range s.terminalIndices() {
		out = append(out, terminalSym(i))
	}
	for i, w := range s.externals {
		for w != 0 {
			b := bits.TrailingZeros64(w)
			out = append(out, externalSym(i*64+b))
			w &= w - 1
		}
	}
	if s.eof {
		out = append(out, endSym())
	}
	if s.eonte {
		out = append(out, Symbol{EndOfNonTerminalExtra, 0})
	}
	return out
}

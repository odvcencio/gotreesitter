package lexbuild

// This file exports the few names that internal/tablebuild reads. Each wraps
// an existing unexported function or field; none adds behaviour.

// TerminalSym returns the terminal symbol with index i.
func TerminalSym(i int) Symbol { return terminalSym(i) }

// NonTerminalSym returns the non-terminal symbol with index i.
func NonTerminalSym(i int) Symbol { return nonTerminalSym(i) }

// ExternalSym returns the external token symbol with index i.
func ExternalSym(i int) Symbol { return externalSym(i) }

// EndSym returns the end-of-input symbol.
func EndSym() Symbol { return endSym() }

// EndOfNonTerminalExtraSym returns the end-of-non-terminal-extra symbol.
func EndOfNonTerminalExtraSym() Symbol { return Symbol{EndOfNonTerminalExtra, 0} }

// IsNonTerminal reports whether the symbol is a non-terminal.
func (s Symbol) IsNonTerminal() bool { return s.isNonTerminal() }

// IsExternal reports whether the symbol is an external token.
func (s Symbol) IsExternal() bool { return s.isExternal() }

// SymbolLess is the derived Ord of rules.rs: struct Symbol.
func SymbolLess(a, b Symbol) bool { return symbolLess(a, b) }

// Clone returns a copy of the set.
func (s *TokenSet) Clone() TokenSet { return s.clone() }

// Symbols lists the members in the iteration order of rules.rs: fn
// TokenSet::iter: terminals, externals, the end token, then the end of
// non-terminal extra token.
func (s *TokenSet) Symbols() []Symbol { return s.symbols() }

// InsertAll is rules.rs: fn TokenSet::insert_all. It reports whether the set
// changed.
func (s *TokenSet) InsertAll(o *TokenSet) bool {
	before := s.clone()
	s.insertAll(o)
	return !s.Equal(&before)
}

// Remove is rules.rs: fn TokenSet::remove. It reports whether the symbol was
// in the set.
func (s *TokenSet) Remove(sym Symbol) bool {
	if !s.Contains(sym) {
		return false
	}
	switch sym.Kind {
	case Terminal:
		s.terminals[sym.Index/64] &^= 1 << (sym.Index % 64)
	case External:
		s.externals[sym.Index/64] &^= 1 << (sym.Index % 64)
	case End:
		s.eof = false
	case EndOfNonTerminalExtra:
		s.eonte = false
	}
	return true
}

// Equal compares two sets as rules.rs does (trailing zero words do not count).
func (s *TokenSet) Equal(o *TokenSet) bool {
	return wordsEqual(s.terminals, o.terminals) && wordsEqual(s.externals, o.externals) && s.eof == o.eof && s.eonte == o.eonte
}

func wordsEqual(a, b []uint64) bool {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var x, y uint64
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return false
		}
	}
	return true
}

// Len is rules.rs: fn TokenSet::len.
func (s *TokenSet) Len() int { return len(s.symbols()) }

// IsEmpty is rules.rs: fn TokenSet::is_empty.
func (s *TokenSet) IsEmpty() bool { return s.Len() == 0 }

// Key returns a string that is equal for equal sets (equal as Equal reports).
func (s *TokenSet) Key() string {
	b := make([]byte, 0, 16)
	put := func(w []uint64) {
		n := len(w)
		for n > 0 && w[n-1] == 0 {
			n--
		}
		// The full length first, in eight bytes, then the words: the encoding of
		// a vector cannot run into the next one.
		for k := 0; k < 8; k++ {
			b = append(b, byte(uint64(n)>>(8*uint(k))))
		}
		for _, x := range w[:n] {
			for k := 0; k < 8; k++ {
				b = append(b, byte(x>>(8*uint(k))))
			}
		}
	}
	put(s.terminals)
	put(s.externals)
	if s.eof {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	if s.eonte {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	return string(b)
}

// FirstLastSets holds the FIRST and LAST sets of item_set_builder.rs: struct
// ParseItemSetBuilder (the fields first_sets and last_sets).
type FirstLastSets struct{ fl *firstLast }

// NewFirstLastSets is the part of item_set_builder.rs: fn
// ParseItemSetBuilder::new that fills first_sets and last_sets.
// It returns a *SizeError if the sets would pass the size limit.
func NewFirstLastSets(sg *SyntaxGrammar, lg *LexicalGrammar) (*FirstLastSets, error) {
	var fl *firstLast
	if err := guardSize(func() { fl = newFirstLast(sg, lg) }); err != nil {
		return nil, err
	}
	return &FirstLastSets{fl}, nil
}

// First returns the FIRST set of a terminal, external or non-terminal symbol.
func (f *FirstLastSets) First(s Symbol) *TokenSet { return f.fl.first[s] }

// Last returns the LAST set of a terminal, external or non-terminal symbol.
func (f *FirstLastSets) Last(s Symbol) *TokenSet { return f.fl.last[s] }

// DoesConflict is token_conflicts.rs: fn TokenConflictMap::does_conflict.
func (a *Analysis) DoesConflict(i, j int) bool { return a.conflicts.doesConflict(i, j) }

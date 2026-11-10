package ccore

// Tables is the language data the core reads. The root adapter builds one per
// language (ccore_bind.go); lookups are plain slice reads with no interface
// and no error return. Slices of basic types are shared with the language;
// the action, lex-mode and alias tables are converted once.
//
// Ports of C/language.c and C/language.h (runtime 0.27.1):
//   ts_language_lookup, ts_language_table_entry, ts_language_next_state,
//   ts_language_has_actions, ts_language_is_reserved_word,
//   ts_language_symbol_metadata, ts_language_symbol_name,
//   ts_language_lookaheads (ts_lookahead_iterator__next).
// The action-index and goto lookups return the same values as the root
// package's lookupActionIndex and lookupGoto; the cgo_harness test TestCcoreTables checks that.

// SmallPair is one (symbol, value) pair of a small state's sparse lookup.
type SmallPair struct{ Sym, Val uint16 }

// Parse action kinds, in the root package's order.
const (
	ActionShift uint8 = iota
	ActionReduce
	ActionAccept
	ActionRecover
)

// Action is one parse action (C TSParseAction).
type Action struct {
	Type              uint8
	State             uint32
	Symbol            Symbol
	ChildCount        uint8
	DynamicPrecedence int16
	ProductionID      uint16
	Extra             bool
	Repetition        bool
}

// ActionEntry is the group of actions for one (state, symbol) pair.
type ActionEntry struct {
	Reusable bool
	Actions  []Action
}

// LexMode is C's TSLexerMode. Index is the DFA start state the lex walk
// takes (the root's LexMode.LexStateIndex); it is a function of the other
// fields, so struct equality keeps the meaning C gives to mode equality.
type LexMode struct {
	LexState                  uint16
	ExternalLexState          uint16
	ReservedWordSetID         uint16
	AfterWhitespaceLexState   uint16
	LexStateID                uint32
	AfterWhitespaceLexStateID uint32
	Index                     uint32
}

// SymbolMeta is C's TSSymbolMetadata (the fields the core reads).
type SymbolMeta struct{ Visible, Named bool }

// Tables holds one language's parse tables and symbol data.
type Tables struct {
	TokenCount   uint32
	StateCount   uint32
	InitialState uint32
	DenseLimit   int
	SmallBase    int

	Dense            [][]uint16 // [state][symbol] -> action index
	SmallMap         []uint32   // small state -> offset into Small
	Small            []uint16   // compressed sparse table
	SmallTokenLookup [][]uint16 // derived dense rows of small states (may be nil)
	SmallLookup      [][]SmallPair
	LargeStateGotos  map[uint64]uint32 // key: state<<32 | symbol

	Actions  []ActionEntry
	LexModes []LexMode
	Meta     []SymbolMeta
	Names    []string
	Alias    [][]Symbol // [production id][structural child] -> alias symbol

	KeywordCapture         Symbol
	ReservedWords          []Symbol
	MaxReservedWordSetSize uint16

	// The lex automata (lexdfa.go). The ASCII tables are an optional speed-up
	// derived from the transitions; the walk gives the same answer without them.
	LexStates        []LexDFAState
	LexAscii         [][128]int32 `json:"-"`
	KeywordLexStates []LexDFAState
	KeywordLexAscii  [][128]int32 `json:"-"`

	ExternalSymbols   []Symbol // external token index -> symbol
	ExternalLexStates [][]bool // valid-symbol rows; empty: derived from the action table
}

// BuildAscii derives the ASCII fast-path tables from the lex states (a table
// read from a file has none).
func (t *Tables) BuildAscii() {
	t.LexAscii = BuildLexAscii(t.LexStates)
	t.KeywordLexAscii = nil
	if len(t.KeywordLexStates) != 0 {
		t.KeywordLexAscii = BuildLexAscii(t.KeywordLexStates)
	}
}

// ActionIndex is the root's lookupActionIndex: the action index for
// (state, symbol), or 0.
func (t *Tables) ActionIndex(state uint32, sym Symbol) uint16 {
	if int(state) < t.DenseLimit {
		if int(state) >= len(t.Dense) {
			return 0
		}
		row := t.Dense[state]
		if int(sym) >= len(row) {
			return 0
		}
		return row[sym]
	}
	smallIdx := int(state) - t.SmallBase
	if smallIdx < 0 || smallIdx >= len(t.SmallMap) {
		return 0
	}
	if smallIdx < len(t.SmallTokenLookup) {
		row := t.SmallTokenLookup[smallIdx]
		if int(sym) < len(row) {
			return row[sym]
		}
	}
	if smallIdx < len(t.SmallLookup) {
		pairs := t.SmallLookup[smallIdx]
		if len(pairs) > 0 {
			target := uint16(sym)
			if len(pairs) <= 8 {
				for i := 0; i < len(pairs); i++ {
					if pairs[i].Sym == target {
						return pairs[i].Val
					}
					if pairs[i].Sym > target {
						return 0
					}
				}
				return 0
			}
			lo, hi := 0, len(pairs)
			for lo < hi {
				mid := int(uint(lo+hi) >> 1)
				if pairs[mid].Sym < target {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
			if lo < len(pairs) && pairs[lo].Sym == target {
				return pairs[lo].Val
			}
			return 0
		}
	}
	offset := t.SmallMap[smallIdx]
	table := t.Small
	if int(offset) >= len(table) {
		return 0
	}
	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}

// Goto is the root's lookupGoto: the goto target of a nonterminal.
func (t *Tables) Goto(state uint32, sym Symbol) uint32 {
	if t.TokenCount > 0 && uint32(sym) >= t.TokenCount {
		if len(t.LargeStateGotos) != 0 {
			if target := t.LargeStateGotos[uint64(state)<<32|uint64(sym)]; target != 0 {
				return target
			}
		}
	}
	raw := t.ActionIndex(state, sym)
	if raw == 0 {
		return 0
	}
	if t.TokenCount > 0 && uint32(sym) >= t.TokenCount && t.StateCount > 0 && t.InitialState > 0 {
		return uint32(raw)
	}
	if int(raw) < len(t.Actions) {
		entry := &t.Actions[raw]
		if len(entry.Actions) > 0 && entry.Actions[0].Type == ActionShift {
			return entry.Actions[0].State
		}
	}
	return 0
}

type actionEntry struct { // C TableEntry
	actions  []Action
	reusable bool
}

func (p *Parser) tableEntry(state uint16, sym Symbol) actionEntry { // C/language.c ts_language_table_entry
	if sym == symError || sym == symErrorRepeat {
		return actionEntry{}
	}
	idx := p.t.ActionIndex(uint32(state), sym)
	if int(idx) >= len(p.t.Actions) {
		return actionEntry{}
	}
	e := &p.t.Actions[idx]
	return actionEntry{actions: e.Actions, reusable: e.Reusable}
}

func (p *Parser) nextState(state uint16, sym Symbol) uint16 { // C/language.h ts_language_next_state
	if sym == symError || sym == symErrorRepeat {
		return 0
	}
	if uint32(sym) < p.t.TokenCount {
		e := p.tableEntry(state, sym)
		if n := len(e.actions); n > 0 {
			a := e.actions[n-1]
			if a.Type == ActionShift {
				if a.Extra {
					return state
				}
				return uint16(a.State)
			}
		}
		return 0
	}
	return uint16(p.t.Goto(uint32(state), sym))
}

func (p *Parser) hasActions(state uint16, sym Symbol) bool { // C/language.h ts_language_has_actions
	return p.t.ActionIndex(uint32(state), sym) != 0
}

func (p *Parser) isReserved(state uint16, sym Symbol) bool { // C/language.h ts_language_is_reserved_word
	if int(state) >= len(p.t.LexModes) {
		return false
	}
	set := uint64(p.t.LexModes[state].ReservedWordSetID)
	if set == 0 {
		return false
	}
	// The product of two 16-bit values needs 32 bits; int is 32 bits on some
	// targets, so the index is computed in uint64 and bounded by the table.
	n := uint64(p.t.MaxReservedWordSetSize)
	start, words := set*n, uint64(len(p.t.ReservedWords))
	for i := start; i < start+n && i < words; i++ {
		if p.t.ReservedWords[i] == sym {
			return true
		}
		if p.t.ReservedWords[i] == 0 {
			break
		}
	}
	return false
}

func (p *Parser) meta(sym Symbol) (visible, named bool) { // C/language.c ts_language_symbol_metadata
	switch sym {
	case symError:
		return true, true
	case symErrorRepeat:
		return false, false
	}
	if int(sym) < len(p.t.Meta) {
		m := p.t.Meta[sym]
		return m.Visible, m.Named
	}
	return false, false
}

func (p *Parser) symName(sym Symbol) string { // C/language.c ts_language_symbol_name
	switch sym {
	case symError:
		return "ERROR"
	case symErrorRepeat:
		return "_ERROR"
	}
	if int(sym) < len(p.t.Names) {
		return p.t.Names[sym]
	}
	return ""
}

func (p *Parser) aliasSeq(productionID uint16) []Symbol {
	if int(productionID) < len(p.t.Alias) {
		return p.t.Alias[productionID]
	}
	return nil
}

// forEachLookahead is C/language.h ts_language_lookaheads with
// ts_lookahead_iterator__next: the symbols with an entry in a state, in
// table order, with their table value.
func (p *Parser) forEachLookahead(state uint16, visit func(sym Symbol, value uint16)) {
	t := p.t
	if int(state) < t.DenseLimit {
		if int(state) >= len(t.Dense) {
			return
		}
		row := t.Dense[state]
		for sym := range row {
			if row[sym] != 0 {
				visit(Symbol(sym), row[sym])
			}
		}
		return
	}
	idx := int(state) - t.SmallBase
	if idx < 0 || idx >= len(t.SmallMap) {
		return
	}
	table := t.Small
	pos := int(t.SmallMap[idx])
	if pos >= len(table) {
		return
	}
	groups := int(table[pos])
	pos++
	for g := 0; g < groups && pos+1 < len(table); g++ {
		value, count := table[pos], int(table[pos+1])
		pos += 2
		for k := 0; k < count && pos < len(table); k++ {
			visit(Symbol(table[pos]), value)
			pos++
		}
	}
}

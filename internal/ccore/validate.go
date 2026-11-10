package ccore

import (
	"errors"
	"fmt"
)

// Refusals of NewParser. Each has its own text; errors.Is tells them apart and
// the message adds the numbers that caused it.
var (
	ErrNoTables       = errors.New("ccore: no tables")
	ErrInitialState   = errors.New("ccore: initial state is not 1")
	ErrNoStates       = errors.New("ccore: tables have no states")
	ErrTooManyStates  = errors.New("ccore: more than 65,535 states")
	ErrTooManyTokens  = errors.New("ccore: more than 65,535 tokens")
	ErrTooManySymbols = errors.New("ccore: more than 65,534 symbols (65,534 and 65,535 are C's error and error-repeat symbols)")
	ErrLexModesShort  = errors.New("ccore: fewer lex modes than states")
	ErrNoLexAutomaton = errors.New("ccore: tables have no lex automaton")
	ErrKeywordCapture = errors.New("ccore: keyword capture symbol not below 255")
	ErrShiftState     = errors.New("ccore: shift action to a state above 65,535")
	ErrShiftTarget    = errors.New("ccore: shift action to a state at or above len(LexModes)")
	ErrGotoTarget     = errors.New("ccore: goto target at or above len(LexModes)")
	ErrGotoTooLarge   = errors.New("ccore: large-table goto target above 65,535")
	ErrScannerMissing = errors.New("ccore: tables have external symbols but the config has no scanner")
	ErrExternalRow    = errors.New("ccore: an external valid-symbol row is shorter than the external symbols")
)

// maxIndex is the largest state, token or symbol count the core's 16-bit
// fields can hold (C TSStateId and TSSymbol are uint16; 65,535 is the tree's
// "no state" marker, C/subtree.h TS_TREE_STATE_NONE).
const maxIndex = 65535

// maxInlineSymbol bounds the symbol an inline leaf stores (8 bits): C asserts
// symbol < UINT8_MAX in ts_subtree_set_symbol (C/subtree.c:234, reached when
// the keyword capture token replaces the symbol of an inline leaf), so the
// keyword capture symbol must be below 255.
const maxInlineSymbol = 255

// maxSymbols is the largest symbol count that leaves C's built-in symbols
// free: ts_builtin_sym_error is 65,535 and ts_builtin_sym_error_repeat is
// 65,534.
const maxSymbols = 65534

// validate checks the configuration once. The parse loop reads the tables
// without further checks of these facts. It changes nothing.
func (c Config) validate() error {
	t := c.Tables
	if t == nil {
		return ErrNoTables
	}
	if t.InitialState != 1 {
		// C tables start in state 1; state 0 is the error state. A hand-built
		// table with initial state 0 would start in state 1 anyway.
		return fmt.Errorf("%w (got %d)", ErrInitialState, t.InitialState)
	}
	if t.StateCount == 0 {
		return ErrNoStates
	}
	if t.StateCount > maxIndex {
		return fmt.Errorf("%w (got %d)", ErrTooManyStates, t.StateCount)
	}
	if t.TokenCount > maxIndex {
		return fmt.Errorf("%w (got %d)", ErrTooManyTokens, t.TokenCount)
	}
	if n := len(t.Meta); n > maxSymbols {
		return fmt.Errorf("%w (got %d)", ErrTooManySymbols, n)
	}
	if uint64(len(t.LexModes)) < uint64(t.StateCount) {
		return fmt.Errorf("%w (%d modes, %d states)", ErrLexModesShort, len(t.LexModes), t.StateCount)
	}
	if len(t.LexStates) == 0 {
		return ErrNoLexAutomaton
	}
	if len(t.KeywordLexStates) != 0 && t.KeywordCapture >= maxInlineSymbol {
		return fmt.Errorf("%w (got %d)", ErrKeywordCapture, t.KeywordCapture)
	}
	for i := range t.Actions {
		for _, a := range t.Actions[i].Actions {
			if a.Type != ActionShift {
				continue
			}
			if a.State > maxIndex {
				return fmt.Errorf("%w (action entry %d, state %d)", ErrShiftState, i, a.State)
			}
			if uint64(a.State) >= uint64(len(t.LexModes)) {
				return fmt.Errorf("%w (action entry %d, state %d, %d states)", ErrShiftTarget, i, a.State, len(t.LexModes))
			}
		}
	}
	if err := t.validateGotos(); err != nil {
		return err
	}
	if len(t.ExternalSymbols) != 0 && c.Scanner == nil {
		return fmt.Errorf("%w (%d external symbols)", ErrScannerMissing, len(t.ExternalSymbols))
	}
	// The scanner port reads the valid-symbol row of an external lex state by
	// external token index (scanner.go passes rows[ExternalLexState]); a row
	// shorter than the external symbols would be read past its end. Row 0 is
	// "no external scanning" and is never passed. An empty list means the rows
	// are derived from the action table.
	for i, row := range t.ExternalLexStates {
		if i > 0 && len(row) < len(t.ExternalSymbols) {
			return fmt.Errorf("%w (row %d has %d entries, %d external symbols)", ErrExternalRow, i, len(row), len(t.ExternalSymbols))
		}
	}
	return nil
}

// validateGotos checks every goto target of the parse tables: the dense rows,
// the derived dense rows of small states, the compressed small-state table,
// the sparse small-state lookups and the large-state goto map. A goto target
// is the value stored for a nonterminal (a symbol at or above the token count);
// it must be a state, that is, below len(LexModes), because the parse loop
// indexes LexModes with it. A large-table goto above 65,535 does not fit C's
// 16-bit state.
func (t *Tables) validateGotos() error {
	states := uint64(len(t.LexModes))
	bad := func(where string, v uint64) error {
		if v > maxIndex {
			return fmt.Errorf("%w (%s, %d)", ErrGotoTooLarge, where, v)
		}
		return fmt.Errorf("%w (%s, %d, %d states)", ErrGotoTarget, where, v, states)
	}
	for s, row := range t.Dense {
		for sym := int(t.TokenCount); sym < len(row); sym++ {
			if v := uint64(row[sym]); v >= states {
				return bad(fmt.Sprintf("dense state %d symbol %d", s, sym), v)
			}
		}
	}
	for s, row := range t.SmallTokenLookup {
		for sym := int(t.TokenCount); sym < len(row); sym++ {
			if v := uint64(row[sym]); v >= states {
				return bad(fmt.Sprintf("small state %d derived row symbol %d", s, sym), v)
			}
		}
	}
	for key, v := range t.LargeStateGotos {
		if uint64(v) > maxIndex || uint64(v) >= states {
			return bad(fmt.Sprintf("large state %d symbol %d", key>>32, key&0xFFFFFFFF), uint64(v))
		}
	}
	for i, pairs := range t.SmallLookup {
		for _, p := range pairs {
			if uint32(p.Sym) >= t.TokenCount && uint64(p.Val) >= states {
				return bad(fmt.Sprintf("small state %d symbol %d", i, p.Sym), uint64(p.Val))
			}
		}
	}
	for i, offset := range t.SmallMap {
		table := t.Small
		if int(offset) >= len(table) {
			continue
		}
		groups := table[offset]
		pos := int(offset) + 1
		for g := uint16(0); g < groups; g++ {
			if pos+1 >= len(table) {
				break
			}
			value, count := table[pos], table[pos+1]
			pos += 2
			for j := uint16(0); j < count && pos < len(table); j++ {
				if uint32(table[pos]) >= t.TokenCount && uint64(value) >= states {
					return bad(fmt.Sprintf("small state %d symbol %d", i, table[pos]), uint64(value))
				}
				pos++
			}
		}
	}
	return nil
}

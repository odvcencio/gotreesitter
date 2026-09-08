package gotreesitter

import (
	"testing"
)

// makeLookaheadLang builds a minimal Language with both dense and small
// parse tables for testing the LookaheadIterator.
//
// Dense table (states 0-1, LargeStateCount=2):
//
//	state 0: symbol 1 -> action 1, symbol 3 -> action 2
//	state 1: symbol 2 -> action 3
//
// Small table (state 2 = LargeStateCount + 0):
//
//	state 2: group0(action=1, symbols=[0,3]), group1(action=2, symbols=[4])
func makeLookaheadLang() *Language {
	// Dense parse table: 2 states, 5 symbols each.
	denseTable := [][]uint16{
		{0, 1, 0, 2, 0}, // state 0: sym1->1, sym3->2
		{0, 0, 3, 0, 0}, // state 1: sym2->3
	}

	// Small parse table for state 2.
	// Format: groupCount, [sectionValue, symbolCount, sym...] ...
	smallTable := []uint16{
		2,    // groupCount
		1, 2, // group 0: action=1, 2 symbols
		0, 3, // sym 0, sym 3
		2, 1, // group 1: action=2, 1 symbol
		4, // sym 4
	}
	smallMap := []uint32{0} // state 2 -> offset 0

	return &Language{
		Name:            "lookahead_test",
		SymbolCount:     5,
		TokenCount:      5,
		StateCount:      3,
		LargeStateCount: 2,
		SymbolNames:     []string{"end", "identifier", "number", "plus", "star"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "number", Visible: true, Named: true},
			{Name: "+", Visible: true},
			{Name: "*", Visible: true},
		},
		ParseTable:         denseTable,
		SmallParseTable:    smallTable,
		SmallParseTableMap: smallMap,
		ParseActions: []ParseActionEntry{
			{}, // index 0: no action
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}}},
		},
	}
}

func TestLookaheadDenseState(t *testing.T) {
	lang := makeLookaheadLang()
	it, err := NewLookaheadIterator(lang, 0)
	if err != nil {
		t.Fatalf("NewLookaheadIterator state 0: %v", err)
	}

	if it.Language() != lang {
		t.Error("Language() returned wrong pointer")
	}

	var syms []Symbol
	for it.Next() {
		syms = append(syms, it.CurrentSymbol())
	}

	// State 0 has actions for symbols 1 and 3.
	if len(syms) != 2 {
		t.Fatalf("state 0: got %d symbols, want 2: %v", len(syms), syms)
	}
	if syms[0] != 1 || syms[1] != 3 {
		t.Errorf("state 0 symbols: got %v, want [1 3]", syms)
	}
}

func TestLookaheadDenseStateNames(t *testing.T) {
	lang := makeLookaheadLang()
	it, err := NewLookaheadIterator(lang, 0)
	if err != nil {
		t.Fatalf("NewLookaheadIterator: %v", err)
	}

	var names []string
	for it.Next() {
		names = append(names, it.CurrentSymbolName())
	}

	want := []string{"identifier", "plus"}
	if len(names) != len(want) {
		t.Fatalf("got names %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("name[%d]: got %q, want %q", i, names[i], want[i])
		}
	}
}

func TestLookaheadSmallState(t *testing.T) {
	lang := makeLookaheadLang()
	it, err := NewLookaheadIterator(lang, 2)
	if err != nil {
		t.Fatalf("NewLookaheadIterator state 2: %v", err)
	}

	var syms []Symbol
	for it.Next() {
		syms = append(syms, it.CurrentSymbol())
	}

	// State 2 (small table): group0 action=1 with symbols [0,3], group1 action=2 with symbols [4].
	// All have non-zero action indices, so all 3 symbols are valid.
	if len(syms) != 3 {
		t.Fatalf("state 2: got %d symbols, want 3: %v", len(syms), syms)
	}
	if syms[0] != 0 || syms[1] != 3 || syms[2] != 4 {
		t.Errorf("state 2 symbols: got %v, want [0 3 4]", syms)
	}
}

func TestLookaheadSmallStateZeroAction(t *testing.T) {
	// Build a small table where one group has action index 0 (no action).
	smallTable := []uint16{
		2,    // groupCount
		0, 2, // group 0: action=0 (no action), 2 symbols
		1, 2, // sym 1, sym 2
		5, 1, // group 1: action=5, 1 symbol
		3, // sym 3
	}
	lang := &Language{
		Name:               "zero_action_test",
		SymbolCount:        5,
		StateCount:         2,
		LargeStateCount:    1,
		ParseTable:         [][]uint16{{0, 0, 0, 0, 0}}, // state 0: no actions
		SmallParseTable:    smallTable,
		SmallParseTableMap: []uint32{0},
		SymbolNames:        []string{"end", "a", "b", "c", "d"},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}

	it, err := NewLookaheadIterator(lang, 1) // state 1 -> smallIdx 0
	if err != nil {
		t.Fatalf("NewLookaheadIterator: %v", err)
	}

	var syms []Symbol
	for it.Next() {
		syms = append(syms, it.CurrentSymbol())
	}

	// Only group 1 (action=5) should contribute: symbol 3.
	if len(syms) != 1 {
		t.Fatalf("got %d symbols, want 1: %v", len(syms), syms)
	}
	if syms[0] != 3 {
		t.Errorf("got symbol %d, want 3", syms[0])
	}
}

func TestLookaheadResetState(t *testing.T) {
	lang := makeLookaheadLang()
	it, err := NewLookaheadIterator(lang, 0)
	if err != nil {
		t.Fatalf("NewLookaheadIterator: %v", err)
	}

	// Drain state 0.
	count0 := 0
	for it.Next() {
		count0++
	}
	if count0 != 2 {
		t.Fatalf("state 0: got %d symbols, want 2", count0)
	}

	// Reset to state 1.
	if err := it.ResetState(1); err != nil {
		t.Fatalf("ResetState(1): %v", err)
	}

	var syms []Symbol
	for it.Next() {
		syms = append(syms, it.CurrentSymbol())
	}
	if len(syms) != 1 || syms[0] != 2 {
		t.Errorf("state 1 after reset: got %v, want [2]", syms)
	}

	// Reset to small table state 2.
	if err := it.ResetState(2); err != nil {
		t.Fatalf("ResetState(2): %v", err)
	}
	count2 := 0
	for it.Next() {
		count2++
	}
	if count2 != 3 {
		t.Errorf("state 2 after reset: got %d symbols, want 3", count2)
	}
}

func TestLookaheadInvalidState(t *testing.T) {
	lang := makeLookaheadLang()

	// State 3 is out of range (only 3 states: 0, 1, 2).
	_, err := NewLookaheadIterator(lang, 3)
	if err == nil {
		t.Error("expected error for out-of-range state, got nil")
	}

	// State 100 is way out of range.
	_, err = NewLookaheadIterator(lang, 100)
	if err == nil {
		t.Error("expected error for state 100, got nil")
	}
}

func TestLookaheadNilLanguage(t *testing.T) {
	_, err := NewLookaheadIterator(nil, 0)
	if err == nil {
		t.Error("expected error for nil language, got nil")
	}
}

func TestLookaheadEmptyState(t *testing.T) {
	// Test a state with no valid symbols.
	lang2 := &Language{
		Name:            "empty_test",
		SymbolCount:     3,
		StateCount:      1,
		LargeStateCount: 1,
		ParseTable:      [][]uint16{{0, 0, 0}}, // state 0: no actions
		SymbolNames:     []string{"end", "a", "b"},
	}
	it, err := NewLookaheadIterator(lang2, 0)
	if err != nil {
		t.Fatalf("NewLookaheadIterator: %v", err)
	}
	if it.Next() {
		t.Error("expected no symbols for empty state, but Next() returned true")
	}
}

func TestLookaheadCurrentSymbolBeforeNext(t *testing.T) {
	lang := makeLookaheadLang()
	it, err := NewLookaheadIterator(lang, 0)
	if err != nil {
		t.Fatalf("NewLookaheadIterator: %v", err)
	}

	// Before calling Next(), pos is -1 which is out of range,
	// so CurrentSymbol returns 0 (the zero value).
	if sym := it.CurrentSymbol(); sym != 0 {
		t.Errorf("CurrentSymbol before Next: got %d, want 0", sym)
	}
}

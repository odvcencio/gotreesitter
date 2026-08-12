package gotreesitter

import (
	"sort"
	"testing"
)

// TestForEachActionIndexInState_DenseRowPlusOverflow is a regression test
// for a bug in forEachActionIndexInState: for a small (sparse) parser state
// that has both a populated dense smallTokenLookup row and a non-empty
// smallLookup overflow slice, the walk visited only the overflow pairs and
// silently skipped every entry in the dense row.
//
// The dense row and the overflow slice are disjoint by construction (see
// buildSmallLookup, which only admits symbols at or beyond the dense row's
// length into overflow), so the correct walk enumerates both with no
// duplicates.
//
// Witness: Rust's call_expression reduce action lives in the dense row of
// small states 1479 and 1923, both of which also carry overflow entries, so
// a row-enumeration walk silently dropped every one of them.
func TestForEachActionIndexInState_DenseRowPlusOverflow(t *testing.T) {
	lang := loadBlobForDecode(t, "rust")
	p := NewParser(lang)

	witnesses := []struct {
		state StateID
		desc  string
	}{
		{state: 1479, desc: "call_expression reduce state (dense row hides binary/postfix operator entries)"},
		{state: 1923, desc: "call_expression reduce state (dense row hides shift/postfix operator entries)"},
	}
	for _, w := range witnesses {
		t.Run(w.desc, func(t *testing.T) {
			requireDenseAndOverflow(t, p, w.state)
			assertForEachActionIndexMatchesLookup(t, p, lang, w.state)
		})
	}

	// Broader sweep: every rust small state that carries both a populated
	// dense row and a populated overflow slice must enumerate identically
	// to a direct lookupActionIndex probe, not just the two named
	// witnesses above.
	t.Run("all rust dense+overflow small states", func(t *testing.T) {
		stateCount := int(lang.StateCount)
		if stateCount == 0 {
			stateCount = len(lang.ParseTable) + len(lang.SmallParseTableMap)
		}
		checked := 0
		for st := p.smallBase; st < stateCount; st++ {
			smallIdx := st - p.smallBase
			if smallIdx < 0 || smallIdx >= len(p.smallTokenLookup) || smallIdx >= len(p.smallLookup) {
				continue
			}
			if len(p.smallTokenLookup[smallIdx]) == 0 || len(p.smallLookup[smallIdx]) == 0 {
				continue // not a dense+overflow state; does not exercise the bug
			}
			checked++
			assertForEachActionIndexMatchesLookup(t, p, lang, StateID(st))
		}
		if checked == 0 {
			t.Fatal("expected at least one rust small state with both a dense row and overflow entries")
		}
		t.Logf("checked %d dense+overflow small states", checked)
	})
}

// requireDenseAndOverflow fails the test if state is not backed by a small
// state carrying both a populated dense smallTokenLookup row and a
// populated smallLookup overflow slice — i.e. if it does not actually
// exercise the bug this test guards against.
func requireDenseAndOverflow(t *testing.T, p *Parser, state StateID) {
	t.Helper()
	smallIdx := int(state) - p.smallBase
	if smallIdx < 0 || smallIdx >= len(p.smallTokenLookup) || smallIdx >= len(p.smallLookup) {
		t.Fatalf("state %d: not a small state with both smallTokenLookup and smallLookup entries", state)
	}
	if len(p.smallTokenLookup[smallIdx]) == 0 {
		t.Fatalf("state %d: expected a populated dense smallTokenLookup row", state)
	}
	if len(p.smallLookup[smallIdx]) == 0 {
		t.Fatalf("state %d: expected a populated smallLookup overflow slice", state)
	}
}

// assertForEachActionIndexMatchesLookup compares forEachActionIndexInState's
// enumeration for state against a ground-truth probe of lookupActionIndex
// over every symbol id the language defines, failing with the exact
// state/symbol mismatch it finds rather than a generic count diff.
func assertForEachActionIndexMatchesLookup(t *testing.T, p *Parser, lang *Language, state StateID) {
	t.Helper()

	got := map[Symbol]uint16{}
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if prior, dup := got[sym]; dup {
			t.Errorf("state %d: forEachActionIndexInState visited symbol %d twice (idx %d then %d)", state, sym, prior, idx)
		}
		got[sym] = idx
		return true
	})

	want := map[Symbol]uint16{}
	symbolLimit := len(lang.SymbolNames)
	for sym := 0; sym < symbolLimit; sym++ {
		if idx := p.lookupActionIndex(state, Symbol(sym)); idx != 0 {
			want[Symbol(sym)] = idx
		}
	}

	if len(got) != len(want) {
		var missing, extra []int
		for sym := range want {
			if _, ok := got[sym]; !ok {
				missing = append(missing, int(sym))
			}
		}
		for sym := range got {
			if _, ok := want[sym]; !ok {
				extra = append(extra, int(sym))
			}
		}
		sort.Ints(missing)
		sort.Ints(extra)
		t.Fatalf("state %d: forEachActionIndexInState found %d symbols, lookupActionIndex probe found %d; missing=%v extra=%v",
			state, len(got), len(want), missing, extra)
	}
	for sym, wantIdx := range want {
		if gotIdx, ok := got[sym]; !ok || gotIdx != wantIdx {
			t.Fatalf("state %d symbol %d: forEachActionIndexInState=%d(present=%v) lookupActionIndex=%d",
				state, sym, gotIdx, ok, wantIdx)
		}
	}
}

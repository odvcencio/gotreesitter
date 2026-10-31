package grammargen

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Error-mode lexing can return KeywordCaptureToken even when the current
// parse state has no ordinary word or keyword action. An explicit global
// reservation must still reject those keyword spellings as identifiers.
func TestReservedGlobalAppliesWithoutNormalWordAction(t *testing.T) {
	const (
		word        gotreesitter.Symbol = 1
		kwIf        gotreesitter.Symbol = 2
		kwFor       gotreesitter.Symbol = 3
		contextOnly gotreesitter.Symbol = 4
	)
	lang := &gotreesitter.Language{
		KeywordCaptureToken: word,
		LargeStateCount:     4,
		ParseTable: [][]uint16{
			{0, 0, 0, 0, 0}, // 0: C error state retains its special empty set.
			{0, 0, 0, 0, 1}, // 1: no ordinary word/keyword action.
			{0, 1, 0, 0, 0}, // 2: ordinary word action.
			{0, 0, 1, 0, 0}, // 3: "if" is explicitly valid.
		},
		// 4: context-only action; 5: both global keywords explicitly valid.
		SmallParseTable:    []uint16{1, 1, 1, 4, 1, 1, 2, 2, 3},
		SmallParseTableMap: []uint32{0, 4},
		LexModes: []gotreesitter.LexMode{
			{LexState: 0}, {LexState: 1}, {LexState: 1},
			{LexState: 1}, {LexState: 1}, {LexState: 1},
		},
	}
	ng := &NormalizedGrammar{
		GrammarName:      "explicit_global_error_fallback",
		WordSymbolID:     int(word),
		KeywordSymbols:   []int{int(kwIf), int(kwFor)},
		ReservedWordSets: [][]int{{int(kwIf), int(kwFor)}, {int(contextOnly)}},
	}
	for _, state := range []gotreesitter.StateID{1, 4} {
		for _, sym := range []gotreesitter.Symbol{word, kwIf, kwFor} {
			if lookupActionIndexForLanguage(lang, state, sym) != 0 {
				t.Fatalf("witness state %d unexpectedly accepts word/keyword %d", state, sym)
			}
		}
	}
	if err := buildReservedWordTables(lang, ng); err != nil {
		t.Fatal(err)
	}
	for _, state := range []gotreesitter.StateID{1, 2, 4} {
		if !reservedSetContains(lang, state, kwIf) || !reservedSetContains(lang, state, kwFor) {
			t.Fatalf("state %d lost explicit global reservation: set=%d", state, lang.LexModes[state].ReservedWordSetID)
		}
	}
	if reservedSetContains(lang, 3, kwIf) || !reservedSetContains(lang, 3, kwFor) {
		t.Fatal("global-minus-valid-keyword policy changed for a partially valid state")
	}
	if lang.LexModes[0].ReservedWordSetID != 0 || lang.LexModes[5].ReservedWordSetID != 0 {
		t.Fatalf("error state/all-valid state should retain empty sets: %d/%d", lang.LexModes[0].ReservedWordSetID, lang.LexModes[5].ReservedWordSetID)
	}
	if lang.LexModes[1].ReservedWordSetID != lang.LexModes[2].ReservedWordSetID ||
		lang.LexModes[1].ReservedWordSetID != lang.LexModes[4].ReservedWordSetID {
		t.Fatal("equal global subsets were not shared across dense and sparse states")
	}
	for state := gotreesitter.StateID(0); int(state) < len(lang.LexModes); state++ {
		if reservedSetContains(lang, state, contextOnly) {
			t.Fatalf("state %d incorrectly adopted an unrelated context's reserved set", state)
		}
	}
}

func TestReservedGlobalDoesNotInventReservations(t *testing.T) {
	for _, test := range []struct {
		name string
		sets [][]int
	}{
		{name: "no-explicit-sets"},
		{name: "empty-global-context-only", sets: [][]int{{}, {2}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			lang := &gotreesitter.Language{LexModes: make([]gotreesitter.LexMode, 3)}
			ng := &NormalizedGrammar{GrammarName: "without_global", WordSymbolID: 1, KeywordSymbols: []int{2}, ReservedWordSets: test.sets}
			if err := buildReservedWordTables(lang, ng); err != nil {
				t.Fatal(err)
			}
			if len(lang.ReservedWords) != 0 || lang.MaxReservedWordSetSize != 0 {
				t.Fatalf("invented reserved words: size=%d words=%v", lang.MaxReservedWordSetSize, lang.ReservedWords)
			}
			for state, mode := range lang.LexModes {
				if mode.ReservedWordSetID != 0 {
					t.Fatalf("state %d invented reserved set %d", state, mode.ReservedWordSetID)
				}
			}
		})
	}
}

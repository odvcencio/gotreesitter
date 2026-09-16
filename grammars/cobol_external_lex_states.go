//go:build !grammar_subset || grammar_subset_cobol

// Cobol precise ExternalLexStates - DEFAULT since the Wave 4 Cobol cleanup.
//
// The table below is copied verbatim from tree-sitter-cobol parser.c. It was
// previously staged behind -tags cobol_precise_els while BBANK10P.cbl timed out
// before oracle comparison. The BBANK10P witness is now bounded and matches C,
// so the standard tier runner can use C recovery with the precise external
// scanner state table by default. Broader Cobol recovery/frontier gaps remain
// tracked in the tier ledger.
//
// Source: https://github.com/yutaro-sakamoto/tree-sitter-cobol e99dbdc3d800d5fa2796476efd60af91f6b43d93 src/parser.c

package grammars

// cobolExternalLexStates mirrors C tree-sitter ts_external_scanner_states.
var cobolExternalLexStates = [][]bool{
	/* 0 */ {false, false, false, false, false, false},
	/* 1 */ {true, true, true, true, true, true},
	/* 2 */ {true, true, true, true, false, false},
	/* 3 */ {true, true, true, true, false, true},
	/* 4 */ {true, true, true, true, true, false},
}

func init() {
	RegisterExternalLexStates("cobol", cobolExternalLexStates)
}

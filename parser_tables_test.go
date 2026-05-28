package gotreesitter

import (
	"reflect"
	"testing"
)

func TestLookupActionIndexSmallUsesDenseTokenRows(t *testing.T) {
	lang := &Language{
		Name:               "cobol",
		TokenCount:         64,
		LargeStateCount:    1,
		SmallParseTableMap: []uint32{0},
		// groupCount=2
		// action 11 for token symbols 1..13
		// action 17 for nonterminal symbol 70
		SmallParseTable: []uint16{
			2,
			11, 13, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13,
			17, 1, 70,
		},
	}

	smallTokenLookup := buildSmallTokenLookup(lang)
	p := &Parser{
		language:         lang,
		smallBase:        int(lang.LargeStateCount),
		smallLookup:      buildSmallLookup(lang, smallTokenLookup),
		smallTokenLookup: smallTokenLookup,
	}

	if got, want := p.lookupActionIndexSmall(1, 1), uint16(11); got != want {
		t.Fatalf("lookupActionIndexSmall token 1 = %d, want %d", got, want)
	}
	if got, want := p.lookupActionIndexSmall(1, 13), uint16(11); got != want {
		t.Fatalf("lookupActionIndexSmall token 13 = %d, want %d", got, want)
	}
	if got := p.lookupActionIndexSmall(1, 14); got != 0 {
		t.Fatalf("lookupActionIndexSmall missing token = %d, want 0", got)
	}
	if got, want := p.lookupActionIndexSmall(1, 70), uint16(17); got != want {
		t.Fatalf("lookupActionIndexSmall nonterminal = %d, want %d", got, want)
	}
	if len(p.smallTokenLookup) != 1 || len(p.smallTokenLookup[0]) != 14 {
		t.Fatalf("smallTokenLookup row missing or wrong size: %+v", p.smallTokenLookup)
	}
	if len(p.smallLookup) != 1 || len(p.smallLookup[0]) != 1 {
		t.Fatalf("smallLookup should retain only nonterminals for dense token rows: %+v", p.smallLookup)
	}
}

func TestLookupActionIndexSmallUsesFullTokenRowsForOtherLanguages(t *testing.T) {
	lang := &Language{
		Name:               "scala",
		TokenCount:         64,
		LargeStateCount:    1,
		SmallParseTableMap: []uint32{0},
		// groupCount=2
		// action 11 for token symbols 1..9
		// action 17 for nonterminal symbol 70
		SmallParseTable: []uint16{
			2,
			11, 9, 1, 2, 3, 4, 5, 6, 7, 8, 9,
			17, 1, 70,
		},
	}

	smallTokenLookup := buildSmallTokenLookup(lang)
	p := &Parser{
		language:         lang,
		smallBase:        int(lang.LargeStateCount),
		smallLookup:      buildSmallLookup(lang, smallTokenLookup),
		smallTokenLookup: smallTokenLookup,
	}

	if got, want := p.lookupActionIndexSmall(1, 9), uint16(11); got != want {
		t.Fatalf("lookupActionIndexSmall token 9 = %d, want %d", got, want)
	}
	if got := p.lookupActionIndexSmall(1, 63); got != 0 {
		t.Fatalf("lookupActionIndexSmall missing high token = %d, want 0", got)
	}
	if got, want := p.lookupActionIndexSmall(1, 70), uint16(17); got != want {
		t.Fatalf("lookupActionIndexSmall nonterminal = %d, want %d", got, want)
	}
	if len(p.smallTokenLookup) != 1 || len(p.smallTokenLookup[0]) != int(lang.TokenCount) {
		t.Fatalf("smallTokenLookup should use full token row for non-COBOL languages: %+v", p.smallTokenLookup)
	}
	if len(p.smallLookup) != 1 || len(p.smallLookup[0]) != 1 {
		t.Fatalf("smallLookup should retain only nonterminals for dense token rows: %+v", p.smallLookup)
	}
}

func TestBuildExternalValidByStateUsesCompactExternalIndexes(t *testing.T) {
	lang := &Language{
		TokenCount:      5,
		StateCount:      3,
		ExternalSymbols: []Symbol{2, 4},
		ParseTable: [][]uint16{
			{0, 0, 7, 0, 8},
			{0, 0, 0, 0, 0},
			{0, 0, 0, 0, 9},
		},
	}
	p := &Parser{
		language:   lang,
		denseLimit: len(lang.ParseTable),
	}

	got := p.buildExternalValidByState()
	want := [][]uint16{{0, 1}, nil, {1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildExternalValidByState() = %#v, want %#v", got, want)
	}

	lang.ExternalLexStates = [][]bool{{false, false}}
	if got := p.buildExternalValidByState(); got != nil {
		t.Fatalf("buildExternalValidByState() with external lex states = %#v, want nil", got)
	}
}

package grammargen

import (
	"context"
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestGeneratedErrorModeRecognizesNonInitialPunctuation(t *testing.T) {
	g := NewGrammar("error_mode_punctuation")
	g.SeparateErrorLexMode = true
	g.Define("source_file", Seq(Str("start"), Str("="), Sym("identifier"), Str(";")))
	g.Define("identifier", Pat("[a-z]+"))
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatal(err)
	}
	tok := gotreesitter.NewLexer(lang.LexStates, []byte("=")).Next(lang.LexModes[0].LexStateIndex())
	if tok.Symbol == 0 || lang.SymbolNames[tok.Symbol] != "=" || tok.StartByte != 0 || tok.EndByte != 1 {
		t.Fatalf("error mode lost a non-initial terminal: %+v", tok)
	}
	initial := gotreesitter.NewLexer(lang.LexStates, []byte("=")).Next(lang.LexModes[1].LexStateIndex())
	if initial.Symbol != 0 {
		t.Fatalf("recovery punctuation widened the initial mode: %+v", initial)
	}
}

func TestErrorRecoveryLexModeExcludesContextualPatterns(t *testing.T) {
	initial := lexModeSpec{validSymbols: map[int]bool{1: true}}
	terminals := []TerminalPattern{
		{SymbolID: 1, Rule: Pat("[a-z]+")},
		{SymbolID: 2, Rule: Str("=")},
		{SymbolID: 3, Rule: Pat("[^`]*")},
		{SymbolID: 4, Rule: Str("\""), Immediate: true},
		{SymbolID: 5, Rule: Str("")},
	}
	mode := errorRecoveryLexMode(initial, terminals)
	if want := map[int]bool{1: true, 2: true}; !reflect.DeepEqual(mode.validSymbols, want) {
		t.Fatalf("recovery symbols=%v, want %v", mode.validSymbols, want)
	}
	if len(initial.validSymbols) != 1 {
		t.Fatal("recovery construction mutated the initial mode")
	}
	if mode.skipWhitespace {
		t.Fatal("recovery introduced whitespace skipping into a grammar without extras")
	}
}

func TestMissingRecoveryLexModeRetainsColon(t *testing.T) {
	const identifier, colon, equal, tokenCount = 1, 2, 3, 4
	tables := &LRTables{
		StateCount: 3,
		ActionTable: map[int]map[int][]lrAction{
			0: {identifier: {{kind: lrShift, state: 1}}},
			1: {colon: {{kind: lrShift, state: 2}}, equal: {{kind: lrShift, state: 2}}},
		},
	}
	ng := &NormalizedGrammar{Symbols: []SymbolInfo{{Name: "end"}, {Name: "identifier"}, {Name: ":"}, {Name: "="}}}
	lookaheads := buildLexModeMissingRecoveryTokensFunc(context.Background(), tables, tokenCount, nil, nil, ng, nil, ":")
	if got, want := lookaheads(0), []int{colon}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recovery lookaheads=%v, want %v", got, want)
	}
}

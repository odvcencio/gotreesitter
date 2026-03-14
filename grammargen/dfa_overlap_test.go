package grammargen

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestBuildLexDFAPrefersLongerStringOverSingleCharPattern(t *testing.T) {
	lexStates, modeOffsets, err := buildLexDFA(
		[]TerminalPattern{
			{SymbolID: 1, Rule: Pat(`[^\n]`), Priority: 0},
			{SymbolID: 2, Rule: Str("*/"), Priority: 0},
		},
		nil,
		nil,
		[]lexModeSpec{{
			validSymbols: map[int]bool{1: true, 2: true},
		}},
	)
	if err != nil {
		t.Fatalf("buildLexDFA: %v", err)
	}
	if len(modeOffsets) != 1 {
		t.Fatalf("len(modeOffsets) = %d, want 1", len(modeOffsets))
	}

	lexer := gotreesitter.NewLexer(lexStates, []byte("*/"))
	tok := lexer.Next(uint16(modeOffsets[0]))
	if got, want := tok.Symbol, gotreesitter.Symbol(2); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

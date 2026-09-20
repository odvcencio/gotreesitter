//go:build !grammar_subset || grammar_subset_ruby

package grammarruntime

import "testing"

func TestRubyScannerMinusSymbolOrderMatchesGrammar(t *testing.T) {
	lang := RubyLanguage()
	if len(lang.ExternalSymbols) <= rbyTokBinaryMinus {
		t.Fatalf("ruby external symbol count = %d, want > %d", len(lang.ExternalSymbols), rbyTokBinaryMinus)
	}

	for _, tc := range []struct {
		name string
		idx  int
		sym  uint16
	}{
		{name: "unary_minus", idx: rbyTokUnaryMinus, sym: uint16(rbyDefaultSymTable[rbyTokUnaryMinus])},
		{name: "unary_minus_num", idx: rbyTokUnaryMinusNum, sym: uint16(rbyDefaultSymTable[rbyTokUnaryMinusNum])},
		{name: "binary_minus", idx: rbyTokBinaryMinus, sym: uint16(rbyDefaultSymTable[rbyTokBinaryMinus])},
	} {
		if got := uint16(lang.ExternalSymbols[tc.idx]); got != tc.sym {
			t.Fatalf("%s symbol = %d, want %d", tc.name, got, tc.sym)
		}
	}
}

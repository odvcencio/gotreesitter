//go:build !grammar_subset || grammar_subset_d

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestDExternalScannerBindsExternalSymbolsPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i. The
	// names are shuffled relative to dExternalScannerSpec.Externals; positional
	// binding maps by index, not by name, so token 0 (directive) must still
	// get external index 0's symbol even though that index carries a
	// different rule name here.
	names := []string{
		"error_sentinel",
		"not_is",
		"not_in",
		"_string",
		"float_literal",
		"int_literal",
		"directive",
		"_after_eof",
	}
	lang := dExternalBindingTestLanguage(names...)

	bound := DExternalScanner{}.ExternalScannerForLanguage(lang)
	scanner, ok := bound.(DExternalScanner)
	if !ok {
		t.Fatalf("DExternalScanner binding type = %T, want DExternalScanner", bound)
	}
	want := make([]int, dTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("d externalToToken = %v, want %v", got, want)
	}
	for i := 0; i < int(dTokenCount); i++ {
		if got, want := scanner.symbols[i], gotreesitter.Symbol(i+1); got != want {
			t.Fatalf("token %d result symbol = %d, want %d", i, got, want)
		}
	}
}

// TestDExternalScannerBindsShippedBlob pins the binding against the real
// d.bin blob: ExternalSymbols must stay [221 222 223 224 225 226 227 228] in
// externals order (directive, int_literal, float_literal, _string, not_in,
// not_is, _after_eof, error_sentinel).
func TestDExternalScannerBindsShippedBlob(t *testing.T) {
	lang := Language("d")
	if lang == nil {
		t.Fatal("d language did not load")
	}
	if got, want := lang.ExternalSymbols, []gotreesitter.Symbol{221, 222, 223, 224, 225, 226, 227, 228}; !slices.Equal(got, want) {
		t.Fatalf("d.ExternalSymbols = %v, want %v (update the scanner's default fallback table if this legitimately changed)", got, want)
	}

	bound := DExternalScanner{}.ExternalScannerForLanguage(lang)
	scanner, ok := bound.(DExternalScanner)
	if !ok {
		t.Fatalf("DExternalScanner binding type = %T, want DExternalScanner", bound)
	}
	for i, want := range []gotreesitter.Symbol{221, 222, 223, 224, 225, 226, 227, 228} {
		if got := scanner.symbols[i]; got != want {
			t.Fatalf("bound symbol[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestDExternalScannerZeroValueKeepsDefaultSymbols(t *testing.T) {
	scanner := DExternalScanner{}
	symbols := scanner.symbolTable()
	for i := 0; i < int(dTokenCount); i++ {
		if got, want := symbols[i], dDefaultSymTable[i]; got != want {
			t.Fatalf("zero-value symbol[%d] = %d, want %d", i, got, want)
		}
	}
}

func dExternalBindingTestLanguage(names ...string) *gotreesitter.Language {
	symbolNames := make([]string, len(names)+1)
	symbols := make([]gotreesitter.Symbol, len(names))
	for i, name := range names {
		symbolNames[i+1] = name
		symbols[i] = gotreesitter.Symbol(i + 1)
	}
	return &gotreesitter.Language{
		SymbolNames:     symbolNames,
		ExternalSymbols: symbols,
	}
}

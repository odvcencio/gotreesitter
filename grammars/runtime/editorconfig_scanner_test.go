//go:build !grammar_subset || grammar_subset_editorconfig

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestEditorconfigExternalScannerBindsExternalSymbolsPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i. The names
	// are shuffled relative to editorconfigExternalScannerSpec.Externals
	// (_end_of_file, _integer_range_start); positional binding maps by index,
	// not by name, so token 0 (end-of-file) must still get external index 0's
	// symbol even though that index carries a different rule name here.
	lang := editorconfigExternalBindingTestLanguage("_integer_range_start", "_end_of_file")

	bound := EditorconfigExternalScanner{}.ExternalScannerForLanguage(lang)
	scanner, ok := bound.(EditorconfigExternalScanner)
	if !ok {
		t.Fatalf("EditorconfigExternalScanner binding type = %T, want EditorconfigExternalScanner", bound)
	}
	if got, want := scanner.externalToToken, []int{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("editorconfig externalToToken = %v, want %v", got, want)
	}
	if got, want := scanner.symbols[editorconfigTokEndOfFile], gotreesitter.Symbol(1); got != want {
		t.Fatalf("token 0 (end-of-file) result symbol = %d, want %d", got, want)
	}
	if got, want := scanner.symbols[editorconfigTokIntegerRangeStart], gotreesitter.Symbol(2); got != want {
		t.Fatalf("token 1 (integer-range-start) result symbol = %d, want %d", got, want)
	}
}

// TestEditorconfigExternalScannerBindsShippedBlob pins the fix against the
// real editorconfig.bin blob shipped on 2026-09-20: ExternalSymbols is
// [23 24], not the [31 32] the scanner used to hardcode. Symbol 31 in that
// blob names glob and 32 names brace_expansion, so the pre-fix scanner
// mislabeled both the end-of-file and integer-range-start tokens as
// unrelated grammar rules.
func TestEditorconfigExternalScannerBindsShippedBlob(t *testing.T) {
	lang := Language("editorconfig")
	if lang == nil {
		t.Fatal("editorconfig language did not load")
	}
	if got, want := lang.ExternalSymbols, []gotreesitter.Symbol{23, 24}; !slices.Equal(got, want) {
		t.Fatalf("editorconfig.ExternalSymbols = %v, want %v (update the scanner's default fallback table if this legitimately changed)", got, want)
	}

	bound := EditorconfigExternalScanner{}.ExternalScannerForLanguage(lang)
	scanner, ok := bound.(EditorconfigExternalScanner)
	if !ok {
		t.Fatalf("EditorconfigExternalScanner binding type = %T, want EditorconfigExternalScanner", bound)
	}
	if got, want := scanner.symbols[editorconfigTokEndOfFile], gotreesitter.Symbol(23); got != want {
		t.Fatalf("bound end-of-file symbol = %d, want %d", got, want)
	}
	if got, want := scanner.symbols[editorconfigTokIntegerRangeStart], gotreesitter.Symbol(24); got != want {
		t.Fatalf("bound integer-range-start symbol = %d, want %d", got, want)
	}
}

func TestEditorconfigExternalScannerZeroValueKeepsDefaultSymbols(t *testing.T) {
	scanner := EditorconfigExternalScanner{}
	symbols := scanner.symbolTable()
	if got, want := symbols[editorconfigTokEndOfFile], editorconfigDefaultSymTable[editorconfigTokEndOfFile]; got != want {
		t.Fatalf("zero-value end-of-file symbol = %d, want %d", got, want)
	}
	if got, want := symbols[editorconfigTokIntegerRangeStart], editorconfigDefaultSymTable[editorconfigTokIntegerRangeStart]; got != want {
		t.Fatalf("zero-value integer-range symbol = %d, want %d", got, want)
	}
}

func editorconfigExternalBindingTestLanguage(names ...string) *gotreesitter.Language {
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

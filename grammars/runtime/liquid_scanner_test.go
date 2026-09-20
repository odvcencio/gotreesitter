//go:build !grammar_subset || grammar_subset_liquid

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestLiquidExternalScannerBindsExternalSymbolsPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i. The names
	// here are a rotated permutation of liquidExternalScannerSpec.Externals;
	// positional binding maps by index, not by name.
	lang := liquidExternalBindingTestLanguage(
		"error_sentinel",
		"_inline_comment_content",
		"_paired_comment_content",
		"_paired_comment_content_liq",
		"raw_content",
		"front_matter",
	)

	bound := LiquidExternalScanner{}.ExternalScannerForLanguage(lang)
	scanner, ok := bound.(LiquidExternalScanner)
	if !ok {
		t.Fatalf("LiquidExternalScanner binding type = %T, want LiquidExternalScanner", bound)
	}
	if got, want := scanner.externalToToken, []int{0, 1, 2, 3, 4, 5}; !slices.Equal(got, want) {
		t.Fatalf("liquid externalToToken = %v, want %v", got, want)
	}
	// External index 1 carries the name _inline_comment_content here, and
	// token 0 (liquidTokInlineCommentContent) must still bind to external
	// index 0's symbol, not the index whose name happens to match.
	if got, want := scanner.symbols[liquidTokInlineCommentContent], gotreesitter.Symbol(1); got != want {
		t.Fatalf("token 0 (inline comment content) result symbol = %d, want %d", got, want)
	}
	if got, want := scanner.symbols[liquidTokErrorSentinel], gotreesitter.Symbol(6); got != want {
		t.Fatalf("token 5 (error sentinel) result symbol = %d, want %d", got, want)
	}
}

// TestLiquidExternalScannerBindsShippedBlob pins the fix against the real
// liquid.bin blob shipped on 2026-09-20: ExternalSymbols is
// [98 99 100 101 102 103], not the [96 97 98 99 100 101] the scanner used to
// hardcode. Two of those six constants (96, 97) fell outside ExternalSymbols
// entirely; the other four (98, 99, 100, 101) were each bound one external
// position too low while still passing a naive membership check, since
// every one of those four values is still a member of ExternalSymbols.
func TestLiquidExternalScannerBindsShippedBlob(t *testing.T) {
	lang := Language("liquid")
	if lang == nil {
		t.Fatal("liquid language did not load")
	}
	want := []gotreesitter.Symbol{98, 99, 100, 101, 102, 103}
	if got := lang.ExternalSymbols; !slices.Equal(got, want) {
		t.Fatalf("liquid.ExternalSymbols = %v, want %v (update the scanner's default fallback table if this legitimately changed)", got, want)
	}

	bound := LiquidExternalScanner{}.ExternalScannerForLanguage(lang)
	scanner, ok := bound.(LiquidExternalScanner)
	if !ok {
		t.Fatalf("LiquidExternalScanner binding type = %T, want LiquidExternalScanner", bound)
	}
	wantSymbols := [liquidTokenCount]gotreesitter.Symbol{98, 99, 100, 101, 102, 103}
	if scanner.symbols != wantSymbols {
		t.Fatalf("bound liquid symbols = %v, want %v", scanner.symbols, wantSymbols)
	}
}

func TestLiquidExternalScannerZeroValueKeepsDefaultSymbols(t *testing.T) {
	scanner := LiquidExternalScanner{}
	symbols := scanner.symbolTable()
	if *symbols != liquidDefaultSymTable {
		t.Fatalf("zero-value symbols = %v, want %v", *symbols, liquidDefaultSymTable)
	}
}

func liquidExternalBindingTestLanguage(names ...string) *gotreesitter.Language {
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

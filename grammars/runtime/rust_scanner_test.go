//go:build !grammar_subset || grammar_subset_rust

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestRustExternalScannerBindsExternalSymbolsPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i. The duplicate
	// "string_content" externals (indices 0 and 3) bind to their positional tokens
	// naturally, with no name-based disambiguation.
	lang := rustExternalBindingTestLanguage(
		"string_content",
		"_raw_string_literal_start",
		"string_content",
		"_raw_string_literal_end",
		"float_literal",
	)

	scanner, ok := RustExternalScanner{}.ExternalScannerForLanguage(lang).(RustExternalScanner)
	if !ok {
		t.Fatalf("RustExternalScanner binding type = %T, want RustExternalScanner", RustExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	if got, want := scanner.externalToToken, []int{0, 1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("rust externalToToken = %v, want %v", got, want)
	}
	if got, want := scanner.symbols[rustTokStringContent], gotreesitter.Symbol(1); got != want {
		t.Fatalf("token 0 (string-content) result symbol = %d, want %d", got, want)
	}
	if got, want := scanner.symbols[rustTokRawStringContent], gotreesitter.Symbol(4); got != want {
		t.Fatalf("token 3 (raw string-content) result symbol = %d, want %d", got, want)
	}

	validExternal := []bool{false, true, false, false, true}
	var semanticValid [rustTokenCount]bool
	validSemantic := scanner.remapValidSymbols(validExternal, &semanticValid)
	if !validSemantic[rustTokStringClose] {
		t.Fatalf("external index 1 did not map to token 1 (string-close): %v", validSemantic)
	}
	if !validSemantic[rustTokRawStringEnd] {
		t.Fatalf("external index 4 did not map to token 4 (raw-string-end): %v", validSemantic)
	}
	if validSemantic[rustTokRawStringStart] {
		t.Fatalf("token 2 (raw-string-start) unexpectedly valid: %v", validSemantic)
	}
}

func TestRustExternalScannerZeroValueKeepsBuiltinSymbols(t *testing.T) {
	symbols := RustExternalScanner{}.symbolTable()
	if got, want := symbols[rustTokStringContent], rustSymStringContent; got != want {
		t.Fatalf("zero-value string-content symbol = %d, want %d", got, want)
	}
	if got, want := symbols[rustTokStringClose], rustSymStringClose; got != want {
		t.Fatalf("zero-value string-close symbol = %d, want %d", got, want)
	}
}

func rustExternalBindingTestLanguage(names ...string) *gotreesitter.Language {
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

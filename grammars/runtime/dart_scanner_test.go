//go:build !grammar_subset || grammar_subset_dart

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestDartExternalScannerBindsExternalSymbolsPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i. The Language
	// names here are a shuffled subset; positional binding maps by position.
	lang := dartExternalBindingTestLanguage(
		"_extension_only",
		"_documentation_block_comment",
		"_template_chars_raw_slash",
		"_template_chars_double",
		"_block_comment",
	)

	scanner, ok := DartExternalScanner{}.ExternalScannerForLanguage(lang).(DartExternalScanner)
	if !ok {
		t.Fatalf("DartExternalScanner binding type = %T, want DartExternalScanner", DartExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	if got, want := scanner.externalToToken, []int{0, 1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("dart externalToToken = %v, want %v", got, want)
	}
	// External index 3 -> scanner token 3 (single-single template chars), symbol 4.
	if got, want := scanner.symbols[dartTokTemplateCharsSingleSingle], gotreesitter.Symbol(4); got != want {
		t.Fatalf("token 3 result symbol = %d, want %d", got, want)
	}

	validExternal := []bool{false, false, false, true, false}
	var semanticValid [dartTokenCount]bool
	validSemantic := scanner.remapValidSymbols(validExternal, &semanticValid)
	if !validSemantic[dartTokTemplateCharsSingleSingle] {
		t.Fatalf("external index 3 did not become valid semantic token 3: %v", validSemantic)
	}
	if got, want := dartTemplateResultSymbol(validSemantic, scanner.symbolTable()), gotreesitter.Symbol(4); got != want {
		t.Fatalf("template scanner result symbol = %d, want %d", got, want)
	}
}

func TestDartExternalScannerZeroValueKeepsLegacySymbols(t *testing.T) {
	scanner := DartExternalScanner{}
	symbols := scanner.symbolTable()
	if got, want := symbols[dartTokTemplateCharsDouble], dartSymTemplateCharsDouble; got != want {
		t.Fatalf("zero-value template-double symbol = %d, want %d", got, want)
	}
	if got, want := symbols[dartTokDocBlockComment], dartSymDocBlockComment; got != want {
		t.Fatalf("zero-value doc-comment symbol = %d, want %d", got, want)
	}
}

func dartExternalBindingTestLanguage(names ...string) *gotreesitter.Language {
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

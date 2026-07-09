//go:build !grammar_subset || grammar_subset_hcl

package grammars

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestHclExternalScannerBindsExternalSymbolsPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i. The Language
	// names here are a shuffled subset; positional binding maps by position, not
	// by matching names or hardcoded absolute Symbol IDs.
	lang := hclExternalBindingTestLanguage(
		"heredoc_identifier",
		"quoted_template_end",
		"_template_literal_chunk",
		"quoted_template_start",
		"template_interpolation_start",
	)

	scanner, ok := HclExternalScanner{}.ExternalScannerForLanguage(lang).(HclExternalScanner)
	if !ok {
		t.Fatalf("HclExternalScanner binding type = %T, want HclExternalScanner", HclExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	if got, want := scanner.externalToToken, []int{0, 1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("hcl externalToToken = %v, want %v", got, want)
	}
	// External index 3 -> scanner token 3 (interpolation start), symbol 4. The
	// Language name at index 3 is "quoted_template_start" (shuffled/mismatched
	// on purpose): positional binding must ignore that and bind by position.
	if got, want := scanner.symbols[hclTokTemplateInterpolationStart], gotreesitter.Symbol(4); got != want {
		t.Fatalf("token 3 (interpolation start) result symbol = %d, want %d", got, want)
	}

	validExternal := []bool{false, false, false, true, false}
	var semanticValid [hclTokenCount]bool
	validSemantic := scanner.remapValidSymbols(validExternal, &semanticValid)
	if !validSemantic[hclTokTemplateInterpolationStart] {
		t.Fatalf("external index 3 did not become valid semantic token 3 (interpolation start): %v", validSemantic)
	}
}

func TestHclExternalScannerZeroValueKeepsLegacySymbols(t *testing.T) {
	scanner := HclExternalScanner{}
	symbols := scanner.symbolTable()
	if got, want := symbols[hclTokQuotedTemplateStart], hclSymQuotedTemplateStart; got != want {
		t.Fatalf("zero-value quoted-template-start symbol = %d, want %d", got, want)
	}
	if got, want := symbols[hclTokHeredocIdentifier], hclSymHeredocIdentifier; got != want {
		t.Fatalf("zero-value heredoc-identifier symbol = %d, want %d", got, want)
	}
}

// TestHclLanguageExternalBindingTableIsIdentityOnCurrentBlob pins the exact
// externalToToken table and bound symbols for the currently-shipped ts2go HCL
// blob. This is the no-regression witness for the positional-binding
// conversion: on today's blob, external index i is scanner token i for every
// one of the 8 externals, and the bound symbols equal hclDefaultSymTable (the
// previously-hardcoded absolute IDs) exactly. A future grammargen-generated hcl
// blob that renumbers absolute Symbol IDs while preserving external order would
// still pass this externalToToken check (it is blob-ID-independent) and would
// only fail the symbols comparison if hclDefaultSymTable itself needed updating
// for a *new* canonical blob — not if the scanner were bound incorrectly.
func TestHclLanguageExternalBindingTableIsIdentityOnCurrentBlob(t *testing.T) {
	lang := HclLanguage()
	scanner, ok := HclExternalScanner{}.ExternalScannerForLanguage(lang).(HclExternalScanner)
	if !ok {
		t.Fatalf("HclExternalScanner binding type = %T, want HclExternalScanner", HclExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	if got, want := scanner.externalToToken, []int{0, 1, 2, 3, 4, 5, 6, 7}; !slices.Equal(got, want) {
		t.Fatalf("hcl externalToToken = %v, want %v (identity on the current blob)", got, want)
	}
	if got, want := scanner.symbols, hclDefaultSymTable; got != want {
		t.Fatalf("hcl post-bind symbols = %v, want default table %v", got, want)
	}
}

func hclExternalBindingTestLanguage(names ...string) *gotreesitter.Language {
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

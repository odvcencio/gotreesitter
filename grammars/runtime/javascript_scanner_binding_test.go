//go:build !grammar_subset || grammar_subset_javascript

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestJavaScriptExternalScannerBindsPositionally is the regression witness for
// the javascript blob-regen scanner-adaptation corruption: a Language whose
// automaton grew (or otherwise shifted overall symbol numbering) still emits
// the same 8 externals in the same declared order, but under different
// absolute Symbol IDs than the ones jsSym* were pinned against. Before
// JavaScriptExternalScanner implemented ExternalScannerForLanguage, the
// scanner was attached raw (attachExternalScannerForLanguage's non-bound
// branch) and every SetResultSymbol call emitted the stale, hardcoded jsSym*
// IDs regardless of the attached Language's real numbering — silently
// mistyping every external scan result on any regenerated table whose
// automaton size differs from the one those constants were pinned against.
// This test simulates that shift with a synthetic Language (no blob
// dependency) and asserts the bound scanner resolves each token to the
// synthetic Language's own symbol, not the pinned default.
//
// Tagged to match javascript_scanner.go exactly (not the broader
// external_scanner_binding_test.go umbrella): a grammar_subset build that
// selects only kotlin+swift, for example, would not compile
// javascript_scanner.go, so this test must not be visible there either.
func TestJavaScriptExternalScannerBindsPositionally(t *testing.T) {
	jsLang := externalBindingTestLanguage(
		"_automatic_semicolon",
		"_template_chars",
		"_ternary_qmark",
		"html_comment",
		"||",
		"escape_sequence",
		"regex_pattern",
		"jsx_text",
	)
	jsScanner, ok := JavaScriptExternalScanner{}.ExternalScannerForLanguage(jsLang).(JavaScriptExternalScanner)
	if !ok {
		t.Fatalf("JavaScriptExternalScanner binding type = %T, want JavaScriptExternalScanner", JavaScriptExternalScanner{}.ExternalScannerForLanguage(jsLang))
	}
	if got, want := jsScanner.externalToToken, []int{0, 1, 2, 3, 4, 5, 6, 7}; !slices.Equal(got, want) {
		t.Fatalf("javascript externalToToken = %v, want %v", got, want)
	}
	// externalBindingTestLanguage assigns ExternalSymbols[i] = Symbol(i+1), which
	// deliberately does not match any jsSym* default (129, 130, 131, 132, 78,
	// 107, 112, 133) -- simulating a regenerated table with shifted numbering.
	wantBound := map[int]gotreesitter.Symbol{
		jsTokAutoSemicolon:  1,
		jsTokTemplateChars:  2,
		jsTokTernaryQmark:   3,
		jsTokHtmlComment:    4,
		jsTokLogicalOr:      5,
		jsTokEscapeSequence: 6,
		jsTokRegexPattern:   7,
		jsTokJsxText:        8,
	}
	for tokenIdx, want := range wantBound {
		if got := jsScanner.symbols[tokenIdx]; got != want {
			t.Fatalf("javascript token %d bound symbol = %d, want %d (synthetic Language's own numbering, not the pinned default)", tokenIdx, got, want)
		}
	}
	// The pinned defaults must NOT leak through once a Language is bound --
	// that leak is exactly the pre-fix corruption (attaching the raw,
	// unbound scanner to a table whose automaton had grown).
	if jsScanner.symbols == jsDefaultSymTable {
		t.Fatal("javascript bound symbols still equal jsDefaultSymTable; binding did not override the pinned defaults")
	}
}

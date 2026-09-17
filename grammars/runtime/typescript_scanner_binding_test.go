//go:build !grammar_subset || grammar_subset_typescript

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestTypeScriptExternalScannerBindsPositionally is the regression witness for
// the typescript blob-regen scanner-adaptation corruption: a Language whose
// automaton grew (or otherwise shifted overall symbol numbering) still emits
// the same 10 externals in the same declared order, but under different
// absolute Symbol IDs than the ones tsSym* were pinned against. Before
// TypeScriptExternalScanner implemented ExternalScannerForLanguage, the
// scanner was attached raw (attachExternalScannerForLanguage's non-bound
// branch) and every SetResultSymbol call emitted the stale, hardcoded tsSym*
// IDs regardless of the attached Language's real numbering -- silently
// mistyping every external scan result on any regenerated table whose
// automaton size differs from the one those constants were pinned against.
// This test simulates that shift with a synthetic Language (no blob
// dependency) and asserts the bound scanner resolves each token to the
// synthetic Language's own symbol, not the pinned default.
//
// Tagged to match typescript_scanner.go exactly (not the broader
// external_scanner_binding_test.go umbrella): a grammar_subset build that
// selects only kotlin+swift, for example, would not compile
// typescript_scanner.go, so this test must not be visible there either.
func TestTypeScriptExternalScannerBindsPositionally(t *testing.T) {
	tsLang := externalBindingTestLanguage(
		"_automatic_semicolon",
		"_template_chars",
		"_ternary_qmark",
		"html_comment",
		"||",
		"escape_sequence",
		"regex_pattern",
		"jsx_text",
		"_function_signature_automatic_semicolon",
		"__error_recovery",
	)
	tsScanner, ok := TypeScriptExternalScanner{}.ExternalScannerForLanguage(tsLang).(TypeScriptExternalScanner)
	if !ok {
		t.Fatalf("TypeScriptExternalScanner binding type = %T, want TypeScriptExternalScanner", TypeScriptExternalScanner{}.ExternalScannerForLanguage(tsLang))
	}
	if got, want := tsScanner.externalToToken, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}; !slices.Equal(got, want) {
		t.Fatalf("typescript externalToToken = %v, want %v", got, want)
	}
	// externalBindingTestLanguage assigns ExternalSymbols[i] = Symbol(i+1), which
	// deliberately does not match any tsSym* default (160, 161, 162, 163, 72,
	// 103, 108, 164, 165, 166) -- simulating a regenerated table with shifted
	// numbering.
	wantBound := map[int]gotreesitter.Symbol{
		tsTokAutoSemicolon:   1,
		tsTokTemplateChars:   2,
		tsTokTernaryQmark:    3,
		tsTokHtmlComment:     4,
		tsTokLogicalOr:       5,
		tsTokEscapeSequence:  6,
		tsTokRegexPattern:    7,
		tsTokJsxText:         8,
		tsTokFuncSigAutoSemi: 9,
		tsTokErrorRecovery:   10,
	}
	for tokenIdx, want := range wantBound {
		if got := tsScanner.symbols[tokenIdx]; got != want {
			t.Fatalf("typescript token %d bound symbol = %d, want %d (synthetic Language's own numbering, not the pinned default)", tokenIdx, got, want)
		}
	}
	// The pinned defaults must NOT leak through once a Language is bound --
	// that leak is exactly the pre-fix corruption (attaching the raw,
	// unbound scanner to a table whose automaton had grown).
	if tsScanner.symbols == tsDefaultSymTable {
		t.Fatal("typescript bound symbols still equal tsDefaultSymTable; binding did not override the pinned defaults")
	}
}

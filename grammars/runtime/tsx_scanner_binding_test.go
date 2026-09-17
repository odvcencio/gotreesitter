//go:build !grammar_subset || grammar_subset_tsx

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestTsxExternalScannerBindsPositionally is the regression witness for the
// tsx blob-regen scanner-adaptation corruption: a Language whose automaton
// grew (or otherwise shifted overall symbol numbering) still emits the same
// 10 externals in the same declared order, but under different absolute
// Symbol IDs than the ones tsxSym* were pinned against. Before
// TsxExternalScanner implemented ExternalScannerForLanguage, the scanner was
// attached raw (attachExternalScannerForLanguage's non-bound branch) and
// every SetResultSymbol call emitted the stale, hardcoded tsxSym* IDs
// regardless of the attached Language's real numbering -- silently
// mistyping every external scan result on any regenerated table whose
// automaton size differs from the one those constants were pinned against.
// This test simulates that shift with a synthetic Language (no blob
// dependency) and asserts the bound scanner resolves each token to the
// synthetic Language's own symbol, not the pinned default.
//
// Tagged to match tsx_scanner.go exactly (not the broader
// external_scanner_binding_test.go umbrella): a grammar_subset build that
// selects only kotlin+swift, for example, would not compile tsx_scanner.go,
// so this test must not be visible there either.
func TestTsxExternalScannerBindsPositionally(t *testing.T) {
	tsxLang := externalBindingTestLanguage(
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
	tsxScanner, ok := TsxExternalScanner{}.ExternalScannerForLanguage(tsxLang).(TsxExternalScanner)
	if !ok {
		t.Fatalf("TsxExternalScanner binding type = %T, want TsxExternalScanner", TsxExternalScanner{}.ExternalScannerForLanguage(tsxLang))
	}
	if got, want := tsxScanner.externalToToken, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}; !slices.Equal(got, want) {
		t.Fatalf("tsx externalToToken = %v, want %v", got, want)
	}
	// externalBindingTestLanguage assigns ExternalSymbols[i] = Symbol(i+1), which
	// deliberately does not match any tsxSym* default (166, 167, 168, 169, 81,
	// 109, 114, 170, 171, 172) -- simulating a regenerated table with shifted
	// numbering.
	wantBound := map[int]gotreesitter.Symbol{
		tsxTokAutoSemicolon:   1,
		tsxTokTemplateChars:   2,
		tsxTokTernaryQmark:    3,
		tsxTokHtmlComment:     4,
		tsxTokLogicalOr:       5,
		tsxTokEscapeSequence:  6,
		tsxTokRegexPattern:    7,
		tsxTokJsxText:         8,
		tsxTokFuncSigAutoSemi: 9,
		tsxTokErrorRecovery:   10,
	}
	for tokenIdx, want := range wantBound {
		if got := tsxScanner.symbols[tokenIdx]; got != want {
			t.Fatalf("tsx token %d bound symbol = %d, want %d (synthetic Language's own numbering, not the pinned default)", tokenIdx, got, want)
		}
	}
	// The pinned defaults must NOT leak through once a Language is bound --
	// that leak is exactly the pre-fix corruption (attaching the raw,
	// unbound scanner to a table whose automaton had grown).
	if tsxScanner.symbols == tsxDefaultSymTable {
		t.Fatal("tsx bound symbols still equal tsxDefaultSymTable; binding did not override the pinned defaults")
	}
}

//go:build !grammar_subset || (grammar_subset_kotlin && grammar_subset_swift)

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestKotlinSwiftExternalScannerSpecs(t *testing.T) {
	kotlinSpec, ok := LookupExternalScannerSpec("kotlin")
	if !ok {
		t.Fatal("missing kotlin external scanner spec")
	}
	if got, want := kotlinSpec.UpstreamRepo, "https://github.com/fwcd/tree-sitter-kotlin"; got != want {
		t.Fatalf("kotlin repo = %q, want %q", got, want)
	}
	if got, want := kotlinSpec.UpstreamCommit, "cbed96ab13dbc082eeeb2e8333c342a62829c29d"; got != want {
		t.Fatalf("kotlin commit = %q, want %q", got, want)
	}
	if got, want := kotlinSpec.Externals, []string{
		"_automatic_semicolon",
		"_import_list_delimiter",
		"safe_nav",
		"multiline_comment",
		"_string_start",
		"_string_end",
		"string_content",
		"_primary_constructor_keyword",
		"_import_dot",
	}; !slices.Equal(got, want) {
		t.Fatalf("kotlin externals = %v, want %v", got, want)
	}

	swiftSpec, ok := LookupExternalScannerSpec("SWIFT")
	if !ok {
		t.Fatal("missing swift external scanner spec")
	}
	if got, want := swiftSpec.UpstreamRepo, "https://github.com/alex-pinkus/tree-sitter-swift"; got != want {
		t.Fatalf("swift repo = %q, want %q", got, want)
	}
	if got, want := swiftSpec.UpstreamCommit, "41d6e5fe811ec94229ee71771174a8cce558dfee"; got != want {
		t.Fatalf("swift commit = %q, want %q", got, want)
	}
	if got, want := len(swiftSpec.Externals), swtTokenCount; got != want {
		t.Fatalf("swift external count = %d, want %d", got, want)
	}

	swiftSpec.Externals[0] = "mutated"
	again, ok := LookupExternalScannerSpec("swift")
	if !ok {
		t.Fatal("missing swift external scanner spec after mutation")
	}
	if got, want := again.Externals[0], "multiline_comment"; got != want {
		t.Fatalf("swift spec registry was mutated through lookup: got %q, want %q", got, want)
	}
}

func TestLanguageBoundExternalScannersBindPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i for every
	// provenance. These synthetic languages present a shuffled subset of the real
	// externals; positional binding maps them by position, not by symbol name.
	kotlinLang := externalBindingTestLanguage(
		"_extension_only",
		"_import_dot",
		"safe_nav",
		"_automatic_semicolon",
	)
	kotlinScanner, ok := KotlinExternalScanner{}.ExternalScannerForLanguage(kotlinLang).(KotlinExternalScanner)
	if !ok {
		t.Fatalf("KotlinExternalScanner binding type = %T, want KotlinExternalScanner", KotlinExternalScanner{}.ExternalScannerForLanguage(kotlinLang))
	}
	if got, want := kotlinScanner.externalToToken, []int{0, 1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("kotlin externalToToken = %v, want %v", got, want)
	}
	// safe_nav sits at external index 2 == scanner token index 2, so positional
	// binding lands it on kotlinTokSafeNav with its real symbol (3 here). By-name
	// binding dropped this slot for the real grammar because the Language display
	// name is "\?." rather than the spec rule name "safe_nav".
	if got, want := kotlinScanner.externalToToken[2], kotlinTokSafeNav; got != want {
		t.Fatalf("kotlin safe-nav external mapped to token %d, want %d", got, want)
	}
	if got, want := kotlinScanner.symbols[kotlinTokSafeNav], gotreesitter.Symbol(3); got != want {
		t.Fatalf("kotlin safe-nav result symbol = %d, want %d", got, want)
	}

	swiftLang := externalBindingTestLanguage(
		"_fake_try_bang",
		"else",
		"_directive_else",
	)
	swiftScanner, ok := SwiftExternalScanner{}.ExternalScannerForLanguage(swiftLang).(SwiftExternalScanner)
	if !ok {
		t.Fatalf("SwiftExternalScanner binding type = %T, want SwiftExternalScanner", SwiftExternalScanner{}.ExternalScannerForLanguage(swiftLang))
	}
	if got, want := swiftScanner.externalToToken, []int{0, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("swift externalToToken = %v, want %v", got, want)
	}
	if got, want := swiftScanner.symbols[2], gotreesitter.Symbol(3); got != want {
		t.Fatalf("swift token-2 result symbol = %d, want %d", got, want)
	}
}

func TestExternalScannerBindingBindsPositionally(t *testing.T) {
	lang := externalBindingTestLanguage(
		"",
		"named_two",
		"",
	)
	names := []string{
		"missing_zero",
		"missing_one",
		"named_two",
	}

	symbols := make([]gotreesitter.Symbol, len(names))
	externalToToken := bindExternalScannerSymbolNames(lang, names, func(tokenIdx int, sym gotreesitter.Symbol) {
		symbols[tokenIdx] = sym
	})

	// External index i binds to token i; empty and mismatched Language names do
	// not change the mapping.
	if got, want := externalToToken, []int{0, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("externalToToken = %v, want %v", got, want)
	}
	if got, want := symbols[0], gotreesitter.Symbol(1); got != want {
		t.Fatalf("token 0 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[1], gotreesitter.Symbol(2); got != want {
		t.Fatalf("token 1 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[2], gotreesitter.Symbol(3); got != want {
		t.Fatalf("token 2 symbol = %d, want %d", got, want)
	}
}

func TestExternalScannerBindingPositionalRecordsNameDrift(t *testing.T) {
	// Positional binding ignores name disagreement (the old algorithm left these
	// unbound as [-1, 2, -1]); every disagreeing index is recorded as drift so a
	// genuine spec/grammar ordering skew is observable instead of silent.
	lang := externalBindingTestLanguage(
		"alias_zero",
		"named_two",
		"alias_one",
	)
	names := []string{
		"missing_zero",
		"missing_one",
		"named_two",
	}

	symbols := make([]gotreesitter.Symbol, len(names))
	externalBindingDriftBeginCapture()
	t.Cleanup(func() { externalBindingDriftEndCapture() })
	externalToToken := bindExternalScannerSymbolNames(lang, names, func(tokenIdx int, sym gotreesitter.Symbol) {
		symbols[tokenIdx] = sym
	})
	drift := externalBindingDriftEndCapture()

	if got, want := externalToToken, []int{0, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("externalToToken = %v, want %v", got, want)
	}
	if got, want := symbols[0], gotreesitter.Symbol(1); got != want {
		t.Fatalf("token 0 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[1], gotreesitter.Symbol(2); got != want {
		t.Fatalf("token 1 symbol = %d, want %d", got, want)
	}
	if got, want := symbols[2], gotreesitter.Symbol(3); got != want {
		t.Fatalf("token 2 symbol = %d, want %d", got, want)
	}
	if len(drift) != 3 {
		t.Fatalf("drift entries = %d (%v), want 3 (every index disagrees)", len(drift), drift)
	}
}

// TestExternalBindingDriftFiresOnMismatchSilentOnAligned and
// TestExternalBindingLengthMismatchBindsMin live in this file, rather than in
// external_scanner_positional_binding_test.go (gated !grammar_subset only),
// because neither depends on any real language: both exercise
// bindExternalScannerSymbolNames purely through the synthetic
// externalBindingTestLanguage fixture below. This file's broader tag keeps them
// visible to grammar_subset builds that select kotlin+swift.

// TestExternalBindingDriftFiresOnMismatchSilentOnAligned verifies the
// verification-only drift signal: aligned specs are silent, name disagreements are
// recorded (with their index), and empty Language names never drift.
func TestExternalBindingDriftFiresOnMismatchSilentOnAligned(t *testing.T) {
	totalBefore := externalBindingDriftTotal()

	// Aligned: Language symbol names match spec names at every index -> no drift.
	aligned := externalBindingTestLanguage("alpha", "beta", "gamma")
	externalBindingDriftBeginCapture()
	t.Cleanup(func() { externalBindingDriftEndCapture() })
	bindExternalScannerSymbolNames(aligned, []string{"alpha", "beta", "gamma"}, func(int, gotreesitter.Symbol) {})
	if drift := externalBindingDriftEndCapture(); len(drift) != 0 {
		t.Fatalf("aligned spec produced drift: %v", drift)
	}

	// Skewed at indices 0 and 2; index 1 matches.
	skewed := externalBindingTestLanguage("alpha", "beta", "gamma")
	externalBindingDriftBeginCapture()
	t.Cleanup(func() { externalBindingDriftEndCapture() })
	bindExternalScannerSymbolNames(skewed, []string{"ALPHA", "beta", "GAMMA"}, func(int, gotreesitter.Symbol) {})
	drift := externalBindingDriftEndCapture()
	if len(drift) != 2 {
		t.Fatalf("skewed spec drift = %d (%v), want 2", len(drift), drift)
	}
	if drift[0].Index != 0 || drift[1].Index != 2 {
		t.Fatalf("drift indices = %v, want [0 2]", drift)
	}
	if drift[0].Got != "alpha" || drift[0].Want != "ALPHA" {
		t.Fatalf("drift[0] = %+v, want got=alpha want=ALPHA", drift[0])
	}

	// Empty Language names cannot be verified and never drift.
	unnamed := externalBindingTestLanguage("", "", "")
	externalBindingDriftBeginCapture()
	t.Cleanup(func() { externalBindingDriftEndCapture() })
	bindExternalScannerSymbolNames(unnamed, []string{"a", "b", "c"}, func(int, gotreesitter.Symbol) {})
	if drift := externalBindingDriftEndCapture(); len(drift) != 0 {
		t.Fatalf("empty Language names produced drift: %v", drift)
	}

	// The process-wide counter tracks every disagreement; only the skewed bind
	// above contributed (two indices). This test is non-parallel, so no other
	// binder call overlaps this delta.
	if got := externalBindingDriftTotal() - totalBefore; got != 2 {
		t.Fatalf("drift counter delta = %d, want 2", got)
	}
}

// TestExternalBindingLengthMismatchBindsMin documents the length-mismatch contract:
// bind min(externals, specTokens); surplus scanner tokens keep defaults (never
// bound), surplus externals stay -1.
func TestExternalBindingLengthMismatchBindsMin(t *testing.T) {
	// Spec longer than externals: only the first len(externals) tokens are bound.
	lang := externalBindingTestLanguage("a", "b")
	var boundTokens []int
	ett := bindExternalScannerSymbolNames(lang, []string{"a", "b", "c", "d"}, func(tok int, _ gotreesitter.Symbol) {
		boundTokens = append(boundTokens, tok)
	})
	if got, want := ett, []int{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("spec-longer externalToToken = %v, want %v", got, want)
	}
	if got, want := boundTokens, []int{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("spec-longer bound tokens = %v, want %v", got, want)
	}

	// Externals longer than spec: surplus externals stay unbound (-1).
	lang2 := externalBindingTestLanguage("a", "b", "c", "d")
	ett2 := bindExternalScannerSymbolNames(lang2, []string{"a", "b"}, func(int, gotreesitter.Symbol) {})
	if got, want := ett2, []int{0, 1, -1, -1}; !slices.Equal(got, want) {
		t.Fatalf("externals-longer externalToToken = %v, want %v", got, want)
	}
}

func externalBindingTestLanguage(names ...string) *gotreesitter.Language {
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

package gotreesitter

import "testing"

// TestParserWantsForestFieldDrivesDispatch verifies that Language.WantsForest
// drives the forest dispatch gate independently of the curated
// builtinForestDefaults allowlist: a synthetic non-built-in language opts in
// via the field, and a built-in name dispatches even when the field is left
// false (its default comes from the curated map).
func TestParserWantsForestFieldDrivesDispatch(t *testing.T) {
	optedIn := &Parser{language: &Language{Name: "x", WantsForest: true}}
	if !parserWantsForest(optedIn) {
		t.Errorf("Language{Name:x, WantsForest:true} should dispatch to forest")
	}

	notOptedIn := &Parser{language: &Language{Name: "x", WantsForest: false}}
	if parserWantsForest(notOptedIn) {
		t.Errorf("Language{Name:x, WantsForest:false} (non-builtin) should NOT dispatch to forest")
	}

	builtinNoField := &Parser{language: &Language{Name: "bash", WantsForest: false}}
	if !parserWantsForest(builtinNoField) {
		t.Errorf("built-in language %q should dispatch to forest via builtinForestDefaults even with WantsForest=false", "bash")
	}

	if parserWantsForest(nil) {
		t.Errorf("parserWantsForest(nil) should be false")
	}
	if parserWantsForest(&Parser{}) {
		t.Errorf("parserWantsForest with nil language should be false")
	}
}

// TestBuiltinForestDefaultsCuratedSet is a regression test asserting the
// curated built-in forest allowlist (migrated from the former
// languageWantsForest name switch) still contains exactly the languages
// validated by TestForestCorpusParity / TestForestVsCOracleParity, including
// "go" (promoted 2026-06-03; see the builtinForestDefaults doc comment).
func TestBuiltinForestDefaultsCuratedSet(t *testing.T) {
	want := []string{"bash", "erlang", "cmake", "css", "scss", "awk", "javascript", "c_sharp", "go"}
	if len(builtinForestDefaults) != len(want) {
		t.Fatalf("builtinForestDefaults has %d entries, want %d: %v", len(builtinForestDefaults), len(want), builtinForestDefaults)
	}
	for _, name := range want {
		if !builtinForestDefaults[name] {
			t.Errorf("builtinForestDefaults missing curated language %q", name)
		}
	}
	// A handful of explicitly-NOT-forest-amenable languages (see the doc
	// comment) must stay out of the curated set.
	notWanted := []string{"python", "rust", "dart", "ruby", "haskell", "php"}
	for _, name := range notWanted {
		if builtinForestDefaults[name] {
			t.Errorf("builtinForestDefaults unexpectedly contains %q", name)
		}
	}
}

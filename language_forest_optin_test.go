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

	certified := &Parser{language: &Language{Name: "x", AutomaticForestEnabledByDefault: true}}
	if !parserWantsForest(certified) {
		t.Error("certified automatic forest profile should dispatch")
	}

	for _, name := range []string{"awk", "kdl", "uxntal"} {
		sameNameCustom := &Parser{language: &Language{Name: name}}
		if parserWantsForest(sameNameCustom) {
			t.Errorf("same-name custom language %q unexpectedly dispatched", name)
		}
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
// validated by TestForestCorpusParity / TestForestVsCOracleParity. "go" is
// deliberately EXCLUDED (commit 6894fc9f): the forest path is correct on
// curated Go corpora, but default dispatch would make Go's full-parse and
// incremental hot paths pay raw-shape/forest/result selection cost the
// ordinary path does not need; see the builtinForestDefaults doc comment.
func TestBuiltinForestDefaultsCuratedSet(t *testing.T) {
	want := []string{
		"bash", "erlang", "cmake", "css", "scss", "javascript", "c_sharp",
		"gitignore", "nix", "squirrel", "prisma",
		"agda", "ledger", "yuck", "json5",
		"bibtex", "faust", "arduino", "authzed", "make", "tlaplus",
		"gitattributes",
	}
	if len(builtinForestDefaults) != len(want) {
		t.Fatalf("builtinForestDefaults has %d entries, want %d: %v", len(builtinForestDefaults), len(want), builtinForestDefaults)
	}
	for _, name := range want {
		if !builtinForestDefaults[name] {
			t.Errorf("builtinForestDefaults missing curated language %q", name)
		}
	}
	// A handful of explicitly-NOT-forest-amenable languages (see the doc
	// comment) must stay out of the curated set. "go" is intentionally held
	// out of default dispatch (commit 6894fc9f) even though it is forest-clean.
	notWanted := []string{"python", "rust", "dart", "ruby", "haskell", "php", "go", "csv", "beancount", "org", "vimdoc", "fish", "racket", "commonlisp"}
	for _, name := range notWanted {
		if builtinForestDefaults[name] {
			t.Errorf("builtinForestDefaults unexpectedly contains %q", name)
		}
	}

	// CSV is a default-dispatch demotion, not removal of the experimental
	// mechanisms. Callers can still opt a CSV language into the forest path
	// explicitly, and its certified forest-recovery policy remains available
	// to that path.
	csv := &Parser{language: &Language{Name: "csv", WantsForest: true}}
	if !parserWantsForest(csv) {
		t.Error("explicit Language.WantsForest should still dispatch CSV to forest")
	}
	if !languageWantsForestRecover("csv") {
		t.Error("CSV forest recovery should remain available to explicit forest runs")
	}

	// Beancount has the same policy split: the certified built-in no longer
	// speculates automatically, while explicit callers retain both the forest
	// entry point and its certified recovery behavior.
	beancount := &Parser{language: &Language{Name: "beancount", WantsForest: true}}
	if !parserWantsForest(beancount) {
		t.Error("explicit Language.WantsForest should still dispatch Beancount to forest")
	}
	if !languageWantsForestRecover("beancount") {
		t.Error("Beancount forest recovery should remain available to explicit forest runs")
	}

	// Org, Vimdoc, and Common Lisp retain the same explicit-only split without
	// losing their recovery profiles.
	for _, name := range []string{"org", "vimdoc", "commonlisp"} {
		parser := &Parser{language: &Language{Name: name, WantsForest: true}}
		if !parserWantsForest(parser) {
			t.Errorf("explicit Language.WantsForest should still dispatch %s to forest", name)
		}
		if !languageWantsForestRecover(name) {
			t.Errorf("%s forest recovery should remain available to explicit forest runs", name)
		}
	}

	// Fish and Racket also keep their certified recovery profiles and direct
	// opt-in path after automatic routing is removed.
	for _, name := range []string{"fish", "racket"} {
		parser := &Parser{language: &Language{Name: name, WantsForest: true}}
		if !parserWantsForest(parser) {
			t.Errorf("explicit Language.WantsForest should still dispatch %s to forest", name)
		}
		if !languageWantsForestRecover(name) {
			t.Errorf("%s forest recovery should remain available to explicit forest runs", name)
		}
	}
}

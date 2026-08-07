package gotreesitter

import "testing"

// TestParserWantsForestFieldDrivesDispatch verifies that Language.WantsForest
// drives the forest admission gate independently of the curated
// builtinForestDefaults allowlist when the scanner proof is present. A
// synthetic non-built-in language opts in via the field, and a built-in name
// dispatches even when the field is left false.
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

	unsafeAutomatic := &Parser{language: &Language{
		Name:                            "x",
		AutomaticForestEnabledByDefault: true,
		ExternalScanner:                 quiescenceOptOutScanner{},
	}}
	if parserWantsForest(unsafeAutomatic) {
		t.Error("automatic forest profile without incremental reuse proof should not dispatch")
	}

	explicitUnsafe := &Parser{language: &Language{
		Name:            "x",
		WantsForest:     true,
		ExternalScanner: quiescenceOptOutScanner{},
	}}
	if parserWantsForest(explicitUnsafe) {
		t.Error("unproven explicit forest opt-in should not dispatch through normal Parse")
	}

	for _, name := range []string{"awk", "kdl", "uxntal"} {
		sameNameCustom := &Parser{language: &Language{Name: name}}
		if parserWantsForest(sameNameCustom) {
			t.Errorf("same-name custom language %q unexpectedly dispatched", name)
		}
	}

	builtinNoField := &Parser{language: &Language{Name: "javascript", WantsForest: false}}
	if !parserWantsForest(builtinNoField) {
		t.Errorf("built-in language %q should dispatch to forest via builtinForestDefaults even with WantsForest=false", "javascript")
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
		"javascript",
		"gitignore", "nix", "squirrel", "prisma",
		"json5", "arduino",
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
	notWanted := []string{
		"python", "rust", "dart", "ruby", "haskell", "php", "go", "csv",
		"beancount", "org", "vimdoc", "fish", "racket", "commonlisp",
		"faust", "cmake", "erlang", "bibtex", "css", "yuck", "bash",
		"scss", "c_sharp", "agda", "ledger", "authzed", "make", "tlaplus",
	}
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

	// Full-corpus recertification moved these legacy promotions to the same
	// explicit-only policy without removing their experimental entry point.
	for _, name := range []string{"faust", "cmake", "erlang"} {
		parser := &Parser{language: &Language{Name: name, WantsForest: true}}
		if !parserWantsForest(parser) {
			t.Errorf("explicit Language.WantsForest should still dispatch %s to forest", name)
		}
	}

	// Full-manifest recertification also moved these legacy defaults to
	// explicit-only routing without removing their experimental entry point.
	for _, name := range []string{
		"bibtex", "css", "yuck", "bash", "scss", "c_sharp", "agda",
		"ledger", "authzed", "make", "tlaplus",
	} {
		if parserWantsForest(&Parser{language: &Language{Name: name}}) {
			t.Errorf("%s should not dispatch to forest automatically", name)
		}
		parser := &Parser{language: &Language{Name: name, WantsForest: true}}
		if !parserWantsForest(parser) {
			t.Errorf("explicit Language.WantsForest should still dispatch %s to forest", name)
		}
	}
}

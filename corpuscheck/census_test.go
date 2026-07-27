package corpuscheck

import (
	"path/filepath"
	"testing"
)

// These tests use the fixtures in testdata/upstream_corpus.
// See NOTICE.md for their provenance.
// The fixtures give continuous integration a stable signal.
// They do not require the gts-corpora sibling checkout.
// Each test records the cause of every known failure.
// TestCorpusFormat_RealWorld runs the larger optional corpus sweep.

func vendoredRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "upstream_corpus"))
	if err != nil {
		t.Fatalf("resolve vendored corpus root: %v", err)
	}
	return abs
}

// TestVendoredCorpus_CleanLanguages is a regression gate.
// The CSS and HTML fixtures pass the strict comparison.
// A failure identifies a gotreesitter defect.
// These fixtures include field annotations.
func TestVendoredCorpus_CleanLanguages(t *testing.T) {
	root := vendoredRoot(t)
	for _, lang := range []string{"css", "html"} {
		lang := lang
		t.Run(lang, func(t *testing.T) {
			r := RunLanguage(lang, filepath.Join(root, lang))
			if r.SkipReason != "" {
				t.Fatalf("%s: unexpected skip: %s", lang, r.SkipReason)
			}
			if r.Cases == 0 {
				t.Fatalf("%s: no cases found under %s", lang, r.CorpusRoot)
			}
			if r.Pass != r.Cases {
				t.Fatalf("%s: %d/%d strict passes (want %d/%d); first failure: %s",
					lang, r.Pass, r.Cases, r.Cases, r.Cases, firstFailure(r))
			}
		})
	}
}

// TestVendoredCorpus_JSONFieldStaleness pins a specific, verified finding:
// tree-sitter-json's own test/corpus/main.txt was never regenerated after
// grammar.js added `key`/`value` fields to the `pair` rule in 2019
// (upstream commit fc89d8a, "Add key/value fields to JSON") -- confirmed
// by walking tree-sitter-json's git history. gotreesitter's tree
// correctly reports those fields; the fixture just doesn't expect them.
// If this test starts failing because MORE than 2 cases fail, or the
// failures are no longer both "field" mismatches at these exact paths,
// that's a real regression worth investigating, not fixture drift.
func TestVendoredCorpus_JSONFieldStaleness(t *testing.T) {
	root := vendoredRoot(t)
	r := RunLanguage("json", filepath.Join(root, "json"))
	if r.SkipReason != "" {
		t.Fatalf("unexpected skip: %s", r.SkipReason)
	}
	if r.Cases != 6 {
		t.Fatalf("Cases = %d, want 6 (has the vendored fixture changed?)", r.Cases)
	}
	if r.PassFieldLenient != 6 {
		t.Fatalf("PassFieldLenient = %d, want 6: every case should match once missing "+
			"upstream field annotations are excused; a lower number means a real (non-field) "+
			"divergence crept in: %s", r.PassFieldLenient, firstFailure(r))
	}
	if r.Pass != 4 {
		t.Fatalf("Pass (strict) = %d, want 4 -- either gotreesitter regressed, or someone "+
			"regenerated the vendored fixture to include fields (update this test to Pass=%d "+
			"if so): failures=%v", r.Pass, len(r.Failed), r.Failed)
	}
	for _, f := range r.Failed {
		if f.Reason != CategoryField {
			t.Errorf("unexpected non-field failure %q at %s: %s", f.Reason, f.Path, f.Detail)
		}
	}
}

// TestVendoredCorpus_GoKnownDefect pins a genuine gotreesitter parser
// defect (not fixture staleness): given a dangling `a.` selector
// followed by a comment and then an `if` statement, gotreesitter's error
// recovery produces a materially wrong tree (folds the `if` statement
// into a bogus binary/composite-literal expression, and loses track of
// the second function's closing brace, which surfaces as a spurious
// top-level ERROR). This is tracked here, deliberately, as a check that
// fails loudly if the shape of the bug changes (in which case this test
// needs updating, not silencing) rather than a TODO comment nobody reads.
func TestVendoredCorpus_GoKnownDefect(t *testing.T) {
	root := vendoredRoot(t)
	r := RunLanguage("go", filepath.Join(root, "go"))
	if r.SkipReason != "" {
		t.Fatalf("unexpected skip: %s", r.SkipReason)
	}
	if r.Cases != 7 {
		t.Fatalf("Cases = %d, want 7 (has the vendored fixture changed?)", r.Cases)
	}

	var errorsCaseFailure *CaseResult
	for i := range r.Failed {
		if filepath.Base(r.Failed[i].File) == "errors.txt" {
			errorsCaseFailure = &r.Failed[i]
			break
		}
	}
	if errorsCaseFailure == nil {
		t.Fatalf("errors.txt's case no longer fails -- if gotreesitter's error recovery for " +
			"dangling selector expressions was fixed, delete this test (or replace it with a " +
			"regression guard) instead of leaving it stale")
	}
	if errorsCaseFailure.Reason != CategoryShape || errorsCaseFailure.Path != "/source_file" {
		t.Fatalf("errors.txt now fails a different way than the known defect (reason=%s path=%s: %s); "+
			"update this test to describe the current failure",
			errorsCaseFailure.Reason, errorsCaseFailure.Path, errorsCaseFailure.Detail)
	}

	// The rest of the vendored go corpus (source_files.txt) is stale-field
	// only, same as json -- confirm that's still true so a genuine new
	// gotreesitter defect there doesn't hide behind "it's just fields".
	if r.PassFieldLenient != 6 {
		t.Fatalf("PassFieldLenient = %d, want 6: a non-field divergence appeared outside "+
			"the known errors.txt defect: %v", r.PassFieldLenient, r.Failed)
	}
}

// TestVendoredCorpus_TOMLSanity is a coarser tripwire on a real, mixed
// upstream corpus (deliberately not narrowed to a single mismatch
// category): TOML's error-recovery paths ("INVALID - ..." cases
// asserting a parse error, several of which gotreesitter currently
// mis-shapes) are where most of its failures live. This just guards
// against silent regression in the ~80% that currently pass.
func TestVendoredCorpus_TOMLSanity(t *testing.T) {
	root := vendoredRoot(t)
	r := RunLanguage("toml", filepath.Join(root, "toml"))
	if r.SkipReason != "" {
		t.Fatalf("unexpected skip: %s", r.SkipReason)
	}
	if r.Cases != 63 {
		t.Fatalf("Cases = %d, want 63 (has the vendored fixture changed?)", r.Cases)
	}
	if r.Pass < 50 {
		t.Fatalf("Pass = %d, want >= 50 (regression); failures: %v", r.Pass, r.Failed)
	}
}

func firstFailure(r *LanguageReport) string {
	if len(r.Failed) == 0 {
		return "<no failures recorded>"
	}
	f := r.Failed[0]
	return f.Detail
}

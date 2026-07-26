package corpuscheck

import (
	"os"
	"path/filepath"
	"testing"
)

// gtsCorporaRoot returns the sibling gts-corpora checkout's
// grammar_parity directory if present on this machine, or "" if not.
// gts-corpora is NOT part of this repository and must never be required
// for `go test ./...` to pass -- this is an opportunistic, skip-if-absent
// sweep for local/manual use, not a CI gate. TestVendoredCorpus_* in
// census_test.go is the always-on equivalent, running against a small
// vendored fixture set instead.
func gtsCorporaRoot(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.Getenv("GTS_CORPORA_GRAMMAR_PARITY"),
		filepath.Join("..", "..", "gts-corpora", "grammar_parity"),
		filepath.Join("..", "..", "..", "gts-corpora", "grammar_parity"),
		"/home/draco/work/gts-corpora/grammar_parity",
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	t.Skip("gts-corpora/grammar_parity not found; skipping opportunistic upstream corpus sweep (set GTS_CORPORA_GRAMMAR_PARITY to run it)")
	return ""
}

// languageCorpusDir mirrors cmd/corpuscheck's directory discovery for a
// handful of representative languages, without pulling in the full
// alias table (that lives in the CLI, which this package doesn't import
// to avoid an import cycle risk and keep the library dependency-free).
func languageCorpusDir(root, name string) (string, bool) {
	langRoot := filepath.Join(root, name)
	for _, candidate := range []string{
		filepath.Join(langRoot, "test", "corpus"),
		filepath.Join(langRoot, "corpus"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
	}
	dirs, err := DiscoverCorpusDirs(langRoot)
	if err != nil || len(dirs) == 0 {
		return "", false
	}
	return dirs[0], true
}

// TestCorpusFormat_RealWorld runs a modest, deliberately bounded sample
// of upstream grammars against the real gts-corpora checkout when it's
// present. It exists to sanity-check the corpus-format parser and
// renderer against real, unmodified upstream fixtures beyond the small
// vendored set -- see cmd/corpuscheck for the full (188+ language) sweep
// this project's report is based on, which is intentionally not
// reproduced as a `go test` because it takes an explicit, scoped
// invocation on a shared machine rather than running unattended in CI.
func TestCorpusFormat_RealWorld(t *testing.T) {
	root := gtsCorporaRoot(t)

	// A small, fixed sample: clean baselines (json, go) plus a few of
	// the languages this project has specifically had to reason about
	// upstream construct coverage for (yaml, bash) -- not the full
	// sweep, to keep this fast and deterministic for local use.
	for _, lang := range []string{"json", "go", "yaml", "bash"} {
		lang := lang
		t.Run(lang, func(t *testing.T) {
			dir, ok := languageCorpusDir(root, lang)
			if !ok {
				t.Skipf("no corpus directory found for %s under %s", lang, root)
			}
			r := RunLanguage(lang, dir)
			if r.SkipReason != "" {
				t.Skipf("%s: %s", lang, r.SkipReason)
			}
			if r.Cases == 0 {
				t.Skipf("%s: no cases found under %s", lang, dir)
			}
			t.Logf("%s: %d cases, %d strict passes (%.1f%%), %d field-lenient passes (%.1f%%)",
				lang, r.Cases, r.Pass, 100*float64(r.Pass)/float64(r.Cases),
				r.PassFieldLenient, 100*float64(r.PassFieldLenient)/float64(r.Cases))
			if r.Pass == 0 {
				t.Errorf("%s: 0/%d strict passes -- either the grammar mapping broke or "+
					"gotreesitter regressed completely for this language; first failure: %s",
					lang, r.Cases, firstFailure(r))
			}
		})
	}
}

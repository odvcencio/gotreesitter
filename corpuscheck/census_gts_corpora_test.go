package corpuscheck

import (
	"os"
	"path/filepath"
	"testing"
)

// gtsCorporaRoot returns a sibling gts-corpora checkout's grammar_parity
// directory. It returns an empty string when no checkout is available.
// This repository does not contain gts-corpora.
// Do not require it for `go test ./...`.
// This optional sweep supports local use. It is not a continuous integration
// gate. TestVendoredCorpus_* provides the small, always-on equivalent.
func gtsCorporaRoot(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.Getenv("GTS_CORPORA_GRAMMAR_PARITY"),
		filepath.Join("..", "..", "gts-corpora", "grammar_parity"),
		filepath.Join("..", "..", "..", "gts-corpora", "grammar_parity"),
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

// languageCorpusDir mirrors the command's directory search for a small
// language sample. The command owns the full alias table.
// This package does not import the command. That rule prevents an import
// cycle and keeps the library independent.
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

// TestCorpusFormat_RealWorld runs a small, bounded upstream sample when
// gts-corpora is available. It checks the format parser and renderer against
// unchanged upstream fixtures.
// Use cmd/corpuscheck for the full sweep. Run that command explicitly on a
// shared machine. Do not run the full sweep through `go test`.
func TestCorpusFormat_RealWorld(t *testing.T) {
	root := gtsCorporaRoot(t)

	// Use clean baselines and two languages with known coverage questions.
	// Keep this sample small and deterministic for local use.
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

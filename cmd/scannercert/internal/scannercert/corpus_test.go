package scannercert

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const sampleText = "local x = 1\nreturn x\n"

type manifestEntry struct {
	Language      string `json:"language"`
	Role          string `json:"role"`
	CommittedPath string `json:"committed_path"`
	SHA256        string `json:"sha256"`
	SessionSHA256 string `json:"session_sha256,omitempty"`
}

// writeCorpus lays out a one-grammar corpus under a temporary repository root.
func writeCorpus(t *testing.T, mutate func(*manifestEntry)) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "benchfixtures")
	if err := os.MkdirAll(filepath.Join(dir, "testdata", "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", "real", "x"), []byte(sampleText), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := manifestEntry{
		Language: "x", Role: "sample", CommittedPath: "testdata/real/x",
		SHA256:        fmt.Sprintf("%x", sha256.Sum256([]byte(sampleText))),
		SessionSHA256: benchfixtures.EditingSessionSHA256([]byte(sampleText)),
	}
	if mutate != nil {
		mutate(&entry)
	}
	data, err := json.Marshal(map[string]any{"schema": "benchfixture-real-corpus-v1", "entries": []manifestEntry{entry}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real_corpus.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadCorpusReturnsTheSampleThenItsEditingSession(t *testing.T) {
	root := writeCorpus(t, nil)
	c, err := LoadCorpus(root, "x", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 1+benchfixtures.EditSessionSteps || string(c.Sources[0]) != sampleText {
		t.Fatalf("sources = %d, first %q; want the sample then %d steps", len(c.Sources), c.Sources[0], benchfixtures.EditSessionSteps)
	}
	steps := benchfixtures.EditingSession([]byte(sampleText))
	if string(c.Sources[len(c.Sources)-1]) != string(steps[len(steps)-1].Source) {
		t.Fatal("the last source is not the last editing-session step")
	}
	if c.Language != "x" || c.Path != "testdata/real/x" || len(c.ManifestSHA256) != 64 ||
		c.SampleSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(sampleText))) || c.SessionSHA256 != benchfixtures.EditingSessionSHA256([]byte(sampleText)) {
		t.Fatalf("corpus identity = %+v", c)
	}
}

func TestLoadCorpusCanSkipTheEditingSession(t *testing.T) {
	c, err := LoadCorpus(writeCorpus(t, nil), "x", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 1 {
		t.Fatalf("sources = %d, want only the sample", len(c.Sources))
	}
}

func TestLoadCorpusRejectsInputsThatDriftFromTheManifest(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*manifestEntry)
		want   string
	}{
		"sample bytes": {func(e *manifestEntry) { e.SHA256 = strings.Repeat("0", 64) }, "digest"},
		"session":      {func(e *manifestEntry) { e.SessionSHA256 = strings.Repeat("0", 64) }, "editing session"},
		"no sample":    {func(e *manifestEntry) { e.CommittedPath = "" }, "no committed sample"},
		"other role":   {func(e *manifestEntry) { e.Role = "median_1" }, "no committed sample"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadCorpus(writeCorpus(t, tc.mutate), "x", true)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
	if _, err := LoadCorpus(writeCorpus(t, nil), "missing", true); err == nil || !strings.Contains(err.Error(), "no committed sample") {
		t.Fatalf("err = %v, want an unknown grammar to have no committed sample", err)
	}
	if _, err := LoadCorpus(t.TempDir(), "x", true); err == nil {
		t.Fatal("a root without a manifest loaded")
	}
}

// The checked-in manifest pins every committed sample and its editing
// session. Loading lua here proves LoadCorpus agrees with those pins.
func TestLoadCorpusReadsThePinnedRepositorySample(t *testing.T) {
	c, err := LoadCorpus(repoRoot(t), "lua", true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != "testdata/real/lua" || len(c.Sources) != 1+benchfixtures.EditSessionSteps {
		t.Fatalf("corpus = %q with %d sources", c.Path, len(c.Sources))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no go.mod at %s: %v", root, err)
	}
	return root
}

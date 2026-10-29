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
	Bytes         int    `json:"bytes"`
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
	c, err := LoadCorpus(root, "x", CorpusOptions{Session: true})
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
	c, err := LoadCorpus(writeCorpus(t, nil), "x", CorpusOptions{})
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
			_, err := LoadCorpus(writeCorpus(t, tc.mutate), "x", CorpusOptions{Session: true})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
	if _, err := LoadCorpus(writeCorpus(t, nil), "missing", CorpusOptions{Session: true}); err == nil || !strings.Contains(err.Error(), "no committed sample") {
		t.Fatalf("err = %v, want an unknown grammar to have no committed sample", err)
	}
	if _, err := LoadCorpus(t.TempDir(), "x", CorpusOptions{Session: true}); err == nil {
		t.Fatal("a root without a manifest loaded")
	}
}

// fetchedRow is one manifest row for an input the repository does not commit.
// The manifest pins pinned. The fetch directory holds onDisk, which defaults to
// the pinned bytes, unless skipWrite leaves the file out.
type fetchedRow struct {
	role      string
	pinned    []byte
	onDisk    []byte
	skipWrite bool
}

// writeFetchedCorpus lays out the committed sample as writeCorpus does, adds a
// manifest row for each fetched row in the order given, and writes the
// rows' files the way scripts/benchfixture_corpus.py fetch does: DIR/x/<role>.
func writeFetchedCorpus(t *testing.T, rows []fetchedRow) (root, fetched string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, "internal", "benchfixtures")
	if err := os.MkdirAll(filepath.Join(dir, "testdata", "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", "real", "x"), []byte(sampleText), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []manifestEntry{{
		Language: "x", Role: "sample", CommittedPath: "testdata/real/x",
		SHA256:        fmt.Sprintf("%x", sha256.Sum256([]byte(sampleText))),
		SessionSHA256: benchfixtures.EditingSessionSHA256([]byte(sampleText)),
		Bytes:         len(sampleText),
	}}
	fetched = t.TempDir()
	for _, row := range rows {
		entries = append(entries, manifestEntry{
			Language: "x", Role: row.role, Bytes: len(row.pinned),
			SHA256: fmt.Sprintf("%x", sha256.Sum256(row.pinned)),
		})
		if row.skipWrite {
			continue
		}
		data := row.onDisk
		if data == nil {
			data = row.pinned
		}
		if err := os.MkdirAll(filepath.Join(fetched, "x"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fetched, "x", row.role), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := json.Marshal(map[string]any{"schema": "benchfixture-real-corpus-v1", "entries": entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real_corpus.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, fetched
}

func digestOf(data string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(data))) }

// The fetched inputs follow the sample and its session, in a fixed role order
// whatever the order of the manifest rows.
func TestLoadCorpusAddsTheFetchedInputsAfterTheSessionInRoleOrder(t *testing.T) {
	root, fetched := writeFetchedCorpus(t, []fetchedRow{
		{role: "largest", pinned: []byte("local big = 1\n")},
		{role: "median_2", pinned: []byte("local m2 = 2\n")},
		{role: "median_1", pinned: []byte("local m1 = 1\n")},
	})
	c, err := LoadCorpus(root, "x", CorpusOptions{Session: true, Fetched: fetched})
	if err != nil {
		t.Fatal(err)
	}
	first := 1 + benchfixtures.EditSessionSteps
	if len(c.Sources) != first+3 {
		t.Fatalf("sources = %d, want the sample, its session and three fetched inputs (%d)", len(c.Sources), first+3)
	}
	want := []InputInfo{
		{Role: "median_1", Source: first, Bytes: 13, SHA256: digestOf("local m1 = 1\n")},
		{Role: "median_2", Source: first + 1, Bytes: 13, SHA256: digestOf("local m2 = 2\n")},
		{Role: "largest", Source: first + 2, Bytes: 14, SHA256: digestOf("local big = 1\n")},
	}
	if len(c.Fetched) != len(want) {
		t.Fatalf("fetched = %+v, want %+v", c.Fetched, want)
	}
	for i := range want {
		if c.Fetched[i] != want[i] || string(c.Sources[want[i].Source]) != map[string]string{"median_1": "local m1 = 1\n", "median_2": "local m2 = 2\n", "largest": "local big = 1\n"}[want[i].Role] {
			t.Fatalf("fetched[%d] = %+v, want %+v at its source position", i, c.Fetched[i], want[i])
		}
	}
}

// The committed inputs alone are the default, so tests and CI need no fetch.
func TestLoadCorpusReadsNothingFetchedUnlessAsked(t *testing.T) {
	root, _ := writeFetchedCorpus(t, []fetchedRow{{role: "median_1", pinned: []byte("local m = 1\n")}})
	c, err := LoadCorpus(root, "x", CorpusOptions{Session: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 1+benchfixtures.EditSessionSteps || len(c.Fetched) != 0 {
		t.Fatalf("sources %d fetched %+v, want only the sample and its session", len(c.Sources), c.Fetched)
	}
}

// Two rows can pin the same bytes (a language's second and third median, or its
// largest file and its sample). One parse stands for both.
func TestLoadCorpusParsesIdenticalBytesOnce(t *testing.T) {
	same := []byte("local m = 2\n")
	root, fetched := writeFetchedCorpus(t, []fetchedRow{
		{role: "median_1", pinned: []byte(sampleText)},
		{role: "median_2", pinned: same},
		{role: "median_3", pinned: same},
	})
	c, err := LoadCorpus(root, "x", CorpusOptions{Fetched: fetched})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 2 || string(c.Sources[1]) != string(same) {
		t.Fatalf("sources = %d, want the sample and one more", len(c.Sources))
	}
	sources := []int{c.Fetched[0].Source, c.Fetched[1].Source, c.Fetched[2].Source}
	if len(c.Fetched) != 3 || sources[0] != 0 || sources[1] != 1 || sources[2] != 1 {
		t.Fatalf("fetched sources = %v, want 0 (the sample's bytes), 1 and 1", sources)
	}
}

func TestLoadCorpusRejectsFetchedInputsThatDriftFromTheManifest(t *testing.T) {
	pinned := []byte("local m = 1\n")
	for name, tc := range map[string]struct {
		rows []fetchedRow
		want string
	}{
		"missing file":   {[]fetchedRow{{role: "median_1", pinned: pinned, skipWrite: true}}, "benchfixture_corpus.py fetch"},
		"longer file":    {[]fetchedRow{{role: "median_1", pinned: pinned, onDisk: append([]byte("local m = 1\n"), '-')}}, "bytes"},
		"changed bytes":  {[]fetchedRow{{role: "median_1", pinned: pinned, onDisk: []byte("local m = 9\n")}}, "digest"},
		"nothing pinned": {nil, "pins no median or largest"},
	} {
		t.Run(name, func(t *testing.T) {
			root, fetched := writeFetchedCorpus(t, tc.rows)
			_, err := LoadCorpus(root, "x", CorpusOptions{Fetched: fetched})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
	// A directory where a file belongs is not an input.
	root, fetched := writeFetchedCorpus(t, []fetchedRow{{role: "median_1", pinned: pinned, skipWrite: true}})
	if err := os.MkdirAll(filepath.Join(fetched, "x", "median_1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCorpus(root, "x", CorpusOptions{Fetched: fetched}); err == nil {
		t.Fatal("a directory loaded as a fetched input")
	}
}

// The checked-in manifest pins every committed sample and its editing
// session. Loading lua here proves LoadCorpus agrees with those pins.
func TestLoadCorpusReadsThePinnedRepositorySample(t *testing.T) {
	c, err := LoadCorpus(repoRoot(t), "lua", CorpusOptions{Session: true})
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

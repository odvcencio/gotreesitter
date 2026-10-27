package scannercert

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Corpus is the pinned input for one grammar: its committed real-corpus
// sample, then the sources of the sample's pinned editing session, then any
// fetched inputs.
type Corpus struct {
	Language       string
	Path           string // sample path relative to internal/benchfixtures
	SampleSHA256   string
	SessionSHA256  string
	ManifestSHA256 string
	Sources        [][]byte
	// Fetched lists the inputs read from a fetch directory, in the order of
	// FetchedRoles. Source is the index in Sources of the parse of each one.
	Fetched []InputInfo
}

// FetchedRoles are the manifest roles that name inputs the repository does not
// commit, in the order LoadCorpus adds them.
var FetchedRoles = []string{"median_1", "median_2", "median_3", "largest"}

// CorpusOptions chooses what LoadCorpus reads beyond the committed sample.
type CorpusOptions struct {
	// Session adds the 72 sources of the sample's pinned editing session.
	Session bool
	// Fetched names a directory that scripts/benchfixture_corpus.py wrote with
	// its fetch command (python3 scripts/benchfixture_corpus.py fetch
	// --language L --output DIR), which holds DIR/L/<role>. When it is set,
	// LoadCorpus adds the language's median and largest inputs after the sample
	// and its session. Each is parsed whole, with no editing session.
	Fetched string
}

// manifestRow is the part of a real-corpus manifest entry that LoadCorpus reads.
type manifestRow struct {
	Language      string `json:"language"`
	Role          string `json:"role"`
	CommittedPath string `json:"committed_path"`
	Bytes         int    `json:"bytes"`
	SHA256        string `json:"sha256"`
	SessionSHA256 string `json:"session_sha256"`
}

// LoadCorpus reads language's committed sample from the real-corpus manifest
// under root, which is the repository root. It refuses a sample or editing
// session whose digest differs from the manifest's, and a fetched input whose
// length or digest differs.
func LoadCorpus(root, language string, opts CorpusOptions) (*Corpus, error) {
	dir := filepath.Join(root, "internal", "benchfixtures")
	data, err := os.ReadFile(filepath.Join(dir, "real_corpus.json"))
	if err != nil {
		return nil, fmt.Errorf("scannercert: read the real-corpus manifest: %w (run from the repository root or pass it with -root)", err)
	}
	var manifest struct {
		Entries []manifestRow `json:"entries"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("scannercert: decode the real-corpus manifest: %w", err)
	}
	var sample *manifestRow
	fetched := map[string]manifestRow{}
	for i, entry := range manifest.Entries {
		if entry.Language != language {
			continue
		}
		switch {
		case entry.Role == "sample" && entry.CommittedPath != "" && sample == nil:
			sample = &manifest.Entries[i]
		case slices.Contains(FetchedRoles, entry.Role):
			if _, seen := fetched[entry.Role]; !seen {
				fetched[entry.Role] = entry
			}
		}
	}
	if sample == nil {
		return nil, fmt.Errorf("scannercert: %s has no committed sample in the real-corpus manifest", language)
	}
	path := filepath.FromSlash(sample.CommittedPath)
	if !filepath.IsLocal(path) {
		return nil, fmt.Errorf("scannercert: %s sample path %q leaves the fixture directory", language, sample.CommittedPath)
	}
	text, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		return nil, fmt.Errorf("scannercert: read the %s sample: %w", language, err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(text)); got != sample.SHA256 {
		return nil, fmt.Errorf("scannercert: %s sample digest %s differs from the manifest's %s", language, got, sample.SHA256)
	}
	if got := benchfixtures.EditingSessionSHA256(text); got != sample.SessionSHA256 {
		return nil, fmt.Errorf("scannercert: %s editing session digest %s differs from the manifest's %s", language, got, sample.SessionSHA256)
	}
	corpus := &Corpus{
		Language:       language,
		Path:           sample.CommittedPath,
		SampleSHA256:   sample.SHA256,
		SessionSHA256:  sample.SessionSHA256,
		ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
		Sources:        [][]byte{text},
	}
	if opts.Session {
		for _, step := range benchfixtures.EditingSession(text) {
			corpus.Sources = append(corpus.Sources, step.Source)
		}
	}
	if opts.Fetched != "" {
		if err := corpus.addFetched(opts.Fetched, fetched); err != nil {
			return nil, err
		}
	}
	return corpus, nil
}

// addFetched reads the language's median and largest inputs from dir, checks
// each against the manifest, and appends the ones whose bytes are new. A role
// whose bytes equal an earlier source's (the sample, or another role) is not
// parsed twice; its entry in Fetched names the earlier source.
func (c *Corpus) addFetched(dir string, rows map[string]manifestRow) error {
	if len(rows) == 0 {
		return fmt.Errorf("scannercert: the manifest pins no median or largest input for %s", c.Language)
	}
	seen := map[string]int{c.SampleSHA256: 0}
	for _, role := range FetchedRoles {
		row, ok := rows[role]
		if !ok {
			continue
		}
		data, err := readFetched(dir, c.Language, role, row)
		if err != nil {
			return err
		}
		source, duplicate := seen[row.SHA256]
		if !duplicate {
			source = len(c.Sources)
			c.Sources = append(c.Sources, data)
			seen[row.SHA256] = source
		}
		c.Fetched = append(c.Fetched, InputInfo{Role: role, Source: source, Bytes: len(data), SHA256: row.SHA256})
	}
	return nil
}

// readFetched reads dir/language/role and checks its length and SHA-256 against
// the manifest row, as scripts/benchfixture_corpus.py does when it writes the
// file. The length is checked before the read, so a wrong file is not loaded.
func readFetched(dir, language, role string, row manifestRow) ([]byte, error) {
	rel := filepath.Join(language, role)
	if !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("scannercert: the fetched path %q leaves the fetch directory", rel)
	}
	path := filepath.Join(dir, rel)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("scannercert: read the fetched %s: %w (fetch it with: python3 scripts/benchfixture_corpus.py fetch --language %s --output %s)",
			rel, err, language, dir)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("scannercert: the fetched %s is not a regular file", rel)
	}
	if info.Size() != int64(row.Bytes) {
		return nil, fmt.Errorf("scannercert: the fetched %s has %d bytes, the manifest pins %d", rel, info.Size(), row.Bytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scannercert: read the fetched %s: %w", rel, err)
	}
	if len(data) != row.Bytes {
		return nil, fmt.Errorf("scannercert: the fetched %s has %d bytes, the manifest pins %d", rel, len(data), row.Bytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != row.SHA256 {
		return nil, fmt.Errorf("scannercert: the fetched %s has digest %s, the manifest pins %s", rel, got, row.SHA256)
	}
	return data, nil
}

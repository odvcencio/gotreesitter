package scannercert

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Corpus is the pinned input for one grammar: its committed real-corpus
// sample, then the sources of the sample's pinned editing session.
type Corpus struct {
	Language       string
	Path           string // sample path relative to internal/benchfixtures
	SampleSHA256   string
	SessionSHA256  string
	ManifestSHA256 string
	Sources        [][]byte
}

// LoadCorpus reads language's committed sample from the real-corpus manifest
// under root, which is the repository root. It refuses a sample or editing
// session whose digest differs from the manifest's. With session set, the 72
// sources of the editing session follow the sample.
func LoadCorpus(root, language string, session bool) (*Corpus, error) {
	dir := filepath.Join(root, "internal", "benchfixtures")
	data, err := os.ReadFile(filepath.Join(dir, "real_corpus.json"))
	if err != nil {
		return nil, fmt.Errorf("scannercert: read the real-corpus manifest: %w (run from the repository root or pass it with -root)", err)
	}
	var manifest struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			CommittedPath string `json:"committed_path"`
			SHA256        string `json:"sha256"`
			SessionSHA256 string `json:"session_sha256"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("scannercert: decode the real-corpus manifest: %w", err)
	}
	for _, entry := range manifest.Entries {
		if entry.Language != language || entry.Role != "sample" || entry.CommittedPath == "" {
			continue
		}
		path := filepath.FromSlash(entry.CommittedPath)
		if !filepath.IsLocal(path) {
			return nil, fmt.Errorf("scannercert: %s sample path %q leaves the fixture directory", language, entry.CommittedPath)
		}
		sample, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			return nil, fmt.Errorf("scannercert: read the %s sample: %w", language, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(sample)); got != entry.SHA256 {
			return nil, fmt.Errorf("scannercert: %s sample digest %s differs from the manifest's %s", language, got, entry.SHA256)
		}
		if got := benchfixtures.EditingSessionSHA256(sample); got != entry.SessionSHA256 {
			return nil, fmt.Errorf("scannercert: %s editing session digest %s differs from the manifest's %s", language, got, entry.SessionSHA256)
		}
		corpus := &Corpus{
			Language:       language,
			Path:           entry.CommittedPath,
			SampleSHA256:   entry.SHA256,
			SessionSHA256:  entry.SessionSHA256,
			ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
			Sources:        [][]byte{sample},
		}
		if session {
			for _, step := range benchfixtures.EditingSession(sample) {
				corpus.Sources = append(corpus.Sources, step.Source)
			}
		}
		return corpus, nil
	}
	return nil, fmt.Errorf("scannercert: %s has no committed sample in the real-corpus manifest", language)
}

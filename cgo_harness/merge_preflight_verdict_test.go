//go:build linux && cgo && treesitter_c_parity && gts_merge_census

package cgoharness

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const q1PreflightVerdictReceiptPath = "testdata/q1-preflight-verdicts.json"

type q1PreflightVerdictFixture struct {
	ID      string `json:"id"`
	Grammar string `json:"grammar"`
	SHA256  string `json:"sha256"`
	Path    string `json:"-"`
	Source  []byte `json:"-"`
}

type q1PreflightVerdictRow struct {
	ID      string `json:"id"`
	Grammar string `json:"grammar"`
	SHA256  string `json:"sha256"`
	Accepts uint64 `json:"accepts"`
	Rejects uint64 `json:"rejects"`
	Digest  string `json:"digest"`
}

type q1PreflightVerdictReceipt struct {
	Schema string                  `json:"schema"`
	Rows   []q1PreflightVerdictRow `json:"rows"`
}

type q1RealCorpusManifest struct {
	Entries []struct {
		Language      string `json:"language"`
		Role          string `json:"role"`
		CommittedPath string `json:"committed_path"`
		SHA256        string `json:"sha256"`
	} `json:"entries"`
}

// TestMergePreflightVerdictEquivalence pins every recursive canMergeNodes
// decision made by the R7 cliffs and the top-50 R4 source samples. The census
// build tag keeps this diagnostic instrumentation out of shipped builds.
func TestMergePreflightVerdictEquivalence(t *testing.T) {
	selectedLanguage := strings.TrimSpace(os.Getenv("GTS_Q1_VERDICT_LANGUAGE"))
	fixtures := q1PreflightVerdictFixtures(t, selectedLanguage)
	rows := make([]q1PreflightVerdictRow, 0, len(fixtures))
	var maxWorkPerMerge, workLimitTrips uint64
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(fixture.Grammar)
			if entry == nil || entry.Language() == nil {
				t.Fatalf("Go grammar %q unavailable", fixture.Grammar)
			}
			lang := entry.Language()
			gts.MergeEventCensusReset()
			parser := gts.NewParser(lang)
			tree, err := parser.Parse(fixture.Source)
			if err != nil {
				t.Fatalf("parse %s: %v", fixture.ID, err)
			}
			if tree != nil {
				tree.Release()
			}
			counts := gts.MergeEventCensusSnapshot()
			if counts.PreflightMaxWorkPerMerge > maxWorkPerMerge {
				maxWorkPerMerge = counts.PreflightMaxWorkPerMerge
			}
			workLimitTrips += counts.PreflightWorkLimitTrips
			rows = append(rows, q1PreflightVerdictRow{
				ID: fixture.ID, Grammar: fixture.Grammar, SHA256: fixture.SHA256,
				Accepts: counts.PreflightVerdictAccepts,
				Rejects: counts.PreflightVerdictRejects,
				Digest:  fmt.Sprintf("%016x", counts.PreflightVerdictDigest),
			})
		})
	}
	if t.Failed() {
		return
	}
	got := q1PreflightVerdictReceipt{Schema: "q1-preflight-verdicts-v1", Rows: rows}
	encoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := q1PreflightVerdictReceiptPath
	if strings.TrimSpace(os.Getenv("GTS_Q1_VERDICT_CORPUS_ROOT")) != "" {
		path = "testdata/q1-preflight-verdicts-full-corpus.json"
	}
	if output := strings.TrimSpace(os.Getenv("GTS_Q1_VERDICT_OUTPUT")); output != "" {
		path = output
	}
	if os.Getenv("GTS_Q1_VERDICT_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d verdict receipts to %s", len(rows), path)
		return
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read baseline verdict receipt: %v (set GTS_Q1_VERDICT_UPDATE=1 only on the pre-change implementation)", err)
	}
	var want q1PreflightVerdictReceipt
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatalf("decode baseline verdict receipt: %v", err)
	}
	if want.Schema != got.Schema {
		t.Fatalf("verdict receipt schema changed: got %q, want %q", got.Schema, want.Schema)
	}
	wantByID := make(map[string]q1PreflightVerdictRow, len(want.Rows))
	for _, row := range want.Rows {
		if selectedLanguage == "" || row.Grammar == selectedLanguage {
			wantByID[row.ID] = row
		}
	}
	if len(got.Rows) != len(wantByID) {
		t.Fatalf("verdict receipt row count changed for %q: got %d, want %d", selectedLanguage, len(got.Rows), len(wantByID))
	}
	gotByID := make(map[string]q1PreflightVerdictRow, len(got.Rows))
	for _, row := range got.Rows {
		wantRow, ok := wantByID[row.ID]
		if !ok {
			t.Errorf("merge verdict fixture %s has no baseline receipt", row.ID)
			continue
		}
		if _, duplicate := gotByID[row.ID]; duplicate {
			t.Errorf("merge verdict fixture %s was counted more than once", row.ID)
			continue
		}
		gotByID[row.ID] = row
		if row != wantRow {
			t.Errorf("merge verdicts changed for %s: got %+v, want %+v", row.ID, row, wantRow)
		}
	}
	for id := range wantByID {
		if _, ok := gotByID[id]; !ok {
			t.Errorf("merge verdict fixture %s is missing from the candidate census", id)
		}
	}
	t.Logf("merge verdicts unchanged for %d pinned inputs; max preflight work per merge=%d; work-limit trips=%d", len(rows), maxWorkPerMerge, workLimitTrips)
}

func q1PreflightVerdictFixtures(t *testing.T, selectedLanguage string) []q1PreflightVerdictFixture {
	t.Helper()
	var manifest q1RealCorpusManifest
	manifestBytes, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	corpusRoot := strings.TrimSpace(os.Getenv("GTS_Q1_VERDICT_CORPUS_ROOT"))
	locked, err := os.ReadFile(filepath.Join("..", "grammars", "update_tier1_top50.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []q1PreflightVerdictFixture
	for _, line := range strings.Split(string(locked), "\n") {
		language := strings.TrimSpace(line)
		if language == "" || strings.HasPrefix(language, "#") {
			continue
		}
		if selectedLanguage != "" && language != selectedLanguage {
			continue
		}
		rows := 0
		for _, row := range manifest.Entries {
			if row.Language != language || row.CommittedPath == "" && corpusRoot == "" {
				continue
			}
			path := row.CommittedPath
			id := "top50/" + language
			if corpusRoot != "" {
				path = filepath.Join(corpusRoot, language, row.Role)
				id += "/" + row.Role
			}
			fixture := q1PreflightVerdictFixture{ID: id, Grammar: language, SHA256: row.SHA256, Path: path}
			if corpusRoot == "" {
				fixture.Source, err = os.ReadFile(filepath.Join("..", "internal", "benchfixtures", path))
			} else {
				fixture.Source, err = os.ReadFile(path)
			}
			if err != nil {
				t.Fatalf("read R4 fixture %s: %v", id, err)
			}
			if got := q1PreflightSHA256(fixture.Source); got != fixture.SHA256 {
				t.Fatalf("R4 fixture digest for %s is %s, want %s", id, got, fixture.SHA256)
			}
			fixtures = append(fixtures, fixture)
			rows++
			if corpusRoot == "" {
				break
			}
		}
		if rows == 0 {
			t.Fatalf("R4 fixture missing for top-50 language %q", language)
		}
	}
	for _, cliff := range cliffFixtures {
		if selectedLanguage != "" && cliff.Grammar != selectedLanguage {
			continue
		}
		archivePath := filepath.Join("..", "internal", "benchfixtures", "testdata", "cliffs", cliff.File)
		archive, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			t.Fatal(err)
		}
		source, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		if got := q1PreflightSHA256(source); got != cliff.SHA256 {
			t.Fatalf("R7 fixture digest for %s is %s, want %s", cliff.Name, got, cliff.SHA256)
		}
		fixtures = append(fixtures, q1PreflightVerdictFixture{
			ID: "r7/" + cliff.Name, Grammar: cliff.Grammar, SHA256: cliff.SHA256, Source: source,
		})
	}
	if len(fixtures) == 0 {
		t.Fatalf("no Q1 verdict fixtures for language %q", selectedLanguage)
	}
	return fixtures
}

func q1PreflightSHA256(source []byte) string {
	digest := sha256.Sum256(source)
	return hex.EncodeToString(digest[:])
}

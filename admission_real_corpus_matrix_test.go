//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

type admissionRealCorpusManifest struct {
	Entries []admissionRealCorpusEntry `json:"entries"`
}

type admissionRealCorpusEntry struct {
	Language   string `json:"language"`
	Bucket     string `json:"bucket"`
	Bytes      int    `json:"bytes"`
	OutputPath string `json:"output_path"`
}

type admissionRealCorpusRow struct {
	scorecardRow
	bucket string
	bytes  int
	path   string
}

func TestAdmissionCandidateRealCorpusMatrix(t *testing.T) {
	if os.Getenv("GTS_ADMISSION_REAL_CORPUS") != "1" {
		t.Skip("set GTS_ADMISSION_REAL_CORPUS=1 to run the real-corpus admission matrix")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	manifestPath := strings.TrimSpace(os.Getenv("GTS_ADMISSION_REAL_CORPUS_MANIFEST"))
	if manifestPath == "" {
		manifestPath = filepath.Join("cgo_harness", "corpus_real", "manifest.json")
	}
	manifestSource, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest admissionRealCorpusManifest
	if err := json.Unmarshal(manifestSource, &manifest); err != nil {
		t.Fatal(err)
	}

	languages := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		languages[entry.Name] = entry
	}
	selectedLanguages := admissionRealCorpusFilter("GTS_ADMISSION_REAL_CORPUS_LANGS")
	excludedLanguages := admissionRealCorpusFilter("GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS")
	selectedBuckets := admissionRealCorpusFilter("GTS_ADMISSION_REAL_CORPUS_BUCKETS")
	maxBytes := 0
	if raw := strings.TrimSpace(os.Getenv("GTS_ADMISSION_REAL_CORPUS_MAX_BYTES")); raw != "" {
		maxBytes, err = strconv.Atoi(raw)
		if err != nil || maxBytes <= 0 {
			t.Fatalf("invalid GTS_ADMISSION_REAL_CORPUS_MAX_BYTES %q", raw)
		}
	}

	rows := make([]admissionRealCorpusRow, 0, len(manifest.Entries))
	for _, corpus := range manifest.Entries {
		if len(selectedLanguages) != 0 && !selectedLanguages[corpus.Language] {
			continue
		}
		if excludedLanguages[corpus.Language] {
			continue
		}
		if len(selectedBuckets) != 0 && !selectedBuckets[corpus.Bucket] {
			continue
		}
		if maxBytes != 0 && corpus.Bytes > maxBytes {
			continue
		}
		corpus := corpus
		t.Run(fmt.Sprintf("%s/%s/%s", corpus.Language, corpus.Bucket, filepath.Base(corpus.OutputPath)), func(t *testing.T) {
			entry, ok := languages[corpus.Language]
			if !ok {
				rows = append(rows, admissionRealCorpusRow{
					scorecardRow: scorecardRow{
						name:   corpus.Language,
						status: scorecardError,
						detail: "language is absent from the registry",
					},
					bucket: corpus.Bucket,
					bytes:  corpus.Bytes,
					path:   corpus.OutputPath,
				})
				return
			}
			path := admissionRealCorpusPath(manifestPath, corpus.OutputPath)
			source, err := os.ReadFile(path)
			if err != nil {
				rows = append(rows, admissionRealCorpusRow{
					scorecardRow: scorecardRow{
						name:   corpus.Language,
						status: scorecardError,
						detail: "read corpus: " + err.Error(),
					},
					bucket: corpus.Bucket,
					bytes:  corpus.Bytes,
					path:   corpus.OutputPath,
				})
				return
			}
			if len(source) != corpus.Bytes {
				rows = append(rows, admissionRealCorpusRow{
					scorecardRow: scorecardRow{
						name:   corpus.Language,
						status: scorecardError,
						detail: fmt.Sprintf("bytes=%d, manifest=%d", len(source), corpus.Bytes),
					},
					bucket: corpus.Bucket,
					bytes:  corpus.Bytes,
					path:   corpus.OutputPath,
				})
				return
			}
			row := runAdmissionScorecardSource(entry, source)
			rows = append(rows, admissionRealCorpusRow{
				scorecardRow: row,
				bucket:       corpus.Bucket,
				bytes:        corpus.Bytes,
				path:         corpus.OutputPath,
			})
		})
	}
	if len(rows) == 0 {
		t.Fatal("real-corpus filters selected no files")
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].name != rows[j].name {
			return rows[i].name < rows[j].name
		}
		if rows[i].bucket != rows[j].bucket {
			return rows[i].bucket < rows[j].bucket
		}
		return rows[i].path < rows[j].path
	})

	counts := map[string]int{}
	parseClassCounts := map[string]int{}
	languageCounts := map[string]map[string]int{}
	for _, row := range rows {
		counts[row.status]++
		parseClass := "clean"
		if row.productionHasError {
			parseClass = "error-tree"
		}
		parseClassCounts[row.status+"/"+parseClass]++
		if languageCounts[row.name] == nil {
			languageCounts[row.name] = map[string]int{}
		}
		languageCounts[row.name][row.status]++
		t.Logf(
			"%-9s %-12s %-6s %7d %-10s %-5s %s",
			row.status,
			row.name,
			row.backend,
			row.bytes,
			parseClass,
			row.bucket,
			row.detail,
		)
	}
	t.Logf(
		"--- file summary: PASS=%d DIVERGE=%d FALLBACK=%d SKIP=%d ERROR=%d total=%d ---",
		counts[scorecardPass],
		counts[scorecardDiverge],
		counts[scorecardFallback],
		counts[scorecardSkip],
		counts[scorecardError],
		len(rows),
	)
	t.Logf(
		"--- parse class: FALLBACK clean=%d error-tree=%d; PASS clean=%d error-tree=%d ---",
		parseClassCounts[scorecardFallback+"/clean"],
		parseClassCounts[scorecardFallback+"/error-tree"],
		parseClassCounts[scorecardPass+"/clean"],
		parseClassCounts[scorecardPass+"/error-tree"],
	)

	names := make([]string, 0, len(languageCounts))
	for name := range languageCounts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		count := languageCounts[name]
		t.Logf(
			"MATRIX %-12s PASS=%d DIVERGE=%d FALLBACK=%d SKIP=%d ERROR=%d",
			name,
			count[scorecardPass],
			count[scorecardDiverge],
			count[scorecardFallback],
			count[scorecardSkip],
			count[scorecardError],
		)
	}

	if counts[scorecardDiverge] != 0 || counts[scorecardError] != 0 {
		t.Fatalf(
			"real-corpus matrix has DIVERGE=%d ERROR=%d",
			counts[scorecardDiverge],
			counts[scorecardError],
		)
	}
}

func admissionRealCorpusFilter(name string) map[string]bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func admissionRealCorpusPath(manifestPath, outputPath string) string {
	if filepath.IsAbs(outputPath) {
		return outputPath
	}
	if _, err := os.Stat(outputPath); err == nil {
		return outputPath
	}
	if path := filepath.Join("cgo_harness", outputPath); path != outputPath {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(filepath.Dir(manifestPath), outputPath)
}

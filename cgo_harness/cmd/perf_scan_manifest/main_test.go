package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildManifestPreservesOverlappingPredicates(t *testing.T) {
	dir := t.TempDir()
	scoreboardPath := filepath.Join(dir, "scoreboard.json")
	writeManifestFixture(t, scoreboardPath, scoreboard{
		Schema:      scoreboardSchema,
		GitRevision: "fixture-revision",
		GeneratedAt: "2026-08-11T20:00:00Z",
		Config: scoreboardConfig{
			Reps:                5,
			Warmup:              1,
			FileBudgetMS:        10000,
			MaxFiles:            8,
			Order:               "largest",
			Axes:                []string{"full"},
			CorpusLockSHA256:    strings.Repeat("a", 64),
			CorpusLockLanguages: 2,
		},
		Corpus: scoreboardCorpus{
			LockSHA256:        strings.Repeat("a", 64),
			LockLanguages:     2,
			SelectedLanguages: 2,
		},
		Languages: []scoreboardLanguage{
			{
				Language: "alpha",
				Status:   "ok",
				Verdict:  "cliff>10x",
				Files: []scoreboardFileRow{
					fixtureRow("alpha", "clean-large.txt", "clean", "ok", 2000, 8000, 4),
					fixtureRow("alpha", "clean-tiny.txt", "clean", "ok", 500, 2000, 4),
				},
			},
			{
				Language: "beta",
				Status:   "ok",
				Verdict:  "n/a",
				Files: []scoreboardFileRow{
					fixtureRow("beta", "error.txt", "error", "go_error", 7000, 0, 0),
					fixtureRow("beta", "stopped.txt", "stopped", "go_timeout", 7000, 0, 0),
				},
			},
		},
		HardGate: &scoreboardGate{
			Status:        "fail",
			FilesExpected: 4,
			FilesMeasured: 4,
			Failures: []scoreboardGateFinding{
				{Kind: "ratio", Language: "alpha", Path: "clean-large.txt", Axis: "full", Status: "ok"},
				{Kind: "coverage", Language: "beta", Path: "error.txt", Axis: "full", Status: "go_error"},
				{Kind: "go_stop", Language: "beta", Status: "go_timeout"},
				{Kind: "oracle", Language: "alpha", Status: "inadmissible"},
			},
		},
	})

	doc, err := buildManifest(options{
		ScoreboardPath:      scoreboardPath,
		GeneratedAt:         "2026-08-11T22:20:00Z",
		Axis:                "full",
		TimerThresholdNS:    1000,
		CleanRatioThreshold: 3,
		ExpectedLanguages:   2,
		ExpectedRows:        4,
	})
	if err != nil {
		t.Fatalf("buildManifest returned error: %v", err)
	}

	if doc.Validation.Valid != true {
		t.Fatalf("manifest validation = %+v", doc.Validation)
	}
	if doc.Summary.TotalLanguages != 2 || doc.Summary.TotalRows != 4 {
		t.Fatalf("summary totals = %+v", doc.Summary)
	}
	if doc.Summary.CleanRows != 2 || doc.Summary.ErrorRows != 1 || doc.Summary.StoppedRows != 1 {
		t.Fatalf("semantic counts = %+v", doc.Summary.SemanticClassRows)
	}
	if doc.Summary.MeasuredOKRows != 2 || doc.Summary.NonMeasuredRows != 2 {
		t.Fatalf("timing counts = measured %d non-measured %d", doc.Summary.MeasuredOKRows, doc.Summary.NonMeasuredRows)
	}
	if doc.Summary.RatioNoninterpretableRows != 1 || doc.Summary.RatioInterpretableRows != 1 {
		t.Fatalf("hygiene counts = noninterpretable %d interpretable %d", doc.Summary.RatioNoninterpretableRows, doc.Summary.RatioInterpretableRows)
	}
	if doc.Summary.CleanRowsAbove3x != 2 || doc.Summary.CleanRowsAbove3xSignal != 1 {
		t.Fatalf("clean tail counts = %d/%d", doc.Summary.CleanRowsAbove3x, doc.Summary.CleanRowsAbove3xSignal)
	}
	if doc.Summary.FindingRecords != 4 || doc.Summary.LanguageFindingRecords != 2 {
		t.Fatalf("finding counts = %+v", doc.Summary)
	}
	if !doc.Rows[0].Predicates.HasPathRatioFinding {
		t.Fatalf("row predicates did not retain path ratio finding: %+v", doc.Rows[0].Predicates)
	}
	if !doc.Rows[0].Predicates.HasLanguageOracleFinding {
		t.Fatalf("row predicates did not retain language oracle finding: %+v", doc.Rows[0].Predicates)
	}
	if !doc.Rows[1].Predicates.RawCMedianBelowThreshold || doc.Rows[1].Predicates.RatioInterpretable {
		t.Fatalf("hygiene predicate = %+v", doc.Rows[1].Predicates)
	}
	if !doc.Rows[2].Predicates.HasPathCoverageFinding || !doc.Rows[3].Predicates.HasLanguageGoStopFinding {
		t.Fatalf("overlapping gate predicates were lost: %+v / %+v", doc.Rows[2].Predicates, doc.Rows[3].Predicates)
	}
	if doc.Summary.Hygiene.MeasuredRawCMedianRows != 2 || doc.Summary.Hygiene.MeasuredRawCMedianMissingRows != 0 {
		t.Fatalf("hygiene denominator = %+v", doc.Summary.Hygiene)
	}
	md := renderMarkdown(doc)
	for _, needle := range []string{"Clean rows above 3.0x", "The 4 gate findings", "hygiene threshold"} {
		if !strings.Contains(strings.ToLower(md), strings.ToLower(needle)) {
			t.Fatalf("markdown missing %q:\n%s", needle, md)
		}
	}
}

func TestBuildManifestRejectsExpectedRowMismatch(t *testing.T) {
	dir := t.TempDir()
	scoreboardPath := filepath.Join(dir, "scoreboard.json")
	writeManifestFixture(t, scoreboardPath, scoreboard{
		Schema: scoreboardSchema,
		Corpus: scoreboardCorpus{LockLanguages: 1},
		Languages: []scoreboardLanguage{{
			Language: "alpha",
			Files:    []scoreboardFileRow{fixtureRow("alpha", "one.txt", "clean", "ok", 2000, 8000, 4)},
		}},
		HardGate: &scoreboardGate{FilesExpected: 1},
	})

	_, err := buildManifest(options{
		ScoreboardPath:      scoreboardPath,
		Axis:                "full",
		TimerThresholdNS:    1000,
		CleanRatioThreshold: 3,
		ExpectedLanguages:   1,
		ExpectedRows:        2,
	})
	if err == nil || !strings.Contains(err.Error(), "manifest validation failed") {
		t.Fatalf("buildManifest error = %v, want validation failure", err)
	}
}

func fixtureRow(language, path, class, status string, cMedian, goMedian int64, ratio float64) scoreboardFileRow {
	axis := scoreboardFileAxis{Status: status}
	axis.CMedianNS = int64Pointer(cMedian)
	if goMedian > 0 {
		axis.GoMedianNS = int64Pointer(goMedian)
	}
	if ratio > 0 {
		axis.Ratio = float64Pointer(ratio)
	}
	return scoreboardFileRow{
		Path:  path,
		Bytes: 100,
		Classification: scoreboardClassification{
			Class:  class,
			Reason: "fixture",
		},
		Axes:             map[string]scoreboardFileAxis{"full": axis},
		MeasurementOrder: language + "-order",
	}
}

func writeManifestFixture(t *testing.T, path string, board scoreboard) {
	t.Helper()
	data, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func float64Pointer(value float64) *float64 {
	return &value
}

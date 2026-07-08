package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBudgetAcceptsCurrentFile(t *testing.T) {
	b, err := loadBudget(filepath.Join("..", "..", "perf_scan", "perf_ratio_budgets.json"))
	if err != nil {
		t.Fatalf("load budget: %v", err)
	}
	if findings := validateBudget(b); len(findings) != 0 {
		t.Fatalf("budget findings: %#v", findings)
	}
}

func TestCompareScoreboardPassesWithinBudget(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 1, 1)

	findings := compareScoreboard(b, s, compareOptions{
		RequireAllBudgetLangs: true,
		StrictConfig:          true,
	})
	if len(findings) != 0 {
		t.Fatalf("findings: %#v", findings)
	}
}

func TestCompareScoreboardReportsRegressions(t *testing.T) {
	b := testBudget()
	s := testScoreboard(4.1, 2, 2)

	findings := compareScoreboard(b, s, compareOptions{
		RequireAllBudgetLangs: true,
		StrictConfig:          true,
	})
	got := renderFindingKeys(findings)
	for _, want := range []string{
		"go:full:go_timeouts",
		"go:full:go_errors",
		"go:full:ratio_by_total",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("findings %q missing %q (%#v)", got, want, findings)
		}
	}
}

func TestCompareScoreboardRequiresConfiguredLanguage(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Languages = nil

	findings := compareScoreboard(b, s, compareOptions{
		Languages:             []string{"go"},
		RequireAllBudgetLangs: false,
		StrictConfig:          true,
	})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go::scoreboard") {
		t.Fatalf("findings %q missing missing-language failure", got)
	}
}

func TestMainWritesMarkdownSummary(t *testing.T) {
	dir := t.TempDir()
	budgetPath := filepath.Join(dir, "budget.json")
	scoreboardPath := filepath.Join(dir, "scoreboard.json")
	outPath := filepath.Join(dir, "summary.md")

	writeFile(t, budgetPath, `{
  "schema": "gts-perf-ratio-budgets/v1",
  "measurement_basis": {"reps": 5, "warmup": 1, "file_budget_ms": 10000, "max_files": 8, "order": "largest", "axes": ["full", "noedit"]},
  "languages": {
    "go": {
      "status": "green",
      "full_axis": {"max_timeouts": 0, "max_errors": 0, "max_ratio_by_total": 3},
      "noedit_axis": {"max_timeouts": 0, "max_errors": 0, "max_ratio_by_total": 1}
    }
  }
}`)
	writeFile(t, scoreboardPath, `{
  "schema": "gts-perf-scan/v1",
  "config": {"reps": 5, "warmup": 1, "file_budget_ms": 10000, "max_files": 8, "order": "largest", "axes": ["full", "noedit"]},
  "languages": [{
    "language": "go",
    "status": "ok",
    "axes": {
      "full": {"files_ok": 1, "ratio_by_total": 1.5, "ratio_median_of_files": 1.5, "go_timeouts": 0},
      "noedit": {"files_ok": 1, "ratio_by_total": 0.1, "ratio_median_of_files": 0.1, "go_timeouts": 0}
    },
    "files": [{"path": "x.go", "axes": {"full": {"status": "ok"}, "noedit": {"status": "ok"}}}]
  }]
}`)

	b, err := loadBudget(budgetPath)
	if err != nil {
		t.Fatalf("load budget: %v", err)
	}
	s, err := loadScoreboard(scoreboardPath)
	if err != nil {
		t.Fatalf("load scoreboard: %v", err)
	}
	summary := renderSummary(b, scoreboardPath, compareScoreboard(b, s, compareOptions{
		RequireAllBudgetLangs: true,
		StrictConfig:          true,
	}))
	if err := os.WriteFile(outPath, []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if !strings.Contains(string(data), "outcome: `PASS`") {
		t.Fatalf("summary = %s", data)
	}
}

func testBudget() *budgetFile {
	return &budgetFile{
		Schema: budgetSchema,
		MeasurementBasis: budgetMeasurementBasis{
			Reps:         5,
			Warmup:       1,
			FileBudgetMS: 10000,
			MaxFiles:     8,
			Order:        "largest",
			Axes:         []string{axisFull, axisNoEdit},
		},
		Languages: map[string]budgetLang{
			"go": {
				Status: "green",
				FullAxis: budgetAxis{
					MaxTimeouts:     1,
					MaxErrors:       intPtr(1),
					MaxRatioByTotal: 3,
				},
				NoEditAxis: budgetAxis{
					MaxTimeouts:     1,
					MaxRatioByTotal: 1,
				},
			},
		},
	}
}

func testScoreboard(fullRatio float64, timeouts, errors int) *scoreboardFile {
	files := []scoreboardFileRow{
		{Path: "ok.go", Axes: map[string]scoreboardFileAxis{
			axisFull:   {Status: statusOK},
			axisNoEdit: {Status: statusOK},
		}},
	}
	for i := 0; i < timeouts; i++ {
		files = append(files, scoreboardFileRow{Path: "timeout.go", Axes: map[string]scoreboardFileAxis{
			axisFull:   {Status: "go_timeout"},
			axisNoEdit: {Status: "go_timeout"},
		}})
	}
	for i := 0; i < errors; i++ {
		files = append(files, scoreboardFileRow{Path: "truncated.go", Axes: map[string]scoreboardFileAxis{
			axisFull:   {Status: "go_error"},
			axisNoEdit: {Status: statusOK},
		}})
	}
	return &scoreboardFile{
		Schema: scoreboardSchema,
		Config: scoreboardConfig{
			Reps:         5,
			Warmup:       1,
			FileBudgetMS: 10000,
			MaxFiles:     8,
			Order:        "largest",
			Axes:         []string{axisFull, axisNoEdit},
		},
		Languages: []scoreboardLang{{
			Language: "go",
			Status:   statusOK,
			Axes: map[string]scoreboardAxis{
				axisFull: {
					FilesOK:            1,
					RatioByTotal:       fullRatio,
					RatioMedianOfFiles: fullRatio,
					GoTimeouts:         timeouts,
				},
				axisNoEdit: {
					FilesOK:            1,
					RatioByTotal:       0.1,
					RatioMedianOfFiles: 0.1,
					GoTimeouts:         timeouts,
				},
			},
			Files: files,
		}},
	}
}

func renderFindingKeys(findings []evalFinding) string {
	var parts []string
	for _, f := range findings {
		parts = append(parts, f.Language+":"+f.Axis+":"+f.Metric)
	}
	return strings.Join(parts, "\n")
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func intPtr(n int) *int {
	return &n
}

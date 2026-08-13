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
	s := testScoreboard(2.5, 0, 0)

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

func TestCompareScoreboardReportsMedianRatioRegression(t *testing.T) {
	b := testBudget()
	lang := b.Languages["go"]
	lang.FullAxis.MaxRatioMedianOfFiles = 2
	b.Languages["go"] = lang
	s := testScoreboard(1.5, 0, 0)
	s.Languages[0].Axes[axisFull] = scoreboardAxis{
		FilesOK:            1,
		RatioByTotal:       1.5,
		RatioMedianOfFiles: 2.5,
	}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:ratio_median_of_files") {
		t.Fatalf("findings %q missing median ratio failure (%#v)", got, findings)
	}
}

func TestCompareScoreboardEnforcesExactPerFileFullRatio(t *testing.T) {
	b := testBudget()
	s := testScoreboard(1.5, 0, 0)
	full := s.Languages[0].Files[0].Axes[axisFull]
	full.GoMedianNs = 10_001
	full.CMedianNs = 1_000
	full.Ratio = 10.001
	s.Languages[0].Files[0].Axes[axisFull] = full

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:hard_full_ratio") {
		t.Fatalf("findings %q missing per-file >10x failure (%#v)", got, findings)
	}

	full.GoMedianNs = 10_000
	full.Ratio = 10
	s.Languages[0].Files[0].Axes[axisFull] = full
	findings = compareScoreboard(b, s, compareOptions{StrictConfig: true})
	if got := renderFindingKeys(findings); strings.Contains(got, "hard_full_ratio") {
		t.Fatalf("exactly 10x must pass the hard ratio boundary: %q (%#v)", got, findings)
	}
}

func TestCompareScoreboardRejectsStructuredParserBudgetStop(t *testing.T) {
	b := testBudget()
	s := testScoreboard(1.5, 0, 0)
	full := s.Languages[0].Files[0].Axes[axisFull]
	full.Status = "go_budget_stop"
	full.Stop = &scoreboardStop{Class: "parser_budget", Reason: "memory_budget", Implementation: "go"}
	s.Languages[0].Files[0].Axes[axisFull] = full

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:hard_go_stop") {
		t.Fatalf("findings %q missing structured parser-budget failure (%#v)", got, findings)
	}
}

func TestCompareScoreboardReportsStrictConfigMismatch(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Config.Reps = 7

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "::config.reps") {
		t.Fatalf("findings %q missing config reps failure (%#v)", got, findings)
	}
}

func TestLoadScoreboardReadsRuntimeEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scoreboard.json")
	writeFile(t, path, `{"config":{"runtime_evidence":true}}`)

	s, err := loadScoreboard(path)
	if err != nil {
		t.Fatalf("load scoreboard: %v", err)
	}
	if !s.Config.RuntimeEvidence {
		t.Fatal("runtime evidence was not decoded")
	}
}

func TestCompareScoreboardRejectsRuntimeEvidenceLane(t *testing.T) {
	b := testBudget()
	tests := []struct {
		name         string
		hardGateOnly bool
	}{
		{name: "historical budget"},
		{name: "current hard gate", hardGateOnly: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := testScoreboard(2.5, 0, 0)
			if test.hardGateOnly {
				makeV2FullBoard(s)
			}
			s.Config.RuntimeEvidence = true

			findings := compareScoreboard(b, s, compareOptions{
				HardGateOnly: test.hardGateOnly,
			})
			got := renderFindingKeys(findings)
			if !strings.Contains(got, "::config.runtime_evidence") {
				t.Fatalf("findings %q accepted a runtime-evidence lane (%#v)", got, findings)
			}
		})
	}
}

func TestCompareScoreboardKeepsLegacyRatchetComparisonWithoutHardGate(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Config.HardGate = false
	s.Gate = nil
	// Legacy v1 rows did not need the exact per-file timing fields now used by
	// the hard gate. Aggregate ratchets must remain comparable when strict
	// config is explicitly disabled for historical analysis.
	s.Languages[0].Files[0].Axes[axisFull] = scoreboardFileAxis{Status: statusOK}
	s.Languages[0].Files[0].Axes[axisNoEdit] = scoreboardFileAxis{Status: statusOK}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: false})
	if len(findings) != 0 {
		t.Fatalf("legacy ratchet comparison unexpectedly applied hard semantics: %#v", findings)
	}
}

func TestCompareScoreboardHardGateOnlyUsesUniversalUnexcludedBasis(t *testing.T) {
	b := testBudget()
	b.MeasurementBasis.ExcludePaths = []string{"groovy/heldout.groovy"}
	lang := b.Languages["go"]
	lang.FullAxis.MaxRatioByTotal = 1
	b.Languages["go"] = lang
	s := testScoreboard(2.5, 0, 0)
	makeV2FullBoard(s)

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true, HardGateOnly: true})
	if len(findings) != 0 {
		t.Fatalf("hard-gate-only comparison applied historical basis or ratchet: %#v", findings)
	}

	s.Config.ExcludePaths = []string{"groovy/heldout.groovy"}
	findings = compareScoreboard(b, s, compareOptions{StrictConfig: true, HardGateOnly: true})
	if got := renderFindingKeys(findings); !strings.Contains(got, "::config.exclude_paths") {
		t.Fatalf("hard-gate-only comparison accepted an exclusion: %q (%#v)", got, findings)
	}
}

func TestCompareScoreboardKeepsV1HistoricalAndV2HardGateModesSeparate(t *testing.T) {
	b := testBudget()

	legacy := testScoreboard(1.5, 0, 0)
	findings := compareScoreboard(b, legacy, compareOptions{StrictConfig: true, HardGateOnly: true})
	if got := renderFindingKeys(findings); !strings.Contains(got, "::scoreboard.schema_mode") {
		t.Fatalf("v1 scoreboard established a current hard-gate verdict: %q (%#v)", got, findings)
	}

	current := testScoreboard(1.5, 0, 0)
	makeV2FullBoard(current)
	findings = compareScoreboard(b, current, compareOptions{StrictConfig: true})
	if got := renderFindingKeys(findings); !strings.Contains(got, "::scoreboard.schema_mode") {
		t.Fatalf("v2 scoreboard entered historical aggregate comparison: %q (%#v)", got, findings)
	}

	current.Schema = "gts-perf-scan/v99"
	findings = compareScoreboard(b, current, compareOptions{StrictConfig: true, HardGateOnly: true})
	if got := renderFindingKeys(findings); !strings.Contains(got, "::scoreboard.schema") {
		t.Fatalf("unknown scoreboard schema was accepted: %q (%#v)", got, findings)
	}
}

func TestCompareScoreboardV2HardGateRequiresFullOnlyAxis(t *testing.T) {
	b := testBudget()
	s := testScoreboard(1.5, 0, 0)
	makeV2FullBoard(s)
	s.Config.Axes = []string{axisFull, axisNoEdit}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true, HardGateOnly: true})
	if got := renderFindingKeys(findings); !strings.Contains(got, ":full:config.axes") {
		t.Fatalf("v2 publication admitted a legacy axis: %q (%#v)", got, findings)
	}
}

func TestCompareScoreboardReportsExcludePathConfigMismatch(t *testing.T) {
	b := testBudget()
	b.MeasurementBasis.ExcludePaths = []string{"d/compiler/src/dmd/expressionsem.d"}
	s := testScoreboard(2.5, 0, 0)

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "::config.exclude_paths") {
		t.Fatalf("findings %q missing config exclude_paths failure (%#v)", got, findings)
	}

	s.Config.ExcludePaths = []string{"d/compiler/src/dmd/expressionsem.d"}
	findings = compareScoreboard(b, s, compareOptions{StrictConfig: true})
	if len(findings) != 0 {
		t.Fatalf("matching exclude_paths should pass: %#v", findings)
	}

	b.MeasurementBasis.ExcludePaths = []string{"groovy/subprojects/"}
	s.Config.ExcludePaths = []string{"groovy/subprojects"}
	findings = compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got = renderFindingKeys(findings)
	if !strings.Contains(got, "::config.exclude_paths") {
		t.Fatalf("trailing slash should remain semantically distinct, findings %q (%#v)", got, findings)
	}
}

func TestValidateBudgetReportsMalformedExcludePathGlob(t *testing.T) {
	b := testBudget()
	b.MeasurementBasis.ExcludePaths = []string{"bad/[glob"}

	findings := validateBudget(b)
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "::measurement_basis.exclude_paths") {
		t.Fatalf("findings %q missing malformed exclude path glob (%#v)", got, findings)
	}
}

func TestCompareScoreboardReportsCReferenceFailure(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Languages[0].Files[0].Axes[axisFull] = scoreboardFileAxis{Status: "c_timeout"}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:c_reference_failures") {
		t.Fatalf("findings %q missing C reference failure (%#v)", got, findings)
	}
}

func TestCompareScoreboardRejectsMissingFullRatioDespiteHistoricalCFailureBudget(t *testing.T) {
	b := testBudget()
	lang := b.Languages["go"]
	lang.FullAxis.MaxCReferenceFailures = intPtr(1)
	b.Languages["go"] = lang

	s := testScoreboard(2.5, 0, 0)
	s.Languages[0].Files[0].Axes[axisFull] = scoreboardFileAxis{Status: "c_timeout"}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:hard_full_measurement") {
		t.Fatalf("hard gate must reject a missing full ratio despite the historical allowance: %q (%#v)", got, findings)
	}
	if strings.Contains(got, "go:full:c_reference_failures") {
		t.Fatalf("historical C-reference allowance should still suppress the ratchet finding: %q (%#v)", got, findings)
	}
}

func TestCompareScoreboardKeepsCorrectnessFailureSeparateFromRatioGate(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Languages[0].Files[0].Axes[axisFull] = scoreboardFileAxis{Status: "go_truncated"}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:hard_full_measurement") {
		t.Fatalf("findings %q missing fail-closed full measurement (%#v)", got, findings)
	}

	lang := b.Languages["go"]
	lang.FullAxis.MaxErrors = intPtr(0)
	b.Languages["go"] = lang
	findings = compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got = renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:go_errors") {
		t.Fatalf("findings %q missing go_truncated error accounting (%#v)", got, findings)
	}
}

func TestCompareScoreboardRejectsTimeoutsDespiteHistoricalBudget(t *testing.T) {
	b := testBudget()
	lang := b.Languages["go"]
	lang.FullAxis.MaxTimeouts = 2
	lang.NoEditAxis.MaxTimeouts = 2
	b.Languages["go"] = lang

	s := testScoreboard(0, 2, 0)
	s.Languages[0].Axes[axisFull] = scoreboardAxis{GoTimeouts: 2}
	s.Languages[0].Axes[axisNoEdit] = scoreboardAxis{GoTimeouts: 2}
	s.Languages[0].Files = []scoreboardFileRow{
		{Path: "timeout-a.go", Axes: map[string]scoreboardFileAxis{
			axisFull:   {Status: "go_timeout"},
			axisNoEdit: {Status: "go_timeout"},
		}},
		{Path: "timeout-b.go", Axes: map[string]scoreboardFileAxis{
			axisFull:   {Status: "go_timeout"},
			axisNoEdit: {Status: "go_timeout"},
		}},
	}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "go:full:hard_go_stop") || !strings.Contains(got, "go:noedit:hard_go_stop") {
		t.Fatalf("hard gate must reject timeout allowances, got %q (%#v)", got, findings)
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

func TestCompareScoreboardReportsUnknownBudgetLanguage(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Languages = append(s.Languages, scoreboardLang{
		Language:      "unknown",
		Status:        statusOK,
		FilesSelected: 1,
		FilesMeasured: 1,
		Axes: map[string]scoreboardAxis{
			axisFull:   {FilesOK: 1, RatioByTotal: 1},
			axisNoEdit: {FilesOK: 1, RatioByTotal: 1},
		},
		Files: []scoreboardFileRow{{Path: "x", Axes: map[string]scoreboardFileAxis{
			axisFull:   {Status: statusOK},
			axisNoEdit: {Status: statusOK},
		}}},
	})

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	got := renderFindingKeys(findings)
	if !strings.Contains(got, "unknown::budget") {
		t.Fatalf("findings %q missing unknown budget language failure (%#v)", got, findings)
	}
}

func TestCompareScoreboardAllowsUnbudgetedLanguageInAuthenticatedFleetGate(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Config.RequireFleet = true
	s.Corpus = scoreboardCorpus{
		LockSHA256:        strings.Repeat("a", 64),
		LockLanguages:     2,
		SelectedLanguages: 2,
	}
	s.Languages = append(s.Languages, scoreboardLang{
		Language:      "unbudgeted",
		Status:        statusOK,
		FilesSelected: 1,
		FilesMeasured: 1,
		Axes: map[string]scoreboardAxis{
			axisFull:   {FilesOK: 1, RatioByTotal: 1},
			axisNoEdit: {FilesOK: 1, RatioByTotal: 0.1},
		},
		Files: []scoreboardFileRow{{Path: "x", Axes: map[string]scoreboardFileAxis{
			axisFull:   {Status: statusOK, GoMedianNs: 1000, CMedianNs: 1000, Ratio: 1},
			axisNoEdit: {Status: statusOK, GoMedianNs: 100, CMedianNs: 1000, Ratio: 0.1},
		}}},
	})
	s.Gate.FilesExpected = 2
	s.Gate.FilesMeasured = 2
	s.Gate.FullFilesEvaluated = 2

	findings := compareScoreboard(b, s, compareOptions{RequireAllBudgetLangs: true, StrictConfig: true})
	if got := renderFindingKeys(findings); strings.Contains(got, "unbudgeted::budget") {
		t.Fatalf("authenticated fleet hard gate must admit languages without historical ratchets: %q (%#v)", got, findings)
	}
}

func TestCompareScoreboardRejectsIncompleteFleetCoverage(t *testing.T) {
	b := testBudget()
	s := testScoreboard(2.5, 0, 0)
	s.Config.RequireFleet = true
	s.Corpus = scoreboardCorpus{LockLanguages: 2, SelectedLanguages: 1}

	findings := compareScoreboard(b, s, compareOptions{StrictConfig: true})
	if got := renderFindingKeys(findings); !strings.Contains(got, "::hard_fleet_coverage") {
		t.Fatalf("findings %q missing fail-closed fleet coverage failure (%#v)", got, findings)
	}
}

func TestMainWritesMarkdownSummary(t *testing.T) {
	dir := t.TempDir()
	budgetPath := filepath.Join(dir, "budget.json")
	scoreboardPath := filepath.Join(dir, "scoreboard.json")
	outPath := filepath.Join(dir, "summary.md")

	writeFile(t, budgetPath, `{
  "schema": "gts-perf-ratio-budgets/v1",
  "measurement_basis": {"reps": 5, "warmup": 1, "file_budget_ms": 10000, "max_files": 8, "order": "largest", "axes": ["full", "noedit"], "hard_max_full_parse_ratio": 10, "fast_full_parse_ratio": 0.1, "corpus_lock_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
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
  "config": {"reps": 5, "warmup": 1, "file_budget_ms": 10000, "max_files": 8, "order": "largest", "axes": ["full", "noedit"], "hard_gate": true, "corpus_lock_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  "corpus_coverage": {"lock_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "lock_languages": 1, "selected_languages": 1},
  "hard_gate": {"status": "pass", "max_full_parse_ratio": 10, "fast_full_parse_ratio": 0.1, "files_expected": 1, "files_measured": 1, "full_files_evaluated": 1},
  "languages": [{
    "language": "go",
    "status": "ok",
    "files_selected": 1,
    "files_measured": 1,
    "axes": {
      "full": {"files_ok": 1, "ratio_by_total": 1.5, "ratio_median_of_files": 1.5, "go_timeouts": 0},
      "noedit": {"files_ok": 1, "ratio_by_total": 0.1, "ratio_median_of_files": 0.1, "go_timeouts": 0}
    },
    "files": [{"path": "x.go", "axes": {"full": {"status": "ok", "go_median_ns": 1500, "c_median_ns": 1000, "ratio": 1.5}, "noedit": {"status": "ok", "go_median_ns": 100, "c_median_ns": 1000, "ratio": 0.1}}}]
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
			Reps:                  5,
			Warmup:                1,
			FileBudgetMS:          10000,
			MaxFiles:              8,
			Order:                 "largest",
			Axes:                  []string{axisFull, axisNoEdit},
			HardMaxFullParseRatio: hardMaxFullParseRatio,
			FastFullParseRatio:    fastFullParseRatio,
			CorpusLockSHA256:      strings.Repeat("a", 64),
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
			axisFull:   {Status: statusOK, GoMedianNs: int64(fullRatio * 1000), CMedianNs: 1000, Ratio: fullRatio},
			axisNoEdit: {Status: statusOK, GoMedianNs: 100, CMedianNs: 1000, Ratio: 0.1},
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
		Schema: scoreboardSchemaV1,
		Config: scoreboardConfig{
			Reps:             5,
			Warmup:           1,
			FileBudgetMS:     10000,
			MaxFiles:         8,
			Order:            "largest",
			Axes:             []string{axisFull, axisNoEdit},
			HardGate:         true,
			CorpusLockSHA256: strings.Repeat("a", 64),
		},
		Corpus: scoreboardCorpus{
			LockSHA256:        strings.Repeat("a", 64),
			LockLanguages:     1,
			SelectedLanguages: 1,
		},
		Languages: []scoreboardLang{{
			Language:      "go",
			Status:        statusOK,
			FilesSelected: len(files),
			FilesMeasured: len(files),
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
		Gate: &scoreboardGate{
			Status:             "pass",
			MaxFullParseRatio:  hardMaxFullParseRatio,
			FastFullParseRatio: fastFullParseRatio,
			FilesExpected:      len(files),
			FilesMeasured:      len(files),
			FullFilesEvaluated: len(files),
		},
	}
}

func makeV2FullBoard(s *scoreboardFile) {
	s.Schema = scoreboardSchemaV2
	s.Config.Axes = []string{axisFull}
	for i := range s.Languages {
		delete(s.Languages[i].Axes, axisNoEdit)
		for j := range s.Languages[i].Files {
			delete(s.Languages[i].Files[j].Axes, axisNoEdit)
		}
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

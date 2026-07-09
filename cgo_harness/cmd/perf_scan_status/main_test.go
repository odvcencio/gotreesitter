package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildStatusFromTrackedInputs(t *testing.T) {
	dir := t.TempDir()
	budgetPath := filepath.Join(dir, "budget.json")
	fleetPath := filepath.Join(dir, "exts.tsv")
	writeFile(t, fleetPath, "go\t.go\npython\t.py\ntypescript\t.ts\ngroovy\t.groovy\n")
	writeJSON(t, budgetPath, map[string]any{
		"schema":       budgetSchema,
		"generated_at": "2026-07-09T01:17:47Z",
		"generated_by": "test generator",
		"measurement_basis": map[string]any{
			"reps": 5, "warmup": 1, "file_budget_ms": 10000,
			"max_files": 8, "order": "largest", "axes": []string{"full", "noedit"},
		},
		"_seed_sources": map[string]string{
			"wave3_batch1": "first batch",
		},
		"known_budget_class_gaps": map[string]any{
			"groovy_gap": map[string]any{
				"file": "groovy/large.groovy", "action": "dedicated RCA",
			},
		},
		"languages": map[string]any{
			"go": map[string]any{
				"status":         "green_with_caveat",
				"measured_today": true,
				"full_axis":      map[string]any{"max_timeouts": 0, "max_ratio_by_total": 2.0},
				"noedit_axis":    map[string]any{"max_timeouts": 0, "max_ratio_by_total": 1.0},
			},
			"python": map[string]any{
				"status":         "wave2b_pending",
				"wave2b_pending": true,
				"full_axis":      map[string]any{"max_timeouts": 1, "max_ratio_by_total": 12.0},
				"noedit_axis":    map[string]any{"max_timeouts": 1, "max_ratio_by_total": 1.0},
			},
			"typescript": map[string]any{
				"status":         "green_with_caveat",
				"measured_today": "partial oracle spot-check only",
				"full_axis":      map[string]any{"max_timeouts": 0, "max_ratio_by_total": 2.0},
				"noedit_axis":    map[string]any{"max_timeouts": 0, "max_ratio_by_total": 1.0},
			},
		},
	})

	doc, err := buildStatus(options{
		BudgetPath:  budgetPath,
		FleetPath:   fleetPath,
		GeneratedAt: "2026-07-09T05:00:00Z",
	})
	if err != nil {
		t.Fatalf("buildStatus returned error: %v", err)
	}
	if doc.Coverage.FleetLanguages != 4 {
		t.Fatalf("fleet languages = %d, want 4", doc.Coverage.FleetLanguages)
	}
	if doc.Coverage.BudgetedLanguages != 3 {
		t.Fatalf("budgeted languages = %d, want 3", doc.Coverage.BudgetedLanguages)
	}
	if got := strings.Join(doc.HeldOutLanguages, ","); got != "groovy" {
		t.Fatalf("held out languages = %q, want groovy", got)
	}
	if doc.BudgetStatusCounts["wave2b_pending"] != 1 {
		t.Fatalf("wave2b status count = %d, want 1", doc.BudgetStatusCounts["wave2b_pending"])
	}
	if doc.Coverage.MeasuredTodayBudgets != 1 {
		t.Fatalf("measured today = %d, want 1", doc.Coverage.MeasuredTodayBudgets)
	}
	if doc.Coverage.PartialMeasuredTodayBudgets != 1 {
		t.Fatalf("partial measured today = %d, want 1", doc.Coverage.PartialMeasuredTodayBudgets)
	}
	md := renderMarkdown(doc)
	for _, needle := range []string{"budgeted languages", "Held out of the ratchet", "groovy_gap", "wave3_batch1"} {
		if !strings.Contains(md, needle) {
			t.Fatalf("markdown missing %q:\n%s", needle, md)
		}
	}
}

func TestBuildStatusWithScoreboards(t *testing.T) {
	dir := t.TempDir()
	budgetPath := filepath.Join(dir, "budget.json")
	fleetPath := filepath.Join(dir, "exts.tsv")
	scoreboardPath := filepath.Join(dir, "scoreboard.json")
	writeFile(t, fleetPath, "go\t.go\ngroovy\t.groovy\n")
	writeJSON(t, budgetPath, map[string]any{
		"schema":       budgetSchema,
		"generated_at": "2026-07-09T01:17:47Z",
		"generated_by": "test generator",
		"measurement_basis": map[string]any{
			"reps": 5, "warmup": 1, "file_budget_ms": 10000,
			"max_files": 8, "order": "largest", "axes": []string{"full", "noedit"},
		},
		"known_budget_class_gaps": map[string]any{"groovy_gap": map[string]any{"action": "RCA"}},
		"languages": map[string]any{
			"go": map[string]any{
				"status":      "green",
				"full_axis":   map[string]any{"max_timeouts": 0, "max_ratio_by_total": 2.0},
				"noedit_axis": map[string]any{"max_timeouts": 0, "max_ratio_by_total": 1.0},
			},
		},
	})
	writeJSON(t, scoreboardPath, map[string]any{
		"schema":       scoreboardSchema,
		"generated_at": "2026-07-09T04:34:09Z",
		"config": map[string]any{
			"reps": 5, "warmup": 1, "file_budget_ms": 10000, "lang_timeout_ms": 900000,
			"max_files": 8, "order": "largest", "axes": []string{"full", "noedit"},
			"contended": true, "contended_note": "test contention",
		},
		"host":             map[string]any{"loadavg_start": "7.00 5.00 4.00", "loadavg_end": "6.00 5.00 4.00"},
		"summary_verdicts": map[string]int{"<=2x": 1, "cliff>10x": 1},
		"languages": []any{
			map[string]any{
				"language": "go", "status": "ok", "files_selected": 8, "files_measured": 8,
				"bytes_measured": 1234, "verdict": "<=2x",
				"axes": map[string]any{
					"full":   map[string]any{"files_ok": 8, "go_timeouts": 0, "cliffs": 0, "ratio_by_total": 1.5, "ratio_median_of_files": 1.2, "verdict": "<=2x"},
					"noedit": map[string]any{"files_ok": 8, "go_timeouts": 0, "cliffs": 0, "ratio_by_total": 0.1, "ratio_median_of_files": 0.1, "verdict": "<=1.2x"},
				},
			},
			map[string]any{
				"language": "groovy", "status": "error", "files_selected": 8, "files_measured": 0,
				"bytes_measured": 0, "verdict": "cliff>10x",
				"axes": map[string]any{},
			},
		},
	})

	doc, err := buildStatus(options{
		BudgetPath:         budgetPath,
		FleetPath:          fleetPath,
		ScoreboardPatterns: scoreboardPath,
		GeneratedAt:        "2026-07-09T05:00:00Z",
	})
	if err != nil {
		t.Fatalf("buildStatus returned error: %v", err)
	}
	if doc.ScoreboardCoverage == nil {
		t.Fatal("scoreboard coverage was nil")
	}
	if doc.ScoreboardCoverage.MeasuredLanguages != 2 {
		t.Fatalf("measured languages = %d, want 2", doc.ScoreboardCoverage.MeasuredLanguages)
	}
	if doc.ScoreboardCoverage.MeasuredBudgetLanguages != 1 {
		t.Fatalf("measured budget languages = %d, want 1", doc.ScoreboardCoverage.MeasuredBudgetLanguages)
	}
	if got := strings.Join(doc.ScoreboardCoverage.MeasuredHeldOutLanguages, ","); got != "groovy" {
		t.Fatalf("measured held-out languages = %q, want groovy", got)
	}
	if doc.ScoreboardCoverage.ContendedScoreboards != 1 {
		t.Fatalf("contended scoreboards = %d, want 1", doc.ScoreboardCoverage.ContendedScoreboards)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	writeFile(t, path, string(data))
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

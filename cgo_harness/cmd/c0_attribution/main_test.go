package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildReportUsesF0SignalAndKeepsCohortsSeparate(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	sealedPath := filepath.Join(dir, "sealed.json")
	rows := make([]map[string]any, 1435)
	rows[0] = map[string]any{"language": "a", "path": "a", "semantic_class": "clean", "axis": map[string]any{"status": "ok", "go_median_ns": 3000, "c_median_ns": 1000, "ratio": 3}, "predicates": map[string]any{"ratio_interpretable": true}}
	rows[1] = map[string]any{"language": "a", "path": "b", "semantic_class": "error", "axis": map[string]any{"status": "ok", "go_median_ns": 6000, "c_median_ns": 2000, "ratio": 3}, "predicates": map[string]any{"ratio_interpretable": true}}
	for i := 2; i < len(rows); i++ {
		rows[i] = map[string]any{"language": "a", "path": "stopped", "semantic_class": "stopped", "axis": map[string]any{"status": "go_timeout", "go_median_ns": 0}, "predicates": map[string]any{"ratio_interpretable": false}}
	}
	manifest, err := json.Marshal(map[string]any{"schema": "gts-perf-fleet-manifest/v1", "axis": "full", "validation": map[string]any{"valid": true}, "source": map[string]any{"scoreboard_sha256": "score"}, "policy": map[string]any{"timer_threshold_ns": 1000}, "summary": map[string]any{"total_rows": 1435, "hygiene_denominator": map[string]any{"ratio_eligible_rows": 2}, "finding_records_by_kind": map[string]int{"coverage": 110}}, "rows": rows})
	if err != nil {
		t.Fatal(err)
	}
	sealed := `{"schema":"gts-c0e-sealed-board/v1","epoch":"e","commit":"c","compact":[{"fixture":"a","ratio":1},{"fixture":"b","ratio":1},{"fixture":"c","ratio":1},{"fixture":"d","ratio":1}],"production":[],"compact_geomean":1,"noise":{"receipt_path":"n","receipt_sha256":"h","host_class":"local","p95_pct_of_median_by_fixture":{}}}`
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealedPath, []byte(sealed), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := buildReport(options{ManifestPath: manifestPath, SealedPath: sealedPath, GeneratedAt: "fixed"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.C0f.SignalRows != 2 || doc.C0f.Classes["error"].Rows != 1 {
		t.Fatalf("unexpected signal counts: %#v", doc.C0f)
	}
	if doc.C0f.Cohorts["alternative_lifetime"].Observed {
		t.Fatal("unobserved cohort marked observed")
	}
	markdown := renderMarkdown(doc)
	if !strings.Contains(markdown, "partially gated") {
		t.Fatal("markdown omitted status")
	}
	if !strings.HasSuffix(markdown, "\n") {
		t.Fatal("markdown lacks a final newline")
	}
	if strings.HasSuffix(markdown, "\n\n") {
		t.Fatal("markdown has an extra final newline")
	}
}

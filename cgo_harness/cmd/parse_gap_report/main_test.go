//go:build cgo && treesitter_c_parity

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestFormatHighlightCaptureMismatchIncludesCountsAndFirstCaptures(t *testing.T) {
	onlyGo := []highlightCapture{
		{Name: "keyword", StartByte: 10, EndByte: 15},
		{Name: "type", StartByte: 20, EndByte: 24},
	}
	onlyC := []highlightCapture{
		{Name: "function", StartByte: 30, EndByte: 38},
	}

	got := formatHighlightCaptureMismatch(onlyGo, onlyC)
	want := "capture mismatch go_only=2 c_only=1 first_go_only=@keyword[10:15] first_c_only=@function[30:38]"
	if got != want {
		t.Fatalf("mismatch detail = %q, want %q", got, want)
	}
}

func TestDiffHighlightCaptures(t *testing.T) {
	goCaps := []highlightCapture{
		{Name: "keyword", StartByte: 0, EndByte: 5},
		{Name: "type", StartByte: 6, EndByte: 9},
	}
	cCaps := []highlightCapture{
		{Name: "keyword", StartByte: 0, EndByte: 5},
		{Name: "function", StartByte: 6, EndByte: 9},
	}

	onlyGo, onlyC := diffHighlightCaptures(goCaps, cCaps)
	if len(onlyGo) != 1 || onlyGo[0] != goCaps[1] {
		t.Fatalf("onlyGo = %#v, want %#v", onlyGo, []highlightCapture{goCaps[1]})
	}
	if len(onlyC) != 1 || onlyC[0] != cCaps[1] {
		t.Fatalf("onlyC = %#v, want %#v", onlyC, []highlightCapture{cCaps[1]})
	}
}

func TestCollectSamplesUsesRequestedLanguageOrder(t *testing.T) {
	root := t.TempDir()
	rubyPath := filepath.Join(root, "a.rb")
	tsPath := filepath.Join(root, "a.ts")
	if err := os.WriteFile(rubyPath, []byte("puts 'x'\n"), 0o644); err != nil {
		t.Fatalf("write ruby sample: %v", err)
	}
	if err := os.WriteFile(tsPath, []byte("const x = 1;\n"), 0o644); err != nil {
		t.Fatalf("write typescript sample: %v", err)
	}

	manifest := corpusManifest{Sets: []corpusSetSpec{
		{Name: "ruby", Language: "ruby", Files: []string{rubyPath}},
		{Name: "typescript", Language: "typescript", Files: []string{tsPath}},
	}}
	selected := map[string]struct{}{"ruby": {}, "typescript": {}}

	samples, err := collectSamples(root, manifest, selected, []string{"typescript", "ruby"})
	if err != nil {
		t.Fatalf("collectSamples: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples length = %d, want 2", len(samples))
	}
	if got, want := samples[0].Language, "typescript"; got != want {
		t.Fatalf("first language = %q, want %q", got, want)
	}
}

func TestRenderSummaryUsesRequestedLanguageOrder(t *testing.T) {
	rows := []reportRow{
		{Language: "ruby", Sample: "ruby.rb", Mode: "cgo_full", MedianNS: 10},
		{Language: "ruby", Sample: "ruby.rb", Mode: "go_full", MedianNS: 10},
		{Language: "typescript", Sample: "a.ts", Mode: "cgo_full", MedianNS: 10},
		{Language: "typescript", Sample: "a.ts", Mode: "go_full", MedianNS: 10},
		{Language: "php", Sample: "a.php", Mode: "cgo_full", MedianNS: 10},
		{Language: "php", Sample: "a.php", Mode: "go_full", MedianNS: 10},
	}

	summary := renderSummary(rows, []string{"typescript", "php", "ruby"})
	ts := strings.Index(summary, "| typescript |")
	php := strings.Index(summary, "| php |")
	ruby := strings.Index(summary, "| ruby |")
	if ts < 0 || php < 0 || ruby < 0 {
		t.Fatalf("summary missing rows:\n%s", summary)
	}
	if !(ts < php && php < ruby) {
		t.Fatalf("summary order mismatch:\n%s", summary)
	}
}

func TestCleanErrorFileCountsSplitsUniqueSamplesByCleanliness(t *testing.T) {
	rows := []reportRow{
		{Language: "kdl", Sample: "clean1.kdl", Mode: "cgo_full", Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "clean1.kdl", Mode: "go_full", Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "clean2.kdl", Mode: "cgo_full", Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "clean2.kdl", Mode: "go_full", Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "err1.kdl", Mode: "cgo_full", Parity: paritySummary{Clean: false}},
		{Language: "kdl", Sample: "err1.kdl", Mode: "go_full", Parity: paritySummary{Clean: false}},
		{Language: "other", Sample: "x.other", Mode: "go_full", Parity: paritySummary{Clean: false}},
	}

	clean, errorCount := cleanErrorFileCounts(rows, "kdl")
	if clean != 2 {
		t.Fatalf("clean = %d, want 2", clean)
	}
	if errorCount != 1 {
		t.Fatalf("errorCount = %d, want 1", errorCount)
	}
}

func TestFileShareStringFormatsPercentageOrNA(t *testing.T) {
	if got, want := fileShareString(0, 0), "n/a"; got != want {
		t.Fatalf("fileShareString(0,0) = %q, want %q", got, want)
	}
	if got, want := fileShareString(0, 3), "0.0%"; got != want {
		t.Fatalf("fileShareString(0,3) = %q, want %q", got, want)
	}
	if got, want := fileShareString(1, 3), "33.3%"; got != want {
		t.Fatalf("fileShareString(1,3) = %q, want %q", got, want)
	}
	if got, want := fileShareString(3, 3), "100.0%"; got != want {
		t.Fatalf("fileShareString(3,3) = %q, want %q", got, want)
	}
}

func TestRenderSummaryReportsCleanAndErrorRatiosSeparately(t *testing.T) {
	rows := []reportRow{
		{Language: "kdl", Sample: "clean1.kdl", Mode: "cgo_full", MedianNS: 10, Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "clean1.kdl", Mode: "go_full", MedianNS: 15, Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "clean2.kdl", Mode: "cgo_full", MedianNS: 10, Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "clean2.kdl", Mode: "go_full", MedianNS: 17, Parity: paritySummary{Clean: true}},
		{Language: "kdl", Sample: "err1.kdl", Mode: "cgo_full", MedianNS: 10, Parity: paritySummary{Clean: false}},
		{Language: "kdl", Sample: "err1.kdl", Mode: "go_full", MedianNS: 1000, Parity: paritySummary{Clean: false}},
	}

	summary := renderSummary(rows, []string{"kdl"})

	// Combined ratio mixes clean and error samples: (15+17+1000)/(10+10+10) = 34.40x.
	if !strings.Contains(summary, "34.40x") {
		t.Fatalf("summary missing combined ratio 34.40x:\n%s", summary)
	}
	// Clean-only ratio should isolate the two well-behaved files: (15+17)/(10+10) = 1.60x.
	if !strings.Contains(summary, "1.60x") {
		t.Fatalf("summary missing clean_ratio 1.60x:\n%s", summary)
	}
	// Error-only ratio should isolate the single error-bearing file: 1000/10 = 100.00x.
	if !strings.Contains(summary, "100.00x") {
		t.Fatalf("summary missing error_ratio 100.00x:\n%s", summary)
	}
	// Two clean files, one error file, so error share is 1/3.
	if !strings.Contains(summary, "33.3%") {
		t.Fatalf("summary missing error_share 33.3%%:\n%s", summary)
	}
}

func TestStatsFromRuntimeReportsScratchAndTransientMemory(t *testing.T) {
	stats := statsFromRuntime(gotreesitter.ParseRuntime{
		StopDiagnosticCaptured:             true,
		StopDiagnosticTokenSymbol:          25,
		MemoryBudgetStopSource:             "runtime_heap",
		RuntimeHeapGrowthBytes:             567,
		RuntimeSysGrowthBytes:              890,
		ArenaBytesAllocated:                2000,
		ArenaBaselineBytes:                 500,
		ScratchBytesAllocated:              1234,
		ScratchBaselineBytes:               100,
		EntryScratchBytesAllocated:         234,
		GSSBytesAllocated:                  345,
		GSSBaselineBytes:                   50,
		GSSSlabCount:                       4,
		GSSNodesUsed:                       55,
		GSSNodesCapacity:                   80,
		GSSDemotions:                       6,
		GSSNodesDemoted:                    77,
		TransientScratchCheckpoints:        8,
		SingleStackGSSNodes:                44,
		MultiStackGSSNodes:                 11,
		TransientChildSlicesAllocated:      12,
		TransientChildPointersAllocated:    34,
		TransientChildSlicesMaterialized:   5,
		TransientChildPointersMaterialized: 8,
		TransientParentNodesAllocated:      13,
		TransientParentNodesMaterialized:   21,
	})
	if !strings.Contains(stats.StopDiagnostic, "stopDiag={") || !strings.Contains(stats.StopDiagnostic, "token=25[") {
		t.Fatalf("StopDiagnostic = %q, want captured token diagnostic", stats.StopDiagnostic)
	}

	if got, want := stats.MemoryBudgetStopSource, "runtime_heap"; got != want {
		t.Fatalf("MemoryBudgetStopSource = %q, want %q", got, want)
	}
	if got, want := stats.RuntimeHeapGrowthB, uint64(567); got != want {
		t.Fatalf("RuntimeHeapGrowthB = %d, want %d", got, want)
	}
	if got, want := stats.RuntimeSysGrowthB, uint64(890); got != want {
		t.Fatalf("RuntimeSysGrowthB = %d, want %d", got, want)
	}
	if got, want := stats.ArenaCapacityB, int64(2000); got != want {
		t.Fatalf("ArenaCapacityB = %d, want %d", got, want)
	}
	if got, want := stats.ArenaBaselineB, int64(500); got != want {
		t.Fatalf("ArenaBaselineB = %d, want %d", got, want)
	}
	if got, want := stats.ScratchAllocatedB, int64(1234); got != want {
		t.Fatalf("ScratchAllocatedB = %d, want %d", got, want)
	}
	if got, want := stats.ScratchBaselineB, int64(100); got != want {
		t.Fatalf("ScratchBaselineB = %d, want %d", got, want)
	}
	if got, want := stats.EntryScratchAllocatedB, int64(234); got != want {
		t.Fatalf("EntryScratchAllocatedB = %d, want %d", got, want)
	}
	if got, want := stats.GSSScratchAllocatedB, int64(345); got != want {
		t.Fatalf("GSSScratchAllocatedB = %d, want %d", got, want)
	}
	if got, want := stats.GSSScratchBaselineB, int64(50); got != want {
		t.Fatalf("GSSScratchBaselineB = %d, want %d", got, want)
	}
	if got, want := stats.GSSSlabCount, 4; got != want {
		t.Fatalf("GSSSlabCount = %d, want %d", got, want)
	}
	if got, want := stats.GSSNodesUsed, 55; got != want {
		t.Fatalf("GSSNodesUsed = %d, want %d", got, want)
	}
	if got, want := stats.GSSNodesCapacity, 80; got != want {
		t.Fatalf("GSSNodesCapacity = %d, want %d", got, want)
	}
	if got, want := stats.GSSDemotions, uint64(6); got != want {
		t.Fatalf("GSSDemotions = %d, want %d", got, want)
	}
	if got, want := stats.GSSNodesDemoted, uint64(77); got != want {
		t.Fatalf("GSSNodesDemoted = %d, want %d", got, want)
	}
	if got, want := stats.TransientScratchCheckpoints, uint64(8); got != want {
		t.Fatalf("TransientScratchCheckpoints = %d, want %d", got, want)
	}
	if got, want := stats.SingleStackGSSNodes, uint64(44); got != want {
		t.Fatalf("SingleStackGSSNodes = %d, want %d", got, want)
	}
	if got, want := stats.MultiStackGSSNodes, uint64(11); got != want {
		t.Fatalf("MultiStackGSSNodes = %d, want %d", got, want)
	}
	if got, want := stats.TransientChildSlicesAllocated, uint64(12); got != want {
		t.Fatalf("TransientChildSlicesAllocated = %d, want %d", got, want)
	}
	if got, want := stats.TransientChildPointersAllocated, uint64(34); got != want {
		t.Fatalf("TransientChildPointersAllocated = %d, want %d", got, want)
	}
	if got, want := stats.TransientChildSlicesMaterialized, uint64(5); got != want {
		t.Fatalf("TransientChildSlicesMaterialized = %d, want %d", got, want)
	}
	if got, want := stats.TransientChildPointersMaterialized, uint64(8); got != want {
		t.Fatalf("TransientChildPointersMaterialized = %d, want %d", got, want)
	}
	if got, want := stats.TransientParentNodesAllocated, uint64(13); got != want {
		t.Fatalf("TransientParentNodesAllocated = %d, want %d", got, want)
	}
	if got, want := stats.TransientParentNodesMaterialized, uint64(21); got != want {
		t.Fatalf("TransientParentNodesMaterialized = %d, want %d", got, want)
	}

	payload, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal stats: %v", err)
	}
	jsonText := string(payload)
	for _, field := range []string{
		`"memory_budget_stop_source":"runtime_heap"`,
		`"runtime_heap_growth_b":567`,
		`"runtime_sys_growth_b":890`,
		`"arena_capacity_b":2000`,
		`"arena_baseline_b":500`,
		`"scratch_allocated_b":1234`,
		`"scratch_baseline_b":100`,
		`"entry_scratch_allocated_b":234`,
		`"gss_scratch_allocated_b":345`,
		`"gss_scratch_baseline_b":50`,
		`"gss_slab_count":4`,
		`"gss_nodes_used":55`,
		`"gss_nodes_capacity":80`,
		`"gss_demotions":6`,
		`"gss_nodes_demoted":77`,
		`"transient_scratch_checkpoints":8`,
		`"single_stack_gss_nodes":44`,
		`"multi_stack_gss_nodes":11`,
		`"transient_child_slices_allocated":12`,
		`"transient_child_pointers_allocated":34`,
		`"transient_child_slices_materialized":5`,
		`"transient_child_pointers_materialized":8`,
		`"transient_parent_nodes_allocated":13`,
		`"transient_parent_nodes_materialized":21`,
	} {
		if !strings.Contains(jsonText, field) {
			t.Fatalf("marshal stats = %s, missing %s", jsonText, field)
		}
	}
}

func TestArenaLiveBytesIncludesEveryRuntimeArenaComponent(t *testing.T) {
	breakdown := gotreesitter.ArenaBreakdown{
		CompactReuseDependencyBytesAllocated: 16,

		NodeStructBytesAllocated:            1,
		NodeFieldMetadataBytesAllocated:     14,
		NoTreeNodeBytesAllocated:            2,
		CompactFullLeafBytesAllocated:       3,
		PendingParentBytesAllocated:         4,
		PendingChildEntryBytesAllocated:     5,
		RawShapeBytesAllocated:              6,
		RawShapeChildBytesAllocated:         7,
		RawShapeHashCacheBytesAllocated:     13,
		FinalChildSidecarBytesAllocated:     8,
		MissingNodeDependencyBytesAllocated: 15,
		CompactCheckpointLeafBytesAllocated: 9,
		ChildSliceBytesAllocated:            10,
		FieldIDBytesAllocated:               11,
		FieldSourceBytesAllocated:           12,
	}
	runtime := gotreesitter.ParseRuntime{
		ExternalScannerCheckpointBytesAllocated: 13,
		ArenaBytesAllocated:                     149,
	}
	if got, want := arenaLiveBytes(breakdown, runtime.ExternalScannerCheckpointBytesAllocated), runtime.ArenaBytesAllocated; got != want {
		t.Fatalf("arenaLiveBytes = %d, runtime arena total = %d", got, want)
	}
}

func TestRunGoEditReportsIncrementalAttribution(t *testing.T) {
	lang := grammars.RustLanguage()
	r := &runner{
		name:     "rust",
		goLang:   lang,
		goParser: gotreesitter.NewParser(lang),
		support: grammars.ParseSupport{
			Name:    "rust",
			Backend: grammars.ParseBackendDFA,
		},
	}

	source := []byte(strings.Repeat("// comment abc\nfn main() {}\n", 32))
	stats, err := runGoEdit(r, source, false)
	if err != nil {
		t.Fatalf("runGoEdit: %v", err)
	}
	if stats.SetupParseNS <= 0 {
		t.Fatalf("SetupParseNS = %d, want > 0", stats.SetupParseNS)
	}
	if stats.TreeEditNS <= 0 {
		t.Fatalf("TreeEditNS = %d, want > 0", stats.TreeEditNS)
	}
	if stats.ParseWallNS != stats.IncrementalReuseNS+stats.IncrementalReparseNS {
		t.Fatalf("ParseWallNS = %d, want reuse + reparse = %d", stats.ParseWallNS, stats.IncrementalReuseNS+stats.IncrementalReparseNS)
	}
}

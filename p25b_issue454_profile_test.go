package gotreesitter_test

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestP25BIssue454Profile(t *testing.T) {
	mode := os.Getenv("P25B_MODE")
	if mode == "" {
		t.Skip("set P25B_MODE=fresh or incremental")
	}
	runs := 3
	if raw := os.Getenv("P25B_RUNS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			t.Fatalf("invalid P25B_RUNS=%q", raw)
		}
		runs = value
	}

	source := benchfixtures.Issue454CSource()
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C edit marker is absent")
	}
	edited := append(append([]byte(nil), source[:site]...), source[site+1:]...)
	point := p25bPointAt(source, site)
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(site),
		OldEndByte:  uint32(site + 1),
		NewEndByte:  uint32(site),
		StartPoint:  point,
		OldEndPoint: p25bPointAt(source, site+1),
		NewEndPoint: point,
	}

	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { gotreesitter.EnableRecoveryRuntimeTelemetry(false) })

	lang := grammars.CLanguage()
	switch mode {
	case "fresh":
		p25bRunFresh(t, lang, edited, runs)
	case "incremental":
		p25bRunIncremental(t, lang, source, edited, edit, runs)
	default:
		t.Fatalf("unsupported P25B_MODE=%q", mode)
	}
}

func p25bRunFresh(t *testing.T, lang *gotreesitter.Language, source []byte, runs int) {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	for run := 0; run < runs; run++ {
		runtime.GC()
		before := p25bMemStats()
		started := time.Now()
		tree, err := parser.Parse(source)
		wall := time.Since(started)
		if err != nil {
			t.Fatalf("fresh run %d: %v", run, err)
		}
		if tree == nil || tree.RootNode() == nil {
			t.Fatalf("fresh run %d returned no tree", run)
		}
		runtimeStats := tree.ParseRuntime()
		recoveryStats := parser.DebugRecoveryRuntimeStats()
		digest := p25bDigest(t, tree, lang)
		after := p25bMemStats()
		fmt.Printf("P25B_FRESH run=%d bytes=%d wall_ns=%d mallocs=%d total_alloc=%d heap_alloc_delta=%d digest=%s root_end=%d has_error=%v stop=%s nodes=%d arena=%d scratch=%d gss=%d memory_budget=%d memory_stop_source=%s runtime_heap_growth=%d runtime_sys_growth=%d tokens=%d max_stacks=%d parser_loop_ns=%d recovery_entries=%d recovery_cost_competitions=%d recovery_cost_walk_ns=%d\n",
			run, len(source), wall.Nanoseconds(), after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc,
			int64(after.HeapAlloc)-int64(before.HeapAlloc), digest, tree.RootNode().EndByte(), tree.RootNode().HasError(),
			runtimeStats.StopReason, runtimeStats.NodesAllocated, runtimeStats.ArenaBytesAllocated, runtimeStats.ScratchBytesAllocated,
			runtimeStats.GSSBytesAllocated, runtimeStats.MemoryBudgetBytes, runtimeStats.MemoryBudgetStopSource,
			runtimeStats.RuntimeHeapGrowthBytes, runtimeStats.RuntimeSysGrowthBytes, runtimeStats.TokensConsumed,
			runtimeStats.MaxStacksSeen, runtimeStats.ParserLoopNanos, recoveryStats.RecoveryEntryCount,
			recoveryStats.RecoveryCostCompetitionCount, recoveryStats.RecoveryCostWalkNanos)
		p25bPrintAttempts(parser, "fresh", run)
		tree.Release()
	}
}

func p25bRunIncremental(t *testing.T, lang *gotreesitter.Language, source, edited []byte, edit gotreesitter.InputEdit, runs int) {
	t.Helper()
	for run := 0; run < runs; run++ {
		parser := gotreesitter.NewParser(lang)
		oldTree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("base run %d: %v", run, err)
		}
		baseDigest := p25bDigest(t, oldTree, lang)
		runtime.GC()
		beforeEdit := p25bMemStats()
		editStarted := time.Now()
		oldTree.Edit(edit)
		editWall := time.Since(editStarted)
		afterEdit := p25bMemStats()
		parseStarted := time.Now()
		incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
		parseWall := time.Since(parseStarted)
		if err != nil {
			oldTree.Release()
			t.Fatalf("incremental run %d: %v", run, err)
		}
		if incremental == nil || incremental.RootNode() == nil {
			t.Fatalf("incremental run %d returned no tree", run)
		}
		afterParse := p25bMemStats()
		runtimeStats := incremental.ParseRuntime()
		recoveryStats := parser.DebugRecoveryRuntimeStats()
		digest := p25bDigest(t, incremental, lang)
		fmt.Printf("P25B_INCREMENTAL run=%d bytes=%d site=%d base_digest=%s incremental_digest=%s edit_ns=%d edit_mallocs=%d edit_total_alloc=%d parse_wall_ns=%d parse_mallocs=%d parse_total_alloc=%d parse_heap_alloc_delta=%d reuse_ns=%d reparse_ns=%d reused_subtrees=%d reused_bytes=%d new_nodes=%d reuse_unsupported=%v reuse_reason=%s old_tree_reuse_route=%v reject_dirty=%d reject_ancestor_dirty=%d reject_root_nonleaf=%d reject_fragile=%d reject_scanner=%d block_splice_steps=%d recover_searches=%d recover_state_checks=%d recover_state_skips=%d recover_symbol_skips=%d recover_lookups=%d recover_hits=%d tokens=%d max_stacks=%d entry_scratch_peak=%d stop=%s nodes=%d arena=%d scratch=%d gss=%d memory_budget=%d memory_stop_source=%s runtime_heap_growth=%d runtime_sys_growth=%d c_recovery_entered=%v c_recovery_dropped_clean=%v retry_passes=%d recovery_entries=%d recovery_cost_competitions=%d recovery_cost_walk_ns=%d parser_loop_ns=%d result_selection_ns=%d result_tree_build_ns=%d normalization_ns=%d root_end=%d has_error=%v\n",
			run, len(edited), edit.StartByte, baseDigest, digest, editWall.Nanoseconds(),
			afterEdit.Mallocs-beforeEdit.Mallocs, afterEdit.TotalAlloc-beforeEdit.TotalAlloc,
			parseWall.Nanoseconds(), afterParse.Mallocs-afterEdit.Mallocs, afterParse.TotalAlloc-afterEdit.TotalAlloc,
			int64(afterParse.HeapAlloc)-int64(afterEdit.HeapAlloc), profile.ReuseCursorNanos, profile.ReparseNanos,
			profile.ReusedSubtrees, profile.ReusedBytes, profile.NewNodesAllocated, profile.ReuseUnsupported,
			profile.ReuseUnsupportedReason, profile.OldTreeReuseRoute, profile.ReuseRejectDirty,
			profile.ReuseRejectAncestorDirtyBeforeEdit, profile.ReuseRejectRootNonLeafChanged,
			profile.ReuseRejectFragileNonLeaf, profile.ReuseRejectScannerUnquiescent, profile.BlockSpliceSteps,
			profile.RecoverSearches, profile.RecoverStateChecks, profile.RecoverStateSkips, profile.RecoverSymbolSkips,
			profile.RecoverLookups, profile.RecoverHits, profile.TokensConsumed, profile.MaxStacksSeen,
			profile.EntryScratchPeak, profile.StopReason, runtimeStats.NodesAllocated, runtimeStats.ArenaBytesAllocated,
			runtimeStats.ScratchBytesAllocated, runtimeStats.GSSBytesAllocated, runtimeStats.MemoryBudgetBytes,
			runtimeStats.MemoryBudgetStopSource, runtimeStats.RuntimeHeapGrowthBytes, runtimeStats.RuntimeSysGrowthBytes,
			runtimeStats.CRecoveryEnteredErrorState, runtimeStats.CRecoveryDroppedErrorForClean,
			runtimeStats.IncrementalAcceptedErrorRetryAttempts, recoveryStats.RecoveryEntryCount,
			recoveryStats.RecoveryCostCompetitionCount, recoveryStats.RecoveryCostWalkNanos, runtimeStats.ParserLoopNanos,
			runtimeStats.ResultSelectionNanos, runtimeStats.ResultTreeBuildNanos, profile.NormalizationNanos,
			incremental.RootNode().EndByte(), incremental.RootNode().HasError())
		p25bPrintAttempts(parser, "incremental", run)
		if incremental != oldTree {
			oldTree.Release()
		}
		incremental.Release()
	}
}

func p25bDigest(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect tree: %v", err)
	}
	return inspection.SHA256
}

func p25bPointAt(source []byte, offset int) gotreesitter.Point {
	row := bytes.Count(source[:offset], []byte{'\n'})
	column := offset
	if newline := bytes.LastIndexByte(source[:offset], '\n'); newline >= 0 {
		column = offset - newline - 1
	}
	return gotreesitter.Point{Row: uint32(row), Column: uint32(column)}
}

func p25bMemStats() runtime.MemStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}

func p25bPrintAttempts(parser *gotreesitter.Parser, phase string, run int) {
	for _, attempt := range parser.DebugRecoveryRuntimeAttempts() {
		fmt.Printf("P25B_ATTEMPT phase=%s run=%d ordinal=%d rung=%s cause=%s stop=%s truncated=%v full_span=%v has_error=%v wall_ns=%d heap_delta=%d total_alloc=%d mallocs=%d recovery_entries=%d recovery_cost_competitions=%d recovery_cost_walk_ns=%d materialization_ns=%d arena_peak=%d scratch_peak=%d entry_scratch_peak=%d gss_peak=%d nodes=%d max_stacks=%d peak_depth=%d live_versions=%d peak_live_versions=%d selected=%v replaced=%v\n",
			phase, run, attempt.Ordinal, attempt.Rung, attempt.Cause, attempt.StopReason, attempt.Truncated,
			attempt.AttemptFullSpan, attempt.AttemptHasError, attempt.WallNanos, attempt.HeapAllocDeltaBytes,
			attempt.TotalAllocDeltaBytes, attempt.MallocsDelta, attempt.RecoveryEntryCount,
			attempt.RecoveryCostCompetitionCount, attempt.RecoveryCostWalkNanos, attempt.MaterializationNanos,
			attempt.ArenaBytesPeak, attempt.ScratchBytesPeak, attempt.EntryScratchBytesPeak, attempt.GSSBytesPeak,
			attempt.NodesAllocated, attempt.MaxStacksSeen, attempt.PeakStackDepth, attempt.LiveVersions,
			attempt.PeakLiveVersions, attempt.CandidateSelected, attempt.CandidateReplacedIncumbent)
	}
}

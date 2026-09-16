package gotreesitter_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type dfaBenchmarkSpec struct {
	name            string
	lang            func() *gotreesitter.Language
	source          func(int) []byte
	marker          string
	funcCount       int
	requireNoErrors bool
	warmupBeforeRun bool
}

func makeTypeScriptBenchmarkSource(funcCount int) []byte {
	const lineLen = len("export function f0(): number { const v = 0; return v }\n")
	buf := make([]byte, 0, funcCount*lineLen)
	for i := 0; i < funcCount; i++ {
		buf = append(buf, []byte("export function f")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("(): number { const v = ")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("; return v }\n")...)
	}
	return buf
}

func makePythonBenchmarkSource(funcCount int) []byte {
	const lineLen = len("def f0():\n    v = 0\n    return v\n\n")
	buf := make([]byte, 0, funcCount*lineLen)
	for i := 0; i < funcCount; i++ {
		buf = append(buf, []byte("def f")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("():\n    v = ")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("\n    return v\n\n")...)
	}
	return buf
}

func makePythonFStringCompatibilityBenchmarkSource(lineCount int) []byte {
	const line = "f\"{alpha, beta}\"\n"
	buf := make([]byte, 0, lineCount*len(line))
	for i := 0; i < lineCount; i++ {
		buf = append(buf, line...)
	}
	return buf
}

func makePythonFStringSplatBenchmarkSource(lineCount int) []byte {
	const line = "f\"{*values,}\"\n"
	buf := make([]byte, 0, lineCount*len(line))
	for i := 0; i < lineCount; i++ {
		buf = append(buf, line...)
	}
	return buf
}

func benchmarkParseFullDFA(b *testing.B, spec dfaBenchmarkSpec) {
	lang := spec.lang()
	parser := gotreesitter.NewParser(lang)
	funcCount := benchmarkFuncCount(b)
	if spec.funcCount > 0 {
		funcCount = spec.funcCount
	}
	src := spec.source(funcCount)
	statsEnabled := strings.TrimSpace(os.Getenv("GOT_STATS")) != ""
	if statsEnabled {
		gotreesitter.ResetArenaProfile()
		gotreesitter.ResetPerfCounters()
		gotreesitter.EnableArenaProfile(true)
		defer gotreesitter.EnableArenaProfile(false)
	}
	if spec.warmupBeforeRun {
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("warmup parse error: %v", err)
		}
		root := requireCompleteParse(b, tree, src, lang, "warmup full dfa")
		if spec.requireNoErrors && root.HasError() {
			b.Fatalf("warmup full dfa parse produced errors: root=%q %s", root.Type(lang), tree.ParseRuntime().Summary())
		}
		tree.Release()
		if statsEnabled {
			gotreesitter.ResetArenaProfile()
			gotreesitter.ResetPerfCounters()
		}
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	var lastRuntime gotreesitter.ParseRuntime
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		root := requireCompleteParse(b, tree, src, lang, "full dfa")
		if spec.requireNoErrors && root.HasError() {
			b.Fatalf("full dfa parse produced errors: root=%q %s", root.Type(lang), tree.ParseRuntime().Summary())
		}
		lastRuntime = tree.ParseRuntime()
		tree.Release()
	}
	if statsEnabled {
		a := gotreesitter.ArenaProfileSnapshot()
		p := gotreesitter.PerfCountersSnapshot()
		fmt.Printf(
			"STATS_LANG name=%s glr_max_stacks=%d\n",
			spec.name, effectiveGLRMaxStacksForStats(),
		)
		fmt.Printf(
			"STATS arena_full_acquire=%d arena_full_new=%d arena_inc_acquire=%d arena_inc_new=%d\n",
			a.FullAcquire, a.FullNew, a.IncrementalAcquire, a.IncrementalNew,
		)
		fmt.Printf(
			"STATS_PERF merge_calls=%d merge_dead_pruned=%d merge_perkey_overflow=%d merge_replacements=%d stackeq_calls=%d stackeq_true=%d stackeq_hash_miss_skips=%d stackcmp_calls=%d forks=%d first_conflict_token=%d max_stacks=%d lex_bytes=%d lex_tokens=%d\n",
			p.MergeCalls, p.MergeDeadPruned, p.MergePerKeyOverflow, p.MergeReplacements, p.StackEquivalentCalls, p.StackEquivalentTrue, p.StackEqHashMissSkips, p.StackCompareCalls, p.ForkCount, p.FirstConflictToken, p.MaxConcurrentStacks, p.LexBytes, p.LexTokens,
		)
		reportReduceForkPerfCounters(p)
		fmt.Printf(
			"STATS_PARSE nodes_new=%d children_ptrs=%d extras=%d errors=%d reuse_bytes=%d max_stacks=%d\n",
			lastRuntime.NodesAllocated, p.ParentChildPointers, p.ExtraNodes, p.ErrorNodes, p.ReuseNonLeafBytes, lastRuntime.MaxStacksSeen,
		)
	}
	reportTransientReduceMetrics(b, lastRuntime)
}

func reportTransientReduceMetrics(b *testing.B, rt gotreesitter.ParseRuntime) {
	if rt.TokensConsumed == 0 {
		return
	}
	if rt.TransientChildSlicesAllocated == 0 &&
		rt.TransientChildSlicesMaterialized == 0 &&
		rt.TransientParentNodesAllocated == 0 &&
		rt.TransientParentNodesMaterialized == 0 {
		return
	}
	tokens := float64(rt.TokensConsumed)
	b.ReportMetric(float64(rt.TransientChildSlicesAllocated)/tokens, "transient_child_slices_alloc/token")
	b.ReportMetric(float64(rt.TransientChildPointersAllocated)/tokens, "transient_child_ptrs_alloc/token")
	b.ReportMetric(float64(rt.TransientChildSlicesMaterialized)/tokens, "transient_child_slices_materialized/token")
	b.ReportMetric(float64(rt.TransientChildPointersMaterialized)/tokens, "transient_child_ptrs_materialized/token")
	b.ReportMetric(float64(rt.TransientParentNodesAllocated)/tokens, "transient_parent_nodes_alloc/token")
	b.ReportMetric(float64(rt.TransientParentNodesMaterialized)/tokens, "transient_parent_nodes_materialized/token")
	if rt.TransientParentNodesAllocated >= rt.TransientParentNodesMaterialized {
		dropped := rt.TransientParentNodesAllocated - rt.TransientParentNodesMaterialized
		b.ReportMetric(float64(dropped)/tokens, "transient_parent_nodes_dropped/token")
	}
}

func benchmarkParseIncrementalSingleByteEditDFA(b *testing.B, spec dfaBenchmarkSpec) {
	lang := spec.lang()
	parser := gotreesitter.NewParser(lang)
	src := spec.source(benchmarkFuncCount(b))
	sites := makeBenchmarkEditSites(src, spec.marker)
	if len(sites) == 0 {
		b.Fatalf("could not find edit marker %q", spec.marker)
	}
	site := sites[0]

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(site.offset),
		OldEndByte:  uint32(site.offset + 1),
		NewEndByte:  uint32(site.offset + 1),
		StartPoint:  site.start,
		OldEndPoint: site.end,
		NewEndPoint: site.end,
	}
	statsEnabled := strings.TrimSpace(os.Getenv("GOT_STATS")) != ""
	if statsEnabled {
		gotreesitter.ResetArenaProfile()
		gotreesitter.ResetPerfCounters()
		gotreesitter.EnableArenaProfile(true)
		defer gotreesitter.EnableArenaProfile(false)
	}
	var editTotalNS uint64
	var reuseTotalNS uint64
	var parseTotalNS uint64
	var reusedSubtrees uint64
	var reusedBytes uint64
	var newNodesAllocated uint64
	var recoverSearches uint64
	var recoverStateChecks uint64
	var recoverStateSkips uint64
	var recoverSymbolSkips uint64
	var recoverLookups uint64
	var recoverHits uint64
	var entryScratchPeak uint64
	maxStacksSeen := 0
	reuseUnsupported := false
	reuseUnsupportedReason := ""

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		toggleDigitAt(src, site.offset)
		editStart := time.Now()
		tree.Edit(edit)
		old := tree
		if statsEnabled {
			editTotalNS += uint64(time.Since(editStart).Nanoseconds())
			var prof gotreesitter.IncrementalParseProfile
			tree, prof, err = parser.ParseIncrementalProfiled(src, tree)
			reuseTotalNS += uint64(prof.ReuseCursorNanos)
			parseTotalNS += uint64(prof.ReparseNanos)
			reusedSubtrees += prof.ReusedSubtrees
			reusedBytes += prof.ReusedBytes
			newNodesAllocated += prof.NewNodesAllocated
			recoverSearches += prof.RecoverSearches
			recoverStateChecks += prof.RecoverStateChecks
			recoverStateSkips += prof.RecoverStateSkips
			recoverSymbolSkips += prof.RecoverSymbolSkips
			recoverLookups += prof.RecoverLookups
			recoverHits += prof.RecoverHits
			if prof.EntryScratchPeak > entryScratchPeak {
				entryScratchPeak = prof.EntryScratchPeak
			}
			if prof.MaxStacksSeen > maxStacksSeen {
				maxStacksSeen = prof.MaxStacksSeen
			}
			if prof.ReuseUnsupported {
				reuseUnsupported = true
				if reuseUnsupportedReason == "" {
					reuseUnsupportedReason = prof.ReuseUnsupportedReason
				}
			}
		} else {
			tree, err = parser.ParseIncremental(src, tree)
		}
		if err != nil {
			b.Fatalf("incremental parse error: %v", err)
		}
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		if old != tree {
			old.Release()
		}
	}
	if statsEnabled {
		a := gotreesitter.ArenaProfileSnapshot()
		p := gotreesitter.PerfCountersSnapshot()
		fmt.Printf(
			"STATS_LANG name=%s glr_max_stacks=%d reuse_unsupported=%t reuse_reason=%q\n",
			spec.name, effectiveGLRMaxStacksForStats(), reuseUnsupported, reuseUnsupportedReason,
		)
		fmt.Printf(
			"STATS edits=%d edit_ns=%d reuse_ns=%d parse_ns=%d reused_subtrees=%d reused_bytes=%d new_nodes=%d recover_searches=%d recover_state_checks=%d recover_state_skips=%d recover_symbol_skips=%d recover_lookups=%d recover_hits=%d max_stacks=%d scratch_peak_entries=%d\n",
			b.N, editTotalNS, reuseTotalNS, parseTotalNS, reusedSubtrees, reusedBytes, newNodesAllocated, recoverSearches, recoverStateChecks, recoverStateSkips, recoverSymbolSkips, recoverLookups, recoverHits, maxStacksSeen, entryScratchPeak,
		)
		fmt.Printf(
			"STATS arena_full_acquire=%d arena_full_new=%d arena_inc_acquire=%d arena_inc_new=%d\n",
			a.FullAcquire, a.FullNew, a.IncrementalAcquire, a.IncrementalNew,
		)
		fmt.Printf(
			"STATS_PERF merge_calls=%d merge_dead_pruned=%d merge_perkey_overflow=%d merge_replacements=%d stackeq_calls=%d stackeq_true=%d stackeq_hash_miss_skips=%d stackcmp_calls=%d forks=%d first_conflict_token=%d max_stacks=%d lex_bytes=%d lex_tokens=%d reuse_nodes_visited=%d reuse_nodes_pushed=%d reuse_nodes_popped=%d reuse_candidates=%d reuse_successes=%d reuse_leaf_successes=%d reuse_nonleaf_checks=%d reuse_nonleaf_successes=%d reuse_nonleaf_bytes=%d reuse_nonleaf_nogoto=%d reuse_nonleaf_nogoto_term=%d reuse_nonleaf_nogoto_nonterm=%d reuse_nonleaf_statemiss=%d reuse_nonleaf_statezero=%d\n",
			p.MergeCalls, p.MergeDeadPruned, p.MergePerKeyOverflow, p.MergeReplacements, p.StackEquivalentCalls, p.StackEquivalentTrue, p.StackEqHashMissSkips, p.StackCompareCalls, p.ForkCount, p.FirstConflictToken, p.MaxConcurrentStacks, p.LexBytes, p.LexTokens, p.ReuseNodesVisited, p.ReuseNodesPushed, p.ReuseNodesPopped, p.ReuseCandidatesChecked, p.ReuseSuccesses, p.ReuseLeafSuccesses, p.ReuseNonLeafChecks, p.ReuseNonLeafSuccesses, p.ReuseNonLeafBytes, p.ReuseNonLeafNoGoto, p.ReuseNonLeafNoGotoTerm, p.ReuseNonLeafNoGotoNt, p.ReuseNonLeafStateMiss, p.ReuseNonLeafStateZero,
		)
		reportReduceForkPerfCounters(p)
	}
	tree.Release()
}

func reportReduceForkPerfCounters(p gotreesitter.PerfCounters) {
	fmt.Printf(
		"STATS_PERF reduce_fork_calls=%d reduce_fork_windows=%d reduce_fork_max_windows=%d post_reduce_merge_attempts=%d post_reduce_merge_primary_successes=%d post_reduce_merge_pending_successes=%d post_reduce_merge_disabled_skips=%d post_reduce_merge_finalization_risk_skips=%d pending_fork_stack_appends=%d pending_fork_stacks_max_len=%d\n",
		p.ReduceForkCalls, p.ReduceForkWindows, p.ReduceForkMaxWindows, p.PostReduceMergeAttempts, p.PostReduceMergePrimarySuccesses, p.PostReduceMergePendingSuccesses, p.PostReduceMergeDisabledSkips, p.PostReduceMergeFinalizationRiskSkips, p.PendingForkStackAppends, p.PendingForkStacksMaxLen,
	)
}

func benchmarkParseIncrementalNoEditDFA(b *testing.B, spec dfaBenchmarkSpec) {
	lang := spec.lang()
	parser := gotreesitter.NewParser(lang)
	src := spec.source(benchmarkFuncCount(b))

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	statsEnabled := strings.TrimSpace(os.Getenv("GOT_STATS")) != ""
	if statsEnabled {
		gotreesitter.ResetPerfCounters()
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		old := tree
		tree, err = parser.ParseIncremental(src, tree)
		if err != nil {
			b.Fatalf("incremental parse error: %v", err)
		}
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		if old != tree {
			old.Release()
		}
	}
	if statsEnabled {
		reportReduceForkPerfCounters(gotreesitter.PerfCountersSnapshot())
	}
	tree.Release()
}

func stringInt(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func BenchmarkTypeScriptParseFullDFA(b *testing.B) {
	benchmarkParseFullDFA(b, dfaBenchmarkSpec{
		name:   "typescript",
		lang:   grammars.TypescriptLanguage,
		source: makeTypeScriptBenchmarkSource,
		marker: "const v = ",
	})
}

func BenchmarkTypeScriptParseIncrementalSingleByteEditDFA(b *testing.B) {
	benchmarkParseIncrementalSingleByteEditDFA(b, dfaBenchmarkSpec{
		name:   "typescript",
		lang:   grammars.TypescriptLanguage,
		source: makeTypeScriptBenchmarkSource,
		marker: "const v = ",
	})
}

func BenchmarkTypeScriptParseIncrementalNoEditDFA(b *testing.B) {
	benchmarkParseIncrementalNoEditDFA(b, dfaBenchmarkSpec{
		name:   "typescript",
		lang:   grammars.TypescriptLanguage,
		source: makeTypeScriptBenchmarkSource,
		marker: "const v = ",
	})
}

func BenchmarkPythonParseFullDFA(b *testing.B) {
	benchmarkParseFullDFA(b, dfaBenchmarkSpec{
		name:   "python",
		lang:   grammars.PythonLanguage,
		source: makePythonBenchmarkSource,
		marker: "v = ",
	})
}

func BenchmarkPythonParseIncrementalSingleByteEditDFA(b *testing.B) {
	benchmarkParseIncrementalSingleByteEditDFA(b, dfaBenchmarkSpec{
		name:   "python",
		lang:   grammars.PythonLanguage,
		source: makePythonBenchmarkSource,
		marker: "v = ",
	})
}

func BenchmarkPythonParseIncrementalNoEditDFA(b *testing.B) {
	benchmarkParseIncrementalNoEditDFA(b, dfaBenchmarkSpec{
		name:   "python",
		lang:   grammars.PythonLanguage,
		source: makePythonBenchmarkSource,
		marker: "v = ",
	})
}

func BenchmarkPythonParseFStringCompatibilityDFA(b *testing.B) {
	lang := grammars.PythonLanguage()
	parser := gotreesitter.NewParser(lang)
	source := makePythonFStringCompatibilityBenchmarkSource(2048)
	warm, err := parser.Parse(source)
	if err != nil {
		b.Fatal(err)
	}
	root := requireCompleteParse(b, warm, source, lang, "Python f-string compatibility warmup")
	if root.HasError() {
		warm.Release()
		b.Fatal("Python f-string compatibility warmup produced errors")
	}
	warm.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()

	var last gotreesitter.ParseRuntime
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		root := requireCompleteParse(b, tree, source, lang, "Python f-string compatibility")
		if root.HasError() {
			tree.Release()
			b.Fatal("Python f-string compatibility parse produced errors")
		}
		last = tree.ParseRuntime()
		tree.Release()
	}
	b.StopTimer()
	b.ReportMetric(float64(last.NormalizationPassesChecked), "norm_checked/op")
	b.ReportMetric(float64(last.NormalizationPassesRun), "norm_runs/op")
	b.ReportMetric(float64(last.NormalizationNodesVisited), "norm_visited/op")
	b.ReportMetric(float64(last.NormalizationNanos), "norm_ns/op")
}

func BenchmarkPythonParseFStringForestDFA(b *testing.B) {
	lang := grammars.PythonLanguage()
	parser := gotreesitter.NewParser(lang)
	source := makePythonFStringSplatBenchmarkSource(2048)
	warm, ok := parser.ParseForestExperimental(source)
	if !ok || warm == nil {
		b.Fatal("Python f-string forest warmup declined")
	}
	root := requireCompleteParse(b, warm, source, lang, "Python f-string forest warmup")
	if root.HasError() {
		warm.Release()
		b.Fatal("Python f-string forest warmup produced errors")
	}
	warm.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree, ok := parser.ParseForestExperimental(source)
		if !ok || tree == nil {
			b.Fatal("Python f-string forest parse declined")
		}
		root := requireCompleteParse(b, tree, source, lang, "Python f-string forest")
		if root.HasError() {
			tree.Release()
			b.Fatal("Python f-string forest parse produced errors")
		}
		tree.Release()
	}
}

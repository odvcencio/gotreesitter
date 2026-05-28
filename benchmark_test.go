package gotreesitter_test

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Run pure-Go benchmarks from this package.
// C baseline benchmarks live in the cgo_harness module:
//   cd cgo_harness
//   go test . -run '^$' -tags treesitter_c_bench -bench BenchmarkCTreeSitter -benchmem

func makeGoBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 48)
	sb.WriteString("package main\n\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "func f%d() int { v := %d; return v }\n", i, i)
	}
	return []byte(sb.String())
}

func pointAtOffset(src []byte, offset int) gotreesitter.Point {
	var row uint32
	var col uint32
	for i := 0; i < offset && i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		i += size
	}
	return gotreesitter.Point{Row: row, Column: col}
}

func benchmarkFuncCount(b *testing.B) int {
	if raw := strings.TrimSpace(os.Getenv("GOT_BENCH_FUNC_COUNT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			return n
		}
		b.Fatalf("invalid GOT_BENCH_FUNC_COUNT=%q", raw)
	}
	if testing.Short() {
		return 100
	}
	return 500
}

func effectiveGLRMaxStacksForStats() int {
	const defaultGLRMaxStacks = 6
	raw := strings.TrimSpace(os.Getenv("GOT_GLR_MAX_STACKS"))
	if raw == "" {
		return defaultGLRMaxStacks
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultGLRMaxStacks
	}
	return n
}

func nonZeroBins(hist []uint64) string {
	var sb strings.Builder
	first := true
	for i, v := range hist {
		if v == 0 {
			continue
		}
		if !first {
			sb.WriteByte(',')
		}
		first = false
		fmt.Fprintf(&sb, "%d:%d", i, v)
	}
	if first {
		return "-"
	}
	return sb.String()
}

type editSite struct {
	offset int
	start  gotreesitter.Point
	end    gotreesitter.Point
}

func makeBenchmarkEditSites(src []byte, marker string) []editSite {
	needle := []byte(marker)
	sites := make([]editSite, 0, 64)
	from := 0
	for from < len(src) {
		idx := bytes.Index(src[from:], needle)
		if idx < 0 {
			break
		}
		offset := from + idx + len(marker)
		if offset >= len(src) {
			break
		}
		sites = append(sites, editSite{
			offset: offset,
			start:  pointAtOffset(src, offset),
			end:    pointAtOffset(src, offset+1),
		})
		from = offset + 1
	}
	return sites
}

func makeGoBenchmarkEditSites(src []byte) []editSite {
	return makeBenchmarkEditSites(src, "v := ")
}

func toggleDigitAt(src []byte, offset int) {
	if offset < 0 || offset >= len(src) {
		return
	}
	if src[offset] == '0' {
		src[offset] = '1'
		return
	}
	src[offset] = '0'
}

func prepareEditedBenchmarkSource(cur, scratch []byte, offset int) []byte {
	if len(scratch) != len(cur) {
		scratch = make([]byte, len(cur))
	}
	copy(scratch, cur)
	toggleDigitAt(scratch, offset)
	return scratch
}

func mustGoTokenSource(tb testing.TB, src []byte, lang *gotreesitter.Language) *grammars.GoTokenSource {
	tb.Helper()
	ts, err := grammars.NewGoTokenSource(src, lang)
	if err != nil {
		tb.Fatalf("NewGoTokenSource failed: %v", err)
	}
	return ts
}

func requireCompleteParse(tb testing.TB, tree *gotreesitter.Tree, src []byte, lang *gotreesitter.Language, phase string) *gotreesitter.Node {
	tb.Helper()
	if tree == nil {
		tb.Fatalf("%s parse returned nil tree", phase)
	}
	root := tree.RootNode()
	if root == nil {
		tb.Fatalf("%s parse returned nil root", phase)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		rt := tree.ParseRuntime()
		tb.Fatalf("%s parse truncated: root.EndByte=%d want=%d type=%q hasError=%v %s",
			phase, got, want, root.Type(lang), root.HasError(), rt.Summary())
	}
	return root
}

func BenchmarkGoParseFull(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	ts := mustGoTokenSource(b, src, lang)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts.Reset(src)
		tree, err := parser.ParseWithTokenSource(src, ts)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		requireCompleteParse(b, tree, src, lang, "full token-source")
		tree.Release()
	}
}

func BenchmarkGoParseFullDFA(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	statsEnabled := strings.TrimSpace(os.Getenv("GOT_STATS")) != ""
	if statsEnabled {
		gotreesitter.ResetArenaProfile()
		gotreesitter.ResetPerfCounters()
		gotreesitter.EnableArenaProfile(true)
		gotreesitter.EnableRuntimeAudit(true)
		defer gotreesitter.EnableArenaProfile(false)
		defer gotreesitter.EnableRuntimeAudit(false)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	var lastRuntime gotreesitter.ParseRuntime
	// The parse-gap harness gates public compatibility; this microbench tracks parser core.
	for i := 0; i < b.N; i++ {
		tree, err := parser.ParseNoResultCompatibilityBenchmarkOnly(src)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		requireCompleteParse(b, tree, src, lang, "full dfa")
		lastRuntime = tree.ParseRuntime()
		tree.Release()
	}
	if statsEnabled {
		a := gotreesitter.ArenaProfileSnapshot()
		p := gotreesitter.PerfCountersSnapshot()
		fmt.Printf(
			"STATS_CFG glr_max_stacks=%d default=6\n",
			effectiveGLRMaxStacksForStats(),
		)
		fmt.Printf(
			"STATS arena_full_acquire=%d arena_full_new=%d arena_inc_acquire=%d arena_inc_new=%d\n",
			a.FullAcquire, a.FullNew, a.IncrementalAcquire, a.IncrementalNew,
		)
		fmt.Printf(
			"STATS_PERF merge_calls=%d merge_dead_pruned=%d merge_perkey_overflow=%d merge_replacements=%d stackeq_calls=%d stackeq_true=%d stackeq_hash_miss_skips=%d stackcmp_calls=%d forks=%d first_conflict_token=%d max_stacks=%d lex_bytes=%d lex_tokens=%d\n",
			p.MergeCalls, p.MergeDeadPruned, p.MergePerKeyOverflow, p.MergeReplacements, p.StackEquivalentCalls, p.StackEquivalentTrue, p.StackEqHashMissSkips, p.StackCompareCalls, p.ForkCount, p.FirstConflictToken, p.MaxConcurrentStacks, p.LexBytes, p.LexTokens,
		)
		fmt.Printf(
			"STATS_PERF merge_hash_zero=%d global_cap_culls=%d global_cap_cull_dropped=%d\n",
			p.MergeHashZero, p.GlobalCapCulls, p.GlobalCapCullDropped,
		)
		fmt.Printf(
			"STATS_PERF conflicts_rr=%d conflicts_rs=%d conflicts_other=%d reduce_chain_steps=%d reduce_chain_max_len=%d reduce_chain_break_multi=%d reduce_chain_break_shift=%d reduce_chain_break_accept=%d\n",
			p.ConflictRR, p.ConflictRS, p.ConflictOther, p.ReduceChainSteps, p.ReduceChainMaxLen, p.ReduceChainBreakMulti, p.ReduceChainBreakShift, p.ReduceChainBreakAccept,
		)
		fmt.Printf(
			"STATS_PERF merge_in_hist=%s merge_alive_hist=%s merge_out_hist=%s fork_actions_hist=%s\n",
			nonZeroBins(p.MergeStacksInHist[:]), nonZeroBins(p.MergeAliveHist[:]), nonZeroBins(p.MergeOutHist[:]), nonZeroBins(p.ForkActionsHist[:]),
		)
		fmt.Printf(
			"STATS_PARSE nodes_new=%d children_ptrs=%d extras=%d errors=%d reuse_bytes=%d max_stacks=%d\n",
			lastRuntime.NodesAllocated, p.ParentChildPointers, p.ExtraNodes, p.ErrorNodes, p.ReuseNonLeafBytes, lastRuntime.MaxStacksSeen,
		)
		fmt.Printf(
			"STATS_RUNTIME single_iters=%d multi_iters=%d single_tokens=%d multi_tokens=%d gss_single=%d gss_multi=%d\n",
			lastRuntime.SingleStackIterations, lastRuntime.MultiStackIterations, lastRuntime.SingleStackTokens, lastRuntime.MultiStackTokens, lastRuntime.SingleStackGSSNodes, lastRuntime.MultiStackGSSNodes,
		)
		fmt.Printf(
			"STATS_SURVIVOR gss_alloc=%d gss_retained=%d gss_dropped=%d parent_alloc=%d parent_retained=%d parent_dropped=%d leaf_alloc=%d leaf_retained=%d leaf_dropped=%d merge_in=%d merge_out=%d merge_slots=%d cull_in=%d cull_out=%d\n",
			lastRuntime.GSSNodesAllocated, lastRuntime.GSSNodesRetained, lastRuntime.GSSNodesDroppedSameToken,
			lastRuntime.ParentNodesAllocated, lastRuntime.ParentNodesRetained, lastRuntime.ParentNodesDroppedSameToken,
			lastRuntime.LeafNodesAllocated, lastRuntime.LeafNodesRetained, lastRuntime.LeafNodesDroppedSameToken,
			lastRuntime.MergeStacksIn, lastRuntime.MergeStacksOut, lastRuntime.MergeSlotsUsed,
			lastRuntime.GlobalCullStacksIn, lastRuntime.GlobalCullStacksOut,
		)
	}
}

func BenchmarkGoParseIncrementalSingleByteEdit(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		b.Fatal("could not find edit marker")
	}
	editAt += len("v := ")
	start := pointAtOffset(src, editAt)
	end := pointAtOffset(src, editAt+1)

	ts := mustGoTokenSource(b, src, lang)
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  start,
		OldEndPoint: end,
		NewEndPoint: end,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	scratch := make([]byte, len(src))

	for i := 0; i < b.N; i++ {
		next := prepareEditedBenchmarkSource(src, scratch, editAt)
		tree.Edit(edit)
		ts.Reset(next)
		old := tree
		var err error
		tree, err = parser.ParseIncrementalWithTokenSource(next, tree, ts)
		if err != nil {
			b.Fatalf("incremental parse error: %v", err)
		}
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		if old != tree {
			old.Release()
		}
		src, scratch = next, src
	}
	tree.Release()
}

func BenchmarkGoParseIncrementalSingleByteEditDFA(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	statsEnabled := strings.TrimSpace(os.Getenv("GOT_STATS")) != ""
	if statsEnabled {
		gotreesitter.ResetArenaProfile()
		gotreesitter.ResetPerfCounters()
		gotreesitter.EnableArenaProfile(true)
		gotreesitter.EnableRuntimeAudit(true)
		defer gotreesitter.EnableArenaProfile(false)
		defer gotreesitter.EnableRuntimeAudit(false)
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
	singleStackIterations := 0
	multiStackIterations := 0
	var singleStackTokens uint64
	var multiStackTokens uint64
	var singleStackGSSNodes uint64
	var multiStackGSSNodes uint64
	var gssNodesAllocated uint64
	var gssNodesRetained uint64
	var gssNodesDropped uint64
	var parentNodesAllocated uint64
	var parentNodesRetained uint64
	var parentNodesDropped uint64
	var leafNodesAllocated uint64
	var leafNodesRetained uint64
	var leafNodesDropped uint64
	var mergeStacksIn uint64
	var mergeStacksOut uint64
	var mergeSlotsUsed uint64
	var globalCullStacksIn uint64
	var globalCullStacksOut uint64

	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		b.Fatal("could not find edit marker")
	}
	editAt += len("v := ")
	start := pointAtOffset(src, editAt)
	end := pointAtOffset(src, editAt+1)

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}
	if statsEnabled {
		gotreesitter.ResetPerfCounters()
		gotreesitter.ResetArenaProfile()
	}

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  start,
		OldEndPoint: end,
		NewEndPoint: end,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	scratch := make([]byte, len(src))

	for i := 0; i < b.N; i++ {
		next := prepareEditedBenchmarkSource(src, scratch, editAt)
		editStart := time.Time{}
		if statsEnabled {
			editStart = time.Now()
		}
		tree.Edit(edit)
		if statsEnabled {
			editTotalNS += uint64(time.Since(editStart).Nanoseconds())
		}
		old := tree
		if statsEnabled {
			var prof gotreesitter.IncrementalParseProfile
			tree, prof, err = parser.ParseIncrementalProfiled(next, tree)
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
			singleStackIterations += prof.SingleStackIterations
			multiStackIterations += prof.MultiStackIterations
			singleStackTokens += prof.SingleStackTokens
			multiStackTokens += prof.MultiStackTokens
			singleStackGSSNodes += prof.SingleStackGSSNodes
			multiStackGSSNodes += prof.MultiStackGSSNodes
			gssNodesAllocated += prof.GSSNodesAllocated
			gssNodesRetained += prof.GSSNodesRetained
			gssNodesDropped += prof.GSSNodesDroppedSameToken
			parentNodesAllocated += prof.ParentNodesAllocated
			parentNodesRetained += prof.ParentNodesRetained
			parentNodesDropped += prof.ParentNodesDroppedSameToken
			leafNodesAllocated += prof.LeafNodesAllocated
			leafNodesRetained += prof.LeafNodesRetained
			leafNodesDropped += prof.LeafNodesDroppedSameToken
			mergeStacksIn += prof.MergeStacksIn
			mergeStacksOut += prof.MergeStacksOut
			mergeSlotsUsed += prof.MergeSlotsUsed
			globalCullStacksIn += prof.GlobalCullStacksIn
			globalCullStacksOut += prof.GlobalCullStacksOut
			if prof.EntryScratchPeak > entryScratchPeak {
				entryScratchPeak = prof.EntryScratchPeak
			}
			if prof.MaxStacksSeen > maxStacksSeen {
				maxStacksSeen = prof.MaxStacksSeen
			}
		} else {
			tree, err = parser.ParseIncremental(next, tree)
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
		src, scratch = next, src
	}
	if statsEnabled {
		a := gotreesitter.ArenaProfileSnapshot()
		p := gotreesitter.PerfCountersSnapshot()
		fmt.Printf(
			"STATS_CFG glr_max_stacks=%d default=6\n",
			effectiveGLRMaxStacksForStats(),
		)
		fmt.Printf(
			"STATS edits=%d edit_ns=%d reuse_ns=%d parse_ns=%d reused_subtrees=%d reused_bytes=%d new_nodes=%d recover_searches=%d recover_state_checks=%d recover_state_skips=%d recover_lookups=%d recover_hits=%d max_stacks=%d\n",
			b.N, editTotalNS, reuseTotalNS, parseTotalNS, reusedSubtrees, reusedBytes, newNodesAllocated, recoverSearches, recoverStateChecks, recoverStateSkips, recoverLookups, recoverHits, maxStacksSeen,
		)
		fmt.Printf(
			"STATS recover_symbol_skips=%d\n",
			recoverSymbolSkips,
		)
		fmt.Printf(
			"STATS arena_full_acquire=%d arena_full_new=%d arena_inc_acquire=%d arena_inc_new=%d\n",
			a.FullAcquire, a.FullNew, a.IncrementalAcquire, a.IncrementalNew,
		)
		fmt.Printf(
			"STATS scratch_peak_entries=%d\n",
			entryScratchPeak,
		)
		fmt.Printf(
			"STATS_PERF merge_calls=%d merge_dead_pruned=%d merge_perkey_overflow=%d merge_replacements=%d stackeq_calls=%d stackeq_true=%d stackeq_hash_miss_skips=%d stackcmp_calls=%d forks=%d first_conflict_token=%d max_stacks=%d lex_bytes=%d lex_tokens=%d reuse_nodes_visited=%d reuse_nodes_pushed=%d reuse_nodes_popped=%d reuse_candidates=%d reuse_successes=%d reuse_leaf_successes=%d reuse_nonleaf_checks=%d reuse_nonleaf_successes=%d reuse_nonleaf_bytes=%d reuse_nonleaf_nogoto=%d reuse_nonleaf_nogoto_term=%d reuse_nonleaf_nogoto_nonterm=%d reuse_nonleaf_statemiss=%d reuse_nonleaf_statezero=%d\n",
			p.MergeCalls, p.MergeDeadPruned, p.MergePerKeyOverflow, p.MergeReplacements, p.StackEquivalentCalls, p.StackEquivalentTrue, p.StackEqHashMissSkips, p.StackCompareCalls, p.ForkCount, p.FirstConflictToken, p.MaxConcurrentStacks, p.LexBytes, p.LexTokens, p.ReuseNodesVisited, p.ReuseNodesPushed, p.ReuseNodesPopped, p.ReuseCandidatesChecked, p.ReuseSuccesses, p.ReuseLeafSuccesses, p.ReuseNonLeafChecks, p.ReuseNonLeafSuccesses, p.ReuseNonLeafBytes, p.ReuseNonLeafNoGoto, p.ReuseNonLeafNoGotoTerm, p.ReuseNonLeafNoGotoNt, p.ReuseNonLeafStateMiss, p.ReuseNonLeafStateZero,
		)
		fmt.Printf(
			"STATS_PERF merge_hash_zero=%d global_cap_culls=%d global_cap_cull_dropped=%d\n",
			p.MergeHashZero, p.GlobalCapCulls, p.GlobalCapCullDropped,
		)
		fmt.Printf(
			"STATS_PERF conflicts_rr=%d conflicts_rs=%d conflicts_other=%d reduce_chain_steps=%d reduce_chain_max_len=%d reduce_chain_break_multi=%d reduce_chain_break_shift=%d reduce_chain_break_accept=%d\n",
			p.ConflictRR, p.ConflictRS, p.ConflictOther, p.ReduceChainSteps, p.ReduceChainMaxLen, p.ReduceChainBreakMulti, p.ReduceChainBreakShift, p.ReduceChainBreakAccept,
		)
		fmt.Printf(
			"STATS_PERF merge_in_hist=%s merge_alive_hist=%s merge_out_hist=%s fork_actions_hist=%s\n",
			nonZeroBins(p.MergeStacksInHist[:]), nonZeroBins(p.MergeAliveHist[:]), nonZeroBins(p.MergeOutHist[:]), nonZeroBins(p.ForkActionsHist[:]),
		)
		fmt.Printf(
			"STATS_RUNTIME single_iters=%d multi_iters=%d single_tokens=%d multi_tokens=%d gss_single=%d gss_multi=%d\n",
			singleStackIterations, multiStackIterations, singleStackTokens, multiStackTokens, singleStackGSSNodes, multiStackGSSNodes,
		)
		fmt.Printf(
			"STATS_SURVIVOR gss_alloc=%d gss_retained=%d gss_dropped=%d parent_alloc=%d parent_retained=%d parent_dropped=%d leaf_alloc=%d leaf_retained=%d leaf_dropped=%d merge_in=%d merge_out=%d merge_slots=%d cull_in=%d cull_out=%d\n",
			gssNodesAllocated, gssNodesRetained, gssNodesDropped,
			parentNodesAllocated, parentNodesRetained, parentNodesDropped,
			leafNodesAllocated, leafNodesRetained, leafNodesDropped,
			mergeStacksIn, mergeStacksOut, mergeSlotsUsed,
			globalCullStacksIn, globalCullStacksOut,
		)
	}
	tree.Release()
}

func BenchmarkGoParseIncrementalNoEdit(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	ts := mustGoTokenSource(b, src, lang)

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts.Reset(src)
		old := tree
		tree, err = parser.ParseIncrementalWithTokenSource(src, tree, ts)
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
	tree.Release()
}

func BenchmarkGoParseIncrementalNoEditDFA(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
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
	tree.Release()
}

func BenchmarkGoParseIncrementalRandomSingleByteEdit(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	sites := makeGoBenchmarkEditSites(src)
	if len(sites) == 0 {
		b.Fatal("could not find random edit sites")
	}

	ts := mustGoTokenSource(b, src, lang)
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	seed := uint32(0x9e3779b9)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	scratch := make([]byte, len(src))

	for i := 0; i < b.N; i++ {
		seed = seed*1664525 + 1013904223
		site := sites[int(seed%uint32(len(sites)))]
		next := prepareEditedBenchmarkSource(src, scratch, site.offset)

		edit := gotreesitter.InputEdit{
			StartByte:   uint32(site.offset),
			OldEndByte:  uint32(site.offset + 1),
			NewEndByte:  uint32(site.offset + 1),
			StartPoint:  site.start,
			OldEndPoint: site.end,
			NewEndPoint: site.end,
		}

		tree.Edit(edit)
		ts.Reset(next)
		old := tree
		tree, err = parser.ParseIncrementalWithTokenSource(next, tree, ts)
		if err != nil {
			b.Fatalf("incremental parse error: %v", err)
		}
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		if old != tree {
			old.Release()
		}
		src, scratch = next, src
	}
	tree.Release()
}

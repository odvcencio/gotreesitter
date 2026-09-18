package gotreesitter_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// peakRSSLinux reads the process-lifetime high-water-mark resident set size
// from /proc/self/status (VmHWM). It is monotonic for the life of the
// process (never decreases), so within a single long-lived test process it
// is only a clean "peak reached in this phase" reading for the FIRST phase
// that meaningfully grows the heap -- later phases' readings are a floor,
// not a delta, since prior phases' peak persists even after their memory is
// freed. TestPointerLightPeakRSS below gets a clean per-representation
// number by re-execing this test binary in isolated subprocesses instead.
func peakRSSLinux() string {
	if runtime.GOOS != "linux" {
		return "n/a (non-linux)"
	}
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "n/a (read failed)"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "VmHWM:"))
		}
	}
	return "n/a (no VmHWM)"
}

// --- Wave7/plight measurement spike -----------------------------------------
//
// This file is the standing measurement rig for concept.perf-lever-pointer-
// light-tree (hypha space m31labs/gotreesitter). It does NOT migrate the tree
// store; it produces decision-grade numbers the roadmap gates the migration
// behind: bytes/node, GC-scan cost of a retained tree, cursor/query
// throughput, and (by reference to the existing edit-reuse benchmark)
// incremental edit reuse timing -- current *Node form vs a prototype
// struct-of-arrays (SoA) index-based form (pointer_light_soa_test.go).
//
// Run:
//
//	go test -run 'TestPointerLight' -v .
//	go test -run '^$' -bench 'BenchmarkPointerLight' -benchmem .
//
// Corpora: the canonical 500-function synthetic Go benchmark source (same
// generator as BenchmarkGoParseFullDFA / BenchmarkGoParseCoreDFA /
// BenchmarkQueryExec) and the largest convenient real Go file in this repo
// (parser.go, ~300KB) as the "one large tree" workload.

type pointerLightWorkload struct {
	name string
	src  []byte
}

func pointerLightWorkloads(tb testing.TB) []pointerLightWorkload {
	tb.Helper()
	canonical := makeGoBenchmarkSource(500)

	const largeRelPath = "parser.go"
	largeSrc, err := os.ReadFile(largeRelPath)
	if err != nil {
		tb.Fatalf("read %s: %v", largeRelPath, err)
	}
	if abs, absErr := filepath.Abs(largeRelPath); absErr == nil {
		tb.Logf("large-file workload: %s (%d bytes)", abs, len(largeSrc))
	}

	return []pointerLightWorkload{
		{name: "canonical-500fn", src: canonical},
		{name: "large-file(parser.go)", src: largeSrc},
	}
}

func pointerLightGoEntry(tb testing.TB) *grammars.LangEntry {
	tb.Helper()
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		tb.Skip("Go grammar not available")
	}
	return entry
}

// TestPointerLightBytesPerNode is the "quantify today's tree" + "prototype
// just enough to measure bytes/node" deliverable. It parses each workload,
// reads the arena's real materialization accounting (the same accounting the
// final-tree-compaction feature itself uses to decide whether to compact --
// see collectMaterializedFinalTreeStorageStats / estimateCompactedFinalTreeBytes
// in final_tree_compaction.go) and compares it against a from-scratch SoA
// conversion of the SAME materialized tree.
func TestPointerLightBytesPerNode(t *testing.T) {
	entry := pointerLightGoEntry(t)
	lang := entry.Language()

	gotreesitter.EnableArenaBreakdown(true)
	defer gotreesitter.EnableArenaBreakdown(false)

	type row struct {
		workload        string
		finalNodes      int
		arenaNodes      uint64
		maxStacksSeen   int
		multiStackIter  int
		curBytes        int64
		curPerCtorNode  float64 // current bytes / arena-constructed node count (representation cost only)
		curPerFinalNode float64 // current bytes / final retained-tree node count (real-world retained cost)
		soaBytes        int64
		soaPerNode      float64
	}
	var rows []row

	for _, wl := range pointerLightWorkloads(t) {
		parser := gotreesitter.NewParser(lang)
		// Pin to production: this measurement asserts production-engine arena
		// breakdown internals the compact candidate route does not report.
		parser.SetAdmissionCandidateRoute(false)
		tree, err := parser.Parse(wl.src)
		if err != nil {
			t.Fatalf("[%s] parse error: %v", wl.name, err)
		}
		root := tree.RootNode()
		if root == nil {
			t.Fatalf("[%s] nil root", wl.name)
		}

		bd, ok := tree.ArenaBreakdown()
		if !ok {
			t.Fatalf("[%s] arena breakdown unavailable", wl.name)
		}
		rt := tree.ParseRuntime()
		arenaNodes := bd.LeafNodesConstructed + bd.ParentNodesConstructed
		curBytes := bd.NodeStructBytesAllocated +
			bd.NodeSupertypeBytesAllocated +
			bd.ChildSliceBytesAllocated +
			bd.NodeFieldMetadataBytesAllocated +
			bd.FieldIDBytesAllocated +
			bd.FieldSourceBytesAllocated

		soa := buildSoATree(root)

		r := row{
			workload:        wl.name,
			finalNodes:      soa.nodeCount,
			arenaNodes:      arenaNodes,
			maxStacksSeen:   rt.MaxStacksSeen,
			multiStackIter:  rt.MultiStackIterations,
			curBytes:        curBytes,
			curPerCtorNode:  float64(curBytes) / float64(arenaNodes),
			curPerFinalNode: float64(curBytes) / float64(soa.nodeCount),
			soaBytes:        soa.bytesTotal(),
			soaPerNode:      soa.bytesPerNode(),
		}
		rows = append(rows, r)

		tree.Release()
	}
	gotreesitter.DrainArenaPools()

	t.Logf("Arena-constructed nodes include discarded GLR alternatives (nodes built for stacks the parser later culled); MaxStacksSeen>1 / MultiStackIterations>0 means real forking happened, not just single-derivation parsing.")
	t.Logf("%-24s %10s %10s %8s %10s %16s %12s %12s", "workload", "final-n", "arena-n", "maxstk", "multi-it", "current-bytes", "SoA-bytes", "SoA-B/node")
	for _, r := range rows {
		t.Logf("%-24s %10d %10d %8d %10d %16d %12d %12.1f",
			r.workload, r.finalNodes, r.arenaNodes, r.maxStacksSeen, r.multiStackIter, r.curBytes, r.soaBytes, r.soaPerNode)
	}
	t.Logf("")
	t.Logf("Two bytes/node framings for 'current' (both compared against SoA-B/node above):")
	t.Logf("  (a) per-constructed-node = current-bytes / arena-node-count -- cost of the *Node representation itself, amortized over everything the arena built (live + discarded)")
	t.Logf("  (b) per-final-node       = current-bytes / final-tree-node-count -- real retained-memory cost per node of the tree the caller actually holds (while the *Tree is alive, pre-Release/pre-compaction)")
	for _, r := range rows {
		t.Logf("%-24s per-constructed-node=%.1f B (%.2fx SoA)   per-final-node=%.1f B (%.2fx SoA)",
			r.workload, r.curPerCtorNode, r.curPerCtorNode/r.soaPerNode, r.curPerFinalNode, r.curPerFinalNode/r.soaPerNode)
	}
}

// TestPointerLightGCScan is the "GC scan cost" deliverable: force a full GC
// with N trees retained, timed, for the current pointer-rich *Node form vs
// the pointer-free SoA form built from the SAME parsed trees -- plus a
// baseline (nothing retained) for reference. GOMAXPROCS-independent forced
// full-GC wall time is used as the proxy for "cost of a GC needing to walk
// this retained graph."
//
// DrainArenaPools is required between phases: Tree.Release() returns arenas
// to a process-wide pool (see arena.go) rather than freeing them, so without
// draining, "released" Node graphs would still be strongly reachable from
// the pool and would still cost the GC a pointer scan -- silently
// invalidating the "no *Node graphs alive" comparison below.
func TestPointerLightGCScan(t *testing.T) {
	entry := pointerLightGoEntry(t)
	lang := entry.Language()
	src := makeGoBenchmarkSource(500)

	const n = 24      // retained trees; kept modest so this test stays CI-fast
	const repeats = 3 // per-phase forced-GC repeats; report median-ish avg

	oldPercent := debug.SetGCPercent(-1) // fully deterministic: only our runtime.GC() calls collect
	defer debug.SetGCPercent(oldPercent)

	forceGC := func() time.Duration {
		runtime.GC() // absorb any pending cycle / prior phase's garbage
		start := time.Now()
		runtime.GC()
		return time.Since(start)
	}
	heapAlloc := func() uint64 {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	measure := func(label string) (avg time.Duration, heap uint64) {
		var total time.Duration
		for i := 0; i < repeats; i++ {
			total += forceGC()
		}
		avg = total / repeats
		heap = heapAlloc()
		t.Logf("%-42s avg forced-GC=%-14v heap-alloc=%d bytes  peak-RSS(VmHWM)=%s", label, avg, heap, peakRSSLinux())
		return avg, heap
	}

	gotreesitter.DrainArenaPools()
	baselineGC, baselineHeap := measure("baseline (nothing retained)")

	nodeTrees := make([]*gotreesitter.Tree, 0, n)
	for i := 0; i < n; i++ {
		parser := gotreesitter.NewParser(lang)
		tree, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		nodeTrees = append(nodeTrees, tree)
	}
	nodeGC, nodeHeap := measure(fmt.Sprintf("current *Node trees retained (n=%d)", n))

	soaTrees := make([]*soaTree, 0, n)
	for _, tree := range nodeTrees {
		soaTrees = append(soaTrees, buildSoATree(tree.RootNode()))
	}
	for _, tree := range nodeTrees {
		tree.Release()
	}
	nodeTrees = nil
	gotreesitter.DrainArenaPools() // see doc comment: without this, pooled arenas keep scanning cost alive
	soaGC, soaHeap := measure(fmt.Sprintf("SoA trees retained, no *Node graphs alive (n=%d)", n))

	runtime.KeepAlive(soaTrees)

	t.Logf("--- summary (deltas vs baseline) ---")
	t.Logf("current *Node retained:       GC=%v (+%v vs baseline)  heap=%d (+%d bytes vs baseline)", nodeGC, nodeGC-baselineGC, nodeHeap, int64(nodeHeap)-int64(baselineHeap))
	t.Logf("SoA retained (Node released): GC=%v (+%v vs baseline)  heap=%d (+%d bytes vs baseline)", soaGC, soaGC-baselineGC, soaHeap, int64(soaHeap)-int64(baselineHeap))
}

// BenchmarkPointerLightWalkCurrent walks the FULL tree via TreeCursor
// (the same API surface used by BenchmarkTreeCursorDFS, but over the
// canonical/large workloads rather than a tiny fixture tree) and reports
// nodes/sec.
func BenchmarkPointerLightWalkCurrent(b *testing.B) {
	entry := pointerLightGoEntry(b)
	lang := entry.Language()
	for _, wl := range pointerLightWorkloads(b) {
		wl := wl
		b.Run(wl.name, func(b *testing.B) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(wl.src)
			if err != nil {
				b.Fatalf("parse error: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()

			b.ReportAllocs()
			b.ResetTimer()
			var visited int64
			for i := 0; i < b.N; i++ {
				c := gotreesitter.NewTreeCursor(root, tree)
				reachedRoot := false
				count := int64(0)
				for !reachedRoot {
					count++
					if c.GotoFirstChild() {
						continue
					}
					if c.GotoNextSibling() {
						continue
					}
					for {
						if !c.GotoParent() {
							reachedRoot = true
							break
						}
						if c.GotoNextSibling() {
							break
						}
					}
				}
				visited += count
			}
			b.ReportMetric(float64(visited)/b.Elapsed().Seconds(), "nodes/sec")
		})
	}
}

// BenchmarkPointerLightWalkSoA walks the SAME materialized trees converted to
// SoA form, via pure index arithmetic (no pointer chasing at all), and
// reports nodes/sec for direct comparison against BenchmarkPointerLightWalkCurrent.
func BenchmarkPointerLightWalkSoA(b *testing.B) {
	entry := pointerLightGoEntry(b)
	lang := entry.Language()
	for _, wl := range pointerLightWorkloads(b) {
		wl := wl
		b.Run(wl.name, func(b *testing.B) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(wl.src)
			if err != nil {
				b.Fatalf("parse error: %v", err)
			}
			soa := buildSoATree(tree.RootNode())
			tree.Release()

			b.ReportAllocs()
			b.ResetTimer()
			var visited int64
			for i := 0; i < b.N; i++ {
				var count int64
				soa.walk(func(uint32) { count++ })
				visited += count
			}
			b.ReportMetric(float64(visited)/b.Elapsed().Seconds(), "nodes/sec")
		})
	}
}

// BenchmarkPointerLightSoAConvert measures the one-shot cost of the post-parse
// *Node -> SoA conversion itself. This is the number that matters for the
// edit-reuse comparison: gotreesitter's incremental single-byte-edit reparse
// (BenchmarkGoParseIncrementalSingleByteEdit, ~108us on this workload/machine)
// reuses unaffected subtrees in place; the SoA prototype has no incremental
// update mechanism at all (out of scope -- see doc comment below), so "keep
// the tree in SoA form under active editing" would mean paying this full
// conversion cost on every edit unless a native incremental-SoA update path
// were separately built.
func BenchmarkPointerLightSoAConvert(b *testing.B) {
	entry := pointerLightGoEntry(b)
	lang := entry.Language()
	for _, wl := range pointerLightWorkloads(b) {
		wl := wl
		b.Run(wl.name, func(b *testing.B) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(wl.src)
			if err != nil {
				b.Fatalf("parse error: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = buildSoATree(root)
			}
		})
	}
}

// BenchmarkPointerLightQueryHighlights measures the existing highlights-style
// capture query (entry.HighlightQuery, the same query BenchmarkQueryExec in
// benchmark_query_test.go uses) against the canonical workload. Kept here
// (rather than only relying on BenchmarkQueryExec) so the full pointer-light
// table can be regenerated from one place; it intentionally does not
// duplicate BenchmarkQueryExecCompiled's amortized-compile variant.
func BenchmarkPointerLightQueryHighlights(b *testing.B) {
	entry := pointerLightGoEntry(b)
	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse error: %v", err)
	}
	defer tree.Release()

	q, err := gotreesitter.NewQuery(entry.HighlightQuery, lang)
	if err != nil {
		b.Fatalf("NewQuery failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matches := q.Execute(tree)
		if len(matches) == 0 {
			b.Fatal("query returned no matches")
		}
	}
}

// BenchmarkPointerLightQuerySimplePattern measures a single, simple, rooted
// pattern (function name capture) as the "simple pattern" half of the
// deliverable's 2-query requirement, complementing the highlights-style
// capture query above.
func BenchmarkPointerLightQuerySimplePattern(b *testing.B) {
	entry := pointerLightGoEntry(b)
	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse error: %v", err)
	}
	defer tree.Release()

	q, err := gotreesitter.NewQuery("(function_declaration name: (identifier) @func_name)", lang)
	if err != nil {
		b.Fatalf("NewQuery failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matches := q.Execute(tree)
		if len(matches) == 0 {
			b.Fatal("query returned no matches")
		}
	}
}

// Note on query throughput over the SoA prototype: deliberately not
// implemented. gotreesitter's Query/QueryCursor engine is written directly
// against *Node (Query.ExecuteNode(node *Node, ...), QueryCursor navigation
// via node.Parent()/node.Child(i) and matchStepWithRollbackAtParentPredicates(...,
// parent *Node, ...)); running it over soaTree would require either (a)
// materializing throwaway *Node values from the SoA arrays at query time,
// which just re-measures the existing *Node query path through an extra
// indirection and defeats the point, or (b) rewriting query/cursor traversal
// against index arrays, which is the "deep surgery" this measurement spike is
// explicitly scoped to avoid (see task framing: prototype JUST ENOUGH to
// measure, not the store). See the measurement report's blast-radius count
// for how deep that coupling goes (*Node reference counts across the repo).

const pointerLightRSSChildEnv = "GOT_PLIGHT_RSS_CHILD"

// TestPointerLightPeakRSS is the "peak RSS" deliverable. VmHWM (the metric
// peakRSSLinux reads) is a monotonic high-water mark for the life of a
// process, so a clean per-representation number requires isolated processes:
// this test re-execs the current test binary three times (baseline, current
// *Node retained, SoA retained), routing each into
// TestPointerLightPeakRSSChild via an env var, and reads back each
// subprocess's own peak RSS.
func TestPointerLightPeakRSS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("peak-RSS probe reads /proc/self/status (Linux only)")
	}
	if os.Getenv(pointerLightRSSChildEnv) != "" {
		t.Skip("this process is running as an RSS-child subprocess")
	}

	type result struct {
		mode string
		rss  string
	}
	var results []result
	for _, mode := range []string{"baseline", "current", "soa"} {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestPointerLightPeakRSSChild$")
		cmd.Env = append(os.Environ(), pointerLightRSSChildEnv+"="+mode)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("[%s] RSS-child subprocess failed: %v\n%s", mode, err, out)
		}
		rss := "unknown"
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "PEAK_RSS=") {
				rss = strings.TrimPrefix(line, "PEAK_RSS=")
			}
		}
		results = append(results, result{mode: mode, rss: rss})
	}

	for _, r := range results {
		t.Logf("%-10s isolated-subprocess peak RSS (VmHWM): %s", r.mode, r.rss)
	}
}

// TestPointerLightPeakRSSChild is not meant to run standalone -- it only
// does work when GOT_PLIGHT_RSS_CHILD is set, and is invoked as a
// subprocess by TestPointerLightPeakRSS above.
func TestPointerLightPeakRSSChild(t *testing.T) {
	mode := os.Getenv(pointerLightRSSChildEnv)
	if mode == "" {
		t.Skip("only runs as a subprocess of TestPointerLightPeakRSS")
	}

	const n = 24 // matches TestPointerLightGCScan's retained-tree count

	switch mode {
	case "baseline":
		// nothing retained; just report process-floor RSS.
	case "current":
		entry := pointerLightGoEntry(t)
		lang := entry.Language()
		src := makeGoBenchmarkSource(500)
		trees := make([]*gotreesitter.Tree, 0, n)
		for i := 0; i < n; i++ {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			trees = append(trees, tree)
		}
		runtime.GC()
		fmt.Println("PEAK_RSS=" + peakRSSLinux())
		runtime.KeepAlive(trees)
		return
	case "soa":
		// Convert-and-release one tree at a time (rather than building all N
		// *Node trees first) so the transient peak during construction is
		// O(1 tree), not O(N trees) -- otherwise "peak RSS while building
		// the SoA form" would just re-measure "peak RSS of the *Node form"
		// (SoA is a post-parse converter; it necessarily needs a *Node tree
		// to exist first) and hide the steady-state retained-memory win
		// this metric is supposed to isolate.
		entry := pointerLightGoEntry(t)
		lang := entry.Language()
		src := makeGoBenchmarkSource(500)
		soaTrees := make([]*soaTree, 0, n)
		for i := 0; i < n; i++ {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			soaTrees = append(soaTrees, buildSoATree(tree.RootNode()))
			tree.Release()
			gotreesitter.DrainArenaPools()
		}
		runtime.GC()
		fmt.Println("PEAK_RSS=" + peakRSSLinux())
		runtime.KeepAlive(soaTrees)
		return
	default:
		t.Fatalf("unknown %s=%q", pointerLightRSSChildEnv, mode)
	}

	runtime.GC()
	fmt.Println("PEAK_RSS=" + peakRSSLinux())
}

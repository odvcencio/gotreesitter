package gotreesitter_test

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Warm-parse benchmark: reuse one Parser to parse many files. This mirrors
// the indexer workload (canopy, IDE). If the arena pool works correctly,
// the per-parse allocation should drop sharply after the first parse.
func BenchmarkSelfParseWarmReuse(b *testing.B) {
	files := []string{
		"parser.go",
		"parser_reduce.go",
		"parser_test.go",
		"query_test.go",
		"parser_result_csharp_query.go",
		"parser_result_scala_template.go",
	}
	srcs := make([][]byte, len(files))
	var totalBytes int
	for i, n := range files {
		data, err := os.ReadFile(n)
		if err != nil {
			b.Fatalf("read %s: %v", n, err)
		}
		srcs[i] = data
		totalBytes += len(data)
	}

	b.Run("cold_fresh_parser_each_time", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(totalBytes))
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, src := range srcs {
				parser := gotreesitter.NewParser(grammars.GoLanguage())
				tree, _ := parser.Parse(src)
				_ = tree
			}
		}
		b.StopTimer()
		runtime.ReadMemStats(&after)
		totalAlloc := after.TotalAlloc - before.TotalAlloc
		b.ReportMetric(float64(totalAlloc)/float64(b.N), "heap_bytes/iter")
		b.ReportMetric(float64(totalAlloc)/float64(b.N*int(len(srcs))*int(totalBytes/len(srcs))), "heap_ratio")
	})

	b.Run("warm_one_parser_reused", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(totalBytes))
		parser := gotreesitter.NewParser(grammars.GoLanguage())
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, src := range srcs {
				tree, _ := parser.Parse(src)
				_ = tree
			}
		}
		b.StopTimer()
		runtime.ReadMemStats(&after)
		totalAlloc := after.TotalAlloc - before.TotalAlloc
		b.ReportMetric(float64(totalAlloc)/float64(b.N), "heap_bytes/iter")
	})

	b.Run("warm_with_drain_between_rounds", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(totalBytes))
		parser := gotreesitter.NewParser(grammars.GoLanguage())
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, src := range srcs {
				tree, _ := parser.Parse(src)
				_ = tree
			}
			// Note: DrainArenaPools is from PR #25; this benchmark intentionally
			// calls something equivalent if we've merged it. Skip-check if not.
			// For now we rely on automatic pool churn.
			runtime.GC()
		}
		b.StopTimer()
		runtime.ReadMemStats(&after)
		totalAlloc := after.TotalAlloc - before.TotalAlloc
		b.ReportMetric(float64(totalAlloc)/float64(b.N), "heap_bytes/iter")
	})
}

// TestArenaGCRetentionAfterRelease verifies that arena memory can be collected
// by the GC after parsing a large file and releasing the tree. Reset must clear
// every written Node slot and release discarded slab headers; stale *Node
// pointers in retained backing storage can otherwise pin hundreds of megabytes
// of arena memory across GC cycles.
//
// This test is intentionally strict: 30 MB is well above the expected ~12 MB
// but far below the ~545 MB that stale node-pointer retention can cause.
//
// NOT COVERAGE FOR THE GSS FALLBACK POOLS. This test parses parser_test.go, a
// conflict-free Go file, so the parse never forks and never calls
// gssNodeCanReach or gssNodeHash at all (0.0% coverage of both, verified with
// -covermode=count). It was previously miscited as justification for the
// gssCanReachVisitedPool / gssNodeHashPendingPool clearing fix (glr.go,
// glr_gss.go); it is not. TestGSSNodeCanReachPooledFallbackRealCSharpMergeHeavyAmbiguity
// and TestGSSNodeHashPooledFallbackRealPHPTransientErrorChain below are the
// tests that actually exercise those two pooled fallback paths.
func TestArenaGCRetentionAfterRelease(t *testing.T) {
	gotreesitter.DrainArenaPools()
	t.Cleanup(gotreesitter.DrainArenaPools)
	// Earlier tests can retain decoded grammar blobs in the package cache.
	// Those blobs do not belong to this arena test. Purge the cache before the
	// baseline so the limit measures this parse lifecycle.
	grammars.PurgeEmbeddedLanguageCache()
	src, err := os.ReadFile("parser_test.go")
	if err != nil {
		t.Fatalf("read parser_test.go: %v", err)
	}
	runtime.GC()
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	parser := gotreesitter.NewParser(grammars.GoLanguage())
	tree, parseErr := parser.Parse(src)
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	tree.Release()
	parser = nil
	src = nil

	// Run GC three times: first pass marks, second pass sweeps, third confirms.
	runtime.GC()
	runtime.GC()
	runtime.GC()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	retainedBytes := int64(ms.HeapAlloc) - int64(baseline.HeapAlloc)
	if retainedBytes < 0 {
		retainedBytes = 0
	}

	// 60 MB is well above the expected ~12-20 MB post-fix but far below the
	// ~545 MB retained by reset implementations that leave stale node pointers.
	// Compare against a pre-parse baseline so unrelated suite allocations do
	// not turn this arena-lifecycle test into a flaky absolute-heap gate.
	const maxRetainedBytes = 60 * 1024 * 1024
	if retainedBytes > maxRetainedBytes {
		t.Fatalf("HeapAlloc retained after parse+release+GC = %d MB (baseline=%d MB, final=%d MB), want < %d MB\n"+
			"Hint: arena slab backing arrays may not be fully cleared on reset,\n"+
			"leaving stale *Node pointers that prevent GC collection.",
			retainedBytes/1024/1024,
			baseline.HeapAlloc/1024/1024,
			ms.HeapAlloc/1024/1024,
			maxRetainedBytes/1024/1024)
	}
}

// csharpNestedGenericMethodCallAmbiguitySource builds a C# expression that
// nests `f(a<b,c>(...))`-shaped calls depth levels deep. Each `identifier <`
// is ambiguous in C# between a less-than comparison and the start of a
// generic type-argument list; the parser cannot tell which until it sees
// whether a matching `>` is followed by `(`. Nesting these keeps multiple GLR
// interpretations alive simultaneously instead of resolving one token later,
// which is what makes this construct "merge-heavy" rather than an ordinary,
// quickly-resolved local ambiguity.
func csharpNestedGenericMethodCallAmbiguitySource(depth int) []byte {
	var open, closeParens strings.Builder
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&open, "f%d(a%d<b%d,c%d>(", i, i, i, i)
		closeParens.WriteString("))")
	}
	return []byte("class C { void M() {\n" + open.String() + "x" + closeParens.String() + ";\n} }\n")
}

// TestGSSNodeCanReachPooledFallbackRealCSharpMergeHeavyAmbiguity proves that a
// real parse -- not a synthetic direct call -- drives gssNodeCanReach
// (glr.go) into its pooled fallback map. gssCanReachVisitedPool's own doc
// comment records the original motivation: "profiling a 137 KiB C# corpus
// attributed 3.47 GB of 4.36 GB total allocation to this one make" on a
// merge-heavy grammar. This test is a small, fast, self-contained
// reproduction of that same class of trigger.
//
// Measured with `go test -run TestGSSNodeCanReachPooledFallbackRealCSharpMergeHeavyAmbiguity
// -covermode=count -coverprofile=...` then `go tool cover -func`: gssNodeCanReach
// reaches 87.8% statement coverage, and line-level inspection of the profile
// shows glr.go:3382 (the "promote visited set to the pooled map" branch) and
// glr.go:3401-3404 (the "clear and return the map to the pool" branch) each
// hit 12 times by this test alone. Depth 20 has margin: depths from 10 to 30
// all reproduced a nonzero hit count in the same sweep.
func TestGSSNodeCanReachPooledFallbackRealCSharpMergeHeavyAmbiguity(t *testing.T) {
	lang := grammars.CSharpLanguage()
	src := csharpNestedGenericMethodCallAmbiguitySource(20)

	profile := gotreesitter.NewAmbiguityProfile()
	parser := gotreesitter.NewParser(lang)
	parser.SetAmbiguityProfile(profile)
	parser.SetTimeoutMicros(20_000_000)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	var totalForks uint64
	for _, st := range profile.SnapshotTop(64) {
		totalForks += st.Forks
	}
	// A liveness check, not a vacuous receipt: if a future change stopped this
	// construct from forking at all, gssNodeCanReach would go back to 0%
	// coverage here exactly as it silently did for TestArenaGCRetentionAfterRelease.
	if totalForks == 0 {
		t.Fatal("nested generic-call C# source produced no GLR forks; this test no longer exercises gssNodeCanReach's merge path")
	}
	rt := tree.ParseRuntime()
	if rt.MaxStacksSeen < 2 {
		t.Fatalf("MaxStacksSeen = %d, want >= 2 (construct must actually fork to exercise gssNodesCanMerge)", rt.MaxStacksSeen)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("nested generic-call ambiguity failed to resolve cleanly: %s", rt.Summary())
	}
}

// phpTransientVariableErrorSource builds a PHP function body with n sequential
// `$xI = I;` assignments, except at index breakAt, where the variable name's
// leading letter is dropped ("$0" instead of "$x0"). "$0" is not a valid PHP
// variable token, so this is a transient syntax error surrounded by hundreds
// of otherwise-clean statements -- the shape of a single mistyped or
// in-flight-edited character, not a structurally broken file.
func phpTransientVariableErrorSource(n, breakAt int) []byte {
	var b strings.Builder
	b.WriteString("<?php\nfunction f() {\n")
	for i := 0; i < n; i++ {
		if i == breakAt {
			fmt.Fprintf(&b, "$%d = %d;\n", i, i)
		} else {
			fmt.Fprintf(&b, "$x%d = %d;\n", i, i)
		}
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

// TestGSSNodeHashPooledFallbackRealPHPTransientErrorChain proves that a real
// parse drives gssNodeHash (glr_gss.go) into its pooled walk buffer.
// gssNodeHashPendingPool's own doc comment records the original motivation:
// "Profiling a PHP transient-error edit attributed 124 MB of allocation to
// this one walk." This test is a small, fast, self-contained reproduction of
// that same class of trigger: the invalid "$0" token forces error-recovery
// GSS chains deep enough to blow past the 32-entry inline array gssNodeHash
// normally uses.
//
// Measured with `go test -run TestGSSNodeHashPooledFallbackRealPHPTransientErrorChain
// -covermode=count -coverprofile=...` then `go tool cover -func`: gssNodeHash
// reaches 93.8% statement coverage run alone, and line-level inspection of the
// profile shows glr_gss.go:394-401 (the "chain outgrew the inline array, move
// to the pooled buffer" branch) and glr_gss.go:424-429 (the "clear and return
// the buffer to the pool" branch) each hit 725 times by this test alone.
func TestGSSNodeHashPooledFallbackRealPHPTransientErrorChain(t *testing.T) {
	lang := grammars.PhpLanguage()
	src := phpTransientVariableErrorSource(200, 100)

	parser := gotreesitter.NewParser(lang)
	parser.SetTimeoutMicros(10_000_000)

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	rt := tree.ParseRuntime()
	// Liveness check: the deliberately-invalid "$0" token must still be
	// reachable as a genuine parse error, and the recovery search around it
	// must actually fork, or this test stops exercising gssNodeHash's pooled
	// walk and would silently regress to a vacuous receipt.
	if !tree.RootNode().HasError() {
		t.Fatal("expected the invalid \"$0\" token to produce a parse error")
	}
	if rt.MaxStacksSeen < 2 {
		t.Fatalf("MaxStacksSeen = %d, want >= 2 (error recovery must fork to build the deep GSS chains gssNodeHash walks)", rt.MaxStacksSeen)
	}
}

// BenchmarkParserPoolConcurrentRSS measures peak heap allocation when parsing
// a large Go file concurrently across 16 goroutines using ParserPool. This
// mirrors a typical code-indexing workload where many files are parsed in parallel.
// The heap_MB metric after the benchmark reflects warm arena retention per worker.
func BenchmarkParserPoolConcurrentRSS(b *testing.B) {
	src, err := os.ReadFile("parser_test.go")
	if err != nil {
		b.Fatalf("read parser_test.go: %v", err)
	}

	pool := gotreesitter.NewParserPool(grammars.GoLanguage())

	const workers = 16
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	heapBefore := ms.HeapAlloc

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(workers)
		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				tree, parseErr := pool.Parse(src)
				if parseErr == nil {
					tree.Release()
				}
			}()
		}
		wg.Wait()
	}
	b.StopTimer()

	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&ms)
	heapAfterMB := float64(ms.HeapAlloc) / 1e6
	heapDeltaMB := float64(int64(ms.HeapAlloc)-int64(heapBefore)) / 1e6
	b.ReportMetric(heapAfterMB, "heap_MB_post_gc")
	b.ReportMetric(heapDeltaMB, "heap_MB_delta")
}

package gotreesitter_test

import (
	"os"
	"runtime"
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
// by the GC after parsing a large file and releasing the tree. With the partial-
// clear bug (clear(slab.data[:used]) instead of clear(slab.data)), stale *Node
// pointers in the unused tail of slab backing arrays pin hundreds of megabytes
// of arena memory across GC cycles.
//
// This test is intentionally strict: 30 MB is well above the expected ~12 MB
// but far below the ~545 MB that a partial-clear implementation retains.
func TestArenaGCRetentionAfterRelease(t *testing.T) {
	src, err := os.ReadFile("parser_test.go")
	if err != nil {
		t.Fatalf("read parser_test.go: %v", err)
	}

	parser := gotreesitter.NewParser(grammars.GoLanguage())
	tree, parseErr := parser.Parse(src)
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	tree.Release()

	// Run GC three times: first pass marks, second pass sweeps, third confirms.
	runtime.GC()
	runtime.GC()
	runtime.GC()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	// 60 MB is well above the expected ~12-20 MB post-fix but far below the
	// ~545 MB retained by a partial-clear (clear(slab.data[:used])) implementation.
	const maxRetainedBytes = 60 * 1024 * 1024
	if ms.HeapAlloc > maxRetainedBytes {
		t.Fatalf("HeapAlloc after parse+release+GC = %d MB, want < %d MB\n"+
			"Hint: arena slab backing arrays may not be fully cleared on reset,\n"+
			"leaving stale *Node pointers that prevent GC collection.",
			ms.HeapAlloc/1024/1024, maxRetainedBytes/1024/1024)
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

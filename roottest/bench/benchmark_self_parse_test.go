package bench_test

// Benchmarks that parse gotreesitter's own source files. These files are
// deliberately pathological: very long, dense switch/select bodies and
// enormous string literals containing deeply-nested S-expressions. Canopy
// OOM'd at ~3 GB while indexing this repo; these benchmarks measure whether
// our own runtime has the same latent pathology.
//
// Run with, e.g.:
//   go test -run=^$ -bench=BenchmarkSelfParse -benchmem -benchtime=3x \
//           -memprofile=/tmp/self_mem.out -memprofilerate=1 ./...
//
// The benchmark intentionally recreates the Parser each iteration so peak
// allocation is observed, not steady-state (this mirrors how a code indexer
// like canopy uses the parser).

import (
	"os"
	"runtime"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

var selfParseFiles = []string{
	"parser.go",                       // 82 KB real source, deep switch/select
	"parser_reduce.go",                // 67 KB real source
	"parser_test.go",                  // 175 KB test file
	"query_test.go",                   // 101 KB test file
	"parser_result_csharp_query.go",   // 41 KB largest S-expr literal
	"parser_result_scala_template.go", // 41 KB S-expr literal
}

func loadSelfFile(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		tb.Fatalf("read %s: %v", name, err)
	}
	return data
}

func BenchmarkSelfParse(b *testing.B) {
	for _, name := range selfParseFiles {
		name := name
		src := loadSelfFile(b, name)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))

			// Capture peak heap across the run using MemStats around the
			// first iteration (one-shot is enough because we're interested
			// in allocation during a single parse, not steady-state).
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)

			var rootHasError bool
			var rootChildren int

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				parser := gotreesitter.NewParser(grammars.GoLanguage())
				tree, err := parser.Parse(src)
				if err != nil {
					b.Fatalf("parse %s: %v", name, err)
				}
				root := tree.RootNode()
				rootHasError = root.HasError()
				rootChildren = int(root.ChildCount())
				// Let the parser/tree become unreachable before the next iter.
				_ = tree
				_ = parser
			}
			b.StopTimer()

			runtime.ReadMemStats(&after)
			// TotalAlloc - TotalAlloc gives bytes allocated across b.N iterations.
			totalBytes := after.TotalAlloc - before.TotalAlloc
			perIter := totalBytes / uint64(b.N)
			b.ReportMetric(float64(perIter), "heap_bytes/op")
			b.ReportMetric(float64(perIter)/float64(len(src)), "heap_ratio")
			b.ReportMetric(float64(after.HeapInuse), "heap_inuse_post")
			b.ReportMetric(float64(rootChildren), "root_children")
			if rootHasError {
				b.ReportMetric(1, "has_error")
			} else {
				b.ReportMetric(0, "has_error")
			}
		})
	}
}

// BenchmarkSelfParseSmoke is a one-shot non-benchmark smoke test: it parses
// every file once with a strict wall-time sanity check and reports the tree's
// error status. This is cheap enough to always run, and it answers "did the
// parser choke on any of these inputs the way canopy did?".
func BenchmarkSelfParseSmoke(b *testing.B) {
	for _, name := range selfParseFiles {
		src := loadSelfFile(b, name)
		parser := gotreesitter.NewParser(grammars.GoLanguage())
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("%s: parse error: %v", name, err)
		}
		root := tree.RootNode()
		b.Logf("%-40s size=%7d children=%4d hasError=%v",
			name, len(src), root.ChildCount(), root.HasError())
	}
}

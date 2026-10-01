package gotreesitter_test

import (
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// BenchmarkFreshGenerated measures a complete fresh parse and tree release.
// Source generation, grammar loading, and parser construction precede timing.
// Each language/size can run in its own process for CPU, allocation, and RSS
// profiling. The explicit route override keeps this a legacy-engine benchmark.
func BenchmarkFreshGenerated(b *testing.B) {
	for _, name := range []string{"go", "javascript", "typescript", "python", "rust", "java", "c", "cpp"} {
		b.Run(name, func(b *testing.B) {
			for _, size := range []int{137 << 10, 1 << 20} {
				b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
					source, _, err := benchfixtures.GeneratedSource(name, size)
					if err != nil {
						b.Fatal(err)
					}
					entry := grammars.DetectLanguageByName(name)
					parser := gotreesitter.NewParser(entry.Language())
					parser.SetAdmissionCandidateRoute(false)
					warm, err := parser.Parse(source)
					if err != nil {
						b.Fatal(err)
					}
					validateFreshGeneratedTree(b, warm, source)
					warm.Release()
					b.SetBytes(int64(len(source)))
					b.ReportAllocs()
					b.ResetTimer()
					var runtime gotreesitter.ParseRuntime
					for i := 0; i < b.N; i++ {
						tree, err := parser.Parse(source)
						if err != nil {
							b.Fatal(err)
						}
						validateFreshGeneratedTree(b, tree, source)
						runtime = tree.ParseRuntime()
						tree.Release()
					}
					b.StopTimer()
					b.ReportMetric(float64(runtime.TokensConsumed), "tokens/op")
					b.ReportMetric(float64(runtime.NodesAllocated), "nodes/op")
					b.ReportMetric(float64(runtime.MaxStacksSeen), "stacks/op")
				})
			}
		})
	}
}

func validateFreshGeneratedTree(b *testing.B, tree *gotreesitter.Tree, source []byte) {
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("fresh parse returned no root")
	}
	root := tree.RootNode()
	if root.HasError() || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) || tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
		b.Fatalf("incomplete fresh parse: root=%d..%d bytes=%d error=%t runtime=%s", root.StartByte(), root.EndByte(), len(source), root.HasError(), tree.ParseRuntime().Summary())
	}
}

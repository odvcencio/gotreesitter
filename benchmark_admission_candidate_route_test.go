//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// BenchmarkAdmissionCandidateGoQueryCompileWarmRoute measures the public,
// warmed compact admission route on a fixed Go source. The route counters are
// reported after the timed loop. They prove every measured parse used the
// compact scheduler and that no parse fell back to production.
func BenchmarkAdmissionCandidateGoQueryCompileWarmRoute(b *testing.B) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		b.Fatal(err)
	}
	var source []byte
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "query_compile" {
			source = fixture.Source
			break
		}
	}
	if len(source) == 0 {
		b.Fatal("query_compile benchmark fixture is missing")
	}

	parser := gotreesitter.NewParser(grammars.GoLanguage())
	parser.SetAdmissionCandidateRoute(true)
	gotreesitter.ResetAdmissionCandidateCountersForTest()
	warm, err := parser.Parse(source)
	if err != nil || warm == nil || warm.RootNode() == nil {
		b.Fatalf("compact warm parse failed: tree=%v err=%v", warm != nil, err)
	}
	warm.Release()
	if routed, fallback := gotreesitter.AdmissionCandidateCounters(); routed != 1 || fallback != 0 {
		b.Fatalf("compact warm parse did not route: routed=%d fallback=%d reason=%q", routed, fallback, gotreesitter.AdmissionCandidateLastFallbackReason())
	}

	gotreesitter.ResetAdmissionCandidateCountersForTest()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		tree, parseErr := parser.Parse(source)
		if parseErr != nil || tree == nil || tree.RootNode() == nil {
			b.Fatalf("compact timed parse failed: tree=%v err=%v", tree != nil, parseErr)
		}
		tree.Release()
	}
	b.StopTimer()
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	if routed != uint64(b.N) || fallback != 0 {
		b.Fatalf("compact timed loop changed route: routed=%d want=%d fallback=%d reason=%q", routed, b.N, fallback, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	b.ReportMetric(float64(routed)/float64(b.N), "candidate-routes/op")
	b.ReportMetric(float64(fallback)/float64(b.N), "candidate-fallbacks/op")
}

// BenchmarkAdmissionCandidateJavaScriptCleanWarmRoute measures a clean source
// that completes on the plain-first compact attempt. Recovery remains idle.
func BenchmarkAdmissionCandidateJavaScriptCleanWarmRoute(b *testing.B) {
	source := []byte("const x = (a) => a + 1;\nclass A { m() { return x(1) } }\n")
	parser := gotreesitter.NewParser(grammars.JavascriptLanguage())
	parser.SetAdmissionCandidateRoute(true)

	gotreesitter.ResetAdmissionCandidateCountersForTest()
	warm, err := parser.Parse(source)
	if err != nil || warm == nil || warm.RootNode() == nil {
		b.Fatalf("compact warm parse failed: tree=%v err=%v", warm != nil, err)
	}
	warm.Release()
	if routed, fallback := gotreesitter.AdmissionCandidateCounters(); routed != 1 || fallback != 0 {
		b.Fatalf("compact warm parse did not route: routed=%d fallback=%d reason=%q",
			routed, fallback, gotreesitter.AdmissionCandidateLastFallbackReason())
	}

	gotreesitter.ResetAdmissionCandidateCountersForTest()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		tree, parseErr := parser.Parse(source)
		if parseErr != nil || tree == nil || tree.RootNode() == nil {
			b.Fatalf("compact timed parse failed: tree=%v err=%v", tree != nil, parseErr)
		}
		tree.Release()
	}
	b.StopTimer()
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	if routed != uint64(b.N) || fallback != 0 {
		b.Fatalf("compact timed loop changed route: routed=%d want=%d fallback=%d reason=%q",
			routed, b.N, fallback, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	b.ReportMetric(float64(routed)/float64(b.N), "candidate-routes/op")
	b.ReportMetric(float64(fallback)/float64(b.N), "candidate-fallbacks/op")
}

//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// declineOverheadFixture is one input that the compact route declines.
type declineOverheadFixture struct {
	name     string
	language string
	path     string
}

// declineOverheadFixtures returns the R7 cliff fixtures that compact declines:
// C#, PHP, Elixir, and HTML. Set
// GTS_DECLINE_OVERHEAD_CORPUS to the output directory of
// `python3 scripts/benchfixture_corpus.py fetch --language L --output DIR` to
// add every fetched file as DIR/L/ROLE.
func declineOverheadFixtures(b *testing.B) []declineOverheadFixture {
	cliffs := filepath.Join("internal", "benchfixtures", "testdata", "cliffs")
	fixtures := []declineOverheadFixture{
		{"cliff_c_sharp", "c_sharp", filepath.Join(cliffs, "c_sharp_generated_32k.cs.gz")},
		{"cliff_php", "php", filepath.Join(cliffs, "php_run-tests.php.gz")},
		{"cliff_elixir", "elixir", filepath.Join(cliffs, "elixir_generated_32k.ex.gz")},
		{"cliff_html", "html", filepath.Join(cliffs, "html_go_mem.html.gz")},
	}
	root := os.Getenv("GTS_DECLINE_OVERHEAD_CORPUS")
	if root == "" {
		return fixtures
	}
	languages, err := os.ReadDir(root)
	if err != nil {
		b.Fatal(err)
	}
	for _, language := range languages {
		roles, err := os.ReadDir(filepath.Join(root, language.Name()))
		if err != nil {
			b.Fatal(err)
		}
		for _, role := range roles {
			fixtures = append(fixtures, declineOverheadFixture{
				name:     language.Name() + "_" + role.Name(),
				language: language.Name(),
				path:     filepath.Join(root, language.Name(), role.Name()),
			})
		}
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].name < fixtures[j].name })
	return fixtures
}

// readDeclineOverheadFixture reads a fixture file and decompresses a .gz file.
func readDeclineOverheadFixture(b *testing.B, path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	if filepath.Ext(path) != ".gz" {
		return data
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		b.Fatal(err)
	}
	data, err = io.ReadAll(reader)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

// BenchmarkAdmissionDeclineOverhead measures what a declined compact attempt
// adds to a parse. Each fixture runs on two warmed parsers: "legacy" forces
// the legacy route, and "candidate" forces the candidate route, which starts
// compact, declines, and falls back to legacy. The fallback overhead is
// candidate time divided by legacy time, minus one. A fixture that compact
// accepts is skipped, because it has no fallback.
func BenchmarkAdmissionDeclineOverhead(b *testing.B) {
	for _, fixture := range declineOverheadFixtures(b) {
		entry := grammars.DetectLanguageByName(fixture.language)
		if entry == nil {
			b.Fatalf("%s: unknown language %q", fixture.name, fixture.language)
		}
		lang := entry.Language()
		source := readDeclineOverheadFixture(b, fixture.path)
		for _, route := range []string{"legacy", "candidate"} {
			b.Run(fixture.name+"/"+route, func(b *testing.B) {
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(route == "candidate")
				gotreesitter.ResetAdmissionCandidateCountersForTest()
				warm, err := parser.Parse(source)
				if err != nil || warm == nil {
					b.Fatalf("warm parse failed: %v", err)
				}
				warm.Release()
				if routed, _ := gotreesitter.AdmissionCandidateCounters(); routed != 0 {
					b.Skip("compact accepts this fixture, so it has no fallback")
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				b.ResetTimer()
				for range b.N {
					tree, err := parser.Parse(source)
					if err != nil || tree == nil {
						b.Fatalf("parse failed: %v", err)
					}
					tree.Release()
				}
			})
		}
	}
}

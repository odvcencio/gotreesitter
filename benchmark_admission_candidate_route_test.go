//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// These benchmarks include declined compact attempts in route timing.
// Select exactly one language per process with GTS_ADMISSION_REAL_CORPUS_LANGS.
// The committed R4 source and its digest are shared with the invariant gate.
func BenchmarkAdmissionR4Legacy(b *testing.B)  { benchmarkAdmissionR4(b, false) }
func BenchmarkAdmissionR4Compact(b *testing.B) { benchmarkAdmissionR4(b, true) }

func benchmarkAdmissionR4(b *testing.B, compact bool) {
	name := strings.TrimSpace(os.Getenv("GTS_ADMISSION_REAL_CORPUS_LANGS"))
	if name == "" {
		b.Skip("set GTS_ADMISSION_REAL_CORPUS_LANGS to one R4 language")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil || strings.Contains(name, ",") {
		b.Fatal("select exactly one registered R4 language")
	}
	raw, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		b.Fatal(err)
	}
	var manifest v1InvariantCorpusManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		b.Fatal(err)
	}
	var source []byte
	for _, row := range manifest.Entries {
		if row.Language != name || row.Role != "sample" || row.CommittedPath == "" {
			continue
		}
		source, err = os.ReadFile(filepath.Join("internal", "benchfixtures", row.CommittedPath))
		if err != nil {
			b.Fatal(err)
		}
		sum := sha256.Sum256(source)
		if len(source) != row.Bytes || hex.EncodeToString(sum[:]) != row.SHA256 {
			b.Fatal("R4 sample identity changed")
		}
		break
	}
	if len(source) == 0 {
		b.Fatal("language has no committed real R4 sample")
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(compact)
	parse := func() (*gotreesitter.Tree, error) {
		if entry.TokenSourceFactory != nil {
			return parser.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
		}
		return parser.Parse(source)
	}
	warm, err := parse()
	if err != nil || warm == nil || warm.RootNode() == nil {
		b.Fatalf("warm parse: tree=%v err=%v", warm != nil, err)
	}
	warm.Release()
	gotreesitter.ResetAdmissionCandidateCountersForTest()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		tree, err := parse()
		if err != nil || tree == nil {
			b.Fatalf("timed parse: tree=%v err=%v", tree != nil, err)
		}
		tree.Release()
	}
	b.StopTimer()
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	b.ReportMetric(float64(routed)/float64(b.N), "compact-routes/op")
	b.ReportMetric(float64(fallback)/float64(b.N), "compact-fallbacks/op")
}

// BenchmarkAdmissionCandidateGoQueryCompileWarmRoute measures the public,
// warmed compact admission route on a fixed Go source. The route counters are
// reported after the timed loop. They prove every measured parse used the
// compact scheduler and that no parse fell back to production.
func BenchmarkAdmissionCandidateGoQueryCompileWarmRoute(b *testing.B) {
	benchmarkAdmissionCandidateGoQueryCompile(b, false)
}

// This is a fresh public Parser per file, with one shared immutable Language.
// It measures the setup cost paid by callers that do not keep a Parser pool.
func BenchmarkAdmissionCandidateGoQueryCompileNewParser(b *testing.B) {
	benchmarkAdmissionCandidateGoQueryCompile(b, true)
}

func benchmarkAdmissionCandidateGoQueryCompile(b *testing.B, newParser bool) {
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
		if newParser {
			parser = gotreesitter.NewParser(grammars.GoLanguage())
			parser.SetAdmissionCandidateRoute(true)
		}
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

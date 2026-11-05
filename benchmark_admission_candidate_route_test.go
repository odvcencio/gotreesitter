//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/benchorder"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// These benchmarks include declined compact attempts in route timing.
// Select exactly one language per process with GTS_ADMISSION_REAL_CORPUS_LANGS.
// The committed R4 source and its digest are shared with the invariant gate.
func BenchmarkAdmissionR4Legacy(b *testing.B)  { benchmarkAdmissionR4(b, false) }
func BenchmarkAdmissionR4Compact(b *testing.B) { benchmarkAdmissionR4(b, true) }

// BenchmarkAdmissionR4OwnedExternalLexer measures completed parses through the
// diagnostic ownership activation. Keep it separate from shipped-route timing.
func BenchmarkAdmissionR4OwnedExternalLexer(b *testing.B) {
	b.Cleanup(core.SetExternalLexerOwnershipProbeForTest(true))
	benchmarkAdmissionR4(b, true)
	if routed, fallback := gotreesitter.AdmissionCandidateCounters(); routed != uint64(b.N) || fallback != 0 {
		b.Fatalf("owned lexer benchmark did not stay on compact: routed=%d fallback=%d", routed, fallback)
	}
}

func admissionYAMLDocumentStreamSample(tb testing.TB) []byte {
	tb.Helper()
	sample, err := os.ReadFile("internal/benchfixtures/testdata/real/yaml")
	if err != nil {
		tb.Fatal(err)
	}
	sum := sha256.Sum256(sample)
	if len(sample) != 1998 || hex.EncodeToString(sum[:]) != "5aa2c70bccfe73254f13eca40476c3f88f522c6a47aea499f9d672dbc5aae8c4" {
		tb.Fatal("YAML R4 sample identity changed")
	}
	return sample
}

// Keep the original owned sample's work pinned while resolving wider forests.
func TestCompactYAMLReductionSelectionPreservesSampleWork(t *testing.T) {
	t.Cleanup(core.SetExternalLexerOwnershipProbeForTest(true))
	p := gotreesitter.NewParser(grammars.YamlLanguage())
	p.SetAdmissionCandidateRoute(true)
	source := admissionYAMLDocumentStreamSample(t)
	// Reuse the parser after a pressured session: the next fresh parse must
	// retain the original sample's work contract.
	document := append([]byte("---\n"), source...)
	document = append(document, '\n')
	warm, err := p.Parse(bytes.Repeat(document, 4))
	if err != nil || warm == nil || warm.RootNode() == nil {
		t.Fatalf("document stream: %v", err)
	}
	warm.Release()
	gotreesitter.ResetAdmissionCandidateCountersForTest()
	tree, err := p.Parse(source)
	if err != nil || tree == nil || tree.RootNode() == nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if routed, fallback := gotreesitter.AdmissionCandidateCounters(); routed != 1 || fallback != 0 {
		t.Fatalf("owned sample route=%d fallback=%d", routed, fallback)
	}
	if runtime := tree.ParseRuntime(); runtime.NodesAllocated > 484 || tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
		t.Fatalf("owned sample work or coverage changed: %+v", runtime)
	}
}

// Include the baseline's failed compact attempt and legacy fallback in timing.
// The locked-C regression pins the same 8 KiB source independently.
func BenchmarkAdmissionYAMLDocumentStreamOwned(b *testing.B) {
	b.Cleanup(core.SetExternalLexerOwnershipProbeForTest(true))
	document := append([]byte("---\n"), admissionYAMLDocumentStreamSample(b)...)
	document = append(document, '\n')
	source := bytes.Repeat(document, 4)
	sum := sha256.Sum256(source)
	if len(source) != 8012 || hex.EncodeToString(sum[:]) != "778d1dec01bef4da1cef26acff6b8012c9701a23af67bdb9511b419912f04400" {
		b.Fatal("YAML EOF witness identity changed")
	}
	p := gotreesitter.NewParser(grammars.YamlLanguage())
	p.SetAdmissionCandidateRoute(true)
	parse := func() {
		tree, err := p.Parse(source)
		if err != nil || tree == nil || tree.RootNode() == nil {
			b.Fatalf("parse: %v", err)
		}
		if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
			b.Fatal("document stream is incomplete or erroneous")
		}
		tree.Release()
	}
	parse()
	gotreesitter.ResetAdmissionCandidateCountersForTest()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		parse()
	}
	b.StopTimer()
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	if routed+fallback != uint64(b.N) {
		b.Fatal("inconsistent document-stream route counters")
	}
	b.ReportMetric(float64(routed)/float64(b.N), "compact-routes/op")
	b.ReportMetric(float64(fallback)/float64(b.N), "compact-fallbacks/op")
}

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

// BenchmarkCompactThroughputGo keeps the six Go qualification workloads in
// the same seeded order across revisions, including revisions that add tests.
// Run through scripts/run_randomized_benchmarks.sh with GTS_ADMISSION_CANDIDATE=1.
func BenchmarkCompactThroughputGo(b *testing.B) {
	if os.Getenv("GTS_ADMISSION_CANDIDATE") != "1" {
		b.Fatal("Go compact throughput suite requires GTS_ADMISSION_CANDIDATE=1")
	}
	cases := []benchorder.Case{
		{Label: "full_dfa", Run: BenchmarkGoParseFullDFA},
		{Label: "single_byte_edit_dfa", Run: BenchmarkGoParseIncrementalSingleByteEditDFA},
		{Label: "no_edit_dfa", Run: BenchmarkGoParseIncrementalNoEditDFA},
		{Label: "fresh_one_mib", Run: BenchmarkGoParseCompactFreshOneMiB},
	}
	for _, fixture := range benchfixtures.GoFullParseFixtures() {
		if fixture.ID != "query_compile" && fixture.ID != "grammargen_lr" {
			continue
		}
		cases = append(cases, benchorder.Case{Label: fixture.ID, Run: func(b *testing.B) {
			source, err := fixture.Load()
			if err != nil {
				b.Fatal(err)
			}
			benchmarkCompactThroughputSource(b, grammars.GoLanguage(), source, true)
		}})
	}
	benchorder.Run(b, cases, compactThroughputSeed(b))
}

// BenchmarkCompactThroughputYAML shuffles all six workload/mode pairs within
// each process. Diagnostic ownership stays separate in names and route counts.
func BenchmarkCompactThroughputYAML(b *testing.B) {
	sample := admissionYAMLDocumentStreamSample(b)
	document := append([]byte("---\n"), sample...)
	document = append(document, '\n')
	var cases []benchorder.Case
	for _, mode := range []struct {
		name             string
		candidate, owned bool
	}{{"legacy", false, false}, {"ordinary", true, false}, {"owned", true, true}} {
		for _, fixture := range []struct {
			name   string
			source []byte
		}{{"r4", sample}, {"eof8k", bytes.Repeat(document, 4)}} {
			cases = append(cases, benchorder.Case{Label: fixture.name + "_" + mode.name, Run: func(b *testing.B) {
				b.Cleanup(core.SetExternalLexerOwnershipProbeForTest(mode.owned))
				benchmarkCompactThroughputSource(b, grammars.YamlLanguage(), fixture.source, mode.candidate)
			}})
		}
	}
	benchorder.Run(b, cases, compactThroughputSeed(b))
}

func benchmarkCompactThroughputSource(b *testing.B, language *gotreesitter.Language, source []byte, candidate bool) {
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(candidate)
	parse := func() (gotreesitter.ParseRuntime, error) {
		tree, err := parser.Parse(source)
		if err != nil || tree == nil {
			return gotreesitter.ParseRuntime{}, fmt.Errorf("parse tree=%t: %w", tree != nil, err)
		}
		defer tree.Release()
		if tree.RootNode() == nil || tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
			return gotreesitter.ParseRuntime{}, fmt.Errorf("incomplete or erroneous parse")
		}
		return tree.ParseRuntime(), nil
	}
	warm, err := parse()
	if err != nil {
		b.Fatal(err)
	}
	beforeRoute, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		runtime, err := parse()
		if err != nil {
			b.Fatal(err)
		}
		if runtime.NodesAllocated != warm.NodesAllocated || runtime.TokensConsumed != warm.TokensConsumed {
			b.Fatal("per-parse work changed")
		}
	}
	b.StopTimer()
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	routed -= beforeRoute
	fallback -= beforeFallback
	if candidate && routed+fallback != uint64(b.N) {
		b.Fatal("inconsistent route totals")
	}
	if !candidate && routed+fallback != 0 {
		b.Fatal("legacy control entered candidate routing")
	}
	b.ReportMetric(float64(routed)/float64(b.N), "compact-routes/op")
	b.ReportMetric(float64(fallback)/float64(b.N), "compact-fallbacks/op")
	b.ReportMetric(float64(warm.NodesAllocated), "public-nodes/op")
	b.ReportMetric(float64(warm.TokensConsumed), "tokens/op")
}

// Read the campaign setting only in test code, outside every child timer.
func compactThroughputSeed(b *testing.B) int64 {
	b.Helper()
	seed, err := strconv.ParseInt(os.Getenv(benchorder.SeedEnv), 10, 64)
	if err != nil {
		b.Fatalf("%s must contain an explicit integer seed: %v", benchorder.SeedEnv, err)
	}
	return seed
}

func BenchmarkOrderedReceiptsGo(b *testing.B) {
	if os.Getenv("GTS_ADMISSION_CANDIDATE") != "1" {
		b.Fatal("requires compact route")
	}
	cases := []benchorder.Case{
		{Label: "full_dfa", Run: BenchmarkGoParseFullDFA},
		{Label: "full_dfa_3500", Run: func(b *testing.B) { b.Setenv("GOT_BENCH_FUNC_COUNT", "3500"); BenchmarkGoParseFullDFA(b) }},
		{Label: "single_byte_edit_dfa", Run: BenchmarkGoParseIncrementalSingleByteEditDFA},
		{Label: "no_edit_dfa", Run: BenchmarkGoParseIncrementalNoEditDFA},
		{Label: "fresh_one_mib", Run: BenchmarkGoParseCompactFreshOneMiB},
	}
	for _, fixture := range benchfixtures.GoFullParseFixtures() {
		if fixture.ID != "query_compile" && fixture.ID != "grammargen_lr" {
			continue
		}
		cases = append(cases, benchorder.Case{Label: fixture.ID, Run: func(b *testing.B) {
			source, err := fixture.Load()
			if err != nil {
				b.Fatal(err)
			}
			benchmarkCompactThroughputSource(b, grammars.GoLanguage(), source, true)
		}})
	}
	benchorder.Run(b, cases, compactThroughputSeed(b))
}

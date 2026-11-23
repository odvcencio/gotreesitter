//go:build linux && cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type cliffFixture struct {
	Name, Grammar, File, SHA256 string
}

var cliffFixtures = []cliffFixture{
	{"c_sharp", "c_sharp", "c_sharp_generated_32k.cs.gz", "b712fa3408c0d0c7d8ebd1c7919b47fe4a68281c0e61f5a7619a43004d6cdca9"},
	{"php", "php", "php_run-tests.php.gz", "f19d07587177c12822d604717f2474b91b57a7d7d3a4ca97227e824bc32a6084"},
	{"go", "go", "go_parser.go.gz", "ad0a0a8fce883ab86ec3d2bf1d8d8dcc1072534bff46ab26c0796795f7b3087a"},
	{"python", "python", "python_generated_32k.py.gz", "685ae2d152c1585e22fb875780b4831a5df6e162a0e5fdf55a3cd2dc7608d4f9"},
	{"elixir", "elixir", "elixir_generated_32k.ex.gz", "a8178ccc477384b9d71b6ab9df31fb12e306282abde68b7db1b5b32b494f76d8"},
	{"markdown", "markdown", "markdown_volumes.md.gz", "12f2cd6d66512b683ecd00d0b32cff41b70709068f691c063043e342c88ce193"},
	{"html", "html", "html_go_mem.html.gz", "561b50bca58e07455e993d68cf49281ed80df2150d09a38c77c906eabee36b7a"},
}

// cliffRoute is one Go route on one fixture. Forest marks a row that the GSS
// forest fast path produced. That path does not fill the work counters, so the
// row's Tokens, NewNodes, and Frontier carry no measurement.
type cliffRoute struct {
	Route         string                      `json:"route"`
	TreeSource    string                      `json:"tree_source"`
	Accepted      bool                        `json:"accepted"`
	Decline       string                      `json:"decline,omitempty"`
	Forest        bool                        `json:"forest_fast_path,omitempty"`
	Stop          gts.ParseStopReason         `json:"stop"`
	HasError      bool                        `json:"has_error"`
	RootEnd       uint32                      `json:"root_end"`
	TreeSHA256    string                      `json:"tree_sha256"`
	MatchesC      bool                        `json:"matches_locked_c_tree"`
	MatchesLegacy bool                        `json:"matches_legacy_tree"`
	Frontier      benchfixtures.CliffFrontier `json:"frontier"`
	Tokens        uint64                      `json:"tokens"`
	NewNodes      uint64                      `json:"new_nodes"`
	NSPerOp       int64                       `json:"ns_per_op"`
	AllocsPerOp   float64                     `json:"allocs_per_op"`
	BPerOp        uint64                      `json:"b_per_op"`
	GoC           float64                     `json:"go_c"`
	Failures      []string                    `json:"failures,omitempty"`
}

type cliffC struct {
	PrimaryProcess          benchfixtures.CliffFrontier `json:"primary_process_frontier"`
	LexedTokens             uint64                      `json:"lexed_tokens"`
	MultiVersionLexedTokens uint64                      `json:"multi_version_lexed_tokens"`

	Frontier    benchfixtures.CliffFrontier `json:"frontier"`
	HasError    bool                        `json:"has_error"`
	RootEnd     uint32                      `json:"root_end"`
	TreeSHA256  string                      `json:"tree_sha256"`
	NSPerOp     int64                       `json:"ns_per_op"`
	AllocsPerOp float64                     `json:"allocs_per_op"`
	BPerOp      uint64                      `json:"b_per_op"`
}

// cliffReport is the JSON row for one fixture. SourceKind and Source name where
// the bytes come from (see cliffInput). Revision is the commit that
// actions/checkout measured: the main commit for a scheduled run, the merge
// commit for a pull request. Event is the workflow trigger. OnDefaultBranch
// says whether the run measured the default branch, which a manual run can
// choose. The workflow passes all three, so a copied report says whether it
// describes main. A local run passes none of them, and the report omits them.
type cliffReport struct {
	Fixture         string       `json:"fixture"`
	Grammar         string       `json:"grammar"`
	SourceKind      string       `json:"source_kind"`
	Source          string       `json:"source"`
	SHA256          string       `json:"sha256"`
	Bytes           int          `json:"bytes"`
	Revision        string       `json:"revision,omitempty"`
	Event           string       `json:"event,omitempty"`
	OnDefaultBranch *bool        `json:"on_default_branch,omitempty"`
	COracleCommit   string       `json:"c_oracle_commit"`
	TokenSource     string       `json:"token_source,omitempty"`
	Seeds           int          `json:"seeds"`
	MaxRSSKiB       int64        `json:"max_rss_kib"`
	C               cliffC       `json:"c"`
	Routes          []cliffRoute `json:"routes"`
}

// TestCliffReport is the report-only cliff detector of the performance gate in docs/v1-design.md.
// Run one fixture per process in the locked C-oracle Docker harness.
// Set GTS_CLIFF_TOKEN_SOURCE=registered to measure the grammar's registered source.
func TestCliffReport(t *testing.T) {
	if os.Getenv("GTS_ADMISSION_CANDIDATE") != "1" {
		t.Fatal("set GTS_ADMISSION_CANDIDATE=1 for the candidate route")
	}
	selected := os.Getenv("GTS_CLIFF_LANGUAGE")
	if selected == "" {
		t.Skip("set GTS_CLIFF_LANGUAGE to one fixture name")
	}
	seeds := 20
	if raw := os.Getenv("GTS_CLIFF_SEEDS"); raw != "" {
		var err error
		seeds, err = strconv.Atoi(raw)
		if err != nil || seeds < 1 {
			t.Fatalf("invalid GTS_CLIFF_SEEDS %q", raw)
		}
	}
	var onDefaultBranch *bool
	if raw := os.Getenv("GTS_CLIFF_ON_DEFAULT_BRANCH"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			t.Fatalf("invalid GTS_CLIFF_ON_DEFAULT_BRANCH %q", raw)
		}
		onDefaultBranch = &value
	}
	for _, fixture := range cliffInputs() {
		if fixture.Name != selected {
			continue
		}
		source := loadCliffInput(t, fixture)
		goEntry := grammars.DetectLanguageByName(fixture.Grammar)
		if goEntry == nil {
			t.Fatalf("Go grammar %q unavailable", fixture.Grammar)
		}
		goLanguage := goEntry.Language()
		if goLanguage == nil {
			t.Fatalf("Go grammar %q unavailable", fixture.Grammar)
		}
		cLanguage, err := COracleLanguage(fixture.Grammar)
		if err != nil {
			t.Fatalf("locked C grammar %q: %v", fixture.Grammar, err)
		}
		c := measureCliffC(t, cLanguage, source)
		row := cliffReport{Fixture: fixture.Name, Grammar: fixture.Grammar, SourceKind: string(fixture.Kind), Source: fixture.Source,
			SHA256: fixture.SHA256, Bytes: len(source), Revision: os.Getenv("GTS_CLIFF_REVISION"), Event: os.Getenv("GTS_CLIFF_EVENT"),
			OnDefaultBranch: onDefaultBranch, COracleCommit: COracleRuntimeCommit, Seeds: seeds, C: c}
		parse := func(parser *gts.Parser) (*gts.Tree, error) { return parser.Parse(source) }
		switch row.TokenSource = os.Getenv("GTS_CLIFF_TOKEN_SOURCE"); row.TokenSource {
		case "":
		case "registered":
			if goEntry.TokenSourceFactory == nil {
				t.Fatalf("%s has no registered token source", fixture.Grammar)
			}
			parse = func(parser *gts.Parser) (*gts.Tree, error) {
				return parser.ParseWithTokenSource(source, goEntry.TokenSourceFactory(source, goLanguage))
			}
		default:
			t.Fatalf("unknown GTS_CLIFF_TOKEN_SOURCE %q", row.TokenSource)
		}
		for _, candidate := range []bool{false, true} {
			route := measureCliffGo(t, goLanguage, parse, candidate)
			route.Failures = benchfixtures.CliffFailures(route.Frontier, c.Frontier)
			row.Routes = append(row.Routes, route)
		}
		if len(row.Routes) == 2 {
			legacyDigest := row.Routes[0].TreeSHA256
			for index := range row.Routes {
				row.Routes[index].MatchesC = row.Routes[index].TreeSHA256 == row.C.TreeSHA256
				row.Routes[index].MatchesLegacy = row.Routes[index].TreeSHA256 == legacyDigest
			}
		}
		measureCliffTiming(t, &row, goLanguage, cLanguage, source, parse, seeds)
		var usage syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
			t.Fatal(err)
		}
		row.MaxRSSKiB = usage.Maxrss
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("CLIFF_JSON %s\n", encoded)
		if path := os.Getenv("GTS_CLIFF_REPORT_PATH"); path != "" {
			if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, route := range row.Routes {
			for _, failure := range route.Failures {
				t.Errorf("%s %s: %s", fixture.Name, route.Route, failure)
			}
		}
		return
	}
	t.Fatalf("unknown GTS_CLIFF_LANGUAGE %q", selected)
}

// Exercise the checked-in JSON certificate with the real report entry point.
func TestCliffReportMeasuresCertifiedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	t.Setenv("GTS_ADMISSION_CANDIDATE", "1")
	t.Setenv("GTS_CLIFF_LANGUAGE", "json_sample")
	t.Setenv("GTS_CLIFF_TOKEN_SOURCE", "")
	t.Setenv("GTS_CLIFF_SEEDS", "1")
	t.Setenv("GTS_CLIFF_REPORT_PATH", path)

	TestCliffReport(t)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report cliffReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Routes) != 2 || report.Routes[0].TreeSource != "legacy" || report.Routes[1].TreeSource != "ccore" {
		t.Fatalf("JSON report did not compare legacy and ccore: %+v", report.Routes)
	}
	route := report.Routes[1]
	if !route.Accepted || !route.MatchesC || !route.Frontier.Measured || route.Tokens == 0 || route.NSPerOp <= 0 {
		t.Fatalf("incomplete ccore report: %+v", route)
	}
}

func loadCliffSource(t *testing.T, fixture cliffFixture) []byte {
	t.Helper()
	sourcePath := filepath.Join("..", "internal", "benchfixtures", "testdata", "cliffs", fixture.File)
	archive, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	source, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(source)
	if got := hex.EncodeToString(digest[:]); got != fixture.SHA256 {
		t.Fatalf("fixture %s digest %s, want %s", fixture.Name, got, fixture.SHA256)
	}
	return source
}

func measureCliffC(t *testing.T, language *sitter.Language, source []byte) cliffC {
	t.Helper()
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	var max, primary, multiPrimary, lexed, multiLexed uint64
	var invalid bool
	parser.SetLogger(func(kind sitter.LogType, line string) {
		if kind != sitter.LogTypeParse {
			return
		}
		if strings.HasPrefix(line, "lexed_lookahead sym:") {
			width := parser.LogStackVersionCount()
			if width == 0 {
				invalid = true
				return
			}
			lexed++
			if width > 1 {
				multiLexed++
			}
			return
		}
		if !strings.HasPrefix(line, "process version:") {
			return
		}
		_, suffix, ok := strings.Cut(line, "version_count:")
		if !ok {
			invalid = true
			return
		}
		field, _, _ := strings.Cut(suffix, ",")
		count, err := strconv.ParseUint(field, 10, 64)
		if err != nil || count == 0 {
			invalid = true
			return
		}
		if count > max {
			max = count
		}
		if strings.HasPrefix(line, "process version:0,") {
			primary++
			if count > 1 {
				multiPrimary++
			}
		}
	})
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C oracle returned no root")
	}
	root := tree.RootNode()
	defer tree.Close()
	if invalid || primary == 0 || max == 0 || lexed == 0 {
		t.Fatalf("incomplete C fresh frontier: invalid=%t primary=%d max=%d tokens=%d", invalid, primary, max, lexed)
	}
	inspection, err := canonicalCTreeInspection(root)
	if err != nil {
		t.Fatal(err)
	}
	row := cliffC{
		Frontier:       benchfixtures.CliffFrontier{MaxLive: max, Measured: true, MultiShare: float64(multiLexed) / float64(lexed)},
		PrimaryProcess: benchfixtures.CliffFrontier{MaxLive: max, Measured: true, MultiShare: float64(multiPrimary) / float64(primary)},
		LexedTokens:    lexed, MultiVersionLexedTokens: multiLexed,
		HasError: root.HasError(), RootEnd: uint32(root.EndByte()), TreeSHA256: inspection.SHA256,
	}
	return row
}

func measureCliffGo(t *testing.T, language *gts.Language, parse func(*gts.Parser) (*gts.Tree, error), candidate bool) cliffRoute {
	t.Helper()
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(candidate)
	run := func() *gts.Tree {
		tree, err := parse(parser)
		if err != nil || tree == nil || tree.RootNode() == nil {
			t.Fatalf("Go route candidate=%t returned no root: %v", candidate, err)
		}
		return tree
	}
	_, freshBefore, _ := bind.RouteCounters()
	tree := run()
	_, freshAfter, _ := bind.RouteCounters()
	coreServed := freshAfter - freshBefore
	if coreServed != 0 && (!candidate || coreServed != 1) {
		t.Fatalf("candidate=%t routed %d fresh ccore parses, want at most one on the candidate route", candidate, coreServed)
	}
	if candidate && coreServed == 0 {
		// Compact telemetry would prevent ccore admission. Collect it only
		// after the unobserved request chose the compact candidate.
		tree.Release()
		parser.SetCompactCertificationTelemetry(true)
		gts.ResetAdmissionCandidateCounters()
		tree = run()
	}
	defer tree.Release()
	runtime := tree.ParseRuntime()
	root := tree.RootNode()
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatal(err)
	}
	row := cliffRoute{Route: "legacy", TreeSource: "legacy", Accepted: runtime.StopReason == gts.ParseStopAccepted,
		Stop: runtime.StopReason, HasError: root.HasError(), RootEnd: root.EndByte(), TreeSHA256: inspection.SHA256,
		Tokens: runtime.TokensConsumed, NewNodes: uint64(runtime.NodesAllocated), Forest: runtime.ForestFastPath}
	if candidate && coreServed == 0 {
		row.Route = "compact"
		row.TreeSource = "compact"
		routed, fallbacks := gts.AdmissionCandidateCounters()
		if routed == 0 && fallbacks == 0 {
			t.Fatal("compact route was not attempted")
		}
		if fallbacks > 0 {
			row.Decline = gts.AdmissionCandidateLastFallbackReason()
			row.TreeSource = "legacy_fallback"
			row.Accepted = false
			if row.Decline == "" {
				t.Fatal("compact declined without a named reason")
			}
			return row
		}
		row.Frontier = benchfixtures.CliffFrontier{MaxLive: runtime.CompactPeakHeaders, Measured: true}
		if row.Frontier.MaxLive == 0 {
			t.Fatal("compact route omitted its live-version peak")
		}
		if row.Tokens > 0 {
			row.Frontier.MultiShare = float64(runtime.CompactMultiHeaderTokens) / float64(row.Tokens)
		}
	} else {
		if coreServed == 1 {
			row.Route, row.TreeSource = "ccore", "ccore"
			if runtime.MaxStacksSeen == 0 || row.Tokens == 0 {
				t.Fatal("ccore route omitted its frontier counters")
			}
			if runtime.SingleStackTokens+runtime.MultiStackTokens != row.Tokens {
				t.Fatalf("ccore frontier counts do not cover %d tokens: %d single, %d multi",
					row.Tokens, runtime.SingleStackTokens, runtime.MultiStackTokens)
			}
		}
		row.Frontier = benchfixtures.CliffFrontier{MaxLive: uint64(runtime.MaxStacksSeen), Measured: coreServed == 1 || !runtime.ForestFastPath}
		if row.Tokens > 0 {
			row.Frontier.MultiShare = float64(runtime.MultiStackTokens) / float64(row.Tokens)
		}
	}
	return row
}

func measureCliffTiming(t *testing.T, row *cliffReport, goLanguage *gts.Language, cLanguage *sitter.Language, source []byte, parse func(*gts.Parser) (*gts.Tree, error), seeds int) {
	t.Helper()
	goParsers := [2]*gts.Parser{gts.NewParser(goLanguage), gts.NewParser(goLanguage)}
	goParsers[0].SetAdmissionCandidateRoute(false)
	goParsers[1].SetAdmissionCandidateRoute(true)
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	_, freshBefore, _ := bind.RouteCounters()
	var goCalls [2]int64
	goParse := func(index int) {
		goCalls[index]++
		tree, err := parse(goParsers[index])
		if err != nil || tree == nil {
			t.Fatalf("timed Go parse: %v", err)
		}
		tree.Release()
	}
	cParse := func() {
		tree := cParser.Parse(source, nil)
		if tree == nil {
			t.Fatal("timed C parse returned nil")
		}
		tree.Close()
	}
	row.Routes[0].AllocsPerOp = testing.AllocsPerRun(1, func() { goParse(0) })
	row.Routes[1].AllocsPerOp = testing.AllocsPerRun(1, func() { goParse(1) })
	row.C.AllocsPerOp = testing.AllocsPerRun(1, cParse)
	measureBytes := func(parse func()) uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for index := 0; index < 3; index++ {
			parse()
		}
		runtime.ReadMemStats(&after)
		return (after.TotalAlloc - before.TotalAlloc) / 3
	}
	row.Routes[0].BPerOp = measureBytes(func() { goParse(0) })
	row.Routes[1].BPerOp = measureBytes(func() { goParse(1) })
	row.C.BPerOp = measureBytes(cParse)
	var samples [3][]int64
	for seed := 0; seed < seeds; seed++ {
		first := rand.New(rand.NewSource(int64(seed + 1))).Intn(2)
		for _, route := range [2]int{first, 1 - first} {
			for _, engine := range [4]int{route, 2, 2, route} {
				start := time.Now()
				if engine == 2 {
					cParse()
				} else {
					goParse(engine)
				}
				samples[engine] = append(samples[engine], time.Since(start).Nanoseconds())
			}
		}
	}
	_, freshAfter, _ := bind.RouteCounters()
	var expectedCore int64
	if row.Routes[1].TreeSource == "ccore" {
		expectedCore = goCalls[1]
	}
	if freshAfter-freshBefore != expectedCore {
		t.Fatalf("measured %d fresh ccore parses, want %d for the reported route", freshAfter-freshBefore, expectedCore)
	}
	for index := range samples {
		sort.Slice(samples[index], func(a, b int) bool { return samples[index][a] < samples[index][b] })
	}
	row.Routes[0].NSPerOp = samples[0][len(samples[0])/2]
	row.Routes[1].NSPerOp = samples[1][len(samples[1])/2]
	row.C.NSPerOp = samples[2][len(samples[2])/2]
	for index := range row.Routes {
		row.Routes[index].GoC = float64(row.Routes[index].NSPerOp) / float64(row.C.NSPerOp)
	}
}
func TestCliffCSuccessfulLexFrontier(t *testing.T) {
	ruby := []byte("message = <<~TEXT\n  hello #{name}\nTEXT\nputs message\n")
	cs, _, err := benchfixtures.GeneratedSource("c_sharp", 32<<10)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		src          []byte
		total, multi uint64
	}{{"c_sharp", cs, 10700, 382}, {"ruby", ruby, 15, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			language, err := ParityCLanguage(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			row := measureCliffC(t, language, tc.src)
			if row.HasError || row.RootEnd != uint32(len(tc.src)) || !row.Frontier.Measured || row.LexedTokens != tc.total || row.MultiVersionLexedTokens != tc.multi || row.PrimaryProcess.MultiShare != 0 {
				t.Fatalf("C fresh frontier: %+v", row)
			}
		})
	}
}

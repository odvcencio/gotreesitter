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

type cliffRoute struct {
	Route       string                      `json:"route"`
	Accepted    bool                        `json:"accepted"`
	Decline     string                      `json:"decline,omitempty"`
	Stop        gts.ParseStopReason         `json:"stop"`
	HasError    bool                        `json:"has_error"`
	RootEnd     uint32                      `json:"root_end"`
	Frontier    benchfixtures.CliffFrontier `json:"frontier"`
	Tokens      uint64                      `json:"tokens"`
	NewNodes    uint64                      `json:"new_nodes"`
	NSPerOp     int64                       `json:"ns_per_op"`
	AllocsPerOp float64                     `json:"allocs_per_op"`
	BPerOp      uint64                      `json:"b_per_op"`
	GoC         float64                     `json:"go_c"`
	Failures    []string                    `json:"failures,omitempty"`
}

type cliffC struct {
	Frontier    benchfixtures.CliffFrontier `json:"frontier"`
	HasError    bool                        `json:"has_error"`
	RootEnd     uint32                      `json:"root_end"`
	NSPerOp     int64                       `json:"ns_per_op"`
	AllocsPerOp float64                     `json:"allocs_per_op"`
	BPerOp      uint64                      `json:"b_per_op"`
}

type cliffReport struct {
	Fixture       string       `json:"fixture"`
	Grammar       string       `json:"grammar"`
	SHA256        string       `json:"sha256"`
	Bytes         int          `json:"bytes"`
	COracleCommit string       `json:"c_oracle_commit"`
	Seeds         int          `json:"seeds"`
	MaxRSSKiB     int64        `json:"max_rss_kib"`
	C             cliffC       `json:"c"`
	Routes        []cliffRoute `json:"routes"`
}

// TestCliffReport is the report-only detector from the v1 design, R7 and P3.
// Run one grammar per process in the locked C-oracle Docker harness.
func TestCliffReport(t *testing.T) {
	if os.Getenv("GTS_ADMISSION_CANDIDATE") != "1" {
		t.Fatal("set GTS_ADMISSION_CANDIDATE=1 for the compact route")
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
	for _, fixture := range cliffFixtures {
		if fixture.Name != selected {
			continue
		}
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
		goEntry := grammars.DetectLanguageByName(fixture.Grammar)
		if goEntry == nil || goEntry.Language() == nil {
			t.Fatalf("Go grammar %q unavailable", fixture.Grammar)
		}
		cLanguage, err := COracleLanguage(fixture.Grammar)
		if err != nil {
			t.Fatalf("locked C grammar %q: %v", fixture.Grammar, err)
		}
		c := measureCliffC(t, cLanguage, source)
		row := cliffReport{Fixture: fixture.Name, Grammar: fixture.Grammar, SHA256: fixture.SHA256,
			Bytes: len(source), COracleCommit: COracleRuntimeCommit, Seeds: seeds, C: c}
		for _, candidate := range []bool{false, true} {
			route := measureCliffGo(t, goEntry.Language(), source, candidate)
			route.Failures = benchfixtures.CliffFailures(route.Frontier, c.Frontier)
			row.Routes = append(row.Routes, route)
		}
		measureCliffTiming(t, &row, goEntry.Language(), cLanguage, source, seeds)
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

func measureCliffC(t *testing.T, language *sitter.Language, source []byte) cliffC {
	t.Helper()
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	var max, total, multi uint64
	parser.SetLogger(func(kind sitter.LogType, line string) {
		if kind != sitter.LogTypeParse || !strings.HasPrefix(line, "process version:") {
			return
		}
		_, suffix, ok := strings.Cut(line, "version_count:")
		if !ok {
			return
		}
		field, _, _ := strings.Cut(suffix, ",")
		count, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return
		}
		if count > max {
			max = count
		}
		if strings.HasPrefix(line, "process version:0,") {
			total++
			if count > 1 {
				multi++
			}
		}
	})
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C oracle returned no root")
	}
	root := tree.RootNode()
	if total == 0 || max == 0 {
		t.Fatal("locked C parse logger did not report live versions")
	}
	row := cliffC{Frontier: benchfixtures.CliffFrontier{MaxLive: max, Measured: total > 0}, HasError: root.HasError(), RootEnd: uint32(root.EndByte())}
	if total > 0 {
		row.Frontier.MultiShare = float64(multi) / float64(total)
	}
	tree.Close()
	return row
}

func measureCliffGo(t *testing.T, language *gts.Language, source []byte, candidate bool) cliffRoute {
	t.Helper()
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(candidate)
	parser.SetCompactCertificationTelemetry(candidate)
	gts.ResetAdmissionCandidateCounters()
	tree, err := parser.Parse(source)
	if err != nil || tree == nil || tree.RootNode() == nil {
		t.Fatalf("Go route candidate=%t returned no root: %v", candidate, err)
	}
	defer tree.Release()
	runtime := tree.ParseRuntime()
	root := tree.RootNode()
	row := cliffRoute{Route: "legacy", Accepted: runtime.StopReason == gts.ParseStopAccepted,
		Stop: runtime.StopReason, HasError: root.HasError(), RootEnd: root.EndByte(),
		Tokens: runtime.TokensConsumed, NewNodes: uint64(runtime.NodesAllocated)}
	if candidate {
		row.Route = "compact"
		routed, fallbacks := gts.AdmissionCandidateCounters()
		if routed == 0 && fallbacks == 0 {
			t.Fatal("compact route was not attempted")
		}
		if fallbacks > 0 {
			row.Decline = gts.AdmissionCandidateLastFallbackReason()
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
		row.Frontier = benchfixtures.CliffFrontier{MaxLive: uint64(runtime.MaxStacksSeen), Measured: true}
		if row.Tokens > 0 {
			row.Frontier.MultiShare = float64(runtime.MultiStackTokens) / float64(row.Tokens)
		}
	}
	return row
}

func measureCliffTiming(t *testing.T, row *cliffReport, goLanguage *gts.Language, cLanguage *sitter.Language, source []byte, seeds int) {
	t.Helper()
	goParsers := [2]*gts.Parser{gts.NewParser(goLanguage), gts.NewParser(goLanguage)}
	goParsers[0].SetAdmissionCandidateRoute(false)
	goParsers[1].SetAdmissionCandidateRoute(true)
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	goParse := func(index int) {
		tree, err := goParsers[index].Parse(source)
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

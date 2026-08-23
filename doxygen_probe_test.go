package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestDoxygenA0RawProductionProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	lang := grammars.DoxygenLanguage()
	paths := []string{
		"testdata/dispatcher_census_a0/doxygen/medium__CMakeLists.txt",
		"testdata/dispatcher_census_a0/doxygen/medium__metrics.py",
		"testdata/dispatcher_census_a0/doxygen/small__example.cfg",
	}
	for _, rel := range paths {
		rel := rel
		t.Run(filepath.Base(rel), func(t *testing.T) {
			source, err := os.ReadFile(rel)
			if err != nil {
				t.Fatal(err)
			}
			probeDoxygenRawProduction(t, lang, source, rel)
		})
	}
}

func TestDoxygenPositiveRewriteProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	lang := grammars.DoxygenLanguage()
	for _, tc := range []struct{ name, source string }{
		{"registered_smoke", grammars.ParseSmokeSample("doxygen")},
		{"childless_error", "/** Adds all words in \\a s to document \\a doc with weight \\a wfd */"},
		{"recovered_document", "/**\\n * @param {int} value\\n * @brief Example\\n */"},
	} {
		t.Run(tc.name, func(t *testing.T) { probeDoxygenRawProduction(t, lang, []byte(tc.source), tc.name) })
	}
}

func TestDoxygenDirectArmMutationProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	lang := grammars.DoxygenLanguage()
	for _, tc := range []struct {
		name, source string
	}{
		{"childless_error", "/** Adds all words in \\a s to document \\a doc with weight \\a wfd */"},
		{"recovered_document", "/**\n * @param {int} value\n * @brief Example\n */"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawParser := gotreesitter.NewParser(lang)
			rawParser.SetAdmissionCandidateRoute(false)
			tree, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			before, after, checked, run, visited, rewritten := gotreesitter.DoxygenProbeNormalization(tree.RootNode(), []byte(tc.source), lang)
			t.Logf("source=%q before=%s after=%s checked=%d run=%d visited=%d rewritten=%d", tc.source, before, after, checked, run, visited, rewritten)
			if transcriptPath := os.Getenv("GTS_DISPATCHER_TRANSCRIPT_OUT"); transcriptPath != "" {
				transcript, readErr := os.ReadFile(transcriptPath)
				if readErr != nil {
					t.Fatalf("read dispatcher transcript: %v", readErr)
				}
				t.Logf("dispatcher_transcript=%s", transcript)
			}
		})
	}
}

func probeDoxygenRawProduction(t *testing.T, lang *gotreesitter.Language, source []byte, label string) {
	t.Helper()
	rawParser := gotreesitter.NewParser(lang)
	rawParser.SetAdmissionCandidateRoute(false)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse: %v", err)
	}
	t.Cleanup(raw.Release)
	productionParser := gotreesitter.NewParser(lang)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	t.Cleanup(production.Release)
	rawInspect, err := benchfixtures.InspectGoTree(raw.RootNode(), lang)
	if err != nil {
		t.Fatalf("raw inspect: %v", err)
	}
	productionInspect, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
	if err != nil {
		t.Fatalf("production inspect: %v", err)
	}
	rawRuntime, productionRuntime := raw.ParseRuntime(), production.ParseRuntime()
	rootRaw, rootProduction := raw.RootNode(), production.RootNode()
	t.Logf("label=%s bytes=%d source_sha256=%x raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d raw_passes=%d production_passes=%d raw_root=%s[%d,%d) raw_error=%t production_root=%s[%d,%d) production_error=%t raw_sexpr=%s production_sexpr=%s", label, len(source), sha256.Sum256(source), rawInspect.SHA256, productionInspect.SHA256, rawRuntime.NormalizationNodesRewritten, productionRuntime.NormalizationNodesRewritten, rawRuntime.NormalizationPassesRun, productionRuntime.NormalizationPassesRun, rootRaw.Type(lang), rootRaw.StartByte(), rootRaw.EndByte(), rootRaw.HasError(), rootProduction.Type(lang), rootProduction.StartByte(), rootProduction.EndByte(), rootProduction.HasError(), rootRaw.SExpr(lang), rootProduction.SExpr(lang))
}

func TestDoxygenA0RouteTrace(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	lang := grammars.DoxygenLanguage()
	paths := []string{
		"testdata/dispatcher_census_a0/doxygen/medium__CMakeLists.txt",
		"testdata/dispatcher_census_a0/doxygen/medium__metrics.py",
		"testdata/dispatcher_census_a0/doxygen/small__example.cfg",
	}
	for _, rel := range paths {
		rel := rel
		t.Run(filepath.Base(rel), func(t *testing.T) {
			source, err := os.ReadFile(rel)
			if err != nil {
				t.Fatal(err)
			}
			productionParser := gotreesitter.NewParser(lang)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)
			want, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			traceDoxygenRoutes(t, lang, source, want.SHA256)
		})
	}
}

func TestDoxygenHistoricalRouteTrace(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	lang := grammars.DoxygenLanguage()
	for _, tc := range []struct {
		name, source string
	}{
		{"childless_error", "/** Adds all words in \\a s to document \\a doc with weight \\a wfd */"},
		{"recovered_document", "/**\n * @param {int} value\n * @brief Example\n */"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			productionParser := gotreesitter.NewParser(lang)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			want, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("source_sha256=%x production_digest=%s production_sexpr=%s", sha256.Sum256([]byte(tc.source)), want.SHA256, production.RootNode().SExpr(lang))
			traceDoxygenRoutes(t, lang, []byte(tc.source), want.SHA256)
		})
	}
}

func traceDoxygenRoutes(t *testing.T, lang *gotreesitter.Language, source []byte, wantDigest string) {
	t.Helper()
	check := func(route string, tree *gotreesitter.Tree) {
		t.Helper()
		inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
		if err != nil {
			t.Fatalf("%s inspect: %v", route, err)
		}
		runtime := tree.ParseRuntime()
		var named []gotreesitter.NormalizationPassRuntime
		if runtime.NormalizationPasses != nil {
			named = *runtime.NormalizationPasses
		}
		t.Logf("route=%s digest=%s root=%s[%d,%d) error=%t passes=%d checked=%d visited=%d rewritten=%d forest=%t named=%+v", route, inspection.SHA256, tree.RootNode().Type(lang), tree.RootNode().StartByte(), tree.RootNode().EndByte(), tree.RootNode().HasError(), runtime.NormalizationPassesRun, runtime.NormalizationPassesChecked, runtime.NormalizationNodesVisited, runtime.NormalizationNodesRewritten, runtime.ForestFastPath, named)
		if inspection.SHA256 != wantDigest {
			t.Fatalf("%s digest=%s want production=%s", route, inspection.SHA256, wantDigest)
		}
		t.Cleanup(tree.Release)
	}
	compactParser := gotreesitter.NewParser(lang)
	compactParser.SetAdmissionCandidateRoute(true)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatalf("compact parse: %v", err)
	}
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	compactRoute := "compact-direct"
	if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
		compactRoute = "compact-fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
	}
	check(compactRoute, compact)
	forestParser := gotreesitter.NewParser(lang)
	forest, ok := forestParser.ParseForestExperimental(source)
	if ok && forest != nil {
		check("forest-direct", forest)
	} else {
		if forest != nil {
			forest.Release()
		}
		_, _, reason, _ := forestParser.ForestDeclineInfo()
		fallback, fallbackErr := forestParser.Parse(source)
		if fallbackErr != nil {
			t.Fatalf("forest fallback parse: %v", fallbackErr)
		}
		check("forest-fallback:"+reason, fallback)
	}
	if len(source) == 0 {
		return
	}
	oldSource := append([]byte(nil), source...)
	oldSource = append(oldSource, ' ')
	incrementalParser := gotreesitter.NewParser(lang)
	incrementalParser.SetAdmissionCandidateRoute(false)
	oldTree, err := incrementalParser.Parse(oldSource)
	if err != nil {
		t.Fatalf("incremental base parse: %v", err)
	}
	t.Cleanup(oldTree.Release)
	p := doxygenProbePointAtByte(source, len(source))
	oldTree.Edit(gotreesitter.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(oldSource)), NewEndByte: uint32(len(source)), StartPoint: p, OldEndPoint: doxygenProbePointAtByte(oldSource, len(oldSource)), NewEndPoint: p})
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	route := "incremental-fresh"
	if profile.ReuseUnsupported {
		route = "incremental-fallback:" + profile.ReuseUnsupportedReason
	} else if profile.OldTreeReuseRoute && profile.ReusedSubtrees > 0 {
		route = fmt.Sprintf("incremental-reuse:%d:%d", profile.ReusedSubtrees, profile.ReusedBytes)
	}
	check(route, incremental)
}

func doxygenProbePointAtByte(source []byte, offset int) gotreesitter.Point {
	var p gotreesitter.Point
	for i, b := range source {
		if i >= offset {
			break
		}
		if b == '\n' {
			p.Row++
			p.Column = 0
		} else {
			p.Column++
		}
	}
	return p
}

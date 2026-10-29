//go:build !grammar_subset

package grammars

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	ninjaA0InheritedFDSDigest  = "82f80fe2924d6ad679aad38ae8393ed6f33eba2bd7238dc0ce9b8d38b5b94d4c"
	ninjaA0LongSlowBuildDigest = "a0783ff8f7a1bf09509da3911f9c9540aab4b5930e6402c647b069cd3f990b6b"
)

// TestNinjaDispatchRetirementRoutes records exact A0 receipts for every Go
// route and accepts a documented compact or forest fallback.
func TestNinjaDispatchRetirementRoutes(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		wantDigest string
	}{
		{
			name:       "inherited-fds",
			file:       "small__inherited-fds.ninja",
			wantDigest: ninjaA0InheritedFDSDigest,
		},
		{
			name:       "long-slow-build",
			file:       "small__long-slow-build.ninja",
			wantDigest: ninjaA0LongSlowBuildDigest,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(
				"..", "testdata", "dispatcher_census_a0", "ninja", test.file,
			))
			if err != nil {
				t.Fatal(err)
			}
			runNinjaRouteReceipt(t, NinjaLanguage(), source, test.wantDigest)
		})
	}
}

func runNinjaRouteReceipt(
	t *testing.T,
	language *gotreesitter.Language,
	source []byte,
	wantDigest string,
) {
	t.Helper()

	rawParser := gotreesitter.NewParser(language)
	rawParser.SetAdmissionCandidateRoute(false)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse: %v", err)
	}
	t.Cleanup(raw.Release)
	if raw.ParseRuntime().ForestFastPath {
		t.Fatal("raw route reported the forest fast path")
	}
	assertNinjaRouteReceipt(t, "raw", raw, language, wantDigest)
	assertNinjaNoRewrites(t, "raw", raw)

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	t.Cleanup(production.Release)
	if production.ParseRuntime().ForestFastPath {
		t.Fatal("production route reported the forest fast path")
	}
	assertNinjaRouteReceipt(t, "production", production, language, wantDigest)
	assertNinjaNoRewrites(t, "production", production)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(language)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatalf("compact parse: %v", err)
	}
	t.Cleanup(compact.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	direct := routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore
	fallback := routedAfter == routedBefore && fallbackAfter == fallbackBefore+1
	if !direct && !fallback {
		t.Fatalf(
			"compact route counters routed=%d/%d fallback=%d/%d: %s",
			routedBefore,
			routedAfter,
			fallbackBefore,
			fallbackAfter,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
	compactRoute := "compact-direct"
	if fallback {
		compactRoute = "compact-fallback: " + gotreesitter.AdmissionCandidateLastFallbackReason()
	}
	assertNinjaRouteReceipt(t, compactRoute, compact, language, wantDigest)
	assertNinjaNoRewrites(t, "compact", compact)

	forestParser := gotreesitter.NewParser(language)
	forest, ok := forestParser.ParseForestExperimental(source)
	forestRoute := "forest"
	if ok && forest != nil {
		t.Cleanup(forest.Release)
		if !forest.ParseRuntime().ForestFastPath {
			t.Fatal("forest route returned a tree without the forest fast path")
		}
		assertNinjaRouteReceipt(t, forestRoute, forest, language, wantDigest)
		assertNinjaNoRewrites(t, "forest", forest)
	} else {
		if forest != nil {
			forest.Release()
			t.Fatal("forest route returned a tree with a decline")
		}
		_, _, reason, _ := forestParser.ForestDeclineInfo()
		if reason == "" {
			t.Fatal("forest declined without a reason")
		}
		forestRoute = "forest-fallback: " + reason
		forestFallback, fallbackErr := forestParser.Parse(source)
		if fallbackErr != nil {
			t.Fatalf("forest fallback parse: %v", fallbackErr)
		}
		t.Cleanup(forestFallback.Release)
		assertNinjaRouteReceipt(t, forestRoute, forestFallback, language, wantDigest)
		assertNinjaNoRewrites(t, "forest-fallback", forestFallback)
	}

	if len(source) == 0 || source[len(source)-1] != '\n' {
		t.Fatal("A0 Ninja witness must end with a newline for the incremental receipt")
	}
	oldSource := source[:len(source)-1]
	incrementalParser := gotreesitter.NewParser(language)
	incrementalParser.SetAdmissionCandidateRoute(false)
	oldTree, err := incrementalParser.Parse(oldSource)
	if err != nil {
		t.Fatalf("incremental base parse: %v", err)
	}
	t.Cleanup(oldTree.Release)
	startPoint := pointAtByte(oldSource, len(oldSource))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(oldSource)),
		OldEndByte:  uint32(len(oldSource)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  startPoint,
		OldEndPoint: startPoint,
		NewEndPoint: pointAtByte(source, len(source)),
	})
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	t.Cleanup(incremental.Release)
	incrementalRoute := "incremental-fresh"
	if profile.ReuseUnsupported {
		if profile.ReuseUnsupportedReason == "" {
			t.Fatalf("incremental fallback has no reason: %+v", profile)
		}
		incrementalRoute = "incremental-fallback: " + profile.ReuseUnsupportedReason
	} else if profile.OldTreeReuseRoute && profile.ReusedSubtrees > 0 && profile.ReusedBytes > 0 {
		incrementalRoute = "incremental-reuse"
	}
	assertNinjaRouteReceipt(t, incrementalRoute, incremental, language, wantDigest)
	assertNinjaNoRewrites(t, "incremental", incremental)
	t.Logf(
		"route receipt production=%s compact=%s forest=%s incremental=%s profile=%+v",
		wantDigest,
		compactRoute,
		forestRoute,
		incrementalRoute,
		profile,
	)
}

func assertNinjaRouteReceipt(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	wantDigest string,
) {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect %s route: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	t.Logf("%s digest=%s", route, inspection.SHA256)
}

func assertNinjaNoRewrites(t *testing.T, route string, tree *gotreesitter.Tree) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.NormalizationNodesRewritten != 0 {
		t.Fatalf("%s normalization rewrote %d nodes", route, runtime.NormalizationNodesRewritten)
	}
	t.Logf(
		"%s normalization checked=%d run=%d visited=%d rewritten=%d",
		route,
		runtime.NormalizationPassesChecked,
		runtime.NormalizationPassesRun,
		runtime.NormalizationNodesVisited,
		runtime.NormalizationNodesRewritten,
	)
}

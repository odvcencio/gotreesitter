//go:build !grammar_subset

package grammars

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	ledgerRecoveredDateSuffixDigest = "d89a89fd6a1397d5ccb1c9f38271dcef632e7bd4a8d307146483f180af66e173"
	ledgerYearDirectiveDigest       = "1d4aa453808e5f85eba81c33fff5bc91c217c9ec2cf66264bff2f8e4092a563b"
	ledgerA0NonProfitDigest         = "5b3390501b53e39e8753026802942fe91fc58c967268b03259207a25c304a5d5"
)

// TestLedgerDispatchRetirementRoutes records exact receipts for every Go route.
func TestLedgerDispatchRetirementRoutes(t *testing.T) {
	tests := []struct {
		name       string
		source     []byte
		file       string
		wantSource string
		wantDigest string
	}{
		{
			name:       "recovered-date-suffix",
			source:     []byte("\n15.03.2006 Exxon\n    Expenses:Auto:Gas          10,00 EUR\n    Liabilities:MasterCard    -10,00 EUR\n"),
			wantSource: "c9363c584f9bdb4cd3aa56a3fa6df98acc3a180c40e82ba3724d00efbc238210",
			wantDigest: ledgerRecoveredDateSuffixDigest,
		},
		{
			name: "year-directive",
			source: []byte(`
--input-date-format %d.%m

Y2010
03.01 * Foo
    A                 10.00 EUR
    B

05.02 * Bar
    A                 20.00 EUR
    B

test reg A
10-Jan-03 Foo                   A                         10.00 EUR    10.00 EUR
10-Feb-05 Bar                   A                         20.00 EUR    30.00 EUR
end test

`),
			wantSource: "531a9ddc9e5fe851095ed38b5dd896dc0de4a984f56ef0269c92254e5e84a02e",
			wantDigest: ledgerYearDirectiveDigest,
		},
		{
			name:       "a0-non-profit-test-data",
			file:       "small__non-profit-test-data.ledger",
			wantSource: "346d9eb6257c54d9fe53aabf9d3d32f8b8d5ebfece9f9142ebcfdedfa88cbbd2",
			wantDigest: ledgerA0NonProfitDigest,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := test.source
			if test.file != "" {
				var err error
				source, err = os.ReadFile(filepath.Join(
					"..", "testdata", "dispatcher_census_a0", "ledger", test.file,
				))
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != test.wantSource {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.wantSource)
			}
			runLedgerRouteReceipt(t, LedgerLanguage(), source, test.wantDigest)
		})
	}
}

func runLedgerRouteReceipt(
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
	assertLedgerRouteReceipt(t, "raw", raw, language, wantDigest)
	assertLedgerNoRewrites(t, "raw", raw)

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
	assertLedgerRouteReceipt(t, "production", production, language, wantDigest)
	assertLedgerNoRewrites(t, "production", production)

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
	assertLedgerRouteReceipt(t, compactRoute, compact, language, wantDigest)
	assertLedgerNoRewrites(t, "compact", compact)

	forestParser := gotreesitter.NewParser(language)
	forest, ok := forestParser.ParseForestExperimental(source)
	forestRoute := "forest"
	if ok && forest != nil {
		t.Cleanup(forest.Release)
		if !forest.ParseRuntime().ForestFastPath {
			t.Fatal("forest route returned a tree without the forest fast path")
		}
		assertLedgerRouteReceipt(t, forestRoute, forest, language, wantDigest)
		assertLedgerNoRewrites(t, "forest", forest)
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
		assertLedgerRouteReceipt(t, forestRoute, forestFallback, language, wantDigest)
		assertLedgerNoRewrites(t, "forest-fallback", forestFallback)
	}

	if len(source) == 0 || source[len(source)-1] != '\n' {
		t.Fatal("Ledger witness must end with a newline for the incremental receipt")
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
	assertLedgerRouteReceipt(t, incrementalRoute, incremental, language, wantDigest)
	assertLedgerNoRewrites(t, "incremental", incremental)
	t.Logf(
		"route receipt digest=%s compact=%s forest=%s incremental=%s profile=%+v",
		wantDigest,
		compactRoute,
		forestRoute,
		incrementalRoute,
		profile,
	)
}

func assertLedgerRouteReceipt(
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

func assertLedgerNoRewrites(t *testing.T, route string, tree *gotreesitter.Tree) {
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

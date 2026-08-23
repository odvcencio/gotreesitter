//go:build !grammar_subset

package grammars

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestDTDDispatchRouteReceipts(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	witnesses := []struct {
		name   string
		source []byte
		path   string
	}{
		{
			name:   "parser-produced-pe-reference-trigger",
			source: []byte("<!ELEMENT colspec %ho; EMPTY >"),
		},
		{
			name: "historical-medium-calstblx",
			path: filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "medium__calstblx.dtd"),
		},
		{
			name: "historical-large-dbits",
			path: filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "large__dbits.dtd"),
		},
		{
			name: "historical-large-docbook",
			path: filepath.Join("..", "testdata", "dispatcher_census_a0", "dtd", "large__docbook.dtd"),
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(witness.path)
				if err != nil {
					t.Fatal(err)
				}
			}

			language := DtdLanguage()
			raw, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse failed: %v", err)
			}
			t.Cleanup(raw.Release)
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}

			receipts := retiredDispatchRouteReceiptsAllowCompactAndForestFallbackExactSource(t, language, source)

			var productionDigest string
			for _, receipt := range receipts {
				inspection, inspectErr := benchfixtures.InspectGoTree(receipt.tree.RootNode(), language)
				if inspectErr != nil {
					t.Fatal(inspectErr)
				}
				if receipt.name == "production" {
					productionDigest = inspection.SHA256
				}
				runtime := receipt.tree.ParseRuntime()
				if runtime.NormalizationPasses != nil {
					for _, pass := range *runtime.NormalizationPasses {
						if pass.Name != "dispatch.dtd" {
							continue
						}
						t.Logf(
							"source_sha256=%x route=%s digest=%s dispatch.dtd checked=%d run=%d visited=%d rewritten=%d",
							sha256.Sum256(source),
							receipt.name,
							inspection.SHA256,
							pass.Checked,
							pass.Run,
							pass.NodesVisited,
							pass.NodesRewritten,
						)
						if pass.NodesRewritten != 0 {
							t.Fatalf("route %s rewrote %d nodes", receipt.name, pass.NodesRewritten)
						}
					}
				}
				if receipt.name == "incremental" {
					profile := receipt.incrementalProfile
					t.Logf(
						"source_sha256=%x route=incremental digest=%s reuse=%t reused_subtrees=%d reused_bytes=%d fresh_fallback=%t reason=%q",
						sha256.Sum256(source),
						inspection.SHA256,
						profile.OldTreeReuseRoute,
						profile.ReusedSubtrees,
						profile.ReusedBytes,
						profile.ReuseUnsupported,
						profile.ReuseUnsupportedReason,
					)
					if !profile.ReuseUnsupported && (!profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0) {
						t.Fatalf("incremental route has neither reuse nor documented fallback: %+v", profile)
					}
				}
				if receipt.name != "production" && productionDigest != "" && inspection.SHA256 != productionDigest {
					t.Fatalf("route %s digest=%s, want production %s", receipt.name, inspection.SHA256, productionDigest)
				}
			}
			if productionDigest == "" {
				t.Fatal("production receipt missing")
			}
			if rawInspection.SHA256 != productionDigest {
				t.Fatalf("raw digest=%s production=%s", rawInspection.SHA256, productionDigest)
			}
			t.Logf("source_bytes=%d raw_digest=%s production_digest=%s", len(source), rawInspection.SHA256, productionDigest)
		})
	}
}

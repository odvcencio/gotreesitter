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

// TestDoxygenDispatchBlockerRoutes records the exact Doxygen witnesses that
// keep dispatch.doxygen live. The A0 files stay rewrite-free. The historical
// triggers retain their observed rewrite counts on every covered route.
func TestDoxygenDispatchBlockerRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	witnesses := []struct {
		name             string
		path             string
		source           string
		sourceSHA256     string
		rawDigest        string
		productionDigest string
		routeRewrites    uint64
		wantArmPass      bool
	}{
		{
			name:             "a0_CMakeLists",
			path:             filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", "medium__CMakeLists.txt"),
			sourceSHA256:     "66408d6539b27d7c49b1e51777605c38c91b6d924267db5109ee00e2a1cfcf41",
			rawDigest:        "a206903ee351591886014cb963d527769fc710d513af7b84c6dba9d9cc77cd2b",
			productionDigest: "a206903ee351591886014cb963d527769fc710d513af7b84c6dba9d9cc77cd2b",
			routeRewrites:    0,
			wantArmPass:      true,
		},
		{
			name:             "a0_metrics",
			path:             filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", "medium__metrics.py"),
			sourceSHA256:     "31622a6c075ffa6f78a16af6e379f517213d42ff67729bbd0d10551c5fca9702",
			rawDigest:        "5adbacb1ec949237a802a56a5c95c3c7a1ce17fe9c8db5423b63f083da62d5d1",
			productionDigest: "5adbacb1ec949237a802a56a5c95c3c7a1ce17fe9c8db5423b63f083da62d5d1",
			routeRewrites:    0,
			wantArmPass:      false,
		},
		{
			// The tag_name absorbed at the top level no longer carries an
			// error bit (pushOrExtendErrorNode now matches C's
			// ts_subtree_error_cost: only the ERROR container is
			// erroneous). expectedRootCanFrameRecoveredFragments used that
			// bit to recognize the token as recovery debris it could elide
			// from the "document" root; without it, the token no longer
			// replays as valid top-level content, so the root keeps the
			// ERROR wrapper: document(ERROR[extra](tag_name)) instead of the
			// former document(tag_name). Both digests move together because
			// the arm is a no-op on this shape either way.
			name:             "a0_example_cfg",
			path:             filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", "small__example.cfg"),
			sourceSHA256:     "86998161914382f8152e4984db091e7bf486799c1091fc6c57db4e704eee4a3b",
			rawDigest:        "b9bb1f5701ae912a89cde155e0b18ba39bbd6db5619b7d1b5d52a4b079e6219b",
			productionDigest: "b9bb1f5701ae912a89cde155e0b18ba39bbd6db5619b7d1b5d52a4b079e6219b",
			routeRewrites:    0,
			wantArmPass:      true,
		},
		{
			name:             "historical_childless_error",
			source:           "/** Adds all words in \\a s to document \\a doc with weight \\a wfd */",
			sourceSHA256:     "ff90d209911d0d32bf44ebff0742e6f42ff40a6f4978860a00ec3f7228b2af24",
			rawDigest:        "6c16ff1b99a3b116d575f90aa0fe5456381b442a58af021dac36e6954345ce4c",
			productionDigest: "0e1129b2130636e62dd05b2494c22a9a2b5b6ec044aea2eeb4dc836380e38b38",
			routeRewrites:    3,
			wantArmPass:      true,
		},
		{
			// One of the recovered @-tag spans absorbs a bare tag_name token
			// directly under a nested ERROR (the same
			// pushOrExtendErrorNode shape as a0_example_cfg above); its
			// absorbed leaf no longer carries an error bit, so both digests
			// move.
			name:             "historical_recovered_document",
			source:           "/**\n * @param {int} value\n * @brief Example\n */",
			sourceSHA256:     "f6deae068bcf0fe684f8623d671ee5dfbfab47c93d7827ec03c3b4b5330f8309",
			rawDigest:        "131d2425cc4bbd59336c92a6f728d15142cf72940dc0b92426e26476fcb5d1c1",
			productionDigest: "df23e9820913fced85dd38bd693bbb6bc746b0a65c9b4dd2e3812e83ff9df5f0",
			routeRewrites:    16,
			wantArmPass:      true,
		},
	}

	language := DoxygenLanguage()
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := []byte(witness.source)
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(witness.path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.sourceSHA256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, witness.sourceSHA256)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)
			assertDoxygenRouteReceipt(t, "raw", raw, language, witness.rawDigest, 0, false, 0)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)
			assertDoxygenRouteReceipt(t, "production", production, language, witness.productionDigest, 0, witness.wantArmPass, witness.routeRewrites)

			routeReceipts := retiredDispatchRouteReceiptsAllowCompactAndForestFallbackExactSource(t, language, source)
			for _, receipt := range routeReceipts {
				assertDoxygenRouteReceipt(t, receipt.name, receipt.tree, language, witness.productionDigest, 0, witness.wantArmPass, witness.routeRewrites)
				if receipt.name == "incremental" {
					profile := receipt.incrementalProfile
					if !profile.ReuseUnsupported && (!profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0) {
						t.Fatalf("incremental route has neither reuse nor documented fallback: %+v", profile)
					}
					t.Logf(
						"incremental reuse=%t reused_subtrees=%d reused_bytes=%d fallback=%t reason=%q",
						profile.OldTreeReuseRoute,
						profile.ReusedSubtrees,
						profile.ReusedBytes,
						profile.ReuseUnsupported,
						profile.ReuseUnsupportedReason,
					)
				}
			}
		})
	}
}

func assertDoxygenRouteReceipt(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	wantDigest string,
	wantTotalRewrites uint64,
	wantArmPass bool,
	wantArmRewrites uint64,
) {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect %s route: %v", route, err)
	}
	runtime := tree.ParseRuntime()
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest = %s, want %s", route, inspection.SHA256, wantDigest)
	}
	if runtime.NormalizationNodesRewritten != wantTotalRewrites {
		t.Fatalf("%s total rewrites = %d, want %d", route, runtime.NormalizationNodesRewritten, wantTotalRewrites)
	}

	var pass *gotreesitter.NormalizationPassRuntime
	if runtime.NormalizationPasses != nil {
		for index := range *runtime.NormalizationPasses {
			candidate := &(*runtime.NormalizationPasses)[index]
			if candidate.Name == "dispatch.doxygen" {
				pass = candidate
				break
			}
		}
	}
	if wantArmPass && pass == nil {
		t.Fatalf("%s route did not record dispatch.doxygen", route)
	}
	if !wantArmPass && pass != nil {
		t.Fatalf("%s route unexpectedly recorded dispatch.doxygen: %+v", route, *pass)
	}
	if pass != nil && pass.NodesRewritten != wantArmRewrites {
		t.Fatalf("%s dispatch.doxygen rewrites = %d, want %d", route, pass.NodesRewritten, wantArmRewrites)
	}
	t.Logf(
		"route=%s source_digest=%s root=%s[%d,%d) error=%t total_rewrites=%d dispatch_doxygen=%+v",
		route,
		inspection.SHA256,
		tree.RootNode().Type(language),
		tree.RootNode().StartByte(),
		tree.RootNode().EndByte(),
		tree.RootNode().HasError(),
		runtime.NormalizationNodesRewritten,
		pass,
	)
}

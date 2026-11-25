//go:build gts_parsercorephase0

package admission_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// This file is gts_parsercorephase0-tagged (positive opt-in, matching
// admission_scorecard_test.go and the other large-grammar-loading tests),
// not !gts_no_parsercorephase0 like admission_route_equality_leaf_tiling_test.go.
// Loading the swift grammar alone -- regardless of route, verified with the
// production route too -- retains roughly 20 MB past a full
// DrainArenaPools+PurgeEmbeddedLanguageCache+runtime.GC() cycle. That is a
// pre-existing characteristic reproduced identically on origin/main with
// none of this tranche's changes present; it is unrelated to the B1 tiling
// fix. No always-on (untagged) test loads swift today, so
// TestArenaGCRetentionAfterRelease (benchmark_warm_reuse_test.go) has never
// observed it. Keeping this witness pin out of the untagged suite avoids
// tripping that whole-process heap budget over an unrelated, pre-existing
// retention characteristic.

// TestCompactRouteSwiftShiftComparisonGapIsNotATilingDefect documents the
// manifest's 2 swift witnesses (swift_log_1, swift_log_2) as a deliberately
// out-of-scope residual for B1. Both are adjudicated false-clean divergences
// (c_has_error=production_has_error=true, compact_has_error=false), but
// direct tree inspection shows the compact route's accepted derivation
// fully tiles the source for both -- there is no byte range left uncovered,
// so the B1 leaf-tiling invariant this tranche adds does not fire and must
// not be forced to. The divergence is a shift/reduce conflict-resolution
// difference: given the ambiguous trailing `>>` in `x >> y>>`, production
// completes `x >> y` as a bitwise_operation and wraps the dangling `> >` in
// an ERROR node, while compact's generic scheduler instead reduces `> >` as
// two more comparison_expression operators onto the same left-hand side --
// a different, but still fully gapless, derivation. This test pins today's
// actual (still-open) behavior so a future fix to the scheduler's conflict
// resolution is visible here (the test starts failing) rather than silently
// drifting; it is not an endorsement of the outcome. See the campaign
// finding filed alongside this tranche's spore.
func TestCompactRouteSwiftShiftComparisonGapIsNotATilingDefect(t *testing.T) {
	witnesses := loadRouteEqualityWitnesses(t)
	for _, id := range []string{"swift_log_1", "swift_log_2"} {
		id := id
		t.Run(id, func(t *testing.T) {
			witness, ok := witnesses[id]
			if !ok {
				t.Fatalf("witness %q is missing from %s", id, compactT3RouteEqualityWitnessManifestPath)
			}
			entry := grammars.DetectLanguageByName(witness.Language)
			if entry == nil {
				t.Fatalf("witness %q: language %q is not registered", id, witness.Language)
			}
			lang := entry.Language()
			source := []byte(witness.SourceUTF8)

			gts.ResetAdmissionCandidateCounters()
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(true)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("witness %q: parse: %v", id, err)
			}
			defer tree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			if routed == 0 && fallback == 1 {
				reason := gts.AdmissionCandidateLastFallbackReason()
				if strings.Contains(reason, "accepted-leaf-tiling-gap") {
					t.Fatalf("witness %q now declines at the tiling gate (reason=%q); this contradicts the documented "+
						"gapless-derivation root cause -- re-verify the finding before removing this pin", id, reason)
				}
				t.Skipf("witness %q now declines (reason=%q); the shift/reduce residual appears fixed -- "+
					"promote it into TestCompactRouteDeclinesAdjudicatedFalseCleanWitnesses and delete this pin", id, reason)
			}
			if routed != 1 || fallback != 0 {
				t.Fatalf("witness %q: route counters routed=%d fallback=%d, want the documented still-open routed=1 fallback=0", id, routed, fallback)
			}
			if tree.RootNode().HasError() {
				t.Fatalf("witness %q: compact tree now reports HasError=true; the false-clean residual appears fixed -- "+
					"re-verify and update this pin", id)
			}
		})
	}
}

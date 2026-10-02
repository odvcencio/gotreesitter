//go:build cgo && treesitter_c_parity && gts_engine_ceiling && gts_workcount

package cgoharness

import (
	"encoding/json"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Count the complete operation, including a declined compact attempt and the
// ensuing legacy retry. A selected-tree profile alone omits that work.
func TestCompactEditsWork(t *testing.T) {
	for _, input := range ceilingInputs(t) {
		for _, engine := range []string{"legacy", "compact"} {
			t.Run(input.size+"/"+input.mode+"/"+engine, func(t *testing.T) {
				p := gts.NewParser(grammars.DetectLanguageByName(input.language).Language())
				p.SetAdmissionCandidateRoute(engine == "compact")
				p.SetCompactCertificationTelemetry(engine == "compact")
				tree, err := p.Parse(input.source[0])
				ceilingGoTree(t, tree, input.source[0], err)
				if input.mode == "fresh" {
					tree.Release()
					tree = nil
				}
				defer func() {
					if tree != nil {
						tree.Release()
					}
				}()
				for direction := 0; direction < 2; direction++ {
					gts.BeginDiagnosticWorkCount()
					var next *gts.Tree
					var profile gts.IncrementalParseProfile
					var source []byte
					if input.mode == "fresh" {
						source = input.source[0]
						next, err = p.Parse(source)
					} else {
						source = input.source[1-direction]
						tree.Edit(input.edit[direction])
						next, profile, err = p.ParseIncrementalProfiled(source, tree)
						tree.Release()
					}
					counts := gts.EndDiagnosticWorkCount()
					ceilingGoTree(t, next, source, err)
					if counts.Overflow {
						t.Fatal("work counters overflowed")
					}
					runtime := next.ParseRuntime()
					whole := runtime.OperationWork.Total
					peak := runtime.MaxStacksSeen
					if engine == "compact" && runtime.CompactPeakHeaders != 0 {
						peak = runtime.CompactPeakHeaders
					}
					receipt, err := json.Marshal(map[string]any{
						"language": input.language, "size": input.size, "mode": input.mode, "engine": engine, "direction": direction,
						"work": counts, "whole_work": whole, "max_live_versions": peak,
						// A single accounted attempt makes the selected frontier peak
						// a complete-operation peak. Retries remain explicitly incomplete.
						"whole_counters_complete": whole.Attempts == 1 && peak != 0,
						"profile_tokens":          profile.TokensConsumed, "profile_nodes": profile.NewNodesAllocated,
						"reused_subtrees": profile.ReusedSubtrees, "reused_bytes": profile.ReusedBytes,
						"compact_edit_route": next.ParseRuntime().CompactIncrementalReuseRoute,
						"decline_reason":     next.ParseRuntime().CompactIncrementalFallbackReason,
					})
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("WHOLE_WORK %s", receipt)
					if input.mode == "fresh" {
						next.Release()
					} else {
						tree = next
					}
				}
			})
		}
	}
}

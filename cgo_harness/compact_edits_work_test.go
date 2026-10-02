//go:build cgo && treesitter_c_parity && gts_engine_ceiling && gts_workcount

package cgoharness

import (
	"encoding/json"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/graduation"
	"github.com/odvcencio/gotreesitter/internal/sched"
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
					phaseWork := sched.OperationWork{
						Total: sched.Work(whole), Initial: sched.Work(runtime.OperationWork.Initial),
						Compact: sched.Work(runtime.OperationWork.Compact), Retry: sched.Work(runtime.OperationWork.Retry),
						Fallback: sched.Work(runtime.OperationWork.Fallback), Verification: sched.Work(runtime.OperationWork.Verification),
						Recovery: sched.Work(runtime.OperationWork.Recovery), Forest: sched.Work(runtime.OperationWork.Forest),
					}
					peak := uint64(runtime.MaxStacksSeen)
					compactWork := runtime.OperationWork.Compact
					if engine == "compact" && runtime.CompactPeakHeaders != 0 && compactWork.Attempts == 1 &&
						compactWork.Tokens == whole.Tokens && compactWork.Nodes == whole.Nodes &&
						compactWork.Iterations == whole.Iterations && compactWork.Bytes == whole.Bytes {
						peak = runtime.CompactPeakHeaders
					}
					receipt, err := json.Marshal(map[string]any{
						"language": input.language, "size": input.size, "mode": input.mode, "engine": engine, "direction": direction,
						"work": counts, "whole_work": whole, "phase_work": runtime.OperationWork, "max_live_versions": peak,
						// Authenticate the peak across all phases, including zero-work
						// verification; actual discarded scheduler work stays incomplete.
						"whole_counters_complete": graduation.CompleteFrontierPeak(phaseWork, peak),
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

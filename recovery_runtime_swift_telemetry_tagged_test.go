//go:build gts_recovery_telemetry

package gotreesitter_test

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestSwiftRecoveryAttemptTelemetryTagged(t *testing.T) {
	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { gotreesitter.EnableRecoveryRuntimeTelemetry(false) })

	source, err := os.ReadFile(filepath.Join("grammars", "testdata", "swift_corpus", "stdlib_FloatingPointToString.swift"))
	if err != nil {
		t.Fatalf("read Swift issue 586 witness: %v", err)
	}
	parser := gotreesitter.NewParser(grammars.SwiftLanguage())
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse Swift issue 586 witness: %v", err)
	}
	defer tree.Release()

	stats := parser.DebugRecoveryRuntimeStats()
	attempts := parser.DebugRecoveryRuntimeAttempts()
	if !stats.Enabled || !stats.Completed {
		t.Fatalf("selected-tree receipt = %+v, want completed retry facts", stats)
	}
	// fix/gss-demotion-hysteresis bounds GSS demotion and reachability work
	// (glr.go, glr_gss.go, parser.go), which cuts this witness's peak
	// concurrent stack count from 62 to under the
	// shouldRetryAcceptedErrorParse ceiling of 8 (maxGLRStacks, glr.go): see
	// the matching comment on the swift-586-floating-point case in
	// benchmark_recovery_test.go. The initial pass alone now produces the
	// correct error/full-span tree, so RetryAttemptCount is legitimately 0
	// and DebugRecoveryRuntimeAttempts() reports only that one attempt --
	// this witness no longer exercises the multi-attempt receipt fields
	// below. If a retry does fire (older commit, different build tag, or a
	// future grammar-lock bump), every attempt's receipt must stay complete
	// and at least one candidate must be selected.
	if stats.RetryAttemptCount == 0 {
		if len(attempts) != 1 {
			t.Fatalf("attempt count = %d, want 1 with no retry: %+v", len(attempts), attempts)
		}
		return
	}
	if len(attempts) != int(stats.RetryAttemptCount)+1 {
		t.Fatalf("attempt count = %d, want %d: %+v", len(attempts), stats.RetryAttemptCount+1, attempts)
	}
	selected := 0
	for _, attempt := range attempts {
		if attempt.Rung == "" || attempt.Cause == "" || attempt.StopReason == "" || attempt.WallNanos == 0 {
			t.Fatalf("incomplete attempt receipt: %+v", attempt)
		}
		if attempt.CandidateSelected {
			selected++
		}
	}
	if selected == 0 {
		t.Fatalf("attempt receipt has no selected candidate: %+v", attempts)
	}
}

//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestAdmissionCandidateSvelteButtonCOracle(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	source, err := os.ReadFile("../testdata/admission_direct/svelte_button.svelte")
	if err != nil {
		t.Fatalf("read Svelte admission fixture: %v", err)
	}

	candidateRoute := true
	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	runParityCase(t, parityCase{
		name:           "svelte",
		source:         string(source),
		candidateRoute: &candidateRoute,
	}, "admission-direct", source)
	afterRouted, afterFallback := gotreesitter.AdmissionCandidateCounters()

	routed := afterRouted - beforeRouted
	fallback := afterFallback - beforeFallback
	if routed != 1 || fallback != 0 {
		t.Fatalf(
			"candidate route counters = %d/%d, want 1/0; reason=%s",
			routed,
			fallback,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
}

//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestAdmissionCandidateGraphQLRootFieldsCOracle(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	source, err := os.ReadFile("../testdata/admission_direct/graphql_root_fields.graphql")
	if err != nil {
		t.Fatalf("read GraphQL admission fixture: %v", err)
	}

	candidateRoute := true
	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	runParityCase(t, parityCase{
		name:           "graphql",
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

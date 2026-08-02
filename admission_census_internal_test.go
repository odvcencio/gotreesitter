//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"sync"
	"testing"
)

func TestAdmissionCensusSeparatesNoTableActionFromPausedFrontier(t *testing.T) {
	if got := admissionCensusClassify(
		DiagnosticParserCoreNoAction,
		diagnosticParserCoreNoTableActionDetail,
	); got != censusMechanismNoTableAction {
		t.Fatalf("no-table-action mechanism=%q", got)
	}
	if got := admissionCensusClassify(
		DiagnosticParserCoreNoAction,
		"generic scheduler has only paused heads for the elected token",
	); got != censusMechanismSchedulerShape {
		t.Fatalf("paused-frontier mechanism=%q", got)
	}
}

// TestAdmissionCensusClassifiesMaterialAcceptanceElection pins the census
// mechanism the R1 materiality gate's three decline reasons classify into
// (compactAcceptanceElectionMaterialDetail,
// compactAcceptanceElectionCandidateCapDetail, and
// compactAcceptanceElectionNoContextDetail, parsercore_phase0_driver.go):
// all three share the DiagnosticParserCoreAccept boundary and all three
// contain the "material-acceptance-election" substring, so all three must
// classify the same way, distinct from the pre-existing
// accepted-leaf-tiling-gap and accepted-root-leading-gap cases on the same
// boundary.
func TestAdmissionCensusClassifiesMaterialAcceptanceElection(t *testing.T) {
	if got := admissionCensusClassify(
		DiagnosticParserCoreAccept, compactAcceptanceElectionMaterialDetail,
	); got != censusMechanismMaterialAcceptanceElection {
		t.Fatalf("material-election mechanism=%q, want %q", got, censusMechanismMaterialAcceptanceElection)
	}
	if got := admissionCensusClassify(
		DiagnosticParserCoreAccept, compactAcceptanceElectionCandidateCapDetail,
	); got != censusMechanismMaterialAcceptanceElection {
		t.Fatalf("candidate-cap mechanism=%q, want %q", got, censusMechanismMaterialAcceptanceElection)
	}
	if got := admissionCensusClassify(
		DiagnosticParserCoreAccept, compactAcceptanceElectionNoContextDetail,
	); got != censusMechanismMaterialAcceptanceElection {
		t.Fatalf("no-context mechanism=%q, want %q", got, censusMechanismMaterialAcceptanceElection)
	}
	if compactAcceptanceElectionNoContextDetail == compactAcceptanceElectionMaterialDetail {
		t.Fatalf("no-context detail must be distinct wording from the ran-a-comparison detail")
	}
	// A soft decline wraps the recorded Stop detail as "did not accept EOF:
	// <detail>" (admissionCensusStopDecline) before this classifier ever
	// sees it; the substring match must still hit through that prefix.
	if got := admissionCensusClassify(
		DiagnosticParserCoreAccept, "did not accept EOF: "+compactAcceptanceElectionMaterialDetail,
	); got != censusMechanismMaterialAcceptanceElection {
		t.Fatalf("wrapped material-election mechanism=%q, want %q", got, censusMechanismMaterialAcceptanceElection)
	}
	if got := admissionCensusClassify(DiagnosticParserCoreAccept, "accepted-leaf-tiling-gap: unrelated"); got != censusMechanismAcceptedLeafTilingGap {
		t.Fatalf("leaf-tiling-gap mechanism=%q, want %q (regression check: still distinct)", got, censusMechanismAcceptedLeafTilingGap)
	}
}

// ResetAdmissionCensusEnabledForTest clears the cached GTS_ADMISSION_CENSUS
// read (admissionCensusEnabled's sync.Once) so a test that sets the
// environment variable with t.Setenv sees it take effect immediately,
// regardless of whether an earlier test in the same binary already read and
// cached a value. Call it again (for example via t.Cleanup) after restoring
// the environment variable, so later tests re-read rather than inherit this
// test's cached value. Never copies the sync.Once (which would trip
// go vet's copylocks check); it only replaces the package-level variable
// with a fresh zero value.
func ResetAdmissionCensusEnabledForTest() {
	admissionCensusEnabledOnce = sync.Once{}
	admissionCensusEnabledVal = false
}

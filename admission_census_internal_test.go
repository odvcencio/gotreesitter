//go:build !gts_no_parsercorephase0

package gotreesitter

import "testing"

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

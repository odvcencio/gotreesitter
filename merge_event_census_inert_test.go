//go:build !gts_merge_census

package gotreesitter

import "testing"

// TestMergeEventCensusIsAbsentFromTheDefaultBuild is the inertness ratchet for
// the stage M0 merge-event census (spec.merge-time-election.v1).
//
// The census sits on the production merge path, which every language and
// every route shares, and its link-payload hook runs a second shallow
// comparison beside the deep one. That cost must never reach a shipped parse.
// A build tag, not an env var, keeps it out: the default build compiles
// merge_event_census_disabled.go, where mergeCensusEnabled is a false
// constant, so the compiler removes every guarded block.
//
// This test fails the moment the default build starts carrying the census.
func TestMergeEventCensusIsAbsentFromTheDefaultBuild(t *testing.T) {
	if MergeEventCensusBuilt() {
		t.Fatal("the merge-event census is compiled into the default build; it must stay behind the gts_merge_census build tag")
	}
	if mergeCensusEnabled {
		t.Fatal("mergeCensusEnabled is true in the default build; every census block must be removed at compile time")
	}
}

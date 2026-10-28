//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// knownCOracleDifferences lists the test cases where Go does not match the
// locked C oracle since the pin moved to tree-sitter 0.27.1. Two causes:
//
//   - Error recovery. Both trees contain errors, and the C tree changed in
//     0.27.0 (error-state restart 15ea3328 and error nesting f30ee2b3).
//   - Query matching. C returns more matches for three repetition patterns.
//
// Issue 1545 tracks the Go repairs. A repair removes its entry here. Only the
// names below are skipped or relaxed. Any other mismatch with the oracle still
// fails.
const knownCOracleDifferencesIssue = "https://github.com/odvcencio/gotreesitter/issues/1545"

var knownCOracleDifferences = map[string]string{
	"TestCompactT3OracleAdjudication/html/html_erroneous_end_tag/html_log_7":                                   "compact HTML recovery differs from the 0.27.1 error-state restart",
	"TestFIDLVersionedLayoutModifiersLockedCParity/two_modifiers_with_args":                                    "error children are nested differently in 0.27.1",
	"TestFIDLVersionedLayoutModifiersLockedCParity/single_modifier_with_args":                                  "error children are nested differently in 0.27.1",
	"TestFIDLVersionedLayoutModifiersLockedCParity/three_modifiers_with_args":                                  "error children are nested differently in 0.27.1",
	"TestBibtexInertRecoveryMaterializationLockedCParity/missing_comma_after_key":                              "error children are nested differently in 0.27.1",
	"TestBibtexInertRecoveryMaterializationLockedCParity/missing_key":                                          "error children are nested differently in 0.27.1",
	"TestAWKDispatchBlockerRoutes/recovery":                                                                    "error children are nested differently in 0.27.1",
	"TestAWKRecoveredProductionMatchesLockedC":                                                                 "error children are nested differently in 0.27.1",
	"TestParityQueryExactJavaScriptEcosystemPatterns/anchor_after_zero_or_more_comments":                       "query matching in 0.27.1 returns more matches than Go",
	"TestParityQueryExactJavaScriptEcosystemPatterns/parent_repetition_uses_first_contiguous_comment_run":      "query matching in 0.27.1 returns a second comment run that Go omits",
	"TestParityQueryExactJavaScriptEcosystemPatterns/parent_star_repetition_uses_first_contiguous_comment_run": "query matching in 0.27.1 returns a second comment run that Go omits",
	"TestIncrementalCReuseLedgerReadDependencies/ron":                                                          "the malformed edited sample recovers into a different tree in 0.27.1",
}

// knownCOracleDifference returns the reason the running test case is a known
// difference, or "" when it is not.
func knownCOracleDifference(t *testing.T) string {
	t.Helper()
	return knownCOracleDifferences[t.Name()]
}

// skipKnownCOracleDifference skips the running test case when it is listed in
// knownCOracleDifferences.
func skipKnownCOracleDifference(t *testing.T) {
	t.Helper()
	if reason := knownCOracleDifference(t); reason != "" {
		t.Skipf("known difference from the C oracle 0.27.1 pin (%s): %s", knownCOracleDifferencesIssue, reason)
	}
}

package gotreesitter

import "testing"

func TestRestoreOmittedErrorPreservesHiddenMissingSummary(t *testing.T) {
	for _, tc := range []struct {
		name         string
		omitted      bool
		before, want resultErrorSummary
		wantError    bool
	}{
		{"clean", false, resultErrorSummaryClean, resultErrorSummaryClean, false},
		{"ordinary error", true, resultErrorSummaryClean, resultErrorSummaryPresent, true},
		{"hidden missing", true, resultErrorSummaryHiddenMissing, resultErrorSummaryHiddenMissing, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &Node{}
			tree := &Tree{root: root, omittedRootError: tc.omitted, resultErrorSummary: tc.before}
			tree.restoreOmittedRootError()
			if root.HasError() != tc.wantError || tree.resultErrorSummary != tc.want {
				t.Fatalf("error=%v summary=%v, want %v/%v", root.HasError(), tree.resultErrorSummary, tc.wantError, tc.want)
			}
		})
	}
}

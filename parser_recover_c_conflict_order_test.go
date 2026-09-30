package gotreesitter

import "testing"

func TestConflictOrderRequiresOriginalVersionReceipt(t *testing.T) {
	parser := &Parser{}
	shift := glrStack{cPaused: true, cConflictGroup: 0}
	reduction := glrStack{cPaused: true, cConflictGroup: 0, cConflictReduced: true}
	status := cErrorStatus{cost: 100, isInError: true}
	if got := parser.cCompareCondenseVersions(status, status, &shift, &reduction); got != cErrorComparisonNone {
		t.Fatalf("unproven frontier order = %v, want no preference", got)
	}
	shift.cConflictGroup, reduction.cConflictGroup = 1, 1
	if got := parser.cCompareCondenseVersions(status, status, &shift, &reduction); got != cErrorComparisonPreferLeft {
		t.Fatalf("proven conflict order = %v, want original shift", got)
	}
	if got := parser.cCompareCondenseVersions(status, status, &reduction, &shift); got != cErrorComparisonPreferRight {
		t.Fatalf("reversed proven conflict order = %v, want original shift", got)
	}
}

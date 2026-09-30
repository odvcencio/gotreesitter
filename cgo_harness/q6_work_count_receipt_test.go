//go:build linux && cgo && treesitter_c_parity && gts_workcount

package cgoharness

import (
	"encoding/json"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestQ6GoWholeParseWorkReceipt includes every retry, rather than only the
// winning attempt reported by Tree.ParseRuntime. Timing uses untagged builds.
func TestQ6GoWholeParseWorkReceipt(t *testing.T) {
	source := q6GoOriginalSource(t)
	language := grammars.GoLanguage()
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	gts.BeginDiagnosticWorkCount()
	tree, err := parser.Parse(source)
	counts := gts.EndDiagnosticWorkCount()
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Overflow || len(counts.Attempts) == 0 {
		t.Fatal("work counters overflowed or omitted attempt attribution")
	}
	receipt := struct {
		TreeSHA256 string                           `json:"tree_sha256"`
		Winner     gts.ParseRuntime                 `json:"winner"`
		Aggregate  gts.DiagnosticWorkCountValues    `json:"aggregate"`
		Attempts   []gts.DiagnosticWorkCountAttempt `json:"attempts"`
		Outside    gts.DiagnosticWorkCountValues    `json:"outside_attempt"`
	}{inspection.SHA256, tree.ParseRuntime(), counts.DiagnosticWorkCountValues, counts.Attempts, counts.OutsideAttempt}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("counter_receipt=%s", encoded)
	const lockedCFresh = "9ed06ed630d8e18a6f191271d9da9339765ad1bd8ca15df2829d9b58cf3cd0f1"
	if inspection.SHA256 != lockedCFresh || tree.RootNode().HasError() {
		t.Fatalf("diagnostic tree=%s, want fresh locked C=%s", inspection.SHA256, lockedCFresh)
	}
}

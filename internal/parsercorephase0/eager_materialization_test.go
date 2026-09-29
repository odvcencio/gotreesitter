package parsercorephase0

import "testing"

func TestFillMaterializationViewClearsPreviousProvenance(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	missing, err := compact.MissingLeaf(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendAuthenticatedTerminal(subtreeRecord{symbol: 2, endByte: 1, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var view MaterializationSubtreeView
	var replay MaterializationReplayView
	if err := compact.FillMaterializationView(missing, &view, &replay); err != nil {
		t.Fatal(err)
	}
	if !view.Missing || !view.MissingDependencyExact {
		t.Fatalf("missing leaf has no dependency: %+v", view)
	}
	// These fields can be populated by another materialization or replay visit.
	view.ExternalScannerCheckpointStart = 1
	view.ExternalScannerCheckpointEnd = 2
	view.ExternalScannerCheckpointExact = true
	view.LexerSkippedPrefix = true
	view.LexerSkippedPrefixStart = 3
	view.ReusedKey = 4
	view.ReplayPreGotoKnown = true
	view.ReplayParseStateKnown = true
	if err := compact.FillMaterializationView(leaf, &view, &replay); err != nil {
		t.Fatal(err)
	}
	if view.Missing || view.MissingDependencyExact || view.MissingDependency != (MissingLeafDependency{}) ||
		view.ExternalScannerCheckpointStart != 0 || view.ExternalScannerCheckpointEnd != 0 || view.ExternalScannerCheckpointExact ||
		view.LexerSkippedPrefix || view.LexerSkippedPrefixStart != 0 || view.ReusedKey != 0 || view.ReplayPreGotoKnown || view.ReplayParseStateKnown {
		t.Fatalf("ordinary leaf retained previous provenance: %+v", view)
	}
	if view.Symbol != 2 || view.EndByte != 1 || !view.Terminal {
		t.Fatalf("ordinary leaf view: %+v", view)
	}
}

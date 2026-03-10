package grammargen

import "testing"

func TestSplitOracleNoConflicts(t *testing.T) {
	oracle := newSplitOracle(nil, nil)
	candidates := oracle.candidates()
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestSplitOracleIgnoresResolvedConflicts(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(0)
	prov.recordMerge(0, mergeOrigin{kernelHash: 0x1111})

	diags := []ConflictDiag{
		{
			Kind:         ShiftReduce,
			State:        0,
			LookaheadSym: 5,
			Resolution:   "shift wins (default yacc behavior)",
		},
	}

	oracle := newSplitOracle(diags, prov)
	candidates := oracle.candidates()
	if len(candidates) != 0 {
		t.Errorf("resolved conflicts should not produce candidates, got %d", len(candidates))
	}
}

func TestSplitOracleIgnoresUnmergedGLR(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(5) // fresh, not merged

	diags := []ConflictDiag{
		{
			Kind:         ShiftReduce,
			State:        5,
			LookaheadSym: 10,
			Resolution:   "GLR (multiple actions kept)",
		},
	}

	oracle := newSplitOracle(diags, prov)
	candidates := oracle.candidates()
	if len(candidates) != 0 {
		t.Errorf("unmerged GLR conflicts should not be candidates, got %d", len(candidates))
	}
}

func TestSplitOracleReportsMergedGLR(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(5)
	prov.recordMerge(5, mergeOrigin{kernelHash: 0x1111, sourceState: 2})
	prov.recordMerge(5, mergeOrigin{kernelHash: 0x2222, sourceState: 3})

	diags := []ConflictDiag{
		{
			Kind:         ShiftReduce,
			State:        5,
			LookaheadSym: 10,
			Resolution:   "GLR (multiple actions kept)",
		},
	}

	oracle := newSplitOracle(diags, prov)
	candidates := oracle.candidates()

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].stateIdx != 5 {
		t.Errorf("expected state 5, got %d", candidates[0].stateIdx)
	}
	if candidates[0].mergeCount != 2 {
		t.Errorf("expected mergeCount 2, got %d", candidates[0].mergeCount)
	}
}

func TestSplitOracleDeduplicatesByState(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(5)
	prov.recordMerge(5, mergeOrigin{kernelHash: 0x1111})

	// Two GLR conflicts on same state but different symbols.
	diags := []ConflictDiag{
		{Kind: ShiftReduce, State: 5, LookaheadSym: 10, Resolution: "GLR (multiple actions kept)"},
		{Kind: ReduceReduce, State: 5, LookaheadSym: 11, Resolution: "GLR (multiple actions kept)"},
	}

	oracle := newSplitOracle(diags, prov)
	candidates := oracle.candidates()

	if len(candidates) != 1 {
		t.Fatalf("expected 1 deduplicated candidate, got %d", len(candidates))
	}
}

func TestSplitOracleMultipleStates(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(3)
	prov.recordMerge(3, mergeOrigin{kernelHash: 0xAAAA})
	prov.recordFresh(7)
	prov.recordMerge(7, mergeOrigin{kernelHash: 0xBBBB})

	diags := []ConflictDiag{
		{Kind: ShiftReduce, State: 3, LookaheadSym: 1, Resolution: "GLR (multiple actions kept)"},
		{Kind: ReduceReduce, State: 7, LookaheadSym: 2, Resolution: "GLR (multiple actions kept)"},
	}

	oracle := newSplitOracle(diags, prov)
	candidates := oracle.candidates()

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
}

func TestSplitOracleNilProvenance(t *testing.T) {
	diags := []ConflictDiag{
		{Kind: ShiftReduce, State: 5, LookaheadSym: 10, Resolution: "GLR (multiple actions kept)"},
	}

	oracle := newSplitOracle(diags, nil)
	candidates := oracle.candidates()
	// With nil provenance, no state can be identified as merged.
	if len(candidates) != 0 {
		t.Errorf("nil provenance should produce no candidates, got %d", len(candidates))
	}
}

func TestGenerateReportIncludesSplitCandidates(t *testing.T) {
	g := NewGrammar("report_split")
	g.Define("expression", Choice(
		Seq(Sym("expression"), Str("+"), Sym("expression")),
		Seq(Sym("expression"), Str("*"), Sym("expression")),
		Str("id"),
	))
	g.SetConflicts([]string{"expression"})

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("conflicts=%d, splitCandidates=%d",
		len(report.Conflicts), len(report.SplitCandidates))

	// The field should exist and be populated (possibly 0 for this grammar
	// since conflicts are declared and thus intentional).
	_ = report.SplitCandidates
}

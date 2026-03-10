package grammargen

import "testing"

func TestLocalLR1Rebuild(t *testing.T) {
	// Create a grammar known to have LALR merge pathology.
	// Two rules that share a common prefix but diverge:
	//   A → a b c d
	//   B → a b c e
	// In LALR, the states for "a b c ." merge. But the reduce on
	// lookahead {d} vs {e} creates a conflict if both are viable.
	g := NewGrammar("split_test")
	g.Define("start", Choice(Sym("a_rule"), Sym("b_rule")))
	g.Define("a_rule", Seq(Str("a"), Str("b"), Str("c"), Str("d")))
	g.Define("b_rule", Seq(Str("a"), Str("b"), Str("c"), Str("e")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatal(err)
	}

	tables, ctx, err := buildLRTablesWithProvenance(ng)
	if err != nil {
		t.Fatal(err)
	}
	prov := ctx.provenance

	// Run conflict resolution with diagnostics.
	diags, err := resolveConflictsWithDiag(tables, ng, prov)
	if err != nil {
		t.Fatal(err)
	}

	oracle := newSplitOracle(diags, prov)
	candidates := oracle.candidates()

	t.Logf("states=%d, conflicts=%d, candidates=%d",
		tables.StateCount, len(diags), len(candidates))

	if len(candidates) == 0 {
		t.Skip("no split candidates — grammar may be too simple for LALR pathology")
	}

	// Apply local rebuild.
	splitCount, err := localLR1Rebuild(tables, ng, ctx, candidates, 100)
	if err != nil {
		t.Fatalf("localLR1Rebuild failed: %v", err)
	}

	t.Logf("split %d states", splitCount)

	// After splitting, re-resolve conflicts — should have fewer GLR entries.
	diagsAfter, err := resolveConflictsWithDiag(tables, ng, prov)
	if err != nil {
		t.Fatal(err)
	}

	glrBefore := 0
	for _, d := range diags {
		if d.Resolution == "GLR (multiple actions kept)" {
			glrBefore++
		}
	}
	glrAfter := 0
	for _, d := range diagsAfter {
		if d.Resolution == "GLR (multiple actions kept)" {
			glrAfter++
		}
	}

	t.Logf("GLR conflicts: before=%d, after=%d", glrBefore, glrAfter)
	if glrAfter > glrBefore {
		t.Errorf("splitting should not increase GLR conflicts")
	}
}

func TestGenerateWithReportSplitting(t *testing.T) {
	g := NewGrammar("split_gen_test")
	g.Define("start", Choice(Sym("a_rule"), Sym("b_rule")))
	g.Define("a_rule", Seq(Str("a"), Str("b"), Str("c"), Str("d")))
	g.Define("b_rule", Seq(Str("a"), Str("b"), Str("c"), Str("e")))

	// Enable splitting.
	g.EnableLRSplitting = true

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("states=%d, conflicts=%d, candidates=%d, splitResult=%v",
		report.StateCount, len(report.Conflicts),
		len(report.SplitCandidates), report.SplitResult)
}

package grammargen

import "testing"

func TestMergeProvenanceRecordsMerge(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(5)
	if prov.isMerged(5) {
		t.Fatal("fresh state should not be merged")
	}

	prov.recordMerge(5, mergeOrigin{
		kernelHash:  0xABCD,
		sourceState: -1,
	})
	if !prov.isMerged(5) {
		t.Fatal("state with merge should report isMerged=true")
	}

	origins := prov.origins(5)
	if len(origins) != 1 {
		t.Fatalf("expected 1 origin, got %d", len(origins))
	}
	if origins[0].kernelHash != 0xABCD {
		t.Fatalf("expected kernelHash 0xABCD, got %x", origins[0].kernelHash)
	}
}

func TestMergeProvenanceMultipleMerges(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(10)
	prov.recordMerge(10, mergeOrigin{kernelHash: 0x1111})
	prov.recordMerge(10, mergeOrigin{kernelHash: 0x2222})
	prov.recordMerge(10, mergeOrigin{kernelHash: 0x3333})

	origins := prov.origins(10)
	if len(origins) != 3 {
		t.Fatalf("expected 3 origins, got %d", len(origins))
	}
}

func TestMergeProvenanceLookaheadContributors(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(7)

	prov.recordLookaheadContributor(7, 3, 42)
	prov.recordLookaheadContributor(7, 3, 55)

	contribs := prov.lookaheadContributors(7, 3)
	if len(contribs) != 2 {
		t.Fatalf("expected 2 contributors, got %d", len(contribs))
	}
}

func TestMergeProvenanceMergedStateCount(t *testing.T) {
	prov := newMergeProvenance()
	prov.recordFresh(0)
	prov.recordFresh(1)
	prov.recordFresh(2)
	prov.recordMerge(1, mergeOrigin{kernelHash: 0xAAAA})
	prov.recordMerge(2, mergeOrigin{kernelHash: 0xBBBB})

	if prov.mergedStateCount() != 2 {
		t.Fatalf("expected 2 merged states, got %d", prov.mergedStateCount())
	}
}

func TestMergeProvenanceNoContributors(t *testing.T) {
	prov := newMergeProvenance()
	contribs := prov.lookaheadContributors(99, 42)
	if len(contribs) != 0 {
		t.Fatalf("expected 0 contributors for unknown state, got %d", len(contribs))
	}
}

func TestLALRProvenanceEndToEnd(t *testing.T) {
	g := NewGrammar("prov_test")
	g.Define("start", Seq(Sym("a"), Sym("b")))
	g.Define("a", Choice(Str("x"), Str("y")))
	g.Define("b", Choice(Str("x"), Str("z")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &lrContext{
		ng:              ng,
		firstSets:       make([]bitset, len(ng.Symbols)),
		nullables:       make([]bool, len(ng.Symbols)),
		prodsByLHS:      make(map[int][]int),
		betaCache:       make(map[uint32]*betaResult),
		dot0Index:       make([]int, len(ng.Productions)),
		tokenCount:      ng.TokenCount(),
		trackProvenance: true,
	}
	for i := range ctx.dot0Index {
		ctx.dot0Index[i] = -1
	}
	for i := range ng.Productions {
		ctx.prodsByLHS[ng.Productions[i].LHS] = append(ctx.prodsByLHS[ng.Productions[i].LHS], i)
	}
	ctx.computeFirstSets()

	// Force LALR path.
	ctx.buildLR0()
	ctx.computeLALRLookaheads()

	if ctx.provenance == nil {
		t.Fatal("provenance should be initialized after LALR build")
	}

	if !ctx.provenance.fresh[0] {
		t.Error("state 0 should be fresh")
	}

	t.Logf("states=%d, merged=%d", len(ctx.itemSets), ctx.provenance.mergedStateCount())
}

func TestFullLRProvenanceEndToEnd(t *testing.T) {
	// Small grammar uses full LR(1) path (<=400 productions).
	g := NewGrammar("lr_prov_test")
	g.Define("start", Seq(Sym("a"), Sym("b")))
	g.Define("a", Choice(Str("x"), Str("y")))
	g.Define("b", Choice(Str("x"), Str("z")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatal(err)
	}

	tables, err := buildLRTables(ng)
	if err != nil {
		t.Fatal(err)
	}
	_ = tables

	// Verify the lrContext path also sets provenance.
	// Since buildLRTables doesn't expose lrContext, we test via
	// the full LR path by constructing lrContext directly.
	ctx := &lrContext{
		ng:              ng,
		firstSets:       make([]bitset, len(ng.Symbols)),
		nullables:       make([]bool, len(ng.Symbols)),
		prodsByLHS:      make(map[int][]int),
		betaCache:       make(map[uint32]*betaResult),
		dot0Index:       make([]int, len(ng.Productions)),
		tokenCount:      ng.TokenCount(),
		trackProvenance: true,
	}
	for i := range ctx.dot0Index {
		ctx.dot0Index[i] = -1
	}
	for i := range ng.Productions {
		ctx.prodsByLHS[ng.Productions[i].LHS] = append(ctx.prodsByLHS[ng.Productions[i].LHS], i)
	}
	ctx.computeFirstSets()

	// Use full LR(1) path.
	ctx.buildItemSets()

	if ctx.provenance == nil {
		t.Fatal("provenance should be initialized after full LR build")
	}

	if !ctx.provenance.fresh[0] {
		t.Error("state 0 should be fresh")
	}

	t.Logf("states=%d, merged=%d", len(ctx.itemSets), ctx.provenance.mergedStateCount())
}

func TestConflictDiagHasProvenance(t *testing.T) {
	g := NewGrammar("conflict_prov")
	g.Define("expression", Choice(
		PrecLeft(1, Seq(Sym("expression"), Str("+"), Sym("expression"))),
		PrecLeft(2, Seq(Sym("expression"), Str("*"), Sym("expression"))),
		Str("id"),
	))
	g.SetConflicts([]string{"expression"})

	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Conflicts) == 0 {
		t.Skip("no conflicts generated")
	}

	for _, c := range report.Conflicts {
		t.Logf("conflict: state=%d, sym=%d, merged=%v, mergeCount=%d, resolution=%s",
			c.State, c.LookaheadSym, c.IsMergedState, c.MergeCount, c.Resolution)
	}
}

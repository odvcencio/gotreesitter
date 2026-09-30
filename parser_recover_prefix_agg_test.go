package gotreesitter

import "testing"

func TestCNodeErrorCostRawTokenDoesNotCountAsSkippedTree(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	p := NewParser(lang)
	p.cNodeMemoCache = make([]cNodeMemoCacheEntry, 512)
	ordinary := NewLeafNode(1, false, 16, 18, Point{Column: 16}, Point{Column: 18})
	raw := NewLeafNode(errorSymbol, true, 18, 22, Point{Column: 18}, Point{Column: 22})
	region := NewParentNode(errorSymbol, true, []*Node{ordinary}, nil, 0)
	pre := p.cErrRegionPreAbsorb(region)
	if !pre.valid {
		t.Fatal("incremental cost cache was not primed")
	}
	region.children = append(region.children, raw)
	region.endByte, region.endPoint = raw.endByte, raw.endPoint
	nodeBumpEquivVersion(region)
	p.cErrRegionPostAbsorb(pre, raw)
	const want = 606 // C: 500 recovery + 100 skipped terminal + 6 bytes.
	var scratch glrMergeScratch
	aggregate, _ := p.cNodeErrorCostAndVisibleSubtreeCount(region)
	for name, got := range map[string]uint32{
		"unmemoized":            cNodeErrorCostLang(lang, region),
		"merge scratch":         cNodeErrorCostLangWithScratch(&scratch, lang, region),
		"parser memo":           p.cNodeErrorCost(region),
		"incremental aggregate": aggregate,
	} {
		if got != want {
			t.Errorf("%s cost=%d, want %d", name, got, want)
		}
	}
}

func TestCNodeErrorCostAndVisibleSubtreeCountMatchesIndependentWalks(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	missing := &Node{symbol: 1, equivVersion: 1}
	missing.setMissing(true)
	plain := &Node{symbol: 1, equivVersion: 1}
	root := &Node{
		symbol:       errorSymbol,
		equivVersion: 1,
		endByte:      4,
		children:     []*Node{plain, missing},
	}
	p := &Parser{
		language:       lang,
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
	}

	wantCost := cNodeErrorCostLang(lang, root)
	wantVisible := cNodeVisibleSubtreeCountUncachedLang(lang, root)
	gotCost, gotVisible := p.cNodeErrorCostAndVisibleSubtreeCount(root)
	if gotCost != wantCost || gotVisible != wantVisible {
		t.Fatalf("combined aggregate = (%d, %d), want (%d, %d)", gotCost, gotVisible, wantCost, wantVisible)
	}
	slot := p.cNodeMemoSlot(root)
	if slot.ver != root.equivVersion || !slot.hasCost || !slot.hasVis {
		t.Fatalf("combined aggregate did not cache both values: %#v", *slot)
	}
}

func TestBeforePublicationVersionBumpDoesNotInvalidateLivePrefixes(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	p := &Parser{
		language:       lang,
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
	}
	published := &Node{symbol: 1, equivVersion: 1}
	head := &gssNode{entry: newStackEntryNode(1, published), depth: 1}
	if cost, visible := p.cStackPrefixAgg(head); cost != 0 || visible != 1 {
		t.Fatalf("initial published aggregate = (%d, %d), want (0, 1)", cost, visible)
	}
	gen := gssPrefixAggGen.Load()

	fresh := &Node{symbol: 1, equivVersion: 1, errorRankCache: 3}
	fresh.setMissing(true) // Recovery-relevant, but the node is not published.
	nodeBumpEquivVersionBeforePublication(fresh)
	if fresh.equivVersion != 2 {
		t.Fatalf("fresh equiv version = %d, want 2", fresh.equivVersion)
	}
	if fresh.errorRankCache != 0 {
		t.Fatalf("fresh error-rank cache = %d, want invalidated", fresh.errorRankCache)
	}
	if got := gssPrefixAggGen.Load(); got != gen {
		t.Fatalf("before-publication bump changed prefix generation from %d to %d", gen, got)
	}
	if cost, visible := p.cStackPrefixAgg(head); cost != 0 || visible != 1 {
		t.Fatalf("aggregate after fresh bump = (%d, %d), want (0, 1)", cost, visible)
	}

	published.setMissing(true)
	nodeBumpEquivVersion(published)
	if got := gssPrefixAggGen.Load(); got == gen {
		t.Fatalf("published recovery mutation left prefix generation at %d", got)
	}
	wantCost := uint32(cErrCostPerMissingTree + cErrCostPerRecovery)
	if cost, visible := p.cStackPrefixAgg(head); cost != wantCost || visible != 1 {
		t.Fatalf("aggregate after published bump = (%d, %d), want (%d, 1)", cost, visible, wantCost)
	}
}

func TestMetadataVersionBumpKeepsRecoveryPrefixAggregate(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	p := &Parser{
		language:       lang,
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
	}
	payload := &Node{symbol: 1, equivVersion: 1, errorRankCache: 3}
	head := &gssNode{entry: newStackEntryNode(1, payload), depth: 1}

	cost, visible := p.cStackPrefixAgg(head)
	if cost != 0 || visible != 1 {
		t.Fatalf("initial aggregate = (%d, %d), want (0, 1)", cost, visible)
	}
	gen := gssPrefixAggGen.Load()
	valid := head.aggValid&(gssAggCostValid|gssAggVisValid) == (gssAggCostValid | gssAggVisValid)
	if head.aggGen != gen || !valid {
		t.Fatalf("initial cache generation = %d valid=%v, want %d valid", head.aggGen, valid, gen)
	}

	payload.parseState = 2
	nodeBumpEquivVersionMetadata(payload)
	if payload.equivVersion != 2 {
		t.Fatalf("equiv version = %d, want 2", payload.equivVersion)
	}
	if payload.errorRankCache != 0 {
		t.Fatalf("error-rank cache = %d, want invalidated", payload.errorRankCache)
	}
	if got := gssPrefixAggGen.Load(); got != gen {
		t.Fatalf("metadata bump changed prefix generation from %d to %d", gen, got)
	}
	if cost, visible = p.cStackPrefixAgg(head); cost != 0 || visible != 1 {
		t.Fatalf("post-metadata aggregate = (%d, %d), want (0, 1)", cost, visible)
	}

	payload.setMissing(true)
	nodeBumpEquivVersion(payload)
	if got := gssPrefixAggGen.Load(); got == gen {
		t.Fatalf("recovery-relevant bump left prefix generation at %d", got)
	}
	cost, visible = p.cStackPrefixAgg(head)
	wantCost := uint32(cErrCostPerMissingTree + cErrCostPerRecovery)
	if cost != wantCost || visible != 1 {
		t.Fatalf("post-missing aggregate = (%d, %d), want (%d, 1)", cost, visible, wantCost)
	}
}

func TestMergeCostFillInvalidatesAndParserRefillsVisibility(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	p := &Parser{language: lang, cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
	base := &gssNode{entry: newStackEntryNode(1, &Node{symbol: 1, equivVersion: 1}), depth: 1}
	head := &gssNode{entry: newStackEntryNode(2, &Node{symbol: 1, equivVersion: 1}), prev: base, depth: 2}

	if cost, visible := p.cStackPrefixAgg(head); cost != 0 || visible != 2 {
		t.Fatalf("initial aggregate = (%d, %d), want (0, 2)", cost, visible)
	}
	if base.aggValid&gssAggVisValid == 0 || head.aggValid&gssAggVisValid == 0 {
		t.Fatal("parser aggregate did not validate visibility")
	}

	gssPrefixAggGen.Add(1)
	merge := glrMergeScratch{language: lang}
	if cost := cStackPrefixCostForMerge(&merge, lang, head); cost != 0 {
		t.Fatalf("merge-side cost = %d, want 0", cost)
	}
	if base.aggValid&gssAggVisValid != 0 || head.aggValid&gssAggVisValid != 0 {
		t.Fatal("cost-only refill left stale visibility valid")
	}
	if cost, visible := p.cStackPrefixAgg(head); cost != 0 || visible != 2 {
		t.Fatalf("parser refill = (%d, %d), want (0, 2)", cost, visible)
	}
	if base.aggValid&gssAggVisValid == 0 || head.aggValid&gssAggVisValid == 0 {
		t.Fatal("parser refill did not restore visibility validity")
	}
}

func TestMergeCostFillTracksPublishedRecoveryMutation(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	p := &Parser{language: lang, cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)}
	first := &Node{symbol: 1, equivVersion: 1}
	second := &Node{symbol: 1, equivVersion: 1}
	base := &gssNode{entry: newStackEntryNode(1, first), depth: 1}
	head := &gssNode{entry: newStackEntryNode(2, second), prev: base, depth: 2}

	if cost, visible := p.cStackPrefixAgg(head); cost != 0 || visible != 2 {
		t.Fatalf("initial aggregate = (%d, %d), want (0, 2)", cost, visible)
	}

	merge := glrMergeScratch{language: lang}
	gssPrefixAggGen.Add(1)
	if cost := cStackPrefixCostForMerge(&merge, lang, head); cost != 0 {
		t.Fatalf("initial merge cost = %d, want 0", cost)
	}
	if base.aggValid&gssAggVisValid != 0 || head.aggValid&gssAggVisValid != 0 {
		t.Fatal("initial merge fill left visibility valid")
	}

	first.setMissing(true)
	nodeBumpEquivVersion(first)
	wantCost := uint32(cErrCostPerMissingTree + cErrCostPerRecovery)
	if cost := cStackPrefixCostForMerge(&merge, lang, head); cost != wantCost {
		t.Fatalf("mutated merge cost = %d, want %d", cost, wantCost)
	}
	if base.aggValid&gssAggVisValid != 0 || head.aggValid&gssAggVisValid != 0 {
		t.Fatal("mutated merge fill left visibility valid")
	}
	if cost, visible := p.cStackPrefixAgg(head); cost != wantCost || visible != 2 {
		t.Fatalf("parser refill after mutation = (%d, %d), want (%d, 2)", cost, visible, wantCost)
	}
}

func TestContiguousRecoveryAggregateCacheTracksStackAndNodeMutations(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	p := &Parser{
		language:       lang,
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
	}
	first := &Node{symbol: 1, equivVersion: 1}
	missing := &Node{symbol: 1, equivVersion: 1}
	missing.setMissing(true)
	stack := glrStack{
		entries: []stackEntry{newStackEntryNode(1, first)},
		cRec:    &cRecoverState{},
	}

	if cost, visible := p.cStackEntryAgg(&stack); cost != 0 || visible != 1 {
		t.Fatalf("initial aggregate = (%d, %d), want (0, 1)", cost, visible)
	}
	if got, want := stack.cEntryAggGen, gssPrefixAggGen.Load(); got != want {
		t.Fatalf("initial cache generation = %d, want %d", got, want)
	}

	stack.push(2, missing, nil, nil)
	if stack.cEntryAggGen != 0 {
		t.Fatalf("push left cache generation at %d, want invalid", stack.cEntryAggGen)
	}
	wantMissingCost := uint32(cErrCostPerMissingTree + cErrCostPerRecovery)
	if cost, visible := p.cStackEntryAgg(&stack); cost != wantMissingCost || visible != 2 {
		t.Fatalf("post-push aggregate = (%d, %d), want (%d, 2)", cost, visible, wantMissingCost)
	}

	if !stack.truncate(1) {
		t.Fatal("truncate failed")
	}
	if stack.cEntryAggGen != 0 {
		t.Fatalf("truncate left cache generation at %d, want invalid", stack.cEntryAggGen)
	}
	if cost, visible := p.cStackEntryAgg(&stack); cost != 0 || visible != 1 {
		t.Fatalf("post-truncate aggregate = (%d, %d), want (0, 1)", cost, visible)
	}

	child := &Node{symbol: 1, equivVersion: 1}
	first.children = append(first.children, child)
	oldGen := gssPrefixAggGen.Load()
	nodeBumpEquivVersion(first)
	if got := gssPrefixAggGen.Load(); got == oldGen {
		t.Fatalf("recovery-relevant mutation left generation at %d", got)
	}
	if cost, visible := p.cStackEntryAgg(&stack); cost != 0 || visible != 2 {
		t.Fatalf("post-node-mutation aggregate = (%d, %d), want (0, 2)", cost, visible)
	}

	cachedGen := stack.cEntryAggGen
	first.parseState++
	nodeBumpEquivVersionMetadata(first)
	if got := gssPrefixAggGen.Load(); got != cachedGen {
		t.Fatalf("metadata-only mutation changed generation from %d to %d", cachedGen, got)
	}
	if cost, visible := p.cStackEntryAgg(&stack); cost != 0 || visible != 2 {
		t.Fatalf("post-metadata aggregate = (%d, %d), want (0, 2)", cost, visible)
	}
}

func TestExpandedGSSResultPathsKeepIndependentRecoveryAggregates(t *testing.T) {
	lang := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}
	p := &Parser{
		language:       lang,
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
	}
	plain := &Node{symbol: 1, equivVersion: 1}
	errChild := &Node{symbol: 1, equivVersion: 1, endByte: 1}
	errNode := &Node{
		symbol:       errorSymbol,
		equivVersion: 1,
		endByte:      1,
		children:     []*Node{errChild},
	}
	head := gssNodeWithExtraLinks(gssNode{
		entry: newStackEntryNode(1, plain),
		depth: 1,
	}, gssMainLink{entry: newStackEntryNode(1, errNode)})
	source := glrStack{
		gss:  gssStack{head: head},
		cRec: &cRecoverState{},
	}

	paths := appendExpandedGSSResultPaths(nil, source, 2)
	if len(paths) != 2 {
		t.Fatalf("expanded path count = %d, want 2", len(paths))
	}
	if cost, visible := p.cStackEntryAgg(&paths[0]); cost != 0 || visible != 1 {
		t.Fatalf("plain path aggregate = (%d, %d), want (0, 1)", cost, visible)
	}
	wantErrorCost := uint32(cErrCostPerRecovery + cErrCostPerSkippedTree + cErrCostPerSkippedChar)
	if cost, visible := p.cStackEntryAgg(&paths[1]); cost != wantErrorCost || visible != 2 {
		t.Fatalf("error path aggregate = (%d, %d), want (%d, 2)", cost, visible, wantErrorCost)
	}
	if paths[0].cEntryAggCost != 0 || paths[0].cEntryAggVis != 1 {
		t.Fatalf("plain path cache changed to (%d, %d)", paths[0].cEntryAggCost, paths[0].cEntryAggVis)
	}
}

func TestSetGSSMainLinkInvalidatesOnlyChangedRecoveryContribution(t *testing.T) {
	prev := &gssNode{depth: 1}
	payload := &Node{symbol: 1, equivVersion: 1}
	head := &gssNode{
		prev:     prev,
		entry:    newStackEntryNode(1, payload),
		depth:    2,
		aggGen:   gssPrefixAggGen.Load(),
		aggValid: gssAggCostValid | gssAggVisValid,
	}

	gen := gssPrefixAggGen.Load()
	setGSSMainLink(head, 0, prev, newStackEntryNode(2, payload))
	if got := gssPrefixAggGen.Load(); got != gen {
		t.Fatalf("same contribution changed prefix generation from %d to %d", gen, got)
	}
	compactHead := &gssNode{
		prev:  prev,
		entry: newStackEntryCompactFullLeaf(1, &compactFullLeaf{noTreeNode: noTreeNode{symbol: 1}}),
		depth: 2,
	}
	setGSSMainLink(compactHead, 0, prev, newStackEntryCompactFullLeaf(2, &compactFullLeaf{noTreeNode: noTreeNode{symbol: 2}}))
	if got := gssPrefixAggGen.Load(); got != gen {
		t.Fatalf("zero-contribution compact rewrite changed prefix generation from %d to %d", gen, got)
	}

	otherPayload := &Node{symbol: 1, equivVersion: 1}
	setGSSMainLink(head, 0, prev, newStackEntryNode(2, otherPayload))
	changedPayloadGen := gssPrefixAggGen.Load()
	if changedPayloadGen == gen {
		t.Fatalf("changed payload left prefix generation at %d", changedPayloadGen)
	}

	otherPrev := &gssNode{depth: 1}
	setGSSMainLink(head, 0, otherPrev, newStackEntryNode(2, otherPayload))
	if got := gssPrefixAggGen.Load(); got == changedPayloadGen {
		t.Fatalf("changed predecessor left prefix generation at %d", got)
	}
}

func TestResetGSSPrefixPathClearsAndBoundsRetention(t *testing.T) {
	nodes := []*gssNode{{}, {}, {}, {}, {}}
	path := nodes[:1]
	resetGSSPrefixPath(&path)
	if len(path) != 0 || cap(path) != len(nodes) {
		t.Fatalf("retained path shape = len %d cap %d, want len 0 cap %d", len(path), cap(path), len(nodes))
	}
	for i, node := range path[:cap(path)] {
		if node != nil {
			t.Fatalf("retained path[%d] = %p, want nil", i, node)
		}
	}

	path = make([]*gssNode, 1, maxRetainedGSSPrefixPath+1)
	path[0] = &gssNode{}
	resetGSSPrefixPath(&path)
	if path != nil {
		t.Fatalf("oversized path retained len %d cap %d, want nil", len(path), cap(path))
	}
}

func TestParseFinalizationClearsPrefixPathPointers(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	// Pin to production: this test asserts a production-engine internal (the C
	// recovery prefix-path pointer pool), which the compact route never uses.
	parser.SetAdmissionCandidateRoute(false)
	nodes := []*gssNode{{}, {}, {}}
	parser.cPrefixPath = nodes[:1]

	tree := mustParse(t, parser, []byte("42"))
	tree.Release()

	if len(parser.cPrefixPath) != 0 || cap(parser.cPrefixPath) != len(nodes) {
		t.Fatalf("finalized path shape = len %d cap %d, want len 0 cap %d", len(parser.cPrefixPath), cap(parser.cPrefixPath), len(nodes))
	}
	for i, node := range parser.cPrefixPath[:cap(parser.cPrefixPath)] {
		if node != nil {
			t.Fatalf("finalized path[%d] = %p, want nil", i, node)
		}
	}
}

func TestGLRMergeScratchResetClearsPrefixPathPointers(t *testing.T) {
	nodes := []*gssNode{{}, {}, {}}
	var scratch glrMergeScratch
	scratch.cPrefixPath = nodes[:1]
	scratch.reset()
	if len(scratch.cPrefixPath) != 0 || cap(scratch.cPrefixPath) != len(nodes) {
		t.Fatalf("reset path shape = len %d cap %d, want len 0 cap %d", len(scratch.cPrefixPath), cap(scratch.cPrefixPath), len(nodes))
	}
	for i, node := range scratch.cPrefixPath[:cap(scratch.cPrefixPath)] {
		if node != nil {
			t.Fatalf("reset path[%d] = %p, want nil", i, node)
		}
	}
}

func TestParserScratchBudgetIncludesMergePrefixPathGrowth(t *testing.T) {
	var scratch parserScratch
	scratch.setBudget(1)
	scratch.merge.cPrefixPath = make([]*gssNode, 0, 2)
	if !scratch.budgetExhausted() {
		t.Fatal("budget not exhausted after merge prefix-path growth")
	}
}

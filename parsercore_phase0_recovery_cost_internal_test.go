package gotreesitter

import (
	"math/rand"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parsercore_phase0_recovery_cost_internal_test.go carries campaign v7
// tranche B3 stage S2's differential parity obligation (design section 6,
// obligations 1/3; section 4 stage S2 gate): "property tests proving
// equality with the production cost model on a shared corpus." It parses
// (here: hand-builds, to reach ERROR/MISSING shapes deterministically)
// production *Node trees, computes production cost via cNodeErrorCostLang,
// builds the compact RecoveryCostNode records for the identical trees, and
// asserts cost equality node for node -- not just at the root.
//
// This differential test must live in package gotreesitter, not in
// internal/parsercorephase0: it needs same-package access to production's
// unexported cNodeErrorCostLang/cCompareVersions (parser_recover_c.go), and
// internal/parsercorephase0 cannot import this package (it is the
// dependency-neutral compact core; this package imports it, not the other
// way around).

// compactRecoveryCostFixture is a minimal core.RecoveryCostSource backed by
// a plain map, built by compactifyRecoveryCostTree below.
type compactRecoveryCostFixture map[core.SubtreeID]core.RecoveryCostNode

func (f compactRecoveryCostFixture) RecoveryCostNode(id core.SubtreeID) (core.RecoveryCostNode, error) {
	node, ok := f[id]
	if !ok {
		return core.RecoveryCostNode{}, core.ErrRecoveryCostNodeMissing
	}
	return node, nil
}

// compactifyRecoveryCostTree walks a production *Node tree (reading the same
// unexported fields cNodeErrorCostLang itself reads: children, symbol,
// isExtra, isMissing, startByte/endByte, startPoint/endPoint.Row) and
// publishes an equivalent compact RecoveryCostNode fixture, children before
// parents. It returns the fixture, the root's SubtreeID, and a *Node ->
// SubtreeID map for the node-for-node walk.
func compactifyRecoveryCostTree(root *Node) (compactRecoveryCostFixture, core.SubtreeID, map[*Node]core.SubtreeID) {
	fixture := compactRecoveryCostFixture{}
	ids := map[*Node]core.SubtreeID{}
	var next core.SubtreeID
	var walk func(n *Node) core.SubtreeID
	walk = func(n *Node) core.SubtreeID {
		if n == nil {
			return 0
		}
		childIDs := make([]core.SubtreeID, 0, len(n.children))
		for _, c := range n.children {
			childIDs = append(childIDs, walk(c))
		}
		next++
		id := next
		fixture[id] = core.RecoveryCostNode{
			Symbol:    core.Symbol(n.symbol),
			Extra:     n.isExtra(),
			Missing:   n.isMissing(),
			StartByte: n.startByte,
			EndByte:   n.endByte,
			StartRow:  n.startPoint.Row,
			EndRow:    n.endPoint.Row,
			Children:  childIDs,
		}
		ids[n] = id
		return id
	}
	rootID := walk(root)
	return fixture, rootID, ids
}

// recoveryCostTreeCursor advances byte offsets and rows left to right so
// generated leaves and their auto-spanned parents (NewParentNode derives a
// parent's span from its first/last child, tree.go's populateParentNode)
// stay monotonic, like a real parse.
type recoveryCostTreeCursor struct {
	byteOffset uint32
	row        uint32
}

// genRecoveryCostProductionTree builds a small random *Node tree via the
// same public constructors production tests use for hand-built cost fixtures
// (NewLeafNode/NewParentNode, glr_test.go's TestCNodeErrorCostScratchTracksEquivVersion),
// mixing ordinary, ERROR, MISSING, and extra nodes.
func genRecoveryCostProductionTree(random *rand.Rand, cursor *recoveryCostTreeCursor, symbolCount int, depth int) *Node {
	isError := random.Intn(6) == 0
	childCount := 0
	if depth < 4 && random.Intn(3) == 0 {
		childCount = 1 + random.Intn(3)
	}
	sym := Symbol(random.Intn(symbolCount))
	if isError {
		sym = errorSymbol
	}

	if childCount == 0 {
		missing := !isError && random.Intn(8) == 0
		startByte := cursor.byteOffset
		width := uint32(random.Intn(5))
		startRow := cursor.row
		rowBump := uint32(0)
		if random.Intn(5) == 0 {
			rowBump = uint32(1 + random.Intn(2))
		}
		endByte := startByte + width
		endRow := startRow + rowBump
		cursor.byteOffset = endByte
		cursor.row = endRow
		leaf := NewLeafNode(sym, true, startByte, endByte, Point{Row: startRow}, Point{Row: endRow})
		if missing {
			leaf.setMissing(true)
		} else if random.Intn(5) == 0 {
			leaf.setExtra(true)
		}
		if random.Intn(4) == 0 {
			cursor.byteOffset++
		}
		return leaf
	}

	children := make([]*Node, 0, childCount)
	for i := 0; i < childCount; i++ {
		children = append(children, genRecoveryCostProductionTree(random, cursor, symbolCount, depth+1))
		if random.Intn(4) == 0 {
			cursor.byteOffset++
		}
	}
	parent := NewParentNode(sym, true, children, nil, 0)
	if random.Intn(6) == 0 {
		parent.setExtra(true)
	}
	return parent
}

// assertRecoveryCostNodeForNode walks n and its compact translation in
// lockstep, asserting the compact unmemoized, compact memoized (cold, one
// memo per node -- the strongest independent check), and production costs
// all agree, for every node in the tree, not only the root.
func assertRecoveryCostNodeForNode(t *testing.T, trial int, lang *Language, symbols []core.SelectedSymbolPolicy, fixture compactRecoveryCostFixture, ids map[*Node]core.SubtreeID, n *Node) {
	t.Helper()
	if n == nil {
		return
	}
	id, ok := ids[n]
	if !ok {
		t.Fatalf("trial %d: node has no compact id", trial)
	}
	want := cNodeErrorCostLang(lang, n)

	got, err := core.RecoveryNodeErrorCost(symbols, fixture, id)
	if err != nil {
		t.Fatalf("trial %d: RecoveryNodeErrorCost(id=%d): %v", trial, id, err)
	}
	if got != want {
		t.Fatalf("trial %d: compact unmemoized cost for id %d = %d, want %d (production)", trial, id, got, want)
	}

	var memo core.RecoveryCostMemo
	gotMemo, err := core.RecoveryNodeErrorCostMemo(symbols, fixture, &memo, id)
	if err != nil {
		t.Fatalf("trial %d: RecoveryNodeErrorCostMemo(id=%d): %v", trial, id, err)
	}
	if gotMemo != want {
		t.Fatalf("trial %d: compact memoized cost for id %d = %d, want %d (production)", trial, id, gotMemo, want)
	}
	gotVisible, err := core.RecoveryNodeVisibleSubtreeCount(symbols, fixture, id)
	if err != nil {
		t.Fatalf("trial %d: RecoveryNodeVisibleSubtreeCount(id=%d): %v", trial, id, err)
	}
	if wantVisible := cNodeVisibleSubtreeCountUncachedLang(lang, n); uint64(gotVisible) != uint64(wantVisible) {
		t.Fatalf("trial %d: compact visible count for id %d = %d, want %d (production)",
			trial, id, gotVisible, wantVisible)
	}

	for _, c := range n.children {
		assertRecoveryCostNodeForNode(t, trial, lang, symbols, fixture, ids, c)
	}
}

// TestRecoveryCostCompactMatchesProductionNodeForNode is stage S2's parity
// gate (design section 4): differential unit/property parity against
// cNodeErrorCostLang, including the childless-ERROR-leaf rule and the cost
// constants, on a shared corpus of generated tree shapes.
func TestRecoveryCostCompactMatchesProductionNodeForNode(t *testing.T) {
	const symbolCount = 12
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, symbolCount)}
	symbols := make([]core.SelectedSymbolPolicy, symbolCount)
	for i := range lang.SymbolMetadata {
		visible := i%2 == 0
		lang.SymbolMetadata[i] = SymbolMetadata{Visible: visible, Named: true}
		symbols[i] = core.SelectedSymbolPolicy{Visible: visible, Named: true}
	}

	random := rand.New(rand.NewSource(0x5332_6c6f))
	for trial := 0; trial < 400; trial++ {
		cursor := &recoveryCostTreeCursor{}
		root := genRecoveryCostProductionTree(random, cursor, symbolCount, 0)

		fixture, _, ids := compactifyRecoveryCostTree(root)
		assertRecoveryCostNodeForNode(t, trial, lang, symbols, fixture, ids, root)
	}
}

func TestRecoveryVisibleNodeCountMatchesProductionAliasRelabel(t *testing.T) {
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 8)}
	lang.SymbolMetadata[5] = SymbolMetadata{Visible: true, Named: true}
	lang.SymbolMetadata[6] = SymbolMetadata{Visible: false}
	lang.SymbolMetadata[7] = SymbolMetadata{Visible: true, Named: true}
	symbols := make([]core.SelectedSymbolPolicy, len(lang.SymbolMetadata))
	for index := range symbols {
		symbols[index] = core.SelectedSymbolPolicy{
			Visible: lang.SymbolMetadata[index].Visible,
			Named:   lang.SymbolMetadata[index].Named,
		}
	}

	production := NewParentNode(5, true, []*Node{
		NewLeafNode(7, true, 0, 1, Point{}, Point{Column: 1}),
	}, nil, 1)
	fixture := compactRecoveryCostFixture{
		1: {Symbol: 6, StartByte: 0, EndByte: 1},
		2: {Symbol: 5, StartByte: 0, EndByte: 1, Children: []core.SubtreeID{1}, Aliases: []core.Symbol{7}},
	}
	got, err := core.RecoveryNodeVisibleSubtreeCount(symbols, fixture, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := cNodeVisibleSubtreeCountUncachedLang(lang, production)
	if uint64(got) != uint64(want) {
		t.Fatalf("alias-aware compact visible count=%d, want production count %d", got, want)
	}
}

func TestRecoveryErrorRegionCostMatchesUnpublishedProductionNode(t *testing.T) {
	const symbolCount = 8
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, symbolCount)}
	symbols := make([]core.SelectedSymbolPolicy, symbolCount)
	for index := range lang.SymbolMetadata {
		lang.SymbolMetadata[index] = SymbolMetadata{Visible: true, Named: true}
		symbols[index] = core.SelectedSymbolPolicy{Visible: true, Named: true}
	}
	children := []*Node{
		NewLeafNode(2, true, 1, 3, Point{}, Point{}),
		NewLeafNode(3, true, 3, 8, Point{}, Point{Row: 2}),
	}
	region := NewParentNode(errorSymbol, true, children, nil, 0)
	fixture, _, ids := compactifyRecoveryCostTree(region)
	childIDs := []core.SubtreeID{ids[children[0]], ids[children[1]]}
	got, err := core.RecoveryErrorRegionCost(
		symbols, fixture, nil,
		region.startByte, region.startPoint.Row,
		region.endByte, region.endPoint.Row,
		childIDs,
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := cNodeErrorCostLang(lang, region); got != want {
		t.Fatalf("unpublished compact ERROR cost=%d, want production %d", got, want)
	}
}

// TestRecoveryCostConstantsMatchProduction pins the five cost weights
// (parser_recover_c.go:52-58) and the max-cost-difference band
// (parser_recover_c.go:62-69) against the compact port's exported values, so
// a transcription slip fails loudly and independently of the behavioral
// walk above.
func TestRecoveryCostConstantsMatchProduction(t *testing.T) {
	cases := []struct {
		name      string
		got, want uint32
	}{
		{"RecoveryCostPerRecovery", core.RecoveryCostPerRecovery, cErrCostPerRecovery},
		{"RecoveryCostPerMissingTree", core.RecoveryCostPerMissingTree, cErrCostPerMissingTree},
		{"RecoveryCostPerSkippedTree", core.RecoveryCostPerSkippedTree, cErrCostPerSkippedTree},
		{"RecoveryCostPerSkippedLine", core.RecoveryCostPerSkippedLine, cErrCostPerSkippedLine},
		{"RecoveryCostPerSkippedChar", core.RecoveryCostPerSkippedChar, cErrCostPerSkippedChar},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d (production)", tc.name, tc.got, tc.want)
		}
	}
}

// TestRecoveryCompareVersionsMatchesProduction is exact-parity obligation 3
// (design section 6): cCompareVersions tie-breaking, differentially tested
// against the compact port across generated (cost, nodeCount, dynPrec,
// isInError) tuples.
func TestRecoveryCompareVersionsMatchesProduction(t *testing.T) {
	toProduction := map[core.RecoveryComparison]cErrorComparison{
		core.RecoveryComparisonTakeLeft:    cErrorComparisonTakeLeft,
		core.RecoveryComparisonPreferLeft:  cErrorComparisonPreferLeft,
		core.RecoveryComparisonNone:        cErrorComparisonNone,
		core.RecoveryComparisonPreferRight: cErrorComparisonPreferRight,
		core.RecoveryComparisonTakeRight:   cErrorComparisonTakeRight,
	}
	random := rand.New(rand.NewSource(0x74696531))
	for trial := 0; trial < 3000; trial++ {
		// Bias cost toward the tie-break band around recoveryMaxCostDifference
		// (1800) so both the "clearly worse" and "close enough" arms fire
		// often, not just the isInError short-circuits.
		aCost := uint32(random.Intn(4000))
		bCost := aCost + uint32(random.Intn(3600)) - 1800
		if int32(bCost) < 0 {
			bCost = uint32(random.Intn(4000))
		}
		a := cErrorStatus{
			cost: aCost, nodeCount: random.Intn(15), dynPrec: random.Intn(5) - 2,
			isInError: random.Intn(2) == 0,
		}
		b := cErrorStatus{
			cost: bCost, nodeCount: random.Intn(15), dynPrec: random.Intn(5) - 2,
			isInError: random.Intn(2) == 0,
		}

		coreA := core.RecoveryErrorStatus{Cost: a.cost, NodeCount: a.nodeCount, DynPrec: a.dynPrec, IsInError: a.isInError}
		coreB := core.RecoveryErrorStatus{Cost: b.cost, NodeCount: b.nodeCount, DynPrec: b.dynPrec, IsInError: b.isInError}

		want := cCompareVersions(a, b)
		got := core.RecoveryCompareVersions(coreA, coreB)
		if toProduction[got] != want {
			t.Fatalf("trial %d: a=%+v b=%+v: RecoveryCompareVersions=%v (-> %v), want production %v",
				trial, a, b, got, toProduction[got], want)
		}
	}
}

//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func compactBorrowedMaterializationFixture(t *testing.T) (*Parser, *compactIncrementalReuseSession, *Node, core.MaterializationSubtreeView, *compactReplayStates, diagnosticParserCorePointIndex) {
	t.Helper()
	lang := &Language{
		TokenCount:     2,
		SymbolNames:    []string{"end", "a", "_hidden", "item", "root", "alias"},
		SymbolMetadata: []SymbolMetadata{{}, {Visible: true}, {}, {Visible: true, Named: true}, {Visible: true, Named: true}, {Visible: true, Named: true}},
		AliasSequences: [][]Symbol{nil, {5}},
	}
	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, false, 0, 1, Point{}, Point{Column: 1})
	node := newParentNodeInArena(arena, 3, true, []*Node{leaf}, nil, 0)
	node.parseState, node.preGotoState = 7, 4
	node.dynamicPrecedence = 40000
	node.setCompactMaterialized(true)
	node.setCompactParseStateProof(true)
	node.setCompactPreGotoStateProof(true)
	root := newParentNodeInArena(arena, 4, true, []*Node{node}, nil, 0)
	oldTree := newTreeWithArenas(root, []byte("a"), lang, arena, nil)
	t.Cleanup(func() {
		if oldTree.root != nil {
			oldTree.Release()
		}
	})
	session := &compactIncrementalReuseSession{oldTree: oldTree, nodes: []*Node{node}}
	view := core.MaterializationSubtreeView{
		Symbol: 3, StartByte: 0, EndByte: 1, DynamicPrecedence: 40000,
		ReusedKey: 1, ReusedPreGotoState: 4, ReusedState: 7,
	}
	states := &compactReplayStates{
		parseState: []StateID{0, 7}, preGotoState: []StateID{0, 4},
		psKnown: []bool{false, true}, preKnown: []bool{false, true},
	}
	points := diagnosticParserCorePointIndex{lineStarts: []uint32{0}}
	return parser, session, node, view, states, points
}

func TestCompactBorrowedMaterializationAuthenticatesNode(t *testing.T) {
	parser, session, node, view, states, points := compactBorrowedMaterializationFixture(t)
	parent := node.parent
	got, err := session.materializeBorrowed(parser, 1, view, states, &points)
	if err != nil || got != node {
		t.Fatalf("borrowed node = %p, error = %v; want %p", got, err, node)
	}
	if node.parent != parent || node.parseState != 7 || node.preGotoState != 4 ||
		!node.hasCompactParseStateProof() || !node.hasCompactPreGotoStateProof() || node.dynamicPrecedence != 40000 {
		t.Fatal("borrowed materialization changed node metadata")
	}
	for _, test := range []struct {
		name   string
		change func(*core.MaterializationSubtreeView)
	}{
		{"key", func(v *core.MaterializationSubtreeView) { v.ReusedKey = 2 }},
		{"symbol", func(v *core.MaterializationSubtreeView) { v.Symbol = 4 }},
		{"span", func(v *core.MaterializationSubtreeView) { v.EndByte = 2 }},
		{"state", func(v *core.MaterializationSubtreeView) { v.ReusedState = 8 }},
		{"pre_state", func(v *core.MaterializationSubtreeView) { v.ReusedPreGotoState = 5 }},
		{"precedence", func(v *core.MaterializationSubtreeView) { v.DynamicPrecedence = 7232 }},
		{"terminal", func(v *core.MaterializationSubtreeView) { v.Terminal = true }},
		{"alias", func(v *core.MaterializationSubtreeView) { v.Aliases = []core.Symbol{5} }},
		{"children", func(v *core.MaterializationSubtreeView) { v.Children = []core.SubtreeID{2} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := view
			test.change(&changed)
			if _, err := session.materializeBorrowed(parser, 1, changed, states, &points); err == nil {
				t.Fatal("invalid borrowed descriptor was accepted")
			}
		})
	}
	states.preKnown[1] = false
	if _, err := session.materializeBorrowed(parser, 1, view, states, &points); err == nil {
		t.Fatal("borrowed node without replay proof was accepted")
	}
	states.preKnown[1] = true
	var missing *compactIncrementalReuseSession
	if _, err := missing.materializeBorrowed(parser, 1, view, states, &points); err == nil {
		t.Fatal("borrowed node without ownership session was accepted")
	}
}

func TestCompactBorrowedMaterializationRetainsArena(t *testing.T) {
	parser, session, node, _, _, _ := compactBorrowedMaterializationFixture(t)
	oldArena := node.ownerArena
	childArena := acquireNodeArena(arenaClassFull)
	node.children[0] = newLeafNodeInArena(childArena, 1, false, 0, 1, Point{}, Point{Column: 1})
	session.oldTree.borrowedArena = append(session.oldTree.borrowedArena, childArena)
	arena := acquireNodeArena(arenaClassFull)
	root := newParentNodeInArenaNoLinksWithFieldSources(arena, 4, true, []*Node{node}, nil, nil, 0, true)
	oldParent := node.parent
	session.reuseState.markReused(node, arena)
	var links []*Node
	tree := parser.buildResultFromNodes([]*Node{root}, []byte("a"), arena, session.oldTree, &session.reuseState, &links)
	if tree == nil {
		arena.Release()
		t.Fatal("result builder returned no tree")
	}
	defer tree.Release()
	if len(tree.borrowedArena) != 2 || tree.borrowedArena[0] != oldArena || oldArena.refs.Load() != 2 || childArena.refs.Load() != 2 {
		t.Fatalf("result did not retain the borrowed arenas: borrowed=%d refs=%d child_refs=%d", len(tree.borrowedArena), oldArena.refs.Load(), childArena.refs.Load())
	}
	if node.parent != oldParent {
		t.Fatal("result construction changed the borrowed parent link")
	}
	session.oldTree.Release()
	if oldArena.refs.Load() != 1 || childArena.refs.Load() != 1 || node.symbol != 3 || node.children[0].symbol != 1 || tree.root.children[0] != node {
		t.Fatal("old tree release invalidated the reused subtree")
	}
}

func TestCompactBorrowedMaterializationCoverage(t *testing.T) {
	_, _, node, _, _, _ := compactBorrowedMaterializationFixture(t)
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.appendBorrowed(1, node, 1); err != nil {
		t.Fatal(err)
	}
	nodes := []*Node{nil, node}
	if !coverage.hasBorrowedSubtree(node) || coverage.hasVisibleTerminalNode(0, 1, node, nodes) {
		t.Fatal("borrowed nonterminal was not distinguished from a terminal")
	}
	if _, _, gap, err := diagnosticParserCoreAcceptedDerivationLeafCoverageGap(&coverage, []byte("a"), 0, 1, nil); err != nil || gap {
		t.Fatalf("borrowed derivation coverage: gap=%t error=%v", gap, err)
	}
	if _, _, gap, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(node, []byte("a"), 0, 1, 2, &coverage, nodes, nil); err != nil || gap {
		t.Fatalf("borrowed public coverage: gap=%t error=%v", gap, err)
	}
	if err := coverage.appendBorrowed(2, node, 1); err == nil {
		t.Fatal("overlapping borrowed coverage was accepted")
	}
	coverage.reset()
	if coverage.hasBorrowedSubtree(node) {
		t.Fatal("coverage reset retained a borrowed node")
	}
}

func TestCompactBorrowedMaterializationRejectsProjection(t *testing.T) {
	parser, _, node, _, _, _ := compactBorrowedMaterializationFixture(t)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	hidden := newParentNodeInArenaNoLinksWithFieldSources(arena, 2, false, []*Node{node}, nil, nil, 0, true)
	for _, entry := range []*Node{node, hidden} {
		entries := []stackEntry{newStackEntryNode(0, entry)}
		if err := validateCompactBorrowedReduceInputs(parser, entries, 1, arena); err == nil {
			t.Fatal("alias of borrowed subtree was accepted")
		}
		if err := validateCompactBorrowedReduceInputs(parser, entries, 0, arena); err != nil {
			t.Fatal(err)
		}
		if err := validateCompactBorrowedReduceProjection(parser, entries, []*Node{node}, arena); err != nil {
			t.Fatal(err)
		}
		if err := validateCompactBorrowedReduceProjection(parser, entries, []*Node{hidden}, arena); err != nil {
			t.Fatalf("preserved hidden wrapper lost its borrowed projection: %v", err)
		}
		for _, children := range [][]*Node{nil, {node, node}, {hidden, node}, {hidden, hidden}} {
			if err := validateCompactBorrowedReduceProjection(parser, entries, children, arena); err == nil {
				t.Fatal("changed borrowed projection was accepted")
			}
		}
	}
}

func TestCompactBorrowedMaterializationClearsSessionOnError(t *testing.T) {
	_, session, _, _, _, _ := compactBorrowedMaterializationFixture(t)
	scratch := parserCoreRunnerScratch{incrementalReuse: session}
	_, err := materializeDiagnosticParserCoreAcceptedSelectionWithRootFinalization(nil, core.Head{}, nil, nil, nil, &scratch, false, false, diagnosticParserCoreFinalizeDefault)
	if err == nil || scratch.incrementalReuse != nil {
		t.Fatal("failed materialization retained the reuse session")
	}
}

func TestCompactBorrowedMaterializationReplayChecksTransition(t *testing.T) {
	parser := &Parser{denseLimit: 8, language: &Language{
		TokenCount: 2, SymbolCount: 4, StateCount: 8, InitialState: 1, LargeStateCount: 8,
		ParseTable: [][]uint16{nil, nil, nil, nil, {0, 0, 0, 7}},
	}}
	view := core.MaterializationReplayView{Symbol: 3, ReusedKey: 1, ReusedPreGotoState: 4, ReusedState: 7}
	state, known, err := parser.replayCompactMaterializationTransition(4, view)
	if err != nil || !known || state != 7 {
		t.Fatalf("borrowed replay: state=%d known=%t error=%v", state, known, err)
	}
	view.ReusedPreGotoState = 3
	if _, _, err := parser.replayCompactMaterializationTransition(4, view); err == nil {
		t.Fatal("borrowed replay accepted a mismatched parent state")
	}
	view.ReusedPreGotoState, view.ReusedState = 4, 6
	if _, _, err := parser.replayCompactMaterializationTransition(4, view); err == nil {
		t.Fatal("borrowed replay accepted a mismatched goto state")
	}
	view.ReusedState, view.Terminal = 7, true
	if _, _, err := parser.replayCompactMaterializationTransition(4, view); err == nil {
		t.Fatal("borrowed replay accepted a terminal classification")
	}
}

func TestCompactBorrowedMaterializationProjectionMultiplicity(t *testing.T) {
	parser, _, first, _, _, _ := compactBorrowedMaterializationFixture(t)
	second := newParentNodeInArenaNoLinksWithFieldSources(first.ownerArena, 3, true, []*Node{first.children[0]}, nil, nil, 0, true)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	input := newParentNodeInArenaNoLinksWithFieldSources(arena, 2, false, []*Node{first, second}, nil, nil, 0, true)
	entries := []stackEntry{newStackEntryNode(0, input)}
	for _, children := range [][]*Node{{first, second}, {input}} {
		if err := validateCompactBorrowedReduceProjection(parser, entries, children, arena); err != nil {
			t.Fatalf("unchanged borrowed identities declined: %v", err)
		}
	}
	for _, children := range [][]*Node{{first, first}, {first}, {first, second, second}} {
		if err := validateCompactBorrowedReduceProjection(parser, entries, children, arena); err == nil {
			t.Fatal("changed borrowed multiplicity was accepted")
		}
	}
}

func TestCompactBorrowedMaterializationProjectionPolling(t *testing.T) {
	parser, _, first, _, _, _ := compactBorrowedMaterializationFixture(t)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	const width = 2048
	children := make([]*Node, width)
	for i := range children {
		children[i] = newParentNodeInArenaNoLinksWithFieldSources(first.ownerArena, 3, true, []*Node{first.children[0]}, nil, nil, 0, true)
	}
	input := newParentNodeInArenaNoLinksWithFieldSources(arena, 2, false, children, nil, nil, 0, true)
	output := newParentNodeInArenaNoLinksWithFieldSources(arena, 2, false, children, nil, nil, 0, true)
	entries := []stackEntry{newStackEntryNode(0, input)}
	var scratch compactBorrowedProjectionScratch
	polls := 0
	if err := validateCompactBorrowedReduceProjectionWithScratch(parser, entries, []*Node{input}, arena, &scratch, func() error {
		polls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if polls != 2 || scratch.borrowedPeak != 0 {
		t.Fatalf("unchanged hidden wrapper expanded its descendants: polls=%d borrowed_peak=%d", polls, scratch.borrowedPeak)
	}
	polls = 0
	if err := validateCompactBorrowedReduceProjectionWithScratch(parser, entries, []*Node{output}, arena, &scratch, func() error {
		polls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if polls < 3 || polls > 5*width/256+4 {
		t.Fatalf("projection work did not remain linear: polls=%d", polls)
	}
	if scratch.footprintBytes() < uint64(width)*96 {
		t.Fatal("projection accounting omitted retained identity maps")
	}
	stop := errors.New("cancel projection")
	polls = 0
	err := validateCompactBorrowedReduceProjectionWithScratch(parser, entries, []*Node{output}, arena, &scratch, func() error {
		polls++
		if polls > 1 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || polls != 2 {
		t.Fatalf("projection did not stop at its next poll: polls=%d error=%v", polls, err)
	}
	if len(scratch.roots) != 0 || len(scratch.borrowed) != 0 || len(scratch.walk) != 0 {
		t.Fatal("cancelled projection retained node references")
	}
}

func TestCompactBorrowedMaterializationProjectionMemoryBudget(t *testing.T) {
	parser, session, first, _, _, _ := compactBorrowedMaterializationFixture(t)
	var scheduler diagnosticParserCoreGenericScheduler
	scheduler.options.compactIncrementalReuse = session
	session.scheduler = &scheduler
	baseline := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	const width = 512
	entries := make([]stackEntry, width)
	children := make([]*Node, width)
	for i := range children {
		children[i] = newParentNodeInArenaNoLinksWithFieldSources(first.ownerArena, 3, true, []*Node{first.children[0]}, nil, nil, 0, true)
		entries[i] = newStackEntryNode(0, children[i])
	}
	if got := diagnosticParserCoreSchedulerFootprintBytes(&scheduler); got != baseline {
		t.Fatal("session accounting included borrowed node arena contents")
	}
	scheduler.options.stopControlMemoryBudgetBytes = int64(baseline + 4096)
	stop := errors.New("projection memory budget")
	polls := 0
	err := validateCompactBorrowedReduceProjectionWithScratch(parser, entries, children, arena, &session.projection, func() error {
		polls++
		if scheduler.stopControlMemoryBudgetReasonWithAdditionalBytes(0) == ParseStopMemoryBudget {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || polls != 2 {
		t.Fatalf("projection allocation escaped the budget poll: polls=%d error=%v", polls, err)
	}
	if len(session.projection.roots) != 0 || len(session.projection.borrowed) != 0 || len(session.projection.walk) != 0 {
		t.Fatal("budget rejection retained borrowed references")
	}
	retained := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
	scheduler.options.stopControlMemoryBudgetBytes = int64(retained + 128)
	if reason := scheduler.stopControlMemoryBudgetReasonWithAdditionalBytes(0); reason != ParseStopNone {
		t.Fatalf("unexpected stop below the combined budget: %s", reason)
	}
	if reason := scheduler.stopControlMemoryBudgetReasonWithAdditionalBytes(128); reason != ParseStopMemoryBudget {
		t.Fatal("combined budget omitted materialization arena bytes")
	}
	if reason := scheduler.stopControlMemoryBudgetReasonWithAdditionalBytes(^uint64(0)); reason != ParseStopMemoryBudget {
		t.Fatal("combined budget overflow bypassed the limit")
	}
}

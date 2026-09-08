package parsercorephase0

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestMaterializationOrderRejectsRepeatedOwnership(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf, leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "repeated public-tree ownership") {
		t.Fatalf("repeated compact child error = %v", err)
	}
	visits := 0
	if err := compact.VisitMaterializationPostorder([]SubtreeID{parent}, nil, func(SubtreeID, MaterializationSubtreeView) error {
		visits++
		return nil
	}); err == nil || !strings.Contains(err.Error(), "repeated public-tree ownership") {
		t.Fatalf("fused repeated compact child error = %v", err)
	}
	if visits != 1 {
		t.Fatalf("fused repeated compact child published %d visits", visits)
	}
}

func TestPrebuiltMaterializationRejectsSharedDescendants(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, roots := range [][]SubtreeID{{parent, leaf}, {leaf, parent}} {
		var scratch MaterializationPostorderScratch
		err := compact.VisitMaterializationPostorderPrebuilt(roots, nil, &scratch, 0, nil,
			func(SubtreeID) bool { return true },
			func(SubtreeID, *MaterializationSubtreeView) error { t.Fatal("visited a prebuilt subtree"); return nil })
		if err == nil || !strings.Contains(err.Error(), "repeated public-tree ownership") {
			t.Fatalf("roots=%v: repeated descendant error=%v", roots, err)
		}
	}
	var scratch MaterializationPostorderScratch
	if err := compact.VisitMaterializationPostorderPrebuilt([]SubtreeID{parent}, nil, &scratch, 0, nil,
		func(SubtreeID) bool { return true },
		func(SubtreeID, *MaterializationSubtreeView) error { t.Fatal("visited a prebuilt subtree"); return nil }); err != nil {
		t.Fatalf("valid prebuilt tree: %v", err)
	}
}

func TestMaterializationOrderRejectsCycle(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := compact.subtree(parent)
	if err != nil {
		t.Fatal(err)
	}
	compact.children[record.firstChild] = parent
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("compact cycle error = %v", err)
	}
	if err := compact.VisitMaterializationPostorder([]SubtreeID{parent}, nil, func(SubtreeID, MaterializationSubtreeView) error {
		return nil
	}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("fused compact cycle error = %v", err)
	}
}

func TestMaterializationOrderValidatesRemappedMetadata(t *testing.T) {
	tables := &fakeTable{
		fields: map[uint16][]FieldMapEntry{7: {{FieldID: 3, ChildIndex: 0}}},
		aliases: map[productionKey][]Symbol{
			{productionID: 7, childCount: 1}: {9},
		},
	}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	extra, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, extra: true, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(
		subtreeRecord{symbol: 3, productionID: 7, endByte: 1},
		[]SubtreeID{extra, child},
		[]FieldMapEntry{{FieldID: 3, ChildIndex: 1}},
		[]Symbol{0, 9},
	)
	if err != nil {
		t.Fatal(err)
	}
	order, err := compact.MaterializationOrder([]SubtreeID{parent}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 || order[2] != parent {
		t.Fatalf("materialization order = %v", order)
	}
	var fusedOrder []SubtreeID
	var parentView MaterializationSubtreeView
	if err := compact.VisitMaterializationPostorder([]SubtreeID{parent}, nil, func(id SubtreeID, view MaterializationSubtreeView) error {
		fusedOrder = append(fusedOrder, id)
		if id == parent {
			parentView = view
			parentView.Children = append([]SubtreeID(nil), view.Children...)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fusedOrder, order) {
		t.Fatalf("fused materialization order = %v want %v", fusedOrder, order)
	}
	if parentView.Symbol != 3 || parentView.ProductionID != 7 || parentView.StartByte != 0 || parentView.EndByte != 1 ||
		parentView.Terminal || parentView.Extra || parentView.External || !reflect.DeepEqual(parentView.Children, []SubtreeID{extra, child}) {
		t.Fatalf("fused parent view = %+v", parentView)
	}

	record, err := compact.subtree(parent)
	if err != nil {
		t.Fatal(err)
	}
	compact.fields[record.firstField].ChildIndex = 0
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("field metadata mismatch error = %v", err)
	}
	if err := compact.VisitMaterializationPostorder([]SubtreeID{parent}, nil, func(SubtreeID, MaterializationSubtreeView) error { return nil }); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("fused field metadata mismatch error = %v", err)
	}
	compact.fields[record.firstField].ChildIndex = 1
	compact.aliases[record.firstAlias+1] = 8
	if _, err := compact.MaterializationOrder([]SubtreeID{parent}, nil); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("alias metadata mismatch error = %v", err)
	}
	if err := compact.VisitMaterializationPostorder([]SubtreeID{parent}, nil, func(SubtreeID, MaterializationSubtreeView) error { return nil }); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("fused alias metadata mismatch error = %v", err)
	}
}

func TestMaterializationMetadataAuthenticationIsConstructionScoped(t *testing.T) {
	tables := &fakeTable{
		fields: map[uint16][]FieldMapEntry{7: {{FieldID: 3, ChildIndex: 0}}},
		aliases: map[productionKey][]Symbol{
			{productionID: 7, childCount: 1}: {9},
		},
	}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}

	// Generic publication invalidates the Core-wide construction invariant.
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if compact.metadataConstructionAuthenticated {
		t.Fatal("generic terminal append retained construction authentication")
	}
	parent, err := compact.appendSubtree(
		subtreeRecord{symbol: 3, productionID: 7, endByte: 1},
		[]SubtreeID{leaf},
		[]FieldMapEntry{{FieldID: 3, ChildIndex: 1}},
		[]Symbol{8},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.VisitMaterializationPostorder([]SubtreeID{parent}, nil, func(SubtreeID, MaterializationSubtreeView) error { return nil }); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("generic metadata error = %v", err)
	}

	if err := compact.Reset(); err != nil {
		t.Fatal(err)
	}
	if !compact.metadataConstructionAuthenticated {
		t.Fatal("Reset did not restore empty-Core construction authentication")
	}

	// The authenticated terminal seam preserves the Core invariant and accepts
	// only statically ratcheted metadata-trivial terminal literals in production.
	trustedLeaf, err := compact.appendAuthenticatedTerminal(subtreeRecord{symbol: 4, endByte: 1, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	trustedLeafRecord, _ := compact.subtree(trustedLeaf)
	if !compact.metadataConstructionAuthenticated || !trustedLeafRecord.terminal || trustedLeafRecord.childCount != 0 || trustedLeafRecord.fieldCount != 0 || trustedLeafRecord.aliasCount != 0 {
		t.Fatalf("authenticated terminal = %+v core-authenticated=%t, want terminal with no metadata", trustedLeafRecord, compact.metadataConstructionAuthenticated)
	}
}

func TestReductionMetadataAuthenticationRequiresSuccessfulCanonicalRemap(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 2, symbol: 9}: {{Type: ActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 7}},
		},
		gotos: map[tableCell]StateID{{state: 1, symbol: 3}: 4},
		fields: map[uint16][]FieldMapEntry{
			7: {{FieldID: 3, ChildIndex: 0}},
		},
		aliases: map[productionKey][]Symbol{{productionID: 7, childCount: 1}: {9}},
	}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.appendDiagnosticPayload(head, 2, Token{Symbol: 2, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	frontier, err := compact.ReduceOutputs(head, 9, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := compact.Derivations(frontier[0].Head)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compact.subtree(paths[0].Payloads[0]); err != nil {
		t.Fatal(err)
	}
	if !compact.metadataConstructionAuthenticated {
		t.Fatal("canonical reduction invalidated construction authentication")
	}
	if err := compact.VisitMaterializationPostorder(paths[0].Payloads, nil, func(SubtreeID, MaterializationSubtreeView) error { return nil }); err != nil {
		t.Fatalf("authenticated canonical reduction failed materialization: %v", err)
	}

	badTables := &fakeTable{
		actions: tables.actions,
		gotos:   tables.gotos,
		fields:  map[uint16][]FieldMapEntry{7: {{FieldID: 3, ChildIndex: 1}}},
	}
	bad, err := New(badTables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	badHead, _ := bad.Seed(1, 0)
	badHead, err = bad.appendDiagnosticPayload(badHead, 2, Token{Symbol: 2, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	before := len(bad.subtrees)
	if _, err := bad.ReduceOutputs(badHead, 9, 0, ForkOrder{}); err == nil || !strings.Contains(err.Error(), "field child index") {
		t.Fatalf("invalid remap error = %v", err)
	}
	if len(bad.subtrees) != before {
		t.Fatalf("failed remap published %d subtrees, want %d", len(bad.subtrees), before)
	}
	if !bad.metadataConstructionAuthenticated {
		t.Fatal("failed authenticated remap invalidated construction authentication")
	}
}

func TestMaterializationOrderPollsAndPublishesNothingOnStop(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	polls := 0
	if order, err := compact.MaterializationOrder([]SubtreeID{leaf}, func() error {
		polls++
		return errMaterializationOrderStop
	}); err != errMaterializationOrderStop || order != nil || polls != 1 {
		t.Fatalf("stopped order=%v err=%v polls=%d", order, err, polls)
	}
}

func TestVisitMaterializationPostorderDeepIterativeTree(t *testing.T) {
	const depth = 20_000
	compact, err := New(&fakeTable{}, Limits{MaxSubtrees: depth + 1})
	if err != nil {
		t.Fatal(err)
	}
	root, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < depth; i++ {
		root, err = compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{root}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	visits := 0
	if err := compact.VisitMaterializationPostorder([]SubtreeID{root}, nil, func(_ SubtreeID, view MaterializationSubtreeView) error {
		if visits == 0 && !view.Terminal {
			t.Fatal("deep iterative traversal did not visit the leaf first")
		}
		visits++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if visits != depth+1 {
		t.Fatalf("deep iterative visits=%d want=%d", visits, depth+1)
	}
}

func TestVisitMaterializationPostorderStopsBeforeVisit(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	polls, visits := 0, 0
	err = compact.VisitMaterializationPostorder([]SubtreeID{leaf}, func() error {
		polls++
		return errMaterializationOrderStop
	}, func(SubtreeID, MaterializationSubtreeView) error {
		visits++
		return nil
	})
	if err != errMaterializationOrderStop || polls != 1 || visits != 0 {
		t.Fatalf("stopped err=%v polls=%d visits=%d", err, polls, visits)
	}
}

func TestVisitMaterializationPostorderScratchReusesAndResets(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var scratch MaterializationPostorderScratch
	want := []SubtreeID{leaf, root}
	for run := 0; run < 2; run++ {
		var got []SubtreeID
		if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{root}, nil, &scratch, func(id SubtreeID, _ MaterializationSubtreeView) error {
			got = append(got, id)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("run %d order=%v want=%v", run, got, want)
		}
		requireMaterializationPostorderScratchReset(t, &scratch)
		if run == 0 {
			continue
		}
	}
	if cap(scratch.colors) < len(compact.subtrees)+1 || cap(scratch.frames) < 64 {
		t.Fatalf("scratch did not retain traversal storage: colors=%d frames=%d", cap(scratch.colors), cap(scratch.frames))
	}
	colorStorage := &scratch.colors[:cap(scratch.colors)][0]
	frameStorage := &scratch.frames[:cap(scratch.frames)][0]
	if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{root}, nil, &scratch, func(SubtreeID, MaterializationSubtreeView) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if colorStorage != &scratch.colors[:cap(scratch.colors)][0] ||
		frameStorage != &scratch.frames[:cap(scratch.frames)][0] {
		t.Fatal("postorder traversal replaced reusable scratch storage")
	}
	requireMaterializationPostorderScratchReset(t, &scratch)
}

func TestVisitMaterializationPostorderScratchRollsBackVisitorFailure(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var scratch MaterializationPostorderScratch
	if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{root}, nil, &scratch, func(SubtreeID, MaterializationSubtreeView) error {
		return errMaterializationOrderStop
	}); err != errMaterializationOrderStop {
		t.Fatalf("visitor failure=%v want=%v", err, errMaterializationOrderStop)
	}
	requireMaterializationPostorderScratchReset(t, &scratch)
	colorStorage := &scratch.colors[:cap(scratch.colors)][0]
	frameStorage := &scratch.frames[:cap(scratch.frames)][0]

	var got []SubtreeID
	if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{root}, nil, &scratch, func(id SubtreeID, _ MaterializationSubtreeView) error {
		got = append(got, id)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if want := []SubtreeID{leaf, root}; !slices.Equal(got, want) {
		t.Fatalf("post-rollback order=%v want=%v", got, want)
	}
	if colorStorage != &scratch.colors[:cap(scratch.colors)][0] ||
		frameStorage != &scratch.frames[:cap(scratch.frames)][0] {
		t.Fatal("rollback discarded reusable scratch storage")
	}
	requireMaterializationPostorderScratchReset(t, &scratch)
}

func TestVisitMaterializationPostorderScratchRejectsInvalidScratchBeforePoll(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	polls := 0
	poll := func() error {
		polls++
		return nil
	}
	visit := func(SubtreeID, MaterializationSubtreeView) error { return nil }
	if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{leaf}, poll, nil, visit); err == nil ||
		!strings.Contains(err.Error(), "scratch is nil") {
		t.Fatalf("nil scratch error=%v", err)
	}
	busy := MaterializationPostorderScratch{inUse: true}
	if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{leaf}, poll, &busy, visit); err == nil ||
		!strings.Contains(err.Error(), "already in use") {
		t.Fatalf("busy scratch error=%v", err)
	}
	if polls != 0 {
		t.Fatalf("invalid scratch invoked poll %d times", polls)
	}
}

func TestVisitMaterializationPostorderScratchRollsBackFinalPollFailure(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var scratch MaterializationPostorderScratch
	polls := 0
	poll := func() error {
		polls++
		if polls == 2 {
			return errMaterializationOrderStop
		}
		return nil
	}
	if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{leaf}, poll, &scratch, func(SubtreeID, MaterializationSubtreeView) error {
		return nil
	}); err != errMaterializationOrderStop {
		t.Fatalf("final poll failure=%v want=%v", err, errMaterializationOrderStop)
	}
	requireMaterializationPostorderScratchReset(t, &scratch)
	if err := compact.VisitMaterializationPostorderWithScratch([]SubtreeID{leaf}, nil, &scratch, func(SubtreeID, MaterializationSubtreeView) error {
		return nil
	}); err != nil {
		t.Fatalf("reuse after final poll failure: %v", err)
	}
	requireMaterializationPostorderScratchReset(t, &scratch)
}

func requireMaterializationPostorderScratchReset(t *testing.T, scratch *MaterializationPostorderScratch) {
	t.Helper()
	if scratch.inUse || len(scratch.colors) != 0 || len(scratch.frames) != 0 {
		t.Fatalf("scratch was not reset: inUse=%t colors=%d frames=%d", scratch.inUse, len(scratch.colors), len(scratch.frames))
	}
	for index, color := range scratch.colors[:cap(scratch.colors)] {
		if color != 0 {
			t.Fatalf("scratch color %d was not cleared: %d", index, color)
		}
	}
	for index, frame := range scratch.frames[:cap(scratch.frames)] {
		if frame.record != nil {
			t.Fatalf("scratch frame %d retained a compact record", index)
		}
	}
}

var errMaterializationOrderStop = materializationOrderTestError("stop")

type materializationOrderTestError string

func (err materializationOrderTestError) Error() string { return string(err) }

//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"math"
	"reflect"
	"testing"
)

func phase0ARouteLimits() Phase0AObserverLimits {
	limits := phase0AReductionLimits()
	limits.MaxPopRoutes = 256
	limits.MaxPopRouteLinks = 1024
	limits.MaxSelectionCapabilities = 32
	limits.MaxAcceptedSelections = 16
	limits.MaxAcceptedLinks = 1024
	limits.MaxSelectedOccurrences = 128
	limits.MaxSelectedDepth = 4096
	limits.MaxSelectedIndexEntries = 16_384
	limits.MaxSelectedIndexBytes = 4 << 20
	return limits
}

func phase0ACaptureSelection(t *testing.T, core *Core, head Head) Phase0AAcceptedSelectionCapability {
	t.Helper()
	capability, err := core.CapturePhase0ASelectionCapability(head)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func TestPhase0APopRouteProofMatchesProductionOrdinals(t *testing.T) {
	core, head := newSharedReductionFixture(t, true)
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	frontier, err := core.Reduce(head, 9, 0, ForkOrder{})
	if err != nil || len(frontier) != 2 {
		t.Fatalf("reduce frontier=%v err=%v", frontier, err)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.PopRoutes) != 2 || len(proof.PopRouteLinks) != 2 {
		t.Fatalf("pop routes=%+v links=%+v", proof.PopRoutes, proof.PopRouteLinks)
	}
	for ordinal, route := range proof.PopRoutes {
		if route.PopOrdinal != uint32(ordinal) || route.Head != head.Node || route.Predecessor == 0 || route.LinkCount != 1 || route.Occurrence == (ConstructionOccurrenceKey{}) || route.Edge == (IncomingEdgeKey{}) || route.RolledBack {
			t.Fatalf("route %d=%+v", ordinal, route)
		}
		link := proof.PopRouteLinks[route.FirstLink]
		if link.Route != route.ID || link.Ordinal != 0 || link.Link == 0 || link.Payload == 0 || link.RolledBack {
			t.Fatalf("route %d link=%+v", ordinal, link)
		}
		mutation, err := Phase0AResolveCommittedConstructionOccurrence(core, run, route.Occurrence)
		if err != nil || mutation.Edge != route.Edge || mutation.Predecessor != route.Predecessor {
			t.Fatalf("route %d occurrence=%+v err=%v", ordinal, mutation, err)
		}
	}
}

func TestPhase0APopRouteProofCoversLinklessZeroChildReduction(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 2}}},
		gotos:   map[tableCell]StateID{{state: 3, symbol: 2}: 4},
	}
	core, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := core.Seed(1, 0)
	head, err := core.appendDiagnosticPayload(seed, 3, Token{Symbol: 10, EndByte: 1, Extra: true}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	frontier, parseErr := core.Reduce(head, 1, 0, ForkOrder{})
	if parseErr != nil || len(frontier) != 1 {
		t.Fatalf("zero-child reduction frontier=%v err=%v", frontier, parseErr)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if proofErr != nil {
		t.Fatal(proofErr)
	}
	if len(proof.PopRoutes) != 1 || len(proof.PopRouteLinks) != 0 || len(proof.ReductionOccurrences) != 1 {
		t.Fatalf("zero-child proof routes=%+v links=%+v reductions=%+v", proof.PopRoutes, proof.PopRouteLinks, proof.ReductionOccurrences)
	}
	route := proof.PopRoutes[0]
	if route.PopOrdinal != 0 || route.Head != head.Node || route.Predecessor != head.Node || route.FirstLink != 0 || route.LinkCount != 0 || route.Occurrence == (ConstructionOccurrenceKey{}) || route.Edge == (IncomingEdgeKey{}) || route.RolledBack {
		t.Fatalf("zero-child route=%+v", route)
	}
	mutation, resolveErr := Phase0AResolveCommittedConstructionOccurrence(core, run, route.Occurrence)
	if resolveErr != nil || mutation.Edge != route.Edge || mutation.Predecessor != head.Node {
		t.Fatalf("zero-child occurrence=%+v err=%v", mutation, resolveErr)
	}
	paths, derivationErr := core.Derivations(frontier[0])
	if derivationErr != nil || len(paths) != 1 || len(paths[0].Payloads) != 2 {
		t.Fatalf("zero-child derivations=%+v err=%v", paths, derivationErr)
	}
	parent, subtreeErr := core.Subtree(paths[0].Payloads[1])
	if subtreeErr != nil || parent.Symbol != 2 || parent.StartByte != 1 || parent.EndByte != 1 || len(parent.Children) != 0 {
		t.Fatalf("zero-child parent=%+v err=%v", parent, subtreeErr)
	}
}

func TestPhase0APopRouteProofPreservesMixedAndTrailingPayloadIdentity(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 20, ChildCount: 2}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 20}: 4},
	}
	core, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4})
	if err != nil {
		t.Fatal(err)
	}
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	seed, _ := core.Seed(1, 0)
	head := seed
	inputs := []struct {
		state StateID
		token Token
		score int64
	}{
		{state: 2, token: Token{Symbol: 30, EndByte: 1}, score: 1},
		{state: 2, token: Token{Symbol: 31, StartByte: 1, EndByte: 2, Extra: true}, score: 2},
		{state: 3, token: Token{Symbol: 32, StartByte: 2, EndByte: 3}, score: 4},
		{state: 3, token: Token{Symbol: 33, StartByte: 3, EndByte: 4, Extra: true}, score: 8},
		{state: 3, token: Token{Symbol: 34, StartByte: 4, EndByte: 5, Extra: true}, score: 16},
	}
	wantPayloads := make([]SubtreeID, 0, len(inputs))
	for index, input := range inputs {
		payload, appendErr := core.appendSubtree(subtreeRecord{
			symbol: input.token.Symbol, startByte: input.token.StartByte, endByte: input.token.EndByte,
			extra: input.token.Extra, terminal: true,
		}, nil, nil, nil)
		if appendErr != nil {
			t.Fatal(appendErr)
		}
		wantPayloads = append(wantPayloads, payload)
		head = phase0AObservedTerminalPrivate(t, core, payload, head.Node, input.state, input.token.EndByte, input.token.Extra, input.score, ForkOrder{Present: true, Value: uint64(index + 1)})
	}
	derivations, err := core.Derivations(head)
	if err != nil || len(derivations) != 1 || len(derivations[0].Payloads) != len(inputs) {
		t.Fatalf("mixed fixture derivations=%+v err=%v", derivations, err)
	}
	if !phase0ASameSubtrees(wantPayloads, derivations[0].Payloads) {
		t.Fatalf("mixed construction payloads=%v derivation=%v", wantPayloads, derivations[0].Payloads)
	}
	production, err := core.popPaths(head.Node, 2)
	if err != nil || len(production) != 1 || len(production[0].children) != 3 || len(production[0].trailing) != 2 || production[0].prev != seed.Node || production[0].score != 7 || production[0].order != (ForkOrder{Present: true, Value: 5}) || production[0].startByte != 0 || production[0].structuralEnd != 3 {
		t.Fatalf("mixed production pop paths=%+v err=%v", production, err)
	}
	for index, payload := range production[0].children {
		if payload != wantPayloads[index] {
			t.Fatalf("mixed child payload %d=%d want=%d", index, payload, wantPayloads[index])
		}
	}
	for index, payload := range production[0].trailing {
		if payload.payload != wantPayloads[index+3] {
			t.Fatalf("trailing payload %d=%d want=%d", index, payload.payload, wantPayloads[index+3])
		}
	}
	wantLinks := make([]LinkID, 0, len(inputs))
	for nodeID := head.Node; nodeID != seed.Node; {
		node := *core.nodes.At(int(nodeID - 1))
		if node.linkCount != 1 || node.firstLink == 0 {
			t.Fatalf("mixed physical node %d links=%d first=%d", nodeID, node.linkCount, node.firstLink)
		}
		linkID := LinkID(node.firstLink)
		wantLinks = append(wantLinks, linkID)
		nodeID = core.links.At(int(linkID - 1)).prev
	}
	for left, right := 0, len(wantLinks)-1; left < right; left, right = left+1, right-1 {
		wantLinks[left], wantLinks[right] = wantLinks[right], wantLinks[left]
	}

	frontier, parseErr := core.Reduce(head, 1, 0, ForkOrder{})
	if parseErr != nil || len(frontier) != 1 {
		t.Fatalf("mixed reduction frontier=%v err=%v", frontier, parseErr)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if proofErr != nil || len(proof.PopRoutes) != 1 || len(proof.PopRouteLinks) != len(inputs) || len(proof.TrailingExtraMigrations) != 2 {
		t.Fatalf("mixed route proof=%+v err=%v", proof, proofErr)
	}
	route := proof.PopRoutes[0]
	if route.Head != head.Node || route.Predecessor != seed.Node || route.FirstLink != 0 || route.LinkCount != uint32(len(inputs)) || route.RetainedLinkCount != 3 || route.TrailingLinkCount != 2 || route.Occurrence == (ConstructionOccurrenceKey{}) || route.Edge == (IncomingEdgeKey{}) || route.RolledBack {
		t.Fatalf("mixed route=%+v", route)
	}
	for index, row := range proof.PopRouteLinks {
		wantSegment, wantSegmentOrdinal := Phase0APopRouteRetained, uint32(index)
		if index >= 3 {
			wantSegment, wantSegmentOrdinal = Phase0APopRouteTrailing, uint32(index-3)
		}
		if row.Route != route.ID || row.Ordinal != uint32(index) || row.Segment != wantSegment || row.SegmentOrdinal != wantSegmentOrdinal || row.Link != wantLinks[index] || row.Payload != wantPayloads[index] || row.ScoreDelta != inputs[index].score || !row.HasOrder || row.Order != uint64(index+1) || row.RolledBack {
			t.Fatalf("mixed route link %d=%+v want link=%d payload=%d", index, row, wantLinks[index], wantPayloads[index])
		}
	}
	capability := phase0ACaptureSelection(t, core, frontier[0])
	if err := core.ObservePhase0AAcceptedSelection(capability); err != nil {
		t.Fatal(err)
	}
	proof, proofErr = Phase0AObserverProof(core, run)
	if proofErr != nil || len(proof.SelectedOccurrenceTrees) != 1 || proof.SelectedOccurrenceTrees[0].OccurrenceCount != 6 || proof.SelectedOccurrenceTrees[0].RootCount != 3 || proof.SelectedOccurrenceTrees[0].MigratedCount != 2 || proof.SelectedOccurrences[0].ChildCount != 3 {
		t.Fatalf("mixed selected occurrence proof=%+v rows=%+v err=%v", proof.SelectedOccurrenceTrees, proof.SelectedOccurrences, proofErr)
	}
}

func TestPhase0APopRouteRollbackTombstonesAndRetryRemainsDistinct(t *testing.T) {
	core, head := newSharedReductionFixture(t, true)
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	core.limits.MaxNodes = uint32(core.nodes.Len())
	if _, err := core.Reduce(head, 9, 0, ForkOrder{}); err == nil {
		t.Fatal("node-capped reduction succeeded")
	}
	core.limits.MaxNodes = 4096
	if _, err := core.Reduce(head, 9, 0, ForkOrder{}); err != nil {
		t.Fatal(err)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatal(err)
	}
	rolled, live := 0, 0
	ids := make(map[uint64]bool)
	for _, route := range proof.PopRoutes {
		if ids[route.ID] {
			t.Fatalf("pop route serial reused: %+v", proof.PopRoutes)
		}
		ids[route.ID] = true
		if route.RolledBack {
			rolled++
		} else {
			live++
		}
	}
	if rolled != 2 || live != 2 {
		t.Fatalf("rollback/live routes=%d/%d rows=%+v", rolled, live, proof.PopRoutes)
	}
	for _, link := range proof.PopRouteLinks {
		if link.RolledBack != (link.Route <= 2) {
			t.Fatalf("route-link tombstone mismatch: %+v", link)
		}
	}
}

func TestPhase0APopRouteCapIsAtomic(t *testing.T) {
	core, head := newSharedReductionFixture(t, true)
	limits := phase0ARouteLimits()
	limits.MaxPopRoutes = 1
	run := phase0ABeginShiftProvenanceRun(t, core, limits)
	frontier, parseErr := core.Reduce(head, 9, 0, ForkOrder{})
	if parseErr != nil || len(frontier) != 2 {
		t.Fatalf("observer cap changed reduction frontier=%v err=%v", frontier, parseErr)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if !isPhase0AError(proofErr, Phase0AErrorRecordCap, "") || len(proof.PopRoutes) != 0 || len(proof.PopRouteLinks) != 0 {
		t.Fatalf("cap proof=%+v err=%v", proof, proofErr)
	}
}

func TestPhase0AAcceptedSpineRejectsRolledCapabilityAfterHeadReuse(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	root, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
	var rolled Head
	var stale Phase0AAcceptedSelectionCapability
	_ = core.ApplyAtomic(func() error {
		rolled = phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
		stale = phase0ACaptureSelection(t, core, rolled)
		return errPhase0ATestRollback
	})
	live := phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
	if rolled != live || stale.head != live {
		t.Fatalf("fixture did not reuse head rolled=%+v stale=%+v live=%+v", rolled, stale.head, live)
	}
	if err := core.ObservePhase0AAcceptedSelection(stale); !isPhase0AError(err, Phase0AErrorStaleReference, "") {
		t.Fatalf("stale accepted capability error=%v", err)
	}
	proof, _ := Phase0AObserverProof(core, run)
	if len(proof.SelectionCapabilities) != 1 || !proof.SelectionCapabilities[0].RolledBack || proof.SelectionCapabilities[0].Serial != stale.serial || len(proof.AcceptedSelections) != 0 || len(proof.AcceptedLinks) != 0 {
		t.Fatalf("stale accepted capability published rows: %+v", proof)
	}
}

func TestPhase0AAcceptedSpineResolvesLiveCapabilityAfterLinkReuse(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	root, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
	var rolled Head
	var rolledCapability Phase0AAcceptedSelectionCapability
	_ = core.ApplyAtomic(func() error {
		rolled = phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
		rolledCapability = phase0ACaptureSelection(t, core, rolled)
		return errPhase0ATestRollback
	})
	live := phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
	if rolled.Node != live.Node {
		t.Fatalf("fixture did not reuse node/link ordinals rolled=%+v live=%+v", rolled, live)
	}
	liveCapability := phase0ACaptureSelection(t, core, live)
	if rolledCapability.serial == liveCapability.serial {
		t.Fatalf("selection capability serial reused: rolled=%+v live=%+v", rolledCapability, liveCapability)
	}
	if err := core.ObservePhase0AAcceptedSelection(liveCapability); err != nil {
		t.Fatal(err)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.AcceptedSelections) != 1 || len(proof.AcceptedLinks) != 1 {
		t.Fatalf("accepted proof=%+v", proof)
	}
	selection, selected := proof.AcceptedSelections[0], proof.AcceptedLinks[0]
	if selection.Namespace != run || selection.Generation != 1 || selection.Capability != liveCapability.serial || selection.Head != live.Node || selection.LinkCount != 1 || selected.Namespace != run || selected.Generation != 1 || selected.Ordinal != 0 || selected.Link == 0 || selected.Occurrence == (ConstructionOccurrenceKey{}) || selected.Edge == (IncomingEdgeKey{}) {
		t.Fatalf("selection=%+v selected=%+v", selection, selected)
	}
	if len(proof.SelectionCapabilities) != 2 || !proof.SelectionCapabilities[0].RolledBack || proof.SelectionCapabilities[1].RolledBack {
		t.Fatalf("selection capabilities=%+v", proof.SelectionCapabilities)
	}
	rolledBindings, liveBindings := 0, 0
	for _, binding := range proof.Bindings {
		if binding.Link != selected.Link {
			continue
		}
		if binding.RolledBack {
			rolledBindings++
		} else {
			liveBindings++
		}
	}
	if rolledBindings != 1 || liveBindings != 1 {
		t.Fatalf("accepted link reuse bindings=%+v", proof.Bindings)
	}
}

func TestPhase0AAcceptedSpineCompoundByteCapIsAtomic(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	root, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
	head := phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
	capability := phase0ACaptureSelection(t, core, head)
	phase0AObservers.Lock()
	beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
	phase0AObservers.limits.MaxBytes = beforeBytes + phase0AAcceptedSelectionBytes + phase0AAcceptedLinkBytes - 1
	phase0AObservers.Unlock()
	if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorByteCap, "") {
		t.Fatalf("accepted cap error=%v", err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if !isPhase0AError(proofErr, Phase0AErrorByteCap, "") || len(proof.AcceptedSelections) != 0 || len(proof.AcceptedLinks) != 0 {
		t.Fatalf("accepted cap proof=%+v err=%v", proof, proofErr)
	}
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	if observer.route.nextSelection != 0 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
		t.Errorf("accepted cap mutated generation=%d records=%d/%d bytes=%d/%d", observer.route.nextSelection, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
	}
	phase0AObservers.Unlock()
}

func TestPhase0AAcceptedSnapshotIsImmutableAcrossRunReuse(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	root, _ := core.Seed(1, 0)
	firstPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
	firstHead := phase0AObservedPrivate(t, core, firstPayload, root.Node, 2, 1)
	firstCapability := phase0ACaptureSelection(t, core, firstHead)
	if err := core.ObservePhase0AAcceptedSelection(firstCapability); err != nil {
		t.Fatal(err)
	}
	firstProof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatal(err)
	}
	frozenProof, err := Phase0AObserverProof(core, run)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.EndRun(run); err != nil {
		t.Fatal(err)
	}
	nextRun, err := core.BeginRun()
	if err != nil {
		t.Fatal(err)
	}
	secondPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 1, terminal: true}, nil, nil, nil)
	secondHead := phase0AObservedPrivate(t, core, secondPayload, root.Node, 3, 1)
	secondCapability := phase0ACaptureSelection(t, core, secondHead)
	if err := core.ObservePhase0AAcceptedSelection(secondCapability); err != nil {
		t.Fatal(err)
	}
	secondProof, err := Phase0AObserverProof(core, nextRun)
	if err != nil {
		t.Fatal(err)
	}
	if nextRun == run || secondProof.Namespace != nextRun || len(secondProof.AcceptedSelections) != 1 || secondProof.AcceptedSelections[0].Namespace != nextRun {
		t.Fatalf("next-run proof=%+v old=%+v new=%+v", secondProof, run, nextRun)
	}
	if firstProof.Namespace != run || !reflect.DeepEqual(firstProof, frozenProof) {
		t.Fatalf("first snapshot mutated across run reuse: %+v", firstProof)
	}
}

func TestPhase0AAcceptedSpineRejectsAmbiguousPhysicalHead(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	root, _ := core.Seed(1, 0)
	firstPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
	secondPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, endByte: 1, terminal: true}, nil, nil, nil)
	first := phase0AObservedCondense(t, core, firstPayload, root.Node, 2, 1)
	second := phase0AObservedCondense(t, core, secondPayload, root.Node, 2, 1)
	if first.change != condenseNew || second.change != condenseUpdated {
		t.Fatalf("ambiguous fixture first=%+v second=%+v", first, second)
	}
	derivations, err := core.Derivations(second.head)
	if err != nil || len(derivations) != 2 {
		t.Fatalf("ambiguous fixture derivations=%+v err=%v", derivations, err)
	}
	capability := phase0ACaptureSelection(t, core, second.head)
	if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorAmbiguousReference, "") {
		t.Fatalf("ambiguous accepted head error=%v", err)
	}
	proof, _ := Phase0AObserverProof(core, run)
	if len(proof.AcceptedSelections) != 0 || len(proof.AcceptedLinks) != 0 || len(proof.SelectionCapabilities) != 1 {
		t.Fatalf("ambiguous accepted head published rows: %+v", proof)
	}
}

func TestPhase0ARouteAndAcceptanceCapsAreAtomic(t *testing.T) {
	t.Run("selection-capability-cap", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		limits := phase0ARouteLimits()
		limits.MaxSelectionCapabilities = 1
		_ = phase0ABeginShiftProvenanceRun(t, core, limits)
		head, _ := core.Seed(1, 0)
		_ = phase0ACaptureSelection(t, core, head)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
		phase0AObservers.Unlock()
		if _, err := core.CapturePhase0ASelectionCapability(head); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
			t.Fatalf("selection capability cap error=%v", err)
		}
		phase0AObservers.Lock()
		if len(observer.route.capabilities) != 1 || observer.route.nextCapability != 1 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
			t.Errorf("selection capability cap mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
		}
		phase0AObservers.Unlock()
	})

	t.Run("selection-capability-serial-overflow", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		_ = phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
		head, _ := core.Seed(1, 0)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		observer.route.nextCapability = math.MaxUint64
		beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
		phase0AObservers.Unlock()
		if _, err := core.CapturePhase0ASelectionCapability(head); !isPhase0AError(err, Phase0AErrorCounterOverflow, Phase0ACounterSelectionCapability) {
			t.Fatalf("selection capability overflow error=%v", err)
		}
		phase0AObservers.Lock()
		if len(observer.route.capabilities) != 0 || observer.route.nextCapability != math.MaxUint64 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
			t.Errorf("selection capability overflow mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
		}
		phase0AObservers.Unlock()
	})

	t.Run("pop-route-link-cap", func(t *testing.T) {
		core, head := newSharedReductionFixture(t, true)
		limits := phase0ARouteLimits()
		limits.MaxPopRouteLinks = 1
		run := phase0ABeginShiftProvenanceRun(t, core, limits)
		production, err := core.popPaths(head.Node, 1)
		if err != nil || len(production) != 2 {
			t.Fatalf("pop-route cap fixture=%+v err=%v", production, err)
		}
		if err := core.ApplyAtomic(func() error {
			phase0ABeginReductionConstruction(core, uint64(len(production)))
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
			phase0AObservers.Unlock()
			phase0AObservePopRoutes(core, head.Node, 1, production)
			phase0AObservers.Lock()
			if len(observer.route.popRoutes) != 0 || len(observer.route.popLinks) != 0 || observer.route.nextPopRoute != 0 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
				t.Errorf("pop-route link cap mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
			}
			phase0AObservers.Unlock()
			phase0AFinishReductionConstruction(core)
			return nil
		}); err != nil {
			t.Fatalf("pop-route link cap changed parser transaction: %v", err)
		}
		_, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorRecordCap, "") {
			t.Fatalf("pop-route link cap proof error=%v", proofErr)
		}
	})

	t.Run("pop-route-serial-overflow", func(t *testing.T) {
		core, head := newSharedReductionFixture(t, true)
		run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
		production, _ := core.popPaths(head.Node, 1)
		if err := core.ApplyAtomic(func() error {
			phase0ABeginReductionConstruction(core, uint64(len(production)))
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			observer.route.nextPopRoute = math.MaxUint64
			beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
			phase0AObservers.Unlock()
			phase0AObservePopRoutes(core, head.Node, 1, production)
			phase0AObservers.Lock()
			if len(observer.route.popRoutes) != 0 || len(observer.route.popLinks) != 0 || observer.route.nextPopRoute != math.MaxUint64 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
				t.Errorf("pop-route overflow mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
			}
			phase0AObservers.Unlock()
			phase0AFinishReductionConstruction(core)
			return nil
		}); err != nil {
			t.Fatalf("pop-route overflow changed parser transaction: %v", err)
		}
		_, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorCounterOverflow, "") {
			t.Fatalf("pop-route overflow proof error=%v", proofErr)
		}
	})

	t.Run("accepted-link-cap", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
		limits := phase0ARouteLimits()
		limits.MaxAcceptedLinks = 1
		run := phase0ABeginShiftProvenanceRun(t, core, limits)
		root, _ := core.Seed(1, 0)
		firstPayload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		secondPayload, _ := core.appendSubtree(subtreeRecord{symbol: 21, startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
		first := phase0AObservedPrivate(t, core, firstPayload, root.Node, 2, 1)
		head := phase0AObservedPrivate(t, core, secondPayload, first.Node, 3, 2)
		capability := phase0ACaptureSelection(t, core, head)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
		phase0AObservers.Unlock()
		if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
			t.Fatalf("accepted-link cap error=%v", err)
		}
		phase0AObservers.Lock()
		if len(observer.route.acceptedSelections) != 0 || len(observer.route.acceptedLinks) != 0 || observer.route.nextSelection != 0 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
			t.Errorf("accepted-link cap mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
		}
		phase0AObservers.Unlock()
		_, _ = Phase0AObserverProof(core, run)
	})

	t.Run("accepted-selection-cap", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
		limits := phase0ARouteLimits()
		limits.MaxAcceptedSelections = 1
		_ = phase0ABeginShiftProvenanceRun(t, core, limits)
		root, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		head := phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
		first := phase0ACaptureSelection(t, core, head)
		if err := core.ObservePhase0AAcceptedSelection(first); err != nil {
			t.Fatal(err)
		}
		second := phase0ACaptureSelection(t, core, head)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
		phase0AObservers.Unlock()
		if err := core.ObservePhase0AAcceptedSelection(second); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
			t.Fatalf("accepted-selection cap error=%v", err)
		}
		phase0AObservers.Lock()
		if len(observer.route.acceptedSelections) != 1 || len(observer.route.acceptedLinks) != 1 || observer.route.nextSelection != 1 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
			t.Errorf("accepted-selection cap mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
		}
		phase0AObservers.Unlock()
	})

	t.Run("accepted-selection-serial-overflow", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
		_ = phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
		root, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		head := phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
		capability := phase0ACaptureSelection(t, core, head)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		observer.route.nextSelection = math.MaxUint64
		beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
		phase0AObservers.Unlock()
		if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorCounterOverflow, Phase0ACounterAcceptedSelectionGeneration) {
			t.Fatalf("accepted-selection overflow error=%v", err)
		}
		phase0AObservers.Lock()
		if len(observer.route.acceptedSelections) != 0 || len(observer.route.acceptedLinks) != 0 || observer.route.nextSelection != math.MaxUint64 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
			t.Errorf("accepted-selection overflow mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
		}
		phase0AObservers.Unlock()
	})

	for _, test := range []struct {
		name     string
		setLimit func(records, bytes, requiredRecords, requiredBytes uint64)
		kind     Phase0AErrorKind
	}{
		{name: "global-record-cap", kind: Phase0AErrorRecordCap, setLimit: func(records, _, requiredRecords, _ uint64) {
			phase0AObservers.limits.MaxRecords = records + requiredRecords - 1
		}},
		{name: "global-byte-cap", kind: Phase0AErrorByteCap, setLimit: func(_, bytes, _, requiredBytes uint64) {
			phase0AObservers.limits.MaxBytes = bytes + requiredBytes - 1
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
			_ = phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
			root, _ := core.Seed(1, 0)
			payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
			head := phase0AObservedPrivate(t, core, payload, root.Node, 2, 1)
			capability := phase0ACaptureSelection(t, core, head)
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
			test.setLimit(beforeRecords, beforeBytes, 2, phase0AAcceptedSelectionBytes+phase0AAcceptedLinkBytes)
			phase0AObservers.Unlock()
			if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, test.kind, "") {
				t.Fatalf("accepted global cap error=%v", err)
			}
			phase0AObservers.Lock()
			if len(observer.route.acceptedSelections) != 0 || len(observer.route.acceptedLinks) != 0 || observer.route.nextSelection != 0 || phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
				t.Errorf("accepted global cap mutated observer=%+v records=%d/%d bytes=%d/%d", observer.route, phase0AObservers.records, beforeRecords, phase0AObservers.bytes, beforeBytes)
			}
			phase0AObservers.Unlock()
		})
	}
}

func TestPhase0AAcceptedSpineFailsClosedOnMissingAndCyclicReferences(t *testing.T) {
	t.Run("missing binding", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
		root, _ := core.Seed(1, 0)
		payload, _ := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1, terminal: true}, nil, nil, nil)
		head, err := core.appendPrivate(2, 1, linkInput{prev: root.Node, payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
		capability := phase0ACaptureSelection(t, core, head)
		if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorMissingReference, "") {
			t.Fatalf("missing-binding error=%v", err)
		}
		proof, _ := Phase0AObserverProof(core, run)
		if len(proof.AcceptedSelections) != 0 || len(proof.AcceptedLinks) != 0 {
			t.Fatalf("missing binding published acceptance: %+v", proof)
		}
	})

	t.Run("factor cycle", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: 1, Kind: Phase0AExpressionFactorChoice, Selector: 1, Left: 1, Right: 1})
		observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: 1})
		observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: 1, SelectedLowerLink: 1, SourceLowerLink: 1, Branch: Phase0ASelectorLeft})
		_, err := phase0AResolveAcceptedExpressionLocked(observer, 1, 1)
		phase0AObservers.Unlock()
		if !isPhase0AError(err, Phase0AErrorCyclicReference, "") {
			t.Fatalf("cycle error=%v", err)
		}
		_ = run
	})

	t.Run("nested factor translates lower link", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		_ = phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		observer.factor.expressions = append(observer.factor.expressions,
			Phase0AExpressionRecord{ID: 1, Kind: Phase0AExpressionDirect, Occurrence: ConstructionOccurrenceKey{Event: ConstructionEventKey{Attempt: AttemptKey{Run: observer.run, AttemptEpoch: 1}}}, Edge: IncomingEdgeKey{Event: ConstructionEventKey{Attempt: AttemptKey{Run: observer.run, AttemptEpoch: 1}}}},
			Phase0AExpressionRecord{ID: 2, Kind: Phase0AExpressionFactorChoice, Selector: 1, Left: 1, Right: 1},
			Phase0AExpressionRecord{ID: 3, Kind: Phase0AExpressionFactorChoice, Selector: 2, Left: 2, Right: 1},
		)
		observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: 1}, Phase0ASelectorRecord{ID: 2})
		observer.factor.selectorRoutes = append(observer.factor.selectorRoutes,
			Phase0ASelectorRouteRecord{Selector: 2, SelectedLowerLink: 20, SourceLowerLink: 10, Branch: Phase0ASelectorLeft},
			Phase0ASelectorRouteRecord{Selector: 1, SelectedLowerLink: 10, SourceLowerLink: 5, Branch: Phase0ASelectorLeft},
		)
		resolved, resolvedLower, err := phase0AResolveAcceptedExpressionAndLowerLocked(observer, 3, 20)
		phase0AObservers.Unlock()
		if err != nil || resolved.ID != 1 || resolved.Kind != Phase0AExpressionDirect || resolvedLower != 5 {
			t.Fatalf("nested factor resolved=%+v lower=%d err=%v", resolved, resolvedLower, err)
		}
	})

	t.Run("direct expression preserves lower link", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		_ = phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		event := ConstructionEventKey{Attempt: AttemptKey{Run: observer.run, AttemptEpoch: 1}}
		observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: 1, Kind: Phase0AExpressionDirect, Occurrence: ConstructionOccurrenceKey{Event: event}, Edge: IncomingEdgeKey{Event: event}})
		resolved, resolvedLower, err := phase0AResolveAcceptedExpressionAndLowerLocked(observer, 1, 77)
		phase0AObservers.Unlock()
		if err != nil || resolved.ID != 1 || resolvedLower != 77 {
			t.Fatalf("direct factor resolved=%+v lower=%d err=%v", resolved, resolvedLower, err)
		}
	})
}

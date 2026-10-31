//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"errors"
	"testing"
)

func phase0ASelectedSingleReduction(t *testing.T, limits Phase0AObserverLimits) (*Core, CoreRunNamespace, Head) {
	t.Helper()
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 2, symbol: 1}: {{Type: ActionReduce, Symbol: 20, ChildCount: 1}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 20}: 3},
	}
	core, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	run := phase0ABeginShiftProvenanceRun(t, core, limits)
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := core.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := phase0AObservedTerminalPrivate(t, core, leaf, seed.Node, 2, 1, false, 0, ForkOrder{})
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("single reduction=%v err=%v", frontier, err)
	}
	return core, run, frontier[0]
}

func TestPhase0ASelectedOccurrencesKeepRepeatedPayloadPositionsAndSharedEventSlots(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
	first, err := core.appendPrivate(2, 1, linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	second, err := core.appendPrivate(2, 1, linkInput{prev: first.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}

	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	event := ConstructionEventKey{Attempt: AttemptKey{Run: observer.run, AttemptEpoch: 1}, Serial: 1}
	observer.nextEvent, observer.nextEdge = 1, 2
	observer.nextOccurrenceSlot[event] = 2
	observer.mutations = append(observer.mutations, Phase0AMutationRecord{Kind: Phase0AMutationEvent, Event: event, Payload: payload, ConstructionKind: Phase0AConstructionOrdinaryTerminal})
	observer.eventIndex[event] = 0
	for slot, predecessor := range []NodeID{seed.Node, first.Node} {
		occurrence := ConstructionOccurrenceKey{Event: event, Slot: uint32(slot + 1)}
		edge := IncomingEdgeKey{Event: event, Serial: uint64(slot + 1)}
		row := Phase0AMutationRecord{Event: event, Occurrence: occurrence, Edge: edge, Payload: payload, Predecessor: predecessor, ConstructionKind: Phase0AConstructionOrdinaryTerminal}
		row.Kind = Phase0AMutationOccurrence
		observer.occurrenceIndex[occurrence] = uint64(len(observer.mutations))
		observer.mutations = append(observer.mutations, row)
		row.Kind = Phase0AMutationEdge
		observer.edgeIndex[edge] = uint64(len(observer.mutations))
		observer.mutations = append(observer.mutations, row)
	}
	firstOccurrence := observer.mutations[1]
	secondOccurrence := observer.mutations[3]
	accepted := []Phase0AAcceptedLinkRecord{
		{Link: LinkID((*core.nodes.At(int(first.Node - 1))).firstLink), Payload: payload, Occurrence: firstOccurrence.Occurrence, Edge: firstOccurrence.Edge},
		{Link: LinkID((*core.nodes.At(int(second.Node - 1))).firstLink), Payload: payload, ResolvedLowerLink: LinkID((*core.nodes.At(int(first.Node - 1))).firstLink), Occurrence: secondOccurrence.Occurrence, Edge: secondOccurrence.Edge, BoundExpression: 1, ResolvedExpression: 2},
	}
	selectedIndex := phase0ABuildSelectedIndex(observer)
	tree, records, buildErr := phase0ABuildSelectedOccurrenceSnapshotLocked(core, observer, selectedIndex, 1, accepted)
	duplicateRoots := []Phase0AAcceptedLinkRecord{accepted[0], accepted[0]}
	duplicateRoots[1].BoundExpression, duplicateRoots[1].ResolvedExpression = 1, 2
	_, _, duplicateErr := phase0ABuildSelectedOccurrenceSnapshotLocked(core, observer, selectedIndex, 2, duplicateRoots)
	phase0AObservers.Unlock()
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if tree.OccurrenceCount != 2 || tree.RootCount != 2 || tree.TerminalCount != 2 || tree.ReductionCount != 0 || len(records) != 2 ||
		records[0].Payload != records[1].Payload || records[0].Occurrence.Event != records[1].Occurrence.Event || records[0].Occurrence.Slot != 1 || records[1].Occurrence.Slot != 2 {
		t.Fatalf("repeated/shared occurrence tree=%+v records=%+v", tree, records)
	}
	rawDigest, err := phase0ARawSelectedCompactDigest(core, []SubtreeID{payload, payload})
	if err != nil || rawDigest != tree.Digest {
		t.Fatalf("repeated digest raw=%x selected=%x err=%v", rawDigest, tree.Digest, err)
	}
	if !isPhase0AError(duplicateErr, Phase0AErrorAmbiguousReference, "") {
		t.Fatalf("duplicate multi-root identity error=%v", duplicateErr)
	}
	_ = run
}

func TestPhase0ASelectedOccurrencesFailClosedOnRouteIdentityCorruption(t *testing.T) {
	tests := []struct {
		name   string
		kind   Phase0AErrorKind
		mutate func(*Core, *phase0AObserver)
	}{
		{name: "missing", kind: Phase0AErrorMissingReference, mutate: func(_ *Core, observer *phase0AObserver) { observer.route.popRoutes = nil }},
		{name: "ambiguous", kind: Phase0AErrorAmbiguousReference, mutate: func(_ *Core, observer *phase0AObserver) {
			observer.route.popRoutes = append(observer.route.popRoutes, observer.route.popRoutes[0])
		}},
		{name: "cross-boundary", kind: Phase0AErrorCrossBoundary, mutate: func(_ *Core, observer *phase0AObserver) {
			observer.route.popRoutes[0].Predecessor++
		}},
		{name: "stale-link", kind: Phase0AErrorStaleReference, mutate: func(core *Core, observer *phase0AObserver) {
			row := observer.route.popLinks[observer.route.popRoutes[0].FirstLink]
			{
				linkStorageMutation := core.links.At(int(row.Link - 1))
				linkStorageMutation.payload++
				core.links.Set(int(row.Link-1), linkStorageMutation)
			}
		}},
		{name: "cycle", kind: Phase0AErrorCyclicReference, mutate: func(core *Core, observer *phase0AObserver) {
			route := observer.route.popRoutes[0]
			var parentExpression Phase0AExpressionID
			for _, expression := range observer.factor.expressions {
				if expression.Occurrence == route.Occurrence && expression.Edge == route.Edge && !expression.RolledBack {
					parentExpression = expression.ID
					break
				}
			}
			for index := range observer.factor.bindings {
				if observer.factor.bindings[index].Link == observer.route.popLinks[route.FirstLink].Link && !observer.factor.bindings[index].RolledBack {
					observer.factor.bindings[index].Expression = parentExpression
				}
			}
			occurrence := observer.mutations[observer.occurrenceIndex[route.Occurrence]]
			row := &observer.route.popLinks[route.FirstLink]
			row.Payload = occurrence.Payload
			{
				linkStorageMutation := core.links.At(int(row.Link - 1))
				linkStorageMutation.payload = occurrence.Payload
				core.links.Set(int(row.Link-1), linkStorageMutation)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, run, head := phase0ASelectedSingleReduction(t, phase0ARouteLimits())
			capability := phase0ACaptureSelection(t, core, head)
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			test.mutate(core, observer)
			phase0AObservers.Unlock()
			if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, test.kind, "") {
				t.Fatalf("corrupt route error=%v want=%s", err, test.kind)
			}
			proof, _ := Phase0AObserverProof(core, run)
			if len(proof.AcceptedSelections) != 0 || len(proof.SelectedOccurrenceTrees) != 0 || len(proof.SelectedOccurrences) != 0 {
				t.Fatalf("corrupt route published selected proof: %+v", proof)
			}
		})
	}
}

func TestPhase0ASelectedOccurrenceCapIsAtomic(t *testing.T) {
	limits := phase0ARouteLimits()
	limits.MaxSelectedOccurrences = 1
	core, run, head := phase0ASelectedSingleReduction(t, limits)
	capability := phase0ACaptureSelection(t, core, head)
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
	phase0AObservers.Unlock()
	if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
		t.Fatalf("selected occurrence cap error=%v", err)
	}
	phase0AObservers.Lock()
	if len(observer.route.acceptedSelections) != 0 || len(observer.route.acceptedLinks) != 0 || len(observer.route.selectedTrees) != 0 || len(observer.route.selectedOccurrences) != 0 ||
		phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes {
		t.Errorf("selected occurrence cap partially published rows or accounting")
	}
	phase0AObservers.Unlock()
	_ = run
}

func TestPhase0ASelectedOccurrencesIncludeLinklessZeroChildReduction(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 2, symbol: 1}: {{Type: ActionReduce, Symbol: 20}}},
		gotos:   map[tableCell]StateID{{state: 2, symbol: 20}: 3},
	}
	core, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	seed, _ := core.Seed(1, 0)
	extra, _ := core.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, extra: true, terminal: true}, nil, nil, nil)
	head := phase0AObservedTerminalPrivate(t, core, extra, seed.Node, 2, 1, true, 0, ForkOrder{})
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("zero-child reduction=%v err=%v", frontier, err)
	}
	capability := phase0ACaptureSelection(t, core, frontier[0])
	if err := core.ObservePhase0AAcceptedSelection(capability); err != nil {
		t.Fatal(err)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil || len(proof.SelectedOccurrenceTrees) != 1 || proof.SelectedOccurrenceTrees[0].OccurrenceCount != 2 || proof.SelectedOccurrenceTrees[0].ReductionCount != 1 || len(proof.SelectedOccurrences) != 2 || proof.SelectedOccurrences[1].ChildCount != 0 {
		t.Fatalf("zero-child selected proof=%+v rows=%+v err=%v", proof.SelectedOccurrenceTrees, proof.SelectedOccurrences, err)
	}
}

func TestPhase0ASelectedOccurrencesIgnoreRolledBackRouteAndUseLiveRetry(t *testing.T) {
	tables := &fakeTable{
		actions: map[tableCell][]Action{{state: 2, symbol: 1}: {{Type: ActionReduce, Symbol: 20, ChildCount: 1}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 20}: 3},
	}
	core, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	seed, _ := core.Seed(1, 0)
	leaf, _ := core.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
	head := phase0AObservedTerminalPrivate(t, core, leaf, seed.Node, 2, 1, false, 0, ForkOrder{})
	core.limits.MaxNodes = uint32(core.nodes.Len())
	if _, err := core.Reduce(head, 1, 0, ForkOrder{}); err == nil {
		t.Fatal("node-capped reduction succeeded")
	}
	core.limits.MaxNodes = 4096
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) == 0 {
		t.Fatalf("live retry=%v err=%v", frontier, err)
	}
	capability := phase0ACaptureSelection(t, core, frontier[0])
	if err := core.ObservePhase0AAcceptedSelection(capability); err != nil {
		t.Fatal(err)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil || len(proof.SelectedOccurrenceTrees) != 1 || proof.SelectedOccurrenceTrees[0].OccurrenceCount == 0 {
		t.Fatalf("live retry selected proof=%+v err=%v", proof.SelectedOccurrenceTrees, err)
	}
	for _, row := range proof.SelectedOccurrences {
		mutation := proof.Mutations[0]
		for _, candidate := range proof.Mutations {
			if candidate.Occurrence == row.Occurrence && candidate.Kind == Phase0AMutationOccurrence {
				mutation = candidate
				break
			}
		}
		if mutation.RolledBack {
			t.Fatalf("selected occurrence used rolled-back row: %+v", row)
		}
	}
}

func TestPhase0ASelectedOccurrenceDigestRejectsDuplicateSiblingIdentity(t *testing.T) {
	core := newTinyCore(t, 8)
	leaf, _ := core.appendSubtree(subtreeRecord{symbol: 10, terminal: true}, nil, nil, nil)
	parent, _ := core.appendSubtree(subtreeRecord{symbol: 20}, []SubtreeID{leaf, leaf}, nil, nil)
	run := CoreRunNamespace{CoreInstance: 1, RunGeneration: 1}
	event := ConstructionEventKey{Attempt: AttemptKey{Run: run, AttemptEpoch: 1}, Serial: 1}
	parentOccurrence := ConstructionOccurrenceKey{Event: event, Slot: 1}
	parentEdge := IncomingEdgeKey{Event: event, Serial: 1}
	childOccurrence := ConstructionOccurrenceKey{Event: event, Slot: 2}
	childEdge := IncomingEdgeKey{Event: event, Serial: 2}
	records := []Phase0ASelectedOccurrenceRecord{
		{Ordinal: 1, Occurrence: parentOccurrence, Edge: parentEdge, Payload: parent, ConstructionKind: Phase0AConstructionReductionParent, ChildCount: 2},
		{Ordinal: 2, Parent: 1, Occurrence: childOccurrence, Edge: childEdge, Payload: leaf, ConstructionKind: Phase0AConstructionOrdinaryTerminal, Depth: 1},
		{Ordinal: 3, Parent: 1, ChildOrdinal: 1, Occurrence: childOccurrence, Edge: childEdge, Payload: leaf, ConstructionKind: Phase0AConstructionOrdinaryTerminal, Depth: 1},
	}
	if _, err := phase0AValidateAndDigestSelectedOccurrences(core, run, records, 1); !isPhase0AError(err, Phase0AErrorAmbiguousReference, "") {
		t.Fatalf("duplicate sibling identity error=%v", err)
	}
}

func TestPhase0ASelectedOccurrenceEmptySnapshotHasIndependentDigest(t *testing.T) {
	core := newTinyCore(t, 8)
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	index := phase0ABuildSelectedIndex(observer)
	tree, records, err := phase0ABuildSelectedOccurrenceSnapshotLocked(core, observer, index, 1, nil)
	phase0AObservers.Unlock()
	if err != nil || tree.Namespace != run || tree.RootCount != 0 || tree.OccurrenceCount != 0 || len(records) != 0 {
		t.Fatalf("empty selected tree=%+v records=%v err=%v", tree, records, err)
	}
	raw, err := phase0ARawSelectedCompactDigest(core, nil)
	if err != nil || raw != tree.Digest {
		t.Fatalf("empty digest raw=%x selected=%x err=%v", raw, tree.Digest, err)
	}
}

func TestPhase0ASelectedOccurrenceDigestDepthBoundary(t *testing.T) {
	core, err := New(&fakeTable{}, Limits{MaxSubtrees: 5000, MaxChildren: 5000})
	if err != nil {
		t.Fatal(err)
	}
	root, err := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for depth := uint64(0); depth < phase0ASelectedHardMaxDepth; depth++ {
		root, err = core.appendSubtree(subtreeRecord{symbol: 2}, []SubtreeID{root}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	atLimit, err := phase0ARawSelectedCompactDigest(core, []SubtreeID{root})
	if err != nil {
		t.Fatalf("at-limit raw digest: %v", err)
	}
	records := make([]Phase0ASelectedOccurrenceRecord, phase0ASelectedHardMaxDepth+1)
	for index := range records {
		ordinal := uint64(index + 1)
		payload := SubtreeID(uint64(root) - uint64(index))
		event := ConstructionEventKey{Attempt: AttemptKey{Run: CoreRunNamespace{CoreInstance: 1, RunGeneration: 1}, AttemptEpoch: 1}, Serial: ordinal}
		records[index] = Phase0ASelectedOccurrenceRecord{
			Ordinal: ordinal, Parent: ordinal - 1, Depth: uint32(index), Occurrence: ConstructionOccurrenceKey{Event: event, Slot: 1}, Edge: IncomingEdgeKey{Event: event, Serial: ordinal},
			Payload: payload, ConstructionKind: Phase0AConstructionReductionParent, ChildCount: 1,
		}
	}
	records[0].Parent = 0
	records[len(records)-1].ConstructionKind = Phase0AConstructionOrdinaryTerminal
	records[len(records)-1].ChildCount = 0
	proofDigest, err := phase0AValidateAndDigestSelectedOccurrences(core, CoreRunNamespace{CoreInstance: 1, RunGeneration: 1}, records, 1)
	if err != nil || proofDigest != atLimit {
		t.Fatalf("at-limit proof=%x raw=%x err=%v", proofDigest, atLimit, err)
	}
	over, err := core.appendSubtree(subtreeRecord{symbol: 3}, []SubtreeID{root}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := phase0ARawSelectedCompactDigest(core, []SubtreeID{over}); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
		t.Fatalf("over-limit raw digest error=%v", err)
	}
	overRecords := make([]Phase0ASelectedOccurrenceRecord, len(records)+1)
	for index := range overRecords {
		ordinal := uint64(index + 1)
		event := ConstructionEventKey{Attempt: AttemptKey{Run: CoreRunNamespace{CoreInstance: 2, RunGeneration: 1}, AttemptEpoch: 1}, Serial: ordinal}
		overRecords[index] = Phase0ASelectedOccurrenceRecord{
			Ordinal: ordinal, Parent: ordinal - 1, Depth: uint32(index), Occurrence: ConstructionOccurrenceKey{Event: event, Slot: 1}, Edge: IncomingEdgeKey{Event: event, Serial: ordinal},
			Payload: SubtreeID(uint64(over) - uint64(index)), ConstructionKind: Phase0AConstructionReductionParent, ChildCount: 1,
		}
	}
	overRecords[0].Parent = 0
	overRecords[len(overRecords)-1].ChildCount = 0
	overRecords[len(overRecords)-1].ConstructionKind = Phase0AConstructionOrdinaryTerminal
	if _, err := phase0AValidateAndDigestSelectedOccurrences(core, CoreRunNamespace{CoreInstance: 2, RunGeneration: 1}, overRecords, 1); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
		t.Fatalf("over-limit proof digest error=%v", err)
	}
}

func TestPhase0ASelectedCompactWindowsFailClosedWithoutPanic(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Core, SubtreeID)
	}{
		{name: "child-first", mutate: func(core *Core, id SubtreeID) {
			(*core.subtrees.At(int(id - 1))).firstChild = uint32(len(core.children))
		}},
		{name: "child-count", mutate: func(core *Core, id SubtreeID) { (*core.subtrees.At(int(id - 1))).childCount = ^uint32(0) }},
		{name: "field-first", mutate: func(core *Core, id SubtreeID) { (*core.subtrees.At(int(id - 1))).firstField = uint32(len(core.fields)) }},
		{name: "field-count", mutate: func(core *Core, id SubtreeID) { (*core.subtrees.At(int(id - 1))).fieldCount = ^uint32(0) }},
		{name: "alias-first", mutate: func(core *Core, id SubtreeID) {
			(*core.subtrees.At(int(id - 1))).firstAlias = uint32(len(core.aliases))
		}},
		{name: "alias-count", mutate: func(core *Core, id SubtreeID) { (*core.subtrees.At(int(id - 1))).aliasCount = ^uint32(0) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core := newTinyCore(t, 8)
			leaf, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
			parent, _ := core.appendSubtree(subtreeRecord{symbol: 2}, []SubtreeID{leaf}, []FieldMapEntry{{FieldID: 1}}, []Symbol{3})
			test.mutate(core, parent)
			if _, err := phase0ARawSelectedCompactDigest(core, []SubtreeID{parent}); !isPhase0AError(err, Phase0AErrorStaleReference, "") {
				t.Fatalf("compact window error=%v", err)
			}
		})
	}
}

func TestPhase0ASelectedDigestRejectsForwardCompactChild(t *testing.T) {
	core := newTinyCore(t, 8)
	leaf, _ := core.appendSubtree(subtreeRecord{symbol: 1, terminal: true}, nil, nil, nil)
	parent, _ := core.appendSubtree(subtreeRecord{symbol: 2}, []SubtreeID{leaf}, nil, nil)
	core.children[(*core.subtrees.At(int(parent - 1))).firstChild] = parent
	run := CoreRunNamespace{CoreInstance: 1, RunGeneration: 1}
	event := ConstructionEventKey{Attempt: AttemptKey{Run: run, AttemptEpoch: 1}, Serial: 1}
	records := []Phase0ASelectedOccurrenceRecord{
		{Ordinal: 1, Occurrence: ConstructionOccurrenceKey{Event: event, Slot: 1}, Edge: IncomingEdgeKey{Event: event, Serial: 1}, Payload: parent, ConstructionKind: Phase0AConstructionReductionParent, ChildCount: 1},
		{Ordinal: 2, Parent: 1, Depth: 1, Occurrence: ConstructionOccurrenceKey{Event: event, Slot: 2}, Edge: IncomingEdgeKey{Event: event, Serial: 2}, Payload: parent, ConstructionKind: Phase0AConstructionOrdinaryTerminal},
	}
	if _, err := phase0AValidateAndDigestSelectedOccurrences(core, run, records, 1); !isPhase0AError(err, Phase0AErrorCyclicReference, "") {
		t.Fatalf("forward selected child error=%v", err)
	}
	if _, err := phase0ARawSelectedCompactDigest(core, []SubtreeID{parent}); !isPhase0AError(err, Phase0AErrorCyclicReference, "") {
		t.Fatalf("forward raw child error=%v", err)
	}
}

func TestPhase0ASelectedIndexPreflightIsAtomic(t *testing.T) {
	core, _, head := phase0ASelectedSingleReduction(t, phase0ARouteLimits())
	capability := phase0ACaptureSelection(t, core, head)
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	for index := 0; index < 64; index++ {
		observer.route.popRoutes = append(observer.route.popRoutes, Phase0APopRouteRecord{RolledBack: true})
	}
	beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
	phase0AObservers.limits.MaxSelectedIndexBytes = 1
	phase0AObservers.Unlock()
	if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorByteCap, "") {
		t.Fatalf("selected transient preflight error=%v", err)
	}
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	if phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes || len(observer.route.acceptedSelections) != 0 || len(observer.route.selectedTrees) != 0 || len(observer.route.selectedOccurrences) != 0 {
		t.Fatalf("selected transient preflight moved publication/accounting")
	}
}

func TestPhase0ASelectedTransientReservationCoversCombinedPublicationPeak(t *testing.T) {
	core, _, head := phase0ASelectedSingleReduction(t, phase0ARouteLimits())
	capability := phase0ACaptureSelection(t, core, head)
	derivations, err := core.Derivations(head)
	if err != nil || len(derivations) != 1 {
		t.Fatalf("selected derivation=%v err=%v", derivations, err)
	}
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	selectedCount, selectedDepth, err := phase0ACountSelectedCompactOccurrencesLocked(core, observer, derivations[0].Payloads)
	if err != nil {
		phase0AObservers.Unlock()
		t.Fatal(err)
	}
	transientBytes, err := phase0APreflightSelectedIndexLocked(core, observer, 1, selectedCount, selectedDepth)
	if err != nil {
		phase0AObservers.Unlock()
		t.Fatal(err)
	}
	persistentBytes := phase0AAcceptedSelectionBytes + phase0ASelectedTreeBytes + phase0AAcceptedLinkBytes + selectedCount*phase0ASelectedOccurrenceBytes
	beforeRecords, beforeBytes := phase0AObservers.records, phase0AObservers.bytes
	if transientBytes <= persistentBytes {
		phase0AObservers.Unlock()
		t.Fatalf("focused transient reservation=%d persistent=%d", transientBytes, persistentBytes)
	}
	phase0AObservers.limits.MaxBytes = beforeBytes + transientBytes + persistentBytes - 1
	phase0AObservers.Unlock()
	t.Logf("focused selected scratch reservation=%d bytes selected=%d depth=%d persistent=%d", transientBytes, selectedCount, selectedDepth, persistentBytes)
	if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorByteCap, "") {
		t.Fatalf("combined selected peak error=%v", err)
	}
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	if phase0AObservers.records != beforeRecords || phase0AObservers.bytes != beforeBytes || len(observer.route.acceptedSelections) != 0 || len(observer.route.selectedTrees) != 0 || len(observer.route.selectedOccurrences) != 0 {
		t.Fatalf("combined selected peak moved publication/accounting")
	}
}

func TestPhase0ASelectedMigrationFinalRevalidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Phase0ATrailingExtraMigrationRecord)
	}{
		{name: "route", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.Route++ }},
		{name: "trailing-ordinal", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.TrailingOrdinal++ }},
		{name: "source-link", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.SourceLink++ }},
		{name: "source-lower", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.SourceLowerLink++ }},
		{name: "source-expression", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.SourceExpression++ }},
		{name: "source-occurrence", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.SourceOccurrence.Slot++ }},
		{name: "source-edge", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.SourceEdge.Serial++ }},
		{name: "target-link", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.TargetLink++ }},
		{name: "target-occurrence", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.TargetOccurrence.Slot++ }},
		{name: "target-edge", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.TargetEdge.Serial++ }},
		{name: "boundary", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.Boundary.ByteOffset++ }},
		{name: "payload", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.Payload++ }},
		{name: "predecessor", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.Predecessor++ }},
		{name: "rolled", mutate: func(row *Phase0ATrailingExtraMigrationRecord) { row.RolledBack = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, _, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
			frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
			if err != nil || len(frontier) != 1 {
				t.Fatalf("migration reduction=%v err=%v", frontier, err)
			}
			capability := phase0ACaptureSelection(t, core, frontier[0])
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			if len(observer.route.migrations) != 1 {
				phase0AObservers.Unlock()
				t.Fatalf("migration rows=%d", len(observer.route.migrations))
			}
			test.mutate(&observer.route.migrations[0])
			phase0AObservers.Unlock()
			err = core.ObservePhase0AAcceptedSelection(capability)
			var typed *Phase0AError
			if !errors.As(err, &typed) {
				t.Fatalf("migration corruption error=%v", err)
			}
			phase0AObservers.Lock()
			published := len(observer.route.acceptedSelections) + len(observer.route.selectedTrees) + len(observer.route.selectedOccurrences)
			phase0AObservers.Unlock()
			if published != 0 {
				t.Fatalf("migration corruption published %d rows", published)
			}
		})
	}
}

func TestPhase0ASelectedMigrationRejectsDuplicateRouteOrdinalAcrossJoins(t *testing.T) {
	core, _, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
	if frontier, err := core.Reduce(head, 1, 0, ForkOrder{}); err != nil || len(frontier) != 1 {
		t.Fatalf("migration reduction=%v err=%v", frontier, err)
	}
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	duplicate := observer.route.migrations[0]
	duplicate.Occurrence.Slot++
	duplicate.Edge.Serial++
	observer.route.migrations = append(observer.route.migrations, duplicate)
	index := phase0ABuildSelectedIndex(observer)
	if err := phase0AValidateSelectedMigrationLocked(core, observer, index, observer.route.migrations[0]); !isPhase0AError(err, Phase0AErrorAmbiguousReference, "") {
		t.Fatalf("duplicate route ordinal error=%v", err)
	}
}

func TestPhase0ASelectedMigrationRevalidatesTargetPhysicalCandidateNodeAndKinds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Core, *phase0AObserver, Phase0ATrailingExtraMigrationRecord)
	}{
		{name: "physical-prev", mutate: func(core *Core, _ *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			{
				linkStorageMutation := core.links.At(int(row.TargetLink - 1))
				linkStorageMutation.prev++
				core.links.Set(int(row.TargetLink-1), linkStorageMutation)
			}
		}},
		{name: "physical-payload", mutate: func(core *Core, _ *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			{
				linkStorageMutation := core.links.At(int(row.TargetLink - 1))
				linkStorageMutation.payload++
				core.links.Set(int(row.TargetLink-1), linkStorageMutation)
			}
		}},
		{name: "physical-score", mutate: func(core *Core, _ *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			{
				linkStorageMutation := core.links.At(int(row.TargetLink - 1))
				linkStorageMutation.scoreDelta++
				core.links.Set(int(row.TargetLink-1), linkStorageMutation)
			}
		}},
		{name: "physical-order", mutate: func(core *Core, _ *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			{
				linkStorageMutation := core.links.At(int(row.TargetLink - 1))
				linkStorageMutation.order++
				core.links.Set(int(row.TargetLink-1), linkStorageMutation)
			}
		}},
		{name: "physical-has-order", mutate: func(core *Core, _ *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			{
				linkStorageMutation := core.links.At(int(row.TargetLink - 1))
				linkStorageMutation.flags ^= linkFlagHasOrder
				core.links.Set(int(row.TargetLink-1), linkStorageMutation)
			}
		}},
		{name: "target-node-state", mutate: func(core *Core, _ *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			(*core.nodes.At(int(row.Predecessor - 1))).state++
		}},
		{name: "target-node-byte", mutate: func(core *Core, _ *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			(*core.nodes.At(int(row.Predecessor - 1))).byteOffset++
		}},
		{name: "candidate-boundary", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.Boundary.ByteOffset++ })
		}},
		{name: "candidate-transaction", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.TransactionID++ })
		}},
		{name: "candidate-frontier", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.Boundary.Frontier++ })
		}},
		{name: "candidate-checkpoint", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.Boundary.Checkpoint++ })
		}},
		{name: "candidate-shifted", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.Boundary.Shifted = true })
		}},
		{name: "candidate-payload", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.Payload++ })
		}},
		{name: "candidate-predecessor", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.Predecessor++ })
		}},
		{name: "candidate-score", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.ScoreDelta++ })
		}},
		{name: "candidate-order", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.Order++ })
		}},
		{name: "candidate-has-order", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedCandidate(observer, row.TargetOccurrence, row.TargetEdge, func(candidate *Phase0ACandidateRecord) { candidate.HasOrder = !candidate.HasOrder })
		}},
		{name: "source-kind", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedConstructionKind(observer, row.SourceOccurrence, row.SourceEdge, Phase0AConstructionOrdinaryTerminal)
		}},
		{name: "target-kind", mutate: func(_ *Core, observer *phase0AObserver, row Phase0ATrailingExtraMigrationRecord) {
			phase0AMutateSelectedConstructionKind(observer, row.TargetOccurrence, row.TargetEdge, Phase0AConstructionOrdinaryTerminal)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, _, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
			if frontier, err := core.Reduce(head, 1, 0, ForkOrder{}); err != nil || len(frontier) != 1 {
				t.Fatalf("migration reduction=%v err=%v", frontier, err)
			}
			phase0AObservers.Lock()
			defer phase0AObservers.Unlock()
			observer := phase0AObservers.byCore[core]
			migration := observer.route.migrations[0]
			test.mutate(core, observer, migration)
			before := phase0AMigrationAccountingLocked(observer)
			beforePublished := len(observer.route.acceptedSelections) + len(observer.route.acceptedLinks) + len(observer.route.selectedTrees) + len(observer.route.selectedOccurrences)
			index := phase0ABuildSelectedIndex(observer)
			err := phase0AValidateSelectedMigrationLocked(core, observer, index, observer.route.migrations[0])
			var typed *Phase0AError
			if !errors.As(err, &typed) {
				t.Fatalf("target corruption error=%v", err)
			}
			after := phase0AMigrationAccountingLocked(observer)
			afterPublished := len(observer.route.acceptedSelections) + len(observer.route.acceptedLinks) + len(observer.route.selectedTrees) + len(observer.route.selectedOccurrences)
			if after != before || afterPublished != beforePublished {
				t.Fatalf("target corruption moved accounting/publication before=%+v/%d after=%+v/%d", before, beforePublished, after, afterPublished)
			}
		})
	}
}

func phase0AMutateSelectedCandidate(observer *phase0AObserver, occurrence ConstructionOccurrenceKey, edge IncomingEdgeKey, mutate func(*Phase0ACandidateRecord)) {
	for index := range observer.factor.candidates {
		candidate := &observer.factor.candidates[index]
		if !candidate.RolledBack && candidate.Occurrence == occurrence && candidate.Edge == edge {
			mutate(candidate)
			return
		}
	}
}

func phase0AMutateSelectedConstructionKind(observer *phase0AObserver, occurrence ConstructionOccurrenceKey, edge IncomingEdgeKey, kind Phase0AConstructionKind) {
	for index := range observer.mutations {
		row := &observer.mutations[index]
		if row.Event == occurrence.Event && (row.Kind == Phase0AMutationEvent || row.Occurrence == occurrence || row.Edge == edge) {
			row.ConstructionKind = kind
		}
	}
}

func TestPhase0ASelectedMigrationRejectsTranslatedLowerOutsideSourcePredecessor(t *testing.T) {
	core, _, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("migration reduction=%v err=%v", frontier, err)
	}
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	migration := observer.route.migrations[0]
	route := observer.route.popRoutes[migration.Route-1]
	source := observer.route.popLinks[route.FirstLink+uint64(route.RetainedLinkCount)+uint64(migration.TrailingOrdinal)]
	lower := observer.route.popLinks[source.Ordinal-1]
	bound, err := phase0AExactBindingLocked(observer, source.Link)
	if err != nil {
		t.Fatal(err)
	}
	observer.factor.nextSelector++
	selector := Phase0ASelectorID(observer.factor.nextSelector)
	observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: selector, TransactionID: migration.TransactionID, MergedPredecessor: lower.Node, FirstRoute: uint64(len(observer.factor.selectorRoutes)), RouteCount: 1})
	observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: selector, TransactionID: migration.TransactionID, SelectedLowerLink: lower.Link, SourceLowerLink: migration.TargetLink, Branch: Phase0ASelectorLeft})
	observer.factor.nextExpression++
	factor := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: factor, Kind: Phase0AExpressionFactorChoice, TransactionID: migration.TransactionID, Selector: selector, Left: bound, Right: bound})
	for index := range observer.factor.bindings {
		if observer.factor.bindings[index].Link == source.Link && !observer.factor.bindings[index].RolledBack {
			observer.factor.bindings[index].Expression = factor
		}
	}
	index := phase0ABuildSelectedIndex(observer)
	if err := phase0AValidateSelectedMigrationLocked(core, observer, index, migration); !isPhase0AError(err, Phase0AErrorCrossBoundary, "") {
		t.Fatalf("translated lower outside predecessor error=%v", err)
	}
}

func TestPhase0ASelectedSelectorRevalidatesMergedPredecessorAndRouteWindow(t *testing.T) {
	core := newTinyCore(t, 8)
	_ = phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 10, terminal: true}, nil, nil, nil)
	head := phase0AObservedTerminalPrivate(t, core, payload, seed.Node, 2, 0, false, 0, ForkOrder{})
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	link := LinkID((*core.nodes.At(int(head.Node - 1))).firstLink)
	bound, err := phase0AExactBindingLocked(observer, link)
	if err != nil {
		t.Fatal(err)
	}
	observer.factor.nextSelector++
	selector := Phase0ASelectorID(observer.factor.nextSelector)
	firstRoute := uint64(len(observer.factor.selectorRoutes))
	observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: selector, MergedPredecessor: head.Node, FirstRoute: firstRoute, RouteCount: 1})
	observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: selector, SelectedLowerLink: link, SourceLowerLink: link, Branch: Phase0ASelectorLeft})
	observer.factor.nextExpression++
	factor := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: factor, Kind: Phase0AExpressionFactorChoice, Selector: selector, Left: bound, Right: bound})
	index := phase0ABuildSelectedIndex(observer)
	if _, lower, err := phase0AResolveSelectedExpressionLocked(core, observer, index, factor, link); err != nil || lower != link {
		t.Fatalf("valid selected factor lower=%d err=%v", lower, err)
	}
	observer.factor.selectors[len(observer.factor.selectors)-1].MergedPredecessor = seed.Node
	index = phase0ABuildSelectedIndex(observer)
	if _, _, err := phase0AResolveSelectedExpressionLocked(core, observer, index, factor, link); !isPhase0AError(err, Phase0AErrorCrossBoundary, "") {
		t.Fatalf("merged predecessor corruption error=%v", err)
	}
	observer.factor.selectors[len(observer.factor.selectors)-1].MergedPredecessor = head.Node
	observer.factor.selectors[len(observer.factor.selectors)-1].FirstRoute = firstRoute + 1
	index = phase0ABuildSelectedIndex(observer)
	if _, _, err := phase0AResolveSelectedExpressionLocked(core, observer, index, factor, link); !isPhase0AError(err, Phase0AErrorStaleReference, "") {
		t.Fatalf("selector route-window corruption error=%v", err)
	}
}

func TestPhase0ASelectedSelectorResolutionDoesNotScanLargeWindow(t *testing.T) {
	core := newTinyCore(t, 8)
	_ = phase0ABeginShiftProvenanceRun(t, core, phase0ARouteLimits())
	seed, _ := core.Seed(1, 0)
	payload, _ := core.appendSubtree(subtreeRecord{symbol: 10, terminal: true}, nil, nil, nil)
	head := phase0AObservedTerminalPrivate(t, core, payload, seed.Node, 2, 0, false, 0, ForkOrder{})
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	link := LinkID((*core.nodes.At(int(head.Node - 1))).firstLink)
	bound, err := phase0AExactBindingLocked(observer, link)
	if err != nil {
		t.Fatal(err)
	}
	const unrelated = 4096
	for index := 0; index < unrelated; index++ {
		observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: Phase0ASelectorID(index + 100), SelectedLowerLink: LinkID(index + 100), SourceLowerLink: link, Branch: Phase0ASelectorLeft})
	}
	observer.factor.nextSelector++
	selector := Phase0ASelectorID(observer.factor.nextSelector)
	observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: selector, MergedPredecessor: head.Node, FirstRoute: 0, RouteCount: unrelated + 1})
	observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: selector, SelectedLowerLink: link, SourceLowerLink: link, Branch: Phase0ASelectorLeft})
	observer.factor.nextExpression++
	factor := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: factor, Kind: Phase0AExpressionFactorChoice, Selector: selector, Left: bound, Right: bound})
	index := phase0ABuildSelectedIndex(observer)
	observer.factor.selectorRoutes = nil // indexed ordinal/window evidence must be sufficient.
	for resolution := 0; resolution < unrelated; resolution++ {
		if direct, lower, err := phase0AResolveSelectedExpressionLocked(core, observer, index, factor, link); err != nil || direct.ID != bound || lower != link {
			t.Fatalf("large-window resolution=%d direct=%+v lower=%d err=%v", resolution, direct, lower, err)
		}
	}
}

func TestPhase0ASelectedConstructionKindMatchesCompactFlags(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Core, *phase0AObserver)
	}{
		{name: "ordinary-as-extra", mutate: func(core *Core, observer *phase0AObserver) {
			for _, row := range observer.mutations {
				if row.Kind == Phase0AMutationOccurrence && row.ConstructionKind == Phase0AConstructionOrdinaryTerminal {
					(*core.subtrees.At(int(row.Payload - 1))).extra = true
					return
				}
			}
		}},
		{name: "reduction-as-terminal", mutate: func(core *Core, observer *phase0AObserver) {
			for _, row := range observer.mutations {
				if row.Kind == Phase0AMutationOccurrence && row.ConstructionKind == Phase0AConstructionReductionParent {
					(*core.subtrees.At(int(row.Payload - 1))).terminal = true
					return
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, _, head := phase0ASelectedSingleReduction(t, phase0ARouteLimits())
			capability := phase0ACaptureSelection(t, core, head)
			phase0AObservers.Lock()
			observer := phase0AObservers.byCore[core]
			test.mutate(core, observer)
			phase0AObservers.Unlock()
			if err := core.ObservePhase0AAcceptedSelection(capability); !isPhase0AError(err, Phase0AErrorCrossBoundary, "") {
				t.Fatalf("construction-kind mismatch error=%v", err)
			}
		})
	}
}

func TestPhase0ASelectedBuilderDepthBoundary(t *testing.T) {
	limits := phase0ARouteLimits()
	limits.MaxSelectedDepth = 1
	t.Run("at-limit", func(t *testing.T) {
		core, _, head := phase0ASelectedSingleReduction(t, limits)
		if err := core.ObservePhase0AAcceptedSelection(phase0ACaptureSelection(t, core, head)); err != nil {
			t.Fatalf("builder at-limit depth: %v", err)
		}
	})
	t.Run("over-limit", func(t *testing.T) {
		tables := &fakeTable{
			actions: map[tableCell][]Action{
				{state: 2, symbol: 1}: {{Type: ActionReduce, Symbol: 20, ChildCount: 1}},
				{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 21, ChildCount: 1}},
			},
			gotos: map[tableCell]StateID{{state: 1, symbol: 20}: 3, {state: 1, symbol: 21}: 4},
		}
		core, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
		if err != nil {
			t.Fatal(err)
		}
		_ = phase0ABeginShiftProvenanceRun(t, core, limits)
		seed, _ := core.Seed(1, 0)
		leaf, _ := core.appendSubtree(subtreeRecord{symbol: 10, terminal: true}, nil, nil, nil)
		terminal := phase0AObservedTerminalPrivate(t, core, leaf, seed.Node, 2, 0, false, 0, ForkOrder{})
		first, err := core.Reduce(terminal, 1, 0, ForkOrder{})
		if err != nil || len(first) != 1 {
			t.Fatalf("first nested reduction=%v err=%v", first, err)
		}
		second, err := core.Reduce(first[0], 1, 0, ForkOrder{})
		if err != nil || len(second) != 1 {
			t.Fatalf("second nested reduction=%v err=%v", second, err)
		}
		if err := core.ObservePhase0AAcceptedSelection(phase0ACaptureSelection(t, core, second[0])); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
			t.Fatalf("builder over-limit depth error=%v", err)
		}
	})
}

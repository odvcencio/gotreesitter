//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"errors"
	"math"
	"testing"
)

func phase0ATrailingExtraLimits() Phase0AObserverLimits {
	limits := phase0ARouteLimits()
	limits.MaxTrailingExtraMigrations = 256
	return limits
}

type phase0AMigrationAccounting struct {
	mutations       int
	candidates      int
	migrations      int
	occurrenceCount uint64
	occurrenceBytes uint64
	nextEdge        uint64
	records         uint64
	bytes           uint64
}

func phase0AMigrationAccountingLocked(observer *phase0AObserver) phase0AMigrationAccounting {
	return phase0AMigrationAccounting{
		mutations: len(observer.mutations), candidates: len(observer.factor.candidates), migrations: len(observer.route.migrations),
		occurrenceCount: observer.occurrenceCount, occurrenceBytes: observer.occurrenceBytes, nextEdge: observer.nextEdge,
		records: phase0AObservers.records, bytes: phase0AObservers.bytes,
	}
}

func phase0APrepareTrailingMigration(t *testing.T, core *Core, head Head) (popPath, Head) {
	t.Helper()
	paths, err := core.popPaths(head.Node, 1)
	if err != nil || len(paths) != 1 || len(paths[0].trailing) != 1 {
		t.Fatalf("prepare trailing migration pop paths=%v err=%v", paths, err)
	}
	path := paths[0]
	phase0ABeginReductionConstruction(core, 1)
	phase0AObservePopRoutes(core, head.Node, 1, paths)
	parent, err := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1}, []SubtreeID{path.children[0]}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parentInput := linkInput{prev: path.prev, payload: parent, scoreDelta: path.score, order: path.order}
	phase0AObserveReductionOccurrence(core, parentInput, core.boundaryKey(4, 1))
	private, err := core.appendPrivate(4, 1, parentInput)
	if err != nil {
		t.Fatal(err)
	}
	return path, private
}

func TestPhase0ATrailingExtraRejectsNonPrivateMigrationPredecessor(t *testing.T) {
	core, run, head, extra := phase0AObservedTrailingReductionFixture(t, false, false)
	forced := errors.New("force rollback after width adversary")
	err := core.ApplyAtomic(func() error {
		paths, popErr := core.popPaths(head.Node, 1)
		if popErr != nil || len(paths) != 1 || len(paths[0].trailing) != 1 {
			return errors.New("invalid width-adversary pop path")
		}
		phase0ABeginReductionConstruction(core, 1)
		phase0AObservePopRoutes(core, head.Node, 1, paths)
		parent, appendErr := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 1}, []SubtreeID{paths[0].children[0]}, nil, nil)
		if appendErr != nil {
			return appendErr
		}
		parentKey := core.boundaryKey(4, 1)
		parentInput := linkInput{prev: paths[0].prev, payload: parent, scoreDelta: paths[0].score, order: paths[0].order}
		phase0AObserveReductionOccurrence(core, parentInput, parentKey)
		private, appendErr := core.appendPrivate(4, 1, parentInput)
		if appendErr != nil {
			return appendErr
		}
		(*core.nodes.At(int(private.Node - 1))).linkCount = 2
		phase0AObserveTrailingExtraMigration(core, 0, core.boundaryKey(4, 2), linkInput{prev: private.Node, payload: extra, scoreDelta: paths[0].trailing[0].scoreDelta})
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("width adversary transaction err=%v", err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if !isPhase0AError(proofErr, Phase0AErrorInvalidOccurrence, "") || len(proof.TrailingExtraMigrations) != 0 {
		t.Fatalf("width adversary proof=%+v err=%v", proof, proofErr)
	}
}

func TestPhase0ATrailingExtraRevalidatesLowerPhysicalIdentityWithoutAccountingMovement(t *testing.T) {
	core, run, head, extra := phase0AObservedTrailingReductionFixture(t, false, false)
	forced := errors.New("force rollback after lower identity adversary")
	err := core.ApplyAtomic(func() error {
		path, private := phase0APrepareTrailingMigration(t, core, head)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		route := &observer.route.popRoutes[observer.reduction.routeStart]
		lowerIndex := route.FirstLink + uint64(route.RetainedLinkCount-1)
		observer.route.popLinks[lowerIndex].Payload++ // frozen row no longer matches physical LinkID identity
		before := phase0AMigrationAccountingLocked(observer)
		phase0AObservers.Unlock()

		phase0AObserveTrailingExtraMigration(core, 0, core.boundaryKey(4, 2), linkInput{prev: private.Node, payload: extra, scoreDelta: path.trailing[0].scoreDelta})

		phase0AObservers.Lock()
		after := phase0AMigrationAccountingLocked(observer)
		phase0AObservers.Unlock()
		if after != before {
			t.Fatalf("failed lower revalidation moved accounting before=%+v after=%+v", before, after)
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("lower identity adversary transaction err=%v", err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if !isPhase0AError(proofErr, Phase0AErrorStaleReference, "") || len(proof.TrailingExtraMigrations) != 0 {
		t.Fatalf("lower identity adversary proof=%+v err=%v", proof, proofErr)
	}
}

func TestPhase0ATrailingExtraRejectsWrongWidthOneTargetWithoutAccountingMovement(t *testing.T) {
	core, run, head, extra := phase0AObservedTrailingReductionFixture(t, false, false)
	forced := errors.New("force rollback after target identity adversary")
	err := core.ApplyAtomic(func() error {
		path, _ := phase0APrepareTrailingMigration(t, core, head)
		sourceLinks, ok := phase0AStableLinkIDs(core, head.Node)
		if !ok || len(sourceLinks) != 1 {
			return errors.New("target identity source link unavailable")
		}
		// This node has the right state, position, and width, but its sole link
		// is the old extra-terminal publication rather than the newly published
		// reduction parent expected for suffix ordinal zero.
		wrong, appendErr := core.appendNode(nodeInput{state: 4, byteOffset: 1, firstLink: uint32(sourceLinks[0]), linkCount: 1, pathCount: 1})
		if appendErr != nil {
			return appendErr
		}
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		before := phase0AMigrationAccountingLocked(observer)
		phase0AObservers.Unlock()

		phase0AObserveTrailingExtraMigration(core, 0, core.boundaryKey(4, 2), linkInput{prev: wrong, payload: extra, scoreDelta: path.trailing[0].scoreDelta})

		phase0AObservers.Lock()
		after := phase0AMigrationAccountingLocked(observer)
		phase0AObservers.Unlock()
		if after != before {
			t.Fatalf("failed target binding moved accounting before=%+v after=%+v", before, after)
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("target identity adversary transaction err=%v", err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if !isPhase0AError(proofErr, Phase0AErrorInvalidOccurrence, "") || len(proof.TrailingExtraMigrations) != 0 {
		t.Fatalf("target identity adversary proof=%+v err=%v", proof, proofErr)
	}
}

func phase0AObservedTrailingReductionFixture(t *testing.T, duplicateExtra bool, twoReductions bool) (*Core, CoreRunNamespace, Head, SubtreeID) {
	t.Helper()
	actions := map[tableCell][]Action{
		{state: 3, symbol: 1}: {{Type: ActionReduce, Symbol: 20, ChildCount: 1}},
	}
	gotos := map[tableCell]StateID{{state: 1, symbol: 20}: 4}
	if twoReductions {
		actions[tableCell{state: 4, symbol: 1}] = []Action{{Type: ActionReduce, Symbol: 21, ChildCount: 1}}
		gotos[tableCell{state: 1, symbol: 21}] = 5
	}
	core, err := New(&fakeTable{actions: actions, gotos: gotos}, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	run := phase0ABeginShiftProvenanceRun(t, core, phase0ATrailingExtraLimits())
	seed, _ := core.Seed(1, 0)
	ordinary, _ := core.appendSubtree(subtreeRecord{symbol: 10, endByte: 1, terminal: true}, nil, nil, nil)
	extra, _ := core.appendSubtree(subtreeRecord{symbol: 11, startByte: 1, endByte: 2, extra: true, terminal: true}, nil, nil, nil)
	head := phase0AObservedTerminalPrivate(t, core, ordinary, seed.Node, 3, 1, false, 1, ForkOrder{Present: true, Value: 7})
	head = phase0AObservedTerminalPrivate(t, core, extra, head.Node, 3, 2, true, 2, ForkOrder{Present: true, Value: 9})
	if duplicateExtra {
		head = phase0AObservedTerminalPrivate(t, core, extra, head.Node, 3, 2, true, 2, ForkOrder{Present: true, Value: 10})
	}
	return core, run, head, extra
}

func TestPhase0ATrailingExtraRepeatedMigrationKeepsOneEventAndFreshSlots(t *testing.T) {
	core, run, head, extra := phase0AObservedTrailingReductionFixture(t, false, true)
	first, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(first) != 1 {
		t.Fatalf("first reduction=%v err=%v", first, err)
	}
	second, err := core.Reduce(first[0], 1, 0, ForkOrder{})
	if err != nil || len(second) != 1 {
		t.Fatalf("second reduction=%v err=%v", second, err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if proofErr != nil || len(proof.TrailingExtraMigrations) != 2 {
		t.Fatalf("repeated migration proof=%+v err=%v", proof, proofErr)
	}
	firstMigration, secondMigration := proof.TrailingExtraMigrations[0], proof.TrailingExtraMigrations[1]
	if firstMigration.Payload != extra || secondMigration.Payload != extra ||
		firstMigration.SourceOccurrence.Event != firstMigration.Occurrence.Event ||
		secondMigration.SourceOccurrence != firstMigration.Occurrence ||
		firstMigration.TargetOccurrence != proof.PopRoutes[0].Occurrence || firstMigration.TargetEdge != proof.PopRoutes[0].Edge ||
		secondMigration.TargetOccurrence != proof.PopRoutes[1].Occurrence || secondMigration.TargetEdge != proof.PopRoutes[1].Edge ||
		secondMigration.Occurrence.Event != firstMigration.Occurrence.Event ||
		firstMigration.Occurrence.Slot+1 != secondMigration.Occurrence.Slot ||
		firstMigration.Edge.Serial >= secondMigration.Edge.Serial ||
		firstMigration.SourceLink == secondMigration.SourceLink {
		t.Fatalf("repeated migrations first=%+v second=%+v", firstMigration, secondMigration)
	}
	if firstMigration.ScoreDelta != 2 || firstMigration.HasOrder || secondMigration.ScoreDelta != 2 || secondMigration.HasOrder {
		t.Fatalf("migration score/order changed first=%+v second=%+v", firstMigration, secondMigration)
	}
	parentCandidate, migrationCandidate := false, false
	for _, candidate := range proof.Candidates {
		if candidate.Edge == firstMigration.Edge {
			migrationCandidate = candidate.ScoreDelta == 2 && !candidate.HasOrder
		}
		if candidate.Occurrence == proof.PopRoutes[0].Occurrence {
			parentCandidate = candidate.ScoreDelta == 1 && candidate.HasOrder && candidate.Order == 9
		}
	}
	if !parentCandidate || !migrationCandidate {
		t.Fatalf("exact score/order candidates parent=%t migration=%t rows=%+v", parentCandidate, migrationCandidate, proof.Candidates)
	}
	capability := phase0ACaptureSelection(t, core, second[0])
	if err := core.ObservePhase0AAcceptedSelection(capability); err != nil {
		t.Fatal(err)
	}
	proof, proofErr = Phase0AObserverProof(core, run)
	if proofErr != nil || len(proof.SelectedOccurrenceTrees) != 1 || proof.SelectedOccurrenceTrees[0].OccurrenceCount != 4 || proof.SelectedOccurrenceTrees[0].MigratedCount != 1 {
		t.Fatalf("nested migrated selected occurrence proof=%+v err=%v", proof.SelectedOccurrenceTrees, proofErr)
	}
}

func TestPhase0ATrailingExtraDuplicatePayloadUsesDistinctPhysicalSources(t *testing.T) {
	core, run, head, extra := phase0AObservedTrailingReductionFixture(t, true, false)
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("duplicate-payload reduction=%v err=%v", frontier, err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if proofErr != nil || len(proof.TrailingExtraMigrations) != 2 {
		t.Fatalf("duplicate-payload proof=%+v err=%v", proof, proofErr)
	}
	left, right := proof.TrailingExtraMigrations[0], proof.TrailingExtraMigrations[1]
	if left.Payload != extra || right.Payload != extra || left.SourceLink == right.SourceLink || left.SourceLowerLink == right.SourceLowerLink || left.SourceOccurrence == right.SourceOccurrence || left.Occurrence == right.Occurrence ||
		left.TargetOccurrence != proof.PopRoutes[0].Occurrence || left.TargetEdge != proof.PopRoutes[0].Edge ||
		right.TargetOccurrence != left.Occurrence || right.TargetEdge != left.Edge || left.TargetLink == right.TargetLink ||
		left.TrailingOrdinal != 0 || right.TrailingOrdinal != 1 {
		t.Fatalf("duplicate payload sources left=%+v right=%+v", left, right)
	}
}

func TestPhase0ATrailingExtraRollbackRetainsTombstoneAndDoesNotReuseSlot(t *testing.T) {
	core, run, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
	core.limits.MaxNodes = uint32(core.nodes.Len() + 1)
	if _, err := core.Reduce(head, 1, 0, ForkOrder{}); err == nil {
		t.Fatal("node-capped trailing-extra reduction succeeded")
	}
	core.limits.MaxNodes = math.MaxUint32
	if _, err := core.Reduce(head, 1, 0, ForkOrder{}); err != nil {
		t.Fatalf("retry reduction: %v", err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if proofErr != nil || len(proof.TrailingExtraMigrations) != 2 {
		t.Fatalf("rollback proof=%+v err=%v", proof, proofErr)
	}
	rolled, live := proof.TrailingExtraMigrations[0], proof.TrailingExtraMigrations[1]
	if !rolled.RolledBack || rolled.RollbackCause != Phase0ARollbackReturnedError || live.RolledBack || rolled.SourceLink != live.SourceLink || rolled.Occurrence.Event != live.Occurrence.Event || live.Occurrence.Slot != rolled.Occurrence.Slot+1 || live.Edge.Serial <= rolled.Edge.Serial {
		t.Fatalf("rollback/live migrations rolled=%+v live=%+v", rolled, live)
	}
}

func TestPhase0ATrailingExtraMigrationResolvesNestedFactorChoice(t *testing.T) {
	core, run, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
	sourceLinks, ok := phase0AStableLinkIDs(core, head.Node)
	if !ok || len(sourceLinks) != 1 {
		t.Fatalf("source links=%v ok=%t", sourceLinks, ok)
	}
	sourceLink := sourceLinks[0]
	lowerNode := core.links.At(int(sourceLink - 1)).prev
	lowerLinks, ok := phase0AStableLinkIDs(core, lowerNode)
	if !ok || len(lowerLinks) != 1 {
		t.Fatalf("lower links=%v ok=%t", lowerLinks, ok)
	}
	lowerLink := lowerLinks[0]
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	direct, live := phase0ALiveExpressionLocked(observer, sourceLink)
	if !live {
		phase0AObservers.Unlock()
		t.Fatal("source direct expression unavailable")
	}
	transaction := observer.factor.bindings[len(observer.factor.bindings)-1].TransactionID
	observer.factor.nextSelector++
	innerSelector := Phase0ASelectorID(observer.factor.nextSelector)
	observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: innerSelector, TransactionID: transaction, MergedPredecessor: lowerNode, FirstRoute: uint64(len(observer.factor.selectorRoutes)), RouteCount: 1})
	observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: innerSelector, TransactionID: transaction, SelectedLowerLink: lowerLink, SourceLowerLink: lowerLink, Branch: Phase0ASelectorLeft})
	observer.factor.nextExpression++
	inner := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: inner, Kind: Phase0AExpressionFactorChoice, TransactionID: transaction, Selector: innerSelector, Left: direct, Right: direct})
	observer.factor.nextSelector++
	outerSelector := Phase0ASelectorID(observer.factor.nextSelector)
	observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: outerSelector, TransactionID: transaction, MergedPredecessor: lowerNode, FirstRoute: uint64(len(observer.factor.selectorRoutes)), RouteCount: 1})
	observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: outerSelector, TransactionID: transaction, SelectedLowerLink: lowerLink, SourceLowerLink: lowerLink, Branch: Phase0ASelectorLeft})
	observer.factor.nextExpression++
	outer := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: outer, Kind: Phase0AExpressionFactorChoice, TransactionID: transaction, Selector: outerSelector, Left: inner, Right: direct})
	for index := len(observer.factor.bindings) - 1; index >= 0; index-- {
		if observer.factor.bindings[index].Link == sourceLink && !observer.factor.bindings[index].RolledBack {
			observer.factor.bindings[index].Expression = outer
			break
		}
	}
	phase0AObservers.Unlock()

	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("nested-factor reduction=%v err=%v", frontier, err)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if proofErr != nil || len(proof.TrailingExtraMigrations) != 1 || proof.TrailingExtraMigrations[0].SourceExpression != direct {
		t.Fatalf("nested-factor migration proof=%+v err=%v direct=%d", proof, proofErr, direct)
	}
}

func TestPhase0ATrailingExtraCorruptSourceIdentityFailsClosedWithoutMigration(t *testing.T) {
	core, run, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
	sourceLinks, ok := phase0AStableLinkIDs(core, head.Node)
	if !ok || len(sourceLinks) != 1 {
		t.Fatalf("source links=%v ok=%t", sourceLinks, ok)
	}
	phase0AObservers.Lock()
	observer := phase0AObservers.byCore[core]
	for index := len(observer.factor.bindings) - 1; index >= 0; index-- {
		if observer.factor.bindings[index].Link == sourceLinks[0] && !observer.factor.bindings[index].RolledBack {
			observer.factor.bindings[index].Expression = Phase0AExpressionID(math.MaxUint64)
			break
		}
	}
	phase0AObservers.Unlock()
	frontier, parseErr := core.Reduce(head, 1, 0, ForkOrder{})
	if parseErr != nil || len(frontier) != 1 {
		t.Fatalf("corrupt observer changed parse=%v err=%v", frontier, parseErr)
	}
	proof, proofErr := Phase0AObserverProof(core, run)
	if !isPhase0AError(proofErr, Phase0AErrorMissingReference, "") || len(proof.TrailingExtraMigrations) != 0 {
		t.Fatalf("corrupt source proof=%+v err=%v", proof, proofErr)
	}
}

func TestPhase0ATrailingExtraMigrationCapAndSlotOverflowAreAtomic(t *testing.T) {
	t.Run("occurrence-cap", func(t *testing.T) {
		core, run, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		phase0AObservers.limits.MaxOccurrences = observer.occurrenceCount + 1 // parent fits; migration does not
		phase0AObservers.Unlock()
		frontier, parseErr := core.Reduce(head, 1, 0, ForkOrder{})
		if parseErr != nil || len(frontier) != 1 {
			t.Fatalf("cap changed parse=%v err=%v", frontier, parseErr)
		}
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorOccurrenceCap, "") || len(proof.TrailingExtraMigrations) != 0 || proof.OccurrenceCount != 3 {
			t.Fatalf("cap proof=%+v err=%v", proof, proofErr)
		}
		phase0AObservers.Lock()
		if observer.nextEdge != 3 {
			t.Errorf("cap advanced edge serial=%d want=3", observer.nextEdge)
		}
		phase0AObservers.Unlock()
	})

	t.Run("slot-overflow", func(t *testing.T) {
		core, run, head, _ := phase0AObservedTrailingReductionFixture(t, false, false)
		sourceLinks, _ := phase0AStableLinkIDs(core, head.Node)
		phase0AObservers.Lock()
		observer := phase0AObservers.byCore[core]
		expression, _ := phase0ALiveExpressionLocked(observer, sourceLinks[0])
		direct, _ := phase0AExactExpressionLocked(observer, expression)
		observer.nextOccurrenceSlot[direct.Occurrence.Event] = math.MaxUint32
		phase0AObservers.Unlock()
		frontier, parseErr := core.Reduce(head, 1, 0, ForkOrder{})
		if parseErr != nil || len(frontier) != 1 {
			t.Fatalf("overflow changed parse=%v err=%v", frontier, parseErr)
		}
		proof, proofErr := Phase0AObserverProof(core, run)
		if !isPhase0AError(proofErr, Phase0AErrorCounterOverflow, Phase0ACounterOccurrenceSlot) || len(proof.TrailingExtraMigrations) != 0 || proof.OccurrenceCount != 3 {
			t.Fatalf("overflow proof=%+v err=%v", proof, proofErr)
		}
		phase0AObservers.Lock()
		if observer.nextEdge != 3 || observer.nextOccurrenceSlot[direct.Occurrence.Event] != math.MaxUint32 {
			t.Errorf("overflow advanced counters edge=%d slot=%d", observer.nextEdge, observer.nextOccurrenceSlot[direct.Occurrence.Event])
		}
		phase0AObservers.Unlock()
	})
}

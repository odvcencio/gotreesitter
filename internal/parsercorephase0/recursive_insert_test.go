package parsercorephase0

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recursiveInsertFixture struct {
	core                       *Core
	rootA, rootB               Head
	leftPredecessor, rightPrev Head
	oldTop                     Head
	topPayload                 SubtreeID
	classAFirst, classBFirst   SubtreeID
	checkpoint                 CheckpointID
}

type certifiedExternalRecursiveInsertFixture struct {
	core                       *Core
	oldTop, rightPrev          Head
	lowerLeft, lowerRight, top SubtreeID
	key                        boundaryKey
}

func newRecursiveInsertFixture(t *testing.T) recursiveInsertFixture {
	return newRecursiveInsertFixtureWithCheckpoint(t, [32]byte{})
}

func newRecursiveInsertFixtureWithCheckpoint(t *testing.T, checkpoint [32]byte) recursiveInsertFixture {
	t.Helper()
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 32, MaxPopPaths: 32})
	rootA, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := core.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	payloads := make([]SubtreeID, 9)
	for ordinal := range payloads {
		classA := ordinal == 0 || ordinal == 1 || ordinal == 3 || ordinal == 7
		start := uint32(2)
		childSymbol := Symbol(40 + ordinal)
		if classA {
			start = 0
		}
		payloads[ordinal] = appendShallowPayload(t, core, shallowPayloadSpec{
			symbol: 20, productionID: uint16(ordinal + 1), startByte: start, endByte: 10,
			childSymbols: []Symbol{childSymbol},
		})
	}
	leftID, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootA.Node, payload: payloads[0]}})
	if err != nil {
		t.Fatal(err)
	}
	rightLinks := make([]linkRecord, 0, 8)
	for ordinal := 1; ordinal < 9; ordinal++ {
		prev := rootB.Node
		if ordinal == 1 || ordinal == 3 || ordinal == 7 {
			prev = rootA.Node
		}
		rightLinks = append(rightLinks, linkRecord{prev: prev, payload: payloads[ordinal]})
	}
	rightID, err := core.appendAdjacencyNode(7, 10, rightLinks)
	if err != nil {
		t.Fatal(err)
	}
	top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, startByte: 10, endByte: 11})
	checkpointID := mustInternCheckpoint(t, core, checkpoint[:])
	if err := core.SetPhaseCheckpoint(checkpointID); err != nil {
		t.Fatal(err)
	}
	oldTop, err := core.condense(core.boundaryKey(8, 11), linkInput{prev: leftID, payload: top})
	if err != nil {
		t.Fatal(err)
	}
	return recursiveInsertFixture{
		core: core, rootA: rootA, rootB: rootB,
		leftPredecessor: Head{Node: leftID}, rightPrev: Head{Node: rightID}, oldTop: oldTop,
		topPayload: top, classAFirst: payloads[0], classBFirst: payloads[2], checkpoint: checkpointID,
	}
}

func newCertifiedExternalRecursiveInsertFixture(t *testing.T, externalDescendant bool) certifiedExternalRecursiveInsertFixture {
	t.Helper()
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	core.CertifyExternalPayloadsQuiescent()
	rootA, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := core.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	lowerLeft := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 10})
	lowerRight := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 21, endByte: 10})
	left, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootA.Node, payload: lowerLeft}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootB.Node, payload: lowerRight}})
	if err != nil {
		t.Fatal(err)
	}
	var top SubtreeID
	if externalDescendant {
		externalChild := appendShallowPayload(t, core, shallowPayloadSpec{
			symbol: 31, startByte: 10, endByte: 11, external: true,
		})
		top, err = core.appendSubtree(
			subtreeRecord{symbol: 30, startByte: 10, endByte: 11},
			[]SubtreeID{externalChild}, nil, nil,
		)
	} else {
		top = appendShallowPayload(t, core, shallowPayloadSpec{
			symbol: 30, startByte: 10, endByte: 11, external: true,
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	key := core.boundaryKey(8, 11)
	oldTop, err := core.condense(key, linkInput{prev: left, payload: top})
	if err != nil {
		t.Fatal(err)
	}
	return certifiedExternalRecursiveInsertFixture{
		core: core, oldTop: oldTop, rightPrev: Head{Node: right},
		lowerLeft: lowerLeft, lowerRight: lowerRight, top: top, key: key,
	}
}

func newExactExternalRecursiveInsertFixture(t *testing.T, externalDescendant bool) certifiedExternalRecursiveInsertFixture {
	t.Helper()
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	start := mustInternCheckpoint(t, core, []byte{1, 2})
	end := mustInternCheckpoint(t, core, []byte{3, 4})
	if err := core.SetPhaseCheckpoint(end); err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, end); err != nil {
		t.Fatal(err)
	}
	rootA, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := core.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	lowerLeft := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 10})
	lowerRight := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 21, endByte: 10})
	left, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootA.Node, payload: lowerLeft}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootB.Node, payload: lowerRight}})
	if err != nil {
		t.Fatal(err)
	}
	externalChild, err := core.appendAuthenticatedTerminal(subtreeRecord{
		symbol: 31, startByte: 10, endByte: 11, external: true, terminal: true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	top := externalChild
	if externalDescendant {
		top, err = core.appendSubtree(
			subtreeRecord{symbol: 30, startByte: 10, endByte: 11},
			[]SubtreeID{externalChild}, nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	key := core.boundaryKey(8, 11)
	oldTop, err := core.condense(key, linkInput{prev: left, payload: top})
	if err != nil {
		t.Fatal(err)
	}
	return certifiedExternalRecursiveInsertFixture{
		core: core, oldTop: oldTop, rightPrev: Head{Node: right},
		lowerLeft: lowerLeft, lowerRight: lowerRight, top: top, key: key,
	}
}

func TestRecursiveInsertAcceptsAuthenticatedNonzeroCheckpoint(t *testing.T) {
	checkpoint := [32]byte{1, 3, 5, 7}
	fixture := newRecursiveInsertFixtureWithCheckpoint(t, checkpoint)
	merged, err := fixture.core.condense(fixture.core.boundaryKey(8, 11), linkInput{
		prev: fixture.rightPrev.Node, payload: fixture.topPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	top, err := fixture.core.node(merged.Node)
	if err != nil || top.pathCount != 2 {
		t.Fatalf("nonzero-checkpoint top=%+v err=%v", top, err)
	}
	canonical, ok := fixture.core.CanonicalBoundary(8, 11, false, fixture.checkpoint)
	if !ok || canonical != merged {
		t.Fatalf("nonzero-checkpoint canonical=%+v ok=%t want=%+v", canonical, ok, merged)
	}
}

func TestRecursiveInsertTwoShallowClassesPreservesABCAndReplay(t *testing.T) {
	fixture := newRecursiveInsertFixture(t)
	core := fixture.core
	oldTopPaths, err := core.Derivations(fixture.oldTop)
	if err != nil {
		t.Fatal(err)
	}
	leftBefore, err := core.Derivations(fixture.leftPredecessor)
	if err != nil {
		t.Fatal(err)
	}
	rightBefore, err := core.Derivations(fixture.rightPrev)
	if err != nil {
		t.Fatal(err)
	}

	workBeforeMerge := core.Work()
	merged, err := core.condense(core.boundaryKey(8, 11), linkInput{
		prev: fixture.rightPrev.Node, payload: fixture.topPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged == fixture.oldTop {
		t.Fatal("recursive insertion did not publish a fresh top snapshot")
	}
	workAfterMerge := core.Work()
	if workAfterMerge.PredecessorLinkUnionAttempts-workBeforeMerge.PredecessorLinkUnionAttempts != 9 ||
		workAfterMerge.PredecessorLinkUnionDuplicateNoop-workBeforeMerge.PredecessorLinkUnionDuplicateNoop != 7 ||
		workAfterMerge.PredecessorLinkUnionRecursiveChanged-workBeforeMerge.PredecessorLinkUnionRecursiveChanged != 1 ||
		workAfterMerge.PredecessorLinkUnionAlternateAppended-workBeforeMerge.PredecessorLinkUnionAlternateAppended != 1 ||
		workAfterMerge.PredecessorLinkUnionPrecedenceReplaced != workBeforeMerge.PredecessorLinkUnionPrecedenceReplaced ||
		workAfterMerge.PredecessorLinkUnionRejected != workBeforeMerge.PredecessorLinkUnionRejected {
		t.Fatalf("recursive insertion outcome vector before=%+v after=%+v", workBeforeMerge, workAfterMerge)
	}
	top, err := core.node(merged.Node)
	if err != nil || top.linkCount != 1 || top.pathCount != 2 {
		t.Fatalf("merged top=%+v err=%v, want one link/two paths", top, err)
	}
	topLinks, err := core.nodeLinks(*top)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := core.node(topLinks[0].prev)
	if err != nil || predecessor.linkCount != 2 || predecessor.pathCount != 2 {
		t.Fatalf("merged predecessor=%+v err=%v, want two links/two paths", predecessor, err)
	}
	lower, err := core.nodeLinks(*predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if lower[0].prev != fixture.rootA.Node || lower[0].payload != fixture.classAFirst || lower[1].prev != fixture.rootB.Node || lower[1].payload != fixture.classBFirst {
		t.Fatalf("4+5 shallow classes lost stable incumbents: %#v", lower)
	}
	paths, err := core.Derivations(merged)
	if err != nil {
		t.Fatal(err)
	}
	want := []Derivation{
		{Payloads: []SubtreeID{fixture.classAFirst, fixture.topPayload}},
		{Payloads: []SubtreeID{fixture.classBFirst, fixture.topPayload}},
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("merged derivations=%#v, want %#v", paths, want)
	}

	if got, _ := core.Derivations(fixture.oldTop); !reflect.DeepEqual(got, oldTopPaths) {
		t.Fatalf("historical top A mutated: got=%#v want=%#v", got, oldTopPaths)
	}
	if got, _ := core.Derivations(fixture.leftPredecessor); !reflect.DeepEqual(got, leftBefore) {
		t.Fatalf("historical predecessor B mutated: got=%#v want=%#v", got, leftBefore)
	}
	if got, _ := core.Derivations(fixture.rightPrev); !reflect.DeepEqual(got, rightBefore) {
		t.Fatalf("historical predecessor C mutated: got=%#v want=%#v", got, rightBefore)
	}

	beforeReplay, err := core.Stats(merged)
	if err != nil {
		t.Fatal(err)
	}
	workBeforeReplay := core.Work()
	replayed, err := core.condenseWithOutcome(core.boundaryKey(8, 11), linkInput{
		prev: fixture.rightPrev.Node, payload: fixture.topPayload,
	})
	if err != nil || replayed.head != merged || replayed.change != condenseUnchanged {
		t.Fatalf("replay=%+v want unchanged head=%+v err=%v", replayed, merged, err)
	}
	afterReplay, err := core.Stats(replayed.head)
	workAfterReplay := core.Work()
	if err != nil || afterReplay != beforeReplay ||
		workAfterReplay.PredecessorLinkUnionAttempts-workBeforeReplay.PredecessorLinkUnionAttempts != 9 ||
		workAfterReplay.PredecessorLinkUnionDuplicateNoop-workBeforeReplay.PredecessorLinkUnionDuplicateNoop != 9 {
		t.Fatalf("replay changed storage/work: before=%+v after=%+v work=%+v/%+v err=%v", beforeReplay, afterReplay, core.Work(), workBeforeReplay, err)
	}
}

func TestRecursiveInsertLowerFoldRunsBeforeLocalCap(t *testing.T) {
	newFixture := func(t *testing.T, includeDistinct bool) (*Core, Head, NodeID, SubtreeID) {
		t.Helper()
		core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8})
		root, _ := core.Seed(1, 0)
		incumbent := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 1, endByte: 10, childSymbols: []Symbol{40}})
		equivalent := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 2, endByte: 10, childSymbols: []Symbol{41}})
		left, _ := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: root.Node, payload: incumbent, scoreDelta: 10}})
		rightLinks := []linkRecord{{prev: root.Node, payload: equivalent, scoreDelta: 9}}
		if includeDistinct {
			distinct := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 21, endByte: 10})
			rightLinks = append(rightLinks, linkRecord{prev: root.Node, payload: distinct})
		}
		right, _ := core.appendAdjacencyNode(7, 10, rightLinks)
		top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, startByte: 10, endByte: 11})
		oldTop, _ := core.condense(core.boundaryKey(8, 11), linkInput{prev: left, payload: top})
		core.limits.MaxLinksPerBoundary = 1
		return core, oldTop, right, top
	}

	t.Run("equivalent-at-cap", func(t *testing.T) {
		core, oldTop, right, top := newFixture(t, false)
		before, _ := core.Stats(oldTop)
		beforeWork := core.Work()
		outcome, err := core.condenseWithOutcome(core.boundaryKey(8, 11), linkInput{prev: right, payload: top})
		if err != nil || outcome.head != oldTop || outcome.change != condenseUnchanged {
			t.Fatalf("equivalent at cap outcome=%+v err=%v", outcome, err)
		}
		after, _ := core.Stats(oldTop)
		afterWork := core.Work()
		if after != before ||
			afterWork.PredecessorLinkUnionAttempts-beforeWork.PredecessorLinkUnionAttempts != 2 ||
			afterWork.PredecessorLinkUnionDuplicateNoop-beforeWork.PredecessorLinkUnionDuplicateNoop != 2 {
			t.Fatalf("equivalent fold changed storage/work: stats=%+v/%+v work=%+v/%+v", after, before, core.Work(), beforeWork)
		}
	})

	t.Run("distinct-second-class", func(t *testing.T) {
		core, oldTop, right, top := newFixture(t, true)
		before, _ := core.Stats(oldTop)
		beforeWork := core.Work()
		_, err := core.condense(core.boundaryKey(8, 11), linkInput{prev: right, payload: top})
		var capacity *LiveLinkCapacityError
		if !errors.As(err, &capacity) || capacity.State != 7 || capacity.ByteOffset != 10 || capacity.ObservedLinks != 2 || capacity.Limit != 1 {
			t.Fatalf("lower distinct-class cap=%+v err=%v", capacity, err)
		}
		after, _ := core.Stats(oldTop)
		canonical, ok := core.CanonicalBoundary(8, 11, false, 0)
		if after != before || core.Work() != beforeWork || !ok || canonical != oldTop {
			t.Fatalf("lower cap rollback drift: stats=%+v/%+v work=%+v/%+v canonical=%+v ok=%t", after, before, core.Work(), beforeWork, canonical, ok)
		}
	})
}

func TestRecursiveInsertSamePairPrecedenceAndDistinctClass(t *testing.T) {
	for _, test := range []struct {
		name          string
		incomingScore int64
		incomingSpec  shallowPayloadSpec
		wantLinks     uint32
		wantIncoming  bool
	}{
		{name: "lower", incomingScore: 9, incomingSpec: shallowPayloadSpec{symbol: 20, productionID: 2, startByte: 0, endByte: 10, childSymbols: []Symbol{42}}, wantLinks: 1},
		{name: "equal", incomingScore: 10, incomingSpec: shallowPayloadSpec{symbol: 20, productionID: 2, startByte: 0, endByte: 10, childSymbols: []Symbol{42}}, wantLinks: 1},
		{name: "higher", incomingScore: 11, incomingSpec: shallowPayloadSpec{symbol: 20, productionID: 2, startByte: 0, endByte: 10, childSymbols: []Symbol{42}}, wantLinks: 1, wantIncoming: true},
		{name: "distinct", incomingScore: 11, incomingSpec: shallowPayloadSpec{symbol: 21, productionID: 2, startByte: 0, endByte: 10, childSymbols: []Symbol{42}}, wantLinks: 2, wantIncoming: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8})
			root, _ := core.Seed(1, 0)
			incumbent := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 1, startByte: 0, endByte: 10, childSymbols: []Symbol{41}})
			incoming := appendShallowPayload(t, core, test.incomingSpec)
			left, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: root.Node, payload: incumbent, scoreDelta: 10}})
			if err != nil {
				t.Fatal(err)
			}
			right, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: root.Node, payload: incoming, scoreDelta: test.incomingScore}})
			if err != nil {
				t.Fatal(err)
			}
			topPayload := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, startByte: 10, endByte: 11})
			oldTop, err := core.condense(core.boundaryKey(8, 11), linkInput{prev: left, payload: topPayload})
			if err != nil {
				t.Fatal(err)
			}
			merged, err := core.condense(core.boundaryKey(8, 11), linkInput{prev: right, payload: topPayload})
			if err != nil {
				t.Fatal(err)
			}
			wantChanged := test.wantIncoming
			if (merged != oldTop) != wantChanged {
				t.Fatalf("top changed=%t want=%t old=%+v merged=%+v", merged != oldTop, wantChanged, oldTop, merged)
			}
			top, _ := core.node(merged.Node)
			topLinks, _ := core.nodeLinks(*top)
			lowerNode, _ := core.node(topLinks[0].prev)
			lower, _ := core.nodeLinks(*lowerNode)
			if lowerNode.linkCount != test.wantLinks || lowerNode.pathCount != uint64(test.wantLinks) {
				t.Fatalf("lower node=%+v links=%#v", lowerNode, lower)
			}
			wantFirst := incumbent
			if test.wantIncoming {
				if test.wantLinks == 1 {
					wantFirst = incoming
				} else if lower[1].payload != incoming {
					t.Fatalf("distinct incoming lost stable append position: %#v", lower)
				}
			}
			if lower[0].payload != wantFirst {
				t.Fatalf("selected lower payload=%d want=%d links=%#v", lower[0].payload, wantFirst, lower)
			}
			oldPaths, err := core.Derivations(oldTop)
			if err != nil || len(oldPaths) != 1 || oldPaths[0].Payloads[0] != incumbent {
				t.Fatalf("historical incumbent changed: paths=%#v err=%v", oldPaths, err)
			}
		})
	}
}

func TestRecursiveInsertPersistsTwoPredecessorLevels(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	rootA, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := core.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	lowerA := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 10})
	lowerB := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 21, endByte: 10})
	leftLower, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootA.Node, payload: lowerA}})
	if err != nil {
		t.Fatal(err)
	}
	rightLower, err := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootB.Node, payload: lowerB}})
	if err != nil {
		t.Fatal(err)
	}
	wrapper := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, startByte: 10, endByte: 11})
	leftUpper, err := core.appendAdjacencyNode(8, 11, []linkRecord{{prev: leftLower, payload: wrapper}})
	if err != nil {
		t.Fatal(err)
	}
	rightUpper, err := core.appendAdjacencyNode(8, 11, []linkRecord{{prev: rightLower, payload: wrapper}})
	if err != nil {
		t.Fatal(err)
	}
	top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 40, startByte: 11, endByte: 12})
	key := core.boundaryKey(9, 12)
	old, err := core.condense(key, linkInput{prev: leftUpper, payload: top})
	if err != nil {
		t.Fatal(err)
	}
	oldPaths, err := core.Derivations(old)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := core.condense(key, linkInput{prev: rightUpper, payload: top})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := core.Derivations(merged)
	if err != nil {
		t.Fatal(err)
	}
	want := []Derivation{
		{Payloads: []SubtreeID{lowerA, wrapper, top}},
		{Payloads: []SubtreeID{lowerB, wrapper, top}},
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("bounded recursive derivations=%#v, want %#v", paths, want)
	}
	if got, err := core.Derivations(old); err != nil || !reflect.DeepEqual(got, oldPaths) {
		t.Fatalf("historical recursive head changed: paths=%#v want=%#v err=%v", got, oldPaths, err)
	}
}

func TestRecursiveInsertDeclinesUnsupportedProvenanceAndDepth(t *testing.T) {
	t.Run("external-top-leaf", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		left, _ := core.Seed(1, 0)
		rightID, _ := core.appendNode(nodeRecord{state: 1, byteOffset: 0, pathCount: 1})
		top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, endByte: 1, external: true})
		key := core.boundaryKey(8, 1)
		old, err := core.condense(key, linkInput{prev: left.Node, payload: top})
		if err != nil {
			t.Fatal(err)
		}
		before, _ := core.Stats(old)
		if _, err := core.condense(key, linkInput{prev: rightID, payload: top}); err == nil || !strings.Contains(err.Error(), "external payload") {
			t.Fatalf("external top error=%v", err)
		}
		after, _ := core.Stats(old)
		if after != before {
			t.Fatalf("external top decline mutated storage: before=%+v after=%+v", before, after)
		}
	})

	t.Run("external-descendant", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		root, _ := core.Seed(1, 0)
		clean := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 10, childSymbols: []Symbol{40}})
		externalChild := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 41, endByte: 10, external: true})
		externalParent, err := core.appendSubtree(subtreeRecord{symbol: 20, endByte: 10}, []SubtreeID{externalChild}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		left, _ := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: root.Node, payload: clean}})
		right, _ := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: root.Node, payload: externalParent}})
		top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, startByte: 10, endByte: 11})
		key := core.boundaryKey(8, 11)
		old, _ := core.condense(key, linkInput{prev: left, payload: top})
		before, _ := core.Stats(old)
		if _, err := core.condense(key, linkInput{prev: right, payload: top}); err == nil || !strings.Contains(err.Error(), "external payload") {
			t.Fatalf("external descendant error=%v", err)
		}
		after, _ := core.Stats(old)
		if after != before {
			t.Fatalf("external descendant decline mutated storage: before=%+v after=%+v", before, after)
		}
	})

	t.Run("non-exact-nested-edge", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		rootA, _ := core.Seed(1, 0)
		rootBID, _ := core.appendNode(nodeRecord{state: 1, byteOffset: 0, pathCount: 1})
		lowerA := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 1, endByte: 10})
		lowerB := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 2, endByte: 10})
		leftLower, _ := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootA.Node, payload: lowerA}})
		rightLower, _ := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootBID, payload: lowerB}})
		wrapper := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, startByte: 10, endByte: 11})
		left, _ := core.appendAdjacencyNode(8, 11, []linkRecord{{prev: leftLower, payload: wrapper}})
		right, _ := core.appendAdjacencyNode(8, 11, []linkRecord{{prev: rightLower, payload: wrapper}})
		top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 40, startByte: 11, endByte: 12})
		key := core.boundaryKey(9, 12)
		old, _ := core.condense(key, linkInput{prev: left, payload: top})
		before, _ := core.Stats(old)
		beforeWork := core.Work()
		if _, err := core.condense(key, linkInput{prev: right, payload: top}); err == nil || !strings.Contains(err.Error(), "non-exact nested edge") {
			t.Fatalf("non-exact nested edge error=%v", err)
		}
		after, _ := core.Stats(old)
		if after != before || core.Work() != beforeWork {
			t.Fatalf("non-exact nested decline mutated storage/work: before=%+v after=%+v work=%+v/%+v", before, after, core.Work(), beforeWork)
		}
	})

	t.Run("depth-limit", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		left, _ := core.Seed(1, 0)
		rightID, _ := core.appendNode(nodeRecord{state: 1, byteOffset: 0, pathCount: 1})
		right := Head{Node: rightID}
		for depth := 0; depth <= maxRecursiveInsertDepth; depth++ {
			offset := uint32(depth + 1)
			payload := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: Symbol(20 + depth), startByte: offset - 1, endByte: offset,
			})
			leftID, err := core.appendAdjacencyNode(StateID(10+depth), offset, []linkRecord{{prev: left.Node, payload: payload}})
			if err != nil {
				t.Fatal(err)
			}
			rightID, err := core.appendAdjacencyNode(StateID(10+depth), offset, []linkRecord{{prev: right.Node, payload: payload}})
			if err != nil {
				t.Fatal(err)
			}
			left, right = Head{Node: leftID}, Head{Node: rightID}
		}
		top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 50, startByte: 17, endByte: 18})
		key := core.boundaryKey(60, 18)
		old, err := core.condense(key, linkInput{prev: left.Node, payload: top})
		if err != nil {
			t.Fatal(err)
		}
		before, _ := core.Stats(old)
		beforeWork := core.Work()
		if _, err := core.condense(key, linkInput{prev: right.Node, payload: top}); err == nil || !strings.Contains(err.Error(), "depth limit") {
			t.Fatalf("recursive depth error=%v", err)
		}
		after, _ := core.Stats(old)
		if after != before || core.Work() != beforeWork {
			t.Fatalf("recursive depth decline mutated storage/work: before=%+v after=%+v work=%+v/%+v", before, after, core.Work(), beforeWork)
		}
	})
}

func TestRecursiveInsertCertifiedQuiescentExternalPayloads(t *testing.T) {
	for _, test := range []struct {
		name               string
		externalDescendant bool
	}{
		{name: "top"},
		{name: "descendant", externalDescendant: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCertifiedExternalRecursiveInsertFixture(t, test.externalDescendant)
			merged, err := fixture.core.condense(fixture.key, linkInput{
				prev: fixture.rightPrev.Node, payload: fixture.top,
			})
			if err != nil {
				t.Fatal(err)
			}
			paths, err := fixture.core.Derivations(merged)
			if err != nil {
				t.Fatal(err)
			}
			want := []Derivation{
				{Payloads: []SubtreeID{fixture.lowerLeft, fixture.top}},
				{Payloads: []SubtreeID{fixture.lowerRight, fixture.top}},
			}
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("certified external derivations=%#v, want %#v", paths, want)
			}
		})
	}
}

func TestRecursiveInsertAcceptsExactExternalScannerProvenance(t *testing.T) {
	for _, test := range []struct {
		name               string
		externalDescendant bool
	}{
		{name: "top"},
		{name: "descendant", externalDescendant: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newExactExternalRecursiveInsertFixture(t, test.externalDescendant)
			merged, err := fixture.core.condense(fixture.key, linkInput{
				prev: fixture.rightPrev.Node, payload: fixture.top,
			})
			if err != nil {
				t.Fatal(err)
			}
			paths, err := fixture.core.Derivations(merged)
			if err != nil {
				t.Fatal(err)
			}
			want := []Derivation{
				{Payloads: []SubtreeID{fixture.lowerLeft, fixture.top}},
				{Payloads: []SubtreeID{fixture.lowerRight, fixture.top}},
			}
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("exact external derivations=%#v, want %#v", paths, want)
			}
		})
	}
}

func TestSubtreeExternalProvenanceUsesPackedCache(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	ordinary, err := core.appendSubtree(subtreeRecord{symbol: 10, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := core.subtrees[ordinary-1].externalProvenanceState; got != subtreeExternalProvenanceExactNoExternal {
		t.Fatalf("ordinary provenance state = %d", got)
	}

	start := mustInternCheckpoint(t, core, []byte{1})
	end := mustInternCheckpoint(t, core, []byte{2})
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, end); err != nil {
		t.Fatal(err)
	}
	external, err := core.appendAuthenticatedTerminal(subtreeRecord{
		symbol: 11, external: true, terminal: true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := core.subtrees[external-1].externalProvenanceState; got != subtreeExternalProvenanceExactHasExternal {
		t.Fatalf("external provenance state = %d", got)
	}
	parent, err := core.appendSubtree(subtreeRecord{symbol: 12}, []SubtreeID{ordinary, external}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := core.subtrees[parent-1].externalProvenanceState; got != subtreeExternalProvenanceExactHasExternal {
		t.Fatalf("parent provenance state = %d", got)
	}

	var hasExternal, exact bool
	var provenanceErr error
	if allocs := testing.AllocsPerRun(100, func() {
		hasExternal, exact, provenanceErr = core.subtreeExternalProvenance(parent)
	}); allocs != 0 {
		t.Fatalf("cached provenance allocations = %f", allocs)
	}
	if provenanceErr != nil || !hasExternal || !exact {
		t.Fatalf("cached provenance = has:%t exact:%t err:%v", hasExternal, exact, provenanceErr)
	}

	unproven, err := core.appendSubtree(subtreeRecord{
		symbol: 13, external: true, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hasExternal, exact, err := core.subtreeExternalProvenance(unproven); err != nil || !hasExternal || exact {
		t.Fatalf("unproven provenance = has:%t exact:%t err:%v", hasExternal, exact, err)
	}
}

func TestAuthenticatedTerminalScannerProvenanceCoversOrdinaryLeaf(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{})
	start := mustInternCheckpoint(t, compact, []byte{1})
	end := mustInternCheckpoint(t, compact, []byte{2})
	if err := compact.SetPhaseExternalTokenScannerCheckpoints(start, end); err != nil {
		t.Fatal(err)
	}

	withoutCapability, err := compact.appendAuthenticatedTerminal(subtreeRecord{
		symbol: 10, terminal: true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := compact.externalPayloadScannerProvenance(withoutCapability); ok {
		t.Fatal("ordinary terminal retained scanner checkpoints without the language capability")
	}

	compact.EnableTerminalScannerCheckpointProvenance()
	ordinary, err := compact.appendAuthenticatedTerminal(subtreeRecord{
		symbol: 11, terminal: true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := compact.subtrees[ordinary-1].externalProvenanceState; got != subtreeExternalProvenanceExactNoExternal {
		t.Fatalf("ordinary terminal changed external provenance state: %d", got)
	}
	provenance, ok := compact.externalPayloadScannerProvenance(ordinary)
	if !ok || provenance.start != start || provenance.end != end {
		t.Fatalf("ordinary terminal scanner provenance = %+v, %t", provenance, ok)
	}

	view, err := compact.MaterializationView(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if !view.ExternalScannerCheckpointExact || view.ExternalScannerCheckpointStart != start || view.ExternalScannerCheckpointEnd != end {
		t.Fatalf("ordinary terminal materialization checkpoint = %+v", view)
	}

	var visited MaterializationSubtreeView
	var scratch MaterializationPostorderScratch
	if err := compact.VisitMaterializationPostorderWithScratch(
		[]SubtreeID{ordinary}, nil, &scratch,
		func(_ SubtreeID, got MaterializationSubtreeView) error {
			visited = got
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if !visited.ExternalScannerCheckpointExact || visited.ExternalScannerCheckpointStart != start || visited.ExternalScannerCheckpointEnd != end {
		t.Fatalf("ordinary terminal postorder checkpoint = %+v", visited)
	}
}

func TestRecursiveInsertKeepsScannerCheckpointMismatchSeparate(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	start := mustInternCheckpoint(t, core, []byte{1})
	leftCheckpoint := mustInternCheckpoint(t, core, []byte{2})
	rightCheckpoint := mustInternCheckpoint(t, core, []byte{3})
	if err := core.SetPhaseCheckpoint(rightCheckpoint); err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, rightCheckpoint); err != nil {
		t.Fatal(err)
	}
	leftRoot, _ := core.appendNodeAt(nodeRecord{
		state: 1, byteOffset: 0, pathCount: 1,
	}, leftCheckpoint)
	rightRoot, _ := core.appendNodeAt(nodeRecord{
		state: 1, byteOffset: 0, pathCount: 1,
	}, rightCheckpoint)
	lower := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 10})
	left, err := core.appendAdjacencyNodeAt(
		7, 10, leftCheckpoint, []linkRecord{{prev: leftRoot, payload: lower}},
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAdjacencyNodeAt(
		7, 10, rightCheckpoint, []linkRecord{{prev: rightRoot, payload: lower}},
	)
	if err != nil {
		t.Fatal(err)
	}
	top, err := core.appendAuthenticatedTerminal(subtreeRecord{
		symbol: 30, startByte: 10, endByte: 11, external: true, terminal: true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	key := core.boundaryKey(8, 11)
	if _, err := core.condense(key, linkInput{prev: left, payload: top}); err != nil {
		t.Fatal(err)
	}
	merged, err := core.condense(key, linkInput{prev: right, payload: top})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := core.Derivations(merged)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || core.Work().PredecessorLinkUnionRecursiveChanged != 0 {
		t.Fatalf("checkpoint mismatch paths=%#v work=%+v", paths, core.Work())
	}
}

func TestExternalTokenScannerCheckpointValidation(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	known := mustInternCheckpoint(t, core, []byte{1})
	if err := core.SetPhaseCheckpoint(known); err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(known, CheckpointID(99)); err == nil {
		t.Fatal("unknown end checkpoint was accepted")
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(CheckpointID(99), known); err == nil {
		t.Fatal("unknown start checkpoint was accepted")
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(known, known); err != nil {
		t.Fatal(err)
	}
	if !core.externalTokenScannerExact {
		t.Fatal("exact token checkpoint pair was not retained")
	}
	phase := core.classificationPhase
	if err := core.SetPhaseExternalTokenScannerCheckpoints(known, CheckpointID(99)); err == nil {
		t.Fatal("unknown end checkpoint was accepted after exact proof")
	}
	if !core.externalTokenScannerExact || core.externalTokenScannerStart != known ||
		core.externalTokenScannerEnd != known || core.classificationPhase != phase {
		t.Fatal("rejected token checkpoint pair changed the prior exact proof")
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(CheckpointID(99), known); err == nil {
		t.Fatal("unknown start checkpoint was accepted after exact proof")
	}
	if !core.externalTokenScannerExact || core.externalTokenScannerStart != known ||
		core.externalTokenScannerEnd != known || core.classificationPhase != phase {
		t.Fatal("rejected token start checkpoint changed the prior exact proof")
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(known, known); err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseCheckpoint(known); err != nil {
		t.Fatal(err)
	}
	if core.externalTokenScannerExact {
		t.Fatal("phase checkpoint election retained stale token proof")
	}
	if err := core.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	if core.externalTokenScannerExact {
		t.Fatal("frontier advance retained a stale token checkpoint pair")
	}
}

func TestExternalTokenScannerCheckpointStartChangeInvalidatesBoundary(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	firstStart := mustInternCheckpoint(t, core, []byte{1})
	secondStart := mustInternCheckpoint(t, core, []byte{2})
	end := mustInternCheckpoint(t, core, []byte{3})
	if err := core.SetPhaseExternalTokenScannerCheckpoints(firstStart, end); err != nil {
		t.Fatal(err)
	}
	head, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	classified, err := core.ClassifyBoundary(head, 9)
	if err != nil {
		t.Fatal(err)
	}
	generation := core.AuthenticationGeneration()
	if err := core.SetPhaseExternalTokenScannerCheckpoints(secondStart, end); err != nil {
		t.Fatal(err)
	}
	if core.AuthenticationGeneration() <= generation {
		t.Fatalf("scanner start change did not advance authentication: before=%d after=%d", generation, core.AuthenticationGeneration())
	}
	_, start, gotEnd, exact := core.PhaseScannerCheckpoints()
	if !exact || start != secondStart || gotEnd != end {
		t.Fatalf("scanner pair=(%d,%d,%t), want (%d,%d,true)", start, gotEnd, exact, secondStart, end)
	}
	if err := core.validateClassification(classified); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("old boundary validation error=%v, want stale", err)
	}
}

func TestRecursiveInsertCertifiedExternalIdentityAndValidation(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	core.CertifyExternalPayloadsQuiescent()
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	clean := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 1})
	external := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 1, external: true})
	equal, err := core.shallowPayloadsEqual(seed.Node, clean, seed.Node, external)
	if err != nil {
		t.Fatal(err)
	}
	if equal {
		t.Fatal("certified external payload matched a non-external shallow class")
	}
	if _, _, err := core.subtreeExternalProvenance(SubtreeID(len(core.subtrees) + 1)); err == nil {
		t.Fatal("certified external fast path accepted an invalid subtree identifier")
	}
	if err := core.Reset(); err != nil {
		t.Fatal(err)
	}
	external = appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 1, external: true})
	if _, exact, err := core.subtreeExternalProvenance(external); err != nil || !exact {
		t.Fatalf("reset lost the external-payload certificate: exact=%t err=%v", exact, err)
	}
}

func TestRecursiveInsertCertifiedExternalRollback(t *testing.T) {
	fixture := newCertifiedExternalRecursiveInsertFixture(t, false)
	core := fixture.core
	before, err := core.Stats(fixture.oldTop)
	if err != nil {
		t.Fatal(err)
	}
	beforeWork := core.Work()
	beforePaths, err := core.Derivations(fixture.oldTop)
	if err != nil {
		t.Fatal(err)
	}
	core.limits.MaxLinks = uint32(len(core.links) + 2)
	if _, err := core.condense(fixture.key, linkInput{
		prev: fixture.rightPrev.Node, payload: fixture.top,
	}); err == nil || !strings.Contains(err.Error(), "link arena cap") {
		t.Fatalf("certified external cap error=%v", err)
	}
	after, err := core.Stats(fixture.oldTop)
	if err != nil {
		t.Fatal(err)
	}
	afterPaths, err := core.Derivations(fixture.oldTop)
	if err != nil {
		t.Fatal(err)
	}
	canonical, ok := core.CanonicalBoundary(8, 11, false, 0)
	if after != before || core.Work() != beforeWork ||
		!reflect.DeepEqual(afterPaths, beforePaths) || !ok || canonical != fixture.oldTop {
		t.Fatalf(
			"certified external rollback drift: stats=%+v/%+v work=%+v/%+v paths=%#v/%#v canonical=%+v ok=%t",
			after, before, core.Work(), beforeWork, afterPaths, beforePaths, canonical, ok,
		)
	}
}

func TestRecursiveInsertExactExternalRollback(t *testing.T) {
	fixture := newExactExternalRecursiveInsertFixture(t, false)
	core := fixture.core
	before, _ := core.Stats(fixture.oldTop)
	beforeWork := core.Work()
	beforePaths, _ := core.Derivations(fixture.oldTop)
	core.limits.MaxLinks = uint32(len(core.links) + 2)
	if _, err := core.condense(fixture.key, linkInput{
		prev: fixture.rightPrev.Node, payload: fixture.top,
	}); err == nil || !strings.Contains(err.Error(), "link arena cap") {
		t.Fatalf("exact external cap error=%v", err)
	}
	after, _ := core.Stats(fixture.oldTop)
	afterPaths, _ := core.Derivations(fixture.oldTop)
	canonical, ok := core.CanonicalBoundary(
		fixture.key.state,
		fixture.key.byteOffset,
		fixture.key.shifted,
		fixture.key.checkpoint,
	)
	if after != before || core.Work() != beforeWork ||
		!reflect.DeepEqual(afterPaths, beforePaths) || !ok || canonical != fixture.oldTop {
		t.Fatalf(
			"exact external rollback drift: stats=%+v/%+v work=%+v/%+v paths=%#v/%#v canonical=%+v ok=%t",
			after, before, core.Work(), beforeWork, afterPaths, beforePaths, canonical, ok,
		)
	}
}

func TestRecursiveInsertDeclinesSelfAndAncestry(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	seed, _ := core.Seed(1, 0)
	if _, _, err := core.mergePredecessorsBounded(seed.Node, seed.Node, 0, &precedenceMaximumWitness{}); err == nil || !strings.Contains(err.Error(), "self-merge") {
		t.Fatalf("self merge error=%v", err)
	}
	payload := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 1})
	rightID, _ := core.appendNode(nodeRecord{state: 7, byteOffset: 1, pathCount: 1})
	leftID, _ := core.appendAdjacencyNode(7, 1, []linkRecord{{prev: rightID, payload: payload}})
	if _, _, err := core.mergePredecessorsBounded(leftID, rightID, 0, &precedenceMaximumWitness{}); err == nil || !strings.Contains(err.Error(), "ancestry-related") {
		t.Fatalf("ancestry merge error=%v", err)
	}
}

func TestRecursiveInsertDirectAndNestedRollback(t *testing.T) {
	t.Run("direct-cap", func(t *testing.T) {
		fixture := newRecursiveInsertFixture(t)
		core := fixture.core
		before, _ := core.Stats(fixture.oldTop)
		beforeWork := core.Work()
		core.limits.MaxLinks = uint32(len(core.links) + 2)
		if _, err := core.condense(core.boundaryKey(8, 11), linkInput{prev: fixture.rightPrev.Node, payload: fixture.topPayload}); err == nil || !strings.Contains(err.Error(), "link arena cap") {
			t.Fatalf("direct cap error=%v", err)
		}
		after, _ := core.Stats(fixture.oldTop)
		canonical, ok := core.CanonicalBoundary(8, 11, false, fixture.checkpoint)
		if after != before || core.Work() != beforeWork || !ok || canonical != fixture.oldTop {
			t.Fatalf("direct rollback drift: stats=%+v/%+v work=%+v/%+v canonical=%+v ok=%t", after, before, core.Work(), beforeWork, canonical, ok)
		}
	})

	t.Run("outer-atomic", func(t *testing.T) {
		fixture := newRecursiveInsertFixture(t)
		core := fixture.core
		before, _ := core.Stats(fixture.oldTop)
		beforeWork := core.Work()
		sentinel := errors.New("outer rollback")
		err := core.ApplyAtomic(func() error {
			if _, err := core.condense(core.boundaryKey(8, 11), linkInput{prev: fixture.rightPrev.Node, payload: fixture.topPayload}); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("outer rollback error=%v", err)
		}
		after, _ := core.Stats(fixture.oldTop)
		canonical, ok := core.CanonicalBoundary(8, 11, false, fixture.checkpoint)
		if after != before || core.Work() != beforeWork || !ok || canonical != fixture.oldTop {
			t.Fatalf("nested rollback drift: stats=%+v/%+v work=%+v/%+v canonical=%+v ok=%t", after, before, core.Work(), beforeWork, canonical, ok)
		}
	})
}

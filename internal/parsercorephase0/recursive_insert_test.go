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
	core.SetPhaseCheckpoint(checkpoint)
	oldTop, err := core.condense(core.boundaryKey(8, 11), linkInput{prev: leftID, payload: top})
	if err != nil {
		t.Fatal(err)
	}
	return recursiveInsertFixture{
		core: core, rootA: rootA, rootB: rootB,
		leftPredecessor: Head{Node: leftID}, rightPrev: Head{Node: rightID}, oldTop: oldTop,
		topPayload: top, classAFirst: payloads[0], classBFirst: payloads[2],
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
	canonical, ok := fixture.core.CanonicalBoundary(8, 11, false, checkpoint)
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

	merged, err := core.condense(core.boundaryKey(8, 11), linkInput{
		prev: fixture.rightPrev.Node, payload: fixture.topPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged == fixture.oldTop {
		t.Fatal("recursive insertion did not publish a fresh top snapshot")
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
	if err != nil || afterReplay != beforeReplay || core.Work() != workBeforeReplay {
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
		if after != before || core.Work() != beforeWork {
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
		canonical, ok := core.CanonicalBoundary(8, 11, false, [32]byte{})
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

	t.Run("second-recursive-layer", func(t *testing.T) {
		core := newTinyCoreWithLimits(t, Limits{})
		rootA, _ := core.Seed(1, 0)
		rootBID, _ := core.appendNode(nodeRecord{state: 1, byteOffset: 0, pathCount: 1})
		lower := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 10})
		left, _ := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootA.Node, payload: lower}})
		right, _ := core.appendAdjacencyNode(7, 10, []linkRecord{{prev: rootBID, payload: lower}})
		top := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 30, startByte: 10, endByte: 11})
		key := core.boundaryKey(8, 11)
		old, _ := core.condense(key, linkInput{prev: left, payload: top})
		before, _ := core.Stats(old)
		if _, err := core.condense(key, linkInput{prev: right, payload: top}); err == nil || !strings.Contains(err.Error(), "beyond one predecessor layer") {
			t.Fatalf("second-layer error=%v", err)
		}
		after, _ := core.Stats(old)
		if after != before {
			t.Fatalf("second-layer decline mutated storage: before=%+v after=%+v", before, after)
		}
	})
}

func TestRecursiveInsertDeclinesSelfAndAncestry(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	seed, _ := core.Seed(1, 0)
	if _, _, err := core.mergePredecessorsOneLayer(seed.Node, seed.Node); err == nil || !strings.Contains(err.Error(), "self-merge") {
		t.Fatalf("self merge error=%v", err)
	}
	payload := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, endByte: 1})
	rightID, _ := core.appendNode(nodeRecord{state: 7, byteOffset: 1, pathCount: 1})
	leftID, _ := core.appendAdjacencyNode(7, 1, []linkRecord{{prev: rightID, payload: payload}})
	if _, _, err := core.mergePredecessorsOneLayer(leftID, rightID); err == nil || !strings.Contains(err.Error(), "ancestry-related") {
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
		canonical, ok := core.CanonicalBoundary(8, 11, false, [32]byte{})
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
		canonical, ok := core.CanonicalBoundary(8, 11, false, [32]byte{})
		if after != before || core.Work() != beforeWork || !ok || canonical != fixture.oldTop {
			t.Fatalf("nested rollback drift: stats=%+v/%+v work=%+v/%+v canonical=%+v ok=%t", after, before, core.Work(), beforeWork, canonical, ok)
		}
	})
}

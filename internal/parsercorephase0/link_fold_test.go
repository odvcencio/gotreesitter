package parsercorephase0

import (
	"reflect"
	"strings"
	"testing"
)

type shallowPayloadSpec struct {
	symbol       Symbol
	productionID uint16
	startByte    uint32
	endByte      uint32
	childSymbols []Symbol
	extra        bool
	external     bool
}

func newDiagnosticShallowFoldCore(t *testing.T, limits Limits) (*Core, Head) {
	t.Helper()
	core := newTinyCoreWithLimits(t, limits)
	seed, err := core.Seed(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	return core, seed
}

func appendShallowPayload(t *testing.T, core *Core, spec shallowPayloadSpec) SubtreeID {
	t.Helper()
	children := make([]SubtreeID, len(spec.childSymbols))
	for index, symbol := range spec.childSymbols {
		child, err := core.appendSubtree(subtreeRecord{
			symbol: symbol, startByte: spec.startByte, endByte: spec.endByte, terminal: true,
		}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		children[index] = child
	}
	payload, err := core.appendSubtree(subtreeRecord{
		symbol: spec.symbol, productionID: spec.productionID,
		startByte: spec.startByte, endByte: spec.endByte,
		extra: spec.extra, external: spec.external, terminal: len(children) == 0,
	}, children, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestDiagnosticShallowFoldChildBearingParentSelectsHigherAggregateScore(t *testing.T) {
	for _, test := range []struct {
		name          string
		incomingScore int64
		wantIncoming  bool
	}{
		{name: "lower", incomingScore: 9},
		{name: "equal", incomingScore: 10},
		{name: "higher", incomingScore: 11, wantIncoming: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			core, seed := newDiagnosticShallowFoldCore(t, Limits{MaxDerivations: 4})
			incumbent := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: 20, productionID: 1, startByte: 12, endByte: 17, childSymbols: []Symbol{30},
			})
			incoming := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: 20, productionID: 99, startByte: 12, endByte: 17, childSymbols: []Symbol{31},
			})
			key := core.boundaryKey(2, 17)
			oldHead, err := core.condense(key, linkInput{
				prev: seed.Node, payload: incumbent, scoreDelta: 10,
				order: ForkOrder{Present: true, Value: 7},
			})
			if err != nil {
				t.Fatal(err)
			}
			newHead, err := core.condense(key, linkInput{
				prev: seed.Node, payload: incoming, scoreDelta: test.incomingScore,
				order: ForkOrder{Present: true, Value: 99},
			})
			if err != nil {
				t.Fatal(err)
			}

			wantPayload, wantScore, wantOrder := incumbent, int64(10), uint64(7)
			if test.wantIncoming {
				wantPayload, wantScore, wantOrder = incoming, test.incomingScore, 99
				if newHead == oldHead {
					t.Fatalf("higher score retained historical head %+v", oldHead)
				}
			} else if newHead != oldHead {
				t.Fatalf("non-winning score published head %+v, want historical %+v", newHead, oldHead)
			}
			paths, err := core.Derivations(newHead)
			if err != nil {
				t.Fatal(err)
			}
			want := []Derivation{{
				Payloads: []SubtreeID{wantPayload}, Score: wantScore,
				BranchOrder: wantOrder, HasBranchOrder: true,
			}}
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("selected derivations = %#v, want %#v", paths, want)
			}
			oldPaths, err := core.Derivations(oldHead)
			if err != nil {
				t.Fatal(err)
			}
			wantOld := []Derivation{{
				Payloads: []SubtreeID{incumbent}, Score: 10,
				BranchOrder: 7, HasBranchOrder: true,
			}}
			if !reflect.DeepEqual(oldPaths, wantOld) {
				t.Fatalf("historical head mutated: got %#v, want %#v", oldPaths, wantOld)
			}
		})
	}
}

func TestCondenseOutcomeClassifiesBoundaryFreshness(t *testing.T) {
	core, seed := newDiagnosticShallowFoldCore(t, Limits{MaxDerivations: 4})
	incumbent := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 1, startByte: 12, endByte: 17, childSymbols: []Symbol{30},
	})
	key := core.boundaryKey(2, 17)
	created, err := core.condenseWithOutcome(key, linkInput{prev: seed.Node, payload: incumbent, scoreDelta: 10})
	if err != nil || created.change != condenseNew {
		t.Fatalf("created outcome=%+v err=%v", created, err)
	}
	exact, err := core.condenseWithOutcome(key, linkInput{prev: seed.Node, payload: incumbent, scoreDelta: 10})
	if err != nil || exact.change != condenseUnchanged || exact.head != created.head {
		t.Fatalf("exact outcome=%+v err=%v, want unchanged head %+v", exact, err, created.head)
	}

	lower := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 2, startByte: 12, endByte: 17, childSymbols: []Symbol{31},
	})
	for _, score := range []int64{9, 10} {
		outcome, err := core.condenseWithOutcome(key, linkInput{prev: seed.Node, payload: lower, scoreDelta: score})
		if err != nil || outcome.change != condenseUnchanged || outcome.head != created.head {
			t.Fatalf("non-winning score %d outcome=%+v err=%v", score, outcome, err)
		}
	}

	higher := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 3, startByte: 12, endByte: 17, childSymbols: []Symbol{32},
	})
	updated, err := core.condenseWithOutcome(key, linkInput{prev: seed.Node, payload: higher, scoreDelta: 11})
	if err != nil || updated.change != condenseUpdated || updated.head == created.head {
		t.Fatalf("winning outcome=%+v err=%v", updated, err)
	}

	distinct := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 21, startByte: 12, endByte: 17,
	})
	linked, err := core.condenseWithOutcome(key, linkInput{prev: seed.Node, payload: distinct})
	if err != nil || linked.change != condenseUpdated || linked.head == updated.head {
		t.Fatalf("distinct-link outcome=%+v err=%v", linked, err)
	}
}

func TestDiagnosticShallowFoldZeroChildParentHasZeroEffectivePrecedence(t *testing.T) {
	core, seed := newDiagnosticShallowFoldCore(t, Limits{MaxDerivations: 4})
	incumbent, err := core.appendSubtree(subtreeRecord{
		symbol: 20, productionID: 1, startByte: 12, endByte: 12,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := core.appendSubtree(subtreeRecord{
		symbol: 20, productionID: 2, startByte: 12, endByte: 12,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := core.boundaryKey(2, 12)
	oldHead, err := core.condense(key, linkInput{
		prev: seed.Node, payload: incumbent, scoreDelta: 7,
		order: ForkOrder{Present: true, Value: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	head, err := core.condense(key, linkInput{
		prev: seed.Node, payload: incoming, scoreDelta: 99,
		order: ForkOrder{Present: true, Value: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if head != oldHead {
		t.Fatalf("effective-zero incoming parent replaced incumbent: old=%+v new=%+v", oldHead, head)
	}
	paths, err := core.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	want := []Derivation{{
		Payloads: []SubtreeID{incumbent}, Score: 7,
		BranchOrder: 3, HasBranchOrder: true,
	}}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("zero-child parent paths=%#v, want stored incumbent score/order %#v", paths, want)
	}
}

func TestDiagnosticShallowFoldKeepsDistinctClasses(t *testing.T) {
	base := shallowPayloadSpec{symbol: 20, productionID: 1, startByte: 12, endByte: 17, childSymbols: []Symbol{30}}
	for _, test := range []struct {
		name string
		edit func(*shallowPayloadSpec)
	}{
		{name: "symbol", edit: func(spec *shallowPayloadSpec) { spec.symbol++ }},
		{name: "padding", edit: func(spec *shallowPayloadSpec) { spec.startByte++ }},
		{name: "size", edit: func(spec *shallowPayloadSpec) { spec.endByte++ }},
		{name: "child-count", edit: func(spec *shallowPayloadSpec) { spec.childSymbols = append(spec.childSymbols, 31) }},
		{name: "extra", edit: func(spec *shallowPayloadSpec) { spec.extra = true }},
		{name: "external-ineligible", edit: func(spec *shallowPayloadSpec) { spec.external = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			core, seed := newDiagnosticShallowFoldCore(t, Limits{MaxDerivations: 4})
			left := appendShallowPayload(t, core, base)
			rightSpec := base
			rightSpec.childSymbols = append([]Symbol(nil), base.childSymbols...)
			test.edit(&rightSpec)
			right := appendShallowPayload(t, core, rightSpec)
			key := core.boundaryKey(2, 17)
			head, err := core.condense(key, linkInput{prev: seed.Node, payload: left, scoreDelta: 1})
			if err != nil {
				t.Fatal(err)
			}
			head, err = core.condense(key, linkInput{prev: seed.Node, payload: right, scoreDelta: 2})
			if err != nil {
				t.Fatal(err)
			}
			paths, err := core.Derivations(head)
			if err != nil {
				t.Fatal(err)
			}
			if len(paths) != 2 {
				t.Fatalf("distinct class folded to %#v", paths)
			}
		})
	}
}

func TestDiagnosticShallowFoldRequiresExactPredecessor(t *testing.T) {
	core, leftSeed := newDiagnosticShallowFoldCore(t, Limits{MaxDerivations: 4})
	rightNode, err := core.appendNode(nodeRecord{state: 1, byteOffset: 10, pathCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	rightSeed := Head{Node: rightNode}
	if rightSeed.Node == leftSeed.Node {
		t.Fatalf("test fixture reused predecessor node %d", leftSeed.Node)
	}
	leftState, leftByte, err := core.Boundary(leftSeed)
	if err != nil {
		t.Fatal(err)
	}
	rightState, rightByte, err := core.Boundary(rightSeed)
	if err != nil {
		t.Fatal(err)
	}
	if leftState != rightState || leftByte != rightByte {
		t.Fatalf("predecessor boundaries differ: left=(%d,%d) right=(%d,%d)", leftState, leftByte, rightState, rightByte)
	}
	left := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, startByte: 12, endByte: 17})
	right := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 7, startByte: 12, endByte: 17})
	key := core.boundaryKey(2, 17)
	head, err := core.condense(key, linkInput{prev: leftSeed.Node, payload: left, scoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	head, err = core.condense(key, linkInput{prev: rightSeed.Node, payload: right, scoreDelta: 2})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := core.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("distinct exact predecessors folded to %#v", paths)
	}
}

func TestDiagnosticShallowFoldRebuildsAdjacencyWithoutMutatingOldHead(t *testing.T) {
	core, seed := newDiagnosticShallowFoldCore(t, Limits{MaxDerivations: 4})
	leading := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 21, startByte: 12, endByte: 17})
	incumbent := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, startByte: 12, endByte: 17, childSymbols: []Symbol{30}})
	trailing := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 22, startByte: 12, endByte: 17})
	incoming := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 7, startByte: 12, endByte: 17, childSymbols: []Symbol{31}})
	key := core.boundaryKey(2, 17)
	firstHead, err := core.condense(key, linkInput{prev: seed.Node, payload: leading, scoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	middleHead, err := core.condense(key, linkInput{prev: seed.Node, payload: incumbent, scoreDelta: 2})
	if err != nil {
		t.Fatal(err)
	}
	oldHead, err := core.condense(key, linkInput{prev: seed.Node, payload: trailing, scoreDelta: 3})
	if err != nil {
		t.Fatal(err)
	}
	before, err := core.Stats(oldHead)
	if err != nil {
		t.Fatal(err)
	}
	newHead, err := core.condense(key, linkInput{
		prev: seed.Node, payload: incoming, scoreDelta: 4,
		order: ForkOrder{Present: true, Value: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if newHead == oldHead || newHead == middleHead || newHead == firstHead {
		t.Fatalf("replacement did not publish a new canonical head: first=%+v middle=%+v old=%+v new=%+v", firstHead, middleHead, oldHead, newHead)
	}
	after, err := core.Stats(newHead)
	if err != nil {
		t.Fatal(err)
	}
	if after.Nodes != before.Nodes+1 || after.Links != before.Links+3 || after.CurrentExactPaths != 3 {
		t.Fatalf("replacement stats=%+v, before=%+v; want one node, copied adjacency, three paths", after, before)
	}
	oldPaths, err := core.Derivations(oldHead)
	if err != nil {
		t.Fatal(err)
	}
	wantOld := []Derivation{
		{Payloads: []SubtreeID{leading}, Score: 1},
		{Payloads: []SubtreeID{incumbent}, Score: 2},
		{Payloads: []SubtreeID{trailing}, Score: 3},
	}
	if !reflect.DeepEqual(oldPaths, wantOld) {
		t.Fatalf("historical adjacency mutated: got %#v, want %#v", oldPaths, wantOld)
	}
	newPaths, err := core.Derivations(newHead)
	if err != nil {
		t.Fatal(err)
	}
	wantNew := []Derivation{
		{Payloads: []SubtreeID{leading}, Score: 1},
		{Payloads: []SubtreeID{incoming}, Score: 4, BranchOrder: 8, HasBranchOrder: true},
		{Payloads: []SubtreeID{trailing}, Score: 3},
	}
	if !reflect.DeepEqual(newPaths, wantNew) {
		t.Fatalf("rebuilt adjacency = %#v, want %#v", newPaths, wantNew)
	}
}

func TestDiagnosticShallowFoldCapFailureRollsBack(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits Limits
		cap    string
	}{
		{name: "links", limits: Limits{MaxLinks: 1, MaxDerivations: 4}, cap: "link arena cap"},
		{name: "nodes", limits: Limits{MaxNodes: 2, MaxDerivations: 4}, cap: "node arena cap"},
	} {
		t.Run(test.name, func(t *testing.T) {
			core, seed := newDiagnosticShallowFoldCore(t, test.limits)
			incumbent := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, startByte: 12, endByte: 17, childSymbols: []Symbol{30}})
			incoming := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, productionID: 7, startByte: 12, endByte: 17, childSymbols: []Symbol{31}})
			key := core.boundaryKey(2, 17)
			oldHead, err := core.condense(key, linkInput{prev: seed.Node, payload: incumbent, scoreDelta: 1})
			if err != nil {
				t.Fatal(err)
			}
			before, err := core.Stats(oldHead)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := core.condense(key, linkInput{prev: seed.Node, payload: incoming, scoreDelta: 2}); err == nil || !strings.Contains(err.Error(), test.cap) {
				t.Fatalf("replacement error=%v, want %q", err, test.cap)
			}
			after, err := core.Stats(oldHead)
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("cap failure mutated storage: before=%+v after=%+v", before, after)
			}
			canonical, ok := core.CanonicalBoundary(2, 17, false, [32]byte{})
			if !ok || canonical != oldHead {
				t.Fatalf("cap failure changed canonical head: got=%+v ok=%t want=%+v", canonical, ok, oldHead)
			}
		})
	}
}

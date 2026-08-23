package parsercorephase0

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCondenseDirectAppendPublishesSingleLink(t *testing.T) {
	core := newTinyCore(t, 4)
	seed, err := core.Seed(7, 3)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 11, terminal: true, startByte: 3, endByte: 5}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := core.boundaryKey(9, 5)
	in := linkInput{prev: seed.Node, payload: payload, scoreDelta: 13, order: ForkOrder{Present: true, Value: 21}}
	out, err := core.condenseWithOutcome(key, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.change != condenseNew || out.head.Node == 0 {
		t.Fatalf("outcome=%+v, want a new head", out)
	}
	if out.historicalDropCohortRefs.Len() != 0 || out.historicalBoundarySplit || out.historicalConvergedSplit ||
		out.historicalForestDeterministic || out.historicalCleanPathRank != CleanPathRankNotApplicable ||
		out.historicalLineage != 0 || out.historicalNode != 0 {
		t.Fatalf("direct append fabricated history: %+v", out)
	}
	node, err := core.node(out.head.Node)
	if err != nil {
		t.Fatal(err)
	}
	if node.state != key.state || node.byteOffset != key.byteOffset || node.linkCount != 1 || node.pathCount != 1 {
		t.Fatalf("node=%+v, want one path at (%d,%d)", *node, key.state, key.byteOffset)
	}
	link := core.links[node.firstLink-1]
	want := linkRecord{prev: seed.Node, payload: payload, scoreDelta: 13, order: 21, flags: linkFlagHasOrder}
	if !reflect.DeepEqual(link, want) {
		t.Fatalf("link=%+v, want %+v", link, want)
	}
	if link.next != 0 {
		t.Fatalf("single link next=%d, want zero", link.next)
	}
	canonical, ok := core.CanonicalBoundary(key.state, key.byteOffset, false, key.checkpoint)
	if !ok || canonical != out.head {
		t.Fatalf("canonical=(%+v,%t), want (%+v,true)", canonical, ok, out.head)
	}
	work := core.Work()
	if work.GraphLinkAdditionsProxy != 1 || work.PredecessorLinkUnionAttempts != 0 {
		t.Fatalf("work=%+v, want one graph-link append and no union", work)
	}
}

func TestCondenseDirectAppendRollsBackPublication(t *testing.T) {
	core := newTinyCore(t, 4)
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 2, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := core.boundaryKey(3, 1)
	before, err := core.Stats(seed)
	if err != nil {
		t.Fatal(err)
	}
	beforeWork := core.Work()
	sentinel := errors.New("direct append rollback")
	err = core.ApplyAtomic(func() error {
		if _, err := core.condenseWithOutcomeAtomic(key, linkInput{prev: seed.Node, payload: payload}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error=%v, want %v", err, sentinel)
	}
	after, err := core.Stats(seed)
	if err != nil {
		t.Fatal(err)
	}
	if after != before || core.Work() != beforeWork {
		t.Fatalf("rollback changed state: before=%+v/%+v after=%+v/%+v", before, beforeWork, after, core.Work())
	}
	if _, ok := core.CanonicalBoundary(key.state, key.byteOffset, false, key.checkpoint); ok {
		t.Fatal("rolled-back direct append left a published boundary")
	}
}

func TestCondenseDirectAppendPreservesCapRollback(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxLinks: 1, MaxNodes: 3, MaxDerivations: 4, MaxPopPaths: 4})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := core.appendSubtree(subtreeRecord{symbol: 2, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := core.condense(core.boundaryKey(3, 1), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	before, err := core.Stats(first)
	if err != nil {
		t.Fatal(err)
	}
	_, err = core.condense(core.boundaryKey(4, 1), linkInput{prev: seed.Node, payload: payload})
	if err == nil || !strings.Contains(err.Error(), "link arena cap") {
		t.Fatalf("second direct append error=%v, want link arena cap", err)
	}
	after, err := core.Stats(first)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("cap failure changed state: before=%+v after=%+v", before, after)
	}
	if _, ok := core.CanonicalBoundary(4, 1, false, 0); ok {
		t.Fatal("cap failure published a boundary")
	}
}

package gotreesitter

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/incr"
)

func TestLegacyReuseLookaheadInvalidatesEarlierChildren(t *testing.T) {
	a := newNodeArena(arenaClassIncremental)
	defer a.Release()
	a.legacyReuseReads = incr.NewReads(8)
	a.legacyReuseReads.Record(0, 6) // failed longer token before rollback
	a.legacyReuseReads.Record(2, 3)
	a.legacyReuseReads.Record(6, 9) // EOF was examined
	first := newLeafNodeInArena(a, 1, true, 0, 2, Point{}, Point{Column: 2})
	second := newLeafNodeInArena(a, 1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	tail := newLeafNodeInArena(a, 1, true, 6, 8, Point{Column: 6}, Point{Column: 8})
	root := newLeafNodeInArena(a, 2, true, 0, 8, Point{}, Point{Column: 8})
	root.children = []*Node{first, second, tail}
	a.prepareLegacyReuseDependencies()
	if count, known := legacyReuseLookahead(first); !known || count != 4 {
		t.Fatalf("lookahead=%d known=%t", count, known)
	}
	editNode(root, InputEdit{StartByte: 5, OldEndByte: 6, NewEndByte: 6, StartPoint: Point{Column: 5}, OldEndPoint: Point{Column: 6}, NewEndPoint: Point{Column: 6}})
	if !root.dirty() || !first.dirty() || !second.dirty() || tail.dirty() {
		t.Fatal("changes did not follow the recorded read dependencies")
	}
	if first.endByte != 2 || second.endByte != 3 {
		t.Fatal("lookahead-only invalidation changed visible coordinates")
	}
}

func TestLegacyReuseLookaheadOverflowAndKeywordProvenance(t *testing.T) {
	a := newNodeArena(arenaClassIncremental)
	defer a.Release()
	a.legacyReuseReads = incr.NewReads(3)
	a.legacyReuseReads.Record(0, 4)
	for i := 0; i < len(a.nodes); i++ {
		a.allocNode()
	}
	n := newLeafNodeInArena(a, 1, true, 0, 3, Point{}, Point{Column: 3})
	tok := Token{}
	tok.setLexFlag(tokenFlagKeyword, true)
	noteLegacyReuseLeaf(n, tok)
	a.prepareLegacyReuseDependencies()
	if count, known := legacyReuseLookahead(n); !known || count != 1 {
		t.Fatalf("overflow lookahead=%d known=%t", count, known)
	}
	word := legacyReuseWord(n, false)
	if word == nil || *word&(legacyReuseLeafKnown|legacyReuseKeyword) != legacyReuseLeafKnown|legacyReuseKeyword {
		t.Fatal("sealing lost the leaf's keyword provenance")
	}
	a.resetLegacyReuseDependencies()
	if _, known := legacyReuseLookahead(n); known {
		t.Fatal("arena reset kept a stale reuse certificate")
	}
}

func TestLegacyReuseLookaheadRespectsSidecarBudget(t *testing.T) {
	a := newNodeArena(arenaClassIncremental)
	defer a.Release()
	n := newLeafNodeInArena(a, 1, true, 0, 3, Point{}, Point{Column: 3})
	a.legacyReuseReads = incr.NewReads(3)
	a.legacyReuseReads.Record(0, 4)
	a.setBudget(2)
	before := a.allocatedBytes
	a.prepareLegacyReuseDependencies()
	if a.allocatedBytes != before || len(a.nodeReuseLookahead) != 0 {
		t.Fatal("unfunded dependency sidecar allocated")
	}
	if _, known := legacyReuseLookahead(n); known {
		t.Fatal("missing metadata authenticated a subtree")
	}
}

func TestLegacyReuseChangedSourceAbstainsFromOldReadHistory(t *testing.T) {
	a := newNodeArena(arenaClassIncremental)
	defer a.Release()
	a.legacyReuseReads = incr.NewReads(3)
	a.legacyReuseReads.Record(0, 4)
	n := newLeafNodeInArena(a, 1, true, 0, 3, Point{}, Point{Column: 3})
	a.prepareLegacyReuseDependencies()
	if _, known := legacyReuseLookahead(n); !known {
		t.Fatal("initial parse did not authenticate its read history")
	}
	tree := &Tree{root: n, arena: a}
	tree.abstainLegacyReuseDependencies()
	if _, known := legacyReuseLookahead(n); known {
		t.Fatal("changed source retained the previous parse's scan bound")
	}
	a.resetLegacyReuseDependencies()
	if a.legacyReuseSourceChanged {
		t.Fatal("arena reset did not begin an independent source")
	}
}

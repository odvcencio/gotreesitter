package gotreesitter

import (
	"fmt"
	"testing"
	"unsafe"
)

func TestForestAlternativeIndexCapacityForSource(t *testing.T) {
	const maxCapacity = 1 << 20
	tests := []struct {
		name      string
		sourceLen int
		want      int
	}{
		{name: "empty", sourceLen: 0, want: 1024},
		{name: "below floor", sourceLen: 8191, want: 1024},
		{name: "at floor", sourceLen: 8192, want: 1024},
		{name: "above floor", sourceLen: 8200, want: 1025},
		{name: "at ceiling", sourceLen: maxCapacity * 8, want: maxCapacity},
		{name: "above ceiling", sourceLen: maxCapacity*8 + 8, want: maxCapacity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forestAlternativeIndexCapacityForSource(tt.sourceLen); got != tt.want {
				t.Fatalf("forestAlternativeIndexCapacityForSource(%d) = %d, want %d", tt.sourceLen, got, tt.want)
			}
		})
	}
}

func TestForestAlternativeIndexInlineTierBelowCapacity(t *testing.T) {
	index := newForestAlternativeIndex(1024)
	for i := 0; i < forestAlternativeIndexInlineCapacity; i++ {
		node := &Node{startByte: uint32(i % 3)}
		index.setNode(node, &gssForestNode{state: StateID(i + 1)})
		index.setSlot(forestAlternativeSlotKey{parent: node, childIndex: i}, forestAlternativeSlot{node: &gssForestNode{state: StateID(i + 2)}})
	}
	if index.promoted || index.nodes != nil || index.slots != nil || index.byStart != nil {
		t.Fatal("inline alternative index allocated maps at capacity")
	}
	if got := len(forestAlternativeCandidatesStartingAt(index, 1)); got != 5 {
		t.Fatalf("inline by-start candidate count = %d, want 5", got)
	}
}

func TestForestAlternativeIndexPromotionPreservesEntries(t *testing.T) {
	index := newForestAlternativeIndex(1024)
	var first *Node
	var firstSlotKey forestAlternativeSlotKey
	var firstSlot forestAlternativeSlot
	for i := 0; i < forestAlternativeIndexInlineCapacity; i++ {
		node := &Node{startByte: uint32(i)}
		value := &gssForestNode{state: StateID(i + 1)}
		slotKey := forestAlternativeSlotKey{parent: node, childIndex: i}
		slot := forestAlternativeSlot{node: value, prev: &gssForestNode{state: StateID(i)}}
		if i == 0 {
			first, firstSlotKey, firstSlot = node, slotKey, slot
		}
		index.setNode(node, value)
		index.setSlot(slotKey, slot)
	}
	extra := &Node{startByte: 99}
	index.setNode(extra, &gssForestNode{state: 99})
	if !index.promoted {
		t.Fatal("alternative index did not promote on entry 17")
	}
	if index.node(first) == nil || index.node(extra) == nil {
		t.Fatal("promotion lost node entries")
	}
	if got, ok := index.slot(firstSlotKey); !ok || got != firstSlot {
		t.Fatalf("promotion lost slot entry: got %#v, %v want %#v", got, ok, firstSlot)
	}
	if got := forestAlternativeCandidatesStartingAt(index, first.startByte); len(got) != 1 || got[0] != first {
		t.Fatalf("promotion lost by-start entry: %#v", got)
	}
	if index.inlineNodeCount != 0 || index.inlineSlotCount != 0 {
		t.Fatalf("promotion retained inline counts: nodes=%d slots=%d", index.inlineNodeCount, index.inlineSlotCount)
	}
}

func TestForestAlternativeIndexSlotPromotionRunsOnce(t *testing.T) {
	index := newForestAlternativeIndex(1024)
	for i := 0; i <= forestAlternativeIndexInlineCapacity; i++ {
		index.setSlot(forestAlternativeSlotKey{parent: &Node{}, childIndex: i}, forestAlternativeSlot{node: &gssForestNode{state: StateID(i + 1)}})
	}
	if !index.promoted || len(index.slots) != forestAlternativeIndexInlineCapacity+1 {
		t.Fatalf("slot promotion state: promoted=%v slots=%d", index.promoted, len(index.slots))
	}
	if index.promote() {
		t.Fatal("alternative index promoted more than once")
	}
}

func TestForestAlternativeIndexInlineLayoutBounded(t *testing.T) {
	if got, maxSize := unsafe.Sizeof(forestAlternativeIndex{}), uintptr(1024); got > maxSize {
		t.Fatalf("forest alternative inline index size = %d, exceeds %d", got, maxSize)
	}
}

// pathsOf reduces childCount children over node and returns each visited path as
// "child-states|popToState" so order and fan-out are easy to assert.
func pathsOf(node *gssForestNode, childCount int) []string {
	var out []string
	reduceOverForest(node, childCount, func(children []stackEntry, _ int, popTo *gssForestNode) {
		states := make([]uint32, len(children))
		for i, c := range children {
			states[i] = uint32(c.state)
		}
		out = append(out, fmt.Sprintf("%v|%d", states, popTo.state))
	})
	return out
}

func TestReduceOverForestLinearChain(t *testing.T) {
	// n0 <-(a:10)- n1 <-(b:11)- n2 <-(c:12)- n3
	n0 := &gssForestNode{state: 0, byteOffset: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 10}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{{prev: n1, subtree: stackEntry{state: 11}}}}
	n3 := &gssForestNode{state: 3, links: []gssLink{{prev: n2, subtree: stackEntry{state: 12}}}}

	// reduce 2 children over n3 → [b,c] = [11 12], pop back to n1.
	got := pathsOf(n3, 2)
	want := []string{"[11 12]|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("childCount=2: got %v want %v", got, want)
	}
	// reduce 0 children → empty, pop to n3 itself.
	if got := pathsOf(n3, 0); fmt.Sprint(got) != "[[]|3]" {
		t.Fatalf("childCount=0: got %v", got)
	}
	// reduce all 3 → [a b c] = [10 11 12], pop to n0.
	if got := pathsOf(n3, 3); fmt.Sprint(got) != "[[10 11 12]|0]" {
		t.Fatalf("childCount=3: got %v", got)
	}
}

func TestReduceOverForestLinearChainWithExtra(t *testing.T) {
	// n0 <-(a:10)- n1 <-(b:11)- n2 <-(extra:90)- n3 <-(c:12)- n4
	extra := &Node{}
	extra.setExtra(true)
	n0 := &gssForestNode{state: 0, byteOffset: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 10}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{{prev: n1, subtree: stackEntry{state: 11}}}}
	n3 := &gssForestNode{state: 3, links: []gssLink{{prev: n2, subtree: newStackEntryNode(90, extra)}}}
	n4 := &gssForestNode{state: 4, links: []gssLink{{prev: n3, subtree: stackEntry{state: 12}}}}

	// Extras are included in the reduce window but do not count toward childCount.
	got := pathsOf(n4, 2)
	want := []string{"[11 90 12]|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("childCount=2 with extra: got %v want %v", got, want)
	}
}

func TestReduceOverForestForkedNode(t *testing.T) {
	// Shared base n0 <-(a:10)- n1, then two alternatives reaching a coalesced n3:
	//   path A: n1 <-(b:11)- n2  <-(c:12)- n3
	//   path B: n1 <-(x:21)- n2a <-(y:22)- n3
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 10}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{{prev: n1, subtree: stackEntry{state: 11}}}}
	n2a := &gssForestNode{state: 20, links: []gssLink{{prev: n1, subtree: stackEntry{state: 21}}}}
	n3 := &gssForestNode{state: 3, links: []gssLink{
		{prev: n2, subtree: stackEntry{state: 12}},
		{prev: n2a, subtree: stackEntry{state: 22}},
	}}

	// reduce 2 children over the coalesced n3 → BOTH alternatives, each popping to n1.
	got := pathsOf(n3, 2)
	want := []string{"[11 12]|1", "[21 22]|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("forked childCount=2: got %v want %v", got, want)
	}
	if got, want := pathsOf(n3, 1), []string{"[12]|2", "[22]|20"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("forked childCount=1: got %v want %v", got, want)
	}
}

func TestReduceOverForestNestedForkNoExtras(t *testing.T) {
	// Two no-extra fork levels:
	//   n0 <-(a:10 or b:20)- n1 <-(c:11 or d:21)- n2
	// The reducer should enumerate the cartesian product in left-to-right order.
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{
		{prev: n0, subtree: stackEntry{state: 10}},
		{prev: n0, subtree: stackEntry{state: 20}},
	}, noExtraDepth: 1}
	n2 := &gssForestNode{state: 2, links: []gssLink{
		{prev: n1, subtree: stackEntry{state: 11}},
		{prev: n1, subtree: stackEntry{state: 21}},
	}, noExtraDepth: 2}

	got := pathsOf(n2, 2)
	want := []string{"[10 11]|0", "[20 11]|0", "[10 21]|0", "[20 21]|0"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("nested fork no-extra childCount=2: got %v want %v", got, want)
	}
}

func TestReduceOverForestForkedTopWithForkedPredecessorChildCount2(t *testing.T) {
	// A repeated-list reduction often pops [previous-list, new-item]. When the
	// new item has multiple raw-distinct alternatives, each top link can point
	// at a predecessor that is itself forked. childCount=2 must enumerate those
	// predecessor alternatives, not only the first predecessor link.
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{
		{prev: n0, subtree: stackEntry{state: 10}},
		{prev: n0, subtree: stackEntry{state: 20}},
	}, noExtraDepth: 1}
	n2 := &gssForestNode{state: 2, links: []gssLink{
		{prev: n1, subtree: stackEntry{state: 11}},
		{prev: n1, subtree: stackEntry{state: 21}},
	}, noExtraDepth: 2}

	got := pathsOf(n2, 2)
	want := []string{"[10 11]|0", "[20 11]|0", "[10 21]|0", "[20 21]|0"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("forked top with forked predecessor childCount=2: got %v want %v", got, want)
	}
}

func TestReduceOverForestVisitorMutationDoesNotCorruptEnumeration(t *testing.T) {
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{
		{prev: n0, subtree: stackEntry{state: 10}},
		{prev: n0, subtree: stackEntry{state: 20}},
	}, noExtraDepth: 1}
	n2 := &gssForestNode{state: 2, links: []gssLink{
		{prev: n1, subtree: stackEntry{state: 11}},
		{prev: n1, subtree: stackEntry{state: 21}},
	}, noExtraDepth: 2}

	var out []string
	mutated := false
	reduceOverForest(n2, 2, func(children []stackEntry, _ int, popTo *gssForestNode) {
		states := make([]uint32, len(children))
		for i, c := range children {
			states[i] = uint32(c.state)
		}
		out = append(out, fmt.Sprintf("%v|%d", states, popTo.state))
		if !mutated {
			mutated = true
			n1.links[1].subtree = stackEntry{state: 88}
			n2.links[1].subtree = stackEntry{state: 99}
		}
	})

	want := []string{"[10 11]|0", "[20 11]|0", "[10 21]|0", "[20 21]|0"}
	if fmt.Sprint(out) != fmt.Sprint(want) {
		t.Fatalf("visitor mutation corrupted enumeration: got %v want %v", out, want)
	}
}

func TestReduceOverForestNestedForkNoExtrasChildCount4(t *testing.T) {
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{
		{prev: n0, subtree: stackEntry{state: 10}},
		{prev: n0, subtree: stackEntry{state: 20}},
	}, noExtraDepth: 1}
	n2 := &gssForestNode{state: 2, links: []gssLink{
		{prev: n1, subtree: stackEntry{state: 11}},
		{prev: n1, subtree: stackEntry{state: 21}},
	}, noExtraDepth: 2}
	n3 := &gssForestNode{state: 3, links: []gssLink{
		{prev: n2, subtree: stackEntry{state: 12}},
		{prev: n2, subtree: stackEntry{state: 22}},
	}, noExtraDepth: 3}
	n4 := &gssForestNode{state: 4, links: []gssLink{
		{prev: n3, subtree: stackEntry{state: 13}},
		{prev: n3, subtree: stackEntry{state: 23}},
	}, noExtraDepth: 4}

	got := pathsOf(n4, 4)
	want := []string{
		"[10 11 12 13]|0", "[20 11 12 13]|0",
		"[10 21 12 13]|0", "[20 21 12 13]|0",
		"[10 11 22 13]|0", "[20 11 22 13]|0",
		"[10 21 22 13]|0", "[20 21 22 13]|0",
		"[10 11 12 23]|0", "[20 11 12 23]|0",
		"[10 21 12 23]|0", "[20 21 12 23]|0",
		"[10 11 22 23]|0", "[20 11 22 23]|0",
		"[10 21 22 23]|0", "[20 21 22 23]|0",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("nested fork no-extra childCount=4: got %v want %v", got, want)
	}
}

func TestReduceOverForestForkedLinearWithExtra(t *testing.T) {
	extra := &Node{}
	extra.setExtra(true)
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 10}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{{prev: n1, subtree: stackEntry{state: 11}}}}
	n3 := &gssForestNode{state: 3, links: []gssLink{{prev: n2, subtree: newStackEntryNode(90, extra)}}}
	n2a := &gssForestNode{state: 20, links: []gssLink{{prev: n1, subtree: stackEntry{state: 21}}}}
	n4 := &gssForestNode{state: 4, links: []gssLink{
		{prev: n3, subtree: stackEntry{state: 12}},
		{prev: n2a, subtree: stackEntry{state: 22}},
	}}

	got := pathsOf(n4, 2)
	want := []string{"[11 90 12]|1", "[21 22]|1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("forked linear with extra: got %v want %v", got, want)
	}
}

func TestReduceOverForestLinearPrefixForkedWithExtra(t *testing.T) {
	extra := &Node{}
	extra.setExtra(true)
	n0 := &gssForestNode{state: 0}
	n1 := &gssForestNode{state: 1, links: []gssLink{{prev: n0, subtree: stackEntry{state: 11}}}}
	n2 := &gssForestNode{state: 2, links: []gssLink{
		{prev: n1, subtree: newStackEntryNode(90, extra)},
		{prev: n0, subtree: stackEntry{state: 21}},
	}}
	n3 := &gssForestNode{state: 3, links: []gssLink{{prev: n2, subtree: stackEntry{state: 31}}}}

	got := pathsOf(n3, 2)
	want := []string{"[11 90 31]|0", "[21 31]|0"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("linear prefix forked with extra: got %v want %v", got, want)
	}
}

// TestCoalesceForestSharesNode proves coalesceForest dedups by (state,byteOffset)
// into one node and grows its link storage only when a second path arrives.
func TestCoalesceForestSharesNode(t *testing.T) {
	const linkCap = 2
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	base := &gssForestNode{state: 0}
	// Two distinct parses reach (state=5, byteOffset=42).
	a := coalesceForest(&idx, slab, 5, 42, base, stackEntry{state: 100}, 3, 0, linkCap)
	if got := cap(a.links); got != 1 {
		t.Fatalf("first link capacity = %d, want 1", got)
	}
	b := coalesceForest(&idx, slab, 5, 42, base, stackEntry{state: 101}, 7, 0, linkCap)
	if a != b {
		t.Fatal("coalesceForest created two nodes for the same (state,byteOffset)")
	}
	if len(a.links) != 2 {
		t.Fatalf("want 2 links on the coalesced node, got %d", len(a.links))
	}
	if got := cap(a.links); got != linkCap {
		t.Fatalf("promoted link capacity = %d, want %d", got, linkCap)
	}
	if a.noExtraDepth != 1 {
		t.Fatalf("want no-extra depth 1, got %d", a.noExtraDepth)
	}
	if a.minLinkScore != 3 {
		t.Fatalf("want min link score 3, got %d", a.minLinkScore)
	}
	if best := a.bestLink(); best == nil || best.score != 7 {
		t.Fatalf("want best link score 7, got %v", best)
	}
	// A different (state,byteOffset) is a separate node.
	c := coalesceForest(&idx, slab, 6, 42, base, stackEntry{state: 102}, 1, 0, linkCap)
	if c == a {
		t.Fatal("distinct (state,byteOffset) coalesced into the same node")
	}
}

func TestGSSForestIndexUsesInlineTierAndPreservesEntriesOnSpill(t *testing.T) {
	var idx gssForestIndex
	idx.init(len(idx.inline))
	if idx.spill != nil {
		t.Fatal("small index allocated spill storage during initialization")
	}
	for i := range idx.inline {
		key := gssForestKey{state: StateID(i + 1), byteOffset: uint32(i)}
		idx.set(key, &gssForestNode{state: key.state, byteOffset: key.byteOffset})
	}
	if got := idx.len(); got != len(idx.inline) {
		t.Fatalf("inline index length = %d, want %d", got, len(idx.inline))
	}

	spillKey := gssForestKey{state: 99, byteOffset: 99}
	spillNode := &gssForestNode{state: spillKey.state, byteOffset: spillKey.byteOffset}
	idx.set(spillKey, spillNode)
	if idx.spill == nil {
		t.Fatal("index did not spill after exhausting inline storage")
	}
	for i := range idx.inline {
		key := gssForestKey{state: StateID(i + 1), byteOffset: uint32(i)}
		if got := idx.lookup(key); got == nil || got.state != key.state || got.byteOffset != key.byteOffset {
			t.Fatalf("lookup after spill for %+v = %+v", key, got)
		}
	}
	if got := idx.lookup(spillKey); got != spillNode {
		t.Fatalf("spill lookup = %p, want %p", got, spillNode)
	}
}

func TestCoalesceForestRefreshesMinLinkScoreOnReplacement(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	base := &gssForestNode{state: 0}
	loser := newStackEntryNode(100, &Node{symbol: 100, startByte: 1, endByte: 4})
	winner := newStackEntryNode(100, &Node{symbol: 100, startByte: 1, endByte: 4})

	node := coalesceForest(&idx, slab, 5, 4, base, loser, 3, 0, forestMaxLinksPerNode)
	initialDirty := node.dirty
	again := coalesceForest(&idx, slab, 5, 4, base, winner, 7, 0, forestMaxLinksPerNode)
	if again != node {
		t.Fatal("replacement reached a different coalesced node")
	}
	if len(node.links) != 1 {
		t.Fatalf("replacement appended duplicate link: got %d links, want 1", len(node.links))
	}
	if node.minLinkScore != 7 {
		t.Fatalf("want refreshed min link score 7, got %d", node.minLinkScore)
	}
	if best := node.bestLink(); best == nil || best.score != 7 {
		t.Fatalf("want best link score 7 after replacement, got %v", best)
	}
	if node.dirty <= initialDirty {
		t.Fatalf("replacement dirty=%d, want > %d", node.dirty, initialDirty)
	}
}

func TestCoalesceForestKeepsEqualScoreRawDistinctLinks(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	p := &Parser{}
	base := &gssForestNode{state: 0}
	leftChild := newLeafNodeInArena(arena, 11, true, 1, 2, Point{}, Point{})
	rightChild := newLeafNodeInArena(arena, 10, true, 1, 2, Point{}, Point{})
	left := newParentNodeInArena(arena, 100, true, []*Node{leftChild}, nil, 0)
	right := newParentNodeInArena(arena, 100, true, []*Node{rightChild}, nil, 0)
	left.rawShape = p.captureRawShape(nil, arena, 100, 1, []stackEntry{newStackEntryNode(11, leftChild)}, 0, 1)
	right.rawShape = p.captureRawShape(nil, arena, 100, 2, []stackEntry{newStackEntryNode(10, rightChild)}, 0, 1)

	node := coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, base, newStackEntryNode(100, left), 3, 0)
	again := coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, base, newStackEntryNode(100, right), 3, 0)
	if again != node {
		t.Fatal("raw-distinct alternative reached a different coalesced node")
	}
	if len(node.links) != 2 {
		t.Fatalf("raw-distinct equal-score links = %d, want 2", len(node.links))
	}
	if best := node.bestResultLink(p, arena); best == nil || stackEntryNode(best.subtree) != right {
		t.Fatalf("best raw-distinct link = %v, want second raw-preferred link", best)
	}
}

func TestCoalesceForestKeepsEqualScoreOneSidedRawUnknownLinks(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	p := &Parser{}
	base := &gssForestNode{state: 0}
	leftChild := newLeafNodeInArena(arena, 11, true, 1, 2, Point{}, Point{})
	rightChild := newLeafNodeInArena(arena, 11, true, 1, 2, Point{}, Point{})
	left := newParentNodeInArena(arena, 100, true, []*Node{leftChild}, nil, 0)
	right := newParentNodeInArena(arena, 100, true, []*Node{rightChild}, nil, 0)
	left.rawShape = p.captureRawShape(nil, arena, 100, 1, []stackEntry{newStackEntryNode(11, leftChild)}, 0, 1)

	node := coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, base, newStackEntryNode(100, left), 3, 0)
	again := coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, base, newStackEntryNode(100, right), 3, 0)
	if again != node {
		t.Fatal("one-sided raw alternative reached a different coalesced node")
	}
	if len(node.links) != 2 {
		t.Fatalf("one-sided raw equal-score links = %d, want 2", len(node.links))
	}
}

func TestCoalesceForestCapPreservesRawDistinctCandidateOverDuplicateBucket(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	p := &Parser{}
	makeEntry := func(childSym Symbol, prod uint16) stackEntry {
		child := newLeafNodeInArena(arena, childSym, true, 1, 2, Point{}, Point{})
		parent := newParentNodeInArena(arena, 100, true, []*Node{child}, nil, 0)
		parent.rawShape = p.captureRawShape(nil, arena, 100, prod, []stackEntry{newStackEntryNode(StateID(childSym), child)}, 0, 1)
		return newStackEntryNode(100, parent)
	}

	for i := 0; i < forestMaxLinksPerNode-1; i++ {
		prev := &gssForestNode{state: StateID(i + 1)}
		coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, prev, makeEntry(Symbol(10+i), uint16(i+1)), 10, 0)
	}
	duplicatePrev := &gssForestNode{state: 99}
	duplicate := makeEntry(10, 1)
	node := coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, duplicatePrev, duplicate, 9, 0)
	if len(node.links) != forestMaxLinksPerNode {
		t.Fatalf("setup links = %d, want %d", len(node.links), forestMaxLinksPerNode)
	}

	candidatePrev := &gssForestNode{state: 100}
	candidate := makeEntry(99, 99)
	node = coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, candidatePrev, candidate, 0, 0)
	if len(node.links) != forestMaxLinksPerNode {
		t.Fatalf("links after cap = %d, want %d", len(node.links), forestMaxLinksPerNode)
	}
	foundCandidate := false
	duplicateBucketCount := 0
	for i := range node.links {
		if forestRawStackEntriesExactEqual(arena, node.links[i].subtree, candidate) == forestRawEqual {
			foundCandidate = true
		}
		if forestRawStackEntriesExactEqual(arena, node.links[i].subtree, duplicate) == forestRawEqual {
			duplicateBucketCount++
		}
	}
	if !foundCandidate {
		t.Fatal("raw-distinct cap candidate was dropped instead of replacing a duplicate bucket")
	}
	if duplicateBucketCount != 1 {
		t.Fatalf("duplicate raw bucket count = %d, want 1", duplicateBucketCount)
	}
}

func TestForestCoalesceWouldDropForCapDefersUntilRawShapeExists(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	node := &gssForestNode{
		state:        5,
		byteOffset:   4,
		errorCost:    0,
		minLinkScore: 10,
	}
	for i := 0; i < forestMaxLinksPerNode; i++ {
		node.links = append(node.links, gssLink{
			prev:      &gssForestNode{state: StateID(i + 1)},
			subtree:   stackEntry{state: StateID(100 + i)},
			score:     10,
			errorCost: 0,
		})
	}
	idx.set(gssForestKey{state: 5, byteOffset: 4}, node)

	if forestCoalesceWouldDropForCap(&idx, 5, 4, 0, 0, forestMaxLinksPerNode) {
		t.Fatal("pre-cap guard dropped a candidate before raw-shape-aware cap replacement could run")
	}
}

func TestCoalesceForestCapDoesNotLetLowerRankSameRawBucketEvictDistinctBucket(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	p := &Parser{}
	makeEntry := func(childSym Symbol, prod uint16) stackEntry {
		child := newLeafNodeInArena(arena, childSym, true, 1, 2, Point{}, Point{})
		parent := newParentNodeInArena(arena, 100, true, []*Node{child}, nil, 0)
		parent.rawShape = p.captureRawShape(nil, arena, 100, prod, []stackEntry{newStackEntryNode(StateID(childSym), child)}, 0, 1)
		return newStackEntryNode(100, parent)
	}

	for i := 0; i < forestMaxLinksPerNode; i++ {
		prev := &gssForestNode{state: StateID(i + 1)}
		coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, prev, makeEntry(Symbol(10+i), uint16(i+1)), 10, 0)
	}
	candidatePrev := &gssForestNode{state: 100}
	candidate := makeEntry(10, 1)
	node := coalesceForestWithRaw(p, arena, &idx, slab, 5, 4, candidatePrev, candidate, 0, 0)

	count := 0
	for i := range node.links {
		if forestRawStackEntriesExactEqual(arena, node.links[i].subtree, candidate) == forestRawEqual {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("same raw bucket count = %d, want existing representative only", count)
	}
}

func TestCollectForestErrorRootUsesResultLinkErrorCost(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	highScoreHighError := newLeafNodeInArena(arena, 100, true, 1, 4, Point{}, Point{Column: 3})
	lowScoreLowError := newLeafNodeInArena(arena, 101, true, 1, 4, Point{}, Point{Column: 3})
	start := &gssForestNode{state: 0}
	partial := &gssForestNode{
		state:      5,
		byteOffset: 4,
		errorCost:  2,
		links: []gssLink{
			{prev: start, subtree: newStackEntryNode(100, highScoreHighError), score: 9, errorCost: 8},
			{prev: start, subtree: newStackEntryNode(101, lowScoreLowError), score: 1, errorCost: 2},
		},
	}
	var idx gssForestIndex
	idx.init(1)
	idx.set(gssForestKey{state: partial.state, byteOffset: partial.byteOffset}, partial)

	parser := &Parser{rootSymbol: 1}
	root := parser.collectForestErrorRoot(&idx, arena)
	if root == nil || len(root.children) != 1 {
		t.Fatalf("root children = %v, want one lower-error fragment", root)
	}
	if root.children[0] != lowScoreLowError {
		t.Fatalf("error root fragment = %p, want lower-error fragment %p", root.children[0], lowScoreLowError)
	}
}

func TestForestResultLinkLowerErrorCostBeatsHigherScore(t *testing.T) {
	lowError := &Node{symbol: 100, startByte: 1, endByte: 4, dynamicPrecedence: 1}
	highError := &Node{symbol: 101, startByte: 1, endByte: 4, dynamicPrecedence: 9}
	node := &gssForestNode{
		state:      5,
		byteOffset: 4,
		links: []gssLink{
			{subtree: newStackEntryNode(100, highError), score: 9, errorCost: 8},
			{subtree: newStackEntryNode(101, lowError), score: 1, errorCost: 2},
		},
	}

	best := node.bestResultLink(&Parser{}, nil)
	if best == nil || stackEntryNode(best.subtree) != lowError {
		t.Fatalf("best link = %v, want lower error-cost link", best)
	}
}

func TestForestResultLinkUsesCumulativeDynamicPrecedenceBeforeMaterializedShape(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	lang := &Language{
		Name:        "forest-cumulative-dynamic-test",
		SymbolNames: []string{"EOF", "root", "replacement_guard_and", "concatables", "var", "string"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "root", Visible: true, Named: true},
			{Name: "replacement_guard_and", Visible: true, Named: true},
			{Name: "concatables", Visible: true, Named: true},
			{Name: "var", Visible: true, Named: true},
			{Name: "string", Visible: true, Named: true},
		},
	}
	parser := &Parser{language: lang, hasRootSymbol: true, rootSymbol: 1}

	varNode := newLeafNodeInArena(arena, 4, true, 0, 3, Point{}, Point{Column: 3})
	stringNode := newLeafNodeInArena(arena, 5, true, 4, 9, Point{Column: 4}, Point{Column: 9})
	direct := newParentNodeInArena(arena, 3, true, []*Node{varNode, stringNode}, nil, 0)
	direct.rawShape = parser.captureRawShape(nil, arena, 3, 1, []stackEntry{
		newStackEntryNode(4, varNode),
		newStackEntryNode(5, stringNode),
	}, 0, 2)

	wrappedVar := newLeafNodeInArena(arena, 4, true, 0, 3, Point{}, Point{Column: 3})
	wrappedString := newLeafNodeInArena(arena, 5, true, 4, 9, Point{Column: 4}, Point{Column: 9})
	wrappedConcat := newParentNodeInArena(arena, 3, true, []*Node{wrappedVar, wrappedString}, nil, 0)
	wrapper := newParentNodeInArena(arena, 2, true, []*Node{wrappedConcat}, nil, 0)
	wrapper.dynamicPrecedence = 4
	wrapper.rawShape = parser.captureRawShape(nil, arena, 2, 2, []stackEntry{newStackEntryNode(3, wrappedConcat)}, 0, 1)

	node := &gssForestNode{
		state:      7,
		byteOffset: 9,
		links: []gssLink{
			{subtree: newStackEntryNode(2, wrapper), score: 4, errorCost: 0},
			{subtree: newStackEntryNode(3, direct), score: 6, errorCost: 0},
		},
	}

	best := node.bestResultLink(parser, arena)
	if best == nil || stackEntryNode(best.subtree) != direct {
		t.Fatalf("best link = %v, want cumulative-dynamic direct concatables", best)
	}
}

func TestCoalesceForestMarksDirtyWhenPredecessorChanges(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	prev := &gssForestNode{state: 1, dirty: 1}
	entry := newStackEntryNode(2, &Node{symbol: 7, startByte: 10, endByte: 20})

	top := coalesceForest(&idx, slab, 5, 20, prev, entry, 0, 0, forestMaxLinksPerNode)
	initialDirty := top.dirty
	initialLinks := len(top.links)

	prev.dirty++
	again := coalesceForest(&idx, slab, 5, 20, prev, entry, 0, 0, forestMaxLinksPerNode)
	if again != top {
		t.Fatal("same link reached a different coalesced node")
	}
	if len(top.links) != initialLinks {
		t.Fatalf("duplicate link appended: got %d links, want %d", len(top.links), initialLinks)
	}
	if top.dirty <= initialDirty {
		t.Fatalf("coalesced node dirty=%d, want > %d after predecessor changed", top.dirty, initialDirty)
	}
}

func TestGSSForestIndexLookupCacheClearsOnReset(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	key := gssForestKey{state: 7, byteOffset: 11}
	node := &gssForestNode{state: 7, byteOffset: 11}

	idx.set(key, node)
	if got := idx.lookup(key); got != node {
		t.Fatalf("cached lookup returned %p, want %p", got, node)
	}
	idx.reset()
	if got := idx.lookup(key); got != nil {
		t.Fatalf("lookup after reset returned stale node %p", got)
	}
}

func TestGSSForestNodeSlabReleaseClearsPointers(t *testing.T) {
	var index gssForestIndex
	index.init(0)
	slab := &gssForestNodeSlab{}
	base := &gssForestNode{state: 1}
	secondBase := &gssForestNode{state: 2}
	coalesceForest(&index, slab, 2, 1, base, stackEntry{state: 3}, 0, 0, forestMaxLinksPerNode)
	nodeLinkStart := slab.linkIdx - 1
	coalesceForest(&index, slab, 2, 1, secondBase, stackEntry{state: 4}, 0, 0, forestMaxLinksPerNode)
	promotedLinkStart := slab.linkIdx - forestMaxLinksPerNode

	if len(slab.nodeBatches) == 0 || len(slab.linkBatches) == 0 {
		t.Fatal("expected slab batches to be allocated")
	}
	if got := slab.linkBatches[0][nodeLinkStart].prev; got != base {
		t.Fatalf("test setup failed: link batch prev = %p, want %p", got, base)
	}
	if got := slab.linkBatches[0][promotedLinkStart+1].prev; got != secondBase {
		t.Fatalf("test setup failed: promoted link prev = %p, want %p", got, secondBase)
	}
	slab.resetForRelease()
	if got := slab.nodeBatches[0][0].links; got != nil {
		t.Fatalf("node batch retained stale links slice: %v", got)
	}
	if got := slab.linkBatches[0][nodeLinkStart].prev; got != nil {
		t.Fatalf("link batch retained stale prev pointer: %p", got)
	}
	if got := slab.linkBatches[0][promotedLinkStart+1].prev; got != nil {
		t.Fatalf("promoted link batch retained stale prev pointer: %p", got)
	}
}

func TestGSSForestNodeSlabTrimClearsDiscardedBatchReferences(t *testing.T) {
	nodeBatches := [][]gssForestNode{
		make([]gssForestNode, 1),
		make([]gssForestNode, 1),
		make([]gssForestNode, 1),
	}
	linkBatches := [][]gssLink{
		make([]gssLink, forestMaxLinksPerNode),
		make([]gssLink, forestMaxLinksPerNode),
		make([]gssLink, forestMaxLinksPerNode),
	}
	slab := &gssForestNodeSlab{
		nodeBatches: nodeBatches,
		linkBatches: linkBatches,
	}
	retainedNode := &nodeBatches[0][0]
	retainedLink := &linkBatches[0][0]
	limit := cap(nodeBatches[0])*int(unsafe.Sizeof(gssForestNode{})) +
		cap(linkBatches[0])*int(unsafe.Sizeof(gssLink{}))

	slab.trimToRetentionLimit(limit)
	if got := len(slab.nodeBatches); got != 1 {
		t.Fatalf("retained node batches = %d, want 1", got)
	}
	if got := len(slab.linkBatches); got != 1 {
		t.Fatalf("retained link batches = %d, want 1", got)
	}
	if got := slab.retainedBytes(); got != limit {
		t.Fatalf("retained bytes = %d, want %d", got, limit)
	}
	if got := &slab.nodeBatches[0][0]; got != retainedNode {
		t.Fatalf("retained node batch moved: got %p, want %p", got, retainedNode)
	}
	if got := &slab.linkBatches[0][0]; got != retainedLink {
		t.Fatalf("retained link batch moved: got %p, want %p", got, retainedLink)
	}
	for i, batch := range slab.nodeBatches[:cap(slab.nodeBatches)] {
		if i >= len(slab.nodeBatches) && batch != nil {
			t.Fatalf("discarded node batch %d remains reachable", i)
		}
	}
	for i, batch := range slab.linkBatches[:cap(slab.linkBatches)] {
		if i >= len(slab.linkBatches) && batch != nil {
			t.Fatalf("discarded link batch %d remains reachable", i)
		}
	}

	if got := slab.alloc(7, 11, 0, 0); got != retainedNode {
		t.Fatalf("first allocation after trim = %p, want retained node %p", got, retainedNode)
	}
	if got := &slab.linkBatches[0][0]; got != retainedLink {
		t.Fatalf("first link allocation after trim moved: got %p, want %p", got, retainedLink)
	}

	// Exhaust the retained prefix once, then prove append reuses the cleared
	// outer slots and a second release can clear those references again.
	slab.nodeIdx = len(slab.nodeBatches[0])
	slab.linkIdx = len(slab.linkBatches[0])
	slab.alloc(8, 12, 0, 0)
	if len(slab.nodeBatches) != 2 {
		t.Fatalf("node batch count after reuse = %d, want 2", len(slab.nodeBatches))
	}
	if slab.nodeBatches[1] == nil {
		t.Fatal("node batch append did not reuse a cleared outer slot")
	}
	if len(slab.linkBatches) != 2 {
		t.Fatalf("link batch count after reuse = %d, want 2", len(slab.linkBatches))
	}
	if slab.linkBatches[1] == nil {
		t.Fatal("link batch append did not reuse a cleared outer slot")
	}
	slab.resetForRelease()
	slab.trimToRetentionLimit(limit)
	if slab.nodeBatches[:cap(slab.nodeBatches)][1] != nil {
		t.Fatal("reused node batch remains reachable after second trim")
	}
	if slab.linkBatches[:cap(slab.linkBatches)][1] != nil {
		t.Fatal("reused link batch remains reachable after second trim")
	}
}

func TestGSSForestNodeSlabTrimLimitBoundaries(t *testing.T) {
	var empty gssForestNodeSlab
	empty.trimToRetentionLimit(0)
	if len(empty.nodeBatches) != 0 || len(empty.linkBatches) != 0 {
		t.Fatalf("empty trim created batches: nodes=%d links=%d", len(empty.nodeBatches), len(empty.linkBatches))
	}

	slab := &gssForestNodeSlab{
		nodeBatches: [][]gssForestNode{
			make([]gssForestNode, 1),
			make([]gssForestNode, 1),
		},
		linkBatches: [][]gssLink{
			make([]gssLink, 1),
			make([]gssLink, 1),
		},
	}
	exact := slab.retainedBytes()
	slab.trimToRetentionLimit(exact)
	if len(slab.nodeBatches) != 2 || len(slab.linkBatches) != 2 {
		t.Fatalf("exact-limit trim changed batches: nodes=%d links=%d", len(slab.nodeBatches), len(slab.linkBatches))
	}

	slab.trimToRetentionLimit(exact - 1)
	if len(slab.nodeBatches) != 2 || len(slab.linkBatches) != 1 {
		t.Fatalf("over-limit trim removed wrong suffix: nodes=%d links=%d", len(slab.nodeBatches), len(slab.linkBatches))
	}
	if slab.linkBatches[:cap(slab.linkBatches)][1] != nil {
		t.Fatal("over-limit link suffix remains reachable")
	}
}

func TestForestRootMustDeclineErrorRoot(t *testing.T) {
	if forestRootMustDecline(nil) {
		t.Fatal("nil root should not decline")
	}
	errorRoot := &Node{symbol: errorSymbol}
	if !forestRootMustDecline(errorRoot) {
		t.Fatal("ERROR root should decline")
	}
	errorBearingRoot := &Node{symbol: 2}
	errorBearingRoot.setHasError(true)
	if forestRootMustDecline(errorBearingRoot) {
		t.Fatal("non-ERROR root with errors should stay governed by recovery policy")
	}
}

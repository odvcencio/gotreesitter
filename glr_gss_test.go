package gotreesitter

import "testing"

func TestGSSStackPushCloneAndTruncate(t *testing.T) {
	var scratch gssScratch
	base := newGSSStack(1, &scratch)
	if base.len() != 1 {
		t.Fatalf("base len = %d, want 1", base.len())
	}

	clone := base.clone()
	base.push(2, nil, &scratch)
	base.push(3, nil, &scratch)

	if base.len() != 3 {
		t.Fatalf("base len after pushes = %d, want 3", base.len())
	}
	if clone.len() != 1 {
		t.Fatalf("clone len changed = %d, want 1", clone.len())
	}
	if base.top().state != 3 {
		t.Fatalf("base top state = %d, want 3", base.top().state)
	}

	ok := base.truncate(2)
	if !ok {
		t.Fatal("truncate(2) = false, want true")
	}
	if got := base.top().state; got != 2 {
		t.Fatalf("top after truncate = %d, want 2", got)
	}
}

func TestGSSStackMaterializeAndByteOffset(t *testing.T) {
	var scratch gssScratch
	n1 := &Node{endByte: 5}
	n2 := &Node{endByte: 9}

	var s gssStack
	s.push(1, nil, &scratch)
	s.push(2, n1, &scratch)
	s.push(3, nil, &scratch)
	s.push(4, n2, &scratch)

	got := s.materialize(nil)
	if len(got) != 4 {
		t.Fatalf("materialized len = %d, want 4", len(got))
	}
	if got[0].state != 1 || got[1].state != 2 || got[2].state != 3 || got[3].state != 4 {
		t.Fatalf("unexpected materialized states: %+v", got)
	}

	if off := s.byteOffset(); off != 9 {
		t.Fatalf("byteOffset = %d, want 9", off)
	}

	s.truncate(3)
	if off := s.byteOffset(); off != 5 {
		t.Fatalf("byteOffset after truncate = %d, want 5", off)
	}
}

func TestGLRStackToGSS(t *testing.T) {
	var gScratch gssScratch
	var entryScratch glrEntryScratch
	s := newGLRStackWithScratch(1, &entryScratch)
	s.push(2, nil, &entryScratch, &gScratch)
	s.push(3, nil, &entryScratch, &gScratch)

	gs := s.toGSS(&gScratch)
	mat := gs.materialize(nil)
	want := s.ensureEntries(&entryScratch)
	if len(mat) != len(want) {
		t.Fatalf("materialized len = %d, want %d", len(mat), len(want))
	}
	for i := range mat {
		if mat[i].state != want[i].state {
			t.Fatalf("state[%d] = %d, want %d", i, mat[i].state, want[i].state)
		}
	}
}

func TestGSSStackMaterializePanicsOnCorruptDepth(t *testing.T) {
	head := &gssNode{entry: stackEntry{state: 2}, depth: 3}
	head.prev = &gssNode{entry: stackEntry{state: 1}, depth: 1}
	s := gssStack{head: head}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on corrupt GSS depth metadata")
		}
	}()
	_ = s.materialize(nil)
}

func TestGSSNodeHashComputedLazilyForSingleStackNodes(t *testing.T) {
	var scratch gssScratch
	scratch.singleStackMode = true

	n1 := &Node{symbol: 1, startByte: 0, endByte: 1, parseState: 5}
	n2 := &Node{symbol: 2, startByte: 1, endByte: 3, parseState: 6}

	var s gssStack
	s.push(1, nil, &scratch)
	s.push(2, n1, &scratch)
	s.push(3, n2, &scratch)

	if got := s.head.hash; got != 0 {
		t.Fatalf("head hash before demand = %d, want 0", got)
	}

	got := gssNodeHash(s.head)
	if got == 0 {
		t.Fatal("expected lazy hash to compute non-zero value")
	}
	if s.head.hash != got {
		t.Fatalf("cached head hash = %d, want %d", s.head.hash, got)
	}

	entries := s.materialize(nil)
	want := gssHashSeed
	for i := range entries {
		want = gssEntryHash(want, entries[i])
	}
	if got != want {
		t.Fatalf("lazy hash = %d, want %d", got, want)
	}
}

func TestGSSEntryHashMatchesAccessorSemantics(t *testing.T) {
	node := &Node{
		children:     []*Node{{symbol: 20}},
		symbol:       10,
		startByte:    1,
		endByte:      3,
		parseState:   4,
		productionID: 5,
		flags:        nodeFlagNamed | nodeFlagHasError,
	}
	noTree := &noTreeNode{
		symbol:       11,
		startByte:    2,
		endByte:      5,
		parseState:   6,
		productionID: 7,
		flags:        nodeFlagExtra,
	}
	compactLeaf := &compactFullLeaf{
		noTreeNode: noTreeNode{
			symbol:       12,
			startByte:    8,
			endByte:      13,
			parseState:   9,
			productionID: 10,
			flags:        nodeFlagNamed | nodeFlagMissing,
		},
	}
	pending := &pendingParent{
		noTreeNode: noTreeNode{
			symbol:       13,
			startByte:    21,
			endByte:      34,
			parseState:   11,
			productionID: 12,
			flags:        nodeFlagNamed | nodeFlagExtra,
		},
		childRange: newPendingChildRange(0, 0, 3),
	}

	entries := []stackEntry{
		{state: 1},
		newStackEntryNode(2, node),
		newStackEntryNoTreeNode(3, noTree),
		newStackEntryCompactFullLeaf(4, compactLeaf),
		newStackEntryPendingParent(5, pending),
	}
	for _, entry := range entries {
		got := gssEntryHash(gssHashSeed, entry)
		want := gssEntryHashViaAccessors(gssHashSeed, entry)
		if got != want {
			t.Fatalf("gssEntryHash(%+v) = %d, want %d", entry, got, want)
		}
	}
}

func gssEntryHashViaAccessors(prev uint64, entry stackEntry) uint64 {
	h := prev ^ uint64(entry.state)
	h *= gssHashPrime

	if !stackEntryHasNode(entry) {
		h ^= gssNilNodeSentinel
		h *= gssHashPrime
		return h
	}

	h ^= uint64(stackEntryNodeSymbol(entry))
	h *= gssHashPrime
	h ^= (uint64(stackEntryNodeStartByte(entry)) << 32) | uint64(stackEntryNodeEndByte(entry))
	h *= gssHashPrime
	h ^= uint64(stackEntryNodeParseState(entry))
	h *= gssHashPrime
	h ^= uint64(stackEntryNodeProductionID(entry))
	h *= gssHashPrime
	h ^= uint64(stackEntryNodeChildCount(entry))
	h *= gssHashPrime

	var flags uint64
	if stackEntryNodeIsExtra(entry) {
		flags |= 1
	}
	if stackEntryNodeIsNamed(entry) {
		flags |= 1 << 1
	}
	if stackEntryNodeHasError(entry) {
		flags |= 1 << 2
	}
	if stackEntryNodeIsMissing(entry) {
		flags |= 1 << 3
	}
	h ^= flags
	h *= gssHashPrime
	return h
}

func TestGSSStacksEqualWithLazyHashes(t *testing.T) {
	var scratch gssScratch
	scratch.singleStackMode = true

	left := &Node{symbol: 1, startByte: 0, endByte: 1}
	right := &Node{symbol: 2, startByte: 1, endByte: 2}

	build := func() gssStack {
		var s gssStack
		s.push(1, nil, &scratch)
		s.push(2, left, &scratch)
		s.push(3, right, &scratch)
		return s
	}

	a := build()
	b := build()
	if a.head.hash != 0 || b.head.hash != 0 {
		t.Fatal("expected stacks to start with lazy hashes")
	}
	if !gssStacksEqual(a, b) {
		t.Fatal("expected equal GSS stacks with lazy hashes")
	}
	if a.head.hash == 0 || b.head.hash == 0 {
		t.Fatal("expected equality check to populate lazy hashes")
	}
}

func TestGSSScratchResetClearsTouchedSlots(t *testing.T) {
	var scratch gssScratch
	node := &Node{endByte: 1}
	var stack gssStack
	stack.push(1, node, &scratch)
	stack.push(2, node, &scratch)
	if len(scratch.slabs) == 0 || scratch.slabs[0].used != 2 {
		t.Fatalf("expected two used GSS slots, slabs=%d used=%d", len(scratch.slabs), scratch.slabs[0].used)
	}

	scratch.reset()

	slab := scratch.slabs[0]
	if slab.used != 0 {
		t.Fatalf("slab.used after reset = %d, want 0", slab.used)
	}
	for i := 0; i < 2; i++ {
		if stackEntryNode(slab.data[i].entry) != nil {
			t.Fatalf("slab.data[%d].entry node after reset = %p, want nil", i, stackEntryNode(slab.data[i].entry))
		}
		if slab.data[i].prev != nil {
			t.Fatalf("slab.data[%d].prev after reset = %p, want nil", i, slab.data[i].prev)
		}
	}
}

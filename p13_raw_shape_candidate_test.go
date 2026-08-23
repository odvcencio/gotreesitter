package gotreesitter

import "testing"

// p13CandidateRawShapeHash is the one-candidate design under review. It
// reads the packed payload and leaves packedEntry.state private to shapeRef.
func p13CandidateRawShapeHash(arena *nodeArena, parentRef rawShapeRef, symbol Symbol, productionID uint16, childCount uint16, children []rawShapeChild) uint64 {
	h := gssHashSeed
	h ^= uint64(symbol)
	h *= gssHashPrime
	h ^= uint64(productionID)
	h *= gssHashPrime
	h ^= uint64(childCount)
	h *= gssHashPrime
	for i := range children {
		entry := children[i].packedEntry
		if !stackEntryHasNode(entry) {
			h ^= gssNilNodeSentinel
			h *= gssHashPrime
			continue
		}
		h ^= uint64(stackEntryNodeSymbol(entry))
		h *= gssHashPrime
		h ^= (uint64(stackEntryNodeStartByte(entry)) << 32) | uint64(stackEntryNodeEndByte(entry))
		h *= gssHashPrime
		if ref := children[i].shapeRef(); ref != 0 && ref < parentRef && arena != nil {
			if childHash, ok := arena.rawShapeHash(ref); ok {
				h ^= childHash
				h *= gssHashPrime
				continue
			}
		}
		h ^= uint64(stackEntryNodeChildCount(entry))
		h *= gssHashPrime
	}
	return h
}

func TestP13CandidateRawShapeHashIsBitIdentical(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	node := newParentNodeInArena(arena, 11, true, []*Node{}, nil, 0)
	node.startByte, node.endByte, node.parseState = 3, 9, 17
	noTree := newNoTreeLeafNodeInArena(arena, 12, true, 10, 14, Point{}, Point{})
	noTree.parseState = 19
	compact := newCompactFullLeafInArena(arena, 13, true, 15, 21, Point{}, Point{})
	compact.parseState = 23
	pending := newPendingParentInArena(arena, 14, true, 7, nil, 22, 31, Point{}, Point{}, false)
	pending.parseState = 29

	entries := []stackEntry{
		newStackEntryNode(91, node),
		newStackEntryNoTreeNode(92, noTree),
		newStackEntryCompactFullLeaf(93, compact),
		newStackEntryPendingParent(94, pending),
		{},
	}
	children := make([]rawShapeChild, 0, len(entries))
	for _, entry := range entries {
		children = append(children, newRawShapeChild(entry))
	}
	for _, parentRef := range []rawShapeRef{1, 2, ^rawShapeRef(0)} {
		want := rawShapeComputeContentHash(arena, parentRef, 7, 3, uint16(len(children)), children)
		got := p13CandidateRawShapeHash(arena, parentRef, 7, 3, uint16(len(children)), children)
		if got != want {
			t.Fatalf("parentRef=%d candidate hash=%#x, current hash=%#x", parentRef, got, want)
		}
	}
}

func TestP13CandidateRawShapeHashPreservesRecursiveChildGuard(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 3, true, 4, 5, Point{}, Point{Column: 1})
	childRef := (&Parser{}).captureRawShape(nil, arena, 3, 0, []stackEntry{newStackEntryNode(1, leaf)}, 0, 1)
	if childRef == 0 {
		t.Fatal("child raw shape reference is zero")
	}
	parentNode := newParentNodeInArena(arena, 4, true, []*Node{leaf}, nil, 0)
	parentRef := (&Parser{}).captureRawShape(nil, arena, 4, 0, []stackEntry{newStackEntryNode(2, leaf)}, 0, 1)
	if parentRef == 0 || childRef >= parentRef {
		t.Fatalf("references child=%d parent=%d; want child < parent", childRef, parentRef)
	}
	parentNode.rawShape = parentRef
	child := newRawShapeChild(newStackEntryNode(5, leaf))
	child.packedEntry.state = StateID(childRef)
	want := rawShapeComputeContentHash(arena, parentRef, 4, 0, 1, []rawShapeChild{child})
	got := p13CandidateRawShapeHash(arena, parentRef, 4, 0, 1, []rawShapeChild{child})
	if got != want {
		t.Fatalf("recursive candidate hash=%#x, current hash=%#x", got, want)
	}
	// A forward reference must use the child-count fallback, not recurse.
	child.packedEntry.state = StateID(parentRef + 1)
	want = rawShapeComputeContentHash(arena, parentRef, 4, 0, 1, []rawShapeChild{child})
	got = p13CandidateRawShapeHash(arena, parentRef, 4, 0, 1, []rawShapeChild{child})
	if got != want {
		t.Fatalf("forward-reference candidate hash=%#x, current hash=%#x", got, want)
	}
}

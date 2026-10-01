package gotreesitter

import "testing"

func TestReuseOwnershipPrunesLocalArenasWithoutRetainingUnrelatedOwners(t *testing.T) {
	local := acquireNodeArena(arenaClassFull)
	local.ownership.BeginFresh()
	leaf := newLeafNodeInArena(local, 1, true, 0, 1, Point{}, Point{Column: 1})
	closed := newParentNodeInArena(local, 2, true, []*Node{leaf}, nil, 0)
	first := newTreeWithArenas(closed, []byte("x"), nil, local, nil)
	if !local.ownership.Local() {
		t.Fatal("local tree did not seal its ownership")
	}
	unrelated := acquireNodeArena(arenaClassFull)
	unrelatedLeaf := newLeafNodeInArena(unrelated, 1, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	unrelatedTree := newTreeWithArenas(unrelatedLeaf, []byte(" x"), nil, unrelated, nil)
	mixed := acquireNodeArena(arenaClassIncremental)
	parent := newParentNodeInArena(mixed, 3, true, []*Node{closed, unrelatedLeaf}, nil, 0)
	local.Retain()
	unrelated.Retain()
	second := newTreeWithArenas(parent, []byte("xx"), nil, mixed, []*nodeArena{local, unrelated})
	if mixed.ownership.Local() {
		t.Fatal("mixed tree received a local ownership receipt")
	}
	primary := acquireNodeArena(arenaClassIncremental)
	state := parseReuseState{}
	state.markReused(closed, primary)
	newest := newTreeWithArenas(closed, []byte("x"), nil, primary, state.retainBorrowed(primary))
	if len(newest.borrowedArena) != 1 || newest.borrowedArena[0] != local {
		t.Fatalf("reusing a local descendant retained unrelated owners: %v", newest.borrowedArena)
	}
	first.Release()
	unrelatedTree.Release()
	second.Release()
	if unrelated.refs.Load() != 0 || mixed.refs.Load() != 0 || local.refs.Load() != 1 {
		t.Fatalf("unexpected retained owners: local=%d unrelated=%d mixed=%d", local.refs.Load(), unrelated.refs.Load(), mixed.refs.Load())
	}
	if leaf.EndByte() != 1 {
		t.Fatal("local descendant changed after releasing earlier trees")
	}
	newest.Release()
	if local.refs.Load() != 0 || local.ownership.Local() {
		t.Fatal("last release retained local ownership")
	}
}

func TestReuseOwnershipMixedPublicationRevokesLocalReceipt(t *testing.T) {
	local := acquireNodeArena(arenaClassFull)
	local.ownership.BeginFresh()
	leaf := newLeafNodeInArena(local, 1, true, 0, 1, Point{}, Point{Column: 1})
	first := newTreeWithArenas(leaf, []byte("x"), nil, local, nil)
	foreign := acquireNodeArena(arenaClassFull)
	child := newLeafNodeInArena(foreign, 1, true, 0, 1, Point{}, Point{Column: 1})
	foreignTree := newTreeWithArenas(child, []byte("x"), nil, foreign, nil)
	parent := newParentNodeInArena(local, 2, true, []*Node{child}, nil, 0)
	local.Retain()
	foreign.Retain()
	second := newTreeWithArenas(parent, []byte("x"), nil, local, []*nodeArena{foreign})
	primary := acquireNodeArena(arenaClassIncremental)
	state := parseReuseState{}
	state.markReused(parent, primary)
	newest := newTreeWithArenas(parent, []byte("x"), nil, primary, state.retainBorrowed(primary))
	first.Release()
	foreignTree.Release()
	second.Release()
	if len(newest.borrowedArena) != 2 || foreign.refs.Load() != 1 || local.refs.Load() != 1 {
		t.Fatal("revoked local receipt lost a transitive owner")
	}
	newest.Release()
	if foreign.refs.Load() != 0 || local.refs.Load() != 0 {
		t.Fatal("transitive owners survived final release")
	}
}

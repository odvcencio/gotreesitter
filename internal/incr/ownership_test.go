package incr

import (
	"reflect"
	"testing"
)

func TestOwnershipWalkMatchesReachableOwners(t *testing.T) {
	type node struct {
		owner    int
		children []*node
	}
	local := &Ownership{}
	local.BeginFresh()
	local.Publish(false)
	mixed := &Ownership{}
	mixed.Publish(true)
	unknown := &Ownership{}
	unknown.Publish(false)
	if unknown.Local() {
		t.Fatal("an unknown producer received a local receipt")
	}
	certificates := []*Ownership{unknown, local, mixed}
	leaf := &node{owner: 1}
	closed := &node{owner: 1, children: []*node{leaf, leaf}}
	inner := &node{owner: 2, children: []*node{closed}}
	root := &node{owner: 0, children: []*node{nil, inner, closed}}
	want := map[int]bool{}
	var exhaustive func(*node)
	exhaustive = func(n *node) {
		if n != nil {
			want[n.owner] = true
			for _, child := range n.children {
				exhaustive(child)
			}
		}
	}
	exhaustive(root)
	got := map[int]bool{}
	visited := 0
	var scratch []*node
	VisitOwners(root, &scratch, func(n *node) bool {
		visited++
		got[n.owner] = true
		return certificates[n.owner].Local()
	}, func(n *node, stack []*node) []*node { return append(stack, n.children...) })
	if !reflect.DeepEqual(got, want) || visited != 4 {
		t.Fatalf("owners=%v want=%v visited=%d want=4", got, want, visited)
	}
	for _, n := range scratch[:cap(scratch)] {
		if n != nil {
			t.Fatal("ownership scratch retained a node")
		}
	}
	// A later mixed publication must revoke the local receipt, including
	// descendants with the same owner before the foreign edge.
	local.Publish(true)
	leaf.children = []*node{{owner: 0}}
	got = map[int]bool{}
	VisitOwners(closed, &scratch, func(n *node) bool {
		got[n.owner] = true
		return certificates[n.owner].Local()
	}, func(n *node, stack []*node) []*node { return append(stack, n.children...) })
	if !reflect.DeepEqual(got, map[int]bool{0: true, 1: true}) {
		t.Fatalf("revoked receipt lost reachable owners: %v", got)
	}
	local.BeginFresh()
	local.Publish(false)
	if local.Local() {
		t.Fatal("a partial local publication recertified a mixed arena")
	}
	local.Reset()
	if local.Local() {
		t.Fatal("released arena kept its receipt")
	}
}

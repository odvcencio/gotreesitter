package parsercorephase0

import (
	"reflect"
	"testing"
)

func TestDerivationsPackedPathsOwnTheirPayloads(t *testing.T) {
	compact := newTinyCore(t, 4)
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payloads := make([]SubtreeID, 6)
	for i := range payloads {
		payloads[i], err = compact.appendSubtree(
			subtreeRecord{symbol: Symbol(i + 1), terminal: true}, nil, nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	appendHead := func(state StateID, inputs ...linkInput) Head {
		t.Helper()
		var head Head
		for _, input := range inputs {
			head, err = compact.condense(compact.boundaryKey(state, 0), input)
			if err != nil {
				t.Fatal(err)
			}
		}
		return head
	}
	prefix := appendHead(2, linkInput{prev: seed.Node, payload: payloads[0], scoreDelta: 2})
	fork := appendHead(3,
		linkInput{prev: prefix.Node, payload: payloads[1], scoreDelta: 3, order: ForkOrder{Present: true, Value: 7}},
		linkInput{prev: prefix.Node, payload: payloads[2], scoreDelta: -2, order: ForkOrder{Present: true, Value: 8}},
	)
	joined := appendHead(4, linkInput{prev: fork.Node, payload: payloads[3], scoreDelta: -1})
	head := appendHead(5,
		linkInput{prev: joined.Node, payload: payloads[4], scoreDelta: 4, order: ForkOrder{Present: true, Value: 9}},
		linkInput{prev: joined.Node, payload: payloads[5], scoreDelta: -5},
	)
	want := []Derivation{
		{Payloads: []SubtreeID{payloads[0], payloads[1], payloads[3], payloads[4]}, Score: 8, BranchOrder: 9, HasBranchOrder: true},
		{Payloads: []SubtreeID{payloads[0], payloads[2], payloads[3], payloads[4]}, Score: 3, BranchOrder: 9, HasBranchOrder: true},
		{Payloads: []SubtreeID{payloads[0], payloads[1], payloads[3], payloads[5]}, Score: -1, BranchOrder: 7, HasBranchOrder: true},
		{Payloads: []SubtreeID{payloads[0], payloads[2], payloads[3], payloads[5]}, Score: -6, BranchOrder: 8, HasBranchOrder: true},
	}
	paths, err := compact.Derivations(head)
	if err != nil || !reflect.DeepEqual(paths, want) {
		t.Fatalf("packed paths=%+v err=%v, want %+v", paths, err, want)
	}
	paths[0].Payloads[0] = 0
	paths[0].Payloads = append(paths[0].Payloads, 0)
	if !reflect.DeepEqual(paths[1:], want[1:]) {
		t.Fatalf("editing one path changed its siblings: %+v", paths)
	}
	again, err := compact.Derivations(head)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("editing returned paths changed later enumeration: paths=%+v err=%v", again, err)
	}
}

func TestDerivationsDeepPrefixBelowFork(t *testing.T) {
	const depth = 2048
	c := newTinyCoreWithLimits(t, Limits{MaxNodes: depth + 2, MaxLinks: depth + 2, MaxSubtrees: depth + 2, MaxDerivations: 2})
	if _, err := c.Seed(1, 0); err != nil {
		t.Fatal(err)
	}
	c.nodes = append(c.nodes, make([]nodeRecord, depth+1)...)
	c.links = make([]linkRecord, depth+2)
	c.subtrees = make([]subtreeRecord, depth+2)
	for i := 0; i < depth; i++ {
		c.links[i] = linkRecord{prev: NodeID(i + 1), payload: SubtreeID(i + 1)}
		c.nodes[i+1] = nodeRecord{firstLink: uint32(i + 1), linkCount: 1, pathCount: 1}
	}
	c.links[0].scoreDelta = 2
	c.links[depth] = linkRecord{prev: NodeID(depth + 1), payload: SubtreeID(depth + 1), scoreDelta: 3, flags: linkFlagHasOrder, order: 7}
	c.links[depth+1] = linkRecord{prev: NodeID(depth + 1), payload: SubtreeID(depth + 2), scoreDelta: -3, flags: linkFlagHasOrder, order: 8, next: LinkID(depth + 1)}
	c.nodes[depth+1] = nodeRecord{firstLink: depth + 2, linkCount: 2, pathCount: 2}
	head := Head{Node: depth + 2}
	paths, err := c.Derivations(head)
	if err != nil || len(paths) != 2 {
		t.Fatalf("paths=%d err=%v", len(paths), err)
	}
	for i, p := range paths {
		if len(p.Payloads) != depth+1 || p.Payloads[depth] != SubtreeID(depth+1+i) || p.BranchOrder != uint64(7+i) {
			t.Fatalf("path %d lost order or payloads", i)
		}
	}
	if paths[0].Score != 5 || paths[1].Score != -1 {
		t.Fatal("prefix scores changed")
	}
	paths[0].Payloads[0] = 0
	if paths[1].Payloads[0] != 1 {
		t.Fatal("forked paths alias")
	}
	allocs := testing.AllocsPerRun(3, func() {
		if _, err := c.Derivations(head); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("deep prefix allocations per enumeration: %.0f", allocs)
	if allocs > 64 {
		t.Fatalf("single-path prefixes allocate per graph record: %.0f", allocs)
	}
}

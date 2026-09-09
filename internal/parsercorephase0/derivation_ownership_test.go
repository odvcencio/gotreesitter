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

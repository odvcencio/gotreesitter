package gotreesitter

import "testing"

func makeRetentionTestStack(topState StateID, depth int, shifted bool, endByte uint32) glrStack {
	if depth < 1 {
		depth = 1
	}
	s := newGLRStack(1)
	lastByte := uint32(0)
	for i := 1; i < depth; i++ {
		state := StateID(100 + i)
		if i == depth-1 {
			state = topState
		}
		nextByte := lastByte + 1
		if i == depth-1 && endByte > nextByte {
			nextByte = endByte
		}
		s.push(state, NewLeafNode(1, true, lastByte, nextByte, Point{Row: 0, Column: lastByte}, Point{Row: 0, Column: nextByte}), nil, nil)
		lastByte = nextByte
	}
	s.shifted = shifted
	return s
}

func TestRetainTopStacksKeepsUnshiftedCurrentTokenBranch(t *testing.T) {
	shifted := makeRetentionTestStack(3, 3, true, 2)
	unshifted := makeRetentionTestStack(2, 2, false, 1)

	kept := retainTopStacks([]glrStack{shifted, unshifted}, 1)
	if len(kept) != 1 {
		t.Fatalf("len(kept) = %d, want 1", len(kept))
	}
	if kept[0].shifted {
		t.Fatal("retained shifted stack; want unshifted current-token branch")
	}
	if got, want := kept[0].depth(), unshifted.depth(); got != want {
		t.Fatalf("kept depth = %d, want %d", got, want)
	}
}

func TestRetainTopStacksKeepsDistinctTopStateRepresentative(t *testing.T) {
	stacks := []glrStack{
		makeRetentionTestStack(507, 7, true, 6),
		makeRetentionTestStack(507, 6, true, 6),
		makeRetentionTestStack(507, 5, true, 6),
		makeRetentionTestStack(405, 3, true, 6),
		makeRetentionTestStack(506, 2, false, 5),
	}

	kept := retainTopStacks(stacks, 3)
	if len(kept) != 3 {
		t.Fatalf("len(kept) = %d, want 3", len(kept))
	}

	states := map[StateID]bool{}
	for i := range kept {
		states[kept[i].top().state] = true
	}
	for _, state := range []StateID{405, 506, 507} {
		if !states[state] {
			t.Fatalf("retained states = %#v, want representative for state %d", states, state)
		}
	}
}

func TestRetainTopStacksUsesBranchOrderTieBreak(t *testing.T) {
	first := makeRetentionTestStack(3, 3, false, 2)
	first.branchOrder = 1
	later := makeRetentionTestStack(3, 3, false, 2)
	later.branchOrder = 2

	kept := retainTopStacks([]glrStack{later, first}, 1)
	if len(kept) != 1 {
		t.Fatalf("len(kept) = %d, want 1", len(kept))
	}
	if got := kept[0].branchOrder; got != first.branchOrder {
		t.Fatalf("kept branchOrder = %d, want %d", got, first.branchOrder)
	}
}

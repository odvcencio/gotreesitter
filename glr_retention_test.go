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

func TestRetainTopStacksForPythonKeepsShallowerBranch(t *testing.T) {
	deeper := makeRetentionTestStack(1805, 6, false, 10)
	shallower := makeRetentionTestStack(1650, 3, false, 10)
	var (
		selected []int
		chosen   []bool
		keys     []stackCullKey
	)

	kept := retainTopStacksForLanguageWithScratch(
		[]glrStack{deeper, shallower},
		1,
		&Language{Name: "python"},
		&selected,
		&chosen,
		&keys,
	)
	if len(kept) != 1 {
		t.Fatalf("len(kept) = %d, want 1", len(kept))
	}
	if got, want := kept[0].top().state, StateID(1650); got != want {
		t.Fatalf("kept state = %d, want shallower Python branch state %d", got, want)
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

func TestRetainTopStacksKeepsAcceptedOutsideLiveCap(t *testing.T) {
	for _, withScratch := range []bool{false, true} {
		name := "without scratch"
		if withScratch {
			name = "with scratch"
		}
		t.Run(name, func(t *testing.T) {
			makeStack := func(state StateID, score int, order uint64, accepted bool) glrStack {
				stack := makeRetentionTestStack(state, 2, false, 1)
				stack.score = score
				stack.branchOrder = order
				stack.accepted = accepted
				return stack
			}
			stacks := []glrStack{
				makeStack(10, 0, 101, true),
				makeStack(30, 1, 3, false),
				makeStack(10, 3, 1, false),
				makeStack(40, 0, 102, true),
				makeStack(20, 2, 2, false),
				makeStack(50, 0, 103, true),
			}
			var selected []int
			var chosen []bool
			var keys []stackCullKey
			var selectedBuf *[]int
			var chosenBuf *[]bool
			var keyBuf *[]stackCullKey
			if withScratch {
				selectedBuf, chosenBuf, keyBuf = &selected, &chosen, &keys
			}
			kept := retainTopStacksForLanguageWithScratch(stacks, 2, nil, selectedBuf, chosenBuf, keyBuf)
			wantOrder := []uint64{101, 1, 102, 2, 103}
			if len(kept) != len(wantOrder) {
				t.Fatalf("retained %d stacks, want %d (two live and three accepted)", len(kept), len(wantOrder))
			}
			for i, want := range wantOrder {
				if kept[i].branchOrder != want {
					t.Fatalf("retained order[%d] = %d, want %d", i, kept[i].branchOrder, want)
				}
			}
		})
	}
}

func TestCullParseStacksForIterationIgnoresAcceptedAtTrigger(t *testing.T) {
	accepted := makeRetentionTestStack(10, 2, false, 1)
	accepted.accepted = true
	liveA := makeRetentionTestStack(20, 2, false, 1)
	liveB := makeRetentionTestStack(30, 2, false, 1)
	stacks := []glrStack{accepted, liveA, liveB}

	parser := &Parser{}
	kept := parser.cullParseStacksForIteration(stacks, &parserScratch{}, arenaClassFull, 1, 2, false, nil)
	if len(kept) != len(stacks) {
		t.Fatalf("retained %d stacks, want all %d before the live trigger", len(kept), len(stacks))
	}
	for i := range stacks {
		if kept[i].top().state != stacks[i].top().state || kept[i].accepted != stacks[i].accepted {
			t.Fatalf("stack %d changed before the live trigger", i)
		}
	}
}

func TestRetainTopStacksZeroLiveCapKeepsAccepted(t *testing.T) {
	first := makeRetentionTestStack(10, 2, false, 1)
	first.accepted = true
	live := makeRetentionTestStack(20, 2, false, 1)
	last := makeRetentionTestStack(30, 2, false, 1)
	last.accepted = true

	kept := retainTopStacks([]glrStack{first, live, last}, 0)
	if len(kept) != 2 || kept[0].top().state != 10 || kept[1].top().state != 30 {
		t.Fatalf("retained stacks = %+v, want accepted states 10 and 30", kept)
	}
}

package gotreesitter

import "testing"

func TestPrimaryPromotionPreservesCVersionOrder(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		parser := &Parser{errorCostCompetition: enabled}
		shift := newGLRStack(1)
		shift.branchOrder = 2
		reduce := newGLRStack(1)
		reduce.branchOrder = 1
		stacks := []glrStack{shift, reduce}
		parser.promotePrimaryStack(stacks)
		want := uint64(1)
		if enabled {
			want = 2
		}
		if stacks[0].branchOrder != want {
			t.Fatalf("C competition=%t: primary rank=%d, want %d", enabled, stacks[0].branchOrder, want)
		}
		if stackComparePtr(&reduce, &shift) <= 0 {
			t.Fatal("grammar rank no longer prefers the earlier action")
		}
	}
}

func TestCShiftConflictPrimaryIndex(t *testing.T) {
	reduce := ParseAction{Type: ParseActionReduce}
	shift := ParseAction{Type: ParseActionShift}
	for _, tc := range []struct {
		name    string
		actions []ParseAction
		want    int
	}{
		{"empty", nil, 0},
		{"shift", []ParseAction{shift}, 0},
		{"reduce_shift", []ParseAction{reduce, shift}, 1},
		{"multiple_reductions_shift", []ParseAction{reduce, reduce, shift}, 2},
		{"reduce_only", []ParseAction{reduce, reduce}, 0},
		{"repetition", []ParseAction{reduce, {Type: ParseActionShift, Repetition: true}}, 0},
		{"terminal_before_shift", []ParseAction{{Type: ParseActionAccept}, shift}, 0},
		{"shift_before_reduce", []ParseAction{shift, reduce}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cShiftConflictPrimaryIndex(tc.actions); got != tc.want {
				t.Fatalf("primary action=%d, want %d", got, tc.want)
			}
		})
	}
}

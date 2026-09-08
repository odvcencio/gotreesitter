package gotreesitter

import "testing"

func TestHashOverflowPreservesSurvivorAdmissionOrder(t *testing.T) {
	for _, weakFirst := range []bool{true, false} {
		stacks := make([]glrStack, 0, 8)
		add := func(state StateID, score int, branch uint64) {
			s := makeRetentionTestStack(state, int(branch)+2, true, 35)
			s.score = score
			s.branchOrder = branch
			stacks = append(stacks, s)
		}
		if weakFirst {
			add(62, -2, 1)
			add(62, 2, 2)
		} else {
			add(62, 2, 2)
			add(62, -2, 1)
		}
		add(70, -3, 3)
		add(80, 0, 4)
		add(70, 1, 5)
		add(62, 2, 6)
		add(70, 2, 7)
		scratch := &glrMergeScratch{perKeyCap: 2}
		result := mergeStacksWithScratch(stacks, scratch)
		want := []uint64{2, 4, 5, 6, 7}
		if len(result) != len(want) {
			t.Fatalf("weakFirst=%v: got %d survivors", weakFirst, len(result))
		}
		for i, branch := range want {
			if result[i].branchOrder != branch {
				t.Fatalf("weakFirst=%v: slot %d branch=%d, want %d", weakFirst, i, result[i].branchOrder, branch)
			}
		}
		seen := make(map[int]bool)
		for si := range scratch.slots {
			slot := &scratch.slots[si]
			for pos := 0; pos < mergeSlotTrackedCount(slot); pos++ {
				idx := mergeSlotIndexAt(slot, pos)
				if idx < 0 || idx >= len(result) || seen[idx] {
					t.Fatalf("invalid or repeated tracked index %d", idx)
				}
				seen[idx] = true
				if mergeKeyForStack(&result[idx]) != slot.key || mergeSlotHashAt(slot, pos) != stackHashForMerge(scratch, nil, &result[idx]) {
					t.Fatalf("slot %d position %d has stale key or hash", si, pos)
				}
			}
			if slot.worstIndex != recomputeMergeSlotWorst(slot, result) || slot.hashMask != recomputeMergeSlotHashMask(slot) {
				t.Fatalf("slot %d has stale worst index or hash mask", si)
			}
		}
		if len(seen) != len(result) {
			t.Fatal("untracked survivor")
		}
		scratch.reset()
	}
}

package sched

import "testing"

func TestConflictBranchOrderRetainsEveryAlternative(t *testing.T) {
	for _, count := range []int{2, 3, 8} {
		orders := make(map[uint64]bool)
		for ordinal := 0; ordinal < count; ordinal++ {
			order := ConflictBranchOrder(ordinal, count, 40, uint64(41+ordinal))
			if orders[order] {
				t.Fatalf("%d actions share priority %d", count, order)
			}
			orders[order] = true
			if ordinal == count-1 && order != 40 {
				t.Fatal("last action lost source version priority")
			}
			if ordinal != count-1 && order <= 40 {
				t.Fatal("earlier action displaced the source version")
			}
		}
	}
}

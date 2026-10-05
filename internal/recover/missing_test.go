package recover

import "testing"

func TestContainsMissingFollowsSharedRetainedHistory(t *testing.T) {
	children := [][]int{{1, 2}, {3}, {3}, nil}
	for _, want := range []bool{false, true} {
		visits := make([]int, len(children))
		got := ContainsMissing(0, func(i int) (bool, int) {
			visits[i]++
			return want && i == 3, len(children[i])
		}, func(i, child int) (int, bool) { return children[i][child], true })
		if got != want {
			t.Fatalf("ContainsMissing() = %t, want %t", got, want)
		}
		if visits[3] != 1 {
			t.Fatalf("shared history visited %d times, want 1", visits[3])
		}
	}
}

func TestContainsMissingIgnoresUnavailableEdgesAndCycles(t *testing.T) {
	got := ContainsMissing(0, func(int) (bool, int) { return false, 2 }, func(_ int, child int) (int, bool) {
		return 0, child == 0
	})
	if got {
		t.Fatal("clean cyclic history reported a missing node")
	}
}

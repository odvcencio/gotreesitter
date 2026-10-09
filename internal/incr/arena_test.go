package incr

import "testing"

func TestFrontierArenaCapacityBounds(t *testing.T) {
	for _, tc := range []struct{ bytes, hint, base, limit, want int }{
		{1024 * 1024, 0, 1024, 300000, 4096},
		{1024 * 1024, 1500, 1024, 300000, 1500},
		{64, 0, 1024, 300000, 1024},
		{64, 10, 1024, 300000, 1024},
		{1024 * 1024, 400000, 1024, 300000, 300000},
	} {
		if got := FrontierArenaCapacity(tc.bytes, tc.hint, tc.base, tc.limit); got != tc.want {
			t.Fatalf("%+v: capacity=%d", tc, got)
		}
	}
	for _, certified := range []bool{false, true} {
		for _, capacity := range []int{2048, 4096} {
			if got := RetainFrontierArena(certified, capacity, 2048); got != (certified && capacity > 2048) {
				t.Fatalf("certified=%t capacity=%d retained=%t", certified, capacity, got)
			}
		}
	}
}

func TestResetDependencyScratchSharedLimit(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		ends, leaves, keepEnds, keepLeaves int
	}{
		{"empty", 0, 0, 0, 0},
		{"both fit", 3, 5, 3, 5},
		{"leaf fills budget", 1, 8, 0, 8},
		{"prefer leaves", 5, 5, 0, 5},
		{"oversized ends", 9, 3, 0, 3},
		{"oversized leaves", 3, 9, 3, 0},
		{"both oversized", 9, 9, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ends, leaves := make([]uint32, tc.ends), make([]uint32, tc.leaves)
			for _, values := range [][]uint32{ends, leaves} {
				for i := range values {
					values[i] = uint32(i + 1)
				}
			}
			gotEnds, gotLeaves := ResetDependencyScratch(ends[:tc.ends/2], leaves[:tc.leaves/2], 8)
			if cap(gotEnds) != tc.keepEnds || cap(gotLeaves) != tc.keepLeaves || len(gotEnds) != 0 || len(gotLeaves) != 0 {
				t.Fatalf("retained lengths/capacities = %d/%d, %d/%d", len(gotEnds), cap(gotEnds), len(gotLeaves), cap(gotLeaves))
			}
			for _, values := range [][]uint32{gotEnds[:cap(gotEnds)], gotLeaves[:cap(gotLeaves)]} {
				for _, value := range values {
					if value != 0 {
						t.Fatal("retained scratch carries stale authorization")
					}
				}
			}
			if len(gotEnds[:cap(gotEnds)]) != 0 && &gotEnds[:cap(gotEnds)][0] != &ends[0] {
				t.Fatal("retained ends changed backing storage")
			}
			if len(gotLeaves[:cap(gotLeaves)]) != 0 && &gotLeaves[:cap(gotLeaves)][0] != &leaves[0] {
				t.Fatal("retained leaves changed backing storage")
			}
		})
	}
}

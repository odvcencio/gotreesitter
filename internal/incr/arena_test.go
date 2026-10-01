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

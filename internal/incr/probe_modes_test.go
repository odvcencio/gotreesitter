package incr

import (
	"math/rand"
	"slices"
	"testing"
)

func TestProbeModeSetMatchesOrderedLinearMembership(t *testing.T) {
	rng := rand.New(rand.NewSource(454))
	for trial := 0; trial < 8; trial++ {
		var set ProbeModeSet
		previous := make([]uint32, 0, 1024)
		states := []uint32{0, 0, 63, 64, 4095, 4096, 65535, 65536, 1 << 24, ^uint32(0), 4096, ^uint32(0)}
		for i := 0; i < 32768; i++ {
			states = append(states, uint32(rng.Intn(1536))*uint32(1+trial*64))
		}
		for _, state := range states {
			want := slices.Contains(previous, state)
			if got := set.SeenOrAdd(state, previous); got != want {
				t.Fatalf("state %d membership %t want %t", state, got, want)
			}
			if !want {
				previous = append(previous, state)
			}
		}
	}
}

func TestProbeModeSetIsAllocationFree(t *testing.T) {
	if got := testing.AllocsPerRun(100, func() {
		var set ProbeModeSet
		var storage [1024]uint32
		previous := storage[:0]
		for i := uint32(0); i < 2048; i++ {
			state := ((i % 1024) * 4096)
			if !set.SeenOrAdd(state, previous) {
				previous = append(previous, state)
			}
		}
	}); got != 0 {
		t.Fatalf("allocations %g", got)
	}
}

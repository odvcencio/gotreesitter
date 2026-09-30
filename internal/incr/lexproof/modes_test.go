package lexproof

import (
	"slices"
	"testing"
)

func TestModesPreserveProofOrderAndCapacity(t *testing.T) {
	var modes Modes
	var want []uint32
	for i := uint32(0); i < 1024; i++ {
		// Every key hashes to the same slot. Duplicate probes must still
		// succeed at capacity, and a new mode must decline without eviction.
		state := i<<16 | i
		want = append(want, state)
		if !modes.Add(state) || !modes.Add(state) || !slices.Equal(modes.Values(), want) {
			t.Fatalf("mode %d changed the ordered proof set", i)
		}
	}
	if !modes.Add(want[0]) || modes.Add(^uint32(0)-1) || !slices.Equal(modes.Values(), want) {
		t.Fatal("capacity changed or a duplicate displaced a proof mode")
	}
}

func TestModesIncludeZeroAndMaximumState(t *testing.T) {
	var modes Modes
	for _, state := range []uint32{0, ^uint32(0), 0, ^uint32(0)} {
		if !modes.Add(state) {
			t.Fatal("ordinary modes exceeded capacity")
		}
	}
	if !slices.Equal(modes.Values(), []uint32{0, ^uint32(0)}) {
		t.Fatal("sentinel values confused occupied slots")
	}
}

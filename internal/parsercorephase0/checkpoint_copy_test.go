package parsercorephase0

import (
	"bytes"
	"testing"
)

func TestCheckpointCopyScratchOwnershipAndReuse(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	left := mustInternCheckpoint(t, c, []byte("left"))
	right := mustInternCheckpoint(t, c, []byte("right"))
	var scratch CheckpointCopyScratch
	start, end, ok := scratch.CopyPair(c, left, right)
	if !ok || string(start) != "left" || string(end) != "right" {
		t.Fatalf("pair=(%q,%q,%t)", start, end, ok)
	}
	start[0], end[0] = 'x', 'y'
	if !c.CheckpointMatches(left, []byte("left")) || !c.CheckpointMatches(right, []byte("right")) {
		t.Fatal("temporary copies alias the core")
	}
	if got := testing.AllocsPerRun(100, func() {
		a, b, valid := scratch.CopyPair(c, left, right)
		if !valid || string(a) != "left" || string(b) != "right" {
			t.Fatal("reused copy changed checkpoint bytes")
		}
	}); got != 0 {
		t.Fatalf("warm checkpoint copies allocate: %g", got)
	}
	for _, ids := range [][2]CheckpointID{{left, left}, {right, left}, {right, right}} {
		a, b, valid := scratch.CopyPair(c, ids[0], ids[1])
		if !valid || !c.CheckpointMatches(ids[0], a) || !c.CheckpointMatches(ids[1], b) {
			t.Fatalf("pair %v lost exact bytes", ids)
		}
	}
	for _, ids := range [][2]CheckpointID{{0, right}, {left, 0}, {99, right}, {left, 99}} {
		a, b, valid := scratch.CopyPair(c, ids[0], ids[1])
		if valid || a != nil || b != nil {
			t.Fatalf("invalid pair %v published bytes", ids)
		}
	}
	if a, b, valid := scratch.CopyPair(nil, left, right); valid || a != nil || b != nil {
		t.Fatal("nil core published bytes")
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	reused := mustInternCheckpoint(t, c, []byte("changed"))
	if reused != left {
		t.Fatal("fixture did not reuse a checkpoint ID")
	}
	a, b, valid := scratch.CopyPair(c, reused, reused)
	if !valid || string(a) != "changed" || string(b) != "changed" {
		t.Fatal("core reset left stale checkpoint bytes")
	}
}

func TestCheckpointCopyScratchBoundsRetention(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	small := mustInternCheckpoint(t, c, []byte{1, 2, 3})
	largeBytes := bytes.Repeat([]byte{7}, 4097)
	large := mustInternCheckpoint(t, c, largeBytes)
	var scratch CheckpointCopyScratch
	a, b, ok := scratch.CopyPair(c, small, large)
	if !ok || !bytes.Equal(a, []byte{1, 2, 3}) || !bytes.Equal(b, largeBytes) {
		t.Fatal("large checkpoint transfer failed")
	}
	if got, want := scratch.RetainedBytes(), uint64(cap(scratch.start)+cap(scratch.end)); got != want {
		t.Fatalf("retained bytes=%d, want %d", got, want)
	}
	scratch.Reset()
	if len(scratch.start) != 0 || scratch.end != nil || scratch.RetainedBytes() != 3 {
		t.Fatal("reset failed to retain only the bounded buffer")
	}
}

package slicearena

import "testing"

func TestBoundedStorageIsolationAndReuse(t *testing.T) {
	a := Arena[*int]{Limit: 8, Chunk: 4}
	one, two := 1, 2
	first := a.Alloc(3)
	second := a.Alloc(3)
	first[0], second[0] = &one, &two
	// Exhaust the retained bound: this vector must use independent storage.
	extra := a.Alloc(3)
	extra[0] = &two
	if a.capacity != 8 || first[0] != &one || second[0] != &two || cap(first) != len(first) {
		t.Fatal("vectors overlap or exceed the retention bound")
	}
	firstAddress, secondAddress := &first[0], &second[0]
	a.Reset()
	first, second = a.Alloc(3), a.Alloc(3)
	if &first[0] != firstAddress || &second[0] != secondAddress || first[0] != nil || second[0] != nil || extra[0] != &two {
		t.Fatal("reset did not clear and reuse only retained storage")
	}
	if allocs := testing.AllocsPerRun(10, func() {
		a.Reset()
		a.Alloc(3)
		a.Alloc(3)
	}); allocs != 0 {
		t.Fatalf("warm retained vectors allocate: %g", allocs)
	}
}

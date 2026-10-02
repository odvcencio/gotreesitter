package scannerstate

import (
	"math/rand"
	"testing"
	"unsafe"
)

func TestSetOutOfOrderInsertAndReplace(t *testing.T) {
	var s Set[[2]uint64]
	order := rand.New(rand.NewSource(17)).Perm(10000)
	var charged int64
	for _, i := range order {
		charged += s.Upsert(i*3, [2]uint64{uint64(i), uint64(i + 1)})
	}
	for i := 0; i < len(order); i++ {
		want := [2]uint64{uint64(i), uint64(i + 1)}
		if got, ok := s.Lookup(i * 3); !ok || got != want {
			t.Fatalf("lookup %d = %v,%t, want %v", i*3, got, ok, want)
		}
		if _, ok := s.Lookup(i*3 + 1); ok {
			t.Fatalf("unwritten index %d is present", i*3+1)
		}
		if cost := s.Upsert(i*3, [2]uint64{9, 10}); cost != 0 {
			t.Fatalf("replacement grew storage by %d bytes", cost)
		}
		if got, _ := s.Lookup(i * 3); got != [2]uint64{9, 10} {
			t.Fatalf("replacement at %d = %v", i*3, got)
		}
	}
	if charged != s.Bytes() {
		t.Fatalf("charged %d bytes, retained %d", charged, s.Bytes())
	}
}

func TestSetGrowthRetainsValuesAndChargesBacking(t *testing.T) {
	var s Set[[2]uint64]
	charged := s.EnsureCapacity(400000)
	for i := 0; i < 500000; i++ {
		charged += s.Upsert(i*2, [2]uint64{uint64(i), uint64(i + 1)})
	}
	for i := 0; i < 500000; i++ {
		if got, ok := s.Lookup(i * 2); !ok || got != [2]uint64{uint64(i), uint64(i + 1)} {
			t.Fatalf("lookup %d = %v,%t", i*2, got, ok)
		}
	}
	var actual int64
	actual += int64(cap(s.chunks)) * int64(unsafe.Sizeof(chunk[[2]uint64]{}))
	for _, c := range s.chunks {
		actual += int64(cap(c.indexes))*4 + int64(cap(c.refs))*16
		if cap(c.refs) > maxChunk {
			t.Fatalf("chunk capacity %d exceeds %d", cap(c.refs), maxChunk)
		}
	}
	if charged != actual || s.Bytes() != actual {
		t.Fatalf("charged=%d reported=%d actual=%d", charged, s.Bytes(), actual)
	}
	if charged > 500000*20+maxChunk*20+int64(cap(s.chunks))*48 {
		t.Fatalf("excess unused backing: %d bytes", charged)
	}
}

func TestSetResetClearsReferencesAndReusesStorage(t *testing.T) {
	var s Set[*int]
	value := 1
	for i := 0; i < 10000; i++ {
		s.Upsert(i, &value)
	}
	before := s.Bytes()
	s.Reset()
	for _, c := range s.chunks {
		for _, ref := range c.refs[:cap(c.refs)] {
			if ref != nil {
				t.Fatal("reset retained a reference")
			}
		}
	}
	if _, ok := s.Lookup(0); ok {
		t.Fatal("reset retained a checkpoint")
	}
	if allocs := testing.AllocsPerRun(3, func() {
		for i := 0; i < 10000; i++ {
			s.Upsert(i, &value)
		}
		s.Reset()
	}); allocs != 0 {
		t.Fatalf("retained storage allocated %g objects", allocs)
	}
	if s.Bytes() != before {
		t.Fatal("reset changed retained bytes")
	}
}

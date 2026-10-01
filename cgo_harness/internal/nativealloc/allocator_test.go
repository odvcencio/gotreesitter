//go:build cgo && treesitter_c_parity

package nativealloc

import "testing"

func TestRequestedAllocationVolume(t *testing.T) {
	Reset()
	a := Malloc(8)
	if a == nil {
		t.Fatal("malloc failed")
	}
	b := Calloc(2, 4)
	if b == nil {
		Free(a)
		t.Fatal("calloc failed")
	}
	next := Realloc(a, 16)
	if next == nil {
		Free(a)
		Free(b)
		t.Fatal("realloc failed")
	}
	Free(next)
	Free(b)
	got := Snapshot()
	if got.Bytes != 32 || got.Allocations != 3 {
		t.Fatalf("freed allocation requests disappeared: %+v", got)
	}
	Reset()
	if got := Snapshot(); got != (Counts{}) {
		t.Fatalf("receipt did not reset: %+v", got)
	}
}

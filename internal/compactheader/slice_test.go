package compactheader

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestSlicePreservesHeaderAndBacking(t *testing.T) {
	for _, value := range [][]uint16{nil, {}, make([]uint16, 0, 5), make([]uint16, 3, 7)} {
		header := From(value)
		got := header.Get()
		if (got == nil) != (value == nil) || len(got) != len(value) || cap(got) != cap(value) {
			t.Fatalf("header changed: nil=%t len=%d cap=%d; want nil=%t len=%d cap=%d", got == nil, len(got), cap(got), value == nil, len(value), cap(value))
		}
		if cap(got) != 0 {
			got[:cap(got)][cap(got)-1] = 37
			if value[:cap(value)][cap(value)-1] != 37 {
				t.Fatal("backing alias lost")
			}
		}
		copyHeader := header
		header = From(got[:0])
		if len(copyHeader.Get()) != len(value) {
			t.Fatal("header replacement changed a copy")
		}
	}
}

func TestSliceKeepsBackingAliveAcrossGC(t *testing.T) {
	makeHeader := func() Slice[byte] {
		value := make([]byte, 1000)
		value[999] = 41
		return From(value)
	}
	header := makeHeader()
	runtime.GC()
	if header.Get()[999] != 41 {
		t.Fatal("header lost its backing array")
	}
	boxed := wide([]byte{11, 12})
	runtime.GC()
	if got := boxed.Get(); len(got) != 2 || got[0] != 11 || got[1] != 12 {
		t.Fatal("wide header lost its backing")
	}
}

func TestSliceLayoutAndAllocation(t *testing.T) {
	want := uintptr(16)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 12
	}
	if got := unsafe.Sizeof(Slice[uint16]{}); got != want {
		t.Fatalf("size=%d want=%d", got, want)
	}
	value := make([]uint16, 5, 9)
	if allocs := testing.AllocsPerRun(100, func() {
		if got := From(value).Get(); len(got) != 5 || cap(got) != 9 {
			t.Fatal("header changed")
		}
	}); allocs != 0 {
		t.Fatalf("compact header allocated %g objects", allocs)
	}
}

func TestSlicePreservesWideCapacity(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return
	}
	// Zero-sized elements exercise the real wide-slice path without a large
	// backing allocation.
	n := uint64(1)<<32 + 7
	value := make([]struct{}, int(n)-3, int(n))
	header := From(value)
	runtime.GC()
	got := header.Get()
	if len(got) != len(value) || cap(got) != cap(value) || got == nil {
		t.Fatalf("wide header changed: len=%d cap=%d nil=%t", len(got), cap(got), got == nil)
	}
}

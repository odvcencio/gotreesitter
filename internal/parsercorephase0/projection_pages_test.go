package parsercorephase0

import (
	"testing"
	"unsafe"
)

func TestProjectionPagesStableSlotsClearAndAccounting(t *testing.T) {
	type record struct {
		pointer *int
		value   int
	}
	var p ProjectionPages[record]
	value := 73
	if p.Len() != 0 || p.FootprintBytes() != 0 {
		t.Fatal("nonempty zero buffer")
	}
	var first, edge *record
	for i := 0; i < projectionFirstSlots+3*projectionPageSlots+7; i++ {
		if got := p.Append(record{&value, i}); got != i {
			t.Fatalf("index=%d want=%d", got, i)
		}
		if i == 0 {
			first = p.At(i)
		}
		if i == projectionFirstSlots {
			edge = p.At(i)
		}
	}
	for i := 0; i < p.Len(); i++ {
		if r := p.At(i); r.value != i || r.pointer != &value {
			t.Fatalf("corrupt record %d", i)
		}
	}
	if first != p.At(0) || edge != p.At(projectionFirstSlots) {
		t.Fatal("growth moved a published slot")
	}
	want := uint64(unsafe.Sizeof(*p.first)) + uint64(len(p.pages))*uint64(unsafe.Sizeof(*p.pages[0])) + uint64(cap(p.pages))*uint64(unsafe.Sizeof(p.pages[0]))
	if p.FootprintBytes() != want {
		t.Fatalf("footprint=%d want=%d", p.FootprintBytes(), want)
	}
	p.Clear()
	if p.Len() != 0 || p.FootprintBytes() != want || *first != (record{}) || *edge != (record{}) {
		t.Fatal("clear retained records or lost capacity accounting")
	}
	for _, page := range p.pages {
		for _, r := range page {
			if r != (record{}) {
				t.Fatal("retained pointer in unused page")
			}
		}
	}
	if allocs := testing.AllocsPerRun(100, func() {
		for i := 0; i < projectionFirstSlots+3*projectionPageSlots+7; i++ {
			p.Append(record{&value, i})
		}
		p.Clear()
	}); allocs != 0 {
		t.Fatalf("warm reuse allocations=%v", allocs)
	}
	for _, index := range []int{-1, 0, projectionFirstSlots} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("invalid index %d did not panic", index)
				}
			}()
			p.At(index)
		}()
	}
}

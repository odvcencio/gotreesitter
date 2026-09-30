package lexproof

import "testing"

func TestIdentitySnapshotsParametersWithoutAliases(t *testing.T) {
	type parameters struct {
		symbols [2]uint16
		order   []int
		enabled bool
	}
	p := parameters{symbols: [2]uint16{1, 2}, order: []int{0, -1}, enabled: true}
	first, ok := Snapshot(p)
	if !ok {
		t.Fatal("small scanner parameters declined")
	}
	same, ok := Snapshot(p)
	if !ok || first != same {
		t.Fatal("stable parameters changed identity")
	}
	p.order[0]++
	changed, ok := Snapshot(p)
	if !ok || first == changed {
		t.Fatal("mutated slice aliased snapshot")
	}
	p.order = nil
	nilSlice, _ := Snapshot(p)
	p.order = []int{}
	emptySlice, _ := Snapshot(p)
	if nilSlice == emptySlice {
		t.Fatal("nil and non-nil slices collapsed")
	}
	p.order = make([]int, 1, 1)
	narrow, _ := Snapshot(p)
	p.order = make([]int, 1, 2)
	wide, _ := Snapshot(p)
	if narrow == wide {
		t.Fatal("different slice capacities or owners collapsed")
	}
	a, _ := Snapshot(uint16(1))
	b, _ := Snapshot(uint32(1))
	if a == b {
		t.Fatal("different receiver types collapsed")
	}
	if allocs := testing.AllocsPerRun(100, func() { Snapshot(&p) }); allocs != 0 {
		t.Fatalf("snapshot allocations=%g", allocs)
	}
}

func TestIdentityDeclinesUnboundedOrOpaqueParameters(t *testing.T) {
	type link struct{ next *link }
	cycle := &link{}
	cycle.next = cycle
	for _, value := range []any{cycle, make([]int, 100), [1000]struct{}{}, func() {}, map[int]int{1: 2}, 0.1} {
		if _, ok := Snapshot(value); ok {
			t.Fatalf("opaque or unbounded parameters %T accepted", value)
		}
	}
}

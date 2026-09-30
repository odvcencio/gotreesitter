package incr

import "testing"

func TestReachesFrontierAcrossHiddenReductions(t *testing.T) {
	entries := []Entry{{State: 3, Present: true}, {State: 4, Present: true}, {State: 1}}
	entryAt := func(index int) (Entry, bool) {
		if index >= len(entries) {
			return Entry{}, false
		}
		return entries[index], true
	}
	reduction := func(state uint16) (Reduction, bool) {
		switch state {
		case 3:
			return Reduction{Symbol: 11, Children: 2}, true
		case 5:
			return Reduction{Symbol: 12, Children: 1}, true
		}
		return Reduction{}, false
	}
	gotoState := func(state, symbol uint16) uint16 {
		if state == 1 && symbol == 11 {
			return 5
		}
		if state == 1 && symbol == 12 {
			return 7
		}
		return 0
	}
	if !ReachesFrontier(3, 7, entryAt, reduction, gotoState) {
		t.Fatal("two hidden reductions did not reach the recorded leaf frontier")
	}
	if entries[0].State != 3 || entries[1].State != 4 || entries[2].Present {
		t.Fatal("frontier proof changed the original stack")
	}
	if allocations := testing.AllocsPerRun(10, func() {
		if !ReachesFrontier(3, 7, entryAt, reduction, gotoState) {
			panic("frontier changed")
		}
	}); allocations != 0 {
		t.Fatalf("frontier proof allocated %g times", allocations)
	}
	for _, target := range []uint16{0, 6, 8} {
		if ReachesFrontier(3, target, entryAt, reduction, gotoState) {
			t.Fatalf("reached unrelated frontier %d", target)
		}
	}
	entries[0].Extra = true
	if ReachesFrontier(3, 7, entryAt, reduction, gotoState) {
		t.Fatal("extra attachment needs a stronger proof")
	}
	entries[0].Extra = false
	entries[1].Present = false
	if ReachesFrontier(3, 7, entryAt, reduction, gotoState) {
		t.Fatal("reduction popped the initial state sentinel")
	}
}

func TestReachesFrontierFailsClosedOnCycle(t *testing.T) {
	entryAt := func(index int) (Entry, bool) {
		return Entry{State: 1, Present: index == 0}, index < 2
	}
	for _, children := range []int{0, 1} {
		if ReachesFrontier(1, 2, entryAt, func(uint16) (Reduction, bool) {
			return Reduction{Symbol: 3, Children: children}, true
		}, func(uint16, uint16) uint16 { return 1 }) {
			t.Fatal("cyclic reductions proved an unreachable frontier")
		}
	}
}

package ccore

import (
	"fmt"
	"testing"
)

func rg(s, e uint32) Range { return Range{StartByte: s, EndByte: e, StartCol: s, EndCol: e} }

func showR(rs []Range) string {
	out := ""
	for _, r := range rs {
		out += fmt.Sprintf("[%d-%d]", r.StartByte, r.EndByte)
	}
	return out
}

// C/get_changed_ranges.c:10 ts_range_array_add: a range that starts at or before the end of the last
// one merges with it; an empty range is dropped.
func TestRangeArrayAddMergesAndDropsEmpty(t *testing.T) {
	var a rangeArray
	l := func(b uint32) Length { return Length{bytes: b, col: b} }
	a.add(l(5), l(5)) // empty: dropped
	a.add(l(2), l(4))
	a.add(l(4), l(6)) // touches: merged
	a.add(l(5), l(9)) // overlaps: end becomes 9
	a.add(l(10), l(12))
	a.add(l(12), l(14)) // starts at the end of the last: merged, the end is the new end
	if got := showR(a); got != "[2-9][10-14]" {
		t.Errorf("got %s, want [2-9][10-14]", got)
	}
	if a[0].EndCol != 9 {
		t.Errorf("merged range end point %d, want 9", a[0].EndCol)
	}
}

// C/get_changed_ranges.c:30 ts_range_array_intersects.
func TestRangeArrayIntersects(t *testing.T) {
	a := rangeArray{rg(2, 4), rg(8, 10)}
	cases := []struct {
		from       int
		start, end uint32
		want       bool
	}{{0, 0, 2, false}, {0, 3, 5, true}, {0, 4, 8, false}, {0, 9, 20, true}, {1, 3, 5, false}, {2, 0, 99, false}}
	for _, c := range cases {
		if got := a.intersects(c.from, c.start, c.end); got != c.want {
			t.Errorf("intersects(from %d, %d, %d) = %v, want %v", c.from, c.start, c.end, got, c.want)
		}
	}
}

// C/get_changed_ranges.c:46 ts_range_array_get_changed_ranges: the stretches in one list and not the other.
func TestRangeArrayChanged(t *testing.T) {
	cases := []struct {
		name     string
		old, new []Range
		want     string
	}{
		{"equal", []Range{rg(0, 5)}, []Range{rg(0, 5)}, ""},
		{"shorter", []Range{rg(0, 5)}, []Range{rg(0, 3)}, "[3-5]"},
		{"longer", []Range{rg(0, 3)}, []Range{rg(0, 5)}, "[3-5]"},
		{"moved", []Range{rg(0, 3)}, []Range{rg(2, 5)}, "[0-2][3-5]"},
		{"added one", []Range{rg(0, 3)}, []Range{rg(0, 3), rg(6, 8)}, "[6-8]"},
		{"none to one", nil, []Range{rg(1, 4)}, "[1-4]"},
		{"one to none", []Range{rg(1, 4)}, nil, "[1-4]"},
		{"touching pieces merge", []Range{rg(0, 2), rg(2, 4)}, []Range{rg(0, 4)}, ""},
	}
	for _, c := range cases {
		var d rangeArray
		rangeArrayChanged(c.old, c.new, &d)
		if got := showR(d); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// An absent parser range list means all input, not the empty set.
func TestRangeArrayChangedDefaultIncludesAll(t *testing.T) {
	full := []Range{{EndByte: ^uint32(0), EndRow: ^uint32(0), EndCol: ^uint32(0)}}
	var got rangeArray
	rangeArrayChangedEffective(nil, full, &got)
	if len(got) != 0 {
		t.Fatalf("nil versus full: %v", got)
	}
	got = nil
	rangeArrayChangedEffective(nil, []Range{rg(2, 5)}, &got)
	if len(got) != 2 || got[0] != rg(0, 2) || got[1] != (Range{StartByte: 5, EndByte: ^uint32(0), StartCol: 5, EndRow: ^uint32(0), EndCol: ^uint32(0)}) {
		t.Fatalf("default versus [2-5]: %v", got)
	}
}

// A deep/fragmented comparison must not pin oversized scratch in the pool.
func TestChangedRangesScratchRetentionBound(t *testing.T) {
	sc := &crScratch{
		oldIt:   crIter{s: &Store{}, tab: &Tables{}, stack: make([]crEntry, 2, crMaxRetainedEntries+1)},
		newIt:   crIter{s: &Store{}, tab: &Tables{}, stack: make([]crEntry, 2, 4)},
		diffs:   make(rangeArray, 2, crMaxRetainedEntries+1),
		results: make(rangeArray, 2, 4),
	}
	sc.resetForPool()
	if sc.oldIt.s != nil || sc.oldIt.tab != nil || sc.newIt.s != nil || sc.newIt.tab != nil {
		t.Fatal("scratch retains a store or tables")
	}
	if sc.oldIt.stack != nil || sc.diffs != nil {
		t.Fatal("oversized scratch was retained")
	}
	if len(sc.newIt.stack) != 0 || cap(sc.newIt.stack) != 4 || len(sc.results) != 0 || cap(sc.results) != 4 {
		t.Fatal("small scratch was not reset and retained")
	}
}

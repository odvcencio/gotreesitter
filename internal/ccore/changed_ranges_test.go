package ccore_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// Tests of ChangedRanges (C/get_changed_ranges.c, C/tree.c:114 ts_tree_get_changed_ranges).
// The sources are one line, so a point is (0, byte). The equality with C is checked
// by cgo_harness/ccore_changed_ranges_c_test.go; these tests need no C library.

type crFix struct {
	p   *ccore.Parser
	tab *ccore.Tables
}

func newCRFix(t *testing.T) crFix {
	t.Helper()
	g := goldenNamed(t, "json-object")
	return crFix{p: mustParser(t, ccore.Config{Tables: g.Tables}), tab: g.Tables}
}

func (f crFix) parse(t *testing.T, src string, old *ccore.Tree) *ccore.Tree {
	t.Helper()
	tr, stop := f.p.ParseWith([]byte(src), old)
	if stop != "" || tr == nil {
		t.Fatalf("parse %q: stop %q", src, stop)
	}
	return tr
}

// step parses oldSrc, edits it (old tree edited as C requires), parses newSrc with it, and returns
// both trees and the changed ranges.
func (f crFix) step(t *testing.T, oldSrc, newSrc string, start, oldEnd, newEnd uint32) (oldT, newT *ccore.Tree, got []ccore.Range) {
	t.Helper()
	oldT = f.parse(t, oldSrc, nil)
	oldT.Edit(ccore.Edit{StartByte: start, OldEndByte: oldEnd, NewEndByte: newEnd,
		StartCol: start, OldEndCol: oldEnd, NewEndCol: newEnd})
	newT = f.parse(t, newSrc, oldT)
	got, ok := ccore.ChangedRanges(oldT, newT, f.tab, nil, nil)
	if !ok {
		t.Fatal("ChangedRanges declined two trees of one store")
	}
	return oldT, newT, got
}

func show(rs []ccore.Range) string {
	s := ""
	for _, r := range rs {
		s += fmt.Sprintf("[%d-%d (%d,%d)-(%d,%d)]", r.StartByte, r.EndByte, r.StartRow, r.StartCol, r.EndRow, r.EndCol)
	}
	return s
}

func bytesOf(rs []ccore.Range) [][2]uint32 {
	var out [][2]uint32
	for _, r := range rs {
		out = append(out, [2]uint32{r.StartByte, r.EndByte})
	}
	return out
}

// C/get_changed_ranges.c:413: two equal trees give no range.
func TestChangedRangesEqualTreesGiveNone(t *testing.T) {
	f := newCRFix(t)
	src := `{"a": 1, "b": [1, 2, 3]}`
	a := f.parse(t, src, nil)
	b := f.parse(t, src, nil) // a second tree in its own store
	if got, ok := ccore.ChangedRanges(a, b, f.tab, nil, nil); !ok || len(got) != 0 {
		t.Errorf("separate stores: ok %v, ranges %s; want none", ok, show(got))
	}
	cp := a.Copy()
	got, ok := ccore.ChangedRanges(a, cp, f.tab, nil, nil)
	if !ok || len(got) != 0 {
		t.Errorf("a tree and its copy: ok %v, ranges %s; want none", ok, show(got))
	}
	// A reparse with no edit is the same tree.
	c := f.parse(t, src, cp)
	got, ok = ccore.ChangedRanges(cp, c, f.tab, nil, nil)
	if !ok || len(got) != 0 {
		t.Errorf("unedited reparse: ok %v, ranges %s; want none", ok, show(got))
	}
}

// C/get_changed_ranges.c:471 (IteratorDiffers): a leaf that becomes a leaf of another symbol.
func TestChangedRangesChangedLeaf(t *testing.T) {
	f := newCRFix(t)
	_, _, got := f.step(t, `[1, 22, 3]`, `[1, "x", 3]`, 4, 6, 7)
	want := [][2]uint32{{4, 7}}
	if fmt.Sprint(bytesOf(got)) != fmt.Sprint(want) {
		t.Errorf("ranges %s, want %v", show(got), want)
	}
}

// C/get_changed_ranges.c:348 (iterator_compare, the has_changes test): a change that keeps every
// size. The array has the same symbol and size on both sides; only its edited flag makes C look
// inside, where the number became a string.
func TestChangedRangesSameSizeChangeNeedsTheEditedFlag(t *testing.T) {
	f := newCRFix(t)
	_, _, got := f.step(t, `[1, 22, 3]`, `[1, "", 3]`, 4, 6, 6)
	want := [][2]uint32{{4, 6}}
	if fmt.Sprint(bytesOf(got)) != fmt.Sprint(want) {
		t.Errorf("ranges %s, want %v", show(got), want)
	}
}

// C/get_changed_ranges.c:471 (IteratorMayDiffer): a leaf edited into a leaf of the same symbol
// and size is not a structural change; C reports nothing.
func TestChangedRangesSameSymbolLeafIsNotReported(t *testing.T) {
	f := newCRFix(t)
	_, _, got := f.step(t, `[1, 22, 3]`, `[1, 23, 3]`, 4, 6, 6)
	if len(got) != 0 {
		t.Errorf("ranges %s, want none", show(got))
	}
}

// C/get_changed_ranges.c:471: a change inside a hidden repeat node (the array's element list).
func TestChangedRangesInHiddenRepeat(t *testing.T) {
	f := newCRFix(t)
	oldSrc, newSrc := `[1, 2, 3, 4, 5, 6, 7, 8]`, `[1, 2, 3, 4, "x", 6, 7, 8]`
	_, _, got := f.step(t, oldSrc, newSrc, 13, 14, 16)
	if len(got) == 0 {
		t.Fatal("no range for a changed element")
	}
	for _, r := range got {
		if r.StartByte < 13 || r.EndByte > 16 {
			t.Errorf("range %s reaches outside the changed element [13-16]", show([]ccore.Range{r}))
		}
	}
}

// C/get_changed_ranges.c:437: an insertion shifts everything after it; the shifted part that did
// not change gets no range.
func TestChangedRangesInsertionDoesNotReportTheShiftedTail(t *testing.T) {
	f := newCRFix(t)
	oldSrc, newSrc := `[1, 2, 3, 4, 5, 6, 7, 8]`, `[1, 2, 99999, 3, 4, 5, 6, 7, 8]`
	_, _, got := f.step(t, oldSrc, newSrc, 7, 7, 14)
	if len(got) == 0 {
		t.Fatal("no range for an insertion")
	}
	last := got[len(got)-1]
	if last.EndByte > 14 {
		t.Errorf("ranges %s reach into the shifted tail (after byte 14)", show(got))
	}
	if got[0].StartByte < 7 {
		t.Errorf("ranges %s start before the insertion", show(got))
	}
}

// C/get_changed_ranges.c:348 (iterator_compare): an error node on the new side.
func TestChangedRangesErrorOnOneSide(t *testing.T) {
	f := newCRFix(t)
	_, newT, got := f.step(t, `[1, 2, 3]`, `[1, , 3]`, 4, 5, 4)
	if newT.Reader().ErrorCost(newT.Root()) == 0 {
		t.Fatal("the new tree has no error; the test needs one")
	}
	if len(got) == 0 {
		t.Fatal("no range for an error node")
	}
	for _, r := range got {
		if r.StartByte >= r.EndByte {
			t.Errorf("empty range %s", show([]ccore.Range{r}))
		}
	}
	// and back: the old tree has the error
	f2 := newCRFix(t)
	oldT, _, got2 := f2.step(t, `[1, , 3]`, `[1, 2, 3]`, 4, 4, 5)
	if oldT.Reader().ErrorCost(oldT.Root()) == 0 {
		t.Fatal("the old tree has no error; the test needs one")
	}
	if len(got2) == 0 {
		t.Fatal("no range when the error is repaired")
	}
}

// C/tree.c:114: a tree that was released, or an empty table, is declined (no answer, no panic).
func TestChangedRangesDeclines(t *testing.T) {
	f := newCRFix(t)
	oldT, newT, _ := f.step(t, `[1, 22, 3]`, `[1, "x", 3]`, 4, 6, 7)
	if _, ok := ccore.ChangedRanges(oldT, newT, nil, nil, nil); ok {
		t.Error("nil tables answered")
	}
	if _, ok := ccore.ChangedRanges(nil, newT, f.tab, nil, nil); ok {
		t.Error("nil tree answered")
	}
	oldT.Release()
	if _, ok := ccore.ChangedRanges(oldT, newT, f.tab, nil, nil); ok {
		t.Error("released tree answered")
	}
}

// The result is a copy: a second call, which reuses the scratch, does not change a result the
// caller kept.
func TestChangedRangesResultIsACopy(t *testing.T) {
	f := newCRFix(t)
	oldT, newT, first := f.step(t, `[1, 22, 3]`, `[1, "x", 3]`, 4, 6, 7)
	keep := append([]ccore.Range(nil), first...)
	f2 := newCRFix(t)
	_, _, _ = f2.step(t, `{"a": [1, 2, 3], "b": {"c": 4}}`, `{"a": [1, 2, 3], "b": {"c": "dd"}}`, 26, 27, 30)
	if show(first) != show(keep) {
		t.Errorf("a kept result changed: %s -> %s", show(keep), show(first))
	}
	again, _ := ccore.ChangedRanges(oldT, newT, f.tab, nil, nil)
	if show(again) != show(first) {
		t.Errorf("same trees, second call: %s, first %s", show(again), show(first))
	}
}

// A fresh parse uses a separate store, including when incremental parsing falls back.
func TestChangedRangesEditedTreeAgainstFreshStore(t *testing.T) {
	f := newCRFix(t)
	old := f.parse(t, `[1, 22, 3]`, nil)
	old.Edit(ccore.Edit{StartByte: 4, OldEndByte: 6, NewEndByte: 7, StartCol: 4, OldEndCol: 6, NewEndCol: 7})
	fresh := f.parse(t, `[1, "x", 3]`, nil)
	got, ok := ccore.ChangedRanges(old, fresh, f.tab, nil, nil)
	if !ok || fmt.Sprint(bytesOf(got)) != fmt.Sprint([][2]uint32{{4, 7}}) {
		t.Fatalf("fresh store: ok %v, ranges %s; want [4-7]", ok, show(got))
	}
}

// A separate store remains readable after the original tree's retained copy is
// released; each tree owns its root independently.
func TestChangedRangesAfterIndependentRelease(t *testing.T) {
	f := newCRFix(t)
	original := f.parse(t, `[1, 22, 3]`, nil)
	old := original.Copy()
	if old == nil {
		t.Fatal("copy failed")
	}
	original.Release()
	old.Edit(ccore.Edit{StartByte: 4, OldEndByte: 6, NewEndByte: 7, StartCol: 4, OldEndCol: 6, NewEndCol: 7})
	fresh := f.parse(t, `[1, "x", 3]`, nil)
	defer old.Release()
	defer fresh.Release()
	got, ok := ccore.ChangedRanges(old, fresh, f.tab, nil, nil)
	if !ok || fmt.Sprint(bytesOf(got)) != fmt.Sprint([][2]uint32{{4, 7}}) {
		t.Fatalf("after independent release: ok %v, ranges %s", ok, show(got))
	}
}

func TestChangedRangesEmptyFreshStores(t *testing.T) {
	f := newCRFix(t)
	old := f.parse(t, ``, nil)
	fresh := f.parse(t, ``, nil)
	defer old.Release()
	defer fresh.Release()
	got, ok := ccore.ChangedRanges(old, fresh, f.tab, nil, nil)
	if !ok || len(got) != 0 {
		t.Fatalf("empty fresh stores: ok %v, ranges %s", ok, show(got))
	}
}

func TestChangedRangesConcurrentReaders(t *testing.T) {
	f := newCRFix(t)
	old, incremental, _ := f.step(t, `[1, 22, 3]`, `[1, "x", 3]`, 4, 6, 7)
	defer old.Release()
	defer incremental.Release()
	fresh := f.parse(t, `[1, "x", 3]`, nil)
	defer fresh.Release()

	want := fmt.Sprint([][2]uint32{{4, 7}})
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				ranges, ok := ccore.ChangedRanges(old, fresh, f.tab, nil, nil)
				if got := fmt.Sprint(bytesOf(ranges)); !ok || got != want {
					errs <- fmt.Sprintf("ok %v, ranges %s; want %s", ok, got, want)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

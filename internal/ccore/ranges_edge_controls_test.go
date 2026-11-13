package ccore

import "testing"

func TestChangedRangesExternalScannerStateComparison(t *testing.T) {
	p := newTestParser(t)
	s := &p.Store
	stateA := addStateLeaf(s, 20, 12)
	stateB := addStateLeaf(s, 21, 12)
	root := addStateLeaf(s, 3, 12)
	s.rec(root).flags |= heapVisible | heapNamed
	var a, b crIter
	a.reset(s, p.t, root)
	b.reset(s, p.t, root)
	a.prevExternalTok = stateA
	b.prevExternalTok = stateB
	if extStateEqual(s, s, stateA, stateB) {
		t.Fatal("different scanner states compared equal")
	}
	if got := crCompare(&a, &b); got != iterMayDiffer {
		t.Fatalf("different states: got %v", got)
	}
	b.prevExternalTok = stateA
	if !extStateEqual(s, s, stateA, stateA) {
		t.Fatal("identical states differ")
	}
	if got := crCompare(&a, &b); got != iterMatches {
		t.Fatalf("same state: got %v", got)
	}
}
func TestChangedRangesAliasDifference(t *testing.T) {
	p := newTestParser(t)
	s := &p.Store
	leaf := p.newLeaf(1, Length{}, Length{bytes: 1, col: 1}, 0, 0, false, false, false)
	kids := s.addKids([]Handle{leaf})
	parentA := s.add(rec{symbol: 5, productionID: 1, childrenStart: kids, childCount: 1, flags: heapVisible, size: Length{bytes: 1, col: 1}})
	parentB := s.add(rec{symbol: 5, productionID: 2, childrenStart: kids, childCount: 1, flags: heapVisible, size: Length{bytes: 1, col: 1}})
	tab := &Tables{Alias: [][]Symbol{nil, {7}, {8}}}
	a := crIter{s: s, tab: tab, visibleDepth: 2, stack: []crEntry{{h: parentA}, {h: leaf}}}
	b := crIter{s: s, tab: tab, visibleDepth: 2, stack: []crEntry{{h: parentB}, {h: leaf}}}
	if got := crCompare(&a, &b); got != iterDiffers {
		t.Fatalf("different aliases: got %v", got)
	}
	b.stack[0].h = parentA
	if got := crCompare(&a, &b); got != iterMatches {
		t.Fatalf("same alias: got %v", got)
	}
}
func TestChangedRangesVisibleZeroWidthAndMissingTerminate(t *testing.T) {
	leaf := func(missing bool) *Tree {
		h := Handle(inlInline | inlVisible | inlNamed | (uint64(1) << 8))
		if missing {
			h |= Handle(inlMissing)
		}
		c := &cell{}
		c.root.Store(uint64(h))
		return &Tree{c: c}
	}
	normal, missing := leaf(false), leaf(true)
	for _, pair := range [][2]*Tree{{normal, missing}, {missing, normal}} {
		if _, ok := ChangedRanges(pair[0], pair[1], &Tables{}, nil, nil); !ok {
			t.Fatal("visible zero-width comparison declined")
		}
	}
}

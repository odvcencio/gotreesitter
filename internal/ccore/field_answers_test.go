package ccore

import "testing"

func TestFieldAnswersOccurrenceAndExplicitMiss(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, i := range []int{1, 2, 3, 5} {
		meta[i] = SymbolMeta{Visible: true, Named: true}
	}
	p := nodeTestParser(t, meta, nil)
	inner := p.newNode(2, []Handle{nodeLeaf(p, 3)}, 2)
	root := p.newNode(5, []Handle{inner}, 1)
	nt := &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}, {1, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0, Inherited: true}, {FieldID: 9, ChildIndex: 0}}}
	r := NewNodeReader(&p.Store, nt)
	var edges [4]FieldEdgeAnswer
	var answers [3]FieldLookupAnswer
	var paths [16]uint32
	e, a, ps, ok := r.FieldAnswers(r.RootNode(root), []uint16{9, 8, 0}, 100, edges[:0], answers[:0], paths[:0])
	if !ok || len(e) != 1 || !a[0].Found || a[1].Found || a[2].Found || len(ps) != 2 || ps[0] != 0 || ps[1] != 0 {
		t.Fatalf("edges=%+v answers=%+v paths=%v ok=%v", e, a, ps, ok)
	}
	if _, _, _, ok = r.FieldAnswers(r.RootNode(root), []uint16{9}, 100, edges[:0], answers[:0], nil); ok {
		t.Fatal("path capacity must decline")
	}
	if _, _, _, ok = r.FieldAnswers(r.RootNode(root), []uint16{9}, 1, edges[:0], answers[:0], paths[:0]); ok {
		t.Fatal("work limit must decline")
	}
}
func TestFieldAnswersNoAllocation(t *testing.T) {
	meta := make([]SymbolMeta, 8)
	meta[1] = SymbolMeta{Visible: true, Named: true}
	meta[5] = SymbolMeta{Visible: true, Named: true}
	p := nodeTestParser(t, meta, nil)
	root := p.newNode(5, []Handle{nodeLeaf(p, 1)}, 1)
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0}}})
	n := r.RootNode(root)
	var e [2]FieldEdgeAnswer
	var a [2]FieldLookupAnswer
	var path [8]uint32
	fields := []uint16{9, 0}
	alloc := testing.AllocsPerRun(100, func() {
		_, _, _, ok := r.FieldAnswers(n, fields, 100, e[:0], a[:0], path[:0])
		if !ok {
			panic("failed")
		}
	})
	if alloc != 0 {
		t.Fatalf("allocs=%g", alloc)
	}
	t.Logf("caller scratch allocations=%g", alloc)
}
func TestFieldEdgeExplicitZeroCursor(t *testing.T) {
	p := nodeTestParser(t, namedTestMeta(), nil)
	h := p.newNode(6, []Handle{nodeLeaf(p, 3)}, 0)
	a := p.newNode(2, []Handle{h}, 0)
	root := p.newNode(5, []Handle{nodeLeaf(p, 1), a, nodeLeaf(p, 4)}, 1)
	nt := &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 1}}, SuperBit: make([]uint32, 16)}
	nt.SuperBit[6] = 2
	r := NewNodeReader(&p.Store, nt)
	child, ok := r.NamedChild(r.RootNode(root), 1)
	if !ok {
		t.Fatal("missing child")
	}
	edge := FieldEdgeOf(child)
	if edge.Field != 9 || edge.CursorField != 0 {
		t.Fatalf("edge=%+v", edge)
	}
}

func TestFieldAnswersWideOnePassAndBound(t *testing.T) {
	const n = 1024
	meta := make([]SymbolMeta, 8)
	meta[1] = SymbolMeta{Visible: true, Named: true}
	meta[5] = SymbolMeta{Visible: true, Named: true}
	p := nodeTestParser(t, meta, nil)
	leaves := make([]Handle, n)
	for i := range leaves {
		leaves[i] = nodeLeaf(p, 1)
	}
	root := p.newNode(5, leaves, 0)
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t})
	parent := r.RootNode(root)
	edges := make([]FieldEdgeAnswer, n)
	if _, _, _, ok := r.FieldAnswers(parent, nil, n-1, edges[:0], nil, nil); ok {
		t.Fatal("must refuse before completing 1024 child visits")
	}
	es, _, _, ok := r.FieldAnswers(parent, nil, n, edges[:0], nil, nil)
	if !ok || len(es) != n {
		t.Fatalf("wide len=%d ok=%t", len(es), ok)
	}
	for i, got := range es {
		ch, ok := r.Child(parent, uint32(i))
		if !ok || got != FieldEdgeOf(ch) {
			t.Fatalf("edge %d", i)
		}
	}
	alloc := testing.AllocsPerRun(100, func() {
		_, _, _, ok := r.FieldAnswers(parent, nil, n, edges[:0], nil, nil)
		if !ok {
			panic("declined")
		}
	})
	if alloc != 0 {
		t.Fatalf("allocs=%g", alloc)
	}
	t.Logf("children=%d required_child_visits=%d repeated_child_visits=%d allocations=%g", n, n, n*(n+1)/2, alloc)
}

func TestFieldAnswersRejectsMalformedEdgeTables(t *testing.T) {
	meta := make([]SymbolMeta, 8)
	meta[1] = SymbolMeta{Visible: true, Named: true}
	meta[5] = SymbolMeta{Visible: true, Named: true}
	p := nodeTestParser(t, meta, nil)
	leaf := nodeLeaf(p, 1)
	root := p.newNode(5, []Handle{leaf}, 1)
	zero := p.newNode(5, []Handle{leaf}, 0)
	hidden := p.newNode(6, []Handle{leaf}, 1)
	nested := p.newNode(5, []Handle{hidden}, 0)
	var edge [4]FieldEdgeAnswer
	tests := []struct {
		name    string
		root    Handle
		slices  [][2]uint16
		entries []FieldEntry
		want    bool
	}{
		{"zero-production-absent-slice", zero, nil, nil, true},
		{"nonzero-production-missing", root, nil, nil, false},
		{"hidden-production-missing", nested, nil, nil, false},
		{"slice-start-out-of-range", root, [][2]uint16{{}, {2, 0}}, nil, false},
		{"slice-end-out-of-range", root, [][2]uint16{{}, {0, 2}}, []FieldEntry{{FieldID: 9}}, false},
		{"valid-empty-slice", root, [][2]uint16{{}, {0, 0}}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: tc.slices, FieldEntries: tc.entries})
			_, _, _, ok := r.FieldAnswers(r.RootNode(tc.root), nil, 100, edge[:0], nil, nil)
			if ok != tc.want {
				t.Fatalf("ok=%t want=%t", ok, tc.want)
			}
		})
	}
}

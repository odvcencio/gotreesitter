package ccore

import "testing"

func TestFieldLookupPathScratch(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, i := range []int{1, 2, 3, 5} {
		meta[i] = SymbolMeta{Visible: true, Named: true}
	}
	p := nodeTestParser(t, meta, nil)
	inner := p.newNode(2, []Handle{nodeLeaf(p, 3)}, 2)
	root := p.newNode(5, []Handle{inner}, 1)
	nt := &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}, {1, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0, Inherited: true}, {FieldID: 9, ChildIndex: 0}}}
	r := NewNodeReader(&p.Store, nt)
	n := r.RootNode(root)
	path, found, ok := r.FieldLookupPath(n, 9, 100, make([]uint32, 0, 64))
	if !ok || !found || len(path) != 2 || path[0] != 0 || path[1] != 0 {
		t.Fatalf("nested %v %v %v", path, found, ok)
	}
	_, found, ok = r.FieldLookupPath(n, 8, 100, make([]uint32, 0, 64))
	if !ok || found {
		t.Fatal("miss")
	}
	_, _, ok = r.FieldLookupPath(n, 9, 1, make([]uint32, 0, 64))
	if ok {
		t.Fatal("budget must fail closed")
	}
}

func TestFieldLookupPathHiddenAliasAndRepeated(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, i := range []int{1, 2, 3, 5, 7} {
		meta[i] = SymbolMeta{Visible: true, Named: true}
	}
	t.Run("hidden flatten ordinal", func(t *testing.T) {
		p := nodeTestParser(t, meta, nil)
		hidden := p.newNode(6, []Handle{nodeLeaf(p, 1), nodeLeaf(p, 3)}, 0)
		root := p.newNode(5, []Handle{hidden}, 1)
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0}}})
		path, found, ok := r.FieldLookupPath(r.RootNode(root), 9, 100, make([]uint32, 0, 64))
		if !ok || !found || len(path) != 1 || path[0] != 0 {
			t.Fatalf("%v %v %v", path, found, ok)
		}
	})
	t.Run("alias hidden", func(t *testing.T) {
		alias := make([][]Symbol, 2)
		alias[1] = []Symbol{7}
		p := nodeTestParser(t, meta, alias)
		hidden := p.newNode(6, []Handle{nodeLeaf(p, 1)}, 0)
		root := p.newNode(5, []Handle{hidden}, 1)
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0}}})
		path, found, ok := r.FieldLookupPath(r.RootNode(root), 9, 100, make([]uint32, 0, 64))
		if !ok || !found || len(path) != 1 || path[0] != 0 {
			t.Fatalf("%v %v %v", path, found, ok)
		}
		c, _ := r.Child(r.RootNode(root), 0)
		if r.Symbol(c) != 7 {
			t.Fatalf("alias symbol %d", r.Symbol(c))
		}
	})
	t.Run("repeated field chooses later entry", func(t *testing.T) {
		p := nodeTestParser(t, meta, nil)
		empty := p.newNode(2, []Handle{nodeLeaf(p, 1)}, 0)
		root := p.newNode(5, []Handle{empty, nodeLeaf(p, 3)}, 1)
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 2}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0, Inherited: true}, {FieldID: 9, ChildIndex: 1}}})
		path, found, ok := r.FieldLookupPath(r.RootNode(root), 9, 100, make([]uint32, 0, 64))
		if !ok || !found || len(path) != 1 || path[0] != 1 {
			t.Fatalf("%v %v %v", path, found, ok)
		}
	})
}

func TestFieldLookupPathBoundsAndOccurrences(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, i := range []int{1, 2, 3, 5} {
		meta[i] = SymbolMeta{Visible: true, Named: true}
	}
	t.Run("same handle different slot", func(t *testing.T) {
		p := nodeTestParser(t, meta, nil)
		h := nodeLeaf(p, 1)
		root := p.newNode(5, []Handle{h, h}, 1)
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 1}}})
		a, _ := r.Child(r.RootNode(root), 0)
		b, _ := r.Child(r.RootNode(root), 1)
		if a.H != b.H {
			t.Fatal("fixture requires same handle")
		}
		var dst [64]uint32
		path, found, ok := r.FieldLookupPath(r.RootNode(root), 9, 100, dst[:0])
		if !ok || !found || len(path) != 1 || path[0] != 1 {
			t.Fatalf("%v %v %v", path, found, ok)
		}
	})
	t.Run("depth and scratch bounds", func(t *testing.T) {
		p := nodeTestParser(t, meta, nil)
		h := nodeLeaf(p, 1)
		for i := 0; i < 70; i++ {
			h = p.newNode(2, []Handle{h}, 1)
		}
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0, Inherited: true}}})
		var dst [128]uint32
		_, _, ok := r.FieldLookupPath(r.RootNode(h), 9, 100000, dst[:0])
		if ok {
			t.Fatal("deep path must decline")
		}
		h2 := p.newNode(2, []Handle{nodeLeaf(p, 1)}, 2)
		r.T.FieldSlices = append(r.T.FieldSlices, [2]uint16{1, 1})
		r.T.FieldEntries = append(r.T.FieldEntries, FieldEntry{FieldID: 9, ChildIndex: 0})

		_, _, ok = r.FieldLookupPath(r.RootNode(h2), 9, 100, nil)
		if ok {
			t.Fatal("empty destination scratch must decline")
		}
	})
	t.Run("bad field slice", func(t *testing.T) {
		p := nodeTestParser(t, meta, nil)
		root := p.newNode(5, []Handle{nodeLeaf(p, 1)}, 1)
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {9, 1}}})
		var dst [64]uint32
		_, _, ok := r.FieldLookupPath(r.RootNode(root), 9, 100, dst[:0])
		if ok {
			t.Fatal("corrupt table must decline")
		}
	})
}

func TestFieldLookupPathNoAllocation(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, i := range []int{1, 2, 5} {
		meta[i] = SymbolMeta{Visible: true, Named: true}
	}
	p := nodeTestParser(t, meta, nil)
	inner := p.newNode(2, []Handle{nodeLeaf(p, 1)}, 2)
	root := p.newNode(5, []Handle{inner}, 1)
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}, {1, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0, Inherited: true}, {FieldID: 9, ChildIndex: 0}}})
	var dst [64]uint32
	n := r.RootNode(root)
	allocs := testing.AllocsPerRun(100, func() {
		_, found, ok := r.FieldLookupPath(n, 9, 100, dst[:0])
		if !ok || !found {
			panic("lookup")
		}
	})
	if allocs != 0 {
		t.Fatalf("lookup allocations %.0f", allocs)
	}
	baseline := testing.AllocsPerRun(100, func() {
		_, ok := r.Child(n, 0)
		if !ok {
			panic("child")
		}
	})
	search := fieldPathSearch{r: r, remaining: 100}
	var raw [64]uint32
	rp, found := search.find(n.H, 9, raw[:0], 0)
	if !found || search.bad {
		t.Fatal("work fixture")
	}
	_, ok := search.project(n.H, rp, dst[:0], 0)
	if !ok || search.bad {
		t.Fatal("projection")
	}
	t.Logf("WORK lookup_ticks=%d lookup_allocs=%.0f baseline_child_allocs=%.0f", 100-search.remaining, allocs, baseline)
}

func TestFieldLookupPathMalformedAndEntryCap(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	meta[1] = SymbolMeta{Visible: true, Named: true}
	meta[5] = meta[1]
	p := nodeTestParser(t, meta, nil)
	leaf := nodeLeaf(p, 1)
	root := p.newNode(5, []Handle{leaf}, 1)
	n := NewNodeReader(&p.Store, &NodeTables{Base: p.t}).RootNode(root)
	var dst [64]uint32
	t.Run("64 entries valid", func(t *testing.T) {
		e := make([]FieldEntry, 64)
		for i := range e {
			e[i] = FieldEntry{FieldID: 9, ChildIndex: 0}
		}
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 64}}, FieldEntries: e})
		_, found, ok := r.FieldLookupPath(n, 9, 10000, dst[:0])
		if !ok || !found {
			t.Fatal("64 entries must fit")
		}
	})
	t.Run("65 entries decline", func(t *testing.T) {
		e := make([]FieldEntry, 65)
		for i := range e {
			e[i] = FieldEntry{FieldID: 9, ChildIndex: 0}
		}
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 65}}, FieldEntries: e})
		_, _, ok := r.FieldLookupPath(n, 9, 10000, dst[:0])
		if ok {
			t.Fatal("65 entries must decline")
		}
	})
	t.Run("out of range child index", func(t *testing.T) {
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 4}}})
		_, _, ok := r.FieldLookupPath(n, 9, 10000, dst[:0])
		if ok {
			t.Fatal("bad child index must decline")
		}
	})
	t.Run("unsorted same-field entries", func(t *testing.T) {
		r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 2}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 1}, {FieldID: 9, ChildIndex: 0}}})
		_, _, ok := r.FieldLookupPath(n, 9, 10000, dst[:0])
		if ok {
			t.Fatal("unsorted entries must decline")
		}
	})
}

// C's ts_node_child_by_field_id advances the child iterator after an inherited
// candidate misses, even when the next mapping has the same child index.
func TestFieldLookupPathDuplicateChildIndex(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, i := range []int{1, 2, 5} {
		meta[i] = SymbolMeta{Visible: true, Named: true}
	}
	p := nodeTestParser(t, meta, nil)
	child := p.newNode(2, []Handle{nodeLeaf(p, 1)}, 0)
	entries := []FieldEntry{{FieldID: 9, ChildIndex: 0, Inherited: true}, {FieldID: 9, ChildIndex: 0}}
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 2}}, FieldEntries: entries})
	var dst [64]uint32
	two := p.newNode(5, []Handle{child, nodeLeaf(p, 1)}, 1)
	path, found, ok := r.FieldLookupPath(r.RootNode(two), 9, 100, dst[:0])
	if !ok || !found || len(path) != 1 || path[0] != 1 {
		t.Fatalf("two children: %v %v %v", path, found, ok)
	}
	one := p.newNode(5, []Handle{child}, 1)
	path, found, ok = r.FieldLookupPath(r.RootNode(one), 9, 100, dst[:0])
	if !ok || found {
		t.Fatalf("one child: %v %v %v", path, found, ok)
	}
}
func TestFieldLookupPathRejectsUnsortedFieldIDs(t *testing.T) {
	meta := make([]SymbolMeta, 8)
	meta[1] = SymbolMeta{Visible: true, Named: true}
	meta[5] = meta[1]
	p := nodeTestParser(t, meta, nil)
	root := p.newNode(5, []Handle{nodeLeaf(p, 1)}, 1)
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 2}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 0}, {FieldID: 3, ChildIndex: 0}}})
	var dst [8]uint32
	for _, field := range []uint16{3, 9} {
		_, _, ok := r.FieldLookupPath(r.RootNode(root), field, 100, dst[:0])
		if ok {
			t.Fatalf("unsorted IDs must decline field=%d", field)
		}
	}
}

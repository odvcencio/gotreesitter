package ccore

import "testing"

// Trees built by hand: symbols 1 to 7, the productions named in each test.

func nodeTestParser(t *testing.T, meta []SymbolMeta, alias [][]Symbol) *Parser {
	t.Helper()
	p := &Parser{t: &Tables{Meta: meta, Alias: alias, TokenCount: 4}}
	p.Store = newStoreCore().view()
	p.beginParse(true)
	return p
}

func nodeLeaf(p *Parser, sym Symbol) Handle {
	return p.newLeaf(sym, Length{}, Length{bytes: 1, col: 1}, 0, 0, false, false, false)
}

// P1: ProductionID is 0 for a record with no children, as C's
// ts_subtree_production_id (C/subtree.h:343) is.
func TestProductionIDOfAChildlessRecordIsZero(t *testing.T) {
	p := nodeTestParser(t, make([]SymbolMeta, 16), nil)
	h := p.add(rec{symbol: 5, productionID: 7}) // a reduction of no children
	if got := p.rec(h).productionID; got != 7 {
		t.Fatalf("setup: the record keeps production %d, want 7", got)
	}
	if got := p.ProductionID(h); got != 0 {
		t.Fatalf("ProductionID of a childless record = %d, want 0", got)
	}
	parent := p.newNode(5, []Handle{nodeLeaf(p, 1)}, 3)
	if got := p.ProductionID(parent); got != 3 {
		t.Fatalf("ProductionID of a parent = %d, want 3", got)
	}
}

// P1: an alias to the built-in error symbol is named (C/language.c:215).
func TestAliasToTheErrorSymbolIsNamed(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	meta[5] = SymbolMeta{Visible: true, Named: true}
	alias := make([][]Symbol, 4)
	alias[1] = []Symbol{errorSymbol}
	p := nodeTestParser(t, meta, alias)
	root := p.newNode(5, []Handle{nodeLeaf(p, 3)}, 1) // symbol 3 is hidden; the production aliases it to ERROR
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t})
	rn := r.RootNode(root)
	if got := r.NamedChildCount(rn); got != 1 {
		t.Fatalf("setup: named child count %d, want 1", got)
	}
	c, ok := r.NamedChild(rn, 0)
	if !ok {
		t.Fatal("NamedChild(0) found nothing, though the alias is named")
	}
	if !r.Named(c) || !r.IsError(c) {
		t.Fatalf("aliased child: named %v, error %v", r.Named(c), r.IsError(c))
	}
}

// P1: the named walk enters an anonymous visible node that holds a named child
// (X named, A anonymous with field F, Z named; A holds B). C returns B for
// named_child(root, 1); B's visible parent is A, so its cursor field, mask and
// ordinal are those of a first child of A.
func TestNamedChildBelowAnonymousVisibleNodeStartsANewParent(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, s := range []int{1, 3, 4, 5} {
		meta[s] = SymbolMeta{Visible: true, Named: true}
	}
	meta[2] = SymbolMeta{Visible: true} // A
	p := nodeTestParser(t, meta, nil)
	b := p.newNode(2, []Handle{nodeLeaf(p, 3)}, 0) // A holds B
	root := p.newNode(5, []Handle{nodeLeaf(p, 1), b, nodeLeaf(p, 4)}, 1)
	nt := &NodeTables{
		Base:         p.t,
		FieldSlices:  [][2]uint16{{}, {0, 1}},
		FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 1}}, // A fills field 9
		SuperBit:     make([]uint32, 16),
	}
	nt.SuperBit[2] = 1 // A's symbol has a supertype bit
	r := NewNodeReader(&p.Store, nt)
	rn := r.RootNode(root)
	if got := r.NamedChildCount(rn); got != 2 {
		t.Fatalf("setup: named child count %d, want 2 (X and Z)", got)
	}
	c, ok := r.NamedChild(rn, 1)
	if !ok || r.Symbol(c) != 3 {
		t.Fatalf("NamedChild(1) = %v (symbol %d), want B (symbol 3)", ok, r.Symbol(c))
	}
	if c.CField != 0 || c.Mask != 0 || c.Ordinal != 0 {
		t.Fatalf("B read below A: cursor field %d, mask %d, ordinal %d; want 0, 0, 0", c.CField, c.Mask, c.Ordinal)
	}
	// the node-field answer keeps A's field, as ts_node_field_name_for_named_child does
	if c.Field != 9 {
		t.Fatalf("B's field = %d, want 9 (inherited from A)", c.Field)
	}
	// A is a direct child of the root and keeps its own field and ordinal
	a, ok := r.Child(rn, 1)
	if !ok || a.CField != 9 || a.Ordinal != 1 {
		t.Fatalf("A as the root's child: found %v, cursor field %d, ordinal %d; want 9 and 1", ok, a.CField, a.Ordinal)
	}
}

// The indexed and scalar readers allocate nothing.
func TestIndexedAndScalarReadersAllocateNothing(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	for _, s := range []int{1, 3, 4, 5} {
		meta[s] = SymbolMeta{Visible: true, Named: true}
	}
	meta[2] = SymbolMeta{Visible: true}
	p := nodeTestParser(t, meta, nil)
	b := p.newNode(2, []Handle{nodeLeaf(p, 3)}, 0)
	root := p.newNode(5, []Handle{nodeLeaf(p, 1), b, nodeLeaf(p, 4)}, 0)
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, SuperBit: make([]uint32, 16)})
	rn := r.RootNode(root)
	dst := make([]Node, 0, 8)
	var sink uint32
	cases := map[string]func(){
		"RootNode":        func() { sink += r.RootNode(root).Ordinal },
		"Child":           func() { c, _ := r.Child(rn, 1); sink += c.Ordinal },
		"NamedChild":      func() { c, _ := r.NamedChild(rn, 1); sink += c.Ordinal },
		"ChildCount":      func() { sink += r.ChildCount(rn) },
		"NamedChildCount": func() { sink += r.NamedChildCount(rn) },
		"scalars": func() {
			if r.Named(rn) || r.Extra(rn) || r.Missing(rn) || r.HasChanges(rn) || r.HasError(rn) || r.IsError(rn) {
				sink++
			}
			sink += r.EndByte(rn) + uint32(r.Symbol(rn)) + uint32(r.ParseState(rn))
		},
		"Children with room, shallow": func() { dst, _ = r.ChildrenHidden(rn, dst[:0]); sink += uint32(len(dst)) },
	}
	for name, f := range cases {
		if n := testing.AllocsPerRun(100, f); n != 0 {
			t.Errorf("%s allocates %v times per call", name, n)
		}
	}
	_ = sink
}

// Children allocates when the hidden nesting is deeper than its local stack.
func TestChildrenAllocatesBeyondSixteenHiddenLevels(t *testing.T) {
	meta := make([]SymbolMeta, 16)
	meta[5] = SymbolMeta{Visible: true, Named: true}
	meta[1] = SymbolMeta{Visible: true, Named: true}
	p := nodeTestParser(t, meta, nil)
	h := nodeLeaf(p, 1)
	for i := 0; i < 16; i++ {
		h = p.newNode(6, []Handle{h}, 0) // hidden wrapper
	}
	root := p.newNode(5, []Handle{h}, 0)
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, SuperBit: make([]uint32, 16)})
	rn := r.RootNode(root)
	dst := make([]Node, 0, 8)
	got, _ := r.ChildrenHidden(rn, dst)
	if len(got) != 1 || r.Symbol(got[0]) != 1 {
		t.Fatalf("Children found %d nodes", len(got))
	}
	if n := testing.AllocsPerRun(20, func() { dst, _ = r.ChildrenHidden(rn, dst[:0]) }); n == 0 {
		t.Fatal("expected Children to allocate with 16 hidden wrappers; the documented limit is wrong")
	}
	if c, ok := r.Child(rn, 0); !ok || r.Symbol(c) != 1 {
		t.Fatal("Child(0) did not find the leaf")
	}
	if n := testing.AllocsPerRun(20, func() { r.Child(rn, 0) }); n != 0 {
		t.Fatalf("Child allocates %v times at depth 16", n)
	}
}

func namedTestMeta() []SymbolMeta {
	meta := make([]SymbolMeta, 16)
	for _, s := range []int{1, 3, 4, 5} {
		meta[s] = SymbolMeta{Visible: true, Named: true}
	}
	meta[2] = SymbolMeta{Visible: true} // anonymous visible
	return meta                         // symbol 6 stays hidden
}

// A named walk that enters an anonymous visible node and then a hidden supertype below
// it: the visible boundary starts the ordinal, cursor field and mask again, and the hidden
// node's bit is then added (the node is read below A, through H). Two visible anonymous
// levels cannot be reached this way: an anonymous visible node does not pass its named
// children up in the count, so C's descent never enters an anonymous node inside another.
func TestNamedChildBelowAnonymousVisibleThenHiddenKeepsTheHiddenBit(t *testing.T) {
	p := nodeTestParser(t, namedTestMeta(), nil)
	h := p.newNode(6, []Handle{nodeLeaf(p, 3)}, 0)
	a := p.newNode(2, []Handle{h}, 0)
	root := p.newNode(5, []Handle{nodeLeaf(p, 1), a, nodeLeaf(p, 4)}, 1)
	nt := &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 9, ChildIndex: 1}}, SuperBit: make([]uint32, 16)}
	nt.SuperBit[6] = 2
	r := NewNodeReader(&p.Store, nt)
	c, ok := r.NamedChild(r.RootNode(root), 1)
	if !ok || r.Symbol(c) != 3 {
		t.Fatalf("NamedChild(1) = %v, symbol %d; want B", ok, r.Symbol(c))
	}
	if c.Mask != 2 || c.Ordinal != 0 || c.CField != 0 || c.Field != 9 {
		t.Fatalf("mask %d ordinal %d cursor field %d field %d; want 2, 0, 0, 9", c.Mask, c.Ordinal, c.CField, c.Field)
	}
}

// A descent through hidden nodes only keeps the mask, the ordinal among the visible
// children of the same parent and the cursor field.
func TestNamedChildThroughHiddenOnlyKeepsMaskOrdinalAndFields(t *testing.T) {
	p := nodeTestParser(t, namedTestMeta(), nil)
	h := p.newNode(6, []Handle{nodeLeaf(p, 3)}, 0)
	root := p.newNode(5, []Handle{nodeLeaf(p, 1), h}, 1)
	nt := &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, 1}}, FieldEntries: []FieldEntry{{FieldID: 7, ChildIndex: 1}}, SuperBit: make([]uint32, 16)}
	nt.SuperBit[6] = 2
	r := NewNodeReader(&p.Store, nt)
	c, ok := r.NamedChild(r.RootNode(root), 1)
	if !ok || r.Symbol(c) != 3 {
		t.Fatalf("NamedChild(1) = %v, symbol %d; want B", ok, r.Symbol(c))
	}
	if c.Mask != 2 || c.Ordinal != 1 || c.CField != 7 || c.Field != 7 {
		t.Fatalf("mask %d ordinal %d cursor field %d field %d; want 2, 1, 7, 7", c.Mask, c.Ordinal, c.CField, c.Field)
	}
}

// The boolean of ChildrenHidden: true when a hidden node with children passes a field down,
// false when no hidden node does (also when a hidden node carries no field).
func TestChildrenHiddenBoolean(t *testing.T) {
	p := nodeTestParser(t, namedTestMeta(), nil)
	nt := func(entries ...FieldEntry) *NodeTables {
		return &NodeTables{Base: p.t, FieldSlices: [][2]uint16{{}, {0, uint16(len(entries))}}, FieldEntries: entries, SuperBit: make([]uint32, 16)}
	}
	wrapped := p.newNode(6, []Handle{nodeLeaf(p, 1)}, 0) // hidden wrapper around a visible leaf
	withWrapper := p.newNode(5, []Handle{wrapped, nodeLeaf(p, 4)}, 1)
	flat := p.newNode(5, []Handle{nodeLeaf(p, 1), nodeLeaf(p, 4)}, 1)
	cases := []struct {
		name string
		root Handle
		t    *NodeTables
		want bool
	}{
		{"hidden wrapper that carries a field", withWrapper, nt(FieldEntry{FieldID: 3, ChildIndex: 0}), true},
		{"hidden wrapper without a field", withWrapper, nt(FieldEntry{FieldID: 3, ChildIndex: 1}), false},
		{"no hidden node, a field on a visible child", flat, nt(FieldEntry{FieldID: 3, ChildIndex: 0}), false},
	}
	for _, c := range cases {
		r := NewNodeReader(&p.Store, c.t)
		kids, hidden := r.ChildrenHidden(r.RootNode(c.root), nil)
		if hidden != c.want || len(kids) != 2 {
			t.Errorf("%s: hidden %v with %d children; want %v with 2", c.name, hidden, len(kids), c.want)
		}
	}
}

// A node with more than 4,096 children keeps its child list on a page of its own; every
// index reads the right child there.
func TestWideNodeUsesALargeListAndReadsEveryIndex(t *testing.T) {
	p := nodeTestParser(t, namedTestMeta(), nil)
	const n = 5000
	kids := make([]Handle, n)
	for i := range kids {
		kids[i] = p.newLeaf(1, Length{}, Length{bytes: uint32(i%200) + 1, col: uint32(i%200) + 1}, 0, 0, false, false, false)
	}
	root := p.newNode(5, kids, 0)
	r := NewNodeReader(&p.Store, &NodeTables{Base: p.t, SuperBit: make([]uint32, 16)})
	rn := r.RootNode(root)
	list, count := r.S.ChildSlots(root)
	if count != n || !LargeSlot(list) {
		t.Fatalf("setup: %d physical children, large list %v; want %d and true", count, LargeSlot(list), n)
	}
	if got := r.ChildCount(rn); got != n {
		t.Fatalf("ChildCount = %d, want %d", got, n)
	}
	all, _ := r.ChildrenHidden(rn, nil)
	if len(all) != n {
		t.Fatalf("Children returned %d nodes", len(all))
	}
	for i := uint32(0); i < n; i++ {
		c, ok := r.Child(rn, i)
		if !ok || c != all[i] || c.Ordinal != i || r.S.ChildHandle(list, count, i) != kids[i] {
			t.Fatalf("index %d: found %v, equal to Children %v, ordinal %d", i, ok, c == all[i], c.Ordinal)
		}
		if nc, ok := r.NamedChild(rn, i); !ok || nc != c {
			t.Fatalf("NamedChild(%d) differs from Child(%d)", i, i)
		}
	}
	if _, ok := r.Child(rn, n); ok {
		t.Fatal("Child(count) found a node")
	}
}

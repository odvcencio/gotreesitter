package parsercorephase0

import (
	"testing"
	"unsafe"
)

func TestSelectedStoreSupertypeRecordUsesExistingPadding(t *testing.T) {
	if got := unsafe.Sizeof(SelectedNodeRecord{}); got != 36 {
		t.Fatalf("record grew: %d", got)
	}
}

func TestSelectedStoreSupertypePolicyOwnershipAndFootprint(t *testing.T) {
	policy := selectedStoreTestPolicy(t, 2, 1)
	masks := make([]uint32, len(policy.Symbols))
	masks[3] = 1
	if err := policy.SetSupertypeMasks(masks); err != nil {
		t.Fatal(err)
	}
	masks[3] = 2
	if policy.supertypeMask(3) != 1 {
		t.Fatal("policy borrowed caller mask storage")
	}
	expected := uint64(unsafe.Sizeof(policy)) + uint64(cap(policy.Symbols))*uint64(unsafe.Sizeof(SelectedSymbolPolicy{})) + uint64(cap(policy.Unary))*uint64(unsafe.Sizeof(SelectedUnaryRule(0))) + uint64(cap(policy.SupertypeMasks))*4
	if policy.RetainedBytes() != expected {
		t.Fatalf("policy footprint=%d want=%d", policy.RetainedBytes(), expected)
	}
	c := newTinyCore(t, 4)
	before := c.FootprintBytes()
	c.selectedPolicy = &policy
	if c.FootprintBytes()-before != expected {
		t.Fatal("core omitted retained policy")
	}
	if err := policy.SetSupertypeMasks([]uint32{1}); err == nil {
		t.Fatal("partial policy accepted")
	}
	bad := make([]uint32, len(policy.Symbols))
	bad[3] = 3
	if err := policy.SetSupertypeMasks(bad); err == nil {
		t.Fatal("nonsingleton grammar bit accepted")
	}
	if err := policy.SetSupertypeMasks(nil); err != nil || policy.SupertypeMasks != nil {
		t.Fatal("zero-supertype policy allocated mask storage")
	}
}

func TestSelectedStoreSupertypeMasksSurviveHiddenElisionAndUnaryCollapse(t *testing.T) {
	for _, collapse := range []bool{false, true} {
		c := newTinyCore(t, 8)
		leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		inner, err := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{leaf}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		outer, err := c.appendSubtree(subtreeRecord{symbol: 4, endByte: 1}, []SubtreeID{inner}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		root, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{outer}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		policy := selectedStoreTestPolicy(t, 2, 1)
		masks := make([]uint32, len(policy.Symbols))
		masks[3] = 1
		masks[4] = 2
		if err := policy.SetSupertypeMasks(masks); err != nil {
			t.Fatal(err)
		}
		if collapse {
			for _, p := range []Symbol{3, 4} {
				policy.Unary[int(p)*len(policy.Symbols)+1] = SelectedUnaryPass
			}
		}
		store, err := c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
		if err != nil {
			t.Fatal(err)
		}
		rr, _ := store.Record(store.Root())
		child, _ := store.Child(rr, 0)
		if mask, ok := store.NodeSupertypeMask(child); !ok || mask != 3 {
			t.Fatalf("collapse=%v mask=%d valid=%v", collapse, mask, ok)
		}
		if mask, _ := store.NodeSupertypeMask(store.Root()); mask != 0 {
			t.Fatal("child ancestry leaked into visible parent")
		}
		store.Release()
		if _, ok := store.NodeSupertypeMask(child); ok {
			t.Fatal("released mask still available")
		}
	}
}

func TestSelectedStoreSupertypeVisibleAncestorIsBarrier(t *testing.T) {
	c := newTinyCore(t, 8)
	leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	visible, _ := c.appendSubtree(subtreeRecord{symbol: 5, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	hidden, _ := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{visible}, nil, nil)
	root, _ := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{hidden}, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1, 5)
	masks := make([]uint32, len(policy.Symbols))
	masks[3] = 1
	if err := policy.SetSupertypeMasks(masks); err != nil {
		t.Fatal(err)
	}
	store, err := c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	rr, _ := store.Record(store.Root())
	v, _ := store.Child(rr, 0)
	vr, _ := store.Record(v)
	l, _ := store.Child(vr, 0)
	if m, _ := store.NodeSupertypeMask(v); m != 1 {
		t.Fatal("visible child lost ancestry")
	}
	if m, _ := store.NodeSupertypeMask(l); m != 0 {
		t.Fatal("ancestry crossed visible boundary")
	}
}

func TestSelectedStoreSupertypeRepeatedPayloadKeepsOccurrenceContext(t *testing.T) {
	c := newTinyCore(t, 8)
	leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	hidden, _ := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	root, _ := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf, hidden}, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1)
	masks := make([]uint32, len(policy.Symbols))
	masks[3] = 1 << 31
	if err := policy.SetSupertypeMasks(masks); err != nil {
		t.Fatal(err)
	}
	store, err := c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	rr, _ := store.Record(store.Root())
	a, _ := store.Child(rr, 0)
	b, _ := store.Child(rr, 1)
	am, _ := store.NodeSupertypeMask(a)
	bm, _ := store.NodeSupertypeMask(b)
	if a == b || am != 0 || bm != 1<<31 {
		t.Fatalf("occurrence masks bleed: ids=%d/%d masks=%x/%x", a, b, am, bm)
	}
}

func TestSelectedStoreSupertypePromotedAliasFormsVisibleBarrier(t *testing.T) {
	c := newTinyCore(t, 8)
	leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	hidden, _ := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	root, _ := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{hidden}, nil, []Symbol{5})
	policy := selectedStoreTestPolicy(t, 2, 1, 5)
	masks := make([]uint32, len(policy.Symbols))
	masks[3] = 1
	if err := policy.SetSupertypeMasks(masks); err != nil {
		t.Fatal(err)
	}
	store, err := c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	rr, _ := store.Record(store.Root())
	a, _ := store.Child(rr, 0)
	ar, _ := store.Record(a)
	ch, _ := store.Child(ar, 0)
	if ar.Symbol != 5 || ar.SupertypeMask != 0 {
		t.Fatalf("promoted alias boundary=%+v", ar)
	}
	if m, _ := store.NodeSupertypeMask(ch); m != 0 {
		t.Fatal("promoted visible alias did not stop ancestry")
	}
}

func TestSelectedStoreSupertypeRetainedLeafAliasKeepsOccurrenceBoundary(t *testing.T) {
	for _, retained := range []bool{false, true} {
		c := newTinyCore(t, 8)
		leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
		hidden, _ := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{leaf}, nil, []Symbol{5})
		root, _ := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{hidden}, nil, nil)
		policy := selectedStoreTestPolicy(t, 2, 1, 5)
		masks := make([]uint32, len(policy.Symbols))
		masks[3] = 1
		if err := policy.SetSupertypeMasks(masks); err != nil {
			t.Fatal(err)
		}
		if retained {
			policy.SetRetainedAliasChildren([]SelectedAliasChildPair{{Alias: 5, Child: 1}})
		}
		store, err := c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
		if err != nil {
			t.Fatal(err)
		}
		rr, _ := store.Record(store.Root())
		a, _ := store.Child(rr, 0)
		ar, _ := store.Record(a)
		if ar.Symbol != 5 || ar.SupertypeMask != 1 {
			t.Fatalf("alias ancestor mask=%+v retained=%v", ar, retained)
		}
		if retained {
			ch, _ := store.Child(ar, 0)
			cr, ok := store.Record(ch)
			if !ok || cr.Symbol != 1 || cr.SupertypeMask != 0 {
				t.Fatalf("retained raw child=%+v", cr)
			}
		} else if ar.ChildCount != 0 {
			t.Fatal("nonretained leaf alias unexpectedly wrapped")
		}
		store.Release()
	}
}

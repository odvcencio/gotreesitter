package parsercorephase0

import "testing"

func TestSelectedSyntheticSiblingRootMatchesResultFraming(t *testing.T) {
	for _, fielded := range []bool{false, true} {
		c, err := New(&fakeTable{}, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, startByte: 1, endByte: 2, terminal: true, missing: true}, nil, nil, nil)
		var fields []FieldMapEntry
		if fielded {
			fields = []FieldMapEntry{{FieldID: 1, ChildIndex: 0}}
		}
		root, _ := c.appendSubtree(subtreeRecord{symbol: 2, productionID: 7, dynamicPrecedence: 5, startByte: 1, endByte: 2}, []SubtreeID{leaf}, fields, nil)
		extra, _ := c.appendSubtree(subtreeRecord{symbol: 3, startByte: 2, endByte: 3, terminal: true, extra: true}, nil, nil, nil)
		policy := selectedStoreTestPolicy(t, 2, 1, 3)
		policy.ProjectSyntheticSiblingRoot = true
		store, err := c.BuildSelectedStore([]SubtreeID{root, extra}, policy, []byte(" xx"), nil)
		if err != nil {
			t.Fatal(err)
		}
		r, _ := store.Record(store.Root())
		child, _ := store.Child(r, 0)
		cr, _ := store.Record(child)
		if r.ChildCount != 2 || !r.HasError() || r.StartByte != 1 || r.EndByte != 3 {
			t.Fatalf("root=%+v", r)
		}
		if fielded {
			if r.ProductionID != 7 || r.DynamicPrecedence != 5 || cr.Field != 1 {
				t.Fatalf("fielded root lost provenance: %+v %+v", r, cr)
			}
		} else {
			if r.ProductionID != 0 || r.DynamicPrecedence != 0 || cr.Field != 0 {
				t.Fatalf("synthetic root retained reduce provenance: %+v %+v", r, cr)
			}
		}
		store.Release()
	}
}
func TestSelectedSingleRootPreservesReduceProvenance(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	root, _ := c.appendSubtree(subtreeRecord{symbol: 2, productionID: 7, dynamicPrecedence: 5, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1)
	policy.ProjectSyntheticSiblingRoot = true
	store, err := c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	r, _ := store.Record(store.Root())
	if r.ProductionID != 7 || r.DynamicPrecedence != 5 {
		t.Fatalf("sole root reset: %+v", r)
	}
}

func TestSelectedSyntheticRootClearsOnlyRootSupertypeProvenance(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
	hiddenChild, _ := c.appendSubtree(subtreeRecord{symbol: 5, startByte: 1, endByte: 2}, []SubtreeID{leaf}, nil, nil)
	root, _ := c.appendSubtree(subtreeRecord{symbol: 2, productionID: 7, startByte: 1, endByte: 2}, []SubtreeID{hiddenChild}, nil, nil)
	hiddenRoot, _ := c.appendSubtree(subtreeRecord{symbol: 4, startByte: 1, endByte: 2}, []SubtreeID{root}, nil, nil)
	extra, _ := c.appendSubtree(subtreeRecord{symbol: 3, startByte: 2, endByte: 3, terminal: true, extra: true}, nil, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1, 3)
	masks := make([]uint32, len(policy.Symbols))
	masks[4] = 8
	masks[5] = 16
	if err := policy.SetSupertypeMasks(masks); err != nil {
		t.Fatal(err)
	}
	// The control proves the raw root carried the collapsed hidden ancestry.
	control, err := c.BuildSelectedStore([]SubtreeID{hiddenRoot, extra}, policy, []byte(" xx"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := control.Record(control.Root())
	if raw.SupertypeMask != 8 {
		t.Fatalf("control root mask=%d, want 8", raw.SupertypeMask)
	}
	control.Release()
	policy.ProjectSyntheticSiblingRoot = true
	store, err := c.BuildSelectedStore([]SubtreeID{hiddenRoot, extra}, policy, []byte(" xx"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	r, _ := store.Record(store.Root())
	child, _ := store.Child(r, 0)
	cr, _ := store.Record(child)
	if r.SupertypeMask != 0 || cr.SupertypeMask != 16 || r.ProductionID != 0 {
		t.Fatalf("synthetic provenance: root=%+v child=%+v", r, cr)
	}
}

func TestSelectedResultRootRetainedChildOwnsLeadingPadding(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
	root, _ := c.appendSubtree(subtreeRecord{symbol: 2, startByte: 1, endByte: 2}, []SubtreeID{leaf}, nil, nil)
	leading, _ := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1, terminal: true, extra: true}, nil, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1, 3)
	policy.ProjectSyntheticSiblingRoot = true
	store, err := c.BuildSelectedStore([]SubtreeID{leading, root}, policy, []byte(" x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	r, _ := store.Record(store.Root())
	if r.StartByte != 0 || r.EndByte != 2 {
		t.Fatalf("retained leading extra lost prefix: %+v", r)
	}
}

func TestSelectedResultRootDeclinesUnrepresentedPrefixOwnership(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := c.appendSubtree(subtreeRecord{symbol: 1, startByte: 2, endByte: 3, terminal: true}, nil, nil, nil)
	root, _ := c.appendSubtree(subtreeRecord{symbol: 2, startByte: 2, endByte: 3}, []SubtreeID{leaf}, nil, nil)
	policy := selectedStoreTestPolicy(t, 2, 1)
	policy.ProjectSyntheticSiblingRoot = true
	store, err := c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x x"), nil)
	if err == nil {
		store.Release()
		t.Fatal("unrepresented nontrivia prefix admitted")
	}
	store, err = c.BuildSelectedStore([]SubtreeID{root}, policy, []byte("  x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	r, _ := store.Record(store.Root())
	if r.StartByte != 2 {
		t.Fatalf("padding start=%d", r.StartByte)
	}
}

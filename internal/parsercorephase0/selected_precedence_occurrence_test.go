package parsercorephase0

import "testing"

// A hidden reduction contributes its delta for each raw occurrence, even
// though projection flattens it and never publishes the hidden node.
func TestSelectedRawPrecedenceHiddenRepeatedOccurrence(t *testing.T) {
	c, err := New(&selectedStorePolicyTable{policy: selectedStoreTestPolicy(t, 2, 1)}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1, dynamicPrecedence: -3}, []SubtreeID{leaf}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A raw child with two parents must fail closed, so give the second hidden
	// occurrence its own leaf as well.
	leaf2, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hidden2, err := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1, dynamicPrecedence: -3}, []SubtreeID{leaf2}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, dynamicPrecedence: 1}, []SubtreeID{hidden, hidden2}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sink nativeWalkTestSink
	replay := NativeProjectionReplay{Scratch: &MaterializationPostorderScratch{}, Transition: func(pre StateID, _ *MaterializationReplayView) (StateID, bool, error) { return pre + 1, true, nil }}
	ids, err := c.StreamNativeProjection([]SubtreeID{root}, []byte("x"), &sink, replay, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("roots=%v", ids)
	}
	r := sink.records[ids[0]-1]
	if r.DynamicPrecedence != -5 || r.ChildCount != 2 {
		t.Fatalf("root precedence=%d children=%d, want -5 and 2", r.DynamicPrecedence, r.ChildCount)
	}
	children := sink.children[ids[0]-1]
	if len(children) != 2 || children[0] == children[1] {
		t.Fatalf("repeated raw occurrence lost identity: %v", children)
	}
	shared, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{hidden, hidden}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.StreamNativeProjection([]SubtreeID{shared}, []byte("x"), &nativeWalkTestSink{}, replay, nil); err == nil {
		t.Fatal("shared raw subtree did not fail closed")
	}
}

// Collapsing a unary reduction preserves its raw cumulative precedence on
// the surviving occurrence; an extra child must retain its own occurrence.
func TestSelectedRawPrecedenceCollapsedUnaryAndExtra(t *testing.T) {
	for _, extra := range []bool{false, true} {
		t.Run(map[bool]string{false: "collapsed", true: "extra"}[extra], func(t *testing.T) {
			policy := selectedStoreTestPolicy(t, 2, 1)
			policy.Unary[2*len(policy.Symbols)+1] = SelectedUnaryPass
			c, err := New(&selectedStorePolicyTable{policy: policy}, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, extra: extra, endByte: 1, dynamicPrecedence: 2}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			root, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, dynamicPrecedence: -5}, []SubtreeID{leaf}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var sink nativeWalkTestSink
			replay := NativeProjectionReplay{Scratch: &MaterializationPostorderScratch{}, Transition: func(pre StateID, _ *MaterializationReplayView) (StateID, bool, error) { return pre + 1, true, nil }}
			ids, err := c.StreamNativeProjection([]SubtreeID{root}, []byte("x"), &sink, replay, nil)
			if err != nil {
				t.Fatal(err)
			}
			r := sink.records[ids[0]-1]
			if r.DynamicPrecedence != -3 {
				t.Fatalf("precedence=%d, want -3", r.DynamicPrecedence)
			}
			if extra && r.ChildCount != 1 {
				t.Fatal("extra occurrence was collapsed")
			}
		})
	}
}

func TestSelectedRawPrecedenceRetainedAliasWrapper(t *testing.T) {
	policy := selectedStoreTestPolicy(t, 2, 1, 3)
	policy.SetRetainedAliasChildren([]SelectedAliasChildPair{{Alias: 3, Child: 1}})
	c, err := New(&selectedStorePolicyTable{fakeTable: fakeTable{aliases: map[productionKey][]Symbol{{productionID: 0, childCount: 1}: {3}}}, policy: policy}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1, dynamicPrecedence: 2}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, dynamicPrecedence: -5}, []SubtreeID{leaf}, nil, []Symbol{3})
	if err != nil {
		t.Fatal(err)
	}
	var sink nativeWalkTestSink
	replay := NativeProjectionReplay{Scratch: &MaterializationPostorderScratch{}, Transition: func(pre StateID, _ *MaterializationReplayView) (StateID, bool, error) { return pre + 1, true, nil }}
	ids, err := c.StreamNativeProjection([]SubtreeID{root}, []byte("x"), &sink, replay, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := sink.records[ids[0]-1]
	if r.DynamicPrecedence != -3 || r.ChildCount != 1 {
		t.Fatalf("root=%+v", r)
	}
	wrapperID := sink.children[ids[0]-1][0]
	wrapper := sink.records[wrapperID-1]
	if wrapper.Symbol != 3 || wrapper.DynamicPrecedence != 2 || wrapper.ChildCount != 1 {
		t.Fatalf("wrapper=%+v", wrapper)
	}
	child := sink.records[sink.children[wrapperID-1][0]-1]
	if child.Symbol != 1 || child.DynamicPrecedence != 2 {
		t.Fatalf("child=%+v", child)
	}
}

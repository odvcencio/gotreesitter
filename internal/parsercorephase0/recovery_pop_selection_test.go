package parsercorephase0

import "testing"

func TestCRecoveryJoinedPathsPreserveChildBufferOwnership(t *testing.T) {
	c := newCSelectionCompareTestCore(t)
	c.diagnostics.cSubtreeSelectionCertified = true
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ids [3]SubtreeID
	for i, symbol := range []Symbol{4, 2, 3} {
		ids[i], err = c.appendSubtree(subtreeRecord{symbol: symbol}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	paths := []popPath{{prev: seed.Node, children: []SubtreeID{ids[0]}}, {prev: seed.Node, children: []SubtreeID{ids[1]}}, {prev: seed.Node, children: []SubtreeID{ids[2]}}}
	result, err := c.selectCRecoveryJoinedReductionPaths(paths, func(NodeID, SubtreeID) (uint32, error) { return 0, nil })
	if err != nil || len(result) != 1 || result[0].children[0] != ids[1] {
		t.Fatalf("selected paths=%+v err=%v", result, err)
	}
	for i := range paths {
		for j := i + 1; j < len(paths); j++ {
			if &paths[i].children[0] == &paths[j].children[0] {
				t.Fatal("selection aliased reusable child buffers")
			}
		}
	}
	paths[1].children[0] = ids[0]
	paths[2].children[0] = ids[0]
	if result[0].children[0] != ids[1] {
		t.Fatal("discarded path reuse changed selected children")
	}
}

func TestCRecoveryJoinedPathsSelectLowestCostAndIncomingErrorTie(t *testing.T) {
	c := newCSelectionCompareTestCore(t)
	c.diagnostics.cSubtreeSelectionCertified = true
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	paths := []popPath{{prev: seed.Node, children: []SubtreeID{1}}, {prev: seed.Node, children: []SubtreeID{2}}, {prev: seed.Node, children: []SubtreeID{3}}}
	selected, err := c.selectCRecoveryJoinedReductionPaths(paths, func(_ NodeID, id SubtreeID) (uint32, error) {
		if id == 1 {
			return 20, nil
		}
		return 10, nil
	})
	if err != nil || len(selected) != 1 || selected[0].children[0] != 3 {
		t.Fatalf("error-cost winner=%+v err=%v", selected, err)
	}
}

//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"sync"
	"testing"
)

func newCompactReuseDependencyTestTree(t *testing.T) (*Tree, *Node) {
	t.Helper()
	a := acquireNodeArena(arenaClassIncremental)
	child := newLeafNodeInArena(a, 1, true, 2, 4, Point{Column: 2}, Point{Column: 4})
	root := newParentNodeInArena(a, 3, true, []*Node{child}, nil, 0)
	root.startByte, root.endByte = 0, 20
	root.startPoint, root.endPoint = Point{}, Point{Column: 20}
	tree := newTreeWithArenas(root, []byte("abcdefghijklmnopqrst"), testLanguage(), a, nil)
	return tree, child
}

func TestCompactReuseDependencyMembershipAndMaximum(t *testing.T) {
	tree, node := newCompactReuseDependencyTestTree(t)
	defer tree.Release()
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("new node has an unauthenticated receipt")
	}
	if !setCompactReuseDependency(node, 0) {
		t.Fatal("could not authenticate zero lookahead")
	}
	if n, ok := compactReuseDependencyForNode(node); !ok || n != 0 {
		t.Fatalf("zero receipt=%d/%t", n, ok)
	}
	for _, n := range []uint32{7, 2, 0} {
		if !setCompactReuseDependency(node, n) {
			t.Fatal("receipt update failed")
		}
	}
	if n, ok := compactReuseDependencyForNode(node); !ok || n != 7 {
		t.Fatalf("maximum receipt=%d/%t", n, ok)
	}
	clearCompactReuseDependency(node)
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("cleared receipt remained authenticated")
	}
	if setCompactReuseDependency(nil, 1) {
		t.Fatal("nil node acquired a receipt")
	}
}

func TestCompactReuseDependencyCopyAndOffset(t *testing.T) {
	tree, node := newCompactReuseDependencyTestTree(t)
	if !setCompactReuseDependency(node, 7) {
		tree.Release()
		t.Fatal("receipt setup failed")
	}
	copyTree := tree.Copy()
	defer copyTree.Release()
	copyNode := copyTree.RootNode().Child(0)
	if n, ok := compactReuseDependencyForNode(copyNode); !ok || n != 7 {
		t.Fatalf("copy receipt=%d/%t", n, ok)
	}
	offset := cloneTreeNodesWithOffset(tree.RootNode(), 5, Point{Row: 1})
	offsetTree := newTreeWithArenas(offset, nil, testLanguage(), offset.ownerArena, nil)
	defer offsetTree.Release()
	offsetNode := offset.Child(0)
	if offsetNode.StartByte() != 7 || offsetNode.EndByte() != 9 {
		t.Fatal("offset clone has incorrect coordinates")
	}
	if offsetNode.StartPoint() != (Point{Row: 1, Column: 2}) {
		t.Fatal("row-only clone changed the scanner column")
	}
	if n, ok := compactReuseDependencyForNode(offsetNode); !ok || n != 7 {
		t.Fatalf("offset receipt=%d/%t", n, ok)
	}
	columnOffset := cloneTreeNodesWithOffset(tree.RootNode(), 5, Point{Column: 5})
	columnTree := newTreeWithArenas(columnOffset, nil, testLanguage(), columnOffset.ownerArena, nil)
	defer columnTree.Release()
	if _, ok := compactReuseDependencyForNode(columnOffset.Child(0)); ok {
		t.Fatal("column-changing clone retained a scanner-dependent receipt")
	}
	clearCompactReuseDependency(copyNode)
	if n, ok := compactReuseDependencyForNode(node); !ok || n != 7 {
		t.Fatal("clearing a copy changed the original receipt")
	}
	tree.Release()
	if n, ok := compactReuseDependencyForNode(offsetNode); !ok || n != 7 {
		t.Fatal("offset receipt depended on the released arena")
	}
}

func TestCompactReuseDependencyEditInvalidation(t *testing.T) {
	for _, tc := range []struct {
		name               string
		start, end, newEnd uint32
		keep               bool
	}{
		{"inside_node", 3, 4, 4, false},
		{"examined_suffix", 8, 9, 9, false},
		{"insert_at_node_end", 4, 4, 5, false},
		{"before_node", 0, 0, 1, false},
		{"same_width_before_node", 0, 1, 1, true},
		{"last_examined_byte_insertion", 9, 9, 10, false},
		{"after_examined_suffix_insertion", 10, 10, 11, true},
		{"beyond_dependency", 12, 13, 13, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, node := newCompactReuseDependencyTestTree(t)
			defer tree.Release()
			if !setCompactReuseDependency(node, 6) {
				t.Fatal("receipt setup failed")
			}
			tree.Edit(InputEdit{StartByte: tc.start, OldEndByte: tc.end, NewEndByte: tc.newEnd, StartPoint: Point{Column: tc.start}, OldEndPoint: Point{Column: tc.end}, NewEndPoint: Point{Column: tc.newEnd}})
			n, ok := compactReuseDependencyForNode(node)
			if ok != tc.keep || (ok && n != 6) {
				t.Fatalf("receipt=%d/%t, want keep=%t", n, ok, tc.keep)
			}
			if tc.name == "before_node" && node.StartByte() != 3 {
				t.Fatal("receipt invalidation prevented coordinate adjustment")
			}
		})
	}
}

func TestCompactReuseDependencyManyUnaffectedReceipts(t *testing.T) {
	const count = 128
	a := acquireNodeArena(arenaClassIncremental)
	nodes := make([]*Node, count)
	for i := range nodes {
		start := uint32(4*i + 2)
		nodes[i] = newLeafNodeInArena(a, 1, true, start, start+1, Point{Column: start}, Point{Column: start + 1})
		if !setCompactReuseDependency(nodes[i], 0) {
			a.Release()
			t.Fatal("receipt setup failed")
		}
	}
	root := newParentNodeInArena(a, 3, true, nodes, nil, 0)
	tree := newTreeWithArenas(root, make([]byte, 4*count), testLanguage(), a, nil)
	defer tree.Release()
	target := nodes[count/2]
	editStart := target.EndByte() + 1
	edit := InputEdit{StartByte: editStart, OldEndByte: editStart + 1, NewEndByte: editStart + 1, StartPoint: Point{Column: editStart}, OldEndPoint: Point{Column: editStart + 1}, NewEndPoint: Point{Column: editStart + 1}}
	for iteration := 0; iteration < 32; iteration++ {
		// One distant prefix has a long dependency. The others inspect no suffix.
		if !setCompactReuseDependency(nodes[0], editStart+1-nodes[0].EndByte()) || !setCompactReuseDependency(target, 2) {
			t.Fatal("receipt reauthentication failed")
		}
		if _, ok := compactReuseDependencyForNode(target); !ok {
			t.Fatal("old edits invalidated a newly authenticated receipt")
		}
		tree.Edit(edit)
		for i, node := range nodes {
			extent, ok := compactReuseDependencyForNode(node)
			want := i != 0 && i != count/2
			if ok != want || (ok && extent != 0) {
				t.Fatalf("iteration=%d receipt=%d extent=%d present=%t want=%t", iteration, i, extent, ok, want)
			}
		}
	}
}

func TestCompactReuseDependencyIndexBudgetAndRebuild(t *testing.T) {
	tree, node := newCompactReuseDependencyTestTree(t)
	defer tree.Release()
	a := tree.arena
	if !setCompactReuseDependency(node, 6) {
		t.Fatal("receipt setup failed")
	}
	a.setBudget(1)
	before := a.allocatedBytes
	a.editCompactReuseDependencies(InputEdit{StartByte: 8, OldEndByte: 9, NewEndByte: 9})
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("budget-blocked index skipped exact invalidation")
	}
	if cap(a.compactReuseDependencyIndex) != 0 || a.allocatedBytes != before {
		t.Fatal("index allocation exceeded its budget")
	}
	a.setBudget(0)
	if !setCompactReuseDependency(node, 6) {
		t.Fatal("reauthentication failed")
	}
	a.editCompactReuseDependencies(InputEdit{StartByte: 12, OldEndByte: 13, NewEndByte: 13})
	if !a.compactReuseDependencyIndexed || len(a.compactReuseDependencyIndex) != 1 {
		t.Fatal("index did not build after budget reset")
	}
	second := newLeafNodeInArena(a, 1, true, 30, 32, Point{Column: 30}, Point{Column: 32})
	if !setCompactReuseDependency(second, 1) {
		t.Fatal("second receipt setup failed")
	}
	a.setBudget(1)
	before = a.allocatedBytes
	a.editCompactReuseDependencies(InputEdit{StartByte: 31, OldEndByte: 32, NewEndByte: 32})
	if _, ok := compactReuseDependencyForNode(second); ok {
		t.Fatal("budget-blocked growth preserved an overlapping receipt")
	}
	if n, ok := compactReuseDependencyForNode(node); !ok || n != 6 {
		t.Fatal("budget fallback lost the unaffected receipt")
	}
	if a.allocatedBytes != before || cap(a.compactReuseDependencyIndex) != 1 {
		t.Fatal("blocked growth changed index allocation")
	}
	a.setBudget(0)
	if !setCompactReuseDependency(second, 1) {
		t.Fatal("second receipt reauthentication failed")
	}
	a.editCompactReuseDependencies(InputEdit{StartByte: 40, OldEndByte: 41, NewEndByte: 41})
	if !a.compactReuseDependencyIndexed || len(a.compactReuseDependencyIndex) != 2 {
		t.Fatal("index did not grow after budget reset")
	}
	if !setCompactReuseDependency(node, 40) {
		t.Fatal("dependency extension failed")
	}
	a.setBudget(1)
	before = a.allocatedBytes
	a.editCompactReuseDependencies(InputEdit{StartByte: 41, OldEndByte: 42, NewEndByte: 42})
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("rebuilt index ignored an extended dependency")
	}
	if !a.compactReuseDependencyIndexed || a.allocatedBytes != before {
		t.Fatal("same-capacity rebuild required new allocation")
	}
	if _, ok := compactReuseDependencyForNode(second); !ok {
		t.Fatal("rebuild removed an unaffected receipt")
	}
}

func TestCompactReuseDependencyRepeatedSuffixDeletionEmptiesIndex(t *testing.T) {
	a := newNodeArena(arenaClassIncremental)
	defer a.Release()
	nodes := make([]*Node, 32)
	for i := range nodes {
		start := uint32(4 * i)
		nodes[i] = newLeafNodeInArena(a, 1, true, start, start+1, Point{Column: start}, Point{Column: start + 1})
	}
	for cycle := 0; cycle < 4; cycle++ {
		for _, node := range nodes {
			if !setCompactReuseDependency(node, 256) {
				t.Fatal("receipt setup failed")
			}
		}
		a.editCompactReuseDependencies(InputEdit{StartByte: 200, OldEndByte: 201, NewEndByte: 201})
		if len(a.compactReuseDependencies) != 0 || !a.compactReuseDependencyIndexed {
			t.Fatal("suffix invalidation did not empty the indexed receipts")
		}
		root := a.compactReuseDependencyIndex[len(a.compactReuseDependencyIndex)/2]
		if root.live || root.maxEnd != 0 {
			t.Fatal("empty index root retained a live subtree")
		}
		for _, entry := range a.compactReuseDependencyIndex {
			if entry.node != nil || entry.live {
				t.Fatal("empty index retained a node reference")
			}
		}
		a.editCompactReuseDependencies(InputEdit{StartByte: 200, OldEndByte: 201, NewEndByte: 201})
	}
}

func TestCompactReuseDependencyZeroOriginIsLive(t *testing.T) {
	a := newNodeArena(arenaClassIncremental)
	defer a.Release()
	node := newLeafNodeInArena(a, 1, true, 0, 0, Point{}, Point{})
	if !setCompactReuseDependency(node, 0) {
		t.Fatal("zero receipt setup failed")
	}
	a.editCompactReuseDependencies(InputEdit{StartByte: 1, OldEndByte: 2, NewEndByte: 2})
	if n, ok := compactReuseDependencyForNode(node); !ok || n != 0 {
		t.Fatal("zero-origin receipt was confused with an empty entry")
	}
	if len(a.compactReuseDependencyIndex) != 1 || !a.compactReuseDependencyIndex[0].live || a.compactReuseDependencyIndex[0].maxEnd != 0 {
		t.Fatal("zero-origin index did not retain explicit liveness")
	}
	a.editCompactReuseDependencies(InputEdit{StartByte: 0, OldEndByte: 0, NewEndByte: 1, NewEndPoint: Point{Column: 1}})
	if _, ok := compactReuseDependencyForNode(node); ok || a.compactReuseDependencyIndex[0].live {
		t.Fatal("insertion at zero did not invalidate the live zero-width receipt")
	}
}

func TestCompactReuseDependencySharedArenaPreservesUnexaminedReceipt(t *testing.T) {
	a := acquireNodeArena(arenaClassIncremental)
	a.Retain()
	var trees [2]*Tree
	var nodes [2]*Node
	for i := range trees {
		start := uint32(2 + 20*i)
		nodes[i] = newLeafNodeInArena(a, 1, true, start, start+2, Point{Column: start}, Point{Column: start + 2})
		root := newParentNodeInArena(a, 3, true, []*Node{nodes[i]}, nil, 0)
		trees[i] = newTreeWithArenas(root, make([]byte, 32), testLanguage(), a, nil)
		defer trees[i].Release()
		if !setCompactReuseDependency(nodes[i], 1) {
			t.Fatal("receipt setup failed")
		}
	}
	trees[0].Edit(InputEdit{StartByte: 3, OldEndByte: 4, NewEndByte: 4, StartPoint: Point{Column: 3}, OldEndPoint: Point{Column: 4}, NewEndPoint: Point{Column: 4}})
	if _, ok := compactReuseDependencyForNode(nodes[0]); ok {
		t.Fatal("edited receipt remained authenticated")
	}
	if n, ok := compactReuseDependencyForNode(nodes[1]); !ok || n != 1 {
		t.Fatal("same-width edit discarded another tree's unexamined receipt")
	}
}

func TestCompactReuseDependencyRejectsStaleCoordinates(t *testing.T) {
	tree, node := newCompactReuseDependencyTestTree(t)
	defer tree.Release()
	if !setCompactReuseDependency(node, 7) {
		t.Fatal("receipt setup failed")
	}
	node.startByte++
	node.endByte++
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("changed coordinates reused an old receipt")
	}
	copyTree := tree.Copy()
	defer copyTree.Release()
	if _, ok := compactReuseDependencyForNode(copyTree.RootNode().Child(0)); ok {
		t.Fatal("copy authenticated a stale coordinate receipt")
	}
	if !setCompactReuseDependency(node, 2) {
		t.Fatal("coordinate reauthentication failed")
	}
	if n, ok := compactReuseDependencyForNode(node); !ok || n != 2 {
		t.Fatal("reauthentication inherited the stale dependency maximum")
	}
}

func TestCompactReuseDependencyRejectsStaleColumn(t *testing.T) {
	tree, node := newCompactReuseDependencyTestTree(t)
	defer tree.Release()
	if !setCompactReuseDependency(node, 7) {
		t.Fatal("receipt setup failed")
	}
	node.startPoint.Column++
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("changed scanner column reused an old receipt")
	}
	if !setCompactReuseDependency(node, 2) {
		t.Fatal("column reauthentication failed")
	}
	if n, ok := compactReuseDependencyForNode(node); !ok || n != 2 {
		t.Fatal("column reauthentication inherited the stale dependency maximum")
	}
}

func TestCompactReuseDependencyConcurrentSharedArena(t *testing.T) {
	a := acquireNodeArena(arenaClassIncremental)
	a.Retain()
	var trees [2]*Tree
	var nodes [2]*Node
	for i := range trees {
		nodes[i] = newLeafNodeInArena(a, 1, true, 2, 4, Point{Column: 2}, Point{Column: 4})
		root := newParentNodeInArena(a, 3, true, []*Node{nodes[i]}, nil, 0)
		root.endByte, root.endPoint = 20, Point{Column: 20}
		trees[i] = newTreeWithArenas(root, []byte("abcdefghijklmnopqrst"), testLanguage(), a, nil)
		defer trees[i].Release()
	}
	// Each goroutine owns its tree and node. Only the receipt arena is shared.
	var workers sync.WaitGroup
	start := make(chan struct{})
	for i := range trees {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			for iteration := 0; iteration < 200; iteration++ {
				if !setCompactReuseDependency(nodes[i], 6) {
					t.Error("receipt setup failed")
					return
				}
				if n, ok := compactReuseDependencyForNode(nodes[i]); ok && n != 6 {
					t.Error("concurrent invalidation changed the receipt extent")
					return
				}
				trees[i].Edit(InputEdit{StartByte: 0, OldEndByte: 0, NewEndByte: 1, StartPoint: Point{}, OldEndPoint: Point{}, NewEndPoint: Point{Column: 1}})
				if _, ok := compactReuseDependencyForNode(nodes[i]); ok {
					t.Error("position-changing edit retained its receipt")
					return
				}
			}
		}(i)
	}
	close(start)
	workers.Wait()
}

func TestCompactReuseDependencyOverflowAndPastEOF(t *testing.T) {
	tree, node := newCompactReuseDependencyTestTree(t)
	defer tree.Release()
	if !setCompactReuseDependency(node, ^uint32(0)) {
		t.Fatal("wide dependency was rejected")
	}
	if n, ok := compactReuseDependencyForNode(node); !ok || n != ^uint32(0) {
		t.Fatal("wide relative dependency was truncated")
	}
	tree.Edit(InputEdit{StartByte: 15, OldEndByte: 16, NewEndByte: 16, StartPoint: Point{Column: 15}, OldEndPoint: Point{Column: 16}, NewEndPoint: Point{Column: 16}})
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("overflow wrapped the examined suffix before the edit")
	}
}

func TestCompactReuseDependencyBorrowedArenaEdit(t *testing.T) {
	original, node := newCompactReuseDependencyTestTree(t)
	if !setCompactReuseDependency(node, 6) {
		original.Release()
		t.Fatal("receipt setup failed")
	}
	a := acquireNodeArena(arenaClassIncremental)
	root := newParentNodeInArena(a, 3, true, []*Node{node}, nil, 0)
	root.endByte, root.endPoint = 20, Point{Column: 20}
	original.arena.Retain()
	borrowed := newTreeWithArenas(root, []byte("abcdefghijklmnopqrst"), testLanguage(), a, []*nodeArena{original.arena})
	defer borrowed.Release()
	original.Release()
	if _, ok := compactReuseDependencyForNode(node); !ok {
		t.Fatal("borrowed receipt vanished when its source tree was released")
	}
	borrowed.Edit(InputEdit{StartByte: 8, OldEndByte: 9, NewEndByte: 9, StartPoint: Point{Column: 8}, OldEndPoint: Point{Column: 9}, NewEndPoint: Point{Column: 9}})
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("editing a borrowed tree left its examined suffix authenticated")
	}
}

func TestCompactReuseDependencyResetAndBudget(t *testing.T) {
	a := newNodeArena(arenaClassIncremental)
	n := newLeafNodeInArena(a, 1, true, 0, 1, Point{}, Point{Column: 1})
	baseline := a.allocatedBytes
	a.setBudget(1)
	if setCompactReuseDependency(n, 3) || a.allocatedBytes != baseline {
		t.Fatal("receipt allocation exceeded the arena memory budget")
	}
	if _, ok := compactReuseDependencyForNode(n); ok {
		t.Fatal("a rejected allocation published a receipt")
	}
	a.setBudget(4096)
	if !setCompactReuseDependency(n, 3) || a.allocatedBytes <= baseline {
		t.Fatal("receipt storage escaped arena allocation accounting")
	}
	allocated := a.allocatedBytes
	clearCompactReuseDependency(n)
	if a.allocatedBytes != allocated {
		t.Fatal("clearing a receipt undercounted retained map storage")
	}
	if !setCompactReuseDependency(n, 4) {
		t.Fatal("receipt reinsertion failed")
	}
	allocated = a.allocatedBytes
	a.recomputeAllocatedBytes()
	if a.allocatedBytes != allocated {
		t.Fatal("arena recomputation lost the receipt storage charge")
	}
	a.reset()
	if a.compactReuseDependencies != nil {
		t.Fatal("arena reset retained the receipt map")
	}
	if _, ok := compactReuseDependencyForNode(n); ok {
		t.Fatal("arena reset retained a stale node receipt")
	}
	if a.allocatedBytes > baseline {
		t.Fatal("arena reset retained receipt allocation accounting")
	}
	reused := newLeafNodeInArena(a, 1, true, 0, 1, Point{}, Point{Column: 1})
	if _, ok := compactReuseDependencyForNode(reused); ok {
		t.Fatal("a reused arena slot inherited a receipt")
	}
}

func TestCompactReuseDependencyShortcutDoesNotReviveReceipt(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	source := []byte("func a(){_=1}")
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	parameters := compactExecutionNode(old.RootNode(), p.language, "parameter_list", 6)
	if parameters == nil || !setCompactReuseDependency(parameters, 4) {
		old.Release()
		t.Fatal("parameter receipt setup failed")
	}
	edited, edit := compactExecutionEdit(source, 11, 12, "2")
	old.Edit(edit)
	if _, ok := compactReuseDependencyForNode(parameters); ok {
		old.Release()
		t.Fatal("edit retained an overlapping receipt")
	}
	next, err := p.ParseIncremental(edited, old)
	if err != nil {
		old.Release()
		t.Fatal(err)
	}
	if next != old {
		old.Release()
	}
	defer next.Release()
	got := compactExecutionNode(next.RootNode(), p.language, "parameter_list", 6)
	if got == nil {
		t.Fatal("shortcut lost the parameter list")
	}
	if _, ok := compactReuseDependencyForNode(got); ok {
		t.Fatal("the token-invariant shortcut revived an invalid receipt")
	}
}

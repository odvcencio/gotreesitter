package gotreesitter

import "testing"

func recoveryGraphCountFixture() (*Parser, func(int) *gssNode) {
	p := &Parser{
		language:       &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true, Named: true}}},
		cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize),
	}
	p.beginCNodeMemoEpoch()
	chain := func(count int) *gssNode {
		var head *gssNode
		for i := 0; i < count; i++ {
			n := NewLeafNode(1, true, uint32(i), uint32(i+1), Point{}, Point{})
			head = &gssNode{entry: newStackEntryNode(7, n), prev: head, depth: uint32(i + 1)}
		}
		return head
	}
	return p, chain
}

func TestCRecoveryGraphCountUsesMaximumEqualCostPath(t *testing.T) {
	p, chain := recoveryGraphCountFixture()
	missing := &Node{symbol: 1, flags: nodeFlagMissing | nodeFlagHasError}
	head := &gssNode{entry: newStackEntryNode(7, missing), prev: chain(1), depth: 2}
	head.appendExtraLink(gssMainLink{entry: newStackEntryNode(7, missing), prev: chain(20)})
	cost, count := p.cStackPrefixAgg(head)
	if cost != cErrCostPerMissingTree+cErrCostPerRecovery || count != 21 {
		t.Fatalf("equal-cost paths: cost=%d count=%d, want missing cost and count 21", cost, count)
	}
	s := glrStack{gss: gssStack{head: head}, cNodeBaseline: 1}
	if got := p.cNodeCountSinceError(&s); got != 20 {
		t.Fatalf("progress=%d, want 20", got)
	}
	// The cached materialized spine represents only the first graph path.
	s.entries = []stackEntry{head.prev.entry, head.entry}
	if got := p.cStackCumulativeNodeCount(&s); got != 21 {
		t.Fatalf("materialized spine hid the graph maximum: count=%d", got)
	}
	left := cErrorStatus{cost: 100, nodeCount: p.cNodeCountSinceError(&s)}
	right := cErrorStatus{cost: 200}
	if got := cCompareVersions(left, right); got != cErrorComparisonTakeLeft {
		t.Fatalf("graph progress did not retire the costly version: comparison=%v", got)
	}
	if got := cCompareVersions(cErrorStatus{cost: 100, nodeCount: 1}, right); got != cErrorComparisonPreferLeft {
		t.Fatalf("short-path control comparison=%v, want prefer-left", got)
	}
}

func TestCRecoveryGraphCountInvalidatesCachedAncestors(t *testing.T) {
	for _, operation := range []string{"append", "replace", "payload"} {
		t.Run(operation, func(t *testing.T) {
			p, chain := recoveryGraphCountFixture()
			middle := chain(1)
			alternative := chain(2)
			if operation != "append" {
				middle.appendExtraLink(gssMainLink{entry: alternative.entry, prev: alternative.prev})
			}
			head := &gssNode{entry: newStackEntryNode(7, NewLeafNode(1, true, 3, 4, Point{}, Point{})), prev: middle, depth: 3}
			_, before := p.cStackPrefixAgg(head)
			wantBefore := 3
			if operation == "append" {
				wantBefore = 2
			}
			if before != wantBefore {
				t.Fatalf("initial count=%d, want %d", before, wantBefore)
			}
			want := 5
			switch operation {
			case "append":
				long := chain(4)
				middle.appendExtraLink(gssMainLink{entry: long.entry, prev: long.prev})
			case "replace":
				long := chain(4)
				middle.setExtraLink(0, gssMainLink{entry: long.entry, prev: long.prev})
			case "payload":
				n := stackEntryNode(alternative.entry)
				n.children = []*Node{NewLeafNode(1, true, 0, 1, Point{}, Point{}), NewLeafNode(1, true, 1, 2, Point{}, Point{})}
				nodeBumpEquivVersion(n)
			}
			_, got := p.cStackPrefixAgg(head)
			if got != want {
				t.Fatalf("cached ancestor after %s: count=%d, want %d", operation, got, want)
			}
			if _, again := p.cStackPrefixAgg(head); again != want {
				t.Fatalf("warm aggregate count=%d, want %d", again, want)
			}
		})
	}
}

func TestCRecoveryGraphCountClearsPathScratch(t *testing.T) {
	p := &Parser{}
	head := &gssNode{prev: &gssNode{}}
	p.cStackPrefixAgg(head)
	for i, node := range p.cPrefixPath[:cap(p.cPrefixPath)] {
		if node != nil {
			t.Fatalf("scratch slot %d retains a graph node", i)
		}
	}
	p.cStackPrefixAgg(head)
	for i, node := range p.cPrefixPath[:cap(p.cPrefixPath)] {
		if node != nil {
			t.Fatalf("warm scratch slot %d retains a graph node", i)
		}
	}
	resetGSSPrefixPath(&p.cPrefixPath)
	for i, node := range p.cPrefixPath[:cap(p.cPrefixPath)] {
		if node != nil {
			t.Fatalf("reset scratch slot %d retains a graph node", i)
		}
	}
}

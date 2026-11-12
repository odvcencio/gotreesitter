package gotreesitter

import "testing"

// These tests pin the mechanism behind the C-recovery cyclic-transient-tree
// defect (go zerrors_windows.go truncations; the trailing-EOF shape of issue
// #110) and its fix.
//
// Transient reduce parents (transientParentScratch slab nodes) use their
// .parent field as the result-time materializer's {nil: unvisited, self:
// in-progress, other: arena clone} map. Recovery constructions that wrapped
// such nodes with the eager-link-wiring constructor (newParentNodeInArena →
// populateParentNode → setNodeParentLink) corrupted that map:
// materializeNodesUntil skipped the child as "already cloned" and
// transientReplacement() substituted the child's TREE PARENT for the child —
// linking the wrapper under itself. newRecoveryParentNodeInArena must never
// do that while transient parents are active.

func newTransientSentinelFixture(t *testing.T) (*nodeArena, *transientParentScratch, *Node) {
	t.Helper()
	arena := newNodeArena(arenaClassFull)
	scratch := &transientParentScratch{}
	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	transient := scratch.allocParent(arena, 3, true, []*Node{leaf}, 0, true)
	if !scratch.owns(transient) {
		t.Fatal("fixture: allocParent result is not slab-owned")
	}
	if transient.parent != nil {
		t.Fatal("fixture: fresh transient parent must carry the nil sentinel")
	}
	return arena, scratch, transient
}

// TestRecoveryParentPreservesTransientSentinel: with transient parents active,
// recovery construction must leave transient children's .parent nil, and the
// result-time materializer must then replace them with arena clones — never
// with the wrapper itself.
func TestRecoveryParentPreservesTransientSentinel(t *testing.T) {
	arena, scratch, transient := newTransientSentinelFixture(t)
	p := &Parser{reduceScratch: &reduceBuildScratch{transientParents: scratch}}

	wrapper := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, []*Node{transient}, 0)
	if transient.parent != nil {
		t.Fatalf("recovery construction corrupted the transient sentinel: parent=%p", transient.parent)
	}

	nodes := []*Node{wrapper}
	scratch.materializeNodeSliceUntil(nodes, arena, nil, nil)
	if len(wrapper.children) != 1 {
		t.Fatalf("wrapper child count = %d, want 1", len(wrapper.children))
	}
	got := wrapper.children[0]
	if got == wrapper {
		t.Fatal("wrapper became its own child (cyclic transient tree)")
	}
	if scratch.owns(got) {
		t.Fatal("transient child was not materialized into the arena")
	}
	if got.symbol != 3 {
		t.Fatalf("materialized child symbol = %d, want 3", got.symbol)
	}
	if !debugRecoveryCheckNodeAcyclic(p, arena, "test-sentinel-preserved", wrapper) {
		t.Fatal("cycle detector found a back-edge after the fixed construction")
	}
}

// TestEagerWiringOnTransientChildFailsClosedBeforeSelfLoop documents the
// defect mechanism. The materializer must reject the replacement before it
// links the wrapper under itself.
func TestEagerWiringOnTransientChildFailsClosedBeforeSelfLoop(t *testing.T) {
	arena, scratch, transient := newTransientSentinelFixture(t)

	// Simulate a pre-fix construction that wired transient.parent.
	wrapper := newParentNodeInArena(arena, errorSymbol, true, []*Node{transient}, nil, 0)
	if transient.parent != wrapper {
		t.Fatal("simulation expects eager wiring to set the transient child's parent")
	}

	nodes := []*Node{wrapper}
	if reason := scratch.materializeNodeSliceUntil(nodes, arena, nil, nil); reason != ParseStopInvariantViolation {
		t.Fatalf("materialize stop reason = %v, want invariant violation", reason)
	}
	if len(wrapper.children) != 1 || wrapper.children[0] != transient {
		t.Fatal("materializer changed the child before it rejected the replacement")
	}
}

// TestRecoveryParentWiresLinksWithoutTransientParents: incremental parses
// (reuse/oldTree) never allocate transient parents and skip the finalize-time
// parent wiring, so recovery constructions there must keep the eager-wiring
// behavior.
func TestRecoveryParentWiresLinksWithoutTransientParents(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	p := &Parser{}
	wrapper := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, []*Node{leaf}, 0)
	if leaf.parent != wrapper {
		t.Fatal("eager parent wiring expected when transient parents are inactive")
	}
}

// TestTrailingEOFRecoveryPreservesTransientSentinel pins the fix on issue #110's
// actual path: appendTrailingEOFRecoveryNodes wraps a dropped trailing stack
// payload — here a transient reduce parent — in an ERROR node. With transient
// parents active it must leave the child's .parent sentinel nil so the
// result-time materializer replaces the child with an arena clone rather than
// with the wrapper itself.
func TestTrailingEOFRecoveryPreservesTransientSentinel(t *testing.T) {
	arena, scratch, transient := newTransientSentinelFixture(t)
	p := &Parser{reduceScratch: &reduceBuildScratch{transientParents: scratch}}

	entries := []stackEntry{newStackEntryNode(transient.parseState, transient)}
	nodeCount := 0
	nodes, recovered := p.appendTrailingEOFRecoveryNodes(nil, entries, 0, Token{}, arena, &nodeCount)
	if !recovered {
		t.Fatal("trailing-EOF recovery did not wrap the dropped payload in an ERROR node")
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1", nodeCount)
	}
	if len(nodes) != 1 {
		t.Fatalf("recovery nodes = %d, want 1", len(nodes))
	}
	wrapper := nodes[0]
	if wrapper.symbol != errorSymbol {
		t.Fatalf("wrapper symbol = %d, want errorSymbol", wrapper.symbol)
	}
	if transient.parent != nil {
		t.Fatalf("trailing-EOF construction corrupted the transient sentinel: parent=%p", transient.parent)
	}

	if reason := scratch.materializeNodeSliceUntil(nodes, arena, nil, nil); reason != ParseStopNone {
		t.Fatalf("materialize stop reason = %v, want none", reason)
	}
	if len(wrapper.children) != 1 {
		t.Fatalf("wrapper child count = %d, want 1", len(wrapper.children))
	}
	got := wrapper.children[0]
	if got == wrapper {
		t.Fatal("ERROR wrapper became its own child (cyclic transient tree)")
	}
	if scratch.owns(got) {
		t.Fatal("transient child was not materialized into the arena")
	}
	if got.symbol != 3 {
		t.Fatalf("materialized child symbol = %d, want 3", got.symbol)
	}
	if !debugRecoveryCheckNodeAcyclic(p, arena, "test-trailing-eof-fixed", wrapper) {
		t.Fatal("cycle detector found a back-edge after the fixed trailing-EOF construction")
	}
}

// TestTrailingEOFEagerWiringSelfLoopFailsClosedAsInvariantViolation preserves
// the historical issue #110 construction path. The old eager-linking wrapper
// corrupts the transient sentinel and materializes as a self-loop; the result
// validator must leave that evidence untouched and return a standalone error
// tree that publicly reports an invariant stop.
func TestTrailingEOFEagerWiringSelfLoopFailsClosedAsInvariantViolation(t *testing.T) {
	arena, _, transient := newTransientSentinelFixture(t)

	trailing := stackEntryNode(newStackEntryNode(transient.parseState, transient))
	wrapper := newParentNodeInArena(arena, errorSymbol, true, []*Node{trailing}, nil, 0)
	if transient.parent != wrapper {
		t.Fatal("pre-fix simulation expects eager wiring to corrupt the transient sentinel")
	}

	nodes := []*Node{wrapper}
	wrapper.children[0] = wrapper

	tree := recoveredResultInvariantErrorTree(nodes, []byte("x"), &Language{Name: "test"}, arena)
	if tree == nil {
		t.Fatal("cyclic recovered result was not contained")
	}
	defer tree.Release()
	if got := tree.ParseStopReason(); got != ParseStopInvariantViolation {
		t.Fatalf("stop reason = %q, want %q", got, ParseStopInvariantViolation)
	}
	if !tree.ParseStoppedEarly() {
		t.Fatal("invariant error tree did not report stopped-early")
	}
	if root := rawRootOrNil(tree); root == nil || !root.IsError() || root == wrapper {
		t.Fatalf("fail-closed root = %p, want standalone ERROR distinct from corrupt wrapper %p", root, wrapper)
	}
	if len(wrapper.children) != 1 || wrapper.children[0] != wrapper {
		t.Fatal("validator repaired or mutated the historical self-loop")
	}
}

func TestStackRelexReadingsAdmitsEachReadingOnce(t *testing.T) {
	var r stackRelexReadings
	ident := Token{Symbol: 1, StartByte: 4, EndByte: 12}
	keyword := Token{Symbol: 189, StartByte: 4, EndByte: 12}
	if !r.admit(113, ident) || !r.admit(113, keyword) {
		t.Fatal("first readings must be admitted")
	}
	if r.admit(113, ident) || r.admit(113, keyword) {
		t.Fatal("a repeated reading must be refused")
	}
	if !r.admit(114, ident) {
		t.Fatal("the same token in another state is a new reading")
	}
	if !r.admit(113, Token{Symbol: 1, StartByte: 4, EndByte: 13}) {
		t.Fatal("the same symbol over another span is a new reading")
	}
}

func TestStackRelexReadingsRefusesOnceFull(t *testing.T) {
	var r stackRelexReadings
	for i := range len(r.seen) {
		if !r.admit(StateID(i), Token{Symbol: 1}) {
			t.Fatalf("reading %d refused before capacity", i)
		}
	}
	if r.admit(StateID(len(r.seen)), Token{Symbol: 1}) {
		t.Fatal("a new reading beyond capacity must be refused")
	}
}

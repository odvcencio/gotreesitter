package gotreesitter

import "testing"

func TestNodeFieldMetadataStandaloneAndNilReads(t *testing.T) {
	var node Node
	if node.fieldIDs() != nil || node.fieldSources() != nil {
		t.Fatal("zero Node field metadata must read as nil")
	}

	ids := []FieldID{3, 0}
	sources := []uint8{fieldSourceDirect, fieldSourceNone}
	node.setFieldMetadata(ids, sources)
	if node.fieldMetadata == nil {
		t.Fatal("standalone Node did not allocate field metadata")
	}
	if got := nodeFieldIDAt(&node, 0); got != 3 {
		t.Fatalf("field ID = %d, want 3", got)
	}
	if got := fieldSourceAt(node.fieldSources(), 0); got != fieldSourceDirect {
		t.Fatalf("field source = %d, want direct", got)
	}

	node.clearFieldMetadata()
	if node.fieldMetadata != nil || node.fieldIDs() != nil || node.fieldSources() != nil {
		t.Fatal("cleared standalone metadata must be nil")
	}
}

func TestNodeFieldMetadataShallowCopyHeaderCOW(t *testing.T) {
	original := Node{}
	original.setFieldMetadata(
		[]FieldID{1, 2},
		[]uint8{fieldSourceDirect, fieldSourceInherited},
	)

	shared := original
	shared.fieldIDs()[0] = 9
	if got := original.fieldIDs()[0]; got != 9 {
		t.Fatalf("element write no longer shares backing: original ID = %d, want 9", got)
	}

	shared.setFieldIDs(shared.fieldIDs()[:1])
	if shared.fieldMetadata == original.fieldMetadata {
		t.Fatal("field-ID header replacement did not detach sidecar")
	}
	if got := len(original.fieldIDs()); got != 2 {
		t.Fatalf("original field-ID length = %d, want 2", got)
	}
	if got := len(shared.fieldSources()); got != 2 {
		t.Fatalf("one-sided setter changed source length to %d, want 2", got)
	}
	shared.fieldIDs()[0] = 7
	if got := original.fieldIDs()[0]; got != 7 {
		t.Fatalf("header COW stopped backing alias: original ID = %d, want 7", got)
	}

	shared.setFieldSources(shared.fieldSources()[:1])
	if got := len(original.fieldSources()); got != 2 {
		t.Fatalf("original source length = %d, want 2", got)
	}
	if got := len(shared.fieldIDs()); got != 1 {
		t.Fatalf("one-sided source setter changed ID length to %d, want 1", got)
	}

	shared.clearFieldMetadata()
	if shared.fieldMetadata != nil {
		t.Fatal("cleared shallow copy still has metadata")
	}
	if got := len(original.fieldIDs()); got != 2 {
		t.Fatalf("clearing shallow copy changed original ID length to %d", got)
	}
}

func TestNodeFieldMetadataPreservesIndependentNilAndEmptyHeaders(t *testing.T) {
	node := Node{}
	node.setFieldMetadata([]FieldID{}, nil)
	if node.fieldIDs() == nil || node.fieldSources() != nil {
		t.Fatalf("initial headers: ids=%v sources=%v, want nonnil-empty/nil", node.fieldIDs(), node.fieldSources())
	}

	node.setFieldSources([]uint8{})
	if node.fieldIDs() == nil || node.fieldSources() == nil {
		t.Fatalf("empty headers: ids=%v sources=%v, want both nonnil", node.fieldIDs(), node.fieldSources())
	}

	node.setFieldIDs(nil)
	if node.fieldIDs() != nil || node.fieldSources() == nil {
		t.Fatalf("one-sided nil: ids=%v sources=%v, want nil/nonnil-empty", node.fieldIDs(), node.fieldSources())
	}

	arena := newNodeArena(arenaClassIncremental)
	clone := arena.allocNode()
	clone.ownerArena = arena
	cloneNodeFieldMetadataInto(clone, &node, arena)
	if clone.fieldIDs() != nil || clone.fieldSources() == nil {
		t.Fatalf("cloned headers: ids=%v sources=%v, want nil/nonnil-empty", clone.fieldIDs(), clone.fieldSources())
	}
}

func TestEnsureNodeFieldStorageMixedLengths(t *testing.T) {
	node := Node{children: make([]*Node, 2)}
	node.setFieldMetadata(
		[]FieldID{4, 5},
		[]uint8{fieldSourceInherited},
	)
	before := node.fieldMetadata
	idsBefore := node.fieldIDs()

	ensureNodeFieldStorage(&node, 2)
	if node.fieldMetadata == before {
		t.Fatal("mixed-length repair did not detach the metadata header")
	}
	if got := node.fieldIDs(); len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("field IDs after repair = %v, want [4 5]", got)
	}
	if got := node.fieldSources(); len(got) != 2 || got[0] != fieldSourceInherited || got[1] != fieldSourceNone {
		t.Fatalf("field sources after repair = %v, want [inherited none]", got)
	}
	node.fieldIDs()[0] = 8
	if got := idsBefore[0]; got != 8 {
		t.Fatalf("unchanged ID backing stopped aliasing: got %d, want 8", got)
	}

	before = node.fieldMetadata
	ensureNodeFieldStorage(&node, 2)
	if node.fieldMetadata != before {
		t.Fatal("matching field lengths unexpectedly detached metadata")
	}
}

func TestNodeFieldMetadataArenaPointerStableAcrossGrowth(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	first := arena.allocNodeFieldMetadata()
	first.ids = []FieldID{11}
	first.sources = []uint8{fieldSourceDirect}
	firstCapacity := len(arena.nodeFieldMetadataSlabs[0].data)

	for i := 0; i < firstCapacity+1; i++ {
		arena.allocNodeFieldMetadata()
	}
	if len(arena.nodeFieldMetadataSlabs) < 2 {
		t.Fatal("field-metadata arena did not grow to a second slab")
	}
	if got := first.ids[0]; got != 11 {
		t.Fatalf("first sidecar moved or changed after growth: ID = %d", got)
	}
	if got := first.sources[0]; got != fieldSourceDirect {
		t.Fatalf("first sidecar source = %d, want direct", got)
	}
}

func TestNodeFieldMetadataArenaResetClearsAndReuses(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	first := arena.allocNodeFieldMetadata()
	first.ids = []FieldID{6}
	first.sources = []uint8{fieldSourceInherited}

	arena.resetNodeFieldMetadataSlabs()
	if first.ids != nil || first.sources != nil {
		t.Fatalf("reset retained stale headers: ids=%v sources=%v", first.ids, first.sources)
	}
	reused := arena.allocNodeFieldMetadata()
	if reused != first {
		t.Fatal("reset did not reuse the first retained sidecar slot")
	}
	if reused.ids != nil || reused.sources != nil {
		t.Fatal("reused sidecar slot was not zeroed")
	}
}

func TestNodeFieldMetadataArenaAccountingAndBudget(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	wantBytes := nodeFieldMetadataBytesForCap(defaultNodeFieldMetadataSlabCap(arena.class))
	before := arena.allocatedBytes
	arena.setBudget(wantBytes)
	arena.allocNodeFieldMetadata()

	if got := arena.allocatedBytes - before; got != wantBytes {
		t.Fatalf("sidecar allocation delta = %d, want %d", got, wantBytes)
	}
	if !arena.budgetExhausted() {
		t.Fatal("sidecar slab growth was not charged to the memory budget")
	}
	breakdown := arena.collectArenaBreakdown()
	if got := breakdown.NodeFieldMetadataBytesAllocated; got != wantBytes {
		t.Fatalf("sidecar breakdown bytes = %d, want %d", got, wantBytes)
	}
	wantTotal := arena.allocatedBytes
	arena.recomputeAllocatedBytes()
	if got := arena.allocatedBytes; got != wantTotal {
		t.Fatalf("recomputed arena bytes = %d, want %d", got, wantTotal)
	}
}

func TestNodeFieldMetadataDeepCopySurvivesOriginalRelease(t *testing.T) {
	sourceArena := acquireNodeArena(arenaClassIncremental)
	child := newLeafNodeInArena(sourceArena, 1, true, 0, 1, Point{}, Point{Column: 1})
	ids := sourceArena.allocFieldIDSlice(1)
	ids[0] = 2
	sources := sourceArena.allocFieldSourceSlice(1)
	sources[0] = fieldSourceDirect
	root := newParentNodeInArenaWithFieldSources(sourceArena, 3, true, []*Node{child}, ids, sources, 0)
	tree := newTreeWithArenas(root, []byte("x"), testLanguage(), sourceArena, nil)

	copyTree := tree.Copy()
	copyRoot := copyTree.RootNode()
	if copyRoot.fieldMetadata == root.fieldMetadata {
		t.Fatal("deep copy shared field-metadata header")
	}
	copyRoot.fieldIDs()[0] = 9
	copyRoot.fieldSources()[0] = fieldSourceInherited
	if root.fieldIDs()[0] != 2 || root.fieldSources()[0] != fieldSourceDirect {
		t.Fatal("deep copy shared field backing with original")
	}

	tree.Release()
	if got := copyRoot.fieldIDs()[0]; got != 9 {
		t.Fatalf("copied field ID after original release = %d, want 9", got)
	}
	if got := copyRoot.fieldSources()[0]; got != fieldSourceInherited {
		t.Fatalf("copied field source after original release = %d, want inherited", got)
	}
	copyTree.Release()
}

func TestNodeFieldMetadataShallowCloneCopiesHeaderOnly(t *testing.T) {
	sourceArena := newNodeArena(arenaClassIncremental)
	destinationArena := newNodeArena(arenaClassIncremental)
	source := sourceArena.allocNode()
	source.ownerArena = sourceArena
	ids := sourceArena.allocFieldIDSlice(1)
	ids[0] = 3
	sources := sourceArena.allocFieldSourceSlice(1)
	sources[0] = fieldSourceDirect
	source.setFieldMetadata(ids, sources)

	clone := cloneNodeInArena(destinationArena, source)
	if clone.fieldMetadata == source.fieldMetadata {
		t.Fatal("shallow cross-arena clone shared metadata header")
	}
	clone.fieldIDs()[0] = 7
	if got := source.fieldIDs()[0]; got != 7 {
		t.Fatalf("shallow cross-arena clone stopped backing alias: got %d, want 7", got)
	}

	sourceArena.resetNodeFieldMetadataSlabs()
	if got := clone.fieldIDs()[0]; got != 7 {
		t.Fatalf("destination header depended on source sidecar slab: got %d, want 7", got)
	}
}

func TestNodeFieldMetadataOffsetCloneIsIndependent(t *testing.T) {
	child := NewLeafNode(1, true, 2, 3, Point{Column: 2}, Point{Column: 3})
	root := NewParentNode(3, true, []*Node{child}, []FieldID{2}, 0)
	offset := cloneTreeNodesWithOffset(root, 5, Point{Column: 5})

	if offset.fieldMetadata == root.fieldMetadata {
		t.Fatal("offset clone shared field-metadata header")
	}
	offset.fieldIDs()[0] = 8
	if got := root.fieldIDs()[0]; got != 2 {
		t.Fatalf("offset clone shared field backing: original ID = %d", got)
	}
	if got := offset.startByte; got != root.startByte+5 {
		t.Fatalf("offset start byte = %d, want %d", got, root.startByte+5)
	}
}

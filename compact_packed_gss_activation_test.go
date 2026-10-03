package gotreesitter

import "testing"

func TestCompactPackedGSSVersionOrderRequiresIncrementalCertification(t *testing.T) {
	certified := &Language{CompactPackedGSSVersionOrderCertified: true}
	uncertified := &Language{}
	incrementalCertified := &Language{CompactPackedGSSVersionOrderCertified: true, IncrementalPackedGSSVersionOrderCertified: true}
	reuse := &reuseCursor{}
	oldTree := &Tree{}
	tests := []struct {
		name                string
		language            *Language
		reuse               *reuseCursor
		oldTree             *Tree
		noTreeBenchmarkOnly bool
		want                bool
	}{
		{name: "fresh certified", language: certified, want: true},
		{name: "fresh uncertified", language: uncertified},
		{name: "reuse cursor", language: certified, reuse: reuse},
		{name: "old tree", language: certified, oldTree: oldTree},
		{name: "incremental", language: certified, reuse: reuse, oldTree: oldTree},
		{name: "language certification without reuse proof", language: incrementalCertified, reuse: reuse, oldTree: oldTree},
		{name: "certified incremental with reuse proof", language: incrementalCertified, reuse: &reuseCursor{cEquivalentReuse: true}, oldTree: oldTree, want: true},
		{name: "incremental without fresh certification", language: &Language{IncrementalPackedGSSVersionOrderCertified: true}, reuse: reuse, oldTree: oldTree},
		{name: "proven C-equivalent reuse", language: certified, reuse: &reuseCursor{cEquivalentReuse: true}, oldTree: oldTree, want: true},
		{name: "unproven language with C-equivalent reuse", language: uncertified, reuse: &reuseCursor{cEquivalentReuse: true}, oldTree: oldTree},
		{name: "no-tree C-equivalent reuse", language: certified, reuse: &reuseCursor{cEquivalentReuse: true}, oldTree: oldTree, noTreeBenchmarkOnly: true},
		{name: "no-tree benchmark", language: certified, noTreeBenchmarkOnly: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := compactPackedGSSVersionOrderActiveForParse(test.language, test.reuse, test.oldTree, test.noTreeBenchmarkOnly); got != test.want {
				t.Fatalf("active = %t, want %t", got, test.want)
			}
		})
	}
}

func TestGLRMergeScratchResetClearsPackedGSSVersionOrder(t *testing.T) {
	scratch := &glrMergeScratch{packedGSSVersionOrderActive: true}
	scratch.reset()
	if scratch.packedGSSVersionOrderActive {
		t.Fatal("merge scratch retained packed GSS version order")
	}
}

func TestCompactPackedGSSZeroChildReceiptStampsOnlyActiveClassicParse(t *testing.T) {
	language := &Language{CompactPackedGSSVersionOrderCertified: true}
	parser := &Parser{
		language: language,
		mergeScratch: &glrMergeScratch{
			language:                    language,
			packedGSSVersionOrderActive: true,
		},
	}
	var active rawShapeRef
	parser.stampCompactPackedGSSZeroChildReceipt(&active)
	if active != rawShapeZeroChildRef {
		t.Fatalf("active receipt = %d, want zero-child receipt", active)
	}

	parser.mergeScratch.packedGSSVersionOrderActive = false
	var inactive rawShapeRef
	parser.stampCompactPackedGSSZeroChildReceipt(&inactive)
	if inactive != 0 {
		t.Fatalf("inactive receipt = %d, want unknown", inactive)
	}
}

func TestGenericLeafConstructorsDoNotStampParserReceipts(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	entries := map[string]stackEntry{
		"node": newStackEntryNode(1,
			newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})),
		"no-tree": newStackEntryNoTreeNode(1,
			newNoTreeLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})),
		"compact-checkpoint": newStackEntryCompactCheckpointLeaf(1,
			newCompactCheckpointLeafInArena(arena, 1, true, 0, 1, externalScannerCheckpointRef{})),
		"compact-full": newStackEntryCompactFullLeaf(1,
			newCompactFullLeafInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})),
	}
	for name, entry := range entries {
		if got := stackEntryRawShapeRef(entry); got != 0 {
			t.Fatalf("%s generic leaf receipt = %d, want unknown", name, got)
		}
	}
}

func TestCompactPackedGSSExpandedLeafInvalidatesZeroChildReceipt(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	language := &Language{CompactPackedGSSVersionOrderCertified: true}
	scratch := &glrMergeScratch{
		language:                    language,
		packedGSSVersionOrderActive: true,
		arena:                       arena,
	}
	parser := &Parser{language: language, mergeScratch: scratch}
	scratch.parser = parser

	parent := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	parser.stampCompactPackedGSSZeroChildReceipt(&parent.rawShape)
	child := newLeafNodeInArena(arena, 2, false, 0, 1, Point{}, Point{Column: 1})
	for range 4096 {
		parent.children = append(parent.children, child)
		invalidateRawShapeAfterChildMutation(parent)
	}

	if parent.rawShape != 0 {
		t.Fatalf("expanded leaf receipt = %d, want unknown", parent.rawShape)
	}
	if _, ok := cStackLinkPayloadHeader(scratch, newStackEntryNode(1, parent)); ok {
		t.Fatal("expanded leaf retained a false zero-child header")
	}
	for i, slab := range arena.rawShapeSlabs {
		if slab.used != 0 {
			t.Fatalf("raw-shape slab %d used = %d after append-only invalidation", i, slab.used)
		}
	}
	for i, slab := range arena.rawShapeChildSlabs {
		if slab.used != 0 {
			t.Fatalf("raw-shape child slab %d used = %d after append-only invalidation", i, slab.used)
		}
	}
}

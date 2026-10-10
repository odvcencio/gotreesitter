//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/incr"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func compactReusePublicationFixtureView(core.SubtreeID) (core.MaterializationSubtreeView, error) {
	return core.MaterializationSubtreeView{StartByte: 0, EndByte: 1}, nil
}

func TestCompactReuseDependencyPublicationAndReset(t *testing.T) {
	p, session, node, _, _, points := compactBorrowedMaterializationFixture(t)
	root := newParentNodeInArena(node.ownerArena, 4, true, []*Node{session.oldTree.root}, nil, 0)
	s := &diagnosticParserCoreGenericScheduler{}
	base := diagnosticParserCoreSchedulerFootprintBytes(s)
	s.reuseDependencies.ends = []uint32{0, 4, 6, 0}
	publish := func(nodes []*Node) {
		t.Helper()
		if err := s.publishCompactReuseDependencies(p, root, node.ownerArena, nodes, compactReusePublicationFixtureView, &points, 0, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	publish([]*Node{nil, node, node})
	if bytes, ok := compactReuseDependencyForNode(node); !ok || bytes != 5 {
		t.Fatalf("outer receipt=%d/%t", bytes, ok)
	}
	publish([]*Node{nil, node, node, node})
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("unknown outer projection retained inner proof")
	}
	publish([]*Node{nil, node})
	publish([]*Node{nil, cloneNodeInArena(node.ownerArena, node)})
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("unmatched projection retained a receipt")
	}
	publish([]*Node{nil, node})
	s.reuseDependencies.disabled = true
	publish([]*Node{nil, node})
	if _, ok := compactReuseDependencyForNode(node); ok {
		t.Fatal("disabled producer retained a copied receipt")
	}
	if got := diagnosticParserCoreSchedulerFootprintBytes(s) - base; got != 16 {
		t.Fatalf("dependency footprint=%d, want16", got)
	}
	if err := resetDiagnosticParserCoreGenericScheduler(s); err != nil {
		t.Fatal(err)
	}
	if len(s.reuseDependencies.ends) != 0 || s.reuseDependencies.frontier != 0 || s.reuseDependencies.disabled {
		t.Fatal("reset retained a receipt")
	}
	for _, end := range s.reuseDependencies.ends[:cap(s.reuseDependencies.ends)] {
		if end != 0 {
			t.Fatal("reset retained an authenticated payload in scratch storage")
		}
	}
}

func TestCompactReuseDependencyBoundedScratchRetention(t *testing.T) {
	for _, count := range []int{0, 1, compactReuseDependencyRetainedEntries, compactReuseDependencyRetainedEntries + 1} {
		d := compactReuseDependencies{ends: make([]uint32, count), frontier: 9, disabled: true}
		for i := range d.ends {
			d.ends[i] = uint32(i + 1)
		}
		d.ends = d.ends[:count/2]
		d = d.reset()
		if len(d.ends) != 0 || d.frontier != 0 || d.disabled {
			t.Fatalf("reset retained producer state for capacity %d", count)
		}
		if count > compactReuseDependencyRetainedEntries {
			if d.ends != nil {
				t.Fatal("reset retained oversized dependency scratch")
			}
			continue
		}
		if cap(d.ends) != count {
			t.Fatal("reset discarded bounded dependency scratch")
		}
		for _, end := range d.ends[:cap(d.ends)] {
			if end != 0 {
				t.Fatal("reset retained stale dependency authorization")
			}
		}
	}
}

func TestCompactReuseDependencyRetainedScratchBudgetAndBusyReset(t *testing.T) {
	s := &diagnosticParserCoreGenericScheduler{}
	base := diagnosticParserCoreSchedulerFootprintBytes(s)
	s.reuseDependencies.ends = make([]uint32, 1, compactReuseDependencyRetainedEntries)
	s.reuseDependencies.ends[0] = 7
	s.dispatchScratch.busy = true
	if err := resetDiagnosticParserCoreGenericScheduler(s); err == nil || s.reuseDependencies.ends[0] != 7 {
		t.Fatal("busy reset changed dependency authorization")
	}
	s.dispatchScratch.busy = false
	if err := resetDiagnosticParserCoreGenericScheduler(s); err != nil {
		t.Fatal(err)
	}
	if got := diagnosticParserCoreSchedulerFootprintBytes(s) - base; got != 64*1024 {
		t.Fatalf("retained dependency footprint=%d, want 65536", got)
	}
	s.options.stopControlMemoryBudgetBytes = 32 * 1024
	if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(0); !resultMaterializationShouldStop(reason) {
		t.Fatal("the next parse omitted retained scratch from its budget")
	}
}

func TestCompactReuseDependencyRetainsLeafScratchWithinSharedLimit(t *testing.T) {
	s := &diagnosticParserCoreGenericScheduler{}
	base := diagnosticParserCoreSchedulerFootprintBytes(s)
	s.reuseDependencies = compactReuseDependencies{
		ends:      make([]uint32, 1, 128),
		leafWords: make([]uint32, 1, compactReuseDependencyRetainedEntries),
		frontier:  9, disabled: true, readsAllocated: 123,
	}
	leaves := s.reuseDependencies.leafWords[:cap(s.reuseDependencies.leafWords)]
	leaves[0], leaves[len(leaves)-1] = legacyReuseLeafKnown, legacyReuseKeyword
	if err := resetDiagnosticParserCoreGenericScheduler(s); err != nil {
		t.Fatal(err)
	}
	d := &s.reuseDependencies
	if cap(d.ends) != 0 || cap(d.leafWords) != compactReuseDependencyRetainedEntries || len(d.leafWords) != 0 ||
		d.frontier != 0 || d.disabled || d.reads != nil || d.readsAllocated != 0 {
		t.Fatal("reset discarded bounded leaf scratch or retained producer state")
	}
	for _, value := range leaves {
		if value != 0 {
			t.Fatal("reset retained stale leaf authorization")
		}
	}
	if got := diagnosticParserCoreSchedulerFootprintBytes(s) - base; got != 64*1024 {
		t.Fatalf("retained leaf footprint=%d, want 65536", got)
	}
	s.options.stopControlMemoryBudgetBytes = 32 * 1024
	if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(0); !resultMaterializationShouldStop(reason) {
		t.Fatal("the next parse omitted retained leaf scratch from its budget")
	}
}

func TestCompactReuseDependencyPublishesOnlyEligibleDepth(t *testing.T) {
	p, session, node, _, _, points := compactBorrowedMaterializationFixture(t)
	item := session.oldTree.root
	root := newParentNodeInArena(node.ownerArena, 4, true, []*Node{item}, nil, 0)
	deep := newParentNodeInArena(node.ownerArena, 3, true, node.children, nil, 0)
	deep.setCompactMaterialized(true)
	deep.setCompactParseStateProof(true)
	deep.setCompactPreGotoStateProof(true)
	node.children = []*Node{deep}
	deep.parent = node
	s := &diagnosticParserCoreGenericScheduler{}
	s.reuseDependencies.ends = []uint32{0, 4, 5, 6, 7}
	if err := s.publishCompactReuseDependencies(p, root, node.ownerArena, []*Node{nil, deep, node, item, root}, compactReusePublicationFixtureView, &points, 0, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, ignored := range []*Node{deep, item, root} {
		if _, ok := compactReuseDependencyForNode(ignored); ok {
			t.Fatal("publication allocated a receipt outside the candidate depth")
		}
	}
	if bytes, ok := compactReuseDependencyForNode(node); !ok || bytes != 4 {
		t.Fatalf("candidate receipt=%d/%t, want4/true", bytes, ok)
	}
	other := acquireNodeArena(arenaClassFull)
	defer other.Release()
	if err := s.publishCompactReuseDependencies(p, root, other, []*Node{nil, node}, compactReusePublicationFixtureView, &points, 0, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if bytes, ok := compactReuseDependencyForNode(node); !ok || bytes != 4 {
		t.Fatal("publication changed a borrowed arena receipt")
	}
}

func TestCompactReuseDependencyRejectsChangedPublicationGeometry(t *testing.T) {
	for _, change := range []string{"start", "end", "start_point", "end_point", "missing_view", "invalid_view", "missing_points"} {
		t.Run(change, func(t *testing.T) {
			p, session, node, _, _, points := compactBorrowedMaterializationFixture(t)
			root := newParentNodeInArena(node.ownerArena, 4, true, []*Node{session.oldTree.root}, nil, 0)
			s := &diagnosticParserCoreGenericScheduler{}
			s.reuseDependencies.ends = []uint32{0, 6}
			node.startByte, node.endByte = 1, 3
			node.startPoint, node.endPoint = Point{Column: 1}, Point{Column: 3}
			viewFor := func(core.SubtreeID) (core.MaterializationSubtreeView, error) {
				return core.MaterializationSubtreeView{StartByte: 1, EndByte: 3}, nil
			}
			pointIndex := &points
			switch change {
			case "start":
				node.startByte, node.startPoint = 0, Point{}
			case "end":
				node.endByte, node.endPoint = 4, Point{Column: 4}
			case "start_point":
				node.startPoint.Column++
			case "end_point":
				node.endPoint.Row++
			case "missing_view":
				viewFor = nil
			case "invalid_view":
				viewFor = func(core.SubtreeID) (core.MaterializationSubtreeView, error) {
					return core.MaterializationSubtreeView{}, errors.New("unknown payload")
				}
			case "missing_points":
				pointIndex = nil
			}
			// Seed a copied receipt at the final geometry. Publication must not
			// preserve it merely because the public pointer still matches.
			if !setCompactReuseDependency(node, 0) {
				t.Fatal("could not seed the copied receipt")
			}
			if err := s.publishCompactReuseDependencies(p, root, node.ownerArena, []*Node{nil, node}, viewFor, pointIndex, 0, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if _, ok := compactReuseDependencyForNode(node); ok {
				t.Fatal("changed or unknown producer geometry retained a receipt")
			}
		})
	}
}

func TestCompactReuseDependencyBudgetAndPanic(t *testing.T) {
	s := &diagnosticParserCoreGenericScheduler{}
	s.options.stopControlMemoryBudgetBytes = 64
	if err := s.growCompactReuseDependencies(128); err == nil {
		t.Fatal("dependency storage exceeded its budget")
	}
	if cap(s.reuseDependencies.ends) != 0 {
		t.Fatal("budget rejection allocated dependency storage")
	}
	s.reuseDependencies.ends = []uint32{0, 4}
	func() {
		defer func() {
			if recover() != "injected" {
				t.Fatal("dependency cleanup lost the panic")
			}
		}()
		var err error
		defer s.endCompactReuseDependency(1, true, &err)
		panic("injected")
	}()
	if !s.reuseDependencies.disabled || s.reuseDependencies.ends[1] != 0 {
		t.Fatal("panic retained dependency authorization")
	}
}

func TestCompactLegacyReadsPreserveBoundaryAndProjectionProofs(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	reads := incr.NewReads(32)
	for _, probe := range [][2]uint32{{0, 4}, {4, 8}, {8, 16}, {16, 24}, {24, 33}} {
		reads.Record(int(probe[0]), probe[1])
	}
	s := &diagnosticParserCoreGenericScheduler{}
	s.reuseDependencies.reads = reads
	points := diagnosticParserCorePointIndex{lineStarts: []uint32{0}}
	nodes := []*Node{nil}
	geometry := []core.SubtreeGeometry{{}}
	// Both cursors move forward and backward, and both visit the same byte
	// boundary. A parent includes a probe starting there; a leaf excludes it.
	for _, end := range []uint32{4, 16, 8, 24, 4, 32} {
		leaf := newLeafNodeInArena(arena, 1, true, 0, end, Point{}, Point{Column: end})
		parent := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)
		for _, node := range []*Node{leaf, parent} {
			node.setCompactMaterialized(true)
			node.setCompactPreGotoStateProof(true)
			node.setCompactParseStateProof(true)
			nodes = append(nodes, node)
			geometry = append(geometry, core.SubtreeGeometry{EndByte: end})
		}
	}
	// A collapsed outer projection with a different extent invalidates the
	// shared public node even though its inner projection matched.
	nodes = append(nodes, nodes[1], nodes[3], nodes[5])
	geometry = append(geometry, core.SubtreeGeometry{EndByte: 5}, geometry[3], geometry[5])
	nodes[3].endPoint.Column++ // A compatibility rewrite also revokes proof.
	failedID := core.SubtreeID(len(nodes) - 1)
	s.reuseDependencies.leafWords = make([]uint32, len(nodes))
	s.reuseDependencies.leafWords[7] = legacyReuseLeafKnown | legacyReuseKeyword
	err := s.publishCompactLegacyReads(arena, nodes, func(id core.SubtreeID) (core.SubtreeGeometry, error) {
		if id == failedID {
			return core.SubtreeGeometry{}, errors.New("unavailable projection")
		}
		return geometry[id], nil
	}, &points, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for id, node := range nodes[1:13] {
		got, ok := legacyReuseLookahead(node)
		if node == nodes[1] || node == nodes[3] || node == nodes[5] {
			if ok {
				t.Fatalf("projection %d retained an unauthenticated receipt", id+1)
			}
			continue
		}
		want, known := reads.Lookahead(node.EndByte())
		if node.ChildCount() == 0 {
			want, known = reads.LeafLookahead(node.EndByte())
		}
		if got != want || ok != known {
			t.Fatalf("projection %d lookahead=%d/%t, want %d/%t", id+1, got, ok, want, known)
		}
	}
	if word := legacyReuseWord(nodes[7], false); word == nil || *word&(legacyReuseLeafKnown|legacyReuseKeyword) != legacyReuseLeafKnown|legacyReuseKeyword {
		t.Fatal("publication lost elected keyword provenance")
	}
	// Published words belong to the tree, not to the runner's scan history.
	wantWord := *legacyReuseWord(nodes[7], false)
	s.reuseDependencies = s.reuseDependencies.reset()
	reads.Reset(1)
	reads.Record(0, 2)
	reads.Seal()
	if got := *legacyReuseWord(nodes[7], false); got != wantWord {
		t.Fatalf("reusing scan scratch changed a published receipt: %d -> %d", wantWord, got)
	}
}

func TestCompactLegacyReadScratchReuse(t *testing.T) {
	d := &dfaTokenSource{language: &Language{}, lexer: NewLexer(nil, make([]byte, 512))}
	s := &diagnosticParserCoreGenericScheduler{tokenSource: d}
	s.beginCompactLegacyReads()
	reads := s.reuseDependencies.reads
	if reads == nil || d.lexer.reuseReads != reads {
		t.Fatal("eligible producer did not attach its read history")
	}
	for origin := 0; origin < 256; origin++ {
		reads.Record(origin, uint32(origin+1))
	}
	reads.Seal()
	bytes := reads.Bytes()
	// The scheduler run restores the observer before it returns its lease.
	d.lexer.reuseReads = nil
	s.reuseDependencies = s.reuseDependencies.reset()
	if s.reuseDependencies.reads != nil || s.reuseDependencies.idleReads != reads || reads.Bytes() != bytes ||
		reads.Recording() || reads.ValidForSource(512) || reads.SourceBytes() != 0 {
		t.Fatal("reset lost reusable storage or retained old authorization")
	}
	allocs := testing.AllocsPerRun(5, func() {
		s.beginCompactLegacyReads()
		for origin := 256; origin < 512; origin++ {
			s.reuseDependencies.reads.Record(origin, uint32(origin+1))
		}
		s.reuseDependencies.reads.Seal()
		d.lexer.reuseReads = nil
		s.reuseDependencies = s.reuseDependencies.reset()
	})
	if allocs != 0 {
		t.Fatalf("warm scan histories allocated %g times", allocs)
	}
	s.beginCompactLegacyReads()
	reads.Record(300, 400)
	reads.Seal()
	if _, known := reads.Lookahead(128); known {
		t.Fatal("new parse authenticated an earlier parse's probes")
	}
	if got, known := reads.Lookahead(350); !known || got != 50 {
		t.Fatalf("new read bound=%d/%t, want 50/true", got, known)
	}
}

func TestCompactLegacyReadScratchSharedLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		leaves int
		probes int
		keep   bool
		buffer bool
	}{
		{"bounded history", 128, 256, true, true},
		{"oversized history", 0, 10000, true, false},
		{"leaves leave header only", compactReuseDependencyRetainedEntries - 16, 256, true, false},
		{"leaves consume allowance", compactReuseDependencyRetainedEntries, 256, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &diagnosticParserCoreGenericScheduler{}
			base := diagnosticParserCoreSchedulerFootprintBytes(s)
			reads := incr.NewReads(tc.probes)
			var oldCharge int64
			reads.BindBudget(0, 0, &oldCharge)
			for origin := 0; origin < tc.probes; origin++ {
				reads.Record(origin, uint32(origin+1))
			}
			s.reuseDependencies = compactReuseDependencies{reads: reads, leafWords: make([]uint32, tc.leaves)}
			s.reuseDependencies = s.reuseDependencies.reset()
			idle := s.reuseDependencies.idleReads
			if (idle != nil) != tc.keep || s.reuseDependencies.reads != nil {
				t.Fatal("wrong active/idle ownership after reset")
			}
			if idle != nil && (idle.Bytes() > 64) != tc.buffer {
				t.Fatalf("unexpected retained scan capacity: %d bytes", idle.Bytes())
			}
			retained := diagnosticParserCoreSchedulerFootprintBytes(s) - base
			if retained > 64*1024 {
				t.Fatalf("dependency scratch retained %d bytes, exceeds 65536", retained)
			}
			want := uint64(tc.leaves * 4)
			if idle != nil {
				want += uint64(idle.Bytes())
			}
			if retained != want {
				t.Fatalf("idle footprint=%d, want %d", retained, want)
			}
			// Reset must detach the old request's budget even if the object
			// itself is dropped from the runner because the allowance is full.
			before := oldCharge
			reads.TrimCapacity(0)
			reads.Reset(1)
			reads.Record(0, 2)
			if oldCharge != before {
				t.Fatal("reset retained the previous request's budget pointer")
			}
		})
	}
}

func TestCompactLegacyReadScratchInactiveProducer(t *testing.T) {
	d := &dfaTokenSource{language: &Language{}, lexer: NewLexer(nil, []byte("abc"))}
	s := &diagnosticParserCoreGenericScheduler{tokenSource: d}
	s.beginCompactLegacyReads()
	reads := s.reuseDependencies.reads
	reads.Record(0, 2)
	d.lexer.reuseReads = nil
	s.reuseDependencies = s.reuseDependencies.reset()
	s.options.compactIncrementalReuse = &compactIncrementalReuseSession{}
	s.beginCompactLegacyReads()
	if s.reuseDependencies.reads != nil || d.lexer.reuseReads != nil || s.reuseDependencies.idleReads != nil {
		t.Fatal("ineligible producer attached or retained an idle scan history")
	}
	var err error
	s.endCompactLeafReceipt(0, Token{}, &err)
	if err != nil || len(s.reuseDependencies.leafWords) != 0 {
		t.Fatal("ineligible producer recorded leaf provenance")
	}
}

func TestCompactLegacyReadScratchBudget(t *testing.T) {
	d := &dfaTokenSource{language: &Language{}, lexer: NewLexer(nil, make([]byte, 1024))}
	s := &diagnosticParserCoreGenericScheduler{tokenSource: d}
	base := int64(diagnosticParserCoreSchedulerFootprintBytes(s))
	s.beginCompactLegacyReads()
	reads := s.reuseDependencies.reads
	for origin := 0; origin < 512; origin++ {
		reads.Record(origin, uint32(origin+1))
	}
	d.lexer.reuseReads = nil
	s.reuseDependencies = s.reuseDependencies.reset()
	retained := reads.Bytes()
	s.options.stopControlMemoryBudgetBytes = base + retained + 1
	s.beginCompactLegacyReads()
	if s.reuseDependencies.reads != reads || s.reuseDependencies.idleReads != nil ||
		reads.Bytes() != retained || s.reuseDependencies.readsAllocated != base+retained {
		t.Fatal("warm capture double-counted its retained history")
	}
	if got := int64(diagnosticParserCoreSchedulerFootprintBytes(s)); got != base+retained {
		t.Fatalf("active footprint=%d, want %d", got, base+retained)
	}
	d.lexer.reuseReads = nil
	s.reuseDependencies = s.reuseDependencies.reset()
	// Leave room for the header and one 128-entry growth, but not 256.
	s.options.stopControlMemoryBudgetBytes = base + 64 + 128*8 + 1
	s.beginCompactLegacyReads()
	if s.reuseDependencies.reads != reads || reads.Bytes() != 64 || !reads.Recording() {
		t.Fatal("old capacity prevented capture within the smaller budget")
	}
	for origin := 0; origin < 128; origin++ {
		reads.Record(origin, uint32(origin+1))
	}
	if !reads.Recording() || s.reuseDependencies.readsAllocated != base+64+128*8 {
		t.Fatal("new budget charged an allowed growth incorrectly")
	}
	reads.Record(128, 129)
	if reads.Recording() || reads.Bytes() != 64+128*8 || s.reuseDependencies.readsAllocated != base+64+128*8 {
		t.Fatal("scan history grew beyond the smaller request's budget")
	}
}

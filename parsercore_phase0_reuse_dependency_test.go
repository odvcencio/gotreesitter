//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"testing"

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

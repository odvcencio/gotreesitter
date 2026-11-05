//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"errors"
	"testing"
	"time"
)

func phase0ASelectedCapabilityFixture(t *testing.T) (*Core, CoreRunNamespace, Phase0ASelectedOccurrenceCapability) {
	t.Helper()
	core, run, head := phase0ASelectedSingleReduction(t, phase0ARouteLimits())
	accepted := phase0ACaptureSelection(t, core, head)
	if err := core.ObservePhase0AAcceptedSelection(accepted); err != nil {
		t.Fatal(err)
	}
	capability, err := CapturePhase0ASelectedOccurrenceCapability(core, run)
	if err != nil {
		t.Fatal(err)
	}
	return core, run, capability
}

func TestPhase0ASelectedOccurrenceCapabilityBorrowsExactStatesAndSpan(t *testing.T) {
	core, run, capability := phase0ASelectedCapabilityFixture(t)
	if count := capability.Count(); count != 2 {
		t.Fatalf("selected occurrence count=%d, want 2", count)
	}
	current, err := CaptureCurrentPhase0ASelectedOccurrenceCapability(core)
	if err != nil || current != capability {
		t.Fatalf("current capability=%+v want=%+v err=%v", current, capability, err)
	}
	var got []Phase0ASelectedOccurrenceView
	polls := 0
	if err := VisitPhase0ASelectedOccurrences(core, capability, func() error {
		polls++
		return nil
	}, func(view Phase0ASelectedOccurrenceView) error {
		got = append(got, view)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if polls != 2 {
		t.Fatalf("polls=%d, want start/end", polls)
	}
	if len(got) != 2 ||
		got[0].Ordinal != 1 || got[0].Parent != 0 || got[0].ChildOrdinal != 0 || got[0].Depth != 0 || got[0].ConstructionKind != Phase0AConstructionReductionParent || got[0].ChildCount != 1 || got[0].ParseState != 3 || got[0].PreGotoState != 1 || got[0].SubtreeCount != 2 ||
		got[1].Ordinal != 2 || got[1].Parent != 1 || got[1].ChildOrdinal != 0 || got[1].Depth != 1 || got[1].ConstructionKind != Phase0AConstructionOrdinaryTerminal || got[1].ChildCount != 0 || got[1].ParseState != 2 || got[1].PreGotoState != 1 || got[1].SubtreeCount != 1 {
		t.Fatalf("selected occurrence views=%+v", got)
	}
	proof, err := Phase0AObserverProof(core, run)
	if err != nil || len(proof.SelectedOccurrences) != len(got) {
		t.Fatalf("proof rows=%d views=%d err=%v", len(proof.SelectedOccurrences), len(got), err)
	}
}

func TestPhase0ASelectedOccurrenceCapabilityRejectsStaleAndForgedWindows(t *testing.T) {
	core, run, capability := phase0ASelectedCapabilityFixture(t)
	visit := func(Phase0ASelectedOccurrenceView) error { return nil }
	for name, forged := range map[string]Phase0ASelectedOccurrenceCapability{
		"core":       {coreInstance: capability.coreInstance + 1, runGeneration: capability.runGeneration, generation: capability.generation, first: capability.first, count: capability.count},
		"run":        {coreInstance: capability.coreInstance, runGeneration: capability.runGeneration + 1, generation: capability.generation, first: capability.first, count: capability.count},
		"generation": {coreInstance: capability.coreInstance, runGeneration: capability.runGeneration, generation: capability.generation + 1, first: capability.first, count: capability.count},
		"first":      {coreInstance: capability.coreInstance, runGeneration: capability.runGeneration, generation: capability.generation, first: capability.first + 1, count: capability.count},
		"count":      {coreInstance: capability.coreInstance, runGeneration: capability.runGeneration, generation: capability.generation, first: capability.first, count: capability.count + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := VisitPhase0ASelectedOccurrences(core, forged, nil, visit); !isPhase0AError(err, Phase0AErrorStaleReference, "") && !isPhase0AError(err, Phase0AErrorStaleNamespace, "") {
				t.Fatalf("forged capability error=%v", err)
			}
		})
	}
	if err := core.EndRun(run); err != nil {
		t.Fatal(err)
	}
	if err := VisitPhase0ASelectedOccurrences(core, capability, nil, visit); !isPhase0AError(err, Phase0AErrorStaleNamespace, "") {
		t.Fatalf("ended-run capability error=%v", err)
	}
	next, err := core.BeginRun()
	if err != nil {
		t.Fatal(err)
	}
	if next.RunGeneration != run.RunGeneration+1 {
		t.Fatalf("next run=%+v after=%+v", next, run)
	}
	if err := VisitPhase0ASelectedOccurrences(core, capability, nil, visit); !isPhase0AError(err, Phase0AErrorStaleNamespace, "") {
		t.Fatalf("reused-run capability error=%v", err)
	}
}

func TestPhase0ASelectedOccurrenceCapabilityHonorsPollAndCurrentCaps(t *testing.T) {
	core, _, capability := phase0ASelectedCapabilityFixture(t)
	forced := errors.New("stop selected visit")
	visits := 0
	if err := VisitPhase0ASelectedOccurrences(core, capability, func() error { return forced }, func(Phase0ASelectedOccurrenceView) error {
		visits++
		return nil
	}); !errors.Is(err, forced) || visits != 0 {
		t.Fatalf("poll cancellation visits=%d err=%v", visits, err)
	}
	phase0AObservers.Lock()
	prior := phase0AObservers.limits.MaxSelectedOccurrences
	phase0AObservers.limits.MaxSelectedOccurrences = 1
	phase0AObservers.Unlock()
	if err := VisitPhase0ASelectedOccurrences(core, capability, nil, func(Phase0ASelectedOccurrenceView) error { return nil }); !isPhase0AError(err, Phase0AErrorRecordCap, "") {
		t.Fatalf("visitor row cap error=%v", err)
	}
	phase0AObservers.Lock()
	phase0AObservers.limits.MaxSelectedOccurrences = prior
	phase0AObservers.Unlock()
}

func TestPhase0ASelectedOccurrenceCapabilityKeepsRepeatedPayloadIdentity(t *testing.T) {
	core, run, head, extra := phase0AObservedTrailingReductionFixture(t, true, false)
	frontier, err := core.Reduce(head, 1, 0, ForkOrder{})
	if err != nil || len(frontier) != 1 {
		t.Fatalf("duplicate payload reduction=%v err=%v", frontier, err)
	}
	accepted := phase0ACaptureSelection(t, core, frontier[0])
	if err := core.ObservePhase0AAcceptedSelection(accepted); err != nil {
		t.Fatal(err)
	}
	capability, err := CapturePhase0ASelectedOccurrenceCapability(core, run)
	if err != nil {
		t.Fatal(err)
	}
	var repeated []Phase0ASelectedOccurrenceView
	if err := VisitPhase0ASelectedOccurrences(core, capability, nil, func(view Phase0ASelectedOccurrenceView) error {
		if view.Payload == extra {
			repeated = append(repeated, view)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(repeated) != 2 || repeated[0].Ordinal != 3 || repeated[1].Ordinal != 4 ||
		repeated[0].Parent != 0 || repeated[1].Parent != 0 || repeated[0].ChildOrdinal != 1 || repeated[1].ChildOrdinal != 2 ||
		repeated[0].ParseState != 4 || repeated[0].PreGotoState != 4 || repeated[1].ParseState != 4 || repeated[1].PreGotoState != 4 ||
		repeated[0].SubtreeCount != 1 || repeated[1].SubtreeCount != 1 || !repeated[0].Migrated || !repeated[1].Migrated {
		t.Fatalf("repeated payload occurrence views=%+v", repeated)
	}
}

func TestPhase0ASelectedOccurrenceCallbackCanReenterReadOnlyObserverAPIs(t *testing.T) {
	core, run, capability := phase0ASelectedCapabilityFixture(t)
	done := make(chan error, 1)
	go func() {
		done <- VisitPhase0ASelectedOccurrences(core, capability, func() error {
			_, err := Phase0AObserverProof(core, run)
			return err
		}, func(Phase0ASelectedOccurrenceView) error {
			current, err := CaptureCurrentPhase0ASelectedOccurrenceCapability(core)
			if err != nil {
				return err
			}
			if current != capability {
				return errors.New("reentrant capability changed")
			}
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("selected occurrence callback deadlocked on read-only observer re-entry")
	}
}

func TestPhase0ASelectedOccurrenceBorrowRejectsLifecycleMutationAtomically(t *testing.T) {
	core, run, capability := phase0ASelectedCapabilityFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		first := true
		done <- VisitPhase0ASelectedOccurrences(core, capability, nil, func(Phase0ASelectedOccurrenceView) error {
			if first {
				first = false
				close(entered)
				<-release
			}
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("selected occurrence borrow did not enter callback")
	}
	shapeBefore := [6]uint64{uint64(core.nodes.Len()), uint64(core.links.Len()), uint64(core.subtrees.Len()), uint64(len(core.children)), uint64(core.frontier), core.classificationPhase}
	workBefore := core.Work()
	if _, err := core.BeginRun(); !isPhase0AError(err, Phase0AErrorRunActive, "") {
		t.Fatalf("begin run during borrow error=%v", err)
	}
	if err := core.EndRun(run); !isPhase0AError(err, Phase0AErrorRunActive, "") {
		t.Fatalf("end run during borrow error=%v", err)
	}
	if err := core.Reset(); !isPhase0AError(err, Phase0AErrorRunActive, "") {
		t.Fatalf("reset during borrow error=%v", err)
	}
	shapeAfter := [6]uint64{uint64(core.nodes.Len()), uint64(core.links.Len()), uint64(core.subtrees.Len()), uint64(len(core.children)), uint64(core.frontier), core.classificationPhase}
	workAfter := core.Work()
	if shapeBefore != shapeAfter || workBefore != workAfter {
		t.Fatalf("rejected reset mutated compact state shape=%+v/%+v work=%+v/%+v", shapeBefore, shapeAfter, workBefore, workAfter)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("selected occurrence borrow did not release")
	}
	if err := core.EndRun(run); err != nil {
		t.Fatalf("end run after borrow release: %v", err)
	}
	if err := core.Reset(); err != nil {
		t.Fatalf("reset after borrow release: %v", err)
	}
}

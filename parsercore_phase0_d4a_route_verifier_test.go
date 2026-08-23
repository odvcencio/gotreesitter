//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func g18D4ACompleteCohort(
	t *testing.T,
	provider g18FutureCoreCertificateBehavior,
	heads [3]core.Head,
) g18FutureCohortHandle {
	t.Helper()
	record, digest := g18FutureDerivationRecord()
	handle := g18FutureBeginCohort(t, provider, uint16(len(heads)), uint32(len(record)))
	for index, head := range heads {
		g18FutureWriteMember(t, provider, handle, head, uint16(index), g18FutureActionIdentity(), digest, record)
	}
	if err := provider.DiagnosticDropCohortFinalizeForTest(handle); err != nil {
		t.Fatal(err)
	}
	return handle
}

func g18D4ARefsForHandle(
	t *testing.T,
	compact *core.Core,
	handle g18FutureCohortHandle,
	branch uint16,
) core.DropCohortRefSet {
	t.Helper()
	var refs core.DropCohortRefSet
	ref := core.DropCohortRef{
		Owner: handle[0], Epoch: handle[1], Sequence: handle[2], Branch: branch,
	}
	if !compact.AddDropCohortRef(&refs, ref) {
		t.Fatalf("add cohort reference %+v failed", ref)
	}
	return refs
}

func g18D4ASetHeaders(
	scheduler *diagnosticParserCoreGenericScheduler,
	heads [3]core.Head,
	sets [3]core.DropCohortRefSet,
) {
	scheduler.headers = make([]diagnosticParserCoreHeader, len(heads))
	for index, head := range heads {
		scheduler.headers[index] = diagnosticParserCoreHeader{
			head: head, dropCohortRefs: sets[index], convergedReductionSplit: true,
		}
	}
}

func TestG18D4ASelectorPublishesOneProofForCommonCohort(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	cohort := g18D4ACompleteCohort(t, provider, heads)
	var sets [3]core.DropCohortRefSet
	for index := range sets {
		sets[index] = g18D4ARefsForHandle(t, scheduler.compact, cohort, uint16(index))
	}
	g18D4ASetHeaders(scheduler, heads, sets)
	before := g18FutureDecodeCoreSnapshot(t, provider)
	var proved bool
	err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		scheduler.freshSessionOwner = &owner
		defer func() { scheduler.freshSessionOwner = nil }()
		var err error
		proved, err = scheduler.diagnosticParserCoreDropCohortCertificateProof([]int{1, 2})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !proved {
		t.Fatal("common complete cohort was not proved")
	}
	after := g18FutureDecodeCoreSnapshot(t, provider)
	if after.VerifierElections != before.VerifierElections+1 || after.VerifierProofs != before.VerifierProofs+1 ||
		after.VerifierDeclines != before.VerifierDeclines {
		t.Fatalf("verifier counters before=%+v after=%+v, want one proof", before, after)
	}
}

func TestG18D4ASelectorWarmPathAllocatesZero(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	cohort := g18D4ACompleteCohort(t, provider, heads)
	var sets [3]core.DropCohortRefSet
	for index := range sets {
		sets[index] = g18D4ARefsForHandle(t, scheduler.compact, cohort, uint16(index))
	}
	g18D4ASetHeaders(scheduler, heads, sets)
	err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		scheduler.freshSessionOwner = &owner
		defer func() { scheduler.freshSessionOwner = nil }()
		if proved, err := scheduler.diagnosticParserCoreDropCohortCertificateProof([]int{1, 2}); err != nil || !proved {
			return fmt.Errorf("warm-up proof=%t err=%v", proved, err)
		}
		var invalidCalls uint64
		allocs := testing.AllocsPerRun(1000, func() {
			proved, err := scheduler.diagnosticParserCoreDropCohortCertificateProof([]int{1, 2})
			if err != nil || !proved {
				invalidCalls++
			}
		})
		if allocs != 0 {
			return fmt.Errorf("warmed selector allocations=%v, want zero", allocs)
		}
		if invalidCalls != 0 {
			return fmt.Errorf("warmed selector invalid calls=%d", invalidCalls)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestG18D4ASelectorSkipsUnprovedCommonCohort(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	lower := g18D4ACompleteCohort(t, provider, heads)
	if err := provider.DiagnosticDropCohortMarkUnprovedForTest(lower); err != nil {
		t.Fatal(err)
	}
	higher := g18D4ACompleteCohort(t, provider, heads)
	var sets [3]core.DropCohortRefSet
	for index := range sets {
		sets[index] = g18D4ARefsForHandle(t, scheduler.compact, lower, uint16(index))
		higherRef := core.DropCohortRef{Owner: higher[0], Epoch: higher[1], Sequence: higher[2], Branch: uint16(index)}
		if !scheduler.compact.AddDropCohortRef(&sets[index], higherRef) {
			t.Fatalf("add higher cohort reference %+v failed", higherRef)
		}
	}
	g18D4ASetHeaders(scheduler, heads, sets)
	before := g18FutureDecodeCoreSnapshot(t, provider)
	var proved bool
	err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		scheduler.freshSessionOwner = &owner
		defer func() { scheduler.freshSessionOwner = nil }()
		var err error
		proved, err = scheduler.diagnosticParserCoreDropCohortCertificateProof([]int{1, 2})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !proved {
		t.Fatal("selector did not continue to the higher complete cohort")
	}
	after := g18FutureDecodeCoreSnapshot(t, provider)
	if after.VerifierElections != before.VerifierElections+1 || after.VerifierProofs != before.VerifierProofs+1 ||
		after.VerifierDeclines != before.VerifierDeclines {
		t.Fatalf("verifier counters before=%+v after=%+v, want one published proof", before, after)
	}
}

func TestG18D4ASelectorPreservesNoCommonFallback(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	record, digest := g18FutureDerivationRecord()
	left := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	for index := 0; index < 2; index++ {
		g18FutureWriteMember(t, provider, left, heads[index], uint16(index), g18FutureActionIdentity(), digest, record)
	}
	if err := provider.DiagnosticDropCohortFinalizeForTest(left); err != nil {
		t.Fatal(err)
	}
	right := g18FutureBeginCohort(t, provider, 1, uint32(len(record)))
	g18FutureWriteMember(t, provider, right, heads[2], 0, g18FutureActionIdentity(), digest, record)
	if err := provider.DiagnosticDropCohortFinalizeForTest(right); err != nil {
		t.Fatal(err)
	}
	var sets [3]core.DropCohortRefSet
	sets[0] = g18D4ARefsForHandle(t, scheduler.compact, left, 0)
	sets[1] = g18D4ARefsForHandle(t, scheduler.compact, left, 1)
	sets[2] = g18D4ARefsForHandle(t, scheduler.compact, right, 0)
	g18D4ASetHeaders(scheduler, heads, sets)
	before := g18FutureDecodeCoreSnapshot(t, provider)
	var proved bool
	err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		scheduler.freshSessionOwner = &owner
		defer func() { scheduler.freshSessionOwner = nil }()
		var err error
		proved, err = scheduler.diagnosticParserCoreDropCohortCertificateProof([]int{1, 2})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if proved {
		t.Fatal("selector synthesized proof for disjoint cohorts")
	}
	after := g18FutureDecodeCoreSnapshot(t, provider)
	if after.VerifierElections != before.VerifierElections || after.VerifierProofs != before.VerifierProofs ||
		after.VerifierDeclines != before.VerifierDeclines {
		t.Fatalf("no-common selector changed verifier counters before=%+v after=%+v", before, after)
	}
}

func TestG18D4ASelectorChoosesLowestIdentityAndBranch(t *testing.T) {
	scheduler, _, heads := g18FutureBehaviorFixture(t)
	owner, epoch := scheduler.compact.DropCohortArenaIdentity()
	lower := core.DropCohortRef{Owner: owner, Epoch: epoch, Sequence: 3, Branch: 0}
	higher := core.DropCohortRef{Owner: owner, Epoch: epoch, Sequence: 9, Branch: 0}
	var survivor core.DropCohortRefSet
	survivor.Inline[0], survivor.Inline[1], survivor.Count = higher, lower, 2
	var dropped core.DropCohortRefSet
	dropped.Inline[0], dropped.Inline[1], dropped.Count = lower, higher, 2
	g18D4ASetHeaders(scheduler, heads, [3]core.DropCohortRefSet{survivor, dropped, dropped})
	var candidate core.DropCohortRef
	var selected bool
	err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		scheduler.freshSessionOwner = &owner
		defer func() { scheduler.freshSessionOwner = nil }()
		var err error
		candidate, selected, err = dropCohortVerifierNextCommonIdentity(scheduler, owner, 0, []int{1, 2}, survivor, core.DropCohortRef{})
		if err != nil {
			return err
		}
		if !selected || candidate != lower {
			t.Fatalf("candidate=%+v selected=%t, want lowest identity %+v", candidate, selected, lower)
		}
		if ok, err := dropCohortVerifierSelectBranches(scheduler, owner, 0, []int{1, 2}, candidate); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("lowest candidate branch selection declined")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.verifierRefs[0].Branch != 0 || scheduler.verifierRefs[1].Branch != 0 || scheduler.verifierRefs[2].Branch != 0 {
		t.Fatalf("selected branches=%+v, want branch zero", scheduler.verifierRefs[:3])
	}
}

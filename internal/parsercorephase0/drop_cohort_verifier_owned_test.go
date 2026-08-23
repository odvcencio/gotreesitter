package parsercorephase0

import (
	"crypto/sha256"
	"math"
	"strings"
	"testing"
)

func g18OwnedVerifierFixture(t *testing.T) (*Core, []Head, []DropCohortRef) {
	t.Helper()
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxDropCohortMembers: 4, MaxDropCohortBytes: 1 << 20})
	heads := make([]Head, 3)
	for index := range heads {
		var err error
		heads[index], err = compact.Seed(StateID(index+1), uint32(index))
		if err != nil {
			t.Fatal(err)
		}
	}
	record := []byte{2, 4, 5, 1, 0, 8, 13, 21}
	digest := sha256.Sum256(record)
	var caps [7]uint64
	for index := range caps {
		caps[index] = math.MaxUint64
	}
	raw, err := compact.DiagnosticDropCohortBeginForTest(3, uint32(len(record)), caps)
	if err != nil {
		t.Fatal(err)
	}
	action := [14]int64{3, 9, 3, int64(ActionReduce), 0, 2, 5, 1, 2, 0, 0, 0, 0, 1}
	for index, head := range heads {
		if err := compact.DiagnosticDropCohortWriteForTest(raw, head, uint16(index), action, digest, record); err != nil {
			t.Fatal(err)
		}
	}
	if err := compact.DiagnosticDropCohortFinalizeForTest(raw); err != nil {
		t.Fatal(err)
	}
	refs := make([]DropCohortRef, len(heads))
	for index := range refs {
		refs[index] = DropCohortRef{Owner: raw[0], Epoch: raw[1], Sequence: raw[2], Branch: uint16(index)}
	}
	return compact, heads, refs
}

func TestG18DropCohortOwnedAdaptersProbeAndPublishOnce(t *testing.T) {
	compact, heads, _ := g18OwnedVerifierFixture(t)
	drops := []int{1, 2}
	beforeLookups := compact.dropCohortOwnerCheckedLookups
	beforeElections := compact.dropCohortVerifierElections
	beforeProofs := compact.dropCohortVerifierProofs
	var stale SchedulerTransactionToken
	var refs []DropCohortRef
	if err := compact.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		stale = owner
		record := []byte{2, 4, 5, 1, 0, 8, 13, 21}
		digest := sha256.Sum256(record)
		var caps [7]uint64
		for index := range caps {
			caps[index] = math.MaxUint64
		}
		raw, err := compact.DiagnosticDropCohortBeginForTest(3, uint32(len(record)), caps)
		if err != nil {
			return err
		}
		action := [14]int64{3, 9, 3, int64(ActionReduce), 0, 2, 5, 1, 2, 0, 0, 0, 0, 1}
		for index, head := range heads {
			if err := compact.DiagnosticDropCohortWriteForTest(raw, head, uint16(index), action, digest, record); err != nil {
				return err
			}
		}
		if err := compact.DiagnosticDropCohortFinalizeForTest(raw); err != nil {
			return err
		}
		refs = make([]DropCohortRef, len(heads))
		for index := range refs {
			refs[index] = DropCohortRef{Owner: raw[0], Epoch: raw[1], Sequence: raw[2], Branch: uint16(index)}
		}
		reason, err := compact.ClassifyDropCohortRefsOwned(owner, heads, refs, drops)
		if err != nil || reason != "proved" {
			t.Fatalf("probe reason=%q err=%v", reason, err)
		}
		if compact.dropCohortVerifierElections != beforeElections || compact.dropCohortVerifierProofs != beforeProofs {
			t.Fatalf("probe published elections=%d proofs=%d", compact.dropCohortVerifierElections, compact.dropCohortVerifierProofs)
		}
		return compact.VerifyDropCohortRefsOwned(owner, heads, refs, drops)
	}); err != nil {
		t.Fatal(err)
	}
	if got := compact.dropCohortVerifierElections - beforeElections; got != 1 {
		t.Fatalf("published elections=%d, want 1", got)
	}
	if got := compact.dropCohortVerifierProofs - beforeProofs; got != 1 {
		t.Fatalf("published proofs=%d, want 1", got)
	}
	if compact.dropCohortOwnerCheckedLookups <= beforeLookups {
		t.Fatal("published verification did not read the certificate owner")
	}
	afterLookups := compact.dropCohortOwnerCheckedLookups
	if _, err := compact.ClassifyDropCohortRefsOwned(stale, heads, refs, drops); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale owner classification error=%v, want stale token", err)
	}
	if compact.dropCohortOwnerCheckedLookups != afterLookups {
		t.Fatal("stale owner changed lookup accounting")
	}
}

func TestG18DropCohortOwnedRefReadRejectsForeignOwnerBeforeSpillRead(t *testing.T) {
	compact, _, _ := g18OwnedVerifierFixture(t)
	beforeLookups := compact.dropCohortOwnerCheckedLookups
	var refs DropCohortRefSet
	owner, epoch := compact.DropCohortArenaIdentity()
	for index := 0; index < 3; index++ {
		if !compact.AddDropCohortRef(&refs, DropCohortRef{Owner: owner, Epoch: epoch, Sequence: uint64(index + 1), Branch: uint16(index)}) {
			t.Fatalf("add reference %d failed", index)
		}
	}
	if !refs.Spilled() {
		t.Fatal("fixture did not exercise the spill reference store")
	}
	foreign := newTinyCoreWithLimits(t, Limits{MaxDerivations: 2})
	var token SchedulerTransactionToken
	if err := foreign.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		token = owner
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := compact.DropCohortRefAtOwned(token, refs, 0); err == nil || !strings.Contains(err.Error(), "different core") {
		t.Fatalf("foreign owner error=%v, want different core", err)
	}
	if compact.dropCohortOwnerCheckedLookups != beforeLookups || compact.dropCohortVerifierElections != 0 {
		t.Fatalf("foreign owner changed verifier state lookups=%d/%d elections=%d", compact.dropCohortOwnerCheckedLookups, beforeLookups, compact.dropCohortVerifierElections)
	}
}

func TestG18DropCohortOwnedRefReadRejectsStaleOwnerBeforeSpillRead(t *testing.T) {
	compact, _, _ := g18OwnedVerifierFixture(t)
	beforeLookups := compact.dropCohortOwnerCheckedLookups
	owner, epoch := compact.DropCohortArenaIdentity()
	var refs DropCohortRefSet
	for index := 0; index < 3; index++ {
		if !compact.AddDropCohortRef(&refs, DropCohortRef{
			Owner: owner, Epoch: epoch, Sequence: uint64(index + 1), Branch: uint16(index),
		}) {
			t.Fatalf("add reference %d failed", index)
		}
	}
	if !refs.Spilled() {
		t.Fatal("fixture did not exercise the spill reference store")
	}
	var stale SchedulerTransactionToken
	if err := compact.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		stale = owner
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := compact.DropCohortRefAtOwned(stale, refs, 0); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale owner error=%v, want stale token", err)
	}
	if compact.dropCohortOwnerCheckedLookups != beforeLookups || compact.dropCohortVerifierElections != 0 {
		t.Fatalf("stale owner changed verifier state lookups=%d/%d elections=%d", compact.dropCohortOwnerCheckedLookups, beforeLookups, compact.dropCohortVerifierElections)
	}
}

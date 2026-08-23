package parsercorephase0

import (
	"testing"
)

type dropCohortFrontierFixture struct {
	core       *Core
	token      DropCohortFrontierToken
	heads      []Head
	refs       []DropCohortRefSet
	branch     []uint64
	frontier   DropCohortFrontierHandle
	frontierOK bool
}

func newDropCohortFrontierFixture(t *testing.T, participants, refsPerParticipant int) dropCohortFrontierFixture {
	t.Helper()
	if participants <= 0 || refsPerParticipant <= 0 {
		t.Fatalf("invalid frontier fixture shape %d/%d", participants, refsPerParticipant)
	}
	core := newTinyCoreWithLimits(t, Limits{
		MaxDropCohortMembers: uint16(max(participants, refsPerParticipant)),
		MaxDropCohortBytes:   1 << 20,
	})
	start := mustInternCheckpoint(t, core, []byte{1, 2, 3})
	end := mustInternCheckpoint(t, core, []byte{4, 5, 6})
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, end); err != nil {
		t.Fatal(err)
	}
	_, beforeDigest, beforeOK := core.CheckpointReceipt(start)
	_, afterDigest, afterOK := core.CheckpointReceipt(end)
	if !beforeOK || !afterOK {
		t.Fatal("frontier fixture checkpoint receipt is unavailable")
	}
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	heads := make([]Head, participants)
	for index := range heads {
		heads[index] = g18ProducerHead(t, core, seed, Symbol(30+index), uint32(index+1))
	}
	cohortHeads := make([]Head, refsPerParticipant)
	for index := range cohortHeads {
		cohortHeads[index] = g18ProducerHead(t, core, seed, Symbol(90+index), uint32(index+1))
	}
	var refs DropCohortRefSet
	err = core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		cohort, beginErr := core.BeginDropCohortOwned(owner, g18ProducerIdentity(), refsPerParticipant)
		if beginErr != nil {
			return beginErr
		}
		for branch, head := range cohortHeads {
			derivation, buildErr := core.BuildDropCohortDerivationOwned(owner, head, DropCohortSourceCheckpoint{
				StartByte: uint32(branch), EndByte: uint32(branch + 1),
			})
			if buildErr != nil {
				return buildErr
			}
			if writeErr := core.WriteDropCohortMemberOwned(owner, cohort, head, uint16(branch), derivation); writeErr != nil {
				return writeErr
			}
		}
		refs, err = core.FinalizeDropCohortOwned(owner, cohort)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if refs.Len() != refsPerParticipant {
		t.Fatalf("fixture refs=%d, want %d", refs.Len(), refsPerParticipant)
	}
	sets := make([]DropCohortRefSet, participants)
	for index := range sets {
		sets[index] = refs
	}
	branch := make([]uint64, participants)
	for index := range branch {
		branch[index] = uint64(index)
	}
	return dropCohortFrontierFixture{
		core: core, token: DropCohortFrontierToken{
			Symbol: 9, StartByte: 0, EndByte: 1,
			ScannerBefore: start, ScannerAfter: end,
			ScannerBeforeDigest: beforeDigest, ScannerAfterDigest: afterDigest,
		},
		heads: heads, refs: sets, branch: branch,
	}
}

func publishDropCohortFrontierFixture(t *testing.T, fixture *dropCohortFrontierFixture) {
	t.Helper()
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		fixture.frontier, fixture.frontierOK, err = fixture.core.PublishDropCohortFrontierOwned(
			owner, 11, fixture.token, fixture.heads, fixture.branch, fixture.refs,
		)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestG18DropCohortFrontierPublishesOrderedOwnedRecord(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK || fixture.frontier.Sequence == 0 {
		t.Fatalf("frontier=%+v complete=%t", fixture.frontier, fixture.frontierOK)
	}
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		state, expected, written, members, stateErr := fixture.core.DropCohortFrontierStateOwned(owner, fixture.frontier)
		if stateErr != nil || state != DropCohortFrontierComplete || expected != 2 || written != 2 || members != 2 {
			t.Fatalf("frontier state=%v expected=%d written=%d members=%d err=%v", state, expected, written, members, stateErr)
		}
		return fixture.core.ValidateDropCohortFrontierOwned(owner, fixture.frontier, fixture.heads)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.core.dropCohortFrontierParticipants) != 2 || fixture.core.dropCohortFrontierParticipants[0].head != fixture.heads[0] || fixture.core.dropCohortFrontierParticipants[1].head != fixture.heads[1] {
		t.Fatalf("participant order=%+v", fixture.core.dropCohortFrontierParticipants)
	}
	if fixture.core.dropCohortFrontierParticipants[0].branchOrder != 0 {
		t.Fatalf("zero branch order was rejected or changed: %+v", fixture.core.dropCohortFrontierParticipants[0])
	}
}

func TestG18DropCohortFrontierRejectsForeignAndStaleOwnerBeforeStoreReads(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 1, 3)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("spill frontier did not publish")
	}
	beforeRecords := len(fixture.core.dropCohortFrontiers)
	beforeParticipants := len(fixture.core.dropCohortFrontierParticipants)
	beforeMembers := len(fixture.core.dropCohortFrontierMembers)
	beforeChecks := fixture.core.dropCohortOwnerCheckedLookups
	other := newTinyCoreWithLimits(t, Limits{})
	err := other.ApplySchedulerAtomic(func(foreign SchedulerTransactionToken) error {
		state, expected, written, members, stateErr := fixture.core.DropCohortFrontierStateOwned(foreign, fixture.frontier)
		if state != 0 || expected != 0 || written != 0 || members != 0 || stateErr == nil {
			t.Fatalf("foreign token read state=%v expected=%d written=%d members=%d err=%v", state, expected, written, members, stateErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.core.dropCohortOwnerCheckedLookups != beforeChecks || len(fixture.core.dropCohortFrontiers) != beforeRecords || len(fixture.core.dropCohortFrontierParticipants) != beforeParticipants || len(fixture.core.dropCohortFrontierMembers) != beforeMembers {
		t.Fatalf("foreign token changed stores/checks: checks=%d/%d frontiers=%d/%d participants=%d/%d members=%d/%d", fixture.core.dropCohortOwnerCheckedLookups, beforeChecks, len(fixture.core.dropCohortFrontiers), beforeRecords, len(fixture.core.dropCohortFrontierParticipants), beforeParticipants, len(fixture.core.dropCohortFrontierMembers), beforeMembers)
	}
	old := fixture.frontier
	err = fixture.core.Reset()
	if err != nil {
		t.Fatal(err)
	}
	staleErr := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, _, _, stateErr := fixture.core.DropCohortFrontierStateOwned(owner, old)
		if stateErr == nil {
			t.Fatal("stale frontier handle was accepted after reset")
		}
		return nil
	})
	if staleErr == nil {
		t.Fatal("stale frontier transaction was not poisoned")
	}
}

func TestG18DropCohortFrontierPublicationRollsBackIncompleteRecords(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	fixture.heads[1] = fixture.heads[0]
	before := fixture.core.StorageBytes()
	publishDropCohortFrontierFixture(t, &fixture)
	if fixture.frontierOK || fixture.frontier != (DropCohortFrontierHandle{}) {
		t.Fatalf("incomplete publication returned frontier=%+v complete=%t", fixture.frontier, fixture.frontierOK)
	}
	if len(fixture.core.dropCohortFrontiers) != 0 || len(fixture.core.dropCohortFrontierParticipants) != 0 || len(fixture.core.dropCohortFrontierMembers) != 0 || fixture.core.dropCohortFrontierNextSequence != 0 {
		t.Fatalf("incomplete publication retained frontier stores: records=%d participants=%d members=%d next=%d", len(fixture.core.dropCohortFrontiers), len(fixture.core.dropCohortFrontierParticipants), len(fixture.core.dropCohortFrontierMembers), fixture.core.dropCohortFrontierNextSequence)
	}
	if fixture.core.StorageBytes() != before {
		t.Fatalf("incomplete publication changed storage bytes: got=%d want=%d", fixture.core.StorageBytes(), before)
	}
}

func TestG18DropCohortFrontierAcceptsBlendedAndRejectsOverflowedRefs(t *testing.T) {
	blended := newDropCohortFrontierFixture(t, 1, 1)
	blended.refs[0].Flags |= dropCohortRefFlagBlended
	publishDropCohortFrontierFixture(t, &blended)
	if !blended.frontierOK {
		t.Fatal("blended frontier was rejected")
	}
	overflowed := newDropCohortFrontierFixture(t, 1, 1)
	overflowed.refs[0].Flags |= dropCohortRefFlagOverflowed
	publishDropCohortFrontierFixture(t, &overflowed)
	if overflowed.frontierOK || len(overflowed.core.dropCohortFrontiers) != 0 {
		t.Fatalf("overflowed frontier published: complete=%t records=%d", overflowed.frontierOK, len(overflowed.core.dropCohortFrontiers))
	}
}

func TestG18DropCohortFrontierSealRejectsEveryActionFieldMutation(t *testing.T) {
	mutations := []struct {
		name string
		edit func(*DropCohortActionIdentity)
	}{
		{"boundary", func(identity *DropCohortActionIdentity) { identity.BoundaryState++ }},
		{"lookahead", func(identity *DropCohortActionIdentity) { identity.Lookahead++ }},
		{"ordinal", func(identity *DropCohortActionIdentity) { identity.ActionOrdinal++ }},
		{"type", func(identity *DropCohortActionIdentity) { identity.Action.Type = ActionShift }},
		{"state", func(identity *DropCohortActionIdentity) { identity.Action.State++ }},
		{"symbol", func(identity *DropCohortActionIdentity) { identity.Action.Symbol++ }},
		{"child_count", func(identity *DropCohortActionIdentity) { identity.Action.ChildCount++ }},
		{"dynamic_precedence", func(identity *DropCohortActionIdentity) { identity.Action.DynamicPrecedence++ }},
		{"production", func(identity *DropCohortActionIdentity) { identity.Action.ProductionID++ }},
		{"extra", func(identity *DropCohortActionIdentity) { identity.Action.Extra = !identity.Action.Extra }},
		{"extra_chain", func(identity *DropCohortActionIdentity) { identity.Action.ExtraChain = !identity.Action.ExtraChain }},
		{"repetition", func(identity *DropCohortActionIdentity) { identity.Action.Repetition = !identity.Action.Repetition }},
		{"no_lookahead", func(identity *DropCohortActionIdentity) { identity.NoLookahead = !identity.NoLookahead }},
		{"selection", func(identity *DropCohortActionIdentity) { identity.Selection++ }},
	}
	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			fixture := newDropCohortFrontierFixture(t, 1, 1)
			publishDropCohortFrontierFixture(t, &fixture)
			if !fixture.frontierOK {
				t.Fatal("frontier did not publish")
			}
			mutation.edit(&fixture.core.dropCohortActions[0])
			var validateErr error
			err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				validateErr = fixture.core.ValidateDropCohortFrontierOwned(owner, fixture.frontier, fixture.heads)
				return nil
			})
			if validateErr == nil || err == nil {
				t.Fatalf("action mutation %s validation=%v transaction=%v", mutation.name, validateErr, err)
			}
		})
	}
}

func TestG18DropCohortFrontierSealRejectsDerivationByteMutationWithStoredDigest(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 1, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK || len(fixture.core.dropCohortDerivationBytes) == 0 {
		t.Fatal("frontier fixture has no derivation bytes")
	}
	beforeDigest := fixture.core.dropCohortDerivations[0].digest
	fixture.core.dropCohortDerivationBytes[0] ^= 0xff
	if fixture.core.dropCohortDerivations[0].digest != beforeDigest {
		t.Fatal("test changed the stored derivation digest")
	}
	var validateErr error
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		validateErr = fixture.core.ValidateDropCohortFrontierOwned(owner, fixture.frontier, fixture.heads)
		return nil
	})
	if validateErr == nil || err == nil {
		t.Fatalf("derivation byte mutation validation=%v transaction=%v", validateErr, err)
	}
}

func TestG18DropCohortFrontierResetAndOwnedReadsStayBounded(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 1, 3)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK || !fixture.refs[0].Spilled() {
		t.Fatal("frontier fixture did not exercise spill storage")
	}
	before := fixture.core.StorageBytes()
	if before == 0 || before > fixture.core.limits.MaxDropCohortBytes {
		t.Fatalf("frontier storage=%d max=%d", before, fixture.core.limits.MaxDropCohortBytes)
	}
	if err := fixture.core.Reset(); err != nil {
		t.Fatal(err)
	}
	if len(fixture.core.dropCohortFrontiers) != 0 || len(fixture.core.dropCohortFrontierParticipants) != 0 || len(fixture.core.dropCohortFrontierMembers) != 0 {
		t.Fatal("reset retained logical frontier records")
	}
	if fixture.core.dropCohortFrontierNextSequence != 0 {
		t.Fatalf("reset frontier sequence=%d, want 0", fixture.core.dropCohortFrontierNextSequence)
	}
}

func TestG18DropCohortFrontierByteCapDeclinesBeforePublication(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 1, 1)
	fixture.core.limits.MaxDropCohortBytes = fixture.core.dropCohortStoreBytes()
	publishDropCohortFrontierFixture(t, &fixture)
	if fixture.frontierOK || len(fixture.core.dropCohortFrontiers) != 0 {
		t.Fatalf("frontier exceeded byte cap: complete=%t records=%d", fixture.frontierOK, len(fixture.core.dropCohortFrontiers))
	}
}

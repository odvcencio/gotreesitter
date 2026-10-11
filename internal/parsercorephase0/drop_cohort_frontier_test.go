package parsercorephase0

import (
	"errors"
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

func buildDropCohortFrontierTestCohort(
	t *testing.T,
	core *Core,
	identity DropCohortActionIdentity,
	state StateID,
	byteOffset uint32,
) DropCohortRefSet {
	t.Helper()
	seed, err := core.Seed(state, byteOffset)
	if err != nil {
		t.Fatal(err)
	}
	head := g18ProducerHead(t, core, seed, Symbol(120+state), byteOffset+1)
	var refs DropCohortRefSet
	if err := core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		cohort, beginErr := core.BeginDropCohortOwned(owner, identity, 1)
		if beginErr != nil {
			return beginErr
		}
		derivation, buildErr := core.BuildDropCohortDerivationOwned(owner, head, DropCohortSourceCheckpoint{
			StartByte: byteOffset, EndByte: byteOffset + 1,
		})
		if buildErr != nil {
			return buildErr
		}
		if writeErr := core.WriteDropCohortMemberOwned(owner, cohort, head, 0, derivation); writeErr != nil {
			return writeErr
		}
		refs, err = core.FinalizeDropCohortOwned(owner, cohort)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return refs
}

func newDropCohortFrontierTwoCohortFixture(t *testing.T, shared bool) dropCohortFrontierFixture {
	t.Helper()
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	first := fixture.refs[0]
	secondIdentity := g18ProducerIdentity()
	secondIdentity.ActionOrdinal++
	second := buildDropCohortFrontierTestCohort(t, fixture.core, secondIdentity, 3, 10)
	if shared {
		combined := first
		if !fixture.core.UnionDropCohortRefs(&combined, second) {
			t.Fatal("multi-cohort reference union did not change the set")
		}
		fixture.refs[0], fixture.refs[1] = combined, combined
	} else {
		fixture.refs[0], fixture.refs[1] = first, second
	}
	return fixture
}

func newDropCohortFrontierSameCohortDifferentDerivationFixture(t *testing.T) dropCohortFrontierFixture {
	t.Helper()
	fixture := newDropCohortFrontierFixture(t, 2, 2)
	branchZero, ok := fixture.core.DropCohortRefAt(fixture.refs[0], 0)
	if !ok {
		t.Fatal("source branch zero reference is unavailable")
	}
	branchOne, ok := fixture.core.DropCohortRefAt(fixture.refs[0], 1)
	if !ok {
		t.Fatal("source branch one reference is unavailable")
	}
	var survivorRefs, droppedRefs DropCohortRefSet
	if !fixture.core.AddDropCohortRef(&survivorRefs, branchZero) ||
		!fixture.core.AddDropCohortRef(&droppedRefs, branchOne) {
		t.Fatal("same-cohort branch reference subset construction failed")
	}
	fixture.refs[0], fixture.refs[1] = survivorRefs, droppedRefs
	return fixture
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

func TestG18DropCohortFrontierCumulativeBudgetDeclinesWithoutGrowth(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("first frontier did not publish")
	}
	live := fixture.core.dropCohortStoreBytes()
	budget, err := fixture.core.dropCohortRetainedOtherBytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range fixture.core.dropCohortRetainedStoreBytes() {
		budget, err = dropCohortAddChecked(budget, value)
		if err != nil {
			t.Fatal(err)
		}
	}
	budget, err = dropCohortAddChecked(budget, fixture.core.dropCohortReservedBytes)
	if err != nil {
		t.Fatal(err)
	}
	if budget < live {
		t.Fatalf("retained budget=%d is below live storage=%d", budget, live)
	}
	fixture.core.limits.MaxDropCohortBytes = budget

	beforeRecords := len(fixture.core.dropCohortFrontiers)
	beforeParticipants := len(fixture.core.dropCohortFrontierParticipants)
	beforeMembers := len(fixture.core.dropCohortFrontierMembers)
	beforeSequence := fixture.core.dropCohortFrontierNextSequence
	beforeStore := fixture.core.dropCohortStoreBytes()
	var second DropCohortFrontierHandle
	var complete bool
	err = fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var publishErr error
		second, complete, publishErr = fixture.core.PublishDropCohortFrontierOwned(
			owner, 12, fixture.token, fixture.heads, fixture.branch, fixture.refs,
		)
		return publishErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if complete || second != (DropCohortFrontierHandle{}) {
		t.Fatalf("second frontier published: handle=%+v complete=%t", second, complete)
	}
	if len(fixture.core.dropCohortFrontiers) != beforeRecords ||
		len(fixture.core.dropCohortFrontierParticipants) != beforeParticipants ||
		len(fixture.core.dropCohortFrontierMembers) != beforeMembers ||
		fixture.core.dropCohortFrontierNextSequence != beforeSequence ||
		fixture.core.dropCohortStoreBytes() != beforeStore {
		t.Fatalf("declined frontier changed logical store: records=%d/%d participants=%d/%d members=%d/%d sequence=%d/%d store=%d/%d", len(fixture.core.dropCohortFrontiers), beforeRecords, len(fixture.core.dropCohortFrontierParticipants), beforeParticipants, len(fixture.core.dropCohortFrontierMembers), beforeMembers, fixture.core.dropCohortFrontierNextSequence, beforeSequence, fixture.core.dropCohortStoreBytes(), beforeStore)
	}
	frontierBytes, err := dropCohortMulChecked(uint64(cap(fixture.core.dropCohortFrontiers)), coreDropCohortFrontierRecordBytes)
	if err != nil {
		t.Fatal(err)
	}
	frontierBytes, err = dropCohortAddChecked(frontierBytes, uint64(cap(fixture.core.dropCohortFrontierParticipants))*coreDropCohortFrontierParticipantBytes)
	if err != nil {
		t.Fatal(err)
	}
	frontierBytes, err = dropCohortAddChecked(frontierBytes, uint64(cap(fixture.core.dropCohortFrontierMembers))*coreDropCohortFrontierMemberBytes)
	if err != nil {
		t.Fatal(err)
	}
	if frontierBytes > fixture.core.limits.MaxDropCohortBytes {
		t.Fatalf("retained frontier capacity=%d exceeds cap=%d", frontierBytes, fixture.core.limits.MaxDropCohortBytes)
	}
}

func TestG18DropCohortFrontierStaleTokenPoisonsAndRollsBackOuterMutation(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	var stale SchedulerTransactionToken
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		stale = owner
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	beforeWrites := fixture.core.dropCohortProducerWrites
	beforeRecords := len(fixture.core.dropCohortFrontiers)
	beforeParticipants := len(fixture.core.dropCohortFrontierParticipants)
	beforeMembers := len(fixture.core.dropCohortFrontierMembers)
	beforeSequence := fixture.core.dropCohortFrontierNextSequence
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if mutationErr := fixture.core.RecordDropCohortProducerMutation(owner, DropCohortProducerLinearCanonicalization); mutationErr != nil {
			return mutationErr
		}
		_, _, _ = fixture.core.PublishDropCohortFrontierOwned(
			stale, 13, fixture.token, fixture.heads, fixture.branch, fixture.refs,
		)
		return nil
	})
	if err == nil {
		t.Fatal("outer scheduler transaction committed after ignored stale-token error")
	}
	if fixture.core.dropCohortProducerWrites != beforeWrites ||
		len(fixture.core.dropCohortFrontiers) != beforeRecords ||
		len(fixture.core.dropCohortFrontierParticipants) != beforeParticipants ||
		len(fixture.core.dropCohortFrontierMembers) != beforeMembers ||
		fixture.core.dropCohortFrontierNextSequence != beforeSequence {
		t.Fatalf("stale-token rollback retained state: writes=%v/%v records=%d/%d participants=%d/%d members=%d/%d sequence=%d/%d", fixture.core.dropCohortProducerWrites, beforeWrites, len(fixture.core.dropCohortFrontiers), beforeRecords, len(fixture.core.dropCohortFrontierParticipants), beforeParticipants, len(fixture.core.dropCohortFrontierMembers), beforeMembers, fixture.core.dropCohortFrontierNextSequence, beforeSequence)
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedSucceedsAndRejectsReplay(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	}); err != nil {
		t.Fatal(err)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierConsumed {
		t.Fatalf("frontier state=%v, want consumed", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 1 {
		t.Fatalf("frontier journal length=%d, want 1", len(fixture.core.dropCohortFrontierJournal))
	}
	mutation := fixture.core.dropCohortFrontierJournal[0]
	if mutation.index != uint32(fixture.frontier.Sequence-1) || mutation.before != DropCohortFrontierComplete {
		t.Fatalf("frontier mutation=%+v, want prior complete state for sequence %d", mutation, fixture.frontier.Sequence)
	}
	replayErr := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	})
	if replayErr == nil {
		t.Fatal("replayed frontier consumption succeeded")
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierConsumed {
		t.Fatalf("frontier state after replay=%v, want consumed", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 1 {
		t.Fatalf("frontier journal length after replay=%d, want 1", len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRollsBackOuterError(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	beforeJournal := len(fixture.core.dropCohortFrontierJournal)
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if consumeErr := fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		); consumeErr != nil {
			return consumeErr
		}
		return errors.New("forced outer rollback")
	})
	if err == nil {
		t.Fatal("forced outer error did not roll back")
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after rollback=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != beforeJournal {
		t.Fatalf("frontier journal length after rollback=%d, want %d", len(fixture.core.dropCohortFrontierJournal), beforeJournal)
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedJournalByteCapDeclines(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	retained, err := fixture.core.dropCohortRetainedOtherBytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range fixture.core.dropCohortRetainedStoreBytes() {
		retained, err = dropCohortAddChecked(retained, value)
		if err != nil {
			t.Fatal(err)
		}
	}
	retained, err = dropCohortAddChecked(retained, fixture.core.dropCohortReservedBytes)
	if err != nil {
		t.Fatal(err)
	}
	fixture.core.limits.MaxDropCohortBytes = retained
	beforeJournalLen := len(fixture.core.dropCohortFrontierJournal)
	beforeJournalCap := cap(fixture.core.dropCohortFrontierJournal)
	err = fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	})
	if err == nil {
		t.Fatal("journal byte-cap decline was accepted")
	}
	if len(fixture.core.dropCohortFrontierJournal) != beforeJournalLen || cap(fixture.core.dropCohortFrontierJournal) != beforeJournalCap {
		t.Fatalf("frontier journal changed after byte-cap decline: len=%d/%d cap=%d/%d", len(fixture.core.dropCohortFrontierJournal), beforeJournalLen, cap(fixture.core.dropCohortFrontierJournal), beforeJournalCap)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after byte-cap decline=%v, want complete", got)
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedResetClearsConsumedState(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.core.Reset(); err != nil {
		t.Fatal(err)
	}
	if len(fixture.core.dropCohortFrontiers) != 0 || len(fixture.core.dropCohortFrontierParticipants) != 0 ||
		len(fixture.core.dropCohortFrontierMembers) != 0 || len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("reset retained consumed frontier stores: frontiers=%d participants=%d members=%d journal=%d", len(fixture.core.dropCohortFrontiers), len(fixture.core.dropCohortFrontierParticipants), len(fixture.core.dropCohortFrontierMembers), len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bResetReleasingRetentionDropsConsumedFrontierJournal(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	}); err != nil {
		t.Fatal(err)
	}
	if cap(fixture.core.dropCohortFrontierJournal) == 0 {
		t.Fatal("consumed frontier did not retain journal capacity")
	}
	if err := fixture.core.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 || cap(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("retention reset kept frontier journal len=%d cap=%d", len(fixture.core.dropCohortFrontierJournal), cap(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsMalformedStoredOffsets(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Core)
	}{
		{name: "participant_start", mutate: func(core *Core) {
			core.dropCohortFrontiers[0].participantStart = ^uint32(0)
		}},
		{name: "member_start", mutate: func(core *Core) {
			core.dropCohortFrontiers[0].memberStart = ^uint32(0)
		}},
		{name: "participant_member_start", mutate: func(core *Core) {
			core.dropCohortFrontierParticipants[0].memberStart = ^uint32(0)
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newDropCohortFrontierFixture(t, 2, 2)
			publishDropCohortFrontierFixture(t, &fixture)
			if !fixture.frontierOK {
				t.Fatal("frontier did not publish")
			}
			testCase.mutate(fixture.core)
			var consumeErr error
			outerErr := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				consumeErr = fixture.core.ConsumeDropCohortFrontierSequenceOwned(
					owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{0},
				)
				return consumeErr
			})
			if consumeErr == nil || outerErr == nil {
				t.Fatalf("malformed offset accepted: consume=%v outer=%v", consumeErr, outerErr)
			}
			if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
				t.Fatalf("frontier state after malformed offset=%v, want complete", got)
			}
			if len(fixture.core.dropCohortFrontierJournal) != 0 {
				t.Fatalf("frontier journal after malformed offset=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
			}
		})
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsStoredCountCaps(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Core)
	}{
		{name: "expected_participants", mutate: func(core *Core) {
			core.dropCohortFrontiers[0].expectedParticipants = dropCohortFrontierParticipantHardCap + 1
		}},
		{name: "expected_members", mutate: func(core *Core) {
			core.dropCohortFrontiers[0].expectedMembers = dropCohortFrontierMemberHardCap + 1
		}},
		{name: "stored_per_head_references", mutate: func(core *Core) {
			core.dropCohortFrontierParticipants[0].memberCount = dropCohortRefHardCap + 1
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newDropCohortFrontierFixture(t, 2, 2)
			publishDropCohortFrontierFixture(t, &fixture)
			if !fixture.frontierOK {
				t.Fatal("frontier did not publish")
			}
			testCase.mutate(fixture.core)
			var consumeErr error
			outerErr := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				consumeErr = fixture.core.ConsumeDropCohortFrontierSequenceOwned(
					owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{0},
				)
				return consumeErr
			})
			if consumeErr == nil || outerErr == nil {
				t.Fatalf("stored count cap accepted: consume=%v outer=%v", consumeErr, outerErr)
			}
			if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
				t.Fatalf("frontier state after stored count cap=%v, want complete", got)
			}
			if len(fixture.core.dropCohortFrontierJournal) != 0 {
				t.Fatalf("frontier journal after stored count cap=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
			}
		})
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsMismatchedAction(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	fixture.core.dropCohortFrontierMembers[1].action.ActionOrdinal++
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		seal, ok := fixture.core.dropCohortFrontierSealOwned(owner, 0)
		if !ok {
			return errors.New("frontier seal could not be rebuilt")
		}
		fixture.core.dropCohortFrontiers[0].seal = seal
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	})
	if err == nil {
		t.Fatal("mismatched action was accepted")
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after action mismatch=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after action mismatch=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsMismatchedDerivation(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	fixture.core.dropCohortFrontierMembers[1].derivationLength++
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		seal, ok := fixture.core.dropCohortFrontierSealOwned(owner, 0)
		if !ok {
			return errors.New("frontier seal could not be rebuilt")
		}
		fixture.core.dropCohortFrontiers[0].seal = seal
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	})
	if err == nil {
		t.Fatal("mismatched derivation was accepted")
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after derivation mismatch=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after derivation mismatch=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsResealedBlendedRefs(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	fixture.refs[0].Flags |= dropCohortRefFlagBlended
	fixture.core.dropCohortFrontierParticipants[0].referenceFlags |= dropCohortRefFlagBlended
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		seal, ok := fixture.core.dropCohortFrontierSealOwned(owner, 0)
		if !ok {
			return errors.New("frontier seal could not be rebuilt")
		}
		fixture.core.dropCohortFrontiers[0].seal = seal
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	})
	if err == nil {
		t.Fatal("resealed blended references were accepted")
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after blended rejection=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after blended rejection=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsValidBranchDerivationMismatch(t *testing.T) {
	fixture := newDropCohortFrontierSameCohortDifferentDerivationFixture(t)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	})
	if err == nil {
		t.Fatal("valid branch derivation mismatch was accepted")
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after valid branch mismatch=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after valid branch mismatch=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedSelectsMatchingLaterBranch(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 2)
	branchZero, ok := fixture.core.DropCohortRefAt(fixture.refs[0], 0)
	if !ok {
		t.Fatal("source branch zero reference is unavailable")
	}
	branchOne, ok := fixture.core.DropCohortRefAt(fixture.refs[0], 1)
	if !ok {
		t.Fatal("source branch one reference is unavailable")
	}
	var survivorRefs, droppedRefs DropCohortRefSet
	if !fixture.core.AddDropCohortRef(&survivorRefs, branchZero) ||
		!fixture.core.AddDropCohortRef(&survivorRefs, branchOne) ||
		!fixture.core.AddDropCohortRef(&droppedRefs, branchOne) {
		t.Fatal("same-cohort branch reference subset construction failed")
	}
	fixture.refs[0], fixture.refs[1] = survivorRefs, droppedRefs
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	}); err != nil {
		t.Fatal(err)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierConsumed {
		t.Fatalf("frontier state after same-cohort branch fallback=%v, want consumed", got)
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsNoCommonCohort(t *testing.T) {
	fixture := newDropCohortFrontierTwoCohortFixture(t, false)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		consumeErr := fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
		if consumeErr == nil {
			t.Fatal("no-common cohort was accepted")
		}
		if errors.Is(consumeErr, ErrDropCohortFrontierNoCommonProof) {
			t.Fatalf("different cohort returned typed decline: %v", consumeErr)
		}
		if fixture.core.schedulerFrame.poisoned == nil {
			t.Fatal("different-cohort error did not poison the scheduler transaction")
		}
		return consumeErr
	})
	if err == nil {
		t.Fatal("different-cohort frontier did not poison the transaction")
	}
	if errors.Is(err, ErrDropCohortFrontierNoCommonProof) {
		t.Fatalf("different cohort returned typed decline: %v", err)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after no-common decline=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after no-common decline=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bNoCommonProofLeavesSchedulerOwnerUsable(t *testing.T) {
	fixture := newDropCohortFrontierSameCohortDifferentDerivationFixture(t)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	var consumeErr error
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		consumeErr = fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
		if !errors.Is(consumeErr, ErrDropCohortFrontierNoCommonProof) {
			return errors.New("no-common frontier did not return its typed outcome")
		}
		state, _, _, _, stateErr := fixture.core.DropCohortFrontierStateOwned(owner, fixture.frontier)
		if stateErr != nil {
			return stateErr
		}
		if state != DropCohortFrontierComplete {
			return errors.New("no-common frontier changed state before the legacy proof")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(consumeErr, ErrDropCohortFrontierNoCommonProof) {
		t.Fatalf("consume result=%v, want typed no-common proof", consumeErr)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after typed decline=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedSelectsMultiRefCandidate(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 2)
	branchZero, ok := fixture.core.DropCohortRefAt(fixture.refs[0], 0)
	if !ok {
		t.Fatal("first source branch zero reference is unavailable")
	}
	branchOne, ok := fixture.core.DropCohortRefAt(fixture.refs[0], 1)
	if !ok {
		t.Fatal("first source branch one reference is unavailable")
	}
	second := buildDropCohortFrontierTestCohort(t, fixture.core, func() DropCohortActionIdentity {
		identity := g18ProducerIdentity()
		identity.ActionOrdinal++
		return identity
	}(), 3, 10)
	secondRef, ok := fixture.core.DropCohortRefAt(second, 0)
	if !ok {
		t.Fatal("second source reference is unavailable")
	}
	var survivorRefs, droppedRefs DropCohortRefSet
	for _, ref := range []DropCohortRef{branchZero, secondRef} {
		if !fixture.core.AddDropCohortRef(&survivorRefs, ref) {
			t.Fatal("survivor multi-ref construction failed")
		}
	}
	for _, ref := range []DropCohortRef{branchOne, secondRef} {
		if !fixture.core.AddDropCohortRef(&droppedRefs, ref) {
			t.Fatal("dropped multi-ref construction failed")
		}
	}
	fixture.refs[0], fixture.refs[1] = survivorRefs, droppedRefs
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	if fixture.refs[0].Len() != 2 {
		t.Fatalf("multi-ref fixture count=%d, want 2", fixture.refs[0].Len())
	}
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		)
	}); err != nil {
		t.Fatal(err)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierConsumed {
		t.Fatalf("frontier state after multi-ref proof=%v, want consumed", got)
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedJournalRollbackRestoresCheckpointHeader(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 1)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	if err := fixture.core.dropCohortFrontierReserveJournalCapacity(1); err != nil {
		t.Fatal(err)
	}
	beforeJournalLen := len(fixture.core.dropCohortFrontierJournal)
	beforeJournalCap := cap(fixture.core.dropCohortFrontierJournal)
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if consumeErr := fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{1},
		); consumeErr != nil {
			return consumeErr
		}
		return errors.New("forced outer rollback")
	})
	if err == nil {
		t.Fatal("forced outer error did not roll back")
	}
	if len(fixture.core.dropCohortFrontierJournal) != beforeJournalLen ||
		cap(fixture.core.dropCohortFrontierJournal) != beforeJournalCap {
		t.Fatalf("frontier journal checkpoint changed: len=%d/%d cap=%d/%d", len(fixture.core.dropCohortFrontierJournal), beforeJournalLen, cap(fixture.core.dropCohortFrontierJournal), beforeJournalCap)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after journal rollback=%v, want complete", got)
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsCurrentInputMutations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Core, *uint64, *DropCohortFrontierToken, []Head, []DropCohortRefSet, *[]int)
	}{
		{name: "wrong_election", mutate: func(_ *Core, election *uint64, _ *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, _ *[]int) {
			*election = 12
		}},
		{name: "token_symbol", mutate: func(_ *Core, _ *uint64, token *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, _ *[]int) {
			token.Symbol++
		}},
		{name: "scanner_digest", mutate: func(_ *Core, _ *uint64, token *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, _ *[]int) {
			token.ScannerBeforeDigest[0] ^= 0xff
		}},
		{name: "swapped_heads", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, heads []Head, _ []DropCohortRefSet, _ *[]int) {
			heads[0], heads[1] = heads[1], heads[0]
		}},
		{name: "reference_flags", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, refs []DropCohortRefSet, _ *[]int) {
			refs[0].Flags ^= dropCohortRefFlagBlended
		}},
		{name: "reference_count", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, refs []DropCohortRefSet, _ *[]int) {
			refs[0].Count--
		}},
		{name: "ordered_reference", mutate: func(core *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, refs []DropCohortRefSet, _ *[]int) {
			start := refs[0].Spill - 1
			core.dropCohortRefSpill[start], core.dropCohortRefSpill[start+1] = core.dropCohortRefSpill[start+1], core.dropCohortRefSpill[start]
		}},
		{name: "empty_drops", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, drops *[]int) {
			*drops = []int{}
		}},
		{name: "duplicate_drops", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, drops *[]int) {
			*drops = []int{0, 0}
		}},
		{name: "descending_drops", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, drops *[]int) {
			*drops = []int{1, 0}
		}},
		{name: "negative_drop", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, drops *[]int) {
			*drops = []int{-1}
		}},
		{name: "out_of_range_drop", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, drops *[]int) {
			*drops = []int{2}
		}},
		{name: "all_header_drops", mutate: func(_ *Core, _ *uint64, _ *DropCohortFrontierToken, _ []Head, _ []DropCohortRefSet, drops *[]int) {
			*drops = []int{0, 1}
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newDropCohortFrontierFixture(t, 2, 2)
			publishDropCohortFrontierFixture(t, &fixture)
			if !fixture.frontierOK {
				t.Fatal("frontier did not publish")
			}
			election := uint64(11)
			token := fixture.token
			heads := append([]Head(nil), fixture.heads...)
			refs := append([]DropCohortRefSet(nil), fixture.refs...)
			drops := []int{1}
			testCase.mutate(fixture.core, &election, &token, heads, refs, &drops)
			var ownedErr error
			outerErr := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				ownedErr = fixture.core.ConsumeDropCohortFrontierSequenceOwned(
					owner, fixture.frontier.Sequence, election, token, heads, refs, drops,
				)
				return ownedErr
			})
			if ownedErr == nil || outerErr == nil {
				t.Fatalf("mutated input accepted: owned=%v outer=%v", ownedErr, outerErr)
			}
			if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
				t.Fatalf("frontier state after rollback=%v, want complete", got)
			}
			if len(fixture.core.dropCohortFrontierJournal) != 0 {
				t.Fatalf("frontier journal length after rollback=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
			}
		})
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsStoredAuthenticationMutations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Core)
	}{
		{name: "record_seal", mutate: func(core *Core) {
			core.dropCohortFrontiers[0].seal[0] ^= 0xff
		}},
		{name: "participant_head", mutate: func(core *Core) {
			core.dropCohortFrontierParticipants[0].head.Node++
		}},
		{name: "participant_branch_order", mutate: func(core *Core) {
			core.dropCohortFrontierParticipants[0].branchOrder++
		}},
		{name: "participant_reference_flags", mutate: func(core *Core) {
			core.dropCohortFrontierParticipants[0].referenceFlags ^= dropCohortRefFlagBlended
		}},
		{name: "participant_member_count", mutate: func(core *Core) {
			core.dropCohortFrontierParticipants[0].memberCount++
		}},
		{name: "member_participant", mutate: func(core *Core) {
			core.dropCohortFrontierMembers[0].participant++
		}},
		{name: "member_ref_branch", mutate: func(core *Core) {
			core.dropCohortFrontierMembers[0].ref.Branch++
		}},
		{name: "member_participant_head", mutate: func(core *Core) {
			core.dropCohortFrontierMembers[0].participantHead.Node++
		}},
		{name: "member_branch_order", mutate: func(core *Core) {
			core.dropCohortFrontierMembers[0].branchOrder++
		}},
		{name: "member_derivation_digest", mutate: func(core *Core) {
			core.dropCohortFrontierMembers[0].derivationDigest[0] ^= 0xff
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newDropCohortFrontierFixture(t, 2, 2)
			publishDropCohortFrontierFixture(t, &fixture)
			if !fixture.frontierOK {
				t.Fatal("frontier did not publish")
			}
			testCase.mutate(fixture.core)
			var ownedErr error
			outerErr := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				ownedErr = fixture.core.ConsumeDropCohortFrontierSequenceOwned(
					owner, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{0},
				)
				return ownedErr
			})
			if ownedErr == nil || outerErr == nil {
				t.Fatalf("stored mutation accepted: owned=%v outer=%v", ownedErr, outerErr)
			}
			if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
				t.Fatalf("frontier state after rollback=%v, want complete", got)
			}
			if len(fixture.core.dropCohortFrontierJournal) != 0 {
				t.Fatalf("frontier journal length after rollback=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
			}
		})
	}
}

func TestG18D6bConsumeDropCohortFrontierSequenceOwnedRejectsStaleAndForeignOwners(t *testing.T) {
	fixture := newDropCohortFrontierFixture(t, 2, 2)
	publishDropCohortFrontierFixture(t, &fixture)
	if !fixture.frontierOK {
		t.Fatal("frontier did not publish")
	}
	var stale SchedulerTransactionToken
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		stale = owner
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	beforeChecks := fixture.core.dropCohortOwnerCheckedLookups
	var staleOwnedErr error
	staleOuterErr := fixture.core.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		staleOwnedErr = fixture.core.ConsumeDropCohortFrontierSequenceOwned(
			stale, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{0},
		)
		return nil
	})
	if staleOwnedErr == nil || staleOuterErr == nil {
		t.Fatalf("stale owner accepted: owned=%v outer=%v", staleOwnedErr, staleOuterErr)
	}
	if fixture.core.dropCohortOwnerCheckedLookups != beforeChecks {
		t.Fatalf("stale owner changed frontier lookup count: got=%d want=%d", fixture.core.dropCohortOwnerCheckedLookups, beforeChecks)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after stale owner=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after stale owner=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}

	foreignCore := newTinyCoreWithLimits(t, Limits{})
	beforeChecks = fixture.core.dropCohortOwnerCheckedLookups
	var foreignOwnedErr, fixtureOuterErr error
	foreignOuterErr := foreignCore.ApplySchedulerAtomic(func(foreign SchedulerTransactionToken) error {
		fixtureOuterErr = fixture.core.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
			foreignOwnedErr = fixture.core.ConsumeDropCohortFrontierSequenceOwned(
				foreign, fixture.frontier.Sequence, 11, fixture.token, fixture.heads, fixture.refs, []int{0},
			)
			return nil
		})
		return nil
	})
	if foreignOuterErr != nil {
		t.Fatalf("foreign core transaction failed: %v", foreignOuterErr)
	}
	if foreignOwnedErr == nil || fixtureOuterErr == nil {
		t.Fatalf("foreign owner accepted: owned=%v outer=%v", foreignOwnedErr, fixtureOuterErr)
	}
	if fixture.core.dropCohortOwnerCheckedLookups != beforeChecks {
		t.Fatalf("foreign owner changed frontier lookup count: got=%d want=%d", fixture.core.dropCohortOwnerCheckedLookups, beforeChecks)
	}
	if got := fixture.core.dropCohortFrontiers[0].state; got != DropCohortFrontierComplete {
		t.Fatalf("frontier state after foreign owner=%v, want complete", got)
	}
	if len(fixture.core.dropCohortFrontierJournal) != 0 {
		t.Fatalf("frontier journal after foreign owner=%d, want 0", len(fixture.core.dropCohortFrontierJournal))
	}
}

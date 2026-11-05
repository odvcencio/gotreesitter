package parsercorephase0

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func g18ProducerHead(t *testing.T, compact *Core, seed Head, symbol Symbol, endByte uint32) Head {
	t.Helper()
	payload, err := compact.appendSubtree(subtreeRecord{
		symbol: symbol, startByte: endByte - 1, endByte: endByte, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.condense(compact.boundaryKey(2, endByte), linkInput{
		prev: seed.Node, payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func g18ProducerIdentity() DropCohortActionIdentity {
	return DropCohortActionIdentity{
		BoundaryState: 2,
		Lookahead:     7,
		ActionOrdinal: 3,
		Action: Action{
			Type: ActionReduce, State: 19, Symbol: 7, ChildCount: 2,
			DynamicPrecedence: -2, ProductionID: 41, Extra: true,
			ExtraChain: true, Repetition: true,
		},
		NoLookahead: true,
		Selection:   DropCohortSelectionConflictPolicy,
	}
}

func g18ProducerCheckpoint() DropCohortSourceCheckpoint {
	return DropCohortSourceCheckpoint{
		StartByte: 11, EndByte: 17,
		StartRow: 3, StartColumn: 5, EndRow: 4, EndColumn: 2,
		ScannerStart: 9, ScannerEnd: 10,
	}
}

func TestG18DropCohortActionIdentityAcceptsSignedPrecedenceAndChecksWidths(t *testing.T) {
	values := [14]int64{2, 7, 3, int64(ActionReduce), 19, 7, 41, 2, -2, 1, 1, 1, 1, int64(DropCohortSelectionConflictPolicy)}
	identity, err := dropCohortProtocolAction(values)
	if err != nil || identity.Action.DynamicPrecedence != -2 || !identity.NoLookahead {
		t.Fatalf("signed action identity rejected: identity=%+v err=%v", identity, err)
	}
	values[5] = math.MaxUint16 + 1
	if _, err := dropCohortProtocolAction(values); err == nil {
		t.Fatal("out-of-width symbol accepted")
	}
}

func TestG18DropCohortNestedBeginOvercommitRollsBackReservation(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortMembers: 4, MaxDropCohortBytes: 1 << 20})
	before := compact.StorageBytes()
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if _, err := compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), 2); err != nil {
			return err
		}
		compact.limits.MaxDropCohortBytes = compact.dropCohortStoreBytes() + 1
		_, err := compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), 2)
		return err
	})
	if err == nil || len(compact.dropCohortRecords) != 0 || len(compact.dropCohortReservations) != 0 || compact.dropCohortReservedBytes != 0 || compact.StorageBytes() != before {
		t.Fatalf("nested overcommit err=%v records=%d reservations=%d reserved=%d storage=%d/%d", err, len(compact.dropCohortRecords), len(compact.dropCohortReservations), compact.dropCohortReservedBytes, compact.StorageBytes(), before)
	}
}

func TestG18DropCohortResidualReservationParticipatesInNestedPreflight(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortMembers: 4, MaxDropCohortBytes: 1 << 20})
	reserved, err := compact.dropCohortReserveDemandWithExtra([7]uint64{}, 0, 128)
	if err != nil {
		t.Fatal(err)
	}
	if reserved != 128 || compact.dropCohortReservedBytes != 128 {
		t.Fatalf("reservation bytes=%d durable=%d, want 128", reserved, compact.dropCohortReservedBytes)
	}
	compact.limits.MaxDropCohortBytes = 127
	if err := compact.dropCohortPreflightDemand([7]uint64{}, 0, 0); err == nil {
		t.Fatal("nested preflight ignored residual durable reservation bytes")
	}
	compact.dropCohortReleaseDemand([7]uint64{}, reserved)
	if compact.dropCohortReservedBytes != 0 {
		t.Fatalf("reservation rollback left %d bytes", compact.dropCohortReservedBytes)
	}
}

func TestG18DropCohortRetainedOtherUsesCapacity(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortBytes: 1 << 20})
	compact.dropCohortJournal = make([]dropCohortMutation, 0, 3)
	compact.dropCohortReservations = make([]dropCohortReservation, 0, 2)
	got, err := compact.dropCohortRetainedOtherBytes()
	if err != nil {
		t.Fatal(err)
	}
	want, err := dropCohortAddChecked(3*coreDropCohortMutationBytes, 2*coreDropCohortReservationBytes)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("retained other bytes=%d, want %d", got, want)
	}
}

func TestG18DropCohortCheckedCapacityArithmeticRejectsOverflow(t *testing.T) {
	if _, err := dropCohortAddChecked(math.MaxUint64, 1); err == nil {
		t.Fatal("checked addition accepted overflow")
	}
	if _, err := dropCohortMulChecked(math.MaxUint64, 2); err == nil {
		t.Fatal("checked multiplication accepted overflow")
	}
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortBytes: math.MaxUint64})
	compact.dropCohortActions = make([]dropCohortActionIdentity, 1)
	compact.dropCohortReservedBytes = math.MaxUint64
	if _, err := compact.dropCohortGrowPermanentStore(1, 1, 1); err == nil {
		t.Fatal("permanent store growth accepted base-byte overflow")
	}
	compact = newTinyCoreWithLimits(t, Limits{MaxDropCohortBytes: math.MaxUint64})
	if _, err := compact.dropCohortGrowPermanentStore(math.MaxUint64, 1, 1); err == nil {
		t.Fatal("permanent store growth accepted bounded-capacity addition overflow")
	}
}

func TestG18DropCohortHugeDerivationBytesRejectsBeforeAllocation(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortMembers: 2, MaxDropCohortBytes: 1 << 20})
	caps := [7]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
	before := compact.dropCohortStoreBytes()
	if _, err := compact.DiagnosticDropCohortBeginForTest(2, math.MaxUint32, caps); err == nil {
		t.Fatal("huge derivation byte demand was accepted")
	}
	if compact.dropCohortStoreBytes() != before || len(compact.dropCohortReservations) != 0 || compact.dropCohortReservedBytes != 0 {
		t.Fatalf("huge derivation demand changed state: storage=%d/%d reservations=%d reserved=%d", compact.dropCohortStoreBytes(), before, len(compact.dropCohortReservations), compact.dropCohortReservedBytes)
	}
}

func TestG18DropCohortJournalSlotsDoNotGrowAfterBegin(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortMembers: 2, MaxDropCohortBytes: 1 << 20})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head := g18ProducerHead(t, compact, seed, 21, 1)
	var cohort DropCohortHandle
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		cohort, err = compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), 1)
		if err != nil {
			return err
		}
		journalCap := cap(compact.dropCohortJournalStore)
		derivation, buildErr := compact.BuildDropCohortDerivationOwned(owner, head, g18ProducerCheckpoint())
		if buildErr != nil {
			return buildErr
		}
		if writeErr := compact.WriteDropCohortMemberOwned(owner, cohort, head, 0, derivation); writeErr != nil {
			return writeErr
		}
		if _, finalErr := compact.FinalizeDropCohortOwned(owner, cohort); finalErr != nil {
			return finalErr
		}
		if cap(compact.dropCohortJournalStore) != journalCap {
			return fmt.Errorf("journal capacity grew from %d to %d", journalCap, cap(compact.dropCohortJournalStore))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestG18DropCohortConflictPreservesPriorMemberBytes(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortMembers: 2, MaxDropCohortBytes: 1 << 20})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head := g18ProducerHead(t, compact, seed, 22, 1)
	caps := [7]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
	values := [14]int64{3, 9, 3, int64(ActionReduce), 0, 2, 5, 1, -2, 0, 0, 0, 0, 1}
	digest := sha256.Sum256([]byte("prior"))
	record := []byte("prior")
	cohort, err := compact.DiagnosticDropCohortBeginForTest(2, uint32(len(record)), caps)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.DiagnosticDropCohortWriteForTest(cohort, head, 0, values, digest, record); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), compact.dropCohortDerivationBytes...)
	values[2]++
	conflictErr := compact.DiagnosticDropCohortWriteForTest(cohort, head, 0, values, digest, []byte("other"))
	if conflictErr == nil {
		t.Fatal("conflicting branch was accepted")
	}
	if !bytes.Equal(before, compact.dropCohortDerivationBytes) {
		t.Fatal("conflict changed prior derivation bytes")
	}
	index, _ := compact.validateDropCohortIdentity(DropCohortHandle{Owner: cohort[0], Epoch: cohort[1], Sequence: cohort[2]})
	if compact.dropCohortRecords[index].state != DropCohortBlended {
		t.Fatalf("conflict state=%v, want blended", compact.dropCohortRecords[index].state)
	}
}

func TestG18DropCohortPhysicalFootprintSeparatesLogicalSnapshot(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortMembers: 2, MaxDropCohortBytes: 1 << 20})
	caps := [7]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
	if _, err := compact.DiagnosticDropCohortBeginForTest(1, 8, caps); err != nil {
		t.Fatal(err)
	}
	var logical uint64
	for _, value := range dropCohortStoreVector(compact, false) {
		logical += value
	}
	if compact.StorageBytes() <= logical || compact.FootprintBytes() < compact.StorageBytes() {
		t.Fatalf("physical accounting storage=%d logical=%d footprint=%d", compact.StorageBytes(), logical, compact.FootprintBytes())
	}
}

func TestG18DropCohortProducerLifecycleAndStableOrdering(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxDropCohortMembers: 4})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	heads := []Head{
		g18ProducerHead(t, compact, seed, 21, 1),
		g18ProducerHead(t, compact, seed, 22, 2),
		g18ProducerHead(t, compact, seed, 23, 3),
	}
	checkpoint := g18ProducerCheckpoint()
	var cohort DropCohortHandle
	var refs DropCohortRefSet
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		cohort, err = compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), len(heads))
		if err != nil {
			return err
		}
		for _, branch := range []int{2, 0, 1} {
			derivation, buildErr := compact.BuildDropCohortDerivationOwned(owner, heads[branch], checkpoint)
			if buildErr != nil {
				return buildErr
			}
			if writeErr := compact.WriteDropCohortMemberOwned(owner, cohort, heads[branch], uint16(branch), derivation); writeErr != nil {
				return writeErr
			}
		}
		refs, err = compact.FinalizeDropCohortOwned(owner, cohort)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	state, expected, written, err := compact.DropCohortState(cohort)
	if err != nil || state != DropCohortComplete || expected != 3 || written != 3 {
		t.Fatalf("cohort state=%v expected=%d written=%d err=%v", state, expected, written, err)
	}
	if got := refs.Len(); got != 3 {
		t.Fatalf("final refs=%d, want 3", got)
	}
	for index := 0; index < refs.Len(); index++ {
		ref, ok := compact.DropCohortRefAt(refs, index)
		if !ok || ref.Branch != uint16(index) {
			t.Fatalf("ordered ref %d=%+v ok=%t, want branch %d", index, ref, ok, index)
		}
	}
	if compact.dropCohortActions[0] != g18ProducerIdentity() {
		t.Fatalf("stored action identity=%+v, want %+v", compact.dropCohortActions[0], g18ProducerIdentity())
	}
	counters := compact.DropCohortProducerCounts()
	if counters.ReductionEstablishment != 0 || counters.LinearCanonicalizer != 0 || counters.MappedCanonicalizer != 0 ||
		counters.SiblingAdoption != 0 || counters.ConflictReconciliation != 0 || counters.DeadHistoryImport != 0 {
		t.Fatalf("producer counters=%+v, want no reduction establishment for direct arena lifecycle", counters)
	}
}

func TestG18DropCohortDerivationIncludesCheckpointAndStableBytes(t *testing.T) {
	build := func(t *testing.T) (DropCohortDerivationRecord, DropCohortDerivationRecord) {
		t.Helper()
		compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8})
		seed, err := compact.Seed(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		head := g18ProducerHead(t, compact, seed, 31, 4)
		checkpoint := g18ProducerCheckpoint()
		var first, second DropCohortDerivationHandle
		err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			first, err = compact.BuildDropCohortDerivationOwned(owner, head, checkpoint)
			if err != nil {
				return err
			}
			second, err = compact.BuildDropCohortDerivationOwned(owner, head, checkpoint)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		firstRecord, ok := compact.DropCohortDerivationRecord(first)
		if !ok {
			t.Fatal("first derivation record is unavailable")
		}
		secondRecord, ok := compact.DropCohortDerivationRecord(second)
		if !ok {
			t.Fatal("second derivation record is unavailable")
		}
		return firstRecord, secondRecord
	}
	firstA, firstB := build(t)
	secondA, secondB := build(t)
	if firstA.Checkpoint != g18ProducerCheckpoint() || secondA.Checkpoint != g18ProducerCheckpoint() {
		t.Fatalf("checkpoint metadata first=%+v second=%+v", firstA.Checkpoint, secondA.Checkpoint)
	}
	if firstA.StackDepth != 1 || secondA.StackDepth != 1 || firstA.RootSymbol != 31 || secondA.RootSymbol != 31 {
		t.Fatalf("derivation shape first=%+v second=%+v", firstA, secondA)
	}
	if !bytes.Equal(firstA.Bytes, secondA.Bytes) || firstA.Digest != secondA.Digest || !bytes.Equal(firstB.Bytes, secondB.Bytes) {
		t.Fatalf("same graph/checkpoint produced different canonical bytes or digest")
	}
	if len(firstA.Bytes) == 0 {
		t.Fatal("derivation record has no canonical bytes")
	}
}

func g18SemanticDerivation(t *testing.T, padded bool, childSymbol Symbol) DropCohortDerivationRecord {
	t.Helper()
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxDropCohortBytes: 1 << 16})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if padded {
		_ = g18ProducerHead(t, compact, seed, 90, 1)
	}
	child, err := compact.appendSubtree(subtreeRecord{
		symbol: childSymbol, startByte: 1, endByte: 2, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := compact.appendSubtree(subtreeRecord{
		symbol: 91, productionID: 7, dynamicPrecedence: -1,
		startByte: 1, endByte: 2,
	}, []SubtreeID{child}, []FieldMapEntry{{FieldID: 3, ChildIndex: 0}}, []Symbol{92})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.condense(compact.boundaryKey(2, 2), linkInput{prev: seed.Node, payload: parent})
	if err != nil {
		t.Fatal(err)
	}
	var derivation DropCohortDerivationHandle
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		derivation, err = compact.BuildDropCohortDerivationOwned(owner, head, DropCohortSourceCheckpoint{
			StartByte: 1, EndByte: 2, StartRow: 4, StartColumn: 5, EndRow: 4, EndColumn: 6,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	record, ok := compact.DropCohortDerivationRecord(derivation)
	if !ok {
		t.Fatal("semantic derivation record is unavailable")
	}
	return record
}

func TestG18DropCohortDerivationIgnoresAllocationIdentity(t *testing.T) {
	first := g18SemanticDerivation(t, false, 70)
	second := g18SemanticDerivation(t, true, 70)
	if !bytes.Equal(first.Bytes, second.Bytes) || first.Digest != second.Digest {
		t.Fatalf("equal semantic graphs produced different derivation bytes: first=%x second=%x", first.Digest, second.Digest)
	}
}

func TestG18DropCohortDerivationIncludesUnequalDescendants(t *testing.T) {
	first := g18SemanticDerivation(t, false, 70)
	second := g18SemanticDerivation(t, false, 71)
	if bytes.Equal(first.Bytes, second.Bytes) || first.Digest == second.Digest {
		t.Fatal("unequal descendant graphs produced equal derivation bytes")
	}
}

func g18SemanticBranchDerivation(t *testing.T, reverse bool) DropCohortDerivationRecord {
	t.Helper()
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxDropCohortBytes: 1 << 16})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var first, second SubtreeID
	if reverse {
		first, err = compact.appendSubtree(subtreeRecord{symbol: Symbol(82), startByte: 2, endByte: 3, terminal: true}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		second, err = compact.appendSubtree(subtreeRecord{symbol: Symbol(81), startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
	} else {
		first, err = compact.appendSubtree(subtreeRecord{symbol: Symbol(81), startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		second, err = compact.appendSubtree(subtreeRecord{symbol: Symbol(82), startByte: 2, endByte: 3, terminal: true}, nil, nil, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	var firstPayload, secondPayload SubtreeID
	if reverse {
		firstPayload, secondPayload = second, first
	} else {
		firstPayload, secondPayload = first, second
	}
	node, err := compact.appendAdjacencyNodeAt(2, 3, 0, []linkRecord{
		{prev: seed.Node, payload: firstPayload, scoreDelta: 1},
		{prev: seed.Node, payload: secondPayload, scoreDelta: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	var derivation DropCohortDerivationHandle
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		derivation, err = compact.BuildDropCohortDerivationOwned(owner, Head{Node: node}, DropCohortSourceCheckpoint{StartByte: 1, EndByte: 3})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	record, ok := compact.DropCohortDerivationRecord(derivation)
	if !ok {
		t.Fatal("semantic branch derivation record is unavailable")
	}
	return record
}

func TestG18DropCohortDerivationStableOrderingAcrossAllocationOrder(t *testing.T) {
	first := g18SemanticBranchDerivation(t, false)
	second := g18SemanticBranchDerivation(t, true)
	if !bytes.Equal(first.Bytes, second.Bytes) || first.Digest != second.Digest {
		t.Fatalf("equal branch graphs produced different ordering bytes: first=%x second=%x", first.Digest, second.Digest)
	}
}

func TestG18DropCohortTinyCapBoundsScratchAndRollsBack(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxDropCohortBytes: 96})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := compact.appendSubtree(subtreeRecord{symbol: 73, startByte: 1, endByte: 2, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 12; index++ {
		payload, err = compact.appendSubtree(subtreeRecord{symbol: Symbol(74 + index), startByte: 1, endByte: 2}, []SubtreeID{payload}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	head, err := compact.condense(compact.boundaryKey(2, 2), linkInput{prev: seed.Node, payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error { return nil }); err != nil {
		t.Fatal(err)
	}
	beforeStorage, beforeFootprint := compact.StorageBytes(), compact.FootprintBytes()
	beforeDerivations, beforeBytes := len(compact.dropCohortDerivations), len(compact.dropCohortDerivationBytes)
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, buildErr := compact.BuildDropCohortDerivationOwned(owner, head, DropCohortSourceCheckpoint{StartByte: 1, EndByte: 2})
		if buildErr == nil {
			return errors.New("tiny drop-cohort cap accepted a large authentic graph")
		}
		return buildErr
	})
	if err == nil {
		t.Fatal("tiny-cap derivation unexpectedly succeeded")
	}
	if len(compact.dropCohortDerivations) != beforeDerivations || len(compact.dropCohortDerivationBytes) != beforeBytes ||
		compact.StorageBytes() != beforeStorage || compact.FootprintBytes() != beforeFootprint {
		t.Fatalf("tiny-cap rollback changed arena: derivations=%d bytes=%d storage=%d footprint=%d", len(compact.dropCohortDerivations), len(compact.dropCohortDerivationBytes), compact.StorageBytes(), compact.FootprintBytes())
	}
	if uint64(cap(compact.dropCohortDerivationScratch)) > compact.limits.MaxDropCohortBytes {
		t.Fatalf("scratch capacity=%d exceeds cap=%d", cap(compact.dropCohortDerivationScratch), compact.limits.MaxDropCohortBytes)
	}
}

func TestG18DropCohortWideBoundaryTinyCapBoundsEphemeralScratch(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{
		MaxDerivations:       8,
		MaxLinksPerBoundary:  32,
		MaxDropCohortBytes:   256,
		MaxDropCohortMembers: 2,
	})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	links := make([]linkRecord, 0, 32)
	for index := 0; index < cap(links); index++ {
		payload, payloadErr := compact.appendSubtree(subtreeRecord{
			symbol: Symbol(100 + index), startByte: 1, endByte: 2, terminal: true,
		}, nil, nil, nil)
		if payloadErr != nil {
			t.Fatal(payloadErr)
		}
		links = append(links, linkRecord{prev: seed.Node, payload: payload, scoreDelta: int64(index)})
	}
	node, err := compact.appendAdjacencyNodeAt(2, 2, 0, links)
	if err != nil {
		t.Fatal(err)
	}
	head := Head{Node: node}
	if err := compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error { return nil }); err != nil {
		t.Fatal(err)
	}
	beforeStorage, beforeFootprint := compact.StorageBytes(), compact.FootprintBytes()
	beforeDerivations, beforeBytes := len(compact.dropCohortDerivations), len(compact.dropCohortDerivationBytes)
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, buildErr := compact.BuildDropCohortDerivationOwned(owner, head, DropCohortSourceCheckpoint{StartByte: 1, EndByte: 2})
		if buildErr == nil {
			return errors.New("wide tiny-cap derivation unexpectedly succeeded")
		}
		return buildErr
	})
	if err == nil {
		t.Fatal("wide tiny-cap derivation did not fail closed")
	}
	if compact.dropCohortEphemeralPeak == 0 || compact.dropCohortEphemeralPeak > compact.limits.MaxDropCohortBytes {
		t.Fatalf("ephemeral peak=%d, want bounded positive peak at cap=%d", compact.dropCohortEphemeralPeak, compact.limits.MaxDropCohortBytes)
	}
	if compact.dropCohortEphemeralBytes != 0 || uint64(cap(compact.dropCohortDerivationScratch)) > compact.limits.MaxDropCohortBytes {
		t.Fatalf("ephemeral scratch after rollback current=%d capacity=%d cap=%d", compact.dropCohortEphemeralBytes, cap(compact.dropCohortDerivationScratch), compact.limits.MaxDropCohortBytes)
	}
	if len(compact.dropCohortDerivations) != beforeDerivations || len(compact.dropCohortDerivationBytes) != beforeBytes ||
		compact.StorageBytes() != beforeStorage || compact.FootprintBytes() != beforeFootprint {
		t.Fatalf("wide tiny-cap rollback changed arena: derivations=%d bytes=%d storage=%d footprint=%d", len(compact.dropCohortDerivations), len(compact.dropCohortDerivationBytes), compact.StorageBytes(), compact.FootprintBytes())
	}
}

func g18DeepBranchedPathCore(t *testing.T, maxBytes uint64) (*Core, Head) {
	t.Helper()
	compact := newTinyCoreWithLimits(t, Limits{
		MaxDerivations:      8,
		MaxLinksPerBoundary: 8,
		MaxDropCohortBytes:  maxBytes,
	})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := compact.appendSubtree(subtreeRecord{
		symbol: 77, startByte: 1, endByte: 2, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	appendSingle := func(byteOffset uint32, prev NodeID) NodeID {
		node, nodeErr := compact.appendAdjacencyNodeAt(2, byteOffset, 0, []linkRecord{{
			prev: prev, payload: payload,
		}})
		if nodeErr != nil {
			t.Fatal(nodeErr)
		}
		return node
	}
	leftDepthTwo := appendSingle(2, seed.Node)
	rightDepthTwo := appendSingle(2, seed.Node)
	leftDepthThree := appendSingle(3, leftDepthTwo)
	rightDepthThree := appendSingle(3, rightDepthTwo)
	root, err := compact.appendAdjacencyNodeAt(2, 4, 0, []linkRecord{
		{prev: leftDepthThree, payload: payload},
		{prev: rightDepthThree, payload: payload},
	})
	if err != nil {
		t.Fatal(err)
	}
	return compact, Head{Node: root}
}

func TestG18DropCohortDeepBranchedPathScratchOwnershipAndCapRollback(t *testing.T) {
	const generousCap = 1 << 16
	compact, head := g18DeepBranchedPathCore(t, generousCap)
	if err := compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var derivation DropCohortDerivationHandle
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var buildErr error
		derivation, buildErr = compact.BuildDropCohortDerivationOwned(owner, head, DropCohortSourceCheckpoint{StartByte: 1, EndByte: 2})
		return buildErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if derivation.Index == 0 || cap(compact.dropCohortPathScratch) < 4 {
		t.Fatalf("deep path scratch cap=%d derivation=%+v, want authoritative grown capacity", cap(compact.dropCohortPathScratch), derivation)
	}
	record, ok := compact.DropCohortDerivationRecord(derivation)
	if !ok || record.StackDepth < 3 {
		t.Fatalf("deep branched derivation=%+v ok=%t, want at least three path steps", record, ok)
	}
	if compact.dropCohortEphemeralPeak == 0 || compact.dropCohortEphemeralPeak > generousCap {
		t.Fatalf("deep path ephemeral peak=%d, want positive bounded peak at cap=%d", compact.dropCohortEphemeralPeak, generousCap)
	}
	highPeak := compact.dropCohortEphemeralPeak

	tinyCap := highPeak - 1
	if tinyCap == 0 {
		t.Fatal("deep path generous build did not charge scratch")
	}
	tiny, tinyHead := g18DeepBranchedPathCore(t, tinyCap)
	if err := tiny.ApplySchedulerAtomic(func(SchedulerTransactionToken) error { return nil }); err != nil {
		t.Fatal(err)
	}
	tinyBeforeStorage, tinyBeforeFootprint := tiny.StorageBytes(), tiny.FootprintBytes()
	err = tiny.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, buildErr := tiny.BuildDropCohortDerivationOwned(owner, tinyHead, DropCohortSourceCheckpoint{StartByte: 1, EndByte: 2})
		if buildErr == nil {
			return errors.New("deep branched tiny cap accepted scratch above the boundary")
		}
		return buildErr
	})
	if err == nil {
		t.Fatal("deep branched tiny cap unexpectedly succeeded")
	}
	if tiny.dropCohortEphemeralPeak > tinyCap || tiny.dropCohortEphemeralBytes != 0 {
		t.Fatalf("deep branched tiny-cap scratch peak=%d current=%d cap=%d", tiny.dropCohortEphemeralPeak, tiny.dropCohortEphemeralBytes, tinyCap)
	}
	if len(tiny.dropCohortDerivations) != 0 || len(tiny.dropCohortDerivationBytes) != 0 ||
		tiny.StorageBytes() != tinyBeforeStorage || tiny.FootprintBytes() != tinyBeforeFootprint {
		t.Fatalf("deep branched rollback changed arena: derivations=%d bytes=%d storage=%d/%d footprint=%d/%d", len(tiny.dropCohortDerivations), len(tiny.dropCohortDerivationBytes), tiny.StorageBytes(), tinyBeforeStorage, tiny.FootprintBytes(), tinyBeforeFootprint)
	}
	if err := tiny.Reset(); err != nil {
		t.Fatal(err)
	}
	if tiny.nodes.Len() != 0 || tiny.links.Len() != 0 || len(tiny.dropCohortDerivations) != 0 ||
		len(tiny.dropCohortDerivationBytes) != 0 || len(tiny.dropCohortPathScratch) != 0 ||
		tiny.dropCohortEphemeralBytes != 0 || tiny.dropCohortEphemeralPeak != 0 {
		t.Fatalf("deep branched reset retained live state: nodes=%d links=%d derivations=%d bytes=%d path=%d current=%d peak=%d", tiny.nodes.Len(), tiny.links.Len(), len(tiny.dropCohortDerivations), len(tiny.dropCohortDerivationBytes), len(tiny.dropCohortPathScratch), tiny.dropCohortEphemeralBytes, tiny.dropCohortEphemeralPeak)
	}
}

func TestG18DropCohortProducerMutationRequiresAuthenticatedToken(t *testing.T) {
	newCore := func(t *testing.T) *Core {
		t.Helper()
		return newTinyCoreWithLimits(t, Limits{MaxDropCohortBytes: 256})
	}
	t.Run("no-token", func(t *testing.T) {
		compact := newCore(t)
		if err := compact.RecordDropCohortProducerMutation(SchedulerTransactionToken{}, DropCohortProducerLinearCanonicalization); err == nil {
			t.Fatal("zero token was accepted")
		}
		if compact.DropCohortProducerCounts().LinearCanonicalizer != 0 {
			t.Fatal("zero-token mutation changed producer counters")
		}
	})
	t.Run("stale-token", func(t *testing.T) {
		compact := newCore(t)
		var stale SchedulerTransactionToken
		if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			stale = owner
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error {
			if callErr := compact.RecordDropCohortProducerMutation(stale, DropCohortProducerLinearCanonicalization); callErr == nil {
				return errors.New("stale token was accepted")
			}
			return nil
		}); err == nil {
			t.Fatal("stale token did not poison the scheduler transaction")
		}
		if compact.DropCohortProducerCounts().LinearCanonicalizer != 0 {
			t.Fatal("stale-token mutation changed producer counters")
		}
	})
	t.Run("foreign-token", func(t *testing.T) {
		compact := newCore(t)
		foreign := newCore(t)
		if err := compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error {
			return foreign.RunFreshSchedulerSession(func(foreignToken SchedulerTransactionToken) error {
				if callErr := compact.RecordDropCohortProducerMutation(foreignToken, DropCohortProducerLinearCanonicalization); callErr == nil {
					return errors.New("foreign token was accepted")
				}
				return nil
			})
		}); err == nil {
			t.Fatal("foreign token did not poison the scheduler transaction")
		}
		if compact.DropCohortProducerCounts().LinearCanonicalizer != 0 {
			t.Fatal("foreign-token mutation changed producer counters")
		}
	})
	t.Run("rollback", func(t *testing.T) {
		compact := newCore(t)
		sentinel := errors.New("authentic producer rollback")
		err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			if callErr := compact.RecordDropCohortProducerMutation(owner, DropCohortProducerLinearCanonicalization); callErr != nil {
				return callErr
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("rollback error=%v, want sentinel", err)
		}
		if compact.DropCohortProducerCounts().LinearCanonicalizer != 0 {
			t.Fatal("rolled-back mutation changed producer counters")
		}
	})
	t.Run("commit", func(t *testing.T) {
		compact := newCore(t)
		if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			return compact.RecordDropCohortProducerMutation(owner, DropCohortProducerLinearCanonicalization)
		}); err != nil {
			t.Fatal(err)
		}
		if compact.DropCohortProducerCounts().LinearCanonicalizer != 1 {
			t.Fatal("authenticated committed mutation was not counted")
		}
	})
}

func TestG18DropCohortDeadHistoryCounterTracksAuthenticImport(t *testing.T) {
	table := &fakeTable{
		actions: map[tableCell][]Action{{state: 1, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 2},
	}
	compact, err := New(table, Limits{MaxDerivations: 8, MaxPopPaths: 8, MaxDropCohortBytes: 1 << 16})
	if err != nil {
		t.Fatal(err)
	}
	compact.historicalCertificateAuthentication = true
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	makeSource := func(symbols ...Symbol) Head {
		t.Helper()
		links := make([]linkRecord, 0, len(symbols))
		for index, symbol := range symbols {
			payload, payloadErr := compact.appendSubtree(subtreeRecord{symbol: symbol, startByte: 0, endByte: 1, terminal: true}, nil, nil, nil)
			if payloadErr != nil {
				t.Fatal(payloadErr)
			}
			links = append(links, linkRecord{prev: seed.Node, payload: payload, scoreDelta: int64(index)})
		}
		node, nodeErr := compact.appendAdjacencyNodeAt(1, 1, 0, links)
		if nodeErr != nil {
			t.Fatal(nodeErr)
		}
		return Head{Node: node}
	}
	firstHead := makeSource(101, 102)
	secondHead := makeSource(103, 104)
	firstBoundary, err := compact.ClassifyBoundary(firstHead, 9)
	if err != nil {
		t.Fatal(err)
	}
	var firstOutputs []ReductionOutput
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		firstOutputs, err = compact.ReduceOutputsClassifiedIntoOwned(owner, nil, firstBoundary, 0, ForkOrder{})
		if err != nil {
			return err
		}
		if len(firstOutputs) != 1 || !firstOutputs[0].MultiplePopPaths {
			return fmt.Errorf("first reduction outputs=%+v, want one multi-path output", firstOutputs)
		}
		if err := compact.RecordReductionLineageOwned(owner, firstOutputs, 7); err != nil {
			return err
		}
		cohort, err := compact.BeginDropCohortOwned(owner, DropCohortActionIdentity{
			BoundaryState: 1,
			Lookahead:     9,
			ActionOrdinal: 0,
			Action:        Action{Type: ActionReduce, Symbol: 2, ChildCount: 1},
		}, 1)
		if err != nil {
			return err
		}
		derivation, err := compact.BuildDropCohortDerivationOwned(owner, firstOutputs[0].Head, DropCohortSourceCheckpoint{})
		if err != nil {
			return err
		}
		if err := compact.WriteDropCohortMemberOwned(owner, cohort, firstOutputs[0].Head, 0, derivation); err != nil {
			return err
		}
		refs, err := compact.FinalizeDropCohortOwned(owner, cohort)
		if err != nil {
			return err
		}
		return compact.RecordHeadLineageRefsOwned(owner, firstOutputs[0].Head, refs)
	})
	if err != nil {
		t.Fatal(err)
	}
	secondBoundary, err := compact.ClassifyBoundary(secondHead, 9)
	if err != nil {
		t.Fatal(err)
	}
	var secondOutputs []ReductionOutput
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		secondOutputs, err = compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, nil, secondBoundary, 0, ForkOrder{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondOutputs) != 1 || secondOutputs[0].HistoricalBoundaryProvenance != HistoricalBoundaryConverged || secondOutputs[0].HistoricalAlternativeSet.Len() == 0 {
		t.Fatalf("second reduction historical output=%+v, want proven import", secondOutputs)
	}
	counters := compact.DropCohortProducerCounts()
	if counters.DeadHistoryImport != 1 {
		t.Fatalf("authentic dead-history counter=%+v, want one import", counters)
	}
}

func TestG18DropCohortPreflightRollbackRestoresProducerArena(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDropCohortMembers: 2})
	if err := compact.ApplySchedulerAtomic(func(SchedulerTransactionToken) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before := compact.StorageBytes()
	beforeFootprint := compact.FootprintBytes()
	beforeActions := len(compact.dropCohortActions)
	beforeRecords := len(compact.dropCohortRecords)
	sentinel := errors.New("g18 producer preflight")
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if _, err := compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), 3); err == nil {
			return errors.New("drop-cohort preflight accepted three members with a two-member limit")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction error=%v, want sentinel", err)
	}
	if len(compact.dropCohortActions) != beforeActions || len(compact.dropCohortRecords) != beforeRecords ||
		compact.StorageBytes() != before || compact.FootprintBytes() != beforeFootprint {
		t.Fatalf("preflight rollback changed producer arena: actions=%d records=%d storage=%d footprint=%d", len(compact.dropCohortActions), len(compact.dropCohortRecords), compact.StorageBytes(), compact.FootprintBytes())
	}
}

func TestG18DropCohortNestedAndSequentialIsolation(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxDropCohortMembers: 2})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head := g18ProducerHead(t, compact, seed, 41, 1)
	checkpoint := g18ProducerCheckpoint()
	var outer, inner DropCohortHandle
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		outer, err = compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), 1)
		if err != nil {
			return err
		}
		inner, err = compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), 1)
		if err != nil {
			return err
		}
		for _, cohort := range []DropCohortHandle{inner, outer} {
			derivation, buildErr := compact.BuildDropCohortDerivationOwned(owner, head, checkpoint)
			if buildErr != nil {
				return buildErr
			}
			if writeErr := compact.WriteDropCohortMemberOwned(owner, cohort, head, 0, derivation); writeErr != nil {
				return writeErr
			}
			if _, finalErr := compact.FinalizeDropCohortOwned(owner, cohort); finalErr != nil {
				return finalErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if outer.Sequence == 0 || inner.Sequence != outer.Sequence+1 || outer.Owner != inner.Owner || outer.Epoch != inner.Epoch {
		t.Fatalf("nested handles outer=%+v inner=%+v", outer, inner)
	}
	outerRef, err := compact.DropCohortRefForBranch(outer, 0)
	if err != nil {
		t.Fatal(err)
	}
	innerRef, err := compact.DropCohortRefForBranch(inner, 0)
	if err != nil {
		t.Fatal(err)
	}
	if outerRef == innerRef {
		t.Fatalf("nested refs alias: outer=%+v inner=%+v", outerRef, innerRef)
	}
	var sequential DropCohortHandle
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		sequential, err = compact.BeginDropCohortOwned(owner, g18ProducerIdentity(), 1)
		if err != nil {
			return err
		}
		derivation, buildErr := compact.BuildDropCohortDerivationOwned(owner, head, checkpoint)
		if buildErr != nil {
			return buildErr
		}
		if err := compact.WriteDropCohortMemberOwned(owner, sequential, head, 0, derivation); err != nil {
			return err
		}
		_, err = compact.FinalizeDropCohortOwned(owner, sequential)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if sequential.Sequence != inner.Sequence+1 {
		t.Fatalf("sequential sequence=%d, want %d", sequential.Sequence, inner.Sequence+1)
	}
	if !reflect.DeepEqual(outerRef, DropCohortRef{Owner: outer.Owner, Epoch: outer.Epoch, Sequence: outer.Sequence, Branch: 0}) {
		t.Fatalf("outer ref=%+v", outerRef)
	}
}

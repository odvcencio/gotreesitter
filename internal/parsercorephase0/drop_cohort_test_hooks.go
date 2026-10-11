package parsercorephase0

// This file contains the narrow Core contract used by the tagged G18 tests.
// It exposes verifier test hooks while production certificate admission remains inactive.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
)

const (
	dropCohortProtocolActionBytes    = uint64(14 * 8)
	dropCohortProtocolDerivationMeta = uint64(32 + 4 + 4)
	dropCohortProtocolReferenceBytes = uint64(32)
	dropCohortProtocolMapBytes       = uint64(16)
	dropCohortProtocolInternerBytes  = uint64(40)
	dropCohortProtocolJournalBytes   = uint64(16)
	dropCohortProtocolCohortBytes    = uint64(32)
	dropCohortLogicalActionBytes     = uint64(14 * 8)
	dropCohortLogicalDerivationBytes = uint64(32 + 4 + 4)
	dropCohortLogicalRecordBytes     = uint64(32)
)

type dropCohortProtocolCohort struct {
	Handle   [3]uint64 `json:"handle"`
	State    string    `json:"state"`
	Expected uint16    `json:"expected"`
	Written  uint16    `json:"written"`
	Spilled  bool      `json:"spilled"`
}

type dropCohortProtocolSnapshot struct {
	Schema                 string                     `json:"schema"`
	ArenaOwner             uint64                     `json:"arena_owner"`
	ArenaEpoch             uint64                     `json:"arena_epoch"`
	Cohorts                []dropCohortProtocolCohort `json:"cohorts"`
	Storage                [7]uint64                  `json:"storage"`
	Footprint              [7]uint64                  `json:"footprint"`
	StorageBytes           uint64                     `json:"storage_bytes"`
	FootprintBytes         uint64                     `json:"footprint_bytes"`
	LogicalStorageBytes    uint64                     `json:"logical_storage_bytes"`
	LogicalFootprintBytes  uint64                     `json:"logical_footprint_bytes"`
	PhysicalStorageBytes   uint64                     `json:"physical_storage_bytes"`
	PhysicalFootprintBytes uint64                     `json:"physical_footprint_bytes"`
	ProducerWrites         map[string]uint64          `json:"producer_writes"`
	VerifierElections      uint64                     `json:"verifier_elections"`
	VerifierProofs         uint64                     `json:"verifier_proofs"`
	VerifierDeclines       uint64                     `json:"verifier_declines"`
	ActionDeclines         uint64                     `json:"action_identity_declines"`
	DerivationDeclines     uint64                     `json:"derivation_identity_declines"`
	AuthenticatedHistory   uint64                     `json:"authenticated_history_imports"`
	UnprovedHistory        uint64                     `json:"unproved_history_imports"`
	DeclineReasons         map[string]uint64          `json:"decline_reasons"`
	OwnerCheckedLookups    uint64                     `json:"owner_checked_lookups"`
	InlineReads            uint64                     `json:"inline_reads"`
	SpillReads             uint64                     `json:"spill_reads"`
	MapReads               uint64                     `json:"map_reads"`
	InternerReads          uint64                     `json:"interner_reads"`
}

func dropCohortProtocolHandle(h DropCohortHandle) [3]uint64 {
	return [3]uint64{h.Owner, h.Epoch, h.Sequence}
}

func dropCohortProtocolState(state DropCohortState) string {
	switch state {
	case DropCohortBuilding:
		return "building"
	case DropCohortComplete:
		return "complete"
	case DropCohortOverflowed:
		return "overflowed"
	case DropCohortBlended:
		return "blended"
	case DropCohortUnproved:
		return "unproved"
	default:
		return "unknown"
	}
}

func dropCohortProtocolNextPowerOfTwo(value uint64) uint64 {
	if value <= 1 {
		return 1
	}
	value--
	value |= value >> 1
	value |= value >> 2
	value |= value >> 4
	value |= value >> 8
	value |= value >> 16
	value |= value >> 32
	return value + 1
}

func dropCohortStoreVector(c *Core, footprint bool) [7]uint64 {
	var vector [7]uint64
	byteCount := func(length, capacity uint64, element uint64) uint64 {
		if footprint {
			return capacity * element
		}
		return length * element
	}
	vector[0] = byteCount(uint64(len(c.dropCohortActions)), uint64(cap(c.dropCohortActions)), dropCohortLogicalActionBytes)
	vector[1] = byteCount(uint64(len(c.dropCohortDerivations)), uint64(cap(c.dropCohortDerivations)), dropCohortLogicalDerivationBytes) + byteCount(uint64(len(c.dropCohortDerivationBytes)), uint64(cap(c.dropCohortDerivationBytes)), 1)
	vector[2] = byteCount(uint64(len(c.dropCohortCertificateRefs)), uint64(cap(c.dropCohortCertificateRefs)), coreDropCohortRefBytes)
	vector[3] = byteCount(uint64(len(c.dropCohortMapStore)), uint64(cap(c.dropCohortMapStore)), coreDropCohortMapEntryBytes)
	vector[4] = byteCount(uint64(len(c.dropCohortDerivationIntern)), uint64(cap(c.dropCohortDerivationIntern)), coreDropCohortDerivationInternBytes)
	vector[5] = byteCount(uint64(len(c.dropCohortJournalStore)), uint64(cap(c.dropCohortJournalStore)), coreDropCohortJournalStoreBytes)
	vector[6] = byteCount(uint64(len(c.dropCohortRecords)), uint64(cap(c.dropCohortRecords)), dropCohortLogicalRecordBytes)
	return vector
}

func (c *Core) DiagnosticDropCohortArenaIdentityForTest() (uint64, uint64) {
	return c.DropCohortArenaIdentity()
}

func (c *Core) DiagnosticDropCohortSnapshotForTest() []byte {
	if c == nil {
		return []byte(`{"schema":"gts-drop-cohort-certificate-arena/v2"}`)
	}
	var snapshot dropCohortProtocolSnapshot
	snapshot.Schema = "gts-drop-cohort-certificate-arena/v2"
	snapshot.ArenaOwner, snapshot.ArenaEpoch = c.DropCohortArenaIdentity()
	snapshot.ProducerWrites = map[string]uint64{
		"reduction_establishment": c.dropCohortProducerWrites[dropCohortProducerReductionEstablishment],
		"linear_canonicalizer":    c.dropCohortProducerWrites[dropCohortProducerLinearCanonicalizer],
		"mapped_canonicalizer":    c.dropCohortProducerWrites[dropCohortProducerMappedCanonicalizer],
		"sibling_adoption":        c.dropCohortProducerWrites[dropCohortProducerSiblingAdoption],
		"conflict_reconciliation": c.dropCohortProducerWrites[dropCohortProducerConflictReconciliation],
		"dead_history_import":     c.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport],
	}
	snapshot.AuthenticatedHistory = c.dropCohortAuthenticatedHistory
	snapshot.UnprovedHistory = c.dropCohortUnprovedHistory
	snapshot.DeclineReasons = map[string]uint64{}
	snapshot.VerifierElections = c.dropCohortVerifierElections
	snapshot.VerifierProofs = c.dropCohortVerifierProofs
	snapshot.VerifierDeclines = c.dropCohortVerifierDeclines
	snapshot.ActionDeclines = c.dropCohortActionDeclines
	snapshot.DerivationDeclines = c.dropCohortDerivationDeclines
	for reason := dropCohortVerifierReason(1); reason < dropCohortVerifierReasonCount; reason++ {
		if count := c.dropCohortDeclineReasons[reason]; count != 0 {
			snapshot.DeclineReasons[dropCohortVerifierReasonString(reason)] = count
		}
	}
	for _, record := range c.dropCohortRecords {
		snapshot.Cohorts = append(snapshot.Cohorts, dropCohortProtocolCohort{
			Handle: dropCohortProtocolHandle(record.handle), State: dropCohortProtocolState(record.state),
			Expected: record.expected, Written: record.written, Spilled: record.expected > 0,
		})
	}
	snapshot.Storage = dropCohortStoreVector(c, false)
	snapshot.Footprint = dropCohortStoreVector(c, true)
	for _, value := range snapshot.Storage {
		snapshot.LogicalStorageBytes += value
	}
	for _, value := range snapshot.Footprint {
		snapshot.LogicalFootprintBytes += value
	}
	snapshot.StorageBytes = snapshot.LogicalStorageBytes
	snapshot.FootprintBytes = snapshot.LogicalFootprintBytes
	snapshot.PhysicalStorageBytes = c.StorageBytes()
	snapshot.PhysicalFootprintBytes = c.FootprintBytes()
	snapshot.OwnerCheckedLookups = c.dropCohortOwnerCheckedLookups
	snapshot.InlineReads = c.dropCohortInlineReads
	snapshot.SpillReads = c.dropCohortSpillReads
	snapshot.MapReads = c.dropCohortMapReads
	snapshot.InternerReads = c.dropCohortInternerReads
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return []byte(`{"schema":"gts-drop-cohort-certificate-arena/v2"}`)
	}
	return encoded
}

func (c *Core) DiagnosticDropCohortLimitsForTest() (uint16, uint16) {
	if c == nil {
		return 0, 0
	}
	return uint16(c.limits.MaxDropCohorts), c.limits.MaxDropCohortMembers
}

func (c *Core) DiagnosticDropCohortSetLimitsForTest(maxCohorts, maxMembers uint16) error {
	if c == nil || maxCohorts == 0 || maxMembers == 0 || maxMembers > dropCohortRefHardCap {
		return errors.New("parser-core phase zero: invalid drop-cohort test limits")
	}
	c.limits.MaxDropCohorts = uint32(maxCohorts)
	c.limits.MaxDropCohortMembers = maxMembers
	return nil
}

func dropCohortProtocolAction(values [14]int64) (DropCohortActionIdentity, error) {
	for _, index := range []int{0, 1, 2, 3, 4, 5, 6, 7, 13} {
		if values[index] < 0 {
			return DropCohortActionIdentity{}, errors.New("parser-core phase zero: negative action identity field")
		}
	}
	if values[0] > math.MaxUint32 || values[4] > math.MaxUint32 || values[2] > math.MaxInt32 ||
		values[1] > math.MaxUint16 || values[5] > math.MaxUint16 || values[3] > math.MaxUint8 ||
		values[7] > math.MaxUint8 || values[6] > math.MaxUint16 || values[13] > math.MaxUint8 ||
		values[8] < math.MinInt16 || values[8] > math.MaxInt16 {
		return DropCohortActionIdentity{}, errors.New("parser-core phase zero: action identity field overflow")
	}
	for _, index := range []int{9, 10, 11, 12} {
		if values[index] != 0 && values[index] != 1 {
			return DropCohortActionIdentity{}, errors.New("parser-core phase zero: boolean action identity field overflow")
		}
	}
	return DropCohortActionIdentity{
		BoundaryState: StateID(values[0]), Lookahead: Symbol(values[1]), ActionOrdinal: int32(values[2]),
		Action: Action{Type: ActionType(values[3]), State: StateID(values[4]), Symbol: Symbol(values[5]),
			ProductionID: uint16(values[6]), ChildCount: uint8(values[7]), DynamicPrecedence: int16(values[8]),
			Extra: values[9] != 0, ExtraChain: values[10] != 0, Repetition: values[11] != 0},
		NoLookahead: values[12] != 0, Selection: DropCohortSelectionClass(values[13]),
	}, nil
}

func dropCohortRealStorePlan(expected uint16, derivationBytes uint32) ([7]uint64, [7]uint64) {
	var storage, footprint [7]uint64
	members := uint64(expected)
	storage[0] = members * dropCohortProtocolActionBytes
	storage[1] = members * (dropCohortProtocolDerivationMeta + uint64(derivationBytes))
	storage[2] = members * dropCohortProtocolReferenceBytes
	storage[3] = members * dropCohortProtocolMapBytes
	storage[4] = members * dropCohortProtocolInternerBytes
	storage[5] = (members + 2) * dropCohortProtocolJournalBytes
	storage[6] = dropCohortProtocolCohortBytes
	footprint = storage
	mapCapacity := dropCohortProtocolNextPowerOfTwo(members * 2)
	footprint[3] = mapCapacity * dropCohortProtocolMapBytes
	footprint[4] = mapCapacity * dropCohortProtocolInternerBytes
	return storage, footprint
}

func (c *Core) DiagnosticDropCohortBeginForTest(expected uint16, derivationBytes uint32, caps [7]uint64) ([3]uint64, error) {
	var zero [3]uint64
	if c == nil {
		return zero, errors.New("parser-core phase zero: nil drop-cohort core")
	}
	storage, footprint := dropCohortRealStorePlan(expected, derivationBytes)
	for index := range caps {
		if footprint[index] > caps[index] {
			return zero, fmt.Errorf("parser-core phase zero: drop-cohort store %d preflight cap", index)
		}
	}
	var handle DropCohortHandle
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		identity, identityErr := dropCohortProtocolAction([14]int64{3, 9, 3, int64(ActionReduce), 0, 2, 5, 1, 2, 0, 0, 0, 0, 1})
		if identityErr != nil {
			return identityErr
		}
		var beginErr error
		handle, beginErr = c.beginDropCohortOwned(identity, int(expected))
		if beginErr != nil {
			return beginErr
		}
		if len(c.dropCohortReservations) == 0 || c.dropCohortReservations[len(c.dropCohortReservations)-1].handle != handle {
			return errors.New("parser-core phase zero: drop-cohort reservation is unavailable")
		}
		c.dropCohortReservations[len(c.dropCohortReservations)-1].derivationBytesEach = derivationBytes
		derivationDemand, demandErr := dropCohortMulChecked(uint64(expected), uint64(derivationBytes))
		if demandErr != nil {
			return demandErr
		}
		if derivationDemand > uint64(^uint(0)>>1) {
			return errors.New("parser-core phase zero: drop-cohort derivation allocation overflow")
		}
		if _, demandErr = c.dropCohortReserveDemand([7]uint64{}, derivationDemand); demandErr != nil {
			return demandErr
		}
		// Reserve every certificate store with real backing slices. The
		// reservation is part of the active Core transaction and every write
		// fills one reserved slot instead of growing a hidden protocol vector.
		if cap(c.dropCohortActions)-len(c.dropCohortActions) < int(expected)-1 {
			grown := make([]dropCohortActionIdentity, len(c.dropCohortActions), len(c.dropCohortActions)+int(expected)-1)
			copy(grown, c.dropCohortActions)
			c.dropCohortActions = grown
		}
		for count := 1; count < int(expected); count++ {
			c.dropCohortActions = append(c.dropCohortActions, identity)
		}
		c.dropCohortDerivations = append(c.dropCohortDerivations, make([]dropCohortDerivationRecord, int(expected))...)
		c.dropCohortDerivationBytes = append(c.dropCohortDerivationBytes, make([]byte, int(expected)*int(derivationBytes))...)
		internerCapacity := int(dropCohortProtocolNextPowerOfTwo(uint64(expected) * 2))
		internerStore := make([]dropCohortDerivationInternEntry, len(c.dropCohortDerivationIntern)+int(expected), len(c.dropCohortDerivationIntern)+internerCapacity)
		copy(internerStore, c.dropCohortDerivationIntern)
		c.dropCohortDerivationIntern = internerStore
		c.dropCohortCertificateRefs = append(c.dropCohortCertificateRefs, make([]DropCohortRef, int(expected))...)
		mapCapacity := int(dropCohortProtocolNextPowerOfTwo(uint64(expected) * 2))
		mapStore := make([]dropCohortMapEntry, len(c.dropCohortMapStore)+int(expected), len(c.dropCohortMapStore)+mapCapacity)
		copy(mapStore, c.dropCohortMapStore)
		c.dropCohortMapStore = mapStore
		var consumed [7]uint64
		consumed[1], consumed[2], consumed[3], consumed[4] = uint64(expected), uint64(expected), uint64(expected), uint64(expected)
		consumedBytes, consumedErr := dropCohortDemandBytes(consumed, derivationDemand)
		if consumedErr != nil {
			return consumedErr
		}
		c.dropCohortConsumeReservation(handle, consumed, consumedBytes)
		_ = storage
		_ = footprint
		return nil
	})
	if err != nil {
		return zero, err
	}
	return [3]uint64{handle.Owner, handle.Epoch, handle.Sequence}, nil
}

func (c *Core) DiagnosticDropCohortWriteForTest(rawHandle [3]uint64, head Head, branch uint16, values [14]int64, digest [sha256.Size]byte, record []byte) error {
	if c == nil {
		return errors.New("parser-core phase zero: nil drop-cohort core")
	}
	var operationErr error
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		identity, err := dropCohortProtocolAction(values)
		if err != nil {
			return err
		}
		cohort := DropCohortHandle{Owner: rawHandle[0], Epoch: rawHandle[1], Sequence: rawHandle[2]}
		cohortIndex, err := c.validateDropCohortIdentity(cohort)
		if err != nil {
			return err
		}
		var reservation *dropCohortReservation
		for index := range c.dropCohortReservations {
			if c.dropCohortReservations[index].handle == cohort {
				reservation = &c.dropCohortReservations[index]
				break
			}
		}
		if reservation == nil {
			return errors.New("parser-core phase zero: drop-cohort reservation is unavailable")
		}
		if uint64(branch) >= uint64(c.dropCohortRecords[cohortIndex].expected) {
			operationErr = c.writeDropCohortMemberOwned(cohort, head, branch, DropCohortDerivationHandle{Owner: c.dropCohortOwner, Epoch: c.dropCohortEpoch, Index: uint32(reservation.derivations + 1)})
			if operationErr != nil && strings.Contains(operationErr.Error(), "branch exceeds expected") {
				return nil
			}
			return operationErr
		}
		if uint32(len(record)) > reservation.derivationBytesEach {
			return errors.New("parser-core phase zero: drop-cohort derivation record exceeds reservation")
		}
		if prior, found := c.findDropCohortMember(c.dropCohortRecords[cohortIndex], branch); found {
			priorMember := c.dropCohortMembers[prior]
			priorDerivationIndex := int(priorMember.derivation.Index) - 1
			derivationOK := priorMember.derivation.Owner == c.dropCohortOwner && priorMember.derivation.Epoch == c.dropCohortEpoch && priorDerivationIndex >= 0 && priorDerivationIndex < len(c.dropCohortDerivations)
			priorBytesOK := false
			if derivationOK && c.dropCohortDerivations[priorDerivationIndex].digest == digest && c.dropCohortDerivations[priorDerivationIndex].byteLength == uint32(len(record)) {
				priorDerivation := c.dropCohortDerivations[priorDerivationIndex]
				start := int(priorDerivation.byteOffset)
				end := start + int(priorDerivation.byteLength)
				priorBytesOK = start >= 0 && end >= start && end <= len(c.dropCohortDerivationBytes) && bytesCompare(c.dropCohortDerivationBytes[start:end], record) == 0
			}
			if priorMember.head != head || priorMember.action != identity || !priorBytesOK {
				c.recordDropCohortMutation(cohortIndex)
				c.dropCohortRecords[cohortIndex].state = DropCohortBlended
				operationErr = errors.New("parser-core phase zero: drop-cohort branch conflict")
				return nil
			}
			operationErr = errors.New("parser-core phase zero: duplicate drop-cohort member")
			return nil
		}
		slot := reservation.derivations + int(branch)
		byteOffset := reservation.derivationBytes + int(branch)*int(reservation.derivationBytesEach)
		if byteOffset < 0 || byteOffset+int(reservation.derivationBytesEach) > len(c.dropCohortDerivationBytes) {
			return errors.New("parser-core phase zero: drop-cohort derivation slot is invalid")
		}
		for index := byteOffset; index < byteOffset+int(reservation.derivationBytesEach); index++ {
			c.dropCohortDerivationBytes[index] = 0
		}
		copy(c.dropCohortDerivationBytes[byteOffset:], record)
		c.dropCohortDerivationIntern[reservation.interner+int(branch)] = dropCohortDerivationInternEntry{digest: digest, byteOffset: uint32(byteOffset), byteLength: uint32(len(record))}
		c.dropCohortMapStore[reservation.mapEntries+int(branch)] = dropCohortMapEntry{hash: binary.LittleEndian.Uint64(digest[:8]), index: uint32(slot), used: true}
		derivation := DropCohortDerivationHandle{Owner: c.dropCohortOwner, Epoch: c.dropCohortEpoch, Index: uint32(slot + 1)}
		c.dropCohortDerivations[slot] = dropCohortDerivationRecord{handle: derivation, head: head, digest: digest, byteOffset: uint32(byteOffset), byteLength: uint32(len(record))}
		operationErr = c.writeDropCohortMemberOwned(cohort, head, branch, derivation)
		if operationErr == nil {
			for memberIndex := len(c.dropCohortMembers) - 1; memberIndex >= 0; memberIndex-- {
				if c.dropCohortMembers[memberIndex].cohort == cohort && c.dropCohortMembers[memberIndex].branch == branch {
					c.dropCohortMembers[memberIndex].action = identity
					break
				}
			}
		}
		if operationErr != nil && (strings.Contains(operationErr.Error(), "branch exceeds expected") || strings.Contains(operationErr.Error(), "branch conflict")) {
			for index := byteOffset; index < byteOffset+int(reservation.derivationBytesEach); index++ {
				c.dropCohortDerivationBytes[index] = 0
			}
			c.dropCohortDerivations[slot] = dropCohortDerivationRecord{}
			c.dropCohortDerivationIntern[reservation.interner+int(branch)] = dropCohortDerivationInternEntry{}
			c.dropCohortMapStore[reservation.mapEntries+int(branch)] = dropCohortMapEntry{}
			return nil
		}
		return operationErr
	})
	if err != nil {
		return err
	}
	return operationErr
}

func (c *Core) DiagnosticDropCohortFinalizeForTest(rawHandle [3]uint64) error {
	if c == nil {
		return errors.New("parser-core phase zero: nil drop-cohort core")
	}
	return c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, err := c.finalizeDropCohortOwned(DropCohortHandle{Owner: rawHandle[0], Epoch: rawHandle[1], Sequence: rawHandle[2]})
		return err
	})
}

func (c *Core) DiagnosticDropCohortMarkUnprovedForTest(rawHandle [3]uint64) error {
	if c == nil {
		return errors.New("parser-core phase zero: nil drop-cohort core")
	}
	return c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		cohort := DropCohortHandle{Owner: rawHandle[0], Epoch: rawHandle[1], Sequence: rawHandle[2]}
		index, err := c.validateDropCohortIdentity(cohort)
		if err != nil {
			return err
		}
		record := c.dropCohortRecords[index]
		if (record.state != DropCohortBuilding && record.state != DropCohortComplete) ||
			record.expected == 0 || record.written != record.expected {
			return errors.New("parser-core phase zero: drop-cohort is incomplete")
		}
		c.recordDropCohortMutation(index)
		c.dropCohortRecords[index].state = DropCohortUnproved
		return nil
	})
}

func (c *Core) DiagnosticDropCohortRollbackForTest(rawHandle [3]uint64) error {
	if c == nil {
		return errors.New("parser-core phase zero: nil drop-cohort core")
	}
	cohort := DropCohortHandle{Owner: rawHandle[0], Epoch: rawHandle[1], Sequence: rawHandle[2]}
	if len(c.dropCohortReservations) == 0 {
		return errors.New("parser-core phase zero: unknown drop-cohort reservation")
	}
	reservation := c.dropCohortReservations[len(c.dropCohortReservations)-1]
	if reservation.handle != cohort {
		return errors.New("parser-core phase zero: drop-cohort rollback is not the latest reservation")
	}
	if reservation.actions > len(c.dropCohortActions) || reservation.records > len(c.dropCohortRecords) || reservation.members > len(c.dropCohortMembers) || reservation.derivations > len(c.dropCohortDerivations) || reservation.derivationBytes > len(c.dropCohortDerivationBytes) || reservation.interner > len(c.dropCohortDerivationIntern) || reservation.certificateRefs > len(c.dropCohortCertificateRefs) || reservation.mapEntries > len(c.dropCohortMapStore) || reservation.journalEntries > len(c.dropCohortJournalStore) || reservation.journalMutations > len(c.dropCohortJournal) {
		return errors.New("parser-core phase zero: drop-cohort rollback checkpoint is invalid")
	}
	c.dropCohortActions = reservation.actionsHeader
	c.dropCohortRecords = reservation.recordsHeader
	c.dropCohortMembers = reservation.membersHeader
	c.dropCohortDerivations = reservation.derivationsHeader
	c.dropCohortDerivationBytes = reservation.derivationBytesHead
	c.dropCohortDerivationIntern = reservation.internerHeader
	c.dropCohortCertificateRefs = reservation.certificateRefsHead
	c.dropCohortMapStore = reservation.mapEntriesHeader
	c.dropCohortJournalStore = reservation.journalStoreHeader
	c.dropCohortJournal = reservation.journalHeader
	c.dropCohortOwnerCheckedLookups = reservation.ownerChecks
	c.dropCohortNextSequence = reservation.previousNext
	c.dropCohortReleaseReservationRemainder(cohort)
	if reservation.reservationsMoved {
		c.dropCohortReservations = reservation.reservationsHeader
	} else {
		c.dropCohortReservations = c.dropCohortReservations[: len(c.dropCohortReservations)-1 : reservation.reservationsCap]
	}
	return nil
}

func (c *Core) DiagnosticDropCohortOwnerWrapProbeForTest() (uint64, uint64, error) {
	if c == nil {
		return 0, 0, errors.New("parser-core phase zero: nil drop-cohort core")
	}
	// Exercise the same atomic allocator with a private scoped counter. Do not
	// overwrite the process allocator while another Core can allocate an owner.
	var probe atomic.Uint64
	probe.Store(math.MaxUint64)
	_, err := allocateDropCohortOwnerFrom(&probe)
	return math.MaxUint64, 0, err
}

func (c *Core) DiagnosticDropCohortEpochWrapProbeForTest() (uint64, uint64, error) {
	if c == nil {
		return 0, 0, errors.New("parser-core phase zero: nil drop-cohort core")
	}
	previous := c.dropCohortEpoch
	c.dropCohortEpoch = math.MaxUint64
	err := c.advanceDropCohortEpoch()
	c.dropCohortEpoch = previous
	return math.MaxUint64, 0, err
}

func (c *Core) DiagnosticDropCohortSequenceWrapProbeForTest() (uint64, uint64, error) {
	if c == nil {
		return 0, 0, errors.New("parser-core phase zero: nil drop-cohort core")
	}
	previous := c.dropCohortNextSequence
	c.dropCohortNextSequence = math.MaxUint64
	err := c.dropCohortPreflight(1)
	c.dropCohortNextSequence = previous
	return math.MaxUint64, 0, err
}

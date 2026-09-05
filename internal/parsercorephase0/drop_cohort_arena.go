package parsercorephase0

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
)

// DropCohortSelectionClass identifies the scheduler policy that selected an
// action. The value is part of the action identity.
type DropCohortSelectionClass uint8

const (
	DropCohortSelectionNone DropCohortSelectionClass = iota
	DropCohortSelectionConflictPolicy
	DropCohortSelectionRepetitionFold
	DropCohortSelectionRepetitionFork
)

// DropCohortActionIdentity is the exact dispatch action that created a
// producer cohort. Keep the decoded action beside the dispatch coordinates.
type DropCohortActionIdentity struct {
	BoundaryState StateID
	Lookahead     Symbol
	ActionOrdinal int32
	Action        Action
	NoLookahead   bool
	Selection     DropCohortSelectionClass
}

// DropCohortSourceCheckpoint records both byte and point coordinates for the
// lookahead that created a cohort. Scanner identities stay core-local.
type DropCohortSourceCheckpoint struct {
	StartByte    uint32
	EndByte      uint32
	StartRow     uint32
	StartColumn  uint32
	EndRow       uint32
	EndColumn    uint32
	ScannerStart CheckpointID
	ScannerEnd   CheckpointID
}

// DropCohortHandle is an arena-local cohort identity. Owner and epoch are
// checked before the sequence is looked up.
type DropCohortHandle struct {
	Owner    uint64
	Epoch    uint64
	Sequence uint64
}

// DropCohortDerivationHandle identifies one complete derivation record in the
// current arena epoch.
type DropCohortDerivationHandle struct {
	Owner uint64
	Epoch uint64
	Index uint32
}

// DropCohortState describes the producer lifecycle. Only Complete can be
// attached to scheduler output headers.
type DropCohortState uint8

const (
	DropCohortBuilding DropCohortState = iota + 1
	DropCohortComplete
	DropCohortOverflowed
	DropCohortBlended
	DropCohortUnproved
)

const (
	dropCohortProducerReductionEstablishment = iota
	dropCohortProducerLinearCanonicalizer
	dropCohortProducerMappedCanonicalizer
	dropCohortProducerSiblingAdoption
	dropCohortProducerConflictReconciliation
	dropCohortProducerDeadHistoryImport
	dropCohortProducerCount
)

// DropCohortProducerMutation identifies one authentic producer transition.
// These values are production counters, not diagnostic events.
type DropCohortProducerMutation uint8

const (
	DropCohortProducerLinearCanonicalization DropCohortProducerMutation = iota + 1
	DropCohortProducerMappedCanonicalization
	DropCohortProducerSiblingAdoption
	DropCohortProducerConflictReconciliation
	DropCohortProducerDeadHistoryImport
)

// DropCohortProducerCounters contains counters for real producer mutations.
// The producer path does not publish these counters as diagnostic telemetry.
type DropCohortProducerCounters struct {
	ReductionEstablishment uint64
	LinearCanonicalizer    uint64
	MappedCanonicalizer    uint64
	SiblingAdoption        uint64
	ConflictReconciliation uint64
	DeadHistoryImport      uint64
}

type dropCohortActionIdentity = DropCohortActionIdentity

type dropCohortRecord struct {
	handle      DropCohortHandle
	state       DropCohortState
	expected    uint16
	written     uint16
	actionIndex uint32
	memberStart uint32
}

type dropCohortMember struct {
	cohort     DropCohortHandle
	head       Head
	branch     uint16
	action     DropCohortActionIdentity
	derivation DropCohortDerivationHandle
}

type dropCohortDerivationRecord struct {
	handle     DropCohortDerivationHandle
	head       Head
	digest     [32]byte
	byteOffset uint32
	byteLength uint32
	rootSymbol Symbol
	stackDepth uint32
	checkpoint DropCohortSourceCheckpoint
}

type dropCohortDerivationInternEntry struct {
	digest     [32]byte
	byteOffset uint32
	byteLength uint32
}

type dropCohortMapEntry struct {
	hash  uint64
	index uint32
	used  bool
}

type dropCohortJournalStoreEntry struct {
	store uint8
	index uint32
	value uint64
}

type dropCohortReservation struct {
	handle              DropCohortHandle
	previousNext        uint64
	actions             int
	records             int
	members             int
	derivations         int
	derivationBytes     int
	interner            int
	certificateRefs     int
	mapEntries          int
	journalEntries      int
	derivationBytesEach uint32
	actionsCap          int
	recordsCap          int
	membersCap          int
	derivationsCap      int
	derivationBytesCap  int
	internerCap         int
	certificateRefsCap  int
	mapEntriesCap       int
	journalEntriesCap   int
	reservationsCap     int
	journalMutations    int
	ownerChecks         uint64
	reserved            [7]uint64
	reservationBytes    uint64
	journalStart        int
	journalCount        int
	journalUsed         int
	actionsHeader       []dropCohortActionIdentity
	recordsHeader       []dropCohortRecord
	membersHeader       []dropCohortMember
	derivationsHeader   []dropCohortDerivationRecord
	derivationBytesHead []byte
	internerHeader      []dropCohortDerivationInternEntry
	certificateRefsHead []DropCohortRef
	mapEntriesHeader    []dropCohortMapEntry
	journalStoreHeader  []dropCohortJournalStoreEntry
	journalHeader       []dropCohortMutation
	reservationsHeader  []dropCohortReservation
	reservationsMoved   bool
}

type dropCohortMutation struct {
	index  int
	before dropCohortRecord
}

var dropCohortOwnerCounter atomic.Uint64

func allocateDropCohortOwner() (uint64, error) {
	return allocateDropCohortOwnerFrom(&dropCohortOwnerCounter)
}

func allocateDropCohortOwnerFrom(counter *atomic.Uint64) (uint64, error) {
	if counter == nil {
		return 0, errors.New("parser-core phase zero: nil drop-cohort owner allocator")
	}
	for {
		current := counter.Load()
		if current == math.MaxUint64 {
			return 0, errors.New("parser-core phase zero: drop-cohort owner overflow")
		}
		next := current + 1
		if next == 0 {
			return 0, errors.New("parser-core phase zero: drop-cohort owner wrapped")
		}
		if counter.CompareAndSwap(current, next) {
			return next, nil
		}
	}
}

func (c *Core) advanceDropCohortEpoch() error {
	if c == nil || c.dropCohortEpoch == math.MaxUint64 {
		return errors.New("parser-core phase zero: drop-cohort epoch overflow")
	}
	c.dropCohortEpoch++
	if c.dropCohortEpoch == 0 {
		return errors.New("parser-core phase zero: drop-cohort epoch wrapped")
	}
	return nil
}

func (c *Core) dropCohortHandle(sequence uint64) DropCohortHandle {
	return DropCohortHandle{Owner: c.dropCohortOwner, Epoch: c.dropCohortEpoch, Sequence: sequence}
}

func (c *Core) validateDropCohortIdentity(handle DropCohortHandle) (int, error) {
	if c == nil || c.dropCohortOwner == 0 || c.dropCohortEpoch == 0 {
		return -1, errors.New("parser-core phase zero: drop-cohort arena identity is unavailable")
	}
	// Count every attempted lookup before comparing identity. This counter is
	// diagnostic-only and does not authorize a certificate read.
	c.dropCohortOwnerCheckedLookups++
	// Check the owner and epoch before touching the cohort records. This order
	// keeps stale references from reading any producer state.
	if handle.Owner != c.dropCohortOwner {
		return -1, errors.New("parser-core phase zero: drop-cohort owner mismatch")
	}
	if handle.Epoch != c.dropCohortEpoch {
		return -1, errors.New("parser-core phase zero: drop-cohort epoch mismatch")
	}
	if handle.Sequence == 0 {
		return -1, errors.New("parser-core phase zero: zero drop-cohort sequence")
	}
	for index := range c.dropCohortRecords {
		if c.dropCohortRecords[index].handle.Sequence == handle.Sequence {
			return index, nil
		}
	}
	return -1, errors.New("parser-core phase zero: unknown drop-cohort sequence")
}

// DropCohortArenaIdentity returns the stable producer owner and current
// arena epoch. It is an ordinary production read, not diagnostic telemetry.
func (c *Core) DropCohortArenaIdentity() (uint64, uint64) {
	if c == nil {
		return 0, 0
	}
	return c.dropCohortOwner, c.dropCohortEpoch
}

// DropCohortProducerCounts returns authentic producer mutation counts.
func (c *Core) DropCohortProducerCounts() DropCohortProducerCounters {
	if c == nil {
		return DropCohortProducerCounters{}
	}
	return DropCohortProducerCounters{
		ReductionEstablishment: c.dropCohortProducerWrites[dropCohortProducerReductionEstablishment],
		LinearCanonicalizer:    c.dropCohortProducerWrites[dropCohortProducerLinearCanonicalizer],
		MappedCanonicalizer:    c.dropCohortProducerWrites[dropCohortProducerMappedCanonicalizer],
		SiblingAdoption:        c.dropCohortProducerWrites[dropCohortProducerSiblingAdoption],
		ConflictReconciliation: c.dropCohortProducerWrites[dropCohortProducerConflictReconciliation],
		DeadHistoryImport:      c.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport],
	}
}

func (c *Core) addDropCohortProducerWrite(kind int) {
	if kind < 0 || kind >= len(c.dropCohortProducerWrites) {
		return
	}
	if c.dropCohortProducerWrites[kind] != math.MaxUint64 {
		c.dropCohortProducerWrites[kind]++
	}
}

func (c *Core) recordDropCohortProducerMutation(kind DropCohortProducerMutation) error {
	var index int
	switch kind {
	case DropCohortProducerLinearCanonicalization:
		index = dropCohortProducerLinearCanonicalizer
	case DropCohortProducerMappedCanonicalization:
		index = dropCohortProducerMappedCanonicalizer
	case DropCohortProducerSiblingAdoption:
		index = dropCohortProducerSiblingAdoption
	case DropCohortProducerConflictReconciliation:
		index = dropCohortProducerConflictReconciliation
	case DropCohortProducerDeadHistoryImport:
		index = dropCohortProducerDeadHistoryImport
	default:
		return errors.New("parser-core phase zero: unknown drop-cohort producer mutation")
	}
	c.addDropCohortProducerWrite(index)
	return nil
}

// RecordDropCohortProducerMutation records one completed producer mutation.
// The token authenticates both the Core owner and the active scheduler frame.
func (c *Core) RecordDropCohortProducerMutation(owner SchedulerTransactionToken, kind DropCohortProducerMutation) (err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	err = c.recordDropCohortProducerMutation(kind)
	return c.finishSchedulerOwned(owner, err)
}

func (c *Core) dropCohortStoreBytes() uint64 {
	if c == nil {
		return 0
	}
	return uint64(len(c.dropCohortActions))*coreDropCohortActionBytes +
		uint64(len(c.dropCohortRecords))*coreDropCohortRecordBytes +
		uint64(len(c.dropCohortMembers))*uint64(coreDropCohortMemberBytes) +
		uint64(len(c.dropCohortDerivations))*coreDropCohortDerivationRecordBytes +
		uint64(len(c.dropCohortDerivationIntern))*uint64(coreDropCohortDerivationInternBytes) +
		uint64(len(c.dropCohortDerivationBytes)) +
		uint64(len(c.dropCohortCertificateRefs))*uint64(coreDropCohortRefBytes) +
		uint64(len(c.dropCohortMapStore))*uint64(coreDropCohortMapEntryBytes) +
		uint64(len(c.dropCohortJournalStore))*uint64(coreDropCohortJournalStoreBytes) +
		uint64(len(c.dropCohortFrontiers))*coreDropCohortFrontierRecordBytes +
		uint64(len(c.dropCohortFrontierParticipants))*coreDropCohortFrontierParticipantBytes +
		uint64(len(c.dropCohortFrontierMembers))*coreDropCohortFrontierMemberBytes +
		uint64(len(c.dropCohortFrontierJournal))*coreDropCohortFrontierMutationBytes +
		uint64(len(c.dropCohortReservations))*uint64(coreDropCohortReservationBytes) +
		uint64(len(c.dropCohortJournal))*uint64(coreDropCohortMutationBytes)
}

func dropCohortStoreElementBytes(index int) uint64 {
	switch index {
	case 0:
		return coreDropCohortActionBytes
	case 1:
		return coreDropCohortDerivationRecordBytes
	case 2:
		return coreDropCohortRefBytes
	case 3:
		return coreDropCohortMapEntryBytes
	case 4:
		return coreDropCohortDerivationInternBytes
	case 5:
		return coreDropCohortJournalStoreBytes
	case 6:
		return coreDropCohortRecordBytes
	default:
		return 0
	}
}

func (c *Core) dropCohortPhysicalStoreBytes() [7]uint64 {
	return [7]uint64{
		uint64(len(c.dropCohortActions)) * coreDropCohortActionBytes,
		uint64(len(c.dropCohortDerivations))*coreDropCohortDerivationRecordBytes + uint64(len(c.dropCohortDerivationBytes)),
		uint64(len(c.dropCohortCertificateRefs)) * coreDropCohortRefBytes,
		uint64(len(c.dropCohortMapStore)) * coreDropCohortMapEntryBytes,
		uint64(len(c.dropCohortDerivationIntern)) * coreDropCohortDerivationInternBytes,
		uint64(len(c.dropCohortJournalStore)) * coreDropCohortJournalStoreBytes,
		uint64(len(c.dropCohortRecords)) * coreDropCohortRecordBytes,
	}
}

func (c *Core) dropCohortRetainedStoreBytes() [7]uint64 {
	return [7]uint64{
		uint64(cap(c.dropCohortActions)) * coreDropCohortActionBytes,
		uint64(cap(c.dropCohortDerivations))*coreDropCohortDerivationRecordBytes + uint64(cap(c.dropCohortDerivationBytes)),
		uint64(cap(c.dropCohortCertificateRefs)) * coreDropCohortRefBytes,
		uint64(cap(c.dropCohortMapStore)) * coreDropCohortMapEntryBytes,
		uint64(cap(c.dropCohortDerivationIntern)) * coreDropCohortDerivationInternBytes,
		uint64(cap(c.dropCohortJournalStore)) * coreDropCohortJournalStoreBytes,
		uint64(cap(c.dropCohortRecords)) * coreDropCohortRecordBytes,
	}
}

func (c *Core) dropCohortRetainedOtherBytes() (uint64, error) {
	journal, err := dropCohortMulChecked(uint64(cap(c.dropCohortJournal)), coreDropCohortMutationBytes)
	if err != nil {
		return 0, err
	}
	reservations, err := dropCohortMulChecked(uint64(cap(c.dropCohortReservations)), coreDropCohortReservationBytes)
	if err != nil {
		return 0, err
	}
	frontiers, err := dropCohortMulChecked(uint64(cap(c.dropCohortFrontiers)), coreDropCohortFrontierRecordBytes)
	if err != nil {
		return 0, err
	}
	participants, err := dropCohortMulChecked(uint64(cap(c.dropCohortFrontierParticipants)), coreDropCohortFrontierParticipantBytes)
	if err != nil {
		return 0, err
	}
	members, err := dropCohortMulChecked(uint64(cap(c.dropCohortFrontierMembers)), coreDropCohortFrontierMemberBytes)
	if err != nil {
		return 0, err
	}
	frontierJournal, err := dropCohortMulChecked(uint64(cap(c.dropCohortFrontierJournal)), coreDropCohortFrontierMutationBytes)
	if err != nil {
		return 0, err
	}
	retained, err := dropCohortAddChecked(journal, reservations)
	if err != nil {
		return 0, err
	}
	retained, err = dropCohortAddChecked(retained, frontiers)
	if err != nil {
		return 0, err
	}
	retained, err = dropCohortAddChecked(retained, participants)
	if err != nil {
		return 0, err
	}
	retained, err = dropCohortAddChecked(retained, members)
	if err != nil {
		return 0, err
	}
	return dropCohortAddChecked(retained, frontierJournal)
}

func (c *Core) dropCohortStoreGrowthBytes(index int, demand, derivationBytes uint64) (uint64, error) {
	var live, capacity uint64
	switch index {
	case 0:
		live, capacity = uint64(len(c.dropCohortActions)), uint64(cap(c.dropCohortActions))
	case 1:
		var err error
		live, err = dropCohortMulChecked(uint64(len(c.dropCohortDerivations)), coreDropCohortDerivationRecordBytes)
		if err != nil {
			return 0, err
		}
		live, err = dropCohortAddChecked(live, uint64(len(c.dropCohortDerivationBytes)))
		if err != nil {
			return 0, err
		}
		capacity, err = dropCohortMulChecked(uint64(cap(c.dropCohortDerivations)), coreDropCohortDerivationRecordBytes)
		if err != nil {
			return 0, err
		}
		capacity, err = dropCohortAddChecked(capacity, uint64(cap(c.dropCohortDerivationBytes)))
		if err != nil {
			return 0, err
		}
	case 2:
		live, capacity = uint64(len(c.dropCohortCertificateRefs)), uint64(cap(c.dropCohortCertificateRefs))
	case 3:
		live, capacity = uint64(len(c.dropCohortMapStore)), uint64(cap(c.dropCohortMapStore))
	case 4:
		live, capacity = uint64(len(c.dropCohortDerivationIntern)), uint64(cap(c.dropCohortDerivationIntern))
	case 5:
		live, capacity = uint64(len(c.dropCohortJournalStore)), uint64(cap(c.dropCohortJournalStore))
	case 6:
		live, capacity = uint64(len(c.dropCohortRecords)), uint64(cap(c.dropCohortRecords))
	default:
		return 0, errors.New("parser-core phase zero: invalid drop-cohort store")
	}
	if index != 1 {
		var err error
		live, err = dropCohortMulChecked(live, dropCohortStoreElementBytes(index))
		if err != nil {
			return 0, err
		}
		capacity, err = dropCohortMulChecked(capacity, dropCohortStoreElementBytes(index))
		if err != nil {
			return 0, err
		}
	}
	requested, err := dropCohortMulChecked(demand, dropCohortStoreElementBytes(index))
	if err != nil {
		return 0, err
	}
	if index == 1 {
		requested, err = dropCohortAddChecked(requested, derivationBytes)
		if err != nil {
			return 0, err
		}
	}
	required, err := dropCohortAddChecked(live, requested)
	if err != nil {
		return 0, err
	}
	if required <= capacity {
		return 0, nil
	}
	return required - capacity, nil
}

func dropCohortAddChecked(left, right uint64) (uint64, error) {
	if right > math.MaxUint64-left {
		return 0, errors.New("parser-core phase zero: drop-cohort byte accounting overflow")
	}
	return left + right, nil
}

func dropCohortMulChecked(left, right uint64) (uint64, error) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, errors.New("parser-core phase zero: drop-cohort store demand overflow")
	}
	return left * right, nil
}

func (c *Core) dropCohortPreflightDemand(demand [7]uint64, derivationBytes, extraBytes uint64) error {
	used := c.dropCohortPhysicalStoreBytes()
	var totalUsed uint64
	for _, value := range used {
		var addErr error
		totalUsed, addErr = dropCohortAddChecked(totalUsed, value)
		if addErr != nil {
			return addErr
		}
	}
	other := c.dropCohortStoreBytes()
	if other >= totalUsed {
		other -= totalUsed
	} else {
		other = 0
	}
	total, err := dropCohortAddChecked(other, extraBytes)
	if err != nil {
		return err
	}
	for index := range demand {
		reserved, err := dropCohortMulChecked(c.dropCohortReserved[index], dropCohortStoreElementBytes(index))
		if err != nil {
			return err
		}
		requested, err := c.dropCohortStoreGrowthBytes(index, demand[index], derivationBytes)
		if err != nil {
			return err
		}
		perStore, err := dropCohortAddChecked(used[index], reserved)
		if err != nil {
			return err
		}
		if perStore > c.limits.MaxDropCohortBytes || requested > c.limits.MaxDropCohortBytes-perStore {
			return fmt.Errorf("parser-core phase zero: drop-cohort store %d byte cap", index)
		}
		var err2 error
		total, err2 = dropCohortAddChecked(total, used[index])
		if err2 != nil {
			return err2
		}
		total, err2 = dropCohortAddChecked(total, reserved)
		if err2 != nil {
			return err2
		}
		total, err2 = dropCohortAddChecked(total, requested)
		if err2 != nil {
			return err2
		}
	}
	var fixedReserved uint64
	for index, count := range c.dropCohortReserved {
		value, err := dropCohortMulChecked(count, dropCohortStoreElementBytes(index))
		if err != nil {
			return err
		}
		fixedReserved, err = dropCohortAddChecked(fixedReserved, value)
		if err != nil {
			return err
		}
	}
	if c.dropCohortReservedBytes < fixedReserved {
		return errors.New("parser-core phase zero: drop-cohort reservation accounting underflow")
	}
	total, err = dropCohortAddChecked(total, c.dropCohortReservedBytes-fixedReserved)
	if err != nil {
		return err
	}
	if total > c.limits.MaxDropCohortBytes {
		return fmt.Errorf("parser-core phase zero: drop-cohort aggregate byte cap (total=%d max=%d)", total, c.limits.MaxDropCohortBytes)
	}
	return nil
}

func (c *Core) dropCohortReserveDemandWithExtra(demand [7]uint64, derivationBytes, extraBytes uint64) (uint64, error) {
	if err := c.dropCohortPreflightDemand(demand, derivationBytes, extraBytes); err != nil {
		return 0, err
	}
	var bytes uint64
	var err error
	var nextReserved [7]uint64
	for index, count := range demand {
		value, err := dropCohortMulChecked(count, dropCohortStoreElementBytes(index))
		if err != nil {
			return 0, err
		}
		if index == 1 {
			value, err = dropCohortAddChecked(value, derivationBytes)
			if err != nil {
				return 0, err
			}
		}
		if count > math.MaxUint64-c.dropCohortReserved[index] {
			return 0, errors.New("parser-core phase zero: drop-cohort reservation count overflow")
		}
		nextReserved[index] = c.dropCohortReserved[index] + count
		bytes, err = dropCohortAddChecked(bytes, value)
		if err != nil {
			return 0, err
		}
	}
	bytes, err = dropCohortAddChecked(bytes, extraBytes)
	if err != nil {
		return 0, err
	}
	nextBytes, err := dropCohortAddChecked(c.dropCohortReservedBytes, bytes)
	if err != nil {
		return 0, err
	}
	c.dropCohortReserved = nextReserved
	c.dropCohortReservedBytes = nextBytes
	return bytes, nil
}

func (c *Core) dropCohortReserveDemand(demand [7]uint64, derivationBytes uint64) (uint64, error) {
	return c.dropCohortReserveDemandWithExtra(demand, derivationBytes, 0)
}

func (c *Core) dropCohortReleaseDemand(reserved [7]uint64, bytes uint64) {
	for index, count := range reserved {
		if count >= c.dropCohortReserved[index] {
			c.dropCohortReserved[index] = 0
		} else {
			c.dropCohortReserved[index] -= count
		}
	}
	if bytes >= c.dropCohortReservedBytes {
		c.dropCohortReservedBytes = 0
	} else {
		c.dropCohortReservedBytes -= bytes
	}
}

func (c *Core) dropCohortConsumeReservation(cohort DropCohortHandle, consumed [7]uint64, bytes uint64) {
	c.dropCohortReleaseDemand(consumed, bytes)
	for index := range c.dropCohortReservations {
		reservation := &c.dropCohortReservations[index]
		if reservation.handle != cohort {
			continue
		}
		for store, count := range consumed {
			if count >= reservation.reserved[store] {
				reservation.reserved[store] = 0
			} else {
				reservation.reserved[store] -= count
			}
		}
		if bytes >= reservation.reservationBytes {
			reservation.reservationBytes = 0
		} else {
			reservation.reservationBytes -= bytes
		}
		return
	}
}

func (c *Core) dropCohortReleaseReservationRemainder(cohort DropCohortHandle) {
	for index := range c.dropCohortReservations {
		reservation := &c.dropCohortReservations[index]
		if reservation.handle != cohort {
			continue
		}
		remaining := reservation.reserved
		bytes := reservation.reservationBytes
		c.dropCohortReleaseDemand(remaining, bytes)
		clear(reservation.reserved[:])
		reservation.reservationBytes = 0
		return
	}
}

func (c *Core) dropCohortCurrentReservation() *dropCohortReservation {
	for index := len(c.dropCohortReservations) - 1; index >= 0; index-- {
		reservation := &c.dropCohortReservations[index]
		for _, record := range c.dropCohortRecords {
			if record.handle == reservation.handle && record.state == DropCohortBuilding {
				return reservation
			}
		}
	}
	return nil
}

func dropCohortDemandBytes(demand [7]uint64, derivationBytes uint64) (uint64, error) {
	var total uint64
	for index, count := range demand {
		value, err := dropCohortMulChecked(count, dropCohortStoreElementBytes(index))
		if err != nil {
			return 0, err
		}
		if index == 1 {
			value, err = dropCohortAddChecked(value, derivationBytes)
			if err != nil {
				return 0, err
			}
		}
		total, err = dropCohortAddChecked(total, value)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (c *Core) dropCohortAppendJournalSlots(count int) error {
	if count < 0 || count > int(^uint(0)>>1)-len(c.dropCohortJournalStore) {
		return errors.New("parser-core phase zero: drop-cohort journal slot overflow")
	}
	needed := len(c.dropCohortJournalStore) + count
	if needed <= cap(c.dropCohortJournalStore) {
		c.dropCohortJournalStore = c.dropCohortJournalStore[:needed]
		return nil
	}
	grown := make([]dropCohortJournalStoreEntry, needed, needed)
	copy(grown, c.dropCohortJournalStore)
	c.dropCohortJournalStore = grown
	return nil
}

func (c *Core) dropCohortAppendJournalMutationSlots(count int) error {
	if count < 0 || count > int(^uint(0)>>1)-len(c.dropCohortJournal) {
		return errors.New("parser-core phase zero: drop-cohort journal mutation overflow")
	}
	needed := len(c.dropCohortJournal) + count
	if needed <= cap(c.dropCohortJournal) {
		return nil
	}
	grown := make([]dropCohortMutation, len(c.dropCohortJournal), needed)
	copy(grown, c.dropCohortJournal)
	c.dropCohortJournal = grown
	return nil
}

func (c *Core) dropCohortJournalGrowthBytes(count int) (uint64, error) {
	if count < 0 || count > int(^uint(0)>>1)-len(c.dropCohortJournal) {
		return 0, errors.New("parser-core phase zero: drop-cohort journal mutation overflow")
	}
	needed := len(c.dropCohortJournal) + count
	if needed <= cap(c.dropCohortJournal) {
		return 0, nil
	}
	return dropCohortMulChecked(uint64(needed-cap(c.dropCohortJournal)), coreDropCohortMutationBytes)
}

func (c *Core) dropCohortPreflight(expected int) error {
	if c == nil {
		return errors.New("parser-core phase zero: nil drop-cohort core")
	}
	if expected <= 0 {
		return errors.New("parser-core phase zero: empty drop-cohort output set")
	}
	if uint64(expected) > uint64(c.limits.MaxDropCohortMembers) {
		return errors.New("parser-core phase zero: drop-cohort member cap")
	}
	if expected > dropCohortRefHardCap {
		return errors.New("parser-core phase zero: drop-cohort reference hard cap")
	}
	if uint64(len(c.dropCohortRecords))+1 > uint64(c.limits.MaxDropCohorts) {
		return errors.New("parser-core phase zero: drop-cohort count cap")
	}
	if uint64(len(c.dropCohortMembers))+uint64(expected) > uint64(c.limits.MaxDropCohortRefs) {
		return errors.New("parser-core phase zero: drop-cohort member store cap")
	}
	if c.dropCohortNextSequence == math.MaxUint64 {
		return errors.New("parser-core phase zero: drop-cohort sequence overflow")
	}
	var demand [7]uint64
	demand[0], demand[1], demand[2], demand[3], demand[4] = 1, uint64(expected), uint64(expected), uint64(expected), uint64(expected)
	demand[5], demand[6] = uint64(expected+2), 1
	journalBytes, err := c.dropCohortJournalGrowthBytes(expected + 2)
	if err != nil {
		return err
	}
	return c.dropCohortPreflightDemand(demand, 0, journalBytes)
}

func (c *Core) beginDropCohortOwned(identity DropCohortActionIdentity, expected int) (DropCohortHandle, error) {
	if err := c.dropCohortPreflight(expected); err != nil {
		return DropCohortHandle{}, err
	}
	sequence := c.dropCohortNextSequence + 1
	if sequence == 0 {
		return DropCohortHandle{}, errors.New("parser-core phase zero: drop-cohort sequence wrapped")
	}
	handle := c.dropCohortHandle(sequence)
	var demand [7]uint64
	demand[0], demand[1], demand[2], demand[3], demand[4] = 1, uint64(expected), uint64(expected), uint64(expected), uint64(expected)
	demand[5], demand[6] = uint64(expected+2), 1
	journalBytes, err := c.dropCohortJournalGrowthBytes(expected + 2)
	if err != nil {
		return DropCohortHandle{}, err
	}
	reservationBytes, err := c.dropCohortReserveDemandWithExtra(demand, 0, journalBytes)
	if err != nil {
		return DropCohortHandle{}, err
	}
	actionIndex := uint32(len(c.dropCohortActions))
	oldActionsCap, oldRecordsCap := cap(c.dropCohortActions), cap(c.dropCohortRecords)
	oldMembersCap, oldDerivationsCap := cap(c.dropCohortMembers), cap(c.dropCohortDerivations)
	oldDerivationBytesCap, oldInternerCap := cap(c.dropCohortDerivationBytes), cap(c.dropCohortDerivationIntern)
	oldCertificateRefsCap, oldMapEntriesCap := cap(c.dropCohortCertificateRefs), cap(c.dropCohortMapStore)
	oldJournalEntriesCap := cap(c.dropCohortJournalStore)
	oldReservationsCap := cap(c.dropCohortReservations)
	oldActions, oldRecords := len(c.dropCohortActions), len(c.dropCohortRecords)
	oldMembers, oldDerivations := len(c.dropCohortMembers), len(c.dropCohortDerivations)
	oldDerivationBytes, oldInterner := len(c.dropCohortDerivationBytes), len(c.dropCohortDerivationIntern)
	oldCertificateRefs, oldMapEntries := len(c.dropCohortCertificateRefs), len(c.dropCohortMapStore)
	oldJournalEntries := len(c.dropCohortJournalStore)
	oldReservationsHeader := c.dropCohortReservations
	reservationsMoved := len(c.dropCohortReservations) == cap(c.dropCohortReservations)
	oldActionsHeader, oldRecordsHeader := c.dropCohortActions, c.dropCohortRecords
	oldMembersHeader, oldDerivationsHeader := c.dropCohortMembers, c.dropCohortDerivations
	oldDerivationBytesHeader, oldInternerHeader := c.dropCohortDerivationBytes, c.dropCohortDerivationIntern
	oldCertificateRefsHeader, oldMapEntriesHeader := c.dropCohortCertificateRefs, c.dropCohortMapStore
	oldJournalStoreHeader, oldJournalHeader := c.dropCohortJournalStore, c.dropCohortJournal
	c.dropCohortActions = append(c.dropCohortActions, identity)
	c.dropCohortRecords = append(c.dropCohortRecords, dropCohortRecord{
		handle: handle, state: DropCohortBuilding, expected: uint16(expected),
		actionIndex: actionIndex, memberStart: uint32(len(c.dropCohortMembers)),
	})
	journalStart := len(c.dropCohortJournalStore)
	if err := c.dropCohortAppendJournalSlots(int(expected) + 2); err != nil {
		c.dropCohortActions = oldActionsHeader
		c.dropCohortRecords = oldRecordsHeader
		c.dropCohortReleaseDemand(demand, reservationBytes)
		return DropCohortHandle{}, err
	}
	if err := c.dropCohortAppendJournalMutationSlots(expected + 2); err != nil {
		c.dropCohortActions = oldActionsHeader
		c.dropCohortRecords = oldRecordsHeader
		c.dropCohortJournalStore = oldJournalStoreHeader
		c.dropCohortJournal = oldJournalHeader
		c.dropCohortReleaseDemand(demand, reservationBytes)
		return DropCohortHandle{}, err
	}
	c.dropCohortNextSequence = sequence
	var consumed [7]uint64
	consumed[0], consumed[5], consumed[6] = 1, uint64(expected+2), 1
	consumedBytes, _ := dropCohortDemandBytes(consumed, 0)
	consumedBytes, _ = dropCohortAddChecked(consumedBytes, journalBytes)
	c.dropCohortReservations = append(c.dropCohortReservations, dropCohortReservation{
		handle: handle, previousNext: sequence - 1,
		actions: oldActions, records: oldRecords, members: oldMembers, derivations: oldDerivations,
		derivationBytes: oldDerivationBytes, interner: oldInterner,
		certificateRefs: oldCertificateRefs, mapEntries: oldMapEntries,
		journalEntries: oldJournalEntries, journalMutations: len(c.dropCohortJournal),
		ownerChecks: c.dropCohortOwnerCheckedLookups,
		actionsCap:  oldActionsCap, recordsCap: oldRecordsCap, membersCap: oldMembersCap,
		derivationsCap: oldDerivationsCap, derivationBytesCap: oldDerivationBytesCap,
		internerCap: oldInternerCap, certificateRefsCap: oldCertificateRefsCap,
		mapEntriesCap: oldMapEntriesCap, journalEntriesCap: oldJournalEntriesCap,
		reservationsCap: oldReservationsCap,
		reserved:        demand, reservationBytes: reservationBytes,
		journalStart: journalStart, journalCount: expected + 2,
		actionsHeader: oldActionsHeader, recordsHeader: oldRecordsHeader,
		membersHeader: oldMembersHeader, derivationsHeader: oldDerivationsHeader,
		derivationBytesHead: oldDerivationBytesHeader, internerHeader: oldInternerHeader,
		certificateRefsHead: oldCertificateRefsHeader, mapEntriesHeader: oldMapEntriesHeader,
		journalStoreHeader: oldJournalStoreHeader, journalHeader: oldJournalHeader,
		reservationsHeader: func() []dropCohortReservation {
			if reservationsMoved {
				return oldReservationsHeader
			}
			return nil
		}(),
		reservationsMoved: reservationsMoved,
	})
	c.dropCohortConsumeReservation(handle, consumed, consumedBytes)
	return handle, nil
}

// BeginDropCohortOwned reserves one atomic cohort for a stable output set.
func (c *Core) BeginDropCohortOwned(owner SchedulerTransactionToken, identity DropCohortActionIdentity, expected int) (handle DropCohortHandle, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return DropCohortHandle{}, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	handle, err = c.beginDropCohortOwned(identity, expected)
	err = c.finishSchedulerOwned(owner, err)
	return handle, err
}

// AbandonDropCohortOwned removes the latest incomplete cohort after an
// optional derivation producer reaches a bounded-store limit. The operation
// keeps the authenticated scheduler frame usable for the ordinary fallback.
func (c *Core) AbandonDropCohortOwned(owner SchedulerTransactionToken, cohort DropCohortHandle) (err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if len(c.dropCohortReservations) == 0 {
		return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: drop-cohort reservation is unavailable"))
	}
	reservation := c.dropCohortReservations[len(c.dropCohortReservations)-1]
	if reservation.handle != cohort {
		return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: drop-cohort abandonment is not the latest reservation"))
	}
	if index, identityErr := c.validateDropCohortIdentity(cohort); identityErr != nil {
		return c.finishSchedulerOwned(owner, identityErr)
	} else if c.dropCohortRecords[index].state != DropCohortBuilding {
		return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: drop-cohort is not building"))
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

const (
	dropCohortDerivationFormatVersion  uint32 = 2
	dropCohortTagRecordBegin                  = 0xD2
	dropCohortTagRecordEnd                    = 0xD3
	dropCohortTagPathBegin                    = 0xB0
	dropCohortTagPathEnd                      = 0xB1
	dropCohortTagBoundary                     = 0xA0
	dropCohortTagEdge                         = 0xA1
	dropCohortTagRecoveryDiscontinuity        = 0xA2
	dropCohortTagSubtreeBegin                 = 0xC0
	dropCohortTagSubtreeEnd                   = 0xC1
)

// dropCohortPathStep keeps arena identities only while traversing one graph.
// The step is never copied into derivation bytes.
type dropCohortPathStep struct {
	node       NodeID
	payload    SubtreeID
	scoreDelta int64
	order      ForkOrder
}

type dropCohortEncoder struct {
	core  *Core
	dst   []byte
	limit uint64
}

func (c *Core) dropCohortEphemeralReserve(bytes uint64) error {
	if c == nil {
		return errors.New("parser-core phase zero: nil drop-cohort scratch owner")
	}
	if c.dropCohortEphemeralBytes > c.limits.MaxDropCohortBytes ||
		bytes > c.limits.MaxDropCohortBytes-c.dropCohortEphemeralBytes {
		return errors.New("parser-core phase zero: drop-cohort ephemeral scratch byte cap")
	}
	c.dropCohortEphemeralBytes += bytes
	if c.dropCohortEphemeralBytes > c.dropCohortEphemeralPeak {
		c.dropCohortEphemeralPeak = c.dropCohortEphemeralBytes
	}
	return nil
}

func (c *Core) dropCohortEphemeralRelease(bytes uint64) {
	if c == nil || bytes == 0 {
		return
	}
	if bytes >= c.dropCohortEphemeralBytes {
		c.dropCohortEphemeralBytes = 0
		return
	}
	c.dropCohortEphemeralBytes -= bytes
}

func dropCohortScratchBytes(capacity uint64, elementBytes uint64) (uint64, error) {
	if elementBytes != 0 && capacity > math.MaxUint64/elementBytes {
		return 0, errors.New("parser-core phase zero: drop-cohort scratch byte overflow")
	}
	return capacity * elementBytes, nil
}

func (e *dropCohortEncoder) reserve(count int) error {
	if count < 0 {
		return errors.New("parser-core phase zero: negative drop-cohort encoding size")
	}
	if uint64(len(e.dst)) > e.limit || uint64(count) > e.limit-uint64(len(e.dst)) {
		return errors.New("parser-core phase zero: drop-cohort derivation byte cap")
	}
	needed := len(e.dst) + count
	if needed <= cap(e.dst) {
		return nil
	}
	maxInt := int(^uint(0) >> 1)
	if needed > maxInt {
		return errors.New("parser-core phase zero: drop-cohort derivation length overflow")
	}
	newCap := cap(e.dst)
	if newCap < 64 {
		newCap = 64
	}
	for newCap < needed {
		if newCap > maxInt/2 {
			newCap = needed
			break
		}
		newCap *= 2
	}
	if uint64(newCap) > e.limit {
		newCap = int(e.limit)
	}
	if newCap < needed {
		return errors.New("parser-core phase zero: drop-cohort derivation byte cap")
	}
	if e.core == nil {
		return errors.New("parser-core phase zero: drop-cohort encoder has no scratch owner")
	}
	oldBytes := uint64(cap(e.dst))
	if err := e.core.dropCohortEphemeralReserve(uint64(newCap)); err != nil {
		return err
	}
	grown := make([]byte, len(e.dst), newCap)
	copy(grown, e.dst)
	e.core.dropCohortEphemeralRelease(oldBytes)
	e.dst = grown
	return nil
}

func (e *dropCohortEncoder) bytes(value []byte) error {
	if err := e.reserve(len(value)); err != nil {
		return err
	}
	start := len(e.dst)
	e.dst = e.dst[:start+len(value)]
	copy(e.dst[start:], value)
	return nil
}

func (e *dropCohortEncoder) u8(value byte) error {
	return e.bytes([]byte{value})
}

func (e *dropCohortEncoder) bool(value bool) error {
	if value {
		return e.u8(1)
	}
	return e.u8(0)
}

func (e *dropCohortEncoder) u16(value uint16) error {
	var encoded [2]byte
	binary.LittleEndian.PutUint16(encoded[:], value)
	return e.bytes(encoded[:])
}

func (e *dropCohortEncoder) u32(value uint32) error {
	var encoded [4]byte
	binary.LittleEndian.PutUint32(encoded[:], value)
	return e.bytes(encoded[:])
}

func (e *dropCohortEncoder) u64(value uint64) error {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], value)
	return e.bytes(encoded[:])
}

func (e *dropCohortEncoder) i16(value int16) error { return e.u16(uint16(value)) }
func (e *dropCohortEncoder) i64(value int64) error { return e.u64(uint64(value)) }

func (e *dropCohortEncoder) patchU32(offset int, value uint32) error {
	if offset < 0 || offset+4 > len(e.dst) {
		return errors.New("parser-core phase zero: drop-cohort count patch is out of range")
	}
	binary.LittleEndian.PutUint32(e.dst[offset:offset+4], value)
	return nil
}

func (c *Core) dropCohortCheckpointDigest(id CheckpointID) (digest [32]byte, exact bool) {
	_, digest, exact = c.checkpoints.receipt(id)
	return digest, exact
}

func (c *Core) dropCohortEncodeCheckpoint(e *dropCohortEncoder, checkpoint DropCohortSourceCheckpoint) error {
	for _, value := range [...]uint32{
		checkpoint.StartByte, checkpoint.EndByte, checkpoint.StartRow, checkpoint.StartColumn,
		checkpoint.EndRow, checkpoint.EndColumn,
	} {
		if err := e.u32(value); err != nil {
			return err
		}
	}
	for _, id := range [...]CheckpointID{checkpoint.ScannerStart, checkpoint.ScannerEnd} {
		digest, exact := c.dropCohortCheckpointDigest(id)
		if err := e.bool(exact); err != nil {
			return err
		}
		if err := e.bytes(digest[:]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Core) dropCohortEncodeBoundary(e *dropCohortEncoder, id NodeID) error {
	node, err := c.node(id)
	if err != nil {
		return err
	}
	checkpoint, exact := c.nodeScannerCheckpoint(id)
	if err := e.u8(dropCohortTagBoundary); err != nil {
		return err
	}
	if err := e.u32(uint32(node.state)); err != nil {
		return err
	}
	if err := e.u32(node.byteOffset); err != nil {
		return err
	}
	if err := e.bool(exact); err != nil {
		return err
	}
	digest, digestExact := c.dropCohortCheckpointDigest(checkpoint)
	if exact && !digestExact {
		return errors.New("parser-core phase zero: drop-cohort boundary checkpoint is unavailable")
	}
	if err := e.bool(digestExact); err != nil {
		return err
	}
	return e.bytes(digest[:])
}

func dropCohortSliceBounds(first, count uint32, length int) (int, int, error) {
	if uint64(first)+uint64(count) > uint64(length) {
		return 0, 0, errors.New("parser-core phase zero: drop-cohort subtree metadata range is invalid")
	}
	return int(first), int(first + count), nil
}

func (c *Core) dropCohortEncodeSubtree(e *dropCohortEncoder, payload SubtreeID, depth uint64) (Symbol, error) {
	if _, opaque := c.reusedSubtree(payload); opaque {
		return 0, errors.New("parser-core phase zero: drop-cohort encoding cannot inspect a reused subtree")
	}
	if depth >= uint64(c.limits.MaxSubtrees) {
		return 0, errors.New("parser-core phase zero: drop-cohort subtree depth cap")
	}
	record, err := c.subtree(payload)
	if err != nil {
		return 0, err
	}
	childStart, childEnd, err := dropCohortSliceBounds(record.firstChild, record.childCount, len(c.children))
	if err != nil {
		return 0, err
	}
	fieldStart, fieldEnd, err := dropCohortSliceBounds(record.firstField, record.fieldCount, len(c.fields))
	if err != nil {
		return 0, err
	}
	aliasStart, aliasEnd, err := dropCohortSliceBounds(record.firstAlias, record.aliasCount, len(c.aliases))
	if err != nil {
		return 0, err
	}
	if err := e.u8(dropCohortTagSubtreeBegin); err != nil {
		return 0, err
	}
	for _, value := range []uint16{uint16(record.symbol), record.productionID} {
		if err := e.u16(value); err != nil {
			return 0, err
		}
	}
	if err := e.i16(record.dynamicPrecedence); err != nil {
		return 0, err
	}
	for _, value := range []uint32{record.startByte, record.endByte} {
		if err := e.u32(value); err != nil {
			return 0, err
		}
	}
	// record.missing is deliberately NOT encoded here, and that is a scoping
	// decision with a cost, not an oversight.
	//
	// This encoder feeds a receipt digest that is LOCKED by hardcoded digest
	// and length literals (the D6b receipt pinned in the root package's
	// grammargen-LR frontier test). Appending a byte per subtree re-derives
	// every such receipt, which is an evidence change that belongs to a
	// deliberate tranche with its own review, not to a substrate change that
	// no live path can even reach.
	//
	// Safe today, for one narrow reason only: no live parser path publishes a
	// MISSING record, so this encoder cannot meet one and every digest it
	// produces is byte-identical to before the bit existed. The sibling
	// comparator below DOES order on the bit, because ordering feeds no
	// digest.
	//
	// PRECONDITION FOR STAGE S5: the first live path that publishes a MISSING
	// record must add the bit here AND re-derive the locked receipts in the
	// same change. Until both happen together, a receipt over a MISSING
	// payload would hash identically to one over the clean payload it
	// replaced, so the receipt would stop authenticating what was dropped.
	for _, value := range []bool{record.extra, record.external, record.terminal, record.fragile} {
		if err := e.bool(value); err != nil {
			return 0, err
		}
	}
	if record.external {
		provenance, exact := c.externalPayloadScannerProvenance(payload)
		if err := e.bool(exact); err != nil {
			return 0, err
		}
		for _, id := range [...]CheckpointID{provenance.start, provenance.end} {
			digest, digestExact := c.dropCohortCheckpointDigest(id)
			if exact && !digestExact {
				return 0, errors.New("parser-core phase zero: external subtree checkpoint is unavailable")
			}
			if err := e.bool(digestExact); err != nil {
				return 0, err
			}
			if err := e.bytes(digest[:]); err != nil {
				return 0, err
			}
		}
	}
	if err := e.u32(record.childCount); err != nil {
		return 0, err
	}
	for _, child := range c.children[childStart:childEnd] {
		if child == 0 || child >= payload {
			return 0, errors.New("parser-core phase zero: drop-cohort subtree child order is invalid")
		}
		if _, err := c.dropCohortEncodeSubtree(e, child, depth+1); err != nil {
			return 0, err
		}
	}
	if err := e.u32(record.fieldCount); err != nil {
		return 0, err
	}
	for _, field := range c.fields[fieldStart:fieldEnd] {
		if err := e.u16(uint16(field.FieldID)); err != nil {
			return 0, err
		}
		if err := e.u8(field.ChildIndex); err != nil {
			return 0, err
		}
		if err := e.bool(field.Inherited); err != nil {
			return 0, err
		}
	}
	if err := e.u32(record.aliasCount); err != nil {
		return 0, err
	}
	for _, alias := range c.aliases[aliasStart:aliasEnd] {
		if err := e.u16(uint16(alias)); err != nil {
			return 0, err
		}
	}
	if err := e.u8(dropCohortTagSubtreeEnd); err != nil {
		return 0, err
	}
	return record.symbol, nil
}

func (c *Core) dropCohortCompareCheckpoint(left, right CheckpointID) (int, error) {
	leftDigest, leftExact := c.dropCohortCheckpointDigest(left)
	rightDigest, rightExact := c.dropCohortCheckpointDigest(right)
	if leftExact != rightExact {
		if !leftExact {
			return -1, nil
		}
		return 1, nil
	}
	return bytesCompare(leftDigest[:], rightDigest[:]), nil
}

func (c *Core) dropCohortCompareSubtree(left, right SubtreeID) (int, error) {
	if left == right {
		return 0, nil
	}
	leftReuse, leftOpaque := c.reusedSubtree(left)
	rightReuse, rightOpaque := c.reusedSubtree(right)
	if leftOpaque || rightOpaque {
		if leftOpaque && rightOpaque && leftReuse == rightReuse {
			return 0, nil
		}
		return 0, errors.New("parser-core phase zero: drop-cohort comparison cannot inspect a reused subtree")
	}
	l, err := c.subtree(left)
	if err != nil {
		return 0, err
	}
	r, err := c.subtree(right)
	if err != nil {
		return 0, err
	}
	compare := func(a, b int64) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	for _, pair := range [][2]int64{
		{int64(l.symbol), int64(r.symbol)}, {int64(l.productionID), int64(r.productionID)},
		{int64(l.dynamicPrecedence), int64(r.dynamicPrecedence)}, {int64(l.startByte), int64(r.startByte)},
		{int64(l.endByte), int64(r.endByte)}, {int64(l.childCount), int64(r.childCount)},
		{int64(l.fieldCount), int64(r.fieldCount)}, {int64(l.aliasCount), int64(r.aliasCount)},
	} {
		if result := compare(pair[0], pair[1]); result != 0 {
			return result, nil
		}
	}
	for _, pair := range [][2]bool{{l.extra, r.extra}, {l.external, r.external}, {l.terminal, r.terminal}, {l.fragile, r.fragile}, {l.missing, r.missing}} {
		if pair[0] != pair[1] {
			if !pair[0] {
				return -1, nil
			}
			return 1, nil
		}
	}
	if l.external {
		lp, lok := c.externalPayloadScannerProvenance(left)
		rp, rok := c.externalPayloadScannerProvenance(right)
		if lok != rok {
			if !lok {
				return -1, nil
			}
			return 1, nil
		}
		if lok {
			for _, pair := range [...]struct{ left, right CheckpointID }{{lp.start, rp.start}, {lp.end, rp.end}} {
				result, err := c.dropCohortCompareCheckpoint(pair.left, pair.right)
				if err != nil || result != 0 {
					return result, err
				}
			}
		}
	}
	leftChildStart, leftChildEnd, err := dropCohortSliceBounds(l.firstChild, l.childCount, len(c.children))
	if err != nil {
		return 0, err
	}
	rightChildStart, rightChildEnd, err := dropCohortSliceBounds(r.firstChild, r.childCount, len(c.children))
	if err != nil {
		return 0, err
	}
	leftChildren := c.children[leftChildStart:leftChildEnd]
	rightChildren := c.children[rightChildStart:rightChildEnd]
	for index := range leftChildren {
		if leftChildren[index] == 0 || leftChildren[index] >= left || rightChildren[index] == 0 || rightChildren[index] >= right {
			return 0, errors.New("parser-core phase zero: drop-cohort subtree child order is invalid")
		}
		result, err := c.dropCohortCompareSubtree(leftChildren[index], rightChildren[index])
		if err != nil || result != 0 {
			return result, err
		}
	}
	leftFieldStart, leftFieldEnd, err := dropCohortSliceBounds(l.firstField, l.fieldCount, len(c.fields))
	if err != nil {
		return 0, err
	}
	rightFieldStart, _, err := dropCohortSliceBounds(r.firstField, r.fieldCount, len(c.fields))
	if err != nil {
		return 0, err
	}
	for index, field := range c.fields[leftFieldStart:leftFieldEnd] {
		other := c.fields[rightFieldStart+index]
		if field.FieldID != other.FieldID || field.ChildIndex != other.ChildIndex || field.Inherited != other.Inherited {
			if field.FieldID != other.FieldID {
				if field.FieldID < other.FieldID {
					return -1, nil
				}
				return 1, nil
			}
			if field.ChildIndex < other.ChildIndex {
				return -1, nil
			}
			if field.ChildIndex > other.ChildIndex {
				return 1, nil
			}
			if !field.Inherited {
				return -1, nil
			}
			return 1, nil
		}
	}
	leftAliasStart, leftAliasEnd, err := dropCohortSliceBounds(l.firstAlias, l.aliasCount, len(c.aliases))
	if err != nil {
		return 0, err
	}
	rightAliasStart, _, err := dropCohortSliceBounds(r.firstAlias, r.aliasCount, len(c.aliases))
	if err != nil {
		return 0, err
	}
	for index, alias := range c.aliases[leftAliasStart:leftAliasEnd] {
		other := c.aliases[rightAliasStart+index]
		if alias != other {
			if alias < other {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func (c *Core) dropCohortCompareBoundary(left, right NodeID) (int, error) {
	if left == right {
		return 0, nil
	}
	l, err := c.node(left)
	if err != nil {
		return 0, err
	}
	r, err := c.node(right)
	if err != nil {
		return 0, err
	}
	if l.state != r.state {
		if l.state < r.state {
			return -1, nil
		}
		return 1, nil
	}
	if l.byteOffset != r.byteOffset {
		if l.byteOffset < r.byteOffset {
			return -1, nil
		}
		return 1, nil
	}
	leftCheckpoint, leftExact := c.nodeScannerCheckpoint(left)
	rightCheckpoint, rightExact := c.nodeScannerCheckpoint(right)
	if leftExact != rightExact {
		if !leftExact {
			return -1, nil
		}
		return 1, nil
	}
	if result, err := c.dropCohortCompareCheckpoint(leftCheckpoint, rightCheckpoint); err != nil || result != 0 {
		return result, err
	}
	leftLinks, err := c.dropCohortSortedLinks(left)
	if err != nil {
		return 0, err
	}
	defer c.dropCohortReleaseSortedLinks(leftLinks)
	rightLinks, err := c.dropCohortSortedLinks(right)
	if err != nil {
		return 0, err
	}
	defer c.dropCohortReleaseSortedLinks(rightLinks)
	if len(leftLinks.links) != len(rightLinks.links) {
		if len(leftLinks.links) < len(rightLinks.links) {
			return -1, nil
		}
		return 1, nil
	}
	for index := range leftLinks.links {
		result, err := c.dropCohortCompareLinks(leftLinks.links[index], rightLinks.links[index])
		if err != nil || result != 0 {
			return result, err
		}
	}
	return 0, nil
}

func (c *Core) dropCohortCompareLinks(left, right linkRecord) (int, error) {
	if err := left.validateShape(); err != nil {
		return 0, err
	}
	if err := right.validateShape(); err != nil {
		return 0, err
	}
	if left.isRecoveryDiscontinuity() != right.isRecoveryDiscontinuity() {
		if !left.isRecoveryDiscontinuity() {
			return -1, nil
		}
		return 1, nil
	}
	if left.isRecoveryDiscontinuity() {
		return c.dropCohortCompareBoundary(left.prev, right.prev)
	}
	if left.scoreDelta != right.scoreDelta {
		if left.scoreDelta < right.scoreDelta {
			return -1, nil
		}
		return 1, nil
	}
	if left.hasOrder() != right.hasOrder() {
		if !left.hasOrder() {
			return -1, nil
		}
		return 1, nil
	}
	if left.order != right.order {
		if left.order < right.order {
			return -1, nil
		}
		return 1, nil
	}
	if result, err := c.dropCohortCompareSubtree(left.payload, right.payload); err != nil || result != 0 {
		return result, err
	}
	return c.dropCohortCompareBoundary(left.prev, right.prev)
}

type dropCohortSortedLinksResult struct {
	links     []linkRecord
	byteCount uint64
}

func (c *Core) dropCohortGrowLinkScratch(links []linkRecord, needed int) ([]linkRecord, error) {
	if needed <= cap(links) {
		return links[:needed], nil
	}
	maxInt := int(^uint(0) >> 1)
	newCap := cap(links)
	if newCap < 1 {
		newCap = 1
	}
	for newCap < needed {
		if newCap > maxInt/2 {
			newCap = needed
			break
		}
		newCap *= 2
	}
	oldBytes, err := dropCohortScratchBytes(uint64(cap(links)), coreLinkRecordBytes)
	if err != nil {
		return nil, err
	}
	newBytes, err := dropCohortScratchBytes(uint64(newCap), coreLinkRecordBytes)
	if err != nil {
		return nil, err
	}
	if err := c.dropCohortEphemeralReserve(newBytes); err != nil {
		return nil, err
	}
	grown := make([]linkRecord, len(links), newCap)
	copy(grown, links)
	c.dropCohortEphemeralRelease(oldBytes)
	return grown[:needed], nil
}

func (c *Core) dropCohortGrowSeenScratch(seen []LinkID, needed int) ([]LinkID, error) {
	if needed <= cap(seen) {
		return seen[:needed], nil
	}
	maxInt := int(^uint(0) >> 1)
	newCap := cap(seen)
	if newCap < 1 {
		newCap = 1
	}
	for newCap < needed {
		if newCap > maxInt/2 {
			newCap = needed
			break
		}
		newCap *= 2
	}
	oldBytes, err := dropCohortScratchBytes(uint64(cap(seen)), coreUint32Bytes)
	if err != nil {
		return nil, err
	}
	newBytes, err := dropCohortScratchBytes(uint64(newCap), coreUint32Bytes)
	if err != nil {
		return nil, err
	}
	if err := c.dropCohortEphemeralReserve(newBytes); err != nil {
		return nil, err
	}
	grown := make([]LinkID, len(seen), newCap)
	copy(grown, seen)
	c.dropCohortEphemeralRelease(oldBytes)
	return grown[:needed], nil
}

func (c *Core) dropCohortReleaseSortedLinks(result dropCohortSortedLinksResult) {
	c.dropCohortEphemeralRelease(result.byteCount)
}

func (c *Core) dropCohortSortedLinks(id NodeID) (result dropCohortSortedLinksResult, err error) {
	node, err := c.node(id)
	if err != nil {
		return result, err
	}
	var links []linkRecord
	var seen []LinkID
	releaseAll := func() {
		linkBytes, linkErr := dropCohortScratchBytes(uint64(cap(links)), coreLinkRecordBytes)
		if linkErr == nil {
			c.dropCohortEphemeralRelease(linkBytes)
		}
		seenBytes, seenErr := dropCohortScratchBytes(uint64(cap(seen)), coreUint32Bytes)
		if seenErr == nil {
			c.dropCohortEphemeralRelease(seenBytes)
		}
	}
	defer func() {
		if err != nil {
			releaseAll()
		}
	}()
	linkID := LinkID(node.firstLink)
	for linkID != 0 {
		if uint64(linkID) > uint64(len(c.links)) {
			return result, errors.New("parser-core phase zero: drop-cohort derivation adjacency out of range")
		}
		for _, prior := range seen {
			if prior == linkID {
				return result, errors.New("parser-core phase zero: drop-cohort derivation adjacency cycle")
			}
		}
		if uint64(len(seen))+1 > uint64(c.limits.MaxLinksPerBoundary) {
			return result, errors.New("parser-core phase zero: drop-cohort derivation link cap")
		}
		seen, err = c.dropCohortGrowSeenScratch(seen, len(seen)+1)
		if err != nil {
			return result, err
		}
		seen[len(seen)-1] = linkID
		link := c.links[linkID-1]
		if err := link.validateShape(); err != nil {
			return result, err
		}
		if link.prev == 0 || link.prev >= id || uint64(link.next) > uint64(len(c.links)) {
			return result, errors.New("parser-core phase zero: drop-cohort derivation graph is invalid")
		}
		links, err = c.dropCohortGrowLinkScratch(links, len(links)+1)
		if err != nil {
			return result, err
		}
		links[len(links)-1] = link
		linkID = link.next
	}
	for index := 1; index < len(links); index++ {
		current := links[index]
		position := index
		for position > 0 {
			compareResult, compareErr := c.dropCohortCompareLinks(current, links[position-1])
			if compareErr != nil {
				err = compareErr
				return result, err
			}
			if compareResult >= 0 {
				break
			}
			links[position] = links[position-1]
			position--
		}
		links[position] = current
	}
	seenBytes, seenErr := dropCohortScratchBytes(uint64(cap(seen)), coreUint32Bytes)
	if seenErr != nil {
		err = seenErr
		return result, err
	}
	c.dropCohortEphemeralRelease(seenBytes)
	seen = nil
	result = dropCohortSortedLinksResult{links: links}
	result.byteCount, err = dropCohortScratchBytes(uint64(cap(links)), coreLinkRecordBytes)
	if err != nil {
		c.dropCohortEphemeralRelease(result.byteCount)
		return dropCohortSortedLinksResult{}, err
	}
	return result, nil
}

func (c *Core) dropCohortAppendPathStep(path []dropCohortPathStep, step dropCohortPathStep) ([]dropCohortPathStep, error) {
	stepBytes := coreDropCohortPathStepBytes
	if stepBytes == 0 || uint64(len(path)) >= uint64(c.limits.MaxSubtrees) ||
		uint64(len(path))+1 > c.limits.MaxDropCohortBytes/stepBytes {
		return path, errors.New("parser-core phase zero: drop-cohort path scratch byte cap")
	}
	needed := len(path) + 1
	if needed > cap(path) {
		maxInt := int(^uint(0) >> 1)
		newCap := cap(path)
		if newCap < 1 {
			newCap = 1
		}
		for newCap < needed {
			if newCap > maxInt/2 {
				newCap = needed
				break
			}
			newCap *= 2
		}
		maxSteps := c.limits.MaxDropCohortBytes / stepBytes
		if uint64(newCap) > maxSteps {
			newCap = int(maxSteps)
		}
		if newCap < needed {
			return path, errors.New("parser-core phase zero: drop-cohort path scratch byte cap")
		}
		oldBytes, err := dropCohortScratchBytes(uint64(cap(path)), stepBytes)
		if err != nil {
			return path, err
		}
		newBytes, err := dropCohortScratchBytes(uint64(newCap), stepBytes)
		if err != nil {
			return path, err
		}
		if err := c.dropCohortEphemeralReserve(newBytes); err != nil {
			return path, err
		}
		grown := make([]dropCohortPathStep, len(path), newCap)
		copy(grown, path)
		c.dropCohortEphemeralRelease(oldBytes)
		c.dropCohortPathScratch = grown
		path = grown
	}
	return append(path, step), nil
}

// dropCohortEncodeAllPaths returns the authoritative path slice after every
// recursive call, so a deeper capacity growth remains owned by all ancestors.
func (c *Core) dropCohortEncodeAllPaths(e *dropCohortEncoder, id NodeID, path []dropCohortPathStep, score int64, order ForkOrder, count *uint32, maxDepth *uint32, root *Symbol, rootSet *bool) ([]dropCohortPathStep, error) {
	links, err := c.dropCohortSortedLinks(id)
	if err != nil {
		return path, err
	}
	defer c.dropCohortReleaseSortedLinks(links)
	if len(links.links) == 0 {
		if uint64(*count) >= c.limits.MaxDerivations || *count == math.MaxUint32 {
			return path, ErrDerivationEnumerationCap
		}
		if err := e.u8(dropCohortTagPathBegin); err != nil {
			return path, err
		}
		if err := e.i64(score); err != nil {
			return path, err
		}
		if err := e.u64(order.Value); err != nil {
			return path, err
		}
		if err := e.bool(order.Present); err != nil {
			return path, err
		}
		if uint64(len(path)) > uint64(math.MaxUint32) {
			return path, errors.New("parser-core phase zero: drop-cohort path length overflow")
		}
		if err := e.u32(uint32(len(path))); err != nil {
			return path, err
		}
		if err := c.dropCohortEncodeBoundary(e, id); err != nil {
			return path, err
		}
		for index := len(path) - 1; index >= 0; index-- {
			step := path[index]
			if step.payload == 0 {
				if err := e.u8(dropCohortTagRecoveryDiscontinuity); err != nil {
					return path, err
				}
				if err := c.dropCohortEncodeBoundary(e, step.node); err != nil {
					return path, err
				}
				continue
			}
			if err := e.u8(dropCohortTagEdge); err != nil {
				return path, err
			}
			if err := e.i64(step.scoreDelta); err != nil {
				return path, err
			}
			if err := e.u64(step.order.Value); err != nil {
				return path, err
			}
			if err := e.bool(step.order.Present); err != nil {
				return path, err
			}
			encodedRoot, encodeErr := c.dropCohortEncodeSubtree(e, step.payload, 0)
			if encodeErr != nil {
				return path, encodeErr
			}
			if !*rootSet {
				*root = encodedRoot
				*rootSet = true
			}
			if err := c.dropCohortEncodeBoundary(e, step.node); err != nil {
				return path, err
			}
		}
		if err := e.u8(dropCohortTagPathEnd); err != nil {
			return path, err
		}
		(*count)++
		if uint32(len(path)) > *maxDepth {
			*maxDepth = uint32(len(path))
		}
		return path, nil
	}
	for _, link := range links.links {
		if err := link.validateShape(); err != nil {
			return path, err
		}
		childOrder := order
		if link.hasOrder() {
			childOrder = ForkOrder{Value: link.order, Present: true}
		}
		childScore, scoreErr := checkedAddScore(score, link.scoreDelta)
		if scoreErr != nil {
			return path, scoreErr
		}
		var appendErr error
		path, appendErr = c.dropCohortAppendPathStep(path, dropCohortPathStep{
			node: id, payload: link.payload, scoreDelta: link.scoreDelta, order: childOrder,
		})
		if appendErr != nil {
			return path, appendErr
		}
		path, err = c.dropCohortEncodeAllPaths(e, link.prev, path, childScore, childOrder, count, maxDepth, root, rootSet)
		if err != nil {
			return path, err
		}
		path = path[:len(path)-1]
	}
	return path, nil
}

func (c *Core) buildDropCohortDerivationOwned(head Head, checkpoint DropCohortSourceCheckpoint) (DropCohortDerivationHandle, error) {
	if c == nil {
		return DropCohortDerivationHandle{}, errors.New("parser-core phase zero: nil drop-cohort core")
	}
	c.dropCohortEphemeralBytes = 0
	c.dropCohortEphemeralPeak = 0
	defer func() { c.dropCohortEphemeralBytes = 0 }()
	if uint64(cap(c.dropCohortDerivationScratch)) > c.limits.MaxDropCohortBytes {
		c.dropCohortDerivationScratch = nil
	}
	if coreDropCohortPathStepBytes == 0 || uint64(cap(c.dropCohortPathScratch)) > c.limits.MaxDropCohortBytes/coreDropCohortPathStepBytes {
		c.dropCohortPathScratch = nil
	}
	derivationBytes, err := dropCohortScratchBytes(uint64(cap(c.dropCohortDerivationScratch)), 1)
	if err != nil {
		c.dropCohortDerivationScratch = nil
		c.dropCohortPathScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	pathBytes, err := dropCohortScratchBytes(uint64(cap(c.dropCohortPathScratch)), coreDropCohortPathStepBytes)
	if err != nil || derivationBytes > c.limits.MaxDropCohortBytes-pathBytes {
		c.dropCohortDerivationScratch = nil
		c.dropCohortPathScratch = nil
		derivationBytes = 0
		pathBytes = 0
	}
	if err := c.dropCohortEphemeralReserve(derivationBytes + pathBytes); err != nil {
		c.dropCohortDerivationScratch = nil
		c.dropCohortPathScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	c.dropCohortDerivationScratch = c.dropCohortDerivationScratch[:0]
	e := dropCohortEncoder{core: c, dst: c.dropCohortDerivationScratch, limit: c.limits.MaxDropCohortBytes}
	if err := e.u32(dropCohortDerivationFormatVersion); err != nil {
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	if err := e.u8(dropCohortTagRecordBegin); err != nil {
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	if err := c.dropCohortEncodeBoundary(&e, head.Node); err != nil {
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	if err := c.dropCohortEncodeCheckpoint(&e, checkpoint); err != nil {
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	countOffset := len(e.dst)
	if err := e.u32(0); err != nil {
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	var root Symbol
	var rootSet bool
	var count, maxDepth uint32
	path := c.dropCohortPathScratch[:0]
	path, err = c.dropCohortEncodeAllPaths(&e, head.Node, path, 0, ForkOrder{}, &count, &maxDepth, &root, &rootSet)
	if err != nil {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	c.dropCohortPathScratch = path[:0]
	if count == 0 {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, errors.New("parser-core phase zero: drop-cohort head has no derivation")
	}
	if err := e.patchU32(countOffset, count); err != nil {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	if err := e.u8(dropCohortTagRecordEnd); err != nil {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	c.dropCohortDerivationScratch = e.dst
	encoded := c.dropCohortDerivationScratch
	if uint64(len(c.dropCohortDerivationBytes)) > uint64(math.MaxUint32)-uint64(len(encoded)) || uint64(len(encoded)) > uint64(math.MaxUint32) {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, errors.New("parser-core phase zero: drop-cohort derivation offset overflow")
	}
	byteOffset := uint32(len(c.dropCohortDerivationBytes))
	digest := sha256.Sum256(encoded)
	reused := false
	for _, entry := range c.dropCohortDerivationIntern {
		if entry.digest != digest || entry.byteLength != uint32(len(encoded)) {
			continue
		}
		start := int(entry.byteOffset)
		end := start + int(entry.byteLength)
		if start >= 0 && end >= start && end <= len(c.dropCohortDerivationBytes) && bytesCompare(c.dropCohortDerivationBytes[start:end], encoded) == 0 {
			byteOffset = entry.byteOffset
			reused = true
			break
		}
	}
	reservation := c.dropCohortCurrentReservation()
	baseStore, baseErr := dropCohortAddChecked(c.dropCohortStoreBytes(), c.dropCohortReservedBytes)
	if baseErr != nil {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, baseErr
	}
	additionalStore := uint64(0)
	if reservation == nil {
		additionalStore, baseErr = dropCohortAddChecked(uint64(coreDropCohortDerivationRecordBytes), uint64(coreDropCohortMapEntryBytes))
		if baseErr != nil {
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, baseErr
		}
	}
	if !reused {
		encodedBytes, addErr := dropCohortAddChecked(uint64(len(encoded)), uint64(coreDropCohortDerivationInternBytes))
		if addErr != nil {
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, addErr
		}
		additionalStore, addErr = dropCohortAddChecked(additionalStore, encodedBytes)
		if addErr != nil {
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, addErr
		}
	}
	if baseStore > c.limits.MaxDropCohortBytes || additionalStore > c.limits.MaxDropCohortBytes-baseStore {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, fmt.Errorf("parser-core phase zero: drop-cohort derivation byte cap (existing=%d additional=%d max=%d)", baseStore, additionalStore, c.limits.MaxDropCohortBytes)
	}
	var dynamicDemand [7]uint64
	if reservation == nil {
		dynamicDemand[1], dynamicDemand[3] = 1, 1
	}
	if !reused && (reservation == nil || reservation.reserved[4] == 0) {
		dynamicDemand[4] = 1
	}
	dynamicDerivationBytes := uint64(0)
	if !reused {
		dynamicDerivationBytes = uint64(len(encoded))
	}
	dynamicReserved, reserveErr := c.dropCohortReserveDemand(dynamicDemand, dynamicDerivationBytes)
	if reserveErr != nil {
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, reserveErr
	}
	if !reused {
		baseWithoutDerivation, baseErr := c.dropCohortRetainedOtherBytes()
		if baseErr != nil {
			c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, baseErr
		}
		retainedStores := c.dropCohortRetainedStoreBytes()
		for store, value := range retainedStores {
			if store != 1 {
				baseWithoutDerivation, baseErr = dropCohortAddChecked(baseWithoutDerivation, value)
				if baseErr != nil {
					c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
					c.dropCohortPathScratch = nil
					c.dropCohortDerivationScratch = nil
					return DropCohortDerivationHandle{}, baseErr
				}
			}
		}
		recordCapacity, recordErr := dropCohortMulChecked(uint64(cap(c.dropCohortDerivations)), coreDropCohortDerivationRecordBytes)
		if recordErr != nil {
			c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, recordErr
		}
		baseWithoutDerivation, recordErr = dropCohortAddChecked(baseWithoutDerivation, recordCapacity)
		if recordErr != nil {
			c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, recordErr
		}
		baseWithoutDerivation, recordErr = dropCohortAddChecked(baseWithoutDerivation, coreDropCohortDerivationRecordBytes)
		if recordErr != nil {
			c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, recordErr
		}
		if c.dropCohortReservedBytes >= dynamicDerivationBytes {
			baseWithoutDerivation, recordErr = dropCohortAddChecked(baseWithoutDerivation, c.dropCohortReservedBytes-dynamicDerivationBytes)
			if recordErr != nil {
				c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
				c.dropCohortPathScratch = nil
				c.dropCohortDerivationScratch = nil
				return DropCohortDerivationHandle{}, recordErr
			}
		}
		if baseWithoutDerivation >= c.limits.MaxDropCohortBytes {
			c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, errors.New("parser-core phase zero: drop-cohort derivation byte cap")
		}
		permanentLimit := c.limits.MaxDropCohortBytes - baseWithoutDerivation
		if err := appendDropCohortPermanent(&c.dropCohortDerivationBytes, encoded, permanentLimit); err != nil {
			c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, err
		}
		if err := appendDropCohortDerivationIntern(c, dropCohortDerivationInternEntry{digest: digest, byteOffset: byteOffset, byteLength: uint32(len(encoded))}); err != nil {
			c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
			c.dropCohortPathScratch = nil
			c.dropCohortDerivationScratch = nil
			return DropCohortDerivationHandle{}, err
		}
	}
	index := uint32(len(c.dropCohortDerivations))
	handle := DropCohortDerivationHandle{Owner: c.dropCohortOwner, Epoch: c.dropCohortEpoch, Index: index + 1}
	derivationRecord := dropCohortDerivationRecord{
		handle: handle, head: head, digest: digest, byteOffset: byteOffset,
		byteLength: uint32(len(encoded)), rootSymbol: root, stackDepth: maxDepth, checkpoint: checkpoint,
	}
	if err := appendDropCohortDerivationRecord(c, derivationRecord); err != nil {
		c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	if err := appendDropCohortMapEntry(c, dropCohortMapEntry{
		hash: binary.LittleEndian.Uint64(digest[:8]), index: index, used: true,
	}); err != nil {
		c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
		c.dropCohortPathScratch = nil
		c.dropCohortDerivationScratch = nil
		return DropCohortDerivationHandle{}, err
	}
	if dynamicReserved != 0 {
		c.dropCohortReleaseDemand(dynamicDemand, dynamicReserved)
	}
	if reservation != nil {
		var consumed [7]uint64
		consumed[1], consumed[3] = 1, 1
		if !reused {
			consumed[4] = 1
		}
		fixedBytes, _ := dropCohortDemandBytes(consumed, 0)
		c.dropCohortConsumeReservation(reservation.handle, consumed, fixedBytes)
	}
	c.dropCohortDerivationScratch = c.dropCohortDerivationScratch[:0]
	return handle, nil
}

func appendDropCohortPermanent(dst *[]byte, value []byte, limit uint64) error {
	if dst == nil {
		return errors.New("parser-core phase zero: nil drop-cohort derivation store")
	}
	if uint64(len(*dst)) > limit || uint64(len(value)) > limit-uint64(len(*dst)) {
		return errors.New("parser-core phase zero: drop-cohort derivation byte cap")
	}
	needed := len(*dst) + len(value)
	if needed > cap(*dst) {
		maxInt := int(^uint(0) >> 1)
		newCap := cap(*dst)
		if newCap < 64 {
			newCap = 64
		}
		for newCap < needed {
			if newCap > maxInt/2 {
				newCap = needed
				break
			}
			newCap *= 2
		}
		if uint64(newCap) > limit {
			newCap = int(limit)
		}
		if newCap < needed {
			return errors.New("parser-core phase zero: drop-cohort derivation byte cap")
		}
		grown := make([]byte, len(*dst), newCap)
		copy(grown, *dst)
		*dst = grown
	}
	start := len(*dst)
	*dst = (*dst)[:needed]
	copy((*dst)[start:], value)
	return nil
}

func appendDropCohortDerivationRecord(c *Core, value dropCohortDerivationRecord) error {
	if c == nil || len(c.dropCohortDerivations) == int(^uint(0)>>1) {
		return errors.New("parser-core phase zero: drop-cohort derivation record overflow")
	}
	if len(c.dropCohortDerivations) == cap(c.dropCohortDerivations) {
		newCap, err := c.dropCohortGrowPermanentStore(uint64(cap(c.dropCohortDerivations)), uint64(len(c.dropCohortDerivations)+1), coreDropCohortDerivationRecordBytes)
		if err != nil {
			return err
		}
		grown := make([]dropCohortDerivationRecord, len(c.dropCohortDerivations), newCap)
		copy(grown, c.dropCohortDerivations)
		c.dropCohortDerivations = grown
	}
	c.dropCohortDerivations = append(c.dropCohortDerivations, value)
	return nil
}

func appendDropCohortDerivationIntern(c *Core, value dropCohortDerivationInternEntry) error {
	if c == nil || len(c.dropCohortDerivationIntern) == int(^uint(0)>>1) {
		return errors.New("parser-core phase zero: drop-cohort interner entry overflow")
	}
	if len(c.dropCohortDerivationIntern) == cap(c.dropCohortDerivationIntern) {
		newCap, err := c.dropCohortGrowPermanentStore(uint64(cap(c.dropCohortDerivationIntern)), uint64(len(c.dropCohortDerivationIntern)+1), coreDropCohortDerivationInternBytes)
		if err != nil {
			return err
		}
		grown := make([]dropCohortDerivationInternEntry, len(c.dropCohortDerivationIntern), newCap)
		copy(grown, c.dropCohortDerivationIntern)
		c.dropCohortDerivationIntern = grown
	}
	c.dropCohortDerivationIntern = append(c.dropCohortDerivationIntern, value)
	return nil
}

func appendDropCohortMapEntry(c *Core, value dropCohortMapEntry) error {
	if c == nil || len(c.dropCohortMapStore) == int(^uint(0)>>1) {
		return errors.New("parser-core phase zero: drop-cohort map entry overflow")
	}
	if len(c.dropCohortMapStore) == cap(c.dropCohortMapStore) {
		newCap, err := c.dropCohortGrowPermanentStore(uint64(cap(c.dropCohortMapStore)), uint64(len(c.dropCohortMapStore)+1), coreDropCohortMapEntryBytes)
		if err != nil {
			return err
		}
		grown := make([]dropCohortMapEntry, len(c.dropCohortMapStore), newCap)
		copy(grown, c.dropCohortMapStore)
		c.dropCohortMapStore = grown
	}
	c.dropCohortMapStore = append(c.dropCohortMapStore, value)
	return nil
}

func (c *Core) dropCohortGrowPermanentStore(current, needed, elementBytes uint64) (int, error) {
	if needed > uint64(int(^uint(0)>>1)) || elementBytes == 0 {
		return 0, errors.New("parser-core phase zero: drop-cohort store capacity overflow")
	}
	newCap := current
	if newCap == 0 {
		newCap = 1
	}
	for newCap < needed {
		if newCap > math.MaxUint64/2 {
			newCap = needed
			break
		}
		newCap *= 2
	}
	base, err := dropCohortAddChecked(c.dropCohortStoreBytes(), c.dropCohortReservedBytes)
	if err != nil {
		return 0, err
	}
	if base > c.limits.MaxDropCohortBytes {
		return 0, errors.New("parser-core phase zero: drop-cohort store byte cap")
	}
	available := c.limits.MaxDropCohortBytes - base
	boundedGrowth, err := dropCohortAddChecked(current, available/elementBytes)
	if err != nil {
		return 0, err
	}
	if boundedGrowth < newCap {
		newCap = boundedGrowth
	}
	if newCap < needed || newCap > uint64(^uint(0)>>1) {
		return 0, errors.New("parser-core phase zero: drop-cohort store byte cap")
	}
	return int(newCap), nil
}

func bytesCompare(left, right []byte) int {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

// BuildDropCohortDerivationOwned records the complete graph derivation for a
// real parser head. It includes the supplied byte, point, and scanner checks.
func (c *Core) BuildDropCohortDerivationOwned(owner SchedulerTransactionToken, head Head, checkpoint DropCohortSourceCheckpoint) (handle DropCohortDerivationHandle, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return DropCohortDerivationHandle{}, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	handle, err = c.buildDropCohortDerivationOwned(head, checkpoint)
	err = c.finishSchedulerOwned(owner, err)
	return handle, err
}

// TryBuildDropCohortDerivationOwned keeps a bounded certificate producer
// failure non-poisoning so the scheduler can commit its ordinary fallback.
// Authentication failures and malformed graph errors still poison the frame.
func (c *Core) TryBuildDropCohortDerivationOwned(owner SchedulerTransactionToken, head Head, checkpoint DropCohortSourceCheckpoint) (handle DropCohortDerivationHandle, skipped bool, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return DropCohortDerivationHandle{}, false, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	handle, err = c.buildDropCohortDerivationOwned(head, checkpoint)
	if err != nil && strings.Contains(err.Error(), "drop-cohort derivation byte cap") {
		c.dropCohortDerivationScratch = c.dropCohortDerivationScratch[:0]
		c.dropCohortPathScratch = c.dropCohortPathScratch[:0]
		c.dropCohortEphemeralBytes = 0
		return DropCohortDerivationHandle{}, true, nil
	}
	err = c.finishSchedulerOwned(owner, err)
	return handle, false, err
}

func (c *Core) recordDropCohortMutation(index int) {
	if len(c.transactions) == 0 || index < 0 || index >= len(c.dropCohortRecords) {
		return
	}
	c.dropCohortJournal = append(c.dropCohortJournal, dropCohortMutation{index: index, before: c.dropCohortRecords[index]})
	for reservationIndex := range c.dropCohortReservations {
		reservation := &c.dropCohortReservations[reservationIndex]
		if reservation.handle != c.dropCohortRecords[index].handle || reservation.journalUsed >= reservation.journalCount {
			continue
		}
		slot := reservation.journalStart + reservation.journalUsed
		if slot < 0 || slot >= len(c.dropCohortJournalStore) {
			continue
		}
		c.dropCohortJournalStore[slot] = dropCohortJournalStoreEntry{
			store: 1, index: uint32(index), value: uint64(c.dropCohortRecords[index].state),
		}
		reservation.journalUsed++
		return
	}
}

func (c *Core) findDropCohortMember(record dropCohortRecord, branch uint16) (int, bool) {
	for index := range c.dropCohortMembers {
		member := c.dropCohortMembers[index]
		if member.cohort == record.handle && member.branch == branch {
			return index, true
		}
	}
	return -1, false
}

func (c *Core) writeDropCohortMemberOwned(cohort DropCohortHandle, head Head, branch uint16, derivation DropCohortDerivationHandle) error {
	index, err := c.validateDropCohortIdentity(cohort)
	if err != nil {
		return err
	}
	record := &c.dropCohortRecords[index]
	if record.state != DropCohortBuilding {
		return errors.New("parser-core phase zero: drop-cohort is not building")
	}
	if uint64(branch) >= uint64(record.expected) {
		c.recordDropCohortMutation(index)
		record.state = DropCohortOverflowed
		return errors.New("parser-core phase zero: drop-cohort branch exceeds expected members")
	}
	if derivation.Owner != c.dropCohortOwner || derivation.Epoch != c.dropCohortEpoch {
		return errors.New("parser-core phase zero: drop-cohort derivation identity mismatch")
	}
	if derivation.Index == 0 || uint64(derivation.Index) > uint64(len(c.dropCohortDerivations)) ||
		c.dropCohortDerivations[derivation.Index-1].head != head {
		return errors.New("parser-core phase zero: drop-cohort derivation handle is invalid")
	}
	if prior, ok := c.findDropCohortMember(*record, branch); ok {
		if c.dropCohortMembers[prior].head != head || c.dropCohortMembers[prior].derivation != derivation {
			c.recordDropCohortMutation(index)
			record.state = DropCohortBlended
			return errors.New("parser-core phase zero: drop-cohort branch conflict")
		}
		return errors.New("parser-core phase zero: duplicate drop-cohort member")
	}
	c.dropCohortMembers = append(c.dropCohortMembers, dropCohortMember{
		cohort: cohort, head: head, branch: branch, derivation: derivation,
		action: c.dropCohortActions[record.actionIndex],
	})
	c.recordDropCohortMutation(index)
	record.written++
	var consumed [7]uint64
	consumed[2] = 1
	fixedBytes, _ := dropCohortDemandBytes(consumed, 0)
	c.dropCohortConsumeReservation(cohort, consumed, fixedBytes)
	return nil
}

// WriteDropCohortMemberOwned appends one branch after the complete derivation
// record has been built. Duplicate and conflicting branches fail closed.
func (c *Core) WriteDropCohortMemberOwned(owner SchedulerTransactionToken, cohort DropCohortHandle, head Head, branch uint16, derivation DropCohortDerivationHandle) error {
	if err := c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	return c.finishSchedulerOwned(owner, c.writeDropCohortMemberOwned(cohort, head, branch, derivation))
}

func (c *Core) finalizeDropCohortOwned(cohort DropCohortHandle) (DropCohortRefSet, error) {
	index, err := c.validateDropCohortIdentity(cohort)
	if err != nil {
		return DropCohortRefSet{}, err
	}
	record := &c.dropCohortRecords[index]
	if record.state != DropCohortBuilding {
		return DropCohortRefSet{}, errors.New("parser-core phase zero: drop-cohort is not building")
	}
	if record.expected == 0 || record.written != record.expected {
		return DropCohortRefSet{}, errors.New("parser-core phase zero: drop-cohort finalization is partial")
	}
	if record.expected > dropCohortRefInlineCapacity {
		if err := c.dropCohortRefPreflight(int(record.expected) - dropCohortRefInlineCapacity); err != nil {
			return DropCohortRefSet{}, err
		}
	}
	var refs DropCohortRefSet
	for _, member := range c.dropCohortMembers {
		if member.cohort != record.handle {
			continue
		}
		if !c.AddDropCohortRef(&refs, DropCohortRef{
			Owner: cohort.Owner, Epoch: cohort.Epoch, Sequence: cohort.Sequence, Branch: member.branch,
		}) {
			return DropCohortRefSet{}, errors.New("parser-core phase zero: drop-cohort reference overflow")
		}
	}
	if refs.Len() != int(record.expected) {
		return DropCohortRefSet{}, errors.New("parser-core phase zero: drop-cohort member set is incomplete")
	}
	for reservationIndex := range c.dropCohortReservations {
		reservation := &c.dropCohortReservations[reservationIndex]
		if reservation.handle != cohort {
			continue
		}
		neededRefs := reservation.certificateRefs + refs.Len()
		if neededRefs > len(c.dropCohortCertificateRefs) {
			additional := uint64(neededRefs-len(c.dropCohortCertificateRefs)) * uint64(coreDropCohortRefBytes)
			existing := c.dropCohortStoreBytes()
			if existing > c.limits.MaxDropCohortBytes || additional > c.limits.MaxDropCohortBytes-existing {
				return DropCohortRefSet{}, errors.New("parser-core phase zero: drop-cohort certificate reference byte cap")
			}
			c.dropCohortCertificateRefs = append(c.dropCohortCertificateRefs, make([]DropCohortRef, neededRefs-len(c.dropCohortCertificateRefs))...)
		}
		for branch := 0; branch < refs.Len(); branch++ {
			ref, ok := c.DropCohortRefAt(refs, branch)
			if !ok || reservation.certificateRefs+branch >= len(c.dropCohortCertificateRefs) {
				return DropCohortRefSet{}, errors.New("parser-core phase zero: drop-cohort certificate reference store is incomplete")
			}
			c.dropCohortCertificateRefs[reservation.certificateRefs+branch] = ref
		}
		var consumed [7]uint64
		consumed[2] = uint64(refs.Len())
		fixedBytes, _ := dropCohortDemandBytes(consumed, 0)
		c.dropCohortConsumeReservation(cohort, consumed, fixedBytes)
		break
	}
	c.recordDropCohortMutation(index)
	record.state = DropCohortComplete
	c.dropCohortReleaseReservationRemainder(cohort)
	return refs, nil
}

// FinalizeDropCohortOwned exposes a cohort only after every branch write.
func (c *Core) FinalizeDropCohortOwned(owner SchedulerTransactionToken, cohort DropCohortHandle) (refs DropCohortRefSet, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return DropCohortRefSet{}, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	refs, err = c.finalizeDropCohortOwned(cohort)
	err = c.finishSchedulerOwned(owner, err)
	return refs, err
}

func (c *Core) dropCohortRefForBranch(cohort DropCohortHandle, branch uint16) (DropCohortRef, error) {
	index, err := c.validateDropCohortIdentity(cohort)
	if err != nil {
		return DropCohortRef{}, err
	}
	record := c.dropCohortRecords[index]
	if record.state != DropCohortComplete {
		return DropCohortRef{}, errors.New("parser-core phase zero: drop-cohort is not complete")
	}
	if _, ok := c.findDropCohortMember(record, branch); !ok {
		return DropCohortRef{}, fmt.Errorf("parser-core phase zero: drop-cohort branch %d is absent", branch)
	}
	return DropCohortRef{Owner: cohort.Owner, Epoch: cohort.Epoch, Sequence: cohort.Sequence, Branch: branch}, nil
}

// DropCohortRefForBranch returns one finalized reference for a scheduler
// output header. It performs no arena mutation.
func (c *Core) DropCohortRefForBranch(cohort DropCohortHandle, branch uint16) (DropCohortRef, error) {
	return c.dropCohortRefForBranch(cohort, branch)
}

// DropCohortRefForBranchOwned reads one finalized branch only under the
// active scheduler token. It validates ownership before the cohort store read.
func (c *Core) DropCohortRefForBranchOwned(owner SchedulerTransactionToken, cohort DropCohortHandle, branch uint16) (DropCohortRef, error) {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return DropCohortRef{}, errors.New("parser-core phase zero: drop-cohort branch owner mismatch")
	}
	return c.dropCohortRefForBranch(cohort, branch)
}

// DropCohortState returns one cohort's producer state and counts.
func (c *Core) DropCohortState(handle DropCohortHandle) (DropCohortState, uint16, uint16, error) {
	index, err := c.validateDropCohortIdentity(handle)
	if err != nil {
		return 0, 0, 0, err
	}
	record := c.dropCohortRecords[index]
	return record.state, record.expected, record.written, nil
}

// DropCohortAction returns the exact dispatch identity retained by a cohort.
func (c *Core) DropCohortAction(handle DropCohortHandle) (DropCohortActionIdentity, bool) {
	index, err := c.validateDropCohortIdentity(handle)
	if err != nil {
		return DropCohortActionIdentity{}, false
	}
	actionIndex := c.dropCohortRecords[index].actionIndex
	if uint64(actionIndex) >= uint64(len(c.dropCohortActions)) {
		return DropCohortActionIdentity{}, false
	}
	return c.dropCohortActions[actionIndex], true
}

func (c *Core) dropCohortDerivationRecord(handle DropCohortDerivationHandle) (DropCohortDerivationRecord, bool) {
	if c == nil || handle.Owner != c.dropCohortOwner || handle.Epoch != c.dropCohortEpoch || handle.Index == 0 || uint64(handle.Index) > uint64(len(c.dropCohortDerivations)) {
		return DropCohortDerivationRecord{}, false
	}
	record := c.dropCohortDerivations[handle.Index-1]
	start := int(record.byteOffset)
	end := start + int(record.byteLength)
	if start < 0 || end < start || end > len(c.dropCohortDerivationBytes) {
		return DropCohortDerivationRecord{}, false
	}
	return DropCohortDerivationRecord{
		Handle: record.handle, Head: record.head, Digest: record.digest,
		RootSymbol: record.rootSymbol, StackDepth: record.stackDepth,
		Checkpoint: record.checkpoint, Bytes: c.dropCohortDerivationBytes[start:end],
	}, true
}

// DropCohortDerivationRecord returns the immutable record metadata and its
// canonical bytes. The byte slice aliases the Core and is read-only.
func (c *Core) DropCohortDerivationRecord(handle DropCohortDerivationHandle) (DropCohortDerivationRecord, bool) {
	return c.dropCohortDerivationRecord(handle)
}

// dropCohortDerivationRecordOwned reads derivation storage only under the
// active scheduler token. Frontier verification uses this private adapter.
func (c *Core) dropCohortDerivationRecordOwned(owner SchedulerTransactionToken, handle DropCohortDerivationHandle) (DropCohortDerivationRecord, bool) {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return DropCohortDerivationRecord{}, false
	}
	return c.dropCohortDerivationRecord(handle)
}

// DropCohortDerivationRecord is a read-only view of one graph derivation.
type DropCohortDerivationRecord struct {
	Handle     DropCohortDerivationHandle
	Head       Head
	Digest     [32]byte
	RootSymbol Symbol
	StackDepth uint32
	Checkpoint DropCohortSourceCheckpoint
	Bytes      []byte
}

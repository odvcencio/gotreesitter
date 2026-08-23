package parsercorephase0

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
)

// DropCohortFrontierHandle identifies one authenticated scheduler frontier.
// The owner and epoch are checked before the sequence is read.
type DropCohortFrontierHandle struct {
	Owner    uint64
	Epoch    uint64
	Sequence uint64
}

// DropCohortFrontierToken is the immutable token receipt bound to one
// frontier. Checkpoint digests are checked against the Core interner.
type DropCohortFrontierToken struct {
	Symbol               Symbol
	StartByte            uint32
	EndByte              uint32
	StartRow             uint32
	StartColumn          uint32
	EndRow               uint32
	EndColumn            uint32
	NoLookahead          bool
	Missing              bool
	ExternalScannerToken bool
	ScannerBefore        CheckpointID
	ScannerAfter         CheckpointID
	ScannerBeforeDigest  [32]byte
	ScannerAfterDigest   [32]byte
}

// DropCohortFrontierState describes one frontier producer lifecycle.
type DropCohortFrontierState uint8

const (
	DropCohortFrontierBuilding DropCohortFrontierState = iota + 1
	DropCohortFrontierComplete
	DropCohortFrontierOverflowed
	DropCohortFrontierIncomplete
)

const (
	dropCohortFrontierParticipantHardCap = dropCohortRefHardCap
	dropCohortFrontierMemberHardCap      = dropCohortRefHardCap * dropCohortRefHardCap
)

type dropCohortFrontierParticipant struct {
	head           Head
	branchOrder    uint64
	memberStart    uint32
	memberCount    uint16
	referenceFlags uint8
}

type dropCohortFrontierMember struct {
	participant          uint16
	ref                  DropCohortRef
	participantHead      Head
	sourceHead           Head
	branchOrder          uint64
	action               DropCohortActionIdentity
	derivation           DropCohortDerivationHandle
	derivationDigest     [32]byte
	derivationLength     uint32
	derivationRootSymbol Symbol
	derivationStackDepth uint32
	derivationCheckpoint DropCohortSourceCheckpoint
}

type dropCohortFrontierRecord struct {
	handle               DropCohortFrontierHandle
	electionSequence     uint64
	token                DropCohortFrontierToken
	state                DropCohortFrontierState
	expectedParticipants uint16
	writtenParticipants  uint16
	expectedMembers      uint32
	writtenMembers       uint32
	participantStart     uint32
	memberStart          uint32
	seal                 [32]byte
}

func dropCohortFrontierProtocolHandle(handle DropCohortFrontierHandle) [3]uint64 {
	return [3]uint64{handle.Owner, handle.Epoch, handle.Sequence}
}

func dropCohortFrontierProtocolState(state DropCohortFrontierState) string {
	switch state {
	case DropCohortFrontierBuilding:
		return "building"
	case DropCohortFrontierComplete:
		return "complete"
	case DropCohortFrontierOverflowed:
		return "overflowed"
	case DropCohortFrontierIncomplete:
		return "incomplete"
	default:
		return "unknown"
	}
}

func (c *Core) validateDropCohortFrontierIdentity(handle DropCohortFrontierHandle) (int, error) {
	if c == nil || c.dropCohortOwner == 0 || c.dropCohortEpoch == 0 {
		return -1, errors.New("parser-core phase zero: frontier arena identity is unavailable")
	}
	// Count the attempted identity check before comparing it. Do not read the
	// frontier record until owner and epoch checks have passed.
	c.dropCohortOwnerCheckedLookups++
	if handle.Owner != c.dropCohortOwner {
		return -1, errors.New("parser-core phase zero: frontier owner mismatch")
	}
	if handle.Epoch != c.dropCohortEpoch {
		return -1, errors.New("parser-core phase zero: frontier epoch mismatch")
	}
	if handle.Sequence == 0 {
		return -1, errors.New("parser-core phase zero: zero frontier sequence")
	}
	index := handle.Sequence - 1
	if index < uint64(len(c.dropCohortFrontiers)) && c.dropCohortFrontiers[index].handle.Sequence == handle.Sequence {
		return int(index), nil
	}
	return -1, errors.New("parser-core phase zero: unknown frontier sequence")
}

func (c *Core) dropCohortFrontierTokenValid(owner SchedulerTransactionToken, token DropCohortFrontierToken) bool {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return false
	}
	_, before, beforeOK := c.CheckpointReceiptOwned(owner, token.ScannerBefore)
	_, after, afterOK := c.CheckpointReceiptOwned(owner, token.ScannerAfter)
	return beforeOK && afterOK && before == token.ScannerBeforeDigest && after == token.ScannerAfterDigest
}

func (c *Core) dropCohortFrontierWithinByteLimit(additionalRecords, additionalParticipants, additionalMembers int) bool {
	if c == nil || additionalRecords < 0 || additionalParticipants < 0 || additionalMembers < 0 {
		return false
	}
	recordBytes, err := dropCohortMulChecked(uint64(additionalRecords), coreDropCohortFrontierRecordBytes)
	if err != nil {
		return false
	}
	participantBytes, err := dropCohortMulChecked(uint64(additionalParticipants), coreDropCohortFrontierParticipantBytes)
	if err != nil {
		return false
	}
	memberBytes, err := dropCohortMulChecked(uint64(additionalMembers), coreDropCohortFrontierMemberBytes)
	if err != nil {
		return false
	}
	additional, err := dropCohortAddChecked(recordBytes, participantBytes)
	if err != nil {
		return false
	}
	additional, err = dropCohortAddChecked(additional, memberBytes)
	if err != nil {
		return false
	}
	current := c.dropCohortStoreBytes()
	return current <= c.limits.MaxDropCohortBytes && additional <= c.limits.MaxDropCohortBytes-current
}

func (c *Core) beginDropCohortFrontierOwned(
	owner SchedulerTransactionToken,
	electionSequence uint64,
	token DropCohortFrontierToken,
	expectedParticipants, expectedMembers int,
) (DropCohortFrontierHandle, error) {
	if electionSequence == 0 {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: zero frontier election sequence")
	}
	if expectedParticipants <= 0 || expectedParticipants > dropCohortFrontierParticipantHardCap {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: frontier participant cap")
	}
	if expectedMembers < 0 || expectedMembers > dropCohortFrontierMemberHardCap {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: frontier member cap")
	}
	if !c.dropCohortFrontierTokenValid(owner, token) {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: frontier token receipt is invalid")
	}
	if uint64(len(c.dropCohortFrontiers))+1 > uint64(c.limits.MaxDropCohorts) {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: frontier record cap")
	}
	if c.dropCohortFrontierNextSequence == ^uint64(0) {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: frontier sequence overflow")
	}
	sequence := c.dropCohortFrontierNextSequence + 1
	handle := DropCohortFrontierHandle{Owner: c.dropCohortOwner, Epoch: c.dropCohortEpoch, Sequence: sequence}
	participantStart := len(c.dropCohortFrontierParticipants)
	memberStart := len(c.dropCohortFrontierMembers)
	if uint64(participantStart)+uint64(expectedParticipants) > uint64(^uint32(0)) ||
		uint64(memberStart)+uint64(expectedMembers) > uint64(^uint32(0)) {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: frontier arena index overflow")
	}
	if !c.dropCohortFrontierWithinByteLimit(1, expectedParticipants, expectedMembers) {
		return DropCohortFrontierHandle{}, errors.New("parser-core phase zero: frontier byte cap")
	}
	c.dropCohortFrontiers = append(c.dropCohortFrontiers, dropCohortFrontierRecord{
		handle: handle, electionSequence: electionSequence, token: token,
		state: DropCohortFrontierBuilding, expectedParticipants: uint16(expectedParticipants),
		expectedMembers: uint32(expectedMembers), participantStart: uint32(participantStart),
		memberStart: uint32(memberStart),
	})
	c.dropCohortFrontierNextSequence = sequence
	return handle, nil
}

// BeginDropCohortFrontierOwned starts one bounded, owner-authenticated
// frontier record. It does not publish a header reference.
func (c *Core) BeginDropCohortFrontierOwned(
	owner SchedulerTransactionToken,
	electionSequence uint64,
	token DropCohortFrontierToken,
	expectedParticipants, expectedMembers int,
) (handle DropCohortFrontierHandle, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return DropCohortFrontierHandle{}, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	handle, err = c.beginDropCohortFrontierOwned(owner, electionSequence, token, expectedParticipants, expectedMembers)
	return handle, c.finishSchedulerOwned(owner, err)
}

func (c *Core) dropCohortFrontierMarkIncomplete(index int, overflow bool) {
	if index < 0 || index >= len(c.dropCohortFrontiers) {
		return
	}
	if overflow {
		c.dropCohortFrontiers[index].state = DropCohortFrontierOverflowed
	} else {
		c.dropCohortFrontiers[index].state = DropCohortFrontierIncomplete
	}
}

func (c *Core) dropCohortFrontierRef(ref DropCohortRef) (dropCohortRecord, dropCohortMember, error) {
	// The caller must perform the scheduler-token check before this helper.
	if ref.Owner == 0 || ref.Owner != c.dropCohortOwner {
		return dropCohortRecord{}, dropCohortMember{}, errors.New("parser-core phase zero: frontier reference owner mismatch")
	}
	if ref.Epoch == 0 || ref.Epoch != c.dropCohortEpoch {
		return dropCohortRecord{}, dropCohortMember{}, errors.New("parser-core phase zero: frontier reference epoch mismatch")
	}
	if ref.Sequence == 0 {
		return dropCohortRecord{}, dropCohortMember{}, errors.New("parser-core phase zero: frontier reference sequence is zero")
	}
	index, _, reason := c.dropCohortVerifierLookup(ref, false)
	if reason != dropCohortVerifierProved {
		return dropCohortRecord{}, dropCohortMember{}, dropCohortVerifierError(reason)
	}
	record := c.dropCohortRecords[index]
	if dropCohortVerifierClassifyRecord(record) != dropCohortVerifierProved {
		return dropCohortRecord{}, dropCohortMember{}, errors.New("parser-core phase zero: frontier reference cohort is not complete")
	}
	memberIndex, ok := c.findDropCohortMember(record, ref.Branch)
	if !ok {
		return dropCohortRecord{}, dropCohortMember{}, errors.New("parser-core phase zero: frontier reference branch is absent")
	}
	return record, c.dropCohortMembers[memberIndex], nil
}

func (c *Core) dropCohortFrontierMemberFromRef(
	owner SchedulerTransactionToken,
	frontier DropCohortFrontierHandle,
	participant uint16,
	participantHead Head,
	branchOrder uint64,
	ref DropCohortRef,
) (dropCohortFrontierMember, error) {
	if err := c.validateSchedulerTransaction(owner); err != nil {
		return dropCohortFrontierMember{}, err
	}
	if _, err := c.validateDropCohortFrontierIdentity(frontier); err != nil {
		return dropCohortFrontierMember{}, errors.New("parser-core phase zero: frontier member identity mismatch")
	}
	record, source, err := c.dropCohortFrontierRef(ref)
	if err != nil {
		return dropCohortFrontierMember{}, err
	}
	if record.handle.Owner != frontier.Owner || record.handle.Epoch != frontier.Epoch {
		return dropCohortFrontierMember{}, errors.New("parser-core phase zero: frontier member cohort identity mismatch")
	}
	actionIndex := record.actionIndex
	if uint64(actionIndex) >= uint64(len(c.dropCohortActions)) {
		return dropCohortFrontierMember{}, errors.New("parser-core phase zero: frontier action record is absent")
	}
	derivation, ok := c.dropCohortDerivationRecordOwned(owner, source.derivation)
	if !ok || derivation.Head != source.head {
		return dropCohortFrontierMember{}, errors.New("parser-core phase zero: frontier derivation record is absent")
	}
	return dropCohortFrontierMember{
		participant: participant, ref: ref, participantHead: participantHead,
		sourceHead: source.head, branchOrder: branchOrder,
		action: c.dropCohortActions[actionIndex], derivation: source.derivation,
		derivationDigest: derivation.Digest, derivationLength: uint32(len(derivation.Bytes)),
		derivationRootSymbol: derivation.RootSymbol, derivationStackDepth: derivation.StackDepth,
		derivationCheckpoint: derivation.Checkpoint,
	}, nil
}

func (c *Core) writeDropCohortFrontierParticipantOwned(
	owner SchedulerTransactionToken,
	frontier DropCohortFrontierHandle,
	participant uint16,
	head Head,
	branchOrder uint64,
	refs DropCohortRefSet,
) error {
	if err := c.validateSchedulerTransaction(owner); err != nil {
		return err
	}
	index, err := c.validateDropCohortFrontierIdentity(frontier)
	if err != nil {
		return err
	}
	record := &c.dropCohortFrontiers[index]
	if record.state != DropCohortFrontierBuilding {
		return errors.New("parser-core phase zero: frontier is not building")
	}
	if participant != record.writtenParticipants || participant >= record.expectedParticipants {
		c.dropCohortFrontierMarkIncomplete(index, false)
		return errors.New("parser-core phase zero: frontier participant order is incomplete")
	}
	if head.Node == 0 {
		c.dropCohortFrontierMarkIncomplete(index, false)
		return errors.New("parser-core phase zero: frontier participant identity is incomplete")
	}
	for prior := uint16(0); prior < participant; prior++ {
		previous := c.dropCohortFrontierParticipants[int(record.participantStart)+int(prior)]
		if previous.head == head {
			c.dropCohortFrontierMarkIncomplete(index, false)
			return errors.New("parser-core phase zero: frontier participant heads are duplicated")
		}
	}
	count, valid := c.dropCohortRefCount(refs)
	if !valid || refs.Overflowed() || count == 0 || count > dropCohortRefHardCap {
		c.dropCohortFrontierMarkIncomplete(index, refs.Overflowed())
		return errors.New("parser-core phase zero: frontier reference set is incomplete")
	}
	var members [dropCohortRefHardCap]dropCohortFrontierMember
	for refIndex := 0; refIndex < count; refIndex++ {
		if err := c.validateSchedulerTransaction(owner); err != nil {
			return err
		}
		// DropCohortRefAt may read the spill arena. Keep this owner check
		// immediately before the accessor.
		ref, ok := c.DropCohortRefAtOwned(owner, refs, refIndex)
		if !ok {
			c.dropCohortFrontierMarkIncomplete(index, false)
			return errors.New("parser-core phase zero: frontier reference accessor failed")
		}
		for prior := 0; prior < refIndex; prior++ {
			if members[prior].ref == ref {
				c.dropCohortFrontierMarkIncomplete(index, false)
				return errors.New("parser-core phase zero: frontier reference set contains a duplicate")
			}
		}
		member, memberErr := c.dropCohortFrontierMemberFromRef(owner, frontier, participant, head, branchOrder, ref)
		if memberErr != nil {
			c.dropCohortFrontierMarkIncomplete(index, false)
			return memberErr
		}
		members[refIndex] = member
	}
	if uint64(len(c.dropCohortFrontierParticipants))+1 > uint64(^uint32(0)) ||
		uint64(len(c.dropCohortFrontierMembers))+uint64(count) > uint64(^uint32(0)) {
		c.dropCohortFrontierMarkIncomplete(index, true)
		return errors.New("parser-core phase zero: frontier arena index overflow")
	}
	if !c.dropCohortFrontierWithinByteLimit(0, 1, count) {
		c.dropCohortFrontierMarkIncomplete(index, true)
		return errors.New("parser-core phase zero: frontier byte cap")
	}
	participantRecord := dropCohortFrontierParticipant{
		head: head, branchOrder: branchOrder, referenceFlags: refs.Flags,
		memberStart: uint32(len(c.dropCohortFrontierMembers)), memberCount: uint16(count),
	}
	c.dropCohortFrontierParticipants = append(c.dropCohortFrontierParticipants, participantRecord)
	c.dropCohortFrontierMembers = append(c.dropCohortFrontierMembers, members[:count]...)
	record.writtenParticipants++
	record.writtenMembers += uint32(count)
	return nil
}

// WriteDropCohortFrontierParticipantOwned appends one ordered participant.
// It publishes no scheduler header reference.
func (c *Core) WriteDropCohortFrontierParticipantOwned(
	owner SchedulerTransactionToken,
	frontier DropCohortFrontierHandle,
	participant uint16,
	head Head,
	branchOrder uint64,
	refs DropCohortRefSet,
) (err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	err = c.writeDropCohortFrontierParticipantOwned(owner, frontier, participant, head, branchOrder, refs)
	return c.finishSchedulerOwned(owner, err)
}

func (c *Core) dropCohortFrontierMemberValid(owner SchedulerTransactionToken, member dropCohortFrontierMember) bool {
	if c.validateSchedulerTransaction(owner) != nil {
		return false
	}
	if member.ref.Owner != c.dropCohortOwner || member.ref.Epoch != c.dropCohortEpoch || member.ref.Sequence == 0 {
		return false
	}
	index, _, reason := c.dropCohortVerifierLookup(member.ref, false)
	if reason != dropCohortVerifierProved {
		return false
	}
	record := c.dropCohortRecords[index]
	if dropCohortVerifierClassifyRecord(record) != dropCohortVerifierProved {
		return false
	}
	memberIndex, ok := c.findDropCohortMember(record, member.ref.Branch)
	if !ok {
		return false
	}
	source := c.dropCohortMembers[memberIndex]
	if source.head != member.sourceHead || source.derivation != member.derivation || source.action != member.action {
		return false
	}
	if uint64(record.actionIndex) >= uint64(len(c.dropCohortActions)) || c.dropCohortActions[record.actionIndex] != member.action {
		return false
	}
	derivation, ok := c.dropCohortDerivationRecordOwned(owner, member.derivation)
	if !ok || derivation.Head != member.sourceHead || derivation.Digest != member.derivationDigest ||
		uint32(len(derivation.Bytes)) != member.derivationLength || derivation.RootSymbol != member.derivationRootSymbol ||
		derivation.StackDepth != member.derivationStackDepth || derivation.Checkpoint != member.derivationCheckpoint {
		return false
	}
	return sha256.Sum256(derivation.Bytes) == member.derivationDigest
}

func (c *Core) dropCohortFrontierSealOwned(owner SchedulerTransactionToken, index int) ([32]byte, bool) {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return [32]byte{}, false
	}
	if index < 0 || index >= len(c.dropCohortFrontiers) {
		return [32]byte{}, false
	}
	record := c.dropCohortFrontiers[index]
	start := int(record.participantStart)
	end := start + int(record.expectedParticipants)
	memberStart := int(record.memberStart)
	memberEnd := memberStart + int(record.writtenMembers)
	if start < 0 || end < start || end > len(c.dropCohortFrontierParticipants) ||
		memberStart < 0 || memberEnd < memberStart || memberEnd > len(c.dropCohortFrontierMembers) {
		return [32]byte{}, false
	}
	h := sha256.New()
	writeU64 := func(value uint64) {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], value)
		_, _ = h.Write(buf[:])
	}
	writeU32 := func(value uint32) {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], value)
		_, _ = h.Write(buf[:])
	}
	writeBool := func(value bool) {
		if value {
			writeU64(1)
		} else {
			writeU64(0)
		}
	}
	writeU64(record.handle.Owner)
	writeU64(record.handle.Epoch)
	writeU64(record.handle.Sequence)
	writeU64(record.electionSequence)
	writeU64(uint64(record.token.Symbol))
	for _, value := range [...]uint32{record.token.StartByte, record.token.EndByte, record.token.StartRow, record.token.StartColumn, record.token.EndRow, record.token.EndColumn} {
		writeU32(value)
	}
	writeBool(record.token.NoLookahead)
	writeBool(record.token.Missing)
	writeBool(record.token.ExternalScannerToken)
	writeU64(uint64(record.token.ScannerBefore))
	writeU64(uint64(record.token.ScannerAfter))
	_, _ = h.Write(record.token.ScannerBeforeDigest[:])
	_, _ = h.Write(record.token.ScannerAfterDigest[:])
	writeU64(uint64(record.expectedParticipants))
	writeU64(uint64(record.writtenParticipants))
	writeU64(uint64(record.writtenMembers))
	for participantIndex := start; participantIndex < end; participantIndex++ {
		participant := c.dropCohortFrontierParticipants[participantIndex]
		writeU64(uint64(participant.head.Node))
		writeU64(participant.branchOrder)
		writeU64(uint64(participant.referenceFlags))
		writeU64(uint64(participant.memberStart))
		writeU64(uint64(participant.memberCount))
		for memberIndex := int(participant.memberStart); memberIndex < int(participant.memberStart)+int(participant.memberCount); memberIndex++ {
			if memberIndex < memberStart || memberIndex >= memberEnd {
				return [32]byte{}, false
			}
			member := c.dropCohortFrontierMembers[memberIndex]
			writeU64(uint64(member.participant))
			writeU64(member.ref.Owner)
			writeU64(member.ref.Epoch)
			writeU64(member.ref.Sequence)
			writeU64(uint64(member.ref.Branch))
			writeU64(uint64(member.participantHead.Node))
			writeU64(uint64(member.sourceHead.Node))
			writeU64(member.branchOrder)
			writeU64(uint64(member.action.BoundaryState))
			writeU64(uint64(member.action.Lookahead))
			writeU64(uint64(member.action.ActionOrdinal))
			writeU64(uint64(member.action.Action.Type))
			writeU64(uint64(member.action.Action.State))
			writeU64(uint64(member.action.Action.Symbol))
			writeU64(uint64(member.action.Action.ChildCount))
			writeU64(uint64(uint16(member.action.Action.DynamicPrecedence)))
			writeU64(uint64(member.action.Action.ProductionID))
			writeBool(member.action.Action.Extra)
			writeBool(member.action.Action.ExtraChain)
			writeBool(member.action.Action.Repetition)
			writeBool(member.action.NoLookahead)
			writeU64(uint64(member.action.Selection))
			writeU64(member.derivation.Owner)
			writeU64(member.derivation.Epoch)
			writeU64(uint64(member.derivation.Index))
			writeU64(uint64(member.derivationLength))
			writeU64(uint64(member.derivationRootSymbol))
			writeU64(uint64(member.derivationStackDepth))
			for _, value := range [...]uint32{member.derivationCheckpoint.StartByte, member.derivationCheckpoint.EndByte, member.derivationCheckpoint.StartRow, member.derivationCheckpoint.StartColumn, member.derivationCheckpoint.EndRow, member.derivationCheckpoint.EndColumn} {
				writeU32(value)
			}
			writeU64(uint64(member.derivationCheckpoint.ScannerStart))
			writeU64(uint64(member.derivationCheckpoint.ScannerEnd))
			_, _ = h.Write(member.derivationDigest[:])
		}
	}
	var seal [32]byte
	copy(seal[:], h.Sum(nil))
	return seal, true
}

func (c *Core) finalizeDropCohortFrontierOwned(owner SchedulerTransactionToken, frontier DropCohortFrontierHandle) error {
	if err := c.validateSchedulerTransaction(owner); err != nil {
		return err
	}
	index, err := c.validateDropCohortFrontierIdentity(frontier)
	if err != nil {
		return err
	}
	record := &c.dropCohortFrontiers[index]
	if record.state != DropCohortFrontierBuilding {
		return errors.New("parser-core phase zero: frontier is not building")
	}
	if record.writtenParticipants != record.expectedParticipants || record.writtenMembers == 0 ||
		(record.expectedMembers != 0 && record.writtenMembers != record.expectedMembers) {
		c.dropCohortFrontierMarkIncomplete(index, false)
		return errors.New("parser-core phase zero: frontier membership is incomplete")
	}
	start := int(record.participantStart)
	end := start + int(record.expectedParticipants)
	if start < 0 || end < start || end > len(c.dropCohortFrontierParticipants) {
		c.dropCohortFrontierMarkIncomplete(index, false)
		return errors.New("parser-core phase zero: frontier participant store is incomplete")
	}
	memberCount := 0
	for participantIndex := start; participantIndex < end; participantIndex++ {
		participant := c.dropCohortFrontierParticipants[participantIndex]
		if participant.head.Node == 0 || participant.memberCount == 0 ||
			int(participant.memberStart) < 0 || int(participant.memberStart)+int(participant.memberCount) > len(c.dropCohortFrontierMembers) {
			c.dropCohortFrontierMarkIncomplete(index, false)
			return errors.New("parser-core phase zero: frontier participant record is incomplete")
		}
		memberCount += int(participant.memberCount)
		for memberIndex := int(participant.memberStart); memberIndex < int(participant.memberStart)+int(participant.memberCount); memberIndex++ {
			if !c.dropCohortFrontierMemberValid(owner, c.dropCohortFrontierMembers[memberIndex]) {
				c.dropCohortFrontierMarkIncomplete(index, false)
				return errors.New("parser-core phase zero: frontier member identity is invalid")
			}
		}
	}
	if memberCount != int(record.writtenMembers) {
		c.dropCohortFrontierMarkIncomplete(index, false)
		return errors.New("parser-core phase zero: frontier member count is incomplete")
	}
	seal, ok := c.dropCohortFrontierSealOwned(owner, index)
	if !ok {
		c.dropCohortFrontierMarkIncomplete(index, false)
		return errors.New("parser-core phase zero: frontier seal could not be built")
	}
	record.seal = seal
	record.state = DropCohortFrontierComplete
	return nil
}

// FinalizeDropCohortFrontierOwned seals one complete frontier. No header
// reference is published by this method.
func (c *Core) FinalizeDropCohortFrontierOwned(owner SchedulerTransactionToken, frontier DropCohortFrontierHandle) (err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	err = c.finalizeDropCohortFrontierOwned(owner, frontier)
	return c.finishSchedulerOwned(owner, err)
}

// ValidateDropCohortFrontierOwned rechecks every source record and member
// header. It fails closed when a source action or derivation byte changes.
func (c *Core) ValidateDropCohortFrontierOwned(owner SchedulerTransactionToken, frontier DropCohortFrontierHandle, heads []Head) (err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if err = c.validateSchedulerTransaction(owner); err != nil {
		return c.finishSchedulerOwned(owner, err)
	}
	index, lookupErr := c.validateDropCohortFrontierIdentity(frontier)
	if lookupErr != nil {
		return c.finishSchedulerOwned(owner, lookupErr)
	}
	record := c.dropCohortFrontiers[index]
	if record.state != DropCohortFrontierComplete {
		return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: frontier is not complete"))
	}
	if len(heads) != int(record.expectedParticipants) {
		return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: frontier header count mismatch"))
	}
	start := int(record.participantStart)
	end := start + int(record.expectedParticipants)
	if start < 0 || end < start || end > len(c.dropCohortFrontierParticipants) {
		return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: frontier participant store is incomplete"))
	}
	for participantIndex, head := range heads {
		participant := c.dropCohortFrontierParticipants[start+participantIndex]
		if participant.head != head {
			return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: frontier header identity mismatch"))
		}
		memberStart := int(participant.memberStart)
		memberEnd := memberStart + int(participant.memberCount)
		if memberStart < 0 || memberEnd < memberStart || memberEnd > len(c.dropCohortFrontierMembers) {
			return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: frontier member store is incomplete"))
		}
		for memberIndex := memberStart; memberIndex < memberEnd; memberIndex++ {
			if !c.dropCohortFrontierMemberValid(owner, c.dropCohortFrontierMembers[memberIndex]) {
				return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: frontier member identity is invalid"))
			}
		}
	}
	if seal, ok := c.dropCohortFrontierSealOwned(owner, index); !ok || seal != record.seal {
		return c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: frontier seal mismatch"))
	}
	return nil
}

func (c *Core) rollbackDropCohortFrontierPublication(frontierCount, participantCount, memberCount int, nextSequence uint64) {
	if c == nil {
		return
	}
	if frontierCount >= 0 && frontierCount <= len(c.dropCohortFrontiers) {
		clear(c.dropCohortFrontiers[frontierCount:])
		c.dropCohortFrontiers = c.dropCohortFrontiers[:frontierCount]
	}
	if participantCount >= 0 && participantCount <= len(c.dropCohortFrontierParticipants) {
		clear(c.dropCohortFrontierParticipants[participantCount:])
		c.dropCohortFrontierParticipants = c.dropCohortFrontierParticipants[:participantCount]
	}
	if memberCount >= 0 && memberCount <= len(c.dropCohortFrontierMembers) {
		clear(c.dropCohortFrontierMembers[memberCount:])
		c.dropCohortFrontierMembers = c.dropCohortFrontierMembers[:memberCount]
	}
	c.dropCohortFrontierNextSequence = nextSequence
}

// PublishDropCohortFrontierOwned records one complete scheduler frontier.
// Incomplete source facts return complete=false without poisoning the owner;
// the caller keeps its existing fail-closed route.
func (c *Core) PublishDropCohortFrontierOwned(
	owner SchedulerTransactionToken,
	electionSequence uint64,
	token DropCohortFrontierToken,
	heads []Head,
	branchOrders []uint64,
	refs []DropCohortRefSet,
) (handle DropCohortFrontierHandle, complete bool, err error) {
	if err = c.validateSchedulerTransaction(owner); err != nil {
		return DropCohortFrontierHandle{}, false, err
	}
	if len(heads) == 0 || len(heads) != len(branchOrders) || len(heads) != len(refs) {
		return DropCohortFrontierHandle{}, false, nil
	}
	frontierCount := len(c.dropCohortFrontiers)
	participantCount := len(c.dropCohortFrontierParticipants)
	memberCount := len(c.dropCohortFrontierMembers)
	previousSequence := c.dropCohortFrontierNextSequence
	var expectedMembers int
	for index := range refs {
		count, valid := c.dropCohortRefCount(refs[index])
		if !valid || count <= 0 || count > dropCohortRefHardCap {
			return DropCohortFrontierHandle{}, false, nil
		}
		expectedMembers += count
		if expectedMembers > dropCohortFrontierMemberHardCap {
			return DropCohortFrontierHandle{}, false, nil
		}
	}
	handle, err = c.beginDropCohortFrontierOwned(owner, electionSequence, token, len(heads), expectedMembers)
	if err != nil {
		return DropCohortFrontierHandle{}, false, nil
	}
	for index := range heads {
		if writeErr := c.writeDropCohortFrontierParticipantOwned(owner, handle, uint16(index), heads[index], branchOrders[index], refs[index]); writeErr != nil {
			c.rollbackDropCohortFrontierPublication(frontierCount, participantCount, memberCount, previousSequence)
			return DropCohortFrontierHandle{}, false, nil
		}
	}
	if finalizeErr := c.finalizeDropCohortFrontierOwned(owner, handle); finalizeErr != nil {
		c.rollbackDropCohortFrontierPublication(frontierCount, participantCount, memberCount, previousSequence)
		return DropCohortFrontierHandle{}, false, nil
	}
	return handle, true, nil
}

// DropCohortFrontierStateOwned returns a bounded lifecycle view for focused
// tests. The owner token is checked before the frontier store is read.
func (c *Core) DropCohortFrontierStateOwned(owner SchedulerTransactionToken, handle DropCohortFrontierHandle) (state DropCohortFrontierState, expected, written uint16, members uint32, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return 0, 0, 0, 0, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if err = c.validateSchedulerTransaction(owner); err != nil {
		return 0, 0, 0, 0, c.finishSchedulerOwned(owner, err)
	}
	index, err := c.validateDropCohortFrontierIdentity(handle)
	if err != nil {
		return 0, 0, 0, 0, c.finishSchedulerOwned(owner, err)
	}
	record := c.dropCohortFrontiers[index]
	return record.state, record.expectedParticipants, record.writtenParticipants, record.writtenMembers, nil
}

type dropCohortFrontierProtocolRecord struct {
	Handle               [3]uint64                               `json:"handle"`
	ElectionSequence     uint64                                  `json:"election_sequence"`
	State                string                                  `json:"state"`
	ExpectedParticipants uint16                                  `json:"expected_participants"`
	WrittenParticipants  uint16                                  `json:"written_participants"`
	WrittenMembers       uint32                                  `json:"written_members"`
	Seal                 [32]byte                                `json:"seal"`
	Token                DropCohortFrontierToken                 `json:"token"`
	Participants         []dropCohortFrontierProtocolParticipant `json:"participants"`
}

type dropCohortFrontierProtocolParticipant struct {
	Head           Head                               `json:"head"`
	BranchOrder    uint64                             `json:"branch_order"`
	ReferenceFlags uint8                              `json:"reference_flags"`
	MemberCount    uint16                             `json:"member_count"`
	Members        []dropCohortFrontierProtocolMember `json:"members"`
}

type dropCohortFrontierProtocolMember struct {
	Participant          uint16                     `json:"participant"`
	Ref                  DropCohortRef              `json:"ref"`
	ParticipantHead      Head                       `json:"participant_head"`
	SourceHead           Head                       `json:"source_head"`
	BranchOrder          uint64                     `json:"branch_order"`
	Action               DropCohortActionIdentity   `json:"action"`
	Derivation           DropCohortDerivationHandle `json:"derivation"`
	DerivationDigest     [32]byte                   `json:"derivation_digest"`
	DerivationLength     uint32                     `json:"derivation_length"`
	DerivationRootSymbol Symbol                     `json:"derivation_root_symbol"`
	DerivationStackDepth uint32                     `json:"derivation_stack_depth"`
	DerivationCheckpoint DropCohortSourceCheckpoint `json:"derivation_checkpoint"`
}

type dropCohortFrontierProtocolSnapshot struct {
	Schema    string                             `json:"schema"`
	Frontiers []dropCohortFrontierProtocolRecord `json:"frontiers"`
}

// DiagnosticDropCohortFrontierSnapshotOwnedForTest exposes the latest
// immutable frontier metadata after validating the scheduler token.
func (c *Core) DiagnosticDropCohortFrontierSnapshotOwnedForTest(owner SchedulerTransactionToken) []byte {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return []byte(`{"schema":"gts-drop-cohort-frontier/v1"}`)
	}
	snapshot := dropCohortFrontierProtocolSnapshot{Schema: "gts-drop-cohort-frontier/v1"}
	if len(c.dropCohortFrontiers) != 0 {
		record := c.dropCohortFrontiers[len(c.dropCohortFrontiers)-1]
		protocol := dropCohortFrontierProtocolRecord{
			Handle: dropCohortFrontierProtocolHandle(record.handle), ElectionSequence: record.electionSequence,
			State: dropCohortFrontierProtocolState(record.state), ExpectedParticipants: record.expectedParticipants,
			WrittenParticipants: record.writtenParticipants, WrittenMembers: record.writtenMembers, Seal: record.seal,
			Token: record.token,
		}
		participantStart := int(record.participantStart)
		participantEnd := participantStart + int(record.writtenParticipants)
		memberStart := int(record.memberStart)
		memberEnd := memberStart + int(record.writtenMembers)
		if participantStart < 0 || participantEnd < participantStart || participantEnd > len(c.dropCohortFrontierParticipants) ||
			memberStart < 0 || memberEnd < memberStart || memberEnd > len(c.dropCohortFrontierMembers) {
			return []byte(`{"schema":"gts-drop-cohort-frontier/v1"}`)
		}
		for participantIndex := participantStart; participantIndex < participantEnd; participantIndex++ {
			participant := c.dropCohortFrontierParticipants[participantIndex]
			start := int(participant.memberStart)
			end := start + int(participant.memberCount)
			if start < memberStart || end < start || end > memberEnd {
				return []byte(`{"schema":"gts-drop-cohort-frontier/v1"}`)
			}
			protocolParticipant := dropCohortFrontierProtocolParticipant{
				Head: participant.head, BranchOrder: participant.branchOrder,
				ReferenceFlags: participant.referenceFlags, MemberCount: participant.memberCount,
			}
			for memberIndex := start; memberIndex < end; memberIndex++ {
				member := c.dropCohortFrontierMembers[memberIndex]
				protocolParticipant.Members = append(protocolParticipant.Members, dropCohortFrontierProtocolMember{
					Participant: member.participant, Ref: member.ref, ParticipantHead: member.participantHead,
					SourceHead: member.sourceHead, BranchOrder: member.branchOrder, Action: member.action,
					Derivation: member.derivation, DerivationDigest: member.derivationDigest,
					DerivationLength: member.derivationLength, DerivationRootSymbol: member.derivationRootSymbol,
					DerivationStackDepth: member.derivationStackDepth, DerivationCheckpoint: member.derivationCheckpoint,
				})
			}
			protocol.Participants = append(protocol.Participants, protocolParticipant)
		}
		snapshot.Frontiers = append(snapshot.Frontiers, protocol)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return []byte(`{"schema":"gts-drop-cohort-frontier/v1"}`)
	}
	return encoded
}

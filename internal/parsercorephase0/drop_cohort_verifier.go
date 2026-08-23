package parsercorephase0

import (
	"encoding/binary"
	"errors"
)

// dropCohortVerifierReason is the stable classification emitted by the inert
// verifier seam. Keep the values stable because receipts use their strings.
type dropCohortVerifierReason uint8

const (
	dropCohortVerifierProved dropCohortVerifierReason = iota
	dropCohortVerifierForeignOwner
	dropCohortVerifierStaleEpoch
	dropCohortVerifierUnknownCohort
	dropCohortVerifierCertificateBuilding
	dropCohortVerifierCertificateOverflowed
	dropCohortVerifierCertificateBlended
	dropCohortVerifierCertificateUnproved
	dropCohortVerifierForeignSequence
	dropCohortVerifierActionMismatch
	dropCohortVerifierDerivationMismatch
	dropCohortVerifierHeadMismatch
	dropCohortVerifierInvalidDrop
	dropCohortVerifierReasonCount
)

var dropCohortVerifierErrors = [...]error{
	nil,
	errors.New("parser-core phase zero: foreign arena owner"),
	errors.New("parser-core phase zero: stale arena epoch"),
	errors.New("parser-core phase zero: unknown cohort"),
	errors.New("parser-core phase zero: certificate is building"),
	errors.New("parser-core phase zero: certificate is overflowed"),
	errors.New("parser-core phase zero: certificate is blended"),
	errors.New("parser-core phase zero: certificate is unproved"),
	errors.New("parser-core phase zero: foreign cohort sequence"),
	errors.New("parser-core phase zero: action identity mismatch"),
	errors.New("parser-core phase zero: derivation identity mismatch"),
	errors.New("parser-core phase zero: head identity mismatch"),
	errors.New("parser-core phase zero: invalid drop"),
}

func dropCohortVerifierReasonString(reason dropCohortVerifierReason) string {
	switch reason {
	case dropCohortVerifierProved:
		return "proved"
	case dropCohortVerifierForeignOwner:
		return "foreign_arena_owner"
	case dropCohortVerifierStaleEpoch:
		return "stale_arena_epoch"
	case dropCohortVerifierUnknownCohort:
		return "unknown_cohort"
	case dropCohortVerifierCertificateBuilding:
		return "certificate_building"
	case dropCohortVerifierCertificateOverflowed:
		return "certificate_overflowed"
	case dropCohortVerifierCertificateBlended:
		return "certificate_blended"
	case dropCohortVerifierCertificateUnproved:
		return "certificate_unproved"
	case dropCohortVerifierForeignSequence:
		return "foreign_cohort_sequence"
	case dropCohortVerifierActionMismatch:
		return "action_identity_mismatch"
	case dropCohortVerifierDerivationMismatch:
		return "derivation_identity_mismatch"
	case dropCohortVerifierHeadMismatch:
		return "head_identity_mismatch"
	case dropCohortVerifierInvalidDrop:
		return "invalid_drop"
	default:
		return "unknown_cohort"
	}
}

func dropCohortVerifierError(reason dropCohortVerifierReason) error {
	if reason >= dropCohortVerifierReasonCount {
		return dropCohortVerifierErrors[dropCohortVerifierUnknownCohort]
	}
	return dropCohortVerifierErrors[reason]
}

func dropCohortVerifierIncrement(value *uint64) {
	if value != nil && *value != ^uint64(0) {
		(*value)++
	}
}

func (c *Core) dropCohortVerifierLookup(ref DropCohortRef, count bool) (int, DropCohortHandle, dropCohortVerifierReason) {
	handle := DropCohortHandle{Owner: ref.Owner, Epoch: ref.Epoch, Sequence: ref.Sequence}
	if c == nil {
		return -1, handle, dropCohortVerifierUnknownCohort
	}
	if count {
		dropCohortVerifierIncrement(&c.dropCohortOwnerCheckedLookups)
	}
	// Check the arena owner and epoch before reading any certificate store.
	if ref.Owner == 0 || ref.Owner != c.dropCohortOwner {
		return -1, handle, dropCohortVerifierForeignOwner
	}
	if ref.Epoch == 0 || ref.Epoch != c.dropCohortEpoch {
		return -1, handle, dropCohortVerifierStaleEpoch
	}
	if ref.Sequence == 0 {
		return -1, handle, dropCohortVerifierUnknownCohort
	}
	for index := range c.dropCohortRecords {
		if c.dropCohortRecords[index].handle != handle {
			continue
		}
		if count {
			if c.dropCohortRecords[index].expected > dropCohortRefInlineCapacity {
				dropCohortVerifierIncrement(&c.dropCohortSpillReads)
			} else {
				dropCohortVerifierIncrement(&c.dropCohortInlineReads)
			}
		}
		return index, handle, dropCohortVerifierProved
	}
	return -1, handle, dropCohortVerifierUnknownCohort
}

func (c *Core) dropCohortVerifierMember(record dropCohortRecord, head Head, branch uint16) (dropCohortMember, dropCohortVerifierReason) {
	index, found := c.findDropCohortMember(record, branch)
	if !found {
		return dropCohortMember{}, dropCohortVerifierUnknownCohort
	}
	member := c.dropCohortMembers[index]
	if member.head != head {
		return dropCohortMember{}, dropCohortVerifierHeadMismatch
	}
	return member, dropCohortVerifierProved
}

func (c *Core) dropCohortVerifierDerivationEqual(left, right DropCohortDerivationHandle, count bool) bool {
	if left.Owner != c.dropCohortOwner || right.Owner != c.dropCohortOwner ||
		left.Epoch != c.dropCohortEpoch || right.Epoch != c.dropCohortEpoch ||
		left.Index == 0 || right.Index == 0 ||
		uint64(left.Index) > uint64(len(c.dropCohortDerivations)) ||
		uint64(right.Index) > uint64(len(c.dropCohortDerivations)) {
		return false
	}
	if count {
		dropCohortVerifierIncrement(&c.dropCohortMapReads)
		dropCohortVerifierIncrement(&c.dropCohortInternerReads)
	}
	leftRecord := c.dropCohortDerivations[left.Index-1]
	rightRecord := c.dropCohortDerivations[right.Index-1]
	if leftRecord.digest != rightRecord.digest || leftRecord.byteLength != rightRecord.byteLength || leftRecord.rootSymbol != rightRecord.rootSymbol ||
		leftRecord.stackDepth != rightRecord.stackDepth || leftRecord.checkpoint != rightRecord.checkpoint {
		return false
	}
	leftStart := int(leftRecord.byteOffset)
	rightStart := int(rightRecord.byteOffset)
	length := int(leftRecord.byteLength)
	if leftStart < 0 || rightStart < 0 || length < 0 ||
		leftStart > len(c.dropCohortDerivationBytes)-length ||
		rightStart > len(c.dropCohortDerivationBytes)-length {
		return false
	}
	return bytesCompare(
		c.dropCohortDerivationBytes[leftStart:leftStart+length],
		c.dropCohortDerivationBytes[rightStart:rightStart+length],
	) == 0
}

func (c *Core) dropCohortVerifierPublish(reason dropCohortVerifierReason) {
	dropCohortVerifierIncrement(&c.dropCohortVerifierElections)
	if reason == dropCohortVerifierProved {
		dropCohortVerifierIncrement(&c.dropCohortVerifierProofs)
		return
	}
	dropCohortVerifierIncrement(&c.dropCohortVerifierDeclines)
	if reason < dropCohortVerifierReasonCount {
		dropCohortVerifierIncrement(&c.dropCohortDeclineReasons[reason])
	}
	if reason == dropCohortVerifierActionMismatch {
		dropCohortVerifierIncrement(&c.dropCohortActionDeclines)
	}
	if reason == dropCohortVerifierDerivationMismatch {
		dropCohortVerifierIncrement(&c.dropCohortDerivationDeclines)
	}
}

func dropCohortVerifierIsDropped(index int, drops []int) bool {
	for _, drop := range drops {
		if drop == index {
			return true
		}
	}
	return false
}

func (c *Core) dropCohortVerifierResult(reason dropCohortVerifierReason, publish bool) dropCohortVerifierReason {
	if publish {
		c.dropCohortVerifierPublish(reason)
	}
	return reason
}

func dropCohortVerifierValidateInputs(heads []Head, refs []DropCohortRef, drops []int) (int, dropCohortVerifierReason) {
	if len(heads) == 0 || len(heads) != len(refs) || len(drops) == 0 || len(drops) >= len(heads) {
		return -1, dropCohortVerifierInvalidDrop
	}
	for index, drop := range drops {
		if drop < 0 || drop >= len(heads) || heads[drop].Node == 0 || dropCohortVerifierIsDropped(drop, drops[:index]) {
			return -1, dropCohortVerifierInvalidDrop
		}
	}
	for index := range heads {
		if !dropCohortVerifierIsDropped(index, drops) {
			if heads[index].Node == 0 {
				return -1, dropCohortVerifierInvalidDrop
			}
			return index, dropCohortVerifierProved
		}
	}
	return -1, dropCohortVerifierInvalidDrop
}

func dropCohortVerifierClassifyRecord(record dropCohortRecord) dropCohortVerifierReason {
	switch record.state {
	case DropCohortBuilding:
		return dropCohortVerifierCertificateBuilding
	case DropCohortOverflowed:
		return dropCohortVerifierCertificateOverflowed
	case DropCohortBlended:
		return dropCohortVerifierCertificateBlended
	case DropCohortUnproved:
		return dropCohortVerifierCertificateUnproved
	case DropCohortComplete:
		if record.expected == 0 || record.written != record.expected {
			return dropCohortVerifierCertificateBuilding
		}
		return dropCohortVerifierProved
	default:
		return dropCohortVerifierUnknownCohort
	}
}

func (c *Core) dropCohortVerifierSurvivor(head Head, ref DropCohortRef, count bool) (DropCohortHandle, dropCohortMember, dropCohortVerifierReason) {
	index, handle, reason := c.dropCohortVerifierLookup(ref, count)
	if reason != dropCohortVerifierProved {
		return handle, dropCohortMember{}, reason
	}
	record := c.dropCohortRecords[index]
	if reason = dropCohortVerifierClassifyRecord(record); reason != dropCohortVerifierProved {
		return handle, dropCohortMember{}, reason
	}
	member, reason := c.dropCohortVerifierMember(record, head, ref.Branch)
	return handle, member, reason
}

func (c *Core) dropCohortVerifierCandidate(head Head, ref DropCohortRef, survivorHandle DropCohortHandle, survivorMember dropCohortMember, count bool) dropCohortVerifierReason {
	index, candidateHandle, reason := c.dropCohortVerifierLookup(ref, count)
	if reason != dropCohortVerifierProved {
		return reason
	}
	candidateRecord := c.dropCohortRecords[index]
	if reason = dropCohortVerifierClassifyRecord(candidateRecord); reason != dropCohortVerifierProved {
		return reason
	}
	if candidateHandle != survivorHandle {
		return dropCohortVerifierForeignSequence
	}
	candidateMember, reason := c.dropCohortVerifierMember(candidateRecord, head, ref.Branch)
	if reason != dropCohortVerifierProved {
		return reason
	}
	if candidateMember.action != survivorMember.action {
		return dropCohortVerifierActionMismatch
	}
	if !c.dropCohortVerifierDerivationEqual(survivorMember.derivation, candidateMember.derivation, count) {
		return dropCohortVerifierDerivationMismatch
	}
	return dropCohortVerifierProved
}

func (c *Core) dropCohortVerifyRefs(heads []Head, refs []DropCohortRef, drops []int, publish bool) dropCohortVerifierReason {
	if c == nil {
		return dropCohortVerifierUnknownCohort
	}
	survivorIndex, reason := dropCohortVerifierValidateInputs(heads, refs, drops)
	if reason != dropCohortVerifierProved {
		return c.dropCohortVerifierResult(reason, publish)
	}
	survivorHandle, survivorMember, reason := c.dropCohortVerifierSurvivor(heads[survivorIndex], refs[survivorIndex], publish)
	if reason != dropCohortVerifierProved {
		return c.dropCohortVerifierResult(reason, publish)
	}
	for _, drop := range drops {
		if heads[drop] == heads[survivorIndex] {
			return c.dropCohortVerifierResult(dropCohortVerifierInvalidDrop, publish)
		}
		reason = c.dropCohortVerifierCandidate(heads[drop], refs[drop], survivorHandle, survivorMember, publish)
		if reason != dropCohortVerifierProved {
			return c.dropCohortVerifierResult(reason, publish)
		}
	}
	return c.dropCohortVerifierResult(dropCohortVerifierProved, publish)
}

func (c *Core) dropCohortVerifyRefsOwned(
	owner SchedulerTransactionToken,
	heads []Head,
	refs []DropCohortRef,
	drops []int,
	publish bool,
) (dropCohortVerifierReason, error) {
	if c == nil {
		return dropCohortVerifierUnknownCohort, errors.New("parser-core phase zero: verifier on nil core")
	}
	// Validate the capability before any certificate lookup. The verifier's
	// identity checks protect the certificate store after this gate.
	if err := c.validateSchedulerTransaction(owner); err != nil {
		return dropCohortVerifierUnknownCohort, err
	}
	return c.dropCohortVerifyRefs(heads, refs, drops, publish), nil
}

// ClassifyDropCohortRefsOwned checks one candidate without publishing
// counters. It validates the active scheduler owner before reading records.
func (c *Core) ClassifyDropCohortRefsOwned(
	owner SchedulerTransactionToken,
	heads []Head,
	refs []DropCohortRef,
	drops []int,
) (string, error) {
	reason, err := c.dropCohortVerifyRefsOwned(owner, heads, refs, drops, false)
	return dropCohortVerifierReasonString(reason), err
}

// VerifyDropCohortRefsOwned checks one selected candidate and publishes one
// verifier result. It validates the active scheduler owner before any lookup.
func (c *Core) VerifyDropCohortRefsOwned(
	owner SchedulerTransactionToken,
	heads []Head,
	refs []DropCohortRef,
	drops []int,
) error {
	reason, err := c.dropCohortVerifyRefsOwned(owner, heads, refs, drops, true)
	if err != nil {
		return err
	}
	return dropCohortVerifierError(reason)
}

// DiagnosticVerifyDropCohortRefsForTest exposes only the inert verifier
// foundation. It does not compact headers or authorize a production drop.
func (c *Core) DiagnosticVerifyDropCohortRefsForTest(heads []Head, refs []DropCohortRef, drops []int) (string, error) {
	reason := c.dropCohortVerifyRefs(heads, refs, drops, true)
	return dropCohortVerifierReasonString(reason), dropCohortVerifierError(reason)
}

// DiagnosticVerifyDropCohortRefsNonDestructiveForTest evaluates the same
// predicate without counters, storage-read telemetry, or arena mutation.
func (c *Core) DiagnosticVerifyDropCohortRefsNonDestructiveForTest(heads []Head, refs []DropCohortRef, drops []int) (string, error) {
	reason := c.dropCohortVerifyRefs(heads, refs, drops, false)
	return dropCohortVerifierReasonString(reason), dropCohortVerifierError(reason)
}

// DiagnosticDropCohortVerifierStateDigestForTest returns a fixed-width digest
// of verifier counters. The non-destructive seam leaves this value unchanged.
func (c *Core) DiagnosticDropCohortVerifierStateDigestForTest() [32]byte {
	var digest [32]byte
	if c == nil {
		return digest
	}
	binary.LittleEndian.PutUint64(digest[0:8], c.dropCohortVerifierElections)
	binary.LittleEndian.PutUint64(digest[8:16], c.dropCohortVerifierProofs)
	binary.LittleEndian.PutUint64(digest[16:24], c.dropCohortVerifierDeclines)
	binary.LittleEndian.PutUint64(digest[24:32], c.dropCohortOwnerCheckedLookups)
	return digest
}

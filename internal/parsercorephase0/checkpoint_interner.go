package parsercorephase0

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"hash/maphash"
	"math"
)

var emptyCheckpointDigest = sha256.Sum256(nil)

// The immutable seed is shared so parsers without external checkpoints do not
// carry an extra field. Exact byte comparisons still establish identity.
var checkpointHashSeed = maphash.MakeSeed()

// CheckpointID is a compact, core-local identity for one exact serialized
// external-scanner state. ID zero is reserved for the empty checkpoint.
type CheckpointID uint32

// CheckpointInternerStats reports logical interner use. Both dimensions are
// independently bounded by Limits.
type CheckpointInternerStats struct {
	Unique          uint32
	SerializedBytes uint64
	// DigestCollisions counts distinct checkpoints sharing a bucket hash.
	// The field name is retained for existing diagnostic consumers.
	DigestCollisions uint64
}

type checkpointRecord struct {
	digest      [32]byte
	offset      uint32
	length      uint32
	next        CheckpointID
	digestValid bool
}

type checkpointInterner struct {
	records    []checkpointRecord
	bytes      []byte
	buckets    map[uint64]CheckpointID
	maxIDs     uint32
	maxBytes   uint64
	collisions uint64
	// lastInterned is the identity intern returned most recently. Scanner
	// state rarely changes between consecutive tokens, so a byte comparison
	// against that record answers most interns without a hash.
	lastInterned CheckpointID
}

func newCheckpointInterner(maxIDs uint32, maxBytes uint64) checkpointInterner {
	return checkpointInterner{maxIDs: maxIDs, maxBytes: maxBytes}
}

func (i *checkpointInterner) intern(serialized []byte) (CheckpointID, error) {
	if len(serialized) == 0 {
		return 0, nil
	}
	if id := i.lastInterned; id != 0 {
		if record, ok := i.record(id); ok {
			start := uint64(record.offset)
			end := start + uint64(record.length)
			if end <= uint64(len(i.bytes)) && bytes.Equal(i.bytes[start:end], serialized) {
				return id, nil
			}
		}
	}
	id, err := i.internHash(serialized, maphash.Bytes(checkpointHashSeed, serialized))
	if err != nil {
		return 0, err
	}
	i.lastInterned = id
	return id, nil
}

// internHash is the collision-test seam. The hash selects a bucket; only a
// complete byte comparison establishes checkpoint identity.
func (i *checkpointInterner) internHash(serialized []byte, hash uint64) (CheckpointID, error) {
	if len(serialized) == 0 {
		return 0, nil
	}
	bucket := i.buckets[hash]
	for id := bucket; id != 0; {
		record, ok := i.record(id)
		if !ok {
			return 0, errors.New("parser-core phase zero: corrupt checkpoint interner chain")
		}
		start := uint64(record.offset)
		end := start + uint64(record.length)
		if end <= uint64(len(i.bytes)) && bytes.Equal(i.bytes[start:end], serialized) {
			return id, nil
		}
		id = record.next
	}
	if uint64(len(i.records)) >= uint64(i.maxIDs) {
		return 0, errors.New("parser-core phase zero: checkpoint identity cap")
	}
	if uint64(len(serialized)) > math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: checkpoint length overflow")
	}
	if uint64(len(i.bytes)) > i.maxBytes || uint64(len(serialized)) > i.maxBytes-uint64(len(i.bytes)) {
		return 0, errors.New("parser-core phase zero: checkpoint byte cap")
	}
	if uint64(len(i.bytes))+uint64(len(serialized)) > uint64(^uint32(0)) {
		return 0, errors.New("parser-core phase zero: checkpoint byte offset overflow")
	}
	if i.buckets == nil {
		i.buckets = make(map[uint64]CheckpointID)
	}
	id := CheckpointID(len(i.records) + 1)
	offset := uint32(len(i.bytes))
	i.bytes = append(i.bytes, serialized...)
	i.records = append(i.records, checkpointRecord{
		offset: offset, length: uint32(len(serialized)), next: bucket,
	})
	i.buckets[hash] = id
	if bucket != 0 {
		i.collisions++
	}
	return id, nil
}

func (i *checkpointInterner) record(id CheckpointID) (checkpointRecord, bool) {
	if id == 0 || uint64(id) > uint64(len(i.records)) {
		return checkpointRecord{}, false
	}
	return i.records[id-1], true
}

func (i *checkpointInterner) receipt(id CheckpointID) (uint32, [32]byte, bool) {
	if id == 0 {
		return 0, emptyCheckpointDigest, true
	}
	record, ok := i.record(id)
	if !ok {
		return 0, [32]byte{}, false
	}
	if !record.digestValid {
		start := uint64(record.offset)
		end := start + uint64(record.length)
		if end > uint64(len(i.bytes)) {
			return 0, [32]byte{}, false
		}
		// SHA-256 authenticates receipts, never the interner lookup. Cache it
		// on first receipt so repeated diagnostic requests do not rehash.
		record.digest = sha256.Sum256(i.bytes[start:end])
		record.digestValid = true
		i.records[id-1] = record
	}
	return record.length, record.digest, true
}

// matches reports whether serialized equals the retained bytes of id. It is
// the byte-exact form of comparing a receipt digest against a fresh digest of
// serialized, without computing that digest. Checkpoint zero matches only the
// empty checkpoint.
func (i *checkpointInterner) matches(id CheckpointID, serialized []byte) bool {
	if id == 0 {
		return len(serialized) == 0
	}
	record, ok := i.record(id)
	if !ok {
		return false
	}
	start := uint64(record.offset)
	end := start + uint64(record.length)
	if end < start || end > uint64(len(i.bytes)) {
		return false
	}
	return bytes.Equal(i.bytes[start:end], serialized)
}

// copyBytes copies one exact serialized checkpoint into dst. It reuses dst's
// backing array when its capacity is sufficient and never exposes i.bytes.
// Checkpoint zero is the valid empty checkpoint; every other ID must resolve
// to a complete retained record.
func (i *checkpointInterner) copyBytes(id CheckpointID, dst []byte) ([]byte, bool) {
	if id == 0 {
		return dst[:0], true
	}
	record, ok := i.record(id)
	if !ok {
		return nil, false
	}
	start := uint64(record.offset)
	end := start + uint64(record.length)
	if end < start || end > uint64(len(i.bytes)) {
		return nil, false
	}
	length := int(record.length)
	if cap(dst) < length {
		dst = make([]byte, length)
	} else {
		dst = dst[:length]
	}
	copy(dst, i.bytes[int(start):int(end)])
	return dst, true
}

func (i *checkpointInterner) stats() CheckpointInternerStats {
	return CheckpointInternerStats{Unique: uint32(len(i.records)), SerializedBytes: uint64(len(i.bytes)), DigestCollisions: i.collisions}
}

func (i *checkpointInterner) reset() {
	i.records = i.records[:0]
	i.bytes = i.bytes[:0]
	i.lastInterned = 0
	clear(i.buckets)
	i.collisions = 0
}

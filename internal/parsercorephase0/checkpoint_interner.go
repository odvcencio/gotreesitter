package parsercorephase0

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"hash/maphash"
	"math"
)

var emptyCheckpointDigest = sha256.Sum256(nil)

// CheckpointID is a compact, core-local identity for one exact serialized
// external-scanner state. ID zero is reserved for the empty checkpoint.
type CheckpointID uint32

// CheckpointInternerStats reports logical interner use. Both dimensions are
// independently bounded by Limits.
type CheckpointInternerStats struct {
	Unique          uint32
	SerializedBytes uint64
	// DigestCollisions counts collisions in the lookup hash, not the receipt digest.
	DigestCollisions uint64
}

type checkpointRecord struct {
	offset uint32
	length uint32
	next   CheckpointID
}

// Checkpoint receipts are immutable; this cache changes only while interning.
// Concurrent receipt readers never publish cache entries.
type checkpointDigestCacheEntry struct {
	id             CheckpointID
	offset, length uint32
	digest         [32]byte
}

type checkpointInterner struct {
	receipts   []checkpointDigestCacheEntry
	records    []checkpointRecord
	bytes      []byte
	buckets    map[uint64]CheckpointID
	hashSeed   maphash.Seed
	maxIDs     uint32
	maxBytes   uint64
	collisions uint64
	// lastInterned is the identity intern returned most recently. Scanner
	// state rarely changes between consecutive tokens, so a byte comparison
	// against that record answers most interns without a digest.
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
				i.cacheReceipt(id, record, serialized)
				return id, nil
			}
		}
	}
	if i.hashSeed == (maphash.Seed{}) {
		i.hashSeed = maphash.MakeSeed()
	}
	id, err := i.internHash(serialized, maphash.Bytes(i.hashSeed, serialized))
	if err != nil {
		return 0, err
	}
	i.lastInterned = id
	return id, nil
}

// internHash confirms the complete serialized bytes after hash bucketing.
// Interning fills the small digest cache without mutating receipt readers.
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
			i.cacheReceipt(id, record, serialized)
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
	record := checkpointRecord{offset: offset, length: uint32(len(serialized)), next: bucket}
	i.records = append(i.records, record)
	i.cacheReceipt(id, record, serialized)
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
	digest, exact := i.recordDigest(id, record)
	return record.length, digest, exact
}

// cacheReceipt is called only by interning, which already mutates the owner.
// Full SHA-256 receipts remain exact after eviction; they are recomputed from
// the immutable serialized bytes rather than stored on every record.
func (i *checkpointInterner) cacheReceipt(id CheckpointID, record checkpointRecord, serialized []byte) {
	if len(i.receipts) == 0 {
		i.receipts = make([]checkpointDigestCacheEntry, 4)
	}
	entry := &i.receipts[uint32(id)%uint32(len(i.receipts))]
	if entry.id == id && entry.offset == record.offset && entry.length == record.length {
		return
	}
	*entry = checkpointDigestCacheEntry{id: id, offset: record.offset, length: record.length, digest: sha256.Sum256(serialized)}
}

func (i *checkpointInterner) recordDigest(id CheckpointID, record checkpointRecord) ([32]byte, bool) {
	start := uint64(record.offset)
	end := start + uint64(record.length)
	if end < start || end > uint64(len(i.bytes)) {
		return [32]byte{}, false
	}
	if len(i.receipts) != 0 {
		entry := &i.receipts[uint32(id)%uint32(len(i.receipts))]
		if entry.id == id && entry.offset == record.offset && entry.length == record.length {
			return entry.digest, true
		}
	}
	return sha256.Sum256(i.bytes[start:end]), true
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
	clear(i.receipts[:])
	clear(i.buckets)
	i.collisions = 0
}

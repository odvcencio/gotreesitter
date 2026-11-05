package parsercorephase0

import "unsafe"

// A normal graph link needs three identifiers, a small signed score, and an
// ordering value. Common records keep an eight-bit signed score and a
// twenty-one-bit order; all other values retain an exact wide record.
// Keep the complete logical linkRecord at call boundaries;
// only its stored form uses this smaller representation.
type packedLinkRecord struct {
	prev       NodeID
	payload    SubtreeID
	next       LinkID
	attributes uint32
}

// Wide records also retain unknown flags. White-box corruption checks must
// see exactly the flags supplied to storage, rather than a masked subset.
type wideLinkRecord struct {
	scoreDelta int64
	order      uint64
	owner      uint32
	flags      uint32
	prev       NodeID
}

const packedLinkWide uint32 = 1 << 2
const packedLinkRecordBytes = uint64(unsafe.Sizeof(packedLinkRecord{}))
const wideLinkRecordBytes = uint64(unsafe.Sizeof(wideLinkRecord{}))

type linkRecordStorage struct {
	records []packedLinkRecord
	wide    []wideLinkRecord
}

func packLinkRecord(link linkRecord) (packedLinkRecord, bool) {
	packed := packedLinkRecord{prev: link.prev, payload: link.payload, next: link.next}
	if link.scoreDelta < -128 || link.scoreDelta > 127 || link.order >= 1<<21 || link.flags&^linkKnownFlags != 0 {
		packed.attributes = packedLinkWide
		return packed, true
	}
	packed.attributes = uint32(link.order)<<11 | uint32(uint8(int8(link.scoreDelta)))<<3 | link.flags
	return packed, false
}

func (s *linkRecordStorage) Len() int { return len(s.records) }
func (s *linkRecordStorage) Cap() int { return cap(s.records) }

func (s *linkRecordStorage) At(index int) linkRecord {
	packed := s.records[index]
	if packed.attributes&packedLinkWide != 0 {
		return s.wideAt(packed)
	}
	return linkRecord{prev: packed.prev, payload: packed.payload, next: packed.next,
		scoreDelta: int64(int8(packed.attributes >> 3)), order: uint64(packed.attributes >> 11), flags: packed.attributes & linkKnownFlags}
}

func (s *linkRecordStorage) wideAt(packed packedLinkRecord) linkRecord {
	wide := s.wide[uint32(packed.prev)]
	return linkRecord{prev: wide.prev, payload: packed.payload, next: packed.next,
		scoreDelta: wide.scoreDelta, order: wide.order, flags: wide.flags}
}

func (s *linkRecordStorage) Append(link linkRecord) {
	packed, wide := packLinkRecord(link)
	if wide {
		packed.prev = NodeID(len(s.wide))
		s.wide = append(s.wide, wideLinkRecord{scoreDelta: link.scoreDelta, order: link.order, flags: link.flags, prev: link.prev, owner: uint32(len(s.records))})
	}
	s.records = append(s.records, packed)
}

// Graph links are immutable in production. Set also supports the existing
// white-box adversarial tests and preserves ordered wide owners when a test
// changes a normal record into a wide one or back again.
func (s *linkRecordStorage) Set(index int, link linkRecord) {
	old := s.records[index]
	packed, wide := packLinkRecord(link)
	if old.attributes&packedLinkWide != 0 {
		position := int(uint32(old.prev))
		if wide {
			packed.prev = old.prev
			s.wide[position] = wideLinkRecord{scoreDelta: link.scoreDelta, order: link.order, flags: link.flags, prev: link.prev, owner: uint32(index)}
		} else {
			copy(s.wide[position:], s.wide[position+1:])
			clear(s.wide[len(s.wide)-1:])
			s.wide = s.wide[:len(s.wide)-1]
			for i := position; i < len(s.wide); i++ {
				s.records[s.wide[i].owner].prev = NodeID(i)
			}
		}
	} else if wide {
		position := s.widePrefix(index)
		s.wide = append(s.wide, wideLinkRecord{})
		copy(s.wide[position+1:], s.wide[position:])
		s.wide[position] = wideLinkRecord{scoreDelta: link.scoreDelta, order: link.order, flags: link.flags, prev: link.prev, owner: uint32(index)}
		for i := position + 1; i < len(s.wide); i++ {
			s.records[s.wide[i].owner].prev = NodeID(i)
		}
		packed.prev = NodeID(position)
	}
	s.records[index] = packed
}

func (s *linkRecordStorage) widePrefix(length int) int {
	lo, hi := 0, len(s.wide)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if uint64(s.wide[mid].owner) < uint64(length) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (s *linkRecordStorage) Truncate(length int) {
	if length < 0 || length > len(s.records) {
		panic("link record truncation outside arena")
	}
	s.wide = s.wide[:s.widePrefix(length)]
	s.records = s.records[:length]
}

func (s *linkRecordStorage) Grow(capacity int) { s.records = growArena(s.records, capacity) }
func (s *linkRecordStorage) Reserve(capacity int) {
	s.records = reserveArena(s.records, capacity)
	s.wide = s.wide[:0]
}

func (s *linkRecordStorage) FootprintBytes() uint64 {
	return uint64(cap(s.records))*packedLinkRecordBytes + uint64(cap(s.wide))*wideLinkRecordBytes
}

func (s *linkRecordStorage) GrowthFootprintBytes(capacity int) uint64 {
	if capacity <= cap(s.records) {
		return 0
	}
	return uint64(capacity-cap(s.records)) * packedLinkRecordBytes
}

func (s *linkRecordStorage) CloneActiveFootprintBytes() uint64 {
	return uint64(len(s.records))*packedLinkRecordBytes + uint64(len(s.wide))*wideLinkRecordBytes
}

func (s *linkRecordStorage) CloneActive() linkRecordStorage {
	clone := linkRecordStorage{}
	if len(s.records) > 0 {
		clone.records = make([]packedLinkRecord, len(s.records))
		copy(clone.records, s.records)
	}
	if len(s.wide) > 0 {
		clone.wide = make([]wideLinkRecord, len(s.wide))
		copy(clone.wide, s.wide)
	}
	return clone
}

func (s *linkRecordStorage) Values() []linkRecord {
	if len(s.records) == 0 {
		return nil
	}
	values := make([]linkRecord, len(s.records))
	for i := range values {
		values[i] = s.At(i)
	}
	return values
}

func linkStorageFromRecords(records []linkRecord) linkRecordStorage {
	storage := linkRecordStorage{records: make([]packedLinkRecord, 0, len(records))}
	for _, record := range records {
		storage.Append(record)
	}
	return storage
}

func makeLinkRecordStorage(length, capacity int) linkRecordStorage {
	return linkRecordStorage{records: make([]packedLinkRecord, length, capacity)}
}
func (s *linkRecordStorage) AppendValues(records ...linkRecord) {
	for _, record := range records {
		s.Append(record)
	}
}
func (s *linkRecordStorage) CopyInto(destination []linkRecord) int {
	n := min(len(destination), len(s.records))
	for i := 0; i < n; i++ {
		destination[i] = s.At(i)
	}
	return n
}

package parsercorephase0

import "unsafe"

// ScannerSnapshotStorage identifies one immutable allocation. Footprint users
// count it once even when several live records keep the same page reachable.
type ScannerSnapshotStorage struct{ bytes uint64 }

func (s *ScannerSnapshotStorage) FootprintBytes() uint64 {
	if s == nil {
		return 0
	}
	return s.bytes
}

type scannerSnapshotSlot32[T any] struct {
	record T
	bytes  [32]byte
	tried  [128]bool
}
type scannerSnapshotPage32[T any] struct {
	storage ScannerSnapshotStorage
	slots   [2]scannerSnapshotSlot32[T]
}

// ScannerSnapshotPages pairs small immutable records without recycling a
// published address. It retains only the newest page of each shape. Live record
// pointers keep older pages alive; obsolete pages can be collected immediately.
// Reset drops the current pages, so stale pointers always retain their old data.
type ScannerSnapshotPages[T any] struct {
	small      *scannerSnapshotPage32[T]
	smallNext  uint8
	packed     *scannerSnapshotPackedPage32[T]
	packedNext uint8
}

func (p *ScannerSnapshotPages[T]) Reset() {
	if p != nil {
		*p = ScannerSnapshotPages[T]{}
	}
}

// CurrentStorage includes unpublished records and unused slots in the current
// pages. Earlier pages must be charged through their live storage identities.
func (p *ScannerSnapshotPages[T]) CurrentStorage() (*ScannerSnapshotStorage, *ScannerSnapshotStorage) {
	if p == nil {
		return nil, nil
	}
	var small, wide *ScannerSnapshotStorage
	if p.small != nil {
		small = &p.small.storage
	}
	if p.packed != nil {
		wide = &p.packed.storage
	}
	return small, wide
}

// New returns a zero, unpublished record with independent frozen scanner fields.
// Shapes outside the existing coallocation contract retain the exact constructor.
// A non-nil storage identity covers the record and every returned field's backing.
func (p *ScannerSnapshotPages[T]) New(payload, start, end []byte, tried []bool) (*T, []byte, []byte, []byte, []bool, *ScannerSnapshotStorage) {
	if p == nil || len(tried) <= 64 || len(tried) > 128 || len(payload) > 64 || len(start) > 64-len(payload) || len(end) > 64-len(payload)-len(start) || len(payload)+len(start)+len(end) == 0 {
		record, a, b, c, d := NewScannerSnapshotRecord[T](payload, start, end, tried)
		return record, a, b, c, d, nil
	}
	if len(payload)+len(start)+len(end) <= 32 {
		if p.small == nil || p.smallNext == 2 {
			p.small = new(scannerSnapshotPage32[T])
			p.small.storage.bytes = uint64(unsafe.Sizeof(*p.small))
			p.smallNext = 0
		}
		slot := &p.small.slots[p.smallNext]
		p.smallNext++
		a, b, c := freezeScannerFields(slot.bytes[:], payload, start, end)
		copy(slot.tried[:], tried)
		return &slot.record, a, b, c, slot.tried[:len(tried)], &p.small.storage
	}
	record, a, b, c, d := NewScannerSnapshotRecord[T](payload, start, end, tried)
	return record, a, b, c, d, nil
}

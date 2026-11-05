package parsercorephase0

import "unsafe"

type scannerSnapshotPackedSlot32[T any] struct {
	record T
	bytes  [32]byte
}
type scannerSnapshotPackedPage32[T any] struct {
	storage ScannerSnapshotStorage
	slots   [2]scannerSnapshotPackedSlot32[T]
}

// NewPacked pairs immutable records whose trial bits live inside the record.
// Other byte shapes receive one independent exact-sized allocation strategy.
func (p *ScannerSnapshotPages[T]) NewPacked(payload, start, end []byte) (*T, []byte, []byte, []byte, *ScannerSnapshotStorage) {
	if len(payload) <= 32 && len(start) <= 32-len(payload) && len(end) <= 32-len(payload)-len(start) && len(payload)+len(start)+len(end) > 0 && p != nil {
		if p.packed == nil || p.packedNext == 2 {
			p.packed = new(scannerSnapshotPackedPage32[T])
			p.packed.storage.bytes = uint64(unsafe.Sizeof(*p.packed))
			p.packedNext = 0
		}
		slot := &p.packed.slots[p.packedNext]
		p.packedNext++
		a, b, c := freezeScannerFields(slot.bytes[:], payload, start, end)
		return &slot.record, a, b, c, &p.packed.storage
	}
	record, a, b, c := NewPackedScannerSnapshotRecord[T](payload, start, end)
	return record, a, b, c, nil
}

// NewPackedScannerSnapshotRecord freezes all byte fields without a bool array.
func NewPackedScannerSnapshotRecord[T any](payload, start, end []byte) (*T, []byte, []byte, []byte) {
	if len(payload) <= 64 && len(start) <= 64-len(payload) && len(end) <= 64-len(payload)-len(start) && len(payload)+len(start)+len(end) > 0 {
		if len(payload)+len(start)+len(end) <= 32 {
			block := new(struct {
				record T
				bytes  [32]byte
			})
			a, b, c := freezeScannerFields(block.bytes[:], payload, start, end)
			return &block.record, a, b, c
		}
		block := new(struct {
			record T
			bytes  [64]byte
		})
		a, b, c := freezeScannerFields(block.bytes[:], payload, start, end)
		return &block.record, a, b, c
	}
	return new(T), cloneSnapshotBytes(payload), cloneSnapshotBytes(start), cloneSnapshotBytes(end)
}

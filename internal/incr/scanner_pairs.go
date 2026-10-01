package incr

// EmptyScannerPairs records authenticated empty boundary pairs independently
// of absent proof. Nonempty pairs remain in their owning scanner sidecar.
type EmptyScannerPairs []uint64

func (p EmptyScannerPairs) Has(index int) bool {
	return index >= 0 && index/64 < len(p) && p[index/64]&(uint64(1)<<uint(index%64)) != 0
}

// Set returns the allocated-byte delta. Clearing an absent bit allocates
// nothing, so nonempty checkpoint recording retains its sparse storage.
func (p *EmptyScannerPairs) Set(index int, present bool) int64 {
	if index < 0 || (!present && index/64 >= len(*p)) {
		return 0
	}
	before := cap(*p)
	if needed := index/64 + 1; needed > len(*p) {
		*p = append(*p, make([]uint64, needed-len(*p))...)
	}
	mask := uint64(1) << uint(index%64)
	if present {
		(*p)[index/64] |= mask
	} else {
		(*p)[index/64] &^= mask
	}
	return int64(cap(*p)-before) * 8
}

func (p *EmptyScannerPairs) Reset() {
	clear(*p)
	*p = (*p)[:0]
}

func (p EmptyScannerPairs) Bytes() int64 { return int64(cap(p)) * 8 }

func (p EmptyScannerPairs) Capacity() uint64 { return uint64(cap(p)) * 64 }

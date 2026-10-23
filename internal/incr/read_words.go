package incr

import "unsafe"

const readWordPageShift = 14
const readWordPageSize = 1 << readWordPageShift

// readWords keeps the original small-history allocation. Once the first page
// is full, growth adds pages without copying any earlier words. The 64 KiB
// flat prefix fits the existing retention cap without page metadata.
type readWords struct {
	first []uint32
	extra *readWordPages
}

type readWordPages struct {
	pages [][]uint32
	n     int
}

func (w *readWords) len() int {
	if w.extra == nil {
		return len(w.first)
	}
	return w.extra.n
}
func (w *readWords) capacity() int {
	if w.extra == nil {
		return cap(w.first)
	}
	return len(w.extra.pages) * readWordPageSize
}
func (w *readWords) bytes() int64 {
	if w.extra == nil {
		return int64(cap(w.first)) * 4
	}
	return int64(w.capacity())*4 + int64(unsafe.Sizeof(readWordPages{})) + int64(cap(w.extra.pages))*int64(unsafe.Sizeof([]uint32{}))
}
func (w *readWords) at(i int) uint32 {
	if w.extra == nil {
		return w.first[i]
	}
	return w.extra.pages[i>>readWordPageShift][i&(readWordPageSize-1)]
}
func (w *readWords) put(i int, value uint32) {
	if w.extra == nil {
		w.first[i] = value
		return
	}
	w.extra.pages[i>>readWordPageShift][i&(readWordPageSize-1)] = value
}
func (w *readWords) reset() {
	if w.extra == nil {
		w.first = w.first[:0]
	} else {
		w.extra.n = 0
	}
}
func (w *readWords) resize(n int) {
	if w.extra == nil {
		w.first = w.first[:n]
		return
	}
	if n < 0 || n > w.capacity() {
		panic("read-word length exceeds capacity")
	}
	w.extra.n = n
}
func (w *readWords) reserve(want int, charge func(int64) bool) bool {
	if want < 0 || want > int(^uint(0)>>1)-(readWordPageSize-1) {
		return false
	}
	current := w.capacity()
	if want <= current {
		return true
	}
	if want <= readWordPageSize {
		capacity := 128
		for capacity < want {
			capacity *= 2
		}
		if !charge(int64(capacity-current) * 4) {
			return false
		}
		next := make([]uint32, len(w.first), capacity)
		copy(next, w.first)
		w.first = next
		return true
	}
	pages := (want + readWordPageSize - 1) >> readWordPageShift
	// Each suffix allocation contains two pages. The first flat page still
	// stands alone, so its existing 64 KiB retention contract is unchanged.
	pages = 1 + 2*(pages/2)
	const maxBytes = uint64(1<<63 - 1)
	if uint64(pages) > maxBytes/(readWordPageSize*4) {
		return false
	}
	tableCapacity := 2
	if w.extra != nil {
		tableCapacity = cap(w.extra.pages)
	}
	for tableCapacity < pages {
		if tableCapacity > int(^uint(0)>>1)/2 {
			return false
		}
		tableCapacity *= 2
	}
	dataBytes := uint64(pages) * readWordPageSize * 4
	metadataBytes := uint64(unsafe.Sizeof(readWordPages{})) + uint64(tableCapacity)*uint64(unsafe.Sizeof([]uint32{}))
	if metadataBytes > maxBytes-dataBytes {
		return false
	}
	bytes := int64(dataBytes + metadataBytes)
	if !charge(bytes - w.bytes()) {
		return false
	}
	if w.extra == nil {
		n := len(w.first)
		if cap(w.first) < readWordPageSize {
			first := make([]uint32, n, readWordPageSize)
			copy(first, w.first)
			w.first = first
		}
		table := make([][]uint32, 1, tableCapacity)
		table[0] = w.first[:readWordPageSize]
		w.extra = &readWordPages{pages: table, n: n}
	} else if cap(w.extra.pages) < tableCapacity {
		table := make([][]uint32, len(w.extra.pages), tableCapacity)
		copy(table, w.extra.pages)
		w.extra.pages = table
	}
	for len(w.extra.pages) < pages {
		pair := make([]uint32, 2*readWordPageSize)
		w.extra.pages = append(w.extra.pages,
			pair[:readWordPageSize:readWordPageSize],
			pair[readWordPageSize:2*readWordPageSize:2*readWordPageSize])
	}
	return true
}

type packedReadWordSort readWords

func (w *packedReadWordSort) Len() int           { return (*readWords)(w).len() }
func (w *packedReadWordSort) Less(i, j int) bool { r := (*readWords)(w); return r.at(i) < r.at(j) }
func (w *packedReadWordSort) Swap(i, j int) {
	r := (*readWords)(w)
	a, b := r.at(i), r.at(j)
	r.put(i, b)
	r.put(j, a)
}

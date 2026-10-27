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

func readWordGroupChunk(missing int) int { return min(8, missing) }

// readWordPagesFit prevents page rounding from overflowing slice lengths on
// either 32-bit or 64-bit hosts. maxInt is explicit for portable tests.
func readWordPagesFit(pages, maxInt int) bool {
	return pages > 0 && pages <= maxInt/readWordPageSize
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
	minimumPages := (want + readWordPageSize - 1) >> readWordPageShift
	// Prefer bounded eight-page suffix groups. If budget cannot fund that
	// headroom, retry the original two-page reservation before abstaining.
	groupPages := 1 + 8*((minimumPages-1+7)/8)
	pairPages := 1 + 2*((minimumPages-1+1)/2)
	const maxBytes = uint64(1<<63 - 1)
	plan := func(pages int) (int, int64, bool) {
		if !readWordPagesFit(pages, int(^uint(0)>>1)) || uint64(pages) > maxBytes/(readWordPageSize*4) {
			return 0, 0, false
		}
		tableCapacity := 2
		if w.extra != nil {
			tableCapacity = cap(w.extra.pages)
		}
		for tableCapacity < pages {
			if tableCapacity > int(^uint(0)>>1)/2 {
				return 0, 0, false
			}
			tableCapacity *= 2
		}
		dataBytes := uint64(pages) * readWordPageSize * 4
		sliceHeaderBytes := uint64(unsafe.Sizeof([]uint32{}))
		if uint64(tableCapacity) > uint64(int(^uint(0)>>1))/sliceHeaderBytes {
			return 0, 0, false
		}
		metadataBytes := uint64(unsafe.Sizeof(readWordPages{})) + uint64(tableCapacity)*sliceHeaderBytes
		if metadataBytes > maxBytes-dataBytes {
			return 0, 0, false
		}
		return tableCapacity, int64(dataBytes+metadataBytes) - w.bytes(), true
	}
	pages := groupPages
	tableCapacity, cost, ok := plan(pages)
	if !ok || !charge(cost) {
		pages = pairPages
		tableCapacity, cost, ok = plan(pages)
		if !ok || !charge(cost) {
			return false
		}
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
	for missing := pages - len(w.extra.pages); missing > 0; missing = pages - len(w.extra.pages) {
		// Large promotion jumps remain a sequence of bounded allocations;
		// no individual backing array exceeds eight 64 KiB pages.
		chunk := readWordGroupChunk(missing)
		group := make([]uint32, chunk*readWordPageSize)
		for i := 0; i < chunk; i++ {
			start := i * readWordPageSize
			w.extra.pages = append(w.extra.pages, group[start:start+readWordPageSize:start+readWordPageSize])
		}
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

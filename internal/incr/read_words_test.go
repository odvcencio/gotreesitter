package incr

import (
	"math/rand"
	"testing"
	"unsafe"
)

func TestReadWordPagesPreserveValuesAndCharges(t *testing.T) {
	var words readWords
	var charged int64
	allow := func(n int64) bool {
		if n < 0 {
			t.Fatal("negative charge")
		}
		charged += n
		return true
	}
	reference := make([]uint32, 3*readWordPageSize+73)
	var first *uint32
	for i := range reference {
		if words.capacity() == i {
			capacity := max(128, 2*words.capacity())
			if words.capacity() >= readWordPageSize {
				capacity = i + 1
			}
			if !words.reserve(capacity, allow) {
				t.Fatal("unbounded reservation failed")
			}
		}
		words.resize(i + 1)
		reference[i] = uint32(i*17 + 3)
		words.put(i, reference[i])
		if i == readWordPageSize-1 {
			first = &words.first[0]
		}
		if i >= readWordPageSize && &words.first[0] != first {
			t.Fatal("page growth copied the fixed prefix")
		}
		if words.bytes() != charged {
			t.Fatalf("charge=%d retained=%d", charged, words.bytes())
		}
	}
	for i, want := range reference {
		if got := words.at(i); got != want {
			t.Fatalf("word %d=%d, want %d", i, got, want)
		}
	}
	before := words.bytes()
	if words.reserve(words.capacity()+1, func(int64) bool { return false }) || words.bytes() != before {
		t.Fatal("rejected reservation changed storage")
	}
	words.resize(43)
	words.reset()
	before = charged
	for i := 0; i < 2*readWordPageSize; i++ {
		if !words.reserve(i+1, allow) {
			t.Fatal("retained reservation failed")
		}
		words.resize(i + 1)
		words.put(i, uint32(i))
	}
	if charged != before {
		t.Fatal("reset discarded retained pages")
	}
}

func TestPagedReadHistoryDifferentialPromotionAndCursor(t *testing.T) {
	const size = 256 * 1024
	r := NewReads(size)
	type span struct{ start, end uint32 }
	spans := make([]span, 0, 30000)
	for i := 0; i < 28000; i++ {
		start := uint32(i * 7)
		end := start + uint32(1+i%19)
		r.Record(int(start), end)
		spans = append(spans, span{start, end})
	}
	// A long failed probe widens an existing packed history across page edges.
	r.Record(210000, 250000)
	spans = append(spans, span{210000, 250000})
	random := rand.New(rand.NewSource(912))
	for i := 0; i < 1000; i++ {
		start := uint32(random.Intn(220000))
		end := min(uint32(size), start+uint32(random.Intn(2000)))
		r.Record(int(start), end)
		spans = append(spans, span{start, end})
	}
	r.Seal()
	if !r.ValidForSource(size) {
		t.Fatal("valid history abstained")
	}
	for _, boundary := range []bool{false, true} {
		cursor := r.Cursor(boundary)
		for i := 0; i < 500; i++ {
			end := uint32(1 + random.Intn(size))
			frontier := uint32(0)
			seen := false
			for _, s := range spans {
				if s.start < end || (boundary && s.start == end) {
					frontier = max(frontier, s.end)
					seen = true
				}
			}
			wantOK := seen && frontier >= end
			want := uint32(0)
			if wantOK {
				want = frontier - end
			}
			got, ok := r.lookahead(end, boundary)
			fromCursor, cursorOK := cursor.Lookahead(end)
			if ok != wantOK || (ok && got != want) || ok != cursorOK || (ok && fromCursor != got) {
				t.Fatalf("boundary=%t end=%d direct=%d/%t cursor=%d/%t want=%d/%t", boundary, end, got, ok, fromCursor, cursorOK, want, wantOK)
			}
		}
	}
	wantSize := uintptr(64)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantSize = 44
	}
	if unsafe.Sizeof(Reads{}) != wantSize {
		t.Fatalf("read header grew to %d, want %d", unsafe.Sizeof(Reads{}), wantSize)
	}
}

// Histories that fit the shared 64 KiB retention allowance must keep their
// existing flat allocation. Adding descriptors early would discard it on Reset.
func TestReadHistoryRetains64KiBFlatPrefix(t *testing.T) {
	r := NewReads(32768)
	for i := 0; i < 10000; i++ {
		r.Record(i, uint32(i+1))
	}
	if !r.ValidForSource(32768) {
		t.Fatal("unbounded valid history declined")
	}
	const wantBytes = 64 + 64*1024
	if got := r.Bytes(); got != wantBytes {
		t.Fatalf("64 KiB history retains %d bytes, want %d", got, wantBytes)
	}
	r.Reset(32768)
	r.TrimCapacity(8192)
	if got := r.Bytes(); got != wantBytes {
		t.Fatalf("reset discarded storage under the retention cap: %d bytes", got)
	}
}

func TestReadWordPagesRejectOverflowBeforeBudgetCharge(t *testing.T) {
	var words readWords
	called := false
	want := int(^uint(0)>>1) - (readWordPageSize - 1)
	if words.reserve(want, func(cost int64) bool {
		called = true
		if cost < 0 {
			t.Fatalf("overflow produced a negative budget charge: %d", cost)
		}
		return false
	}) || words.bytes() != 0 {
		t.Fatal("rejected oversized reservation changed storage")
	}
	if unsafe.Sizeof(uintptr(0)) == 8 && called {
		t.Fatal("overflowing storage reached budget charging")
	}
}

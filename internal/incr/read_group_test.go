package incr

import "testing"

func TestEightPageReservationFallsBackWithoutLosingHistory(t *testing.T) {
	const source = 100000
	r := NewReads(source)
	var allocated int64
	// Header + a three-page history with spare room for page metadata.
	const limit = int64(64 + 3*readWordPageSize*4 + 4096)
	allocated = r.Bytes()
	r.BindBudget(limit, 0, &allocated)
	r.CertifyDefaultProducer(8195)
	for i := 0; i < readWordPageSize+1; i++ {
		r.Record(i, uint32(i+1))
	}
	if !r.Recording() || r.DefaultProducerSchedule(source) != 8195 {
		t.Fatal("funded pair fallback lost history or grant")
	}
	if got := r.ends.capacity(); got != 3*readWordPageSize {
		t.Fatalf("fallback capacity=%d", got)
	}
	if got := r.Bytes(); got != allocated || got > limit {
		t.Fatalf("accounting bytes=%d allocated=%d limit=%d", got, allocated, limit)
	}
	r.Seal()
	if end, ok := r.Lookahead(100); !ok || end != 1 {
		t.Fatalf("fallback frontier=%d/%t", end, ok)
	}
}

func TestEightPageReservationDenialRevokesHistory(t *testing.T) {
	const source = 100000
	r := NewReads(source)
	var allocated int64
	allocated = r.Bytes()
	// Fund one 64 KiB page, but neither possible grouped reservation.
	r.BindBudget(64+readWordPageSize*4+100, 0, &allocated)
	r.CertifyDefaultProducer(8195)
	for i := 0; i < readWordPageSize; i++ {
		r.Record(i, uint32(i+1))
	}
	before, capacity := allocated, r.ends.capacity()
	r.Record(readWordPageSize, readWordPageSize+1)
	if r.Recording() || r.DefaultProducerSchedule(source) != 0 {
		t.Fatal("denied growth retained authority")
	}
	if allocated != before || r.ends.capacity() != capacity {
		t.Fatal("denied reservation mutated accounting or storage")
	}
}

func TestEightPageReservationHandlesMultiPageJump(t *testing.T) {
	var w readWords
	var used int64
	charge := func(n int64) bool {
		if n < 0 {
			t.Fatal("negative charge")
		}
		used += n
		return true
	}
	want := 5*readWordPageSize + 7
	if !w.reserve(want, charge) || w.capacity() < want {
		t.Fatal("multi-page reservation too small")
	}
	if got := w.bytes(); got != used {
		t.Fatalf("bytes=%d charged=%d", got, used)
	}
	w.resize(want)
	w.put(want-1, 42)
	if got := w.at(want - 1); got != 42 {
		t.Fatal("page boundary lost data")
	}
}

func TestEightPageReservationChecks32BitSliceArithmetic(t *testing.T) {
	const max32 = int(1<<31 - 1)
	if !readWordPagesFit(max32/readWordPageSize, max32) {
		t.Fatal("largest safe page count rejected")
	}
	if readWordPagesFit(max32/readWordPageSize+1, max32) {
		t.Fatal("overflowing 32-bit page count accepted")
	}
}

func TestEightPageReservationBoundsContiguousAllocationOn32Bit(t *testing.T) {
	const max32 = int(1<<31 - 1)
	for _, missing := range []int{1, 2, 8, 9, max32 / readWordPageSize} {
		chunk := readWordGroupChunk(missing)
		if chunk < 1 || chunk > 8 || chunk > max32/(4*readWordPageSize) {
			t.Fatalf("unsafe chunk %d for %d pages", chunk, missing)
		}
	}
}

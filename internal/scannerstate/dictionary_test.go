package scannerstate

import "testing"

func TestDictionarySetPreservesReceiptsAcrossOverflowUpdatesAndReset(t *testing.T) {
	var s DictionarySet[[2]uint64]
	want := map[int][2]uint64{}
	charged := int64(0)
	for i := 0; i < 300; i++ {
		key := (i * 71) % 300
		value := [2]uint64{uint64(i), uint64(i + 1)}
		charged += s.Upsert(key, value)
		want[key] = value
	}
	for i := 0; i < 300; i += 3 {
		value := [2]uint64{0, 1}
		charged += s.Upsert(i, value)
		want[i] = value
	}
	for key, value := range want {
		if got, ok := s.Lookup(key); !ok || got != value {
			t.Fatalf("key=%d got=%v ok=%t want=%v", key, got, ok, value)
		}
	}
	if charged != s.Bytes() {
		t.Fatalf("charged=%d retained=%d", charged, s.Bytes())
	}
	s.Reset()
	if _, ok := s.Lookup(1); ok {
		t.Fatal("reset retained a receipt")
	}
	for i := 0; i < 300; i++ {
		charged += s.Upsert(i, [2]uint64{0, 1})
	}
	if charged != s.Bytes() {
		t.Fatal("reset changed physical accounting")
	}
	for i := 0; i < 300; i++ {
		if got, ok := s.Lookup(i); !ok || got != [2]uint64{0, 1} {
			t.Fatalf("reset reuse lost key=%d", i)
		}
	}
}
func TestDictionarySetBoundsDictionaryAndCutsRepeatedReceiptStorage(t *testing.T) {
	var compressed DictionarySet[[2]uint64]
	var control Set[[2]uint64]
	for i := 0; i < 10000; i++ {
		v := [2]uint64{uint64(i % 3), uint64(i%3 + 1)}
		compressed.Upsert(i, v)
		control.Upsert(i, v)
	}
	if compressed.Bytes()*2 >= control.Bytes() {
		t.Fatalf("compressed=%d control=%d", compressed.Bytes(), control.Bytes())
	}
	for i := 0; i < 10000; i++ {
		v := [2]uint64{uint64(i % 3), uint64(i%3 + 1)}
		if cost := compressed.Upsert(i, v); cost != 0 {
			t.Fatalf("warm overwrite allocated %d bytes", cost)
		}
	}
	if len(compressed.values) > 64 {
		t.Fatal("dictionary exceeds its bound")
	}
}
func TestSnapshotCacheBoundsAndKeepsZeroIdentity(t *testing.T) {
	var c SnapshotCache[int]
	c.Store(0, 7)
	if v, ok := c.Lookup(0); !ok || v != 7 {
		t.Fatal("zero identity missing")
	}
	for i := uint32(1); i <= 8; i++ {
		c.Store(i, int(i))
	}
	if _, ok := c.Lookup(0); ok {
		t.Fatal("oldest identity was not evicted")
	}
	c.Store(8, 20)
	if v, ok := c.Lookup(8); !ok || v != 20 {
		t.Fatal("overwrite failed")
	}
	c = SnapshotCache[int]{}
	if _, ok := c.Lookup(8); ok {
		t.Fatal("reset retained snapshot")
	}
}

func TestSetRangeOnlyVisitsRecordedReceipts(t *testing.T) {
	var s Set[int]
	s.EnsureCapacity(10000)
	for _, index := range []int{400, 3, 950, 25} {
		s.Upsert(index, index+1)
	}
	count, previous := 0, -1
	s.Range(func(index, value int) bool {
		if index <= previous || value != index+1 {
			t.Fatalf("range index=%d value=%d previous=%d", index, value, previous)
		}
		previous = index
		count++
		return true
	})
	if count != 4 {
		t.Fatalf("range count=%d", count)
	}
	count = 0
	s.Range(func(int, int) bool { count++; return false })
	if count != 1 {
		t.Fatal("range did not stop")
	}
	s.Reset()
	s.Range(func(int, int) bool { t.Fatal("range visited reset receipts"); return true })
}

func TestSnapshotWindowBoundsAndResetsReferences(t *testing.T) {
	var w SnapshotWindow[int]
	for i := 0; i < 9; i++ {
		w.Remember(i)
	}
	if _, ok := w.Find(func(value int) bool { return value == 0 }); ok {
		t.Fatal("window retained its evicted reference")
	}
	if value, ok := w.Find(func(value int) bool { return value == 8 }); !ok || value != 8 {
		t.Fatal("window lost recent reference")
	}
	w = SnapshotWindow[int]{}
	if _, ok := w.Find(func(int) bool { return true }); ok {
		t.Fatal("window survived reset")
	}
}

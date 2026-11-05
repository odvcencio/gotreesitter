package parsercorephase0

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestScannerTrialMaskRoundTripOwnershipAndAllocation(t *testing.T) {
	for size := 65; size <= 128; size++ {
		for _, dense := range []bool{false, true} {
			source := make([]bool, size)
			for i := range source {
				source[i] = dense || i == 0 || i == 63 || i == 64 || i == size-1
			}
			expected := append([]bool(nil), source...)
			packed, ok := PackScannerTrialMask(source)
			if !ok || packed.Len() != size {
				t.Fatalf("size=%d packed=%v len=%d", size, ok, packed.Len())
			}
			clear(source)
			dest := make([]bool, 128)
			for i := range dest {
				dest[i] = true
			}
			got := packed.Restore(dest)
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("roundtrip size=%d dense=%v", size, dense)
			}
			got[0] = false
			if !packed.Restore(dest)[0] {
				t.Fatal("restored slice aliases frozen bits")
			}
			empty, _ := PackScannerTrialMask(make([]bool, size))
			if zero := empty.Restore(dest); !reflect.DeepEqual(zero, make([]bool, size)) {
				t.Fatal("stale trial bits survive restore")
			}
			if allocs := testing.AllocsPerRun(100, func() { _ = packed.Restore(dest) }); allocs != 0 {
				t.Fatalf("restore allocated %v", allocs)
			}
		}
	}
	for _, size := range []int{0, 1, 3, 64, 129, 256} {
		if _, ok := PackScannerTrialMask(make([]bool, size)); ok {
			t.Fatalf("unsupported size=%d packed", size)
		}
	}
	wantSize := uintptr(24)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantSize = 20
	}
	if unsafe.Sizeof(ScannerTrialMask{}) != wantSize {
		t.Fatalf("mask footprint=%d", unsafe.Sizeof(ScannerTrialMask{}))
	}
}

func TestPackedSnapshotPagesRemainImmutableAndChargeUnusedSlots(t *testing.T) {
	var pages ScannerSnapshotPages[struct{ key int }]
	first, a, _, _, storage := pages.NewPacked([]byte{7}, nil, nil)
	first.key = 1
	second, b, _, _, same := pages.NewPacked([]byte{8}, nil, nil)
	second.key = 2
	if storage != same || first == second || storage.FootprintBytes() != uint64(unsafe.Sizeof(scannerSnapshotPackedPage32[struct{ key int }]{})) {
		t.Fatal("packed pair storage or charge changed")
	}
	_, c, _, _, next := pages.NewPacked([]byte{9}, nil, nil)
	if next == storage {
		t.Fatal("rollover overwrote a published page")
	}
	pages.Reset()
	if x, y := pages.CurrentStorage(); x != nil || y != nil {
		t.Fatal("reset retained packed pages")
	}
	_, d, _, _, after := pages.NewPacked([]byte{10}, nil, nil)
	if after == storage || after == next {
		t.Fatal("reset reused a published address")
	}
	if first.key != 1 || second.key != 2 || a[0] != 7 || b[0] != 8 || c[0] != 9 || d[0] != 10 {
		t.Fatal("published packed records changed")
	}
}

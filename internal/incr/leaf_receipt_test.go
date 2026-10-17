package incr

import (
	"fmt"
	"testing"
)

func TestPackedLeafReceipts(t *testing.T) {
	words := make([]uint32, 3)
	const known = uint32(1) << 30
	const keyword = uint32(1) << 31
	for id := uint32(0); id < 48; id++ {
		flags := known
		if id%3 == 0 {
			flags |= keyword
		}
		SetLeafReceipt(words, id, flags)
	}
	for id := uint32(0); id < 48; id++ {
		flags := known
		if id%3 == 0 {
			flags |= keyword
		}
		if got := LeafReceipt(words, id); got != flags {
			t.Fatalf("id %d flags %x want %x", id, got, flags)
		}
	}
	// Replacing a keyword receipt must revoke its old keyword provenance.
	for _, id := range []uint32{0, 15, 16, 31, 32, 47} {
		old := LeafReceipt(words, id)
		SetLeafReceipt(words, id, known)
		if got := LeafReceipt(words, id); got != known {
			t.Fatalf("replace id %d: %x", id, got)
		}
		SetLeafReceipt(words, id, old)
	}
	for id := uint32(0); id < 48; id++ {
		SetLeafReceipt(words, id, 0)
	}
	for _, w := range words {
		if w != 0 {
			t.Fatal("stale authorization")
		}
	}
	for _, id := range []uint32{48, 49, ^uint32(0)} {
		if LeafReceipt(words, id) != 0 {
			t.Fatalf("out of range id %d authorized", id)
		}
	}
	if LeafReceipt(nil, 0) != 0 {
		t.Fatal("nil storage authorized leaf")
	}
}

func TestPackedLeafReceiptTail(t *testing.T) {
	for _, last := range []uint32{0, 1, 14, 15, 16, 17, 30, 31, 32, 47} {
		t.Run(fmt.Sprint(last), func(t *testing.T) {
			words := make([]uint32, int(last/16)+1)
			for id := uint32(0); id < uint32(len(words)*16); id++ {
				SetLeafReceipt(words, id, ^uint32(0))
			}
			TrimLeafReceiptTail(words, last)
			for id := uint32(0); id < uint32(len(words)*16)+16; id++ {
				want := uint32(0)
				if id <= last {
					want = uint32(3) << 30
				}
				if got := LeafReceipt(words, id); got != want {
					t.Fatalf("last %d id %d flags %x want %x", last, id, got, want)
				}
			}
		})
	}
}

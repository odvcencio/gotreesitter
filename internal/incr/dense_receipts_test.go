package incr

import (
	"math/rand"
	"testing"
	"unsafe"
)

func TestDenseReceiptsBudgetPresenceAndDifferential(t *testing.T) {
	var store DenseReceipts
	if store.Ensure(100, func(int64) bool { return false }) != nil || store.Bytes() != 0 {
		t.Fatal("budget rejection allocated storage")
	}
	reference := map[int]DenseReceipt{}
	random := rand.New(rand.NewSource(91))
	for step := 0; step < 2000; step++ {
		index := random.Intn(4096)
		if random.Intn(3) == 0 {
			store.Clear(index)
			delete(reference, index)
		} else {
			before := store.Bytes()
			charged := int64(0)
			if store.Ensure(index, func(cost int64) bool { charged += cost; return true }) == nil {
				t.Fatal("unbounded allocation rejected")
			}
			if store.Bytes()-before != charged {
				t.Fatal("dense allocation charge differs from retained capacity")
			}
			record := DenseReceipt{Start: uint32(index), End: uint32(index + 1), Lookahead: uint32(step % 7), Column: uint32(index % 29), Present: true}
			store.Set(index, record)
			reference[index] = record
		}
		if store.Count() != len(reference) {
			t.Fatal("presence count differs from reference")
		}
	}
	seen := 0
	store.Each(func(index int, value DenseReceipt) {
		if reference[index] != value {
			t.Fatal("enumeration differs from reference")
		}
		seen++
	})
	if seen != len(reference) {
		t.Fatal("enumeration omitted live receipts")
	}
	for index, want := range reference {
		if got := store.Lookup(index); got == nil || *got != want {
			t.Fatal("lookup differs from reference")
		}
	}
}

func TestDenseReceiptsPromotionAndBudgetFallback(t *testing.T) {
	var store DenseReceipts
	for index := 0; index < 64; index++ {
		store.Ensure(index, func(int64) bool { return true })
		store.Set(index, DenseReceipt{Start: uint32(index)})
	}
	if store.dense {
		t.Fatal("promoted before density threshold")
	}
	before := store.Bytes()
	if store.Ensure(64, func(int64) bool { return false }) != nil || store.Bytes() != before || store.dense {
		t.Fatal("rejected promotion changed storage")
	}
	charged := int64(0)
	store.Ensure(64, func(cost int64) bool { charged += cost; return true })
	store.Set(64, DenseReceipt{Start: 64})
	if !store.dense || store.Bytes()-before != charged || store.Count() != 65 {
		t.Fatal("promotion lost receipts or charging")
	}
	for i := 0; i < 65; i++ {
		if receipt := store.Lookup(i); receipt == nil || !receipt.Present || receipt.Start != uint32(i) {
			t.Fatal("promotion changed identity")
		}
	}
	before = store.Bytes()
	charged = 0
	limit := int64(unsafe.Sizeof(denseReceiptPage{})) + 8*int64(unsafe.Sizeof((*denseReceiptPage)(nil)))
	if store.Ensure(512, func(cost int64) bool {
		if cost > limit {
			return false
		}
		charged += cost
		return true
	}) == nil {
		t.Fatal("single-page budget fallback rejected")
	}
	if store.Bytes()-before != charged || store.Lookup(576) != nil {
		t.Fatal("fallback allocated uncharged adjacent pages")
	}
}

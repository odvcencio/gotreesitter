package parsercorephase0

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"sync"
	"testing"
	"unsafe"
)

func TestCheckpointReceiptCacheEvictionAndReadOnly(t *testing.T) {
	if unsafe.Sizeof(checkpointRecord{}) != 12 {
		t.Fatalf("record bytes %d", unsafe.Sizeof(checkpointRecord{}))
	}
	interner := newCheckpointInterner(64, 1<<20)
	states := make([][]byte, 32)
	ids := make([]CheckpointID, len(states))
	for n := range states {
		states[n] = []byte(fmt.Sprintf("checkpoint-%d-with-immutable-payload", n))
		var err error
		ids[n], err = interner.intern(states[n])
		if err != nil {
			t.Fatal(err)
		}
	}
	// Re-interning evicted states retains their exact identities.
	for n, id := range ids {
		again, err := interner.intern(states[n])
		if err != nil || again != id {
			t.Fatalf("id %d got %d: %v", id, again, err)
		}
	}
	before := slices.Clone(interner.receipts)
	var workers sync.WaitGroup
	for g := 0; g < 8; g++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for round := 0; round < 16; round++ {
				for n, id := range ids {
					length, digest, ok := interner.receipt(id)
					if !ok || length != uint32(len(states[n])) || digest != sha256.Sum256(states[n]) {
						t.Errorf("receipt %d changed", id)
						return
					}
				}
			}
		}()
	}
	workers.Wait()
	if !slices.Equal(interner.receipts, before) {
		t.Fatal("receipt readers mutated digest cache")
	}
	interner.reset()
	if !slices.Equal(interner.receipts, make([]checkpointDigestCacheEntry, 4)) {
		t.Fatal("reset retained receipt cache")
	}
	id, err := interner.intern([]byte("different"))
	if err != nil || id != 1 {
		t.Fatal("reset did not reuse first identity")
	}
	_, digest, ok := interner.receipt(id)
	if !ok || digest != sha256.Sum256([]byte("different")) {
		t.Fatal("reused identity read stale digest")
	}
}

func TestCheckpointReceiptCacheCorruptRangeAbstains(t *testing.T) {
	i := newCheckpointInterner(16, 1024)
	id, err := i.intern([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	i.records[id-1].offset = ^uint32(0)
	if _, _, ok := i.receipt(id); ok {
		t.Fatal("cached digest authenticated an invalid byte range")
	}
}

func TestCheckpointReceiptCacheCapacityChargedAndTrimmed(t *testing.T) {
	i := newCheckpointInterner(8, 1024)
	if i.footprintBytes() != 0 || i.receipts != nil {
		t.Fatal("empty cache allocated storage")
	}
	if _, err := i.intern([]byte("state")); err != nil {
		t.Fatal(err)
	}
	cacheBytes := uint64(cap(i.receipts)) * uint64(unsafe.Sizeof(checkpointDigestCacheEntry{}))
	if len(i.receipts) != 4 || cap(i.receipts) != 4 || cacheBytes != 176 {
		t.Fatalf("unbounded cache %d/%d bytes %d", len(i.receipts), cap(i.receipts), cacheBytes)
	}
	withoutCache := uint64(cap(i.records))*12 + uint64(cap(i.bytes)) + uint64(len(i.buckets))*12
	if got := i.footprintBytes(); got != withoutCache+cacheBytes {
		t.Fatalf("footprint %d want %d", got, withoutCache+cacheBytes)
	}
	i.reset()
	if i.footprintBytes() < cacheBytes {
		t.Fatal("reset hid retained cache capacity")
	}
	i.dropOversized()
	if i.footprintBytes() != 0 || i.receipts != nil {
		t.Fatal("trim retained cache storage")
	}
}

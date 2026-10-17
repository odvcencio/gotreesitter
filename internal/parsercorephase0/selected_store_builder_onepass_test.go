package parsercorephase0

import (
	"errors"
	"testing"
	"unsafe"
)

func TestSelectedStoreOnePassFinalLogicalSizeGovernsRetainedCap(t *testing.T) {
	const depth = 256
	recordBytes := uint64(unsafe.Sizeof(SelectedNodeRecord{}))
	compact, err := New(&fakeTable{}, Limits{
		MaxSubtrees:            depth + 2,
		MaxChildren:            depth + 2,
		MaxSelectedOccurrences: depth + 2,
		MaxSelectedBytes:       recordBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < depth; index++ {
		root, err = compact.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{root}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, selectedStoreTestPolicy(t, 1), []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	if store.NodeCount() != 1 || store.ChildCount() != 0 || store.RetainedBytes() != recordBytes {
		t.Fatalf("collapsed store nodes=%d children=%d retained=%d want=%d", store.NodeCount(), store.ChildCount(), store.RetainedBytes(), recordBytes)
	}
}

func TestSelectedStoreOnePassCancellationAfterGrowthReusesCleanBacking(t *testing.T) {
	const childCount = 4096
	compact, err := New(&fakeTable{}, Limits{
		MaxSubtrees:            4,
		MaxChildren:            childCount + 1,
		MaxSelectedOccurrences: childCount + 2,
		MaxSelectedBytes:       1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := compact.appendSubtree(subtreeRecord{symbol: 1, endByte: 1, terminal: true}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	children := make([]SubtreeID, childCount)
	for index := range children {
		children[index] = leaf
	}
	root, err := compact.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, children, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := selectedStoreTestPolicy(t, 2, 1)
	stop := errors.New("stop after selected-store growth")
	polls := 0
	store, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), func() error {
		polls++
		if polls == 4 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || store != nil {
		t.Fatalf("cancelled store=%v err=%v polls=%d", store, err, polls)
	}
	compact.selectedPoolMu.Lock()
	pooledRecords := cap(compact.selectedPool.records)
	compact.selectedPoolMu.Unlock()
	if pooledRecords == 0 {
		t.Fatal("cancelled grown backing was not returned to the pool")
	}

	fresh, err := compact.BuildSelectedStore([]SubtreeID{root}, policy, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	if fresh.NodeCount() != childCount+1 || fresh.ChildCount() != childCount {
		t.Fatalf("fresh store nodes=%d children=%d", fresh.NodeCount(), fresh.ChildCount())
	}
	rootRecord, ok := fresh.Record(fresh.Root())
	firstID, firstOK := fresh.Child(rootRecord, 0)
	lastID, lastOK := fresh.Child(rootRecord, childCount-1)
	first, firstRecordOK := fresh.Record(firstID)
	last, lastRecordOK := fresh.Record(lastID)
	if !ok || !firstOK || !lastOK || !firstRecordOK || !lastRecordOK || firstID == lastID ||
		first.Symbol != 1 || last.Symbol != 1 || first.Parent != fresh.Root() || last.Parent != fresh.Root() ||
		first.ChildIndex != 0 || last.ChildIndex != childCount-1 {
		t.Fatalf("fresh backing leaked state: root=%+v first=%+v/%d last=%+v/%d ok=%t/%t/%t/%t/%t",
			rootRecord, first, firstID, last, lastID, ok, firstOK, lastOK, firstRecordOK, lastRecordOK)
	}
}

func TestSelectedStoreOnePassRightSizeCopyPollsCancellation(t *testing.T) {
	compact, err := New(&fakeTable{}, Limits{MaxSelectedBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	const length = 3 * selectedStoreCopyPollElements
	store := &SelectedStore{
		owner:   compact,
		records: make([]SelectedNodeRecord, length, 4*selectedStoreCopyPollElements),
	}
	defer store.Release()
	stop := errors.New("stop during selected-store right-size")
	polls := 0
	err = rightSizeSelectedOnePassStore(store, len(store.records), 0, compact.limits.MaxSelectedBytes, func() error {
		polls++
		if polls == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || polls != 2 {
		t.Fatalf("right-size err=%v polls=%d", err, polls)
	}
	if cap(store.records) != 4*selectedStoreCopyPollElements {
		t.Fatalf("cancelled right-size published partial backing cap=%d", cap(store.records))
	}
}

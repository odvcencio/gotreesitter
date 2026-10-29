package parsercorephase0

import (
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"
)

// The public occurrence must remain readable without keeping the entire
// producing graph alive. Keeping only a snapshot used to fail this test.
func TestSelectedStoreDoesNotRetainProducingCore(t *testing.T) {
	collected := make(chan struct{})
	store := func() *SelectedStore {
		c, root, replay, scratch := nativeSelectedFixture(t)
		runtime.SetFinalizer(c, func(*Core) { close(collected) })
		store, err := c.BuildNativeSelectedOutput([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}()
	defer store.Release()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		select {
		case <-collected:
			root, ok := store.Record(store.Root())
			if !ok || root.Symbol != 2 || root.ChildCount != 1 {
				t.Fatalf("collected graph invalidated result: %+v", root)
			}
			runtime.KeepAlive(store)
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("a live compact result retained its producing Core")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSelectedStorePoolCountsProvenanceAndDetachedGeneration(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	store := c.acquireSelectedStoreWithSources(32, 16, true)
	owner := store.owner
	want := store.RetainedBytes() + uint64(unsafe.Sizeof(*owner))
	store.Release()
	if got := owner.retainedBytes(); got != want {
		t.Fatalf("pool bytes=%d want=%d, including provenance", got, want)
	}
	late := c.acquireSelectedStore(8, 4)
	if err := c.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	// A late release must be isolated from the next pool generation.
	fresh := c.acquireSelectedStore(1, 1)
	defer fresh.Release()
	if fresh.owner == owner {
		t.Fatal("retention release reused the detached pool")
	}
	late.Release()
	if got := fresh.owner.retainedBytes(); got != uint64(unsafe.Sizeof(*owner)) {
		t.Fatalf("late release repopulated the current pool: %d", got)
	}
}

func TestSelectedStorePoolConcurrentReleaseAndFootprint(t *testing.T) {
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	stores := make([]*SelectedStore, 64)
	for i := range stores {
		stores[i] = c.acquireSelectedStoreWithSources(i+1, i+1, true)
	}
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		for range 1024 {
			_ = c.FootprintBytes()
		}
	}()
	for _, store := range stores {
		group.Add(1)
		go func(s *SelectedStore) { defer group.Done(); s.Release() }(store)
	}
	group.Wait()
}

func TestSelectedStorePooledPlanesOverwritePriorTree(t *testing.T) {
	c, root, replay, scratch := nativeSelectedFixture(t)
	// Poison every field, including slack. Publication must not read a slot
	// from a previous tree before overwriting it.
	poison := c.acquireSelectedStoreWithSources(64, 64, true)
	for i := range poison.records[:cap(poison.records)] {
		poison.records[:cap(poison.records)][i] = SelectedNodeRecord{
			Parent: 0xffffffff, FirstChild: 0xffffffff, StartByte: 0xffffffff,
			EndByte: 0xffffffff, DynamicPrecedence: -1, SupertypeMask: 0xffffffff,
			Symbol: 0xffff, Field: 0xffff, ProductionID: 0xffff,
			ChildCount: 0xffff, ChildIndex: 0xffff, flags: 0xff,
		}
		poison.children[:cap(poison.children)][i] = 0xffffffff
		poison.payloadSources[:cap(poison.payloadSources)][i] = 0xffffffff
	}
	poison.Release()
	for iteration := 0; iteration < 4; iteration++ {
		store, err := c.BuildNativeSelectedOutput([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := store.Record(store.Root())
		if !ok || r.Parent != 0 || r.StartByte != 0 || r.EndByte != 1 || r.ChildCount != 1 || r.HasError() || r.Fragile() || r.DynamicPrecedence != 0 || r.SupertypeMask != 0 {
			t.Fatalf("iteration %d inherited prior metadata: %+v", iteration, r)
		}
		id, ok := store.Child(r, 0)
		child, found := store.Record(id)
		if !ok || !found || child.Parent != store.Root() || child.ChildCount != 0 || child.ChildIndex != 0 || child.Symbol != 1 || child.HasError() || child.Field != 0 {
			t.Fatalf("iteration %d inherited prior child: %+v", iteration, child)
		}
		store.Release()
	}
}

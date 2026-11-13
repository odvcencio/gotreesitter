package ccore_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// The limit refusal in the exit drain of a parse is quiet: the parse returns its
// tree, the store is poisoned, the lease is free, and no panic escapes. The
// limit is lowered after the publication walk (at the "sweep" point), and two
// retains are queued after the initial drain (at the "parse-start" point).
func TestExitDrainLimitIsQuiet(t *testing.T) {
	p, base, _, src, _ := overlapSetup(t)
	cp := base.Copy()
	cp.Edit(selfEdit(src, 6))
	ccore.DrainNow(base)
	var extra []*ccore.Tree
	ccore.SetTestHook(base, func(pt string) {
		switch pt {
		case "parse-start":
			extra = append(extra, base.Copy(), base.Copy())
		case "sweep":
			ccore.SetCountLimit(base, 0)
		}
	})
	nt, stop, pan := parseCatch(p, src, cp)
	ccore.SetTestHook(base, nil)
	if pan != nil {
		t.Fatalf("the refusal in the exit drain reached the caller as a panic: %v", pan)
	}
	if stop != "" || nt == nil {
		t.Fatalf("the parse itself succeeded and must return its tree: stop %q", stop)
	}
	if !ccore.Poisoned(base) {
		t.Error("the store is not poisoned after the refused exit drain")
	}
	if !p.LeaseFree() {
		t.Error("the lease is held")
	}
	nt.Release()
	cp.Release()
	for _, e := range extra {
		e.Release()
	}
	base.Release()
}

// A drain that fails in a finalizer's goroutine has no caller: it poisons the
// store and returns. A panic there would end the test binary.
func TestFinalizerDrainFailureIsQuiet(t *testing.T) {
	p, base, nt, _, _ := overlapSetup(t)
	_ = p
	ccore.SetCountLimit(base, 0)
	cp := base.Copy() // queues a retain, which the limit refuses
	ccore.Finalize(cp)
	waitFor(t, "the store to be poisoned by the finalizer's drain", func() bool { return ccore.Poisoned(base) })
	nt.Release()
	base.Release()
}

// The store ends when its last owner leaves while a drain that then fails holds
// the lease: the end check runs after the failed drain too.
func TestStoreEndsAfterAFailedDrain(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	base, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	other := base.Copy()
	fired := false
	ccore.SetTestHook(base, func(pt string) {
		if pt == "drain" && !fired {
			fired = true
			other.Release() // the last other owner leaves; its drain attempt fails TryLock
			panic("injected in the first drain")
		}
	})
	base.Release()
	ccore.SetTestHook(base, nil)
	if !fired {
		t.Fatal("the drain hook did not run")
	}
	if !ccore.Ended(base) {
		t.Fatalf("the store did not end (cells %d, poisoned %t)", ccore.Cells(base), ccore.Poisoned(base))
	}
}

// A released old tree is rejected first, also when its store is poisoned.
func TestReleasedOldTreeOfPoisonedStoreIsRejected(t *testing.T) {
	p, base, _, src, _ := overlapSetup(t)
	cp := base.Copy()
	cp.Release()
	ccore.Poison(base)
	nt, stop := p.ParseWith(src, cp)
	if nt != nil || stop != "old tree released" {
		t.Fatalf("tree %v, stop %q: want the released tree rejected", nt != nil, stop)
	}
	base.Release()
}

// A parse that waits for the lease while another goroutine poisons the store
// parses afresh in a new store when it gets the lease.
func TestWaitingParseOfAStoreThatIsPoisonedMeanwhileParsesAfresh(t *testing.T) {
	p, base, _, src, _ := overlapSetup(t)
	want := p.Digest(base)
	cp := base.Copy()
	cp.Edit(selfEdit(src, 6))
	ccore.LockLease(base)
	var nt *ccore.Tree
	var stop string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); nt, stop = p.ParseWith(src, cp) }()
	time.Sleep(100 * time.Millisecond) // the parse has chosen the store and waits for the lease
	ccore.Poison(base)
	ccore.UnlockLease(base)
	wg.Wait()
	if stop != "" || nt == nil {
		t.Fatalf("stop %q", stop)
	}
	if ccore.SameStore(nt, base) {
		t.Error("the parse used the poisoned store")
	}
	if p.Digest(nt) != want {
		t.Error("the tree differs from the fresh parse")
	}
	nt.Release()
	cp.Release()
	base.Release()
}

// The edit allocator checks the record-index limit: an edit that would pass it
// fails (a limit error reaches the caller of Edit), allocates no record above
// the limit, and poisons the store, whose old tree still reads.
func TestEditAllocatorChecksTheRecordLimit(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	base, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	want := p.Digest(base)
	cp := base.Copy() // no edit has run: the edit free lists are empty
	next := ccore.EditRecIndex(cp)
	ccore.SetRecLimit(base, next-1)
	var pan any
	func() {
		defer func() { pan = recover() }()
		cp.Edit(selfEdit(g.Src, 9))
	}()
	err, _ := pan.(error)
	if err == nil || !strings.Contains(err.Error(), "record index limit") {
		t.Fatalf("panic %v, want a record index limit error", pan)
	}
	if got := ccore.EditRecIndex(cp); got != next {
		t.Errorf("the edit allocator moved from %d to %d past the limit", next, got)
	}
	if !ccore.Poisoned(base) {
		t.Error("the store is not poisoned after the failed edit")
	}
	if p.Digest(cp) != want {
		t.Error("the tree reads differently after the failed edit")
	}
	cp.Release()
	base.Release()
	if !ccore.Ended(base) {
		t.Error("the store did not end")
	}
}

// An edit that fails after its first edited record exists poisons the store (the
// records it made are not tracked) and the failure goes on to the caller; the
// old tree still reads and the store ends when its trees go.
func TestEditFailingAfterTheFirstRecordPoisonsTheStore(t *testing.T) {
	p, base, nt, src, _ := overlapSetup(t)
	want := p.Digest(nt)
	ccore.SetTestHook(base, func(pt string) {
		if pt == "edit-record" {
			panic("injected after the first edited record")
		}
	})
	var pan any
	func() {
		defer func() { pan = recover() }()
		nt.Edit(selfEdit(src, 9))
	}()
	ccore.SetTestHook(base, nil)
	if pan == nil {
		t.Fatal("the failure did not reach the caller of Edit")
	}
	if !ccore.Poisoned(base) {
		t.Error("the store is not poisoned")
	}
	if p.Digest(nt) != want {
		t.Error("the tree reads differently after the failed edit")
	}
	nt.Release()
	base.Release()
	if !ccore.Ended(base) {
		t.Error("the store did not end")
	}
}

// Lowered limits belong to one store: a limit lowered on one store does not
// affect a parse in another.
func TestLoweredLimitsArePerStore(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	a, _ := p.Parse(g.Src)
	b, _ := p.Parse(g.Src)
	ccore.SetRecLimit(a, 1)
	ccore.SetCountLimit(a, 0)
	cp := b.Copy()
	cp.Edit(selfEdit(g.Src, 6))
	nt, stop := p.ParseWith(g.Src, cp)
	if stop != "" || nt == nil {
		t.Fatalf("a parse in another store was affected by the limits of store a: stop %q", stop)
	}
	nt.Release()
	cp.Release()
	a.Release()
	b.Release()
}

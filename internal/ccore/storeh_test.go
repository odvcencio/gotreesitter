package ccore_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// An edit that fails at any point after its first allocation and before its
// ownership is published poisons the store: after subtreeEdit has returned
// (before the retains are made) and inside the commit (under the short mutex).
// The failure reaches the caller, the mutex is free afterwards, and the
// trees still read and release.
func TestEditFailureAfterSubtreeEditAndDuringCommit(t *testing.T) {
	for _, point := range []string{"edit-record", "edit-retain", "edit-commit"} {
		t.Run(point, func(t *testing.T) {
			p, base, nt, src, _ := overlapSetup(t)
			want := p.Digest(nt)
			ccore.SetTestHook(base, func(pt string) {
				if pt == point {
					panic("injected at " + point)
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
			done := make(chan *ccore.Tree, 1)
			go func() { done <- base.Copy() }() // takes the short mutex: it hangs if the failed edit left it locked
			select {
			case cp := <-done:
				if p.Digest(cp) != want {
					t.Error("the copy reads differently after the failed edit")
				}
				cp.Release()
			case <-time.After(10 * time.Second):
				t.Fatal("the short mutex is still held after the failed edit")
			}
			if p.Digest(nt) != want {
				t.Error("the tree reads differently after the failed edit")
			}
			nt.Release()
			base.Release()
			if !ccore.Ended(base) {
				t.Error("the store did not end")
			}
		})
	}
}

// A parse that waits for the lease while its old tree is released and the store
// is then poisoned still rejects the released tree; it does not parse afresh.
func TestWaitingParseWhoseTreeIsReleasedAndStorePoisonedIsRejected(t *testing.T) {
	p, base, _, src, _ := overlapSetup(t)
	cp := base.Copy()
	cp.Edit(selfEdit(src, 6))
	ccore.LockLease(base)
	var nt *ccore.Tree
	var stop string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); nt, stop = p.ParseWith(src, cp) }()
	time.Sleep(100 * time.Millisecond) // the parse has chosen the store and waits for the lease
	cp.Release()
	ccore.Poison(base)
	ccore.UnlockLease(base)
	wg.Wait()
	if nt != nil || stop != "old tree released" {
		t.Fatalf("tree %v, stop %q: want the released tree rejected", nt != nil, stop)
	}
	base.Release()
}

// The first child page is under the child-page limit: with a limit of zero a
// parse stops with the limit's reason (also for an input that needs only the
// first page), no panic escapes, and nothing is left held.
func TestChildPageLimitOfZeroStopsAParse(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	p.SetTestLimits(1<<28-1, 1<<28-1, 0, 1<<16)
	for _, src := range [][]byte{g.Src, nil} {
		nt, stop, pan := parseCatch(p, src, nil)
		if pan != nil {
			t.Fatalf("the refusal escaped as a panic: %v", pan)
		}
		if nt != nil || !strings.Contains(stop, "child page limit") {
			t.Fatalf("tree %v, stop %q: want a child page limit stop", nt != nil, stop)
		}
	}
	if !p.LeaseFree() {
		t.Error("the lease is held")
	}
}

// A parse and an edit of separate trees in one store, with the child-page limit
// two pages above what the store holds: the edits are refused cleanly (a limit
// error to the caller) while parses run on; under -race the page-table reads of
// the limit checks are checked against the appends of the other allocator.
func TestParseAndEditAtTheChildPageLimitRace(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	q := mustParser(t, ccore.Config{Tables: g.Tables})
	for round := 0; round < 30; round++ {
		base, stop := p.Parse(g.Src)
		if stop != "" {
			t.Fatal(stop)
		}
		_, _, kidPages, _, _, _, _, _ := ccore.PagesHeld(base)
		ccore.SetKidPageLimit(base, uint32(kidPages)+1)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				cp := base.Copy()
				func() {
					defer func() {
						if r := recover(); r != nil {
							if err, ok := r.(error); !ok || !strings.Contains(err.Error(), "limit") {
								t.Errorf("unexpected panic from Edit: %v", r)
							}
						}
					}()
					cp.Edit(selfEdit(g.Src, 6+i%4))
				}()
				cp.Release()
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				cp := base.Copy()
				cp.Edit(selfEdit(g.Src, 7)) // an edit of this tree may meet the limit: a quiet panic is allowed
				nt, stop, pan := parseCatch(q, g.Src, cp)
				if pan != nil {
					if err, ok := pan.(error); !ok || !strings.Contains(err.Error(), "limit") {
						t.Errorf("unexpected panic from the parse: %v", pan)
					}
				} else if nt != nil {
					nt.Release()
				} else if stop == "" {
					t.Errorf("no tree and no stop")
				}
				cp.Release()
			}
		}()
		wg.Wait()
		base.Release()
	}
}

// A fresh parse whose finished tree reaches an internal node twice counts the
// owner that the second slot adds (the fresh parse has no publication walk):
// the reference word of the node is 1 and releasing the tree frees every record
// once.
func TestFreshParseCountsANodeTheTreeReachesTwice(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	target := p.DuplicateAChildBeforeBalance()
	tr, stop := p.Parse(g.Src)
	if stop != "" || tr == nil {
		t.Fatalf("stop %q", stop)
	}
	if *target == 0 {
		t.Fatal("the hook found no node to duplicate")
	}
	if got := ccore.RefCount(tr, *target); got != 1 {
		t.Fatalf("the reference word of the node reached twice is %d, want 1", got)
	}
	if p.SharedInTree() == 0 {
		t.Error("the shared-in-tree counter did not see the node reached twice")
	}
	double0 := p.DoubleFree()
	tr.Release()
	if p.DoubleFree() != double0 {
		t.Error("a record was released twice")
	}
	if !ccore.Ended(tr) {
		t.Error("the store did not end")
	}
}

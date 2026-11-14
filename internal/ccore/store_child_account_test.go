package ccore

import (
	"encoding/json"
	"os"
	"testing"
)

func volumeParser(t *testing.T) *Parser {
	t.Helper()
	b, e := os.ReadFile("testdata/tables/json.json")
	if e != nil {
		t.Fatal(e)
	}
	var tab Tables
	if e = json.Unmarshal(b, &tab); e != nil {
		t.Fatal(e)
	}
	tab.BuildAscii()
	p, e := NewParser(Config{Tables: &tab})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestChildVolumeTicketPause(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	s := c.view()
	reserved, advance, acquired, commit, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := hookFn(func(point string) {
		switch point {
		case "child-reserved":
			close(reserved)
			<-advance
		case "child-acquired":
			close(acquired)
			<-commit
		}
	})
	c.hook.Store(&h)
	go func() { s.growKids(&c.pc); close(done) }()
	<-reserved
	h0, p0, v0, ok := c.partialChildSnapshot()
	if !ok || h0 != 0 || p0 != childPageCharge || v0 != childPageCharge {
		t.Errorf("reserved %d/%d/%d %v", h0, p0, v0, ok)
	}
	close(advance)
	<-acquired
	h1, p1, v1, ok := c.partialChildSnapshot()
	if !ok || h1 != h0 || p1 != p0 || v1 != v0 {
		t.Errorf("acquired %d/%d/%d %v", h1, p1, v1, ok)
	}
	close(commit)
	<-done
	h2, p2, v2, ok := c.partialChildSnapshot()
	if !ok || h2 != childPageCharge || p2 != 0 || v2 != v0 {
		t.Errorf("commit %d/%d/%d %v", h2, p2, v2, ok)
	}
	c.hook.Store(nil)
	c.lease.Lock()
	c.cells.Store(0)
	c.endLocked()
	c.lease.Unlock()
	_, _, _, ok = c.partialChildSnapshot()
	if ok || c.childAccount.Load() != nil {
		t.Fatal("end did not detach")
	}
}
func TestChildVolumeCancelAndGeneration(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	s := c.view()
	c.kidPageLimit.Store(0)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected limit")
			}
		}()
		s.growKids(&c.pc)
	}()
	h, p, v, ok := c.partialChildSnapshot()
	if !ok || h != 0 || p != 0 || v != childPageCharge {
		t.Fatalf("cancel %d/%d/%d %v", h, p, v, ok)
	}
	ticket := c.reserveChildPage()
	c.mu.Lock()
	c.gen++
	c.mu.Unlock()
	ticket.cancel()
	_, _, _, ok = c.partialChildSnapshot()
	if ok {
		t.Fatal("old-generation ticket accepted")
	}
}
func TestChildVolumeOverflow(t *testing.T) {
	c := newStoreCore()
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	c.mu.Lock()
	c.childAccount.Load().volume = ^uint64(0) - childPageCharge + 1
	c.mu.Unlock()
	ticket := c.reserveChildPage()
	if ticket.active {
		t.Fatal("overflow accepted")
	}
	_, _, _, ok := c.partialChildSnapshot()
	if ok {
		t.Fatal("overflow not latched")
	}
}
func TestChildVolumeConcurrentEdit(t *testing.T) {
	p := volumeParser(t)
	src := []byte(`{"a":[1,2,3,4]}`)
	old, stop := p.Parse(src)
	if old == nil || stop != "" {
		t.Fatal(stop)
	}
	defer old.Release()
	cp := old.Copy()
	defer cp.Release()
	c := old.c.store
	c.emu.Lock()
	c.mu.Lock()
	c.ec.kidOff = kidPageLen
	clear(c.editFreeKid[:])
	c.mu.Unlock()
	c.emu.Unlock()
	started, resume, editPaused, editGo, editDone, parseDone := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := hookFn(func(point string) {
		switch point {
		case "parse-start":
			if !c.attachChildAccountUnderLease() {
				t.Error("parse install")
			}
			close(started)
			<-resume
		case "child-acquired":
			close(editPaused)
			<-editGo
		}
	})
	c.hook.Store(&h)
	var tr *Tree
	var st string
	go func() { tr, st = p.ParseWith(src, old); close(parseDone) }()
	<-started
	held, pending, vol, ok := c.partialChildSnapshot()
	if !ok || pending != 0 {
		t.Fatal("initial snapshot")
	}
	go func() {
		cp.Edit(Edit{StartByte: 6, OldEndByte: 7, NewEndByte: 8, StartCol: 6, OldEndCol: 7, NewEndCol: 8})
		close(editDone)
	}()
	<-editPaused
	h2, p2, v2, ok := c.partialChildSnapshot()
	if !ok || h2 != held || p2 != childPageCharge || v2-vol != childPageCharge {
		t.Errorf("in flight %d/%d/%d", h2, p2, v2)
	}
	close(editGo)
	<-editDone
	h3, p3, v3, ok := c.partialChildSnapshot()
	if !ok || h3-held != childPageCharge || p3 != 0 || v3-vol != childPageCharge {
		t.Errorf("committed %d/%d/%d", h3, p3, v3)
	}
	c.hook.Store(nil)
	close(resume)
	<-parseDone
	if tr != nil {
		tr.Release()
	}
	if st != "" || !p.LeaseFree() {
		t.Fatal("parse/lease", st)
	}
	t.Logf("EDIT_HANDOFF held_delta=%d pending_seen=%d volume_delta=%d", h3-held, p2, v3-vol)
}
func TestChildVolumePendingEditKeepsStore(t *testing.T) {
	p := volumeParser(t)
	old, stop := p.Parse([]byte(`{"a":[1,2,3,4]}`))
	if old == nil || stop != "" {
		t.Fatal(stop)
	}
	cp := old.Copy()
	if cp == nil {
		t.Fatal("copy")
	}
	c := old.c.store
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	c.emu.Lock()
	c.mu.Lock()
	c.ec.kidOff = kidPageLen
	clear(c.editFreeKid[:])
	c.mu.Unlock()
	c.emu.Unlock()
	paused, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := hookFn(func(point string) {
		if point == "child-acquired" {
			close(paused)
			<-resume
		}
	})
	c.hook.Store(&h)
	go func() {
		cp.Edit(Edit{StartByte: 6, OldEndByte: 7, NewEndByte: 8, StartCol: 6, OldEndCol: 7, NewEndCol: 8})
		close(done)
	}()
	<-paused
	old.Release()
	cp.Release()
	_, pending, _, ok := c.partialChildSnapshot()
	if !ok || pending != childPageCharge || c.ended.Load() {
		t.Errorf("store ended while edit owns ticket")
	}
	c.hook.Store(nil)
	close(resume)
	<-done
	if !c.ended.Load() {
		t.Fatal("store not ended after last edit")
	}
	_, _, _, ok = c.partialChildSnapshot()
	if ok || c.childAccount.Load() != nil {
		t.Fatal("ended sidecar retained")
	}
}

func TestChildVolumeAttachPullOnce(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	base, cursor, ok := c.childVolumeAttach()
	if !ok || base != 0 {
		t.Fatal("attach")
	}
	ticket := c.reserveChildPage()
	base2, cursor2, ok := c.childVolumeAttach()
	if !ok || base2 != childPageCharge {
		t.Fatal("pending omitted from baseline")
	}
	ticket.cancel()
	delta, ok := cursor.pull(c)
	if !ok || delta != childPageCharge {
		t.Fatal("transient growth omitted")
	}
	again, ok := cursor.pull(c)
	if !ok || again != 0 {
		t.Fatal("growth charged twice")
	}
	delta2, ok := cursor2.pull(c)
	if !ok || delta2 != 0 {
		t.Fatal("pending baseline double counted")
	}
	c.lease.Lock()
	c.cells.Store(0)
	c.endLocked()
	c.lease.Unlock()
	if _, ok = cursor.pull(c); ok {
		t.Fatal("ended generation accepted")
	}
}
func TestChildVolumeAttachWaitsForUntrackedEdit(t *testing.T) {
	p := volumeParser(t)
	old, stop := p.Parse([]byte(`{"a":[1,2,3,4]}`))
	if old == nil || stop != "" {
		t.Fatal(stop)
	}
	defer old.Release()
	cp := old.Copy()
	defer cp.Release()
	c := old.c.store
	c.emu.Lock()
	c.mu.Lock()
	c.ec.kidOff = kidPageLen
	clear(c.editFreeKid[:])
	c.mu.Unlock()
	c.emu.Unlock()
	paused, resume, editDone, attaching, attachDone, blocked, ack := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := hookFn(func(point string) {
		if point == "edit" {
			close(paused)
			<-resume
		}
		if point == "child-attach-blocked" {
			close(blocked)
			<-ack
		}
	})
	c.hook.Store(&h)
	go func() {
		cp.Edit(Edit{StartByte: 6, OldEndByte: 7, NewEndByte: 8, StartCol: 6, OldEndCol: 7, NewEndCol: 8})
		close(editDone)
	}()
	<-paused
	go func() {
		c.lease.Lock()
		close(attaching)
		if !c.attachChildAccountUnderLease() {
			t.Error("attach")
		}
		c.lease.Unlock()
		close(attachDone)
	}()
	<-attaching
	<-blocked
	if c.childAccount.Load() != nil {
		t.Error("installed while edit held emu")
	}
	c.childAccount.Store(&childPageAccount{})
	if c.childAccount.Load() == nil {
		t.Error("negative control missed bad installation")
	}
	c.childAccount.Store(nil)
	close(ack)
	close(resume)
	<-editDone
	<-attachDone
	c.hook.Store(nil)
	held, pending, volume, ok := c.partialChildSnapshot()
	c.mu.Lock()
	pages := len(c.kids)
	c.mu.Unlock()
	if !ok || pending != 0 || volume != 0 || held != uint64(pages)*childPageCharge || pages < 2 {
		t.Fatalf("baseline pages=%d held=%d pending=%d volume=%d", pages, held, pending, volume)
	}
}
func TestChildSidecarRequestDetachAndReuse(t *testing.T) {
	p := volumeParser(t)
	old, stop := p.Parse([]byte(`{"a":[1,2,3,4]}`))
	if old == nil || stop != "" {
		t.Fatal(stop)
	}
	cp := old.Copy()
	c := old.c.store
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	base, cursor, ok := c.childVolumeAttach()
	if !ok || base == 0 {
		t.Fatal("baseline")
	}
	c.emu.Lock()
	c.mu.Lock()
	c.ec.kidOff = kidPageLen
	clear(c.editFreeKid[:])
	c.mu.Unlock()
	c.emu.Unlock()
	cp.Edit(Edit{StartByte: 6, OldEndByte: 7, NewEndByte: 8, StartCol: 6, OldEndCol: 7, NewEndCol: 8})
	d, ok := cursor.finish(c)
	if !ok || d != childPageCharge {
		t.Fatalf("delta=%d", d)
	}
	if _, ok = cursor.pull(c); ok {
		t.Fatal("detached cursor valid")
	}
	base2, next, ok := c.childVolumeAttach()
	if !ok || base2 != base+childPageCharge {
		t.Fatal("second baseline")
	}
	if d, ok = next.pull(c); !ok || d != 0 {
		t.Fatal("double count")
	}
	cp.Release()
	old.Release()
	if c.childAccount.Load() != nil || !c.ended.Load() {
		t.Fatal("last release sidecar")
	}
	if _, ok = next.pull(c); ok {
		t.Fatal("ended cursor valid")
	}
	fresh, s := p.Parse([]byte(`[1]`))
	if fresh == nil || s != "" {
		t.Fatal("parser reuse", s)
	}
	if fresh.c.store.childAccount.Load() != nil {
		t.Fatal("new store inherited sidecar")
	}
	fresh.Release()
}

func TestChildVolumePanicBeforeCheckout(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	s := c.view()
	h := hookFn(func(point string) {
		if point == "child-reserved" {
			panic("fault")
		}
	})
	c.hook.Store(&h)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		s.growKids(&c.pc)
	}()
	c.hook.Store(nil)
	held, pending, volume, ok := c.partialChildSnapshot()
	if !ok || held != 0 || pending != 0 || volume != childPageCharge {
		t.Fatalf("panic held=%d pending=%d volume=%d valid=%v", held, pending, volume, ok)
	}
}

func (c *storeCore) partialChildSnapshot() (held, pending, volume uint64, valid bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.childAccount.Load()
	if a == nil {
		return 0, 0, 0, false
	}
	return a.held, a.pending, a.volume, !a.overflow
}

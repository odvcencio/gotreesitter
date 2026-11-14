package ccore

import (
	"runtime"
	"testing"
	"unsafe"
)

func fixedTestCharge(k fixedPageKind) uint64 {
	if k == fixedRecord {
		return uint64(recPageBytes())
	}
	return uint64(stPageLen)
}
func fixedTestGrow(s *Store, c *storeCore, k fixedPageKind) {
	if k == fixedRecord {
		s.growRec(&c.pc)
	} else {
		s.growState(&c.pc)
	}
}
func fixedTestSnapshot(c *storeCore, k fixedPageKind) (uint64, uint64, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.childAccount.Load()
	if a == nil {
		return 0, 0, 0, false
	}
	x, _ := a.fixed(k)
	return x.held, x.pending, x.volume, !x.overflow
}
func TestFixedPageTicketInterleaving(t *testing.T) {
	for _, k := range []fixedPageKind{fixedRecord, fixedState} {
		t.Run(map[fixedPageKind]string{fixedRecord: "record", fixedState: "state"}[k], func(t *testing.T) {
			c := newStoreCore()
			c.cells.Store(1)
			c.lease.Lock()
			if !c.attachChildAccountUnderLease() {
				t.Fatal("install")
			}
			c.lease.Unlock()
			base, first, ok := c.fixedPageAttach(k)
			if !ok || base != 0 {
				t.Fatal("baseline")
			}
			paused, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			event := "fixed-record-acquired"
			if k == fixedState {
				event = "fixed-state-acquired"
			}
			h := hookFn(func(point string) {
				if point == event {
					close(paused)
					<-resume
				}
			})
			c.hook.Store(&h)
			s := c.view()
			go func() { fixedTestGrow(&s, c, k); close(done) }()
			<-paused
			held, pending, vol, ok := fixedTestSnapshot(c, k)
			if !ok || held != 0 || pending != fixedTestCharge(k) || vol != pending {
				t.Errorf("pause %d %d %d", held, pending, vol)
			}
			base2, second, ok := c.fixedPageAttach(k)
			if !ok || base2 != pending {
				t.Error("pending baseline")
			}
			close(resume)
			<-done
			c.hook.Store(nil)
			held, pending, vol, ok = fixedTestSnapshot(c, k)
			if !ok || held != fixedTestCharge(k) || pending != 0 {
				t.Fatal("commit")
			}
			if d, ok := first.pull(c, true); !ok || d != vol {
				t.Fatal("first delta")
			}
			if d, ok := second.pull(c, true); !ok || d != 0 {
				t.Fatal("double charge")
			}
			c.lease.Lock()
			c.cells.Store(0)
			c.endLocked()
			c.lease.Unlock()
			if c.childAccount.Load() != nil {
				t.Fatal("sidecar retained")
			}
		})
	}
}
func TestFixedPageFailureAndOverflow(t *testing.T) {
	for _, k := range []fixedPageKind{fixedRecord, fixedState} {
		t.Run(map[fixedPageKind]string{fixedRecord: "record", fixedState: "state"}[k], func(t *testing.T) {
			c := newStoreCore()
			c.cells.Store(1)
			c.lease.Lock()
			if !c.attachChildAccountUnderLease() {
				t.Fatal("install")
			}
			c.lease.Unlock()
			s := c.view()
			if k == fixedRecord {
				fixedTestGrow(&s, c, k)
				c.recLimit.Store(0)
			} else {
				c.statePageLimit.Store(0)
			}
			h0, _, v0, ok := fixedTestSnapshot(c, k)
			if !ok {
				t.Fatal("initial")
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Error("expected limit")
					}
				}()
				fixedTestGrow(&s, c, k)
			}()
			held, pending, vol, ok := fixedTestSnapshot(c, k)
			if !ok || held != h0 || pending != 0 || vol-v0 != fixedTestCharge(k) {
				t.Fatalf("failed checkout %d %d %d", held, pending, vol)
			}
			c.mu.Lock()
			a := c.childAccount.Load()
			x, n := a.fixed(k)
			x.volume = ^uint64(0) - n + 1
			c.mu.Unlock()
			if tk := c.reserveFixedPage(k); tk.active {
				t.Fatal("overflow accepted")
			}
			_, _, _, ok = fixedTestSnapshot(c, k)
			if ok {
				t.Fatal("overflow not latched")
			}
		})
	}
}
func TestFixedRecordConcurrentEdit(t *testing.T) {
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
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("install")
	}
	c.lease.Unlock()
	c.emu.Lock()
	c.mu.Lock()
	c.ec.recSlot = recPageLen
	c.editFreeRec = 0
	c.mu.Unlock()
	c.emu.Unlock()
	ready, resume, editPaused, editGo, editDone, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := hookFn(func(point string) {
		switch point {
		case "parse-start":
			close(ready)
			<-resume
		case "fixed-record-acquired":
			close(editPaused)
			<-editGo
		}
	})
	c.hook.Store(&h)
	var tr *Tree
	var st string
	go func() { tr, st = p.ParseWith(src, old); close(done) }()
	<-ready
	held, _, vol, ok := fixedTestSnapshot(c, fixedRecord)
	if !ok {
		t.Fatal("initial")
	}
	go func() {
		cp.Edit(Edit{StartByte: 6, OldEndByte: 7, NewEndByte: 8, StartCol: 6, OldEndCol: 7, NewEndCol: 8})
		close(editDone)
	}()
	<-editPaused
	h2, p2, v2, ok := fixedTestSnapshot(c, fixedRecord)
	if !ok || h2 != held || p2 != uint64(recPageBytes()) || v2-vol != p2 {
		t.Error("in-flight")
	}
	close(editGo)
	<-editDone
	c.hook.Store(nil)
	close(resume)
	<-done
	if tr != nil {
		tr.Release()
	}
	if st != "" || !p.LeaseFree() {
		t.Fatal("parse", st)
	}
	t.Logf("RECORD_EDIT pending=%d bytes", p2)
}

func TestFixedRecordPendingEditKeepsStore(t *testing.T) {
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
	c.ec.recSlot = recPageLen
	c.editFreeRec = 0
	c.mu.Unlock()
	c.emu.Unlock()
	paused, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := hookFn(func(point string) {
		if point == "fixed-record-acquired" {
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
	_, pending, _, ok := fixedTestSnapshot(c, fixedRecord)
	if !ok || pending != uint64(recPageBytes()) || c.ended.Load() {
		t.Errorf("store ended while edit owns ticket")
	}
	c.hook.Store(nil)
	close(resume)
	<-done
	if !c.ended.Load() {
		t.Fatal("store not ended after last edit")
	}
	_, _, _, ok = fixedTestSnapshot(c, fixedRecord)
	if ok || c.childAccount.Load() != nil {
		t.Fatal("ended sidecar retained")
	}
}

func TestFixedPagesCombinedSnapshot(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	child := c.reserveChildPage()
	rec := c.reserveFixedPage(fixedRecord)
	state := c.reserveFixedPage(fixedState)
	s, p, ok := c.fixedPagesAttach()
	if !ok || s.held != [3]uint64{childPageCharge, uint64(recPageBytes()), uint64(stPageLen)} || s.watermark != s.held {
		t.Fatalf("snapshot %+v", s)
	}
	child.cancel()
	rec.cancel()
	state.cancel()
	if d, ok := p.pull(c, false); !ok || d != [3]uint64{} {
		t.Fatal("canceled pending double counted", d, ok)
	}
	rec = c.reserveFixedPage(fixedRecord)
	rec.cancel()
	if d, ok := p.pull(c, true); !ok || d != [3]uint64{0, uint64(recPageBytes()), 0} {
		t.Fatal("delta", d, ok)
	}
	if _, ok := p.pull(c, false); ok {
		t.Fatal("finished cursor accepted")
	}
	c.mu.Lock()
	c.childAccount.Load().state.overflow = true
	c.mu.Unlock()
	if _, _, ok := c.fixedPagesAttach(); ok {
		t.Fatal("overflow admitted")
	}
}

func TestFixedPageInstallRefusal(t *testing.T) {
	for _, mode := range []string{"unpaired", "bad-state"} {
		t.Run(mode, func(t *testing.T) {
			c := newStoreCore()
			c.cells.Store(1)
			if mode == "unpaired" {
				c.recs = append(c.recs, nil)
			} else {
				c.state = append(c.state, make([]byte, 1))
			}
			c.lease.Lock()
			ok := c.attachChildAccountUnderLease()
			c.lease.Unlock()
			if ok || c.childAccount.Load() != nil {
				t.Fatal("invalid page layout published")
			}
		})
	}
}
func TestFixedPagesCursorInvalidation(t *testing.T) {
	for _, mode := range []string{"child", "record", "state", "wrong-store", "generation", "end"} {
		t.Run(mode, func(t *testing.T) {
			c := newStoreCore()
			c.cells.Store(1)
			c.lease.Lock()
			if !c.attachChildAccountUnderLease() {
				t.Fatal("attach")
			}
			c.lease.Unlock()
			_, p, ok := c.fixedPagesAttach()
			if !ok {
				t.Fatal("snapshot")
			}
			other := c
			switch mode {
			case "child":
				c.mu.Lock()
				c.childAccount.Load().overflow = true
				c.mu.Unlock()
			case "record":
				c.mu.Lock()
				c.childAccount.Load().record.overflow = true
				c.mu.Unlock()
			case "state":
				c.mu.Lock()
				c.childAccount.Load().state.overflow = true
				c.mu.Unlock()
			case "wrong-store":
				other = newStoreCore()
			case "generation":
				c.mu.Lock()
				c.gen++
				c.mu.Unlock()
			case "end":
				c.lease.Lock()
				c.cells.Store(0)
				c.endLocked()
				c.lease.Unlock()
			}
			if _, ok = p.pull(other, false); ok || p.valid || p.a != nil {
				t.Fatal("invalid cursor survived")
			}
			if _, ok = p.pull(c, true); ok {
				t.Fatal("invalidated cursor accepted")
			}
		})
	}
}
func TestFixedPagePanicBeforeCheckout(t *testing.T) {
	for _, k := range []fixedPageKind{fixedRecord, fixedState} {
		t.Run(map[fixedPageKind]string{fixedRecord: "record", fixedState: "state"}[k], func(t *testing.T) {
			c := newStoreCore()
			c.cells.Store(1)
			c.lease.Lock()
			if !c.attachChildAccountUnderLease() {
				t.Fatal("attach")
			}
			c.lease.Unlock()
			event := "fixed-record-reserved"
			if k == fixedState {
				event = "fixed-state-reserved"
			}
			h := hookFn(func(s string) {
				if s == event {
					panic("injected")
				}
			})
			c.hook.Store(&h)
			s := c.view()
			func() {
				defer func() {
					if recover() == nil {
						t.Error("expected panic")
					}
				}()
				fixedTestGrow(&s, c, k)
			}()
			c.hook.Store(nil)
			held, pending, volume, ok := fixedTestSnapshot(c, k)
			if !ok || held != 0 || pending != 0 || volume != fixedTestCharge(k) {
				t.Fatalf("%d %d %d %v", held, pending, volume, ok)
			}
		})
	}
}
func TestFixedPageChargeSize(t *testing.T) {
	want := unsafe.Sizeof([recPageLen]rec{}) + unsafe.Sizeof([recPageLen]uint32{})
	if uintptr(recPageBytes()) != want {
		t.Fatalf("record/ref charge=%d arrays=%d", recPageBytes(), want)
	}
	t.Logf("record_ref=%d state=%d sidecar_struct=%d store=%d", want, stPageLen, unsafe.Sizeof(childPageAccount{}), unsafe.Sizeof(storeCore{}))
}
func TestFixedPageSidecarAllocation(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	run := func() {
		c.lease.Lock()
		if !c.attachChildAccountUnderLease() {
			panic("attach")
		}
		c.lease.Unlock()
		c.childAccount.Store(nil)
	}
	run()
	a := testing.AllocsPerRun(100, run)
	var x, y runtime.MemStats
	runtime.ReadMemStats(&x)
	run()
	runtime.ReadMemStats(&y)
	t.Logf("SIDECAR allocs=%.0f bytes=%d", a, y.TotalAlloc-x.TotalAlloc)
}

// A failed second table preparation must not publish only a record page.
func TestFixedRecordTablePreparationPanic(t *testing.T) {
	c := newStoreCore()
	c.cells.Store(1)
	c.lease.Lock()
	if !c.attachChildAccountUnderLease() {
		t.Fatal("attach")
	}
	c.lease.Unlock()
	s := c.view()
	for i := 0; i < inlinePages; i++ {
		s.growRec(&c.pc)
	}
	held, _, volume, ok := fixedTestSnapshot(c, fixedRecord)
	if !ok {
		t.Fatal("snapshot")
	}
	h := hookFn(func(point string) {
		if point == "fixed-record-table-prepared" {
			panic("injected")
		}
	})
	c.hook.Store(&h)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		s.growRec(&c.pc)
	}()
	c.hook.Store(nil)
	if len(c.recs) != inlinePages || len(c.refs) != inlinePages || len(s.recs) != inlinePages || len(s.refs) != inlinePages {
		t.Fatal("partial table publication")
	}
	held2, pending, volume2, ok := fixedTestSnapshot(c, fixedRecord)
	if !ok || held2 != held || pending != 0 || volume2-volume != uint64(recPageBytes()) {
		t.Fatalf("account: held=%d pending=%d volume delta=%d ok=%v", held2, pending, volume2-volume, ok)
	}
	s.growRec(&c.pc)
	if len(c.recs) != inlinePages+1 || len(c.refs) != inlinePages+1 {
		t.Fatal("retry")
	}
}

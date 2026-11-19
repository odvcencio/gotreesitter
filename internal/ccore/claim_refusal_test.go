package ccore

import "testing"

// A record leaves the free list only after the log of taken records has room
// for it. When the ledger refuses that room, the free list must be as it was:
// the sweep after a stopped parse finds taken records only through the log, so
// a record unlinked before the refusal would be lost to the store.
func TestClaimRefusedKeepsTheFreeList(t *testing.T) {
	p := linBuild(t, linTables(t), []linItem{{"leaf", 2}, {"leaf", 3}, {"leaf", 4}})
	s := &p.Store
	c := s.core
	// One record from the fresh path, so that a record page exists; its slot is
	// then put on the free list by hand.
	idx := uint32(s.claim() >> 1)
	c.mu.Lock()
	s.syncLocked() // the view reads the record pages through headers it copies from the core
	c.mu.Unlock()
	if int(idx>>recShift) >= len(s.refs) {
		t.Fatalf("no record page for slot %d", idx)
	}
	w := &s.refs[idx>>recShift][idx&recMask]
	*w = 0 // the link of the last free record: no next one
	c.freeRec = idx
	// A full log large enough that its growth is over the ledger's polling
	// threshold, so the reservation asks the limit.
	c.taken = make([]uint32, LedgerGranularity/4)
	s.logging = true
	var led Ledger
	asked := 0
	led.SetPoll(func(int64, bool) bool { asked++; return asked == 1 }) // Begin passes, the reservation is refused
	if !led.Begin(0) {
		t.Fatal("the ledger did not begin")
	}
	s.led = &led
	stopped := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(ledgerStop); !ok {
					panic(r)
				}
				stopped = true
			}
		}()
		s.claim()
	}()
	if !stopped {
		t.Fatal("claim did not stop when the ledger refused room in the log")
	}
	if c.freeRec != idx {
		t.Fatalf("the free list starts at %d after the refusal, want %d: the record was unlinked before the log had room", c.freeRec, idx)
	}
	if got := len(c.taken); got != LedgerGranularity/4 {
		t.Fatalf("the log has %d entries after the refusal, want %d", got, LedgerGranularity/4)
	}
}

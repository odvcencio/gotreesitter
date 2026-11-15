package ccore

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"unsafe"
)

func TestBufAfterStopCountsOnlyLiveBuffers(t *testing.T) {
	p := &Parser{}
	p.bufs = append(p.bufs, make([]Handle, 0, 32))
	before := p.bufferBytes()
	b := p.getBuf() // a stopped operation can retain b elsewhere
	p.trailing = b
	if got := p.bufferBytes(); got != before {
		t.Fatalf("after stop got %d bytes, want %d live bytes", got, before)
	}
	p.trailing = nil
	p.Reset()
	want := cap(p.bufs) * int(unsafe.Sizeof([]Handle(nil)))
	if got := p.bufferBytes(); got != want {
		t.Fatalf("after reset got %d bytes, want %d live bytes", got, want)
	}
}

func TestLedgerMultiMapChargesBeforeInsert(t *testing.T) {
	p := &Parser{}
	p.led.SetPoll(func(int64, bool) bool { return false })
	p.led.pending = LedgerGranularity - 1
	func() {
		defer func() {
			if _, ok := recover().(ledgerStop); !ok {
				t.Error("map insertion did not refuse its reservation")
			}
		}()
		p.addMulti(2)
	}()
	if p.multi != nil {
		t.Fatal("map allocated before refusal")
	}
	p.led.SetPoll(func(int64, bool) bool { return true })
	p.addMulti(2)
	if p.multi[2] != 1 || p.led.Alloc < 256 {
		t.Fatalf("map %v, charged %d", p.multi, p.led.Alloc)
	}
}

func TestStoreScratchConcurrentLargeEdit(t *testing.T) {
	raw, err := os.ReadFile("testdata/tables/json.json")
	if err != nil {
		t.Fatal(err)
	}
	var tables Tables
	if err := json.Unmarshal(raw, &tables); err != nil {
		t.Fatal(err)
	}
	tables.BuildAscii()
	p, err := NewParser(Config{Tables: &tables})
	if err != nil {
		t.Fatal(err)
	}
	src := []byte(`[]`)
	base, stop := p.Parse(src)
	if stop != "" || base == nil {
		t.Fatalf("initial parse: %q", stop)
	}
	defer base.Release()
	c := base.c.store
	c.lease.Lock()
	s := c.view()
	s.beginParse(false)
	root := buildTree(&s, 1, 4100)
	s.rec(root).size = Length{bytes: 4100 * 300, col: 4100 * 300}
	s.publish(root)
	large := &Tree{c: c.newCell(root), rd: s}
	large.rd.owner = large.c
	c.lease.Unlock()
	defer large.Release()
	if s.childCount(root) <= 4096 {
		t.Fatal("large fixture did not have 4097 children")
	}
	keep := large.Copy()
	defer keep.Release()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			x := large.Copy()
			x.Edit(Edit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartCol: 1, OldEndCol: 2, NewEndCol: 2})
			x.Release()
		}
	}()
	q, err := NewParser(Config{Tables: &tables})
	if err != nil {
		t.Fatal(err)
	}
	q.Ledger().SetPoll(func(int64, bool) bool { return true })
	for i := 0; i < 200; i++ {
		x, stop := q.ParseWith(src, base)
		if stop != "" || x == nil {
			t.Errorf("parse: %q", stop)
			break
		}
		x.Release()
	}
	wg.Wait()
}

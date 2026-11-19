package ccore

import (
	"runtime"
	"runtime/debug"
	"testing"
)

// The linear reduction (reduce_linear.go) makes the same kinds of arrays as the general
// walk: the buffer of popped subtrees, the child array of the parent, the stack node and
// the record. These tests run one long linear reduction under a ledger and compare the
// ledger with the allocation that really happened, then refuse a reservation inside the
// reduction and check that nothing was changed.

const linLong = 70000

func linLongParser(t *testing.T) (*Parser, Symbol) {
	t.Helper()
	tb := linTables(t)
	items := make([]linItem, linLong)
	for i := range items {
		items[i] = linItem{"leaf", uint16(2 + i%3)}
	}
	return linBuild(t, tb, items), Symbol(tb.TokenCount)
}

func TestLinearReductionCountsEveryArrayItMakes(t *testing.T) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range bi.Settings {
			if kv.Key == "-race" && kv.Value == "true" {
				t.Skip("the race detector changes the allocation counters this test compares with the ledger")
			}
		}
	}
	for _, check := range []bool{false, true} {
		DropPooledPages() // a pooled page is charged when claimed but costs the heap nothing: start from an empty pool
		p, nt := linLongParser(t)
		p.SetReduceCheck(check)
		p.led = Ledger{}
		p.Store.led = &p.led // pages are reserved on the same account
		p.led.SetPoll(func(int64, bool) bool { return true })
		p.led.Begin(0)
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		rv := p.reduce(0, nt, linLong, 0, 0, false, false)
		runtime.ReadMemStats(&m1)
		real := int64(m1.TotalAlloc - m0.TotalAlloc)
		if p.FastReductions() != 1 {
			t.Fatalf("check %t: %d fast reductions, want 1: the reduction did not take the linear path", check, p.FastReductions())
		}
		if rv != 1 {
			t.Fatalf("check %t: version %d, want 1", check, rv)
		}
		counted := p.led.Alloc
		t.Logf("check %t: counted %d bytes, allocated %d (%.1f%%)", check, counted, real, 100*float64(counted)/float64(real))
		// With the cross-check on, the general walk runs too and its arrays are counted too.
		// Counting an array twice would raise the ceiling falsely; missing one would break the floor.
		if float64(counted) < 0.90*float64(real) || float64(counted) > 1.05*float64(real)+16384 {
			t.Fatalf("check %t: counted %d bytes for %d bytes really allocated; want 0.90 x allocated <= counted <= 1.05 x allocated + 16384", check, counted, real)
		}
		if need := int64(linLong) * ElemBytes[Handle](); counted < need {
			t.Fatalf("check %t: counted %d bytes, less than the %d bytes of the popped subtrees alone", check, counted, need)
		}
	}
}

func TestLinearReductionRefusedInsideLeavesTheStackAsItWas(t *testing.T) {
	p, nt := linLongParser(t)
	p.led = Ledger{}
	p.Store.led = &p.led    // pages are reserved on the same account
	const limit = 300 << 10 // the buffer of popped subtrees reaches 256 KiB and more while it grows
	refused := false
	p.led.SetPoll(func(total int64, _ bool) bool {
		if total > limit {
			refused = true
			return false
		}
		return true
	})
	p.led.Begin(0)
	headsBefore, nodesBefore, recsBefore, top := len(p.heads), len(p.nodes), len(p.recs), p.heads[0].node
	refsBefore := p.nodes[top].refCount
	allocBefore := p.led.Alloc
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	var got any
	func() {
		defer func() { got = recover() }()
		p.reduce(0, nt, linLong, 0, 0, false, false)
	}()
	runtime.ReadMemStats(&m1)
	if _, ok := got.(ledgerStop); !ok || !refused {
		t.Fatalf("panic %v, refused %t: the reduction did not stop at the ledger", got, refused)
	}
	if p.FastReductions() != 0 {
		t.Fatalf("%d fast reductions counted for a reduction that was refused", p.FastReductions())
	}
	if len(p.heads) != headsBefore || len(p.nodes) != nodesBefore || len(p.recs) != recsBefore || p.heads[0].node != top || p.nodes[top].refCount != refsBefore {
		t.Fatalf("the refused reduction changed the stack: heads %d->%d nodes %d->%d recs %d->%d", headsBefore, len(p.heads), nodesBefore, len(p.nodes), recsBefore, len(p.recs))
	}
	if p.led.Alloc < allocBefore || p.led.Alloc > limit {
		t.Fatalf("charged total %d after the refusal; it must not pass the limit %d", p.led.Alloc, limit)
	}
	if real := int64(m1.TotalAlloc - m0.TotalAlloc); real > limit+LedgerGranularity {
		t.Fatalf("%d bytes allocated before the refusal; the limit is %d plus %d", real, limit, LedgerGranularity)
	}
}

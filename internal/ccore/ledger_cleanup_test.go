package ccore

import (
	"math"
	"testing"
	"unsafe"
)

func TestLedgerForcePromptsNextReserve(t *testing.T) {
	calls := 0
	var l Ledger
	l.SetPoll(func(total int64, _ bool) bool { calls++; return total <= 1024 })
	if !l.Begin(0) {
		t.Fatal("begin")
	}
	l.Force(1 << 20)
	before := l.Total()
	if l.Reserve(64) {
		t.Fatal("reservation bypassed forced cleanup charge")
	}
	if calls != 2 || l.Total() != before {
		t.Fatalf("calls=%d total=%d", calls, l.Total())
	}
}
func TestLedgerGrowToNoGrowthRetainsSlice(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length int
	}{{"empty", 0}, {"nonempty", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if v := recover(); v != nil {
					t.Errorf("no-growth case panicked: %v", v)
				}
			}()
			s := make([]int, tc.length, 4)
			before := unsafe.SliceData(s)
			n, ok := GrowSize(s, 1)
			if !ok || n != 0 {
				t.Fatal("setup")
			}
			got := GrowTo(s, n)
			if len(got) != len(s) || cap(got) != cap(s) || unsafe.SliceData(got) != before {
				t.Fatal("changed slice")
			}
		})
	}
}
func TestLedgerForcePollAccountingEdges(t *testing.T) {
	calls := 0
	var l Ledger
	l.SetPoll(func(int64, bool) bool { calls++; return true })
	l.Begin(0)
	l.Force(0)
	if !l.Reserve(1) || calls != 1 {
		t.Fatal("zero Force should not trigger poll")
	}
	l.Force(1)
	l.Force(1)
	if !l.Reserve(1) || calls != 2 || l.Total() != 4 {
		t.Fatal("forced charge not polled exactly once")
	}
	if !l.Reserve(1) || calls != 2 {
		t.Fatal("successful poll did not reset pending")
	}
	l.Force(math.MaxInt64)
	if !l.overflow || l.Check(false) {
		t.Fatal("overflow did not fail closed")
	}
	var noPoll Ledger
	noPoll.Force(1)
	if noPoll.Total() != 0 {
		t.Fatal("unlimited Force changed accounting")
	}
	var nilLedger *Ledger
	nilLedger.Force(1)
}

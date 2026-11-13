package ccore

import (
	"math"
	"testing"
	"unsafe"
)

func TestLedgerReserveRefusal(t *testing.T) {
	var checks []int64
	l := &Ledger{}
	l.SetPoll(func(total int64, _ bool) bool { checks = append(checks, total); return total < LedgerGranularity })
	if !l.Begin(0) {
		t.Fatal("begin")
	}
	if !l.Reserve(LedgerGranularity - 1) {
		t.Fatal("first reserve")
	}
	before := l.Alloc
	if l.Reserve(1) {
		t.Fatal("cross-limit reserve accepted")
	}
	if l.Alloc != before || checks[len(checks)-1] != LedgerGranularity {
		t.Fatalf("wrong refusal alloc=%d checks=%v", l.Alloc, checks)
	}
}
func TestLedgerGrowthRefusal(t *testing.T) {
	l := &Ledger{}
	l.SetPoll(func(total int64, _ bool) bool { return total < LedgerGranularity })
	if !l.Begin(0) {
		t.Fatal("begin")
	}
	old := []uint64{11, 22}
	if !l.Reserve(LedgerGranularity - 1) {
		t.Fatal("prefill")
	}
	got, ok := Push(l, old, 33)
	if ok || len(got) != 2 || got[0] != 11 || got[1] != 22 || unsafe.SliceData(got) != unsafe.SliceData(old) {
		t.Fatal("refused append changed input")
	}
	if _, ok := Make[byte](l, LedgerGranularity); ok {
		t.Fatal("oversized make accepted")
	}
}
func TestLedgerForcedReclaimAndRebase(t *testing.T) {
	l := &Ledger{}
	l.SetPoll(func(total int64, _ bool) bool { return total < 100 })
	if !l.Begin(0) {
		t.Fatal("begin")
	}
	l.Force(101)
	if l.Total() != 101 || l.Check(false) {
		t.Fatal("forced reclaim not reported on next check")
	}
	l.SetPoll(func(int64, bool) bool { return true })
	l.Begin(50)
	l.Reserve(20)
	if !l.Rebase(20, 12) || l.Base != 62 || l.Alloc != 0 {
		t.Fatalf("rebase base=%d alloc=%d", l.Base, l.Alloc)
	}
}
func TestLedgerRefusalPreservesPending(t *testing.T) {
	calls := 0
	l := &Ledger{}
	l.SetPoll(func(total int64, _ bool) bool { calls++; return total < LedgerGranularity })
	if !l.Begin(0) || !l.Reserve(LedgerGranularity-1) {
		t.Fatal("setup")
	}
	before := *l
	if l.Reserve(1) {
		t.Fatal("first refusal accepted")
	}
	if l.Alloc != before.Alloc || l.pending != before.pending {
		t.Fatalf("refusal changed ledger: alloc %d/%d pending %d/%d", l.Alloc, before.Alloc, l.pending, before.pending)
	}
	if l.Reserve(1) {
		t.Fatal("second refused request was accepted")
	}
	if calls != 3 {
		t.Fatalf("poll calls=%d want 3", calls)
	}
}
func TestLedgerFailedCheckPreservesPending(t *testing.T) {
	allow := true
	l := &Ledger{}
	l.SetPoll(func(int64, bool) bool { return allow })
	if !l.Begin(0) || !l.Reserve(7) {
		t.Fatal("setup")
	}
	allow = false
	if l.Check(false) {
		t.Fatal("check accepted")
	}
	if l.pending != 7 || l.Alloc != 7 {
		t.Fatalf("failed check lost accounting: pending=%d alloc=%d", l.pending, l.Alloc)
	}
	allow = true
	if !l.Check(false) || l.pending != 0 || l.Alloc != 7 {
		t.Fatal("successful check did not clear pending")
	}
}

func TestLedgerRefusedRebaseRetainsHeldCharge(t *testing.T) {
	allow := true
	l := &Ledger{}
	l.SetPoll(func(int64, bool) bool { return allow })
	if !l.Begin(10) || !l.Reserve(20) {
		t.Fatal("setup")
	}
	allow = false
	if l.Rebase(20, 30) {
		t.Fatal("rebase accepted")
	}
	if l.Base != 40 || l.Alloc != 0 || l.pending != 20 {
		t.Fatalf("lost actual charge: %+v", l)
	}
}

func TestLedgerArithmeticBoundaries(t *testing.T) {
	l := &Ledger{}
	l.SetPoll(func(int64, bool) bool { return true })
	if l.Begin(-1) {
		t.Fatal("negative base accepted")
	}
	if !l.Begin(math.MaxInt64 - 1) {
		t.Fatal("begin")
	}
	if l.Reserve(-1) || l.Reserve(2) {
		t.Fatal("invalid reservation accepted")
	}
	if l.Total() != math.MaxInt64-1 || !l.Reserve(1) || l.Reserve(1) {
		t.Fatal("reservation overflow")
	}
	if !l.Begin(10) || !l.Reserve(5) {
		t.Fatal("setup")
	}
	if l.Rebase(6, 0) || l.Rebase(-1, 0) || l.Rebase(0, -1) || l.Total() != 15 {
		t.Fatal("invalid rebase")
	}
	if l.Rebase(0, math.MaxInt64) || l.Total() != math.MaxInt64 || l.Check(false) || l.Reserve(0) {
		t.Fatal("rebase overflow not terminal")
	}
	if !l.Begin(math.MaxInt64 - 1) {
		t.Fatal("reset")
	}
	l.Force(2)
	if l.Total() != math.MaxInt64 || l.Check(false) || l.Reserve(0) {
		t.Fatal("force overflow not terminal")
	}
	if !l.Begin(0) {
		t.Fatal("reset")
	}
	l.Force(-1)
	if l.Check(false) {
		t.Fatal("invalid force accepted")
	}
}
func TestLedgerSizeBoundaries(t *testing.T) {
	l := &Ledger{}
	l.SetPoll(func(int64, bool) bool { return true })
	l.Begin(0)
	s := []uint64{1}
	if got, ok := Grow(l, s, -1); ok || len(got) != 1 {
		t.Fatal("negative grow")
	}
	if _, ok := GrowSize(s, math.MaxInt); ok {
		t.Fatal("grow size overflow")
	}
	if _, ok := Make[uint64](l, math.MaxInt); ok {
		t.Fatal("make byte overflow")
	}
	if _, ok := MakeCap[uint64](l, -1); ok {
		t.Fatal("negative make")
	}
	if l.Total() != 0 {
		t.Fatal("invalid sizes changed charge")
	}
}

func TestLedgerStickyOverflowSurvivesPollRemoval(t *testing.T) {
	l := &Ledger{}
	l.SetPoll(func(int64, bool) bool { return true })
	l.Begin(math.MaxInt64)
	l.Force(1)
	l.SetPoll(nil)
	s := []int{1}
	if _, ok := Grow(l, s, 1); ok {
		t.Fatal("Grow accepted overflow")
	}
	if _, ok := Make[int](l, 1); ok {
		t.Fatal("Make accepted overflow")
	}
	if _, ok := MakeCap[int](l, 1); ok {
		t.Fatal("MakeCap accepted overflow")
	}
	if _, ok := Push(l, s, 2); ok {
		t.Fatal("Push accepted overflow")
	}
	if l.Check(false) || l.Reserve(0) {
		t.Fatal("overflow forgotten")
	}
	if !l.Begin(0) {
		t.Fatal("Begin did not reset overflow")
	}
}
func TestLedgerForcedCleanupSizing(t *testing.T) {
	l := &Ledger{}
	l.SetPoll(func(int64, bool) bool { return true })
	l.Begin(0)
	got := fpush(l, []uint64{1}, 2, 3)
	if len(got) != 3 || got[2] != 3 || l.Total() != 64 {
		t.Fatalf("growth: %v charged=%d", got, l.Total())
	}
	defer func() {
		if recover() == nil || !l.overflow || l.Total() != math.MaxInt64 {
			t.Fatal("unrepresentable cleanup did not poison account")
		}
	}()
	fmake[uint64](l, math.MaxInt)
}

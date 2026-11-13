package ccore

import (
	"math"
	"strconv"
	"testing"
)

func TestLedgerInvalidBeginPreservesOverflow(t *testing.T) {
	var l Ledger
	l.SetPoll(func(int64, bool) bool { return true })
	l.Begin(0)
	l.Force(-1)
	before := l
	if l.Begin(-1) || l.Base != before.Base || l.Alloc != before.Alloc || l.overflow != before.overflow || l.Check(false) {
		t.Fatal("invalid Begin cleared poisoned account")
	}
}
func TestLedgerForceInvalidWithoutPoll(t *testing.T) {
	var l Ledger
	l.Begin(0)
	l.Force(-1)
	if !l.overflow || l.Check(false) {
		t.Fatal("invalid forced charge was ignored")
	}
}
func TestLedgerMakeByteLimit(t *testing.T) {
	var l Ledger
	n := math.MaxInt/8 + 1
	if strconv.IntSize == 32 {
		n = 1 << 29
	}
	defer func() {
		if v := recover(); v != nil {
			t.Errorf("Make panicked instead of refusing: %v", v)
		}
	}()
	if _, ok := Make[uint64](&l, n); ok {
		t.Fatal("oversized make accepted")
	}
}
func TestLedgerGrowthSaturatesDoubling(t *testing.T) {
	limit := byteLimit() / 16
	if limit > int64(math.MaxInt) {
		limit = int64(math.MaxInt)
	}
	capN := int(limit/2 + 1)
	got, ok := growthSize(capN, capN, 1, 16)
	if !ok || got < capN+1 || int64(got) > limit {
		t.Fatalf("growth n=%d ok=%t limit=%d", got, ok, limit)
	}
}
func TestLedgerNoPollAccountingContract(t *testing.T) {
	var l Ledger
	if !l.Begin(10) {
		t.Fatal("begin")
	}
	if _, ok := Make[byte](&l, 64); !ok || l.Total() != 10 {
		t.Fatal("unlimited helper accounting")
	}
	if !l.Reserve(64) || l.Total() != 74 {
		t.Fatal("direct reserve accounting")
	}
}
func TestLedgerForcedInvalidMakeNoPoll(t *testing.T) {
	var l Ledger
	l.Begin(0)
	defer func() {
		if recover() == nil || !l.overflow || l.Check(false) {
			t.Fatal("forced invalid make failed to poison")
		}
	}()
	fmake[byte](&l, -1)
}

func TestLedgerByteLimitPermitsAddressableCounts(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("32-bit addressability control")
	}
	n := math.MaxInt/8 + 1
	got, ok := sliceBytes[uint64](n)
	if !ok || got <= int64(math.MaxInt) {
		t.Fatalf("addressable byte count %d ok=%t", got, ok)
	}
}

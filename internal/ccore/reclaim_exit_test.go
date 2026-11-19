package ccore_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

func rangeEdit(lo, hi int) ccore.Edit {
	return ccore.Edit{StartByte: uint32(lo), OldEndByte: uint32(hi), NewEndByte: uint32(hi),
		StartCol: uint32(lo), OldEndCol: uint32(hi), NewEndCol: uint32(hi)}
}

// Every synchronous drain at the exit of a request runs on the request's ledger. A release that
// comes in during the exit drain (while it works on its snapshot) is drained by the late drain of
// endParse (afterParse); that one used to build a view with no ledger, so its growth of the
// reclamation arrays was not counted and came after the last check. The release of a second,
// larger obsolete tree during the exit drain must raise the account.
func TestExitDrainAndTheLateDrainAreOnTheRequestLedger(t *testing.T) {
	g := loadGoldens(t)[0]
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 60000; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(strconv.Itoa(1000 + i%7))
	}
	sb.WriteString("]")
	src0 := []byte(sb.String())
	swap := func(src []byte, lo, hi int) []byte {
		out := append([]byte(nil), src...)
		for i := lo; i < hi; i++ {
			if out[i] == '1' {
				out[i] = '7'
			} else if out[i] == '7' {
				out[i] = '1'
			}
		}
		return out
	}
	n := len(src0)
	src1 := swap(src0, n/2, n/2+300)       // a small replacement
	src2 := swap(src1, n/5, 4*n/5)         // a large one
	src3 := swap(src2, n/2+1000, n/2+1100) // the edit of the request under test
	run := func(releaseSecond bool) int64 {
		p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
		if err != nil {
			t.Fatal(err)
		}
		t0, stop := p.Parse(src0)
		if stop != "" {
			t.Fatalf("stop %q", stop)
		}
		t0.Edit(rangeEdit(n/2, n/2+300))
		t1, stop := p.ParseWith(src1, t0)
		if stop != "" {
			t.Fatalf("stop %q", stop)
		}
		t1.Edit(rangeEdit(n/5, 4*n/5))
		t2, stop := p.ParseWith(src2, t1)
		if stop != "" {
			t.Fatalf("stop %q", stop)
		}
		t2.Edit(rangeEdit(n/2+1000, n/2+1100))
		p.Ledger().SetPoll(func(total int64, sample bool) bool { return true })
		phase := 0
		ccore.SetTestHook(t2, func(pt string) {
			switch {
			case pt == "parse-start" && phase == 0:
				phase = 1
				t0.Release() // queued behind the parse: freed by the drain at its exit
			case pt == "drain" && phase == 1:
				phase = 2
				if releaseSecond {
					t1.Release() // queued while the exit drain works on its snapshot: freed by the late drain
				}
			}
		})
		r, stop := p.ParseWith(src3, t2)
		ccore.SetTestHook(t2, nil)
		if stop != "" || r == nil {
			t.Fatalf("stop %q", stop)
		}
		if phase != 2 {
			t.Fatalf("the hooks did not run in order (phase %d)", phase)
		}
		alloc := p.Ledger().Alloc
		r.Release()
		t2.Release()
		if !releaseSecond {
			t1.Release()
		}
		return alloc
	}
	one := run(false)
	two := run(true)
	t.Logf("account after the request: %d bytes with one release in the exit drain, %d with a second, larger one queued during it", one, two)
	if two-one < 100<<10 {
		t.Fatalf("the late drain of the second release added %d bytes to the account (%d to %d); it ran off the request's ledger", two-one, one, two)
	}
}

package ccore_test

import (
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

type pollCall struct {
	n        int64 // bytes of the reservation (total minus what was counted before it)
	total    int64
	sample   bool
	finished bool
}

func tracePoll(p *ccore.Parser, refuse func(c pollCall, i int) bool) *[]pollCall {
	var calls []pollCall
	l := p.Ledger()
	l.SetPoll(func(total int64, sample bool) bool {
		c := pollCall{n: total - l.Base - l.Alloc, total: total, sample: sample, finished: p.Finished() != 0}
		calls = append(calls, c)
		return refuse == nil || !refuse(c, len(calls)-1)
	})
	return &calls
}

// Test 4: the balance runs in a real parse, its stacks go through the ledger, and a
// refusal during it ends the parse and counts nothing more.
func TestBalanceReservesItsStacksInARealParse(t *testing.T) {
	g := loadGoldens(t)[0]
	src := []byte("[" + strings.Repeat("1,", 50000) + "1]")
	p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	calls := tracePoll(p, nil)
	if _, stop := p.Parse(src); stop != "" {
		t.Fatalf("stop %q", stop)
	}
	if p.Stats().Compressions == 0 {
		t.Fatal("the balance made no compression: the input does not exercise it")
	}
	first := -1
	for i, c := range *calls {
		if c.finished {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("no limit check ran while the balance ran (Finished() != 0)")
	}
	// Refuse the first check of the balance: the parse stops and nothing is reserved after it.
	q, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	var allocAtRefusal int64 = -1
	n := 0
	q.Ledger().SetPoll(func(total int64, sample bool) bool {
		if q.Finished() != 0 {
			n++
			allocAtRefusal = q.Ledger().Alloc
			return false
		}
		return true
	})
	tree, stop := q.Parse(src)
	if stop != "stopped" || tree != nil || n != 1 || q.Ledger().Alloc != allocAtRefusal {
		t.Fatalf("refusal during balance: stop %q tree %v checks after refusal %d, Alloc %d -> %d", stop, tree, n, allocAtRefusal, q.Ledger().Alloc)
	}
}

// Test 5: the acceptance splice (a buffer of every child of the root) is one
// reservation, checked before the buffer is made.
func TestAcceptSpliceIsReservedBeforeItIsMade(t *testing.T) {
	g := loadGoldens(t)[0]
	src := []byte("1 " + strings.Repeat("/**/ ", 100000))
	isSplice := func(c pollCall) bool { return !c.finished && c.n >= 800000 && c.n < 800100 }
	p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	calls := tracePoll(p, nil)
	if _, stop := p.Parse(src); stop != "" {
		t.Fatalf("stop %q", stop)
	}
	at, count := -1, 0
	for i, c := range *calls {
		if isSplice(c) {
			if at < 0 {
				at = i
			}
			count++
		}
	}
	// Two reservations of the size of the root's child list: the splice buffer, then the
	// store's copy of it (newNode). Without the fix the buffer is made by append and
	// only the copy is reserved.
	if count != 2 {
		t.Fatalf("%d reservations of about 800,000 bytes (8 bytes for each of the 100,002 children) before the tree was finished, want 2 (the splice buffer and the store copy)", count)
	}
	// A budget equal to the total before the splice: the parse stops there and allocates almost nothing more.
	q, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	var msAt, msEnd runtime.MemStats
	refusedAt := int64(-1)
	budget := (*calls)[at].total - (*calls)[at].n
	q.Ledger().SetPoll(func(total int64, sample bool) bool {
		if total > budget && refusedAt < 0 {
			refusedAt = total
			runtime.ReadMemStats(&msAt)
			return false
		}
		return refusedAt < 0
	})
	_, stop := q.Parse(src)
	runtime.ReadMemStats(&msEnd)
	if stop != "stopped" {
		t.Fatalf("stop %q, want \"stopped\"", stop)
	}
	if d := msEnd.TotalAlloc - msAt.TotalAlloc; d > 64<<10 {
		t.Fatalf("%d bytes allocated after the refusal, want under 64 KiB", d)
	}
}

// A refusal unwinds by a private panic that parseWith turns into the stop text;
// the same parser then serves a later parse, equal to a fresh parser's.
func TestParserServesAgainAfterALedgerStop(t *testing.T) {
	g := loadGoldens(t)[0]
	big := []byte("[" + strings.Repeat("1,", 20000) + "1]")
	small := []byte(`{"a": [1, 2, {"b": null}], "c": "x"}`)
	fresh, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	ft, stop := fresh.Parse(small)
	if stop != "" {
		t.Fatalf("fresh parse: %q", stop)
	}
	want, wantArena := fresh.Digest(ft), fresh.ArenaDigest(ft)
	ft.Release()

	p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	for _, refuseAt := range []int{2, 5, 40, 200, 1000} { // stops inside different operations
		n := 0
		p.Ledger().SetPoll(func(total int64, sample bool) bool { n++; return n < refuseAt })
		tree, stop := p.Parse(big)
		if stop != "stopped" || tree != nil {
			t.Fatalf("refusal at call %d: stop %q tree %v", refuseAt, stop, tree)
		}
		p.Ledger().SetPoll(nil)
		p.Reset()
		tree, stop = p.Parse(small)
		if stop != "" || p.Digest(tree) != want || p.ArenaDigest(tree) != wantArena {
			t.Fatalf("after a stop at call %d: stop %q, digest equal %v, arena equal %v", refuseAt, stop, p.Digest(tree) == want, p.ArenaDigest(tree) == wantArena)
		}
		tree.Release()
	}
	// Another panic value passes on unchanged.
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("recovered %v, want the original panic value", r)
		}
	}()
	p.Ledger().SetPoll(func(int64, bool) bool { panic("boom") })
	p.Parse(small)
}

// Reset zeroes the slice, iterator and head arrays up to their capacity, so that each
// subtree buffer has one owner (bufs, trailing, ...) and the capacity walk of
// bufferBytes counts it once. A parse leaves stale headers; Reset removes them; the next
// parse equals a fresh parser's.
func TestResetZeroesStaleHeaders(t *testing.T) {
	g := loadGoldens(t)[0]
	src := []byte(`{"a": [1, 2, {"b": null}], "c": "x", "d": [` + strings.Repeat(`{"k": true}, `, 50) + `{}]}`)
	p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	first, _ := p.Parse(src)
	first.Release()
	if p.StaleHeaders() == 0 {
		t.Fatal("the parse left no stale header: the case proves nothing")
	}
	before := p.BufferBytes()
	p.Reset()
	if n := p.StaleHeaders(); n != 0 {
		t.Fatalf("%d stale headers after Reset", n)
	}
	if after := p.BufferBytes(); after >= before {
		t.Fatalf("BufferBytes %d after Reset, %d before: Reset must stop counting buffers twice", after, before)
	}
	fresh, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	ft, _ := fresh.Parse(src)
	pt, _ := p.Parse(src)
	if p.Digest(pt) != fresh.Digest(ft) || p.ArenaDigest(pt) != fresh.ArenaDigest(ft) {
		t.Fatal("a parse after Reset differs from a fresh parser's")
	}
	ft.Release()
	pt.Release()
}

// A fresh parse in a new store has no page in the base of its account, so the first record page
// and the first child page (32,768 bytes plus its table entry) are both reserved on the request's
// ledger. The first child page used to be taken with the ledger detached.
func TestFreshRequestChargesTheFirstChildPage(t *testing.T) {
	g := loadGoldens(t)[0]
	p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	p.Ledger().SetPoll(func(total int64, sample bool) bool { return true })
	tree, stop := p.Parse([]byte(""))
	if stop != "" || tree == nil {
		t.Fatalf("stop %q", stop)
	}
	defer tree.Release()
	recPage, kidPage := ccore.FirstPageCharges()
	rest := p.Ledger().Alloc - recPage - kidPage
	t.Logf("Alloc %d = record page %d + child page %d + %d", p.Ledger().Alloc, recPage, kidPage, rest)
	if kidPage != 32768+8 {
		t.Fatalf("the child page charge is %d, want 32768 plus its table entry (8)", kidPage)
	}
	if rest < 0 || rest > 16<<10 {
		t.Fatalf("Alloc %d: after the two first pages (%d and %d) %d bytes remain; want 0 to 16 KiB (a child page taken without the ledger leaves a negative remainder)", p.Ledger().Alloc, recPage, kidPage, rest)
	}
}

// The page tables of a new store are sized from the parser's last fresh parse. A request
// with a limit does not use that hint: it sizes them from its own source, and its account
// holds the bytes of the tables it made. A small request after a large unlimited parse
// is charged the same as on a parser that never parsed the large one.
func TestPageTableHintDoesNotReachARequestWithALimit(t *testing.T) {
	g := loadGoldens(t)[0]
	big := []byte("[" + strings.Repeat("1,", 60000) + "1]")
	small := []byte("[1,2,3]")
	account := func(warm bool) int64 {
		p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
		if warm {
			tr, stop := p.Parse(big) // no limit: records the page counts of a large parse
			if stop != "" {
				t.Fatalf("stop %q", stop)
			}
			defer tr.Release() // kept until the end, so that no reclamation is charged to the small request
		}
		p.Ledger().SetPoll(func(total int64, sample bool) bool { return true })
		tr, stop := p.Parse(small)
		if stop != "" {
			t.Fatalf("stop %q", stop)
		}
		defer tr.Release()
		return p.Ledger().Alloc
	}
	cold, warm := account(false), account(true)
	if warm > cold+int64(ccore.LedgerGranularity) {
		t.Fatalf("a small request after a large parse was charged %d bytes, %d on a fresh parser", warm, cold)
	}
	// A refusal at the tables stops the request before it makes any.
	p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
	p.Ledger().SetPoll(func(total int64, sample bool) bool { return false })
	if tr, stop := p.Parse(big); tr != nil || stop == "" {
		t.Fatalf("a refusing limit: tree %v stop %q", tr, stop)
	}
}

// A parser that is used again without Reset still has the headers of its last
// parse in its arrays. A request with a limit must not count the buffers behind
// them twice: its account must start from the same base as that of a parser
// that was Reset in between.
func TestLimitedRequestOnAReusedParserStartsFromTheResetBase(t *testing.T) {
	g := loadGoldens(t)[0]
	src := []byte(`{"a": [1, 2, {"b": null}], "c": "x", "d": [` + strings.Repeat(`{"k": true}, `, 50) + `{}]}`)
	base := func(reset bool) int64 {
		p, _ := ccore.NewParser(ccore.Config{Tables: g.Tables})
		first, stop := p.Parse(src)
		if first == nil || stop != "" {
			t.Fatalf("first parse: %q", stop)
		}
		first.Release()
		if p.StaleHeaders() == 0 {
			t.Fatal("the parse left no stale header: the case proves nothing")
		}
		if reset {
			p.Reset()
		}
		p.Ledger().SetPoll(func(int64, bool) bool { return true })
		second, stop := p.Parse(src)
		if second == nil || stop != "" {
			t.Fatalf("second parse: %q", stop)
		}
		second.Release()
		return p.Ledger().Base
	}
	reused, reset := base(false), base(true)
	if reused != reset {
		t.Fatalf("a reused parser starts its account at %d bytes, a reset one at %d: stale headers were counted", reused, reset)
	}
}

func TestIncludedRangeEditRefusalPreservesSnapshot(t *testing.T) {
	g := loadGoldens(t)[0]
	p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
	if err != nil {
		t.Fatal(err)
	}
	ranges := make([]ccore.Range, ccore.LedgerGranularity/int(ccore.ElemBytes[ccore.Range]())+2)
	ranges[0] = ccore.Range{EndByte: 1, EndCol: 1}
	for i := 1; i < len(ranges); i++ {
		ranges[i] = ccore.Range{StartByte: 1, StartCol: 1, EndByte: 1, EndCol: 1}
	}
	tree, stop := p.ParseIncluded([]byte("1"), ranges)
	if tree == nil || stop != "" {
		t.Fatalf("ranged parse stopped: %q", stop)
	}
	defer tree.Release()
	want := p.Digest(tree)
	base := int64(tree.PageBytes())
	var led ccore.Ledger
	led.SetPoll(func(total int64, _ bool) bool { return total <= base+int64(len(ranges))*ccore.ElemBytes[ccore.Range]() })
	if !led.Begin(base) {
		t.Fatal("ledger refused before the edit")
	}
	if tree.EditWithin(ccore.Edit{NewEndByte: 1, NewEndCol: 1}, &led) {
		t.Fatal("edit exceeded the range snapshot allowance")
	}
	if tree.StoreFailure() != "" || p.Digest(tree) != want {
		t.Fatal("refused range snapshot altered or poisoned the tree")
	}
	got := tree.IncludedRanges()
	if !slices.Equal(got, ranges) {
		t.Fatal("refused range snapshot changed the included ranges")
	}
}

package ccore_test

// Tests of the store that need a parser: released trees, panics, finalizers,
// pool use, and the stop and panic checks on the JSON goldens. None needs cgo.

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest"
)

func goldenNamed(t *testing.T, name string) *ccoretest.Golden {
	t.Helper()
	for _, g := range loadGoldens(t) {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("no golden %q", name)
	return nil
}

// selfEdit replaces the byte at i with itself (the text stays the same, so a
// reparse needs no new lexer answer).
func selfEdit(src []byte, i int) ccore.Edit {
	row, col := uint32(0), uint32(0)
	for _, b := range src[:i] {
		if b == '\n' {
			row, col = row+1, 0
		} else {
			col++
		}
	}
	e := ccore.Edit{StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i + 1), StartRow: row, StartCol: col}
	if src[i] == '\n' {
		e.OldEndRow, e.OldEndCol, e.NewEndRow, e.NewEndCol = row+1, 0, row+1, 0
	} else {
		e.OldEndRow, e.OldEndCol, e.NewEndRow, e.NewEndCol = row, col+1, row, col+1
	}
	return e
}

// parseCatch runs ParseWith and returns the value of a panic instead of passing it on.
func parseCatch(p *ccore.Parser, src []byte, old *ccore.Tree) (nt *ccore.Tree, stop string, pan any) {
	defer func() { pan = recover() }()
	nt, stop = p.ParseWith(src, old)
	return
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ParseWith on a tree that is released while the call waits for the
// lease. The call must reject it, the cell count must come back, and a second
// tree of the store must stay intact.
func TestParseWithRejectsTreeReleasedWhileWaiting(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	p2 := mustParser(t, ccore.Config{Tables: g.Tables})
	beforeDouble, beforeBad := p.DoubleFree(), p.BadRetain()
	for round := 0; round < 100; round++ {
		o, stop := p.Parse(g.Src)
		if stop != "" {
			t.Fatal(stop)
		}
		e := o.Copy()
		e.Edit(selfEdit(g.Src, 6))
		x, stop := p.ParseWith(g.Src, e) // the second tree: shares records with o
		if stop != "" {
			t.Fatal(stop)
		}
		e.Release()
		want := p.Digest(x)
		ccore.LockLease(x) // a parse of this document is running
		done := make(chan string, 1)
		go func() {
			nt, stop := p2.ParseWith(g.Src, o)
			if nt != nil {
				stop = "returned a tree"
			}
			done <- stop
		}()
		time.Sleep(time.Duration(round%3) * time.Millisecond) // let the call reach the lease sometimes
		o.Release()
		ccore.UnlockLease(x)
		if got := <-done; got != "old tree released" {
			t.Fatalf("round %d: ParseWith of a released tree gave %q", round, got)
		}
		if got := p.Digest(x); got != want {
			t.Fatalf("round %d: the second tree changed", round)
		}
		if ccore.Cells(x) != 1 {
			t.Fatalf("round %d: cells %d, want 1", round, ccore.Cells(x))
		}
		x.Release()
		if !ccore.Ended(x) {
			t.Fatalf("round %d: the store did not end", round)
		}
	}
	if p.DoubleFree() != beforeDouble || p.BadRetain() != beforeBad {
		t.Fatalf("double free %d to %d, bad retain %d to %d", beforeDouble, p.DoubleFree(), beforeBad, p.BadRetain())
	}
}

// A panic inside a drain, inside the exit sweep and inside the publication
// poisons the store and leaves the lease free and the cell count right; the
// old tree still reads correctly, a later parse of it gives the fresh digest (in
// a new store), and the store ends when its last tree is released.
func TestPanicInDrainSweepAndPublishLeavesLeaseFree(t *testing.T) {
	g := goldenNamed(t, "json-object")
	fresh := func() (*ccore.Parser, *ccore.Tree, *ccore.Tree, string) {
		p := mustParser(t, ccore.Config{Tables: g.Tables})
		base, stop := p.Parse(g.Src)
		if stop != "" {
			t.Fatal(stop)
		}
		ed := base.Copy()
		ed.Edit(selfEdit(g.Src, 6))
		return p, base, ed, p.Digest(base)
	}
	only := func(point string) func(string) {
		return func(pt string) {
			if pt == point {
				panic("injected at " + point)
			}
		}
	}

	// A panic in the drain at the start of a parse.
	p, base, ed, want := fresh()
	ccore.SetTestHook(base, only("drain"))
	pnt, pstop, pan := parseCatch(p, g.Src, ed)
	ccore.SetTestHook(base, nil)
	if pan != nil || pnt != nil || !strings.Contains(pstop, "store failure") {
		t.Fatalf("the drain failure must become a stop with a named reason: tree %v, stop %q, panic %v", pnt != nil, pstop, pan)
	}
	if !p.LeaseFree() {
		t.Fatal("the lease is held after a panic in the drain")
	}
	if ccore.Cells(base) != 2 {
		t.Fatalf("cells %d after a panic in the drain, want 2 (the parse's count must come back)", ccore.Cells(base))
	}
	nt, stop, pan := parseCatch(p, g.Src, ed)
	if pan != nil || stop != "" || p.Digest(nt) != want {
		t.Fatalf("a later parse of the document failed: panic %v stop %q", pan, stop)
	}
	if !ccore.Poisoned(base) {
		t.Fatal("the store is not poisoned after the drain panic")
	}
	if p.Digest(base) != want {
		t.Fatal("the old tree reads differently after the drain panic")
	}
	// This test does not release the trees; it ends the store by hand.
	ccore.EndStore(base)

	// A panic in the sweep at the exit of a parse that stops.
	p, base, ed, want = fresh()
	p.SetStopAfter(5, false)
	ccore.SetTestHook(base, only("sweep"))
	_, _, pan = parseCatch(p, g.Src, ed)
	ccore.SetTestHook(base, nil)
	p.SetStopAfter(0, false)
	if pan != nil || !p.LeaseFree() || ccore.Cells(base) != 2 || !ccore.Poisoned(base) {
		t.Fatalf("sweep panic: panic %v, lease free %t, cells %d, poisoned %t", pan, p.LeaseFree(), ccore.Cells(base), ccore.Poisoned(base))
	}
	nt, stop, pan = parseCatch(p, g.Src, ed)
	if pan != nil || stop != "" || p.Digest(nt) != want {
		t.Fatalf("a later parse after the sweep panic failed: panic %v stop %q", pan, stop)
	}
	nt.Release()
	ed.Release()
	base.Release()
	if !ccore.Ended(base) {
		t.Fatal("the store did not end after the sweep panic and the releases")
	}

	// A panic in the publication walk.
	p, base, ed, want = fresh()
	ccore.SetTestHook(base, only("publish"))
	pnt, pstop, pan = parseCatch(p, g.Src, ed)
	ccore.SetTestHook(base, nil)
	if pan != nil || pnt != nil || !strings.Contains(pstop, "store failure") || !p.LeaseFree() || ccore.Cells(base) != 2 || !ccore.Poisoned(base) {
		t.Fatalf("publish panic: panic %v, lease free %t, cells %d, poisoned %t", pan, p.LeaseFree(), ccore.Cells(base), ccore.Poisoned(base))
	}
	nt, stop, pan = parseCatch(p, g.Src, ed)
	if pan != nil || stop != "" || p.Digest(nt) != want {
		t.Fatalf("a later parse after the publish panic failed: panic %v stop %q", pan, stop)
	}
	nt.Release()
	ed.Release()
	base.Release()
	if !ccore.Ended(base) {
		t.Fatal("the store did not end after the publish panic and the releases")
	}
}

// A finalizer appends its entry and returns; it never waits for a lease, so
// a finalizer of one document cannot delay the finalizer of another. The
// finalizer of the last cell ends the store inline, taking the lease only if
// it is free. This test would also pass with a finalizer that walks inline;
// TestFinalizerDoesNotWalkOnItsGoroutine is the one that fails then.
func TestFinalizerNeverWaitsForALease(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	beforeDouble, beforeBad := p.DoubleFree(), p.BadRetain()
	ta, _ := p.Parse(g.Src)
	tb, _ := p.Parse(g.Src)
	ca, cb := ta.Copy(), tb.Copy() // two documents, each with a second cell
	ccore.LockLease(ta)            // a parse of document A is running
	done := make(chan struct{})
	go func() {
		ccore.Finalize(ca) // as the finalizer goroutine would, one after the other
		ccore.Finalize(cb)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a finalizer waited for a lease held by another document's parse")
	}
	if got := ccore.Pending(ta); got != 2 { // the retain of Copy and the release of the finalizer
		t.Fatalf("document A has %d entries queued, want 2 (the finalizer must only append)", got)
	}
	waitFor(t, "the drain of document B", func() bool { return ccore.Pending(tb) == 0 })
	ccore.UnlockLease(ta)
	ta.Release()
	tb.Release()
	// The goroutine that Finalize started may hold a lease at this moment; it ends
	// the store when it lets go.
	waitFor(t, "both stores to end", func() bool { return ccore.Ended(ta) && ccore.Ended(tb) })
	// The last cell ends its store inline, without a walk.
	tc, _ := p.Parse(g.Src)
	ccore.Finalize(tc)
	if !ccore.Ended(tc) {
		t.Fatal("the finalizer of the last cell did not end the store inline")
	}
	if p.DoubleFree() != beforeDouble || p.BadRetain() != beforeBad {
		t.Fatal("double free or bad retain")
	}
}

// Under steady Copy, Edit and Release traffic a parse still starts.
// This test does not detect a drain that loops over new entries (it would only
// run slower); TestDrainAppliesOneSnapshot does.
func TestParseStartsUnderCopyEditReleaseTraffic(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	base, _ := p.Parse(g.Src)
	want := p.Digest(base)
	e := selfEdit(g.Src, 6)
	var stop atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				c := base.Copy()
				c.Edit(e)
				c.Release()
			}
		}()
	}
	var worst time.Duration
	for i := 0; i < 30; i++ {
		ed := base.Copy()
		ed.Edit(e)
		start := time.Now()
		nt, s := p.ParseWith(g.Src, ed)
		if el := time.Since(start); el > worst {
			worst = el
		}
		if s != "" || p.Digest(nt) != want {
			t.Fatalf("parse %d under traffic: stop %q", i, s)
		}
		nt.Release()
		ed.Release()
	}
	stop.Store(true)
	wg.Wait()
	base.Release()
	if worst > 5*time.Second {
		t.Fatalf("a parse waited %v for the lease under traffic", worst)
	}
	waitFor(t, "the store to end", func() bool { return ccore.Ended(base) })
}

// The parser keeps no reference to its last tree. A tree dropped
// without Release is finalized while its parser stays idle.
func TestIdleParserDoesNotPinItsTree(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	var ended func() bool
	func() {
		tr, stop := p.Parse(g.Src)
		if stop != "" {
			t.Fatal(stop)
		}
		_ = p.Digest(tr)
		ended = ccore.EndedFunc(tr)
	}()
	for i := 0; i < 100 && !ended(); i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if !ended() {
		t.Fatal("a dropped tree was not finalized while its parser was idle")
	}
	runtime.KeepAlive(p)
}

func numbersArray(n int) []byte {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%d", i%1000)
	}
	sb.WriteString("]\n")
	return []byte(sb.String())
}

// A small document parsed after a large one holds a small number of
// pages, and the bytes the store reports are the bytes of the pages it holds.
func TestSmallDocumentAfterLargeHoldsLittle(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	big, stop := p.Parse(numbersArray(60000))
	if stop != "" {
		t.Fatal(stop)
	}
	_, _, _, _, _, _, _, bigBytes := ccore.PagesHeld(big)
	big.Release()
	small, stop := p.Parse([]byte(`{"a": [1, 2]}`))
	if stop != "" {
		t.Fatal(stop)
	}
	recs, refs, kids, state, cRec, cKid, cState, held := ccore.PagesHeld(small)
	if recs != cRec || kids != cKid || state != cState || refs != recs {
		t.Fatalf("pages held %d/%d/%d/%d, claimed %d/%d/%d", recs, refs, kids, state, cRec, cKid, cState)
	}
	if p.StoreBytes() != held {
		t.Fatalf("StoreBytes %d, pages held %d", p.StoreBytes(), held)
	}
	used, reach := p.ArenaUse(small)
	if used != held {
		t.Fatalf("ArenaUse reports %d bytes, pages held %d", used, held)
	}
	if recs > 2 || kids > 2 || state != 0 || held*4 > bigBytes {
		t.Fatalf("large document held %d bytes; small document holds %d bytes in %d record, %d child and %d state pages for %d reachable bytes", bigBytes, held, recs, kids, state, reach)
	}
	small.Release()
}

// Stop and panic at step counts of a fresh and of an incremental parse (every
// count for the goldens, every 40th for the 1,500-number array). After each one the store holds the same bytes, the
// lease is free, the next parse gives the digest of a fresh parse, and two runs
// give the same root handles.
func TestStopAndPanicAtEveryStepCount(t *testing.T) {
	type source struct {
		name   string
		src    []byte
		g      *ccoretest.Golden
		stride int
	}
	var sources []source
	for _, g := range loadGoldens(t) {
		sources = append(sources, source{g.Name, g.Src, g, 1})
	}
	jg := goldenNamed(t, "json-object")
	sources = append(sources, source{"numbers-array", numbersArray(1500), jg, 40})
	for _, s := range sources {
		if len(s.src) < 4 {
			continue
		}
		run := func() []ccore.Handle {
			var handles []ccore.Handle
			p := mustParser(t, ccore.Config{Tables: s.g.Tables})
			base, stop := p.Parse(s.src)
			if stop != "" {
				t.Fatalf("%s: fresh parse stop %q", s.name, stop)
			}
			fresh := p.Digest(base)
			ed := base.Copy()
			ed.Edit(selfEdit(s.src, len(s.src)/2))
			step := func(what string, n int, panicking bool, bytes0 int) (completed bool) {
				p.SetStopAfter(n, panicking)
				nt, stop, pan := parseCatch(p, s.src, ed)
				p.SetStopAfter(0, false)
				if nt != nil {
					handles = append(handles, nt.Root())
					nt.Release()
					return true
				}
				if panicking && pan == nil || !panicking && (pan != nil || stop == "") {
					t.Fatalf("%s %s n=%d: panic %v stop %q", s.name, what, n, pan, stop)
				}
				if !p.LeaseFree() {
					t.Fatalf("%s %s n=%d: the lease is held", s.name, what, n)
				}
				if got := p.StoreBytes(); got != bytes0 {
					t.Fatalf("%s %s n=%d: store bytes %d, were %d", s.name, what, n, got, bytes0)
				}
				nt, stop = p.ParseWith(s.src, ed)
				if stop != "" || p.Digest(nt) != fresh {
					t.Fatalf("%s %s n=%d: the next parse differs from a fresh one (stop %q)", s.name, what, n, stop)
				}
				handles = append(handles, nt.Root())
				nt.Release()
				return false
			}
			for i := 0; i < 2; i++ { // warm up: the free lists reach their steady state
				nt, stop := p.ParseWith(s.src, ed)
				if stop != "" {
					t.Fatalf("%s: incremental parse stop %q", s.name, stop)
				}
				handles = append(handles, nt.Root())
				nt.Release()
			}
			bytes0 := p.StoreBytes()
			for _, panicking := range []bool{false, true} {
				for n := 1; ; n += s.stride {
					if step("incremental", n, panicking, bytes0) {
						break
					}
					if n > 200000 {
						t.Fatalf("%s: no end to the steps", s.name)
					}
				}
			}
			ed.Release()
			base.Release()
			// A fresh parse stopped at every step count: no tree, lease free, next parse equal.
			for _, panicking := range []bool{false, true} {
				for n := 1; ; n += s.stride {
					p.SetStopAfter(n, panicking)
					nt, stop, pan := parseCatch(p, s.src, nil)
					p.SetStopAfter(0, false)
					if nt != nil {
						nt.Release()
						break
					}
					if panicking && pan == nil || !panicking && (pan != nil || stop == "") {
						t.Fatalf("%s fresh n=%d: panic %v stop %q", s.name, n, pan, stop)
					}
					if !p.LeaseFree() {
						t.Fatalf("%s fresh n=%d: the lease is held", s.name, n)
					}
					nt, stop = p.Parse(s.src)
					if stop != "" || p.Digest(nt) != fresh {
						t.Fatalf("%s fresh n=%d: the next parse differs (stop %q)", s.name, n, stop)
					}
					nt.Release()
					if n > 200000 {
						t.Fatalf("%s: no end to the steps", s.name)
					}
				}
			}
			return handles
		}
		a, b := run(), run()
		if len(a) != len(b) {
			t.Fatalf("%s: %d handles in the first run, %d in the second", s.name, len(a), len(b))
		}
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("%s: handle %d differs between runs: %d and %d", s.name, i, a[i], b[i])
			}
		}
	}
}

// mustParser is ccore.NewParser for tests that cannot go on without a parser
// (NewParser checks the tables and hooks and returns an error).
func mustParser(t testing.TB, c ccore.Config) *ccore.Parser {
	t.Helper()
	p, err := ccore.NewParser(c)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The finalizer must not run the release walk on its own goroutine. With the lease free and a hook that blocks in the drain,
// Finalize has to return while the hook is still blocked; the walk runs on the
// goroutine that the finalizer starts.
func TestFinalizerDoesNotWalkOnItsGoroutine(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	ta, _ := p.Parse(g.Src)
	cp := ta.Copy() // a second cell, so the store does not end inline
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	ccore.SetTestHook(ta, func(pt string) {
		if pt == "drain" {
			once.Do(func() { close(entered); <-unblock })
		}
	})
	done := make(chan struct{})
	go func() { ccore.Finalize(cp); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		close(unblock)
		t.Fatal("Finalize did not return while the drain was blocked: the walk ran on the finalizer's goroutine")
	}
	select {
	case <-entered: // the goroutine that Finalize started is in the drain
	case <-time.After(3 * time.Second):
		t.Fatal("no goroutine tried the drain")
	}
	close(unblock)
	waitFor(t, "the drain to finish", func() bool { return ccore.Pending(ta) == 0 })
	ccore.SetTestHook(ta, nil)
	ta.Release() // the goroutine that Finalize started may still hold the lease; it ends the store when it lets go
	waitFor(t, "the store to end", func() bool { return ccore.Ended(ta) })
}

// A panic in the publication walk after its first mark poisons the store: the
// records the walk marked are not freed piecemeal (so nothing is freed twice and
// the old tree still reads), and the store keeps the pages it holds while a tree
// of it lives.
func TestPanicInPublishPoisonsAndFreesNothing(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	base, _ := p.Parse(g.Src)
	want := p.Digest(base)
	ed := base.Copy()
	ed.Edit(selfEdit(g.Src, 6))
	nt, stop := p.ParseWith(g.Src, ed) // a complete parse first: the edit's entries are applied
	if stop != "" {
		t.Fatal(stop)
	}
	nt.Release()
	double, bad := p.DoubleFree(), p.BadRetain()
	_, _, _, _, _, _, _, bytes := ccore.PagesHeld(base)
	ccore.SetTestHook(base, func(pt string) {
		if pt == "publish" {
			panic("injected after the first mark")
		}
	})
	_, pstop, pan := parseCatch(p, g.Src, ed)
	ccore.SetTestHook(base, nil)
	if pan != nil || !strings.Contains(pstop, "store failure") {
		t.Fatalf("the publish failure must become a stop with a named reason: stop %q, panic %v", pstop, pan)
	}
	if !ccore.Poisoned(base) || !p.LeaseFree() {
		t.Fatalf("poisoned %t, lease free %t", ccore.Poisoned(base), p.LeaseFree())
	}
	if p.DoubleFree() != double || p.BadRetain() != bad {
		t.Fatal("a record was freed twice or a freed record retained")
	}
	if _, _, _, _, _, _, _, b2 := ccore.PagesHeld(base); b2 != bytes {
		t.Fatalf("the poisoned store changed its pages: %d bytes before, %d after", bytes, b2)
	}
	if p.Digest(base) != want {
		t.Fatal("the old tree reads differently after the panic")
	}
	ed.Release()
	base.Release()
}

// Info and ArenaDigest hold the lease; a release that
// fails TryLock against them is drained when they let go.
func TestStatisticsDrainWhatTheyBlocked(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	x, _ := p.Parse(g.Src)
	for name, call := range map[string]func(){
		"ArenaDigest": func() { _ = p.ArenaDigest(x) },
		"StoreInfo":   func() { _ = p.StoreInfo() },
	} {
		cp := x.Copy()
		ccore.SetTestHook(x, func(pt string) {
			if pt == "hold" {
				cp.Release() // fails TryLock: the statistic holds the lease
			}
		})
		call()
		ccore.SetTestHook(x, nil)
		if ccore.Pending(x) != 0 {
			t.Fatalf("%s left a queued entry behind", name)
		}
	}
	x.Release()
}

// ArenaDigest reads only the slots an allocator has handed out.
func TestArenaDigestIgnoresStaleSlots(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	base, _ := p.Parse(g.Src)
	ed := base.Copy()
	ed.Edit(selfEdit(g.Src, 6))
	d1 := p.ArenaDigest(ed)
	ccore.PoisonUnused(ed)
	if d2 := p.ArenaDigest(ed); d2 != d1 {
		t.Fatal("the arena digest read a slot that no allocator had handed out")
	}
}

// The readers of a released tree give the zero value.
func TestReadersOfReleasedTree(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	x, _ := p.Parse(g.Src)
	if x.Root() == 0 {
		t.Fatal("a live tree has no root")
	}
	x.Release()
	used, reach := p.ArenaUse(x)
	if x.Root() != 0 || x.Reader() == nil || used != 0 || reach != 0 || p.Digest(x) != "" {
		t.Fatalf("a released tree reads: root %d, arena use %d/%d", x.Root(), used, reach)
	}
}

// ParseWith on a tree that is released while the call waits, also
// when that ends the store (the exit of the call then finds an ended store).
func TestParseWithWhenStoreEndsWhileWaiting(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	p2 := mustParser(t, ccore.Config{Tables: g.Tables})
	for round := 0; round < 100; round++ {
		o, _ := p.Parse(g.Src)
		ccore.LockLease(o)
		done := make(chan string, 1)
		go func() {
			nt, stop := p2.ParseWith(g.Src, o)
			if nt != nil {
				nt.Release()
			}
			done <- stop
		}()
		time.Sleep(3 * time.Millisecond) // the parse is waiting for the lease (it must be past its first check of the old tree, or no parse is left to end the store)
		o.Release()                      // the lease is held: the end of the store is left to the holder
		ccore.UnlockLease(o)
		if stop := <-done; stop != "" && stop != "old tree released" {
			t.Fatalf("round %d: stop %q", round, stop)
		}
		waitFor(t, "the store to end", func() bool { return ccore.Ended(o) })
	}
}

// An Edit that overlaps a Release of the same tree, when the tree owns its
// records alone, must not read freed records or let the store end under it. A
// hook blocks Edit after it started; Release runs on the main goroutine; the old
// root must still be a live record and the store must not have ended; when the
// edit finishes the store ends (without applying the queue) and the counters
// have not moved.
func TestEditOverlappingReleaseOfSoleOwner(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	tr, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	old := tr.Root()
	beforeDouble, beforeBad, beforeShared := p.DoubleFree(), p.BadRetain(), p.SharedInTree()
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	ccore.SetTestHook(tr, func(pt string) {
		if pt == "edit" {
			once.Do(func() { close(entered); <-unblock })
		}
	})
	done := make(chan struct{})
	go func() { tr.Edit(selfEdit(g.Src, 6)); close(done) }()
	<-entered
	tr.Release() // the tree is the only owner; the edit is reading its records
	if ccore.RecordFree(tr, old) {
		close(unblock)
		t.Fatal("the old root was freed while an Edit was reading it")
	}
	if ccore.Ended(tr) {
		close(unblock)
		t.Fatal("the store ended under a running Edit")
	}
	if ccore.Cells(tr) != 1 {
		close(unblock)
		t.Fatalf("cells = %d, want 1 (the running edit's count)", ccore.Cells(tr))
	}
	close(unblock)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Edit did not finish")
	}
	waitFor(t, "the store to end", func() bool { return ccore.Ended(tr) })
	if p.DoubleFree() != beforeDouble || p.BadRetain() != beforeBad || p.SharedInTree() != beforeShared {
		t.Fatalf("counters moved: doubleFree %d, badRetain %d, sharedInTree %d", p.DoubleFree()-beforeDouble, p.BadRetain()-beforeBad, p.SharedInTree()-beforeShared)
	}
	ccore.SetTestHook(tr, nil)
}

// The same overlap when the tree shares its root with a copy: the copy must
// still read an intact tree after the edit of the released one finishes.
func TestEditOverlappingReleaseKeepsTheCopy(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	tr, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	cp := tr.Copy()
	want := p.Digest(cp)
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	ccore.SetTestHook(tr, func(pt string) {
		if pt == "edit" {
			once.Do(func() { close(entered); <-unblock })
		}
	})
	done := make(chan struct{})
	go func() { tr.Edit(selfEdit(g.Src, 6)); close(done) }()
	<-entered
	tr.Release()
	close(unblock)
	<-done
	ccore.SetTestHook(tr, nil)
	if got := p.Digest(cp); got != want {
		t.Fatalf("the copy changed: %s, want %s", got, want)
	}
	cp.Release()
}

// ---- Edit against Release, with another tree keeping the store alive ----

// overlapSetup parses the golden, edits a copy of the first tree and reparses
// with it, so that the second tree owns new records alone while the first tree
// keeps the store alive. It returns the parser, both trees, the source and the
// number of unreachable records after the first parse.
func overlapSetup(t *testing.T) (p *ccore.Parser, base, nt *ccore.Tree, src []byte, leak0 int) {
	t.Helper()
	g := goldenNamed(t, "json-object")
	p = mustParser(t, ccore.Config{Tables: g.Tables})
	base, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	leak0 = ccore.Leaked(base) // a fresh parse leaves one unreachable record; the tests compare with it
	ed := base.Copy()
	ed.Edit(selfEdit(g.Src, 6))
	nt, stop = p.ParseWith(g.Src, ed)
	if stop != "" {
		t.Fatal(stop)
	}
	ed.Release()
	return p, base, nt, g.Src, leak0
}

// blockEdit sets a hook that blocks the first "edit" point until unblock is
// closed (entered is closed when the edit gets there); panicking makes it panic
// afterwards.
func blockEdit(tr *ccore.Tree, panicking bool) (entered, unblock chan struct{}) {
	entered, unblock = make(chan struct{}), make(chan struct{})
	var once sync.Once
	ccore.SetTestHook(tr, func(pt string) {
		if pt != "edit" {
			return
		}
		once.Do(func() {
			close(entered)
			<-unblock
			if panicking {
				panic("edit hook")
			}
		})
	})
	return
}

// finishCounts checks the counters and the end of the store after the last tree.
func finishCounts(t *testing.T, p *ccore.Parser, base *ccore.Tree, leak0 int, double, bad, shared int64) {
	t.Helper()
	if n := ccore.Leaked(base); n != leak0 {
		t.Errorf("%d records unreachable and not free, %d after the first parse: leaked records", n, leak0)
	}
	if p.DoubleFree() != double || p.BadRetain() != bad || p.SharedInTree() != shared {
		t.Errorf("counters moved: doubleFree %d, badRetain %d, sharedInTree %d", p.DoubleFree()-double, p.BadRetain()-bad, p.SharedInTree()-shared)
	}
	ccore.SetTestHook(base, nil)
	base.Release()
	if !ccore.Ended(base) {
		t.Error("the store did not end after the last tree")
	}
}

// A panic inside Edit (here, in the "edit" hook) leaves no count and no mark
// behind: the cell count is back, a Release frees the tree's records, and the
// store ends when the last tree goes.
func TestPanicInEditLeavesNothingBehind(t *testing.T) {
	for _, withCopy := range []bool{false, true} {
		t.Run(fmt.Sprintf("copy=%t", withCopy), func(t *testing.T) {
			p, base, nt, src, leak0 := overlapSetup(t)
			double, bad, shared := p.DoubleFree(), p.BadRetain(), p.SharedInTree()
			var cp *ccore.Tree
			if withCopy {
				cp = nt.Copy()
			}
			root := nt.Root()
			cells := ccore.Cells(base)
			ccore.SetTestHook(base, func(pt string) {
				if pt == "edit" {
					panic("edit hook")
				}
			})
			func() {
				defer func() {
					if recover() == nil {
						t.Error("the hook's panic did not reach the caller")
					}
				}()
				nt.Edit(selfEdit(src, 9))
			}()
			ccore.SetTestHook(base, nil)
			if got := ccore.Cells(base); got != cells {
				t.Fatalf("cells = %d after the panic, want %d", got, cells)
			}
			nt.Release()
			if withCopy {
				if ccore.RecordFree(base, root) {
					t.Error("the root is shared with a live copy and must not be free")
				}
				cp.Release()
			}
			if !ccore.RecordFree(base, root) {
				t.Error("the records of the released tree were not freed")
			}
			finishCounts(t, p, base, leak0, double, bad, shared)
		})
	}
}

// The same panic while a Release of the tree is deferred behind the running
// edit: the panic path queues the release of the old root.
func TestPanicInEditWithReleaseDeferred(t *testing.T) {
	p, base, nt, src, leak0 := overlapSetup(t)
	double, bad, shared := p.DoubleFree(), p.BadRetain(), p.SharedInTree()
	root := nt.Root()
	entered, unblock := blockEdit(base, true)
	done := make(chan any)
	go func() {
		defer func() { done <- recover() }()
		nt.Edit(selfEdit(src, 9))
	}()
	<-entered
	nt.Release() // deferred: the edit is running
	close(unblock)
	if r := <-done; r == nil {
		t.Error("the hook's panic did not reach the caller")
	}
	if !ccore.RecordFree(base, root) {
		t.Error("the deferred release of the old root was not queued and applied")
	}
	finishCounts(t, p, base, leak0, double, bad, shared)
}

// Edit against Release of a tree that owns records alone while its base tree
// lives: the queued retains and both releases must be applied (no leak, no
// double free).
func TestEditOverlappingReleaseWithBaseAlive(t *testing.T) {
	p, base, nt, src, leak0 := overlapSetup(t)
	double, bad, shared := p.DoubleFree(), p.BadRetain(), p.SharedInTree()
	entered, unblock := blockEdit(base, false)
	done := make(chan struct{})
	go func() { nt.Edit(selfEdit(src, 9)); close(done) }()
	<-entered
	nt.Release()
	close(unblock)
	<-done
	finishCounts(t, p, base, leak0, double, bad, shared)
}

// An edit that changes nothing, overlapped by the release of its tree: the
// deferred release of the old root must still be queued.
func TestEditThatChangesNothingOverlappingRelease(t *testing.T) {
	p, base, nt, src, leak0 := overlapSetup(t)
	double, bad, shared := p.DoubleFree(), p.BadRetain(), p.SharedInTree()
	root := nt.Root()
	far := uint32(len(src) + 100) // after the end of the text: no record changes
	e := ccore.Edit{StartByte: far, OldEndByte: far, NewEndByte: far}
	entered, unblock := blockEdit(base, false)
	done := make(chan struct{})
	go func() { nt.Edit(e); close(done) }()
	<-entered
	nt.Release()
	close(unblock)
	<-done
	if !ccore.RecordFree(base, root) {
		t.Error("the old root was not released (this edit changed a record, or the deferred release was lost)")
	}
	finishCounts(t, p, base, leak0, double, bad, shared)
}

// A Copy during the overlap keeps the root alive; the copy still reads an
// intact tree after the edit of the released tree finishes.
func TestCopyDuringEditOverlappingRelease(t *testing.T) {
	p, base, nt, src, leak0 := overlapSetup(t)
	double, bad, shared := p.DoubleFree(), p.BadRetain(), p.SharedInTree()
	want := p.Digest(nt)
	entered, unblock := blockEdit(base, false)
	done := make(chan struct{})
	go func() { nt.Edit(selfEdit(src, 9)); close(done) }()
	<-entered
	cp := nt.Copy()
	nt.Release()
	if nt.Copy() != nil {
		t.Error("Copy of a released tree must return nil")
	}
	close(unblock)
	<-done
	if got := p.Digest(cp); got != want {
		t.Errorf("the copy changed: %s, want %s", got, want)
	}
	cp.Release()
	finishCounts(t, p, base, leak0, double, bad, shared)
}

// A parse holds the lease when the edit commits: the edit's entries stay queued
// (its drain cannot get the lease) and the next parse applies them when it ends.
func TestParseHoldsLeaseAtEditCommit(t *testing.T) {
	p, base, nt, src, leak0 := overlapSetup(t)
	double, bad, shared := p.DoubleFree(), p.BadRetain(), p.SharedInTree()
	entered, unblock := blockEdit(base, false)
	done := make(chan struct{})
	go func() { nt.Edit(selfEdit(src, 9)); close(done) }()
	<-entered
	nt.Release()
	ccore.LockLease(base) // as a running parse does
	close(unblock)
	<-done
	if ccore.Pending(base) == 0 {
		t.Error("the entries of the edit were applied although the lease was held")
	}
	ccore.UnlockLease(base)
	x := base.Copy()
	y, stop := p.ParseWith(src, x) // its exit applies what is queued
	if stop != "" {
		t.Fatal(stop)
	}
	if n := ccore.Pending(base); n != 0 {
		t.Errorf("%d entries still queued after the parse", n)
	}
	y.Release()
	x.Release()
	finishCounts(t, p, base, leak0, double, bad, shared)
}

// ---- Reader contract ----

// Readers of trees of two stores use one parser while that parser parses: the
// readers must not write the parser's state. Run under -race this fails when a
// reader swaps the parser's store view.
func TestReadersDoNotDisturbARunningParse(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	t1, _ := p.Parse(g.Src)
	t2, _ := p.Parse(g.Src)
	want1, want2 := p.Digest(t1), p.Digest(t2)
	var wg sync.WaitGroup
	var bad atomic.Int32
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			nt, stop := p.Parse(g.Src)
			if stop != "" || nt == nil {
				bad.Add(1)
				continue
			}
			nt.Release()
		}
	}()
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				tr, want := t1, want1
				if (i+r)%2 == 1 {
					tr, want = t2, want2
				}
				if p.Digest(tr) != want || p.Walk(tr) == 0 || p.RootEnd(tr) == 0 {
					bad.Add(1)
				}
				if _, n := p.Records(tr, nil); n == 0 {
					bad.Add(1)
				}
			}
		}(r)
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d wrong answers", bad.Load())
	}
	t1.Release()
	t2.Release()
}

// An Edit of the old tree on another goroutine adds the first edit page and
// publishes a new root after the parse took its view and drained: the parse
// must read the root through page-table headers that include that page. The
// hook runs the edit at that point.
func TestParseCapturesRootAndHeadersTogether(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	old, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	fresh := p.Digest(old)
	fired := false
	ccore.SetTestHook(old, func(pt string) {
		if pt == "parse-start" && !fired {
			fired = true
			done := make(chan struct{})
			go func() { old.Edit(selfEdit(g.Src, 6)); close(done) }()
			<-done
		}
	})
	nt, stop, pan := parseCatch(p, g.Src, old)
	ccore.SetTestHook(old, nil)
	if !fired {
		t.Fatal("the hook did not run")
	}
	if pan != nil {
		t.Fatalf("the parse panicked: %v", pan)
	}
	if stop != "" || nt == nil {
		t.Fatalf("stop %q", stop)
	}
	if got := p.Digest(nt); got != fresh {
		t.Fatalf("digest %s, want %s", got, fresh)
	}
	nt.Release()
	old.Release()
}

// A copy of a view keeps the tree's cell reachable: the cell's finalizer does
// not run (the store does not end, its pages are not recycled) while the copy
// exists, and runs after it is dropped.
func TestCopiedViewKeepsTheCellReachable(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	tr, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	ended := ccore.EndedFunc(tr)
	view := *tr.Reader()
	root := tr.Root()
	sym := view.Symbol(root)
	tr = nil
	for i := 0; i < 20; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if ended() {
		t.Fatal("the store ended while a copy of the view exists: the finalizer ran")
	}
	if got := view.Symbol(root); got != sym {
		t.Fatalf("the view reads symbol %d, want %d", got, sym)
	}
	view = ccore.Store{}
	waitFor(t, "the finalizer to end the store after the view is dropped", func() bool {
		runtime.GC()
		return ended()
	})
}

// A released Tree object that stays reachable holds no page.
func TestReleasedTreeHoldsNoPage(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	a, _ := p.Parse(g.Src)
	b := a.Copy()
	if ccore.ViewPages(b) == 0 {
		t.Fatal("a live tree holds pages")
	}
	b.Release()
	if n := ccore.ViewPages(b); n != 0 {
		t.Fatalf("a released tree holds %d page pointers", n)
	}
	a.Release()
	if n := ccore.ViewPages(a); n != 0 {
		t.Fatalf("a released tree holds %d page pointers", n)
	}
}

// ---- Store limits ----

// A parse that would pass the record-index limit stops with an error before it
// allocates the record, returns no tree, leaves the lease free, and the next
// parse (limit restored) gives the fresh digest.
func TestRecordIndexLimitStopsTheParse(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	tr, stop := p.Parse(g.Src)
	if stop != "" {
		t.Fatal(stop)
	}
	want := p.Digest(tr)
	tr.Release()
	p.SetTestLimits(5, 1<<28-1, 1<<19, 1<<16)
	nt, stop := p.Parse(g.Src)
	p.SetTestLimits(1<<28-1, 1<<28-1, 1<<19, 1<<16)
	if nt != nil || !strings.Contains(stop, "record index limit") {
		t.Fatalf("tree %v, stop %q: want no tree and a record index limit text", nt != nil, stop)
	}
	if !p.LeaseFree() {
		t.Fatal("the lease is held after the stop")
	}
	again, stop := p.Parse(g.Src)
	if stop != "" || p.Digest(again) != want {
		t.Fatalf("the next parse: stop %q", stop)
	}
	again.Release()
}

// A reference count that would pass the limit is refused in the publication
// walk: the parse stops with an error, and the store is poisoned (the walk had
// raised counts it cannot take back). The next parse of a tree of that store
// parses afresh.
func TestCountLimitIsRefusedInPublication(t *testing.T) {
	p, base, _, src, _ := overlapSetup(t)
	want := p.Digest(base)
	cp := base.Copy()
	cp.Edit(selfEdit(src, 6))
	ccore.DrainNow(base) // the setup's queued retains are applied before the limit drops, so only the publication walk can refuse
	published := 0
	ccore.SetTestHook(base, func(pt string) {
		if pt == "publish" {
			published++
		}
	})
	ccore.SetCountLimit(base, 0)
	nt, stop := p.ParseWith(src, cp)
	ccore.SetTestHook(base, nil)
	if published == 0 {
		t.Fatal("the publication walk did not run: the limit was refused somewhere else")
	}
	if nt != nil || !strings.Contains(stop, "reference count limit") {
		t.Fatalf("tree %v, stop %q: want no tree and a count limit text", nt != nil, stop)
	}
	if !ccore.Poisoned(base) {
		t.Error("the store is not poisoned after a refused publication")
	}
	if !p.LeaseFree() {
		t.Error("the lease is held after the stop")
	}
	fresh, stop := p.ParseWith(src, cp)
	if stop != "" || fresh == nil || p.Digest(fresh) != want {
		t.Fatalf("a parse of a tree of the poisoned store: stop %q", stop)
	}
	fresh.Release()
	cp.Release()
	base.Release()
}

// ---- Poisoned stores ----

// poisonCase injects a panic at the k-th call of a hook point and checks what a
// poisoned store promises: the old tree still reads correctly, a reparse gives
// the fresh tree in a new store, no record of the poisoned store is freed or
// reused while a tree of it lives, and the pool gets the pages back at the end.
func poisonCase(t *testing.T, point string, k int, trigger func(p *ccore.Parser, base, nt *ccore.Tree, src []byte)) {
	t.Helper()
	p, base, nt, src, _ := overlapSetup(t)
	want := p.Digest(base)
	ccore.TrimPool(1 << 62)
	oldMax := ccore.SetPoolMaxBytes(1 << 30)
	defer ccore.SetPoolMaxBytes(oldMax)
	calls := 0
	ccore.SetTestHook(base, func(pt string) {
		if pt == point {
			calls++
			if calls == k {
				panic("injected at " + point)
			}
		}
	})
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("a failure inside the store reached the caller as a panic: %v", r)
			}
		}()
		trigger(p, base, nt, src)
	}()
	ccore.SetTestHook(base, nil)
	if calls < k {
		t.Fatalf("the hook point %s was called %d times, fewer than %d", point, calls, k)
	}
	if !ccore.Poisoned(base) {
		t.Fatal("the store is not poisoned after the panic")
	}
	if !p.LeaseFree() {
		t.Error("the lease is held after the panic")
	}
	if got := p.Digest(base); got != want {
		t.Errorf("the old tree reads %s, want %s", got, want)
	}
	recs, _, _, _, _, _, _, bytes := ccore.PagesHeld(base)
	cp := base.Copy()
	fresh, stop := p.ParseWith(src, cp)
	if stop != "" || fresh == nil {
		t.Fatalf("reparse: stop %q", stop)
	}
	if got := p.Digest(fresh); got != want {
		t.Errorf("the reparse gives %s, want the fresh digest %s", got, want)
	}
	if r2, _, _, _, _, _, _, b2 := ccore.PagesHeld(base); r2 != recs || b2 != bytes {
		t.Errorf("the poisoned store changed: %d record pages and %d bytes before, %d and %d after the reparse", recs, bytes, r2, b2)
	}
	if got := p.Digest(base); got != want {
		t.Errorf("after the reparse the old tree reads %s, want %s", got, want)
	}
	pool := ccore.PoolBytes()
	fresh.Release()
	cp.Release()
	if nt != nil {
		nt.Release() // may already be released by the trigger
	}
	base.Release()
	if !ccore.Ended(base) {
		t.Error("the poisoned store did not end after its last tree")
	}
	if ccore.PoolBytes() <= pool {
		t.Errorf("the pool did not get the pages back: %d bytes before, %d after", pool, ccore.PoolBytes())
	}
}

// A panic in the middle of a publication walk (the third marked record).
func TestPoisonedByPanicInPublication(t *testing.T) {
	poisonCase(t, "publish", 3, func(p *ccore.Parser, base, nt *ccore.Tree, src []byte) {
		cp := base.Copy()
		cp.Edit(selfEdit(src, 6))
		defer cp.Release()
		p.ParseWith(src, cp)
	})
}

// A panic in the middle of an exit sweep (the third record).
func TestPoisonedByPanicInSweep(t *testing.T) {
	poisonCase(t, "sweep-record", 3, func(p *ccore.Parser, base, nt *ccore.Tree, src []byte) {
		cp := base.Copy()
		cp.Edit(selfEdit(src, 6))
		defer cp.Release()
		defer p.SetStopAfter(0, false)
		for n := 20; n <= 400; n += 20 { // a parse that stops without a tree sweeps what it allocated; the panic ends the loop
			p.SetStopAfter(n, false)
			p.ParseWith(src, cp)
			if ccore.Poisoned(base) {
				break
			}
		}
	})
}

// A panic in the middle of a release walk (the second record).
func TestPoisonedByPanicInReleaseWalk(t *testing.T) {
	poisonCase(t, "release-record", 2, func(p *ccore.Parser, base, nt *ccore.Tree, src []byte) {
		nt.Release()
	})
}

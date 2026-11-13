//go:build cgo && treesitter_c_parity

package cgoharness

// Store gates: the long chains (3), the stopped
// parses (6), the race run (7) and the determinism of handles (8).

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ccoreStopHookPanic is the value of the panic that the core's stop hook raises
// (Parser.SetStopAfter with panicking set); no other panic is accepted by the gates.
const ccoreStopHookPanic = "ccore: stop hook"

// ccoreStopText is the stop reason of a parse that the hook stopped without a panic.
const ccoreStopText = "stopped by hook"

// ccoreInjected reports whether the recovered value r is exactly the injected
// stop-hook panic (the value is compared, not its printed form).
func ccoreInjected(r any) bool {
	v, ok := r.(string)
	return ok && v == ccoreStopHookPanic
}

// ccoreStoreIntact fails the test when the store of tr is poisoned: a store
// failure is quiet (it becomes a stop text, or nothing at all in an exit drain),
// so the gates look at the store itself.
func ccoreStoreIntact(t *testing.T, core *bindtest.Parser, tr bindtest.Tree, where string) {
	t.Helper()
	if tr.Valid() {
		if f := core.StoreFailure(tr); f != "" {
			t.Errorf("%s: the store was poisoned: %s", where, f)
		}
	}
}

func ccoreEnvInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// ccoreChainSource returns the file of a chain and the prefix of the function
// names whose digits the chain changes.
func ccoreChainSource(t testing.TB, lang string) ([]byte, string) {
	switch lang {
	case "go":
		src, _, _, _ := ccoreBigEdits(t)
		return src, "func f"
	case "python":
		var b bytes.Buffer
		for i := 0; b.Len() < 70000; i++ {
			fmt.Fprintf(&b, "def f%d(a, b):\n    x = a + b\n    print(\"f%d\", x)\n    return x\n\n\n", i, i)
		}
		return b.Bytes(), "def f"
	}
	t.Fatalf("no chain for %q", lang)
	return nil, ""
}

// ccoreDigitPositions lists the digits that follow prefix in src.
func ccoreDigitPositions(src []byte, prefix string) []int {
	var digits []int
	for i := 0; i+len(prefix) < len(src); i++ {
		if string(src[i:i+len(prefix)]) == prefix {
			for j := i + len(prefix); j < len(src) && src[j] >= '0' && src[j] <= '9'; j++ {
				digits = append(digits, j)
			}
		}
	}
	return digits
}

func ccoreFirstDiff(a, b []string) int {
	for k := 0; k < len(a) || k < len(b); k++ {
		if k >= len(a) || k >= len(b) || ccoreNormLine(a[k]) != ccoreNormLine(b[k]) {
			return k
		}
	}
	return -1
}

// TestCcoreStoreChain is gate 3: a chain that keeps the source valid (one
// digit of a function name changes per step), with the core's tree edited in
// place and released after each reparse, against C's chain.
func TestCcoreStoreChain(t *testing.T) {
	lang := os.Getenv("CCORE_CHAIN_LANG")
	if lang == "" {
		t.Skip("set CCORE_CHAIN_LANG=go|python")
	}
	steps := ccoreEnvInt("CCORE_CHAIN_STEPS", 10000)
	sample := ccoreEnvInt("CCORE_CHAIN_SAMPLE", 250)
	gl, cl := ccoreLanguages(t, lang)
	src, prefix := ccoreChainSource(t, lang)
	digits := ccoreDigitPositions(src, prefix)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var clog []string
	logOn := false
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if logOn && kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	core := ccoreNew(gl)
	if stop := core.Parse(src, nil); stop != "" {
		t.Fatalf("fresh parse stop %q", stop)
	}
	tree := core.Tree()
	ctree := cp.Parse(src, nil)
	used0, reach0 := core.ArenaUse(tree)
	fmt.Printf("CCMEMCHAIN lang=%s bytes=%d digit_positions=%d steps=%d fresh_used=%d fresh_reachable=%d\n", lang, len(src), len(digits), steps, used0, reach0)
	rng := rand.New(rand.NewSource(77))
	cur := append([]byte(nil), src...)
	maxUsed := used0
	sampled, sampledOK := 0, 0
	var ms runtime.MemStats
	for step := 1; step <= steps; step++ {
		at := digits[rng.Intn(len(digits))]
		b := byte('0' + rng.Intn(10))
		if b == cur[at] {
			b = '0' + (cur[at]-'0'+1)%10
		}
		next, e := ccoreReplaceEdit(cur, at, b)
		smp := step%sample == 0 || step == 1
		var slog []string
		var logf func(string)
		if smp {
			logf = func(s string) { slog = append(slog, s) }
		}
		core.EditInPlace(tree, e)
		editRecs := tree.EditRecords()
		ntree, stop := core.ParseIncremental(next, tree, logf)
		ctree.Edit(ccoreSitterEdit(&e))
		clog = clog[:0]
		logOn = smp
		cnext := cp.Parse(next, ctree)
		logOn = false
		ctree.Close()
		ctree = cnext
		leaves, nodes, copies := core.RecordSources()
		core.Release(tree)
		tree, cur = ntree, next
		used := core.StoreBytes()
		if used > maxUsed {
			maxUsed = used
		}
		line := fmt.Sprintf("CCMEM step=%d stop=%q rec_leaf=%d rec_node=%d rec_copy=%d rec_edit=%d used=%d", step, stop, leaves, nodes, copies, editRecs, used)
		if smp {
			_, reach := core.ArenaUse(tree)
			line += fmt.Sprintf(" reachable=%d", reach)
			reused, _, _ := core.ReuseStats()
			sd, cd := core.DigestOf(tree, false), ccoreCDigestChanges(ctree, false)
			ch, cn, rc := corecords.Hash(ctree, "")
			gh, gn := core.RecordsHash("")
			first := ccoreFirstDiff(clog, slog)
			ok := stop == "" && first < 0 && sd == cd && rc == 0 && ch == gh && cn == gn &&
				reused == ccoreCountLines(clog, "reuse_node") && ccoreCountLines(slog, "lexed_lookahead") == ccoreCountLines(clog, "lexed_lookahead") &&
				ccoreCountLines(slog, "reduce sym") == ccoreCountLines(clog, "reduce sym")
			sampled++
			if ok {
				sampledOK++
			}
			line += fmt.Sprintf(" sampled=1 ok=%t trace_eq=%t first_diff=%d digest_eq=%t rec_eq=%t reused=%d c_reused=%d lexed=%d c_lexed=%d reduce=%d c_reduce=%d",
				ok, first < 0, first, sd == cd, rc == 0 && ch == gh && cn == gn, reused, ccoreCountLines(clog, "reuse_node"),
				ccoreCountLines(slog, "lexed_lookahead"), ccoreCountLines(clog, "lexed_lookahead"), ccoreCountLines(slog, "reduce sym"), ccoreCountLines(clog, "reduce sym"))
		}
		fmt.Println(line)
		if step%1000 == 0 || step == steps {
			runtime.GC()
			runtime.ReadMemStats(&ms)
			fmt.Printf("CCHEAP step=%d heap_alloc=%d heap_inuse=%d heap_sys=%d store_used=%d %s\n", step, ms.HeapAlloc, ms.HeapInuse, ms.HeapSys, used, core.StoreInfo())
		}
	}
	used, reach := core.ArenaUse(tree)
	fresh := ccoreNew(gl)
	fresh.Parse(cur, nil)
	fused, freach := fresh.ArenaUse(fresh.Tree())
	cf := cp.Parse(cur, nil)
	finalEq := core.DigestOf(tree, false) == ccoreCDigestChanges(cf, false) && fresh.Digest() == core.DigestOf(tree, false)
	cf.Close()
	fmt.Printf("CCMEMDONE lang=%s steps=%d held=%d fresh=%d ratio=%.3f max_held=%d reachable=%d fresh_reachable=%d sampled=%d sampled_ok=%d final_eq=%t shared_in_tree=%d double_free=%d bad_retain=%d\n",
		lang, steps, used, fused, float64(used)/float64(fused), maxUsed, reach, freach, sampled, sampledOK, finalEq, core.SharedInTree(), core.DoubleFree(), core.BadRetain())
	core.Release(tree)
	ctree.Close()
	if !finalEq || sampledOK != sampled || float64(used)/float64(fused) > 1.5 || core.SharedInTree() != 0 || core.DoubleFree() != 0 || core.BadRetain() != 0 {
		t.Errorf("gate 3 failed: final_eq=%t sampled_ok=%d of %d ratio=%.3f shared_in_tree=%d double_free=%d bad_retain=%d",
			finalEq, sampledOK, sampled, float64(used)/float64(fused), core.SharedInTree(), core.DoubleFree(), core.BadRetain())
	}
	ccoreCount("TestCcoreStoreChain", lang, sampled)
}

// TestCcoreStopped is gate 6: incremental parses stopped (or panicking) at
// random step counts. The store must hold the same bytes at the end, the lease
// must be free, and the next full parse must equal C.
func TestCcoreStopped(t *testing.T) {
	if os.Getenv("CCORE_STOPPED") == "" {
		t.Skip("set CCORE_STOPPED=1")
	}
	n := ccoreEnvInt("CCORE_STOPPED_N", 1000)
	gl, cl := ccoreLanguages(t, "go")
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	src, _, _, _ := ccoreBigEdits(t)
	core := ccoreNew(gl)
	core.Parse(src, nil)
	base := core.Tree()
	rng := rand.New(rand.NewSource(606))
	pick := func() ([]byte, bindtest.Edit) {
		for {
			at := rng.Intn(len(src))
			if ccoreIsIdentInterior(src, at) {
				b := byte('a' + rng.Intn(26))
				if b == src[at] {
					b = 'z'
					if src[at] == 'z' {
						b = 'y'
					}
				}
				return ccoreReplaceEdit(src, at, b)
			}
		}
	}
	// Warm-up: one edit and one complete reparse, so that the edit allocator has its page.
	next0, e0 := pick()
	ed0 := core.Edit(base, e0)
	nt0, stop0 := core.ParseIncremental(next0, ed0, nil)
	core.Release(nt0)
	core.Release(ed0)
	used0, _ := core.ArenaUse(base)
	fmt.Printf("CCSTOPBASE stop=%q used=%d %s\n", stop0, used0, core.StoreInfo())
	var edited bindtest.Tree
	var nextSrc []byte
	var lastEdit bindtest.Edit
	stopped, completed, panics, withTree := 0, 0, 0, 0
	for i := 0; i < n; i++ {
		if i%20 == 0 {
			if edited.Valid() {
				core.Release(edited)
			}
			var e bindtest.Edit
			nextSrc, e = pick()
			lastEdit = e
			edited = core.Edit(base, e)
		}
		panicking := i%10 == 9
		core.SetStopAfter(1+rng.Intn(14000), panicking)
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Only the injected stop-hook panic, and only in a round that
					// enabled it, is accepted; any other panic fails the gate.
					if panicking && ccoreInjected(r) {
						panics++
					} else {
						t.Errorf("round %d: unexpected panic (injection %t): %v", i, panicking, r)
					}
				}
			}()
			tr, stop := core.ParseIncremental(nextSrc, edited, nil)
			ccoreStoreIntact(t, core, edited, fmt.Sprintf("round %d (after the parse and its exit cleanup)", i))
			if stop != "" {
				if stop != ccoreStopText || panicking { // only the hook's own stop, and only in a round that did not ask for a panic
					t.Errorf("round %d: unexpected stop %q (injection %t)", i, stop, panicking)
				}
				stopped++
				if tr.Valid() {
					withTree++
				}
			} else {
				completed++
				core.Release(tr)
			}
		}()
		ccoreStoreIntact(t, core, edited, fmt.Sprintf("round %d (after a recovered panic)", i))
	}
	core.SetStopAfter(0, false)
	ccoreStoreIntact(t, core, base, "gate 6 end")
	core.Release(edited)
	used1, _ := core.ArenaUse(base)
	free := core.LeaseFree()
	cf := cp.Parse(nextSrc, nil)
	// The recovery parse uses the interrupted parser and its store: the same edit
	// of the base tree, reparsed with the parser that was stopped and made to
	// panic 1,000 times.
	rec := core.Edit(base, lastEdit)
	rtree, fstop := core.ParseIncremental(nextSrc, rec, nil)
	eq := fstop == "" && rtree.Valid() && core.DigestOf(rtree, false) == ccoreCDigestChanges(cf, false)
	core.Release(rtree)
	core.Release(rec)
	cf.Close()
	if stopped == 0 || panics == 0 || completed == 0 {
		t.Errorf("gate 6 did not exercise all paths: stopped %d, panics %d, completed %d", stopped, panics, completed)
	}
	fmt.Printf("CCSTOPPED n=%d stopped=%d completed=%d panics=%d stopped_with_tree=%d used_before=%d used_after=%d unchanged=%t lease_free=%t next_full_eq_c=%t shared_in_tree=%d double_free=%d bad_retain=%d %s\n",
		n, stopped, completed, panics, withTree, used0, used1, used0 == used1, free, eq, core.SharedInTree(), core.DoubleFree(), core.BadRetain(), core.StoreInfo())
	if withTree != 0 || used0 != used1 || !free || !eq || core.SharedInTree() != 0 || core.DoubleFree() != 0 || core.BadRetain() != 0 {
		t.Errorf("gate 6 failed: stopped_with_tree=%d bytes %d to %d lease_free=%t next_full_eq_c=%t shared_in_tree=%d double_free=%d bad_retain=%d",
			withTree, used0, used1, free, eq, core.SharedInTree(), core.DoubleFree(), core.BadRetain())
	}
}

// TestCcoreHandles is gate 8: two (here three) identical edit sequences with
// explicit Release give identical handles.
func TestCcoreHandles(t *testing.T) {
	if os.Getenv("CCORE_HANDLES") == "" {
		t.Skip("set CCORE_HANDLES=1")
	}
	steps := ccoreEnvInt("CCORE_HANDLES_STEPS", 200)
	gl, _ := ccoreLanguages(t, "go")
	src, prefix := ccoreChainSource(t, "go")
	digits := ccoreDigitPositions(src, prefix)
	run := func() []uint64 {
		core := ccoreNew(gl)
		core.Parse(src, nil)
		tree := core.Tree()
		rng := rand.New(rand.NewSource(808))
		cur := append([]byte(nil), src...)
		out := []uint64{tree.Handle()}
		for step := 0; step < steps; step++ {
			at := digits[rng.Intn(len(digits))]
			b := byte('0' + rng.Intn(10))
			if b == cur[at] {
				b = '0' + (cur[at]-'0'+1)%10
			}
			next, e := ccoreReplaceEdit(cur, at, b)
			core.EditInPlace(tree, e)
			out = append(out, tree.Handle())
			nt, _ := core.ParseIncremental(next, tree, nil)
			out = append(out, nt.Handle())
			core.Release(tree)
			tree, cur = nt, next
		}
		core.Release(tree)
		return out
	}
	a, b, c := run(), run(), run()
	same := len(a) == len(b) && len(a) == len(c)
	first := -1
	for i := range a {
		if i >= len(b) || i >= len(c) || a[i] != b[i] || a[i] != c[i] {
			same, first = false, i
			break
		}
	}
	fmt.Printf("CCHANDLES steps=%d handles=%d runs=3 equal=%t first_diff=%d last=%d\n", steps, len(a), same, first, a[len(a)-1])
	if !same {
		t.Errorf("gate 8 failed: handles differ between runs, first at index %d", first)
	}
}

// TestCcoreRace is gate 7. Run it with the race detector. Readers walk old
// trees on other goroutines while parses of the same document run; trees are
// released, dropped without Release, copied and dropped; copies are edited
// during a parse; some parses are stopped.
func TestCcoreRace(t *testing.T) {
	if os.Getenv("CCORE_RACE") == "" {
		t.Skip("set CCORE_RACE=1")
	}
	rounds := ccoreEnvInt("CCORE_RACE_ROUNDS", 60)
	lang := os.Getenv("CCORE_RACE_LANG")
	if lang == "" {
		lang = "go"
	}
	gl, _ := ccoreLanguages(t, lang)
	var src []byte
	var digits []int
	if lang == "python" {
		var prefix string
		src, prefix = ccoreChainSource(t, "python")
		digits = ccoreDigitPositions(src, prefix)
	} else {
		var err error
		src, _, err = benchfixtures.GeneratedSource("go", 140288)
		if err != nil {
			t.Fatal(err)
		}
		digits = ccoreDigitPositions(src, "func f")
	}
	core := ccoreNew(gl)
	core.Parse(src, nil)
	t0 := core.Tree()
	wantN, wantFold := t0.CountNodes()
	wantDigest := core.DigestOf(t0, false)
	var stop atomic.Bool
	var bad atomic.Int64
	var walks, digests atomic.Int64
	var wg sync.WaitGroup
	// readers: walk fixed old trees (t0 and a copy of it) without any lock
	t0copy := core.Copy(t0)
	for r := 0; r < 3; r++ {
		wg.Add(1)
		go func(tr bindtest.Tree) {
			defer wg.Done()
			for !stop.Load() {
				n, fold := tr.CountNodes()
				if n != wantN || fold != wantFold {
					bad.Add(1)
				}
				walks.Add(1)
			}
		}(map[bool]bindtest.Tree{true: t0, false: t0copy}[r%2 == 0])
	}
	// a reader that calls the parser's Digest on the same trees while parses run
	// (the readers use their own read context and must not disturb the parse)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			if core.DigestOf(t0, false) != wantDigest || core.DigestOf(t0copy, false) != wantDigest {
				bad.Add(1)
			}
			digests.Add(1)
		}
	}()
	// editor: edits copies of t0 while parses run, then releases or drops them
	var edits atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		erng := rand.New(rand.NewSource(7))
		for !stop.Load() {
			cpy := core.Copy(t0)
			for k := 0; k < 3; k++ {
				at := digits[erng.Intn(len(digits))]
				_, e := ccoreReplaceEdit(src, at, byte('0'+erng.Intn(10)))
				core.EditInPlace(cpy, e)
				edits.Add(1)
			}
			if erng.Intn(2) == 0 {
				core.Release(cpy)
			} // else: dropped without Release; the finalizer releases it
			runtime.Gosched()
		}
	}()
	// writer: chain of incremental parses of the same document
	rng := rand.New(rand.NewSource(9))
	cur := append([]byte(nil), src...)
	tree := core.Copy(t0)
	stopped, completed, panicsInjected, panicsSeen := 0, 0, 0, 0
	for i := 0; i < rounds; i++ {
		at := digits[rng.Intn(len(digits))]
		b := byte('0' + rng.Intn(10))
		if b == cur[at] {
			b = '0' + (cur[at]-'0'+1)%10
		}
		next, e := ccoreReplaceEdit(cur, at, b)
		ed := core.Copy(tree)
		core.EditInPlace(ed, e)
		injected := false
		if i%7 == 3 {
			injected = i%14 == 3
			core.SetStopAfter(1+rng.Intn(3000), injected)
			if injected {
				panicsInjected++
			}
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					if injected && ccoreInjected(r) {
						panicsSeen++
					} else {
						t.Errorf("round %d: unexpected panic (injection %t): %v", i, injected, r)
					}
				}
			}()
			nt, st := core.ParseIncremental(next, ed, nil)
			ccoreStoreIntact(t, core, ed, fmt.Sprintf("round %d (after the parse and its exit cleanup)", i))
			if st != "" {
				if st != ccoreStopText || i%7 != 3 || injected { // only the hook's own stop, and only in a round that enabled the hook without a panic
					t.Errorf("round %d: unexpected stop %q (hook enabled %t, injection %t)", i, st, i%7 == 3, injected)
				}
				stopped++
			} else {
				completed++
				switch i % 3 {
				case 0: // explicit release of the old tree
					core.Release(tree)
					tree, cur = nt, next
				case 1: // drop the old tree without Release
					tree, cur = nt, next
				default: // copy, then drop the copy
					_ = core.Copy(nt)
					core.Release(tree)
					tree, cur = nt, next
				}
			}
		}()
		ccoreStoreIntact(t, core, ed, fmt.Sprintf("round %d (after a recovered panic)", i))
		core.SetStopAfter(0, false)
		core.Release(ed)
		if i%10 == 0 {
			runtime.GC()
			time.Sleep(time.Millisecond)
		}
	}
	stop.Store(true)
	wg.Wait()
	ccoreStoreIntact(t, core, t0, "gate 7 end")
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	n, fold := t0.CountNodes()
	// A finalizer may still be draining released trees on its own goroutine; the lease must come free.
	leaseFree := core.LeaseFree()
	for i := 0; i < 200 && !leaseFree; i++ {
		time.Sleep(10 * time.Millisecond)
		leaseFree = core.LeaseFree()
	}
	fmt.Printf("CCRACE lang=%s rounds=%d stopped=%d walks=%d walks_bad=%d edits=%d t0_unchanged=%t shared_in_tree=%d double_free=%d bad_retain=%d lease_free=%t\n",
		lang, rounds, stopped, walks.Load(), bad.Load(), edits.Load(), n == wantN && fold == wantFold, core.SharedInTree(), core.DoubleFree(), core.BadRetain(), leaseFree)
	core.Release(tree)
	fmt.Printf("CCRACE2 completed_writes=%d panics_injected=%d panics_seen=%d reader_walks=%d digest_reads=%d editor_edits=%d\n", completed, panicsInjected, panicsSeen, walks.Load(), digests.Load(), edits.Load())
	if completed == 0 || walks.Load() == 0 || digests.Load() == 0 || edits.Load() == 0 {
		t.Errorf("gate 7 did no work: completed writes %d, reader walks %d, digest reads %d, editor edits %d", completed, walks.Load(), digests.Load(), edits.Load())
	}
	if panicsSeen == 0 || panicsSeen > panicsInjected { // an injected panic does not fire when the parse is shorter than its step count
		t.Errorf("gate 7: %d panics seen, %d injected (want at least one seen and none uninjected)", panicsSeen, panicsInjected)
	}
	if bad.Load() != 0 || n != wantN || fold != wantFold || !leaseFree || core.SharedInTree() != 0 || core.DoubleFree() != 0 || core.BadRetain() != 0 {
		t.Errorf("gate 7 failed: walks_bad=%d t0_unchanged=%t lease_free=%t shared_in_tree=%d double_free=%d bad_retain=%d",
			bad.Load(), n == wantN && fold == wantFold, leaseFree, core.SharedInTree(), core.DoubleFree(), core.BadRetain())
	}
}

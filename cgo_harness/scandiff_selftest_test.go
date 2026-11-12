//go:build cgo && treesitter_c_parity

package cgoharness

// Tests of the scanner differential harness itself. They need no grammar: stand-ins replace the two
// scanners. Each fix of the harness has a test that goes through the production code. The mutant switches
// (SCANDIFF_MUTANT=<name>) exist only in a build with the tag scandiff_mutants, which only the self-test
// job uses; with the switch on, the test of that fix must FAIL, and without it (an ordinary build) it must
// pass. The mutants and their tests:
//
//	gated       TestScandiffSelfMutantsGated      the mutant switches are compiled into the build (the tag)
//	len         TestScandiffSelfLength            state length converted to 32 bits before the comparison
//	replayhist  TestScandiffSelfReplayHistory     a replay looks only at its final call
//	replaymask  TestScandiffSelfReplayMask        replay adds the recorded token names to the recorded mask
//	crashbuf    TestScandiffSelfCrashBuffers      a reset keeps the Go serialization buffer
//	buffer      TestScandiffSelfBufferAcrossParses  a new source zeroes the serialization buffers
//	resetfail   TestScandiffSelfResetFails        a failed payload reset is ignored
//	reproverify TestScandiffSelfReproducerVerified  no check that a written reproducer reproduces
//	rows        TestScandiffSelfRowsDerived       absent Go rows are accepted without a comparison
//	ranges      TestScandiffSelfRangeCalls        no calls with an included range
//	fullrange   TestScandiffSelfRangeCategories   a range over the whole source is a range call
//	rangepair   TestScandiffSelfRangeCategories   range calls run on the ordinary pair
//	sig         TestScandiffSelfSignature         a signature without the two results
//	sigrange    TestScandiffSelfSignature         a signature without the call's range
//	watchdog    TestScandiffSelfWatchdogWrite     the result of the hang record's write is ignored
//	crashgate   TestScandiffSelfCrashGate         a crash or a skipped call does not fail a gate run
//	zerobranch  TestScandiffSelfZeroBranches      a file with no branch reaches a coverage plateau
//
// What these tests do not reach: the C driver itself (its length capture, its reset) needs a loaded
// grammar; it is exercised by the grammar runs.

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/scandiff"
	"github.com/odvcencio/gotreesitter/internal/ccore"
)

func newSdRand(n int64) *rand.Rand { return rand.New(rand.NewSource(n)) }

// ---- stand-in scanners -------------------------------------------------------------------------------

// fakeC and fakeScan scan the same way: a token at every offset whose state byte counts the calls made on
// the payload. A difference appears where fakeScan diverges.
type fakeC struct {
	count   int
	resets  int
	parses  int
	calls   []int
	outs    []byte
	crashAt int // crash when the call number on this payload is crashAt (0: never)
	dbg     byte
	rgs     [][4]int // included ranges seen: start, end, source length, byte at start
	rgNow   *[2]int
	src     []byte
}

func (f *fakeC) SetSource(src []byte) { f.src = src }
func (f *fakeC) Reset()               { f.count = 0; f.resets++; f.dbg = 0 }
func (f *fakeC) NewParse()            { f.count = 0; f.parses++ }
func (f *fakeC) SetRange(sb, sr, sc, eb, er, ec uint32) {
	f.rgNow = &[2]int{int(sb), int(eb)}
}
func (f *fakeC) ClearRange() { f.rgNow = nil }
func (f *fakeC) Call(off int, row, col uint32, mask []bool, state []byte) scandiff.Record {
	f.count++
	f.calls = append(f.calls, off)
	f.outs = append(f.outs, byte(f.count))
	if f.rgNow != nil {
		b := 0
		if f.rgNow[0] < len(f.src) {
			b = int(f.src[f.rgNow[0]])
		}
		f.rgs = append(f.rgs, [4]int{f.rgNow[0], f.rgNow[1], len(f.src), b})
	}
	if f.crashAt != 0 && f.count == f.crashAt {
		return scandiff.Record{Crash: 11, CrashPhase: 3}
	}
	f.dbg = byte(f.count)
	return scandiff.Record{Found: true, Symbol: 1, Start: uint32(off), End: uint32(off), LookEnd: uint32(off + 1),
		State: []byte{byte(f.count)}, StateLen: 1}
}

type fakeScan struct {
	count     int
	calls     []int
	srcLens   []int
	diverge   bool // diverge at offset 2
	divergeAt int  // diverge when this is the nth call on the payload (0: never)
	pan       bool
	extra     int // state length the next scan reports (0: 1)
}

func (f *fakeScan) Scan(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	f.count++
	f.calls = append(f.calls, pos)
	f.srcLens = append(f.srcLens, len(src))
	if f.pan {
		panic("fake panic")
	}
	end := uint32(pos)
	if f.diverge && pos == 2 || f.divergeAt != 0 && f.count == f.divergeAt {
		end++
	}
	buf[0] = byte(f.count)
	out.StateLen = 1
	if f.extra != 0 {
		out.StateLen = f.extra
	}
	out.LookaheadEnd = uint32(pos + 1)
	out.Token = ccore.Token{Symbol: 1, StartByte: uint32(pos), EndByte: end}
	return true
}
func (f *fakeScan) Close() { f.count = 0 }

func mkGoSide(f *fakeScan) *goSide { return &goSide{sc: f, buf: make([]byte, sdMaxState)} }

// newFakeRunner builds a runner over stand-ins: every pair (main, damaged, reproducers, verification) has its
// own fakes. diverge makes the Go fakes differ from the C fakes at offset 2.
func newFakeRunner(t *testing.T, out string, legacyMin bool) (*sdRunner, *fakeC, *fakeScan) {
	mk := func() (*fakeC, *fakeScan) { return &fakeC{}, &fakeScan{diverge: true} }
	c1, s1 := mk()
	c2, s2 := mk()
	c3, s3 := mk()
	c4, s4 := mk()
	c5, s5 := mk()
	r := &sdRunner{t: t, lang: "fake", seed: 7, budget: 3000, batch: 100000, out: out, nExt: 3, hostileMode: "all",
		skip: map[int]bool{}, firstRepro: map[string]bool{}, rowKeys: map[string]bool{}, legacyMinMain: legacyMin}
	r.rng = newSdRand(7)
	r.main, r.hp, r.mp, r.vp, r.rp = newSide(c1, mkGoSide(s1)), newSide(c2, mkGoSide(s2)), newSide(c3, mkGoSide(s3)), newSide(c4, mkGoSide(s4)), newSide(c5, mkGoSide(s5))
	r.rows = [][]bool{{true, false, false}, {false, true, true}}
	for _, m := range r.rows {
		r.rowKeys[sdMaskBits(m)] = true
	}
	r.allTrue = []bool{true, true, true}
	r.rowKeys[sdMaskBits(r.allTrue)] = true
	r.lits = [][]byte{[]byte("ab"), []byte("if"), []byte("\xc3\xa9")}
	r.files = [][]byte{[]byte("ab if\nif ab\n  \xc3\xa9b\n")}
	r.known, _ = sdParseKnown("")
	return r, c1, s1
}

func sdHasKind(ks []int, k int) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

// ---- 1. state lengths ---------------------------------------------------------------------------------

func TestScandiffSelfLength(t *testing.T) {
	st := bytes.Repeat([]byte{7}, 1024)
	c := scandiff.Record{Found: true, Symbol: 1, State: st, StateLen: 1025}
	o := scandiff.Record{Found: true, Symbol: 1, State: st, StateLen: 1024}
	if got := sdCompare(c, o); !sdHasKind(got, kOverflow) || !sdHasKind(got, kState) {
		t.Fatalf("C reports 1025 bytes, Go 1024: want overflow and state differences, got %v", got)
	}
	if got := sdCompare(c, scandiff.Record{Found: true, Symbol: 1, State: st, StateLen: 1025}); !sdHasKind(got, kOverflow) {
		t.Fatalf("an overflow on both sides is still a difference of its own, got %v", got)
	}
	neg := scandiff.Record{Found: true, Symbol: 1, State: st, StateLen: -1}
	if got := sdCompare(neg, o); !sdHasKind(got, kOverflow) {
		t.Fatalf("a negative length is an overflow, got %v", got)
	}
	// the Go side keeps the reported length without conversion: 2^32+1024 must not become 1024
	big := 1<<32 + 1024
	gs := mkGoSide(&fakeScan{extra: big})
	src := newSdSource(1, "s", []byte("abc"))
	rec, pan := gs.call(src, 0, 0, 0, nil, nil, nil)
	if pan != "" || rec.StateLen != int64(big) || len(rec.State) != sdMaxState {
		t.Fatalf("Go length: panic %q, reported %d (want %d), kept %d bytes", pan, rec.StateLen, int64(big), len(rec.State))
	}
	cEq := scandiff.Record{Found: true, Symbol: 1, State: rec.State, StateLen: 1024}
	if got := sdCompare(cEq, rec); !sdHasKind(got, kOverflow) {
		t.Fatalf("C 1024 against Go 2^32+1024 with equal bytes must differ, got %v", got)
	}
}

// ---- 2. the Go buffer lives as long as the payload ---------------------------------------------------

type scanTwoBytes struct{ n int }

func (s *scanTwoBytes) Scan(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	s.n++
	buf[0] = 1
	if s.n == 1 {
		buf[1] = 5
	}
	out.StateLen = 2
	out.Token = ccore.Token{Symbol: 1}
	return true
}
func (s *scanTwoBytes) Close() {}

func TestScandiffSelfBufferKept(t *testing.T) {
	src := newSdSource(1, "s", []byte("abc"))
	gs := &goSide{sc: &scanTwoBytes{}, buf: make([]byte, sdMaxState)}
	gs.call(src, 0, 0, 0, nil, nil, nil)
	rec, _ := gs.call(src, 1, 0, 0, nil, nil, nil)
	if rec.State[1] != 5 {
		t.Fatalf("the byte left from the first call must show in the second state (the core keeps its buffer), got %d", rec.State[1])
	}
}

// ---- 3. a replay looks at every call -----------------------------------------------------------------

func TestScandiffSelfReplayHistory(t *testing.T) {
	r, _, _ := newFakeRunner(t, "", false)
	// the first call (offset 2) differs between the fakes; the final call (offset 0) is equal
	hist := []sdHist{{Off: 2, Mask: "100"}}
	res, err := r.replayAll(r.vp, []byte("abcdef"), 0, 0, []bool{true, false, false}, nil, hist)
	if err != nil {
		t.Fatal(err)
	}
	errs := sdReplayVerdictAll(res, false)
	if len(errs) == 0 {
		t.Fatalf("a difference in a history call must fail the replay (%d results)", len(res))
	}
	if len(sdReplayVerdictAll(res, true)) != 0 {
		t.Fatal("report mode excuses differences")
	}
	// every history entry is validated before anything runs
	for name, h := range map[string]sdHist{
		"bad state hex":   {Off: 0, Mask: "100", StateHex: "zz"},
		"mask too short":  {Off: 0, Mask: "10"},
		"mask too long":   {Off: 0, Mask: "1000"},
		"offset past end": {Off: 99, Mask: "100"},
		"range before":    {Off: 3, Mask: "100", RangeEnd: 1},
	} {
		if _, err := r.replayAll(r.vp, []byte("abcdef"), 0, 0, []bool{true, false, false}, nil, []sdHist{h}); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
	if _, err := r.replayAll(r.vp, []byte("abcdef"), 0, 0, []bool{true}, nil, nil); err == nil {
		t.Fatal("a final mask of the wrong width must be rejected, not padded")
	}
	// the helpers of the earlier tests
	a, b := scandiff.Record{Found: true, Symbol: 1}, scandiff.Record{Found: true, Symbol: 2}
	if sdReplayVerdict(a, b, "", false) == nil || sdReplayVerdict(a, a, "boom", false) == nil || sdReplayVerdict(scandiff.Record{Crash: 11}, a, "", false) == nil {
		t.Fatal("a difference, a Go panic and a C crash must fail a replay")
	}
	good := `{"lang":"python","src_hex":"6162","off":1,"mask":"010","state_hex":""}`
	if _, _, _, err := sdParseRepro([]byte(good), "python"); err != nil {
		t.Fatalf("a good reproducer: %v", err)
	}
	if _, _, _, err := sdParseRepro([]byte(good), "javascript"); err == nil {
		t.Fatal("a reproducer of another language must be rejected")
	}
}

// ---- 4. a run that compares nothing fails -------------------------------------------------------------

func TestScandiffSelfEmptyRunFails(t *testing.T) {
	r, _, _ := newFakeRunner(t, "", false)
	if len(sdGate(r, true)) == 0 || len(sdGate(r, false)) == 0 {
		t.Fatal("a run with no ordinary call compared must fail, report mode or not")
	}
	r.callsCat[catOrdinary] = 1
	if errs := sdGate(r, false); len(errs) != 0 {
		t.Fatalf("a run with a compared call and no difference must pass: %v", errs)
	}
}

// ---- crashes and skipped calls fail a gate run --------------------------------------------------------

func TestScandiffSelfCrashGate(t *testing.T) {
	r, _, _ := newFakeRunner(t, "", false)
	r.callsCat[catOrdinary] = 1
	r.cCrash = 1
	if len(sdGate(r, false)) == 0 {
		t.Fatal("a crash of the C scanner must fail a gate run")
	}
	r.cCrash, r.skipped = 0, 1
	if len(sdGate(r, false)) == 0 {
		t.Fatal("a skipped call must fail a gate run")
	}
	r.cCrash = 1
	if len(sdGate(r, true)) != 0 {
		t.Fatal("report mode excuses crashes and skips (they stay in the summary)")
	}
}

// ---- 5. after a crash both sides start again ----------------------------------------------------------

func TestScandiffSelfCrashKeepsHistoriesEqual(t *testing.T) {
	fc, fs := &fakeC{crashAt: 2}, &fakeScan{}
	p := newSide(fc, mkGoSide(fs))
	src := newSdSource(1, "s", []byte("abcdef"))
	p.call(src, 0, nil, nil, nil)
	cr, _, _ := p.call(src, 1, nil, nil, nil) // the C scanner crashes here
	if cr.Crash == 0 {
		t.Fatal("stand-in did not crash")
	}
	if len(fs.calls) != 2 || fs.calls[1] != 1 {
		t.Fatalf("the Go side must receive the call that crashed C, saw %v", fs.calls)
	}
	if fc.count != 0 || fs.count != 0 {
		t.Fatalf("both payloads must be reset after a crash: C %d, Go %d", fc.count, fs.count)
	}
	if len(p.hist) != 0 || len(p.before()) != 1 {
		t.Fatalf("the pair's history restarts, the crashed call keeps its earlier calls: hist %d, before %d", len(p.hist), len(p.before()))
	}
}

func TestScandiffSelfCrashBuffers(t *testing.T) {
	fc, fs := &fakeC{crashAt: 2}, &fakeScan{}
	p := newSide(fc, mkGoSide(fs))
	src := newSdSource(1, "s", []byte("abcdef"))
	p.call(src, 0, nil, nil, nil) // the Go scan writes buf[0] = 1, the C stand-in sets dbg = 1
	p.call(src, 1, nil, nil, nil) // C crashes: both sides are recreated
	if fc.dbg != 0 {
		t.Fatalf("the C buffer must be zero after the reset, got %d", fc.dbg)
	}
	for i, b := range p.g.buf {
		if b != 0 {
			t.Fatalf("the Go serialization buffer must be zeroed after a crash, byte %d is %d", i, b)
		}
	}
}

func TestScandiffSelfDamagedCallsHaveTheirOwnPayload(t *testing.T) {
	r, fc, _ := newFakeRunner(t, "", false)
	src := newSdSource(1, "s", []byte("abcdef"))
	r.doCall(&sdCase{src: src, off: 0, mask: r.rows[0]})
	before, resets := fc.count, fc.resets
	r.doCall(&sdCase{src: src, off: 1, mask: r.rows[0], state: []byte{1, 2, 3}, damaged: true})
	if fc.count != before || fc.resets != resets {
		t.Fatalf("a damaged-state call touched the ordinary payload (count %d -> %d, resets %d -> %d)", before, fc.count, resets, fc.resets)
	}
}

// ---- 6. reproducers do not change the run, and reproduce ----------------------------------------------

func runSummary(r *sdRunner, fc *fakeC) string {
	for r.more() {
		r.work(r.nextSource())
	}
	return fmt.Sprintf("compared=%d cat=%v diff=%v sigs=%d all=%s outs=%s", r.compared, r.callsCat, r.diffCalls, len(r.sigs), sdSha([]byte(strings.Join(r.sigs, "\n"))), sdSha(fc.outs))
}

func TestScandiffSelfSameRunWithAndWithoutOutput(t *testing.T) {
	without, c1, _ := newFakeRunner(t, "", false)
	dir := t.TempDir()
	with, c2, _ := newFakeRunner(t, dir, false)
	a, b := runSummary(without, c1), runSummary(with, c2)
	if a != b {
		t.Fatalf("output changed the measured run:\n without: %s\n with:    %s", a, b)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "repro-*.json"))
	if len(files) == 0 || len(with.repros) == 0 {
		t.Fatal("no reproducer was written, so the comparison above proves nothing")
	}
	if with.notRepro != 0 || with.reproOK == 0 {
		t.Fatalf("every written reproducer must reproduce from its file: reproducible %d, not %d", with.reproOK, with.notRepro)
	}
	legacy, c3, _ := newFakeRunner(t, t.TempDir(), true)
	if c := runSummary(legacy, c3); c == a {
		t.Fatal("legacy check broken: minimizing on the measured payloads should change this run")
	}
}

func TestScandiffSelfReproducerVerified(t *testing.T) {
	dir := t.TempDir()
	r, _, _ := newFakeRunner(t, dir, false)
	// the verifying pair does not see the difference: a reproducer that cannot be reproduced from its file
	r.vp = newSide(&fakeC{}, mkGoSide(&fakeScan{}))
	r.mp = newSide(&fakeC{}, mkGoSide(&fakeScan{diverge: true}))
	src := newSdSource(1, "s", []byte("abcdef"))
	c := &sdCase{src: src, off: 2, mask: r.rows[0]}
	cr, gr, pan := r.main.call(src, 2, c.mask, nil, nil)
	r.writeRepro(c, "ordinary-end", cr, gr, pan, r.main.before())
	if r.notRepro != 1 || r.reproOK != 0 {
		t.Fatalf("a reproducer that does not replay must be counted apart: reproducible %d, not reproducible %d", r.reproOK, r.notRepro)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "repro-fake-ordinary-end.json"))
	if !strings.Contains(string(b), `"not reproducible"`) {
		t.Fatalf("the file must say so: %s", b)
	}
}

func TestScandiffSelfReproducerHistory(t *testing.T) {
	// the Go fake diverges on the third call of a payload: the reproducer needs the two calls before it
	dir := t.TempDir()
	r, _, _ := newFakeRunner(t, dir, false)
	for _, p := range []*side{r.main, r.mp, r.vp} {
		p.g.sc = &fakeScan{divergeAt: 3}
	}
	src := newSdSource(1, "s", []byte("abcdef"))
	var last *sdCase
	var cr, gr scandiff.Record
	var pan string
	for off := 0; off < 3; off++ {
		last = &sdCase{src: src, off: off, mask: r.rows[0]}
		cr, gr, pan = r.main.call(src, off, last.mask, nil, nil)
	}
	r.writeRepro(last, "ordinary-end", cr, gr, pan, r.main.before())
	if r.reproOK != 1 || r.notRepro != 0 {
		t.Fatalf("history reproducer: reproducible %d, not %d", r.reproOK, r.notRepro)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "repro-fake-ordinary-end.json"))
	if !strings.Contains(string(b), `"history"`) || strings.Count(string(b), `"off"`) < 3 {
		t.Fatalf("the reproducer must carry the earlier calls: %s", b)
	}
}

// ---- 7. rows -----------------------------------------------------------------------------------------

func TestScandiffSelfRowsDerived(t *testing.T) {
	c := [][]bool{{false, false}, {true, false}, {true, true}}
	derivedBad := [][]bool{{false, false}, {true, false}, {true, false}}
	if n, _ := sdRowsVerdict(c, nil, derivedBad); n != 1 {
		t.Fatalf("derived rows that differ from C's table must be one mismatch when the language stores none, got %d", n)
	}
	if n, _ := sdRowsVerdict(c, nil, c); n != 0 {
		t.Fatal("equal derived rows pass")
	}
	if n, _ := sdRowsVerdict(c, [][]bool{{false, false}, {true, false}}, c); n != 1 {
		t.Fatal("a stored row missing in Go is a mismatch")
	}
	if n, _ := sdRowsVerdict(c, c, derivedBad); n != 0 {
		t.Fatal("stored rows win: the core does not use the derived ones")
	}
	r, _, _ := newFakeRunner(t, "", false)
	r.callsCat[catOrdinary] = 1
	r.rowMismatch = 1
	if len(sdGate(r, false)) == 0 {
		t.Fatal("a row mismatch must fail the gate")
	}
	// the derivation rule of internal/ccore/scanner.go extRows
	tb := &ccore.Tables{DenseLimit: 3, Dense: [][]uint16{{0, 0, 0}, {0, 1, 0}, {0, 0, 2}},
		LexModes:        []ccore.LexMode{{ExternalLexState: 1}, {ExternalLexState: 1}, {ExternalLexState: 2}},
		ExternalSymbols: []ccore.Symbol{1, 2}}
	d := sdDeriveRows(tb)
	if len(d) != 3 || sdMaskBits(d[1]) != "11" || sdMaskBits(d[2]) != "01" || sdMaskBits(d[0]) != "00" {
		t.Fatalf("derived rows: %v", d)
	}
}

// ---- 8. included ranges -------------------------------------------------------------------------------

func TestScandiffSelfRangeCalls(t *testing.T) {
	r, _, fs := newFakeRunner(t, "", false)
	fc := r.rp.c.(*fakeC)
	for r.more() && r.srcSeq < 40 {
		r.work(r.nextSource())
	}
	if r.rangeCalls == 0 || r.callsCat[catRange] == 0 {
		t.Fatalf("no call with an included range was made (range calls %d)", r.rangeCalls)
	}
	early, mid := false, false
	for _, g := range fc.rgs {
		if g[1] < g[2] {
			early = true
		}
		if g[3]&0xC0 == 0x80 {
			mid = true
		}
	}
	if !early || !mid {
		t.Fatalf("want ranges that end before the end of the source (%v) and that start inside a character (%v)", early, mid)
	}
	// the Go side gets the source cut at the end of the range
	src := newSdSource(1, "s", []byte("abcdef"))
	gs := mkGoSide(fs)
	fs.srcLens = nil
	gs.call(src, 1, 0, 1, nil, nil, &sdRange{End: 4})
	if len(fs.srcLens) != 1 || fs.srcLens[0] != 4 {
		t.Fatalf("the Go scan must see the source cut at the range end, saw %v", fs.srcLens)
	}
}

// ---- 8b. categories -----------------------------------------------------------------------------------

func TestScandiffSelfCategories(t *testing.T) {
	r, _, _ := newFakeRunner(t, "", false)
	s := newSdSource(1, "s", []byte("\xc3\xa9x"))
	if got := r.category(s, 1, r.rows[0], false, nil); got != catMidChar {
		t.Fatalf("offset 1 is inside a character: %d", got)
	}
	if got := r.category(s, 1, r.rows[0], false, &sdRange{End: 3}); got != catRange {
		t.Fatalf("a call with an included range: %d", got)
	}
	if got := r.category(s, 0, []bool{true, true, false}, false, nil); got != catMask {
		t.Fatalf("a mask that is not a table row: %d", got)
	}
	if got := r.category(s, 2, r.rows[0], false, nil); got != catOrdinary {
		t.Fatalf("a table row at a boundary: %d", got)
	}
	if got := r.category(s, 2, r.rows[0], true, nil); got != catDamaged {
		t.Fatalf("a damaged state: %d", got)
	}
	r.rng = newSdRand(3)
	for i := 0; i < 500; i++ {
		if st := r.damagedState([]byte{1, 2, 3}); len(st) == 0 {
			t.Fatal("empty damaged state")
		}
	}
}

// ---- 9. approved differences are exact discrepancies -------------------------------------------------

func TestScandiffSelfSignature(t *testing.T) {
	c := scandiff.Record{Found: true, Symbol: 1, State: []byte{1}, StateLen: 1}
	g1 := scandiff.Record{Found: true, Symbol: 1, State: []byte{2}, StateLen: 1}
	g2 := scandiff.Record{Found: true, Symbol: 1, State: []byte{3}, StateLen: 1}
	mask := []bool{true, false}
	sig1 := sdSignature("state", "ordinary", []byte("ab"), 1, 0, mask, nil, c, g1, "", nil)
	sig2 := sdSignature("state", "ordinary", []byte("ab"), 1, 0, mask, nil, c, g2, "", nil)
	if sig1 == sig2 {
		t.Fatal("a different Go result at the same input must have another signature")
	}
	sig3 := sdSignature("state", "ordinary", []byte("ab"), 1, 0, mask, nil, c, g1, "", []sdHist{{Off: 0, Mask: "10"}})
	if sig1 == sig3 {
		t.Fatal("a different earlier history must have another signature")
	}
	sig4 := sdSignature("state", "ordinary", []byte("ab"), 1, 3, mask, nil, c, g1, "", nil)
	sig5 := sdSignature("state", "ordinary", []byte("ab"), 1, 4, mask, nil, c, g1, "", nil)
	if sig4 == sig5 || sig1 == sig4 {
		t.Fatal("a different range of the call must have another signature")
	}
	k, err := sdParseKnown(sig1 + " # reviewed: Go keeps stale state\n")
	if err != nil || !k.match(sig1) {
		t.Fatalf("a signature line: %v", err)
	}
	if k.match(sig2) {
		t.Fatal("a changed result must not match the approval")
	}
	f := strings.Fields(sig1)
	mod := func(i int, v string) string {
		p := append([]string(nil), f...)
		p[i] = v
		return strings.Join(p, " ") + " # x\n"
	}
	for name, bad := range map[string]string{
		"count only":    "found 5 # x\n",
		"no reason":     sig1 + "\n",
		"empty reason":  sig1 + " #\n",
		"wildcard hash": mod(2, "*"),
		"unknown kind":  mod(0, "everything"),
		"unknown cat":   mod(1, "any"),
		"bad mask":      mod(4, "1x"),
		"duplicate":     sig1 + " # x\n" + sig1 + " # x\n",
		"short hash":    mod(5, "abcd"),
		"offset":        mod(3, "one"),
		"six fields":    strings.Join(f[:6], " ") + " # x\n",
	} {
		if _, err := sdParseKnown(bad); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
	r, _, _ := newFakeRunner(t, "", false)
	r.known, _ = sdParseKnown(sig1 + " # x\n")
	r.unmatched = append(r.unmatched, sig2)
	r.callsCat[catOrdinary] = 1
	if len(sdGate(r, false)) < 2 {
		t.Fatal("want an unmatched difference and an unused allowance")
	}
	if len(sdGate(r, true)) != 0 {
		t.Fatal("report mode excuses differences")
	}
}

// ---- 10. coverage counts ------------------------------------------------------------------------------

func TestScandiffSelfGcov(t *testing.T) {
	text := "        -:    0:Source:scanner.c\n        5:   10:  if (x)\nbranch  0 taken 5 (fallthrough)\nbranch  1 taken 0\nbranch  2 never executed\n    #####:   11:  y();\n        3:   12:  z();\n"
	f, err := sdParseGcov(text)
	if err != nil {
		t.Fatal(err)
	}
	hit := 0
	for _, v := range f.branches {
		if v {
			hit++
		}
	}
	if hit != 1 || len(f.branches) != 3 {
		t.Fatalf("want 1 of 3 branches taken, got %d of %d", hit, len(f.branches))
	}
	if f.lines[11] || !f.lines[12] || !f.lines[10] {
		t.Fatalf("line counts: %v", f.lines)
	}
	if _, err := sdParseGcov("branch  0 taken 0%\n"); err == nil {
		t.Fatal("a percentage means gcov ran without -c; it must be refused")
	}
}

// ---- 11. evidence that was asked for exists -----------------------------------------------------------

func TestScandiffSelfEvidenceWriteFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, _, _ := newFakeRunner(t, file, false)
	r.t = nil // writeRepro would call Fatalf on a real run; here the error is read
	src := newSdSource(1, "s", []byte("ab if"))
	c := &sdCase{src: src, off: 2, mask: r.rows[0]}
	cr := scandiff.Record{Found: true}
	if err := r.writeReproErr(c, "ordinary-end", cr, cr, "", nil); err == nil {
		t.Fatal("writing into a path that is not a folder must fail")
	}
	if len(r.repros) != 0 {
		t.Fatal("a reproducer that was not written must not be counted")
	}
	r.writeRepro(c, "ordinary-end", cr, cr, "", nil)
	if len(r.reproErr) == 0 {
		t.Fatal("the failed write must be recorded")
	}
	r.callsCat[catOrdinary] = 1
	if len(sdGate(r, true)) == 0 {
		t.Fatal("a failed evidence write must fail the gate, report mode or not")
	}
}

func TestScandiffSelfWatchdogWrite(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sdWriteHang(filepath.Join(file, "hang.txt"), "call=1"); err == nil {
		t.Fatal("a failed write of the hang record must be an error")
	}
	if err := sdWriteHang(filepath.Join(t.TempDir(), "hang.txt"), "call=1"); err != nil {
		t.Fatalf("a good write: %v", err)
	}
}

// ---- the mutant switches are not in a normal build ----------------------------------------------------

func TestScandiffSelfMutantsGated(t *testing.T) {
	if sdMutantsCompiled {
		t.Fatal("the mutant switches are compiled into this build; only the self-test job may build with the tag")
	}
	if sdMut("crashgate") || sdMut("len") {
		t.Fatal("a mutant is on in a normal build")
	}
}

// ---- 2. a failed reset is a failed run ----------------------------------------------------------------

type panicClose struct{ fakeScan }

func (p *panicClose) Close() { panic("destroy failed") }

func TestScandiffSelfResetFails(t *testing.T) {
	r, _, _ := newFakeRunner(t, "", false)
	r.main.g.sc = &panicClose{}
	src := newSdSource(1, "s", []byte("abc"))
	r.main.call(src, 0, r.rows[0], nil, nil) // scans correctly
	r.main.reset()
	if r.main.resetErr == nil {
		t.Fatal("a panic of the Go close must be kept as a failed reset")
	}
	r.callsCat[catOrdinary] = 1
	if len(sdGate(r, true)) == 0 || len(sdGate(r, false)) == 0 {
		t.Fatal("a failed reset must fail the run, report mode or not")
	}
	if r.more() || r.stop != "reset failure" {
		t.Fatalf("a run with a failed reset must stop (stop=%q)", r.stop)
	}
}

// ---- 3. replay applies exactly the recorded mask ------------------------------------------------------

func TestScandiffSelfReplayMask(t *testing.T) {
	names := map[string]int{"_automatic_semicolon": 0, "_template_chars": 1, "_ternary_qmark": 2}
	lookup := func(n string) (int, bool) { i, ok := names[n]; return i, ok }
	if _, err := sdReplayMask("10100000", []string{"_template_chars"}, 8, lookup); err == nil {
		t.Fatal("names that add a token to a recorded mask must be rejected, not merged")
	}
	m, err := sdReplayMask("11000000", []string{"_automatic_semicolon", "_template_chars"}, 8, lookup)
	if err != nil || sdMaskBits(m) != "11000000" {
		t.Fatalf("names equal to the recorded mask are accepted: %v %v", m, err)
	}
	if m, err := sdReplayMask("", []string{"_ternary_qmark"}, 8, lookup); err != nil || sdMaskBits(m) != "00100000" {
		t.Fatalf("names alone give the mask: %v %v", m, err)
	}
	if m, err := sdReplayMask("10100000", nil, 8, lookup); err != nil || sdMaskBits(m) != "10100000" {
		t.Fatalf("a recorded mask alone is applied as it is: %v %v", m, err)
	}
	for name, f := range map[string]func() error{
		"short mask": func() error { _, e := sdReplayMask("101", nil, 8, lookup); return e },
		"bad digits": func() error { _, e := sdReplayMask("10100002", nil, 8, lookup); return e },
		"unknown":    func() error { _, e := sdReplayMask("", []string{"nope"}, 8, lookup); return e },
		"nothing":    func() error { _, e := sdReplayMask("", nil, 8, lookup); return e },
	} {
		if f() == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

// ---- 4. range categories and pairs --------------------------------------------------------------------

func TestScandiffSelfRangeCategories(t *testing.T) {
	r, fcMain, _ := newFakeRunner(t, "", false)
	fcRange := r.rp.c.(*fakeC)
	src := newSdSource(1, "s", []byte("abcdef"))
	// a range over the whole source is the call without a range: ordinary, on the ordinary pair
	c := &sdCase{src: src, off: 0, mask: r.rows[0], rg: &sdRange{End: 6}}
	r.doCall(c)
	if r.callsCat[catOrdinary] != 1 || r.callsCat[catRange] != 0 || c.rg != nil {
		t.Fatalf("a range over the whole source must count as ordinary: ordinary %d, range %d", r.callsCat[catOrdinary], r.callsCat[catRange])
	}
	if len(fcMain.calls) != 1 {
		t.Fatalf("it runs on the ordinary pair, saw %v", fcMain.calls)
	}
	// a genuine range call runs on the range pair and leaves the ordinary one alone
	before := len(fcMain.calls)
	r.doCall(&sdCase{src: src, off: 2, mask: r.rows[0], rg: &sdRange{End: 4}})
	if r.callsCat[catRange] != 1 {
		t.Fatalf("a genuine range call counts as range: %v", r.callsCat)
	}
	if len(fcMain.calls) != before || len(fcRange.calls) != 1 {
		t.Fatalf("a range call must not touch the ordinary pair: ordinary saw %d (was %d), range pair %d", len(fcMain.calls), before, len(fcRange.calls))
	}
	// a range from a later offset to the end of the source is genuine too
	r.doCall(&sdCase{src: src, off: 2, mask: r.rows[0], rg: &sdRange{End: 6}})
	if r.callsCat[catRange] != 2 {
		t.Fatalf("a range starting after byte 0 is genuine: %v", r.callsCat)
	}
}

func TestScandiffSelfDiagnostics(t *testing.T) {
	r, _, _ := newFakeRunner(t, "", false)
	src := newSdSource(1, "s", []byte("abcdef"))
	// offset 2 differs between the fakes: a damaged-state call and a range call are diagnostics
	r.doCall(&sdCase{src: src, off: 2, mask: r.rows[0], state: []byte{1}, damaged: true})
	r.doCall(&sdCase{src: src, off: 2, mask: r.rows[0], rg: &sdRange{End: 5}})
	if len(r.unmatched) != 0 || r.diffCalls[catDamaged] != 1 || r.diffCalls[catRange] != 1 {
		t.Fatalf("diagnostics are counted and outside the gate: unmatched %d, damaged %d, range %d", len(r.unmatched), r.diffCalls[catDamaged], r.diffCalls[catRange])
	}
	r.callsCat[catOrdinary] = 1
	if errs := sdGate(r, false); len(errs) != 0 {
		t.Fatalf("diagnostics must not fail the gate: %v", errs)
	}
	r.doCall(&sdCase{src: src, off: 2, mask: r.rows[0]})
	if len(r.unmatched) != 1 || len(sdGate(r, false)) == 0 {
		t.Fatal("an ordinary difference must fail the gate")
	}
	if v, err := sdParseReport("0"); err != nil || v {
		t.Fatal(`SCANDIFF_REPORT=0 is the gate`)
	}
	if v, err := sdParseReport("1"); err != nil || !v {
		t.Fatal("SCANDIFF_REPORT=1 is report mode")
	}
	if _, err := sdParseReport("yes"); err == nil {
		t.Fatal("an unknown value must be rejected")
	}
}

// ---- 5. buffers across parses -------------------------------------------------------------------------

func TestScandiffSelfBufferAcrossParses(t *testing.T) {
	a := newSdSource(1, "a", []byte("abc"))
	b := newSdSource(2, "b", []byte("abc"))
	fc := &fakeC{}
	p := newSide(fc, &goSide{sc: &scanTwoBytes{}, buf: make([]byte, sdMaxState)})
	p.newParse(a)
	p.call(a, 0, nil, nil, nil) // the Go scan leaves byte 1 equal to 5
	p.newParse(b)               // a new source: the payload is recreated, the buffer is kept
	_, rec, _ := p.call(b, 1, nil, nil, nil)
	if rec.State[1] != 5 {
		t.Fatalf("the buffer of the earlier parse must show in the next one (the core keeps its buffer), got %d", rec.State[1])
	}
	if fc.parses < 1 {
		t.Fatal("the C side must be told that a parse ended")
	}
	// and the boundary is in the history, with its source
	if len(p.hist) < 3 || !p.hist[0].Boundary || !p.hist[2].Boundary {
		t.Fatalf("history: %+v", p.hist)
	}
}

// ---- 7. coverage with no branch ------------------------------------------------------------------------

func TestScandiffSelfZeroBranches(t *testing.T) {
	if sdPlateauAllowed(0) {
		t.Fatal("a file with no branch has no branch coverage to reach a plateau on")
	}
	if !sdPlateauAllowed(12) {
		t.Fatal("a file with branches may reach a plateau")
	}
	if sdBranchText(0, 0) != "unavailable" || sdBranchText(3, 10) != "3/10" {
		t.Fatalf("branch text: %q %q", sdBranchText(0, 0), sdBranchText(3, 10))
	}
}

//go:build cgo && treesitter_c_parity

package cgoharness

// Scanner differential test: for ONE grammar per process (SCANDIFF_LANG), call the
// compiled C external scanner (with the C runtime's own lexer, see
// internal/scandiff) and the Go port (through the same hook the core uses,
// bind.Config) side by side on generated inputs, valid-symbol masks and saved
// states, and compare every observable of each call.
//
// Switches (environment):
//   SCANDIFF_LANG     grammar name (required)
//   SCANDIFF_REQUIRE  1: a missing SCANDIFF_LANG fails instead of skipping (the wrapper sets it)
//   SCANDIFF_SEED     seed (default 1); the run is deterministic for a seed, with or without SCANDIFF_OUT
//   SCANDIFF_CALLS    call budget (default 20000; must be above 0)
//   SCANDIFF_BATCH    calls per coverage batch (default 20000)
//   SCANDIFF_OUT      folder for reproducers and logs; a failed write fails the run
//   SCANDIFF_REPORT   1 or true: report mode, never fails on differences (it still fails on a run that compares
//                     nothing and on a failed payload reset); 0, false or empty: the gate; the summary line says which
//   SCANDIFF_COVER    1: build the C scanner with --coverage, report gcov counts, stop when
//                     two batches in a row add no taken branch
//   SCANDIFF_EXTRA    folder with extra seed files, one subfolder per grammar
//   SCANDIFF_SKIP     comma list of call numbers to skip (calls that hung the process)
//   SCANDIFF_MODE     label printed in the summary (the mode itself is set by the adapter)
//   SCANDIFF_ADAPT    n: reach the Go scanner through n nested order adapters
//   SCANDIFF_HOSTILE  all (default), trunc (only shorter copies of real states) or off
//   SCANDIFF_REPLAY   path of a reproducer .json to replay instead of a run (fails on a difference)
//   SCANDIFF_DIFFLOG  path; one line per ordinary difference (kind, offset, mask, input around the offset)
//   SCANDIFF_SIGS     path; every differing (call, kind) is written there as a signature line
// Known differences: testdata/scandiff/known/<grammar>.txt, one signature per line (see sdParseKnown).

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/cgo_harness/internal/scandiff"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

var sdKinds = []string{"found", "symbol", "start", "end", "point", "lookahead", "column", "state", "gopanic", "overflow"}

const (
	kFound = iota
	kSymbol
	kStart
	kEnd
	kPoint
	kLook
	kColumn
	kState
	kPanic
	kOverflow
	nKinds
)

// Generation categories. They describe how a call was generated; they do not prove that a parse can
// produce it.
const (
	catOrdinary = iota // a row of C's table or all true, at a character boundary, with a state C produced
	catMask            // a mask that is neither a table row nor all true
	catMidChar         // an offset inside a character (only an included range could start a scan there)
	catDamaged         // a saved state that was truncated or made up
	catRange           // an included range set on the C side (the core cannot be given one)
	nCats
)

var sdCatNames = []string{"ordinary", "mask", "midchar", "damaged", "range"}

// sdDiagnostic reports whether a category is a diagnostic: its differences are counted and reported but are
// outside the gate. Damaged saved states are made up (no scanner wrote them) and genuine range calls are
// asymmetric probes (the Go lexer has no included ranges); neither is a call of the core.
func sdDiagnostic(cat int) bool { return cat == catDamaged || cat == catRange }

const sdMaxState = 1024 // TREE_SITTER_SERIALIZATION_BUFFER_SIZE; the C parser asserts it (parser.c:409)

// sdSource is one input text with its line table.
type sdSource struct {
	seq  int
	id   string
	data []byte
	ls   []int
	// bound[i] is true when byte i starts a character as the C lexer reads it
	// (a byte that is not valid UTF-8 counts as one character).
	bound []bool
}

func newSdSource(seq int, id string, data []byte) *sdSource {
	s := &sdSource{seq: seq, id: id, data: data, ls: []int{0}}
	for i, b := range data {
		if b == '\n' {
			s.ls = append(s.ls, i+1)
		}
	}
	s.bound = make([]bool, len(data)+1)
	for i := 0; i < len(data); {
		s.bound[i] = true
		_, w := utf8.DecodeRune(data[i:])
		i += w
	}
	s.bound[len(data)] = true
	return s
}

func (s *sdSource) point(off int) (uint32, uint32) {
	i := sort.Search(len(s.ls), func(i int) bool { return s.ls[i] > off }) - 1
	return uint32(i), uint32(off - s.ls[i])
}

// next returns the offset n code points after off (clamped to the end).
func (s *sdSource) next(off, n int) int {
	for ; n > 0 && off < len(s.data); n-- {
		_, w := utf8.DecodeRune(s.data[off:])
		off += w
	}
	return off
}

type sdCase struct {
	src     *sdSource
	off     int
	mask    []bool
	state   []byte
	damaged bool
	rg      *sdRange // an included range from the call's offset (nil: the whole input)
}

// sdRange is one included range that starts at the call's offset and ends at End (a byte offset).
type sdRange struct{ End int }

// sdHist is one earlier call on the same payload.
type sdHist struct {
	Off      int    `json:"off"`
	Mask     string `json:"mask"`
	StateHex string `json:"state_hex"`
	RangeEnd int    `json:"range_end,omitempty"` // 0: no included range
	// A boundary entry ends a parse and starts the next one on the source SrcHex: the payload is destroyed
	// and a fresh one made, the serialization buffers are kept (the core keeps its buffer across parses).
	Boundary bool   `json:"boundary,omitempty"`
	SrcHex   string `json:"src_hex,omitempty"`
}

// cSide is the C driver (or a stand-in in the harness's own tests).
type cSide interface {
	SetSource(src []byte)
	Call(off int, row, col uint32, mask []bool, state []byte) scandiff.Record
	SetRange(startByte, startRow, startCol, endByte, endRow, endCol uint32)
	ClearRange()
	Reset()    // fresh payload, fresh lexer and a zeroed serialization buffer
	NewParse() // fresh payload and lexer, the serialization buffer kept
}

// goSide drives the Go hook with one serialization buffer for the life of the payload,
// as the core's adapter does (internal/ccore/scanner.go allocates it once).
type goSide struct {
	sc  ccore.ExtScanner
	buf []byte
}

func newGoSide(gl *g.Language) (*goSide, error) {
	cfg, err := bind.Config(gl)
	if err != nil {
		return nil, err
	}
	if cfg.Scanner == nil {
		return nil, fmt.Errorf("language has no Go external scanner")
	}
	return &goSide{sc: cfg.Scanner, buf: make([]byte, sdMaxState)}, nil
}

// reset gives the Go side a fresh payload and a zeroed serialization buffer. A panic of the scanner's close
// is returned: the pair is then in an unknown state and the run must fail.
func (s *goSide) reset() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("closing the Go scanner payload: %v", r)
		}
	}()
	s.sc.Close()
	if !sdMut("crashbuf") {
		for i := range s.buf {
			s.buf[i] = 0
		}
	}
	return nil
}

// newParse ends a parse on the Go side: the payload is closed, the serialization buffer is kept.
func (s *goSide) newParse() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("closing the Go scanner payload: %v", r)
		}
	}()
	s.sc.Close()
	return nil
}

func (s *goSide) call(src *sdSource, off0 int, row, col uint32, mask []bool, state []byte, rg *sdRange) (rec scandiff.Record, pan string) {
	defer func() {
		if r := recover(); r != nil {
			pan = fmt.Sprint(r)
		}
	}()
	var out ccore.ExtScan
	off := off0
	// The core starts the scan after a byte order mark at byte 0, as C's ts_lexer_start
	// does (internal/ccore/scanner.go); do the same here.
	if off == 0 && len(src.data) >= 3 && src.data[0] == 0xEF && src.data[1] == 0xBB && src.data[2] == 0xBF {
		off, col = 3, col+3
	}
	data := src.data
	if rg != nil && !sdMut("ranges") && rg.End < len(data) {
		// The core has no included ranges: the end of a range is the end of the input for the scanner, so the
		// Go side gets the source cut there. The start of a range needs nothing (the scan starts at off).
		data = data[:rg.End]
	}
	found := s.sc.Scan(data, off, row, col, state, mask, s.buf, &out)
	rec.Found = found
	rec.DidGetCol = out.DidGetColumn
	rec.LookEnd = out.LookaheadEnd
	if found {
		t := out.Token
		rec.Symbol = uint32(t.Symbol)
		rec.Start, rec.End = t.StartByte, t.EndByte
		rec.StartRow, rec.StartCol, rec.EndRow, rec.EndCol = t.StartRow, t.StartCol, t.EndRow, t.EndCol
		rec.StateLen = int64(out.StateLen) // signed, unconverted
		if sdMut("len") {
			rec.StateLen = int64(uint32(out.StateLen)) // the earlier conversion: 2^32+1024 became 1024
		}
		n := out.StateLen
		if n < 0 || n > len(s.buf) {
			n = len(s.buf)
		}
		rec.State = append([]byte(nil), s.buf[:n]...)
	}
	return
}

// sdCompare returns the kinds in which two records differ.
func sdCompare(c, o scandiff.Record) []int {
	var k []int
	if c.Found != o.Found {
		k = append(k, kFound)
	} else if c.Found {
		if c.Symbol != o.Symbol {
			k = append(k, kSymbol)
		}
		if c.Start != o.Start {
			k = append(k, kStart)
		}
		if c.End != o.End {
			k = append(k, kEnd)
		}
		if c.StartRow != o.StartRow || c.StartCol != o.StartCol || c.EndRow != o.EndRow || c.EndCol != o.EndCol {
			k = append(k, kPoint)
		}
		// A length above the buffer is a difference of its own on whichever side reports it,
		// and the reported lengths take part in the state comparison.
		if c.StateLen < 0 || o.StateLen < 0 || c.StateLen > sdMaxState || o.StateLen > sdMaxState {
			k = append(k, kOverflow)
		}
		if c.StateLen != o.StateLen || !bytes.Equal(c.State, o.State) {
			k = append(k, kState)
		}
	}
	if c.LookEnd != o.LookEnd {
		k = append(k, kLook)
	}
	if c.DidGetCol != o.DidGetCol {
		k = append(k, kColumn)
	}
	return k
}

// side is a pair of payloads (one per language side) that share one history.
type side struct {
	c      cSide
	g      *goSide
	curSeq int
	hist   []sdHist // every call since the last full reset, this pair only
	// after a crash or a panic the side was reset inside call; crashBefore keeps the history of that call
	crashed     bool
	crashBefore []sdHist
	resetErr    error // the first failed reset: the pair is not used any more and the run fails
}

func newSide(c cSide, gs *goSide) *side { return &side{c: c, g: gs, curSeq: -1 << 30} }

// reset recreates both sides completely: fresh payloads, fresh lexer and equal, zeroed serialization
// buffers, and an empty history. A failed reset is kept and fails the run.
func (p *side) reset() {
	p.c.Reset()
	if err := p.g.reset(); err != nil && p.resetErr == nil && !sdMut("resetfail") {
		p.resetErr = err
	}
	p.hist = nil
	p.curSeq = -1 << 30
}

// newParse ends a parse and starts one on src, as a reused parser does: the payloads are recreated, the
// serialization buffers of both sides are kept, and the boundary goes into the history.
func (p *side) newParse(src *sdSource) {
	if sdMut("buffer") {
		p.reset() // the earlier behaviour: a new source zeroed the buffers
	} else {
		p.c.NewParse()
		if err := p.g.newParse(); err != nil && p.resetErr == nil && !sdMut("resetfail") {
			p.resetErr = err
		}
	}
	p.hist = append(p.hist, sdHist{Boundary: true, SrcHex: hex.EncodeToString(src.data)})
	p.c.SetSource(src.data)
	p.curSeq = src.seq
}

// call runs one call on both sides, with an included range when rg is set. After a crash of the C
// scanner the same call is still given to the other side and then both sides are recreated, so that they
// never keep different histories or buffers. A recovered Go panic does the same.
func (p *side) call(src *sdSource, off int, mask []bool, state []byte, rg *sdRange) (cr, gr scandiff.Record, pan string) {
	if p.curSeq != src.seq {
		p.c.SetSource(src.data)
		p.curSeq = src.seq
	}
	row, col := src.point(off)
	h := sdHist{Off: off, Mask: sdMaskBits(mask), StateHex: hex.EncodeToString(state)}
	if rg != nil {
		h.RangeEnd = rg.End
		er, ec := src.point(rg.End)
		p.c.SetRange(uint32(off), row, col, uint32(rg.End), er, ec)
	}
	p.crashed = false
	p.hist = append(p.hist, h)
	cr = p.c.Call(off, row, col, mask, state)
	if rg != nil {
		p.c.ClearRange()
	}
	gr, pan = p.g.call(src, off, row, col, mask, state, rg)
	if cr.Crash != 0 || pan != "" {
		p.crashBefore = append([]sdHist(nil), p.hist[:len(p.hist)-1]...)
		p.crashed = true
		p.reset()
	}
	return
}

// before returns the earlier calls (since the last reset) of the call that just ran.
func (p *side) before() []sdHist {
	if p.crashed {
		return p.crashBefore
	}
	if len(p.hist) == 0 {
		return nil
	}
	return p.hist[:len(p.hist)-1]
}

type sdRunner struct {
	t       *testing.T
	lang    string
	gl      *g.Language
	main    *side // ordinary run; reset at the start of every source
	hp      *side // damaged-state calls; reset before every call
	mp      *side // reproducers and minimizing; never touches the measured run
	rng     *rand.Rand
	seed    int64
	budget  int
	batch   int
	out     string
	report  bool
	skip    map[int]bool
	rows    [][]bool // rows of C's table that allow at least one token
	nExt    int
	allTrue []bool
	files   [][]byte
	lits    [][]byte
	srcSeq  int
	minSeq  int
	subset  uint64

	n                                          int // call number
	compared, skipped                          int
	cCrash                                     int
	callsCat                                   [nCats]int
	diffCalls                                  [nCats]int
	diff                                       [nCats][nKinds]int
	zeroDiff                                   int
	rowKeys                                    map[string]bool
	rowsC, rowsGo                              int
	rowMismatch                                int
	derivedRows, derivedMismatch, derivedExtra int
	hostileMode                                string
	firstRepro                                 map[string]bool
	repros                                     []string
	reproErr                                   []string
	lastFile                                   *os.File
	cover                                      *sdCover
	stop                                       string
	noGain                                     int
	sigs                                       []string
	sigFile                                    *os.File
	diffLog                                    *os.File
	diffLogN                                   int
	known                                      *sdKnown
	vp                                         *side // verifies written reproducers from their file alone
	rp                                         *side // genuine included-range calls, fresh before every call
	sinceFull                                  int   // calls since the ordinary pair got fresh buffers
	reportMode                                 bool
	reproOK, notRepro                          int
	rangeCalls                                 int
	unmatched                                  []string

	legacyMinMain bool // harness self-test only: minimize through the measured payloads, as an earlier version did
}

var sdCallStart atomic.Int64
var sdCur atomic.Pointer[sdCase]
var sdCurN atomic.Int64

func (r *sdRunner) more() bool {
	if r.resetFailure() != nil && r.stop == "" {
		r.stop = "reset failure"
	}
	return r.stop == "" && r.n < r.budget
}

// resetFailure returns the first failed reset of any pair.
func (r *sdRunner) resetFailure() error {
	for _, p := range []*side{r.main, r.hp, r.mp, r.vp, r.rp} {
		if p != nil && p.resetErr != nil {
			return p.resetErr
		}
	}
	return nil
}

// sdBufferCarryCalls is how many calls the ordinary pair keeps its serialization buffers across sources
// before they start again from zero (a bound for the history a reproducer has to carry).
const sdBufferCarryCalls = 2000

func (r *sdRunner) pickMask() []bool {
	x := r.rng.Intn(100)
	switch {
	case x < 55 && len(r.rows) > 0:
		return r.rows[r.rng.Intn(len(r.rows))]
	case x < 65:
		return r.allTrue
	case x < 85:
		m := make([]bool, r.nExt)
		for i := range m {
			m[i] = r.rng.Intn(2) == 0
		}
		return m
	default:
		m := make([]bool, r.nExt)
		m[r.rng.Intn(r.nExt)] = true
		if r.rng.Intn(2) == 0 {
			m[r.rng.Intn(r.nExt)] = true
		}
		return m
	}
}

// category classifies a call by how it was generated.
func (r *sdRunner) category(src *sdSource, off int, mask []bool, damaged bool, rg *sdRange) int {
	switch {
	case damaged:
		return catDamaged
	case rg != nil:
		return catRange
	case !src.bound[off]:
		return catMidChar
	case !r.rowKeys[sdMaskBits(mask)]:
		return catMask
	}
	return catOrdinary
}

func sdSha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:6])
}

// sdRecKey hashes one result record (or the text of a Go panic).
func sdRecKey(x scandiff.Record, pan string) string {
	return sdSha([]byte(fmt.Sprintf("%+v|%s", x, pan)))
}

// sdSignature names one difference of one call: kind, category, input hash, offset, mask, state hash, the
// hash of the C record, the hash of the Go record and the hash of the earlier calls of the payload. An
// approval covers that exact discrepancy; a different result at the same input has another signature.
func sdSignature(kind, cat string, src []byte, off, rangeEnd int, mask []bool, state []byte, cr, gr scandiff.Record, pan string, hist []sdHist) string {
	h := sha256.New()
	for _, e := range hist {
		fmt.Fprintf(h, "%d %s %s %d %v %s;", e.Off, e.Mask, e.StateHex, e.RangeEnd, e.Boundary, e.SrcHex)
	}
	hh := hex.EncodeToString(h.Sum(nil)[:6])
	if sdMut("sig") {
		return fmt.Sprintf("%s %s %s %d %s %s", kind, cat, sdSha(src), off, sdMaskBits(mask), sdSha(state))
	}
	if !sdMut("sigrange") {
		fmt.Fprintf(h, "range %d", rangeEnd)
		hh = hex.EncodeToString(h.Sum(nil)[:6])
	}
	return fmt.Sprintf("%s %s %s %d %s %s %s %s %s", kind, cat, sdSha(src), off, sdMaskBits(mask), sdSha(state), sdRecKey(cr, ""), sdRecKey(gr, pan), hh)
}

// doCall runs one case and records the result. ok is false when the call was skipped or the C scanner crashed.
func (r *sdRunner) doCall(c *sdCase) (cr scandiff.Record, ok bool) {
	r.n++
	if r.skip[r.n] {
		r.skipped++
		return cr, false
	}
	sdCur.Store(c)
	sdCurN.Store(int64(r.n))
	r.writeLast(c)
	// A range from byte 0 to the end of the source is the same call as one without a range.
	if c.rg != nil && c.off == 0 && c.rg.End == len(c.src.data) && !sdMut("fullrange") {
		c.rg = nil
	}
	pair := r.main
	switch {
	case c.damaged:
		pair = r.hp
		pair.reset() // every damaged-state call gets a fresh payload of its own
	case c.rg != nil && !sdMut("rangepair"):
		pair = r.rp
		pair.reset() // genuine range calls never touch the ordinary sequence
	}
	sdCallStart.Store(time.Now().UnixNano())
	cr, gr, pan := pair.call(c.src, c.off, c.mask, c.state, c.rg)
	sdCallStart.Store(0)
	if pair == r.main {
		r.sinceFull++
		if cr.Crash != 0 || pan != "" {
			r.sinceFull = 1 // the pair was recreated with zeroed buffers
		}
	}
	hb := pair.before() // the earlier calls of this pair's payload, since its last reset
	if cr.Crash != 0 {
		r.cCrash++
		name := "ccrash"
		if c.damaged {
			name = "damaged-ccrash"
		}
		if !r.firstRepro[name] {
			r.firstRepro[name] = true
			r.writeRepro(c, name, cr, gr, pan, hb)
		}
		return cr, false
	}
	r.compared++
	cat := r.category(c.src, c.off, c.mask, c.damaged, c.rg)
	r.callsCat[cat]++
	var kinds []int
	if pan != "" {
		kinds = []int{kPanic}
	} else {
		kinds = sdCompare(cr, gr)
	}
	if len(kinds) > 0 {
		r.diffCalls[cat]++
		if cat == catOrdinary && c.off == 0 {
			r.zeroDiff++
		}
		for _, k := range kinds {
			r.diff[cat][k]++
			sig := sdSignature(sdKinds[k], sdCatNames[cat], c.src.data, c.off, rangeEndOf(c.rg), c.mask, c.state, cr, gr, pan, hb)
			r.sigs = append(r.sigs, sig)
			if r.sigFile != nil {
				if _, err := fmt.Fprintln(r.sigFile, sig); err != nil {
					r.t.Fatalf("writing signatures: %v", err)
				}
			}
			if r.diffLog != nil && cat == catOrdinary && r.diffLogN < 20000 {
				// one line per ordinary difference: kind, offset, mask, up to 24 bytes before and 48 from the offset (hex)
				lo := c.off - 24
				if lo < 0 {
					lo = 0
				}
				hi := c.off + 48
				if hi > len(c.src.data) {
					hi = len(c.src.data)
				}
				r.diffLogN++
				if _, err := fmt.Fprintf(r.diffLog, "%s %d %d %s %x|%x\n", sdKinds[k], c.off, len(c.src.data), sdMaskBits(c.mask), c.src.data[lo:c.off], c.src.data[c.off:hi]); err != nil {
					r.t.Fatalf("writing the difference log: %v", err)
				}
			}
			if !sdDiagnostic(cat) && !r.known.match(sig) {
				r.unmatched = append(r.unmatched, sig)
			}
			name := sdCatNames[cat] + "-" + sdKinds[k]
			if !r.firstRepro[name] {
				r.firstRepro[name] = true
				r.writeRepro(c, name, cr, gr, pan, hb)
			}
		}
	}
	if r.compared%r.batch == 0 {
		r.batchEnd()
	}
	return cr, true
}

func (r *sdRunner) batchEnd() {
	line := fmt.Sprintf("SCANDIFF_BATCH lang=%s calls=%d diff_calls=%d", r.lang, r.compared, r.diffCalls[catOrdinary])
	if r.cover != nil {
		hit, tot, lines, ltot, err := r.cover.measure(r.main.c.(*scandiff.Driver))
		if err != nil {
			r.t.Fatalf("coverage measurement failed: %v", err)
		}
		line += fmt.Sprintf(" c_branches=%s c_lines=%d/%d", sdBranchText(hit, tot), lines, ltot)
		if hit > r.cover.best {
			r.cover.best, r.noGain = hit, 0
		} else if sdPlateauAllowed(tot) {
			r.noGain++
			if r.noGain >= 2 && r.compared >= 3*r.batch {
				r.stop = "coverage plateau"
			}
		}
	}
	fmt.Println(line)
}

type sdSnap struct {
	off   int
	state []byte
}

func stateAt(snaps []sdSnap, off int) []byte {
	var st []byte
	for _, s := range snaps {
		if s.off <= off {
			st = s.state
		}
	}
	return st
}

// chain walks a source from off with the given state, one scan per step.
func (r *sdRunner) chain(s *sdSource, off int, st []byte, maxSteps int, snaps *[]sdSnap) {
	for step := 0; step < maxSteps && r.more() && off <= len(s.data); step++ {
		c := &sdCase{src: s, off: off, mask: r.pickMask(), state: st}
		cr, ok := r.doCall(c)
		if off == len(s.data) {
			break
		}
		switch {
		case ok && cr.Found:
			st = cr.State
			if snaps != nil {
				*snaps = append(*snaps, sdSnap{int(cr.End), st})
			}
			if int(cr.End) > off {
				off = int(cr.End)
			} else {
				off = s.next(off, 1)
			}
		default:
			off = s.next(off, 1+r.rng.Intn(6))
		}
	}
}

func (r *sdRunner) damagedState(base []byte) []byte {
	if r.hostileMode == "trunc" { // only shorter copies of a real state: no garbage that a scanner turns into a huge allocation
		if len(base) < 2 {
			return nil
		}
		return append([]byte(nil), base[:1+r.rng.Intn(len(base)-1)]...)
	}
	switch r.rng.Intn(6) {
	case 0:
		if len(base) > 1 {
			return append([]byte(nil), base[:1+r.rng.Intn(len(base)-1)]...)
		}
		fallthrough
	case 1:
		if len(base) > 0 {
			b := append([]byte(nil), base...)
			for i := 0; i <= r.rng.Intn(3); i++ {
				b[r.rng.Intn(len(b))] = byte(r.rng.Intn(256))
			}
			return b
		}
		fallthrough
	case 2:
		n := 1 + r.rng.Intn(64)
		if r.rng.Intn(5) == 0 {
			n = 1 + r.rng.Intn(sdMaxState)
		}
		b := make([]byte, n)
		r.rng.Read(b)
		return b
	case 3:
		return bytes.Repeat([]byte{0xff}, 1+r.rng.Intn(40))
	case 4:
		return make([]byte, 1+r.rng.Intn(40))
	default:
		b := make([]byte, 1+r.rng.Intn(12))
		for i := range b {
			b[i] = byte(r.rng.Intn(4))
		}
		return b
	}
}

// work runs one source: chains, sweeps, damaged-state and subset calls. The ordinary payloads start fresh
// for every source, as a parser's do.
func (r *sdRunner) work(s *sdSource) {
	// A reused parser keeps its serialization buffer from parse to parse: the ordinary pair does too, until
	// sdBufferCarryCalls calls have passed (then both buffers start again from zero).
	if r.sinceFull == 0 || r.sinceFull > sdBufferCarryCalls {
		r.main.reset()
		r.sinceFull = 1
	}
	r.main.newParse(s)
	var snaps []sdSnap
	r.chain(s, 0, nil, 400, &snaps)
	for i := 0; i < 1+r.rng.Intn(2) && r.more(); i++ {
		off := r.rng.Intn(len(s.data) + 1)
		if r.rng.Intn(2) == 0 {
			row, _ := s.point(off)
			off = s.ls[row]
		}
		var st []byte
		if r.rng.Intn(2) == 0 {
			st = stateAt(snaps, off)
		}
		r.chain(s, off, st, 60, nil)
	}
	if len(s.data) <= 64 {
		for off := 0; off <= len(s.data) && r.more(); off++ {
			r.doCall(&sdCase{src: s, off: off, mask: r.pickMask(), state: stateAt(snaps, off)})
		}
	}
	// every row of C's table at a few positions
	for i := 0; i < 2 && r.more(); i++ {
		off := r.rng.Intn(len(s.data) + 1)
		st := stateAt(snaps, off)
		for _, m := range r.rows {
			if !r.more() {
				break
			}
			r.doCall(&sdCase{src: s, off: off, mask: m, state: st})
		}
		r.doCall(&sdCase{src: s, off: off, mask: r.allTrue, state: st})
	}
	// calls with an included range on the C side (the source cut at the range end on the Go side): the range
	// starts at the offset, which may be inside a multi-byte character, and may end before the end of the source
	if !sdMut("ranges") {
		for i := 0; i < 4 && r.more() && len(s.data) > 0; i++ {
			off := r.rng.Intn(len(s.data))
			end := len(s.data)
			if r.rng.Intn(2) == 0 {
				end = off + 1 + r.rng.Intn(len(s.data)-off)
			}
			var st []byte
			if r.rng.Intn(2) == 0 {
				st = stateAt(snaps, off)
			}
			r.rangeCalls++
			r.doCall(&sdCase{src: s, off: off, mask: r.pickMask(), state: st, rg: &sdRange{End: end}})
		}
	}
	// subsets when the scanner has at most 12 external tokens
	if r.nExt <= 12 {
		off := r.rng.Intn(len(s.data) + 1)
		st := stateAt(snaps, off)
		for i := 0; i < 48 && r.more(); i++ {
			r.subset++
			m := make([]bool, r.nExt)
			for b := 0; b < r.nExt; b++ {
				m[b] = r.subset>>uint(b)&1 == 1
			}
			r.doCall(&sdCase{src: s, off: off, mask: m, state: st})
		}
	}
	for i := 0; i < 2 && r.more() && r.hostileMode != "off"; i++ {
		off := r.rng.Intn(len(s.data) + 1)
		st := r.damagedState(stateAt(snaps, off))
		if st == nil {
			continue
		}
		r.doCall(&sdCase{src: s, off: off, mask: r.pickMask(), state: st, damaged: true})
	}
}

var sdInteresting = []string{"\x00", "\xff", "\xc3", "\xe2", "\xe2\x80\xa8", "\xc2\xa0", "\xef\xbb\xbf", "\r", "\r\n", "\t", "\f", "\v", " ", "\n", "\\", "\"", "'", "`", "$", "{", "}", "(", ")", "#", "0", "9", "\xf0\x9f", "\xed\xa0\x80"}

func (r *sdRunner) mutate(b []byte) []byte {
	b = append([]byte(nil), b...)
	for i := 0; i <= r.rng.Intn(3); i++ {
		pos := 0
		if len(b) > 0 {
			pos = r.rng.Intn(len(b) + 1)
		}
		ins := []byte(sdInteresting[r.rng.Intn(len(sdInteresting))])
		if len(r.lits) > 0 && r.rng.Intn(3) == 0 {
			ins = r.lits[r.rng.Intn(len(r.lits))]
		}
		switch r.rng.Intn(3) {
		case 0:
			b = append(b[:pos], append(append([]byte(nil), ins...), b[pos:]...)...)
		case 1:
			end := pos + 1 + r.rng.Intn(4)
			if end > len(b) {
				end = len(b)
			}
			if pos < end {
				b = append(b[:pos], b[end:]...)
			}
		default:
			if pos < len(b) {
				b[pos] = ins[0]
			}
		}
	}
	return b
}

func (r *sdRunner) window(f []byte) []byte {
	if len(f) <= 6000 {
		return f
	}
	start := r.rng.Intn(len(f) - 3000)
	if i := bytes.IndexByte(f[start:], '\n'); i >= 0 {
		start += i + 1
	}
	end := start + 600 + r.rng.Intn(4400)
	if end > len(f) {
		end = len(f)
	}
	return f[start:end]
}

func (r *sdRunner) litText() []byte {
	seps := []string{" ", "\n", "\t", "\r\n", "", "\x00", "\xff", "\xc3", " \n  ", "\n\n", "\n    ", "\n\t"}
	var b []byte
	for i := 0; i < 1+r.rng.Intn(8); i++ {
		if len(r.lits) > 0 {
			b = append(b, r.lits[r.rng.Intn(len(r.lits))]...)
		}
		b = append(b, seps[r.rng.Intn(len(seps))]...)
	}
	return b
}

func (r *sdRunner) nextSource() *sdSource {
	r.srcSeq++
	var data []byte
	id := ""
	x := r.rng.Intn(100)
	switch {
	case x < 35 && len(r.files) > 0:
		data, id = r.window(r.files[r.rng.Intn(len(r.files))]), "file"
	case x < 60 && len(r.files) > 0:
		data, id = r.mutate(r.window(r.files[r.rng.Intn(len(r.files))])), "mutant"
	case x < 90:
		data, id = r.litText(), "literals"
	default:
		data, id = r.mutate(r.litText()), "literals-mutant"
	}
	return newSdSource(r.srcSeq, fmt.Sprintf("%s#%d", id, r.srcSeq), data)
}

// writeLast records the call about to run. A failed write ends the run: evidence that was asked for must exist.
func (r *sdRunner) writeLast(c *sdCase) {
	if r.lastFile == nil {
		return
	}
	s := fmt.Sprintf("call=%d src=%s srclen=%d off=%d mask=%s state=%s damaged=%v\n", r.n, c.src.id, len(c.src.data), c.off, sdMaskBits(c.mask), hex.EncodeToString(c.state), c.damaged)
	if _, err := r.lastFile.WriteAt([]byte(s), 0); err != nil {
		r.t.Fatalf("last-call record: %v", err)
	}
	if err := r.lastFile.Truncate(int64(len(s))); err != nil {
		r.t.Fatalf("last-call record: %v", err)
	}
}

func sdMaskBits(m []bool) string {
	b := make([]byte, len(m))
	for i, v := range m {
		b[i] = '0'
		if v {
			b[i] = '1'
		}
	}
	return string(b)
}

type sdRepro struct {
	Lang       string   `json:"lang"`
	Kind       string   `json:"kind"`
	Call       int      `json:"call"`
	SrcHex     string   `json:"src_hex"`
	Off        int      `json:"off"`
	Row        uint32   `json:"row"`
	Col        uint32   `json:"col"`
	Mask       string   `json:"mask"`
	RangeEnd   int      `json:"range_end,omitempty"` // an included range from Off to here (0: none)
	ValidNames []string `json:"valid_names,omitempty"`
	StateHex   string   `json:"state_hex"`
	Damaged    bool     `json:"damaged"`
	// How the difference was reproduced on fresh payloads: "fresh" (no earlier call needed), "history"
	// (the earlier calls in History, in order, on one payload, are needed) or "not reproduced" (the
	// recorded history did not reproduce it; the full history is kept).
	Reproduced string `json:"reproduced"`
	Minimized  bool   `json:"minimized"`
	// Earlier calls on the same payload and source, oldest first.
	History []sdHist `json:"history,omitempty"`
	C       string   `json:"c"`
	Go      string   `json:"go"`
}

func (r *sdRunner) fmtRec(x scandiff.Record) string {
	if x.Crash != 0 {
		return fmt.Sprintf("CRASH signal=%d phase=%s", x.Crash, scandiff.CrashPhaseName(x.CrashPhase))
	}
	sym := ""
	if r.gl != nil && x.Found && int(x.Symbol) < len(r.gl.SymbolNames) {
		sym = r.gl.SymbolNames[x.Symbol]
	}
	return fmt.Sprintf("found=%v symbol=%d(%s) start=%d end=%d start_pt=%d,%d end_pt=%d,%d lookahead_end=%d got_column=%v state_len=%d state=%x", x.Found, x.Symbol, sym, x.Start, x.End, x.StartRow, x.StartCol, x.EndRow, x.EndCol, x.LookEnd, x.DidGetCol, x.StateLen, x.State)
}

func sdParseMask(s string) []bool {
	m := make([]bool, len(s))
	for i, ch := range s {
		m[i] = ch == '1'
	}
	return m
}

// sdRes is the result of one call of a replay.
type sdRes struct {
	C, G scandiff.Record
	Pan  string
}

// sdCheckCall validates one call of a reproducer: the offset, the range and the mask width (no padding, no
// truncation).
func (r *sdRunner) sdCheckCall(srcLen, off, rangeEnd int, mask string) error {
	if off < 0 || off > srcLen {
		return fmt.Errorf("offset %d outside the %d input bytes", off, srcLen)
	}
	if rangeEnd != 0 && (rangeEnd < off || rangeEnd > srcLen) {
		return fmt.Errorf("range end %d outside offset %d and the %d input bytes", rangeEnd, off, srcLen)
	}
	if len(mask) != r.nExt || strings.Trim(mask, "01") != "" {
		return fmt.Errorf("mask %q must be %d digits of 0 and 1", mask, r.nExt)
	}
	return nil
}

// replayAll runs the history and then the final call on pair p, whose payloads and buffers start fresh, and
// returns the result of every call. Every history entry is validated before anything runs. A boundary entry
// ends a parse and starts the next on its own source, keeping the serialization buffers.
func (r *sdRunner) replayAll(p *side, src []byte, off, rangeEnd int, mask []bool, state []byte, hist []sdHist) ([]sdRes, error) {
	type call struct {
		boundary      bool
		src           []byte
		off, rangeEnd int
		mask          []bool
		state         []byte
	}
	var calls []call
	cur := src
	for i, h := range hist {
		if h.Boundary {
			b, e := hex.DecodeString(h.SrcHex)
			if e != nil {
				return nil, fmt.Errorf("history %d: source: %v", i, e)
			}
			cur = b
			calls = append(calls, call{boundary: true, src: b})
			continue
		}
		st, e := hex.DecodeString(h.StateHex)
		if e != nil {
			return nil, fmt.Errorf("history %d: state: %v", i, e)
		}
		if e := r.sdCheckCall(len(cur), h.Off, h.RangeEnd, h.Mask); e != nil {
			return nil, fmt.Errorf("history %d: %v", i, e)
		}
		calls = append(calls, call{src: cur, off: h.Off, rangeEnd: h.RangeEnd, mask: sdParseMask(h.Mask), state: st})
	}
	if e := r.sdCheckCall(len(src), off, rangeEnd, sdMaskBits(mask)); e != nil {
		return nil, fmt.Errorf("final call: %v", e)
	}
	if !bytes.Equal(cur, src) {
		calls = append(calls, call{boundary: true, src: src})
	}
	calls = append(calls, call{src: src, off: off, rangeEnd: rangeEnd, mask: mask, state: state})
	p.reset()
	var res []sdRes
	var s *sdSource
	for _, c := range calls {
		if c.boundary || s == nil {
			r.minSeq++
			s = newSdSource(-1-r.minSeq, "replay", c.src)
			if c.boundary {
				p.newParse(s)
				continue
			}
		}
		var rg *sdRange
		if c.rangeEnd != 0 {
			rg = &sdRange{End: c.rangeEnd}
		}
		cr, gr, pan := p.call(s, c.off, c.mask, c.state, rg)
		res = append(res, sdRes{cr, gr, pan})
	}
	if err := p.resetErr; err != nil {
		return res, fmt.Errorf("replay: %v", err)
	}
	if sdMut("replayhist") {
		res = res[len(res)-1:] // the earlier behaviour: only the final call was looked at
	}
	return res, nil
}

// reproPair is the pair that reproducers and minimizing run on.
func (r *sdRunner) reproPair() *side {
	if r.legacyMinMain {
		return r.main
	}
	return r.mp
}

func rangeEndOf(rg *sdRange) int {
	if rg == nil {
		return 0
	}
	return rg.End
}

func sdRangeOf(end int) *sdRange {
	if end == 0 {
		return nil
	}
	return &sdRange{End: end}
}

func (r *sdRunner) holds(kind string, damaged bool, origZero bool, origCat int, src []byte, off, rangeEnd int, mask []bool, state []byte, hist []sdHist) bool {
	if off > len(src) {
		return false
	}
	res, err := r.replayAll(r.reproPair(), src, off, rangeEnd, mask, state, hist)
	if err != nil {
		return false
	}
	last := res[len(res)-1]
	cr, gr, pan := last.C, last.G, last.Pan
	if (off == 0) != origZero {
		return false // keep the offset-zero status: at byte 0 the C lexer reports an included-range start
	}
	s := newSdSource(-1, "cat", src)
	if r.category(s, off, mask, damaged, sdRangeOf(rangeEnd)) != origCat {
		return false
	}
	if cr.Crash != 0 {
		return strings.HasSuffix(kind, "ccrash")
	}
	var ks []int
	if pan != "" {
		ks = []int{kPanic}
	} else {
		ks = sdCompare(cr, gr)
	}
	name := kind
	if i := strings.IndexByte(kind, '-'); i >= 0 {
		name = kind[i+1:]
	}
	for _, k := range ks {
		if sdKinds[k] == name {
			return true
		}
	}
	return false
}

// minimize shrinks a case that reproduces on fresh payloads (no range). The classification (category,
// offset zero or not) is kept.
func (r *sdRunner) minimize(kind string, damaged bool, src []byte, off int, mask []bool, state []byte) ([]byte, int) {
	origZero := off == 0
	cat := r.category(newSdSource(-1, "cat", src), off, mask, damaged, nil)
	h := func(s []byte, o int) bool { return r.holds(kind, damaged, origZero, cat, s, o, 0, mask, state, nil) }
	for _, extra := range []int{0, 1, 2, 3, 4, 6, 8, 12, 16, 24, 32, 64, 128, 256, 512, 1024} {
		if off+extra >= len(src) {
			break
		}
		if h(src[:off+extra], off) {
			src = src[:off+extra]
			break
		}
	}
	ls := bytes.LastIndexByte(src[:off], '\n') + 1
	keep := off
	if off > 0 {
		keep = off - 1
		for keep > 0 && !newSdSource(-1, "b", src).bound[keep] {
			keep--
		}
	}
	for _, cut := range []int{keep, ls} {
		if cut > 0 && h(src[cut:], off-cut) {
			src, off = src[cut:], off-cut
			break
		}
	}
	for size := len(src) / 2; size >= 1; size /= 2 {
		for i := 0; i+size <= len(src); {
			cand := append(append([]byte(nil), src[:i]...), src[i+size:]...)
			noff := off
			if i < off {
				noff = off - size
				if noff < i {
					noff = i
				}
			}
			if h(cand, noff) {
				src, off = cand, noff
			} else {
				i += size
			}
		}
	}
	return src, off
}

// minimizeHist removes earlier calls that the difference does not need.
func (r *sdRunner) minimizeHist(kind string, damaged bool, src []byte, off, rangeEnd int, mask []bool, state []byte, hist []sdHist) []sdHist {
	origZero := off == 0
	cat := r.category(newSdSource(-1, "cat", src), off, mask, damaged, sdRangeOf(rangeEnd))
	for size := len(hist) / 2; size >= 1; size /= 2 {
		for i := 0; i+size <= len(hist); {
			cand := append(append([]sdHist(nil), hist[:i]...), hist[i+size:]...)
			if r.holds(kind, damaged, origZero, cat, src, off, rangeEnd, mask, state, cand) {
				hist = cand
			} else {
				i += size
			}
		}
	}
	return hist
}

// writeRepro writes the reproducer of the first difference of a kind. A failed write ends the run and the
// reproducer is counted only after it was written.
func (r *sdRunner) writeRepro(c *sdCase, kind string, cr, gr scandiff.Record, pan string, hist []sdHist) {
	if r.out == "" {
		return
	}
	if err := r.writeReproErr(c, kind, cr, gr, pan, hist); err != nil {
		r.reproErr = append(r.reproErr, err.Error())
		if r.t != nil {
			r.t.Fatalf("reproducer %s: %v", kind, err)
		}
	}
}

// sdRecText is the text of a pair of results, as stored in a reproducer.
func (r *sdRunner) sdRecText(x scandiff.Record, pan string) string {
	if pan != "" {
		return "PANIC " + pan
	}
	return r.fmtRec(x)
}

// writeReproErr writes a reproducer that holds every earlier call of the chain (hist, since the last reset of
// the pair) or none when the difference needs none, then replays it once from its bytes alone on a separate
// pair. A reproducer whose replay does not give the same two records is marked "not reproducible" and counted
// apart.
func (r *sdRunner) writeReproErr(c *sdCase, kind string, cr, gr scandiff.Record, pan string, hist []sdHist) error {
	src, off := c.src.data, c.off
	rangeEnd := 0
	if c.rg != nil {
		rangeEnd = c.rg.End
	}
	hist = append([]sdHist(nil), hist...)
	if c.damaged {
		hist = nil
	}
	reproduced := "not reproducible"
	origZero := off == 0
	cat := r.category(c.src, off, c.mask, c.damaged, c.rg)
	switch {
	case r.holds(kind, c.damaged, origZero, cat, src, off, rangeEnd, c.mask, c.state, nil):
		if rangeEnd == 0 {
			src, off = r.minimize(kind, c.damaged, src, off, c.mask, c.state)
		}
		hist, reproduced = nil, "fresh"
	case r.holds(kind, c.damaged, origZero, cat, src, off, rangeEnd, c.mask, c.state, hist):
		if len(hist) <= 200 {
			hist = r.minimizeHist(kind, c.damaged, src, off, rangeEnd, c.mask, c.state, hist)
		}
		reproduced = "history"
	}
	if res, err := r.replayAll(r.reproPair(), src, off, rangeEnd, c.mask, c.state, hist); err == nil && reproduced != "not reproducible" {
		last := res[len(res)-1]
		cr, gr, pan = last.C, last.G, last.Pan
	}
	s := newSdSource(-1, "repro", src)
	row, col := s.point(off)
	rp := sdRepro{Lang: r.lang, Kind: kind, Call: r.n, SrcHex: hex.EncodeToString(src), Off: off, Row: row, Col: col, RangeEnd: rangeEnd,
		Mask: sdMaskBits(c.mask), StateHex: hex.EncodeToString(c.state), Damaged: c.damaged, Reproduced: reproduced, Minimized: reproduced != "not reproducible",
		History: hist, C: r.fmtRec(cr), Go: r.sdRecText(gr, pan)}
	if pan != "" || cr.Crash != 0 {
		rp.C = r.sdRecText(cr, "")
	}
	// the check: replay the reproducer from the bytes that will be written, on a pair of its own
	if reproduced != "not reproducible" && !sdMut("reproverify") {
		js, _ := json.Marshal(rp)
		if !r.verifyRepro(js, rp.C, rp.Go) {
			rp.Reproduced, rp.Minimized = "not reproducible", false
			reproduced = rp.Reproduced
		}
	}
	if reproduced == "not reproducible" {
		r.notRepro++
	} else {
		r.reproOK++
	}
	name := filepath.Join(r.out, fmt.Sprintf("repro-%s-%s", r.lang, kind))
	js, _ := json.MarshalIndent(rp, "", " ")
	if err := os.WriteFile(name+".json", js, 0o644); err != nil {
		return err
	}
	var ext []string
	if r.gl != nil {
		for i, v := range c.mask {
			if v && i < len(r.gl.ExternalSymbols) {
				ext = append(ext, r.gl.SymbolNames[r.gl.ExternalSymbols[i]])
			}
		}
	}
	txt := fmt.Sprintf("reproducer: %s %s (call %d, seed %d, %s, earlier calls kept: %d, range end: %d)\ninput (%d bytes): %q\noffset: %d (row %d, column %d)\nvalid symbols: %s\nstate (%d bytes): %x\nC:  %s\nGo: %s\n",
		r.lang, kind, r.n, r.seed, reproduced, len(hist), rangeEnd, len(src), src, off, row, col, strings.Join(ext, " "), len(c.state), c.state, rp.C, rp.Go)
	if err := os.WriteFile(name+".txt", []byte(txt), 0o644); err != nil {
		return err
	}
	r.repros = append(r.repros, fmt.Sprintf("%s size=%d %s", filepath.Base(name), len(src)+len(c.state), reproduced))
	return nil
}

// verifyRepro replays a reproducer from its JSON bytes alone, on the verifying pair, and checks that the final
// call gives the stored C and Go records.
func (r *sdRunner) verifyRepro(js []byte, wantC, wantGo string) bool {
	rp, src, st, err := sdParseRepro(js, r.lang)
	if err != nil {
		return false
	}
	pair := r.vp
	if pair == nil {
		pair = r.mp
	}
	res, err := r.replayAll(pair, src, rp.Off, rp.RangeEnd, sdParseMask(rp.Mask), st, rp.History)
	if err != nil {
		return false
	}
	last := res[len(res)-1]
	gotC := r.sdRecText(last.C, "")
	return gotC == wantC && r.sdRecText(last.G, last.Pan) == wantGo
}

// sdKnown is the list of approved differences of one grammar, by signature.
type sdKnown struct {
	sigs map[string]string // signature -> reason
	used map[string]bool
}

// sdParseKnown reads an allowance file. Each non-comment line is
// `<kind> <category> <input hash> <offset> <mask> <state hash> <C record hash> <Go record hash> <history hash> # <reason>`; the kind and category must be
// known, the hashes 12 hex digits, the mask 0 and 1 only, and a reason is required. No count and no wildcard.
func sdParseKnown(text string) (*sdKnown, error) {
	k := &sdKnown{sigs: map[string]string{}, used: map[string]bool{}}
	for i, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		body, reason, ok := strings.Cut(line, "#")
		reason = strings.TrimSpace(reason)
		f := strings.Fields(body)
		bad := func(why string) (*sdKnown, error) { return nil, fmt.Errorf("line %d: %s", i+1, why) }
		if !ok || reason == "" {
			return bad("a reason after # is required")
		}
		if len(f) != 9 {
			return bad("want kind category input-hash offset mask state-hash c-hash go-hash history-hash")
		}
		if !sdIn(f[0], sdKinds) || !sdIn(f[1], sdCatNames) {
			return bad("unknown kind or category")
		}
		if _, err := strconv.Atoi(f[3]); err != nil {
			return bad("offset is not a number")
		}
		for _, h := range []string{f[2], f[5], f[6], f[7], f[8]} {
			if _, err := hex.DecodeString(h); err != nil || len(h) != 12 {
				return bad("a hash is not 12 hex digits")
			}
		}
		if strings.Trim(f[4], "01") != "" {
			return bad("mask must hold only 0 and 1")
		}
		sig := strings.Join(f, " ")
		if _, dup := k.sigs[sig]; dup {
			return bad("duplicate signature")
		}
		k.sigs[sig] = reason
	}
	return k, nil
}

func sdIn(s string, set []string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

func (k *sdKnown) match(sig string) bool {
	if k == nil {
		return false
	}
	if _, ok := k.sigs[sig]; ok {
		k.used[sig] = true
		return true
	}
	return false
}

// stale lists allowances that no call used.
func (k *sdKnown) stale() []string {
	var out []string
	if k == nil {
		return out
	}
	for s := range k.sigs {
		if !k.used[s] {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// sdGate returns the reasons a run fails. Report mode excuses differences, never an empty run or a bad write.
func sdGate(r *sdRunner, report bool) []string {
	var errs []string
	if r.callsCat[catOrdinary] <= 0 {
		errs = append(errs, fmt.Sprintf("no ordinary call was compared (compared %d)", r.compared))
	}
	if err := r.resetFailure(); err != nil {
		errs = append(errs, "a payload reset failed (report mode does not excuse it): "+err.Error())
	}
	for _, e := range r.reproErr {
		errs = append(errs, "reproducer not written: "+e)
	}
	if !report {
		// A certificate run has no crash of the C scanner and no skipped call: a crash returns before the
		// comparison, and a skipped call (after a hang) was never compared.
		if r.cCrash > 0 && !sdMut("crashgate") {
			errs = append(errs, fmt.Sprintf("%d calls crashed the C scanner", r.cCrash))
		}
		if r.skipped > 0 && !sdMut("crashgate") {
			errs = append(errs, fmt.Sprintf("%d calls were skipped", r.skipped))
		}
		if r.rowMismatch > 0 {
			errs = append(errs, fmt.Sprintf("%d external lex state rows differ between C and Go", r.rowMismatch))
		}
		for i, s := range r.unmatched {
			if i >= 20 {
				errs = append(errs, fmt.Sprintf("... %d more unmatched differences", len(r.unmatched)-20))
				break
			}
			errs = append(errs, "unmatched difference: "+s)
		}
		for _, s := range r.known.stale() {
			errs = append(errs, "allowance not used: "+s)
		}
	}
	return errs
}

// sdParseReport reads SCANDIFF_REPORT: 1 or true is report mode, 0, false or empty is the gate; anything else is an error.
func sdParseReport(v string) (bool, error) {
	switch v {
	case "", "0", "false":
		return false, nil
	case "1", "true":
		return true, nil
	}
	return false, fmt.Errorf("SCANDIFF_REPORT=%q: want 1, true, 0 or false", v)
}

// sdReplayMask builds the mask a replay applies from the recorded mask and the recorded token names. When both
// are given they must describe the same mask; they are never merged. The applied mask is compared with the
// recorded one byte for byte.
func sdReplayMask(recorded string, names []string, nExt int, lookup func(string) (int, bool)) ([]bool, error) {
	var named []bool
	if len(names) > 0 {
		named = make([]bool, nExt)
		for _, n := range names {
			i, ok := lookup(n)
			if !ok {
				return nil, fmt.Errorf("no external token %q", n)
			}
			named[i] = true
		}
	}
	switch {
	case recorded == "" && named == nil:
		return nil, fmt.Errorf("the reproducer has neither a mask nor token names")
	case recorded == "":
		return named, nil
	}
	if len(recorded) != nExt || strings.Trim(recorded, "01") != "" {
		return nil, fmt.Errorf("mask %q must be %d digits of 0 and 1", recorded, nExt)
	}
	m := sdParseMask(recorded)
	if sdMut("replaymask") && named != nil { // the earlier behaviour: the names were added to the mask
		for i, v := range named {
			m[i] = m[i] || v
		}
	}
	if named != nil && sdMaskBits(named) != recorded && !sdMut("replaymask") {
		return nil, fmt.Errorf("the token names give mask %s, the file records %s: they must agree", sdMaskBits(named), recorded)
	}
	if sdMaskBits(m) != recorded && !sdMut("replaymask") {
		return nil, fmt.Errorf("the mask to apply (%s) differs from the recorded one (%s)", sdMaskBits(m), recorded)
	}
	return m, nil
}

func sdEnvInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

func TestScannerDiff(t *testing.T) {
	lang := os.Getenv("SCANDIFF_LANG")
	if lang == "" {
		if os.Getenv("SCANDIFF_REQUIRE") != "" {
			t.Fatal("SCANDIFF_REQUIRE is set but SCANDIFF_LANG is not")
		}
		t.Skip("set SCANDIFF_LANG")
	}
	if sdMutantsCompiled {
		t.Fatal("this build has the harness's mutant switches (tag scandiff_mutants): it is for the self-tests only")
	}
	if os.Getenv("SCANDIFF_MUTANT") != "" {
		t.Fatal("SCANDIFF_MUTANT is set: mutant switches exist only in self-test builds and a measurement run refuses them")
	}
	reportMode, rerr := sdParseReport(os.Getenv("SCANDIFF_REPORT"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	budget, batch := sdEnvInt("SCANDIFF_CALLS", 20000), sdEnvInt("SCANDIFF_BATCH", 20000)
	if budget <= 0 || batch <= 0 {
		t.Fatalf("SCANDIFF_CALLS (%d) and SCANDIFF_BATCH (%d) must be above 0", budget, batch)
	}
	gl, cl := ccoreLanguages(t, lang)
	scannerType := fmt.Sprintf("%T", gl.ExternalScanner)
	goLang := gl
	if n := sdEnvInt("SCANDIFF_ADAPT", 0); n > 0 {
		// Reach the scanner through n nested order adapters (identity order), as a generated language does.
		cp := *gl
		cur := gl.ExternalScanner
		for i := 0; i < n; i++ {
			srcCopy := cp
			srcCopy.ExternalScanner = cur
			ad, ok := g.AdaptExternalScannerByExternalOrder(&srcCopy, &cp)
			if !ok {
				t.Fatalf("%s: cannot adapt the scanner", lang)
			}
			cur = ad
		}
		cp.ExternalScanner = cur
		goLang = &cp
		scannerType = fmt.Sprintf("%d x order adapter over %s", n, scannerType)
	}
	mkGo := func() *goSide {
		gs, err := newGoSide(goLang)
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		return gs
	}
	r := &sdRunner{t: t, lang: lang, gl: gl, seed: int64(sdEnvInt("SCANDIFF_SEED", 1)), budget: budget,
		batch: batch, out: os.Getenv("SCANDIFF_OUT"), report: reportMode,
		skip: map[int]bool{}, firstRepro: map[string]bool{}, rowKeys: map[string]bool{}, hostileMode: os.Getenv("SCANDIFF_HOSTILE")}
	r.rng = rand.New(rand.NewSource(r.seed))
	for _, f := range strings.Split(os.Getenv("SCANDIFF_SKIP"), ",") {
		if n, err := strconv.Atoi(f); err == nil {
			r.skip[n] = true
		}
	}
	if txt, err := os.ReadFile(filepath.Join("testdata", "scandiff", "known", lang+".txt")); err == nil {
		if r.known, err = sdParseKnown(string(txt)); err != nil {
			t.Fatalf("known differences of %s: %v", lang, err)
		}
	} else {
		r.known, _ = sdParseKnown("")
	}
	if r.out != "" {
		if err := os.MkdirAll(r.out, 0o755); err != nil {
			t.Fatalf("SCANDIFF_OUT: %v", err)
		}
		f, err := os.Create(filepath.Join(r.out, "last-call.txt"))
		if err != nil {
			t.Fatalf("SCANDIFF_OUT: %v", err)
		}
		r.lastFile = f
	}
	if p := os.Getenv("SCANDIFF_SIGS"); p != "" {
		f, err := os.Create(p)
		if err != nil {
			t.Fatalf("SCANDIFF_SIGS: %v", err)
		}
		defer f.Close()
		r.sigFile = f
	}
	if p := os.Getenv("SCANDIFF_DIFFLOG"); p != "" {
		f, err := os.Create(p)
		if err != nil {
			t.Fatalf("SCANDIFF_DIFFLOG: %v", err)
		}
		defer f.Close()
		r.diffLog = f
	}
	var cover *sdCover
	soPath, soName := "", ""
	if os.Getenv("SCANDIFF_COVER") != "" {
		var err error
		cover, err = newSdCover(t, lang, r.out)
		if err != nil {
			t.Fatalf("coverage build: %v", err)
		}
		soPath, soName = cover.so, cover.symName
		r.cover = cover
	}
	cMain, err := scandiff.New(unsafe.Pointer(cl.Inner), soPath, soName)
	if err != nil {
		t.Fatalf("%s: %v", lang, err)
	}
	defer cMain.Close()
	cHost, err := scandiff.New(unsafe.Pointer(cl.Inner), soPath, soName)
	if err != nil {
		t.Fatalf("%s: %v", lang, err)
	}
	defer cHost.Close()
	cMin, err := scandiff.New(unsafe.Pointer(cl.Inner), "", "") // reproducers use the pinned object, so they add nothing to coverage
	if err != nil {
		t.Fatalf("%s: %v", lang, err)
	}
	defer cMin.Close()
	cVer, err := scandiff.New(unsafe.Pointer(cl.Inner), "", "")
	if err != nil {
		t.Fatalf("%s: %v", lang, err)
	}
	defer cVer.Close()
	cRng, err := scandiff.New(unsafe.Pointer(cl.Inner), "", "")
	if err != nil {
		t.Fatalf("%s: %v", lang, err)
	}
	defer cRng.Close()
	r.main, r.hp, r.mp, r.vp, r.rp = newSide(cMain, mkGo()), newSide(cHost, mkGo()), newSide(cMin, mkGo()), newSide(cVer, mkGo()), newSide(cRng, mkGo())
	r.nExt = cMain.ExternalCount()
	if r.nExt != len(gl.ExternalSymbols) {
		t.Fatalf("external token count: C %d, Go %d", r.nExt, len(gl.ExternalSymbols))
	}
	// Rows: C's own table is the source of masks. The Go language's stored rows must equal it.
	r.rowsC, r.rowsGo = cMain.RowCount(), len(gl.ExternalLexStates)
	cRows := make([][]bool, r.rowsC)
	for i := range cRows {
		cRows[i] = cMain.StateRow(i)
		if i > 0 && sdAnyTrue(cRows[i]) {
			r.rows = append(r.rows, cRows[i])
			r.rowKeys[sdMaskBits(cRows[i])] = true
		}
	}
	// The rows the core derives (internal/ccore/scanner.go extRows) when the language stores none: built
	// the same way from the core's tables, and swept too.
	cfg, cerr := bind.Config(gl)
	if cerr != nil {
		t.Fatalf("%s: %v", lang, cerr)
	}
	derived := sdDeriveRows(cfg.Tables)
	r.derivedRows = len(derived)
	var rowLines []string
	r.rowMismatch, rowLines = sdRowsVerdict(cRows, gl.ExternalLexStates, derived)
	for _, l := range rowLines {
		fmt.Printf("SCANDIFF_ROWS lang=%s %s\n", lang, l)
	}
	r.derivedMismatch, _ = sdRowReport(cRows, derived)
	for i, row := range derived {
		if i > 0 && sdAnyTrue(row) && !r.rowKeys[sdMaskBits(row)] {
			r.rows = append(r.rows, row)
			r.rowKeys[sdMaskBits(row)] = true
			r.derivedExtra++
		}
	}
	fmt.Printf("SCANDIFF_ROWS lang=%s derived_rows=%d derived_mismatch=%d derived_rows_not_in_c=%d stored_rows=%d rows_used_by_core=%s\n", lang, len(derived), r.derivedMismatch, r.derivedExtra, r.rowsGo, map[bool]string{true: "stored", false: "derived"}[r.rowsGo > 0])
	r.allTrue = make([]bool, r.nExt)
	for i := range r.allTrue {
		r.allTrue[i] = true
	}
	r.rowKeys[sdMaskBits(r.allTrue)] = true
	seen := map[string]bool{}
	for _, in := range ccoreInputs(t, lang, false) {
		if !seen[string(in.Src)] && len(in.Src) > 0 {
			seen[string(in.Src)] = true
			r.files = append(r.files, in.Src)
		}
	}
	if d := os.Getenv("SCANDIFF_EXTRA"); d != "" {
		ents, _ := os.ReadDir(filepath.Join(d, lang))
		for _, e := range ents {
			if b, err := os.ReadFile(filepath.Join(d, lang, e.Name())); err == nil && !e.IsDir() && !seen[string(b)] && len(b) > 0 {
				seen[string(b)] = true
				r.files = append(r.files, b)
			}
		}
	}
	if cover != nil {
		r.lits = cover.literals
	} else if p, ok := sdScannerPath(lang); ok {
		r.lits = sdLiterals(p)
	}
	fmt.Printf("SCANDIFF_SETUP scanner=%q lang=%s externals=%d rows_c=%d rows_go=%d table_rows_used=%d files=%d literals=%d seed=%d budget=%d\n", scannerType, lang, r.nExt, r.rowsC, r.rowsGo, len(r.rows), len(r.files), len(r.lits), r.seed, r.budget)
	if rp := os.Getenv("SCANDIFF_REPLAY"); rp != "" {
		sdReplay(t, r, rp)
		return
	}
	go func() {
		for range time.Tick(time.Second) {
			if st := sdCallStart.Load(); st != 0 && time.Now().UnixNano()-st > int64(30*time.Second) {
				if cur := sdCur.Load(); cur != nil && r.out != "" {
					if err := sdWriteHang(filepath.Join(r.out, "hang.txt"), fmt.Sprintf("call=%d src=%s off=%d mask=%s state=%x\n", sdCurN.Load(), cur.src.id, cur.off, sdMaskBits(cur.mask), cur.state)); err != nil {
						fmt.Printf("SCANDIFF_HANG_RECORD_FAILED lang=%s call=%d: %v\n", lang, sdCurN.Load(), err)
						os.Exit(88)
					}
				}
				fmt.Printf("SCANDIFF_HANG lang=%s call=%d\n", lang, sdCurN.Load())
				os.Exit(87)
			}
		}
	}()
	for r.more() {
		r.work(r.nextSource())
	}
	if r.stop == "" {
		r.stop = "budget"
	}
	r.finish(t)
}

// sdDeriveRows builds the external valid-symbol rows the way internal/ccore/scanner.go extRows does when a
// language stores none: an external token is valid in an external lex state when some parse state that uses
// that lex state has an action for it, and every token is valid in parse state 0 (the error state).
func sdDeriveRows(t *ccore.Tables) [][]bool {
	maxID := 0
	for _, m := range t.LexModes {
		if int(m.ExternalLexState) > maxID {
			maxID = int(m.ExternalLexState)
		}
	}
	rows := make([][]bool, maxID+1)
	for id := range rows {
		rows[id] = make([]bool, len(t.ExternalSymbols))
	}
	for s, m := range t.LexModes {
		if m.ExternalLexState == 0 {
			continue
		}
		for i, sym := range t.ExternalSymbols {
			if s == 0 || t.ActionIndex(uint32(s), sym) != 0 {
				rows[m.ExternalLexState][i] = true
			}
		}
	}
	return rows
}

// sdWriteHang records the call that hung. A failed write is an error: the caller ends the run with a failing status.
func sdWriteHang(path, text string) error {
	err := os.WriteFile(path, []byte(text), 0o644)
	if sdMut("watchdog") {
		return nil // the earlier behaviour: the result of the write was not looked at
	}
	return err
}

func sdAnyTrue(row []bool) bool {
	for _, v := range row {
		if v {
			return true
		}
	}
	return false
}

// sdRowReport compares C's external lex state rows with a list of rows. An empty list is compared too: it
// is missing every row of C's table.
func sdRowReport(cRows, other [][]bool) (mismatch int, lines []string) {
	for i := 0; i < len(cRows) || i < len(other); i++ {
		switch {
		case i >= len(other):
			if sdAnyTrue(cRows[i]) {
				mismatch++
				lines = append(lines, fmt.Sprintf("row %d is in C's table but not in the other rows", i))
			}
		case i >= len(cRows):
			if sdAnyTrue(other[i]) {
				mismatch++
				lines = append(lines, fmt.Sprintf("row %d is in the other rows but not in C's table", i))
			}
		case sdMaskBits(cRows[i]) != sdMaskBits(other[i]):
			mismatch++
			lines = append(lines, fmt.Sprintf("row %d differs: C %s, other %s", i, sdMaskBits(cRows[i]), sdMaskBits(other[i])))
		}
	}
	return
}

// sdRowsVerdict compares the rows the core would use with C's table: the Go language's stored rows when it
// has any, else the rows the core derives from its action table (sdDeriveRows copies the rule of
// internal/ccore/scanner.go extRows, a method that the harness cannot call). A difference fails the gate.
func sdRowsVerdict(cRows, goRows, derived [][]bool) (mismatch int, lines []string) {
	if len(goRows) > 0 {
		return sdRowReport(cRows, goRows)
	}
	if sdMut("rows") {
		return 0, nil // the earlier behaviour: absent Go rows were accepted without a comparison
	}
	return sdRowReport(cRows, derived)
}

func (r *sdRunner) finish(t *testing.T) {
	cov := ""
	if r.cover != nil {
		hit, tot, lines, ltot, err := r.cover.measure(r.main.c.(*scandiff.Driver))
		if err != nil {
			t.Fatalf("coverage measurement failed: %v", err)
		}
		cov = fmt.Sprintf(" c_branches=%s c_lines=%d/%d", sdBranchText(hit, tot), lines, ltot)
		if err := r.cover.writeUncovered(); err != nil {
			t.Fatalf("coverage output: %v", err)
		}
	}
	kv := func(a [nKinds]int) string {
		var s []string
		for i, k := range sdKinds {
			s = append(s, fmt.Sprintf("%s=%d", k, a[i]))
		}
		return strings.Join(s, " ")
	}
	sort.Strings(r.repros)
	fmt.Printf("SCANDIFF lang=%s report_mode=%v mode=%s seed=%d calls=%d ordinary_calls=%d skipped=%d c_crashes=%d diff_calls=%d mask_calls=%d mask_diff_calls=%d midchar_calls=%d midchar_diff_calls=%d diag_damaged_calls=%d diag_damaged_diff_calls=%d diag_range_calls=%d diag_range_diff_calls=%d repros_reproducible=%d repros_not_reproducible=%d diff_calls_at_offset0=%d rows_c=%d rows_go=%d row_mismatch=%d derived_rows=%d derived_mismatch=%d unmatched=%d repros=%d stop=%q%s | ordinary: %s | mask: %s | midchar: %s | diag_damaged: %s | diag_range: %s\n",
		r.lang, r.report, os.Getenv("SCANDIFF_MODE"), r.seed, r.compared, r.callsCat[catOrdinary], r.skipped, r.cCrash,
		r.diffCalls[catOrdinary], r.callsCat[catMask], r.diffCalls[catMask], r.callsCat[catMidChar], r.diffCalls[catMidChar], r.callsCat[catDamaged], r.diffCalls[catDamaged], r.callsCat[catRange], r.diffCalls[catRange], r.reproOK, r.notRepro,
		r.zeroDiff, r.rowsC, r.rowsGo, r.rowMismatch, r.derivedRows, r.derivedMismatch, len(r.unmatched), len(r.repros), r.stop, cov,
		kv(r.diff[catOrdinary]), kv(r.diff[catMask]), kv(r.diff[catMidChar]), kv(r.diff[catDamaged]), kv(r.diff[catRange]))
	for _, s := range r.repros {
		fmt.Printf("SCANDIFF_REPRO %s\n", s)
	}
	ccoreCount("TestScannerDiff", r.lang, r.compared)
	for _, e := range sdGate(r, r.report) {
		t.Error(e)
	}
}

// sdParseRepro reads a reproducer and checks it belongs to the language.
func sdParseRepro(b []byte, lang string) (rp sdRepro, src, state []byte, err error) {
	if err = json.Unmarshal(b, &rp); err != nil {
		return rp, nil, nil, fmt.Errorf("reproducer: %v", err)
	}
	if rp.Lang != lang {
		return rp, nil, nil, fmt.Errorf("reproducer is for %q, the run is for %q", rp.Lang, lang)
	}
	if src, err = hex.DecodeString(rp.SrcHex); err != nil {
		return rp, nil, nil, fmt.Errorf("src_hex: %v", err)
	}
	if state, err = hex.DecodeString(rp.StateHex); err != nil {
		return rp, nil, nil, fmt.Errorf("state_hex: %v", err)
	}
	if rp.Off < 0 || rp.Off > len(src) {
		return rp, nil, nil, fmt.Errorf("offset %d outside the %d input bytes", rp.Off, len(src))
	}
	if strings.Trim(rp.Mask, "01") != "" {
		return rp, nil, nil, fmt.Errorf("mask must hold only 0 and 1")
	}
	return rp, src, state, nil
}

// sdReplayVerdict is the gate of a replay: a difference, a recovered Go panic or a recorded C crash fails it
// unless report mode is on.
func sdReplayVerdict(cr, gr scandiff.Record, pan string, report bool) error {
	if report {
		return nil
	}
	if cr.Crash != 0 {
		return fmt.Errorf("the C scanner crashed (signal %d)", cr.Crash)
	}
	if pan != "" {
		return fmt.Errorf("the Go scanner panicked: %s", pan)
	}
	if k := sdCompare(cr, gr); len(k) > 0 {
		var n []string
		for _, i := range k {
			n = append(n, sdKinds[i])
		}
		return fmt.Errorf("C and Go differ in: %s", strings.Join(n, ", "))
	}
	return nil
}

// sdReplayVerdictAll applies the replay gate to every call of a replay (history and final call) and returns
// every failure.
func sdReplayVerdictAll(res []sdRes, report bool) []error {
	var errs []error
	for i, x := range res {
		if err := sdReplayVerdict(x.C, x.G, x.Pan, report); err != nil {
			errs = append(errs, fmt.Errorf("call %d of %d: %v", i+1, len(res), err))
		}
	}
	return errs
}

func sdReplay(t *testing.T, r *sdRunner, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rp, src, st, err := sdParseRepro(b, r.lang)
	if err != nil {
		t.Fatal(err)
	}
	mask, err := sdReplayMask(rp.Mask, rp.ValidNames, r.nExt, func(n string) (int, bool) {
		for i, sym := range r.gl.ExternalSymbols {
			if strings.EqualFold(r.gl.SymbolNames[sym], n) {
				return i, true
			}
		}
		return 0, false
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.replayAll(r.vp, src, rp.Off, rp.RangeEnd, mask, st, rp.History)
	if err != nil {
		t.Fatal(err)
	}
	for i, x := range res {
		fmt.Printf("REPLAY %d C:  %s\nREPLAY %d Go: %s %s\nREPLAY %d kinds=%v\n", i+1, r.fmtRec(x.C), i+1, r.fmtRec(x.G), x.Pan, i+1, sdCompare(x.C, x.G))
	}
	for _, e := range sdReplayVerdictAll(res, r.report) {
		t.Errorf("replay: %v", e)
	}
}

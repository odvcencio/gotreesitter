package grammarruntime

import (
	"reflect"
	"testing"
	"unsafe"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// These helpers drive an external scanner port through a real ExternalLexer
// without cgo, so the tests named *_c_parity_test.go can pin what the pinned C
// scanner does (the C file and line is named in each test comment).

//go:linkname newPortParityLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newPortParityLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

//go:linkname portParityLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func portParityLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)

//go:linkname portLookaheadEnd github.com/odvcencio/gotreesitter.(*ExternalLexer).lookaheadEndByteAtCursor
func portLookaheadEnd(*gotreesitter.ExternalLexer) uint32

// cFaithfulPayload is a new payload of sc with the C-faithful mode on, when the payload has a mode:
// the tests that compare a port with its C scanner run the mode the core turns on.
func cFaithfulPayload(sc gotreesitter.ExternalScanner) any {
	p := sc.Create()
	if m, ok := p.(interface{ SetCFaithful(bool) }); ok {
		m.SetCFaithful(true)
	}
	return p
}

// portScanResult is the outcome of one Scan call. Every observation is taken from
// the lexer's fields without calling a method that records a read or a column use,
// so that taking it does not change what a later observation shows.
type portScanResult struct {
	ok        bool                // value returned by Scan
	hasTok    bool                // the lexer holds a result token
	sym       gotreesitter.Symbol // result symbol
	start     uint32              // token start byte
	end       uint32              // token end byte
	pos       int                 // byte cursor after the scan
	row       uint32              // row of the cursor
	col       uint32              // column of the cursor (bytes from the line start)
	frontier  uint32              // farthest byte the scan examined (read frontier)
	gotColumn bool                // the scan asked for the column

	// The scan's read observer (the lexer's readFrontier, installed by
	// portInstallObserver before the scan): the farthest byte examined by any read
	// during the scan, not only by the last one.
	readLookahead, readExamined uint32
	readNonlocal                bool

	// What Serialize returned after the scan and the buffer it was given (a
	// sentinel-filled buffer, so a write anywhere inside it shows).
	stateLen int
	state    string
}

// portInstallObserver gives lexer the read observer a scan owns in the parser
// (ExternalLexer.readFrontier), so that every read of the scan is recorded.
func portInstallObserver(lexer *gotreesitter.ExternalLexer) {
	f := reflect.ValueOf(lexer).Elem().FieldByName("readFrontier")
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.New(f.Type().Elem()))
}

// portObserve reads the lexer's state after a scan.
func portObserve(lexer *gotreesitter.ExternalLexer, ok bool) portScanResult {
	r := portScanResult{ok: ok}
	rv := reflect.ValueOf(lexer).Elem()
	r.pos = int(rv.FieldByName("pos").Int())
	pt := rv.FieldByName("point")
	r.row = uint32(pt.FieldByName("Row").Uint())
	r.col = uint32(pt.FieldByName("Column").Uint())
	r.gotColumn = rv.FieldByName("didGetColumn").Bool()
	r.frontier = uint32(rv.FieldByName("lookaheadEndByte").Uint())
	if f := portLookaheadEnd(lexer); f > r.frontier {
		r.frontier = f
	}
	if tok, has := portParityLexerToken(lexer); has {
		r.hasTok, r.sym, r.start, r.end = true, tok.Symbol, tok.StartByte, tok.EndByte
	}
	if obs := rv.FieldByName("readFrontier"); !obs.IsNil() {
		o := obs.Elem()
		r.readLookahead = uint32(o.FieldByName("lookahead").Uint())
		r.readExamined = uint32(o.FieldByName("examined").Uint())
		r.readNonlocal = o.FieldByName("nonlocal").Bool()
	}
	return r
}

// portScanAt runs one Scan of scanner on src starting at byte offset pos (row and
// column are those of the offset in src).
func portScanAt(scanner gotreesitter.ExternalScanner, payload any, src string, pos int, valid []bool) portScanResult {
	row, col := uint32(0), uint32(0)
	for i := 0; i < pos; i++ {
		if src[i] == '\n' {
			row, col = row+1, 0
		} else {
			col++
		}
	}
	lexer := newPortParityLexer([]byte(src), pos, row, col)
	ok := scanner.Scan(payload, lexer, valid)
	return portObserve(lexer, ok)
}

// portScan runs one Scan of scanner on src from byte 0.
func portScan(scanner gotreesitter.ExternalScanner, payload any, src string, valid []bool) portScanResult {
	return portScanAt(scanner, payload, src, 0, valid)
}

// portWantScan fails the test unless got has the wanted return value and, when
// the scan succeeds, a token with the wanted symbol and end.
func portWantScan(t *testing.T, name string, got portScanResult, ok bool, sym gotreesitter.Symbol, end uint32) {
	t.Helper()
	if got.ok != ok {
		t.Errorf("%s: Scan returned %v, want %v", name, got.ok, ok)
		return
	}
	if !ok {
		return
	}
	if !got.hasTok || got.sym != sym || got.end != end {
		t.Errorf("%s: token = {has:%v sym:%d end:%d}, want {sym:%d end:%d}", name, got.hasTok, got.sym, got.end, sym, end)
	}
}

// jsDiffRun runs one scan of a scanner that keeps no state at byte offset pos and
// returns every observation the differential test compares.
func jsDiffRun(sc gotreesitter.ExternalScanner, src []byte, pos int, row, col uint32, valid []bool) portScanResult {
	p := sc.Create()
	defer sc.Destroy(p)
	lexer := newPortParityLexer(src, pos, row, col)
	portInstallObserver(lexer)
	ok := sc.Scan(p, lexer, valid)
	r := portObserve(lexer, ok)
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = 0xA5
	}
	r.stateLen = sc.Serialize(p, buf)
	r.state = string(buf)
	return r
}

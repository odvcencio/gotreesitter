package gotreesitter

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

func tokenAfterResult(t *testing.T, l *ExternalLexer, sym Symbol) Token {
	t.Helper()
	l.SetResultSymbol(sym)
	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	return tok
}

func TestExternalLexerDefaultEndWithoutMarkEnd(t *testing.T) {
	l := newExternalLexer([]byte("abc"), 0, 0, 0)
	l.Advance(false) // consume 'a'
	l.SetResultSymbol(1)

	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("StartByte=%d want=%d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("EndByte=%d want=%d", got, want)
	}
}

func TestExternalLexerCarriesCurrentLookaheadFrontier(t *testing.T) {
	l := newExternalLexer([]byte{'a', 0xff}, 0, 0, 0)
	l.Advance(false)
	l.MarkEnd()
	l.SetResultSymbol(1)

	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	if got, want := tokenLookaheadEndByte(tok), uint32(6); got != want {
		t.Fatalf("lookahead end = %d, want invalid UTF-8 frontier %d", got, want)
	}
}

func TestExternalLexerCarriesEOFLookaheadFrontier(t *testing.T) {
	l := newExternalLexer([]byte("a"), 0, 0, 0)
	l.Advance(false)
	l.SetResultSymbol(1)

	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	if got, want := tokenLookaheadEndByte(tok), uint32(2); got != want {
		t.Fatalf("lookahead end = %d, want EOF frontier %d", got, want)
	}
}

func TestExternalLexerAdvanceSpacesSkip(t *testing.T) {
	l := newExternalLexer([]byte("   x"), 0, 2, 4)
	if got := l.AdvanceSpaces(true); got != 3 {
		t.Fatalf("AdvanceSpaces consumed %d, want 3", got)
	}
	if l.Lookahead() != 'x' {
		t.Fatalf("Lookahead after AdvanceSpaces = %q, want x", l.Lookahead())
	}
	tok := tokenAfterResult(t, l, 7)
	if tok.StartByte != 3 || tok.EndByte != 3 {
		t.Fatalf("token range = %d..%d, want 3..3", tok.StartByte, tok.EndByte)
	}
	if tok.StartPoint.Row != 2 || tok.StartPoint.Column != 7 {
		t.Fatalf("token start point = %d:%d, want 2:7", tok.StartPoint.Row, tok.StartPoint.Column)
	}
}

func TestExternalLexerAdvanceUntilNewlineSkip(t *testing.T) {
	l := newExternalLexer([]byte("abcπ\nx"), 0, 1, 2)
	if got := l.AdvanceUntilNewline(true); got != len("abcπ") {
		t.Fatalf("AdvanceUntilNewline consumed %d, want %d", got, len("abcπ"))
	}
	if l.Lookahead() != '\n' {
		t.Fatalf("Lookahead after AdvanceUntilNewline = %q, want newline", l.Lookahead())
	}
	tok := tokenAfterResult(t, l, 11)
	if tok.StartByte != uint32(len("abcπ")) || tok.EndByte != uint32(len("abcπ")) {
		t.Fatalf("token range = %d..%d, want %d..%d", tok.StartByte, tok.EndByte, len("abcπ"), len("abcπ"))
	}
	if tok.StartPoint.Row != 1 || tok.StartPoint.Column != uint32(2+len("abcπ")) {
		t.Fatalf("token start point = %d:%d, want 1:%d", tok.StartPoint.Row, tok.StartPoint.Column, 2+len("abcπ"))
	}
}

func TestExternalLexerAdvanceLineContentMatchesRunePath(t *testing.T) {
	for _, source := range []string{"abcé😀\r\nnext", "\xef\xbb\xbfαé\xfftail", "aé\x00b", "é😀", "\xff", ""} {
		for _, skip := range []bool{false, true} {
			span := newExternalLexer([]byte(source), 0, 0, 0)
			step := newExternalLexer([]byte(source), 0, 0, 0)
			for _, lexer := range []*ExternalLexer{span, step} {
				lexer.readFrontier = &externalReadFrontier{}
				lexer.Column()
				lexer.MarkEnd()
			}

			n, count, last := span.AdvanceLineContent(skip)
			var wantBytes, wantCount int
			var wantLast rune
			for {
				ch := step.Lookahead()
				if ch == 0 || ch == '\r' || ch == '\n' {
					break
				}
				_, width := utf8.DecodeRune(step.source[step.pos:])
				if ch == utf8.RuneError && width == 1 {
					break
				}
				wantBytes += width
				wantCount++
				wantLast = ch
				step.Advance(skip)
			}

			if n != wantBytes || count != wantCount || last != wantLast {
				t.Fatalf("source %q skip=%t: span (%d, %d, %U), rune path (%d, %d, %U)",
					source, skip, n, count, last, wantBytes, wantCount, wantLast)
			}
			if !reflect.DeepEqual(span, step) {
				t.Fatalf("source %q skip=%t: lexer state differs from the rune path", source, skip)
			}
		}
	}
}

func TestExternalLexerMarkEndFreezesSpan(t *testing.T) {
	l := newExternalLexer([]byte("abc"), 0, 0, 0)
	l.Advance(false) // consume 'a'
	l.MarkEnd()      // end at 1
	l.Advance(false) // look ahead through 'b'
	l.SetResultSymbol(1)

	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("StartByte=%d want=%d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("EndByte=%d want=%d", got, want)
	}
}

func TestExternalLexerMarkBeforeSkipZeroWidth(t *testing.T) {
	l := newExternalLexer([]byte(" abc"), 0, 0, 0)
	l.MarkEnd()     // mark at 0
	l.Advance(true) // skip leading space
	l.SetResultSymbol(1)

	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("StartByte=%d want=%d", got, want)
	}
	if got, want := tok.EndByte, uint32(0); got != want {
		t.Fatalf("EndByte=%d want=%d", got, want)
	}
}

func TestExternalLexerSkipOnlyWithoutMarkEndUsesCurrentCursor(t *testing.T) {
	l := newExternalLexer([]byte("\n    }"), 0, 0, 0)
	l.Advance(true)
	l.Advance(true)
	l.Advance(true)
	l.Advance(true)
	l.Advance(true)
	l.SetResultSymbol(1)

	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	if got, want := tok.StartByte, uint32(5); got != want {
		t.Fatalf("StartByte=%d want=%d", got, want)
	}
	if got, want := tok.EndByte, uint32(5); got != want {
		t.Fatalf("EndByte=%d want=%d", got, want)
	}
	if got, want := tok.StartPoint, (Point{Row: 1, Column: 4}); got != want {
		t.Fatalf("StartPoint=%+v want=%+v", got, want)
	}
	if got, want := tok.EndPoint, (Point{Row: 1, Column: 4}); got != want {
		t.Fatalf("EndPoint=%+v want=%+v", got, want)
	}
}

// TestExternalLexerColumnCountsCodePointsButTokenPointStaysBytes pins the
// split contract: Column() counts code points since the start of the line,
// exactly like C's ts_lexer__get_column, while token StartPoint/EndPoint
// columns remain byte offsets (see commit c3701e452).
func TestExternalLexerColumnCountsCodePointsButTokenPointStaysBytes(t *testing.T) {
	l := newExternalLexer([]byte("x✗z"), 0, 0, 0)

	l.Advance(false) // x
	if got, want := l.Column(), uint32(1); got != want {
		t.Fatalf("column after x = %d want %d", got, want)
	}

	l.Advance(false) // ✗ (3 bytes, 1 code point)
	if got, want := l.Column(), uint32(2); got != want {
		t.Fatalf("column after utf8 rune = %d want %d", got, want)
	}

	l.MarkEnd()
	l.SetResultSymbol(1)
	tok, ok := l.token()
	if !ok {
		t.Fatal("token() returned !ok")
	}
	if got, want := tok.EndByte, uint32(4); got != want {
		t.Fatalf("EndByte=%d want=%d", got, want)
	}
	// Token points stay byte-based: 'x' (1 byte) + '✗' (3 bytes) = 4.
	if got, want := tok.EndPoint.Column, uint32(4); got != want {
		t.Fatalf("EndPoint.Column=%d want=%d", got, want)
	}
}

// TestExternalLexerColumnMultibyteRuneBeforeCursor confirms a multibyte rune
// earlier on the same line counts as one code point, not its byte width.
func TestExternalLexerColumnMultibyteRuneBeforeCursor(t *testing.T) {
	l := newExternalLexer([]byte("é1234z"), 0, 0, 0)
	l.Advance(false) // é (2 bytes)
	l.Advance(false) // 1
	l.Advance(false) // 2
	l.Advance(false) // 3
	l.Advance(false) // 4
	if got, want := l.Column(), uint32(5); got != want {
		t.Fatalf("Column() = %d, want %d (5 code points before z)", got, want)
	}
}

// TestExternalLexerColumnSkipsLeadingBOM confirms a byte order mark at byte
// 0 does not count toward Column(), matching C's is_bom check.
func TestExternalLexerColumnSkipsLeadingBOM(t *testing.T) {
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte("ab")...)
	l := newExternalLexer(src, 0, 0, 0)
	l.Advance(false) // BOM, does not count
	if got, want := l.Column(), uint32(0); got != want {
		t.Fatalf("Column() after BOM = %d, want %d", got, want)
	}
	l.Advance(false) // a
	if got, want := l.Column(), uint32(1); got != want {
		t.Fatalf("Column() after BOM+a = %d, want %d", got, want)
	}
	l.Advance(false) // b
	if got, want := l.Column(), uint32(2); got != want {
		t.Fatalf("Column() after BOM+ab = %d, want %d", got, want)
	}
}

// TestExternalLexerColumnResetsOnNewline confirms Column() drops back to 0
// on the character immediately after a newline.
func TestExternalLexerColumnResetsOnNewline(t *testing.T) {
	l := newExternalLexer([]byte("é1\nz"), 0, 0, 0)
	l.Advance(false) // é
	l.Advance(false) // 1
	if got, want := l.Column(), uint32(2); got != want {
		t.Fatalf("Column() before newline = %d, want %d", got, want)
	}
	l.Advance(false) // \n
	if got, want := l.Column(), uint32(0); got != want {
		t.Fatalf("Column() right after newline = %d, want %d", got, want)
	}
	l.Advance(false) // z
	if got, want := l.Column(), uint32(1); got != want {
		t.Fatalf("Column() after z on new line = %d, want %d", got, want)
	}
}

// TestExternalLexerColumnAdvanceUntilNewlineOverMultibyte confirms the bulk
// AdvanceUntilNewline helper keeps Column() consistent with per-rune Advance
// when the skipped run contains multibyte code points.
func TestExternalLexerColumnAdvanceUntilNewlineOverMultibyte(t *testing.T) {
	l := newExternalLexer([]byte("héllo wörld\nnext"), 0, 0, 0)
	if got := l.Column(); got != 0 {
		t.Fatalf("Column() at start = %d, want 0", got)
	}
	n := l.AdvanceUntilNewline(false)
	wantBytes := len("héllo wörld")
	if n != wantBytes {
		t.Fatalf("AdvanceUntilNewline consumed %d bytes, want %d", n, wantBytes)
	}
	wantCols := uint32(utf8.RuneCountInString("héllo wörld"))
	if got := l.Column(); got != wantCols {
		t.Fatalf("Column() after AdvanceUntilNewline = %d, want %d code points", got, wantCols)
	}
}

// TestExternalLexerColumnLazyRecomputeAfterResetWithMultibytePrefix pins the
// lazy-recompute path: a reset lands mid-line (col > 0, byte-based) with
// multibyte runes before pos, and Column() must count code points of that
// prefix, not raw bytes.
func TestExternalLexerColumnLazyRecomputeAfterResetWithMultibytePrefix(t *testing.T) {
	src := []byte("日本語abc")
	prefix := "日本語ab" // 3 multibyte runes (3 bytes each) + 2 ASCII bytes
	pos := len(prefix)
	l := &ExternalLexer{}
	l.reset(src, pos, 0, uint32(pos)) // col is the byte offset of pos on line 0
	if got, want := l.Column(), uint32(utf8.RuneCountInString(prefix)); got != want {
		t.Fatalf("Column() after reset mid-line = %d, want %d code points", got, want)
	}
}

// TestExternalLexerColumnRepeatedCallsStayCorrect exercises a cobol-style
// loop that polls Column() many times per line without advancing, then
// advances a rune, and checks the cached value tracks correctly throughout.
func TestExternalLexerColumnRepeatedCallsStayCorrect(t *testing.T) {
	l := newExternalLexer([]byte("      é123456789012345678901234567890123456789012345678901234567890123456"), 0, 0, 0)
	for i := 0; i < 6; i++ {
		l.Advance(false)
		for j := 0; j < 5; j++ {
			if got, want := l.Column(), uint32(i+1); got != want {
				t.Fatalf("iteration %d.%d: Column() = %d, want %d", i, j, got, want)
			}
		}
	}
	l.Advance(false) // é (multibyte, one code point)
	for j := 0; j < 5; j++ {
		if got, want := l.Column(), uint32(7); got != want {
			t.Fatalf("after é iteration %d: Column() = %d, want %d", j, got, want)
		}
	}
}

func TestExternalLexerResetClearsScannerState(t *testing.T) {
	l := &ExternalLexer{}
	l.reset([]byte("abc"), 0, 0, 0)
	l.Advance(false)
	l.MarkEnd()
	l.SetResultSymbol(7)

	l.reset([]byte("z"), 0, 0, 0)
	if l.hasResult {
		t.Fatal("expected reset lexer to clear prior result")
	}
	if l.endMarked {
		t.Fatal("expected reset lexer to clear end mark")
	}
	tok, ok := l.token()
	if ok {
		t.Fatalf("token() on reset lexer unexpectedly succeeded: %+v", tok)
	}
	l.Advance(false)
	l.SetResultSymbol(1)
	tok, ok = l.token()
	if !ok {
		t.Fatal("token() on reset lexer returned !ok")
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("EndByte=%d want=%d", got, want)
	}
}

func TestExternalLexerPreviousAtStartOfSource(t *testing.T) {
	l := newExternalLexer([]byte("abc"), 0, 0, 0)
	if got := l.Previous(); got != 0 {
		t.Fatalf("Previous() at pos 0 = %q, want 0", got)
	}
}

func TestExternalLexerPreviousAtEndOfSource(t *testing.T) {
	src := []byte("abc")
	l := newExternalLexer(src, len(src), 0, uint32(len(src)))
	if got, want := l.Previous(), rune('c'); got != want {
		t.Fatalf("Previous() at pos len(source) = %q, want %q", got, want)
	}
}

func TestExternalLexerPreviousMultiByteRune(t *testing.T) {
	src := []byte("x✗z")
	// ✗ (U+2717) is 3 bytes; place pos right after it, before 'z'.
	pos := 1 + len("✗")
	l := newExternalLexer(src, pos, 0, uint32(pos))
	if got, want := l.Previous(), rune('✗'); got != want {
		t.Fatalf("Previous() after multi-byte rune = %q, want %q", got, want)
	}
}

func TestExternalLexerPreviousInvalidUTF8(t *testing.T) {
	// 0xFF is never valid as a UTF-8 continuation or lead byte.
	src := []byte{'a', 0xFF}
	l := newExternalLexer(src, len(src), 0, uint32(len(src)))
	if got, want := l.Previous(), rune(utf8.RuneError); got != want {
		t.Fatalf("Previous() after invalid UTF-8 byte = %q, want utf8.RuneError (%q)", got, want)
	}
	if got := l.Previous(); got == 0 {
		t.Fatalf("Previous() after invalid UTF-8 byte returned 0, doc promises utf8.RuneError not 0")
	}
}

func TestExternalLexerHasPreviousBytes(t *testing.T) {
	lexer := newExternalLexer([]byte("abc///doc"), 6, 0, 6)

	cases := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty", text: "", want: true},
		{name: "exact preceding bytes", text: "///", want: true},
		{name: "longer suffix", text: "bc///", want: true},
		{name: "wrong bytes", text: "//!", want: false},
		{name: "past start", text: "zabc///", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lexer.HasPreviousBytes(tc.text); got != tc.want {
				t.Fatalf("HasPreviousBytes(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// --- scanner mode: probes, delivery, wrappers, pass-through ---

// TestScanmodeProbes pins the three questions the public lexer cannot answer:
// a NUL byte is not the end of the input; an invalid UTF-8 byte is -1 for C where
// Lookahead returns U+FFFD (a literal U+FFFD stays 0xFFFD); the cursor is at an
// included range's start only at byte 0 of a non-empty parse over the whole input
// (C's lexer has no range start for an empty source: lexer.c:97, :356).
func TestScanmodeProbes(t *testing.T) {
	l := newExternalLexer([]byte("\x00\xffa"), 0, 0, 0)
	if l.Lookahead() != 0 || scanmode.AtEOF(l) {
		t.Fatal("a NUL byte must read as lookahead 0 and not as the end")
	}
	if !scanmode.AtRangeStart(l) {
		t.Fatal("byte 0 is the start of the range")
	}
	l.Advance(false)
	if scanmode.AtRangeStart(l) {
		t.Fatal("byte 1 is not the start of the range")
	}
	if l.Lookahead() != utf8.RuneError || scanmode.CLookahead(l) != -1 {
		t.Fatalf("invalid byte: Lookahead %U, CLookahead %d, want U+FFFD and -1", l.Lookahead(), scanmode.CLookahead(l))
	}
	l.Advance(false)
	l.Advance(false)
	if l.Lookahead() != 0 || !scanmode.AtEOF(l) {
		t.Fatal("past the last byte the lexer must be at the end")
	}
	if got := scanmode.CLookahead(newExternalLexer([]byte("\xef\xbf\xbd"), 0, 0, 0)); got != 0xFFFD {
		t.Fatalf("literal U+FFFD: CLookahead %d, want 65533", got)
	}
	empty := newExternalLexer(nil, 0, 0, 0)
	if scanmode.AtRangeStart(empty) || !scanmode.AtEOF(empty) {
		t.Fatal("an empty source is at its end and has no range start")
	}
}

// TestScanmodeFlagPayload: the payload of a port without scanner state reads as
// off until the mode is turned on; a typed-nil flag neither panics nor takes it.
func TestScanmodeFlagPayload(t *testing.T) {
	var nilFlag *scanmode.Flag
	if nilFlag.On() || scanmode.Of(nil) || scanmode.Of(&scanmode.Flag{}) {
		t.Fatal("the mode must read as off for a nil flag, a missing payload and a fresh flag")
	}
	f := &scanmode.Flag{}
	var m scanmode.CFaithful = f
	m.SetCFaithful(true)
	if !f.On() || !scanmode.Of(f) {
		t.Fatal("SetCFaithful(true) did not turn the mode on")
	}
	nilFlag.SetCFaithful(true) // a typed-nil flag must not panic
}

func init() {
	scanmode.RegisterScanner(modeProbeScanner{}, (*modeProbe)(nil), true)
	scanmode.RegisterScanner(emittingScanner{}, nil, false)
	scanmode.RegisterScanner(pooledScanner{}, (*modeProbe)(nil), true)
	scanmode.RegisterScanner(panicScanner{}, (*modeProbe)(nil), true)
	scanmode.RegisterScanner(lyingScanner{}, (*modeProbe)(nil), true)
	scanmode.RegisterScanner(panickyEntryScanner{}, (*panickyProbe)(nil), true)
	scanmode.RegisterScanner(recordingScanner{}, nil, false)
	scanmode.RegisterScanner(clearsThenPanicsScanner{}, (*offPanicProbe)(nil), true)
	scanmode.RegisterScanner(panicsBeforeClearingScanner{}, (*offPanicProbe)(nil), true)
	scanmode.RegisterScanner(noModeScanner{}, (*countingProbe)(nil), false)
	scanmode.RegisterScanner(lyingCountingScanner{}, (*modeProbe)(nil), true)
	scanmode.RegisterScanner(destroyPanicScanner{}, (*modeProbe)(nil), true)
	scanmode.RegisterScanner((*pointerScanner)(nil), nil, false)
}

// modeProbe is a scanner payload that records whether the mode is on.
type modeProbe struct{ on, set bool }

func (p *modeProbe) SetCFaithful(on bool) { p.on, p.set = on, on || p.set }

// modeProbeScanner is a registered scanner with a behavioural mode whose payload
// is a *modeProbe.
type modeProbeScanner struct{ payload *modeProbe }

func (s modeProbeScanner) Create() any {
	if s.payload == nil {
		return (*modeProbe)(nil)
	}
	return s.payload
}
func (modeProbeScanner) Destroy(any)                           {}
func (modeProbeScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (modeProbeScanner) Serialize(any, []byte) int             { return 0 }
func (modeProbeScanner) Deserialize(any, []byte)               {}

// scanThroughCore binds sc as the core does and runs one scan with the valid mask.
func scanThroughCore(t *testing.T, sc ExternalScanner, valid []bool) (*ccoreExtScanner, ccore.ExtScan, bool) {
	t.Helper()
	a, err := newCcoreExtScanner(sc)
	if err != nil {
		t.Fatalf("binding failed: %v", err)
	}
	var out ccore.ExtScan
	ok := a.scan([]byte("x"), 0, 0, 0, nil, valid, make([]byte, 16), &out)
	return a, out, ok
}

// identityOrderAdapter wraps inner in an order adapter with an identity mapping
// over one external whose result symbol is 5.
func identityOrderAdapter(inner ExternalScanner) *externalScannerOrderAdapter {
	return &externalScannerOrderAdapter{
		inner:                inner,
		targetToSource:       []int{0},
		sourceCount:          1,
		sourceSymbolToIndex:  map[Symbol]int{5: 0},
		sourceResultToTarget: map[Symbol]Symbol{5: 5},
	}
}

// forwardingWrapper embeds a scanner: it forwards the five scanner methods and
// hides everything else the wrapped value has. The core must not serve it.
type forwardingWrapper struct{ ExternalScanner }

// lyingScanner is registered with payload type *modeProbe but its Create returns
// a payload of another type that carries a mode of its own and holds the
// payload that should have been returned.
type lyingScanner struct{}

type opaqueModePayload struct {
	scanmode.Flag
	inner any
}

func (lyingScanner) Create() any                           { return &opaqueModePayload{inner: &modeProbe{}} }
func (lyingScanner) Destroy(any)                           {}
func (lyingScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (lyingScanner) Serialize(any, []byte) int             { return 0 }
func (lyingScanner) Deserialize(any, []byte)               {}

// unregisteredScanner is not on the list.
type unregisteredScanner struct{ modeProbeScanner }

// TestCcoreAdapterTurnsCFaithfulModeOn: the mode is on while the core uses a
// payload and off after release.
func TestCcoreAdapterTurnsCFaithfulModeOn(t *testing.T) {
	p := &modeProbe{}
	a, _, _ := scanThroughCore(t, modeProbeScanner{payload: p}, []bool{true})
	if !p.on {
		t.Fatalf("payload mode on=%v while the core uses it, want true", p.on)
	}
	a.close()
	if p.on {
		t.Fatal("the mode is still on after the core released the payload")
	}
}

// TestModeReachesInnermostPayloadThroughOrderAdapters: zero to three order adapters.
func TestModeReachesInnermostPayloadThroughOrderAdapters(t *testing.T) {
	for layers := 0; layers <= 3; layers++ {
		p := &modeProbe{}
		var sc ExternalScanner = modeProbeScanner{payload: p}
		for i := 0; i < layers; i++ {
			sc = identityOrderAdapter(sc)
		}
		a, _, _ := scanThroughCore(t, sc, []bool{true})
		if !p.on {
			t.Errorf("%d layers: the mode did not reach the innermost payload", layers)
		}
		a.close()
		if p.on {
			t.Errorf("%d layers: the mode is on after close", layers)
		}
	}
}

// TestBindingFailsForScannersAndPayloadsThatAreNotKnown: nothing is discovered. A
// scanner that is not registered, a struct that embeds a registered scanner and
// forwards only the scanner methods, a wrapper of another type around an adapter,
// and a registered scanner whose payload is not the registered type (even one that
// carries a mode of its own) all fail binding with a named reason.
func TestBindingFailsForScannersAndPayloadsThatAreNotKnown(t *testing.T) {
	probe := func() ExternalScanner { return modeProbeScanner{payload: &modeProbe{}} }
	cases := map[string]struct {
		sc   ExternalScanner
		want string
	}{
		"unregistered scanner":         {unregisteredScanner{}, errScannerNotRegistered},
		"embedding wrapper":            {forwardingWrapper{probe()}, errScannerNotRegistered},
		"order(embedding wrapper)":     {identityOrderAdapter(forwardingWrapper{probe()}), errScannerNotRegistered},
		"embedding wrapper(order)":     {forwardingWrapper{identityOrderAdapter(probe())}, errScannerNotRegistered},
		"nil adapter inner":            {&externalScannerOrderAdapter{}, errScannerNil},
		"payload of another type":      {lyingScanner{}, errPayloadNotRegistered},
		"order(payload another type)":  {identityOrderAdapter(lyingScanner{}), errPayloadNotRegistered},
		"needs a mode, payload is nil": {modeProbeScanner{}, errScanModeNotDelivered},
	}
	for name, c := range cases {
		if _, err := newCcoreExtScanner(c.sc); err == nil || err.Error() != c.want {
			t.Errorf("%s: bind error %v, want %q", name, err, c.want)
		}
	}
	if _, err := ccoreConfig(&Language{ExternalScanner: unregisteredScanner{}}); err == nil {
		t.Error("ccoreConfig must fail for an unknown scanner")
	}
}

// TestRunPathPanicsWithTheNamedReasonWhenTheChainIsNotKnown: even if a scanner got
// past the bind check, the core adapter does not run it: the first scan panics with
// the named reason.
func TestRunPathPanicsWithTheNamedReasonWhenTheChainIsNotKnown(t *testing.T) {
	a := &ccoreExtScanner{sc: lyingScanner{}, reg: scanmode.Registered{Payload: reflect.TypeOf((*modeProbe)(nil)), NeedsMode: true}, layers: 0}
	defer func() {
		if r := recover(); r != errPayloadNotRegistered {
			t.Fatalf("recovered %v, want the named reason", r)
		}
	}()
	a.scan([]byte("x"), 0, 0, 0, nil, []bool{true}, make([]byte, 16), new(ccore.ExtScan))
	t.Fatal("scan returned")
}

// pooledScanner keeps a free list of payloads, as a scanner might: Destroy
// publishes the payload, Create takes it back.
type pooledScanner struct {
	pool *[]*modeProbe
	// onInDestroy, when set, records the payload's mode as Destroy starts, before
	// the payload is published.
	onInDestroy *[]bool
}

func (s pooledScanner) Create() any {
	if n := len(*s.pool); n > 0 {
		p := (*s.pool)[n-1]
		*s.pool = (*s.pool)[:n-1]
		return p
	}
	return &modeProbe{}
}
func (s pooledScanner) Destroy(p any) {
	if s.onInDestroy != nil {
		*s.onInDestroy = append(*s.onInDestroy, p.(*modeProbe).on)
	}
	*s.pool = append(*s.pool, p.(*modeProbe))
}
func (pooledScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (pooledScanner) Serialize(any, []byte) int             { return 0 }
func (pooledScanner) Deserialize(any, []byte)               {}

// panicScanner panics in Scan.
type panicScanner struct{ pooledScanner }

func (panicScanner) Scan(any, *ExternalLexer, []bool) bool { panic("scanner failure") }

// TestPayloadUsedByTheCoreHasTheModeOffForTheDefaultEngine: after a core parse ends
// by close or by a panic in the scanner, the payload that the scanner pooled and the
// default engine then obtains has the mode off.
func TestPayloadUsedByTheCoreHasTheModeOffForTheDefaultEngine(t *testing.T) {
	var pool []*modeProbe
	var onInDestroy []bool
	base := pooledScanner{pool: &pool, onInDestroy: &onInDestroy}
	check := func(name string) {
		t.Helper()
		if len(pool) != 1 {
			t.Fatalf("%s: %d payloads pooled, want 1", name, len(pool))
		}
		// The mode must already be off when Destroy publishes the payload.
		if len(onInDestroy) == 0 {
			t.Fatalf("%s: Destroy was not reached", name)
		}
		for _, on := range onInDestroy {
			if on {
				t.Fatalf("%s: the mode was on inside Destroy (before the payload was published): %v", name, onInDestroy)
			}
		}
		if got := base.Create().(*modeProbe); got.on {
			t.Errorf("%s: the default engine received a payload with the mode on", name)
		}
	}
	a, _, _ := scanThroughCore(t, identityOrderAdapter(base), []bool{true})
	a.close()
	check("close")
	func() {
		defer func() { _ = recover() }()
		a, err := newCcoreExtScanner(identityOrderAdapter(panicScanner{base}))
		if err != nil {
			t.Fatal(err)
		}
		a.scan([]byte("x"), 0, 0, 0, nil, []bool{true}, make([]byte, 16), new(ccore.ExtScan))
	}()
	check("panic")
}

// panickyProbe sets its flag and then panics when the mode is turned on.
type panickyProbe struct{ on bool }

func (p *panickyProbe) SetCFaithful(on bool) {
	p.on = on
	if on {
		panic("mode entry failure")
	}
}

// panickyEntryScanner pools its payload and records destruction.
type panickyEntryScanner struct {
	payload   *panickyProbe
	destroyed *int
}

func (s panickyEntryScanner) Create() any                         { return s.payload }
func (s panickyEntryScanner) Destroy(any)                         { *s.destroyed++ }
func (panickyEntryScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (panickyEntryScanner) Serialize(any, []byte) int             { return 0 }
func (panickyEntryScanner) Deserialize(any, []byte)               {}

// TestPanicDuringModeEntryLeavesFlagsOffAndDestroysThePayload: at bind time and in
// the scan hook, a panic while the mode is turned on, alone or behind adapters,
// leaves the flag off and the payload destroyed.
func TestPanicDuringModeEntryLeavesFlagsOffAndDestroysThePayload(t *testing.T) {
	for _, layers := range []int{0, 2} {
		build := func(p *panickyProbe, destroyed *int) ExternalScanner {
			var sc ExternalScanner = panickyEntryScanner{payload: p, destroyed: destroyed}
			for i := 0; i < layers; i++ {
				sc = identityOrderAdapter(sc)
			}
			return sc
		}
		// bind time
		p, destroyed := &panickyProbe{}, new(int)
		func() {
			defer func() { _ = recover() }()
			newCcoreExtScanner(build(p, destroyed))
		}()
		if p.on || *destroyed != 1 {
			t.Errorf("%d layers, bind: flag on=%v destroyed=%d, want off and 1", layers, p.on, *destroyed)
		}
		// scan hook
		p, destroyed = &panickyProbe{}, new(int)
		a := &ccoreExtScanner{sc: build(p, destroyed), reg: scanmode.Registered{Payload: reflect.TypeOf((*panickyProbe)(nil)), NeedsMode: true}, layers: layers}
		func() {
			defer func() { _ = recover() }()
			a.scan([]byte("x"), 0, 0, 0, nil, []bool{true}, make([]byte, 16), new(ccore.ExtScan))
		}()
		if p.on || *destroyed != 1 {
			t.Errorf("%d layers, scan: flag on=%v destroyed=%d, want off and 1", layers, p.on, *destroyed)
		}
	}
}

// emittingScanner has one behaviour, no mode and no payload: it returns result
// symbol sym whatever the valid-symbol mask says, as a C scanner may.
type emittingScanner struct{ sym Symbol }

func (emittingScanner) Create() any               { return nil }
func (emittingScanner) Destroy(any)               {}
func (emittingScanner) Serialize(any, []byte) int { return 0 }
func (emittingScanner) Deserialize(any, []byte)   {}
func (s emittingScanner) Scan(_ any, l *ExternalLexer, _ []bool) bool {
	l.SetResultSymbol(s.sym)
	l.MarkEnd()
	return true
}

// mappedAdapter has two source externals (symbols 5 and 6) and one target
// external (symbol 9): source 5 maps to target 9; source 6 has no target and keeps
// only the identity entry that the adapter adds for unmatched externals.
func mappedAdapter(inner ExternalScanner) *externalScannerOrderAdapter {
	return &externalScannerOrderAdapter{
		inner:                inner,
		targetToSource:       []int{0},
		sourceCount:          2,
		sourceSymbolToIndex:  map[Symbol]int{5: 0, 6: 1},
		sourceResultToTarget: map[Symbol]Symbol{5: 9, 6: 6},
	}
}

// identityOverMapped wraps a mappedAdapter in a second layer that maps the first
// layer's target external (symbol 9) to itself.
func identityOverMapped(inner ExternalScanner) *externalScannerOrderAdapter {
	return &externalScannerOrderAdapter{
		inner:                inner,
		targetToSource:       []int{0},
		sourceCount:          1,
		sourceSymbolToIndex:  map[Symbol]int{9: 0},
		sourceResultToTarget: map[Symbol]Symbol{9: 9},
	}
}

// TestOrderAdapterPassThroughDependsOnlyOnTheCoreBeingTheCaller: a scanner with one
// behaviour and no mode (nothing to turn on) returns a symbol while the validity
// mask is false or true. The default engine keeps a result only when its bit was
// true. The core keeps a mapped result whatever the bit and drops a source symbol
// with no target external; through one and two layers.
func TestOrderAdapterPassThroughDependsOnlyOnTheCoreBeingTheCaller(t *testing.T) {
	for _, layers := range []int{1, 2} {
		build := func(sym Symbol) ExternalScanner {
			var sc ExternalScanner = mappedAdapter(emittingScanner{sym: sym})
			if layers == 2 {
				sc = identityOverMapped(sc)
			}
			return sc
		}
		for _, bit := range []bool{false, true} {
			for _, sym := range []Symbol{5, 6} {
				sc := build(sym)
				got := sc.Scan(sc.Create(), newExternalLexer([]byte("x"), 0, 0, 0), []bool{bit})
				if want := sym == 5 && bit; got != want {
					t.Errorf("%d layers, symbol %d, bit %v: default engine accepted=%v, want %v", layers, sym, bit, got, want)
				}
			}
			a, out, ok := scanThroughCore(t, build(5), []bool{bit})
			if !ok || out.Token.Symbol != 9 {
				t.Errorf("%d layers, bit %v: the core lost a mapped result (ok=%v symbol %d, want 9)", layers, bit, ok, out.Token.Symbol)
			}
			a.close()
			a, _, ok = scanThroughCore(t, build(6), []bool{bit})
			if ok {
				t.Errorf("%d layers, bit %v: the core accepted a source symbol with no target mapping", layers, bit)
			}
			a.close()
		}
	}
}

// TestOrderAdapterDefaultPathKeepsTheValidityRule: for a payload the core never
// marked, the adapter accepts a result exactly when the symbol is mapped and its
// validity bit was true, for every combination.
func TestOrderAdapterDefaultPathKeepsTheValidityRule(t *testing.T) {
	for _, sym := range []Symbol{5, 6, 7} {
		for _, bit := range []bool{false, true} {
			sc := mappedAdapter(emittingScanner{sym: sym})
			_, exists := sc.sourceResultToTarget[sym]
			idx, known := sc.sourceSymbolToIndex[sym]
			want := exists && known && idx == 0 && bit
			got := sc.Scan(sc.Create(), newExternalLexer([]byte("x"), 0, 0, 0), []bool{bit})
			if got != want {
				t.Errorf("symbol %d, validity bit %v: accepted=%v, want %v", sym, bit, got, want)
			}
		}
	}
}

// recordingScanner counts lifecycle calls and keeps the bytes it was given.
type recordingScanner struct {
	nilPayload bool
	log        *[]string
	saved      *[]byte
}

func (s recordingScanner) Create() any {
	*s.log = append(*s.log, "create")
	return nil
}
func (s recordingScanner) Destroy(any) { *s.log = append(*s.log, "destroy") }
func (s recordingScanner) Serialize(_ any, buf []byte) int {
	*s.log = append(*s.log, "serialize")
	return copy(buf, []byte{1, 2, 3})
}
func (s recordingScanner) Deserialize(_ any, buf []byte) {
	*s.log = append(*s.log, "deserialize")
	*s.saved = append([]byte(nil), buf...)
}
func (recordingScanner) Scan(any, *ExternalLexer, []bool) bool { return false }

// TestWrappedLifecycleCallsReachTheInnerScannerOnce: Serialize, Deserialize and
// Destroy through three layers each reach the innermost scanner exactly once with
// the right bytes.
func TestWrappedLifecycleCallsReachTheInnerScannerOnce(t *testing.T) {
	var log []string
	var saved []byte
	sc := ExternalScanner(identityOrderAdapter(identityOrderAdapter(identityOrderAdapter(recordingScanner{log: &log, saved: &saved}))))
	p := sc.Create()
	sc.Deserialize(p, []byte{7, 8})
	buf := make([]byte, 8)
	n := sc.Serialize(p, buf)
	sc.Destroy(p)
	if !bytes.Equal(buf[:n], []byte{1, 2, 3}) || !bytes.Equal(saved, []byte{7, 8}) {
		t.Errorf("saved %v, restored %v", buf[:n], saved)
	}
	if want := []string{"create", "deserialize", "serialize", "destroy"}; !reflect.DeepEqual(log, want) {
		t.Errorf("inner calls %v, want %v", log, want)
	}
}

// offPanicProbe takes the mode and panics when it is switched off. The flag is
// cleared first in one scanner (clearsThenPanicsScanner) and left on in the other
// (panicsBeforeClearingScanner), as a defective port might.
type offPanicProbe struct {
	on        bool
	clearsOff bool
	offCalls  int
}

func (p *offPanicProbe) SetCFaithful(on bool) {
	if on {
		p.on = true
		return
	}
	p.offCalls++
	if p.clearsOff {
		p.on = false
	}
	panic("mode exit failure")
}

type clearsThenPanicsScanner struct {
	payload   *offPanicProbe
	destroyed *int
	calls     *int // Create, Deserialize and Scan calls
}

func (s clearsThenPanicsScanner) Create() any {
	*s.calls++
	s.payload.clearsOff = true
	return s.payload
}
func (s clearsThenPanicsScanner) Destroy(any)                           { *s.destroyed++ }
func (s clearsThenPanicsScanner) Scan(any, *ExternalLexer, []bool) bool { *s.calls++; return false }
func (clearsThenPanicsScanner) Serialize(any, []byte) int               { return 0 }
func (s clearsThenPanicsScanner) Deserialize(any, []byte)               { *s.calls++ }

type panicsBeforeClearingScanner struct {
	payload   *offPanicProbe
	destroyed *int
	calls     *int
}

func (s panicsBeforeClearingScanner) Create() any                           { *s.calls++; return s.payload }
func (s panicsBeforeClearingScanner) Destroy(any)                           { *s.destroyed++ }
func (s panicsBeforeClearingScanner) Scan(any, *ExternalLexer, []bool) bool { *s.calls++; return false }
func (panicsBeforeClearingScanner) Serialize(any, []byte) int               { return 0 }
func (s panicsBeforeClearingScanner) Deserialize(any, []byte)               { *s.calls++ }

// TestPanicWhileSwitchingTheModeOffEndsCleanupAndStillDestroys: a setter that panics
// when the mode goes off (after clearing its flag, or before) must not make cleanup
// retry it: the walk ends, Destroy is reached once, the setter is called once, and
// the failure is reported with its named reason, at bind time and when the core
// releases a payload it used.
func TestPanicWhileSwitchingTheModeOffEndsCleanupAndStillDestroys(t *testing.T) {
	builders := map[string]func(p *offPanicProbe, destroyed, calls *int) ExternalScanner{
		"clears then panics":     func(p *offPanicProbe, d, c *int) ExternalScanner { return clearsThenPanicsScanner{p, d, c} },
		"panics before clearing": func(p *offPanicProbe, d, c *int) ExternalScanner { return panicsBeforeClearingScanner{p, d, c} },
	}
	within := func(name string, f func()) {
		t.Helper()
		done := make(chan struct{})
		go func() { defer close(done); f() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: cleanup did not end", name)
		}
	}
	for name, build := range builders {
		for _, layers := range []int{0, 2} {
			wrap := func(sc ExternalScanner) ExternalScanner {
				for i := 0; i < layers; i++ {
					sc = identityOrderAdapter(sc)
				}
				return sc
			}
			// bind time
			p, destroyed, calls := &offPanicProbe{}, new(int), new(int)
			var err error
			within(name+" bind", func() { _, err = newCcoreExtScanner(wrap(build(p, destroyed, calls))) })
			if err == nil || err.Error() != errModeNotReset {
				t.Errorf("%s, %d layers, bind: error %v, want %q", name, layers, err, errModeNotReset)
			}
			if p.offCalls != 1 || *destroyed != 1 {
				t.Errorf("%s, %d layers, bind: setter calls %d, Destroy calls %d, want 1 and 1", name, layers, p.offCalls, *destroyed)
			}
			// release of a payload the core used
			p, destroyed, calls = &offPanicProbe{}, new(int), new(int)
			a := &ccoreExtScanner{sc: wrap(build(p, destroyed, calls)), reg: scanmode.Registered{Payload: reflect.TypeOf((*offPanicProbe)(nil)), NeedsMode: true}, layers: layers}
			a.scan([]byte("x"), 0, 0, 0, nil, []bool{true}, make([]byte, 16), new(ccore.ExtScan))
			var got any
			within(name+" close", func() {
				defer func() { got = recover() }()
				a.close()
			})
			if got != errModeNotReset {
				t.Errorf("%s, %d layers, close: recovered %v, want %q", name, layers, got, errModeNotReset)
			}
			within(name+" second close", a.close)
			// The hook is ended: a caller that recovered the panic and scans again
			// reaches no callback of the scanner.
			before := *calls
			var again any
			within(name+" scan after failed reset", func() {
				defer func() { again = recover() }()
				a.scan([]byte("x"), 0, 0, 0, nil, []bool{true}, make([]byte, 16), new(ccore.ExtScan))
			})
			if again != errModeNotReset {
				t.Errorf("%s, %d layers: scan after a failed reset recovered %v, want %q", name, layers, again, errModeNotReset)
			}
			if *calls != before || *destroyed != 1 || p.offCalls != 1 {
				t.Errorf("%s, %d layers: scan after a failed reset reached the scanner: calls %d -> %d, Destroy %d, setter %d", name, layers, before, *calls, *destroyed, p.offCalls)
			}
			if p.offCalls != 1 || *destroyed != 1 {
				t.Errorf("%s, %d layers, close: setter calls %d, Destroy calls %d, want 1 and 1", name, layers, p.offCalls, *destroyed)
			}
		}
	}
}

// countingProbe counts SetCFaithful calls of any value.
type countingProbe struct{ calls int }

func (p *countingProbe) SetCFaithful(bool) { p.calls++ }

// noModeScanner is registered without a behavioural mode, but its payload happens
// to have a SetCFaithful method.
type noModeScanner struct{ payload *countingProbe }

func (s noModeScanner) Create() any                         { return s.payload }
func (noModeScanner) Destroy(any)                           {}
func (noModeScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (noModeScanner) Serialize(any, []byte) int             { return 0 }
func (noModeScanner) Deserialize(any, []byte)               {}

// lyingCountingScanner is registered with payload type *modeProbe and a mode, but
// its Create returns a *countingProbe.
type lyingCountingScanner struct{ payload *countingProbe }

func (s lyingCountingScanner) Create() any                         { return s.payload }
func (lyingCountingScanner) Destroy(any)                           {}
func (lyingCountingScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (lyingCountingScanner) Serialize(any, []byte) int             { return 0 }
func (lyingCountingScanner) Deserialize(any, []byte)               {}

// TestCleanupNeverCallsAnOptionalModeMethod: the mode method of a payload is called
// only on the exact registered payload type of a registration that declares the
// mode. A scanner registered without a mode never receives it, and a payload that
// validation rejected receives nothing, at bind time and at close.
func TestCleanupNeverCallsAnOptionalModeMethod(t *testing.T) {
	for _, layers := range []int{0, 2} {
		wrap := func(sc ExternalScanner) ExternalScanner {
			for i := 0; i < layers; i++ {
				sc = identityOrderAdapter(sc)
			}
			return sc
		}
		p := &countingProbe{}
		a, err := newCcoreExtScanner(wrap(noModeScanner{payload: p}))
		if err != nil {
			t.Fatalf("%d layers: a scanner without a mode must bind: %v", layers, err)
		}
		a.scan([]byte("x"), 0, 0, 0, nil, []bool{true}, make([]byte, 16), new(ccore.ExtScan))
		a.close()
		if p.calls != 0 {
			t.Errorf("%d layers: the mode method of a scanner registered without a mode was called %d times", layers, p.calls)
		}
		q := &countingProbe{}
		if _, err := newCcoreExtScanner(wrap(lyingCountingScanner{payload: q})); err == nil || err.Error() != errPayloadNotRegistered {
			t.Errorf("%d layers: bind error %v, want %q", layers, err, errPayloadNotRegistered)
		}
		if q.calls != 0 {
			t.Errorf("%d layers: a payload rejected by validation received %d mode calls", layers, q.calls)
		}
	}
}

// TestRegisterScannerRejectsDuplicatesAndLateRegistrations: a registration never
// changes: registering a type twice panics and leaves the first registration, and
// so does registering after binding has started.
func TestRegisterScannerRejectsDuplicatesAndLateRegistrations(t *testing.T) {
	panics := func(f func()) (r any) {
		defer func() { r = recover() }()
		f()
		return nil
	}
	before, ok := scanmode.Lookup(modeProbeScanner{})
	if !ok || !before.NeedsMode {
		t.Fatal("the test scanner is not registered with a mode")
	}
	if r := panics(func() { scanmode.RegisterScanner(modeProbeScanner{}, (*modeProbe)(nil), false) }); r == nil || !strings.Contains(r.(string), "twice") {
		t.Errorf("a duplicate registration: recovered %v, want the duplicate panic", r)
	}
	if after, _ := scanmode.Lookup(modeProbeScanner{}); after != before {
		t.Errorf("a rejected duplicate changed the registration: %+v, was %+v", after, before)
	}
	// Binding starts: from here no registration is accepted.
	if _, err := newCcoreExtScanner(modeProbeScanner{payload: &modeProbe{}}); err != nil {
		t.Fatal(err)
	}
	type lateScanner struct{ modeProbeScanner }
	if r := panics(func() { scanmode.RegisterScanner(lateScanner{}, nil, false) }); r == nil || !strings.Contains(r.(string), "after binding started") {
		t.Errorf("a late registration: recovered %v, want the late panic", r)
	}
	if _, ok := scanmode.Lookup(lateScanner{}); ok {
		t.Error("a registration after binding started took effect")
	}
}

// pointerScanner has pointer methods that do not dereference the receiver; Create
// counts its calls.
type pointerScanner struct{}

var pointerScannerCreates int

func (*pointerScanner) Create() any                           { pointerScannerCreates++; return nil }
func (*pointerScanner) Destroy(any)                           {}
func (*pointerScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (*pointerScanner) Serialize(any, []byte) int             { return 0 }
func (*pointerScanner) Deserialize(any, []byte)               {}

// TestBindingRefusesANilScannerValueBeforeCreate: a nil interface, a typed nil
// scanner and an adapter without an inner scanner are refused with the named
// reason, directly and under adapters, without calling Create.
func TestBindingRefusesANilScannerValueBeforeCreate(t *testing.T) {
	var typedNil *pointerScanner
	var nilScanner ExternalScanner
	cases := map[string]ExternalScanner{
		"nil interface":              nilScanner,
		"typed nil":                  typedNil,
		"adapter(typed nil)":         identityOrderAdapter(typedNil),
		"adapter(adapter(typed))":    identityOrderAdapter(identityOrderAdapter(typedNil)),
		"adapter(nil interface)":     &externalScannerOrderAdapter{},
		"typed nil adapter":          (*externalScannerOrderAdapter)(nil),
		"adapter(typed nil adapter)": identityOrderAdapter((*externalScannerOrderAdapter)(nil)),
	}
	for name, sc := range cases {
		if _, err := newCcoreExtScanner(sc); err == nil || err.Error() != errScannerNil {
			t.Errorf("%s: bind error %v, want %q", name, err, errScannerNil)
		}
	}
	if pointerScannerCreates != 0 {
		t.Errorf("Create was called %d times on a nil scanner value", pointerScannerCreates)
	}
	// A non-nil value of the same registered type binds.
	if _, err := newCcoreExtScanner(&pointerScanner{}); err != nil {
		t.Errorf("a non-nil value of the registered type must bind: %v", err)
	}
}

// destroyPanicScanner panics in Scan, and in Destroy once armed.
type destroyPanicScanner struct {
	destroyed *int
	armed     *bool
}

func (destroyPanicScanner) Create() any { return &modeProbe{} }
func (s destroyPanicScanner) Destroy(any) {
	*s.destroyed++
	if *s.armed {
		panic("destroy failure")
	}
}
func (destroyPanicScanner) Scan(any, *ExternalLexer, []bool) bool { panic("scanner failure") }
func (destroyPanicScanner) Serialize(any, []byte) int             { return 0 }
func (destroyPanicScanner) Deserialize(any, []byte)               {}

// TestScanPanicThenDestroyPanicDestroysTheRuntimePayloadOnce: a scan panics, the
// panic handler releases the payload, Destroy publishes it and panics; the parser's
// own deferred release must not destroy the payload a second time.
func TestScanPanicThenDestroyPanicDestroysTheRuntimePayloadOnce(t *testing.T) {
	for _, layers := range []int{0, 2} {
		destroyed, armed := new(int), new(bool)
		var sc ExternalScanner = destroyPanicScanner{destroyed: destroyed, armed: armed}
		for i := 0; i < layers; i++ {
			sc = identityOrderAdapter(sc)
		}
		a, err := newCcoreExtScanner(sc)
		if err != nil {
			t.Fatal(err)
		}
		*destroyed, *armed = 0, true // the bind probe has been destroyed; count the runtime payload
		var got any
		func() {
			defer func() { got = recover() }()
			a.scan([]byte("x"), 0, 0, 0, nil, []bool{true}, make([]byte, 16), new(ccore.ExtScan))
		}()
		if got != "destroy failure" {
			t.Fatalf("%d layers: recovered %v, want the destruction failure", layers, got)
		}
		func() { // the parser's deferred release
			defer func() { _ = recover() }()
			a.close()
		}()
		if *destroyed != 1 {
			t.Errorf("%d layers: Destroy was attempted %d times for one payload, want 1", layers, *destroyed)
		}
	}
}

// A reader belongs to the table binding from which it was created; caller
// mutation of the language must not rewrite an already-bound field map.
func TestCcoreNodeReaderFieldMapSnapshot(t *testing.T) {
	lang := loadBlobForDecode(t, "json")
	src := []byte(`{"a":1}`)
	coreRead := func(l *Language) (*ccore.Tree, ccore.NodeReader) {
		t.Helper()
		cfg, err := ccoreConfig(l)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ccore.NewParser(cfg)
		if err != nil {
			t.Fatal(err)
		}
		tree, stop := p.Parse(src)
		if tree == nil || stop != "" {
			t.Fatalf("core parse %q", stop)
		}
		return tree, ccore.NewNodeReader(tree.Reader(), cfg.Nodes)
	}
	coreFields := func(tree *ccore.Tree, r ccore.NodeReader) []uint16 {
		var out []uint16
		var visit func(ccore.Node)
		visit = func(n ccore.Node) {
			for i := uint32(0); i < r.ChildCount(n); i++ {
				c, ok := r.Child(n, i)
				if !ok {
					t.Fatal("core child")
				}
				if c.Field != 0 {
					out = append(out, c.Field)
				}
				visit(c)
			}
		}
		visit(r.RootNode(tree.Root()))
		return out
	}
	legacyFields := func(l *Language) []string {
		p := NewParser(l)
		p.SetAdmissionCandidateRoute(false)
		tree, err := p.Parse(src)
		if tree == nil || err != nil {
			t.Fatalf("legacy %v", err)
		}
		defer tree.Release()
		var out []string
		var visit func(*Node)
		visit = func(n *Node) {
			for i := 0; i < n.ChildCount(); i++ {
				if f := n.FieldNameForChild(i, l); f != "" {
					out = append(out, f)
				}
				visit(n.Child(i))
			}
		}
		visit(tree.RootNode())
		return out
	}
	tree, reader := coreRead(lang)
	defer tree.Release()
	if got := coreFields(tree, reader); !reflect.DeepEqual(got, []uint16{1, 2}) {
		t.Fatalf("initial core fields: %v", got)
	}
	if got := legacyFields(lang); !reflect.DeepEqual(got, []string{"key", "value"}) {
		t.Fatalf("initial legacy fields: %v", got)
	}
	if len(lang.FieldMapSlices) < 2 || lang.FieldMapSlices[1][1] == 0 {
		t.Fatal("unexpected JSON field table")
	}
	lang.FieldMapSlices[1] = [2]uint16{}
	if got := coreFields(tree, reader); !reflect.DeepEqual(got, []uint16{1, 2}) {
		t.Fatalf("existing reader changed: %v", got)
	}
	if got := legacyFields(lang); !reflect.DeepEqual(got, []string{"key", "value"}) {
		t.Fatalf("warmed legacy changed: %v", got)
	}
	changed := loadBlobForDecode(t, "json")
	changed.FieldMapSlices[1] = [2]uint16{}
	newTree, newReader := coreRead(changed)
	defer newTree.Release()
	if got := coreFields(newTree, newReader); len(got) != 0 {
		t.Fatalf("new core binding ignored change: %v", got)
	}
	if got := legacyFields(changed); len(got) != 0 {
		t.Fatalf("new legacy binding ignored change: %v", got)
	}
}

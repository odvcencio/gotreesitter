package gotreesitter

import (
	"testing"
	"unicode/utf8"
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

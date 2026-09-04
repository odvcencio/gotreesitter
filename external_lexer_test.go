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

func TestExternalLexerUsesByteColumnsForUTF8(t *testing.T) {
	l := newExternalLexer([]byte("x✗z"), 0, 0, 0)

	l.Advance(false) // x
	if got, want := l.Column(), uint32(1); got != want {
		t.Fatalf("column after x = %d want %d", got, want)
	}

	l.Advance(false) // ✗
	if got, want := l.Column(), uint32(4); got != want {
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
	if got, want := tok.EndPoint.Column, uint32(4); got != want {
		t.Fatalf("EndPoint.Column=%d want=%d", got, want)
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

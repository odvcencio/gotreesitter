package gotreesitter

import "testing"

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
	l.MarkEnd()      // mark at 0
	l.Advance(true)  // skip leading space
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

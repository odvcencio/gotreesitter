package gotreesitter

import "testing"

func TestExternalLexerColumnReplaysCompleteRune(t *testing.T) {
	for _, source := range []string{"x\u00e9y", "x\u2028y", "x\U0001f40dy", "x\ufffdy"} {
		width := len(source) - 2
		for offset := 2; offset < 1+width; offset++ {
			lexer := newExternalLexer([]byte(source), offset, 0, uint32(offset))
			if got := lexer.Column(); got != 2 {
				t.Fatalf("source %q byte %d: column=%d, want 2", source, offset, got)
			}
			if lexer.pos != 1+width || lexer.point.Column != uint32(1+width) || lexer.Lookahead() != 'y' ||
				lexer.startPos != offset || lexer.startPoint.Column != uint32(offset) {
				t.Fatalf("source %q byte %d: changed token start or incorrect cursor: %+v", source, offset, lexer)
			}
		}
	}
	lexer := newExternalLexer([]byte{'x', 0xff, 0x80, 'y'}, 2, 0, 2)
	if got := lexer.Column(); got != 2 || lexer.pos != 2 {
		t.Fatalf("invalid byte must not be treated as part of a valid rune: column=%d pos=%d", got, lexer.pos)
	}
}

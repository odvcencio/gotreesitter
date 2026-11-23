//go:build !grammar_subset || grammar_subset_cobol

package grammarruntime

import (
	"testing"
	_ "unsafe"

	"github.com/odvcencio/gotreesitter"
)

func TestCobolShortPrefixScanner(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		offset uint32
		point  gotreesitter.Point
		next   rune
	}{
		{"LF", "A\n      * comment\n", 8, gotreesitter.Point{Row: 1, Column: 6}, '*'},
		{"CRLF", "AB\r\n      / comment\n", 10, gotreesitter.Point{Row: 1, Column: 6}, '/'},
		{"full-prefix", "000100* comment\n", 6, gotreesitter.Point{Column: 6}, '*'},
		{"partial-prefix-at-EOF", "AB", 2, gotreesitter.Point{Column: 2}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lexer := newCobolTestExternalLexer([]byte(tc.source), 0, 0, 0)
			var valid [cobolTokenCount]bool
			valid[cobolTokLinePrefixComment] = true
			if !(CobolExternalScanner{}).Scan(nil, lexer, valid[:]) {
				t.Fatal("scanner declined the prefix")
			}
			token, ok := cobolTestExternalLexerToken(lexer)
			if !ok || token.Symbol != cobolDefaultSymTable[cobolTokLinePrefixComment] ||
				token.StartByte != tc.offset || token.EndByte != tc.offset ||
				token.StartPoint != tc.point || token.EndPoint != tc.point || lexer.Lookahead() != tc.next {
				t.Fatalf("prefix token %+v, lookahead %q, want zero-width prefix at byte %d point %+v before %q",
					token, lexer.Lookahead(), tc.offset, tc.point, tc.next)
			}
		})
	}
}

//go:linkname newCobolTestExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newCobolTestExternalLexer(source []byte, pos int, row, column uint32) *gotreesitter.ExternalLexer

//go:linkname cobolTestExternalLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func cobolTestExternalLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)

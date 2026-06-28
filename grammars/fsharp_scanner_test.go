//go:build !grammar_subset || grammar_subset_fsharp

package grammars

import (
	"reflect"
	"testing"
	_ "unsafe"

	"github.com/odvcencio/gotreesitter"
)

func TestFsharpKeywordDedentFallbackIgnoresEmptyIndentStack(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "then", src: "then"},
		{name: "and", src: "and "},
		{name: "with", src: "with "},
		{name: "else", src: "else"},
		{name: "elif", src: "elif"},
		{name: "end", src: "end"},
	}

	scanner := FsharpExternalScanner{}
	valid := make([]bool, fsTokErrorSentinel+1)
	valid[fsTokDedent] = true
	initialIndents := [][]uint16{nil, []uint16{0}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, initial := range initialIndents {
				want := append([]uint16(nil), initial...)
				if initial == nil {
					want = nil
				}
				state := &fsState{indents: initial}
				lexer := newFsharpExternalLexer([]byte(tt.src), 0, 0, 0)
				if scanner.Scan(state, lexer, valid) {
					t.Fatalf("Scan() emitted DEDENT with indent stack %#v", initial)
				}
				if tok, ok := fsharpExternalLexerToken(lexer); ok {
					t.Fatalf("Scan() returned false but produced token %+v", tok)
				}
				if !reflect.DeepEqual(state.indents, want) {
					t.Fatalf("indents = %#v, want %#v", state.indents, want)
				}
			}
		})
	}
}

//go:linkname newFsharpExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newFsharpExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

//go:linkname fsharpExternalLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func fsharpExternalLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)

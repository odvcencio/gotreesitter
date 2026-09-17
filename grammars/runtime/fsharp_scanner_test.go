//go:build !grammar_subset || grammar_subset_fsharp

package grammarruntime

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

func TestFsharpScannerCheckpointMatchesLockedByteLayout(t *testing.T) {
	scanner := FsharpExternalScanner{}
	original := &fsState{
		indents:             []uint16{0, 4, 256, 260, 1000, 65535},
		preprocessorIndents: []uint16{255, 300, 512},
	}

	buf := make([]byte, 32)
	n := scanner.Serialize(original, buf)
	wantBytes := []byte{3, 255, 44, 0, 4, 0, 4, 232, 255}
	if !reflect.DeepEqual(buf[:n], wantBytes) {
		t.Fatalf("Serialize() = %v, want locked scanner bytes %v", buf[:n], wantBytes)
	}

	restored := &fsState{}
	scanner.Deserialize(restored, buf[:n])
	wantIndents := []uint16{0, 4, 0, 4, 232, 255}
	if !reflect.DeepEqual(restored.indents, wantIndents) {
		t.Fatalf("indents after checkpoint restore = %#v, want %#v", restored.indents, wantIndents)
	}
	wantPreprocessor := []uint16{255, 44, 0}
	if !reflect.DeepEqual(restored.preprocessorIndents, wantPreprocessor) {
		t.Fatalf("preprocessorIndents after checkpoint restore = %#v, want %#v", restored.preprocessorIndents, wantPreprocessor)
	}
}

//go:linkname newFsharpExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newFsharpExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

//go:linkname fsharpExternalLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func fsharpExternalLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)

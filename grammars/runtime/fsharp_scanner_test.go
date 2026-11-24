//go:build !grammar_subset || grammar_subset_fsharp

package grammarruntime

import (
	"reflect"
	"testing"
	_ "unsafe"

	"github.com/odvcencio/gotreesitter"
)

// TestFsharpKeywordDedentFallbackFollowsPinnedC pins the keyword fallback of
// the pinned C scanner: when the keyword itself is not valid but DEDENT is,
// scan returns DEDENT through pop_indent, which pops nothing from an empty
// stack (common/scanner.h:67-74). Only "end" also asks for a non-empty stack
// (scanner.h:926). A keyword followed by a word character is no keyword.
func TestFsharpKeywordDedentFallbackFollowsPinnedC(t *testing.T) {
	tests := []struct {
		src string
		// wantEmpty and wantBase: whether scan emits DEDENT from an empty
		// stack and from the base stack [0].
		wantEmpty, wantBase bool
	}{
		{src: "then", wantEmpty: true, wantBase: true},
		{src: "and ", wantEmpty: true, wantBase: true},
		{src: "with ", wantEmpty: true, wantBase: true},
		{src: "else", wantEmpty: true, wantBase: true},
		{src: "elif", wantEmpty: true, wantBase: true},
		{src: "end", wantEmpty: false, wantBase: true},
		{src: "thenx", wantEmpty: false, wantBase: false},
		{src: "elsewhere", wantEmpty: false, wantBase: false},
		{src: "endif", wantEmpty: false, wantBase: false},
	}

	scanner := FsharpExternalScanner{}
	valid := make([]bool, fsTokenCount)
	valid[fsTokDedent] = true

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			for _, base := range []bool{false, true} {
				state := &fsState{}
				want := tt.wantEmpty
				if base {
					state.indents = []uint16{0}
					state.parenIndents = []bool{false}
					want = tt.wantBase
				}
				lexer := newFsharpExternalLexer([]byte(tt.src), 0, 0, 0)
				got := scanner.Scan(state, lexer, valid)
				if got != want {
					t.Fatalf("base stack %v: Scan() = %v, want %v", base, got, want)
				}
				tok, ok := fsharpExternalLexerToken(lexer)
				if ok != want {
					t.Fatalf("base stack %v: token present = %v, want %v", base, ok, want)
				}
				if want && tok.Symbol != fsDefaultSymTable[fsTokDedent] {
					t.Fatalf("base stack %v: token symbol %d, want DEDENT %d", base, tok.Symbol, fsDefaultSymTable[fsTokDedent])
				}
				wantDepth := 0
				if base && !want {
					wantDepth = 1
				}
				if len(state.indents) != wantDepth || len(state.parenIndents) != wantDepth {
					t.Fatalf("base stack %v: indents %v paren %v, want depth %d", base, state.indents, state.parenIndents, wantDepth)
				}
			}
		})
	}
}

// fsharpCheckpointFixture is a state whose saved bytes cover every field of
// the pinned C layout (common/scanner.h:1090-1138).
func fsharpCheckpointFixture() (*fsState, []byte) {
	state := &fsState{
		indents:             []uint16{0, 4, 256, 260, 1000, 65535},
		parenIndents:        []bool{false, true, false, false, true, true},
		preprocessorIndents: []uint16{255, 300, 512},
		multiDollarCount:    3,
	}
	// multi_dollar_count 3; three preprocessor indents (low bytes);
	// five indents (low bytes, base indent not saved); paren flags of
	// indents 1..5 as bits 0..4: 0b11001.
	saved := []byte{3, 3, 255, 44, 0, 5, 4, 0, 4, 232, 255, 0x19}
	return state, saved
}

func TestFsharpScannerCheckpointMatchesPinnedByteLayout(t *testing.T) {
	scanner := FsharpExternalScanner{}
	original, wantBytes := fsharpCheckpointFixture()

	buf := make([]byte, fsSerializationBufferSize)
	n := scanner.Serialize(original, buf)
	if !reflect.DeepEqual(buf[:n], wantBytes) {
		t.Fatalf("Serialize() = %v, want pinned scanner bytes %v", buf[:n], wantBytes)
	}

	restored := scanner.Create().(*fsState)
	scanner.Deserialize(restored, buf[:n])
	want := &fsState{
		indents:             []uint16{0, 4, 0, 4, 232, 255},
		parenIndents:        []bool{false, true, false, false, true, true},
		preprocessorIndents: []uint16{255, 44, 0},
		multiDollarCount:    3,
	}
	if !reflect.DeepEqual(restored, want) {
		t.Fatalf("state after checkpoint restore = %+v, want %+v", restored, want)
	}
}

// TestFsharpScannerDeserializeTruncatedState follows C's bounds-checked
// deserialize (common/scanner.h:1140-1182) on every prefix of a saved state:
// a count larger than the bytes left reads what is there, and missing paren
// flags read as false.
func TestFsharpScannerDeserializeTruncatedState(t *testing.T) {
	_, saved := fsharpCheckpointFixture()
	tests := []struct {
		length  int
		indents []uint16
		paren   []bool
		preproc []uint16
		dollars uint8
	}{
		{length: 0, indents: []uint16{0}, paren: []bool{false}, preproc: []uint16{}, dollars: 0},
		{length: 1, indents: []uint16{0}, paren: []bool{false}, preproc: []uint16{}, dollars: 3},
		{length: 2, indents: []uint16{0}, paren: []bool{false}, preproc: []uint16{}, dollars: 3},
		{length: 4, indents: []uint16{0}, paren: []bool{false}, preproc: []uint16{255, 44}, dollars: 3},
		{length: 5, indents: []uint16{0}, paren: []bool{false}, preproc: []uint16{255, 44, 0}, dollars: 3},
		{length: 6, indents: []uint16{0}, paren: []bool{false}, preproc: []uint16{255, 44, 0}, dollars: 3},
		{length: 8, indents: []uint16{0, 4, 0}, paren: []bool{false, false, false}, preproc: []uint16{255, 44, 0}, dollars: 3},
		{length: 11, indents: []uint16{0, 4, 0, 4, 232, 255}, paren: []bool{false, false, false, false, false, false}, preproc: []uint16{255, 44, 0}, dollars: 3},
		{length: 12, indents: []uint16{0, 4, 0, 4, 232, 255}, paren: []bool{false, true, false, false, true, true}, preproc: []uint16{255, 44, 0}, dollars: 3},
	}
	scanner := FsharpExternalScanner{}
	for _, tt := range tests {
		state := scanner.Create().(*fsState)
		// A restore replaces every field, whatever the payload held before.
		state.indents = append(state.indents, 9, 9)
		state.parenIndents = append(state.parenIndents, true, true)
		state.preprocessorIndents = append(state.preprocessorIndents, 7)
		state.multiDollarCount = 9
		scanner.Deserialize(state, saved[:tt.length])
		got := &fsState{
			indents:             append([]uint16{}, state.indents...),
			parenIndents:        append([]bool{}, state.parenIndents...),
			preprocessorIndents: append([]uint16{}, state.preprocessorIndents...),
			multiDollarCount:    state.multiDollarCount,
		}
		want := &fsState{indents: tt.indents, parenIndents: tt.paren, preprocessorIndents: tt.preproc, multiDollarCount: tt.dollars}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("length %d: state = %+v, want %+v", tt.length, got, want)
		}
	}
}

// TestFsharpScannerSerializeClampsCounts pins C's 255 bound on both counts
// and the 32 paren bytes of 255 indents.
func TestFsharpScannerSerializeClampsCounts(t *testing.T) {
	state := &fsState{}
	fsDeserialize(state, nil)
	for i := 0; i < 300; i++ {
		fsPushIndent(state, uint16(i), i%3 == 0)
		state.preprocessorIndents = append(state.preprocessorIndents, uint16(i))
	}
	buf := make([]byte, fsSerializationBufferSize)
	n := FsharpExternalScanner{}.Serialize(state, buf)
	if want := 1 + 1 + 255 + 1 + 255 + 32; n != want {
		t.Fatalf("Serialize() wrote %d bytes, want %d", n, want)
	}
	if buf[1] != 255 || buf[2+255] != 255 {
		t.Fatalf("counts = %d, %d, want 255, 255", buf[1], buf[2+255])
	}
	restored := &fsState{}
	fsDeserialize(restored, buf[:n])
	if len(restored.indents) != 256 || len(restored.parenIndents) != 256 || len(restored.preprocessorIndents) != 255 {
		t.Fatalf("restored depths indents %d paren %d preproc %d, want 256, 256, 255",
			len(restored.indents), len(restored.parenIndents), len(restored.preprocessorIndents))
	}
	for i := 1; i < 256; i++ {
		if restored.indents[i] != uint16(i-1) || restored.parenIndents[i] != ((i-1)%3 == 0) {
			t.Fatalf("indent %d = %d paren %v, want %d paren %v", i, restored.indents[i], restored.parenIndents[i], i-1, (i-1)%3 == 0)
		}
	}
}

//go:linkname newFsharpExternalLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newFsharpExternalLexer(source []byte, pos int, row, col uint32) *gotreesitter.ExternalLexer

//go:linkname fsharpExternalLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func fsharpExternalLexerToken(*gotreesitter.ExternalLexer) (gotreesitter.Token, bool)

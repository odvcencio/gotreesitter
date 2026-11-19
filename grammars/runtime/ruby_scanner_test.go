//go:build !grammar_subset || grammar_subset_ruby

package grammarruntime

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

func TestRubyScannerMinusSymbolOrderMatchesGrammar(t *testing.T) {
	lang := RubyLanguage()
	if len(lang.ExternalSymbols) <= rbyTokBinaryMinus {
		t.Fatalf("ruby external symbol count = %d, want > %d", len(lang.ExternalSymbols), rbyTokBinaryMinus)
	}

	for _, tc := range []struct {
		name string
		idx  int
		sym  uint16
	}{
		{name: "unary_minus", idx: rbyTokUnaryMinus, sym: uint16(rbyDefaultSymTable[rbyTokUnaryMinus])},
		{name: "unary_minus_num", idx: rbyTokUnaryMinusNum, sym: uint16(rbyDefaultSymTable[rbyTokUnaryMinusNum])},
		{name: "binary_minus", idx: rbyTokBinaryMinus, sym: uint16(rbyDefaultSymTable[rbyTokBinaryMinus])},
	} {
		if got := uint16(lang.ExternalSymbols[tc.idx]); got != tc.sym {
			t.Fatalf("%s symbol = %d, want %d", tc.name, got, tc.sym)
		}
	}
}

func TestRubyScannerHeredocCheckpointFormats(t *testing.T) {
	scanner := RubyExternalScanner{}
	for _, tc := range []struct {
		name     string
		word     string
		faithful bool
	}{
		{name: "legacy", word: "DOC"},
		{name: "C faithful", word: strings.Repeat("X", 256), faithful: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newPayload := func() *rbyScannerState {
				t.Helper()
				state := scanner.Create().(*rbyScannerState)
				if tc.faithful {
					mode, ok := any(state).(scanmode.CFaithful)
					if !ok {
						t.Fatal("scanner payload does not support C-faithful checkpoints")
					}
					mode.SetCFaithful(true)
				}
				return state
			}
			state := newPayload()
			state.openHeredocs = []rbyHeredoc{{
				word:                      []byte(tc.word),
				endWordIndentationAllowed: true,
				allowsInterpolation:       true,
				started:                   true,
			}}
			want := []byte{0, 1, 1, 1, 1}
			if tc.faithful {
				want = binary.NativeEndian.AppendUint32(want, uint32(len(tc.word)))
			} else {
				want = append(want, byte(len(tc.word)))
			}
			want = append(want, tc.word...)

			buffer := make([]byte, rbyMaxSerialize)
			if count := scanner.Serialize(state, buffer); !bytes.Equal(buffer[:count], want) {
				t.Fatalf("serialized checkpoint has %d bytes, want %d matching bytes", count, len(want))
			}
			restored := newPayload()
			scanner.Deserialize(restored, want)
			if len(restored.openHeredocs) != 1 {
				t.Fatalf("restored %d heredocs, want 1", len(restored.openHeredocs))
			}
			doc := restored.openHeredocs[0]
			if !bytes.Equal(doc.word, []byte(tc.word)) || !doc.endWordIndentationAllowed || !doc.allowsInterpolation || !doc.started {
				t.Fatal("restored heredoc differs from the serialized state")
			}
		})
	}
}

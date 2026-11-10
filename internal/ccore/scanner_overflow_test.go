package ccore_test

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// bigState is a scanner port that reports a state longer than the 1,024-byte
// serialization buffer (C asserts length <= TREE_SITTER_SERIALIZATION_BUFFER_SIZE).
type bigState struct{ n int }

func (b bigState) Scan(_ []byte, _ int, _, _ uint32, _ []byte, _ []bool, _ []byte, out *ccore.ExtScan) bool {
	out.StateLen = b.n
	out.Token = ccore.Token{Symbol: 1}
	return true
}
func (bigState) Close() {}

// TestScannerStateAboveBufferStopsParse: a state length above the buffer (or
// below zero) stops the parse with a clear text; it neither panics nor
// truncates. A length equal to the buffer (1,024) is accepted.
func TestScannerStateAboveBufferStopsParse(t *testing.T) {
	g := loadGoldens(t)[0]
	tables := *g.Tables
	modes := append([]ccore.LexMode(nil), tables.LexModes...)
	modes[tables.InitialState].ExternalLexState = 1
	tables.LexModes = modes
	tables.ExternalSymbols = []ccore.Symbol{1}
	tables.ExternalLexStates = [][]bool{nil, {true}}
	for _, n := range []int{1025, 2000, -1, 1024} {
		p, err := ccore.NewParser(ccore.Config{Tables: &tables, Scanner: bigState{n: n}})
		if err != nil {
			t.Fatal(err)
		}
		var stop string
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("state length %d panicked: %v", n, r)
				}
			}()
			_, stop = p.Parse(g.Src)
		}()
		refused := strings.Contains(stop, "external scanner reported a state of") && strings.Contains(stop, "1024")
		if n == 1024 {
			// A length equal to the buffer is accepted: no overflow stop (what
			// else the stub scanner makes the parse do does not matter here).
			if strings.Contains(stop, "external scanner reported a state of") {
				t.Errorf("state length 1024 was refused: stop text %q", stop)
			}
		} else if !refused {
			t.Errorf("state length %d: stop text %q", n, stop)
		}
	}
}

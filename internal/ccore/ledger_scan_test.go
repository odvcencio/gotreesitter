package ccore_test

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// countScanner returns the same result on every callback and counts the calls.
type countScanner struct {
	found bool
	calls *int
}

func (c countScanner) Scan(src []byte, pos int, row, col uint32, _ []byte, _ []bool, _ []byte, out *ccore.ExtScan) bool {
	*c.calls++
	out.LookaheadEnd = uint32(pos) + 1
	if c.found && pos < len(src) {
		out.Token = ccore.Token{Symbol: 1, StartByte: uint32(pos), EndByte: uint32(pos) + 1,
			StartRow: row, StartCol: col, EndRow: row, EndCol: col + 1, LookaheadEnd: uint32(pos) + 1}
		return true
	}
	return false
}
func (countScanner) Close() {}

func scanTables(t *testing.T) *ccore.Tables {
	g := loadGoldens(t)[0]
	tables := *g.Tables
	modes := append([]ccore.LexMode(nil), tables.LexModes...)
	for i := range modes { // every state, the error state 0 included, runs the scanner first
		modes[i].ExternalLexState = 1
	}
	tables.LexModes = modes
	tables.ExternalSymbols = []ccore.Symbol{1}
	tables.ExternalLexStates = [][]bool{nil, {true}}
	return &tables
}

// Test 3: the limit is asked after a scanner callback whatever it returned, every
// N-th callback, with a heap sample; a refusal ends the parse and no callback runs
// after it. The error lexer retries the scanner after every skipped character, so
// the false-returning scanner reaches the check there.
func TestScannerCallbacksAreCheckedAfterBothResults(t *testing.T) {
	src := []byte("[1, " + strings.Repeat("@", 400) + "]")
	const every = 16
	for _, found := range []bool{false, true} {
		name := map[bool]string{false: "scanner returns false", true: "scanner returns true"}[found]
		t.Run(name, func(t *testing.T) {
			calls := 0
			p, err := ccore.NewParser(ccore.Config{Tables: scanTables(t), Scanner: countScanner{found: found, calls: &calls}})
			if err != nil {
				t.Fatal(err)
			}
			p.SetScanCheckEvery(every)
			samples, callsAtSample := 0, []int{}
			p.Ledger().SetPoll(func(total int64, sample bool) bool {
				if sample {
					samples++
					callsAtSample = append(callsAtSample, calls)
				}
				return true
			})
			tr, stop := p.Parse(src)
			if tr != nil {
				tr.Release()
			}
			if calls < 4*every {
				t.Fatalf("only %d callbacks (stop %q); the input must make at least %d", calls, stop, 4*every)
			}
			if samples != calls/every {
				t.Fatalf("%d callbacks gave %d forced checks, want %d (one per %d); at calls %v", calls, samples, calls/every, every, callsAtSample)
			}
			for i, c := range callsAtSample {
				if c != (i+1)*every {
					t.Fatalf("forced check %d came after %d callbacks, want %d", i, c, (i+1)*every)
				}
			}
			// A refusing poll ends the parse at the first forced check, and no callback runs after it.
			calls = 0
			p2, _ := ccore.NewParser(ccore.Config{Tables: scanTables(t), Scanner: countScanner{found: found, calls: &calls}})
			p2.SetScanCheckEvery(every)
			p2.Ledger().SetPoll(func(total int64, sample bool) bool { return !sample })
			tree, stop2 := p2.Parse(src)
			if stop2 != "stopped" || tree != nil || calls != every {
				t.Fatalf("refusing poll: stop %q tree %v callbacks %d; want \"stopped\", no tree, exactly %d callbacks", stop2, tree, calls, every)
			}
		})
	}
}

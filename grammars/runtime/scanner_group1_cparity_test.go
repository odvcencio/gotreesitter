//go:build !grammar_subset

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"testing"
)

type scannerGroup1Case struct {
	name, src            string
	off                  int
	mask, state          string
	found                bool
	symbol               gotreesitter.Symbol
	start, end, frontier uint32
}

func checkScannerGroup1(t *testing.T, sc gotreesitter.ExternalScanner, tc scannerGroup1Case) {
	t.Helper()
	payload := sc.Create()
	defer sc.Destroy(payload)
	sc.Deserialize(payload, []byte(tc.state))
	valid := make([]bool, len(tc.mask))
	for i := range valid {
		valid[i] = tc.mask[i] == '1'
	}
	row, col := uint32(0), uint32(0)
	for i := 0; i < tc.off; i++ {
		if tc.src[i] == '\n' {
			row, col = row+1, 0
		} else {
			col++
		}
	}
	lexer := newPortParityLexer([]byte(tc.src), tc.off, row, col)
	portInstallObserver(lexer)
	ok := sc.Scan(payload, lexer, valid)
	result := portObserve(lexer, ok)
	if result.ok != tc.found || result.frontier != tc.frontier || (tc.found && (!result.hasTok || result.sym != tc.symbol || result.start != tc.start || result.end != tc.end)) {
		t.Errorf("got found=%v symbol=%d start=%d end=%d frontier=%d; want found=%v symbol=%d start=%d end=%d frontier=%d", result.ok, result.sym, result.start, result.end, result.frontier, tc.found, tc.symbol, tc.start, tc.end, tc.frontier)
	}
}

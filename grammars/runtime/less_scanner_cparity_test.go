//go:build !grammar_subset || grammar_subset_less

package grammarruntime

import "testing"

func TestLessScannerCParity(t *testing.T) {
	scanner := LessExternalScanner{}
	cases := []struct {
		name, source string
		valid, found bool
		end          uint32
	}{
		{"colon despite mask", " : x;", false, true, 1},
		{"vertical tab before selector", "\va", true, true, 1},
		{"non-ASCII letter", " \u00e9", true, false, 0},
		{"asterisk", " *", true, false, 0},
	}
	for _, c := range cases {
		got := portScan(scanner, cFaithfulPayload(scanner), c.source, []bool{c.valid})
		portWantScan(t, c.name, got, c.found, lessDefaultSymTable[lessTokDescendantOp], c.end)
	}
}

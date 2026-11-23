//go:build !grammar_subset || grammar_subset_d

package grammarruntime

import "testing"

func TestDExternalScannerCFaithfulMaskAndBoundary(t *testing.T) {
	scanner := DExternalScanner{}
	valid := make([]bool, dTokenCount)
	valid[dTokNotIn] = true

	// tree-sitter-d src/scanner.c:463-469 leaves a masked token at its
	// final letter and allows an ASCII underscore after a valid operator.
	for _, tc := range []struct {
		name, text string
		ok         bool
		end, col   uint32
	}{
		{"masked operator", "!is ", false, 0, 2},
		{"underscore after operator", "!in_", true, 3, 3},
		{"nonbreaking space before operator", "\u00a0!in", false, 0, 0},
		{"nonbreaking space inside operator", "!\u00a0in", false, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := scanner.Create()
			if mode, ok := payload.(interface{ SetCFaithful(bool) }); ok {
				mode.SetCFaithful(true)
			}
			got := portScan(scanner, payload, tc.text, valid)
			portWantScan(t, tc.name, got, tc.ok, dDefaultSymTable[dTokNotIn], tc.end)
			if got.col != tc.col {
				t.Errorf("scanner cursor column = %d, want %d", got.col, tc.col)
			}
		})
	}

	if got := portScan(scanner, scanner.Create(), "!in_", valid); got.ok {
		t.Errorf("ordinary scanner mode accepted !in_")
	}
}

func TestDExternalScannerCFaithfulNumberDecoderError(t *testing.T) {
	scanner := DExternalScanner{}
	valid := make([]bool, dTokenCount)
	valid[dTokIntLiteral], valid[dTokFloatLiteral] = true, true
	payload := scanner.Create()
	if mode, ok := payload.(interface{ SetCFaithful(bool) }); ok {
		mode.SetCFaithful(true)
	}
	// tree-sitter-d src/scanner.c:380-391 tests the decoded value after
	// the period. A decoder error is -1, whereas U+FFFD is above 0x7f.
	portWantScan(t, "decoder error", portScan(scanner, payload, "1.\xff", valid),
		true, dDefaultSymTable[dTokFloatLiteral], 2)
	portWantScan(t, "replacement character", portScan(scanner, payload, "1.\ufffd", valid),
		true, dDefaultSymTable[dTokIntLiteral], 1)
}

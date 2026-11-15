//go:build !grammar_subset

package grammarruntime

import "testing"

// Reproductions of the pinned C scanner, scanner.c:16, 48, 70-79, 147-148, 175-193.
func TestLuauScannerCParityReproducers(t *testing.T) {
	sc := LuauExternalScanner{}
	for _, tc := range []scannerGroup1Case{
		{"damaged-end", "\x0a\xc3", 1, "100010", "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff", true, 95, 1, 1, 6},
		{"damaged-found", "\xc3", 0, "100001", "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff", true, 96, 0, 1, 2},
		{"damaged-lookahead", "\xc3", 0, "100001", "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff", true, 96, 0, 1, 2},
		{"damaged-point", "\x0a\xc3", 1, "100010", "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff", true, 95, 1, 1, 6},
		{"damaged-state", "\xff", 0, "111111", "\xff\xff\xff\xff\xff\xff\xff\xff", true, 96, 0, 1, 2},
		{"damaged-symbol", "\xff", 0, "111111", "\xff\xff\xff\xff\xff\xff\xff\xff", true, 96, 0, 1, 2},
		{"mask-lookahead", "\x0a\xc2\xa0", 1, "001100", "", false, 0, 0, 0, 2},
		{"ordinary-lookahead", "\x0a\xc2\xa0", 1, "101000", "", false, 0, 0, 0, 2},
		{"range-lookahead", "\x0d\x0a\x20\x09\x0d\x0a\xc2\xa0", 3, "101000", "", false, 0, 0, 0, 7},
	} {
		t.Run(tc.name, func(t *testing.T) { checkScannerGroup1(t, sc, tc) })
	}
}

// scanner.c:16,70-79,175-184: a signed saved delimiter can match a C decode error.
func TestLuauScannerCParityDamagedStateAfterScan(t *testing.T) {
	sc := LuauExternalScanner{}
	payload := sc.Create()
	defer sc.Destroy(payload)
	sc.Deserialize(payload, []byte("\xff\xff\xff\xff\xff\xff\xff\xff"))
	lexer := newPortParityLexer([]byte("\xff"), 0, 0, 0)
	if !sc.Scan(payload, lexer, []bool{true, true, true, true, true, true}) {
		t.Fatal("scan failed")
	}
	buf := make([]byte, 2)
	if n := sc.Serialize(payload, buf); n != 2 || string(buf) != "\x00\x00" {
		t.Fatalf("state = %x (length %d), want 0000 (length 2)", buf, n)
	}
}

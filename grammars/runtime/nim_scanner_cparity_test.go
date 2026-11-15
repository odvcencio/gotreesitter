//go:build !grammar_subset && !gotreesitter_no_copyleft

package grammarruntime

import (
	"bytes"
	"testing"
)

// Pinned C scanner src/scanner.c:393-405.
func TestNimScannerCParityReproducers(t *testing.T) {
	sc := NimExternalScanner{}
	for _, c := range []struct {
		tc    scannerGroup1Case
		state string
	}{
		{scannerGroup1Case{"damaged-found", "\x20\x00", 1, "00000000110101101", "\xff\xff\xff", false, 0, 0, 0, 2}, ""},
		{scannerGroup1Case{"damaged-lookahead", "\x3a\x00", 1, "10000000000100000", "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00", false, 0, 0, 0, 3}, ""},
		{scannerGroup1Case{"mask-end", "\x0a\x00", 1, "01101100000111011", "\x04", true, 129, 1, 2, 3}, "\x04"},
		{scannerGroup1Case{"mask-found", "\x0a\x00", 1, "10000111100100100", "\x00", false, 0, 0, 0, 3}, ""},
		{scannerGroup1Case{"mask-lookahead", "\x20\x00", 1, "10011111000000011", "\x00", false, 0, 0, 0, 3}, ""},
		{scannerGroup1Case{"mask-point", "\x0a\x00", 1, "01101100000111011", "\x04", true, 129, 1, 2, 3}, "\x04"},
		{scannerGroup1Case{"mask-state", "\x20\x00", 0, "00001000000000000", "", true, 137, 0, 0, 2}, "\x01"},
		{scannerGroup1Case{"ordinary-end", "\x20\x00", 1, "00100000000100000", "\x04", true, 129, 1, 2, 3}, "\x04"},
		{scannerGroup1Case{"ordinary-found", "\x0a\x00", 1, "00000010000100010", "\x00", false, 0, 0, 0, 2}, ""},
		{scannerGroup1Case{"ordinary-lookahead", "\x72\x00", 1, "01000000000100000", "\x00", false, 0, 0, 0, 3}, ""},
		{scannerGroup1Case{"ordinary-point", "\x20\x00", 1, "00100000000100000", "\x04", true, 129, 1, 2, 3}, "\x04"},
		{scannerGroup1Case{"ordinary-state", "\x20\x00", 0, "00000010011100011", "", true, 137, 0, 0, 2}, "\x01"},
		{scannerGroup1Case{"ordinary-symbol", "\x09\x0a\x20\x00", 1, "00001010001101100", "\x00", true, 131, 1, 1, 4}, "\x00\x01"},
		{scannerGroup1Case{"range-end", "\x0d\x0a\x0a\x20\x20\x20\x20\x00\xc3\x0a\x20\x20\x20\x20\x20", 7, "10111000001001001", "", true, 129, 7, 9, 10}, ""},
	} {
		t.Run(c.tc.name, func(t *testing.T) {
			checkScannerGroup1(t, sc, c.tc)
			if !c.tc.found {
				return
			}
			p := sc.Create()
			defer sc.Destroy(p)
			sc.Deserialize(p, []byte(c.tc.state))
			valid := make([]bool, len(c.tc.mask))
			for i := range valid {
				valid[i] = c.tc.mask[i] == 49
			}
			portScanAt(sc, p, c.tc.src, c.tc.off, valid)
			b := make([]byte, 1024)
			n := sc.Serialize(p, b)
			if !bytes.Equal(b[:n], []byte(c.state)) {
				t.Errorf("state=%x; want %x", b[:n], c.state)
			}
		})
	}
}

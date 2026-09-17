//go:build gts_parsercorephase0

package gotreesitter

import "testing"

func TestCertifiedGoScannerUsesSharedRuntimeIdentity(t *testing.T) {
	if _, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner); err != nil {
		t.Fatalf("authenticate the packaged Go scanner: %v", err)
	}
	for _, scanner := range []ExternalScanner{nil, struct{ ExternalScanner }{parserCoreWarmGoScanner}} {
		if _, err := authenticatedParserCoreGoLanguage(scanner); err == nil {
			t.Fatalf("accepted an uncertified scanner: %T", scanner)
		}
	}
}

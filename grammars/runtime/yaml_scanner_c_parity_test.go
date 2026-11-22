//go:build !grammar_subset || grammar_subset_yaml

package grammarruntime

import "testing"

func TestYAMLScannerCModeDistinguishesMalformedUTF8FromReplacementRune(t *testing.T) {
	scanner := YamlExternalScanner{}
	valid := make([]bool, yTokCount)
	valid[yTokRDqtStrCtn] = true
	valid[yTokBRDqtStrCtn] = true

	t.Run("invalid byte", func(t *testing.T) {
		got := portScanAt(scanner, cFaithfulPayload(scanner), "\n\xff", 1, valid)
		portWantScan(t, "C decode error", got, true, yamlDefaultSymTable[yTokEndOfFile], 1)
	})
	t.Run("valid replacement rune", func(t *testing.T) {
		got := portScanAt(scanner, cFaithfulPayload(scanner), "\n\uFFFD", 1, valid)
		portWantScan(t, "replacement rune", got, true, yamlDefaultSymTable[yTokRDqtStrCtn], 4)
	})
	t.Run("legacy", func(t *testing.T) {
		got := portScanAt(scanner, scanner.Create(), "\n\xff", 1, valid)
		portWantScan(t, "legacy replacement rune", got, true, yamlDefaultSymTable[yTokRDqtStrCtn], 2)
	})
}

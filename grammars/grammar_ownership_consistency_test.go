package grammars

import "testing"

// TestGrammarOwnershipConsistency asserts that grammar_ownership.go and the
// registered grammar sources agree for every language in the registry: a
// grammar declared Own must resolve to GrammarSourceGrammargenBlob, and a
// grammar that resolves to GrammarSourceGrammargenBlob must declare Own.
//
// This guards against exactly the drift found for swift (declared Mirror
// while its registry entry already advertised GrammarSourceGrammargenBlob)
// and regex (no ownership record while its registry entry already
// advertised GrammarSourceGrammargenBlob). It checks the effective
// GrammarSource returned by AllLanguages, which reflects any runtime upgrade
// Register applies through applyGrammarOwnership, not only the static value
// a registry entry declares before Register runs.
func TestGrammarOwnershipConsistency(t *testing.T) {
	for _, entry := range AllLanguages() {
		ownership, ok := GrammarOwnershipFor(entry.Name)
		isOwn := ok && ownership.MaintenanceClass == GrammarMaintenanceOwn
		isGrammargenBlob := entry.GrammarSource == GrammarSourceGrammargenBlob
		if isOwn != isGrammargenBlob {
			t.Errorf("%s: MaintenanceClass Own=%v but GrammarSource=%q (grammargen blob=%v); ownership and registry source disagree",
				entry.Name, isOwn, entry.GrammarSource, isGrammargenBlob)
		}
	}
}

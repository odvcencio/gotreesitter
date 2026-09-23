package grammars

import "testing"

// copyleftGrammarsUnderTest lists every grammar licenses/grammars.json marks
// copyleft (spdx "copyleft": true). Kept in sync by hand since this file
// runs under every build; see docs/licensing.md and
// copyleft_language_set_no_copyleft.go's copyleftGrammarNames for the
// authoritative gotreesitter_no_copyleft-only list.
var copyleftGrammarsUnderTest = []string{"caddy", "disassembly", "jq", "nim"}

// TestCopyleftLanguageSetMatchesRegistry checks, for both the default build
// and the gotreesitter_no_copyleft build, that a copyleft grammar's registry
// presence agrees exactly with copyleftLanguageAllowed: present when
// allowed, absent (from both DetectLanguageByName and AllLanguages) when
// not. Run this under both configurations:
//
//	go test ./grammars/ -run TestCopyleftLanguageSetMatchesRegistry
//	go test ./grammars/ -run TestCopyleftLanguageSetMatchesRegistry -tags gotreesitter_no_copyleft
func TestCopyleftLanguageSetMatchesRegistry(t *testing.T) {
	all := AllLanguages()
	inAll := make(map[string]bool, len(all))
	for _, e := range all {
		inAll[e.Name] = true
	}

	for _, name := range copyleftGrammarsUnderTest {
		wantPresent := copyleftLanguageAllowed(name)

		if gotPresent := DetectLanguageByName(name) != nil; gotPresent != wantPresent {
			t.Errorf("DetectLanguageByName(%q) presence = %v, want %v (copyleftLanguageAllowed)", name, gotPresent, wantPresent)
		}
		if gotPresent := inAll[name]; gotPresent != wantPresent {
			t.Errorf("AllLanguages() presence of %q = %v, want %v (copyleftLanguageAllowed)", name, gotPresent, wantPresent)
		}
	}
}

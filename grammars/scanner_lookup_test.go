package grammars

import "testing"

func TestLookupExternalScanner(t *testing.T) {
	// Registered scanners should be found.
	for _, name := range []string{"python", "css", "javascript", "html", "yaml"} {
		if s := LookupExternalScanner(name); s == nil {
			t.Errorf("LookupExternalScanner(%q) = nil, want non-nil", name)
		}
	}

	// Non-existent scanner should return nil.
	if s := LookupExternalScanner("nonexistent_language_xyz"); s != nil {
		t.Errorf("LookupExternalScanner(%q) = %v, want nil", "nonexistent_language_xyz", s)
	}
}

func TestLookupExternalLexStates(t *testing.T) {
	// scss and yaml have registered external lex states.
	for _, name := range []string{"scss", "yaml"} {
		if els := LookupExternalLexStates(name); els == nil {
			t.Errorf("LookupExternalLexStates(%q) = nil, want non-nil", name)
		}
	}

	if els := LookupExternalLexStates("nonexistent_language_xyz"); els != nil {
		t.Errorf("LookupExternalLexStates(%q) = %v, want nil", "nonexistent_language_xyz", els)
	}
}

func TestAdaptScannerForLanguageNilTarget(t *testing.T) {
	if AdaptScannerForLanguage("css", nil) {
		t.Fatal("expected false for nil target language")
	}
}

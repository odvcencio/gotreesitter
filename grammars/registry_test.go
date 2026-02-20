package grammars

import "testing"

func TestDetectLanguageGo(t *testing.T) {
	entry := DetectLanguage("main.go")
	if entry == nil {
		t.Fatal("expected to detect Go language for main.go, got nil")
	}
	if entry.Name != "go" {
		t.Fatalf("expected language name %q, got %q", "go", entry.Name)
	}
	if entry.TokenSourceFactory == nil {
		t.Fatal("expected Go language to register a TokenSourceFactory")
	}
}

func TestDetectLanguageUnknown(t *testing.T) {
	entry := DetectLanguage("readme.xyz")
	if entry != nil {
		t.Fatalf("expected nil for unknown extension, got %q", entry.Name)
	}
}

func TestAllLanguages(t *testing.T) {
	langs := AllLanguages()
	if len(langs) == 0 {
		t.Fatal("expected at least one registered language, got 0")
	}

	found := false
	for _, l := range langs {
		if l.Name == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected Go language to be registered")
	}
}

func TestDetectLanguageByShebang(t *testing.T) {
	// No languages have shebangs registered, so this should return nil.
	entry := DetectLanguageByShebang("#!/usr/bin/env python3")
	if entry != nil {
		t.Fatalf("expected nil for unregistered shebang, got %q", entry.Name)
	}
}

func TestAuditParseSupportIncludesGoCustomTokenSource(t *testing.T) {
	reports := AuditParseSupport()
	if len(reports) == 0 {
		t.Fatal("expected parse support reports")
	}

	var goReport *ParseSupport
	for i := range reports {
		if reports[i].Name == "go" {
			goReport = &reports[i]
			break
		}
	}
	if goReport == nil {
		t.Fatal("expected go parse support report")
	}
	if goReport.Backend != ParseBackendTokenSource {
		t.Fatalf("expected go backend %q, got %q", ParseBackendTokenSource, goReport.Backend)
	}
}

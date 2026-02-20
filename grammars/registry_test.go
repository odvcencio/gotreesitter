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

func TestAuditParseSupportIncludesCCustomTokenSource(t *testing.T) {
	reports := AuditParseSupport()
	if len(reports) == 0 {
		t.Fatal("expected parse support reports")
	}

	var cReport *ParseSupport
	for i := range reports {
		if reports[i].Name == "c" {
			cReport = &reports[i]
			break
		}
	}
	if cReport == nil {
		t.Fatal("expected c parse support report")
	}
	if cReport.Backend != ParseBackendTokenSource {
		t.Fatalf("expected c backend %q, got %q", ParseBackendTokenSource, cReport.Backend)
	}
}

func TestAuditParseSupportIncludesJSONCustomTokenSource(t *testing.T) {
	reports := AuditParseSupport()
	if len(reports) == 0 {
		t.Fatal("expected parse support reports")
	}

	var jsonReport *ParseSupport
	for i := range reports {
		if reports[i].Name == "json" {
			jsonReport = &reports[i]
			break
		}
	}
	if jsonReport == nil {
		t.Fatal("expected json parse support report")
	}
	if jsonReport.Backend != ParseBackendTokenSource {
		t.Fatalf("expected json backend %q, got %q", ParseBackendTokenSource, jsonReport.Backend)
	}
}

func TestAuditParseSupportIncludesJavaCustomTokenSource(t *testing.T) {
	reports := AuditParseSupport()
	if len(reports) == 0 {
		t.Fatal("expected parse support reports")
	}

	var javaReport *ParseSupport
	for i := range reports {
		if reports[i].Name == "java" {
			javaReport = &reports[i]
			break
		}
	}
	if javaReport == nil {
		t.Fatal("expected java parse support report")
	}
	if javaReport.Backend != ParseBackendTokenSource {
		t.Fatalf("expected java backend %q, got %q", ParseBackendTokenSource, javaReport.Backend)
	}
}

func TestAuditParseSupportIncludesLuaCustomTokenSource(t *testing.T) {
	reports := AuditParseSupport()
	if len(reports) == 0 {
		t.Fatal("expected parse support reports")
	}

	var luaReport *ParseSupport
	for i := range reports {
		if reports[i].Name == "lua" {
			luaReport = &reports[i]
			break
		}
	}
	if luaReport == nil {
		t.Fatal("expected lua parse support report")
	}
	if luaReport.Backend != ParseBackendTokenSource {
		t.Fatalf("expected lua backend %q, got %q", ParseBackendTokenSource, luaReport.Backend)
	}
}

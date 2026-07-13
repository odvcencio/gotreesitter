package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func isRegisteredLanguage(name string) bool {
	return lookupByName(name) != nil
}

func TestGrammargenBlobProvenance(t *testing.T) {
	for _, name := range []string{"go", "regex", "swift"} {
		t.Run(name, func(t *testing.T) {
			entry := DetectLanguageByName(name)
			if entry == nil {
				t.Fatalf("DetectLanguageByName(%q) returned nil", name)
			}
			if entry.GrammarSource != GrammarSourceGrammargenBlob {
				t.Fatalf("GrammarSource = %q, want %q", entry.GrammarSource, GrammarSourceGrammargenBlob)
			}
			if blob := BlobByName(name); len(blob) == 0 {
				t.Fatalf("BlobByName(%q) returned empty", name)
			}
			if entry.Language == nil {
				t.Fatal("registry language loader is nil")
			}
			lang := entry.Language()
			if lang == nil {
				t.Fatal("registry language loader returned nil")
			}
			if !lang.GeneratedByGrammargen {
				t.Fatal("decoded blob is not marked GeneratedByGrammargen")
			}
		})
	}
}

func TestDetectLanguageGo(t *testing.T) {
	entry := DetectLanguage("main.go")
	if entry == nil {
		t.Fatal("expected to detect Go language for main.go, got nil")
	}
	if entry.Name != "go" {
		t.Fatalf("expected language name %q, got %q", "go", entry.Name)
	}
	// Go no longer registers a default TokenSourceFactory as of 0.14.0 — the
	// grammargen-compiled blob ships DFA tables that parse Go on their own,
	// and the hand-tuned GoTokenSource was calibrated to ts2go's symbol
	// layout. GoTokenSource remains available via the public API for
	// callers that carry a ts2go-compiled Go blob.
	if entry.TokenSourceFactory != nil {
		t.Fatal("Go now uses the DFA backend by default; no TokenSourceFactory should be registered")
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

func TestAllLanguagesDoesNotLoadGrammars(t *testing.T) {
	// Purge all cached grammars so we start clean.
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	// AllLanguages should be a metadata-only operation.
	langs := AllLanguages()
	if len(langs) == 0 {
		t.Fatal("expected at least one registered language")
	}

	loaded, _ := EmbeddedLanguageCacheStats()
	if loaded != 0 {
		t.Fatalf("AllLanguages() loaded %d grammars into cache; want 0 (metadata-only)", loaded)
	}
}

func TestDetectLanguageByShebang(t *testing.T) {
	// Linguist interpreter map resolves shebangs.
	entry := DetectLanguageByShebang("#!/usr/bin/env python3")
	if entry == nil || entry.Name != "python" {
		name := ""
		if entry != nil {
			name = entry.Name
		}
		t.Fatalf("DetectLanguageByShebang(python3) = %q, want %q", name, "python")
	}

	// Unknown interpreter returns nil.
	entry = DetectLanguageByShebang("#!/usr/bin/env nonexistent_interp_xyz")
	if entry != nil {
		t.Fatalf("expected nil for unknown interpreter, got %q", entry.Name)
	}
}

// parseSupportForLang evaluates parse support for a single language by name,
// loading only that grammar instead of all 206.
func parseSupportForLang(t *testing.T, name string) ParseSupport {
	t.Helper()
	report, ok := ParseSupportFor(name)
	if !ok {
		t.Fatalf("language %q not registered", name)
	}
	t.Cleanup(func() { UnloadEmbeddedLanguage(name + ".bin") })
	return report
}

func TestParseSupportForLoadsOnlyRequestedGrammar(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	report, ok := ParseSupportFor("go")
	if !ok || report.Name != "go" || report.Backend != ParseBackendDFA {
		t.Fatalf("ParseSupportFor(go) = (%+v, %t), want Go DFA report", report, ok)
	}
	loaded, _ := EmbeddedLanguageCacheStats()
	if loaded != 1 {
		t.Fatalf("ParseSupportFor(go) loaded %d grammars, want 1", loaded)
	}
	if report, ok := ParseSupportFor("not-a-language"); ok || report != (ParseSupport{}) {
		t.Fatalf("ParseSupportFor(missing) = (%+v, %t), want zero report and false", report, ok)
	}
	loaded, _ = EmbeddedLanguageCacheStats()
	if loaded != 1 {
		t.Fatalf("ParseSupportFor(missing) changed loaded grammar count to %d, want 1", loaded)
	}
}

func TestAuditParseSupportUsesDFAForGo(t *testing.T) {
	// As of 0.14.0 Go uses the DFA backend baked into the grammargen-compiled
	// blob rather than the hand-tuned GoTokenSource, which was calibrated to
	// ts2go's different symbol layout. Previously named
	// TestAuditParseSupportIncludesGoCustomTokenSource.
	report := parseSupportForLang(t, "go")
	if report.Backend != ParseBackendDFA {
		t.Fatalf("expected go backend %q, got %q", ParseBackendDFA, report.Backend)
	}
}

func TestAuditParseSupportIncludesCCustomTokenSource(t *testing.T) {
	report := parseSupportForLang(t, "c")
	if report.Backend != ParseBackendTokenSource {
		t.Fatalf("expected c backend %q, got %q", ParseBackendTokenSource, report.Backend)
	}
}

func TestAuditParseSupportIncludesCppCustomTokenSource(t *testing.T) {
	report := parseSupportForLang(t, "cpp")
	if report.Backend != ParseBackendTokenSource {
		t.Fatalf("expected cpp backend %q, got %q", ParseBackendTokenSource, report.Backend)
	}
}

func TestAuditParseSupportIncludesJSONCustomTokenSource(t *testing.T) {
	report := parseSupportForLang(t, "json")
	if report.Backend != ParseBackendTokenSource {
		t.Fatalf("expected json backend %q, got %q", ParseBackendTokenSource, report.Backend)
	}
}

func TestAuditParseSupportIncludesJavaCustomTokenSource(t *testing.T) {
	report := parseSupportForLang(t, "java")
	if report.Backend != ParseBackendTokenSource {
		t.Fatalf("expected java backend %q, got %q", ParseBackendTokenSource, report.Backend)
	}
}

// Lua parses via the blob's DFA lexer plus LuaExternalScanner (a line-faithful
// port of upstream scanner.c), not the hand-tuned LuaTokenSource — the DFA path
// matches the C oracle where the token source diverged. The public
// LuaTokenSource remains available to downstream callers, but the registry no
// longer routes lua through it.
func TestAuditParseSupportLuaUsesDFAExternalScanner(t *testing.T) {
	report := parseSupportForLang(t, "lua")
	if report.Backend != ParseBackendDFA && report.Backend != ParseBackendDFAPartial {
		t.Fatalf("expected lua backend to use native DFA lexer, got %q", report.Backend)
	}
	if entry := lookupByName("lua"); entry == nil {
		t.Fatal("missing registry entry for lua")
	} else if entry.TokenSourceFactory != nil {
		t.Fatal("lua should no longer register a token source factory")
	}
}

func TestAuditParseSupportIncludesTomlNativeLexerBackend(t *testing.T) {
	report := parseSupportForLang(t, "toml")
	if report.Backend != ParseBackendDFA && report.Backend != ParseBackendDFAPartial {
		t.Fatalf("expected toml backend to use native lexer, got %q", report.Backend)
	}
}

func TestAuditParseSupportIncludesJavaScriptDFA(t *testing.T) {
	report := parseSupportForLang(t, "javascript")
	if report.Backend != ParseBackendDFA {
		t.Fatalf("expected javascript backend %q, got %q", ParseBackendDFA, report.Backend)
	}
}

func TestAuditParseSupportIncludesTypeScriptDFA(t *testing.T) {
	report := parseSupportForLang(t, "typescript")
	if report.Backend != ParseBackendDFA {
		t.Fatalf("expected typescript backend %q, got %q", ParseBackendDFA, report.Backend)
	}
}

func TestAuditParseSupportIncludesRustDFA(t *testing.T) {
	report := parseSupportForLang(t, "rust")
	if report.Backend != ParseBackendDFA {
		t.Fatalf("expected rust backend %q, got %q", ParseBackendDFA, report.Backend)
	}
}

func TestBuiltinLanguagesAdvertiseTS2GoBlobSource(t *testing.T) {
	entry := DetectLanguage("main.py")
	if entry == nil {
		t.Fatal("expected Python language for main.py")
	}
	if entry.GrammarSource != GrammarSourceTS2GoBlob {
		t.Fatalf("Python GrammarSource = %q, want %q", entry.GrammarSource, GrammarSourceTS2GoBlob)
	}
}

func TestRegisterExtensionMarksGrammargenSource(t *testing.T) {
	_ = AllLanguages()

	oldRegistry := append([]LangEntry(nil), registry...)
	oldResolved := highlightInheritanceResolved
	oldAliases := make(map[string]string, len(extensionAliases))
	for k, v := range extensionAliases {
		oldAliases[k] = v
	}
	t.Cleanup(func() {
		registry = oldRegistry
		highlightInheritanceResolved = oldResolved
		extensionAliases = oldAliases
	})

	RegisterExtension(ExtensionEntry{
		Name:       "zz_registry_source_extension",
		Extensions: []string{".zzsrc"},
		Aliases:    []string{"zzsrc"},
		GenerateLanguage: func() (*gotreesitter.Language, error) {
			return nil, nil
		},
		HighlightQuery: "(identifier) @variable",
	})

	entry := DetectLanguage("sample.zzsrc")
	if entry == nil {
		t.Fatal("expected extension language to be registered")
	}
	if entry.GrammarSource != GrammarSourceGrammargen {
		t.Fatalf("extension GrammarSource = %q, want %q", entry.GrammarSource, GrammarSourceGrammargen)
	}
}

func TestCoreLanguagesHaveCompilableTagsQuery(t *testing.T) {
	core := []string{
		"go",
		"python",
		"javascript",
		"typescript",
		"tsx",
		"rust",
		"java",
		"c",
		"cpp",
	}

	entries := AllLanguages()
	entryByName := make(map[string]LangEntry, len(entries))
	for _, entry := range entries {
		entryByName[entry.Name] = entry
	}

	for _, name := range core {
		name := name
		t.Run(name, func(t *testing.T) {
			entry, ok := entryByName[name]
			if !ok {
				t.Fatalf("expected %q language to be registered", name)
			}
			if entry.Language == nil {
				t.Fatalf("expected %q language loader", name)
			}
			tagsQ := ResolveTagsQuery(entry)
			if tagsQ == "" {
				t.Fatalf("expected non-empty TagsQuery for %q", name)
			}
			if _, err := gotreesitter.NewTagger(entry.Language(), tagsQ); err != nil {
				t.Fatalf("compile tags query for %q: %v", name, err)
			}
		})
	}
}

func TestInferredTagsQueryCoverage(t *testing.T) {
	entries := AllLanguages()
	if len(entries) == 0 {
		t.Fatal("expected registered languages")
	}

	withTags := 0
	for _, entry := range entries {
		if ResolveTagsQuery(entry) != "" {
			withTags++
		}
	}

	// Core set (9) is explicit. Inference should expand this materially.
	if withTags < 30 {
		t.Fatalf("expected inferred tags query coverage to be >=30 languages, got %d", withTags)
	}
}

func TestInferredGoTagsQuerySkipsReturnTypes(t *testing.T) {
	entry := DetectLanguageByName("go")
	if entry == nil {
		t.Fatal("expected go language entry")
	}
	query := ResolveTagsQuery(*entry)
	if query == "" {
		t.Fatal("expected inferred Go tags query")
	}
	if strings.Contains(query, "(function_declaration (type_identifier)") {
		t.Fatalf("Go inferred tags query should not capture return type identifiers as functions:\n%s", query)
	}

	tagger, err := gotreesitter.NewTagger(entry.Language(), query)
	if err != nil {
		t.Fatalf("NewTagger: %v", err)
	}
	src := []byte(`package p

func levelFromKind(kind string) int {
	return 2
}

func InitDB() error {
	return nil
}
`)
	tags := tagger.Tag(src)
	var functions []string
	for _, tag := range tags {
		if tag.Kind == "definition.function" {
			functions = append(functions, tag.Name)
		}
	}
	if got, want := strings.Join(functions, ","), "levelFromKind,InitDB"; got != want {
		t.Fatalf("definition.function names = %q, want %q; tags=%+v", got, want, tags)
	}
}

func TestDetectLanguageByName(t *testing.T) {
	tests := []struct {
		input    string
		wantName string // empty = expect nil
	}{
		// Direct grammar name always works (even with empty linguist map).
		{"go", "go"},
		{"python", "python"},
		{"javascript", "javascript"},
		// Unknown.
		{"nonexistent_language_xyz", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := DetectLanguageByName(tt.input)
		if tt.wantName == "" {
			if got != nil {
				t.Errorf("DetectLanguageByName(%q) = %q, want nil", tt.input, got.Name)
			}
		} else {
			if got == nil {
				t.Errorf("DetectLanguageByName(%q) = nil, want %q", tt.input, tt.wantName)
			} else if got.Name != tt.wantName {
				t.Errorf("DetectLanguageByName(%q) = %q, want %q", tt.input, got.Name, tt.wantName)
			}
		}
	}
}

func TestDisplayName(t *testing.T) {
	// Linguist-mapped name.
	entry := &LangEntry{Name: "c_sharp"}
	got := DisplayName(entry)
	if got != "C#" {
		t.Errorf("DisplayName(c_sharp) = %q, want %q", got, "C#")
	}
	// Fallback to title-case for unmapped names.
	entry2 := &LangEntry{Name: "some_unknown_lang"}
	got2 := DisplayName(entry2)
	if got2 != "Some Unknown Lang" {
		t.Errorf("DisplayName(some_unknown_lang) fallback = %q, want %q", got2, "Some Unknown Lang")
	}
	if DisplayName(nil) != "" {
		t.Error("DisplayName(nil) should return empty string")
	}
}

func TestDetectLanguageByNameAliases(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
	}{
		// Linguist canonical names (mixed case).
		{"Go", "go"},
		{"Python", "python"},
		{"JavaScript", "javascript"},
		{"TypeScript", "typescript"},
		{"C++", "cpp"},
		{"C#", "c_sharp"},
		{"Objective-C", "objc"},
		{"F#", "fsharp"},
		{"Shell", "bash"},
		{"Makefile", "make"},
		{"TSX", "tsx"},
		{"Rust", "rust"},
		{"Ruby", "ruby"},
		{"Java", "java"},
		{"HTML", "html"},
		{"CSS", "css"},
		{"YAML", "yaml"},
		{"TOML", "toml"},
		{"SQL", "sql"},
		{"Kotlin", "kotlin"},
		{"Swift", "swift"},
		{"Scala", "scala"},
		{"Elixir", "elixir"},
		// Linguist aliases.
		{"golang", "go"},
		{"js", "javascript"},
		{"ts", "typescript"},
		{"py", "python"},
		{"rb", "ruby"},
		{"rs", "rust"},
		// Case insensitivity.
		{"PYTHON", "python"},
		{"c++", "cpp"},
		{"f#", "fsharp"},
		{"shell", "bash"},
		{"javascript", "javascript"},
		{"makefile", "make"},
		// Edge: gotreesitter name directly.
		{"cpp", "cpp"},
		{"c_sharp", "c_sharp"},
		{"objc", "objc"},
		{"fsharp", "fsharp"},
		{"bash", "bash"},
	}
	for _, tt := range tests {
		if !isRegisteredLanguage(tt.wantName) {
			continue
		}
		got := DetectLanguageByName(tt.input)
		if got == nil {
			t.Errorf("DetectLanguageByName(%q) = nil, want %q", tt.input, tt.wantName)
		} else if got.Name != tt.wantName {
			t.Errorf("DetectLanguageByName(%q) = %q, want %q", tt.input, got.Name, tt.wantName)
		}
	}
}

func TestDetectLanguageFilename(t *testing.T) {
	tests := []struct {
		filename string
		wantName string // empty = expect nil
	}{
		// Exact filename matches via linguist.
		{"Makefile", "make"},
		{"Dockerfile", "dockerfile"},
		{"Gemfile", "ruby"},
		{"Rakefile", "ruby"},
		{"Vagrantfile", "ruby"},
		{"Jakefile", "javascript"},
		{".bashrc", "bash"},
		{".bash_profile", "bash"},
		{".zshrc", "bash"},
		{".profile", "bash"},
		// With directory prefix.
		{"/home/user/.bashrc", "bash"},
		{"some/path/Makefile", "make"},
		// Exact filenames take priority over extension suffix matches.
		// .tmux.conf and nginx.conf are linguist filenames; without
		// correct priority they'd match the generic ".conf" extension.
		{".tmux.conf", "bash"},
		{"nginx.conf", "nginx"},
		// Extended extensions via linguist.
		{"build.mk", "make"},
		{"build.mak", "make"},
		{"task.rake", "ruby"},
		{"app.gemspec", "ruby"},
		{"script.es6", "javascript"},
		// Standard extensions still work (registry path).
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		// Unknown.
		{"random_file_no_ext", ""},
		{"something.xyz_unknown", ""},
	}
	for _, tt := range tests {
		if tt.wantName != "" && !isRegisteredLanguage(tt.wantName) {
			continue
		}
		got := DetectLanguage(tt.filename)
		if tt.wantName == "" {
			if got != nil {
				t.Errorf("DetectLanguage(%q) = %q, want nil", tt.filename, got.Name)
			}
		} else {
			if got == nil {
				t.Errorf("DetectLanguage(%q) = nil, want %q", tt.filename, tt.wantName)
			} else if got.Name != tt.wantName {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.filename, got.Name, tt.wantName)
			}
		}
	}
}

func TestDetectLanguageByShebangComprehensive(t *testing.T) {
	tests := []struct {
		line     string
		wantName string // empty = expect nil
	}{
		// env form.
		{"#!/usr/bin/env python3", "python"},
		{"#!/usr/bin/env python", "python"},
		{"#!/usr/bin/env node", "javascript"},
		{"#!/usr/bin/env ruby", "ruby"},
		{"#!/usr/bin/env bash", "bash"},
		{"#!/usr/bin/env perl", "perl"},
		{"#!/usr/bin/env lua", "lua"},
		// Direct path form.
		{"#!/usr/bin/python3", "python"},
		{"#!/bin/bash", "bash"},
		{"#!/bin/sh", "bash"},
		{"#!/usr/bin/ruby", "ruby"},
		// env with flags (e.g., env -S).
		{"#!/usr/bin/env -S python3", "python"},
		// env with VAR=value assignments.
		{"#!/usr/bin/env PYTHONPATH=/foo python3", "python"},
		{"#!/usr/bin/env -S VAR=val python3", "python"},
		// Not a shebang.
		{"not a shebang", ""},
		{"", ""},
		// Unknown interpreter.
		{"#!/usr/bin/env nonexistent_xyz", ""},
	}
	for _, tt := range tests {
		got := DetectLanguageByShebang(tt.line)
		if tt.wantName == "" {
			if got != nil {
				t.Errorf("DetectLanguageByShebang(%q) = %q, want nil", tt.line, got.Name)
			}
		} else {
			if got == nil {
				t.Errorf("DetectLanguageByShebang(%q) = nil, want %q", tt.line, tt.wantName)
			} else if got.Name != tt.wantName {
				t.Errorf("DetectLanguageByShebang(%q) = %q, want %q", tt.line, got.Name, tt.wantName)
			}
		}
	}
}

func TestDisplayNamePopulated(t *testing.T) {
	tests := []struct {
		grammar string
		want    string
	}{
		{"cpp", "C++"},
		{"c_sharp", "C#"},
		{"objc", "Objective-C"},
		{"fsharp", "F#"},
		{"javascript", "JavaScript"},
		{"typescript", "TypeScript"},
		{"bash", "Shell"},
		{"make", "Makefile"},
		{"go", "Go"},
		{"python", "Python"},
		{"rust", "Rust"},
		{"ruby", "Ruby"},
		{"java", "Java"},
		{"html", "HTML"},
		{"css", "CSS"},
		{"yaml", "YAML"},
		{"sql", "SQL"},
	}
	for _, tt := range tests {
		entry := lookupByName(tt.grammar)
		if entry == nil {
			continue
		}
		got := DisplayName(entry)
		if got != tt.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tt.grammar, got, tt.want)
		}
	}
}

func TestDetectLanguageByNameRoundTrip(t *testing.T) {
	// Every registered grammar must be resolvable by its own name.
	for _, entry := range AllLanguages() {
		got := DetectLanguageByName(entry.Name)
		if got == nil {
			t.Errorf("DetectLanguageByName(%q) = nil, want grammar entry", entry.Name)
		} else if got.Name != entry.Name {
			t.Errorf("DetectLanguageByName(%q) = %q, want %q (alias shadows direct name)", entry.Name, got.Name, entry.Name)
		}
	}
}

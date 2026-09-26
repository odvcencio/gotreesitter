package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR6DetectsNewEngineBranchesAndEnvironmentReads(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"grammars/grammar_blobs", "internal/parsercorephase0"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "parser.go")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("package gotreesitter\n"+body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(`import "os"
type language struct { Name string }
func f(lang language) { _ = lang.Name == "go"; _ = os.Getenv("GOT_OLD") }
`)
	language, env, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	oldLang, oldEnv := countFindings(language), countFindings(env)
	if len(language) != 1 || len(env) != 1 {
		t.Fatalf("baseline: %v, %v", language, env)
	}
	write(`import "os"
type language struct { Name string }
func f(lang language) {
  _ = lang.Name == "go"
  _ = lang.Name == "rust"
  switch lang.Name { case "go": }
  _ = map[string]bool{"go": true}
  _ = os.Getenv("GOT_OLD")
  _ = os.LookupEnv("GOT_NEW")
}
`)
	language, env, err = scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkAllowlist("language", language, oldLang); err == nil || !strings.Contains(err.Error(), "compare|rust") || !strings.Contains(err.Error(), "switch") || !strings.Contains(err.Error(), "map") {
		t.Fatalf("language additions were not rejected: %v", err)
	}
	if err := checkAllowlist("env", env, oldEnv); err == nil || !strings.Contains(err.Error(), "GOT_NEW") {
		t.Fatalf("environment addition was not rejected: %v", err)
	}
}

func TestR6AllowlistMustShrinkAfterRemoval(t *testing.T) {
	allowed := map[string]int{"parser.go|compare|go": 2}
	items := []finding{{key: "parser.go|compare|go"}}
	if err := checkAllowlist("language", items, allowed); err == nil || !strings.Contains(err.Error(), "shrink allowlist") {
		t.Fatalf("stale allowance was not rejected: %v", err)
	}
}

func TestR6EnvironmentRegistryCannotGrowFromBase(t *testing.T) {
	old := map[string]int{"parser.go|env|GOT_OLD": 1}
	current := map[string]int{"parser.go|env|GOT_OLD": 1, "parser.go|env|GOT_NEW": 1}
	if err := checkNoGrowth("env_registry.txt", old, current); err == nil || !strings.Contains(err.Error(), "GOT_NEW") {
		t.Fatalf("new registry row was not rejected: %v", err)
	}
	current = map[string]int{"parser.go|env|GOT_OLD": 2}
	if err := checkNoGrowth("env_registry.txt", old, current); err == nil || !strings.Contains(err.Error(), "GOT_OLD") {
		t.Fatalf("increased registry count was not rejected: %v", err)
	}
}

func TestR6NamedConstantsCannotBypassLanguageGuard(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	source := `package gotreesitter
const grammarName = "go"
type language struct { Name string }
func f(lang language) {
  _ = lang.Name == grammarName
  switch lang.Name { case grammarName: }
  _ = map[string]bool{grammarName: true}
}`
	if err := os.WriteFile(filepath.Join(root, "parser.go"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	language, _, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(language) != 3 {
		t.Fatalf("want comparison, switch, and map findings; got %+v", language)
	}
	if err := checkAllowlist("language", language, nil); err == nil {
		t.Fatal("named constants escaped the guard")
	}
}

func TestR6EnvironmentReadsThroughOSImportAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"alias.go": "package gotreesitter\nimport operatingSystem \"os\"\nvar _ = operatingSystem.Getenv(\"GOT_ALIAS\")\n",
		"dot.go":   "package gotreesitter\nimport . \"os\"\nvar _, _ = LookupEnv(\"GOT_DOT\")\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	_, env, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 2 {
		t.Fatalf("want both aliased reads, got %+v", env)
	}
	if err := checkAllowlist("env", env, nil); err == nil || !strings.Contains(err.Error(), "GOT_ALIAS") || !strings.Contains(err.Error(), "GOT_DOT") {
		t.Fatalf("aliased reads escaped the guard: %v", err)
	}
}

func TestEngineFileScope(t *testing.T) {
	for _, path := range []string{"parser.go", "parser_result_awk.go", "glr.go", "lexer.go", "scanner_dispatch.go", "internal/parsercorephase0/core.go", "internal/recover/new.go"} {
		if !engineFile(path) {
			t.Errorf("engine file missed: %s", path)
		}
	}
	for _, path := range []string{"parser_test.go", "internal/benchfixtures/work.go", "grammars/runtime/scanner.go", "cmd/tool/parser.go"} {
		if engineFile(path) {
			t.Errorf("support file included: %s", path)
		}
	}
}

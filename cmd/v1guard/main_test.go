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
func f(lang language, target string) {
  l := lang
  _ = lang.Name == "go"
  _ = lang.Name == "rust"
  _ = l.Name == target
  switch lang.Name { case "go": }
  switch l.Name { case target: }
  _ = map[string]bool{"go": true}
  m := map[string]bool{}
  m["go"] = true
  m[l.Name] = true
  _ = os.Getenv("GOT_OLD")
  _ = os.LookupEnv("GOT_NEW")
}
`)
	language, env, err = scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkAllowlist("language", language, oldLang); err == nil || !strings.Contains(err.Error(), "compare|rust") || !strings.Contains(err.Error(), "compare-dynamic") || !strings.Contains(err.Error(), "switch-dynamic") || !strings.Contains(err.Error(), "map|go") || !strings.Contains(err.Error(), "map-index|go") || !strings.Contains(err.Error(), "map-index|language.Name") {
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

func TestR6LocalNameAliasesCannotBypassLanguageGuard(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	source := `package gotreesitter
type language struct { Name string }
func f(l language, target string) {
  name := (l.Name)
  copy := name
  _ = (copy) == "go"
  _ = copy == target
  switch (copy) { case "go": }
  m := map[string]bool{}
  m[(copy)] = true
}
`
	if err := os.WriteFile(filepath.Join(root, "parser.go"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	language, _, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(language) != 4 {
		t.Fatalf("want aliased literal and dynamic comparisons, switch, and map index; got %+v", language)
	}
	if err := checkAllowlist("language", language, nil); err == nil || !strings.Contains(err.Error(), "compare|go") || !strings.Contains(err.Error(), "compare-dynamic") || !strings.Contains(err.Error(), "switch|go") || !strings.Contains(err.Error(), "map-index|language.Name") {
		t.Fatalf("local alias escaped the guard: %v", err)
	}
}

func TestR6NamedMapTypeCannotBypassLanguageGuard(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"parser_types.go": "package gotreesitter\ntype langFlags map[string]bool\n",
		"parser.go":       "package gotreesitter\nvar _ = langFlags{\"go\": true}\nvar _ = map[string]langFlags{\"outer\": {\"go\": true}}\n",
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	language, _, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(language) != 2 || language[0].key != "parser.go|map|go" || language[1].key != "parser.go|map|go" {
		t.Fatalf("named or nested map key was not found: %+v", language)
	}
	if err := checkAllowlist("language", language, nil); err == nil {
		t.Fatal("named map key escaped the guard")
	}
}

func TestR6EnvironmentReadsThroughOSImportAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd/tool"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"alias.go":         "package gotreesitter\nimport operatingSystem \"os\"\nvar _ = operatingSystem.Getenv(\"GOT_ALIAS\")\n",
		"dot.go":           "package gotreesitter\nimport . \"os\"\nvar _, _ = LookupEnv(\"GOT_DOT\")\n",
		"cmd/tool/main.go": "package main\nimport \"os\"\nvar _ = os.Getenv(\"GOT_COMMAND\")\nfunc f() { get := os.Getenv; copy := get; _ = copy(\"GOT_LOCAL\"); lookup := os.LookupEnv; _, _ = lookup(\"GOT_LOOKUP\") }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	_, env, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 5 {
		t.Fatalf("want import, command, and local function alias reads, got %+v", env)
	}
	if err := checkAllowlist("env", env, nil); err == nil || !strings.Contains(err.Error(), "GOT_ALIAS") || !strings.Contains(err.Error(), "GOT_DOT") || !strings.Contains(err.Error(), "GOT_COMMAND") || !strings.Contains(err.Error(), "GOT_LOCAL") || !strings.Contains(err.Error(), "GOT_LOOKUP") {
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

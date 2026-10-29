package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scan runs the plain scan, without following names through parameters.
func scan(root string) ([]finding, []finding, error) {
	p, err := loadProject(root)
	if err != nil {
		return nil, nil, err
	}
	language, env, _ := p.scan(nil)
	return language, env, nil
}

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

func TestR6WrappedNamesCannotBypassLanguageGuard(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	source := `package gotreesitter
type language struct { Name string }
func f(l language) {
  _ = l.Name + "" == "go"
  switch l.Name + "" { case "go": }
  m := map[string]bool{}
  m[l.Name + ""] = true
  name := l.Name + ""
  _ = name == "go"
}`
	if err := os.WriteFile(filepath.Join(root, "parser.go"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	language, _, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(language) != 4 {
		t.Fatalf("wrapped language name was not tracked: %+v", language)
	}
	if err := checkAllowlist("language", language, nil); err == nil || !strings.Contains(err.Error(), "compare|go") || !strings.Contains(err.Error(), "switch|go") || !strings.Contains(err.Error(), "map-index|language.Name") {
		t.Fatalf("wrapped language name escaped guard: %v", err)
	}
}

func TestR6ComparisonCallsCannotBypassLanguageGuard(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	source := `package gotreesitter
import "strings"
type language struct { Name string }
func f(l language, target string) {
  _ = strings.EqualFold(l.Name, "go")
  _ = strings.Compare("go", l.Name)
  _ = strings.HasPrefix(l.Name, target)
  equal := strings.EqualFold
  copy := equal
  _ = copy(l.Name, "go")
  _ = strings.EqualFold(target, "go")
}`
	if err := os.WriteFile(filepath.Join(root, "parser.go"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	language, _, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(language) != 4 {
		t.Fatalf("want literal and dynamic comparison calls, got %+v", language)
	}
	if err := checkAllowlist("language", language, nil); err == nil || !strings.Contains(err.Error(), "compare-call|go") || !strings.Contains(err.Error(), "compare-call-dynamic|language.Name") {
		t.Fatalf("comparison call escaped the guard: %v", err)
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
	for _, path := range []string{"parser.go", "parser_result_awk.go", "glr.go", "lexer.go", "scanner_dispatch.go", "incremental_leaf_fastpath.go", "conflict_policy.go", "token_invariant_dependency.go", "result_compat_elision.go", "internal/parsercorephase0/core.go", "internal/recover/new.go"} {
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

// A go:linkname declaration has no body. The scanner must read past it and
// still see the declarations around it.
func TestR6ScansFunctionsDeclaredWithoutABody(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	source := `package tool
import (
	"os"
	_ "unsafe"
)

//go:linkname link example.com/root.link
func link(source []byte) int

func f() { _ = os.Getenv("GOT_AFTER_LINK") }
`
	if err := os.WriteFile(filepath.Join(root, "tool.go"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	_, env, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := countFindings(env); got["tool.go|env|GOT_AFTER_LINK"] != 1 || len(env) != 1 {
		t.Fatalf("environment findings = %v, want one read of GOT_AFTER_LINK", got)
	}
}

func TestR6WidenedPrefixesChangeScope(t *testing.T) {
	if engineFileIn("conflict_policy.go", legacyEnginePrefixes) {
		t.Error("legacy scope must not include conflict_policy.go")
	}
	if !engineFileIn("parser.go", legacyEnginePrefixes) || !engineFileIn("internal/recover/new.go", legacyEnginePrefixes) {
		t.Error("legacy scope lost a file it covered")
	}
}

func TestR6BaseCompareIsScopeAware(t *testing.T) {
	// A base without a prefixes header was written under the legacy scope.
	base := []byte("# R6 baseline.\n1\tparser.go|compare|go\n")
	current := map[string]int{
		"parser.go|compare|go":           1,
		"conflict_policy.go|compare|go":  3, // new scope: a new row is not growth
		"internal/x/y.go|switch|ruby":    1, // internal packages were always in scope
		"incremental_leaf_fastpath.go|x": 1,
	}
	err := checkScopedGrowth("language_allowlist.txt", base, true, current)
	if err == nil || !strings.Contains(err.Error(), "internal/x/y.go|switch|ruby") {
		t.Fatalf("growth inside the base scope was not rejected: %v", err)
	}
	delete(current, "internal/x/y.go|switch|ruby")
	if err := checkScopedGrowth("language_allowlist.txt", base, true, current); err != nil {
		t.Fatalf("rows from the widened scope were rejected: %v", err)
	}
	current["parser.go|compare|go"] = 2
	if err := checkScopedGrowth("language_allowlist.txt", base, true, current); err == nil {
		t.Fatal("growth of an existing row was not rejected")
	}
	// An unscoped registry is compared in full, whatever its header says.
	env := []byte("1\tcmd/tool/main.go|env|GOT_OLD\n")
	if err := checkScopedGrowth("env_registry.txt", env, false, map[string]int{"cmd/tool/main.go|env|GOT_OLD": 1, "cmd/tool/main.go|env|GOT_NEW": 1}); err == nil {
		t.Fatal("new environment row was not rejected")
	}
	// A base written under the current prefixes is compared in full.
	same := []byte(prefixesHeader + strings.Join(rootEnginePrefixes, ",") + "\n1\tparser.go|compare|go\n")
	if err := checkScopedGrowth("language_allowlist.txt", same, true, map[string]int{"parser.go|compare|go": 1, "conflict_policy.go|compare|go": 1}); err == nil {
		t.Fatal("new row under identical scope was not rejected")
	}
}

func TestR6RegistryMustCarryCurrentPrefixes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "language_allowlist.txt")
	if err := writeCounts(path, map[string]int{"parser.go|compare|go": 1}, true); err != nil {
		t.Fatal(err)
	}
	if counts, err := readRegistry(path, true); err != nil || counts["parser.go|compare|go"] != 1 {
		t.Fatalf("round trip failed: %v, %v", counts, err)
	}
	stale := filepath.Join(dir, "stale.txt")
	if err := os.WriteFile(stale, []byte("1\tparser.go|compare|go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegistry(stale, true); err == nil || !strings.Contains(err.Error(), "-init") {
		t.Fatalf("registry without the current prefixes was accepted: %v", err)
	}
	if _, err := readRegistry(stale, false); err != nil {
		t.Fatalf("unscoped registry must not need a header: %v", err)
	}
}

func TestR6NamesPassedThroughParametersAreTracked(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grammars/grammar_blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grammars/grammar_blobs/go.bin"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		// parser.go passes a name directly, through a local, and through the
		// languageName helper. Its helper g is in another file.
		"parser.go": `package gotreesitter
type language struct { Name string }
func languageName(l *language) string { return l.Name }
func f(l *language) {
  _ = g(l.Name)
  n := l.Name
  _ = h(1, "x", n)
  _ = k(languageName(l))
  _ = notEngine(l.Name)
}
func k(first string) bool { return first == "ruby" }
`,
		"glr_helpers.go": `package gotreesitter
func g(name string) bool { return name == "go" }
func h(depth int, a, b string) bool { switch b { case "c": return true }; return a == "unrelated" }
func chain(name string) bool { return deep(name) }
func deep(x string) bool { return x == "chained" }
`,
		"parser_chain.go": `package gotreesitter
func start(l *language) bool { return chain(l.Name) }
`,
		"feature_layer.go": `package gotreesitter
func notEngine(n string) bool { return n == "feature" }
func unrelated(n string) bool { return n == "never-called-with-a-name" }
`,
		"other/parser.go": `package other
func g(name string) bool { return name == "other-package" }
`,
	}
	for name, source := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := loadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	plain, _, _ := p.scan(nil)
	if len(plain) != 0 {
		t.Fatalf("plain scan must not follow parameters: %+v", plain)
	}
	got := countFindings(p.flowFindings(plain))
	want := map[string]int{
		"glr_helpers.go|compare|go":      1, // direct argument
		"glr_helpers.go|switch|c":        1, // second string parameter, through a local
		"parser.go|compare|ruby":         1, // languageName result
		"glr_helpers.go|compare|chained": 1, // two calls deep
	}
	for key, n := range want {
		if got[key] != n {
			t.Errorf("%s: got %d, want %d (all: %v)", key, got[key], n, got)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("unexpected finding %s", key)
		}
	}
}

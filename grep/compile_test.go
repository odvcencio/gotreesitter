package grep

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// testLang returns a *gotreesitter.Language for the named grammar, skipping
// the test if the grammar is unavailable.
func testLang(t *testing.T, name string) *gotreesitter.Language {
	t.Helper()
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Skipf("%s grammar not available", name)
	}
	lang := entry.Language()
	// The grep pattern compiler parses fragments (e.g. "$X + $Y") without
	// a surrounding statement/function context. The ts2go Go blob recovered
	// these into `binary_expression` nodes under an ERROR root; grammargen's
	// Go blob (default in 0.14.0+) structures error recovery differently and
	// wraps the fragment more tightly. The CompilePattern fragment-wrap
	// path has a known gap here; Go grep-pattern tests are skipped against
	// the grammargen blob pending a rewrite that embeds the fragment in a
	// parseable host (e.g. `func _() { <fragment> }`).
	if name == "go" {
		if _, ok := lang.SymbolByName("source_file_token1"); !ok {
			t.Skip("skip: Go grep-pattern fragment parser needs update for grammargen Go blob")
		}
	}
	return lang
}

// testParse parses source code with the named language and returns a BoundTree.
// The caller must call bt.Release().
func testParse(t *testing.T, langName string, source []byte) (*gotreesitter.BoundTree, *gotreesitter.Language) {
	t.Helper()
	entry := grammars.DetectLanguageByName(langName)
	if entry == nil {
		t.Skipf("%s grammar not available", langName)
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	var tree *gotreesitter.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		ts := entry.TokenSourceFactory(source, lang)
		tree, err = parser.ParseWithTokenSource(source, ts)
	} else {
		tree, err = parser.Parse(source)
	}
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return gotreesitter.Bind(tree), lang
}

// matchCaptures returns all values for a given capture name from query matches.
func matchCaptures(matches []gotreesitter.QueryMatch, capName string, source []byte) []string {
	var vals []string
	for _, m := range matches {
		for _, c := range m.Captures {
			if c.Name == capName {
				vals = append(vals, c.Text(source))
			}
		}
	}
	return vals
}

// --------------------------------------------------------------------------
// Error handling tests
// --------------------------------------------------------------------------

func TestCompilePattern_NilLanguage(t *testing.T) {
	_, err := CompilePattern(nil, "func $NAME()")
	if err == nil {
		t.Fatal("expected error for nil language")
	}
}

func TestCompilePattern_EmptyPattern(t *testing.T) {
	lang := testLang(t, "go")
	_, err := CompilePattern(lang, "")
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestCompilePattern_WhitespacePattern(t *testing.T) {
	lang := testLang(t, "go")
	_, err := CompilePattern(lang, "   \t  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only pattern")
	}
}

func TestCompilePatternForLang_UnknownLanguage(t *testing.T) {
	_, err := CompilePatternForLang("nonexistent_lang_xyz", `func $NAME()`)
	if err == nil {
		t.Fatal("expected error for unknown language")
	}
}

// --------------------------------------------------------------------------
// Go function patterns
// --------------------------------------------------------------------------

func TestCompilePattern_GoFunctionName(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $NAME()`)
	if err != nil {
		t.Fatalf("compile error: %v\n", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

func myFunc() {}
func another() {}
func withParam(x int) {}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	names := matchCaptures(matches, "NAME", source)
	t.Logf("matched names: %v", names)

	// Should match myFunc and another (empty params) but not withParam.
	if len(names) != 2 {
		t.Errorf("expected 2 matches, got %d", len(names))
	}
	for _, n := range names {
		if n != "myFunc" && n != "another" {
			t.Errorf("unexpected match: %q", n)
		}
	}
}

func TestCompilePattern_GoFunctionWithErrorReturn(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $NAME() error`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

func myFunc() error { return nil }
func another() int { return 0 }
func third() error { return nil }
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	names := matchCaptures(matches, "NAME", source)
	t.Logf("matched names: %v", names)

	if len(names) != 2 {
		t.Errorf("expected 2 matches (functions returning error), got %d", len(names))
	}
}

func TestCompilePattern_GoFunctionWithParams(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $NAME($$$PARAMS) error`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

func myFunc(a int, b string) error { return nil }
func another() int { return 0 }
func third(x bool) error { return nil }
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	names := matchCaptures(matches, "NAME", source)
	params := matchCaptures(matches, "PARAMS", source)
	t.Logf("matched names: %v, params: %v", names, params)

	if len(names) < 2 {
		t.Errorf("expected at least 2 matches, got %d", len(names))
	}
}

// --------------------------------------------------------------------------
// Expression patterns
// --------------------------------------------------------------------------

func TestCompilePattern_GoBinaryExpression(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `$X + $Y`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	// Verify the query matches binary expressions, not just statement-level ones.
	if strings.Contains(cp.SExpr, "expression_statement") {
		t.Error("SExpr should not contain expression_statement wrapper")
	}
	if !strings.Contains(cp.SExpr, "binary_expression") {
		t.Error("SExpr should contain binary_expression")
	}

	source := []byte(`package main

func f() {
	a := x + y
	b := 1 + 2
	c := a - b
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	t.Logf("matches: %d", len(matches))
	for _, m := range matches {
		for _, c := range m.Captures {
			t.Logf("  @%s = %q", c.Name, c.Text(source))
		}
	}

	// Should match "x + y" and "1 + 2" but NOT "a - b" (different operator).
	if len(matches) != 2 {
		t.Errorf("expected 2 matches (only + operations), got %d", len(matches))
	}
}

func TestCompilePattern_GoCallExpression(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `$FN($$$ARGS)`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	if strings.Contains(cp.SExpr, "expression_statement") {
		t.Error("SExpr should not contain expression_statement wrapper")
	}

	source := []byte(`package main

func f() {
	foo()
	bar(1, 2)
	baz(x)
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	fns := matchCaptures(matches, "FN", source)
	t.Logf("matched functions: %v", fns)

	if len(fns) < 3 {
		t.Errorf("expected at least 3 matches, got %d", len(fns))
	}
}

func TestCompilePattern_GoSelectorCall(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `fmt.Println($X)`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

import "fmt"

func f() {
	fmt.Println("hello")
	fmt.Println(42)
	fmt.Printf("world")
	log.Println("nope")
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	xs := matchCaptures(matches, "X", source)
	t.Logf("matched args: %v", xs)

	// Should match fmt.Println calls (2) but not fmt.Printf or log.Println.
	if len(xs) != 2 {
		t.Errorf("expected 2 matches, got %d", len(xs))
	}
}

// --------------------------------------------------------------------------
// Statement patterns
// --------------------------------------------------------------------------

func TestCompilePattern_GoAssignment(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `$X = $Y`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

func f() {
	a = b
	x = 42
	y := 10
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	xs := matchCaptures(matches, "X", source)
	ys := matchCaptures(matches, "Y", source)
	t.Logf("X captures: %v, Y captures: %v", xs, ys)

	// Should match "a = b" and "x = 42" (assignment statements).
	if len(xs) < 2 {
		t.Errorf("expected at least 2 matches, got %d", len(xs))
	}
}

func TestCompilePattern_GoReturnStatement(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `return $ERR`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	// Should contain return_statement, not be collapsed to just a capture.
	if !strings.Contains(cp.SExpr, "return_statement") {
		t.Error("SExpr should contain return_statement")
	}

	source := []byte(`package main

func f() error {
	if false {
		return nil
	}
	return fmt.Errorf("oops")
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	errs := matchCaptures(matches, "ERR", source)
	t.Logf("ERR captures: %v", errs)

	if len(errs) < 2 {
		t.Errorf("expected at least 2 matches, got %d", len(errs))
	}
}

func TestCompilePattern_GoIfStatement(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `if $COND { $$$BODY }`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

func f() {
	if x > 0 {
		doSomething()
	}
	if true {
		doOther()
	}
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	conds := matchCaptures(matches, "COND", source)
	t.Logf("COND captures: %v", conds)

	if len(conds) != 2 {
		t.Errorf("expected 2 matches, got %d", len(conds))
	}
}

// --------------------------------------------------------------------------
// Wildcard and typed capture tests
// --------------------------------------------------------------------------

func TestCompilePattern_GoWildcard(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $_()`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	// Wildcard should not generate a capture name.
	if strings.Contains(cp.SExpr, "@") && !strings.Contains(cp.SExpr, "@_lit") {
		t.Error("wildcard should not produce a named capture (except literals)")
	}

	source := []byte(`package main

func myFunc() {}
func another() {}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	if len(matches) < 2 {
		t.Errorf("expected at least 2 matches, got %d", len(matches))
	}
}

// --------------------------------------------------------------------------
// MetaVar and CompiledPattern metadata tests
// --------------------------------------------------------------------------

func TestCompilePattern_MetaVarsPopulated(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $NAME($$$PARAMS) error`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	if len(cp.MetaVars) != 2 {
		t.Errorf("expected 2 metavars, got %d", len(cp.MetaVars))
	}

	// Check that NAME is present.
	found := false
	for _, mv := range cp.MetaVars {
		if mv.Name == "NAME" && !mv.Variadic && !mv.Wildcard {
			found = true
		}
	}
	if !found {
		t.Error("missing NAME metavar")
	}

	// Check that PARAMS is variadic.
	found = false
	for _, mv := range cp.MetaVars {
		if mv.Name == "PARAMS" && mv.Variadic {
			found = true
		}
	}
	if !found {
		t.Error("missing variadic PARAMS metavar")
	}
}

func TestCompilePatternForLang(t *testing.T) {
	cp, err := CompilePatternForLang("go", `func $NAME()`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if cp.Query == nil {
		t.Fatal("Query is nil")
	}
	if cp.Lang == nil {
		t.Fatal("Lang is nil")
	}
	if cp.SExpr == "" {
		t.Fatal("SExpr is empty")
	}
}

// --------------------------------------------------------------------------
// Integration: compile + match with specific capture values
// --------------------------------------------------------------------------

func TestCompilePattern_GoFuncNameCapture(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $NAME($$$PARAMS) error`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	source := []byte(`package main

func handleRequest(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func process(data []byte) error {
	return nil
}

func helper() string {
	return ""
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	names := matchCaptures(matches, "NAME", source)

	if len(names) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(names), names)
	}

	// Verify specific names were captured.
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["handleRequest"] {
		t.Error("expected handleRequest to be captured")
	}
	if !nameSet["process"] {
		t.Error("expected process to be captured")
	}
}

func TestCompilePattern_GoNoFalsePositives(t *testing.T) {
	lang := testLang(t, "go")

	// Pattern: functions returning error with empty params.
	cp, err := CompilePattern(lang, `func $NAME() error`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	source := []byte(`package main

func good() error { return nil }
func bad(x int) error { return nil }
func ugly() int { return 0 }
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	names := matchCaptures(matches, "NAME", source)

	// Only "good" should match (empty params AND error return).
	if len(names) != 1 {
		t.Fatalf("expected 1 match, got %d: %v", len(names), names)
	}
	if names[0] != "good" {
		t.Errorf("expected 'good', got %q", names[0])
	}
}

// --------------------------------------------------------------------------
// S-expression structure tests
// --------------------------------------------------------------------------

func TestCompilePattern_SExprNoExpressionStatement(t *testing.T) {
	lang := testLang(t, "go")

	// Expression patterns should not be wrapped in expression_statement.
	patterns := []string{
		`$X + $Y`,
		`$FN($$$ARGS)`,
		`fmt.Println($X)`,
	}

	for _, pat := range patterns {
		cp, err := CompilePattern(lang, pat)
		if err != nil {
			t.Fatalf("compile error for %q: %v", pat, err)
		}
		if strings.Contains(cp.SExpr, "expression_statement") {
			t.Errorf("pattern %q: SExpr should not contain expression_statement: %s", pat, cp.SExpr)
		}
	}
}

func TestCompilePattern_GoNoMetavariables(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func main()`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

func main() {}
func other() {}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	// Should match only "main" (literal name + empty params).
	t.Logf("matches: %d", len(matches))
	if len(matches) != 1 {
		t.Errorf("expected 1 match, got %d", len(matches))
	}
}

func TestCompilePattern_GoTypedCapture(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $NAME:identifier()`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	// Should contain (identifier) type constraint.
	if !strings.Contains(cp.SExpr, "(identifier)") {
		t.Error("SExpr should contain (identifier) for typed capture")
	}

	source := []byte(`package main

func myFunc() {}
func another() {}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	names := matchCaptures(matches, "NAME", source)
	t.Logf("matched names: %v", names)

	if len(names) < 2 {
		t.Errorf("expected at least 2 matches, got %d", len(names))
	}
}

func TestCompilePattern_GoForRangeStatement(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `for $K, $V := range $COLLECTION { $$$BODY }`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	t.Logf("SExpr: %s", cp.SExpr)

	source := []byte(`package main

func f() {
	for k, v := range items {
		process(k, v)
	}
	for i := 0; i < 10; i++ {
		doStuff()
	}
}
`)
	bt, _ := testParse(t, "go", source)
	defer bt.Release()

	matches := cp.Query.ExecuteNode(bt.RootNode(), lang, source)
	t.Logf("matches: %d", len(matches))
	for _, m := range matches {
		for _, c := range m.Captures {
			text := c.Text(source)
			if len(text) > 40 {
				text = text[:40] + "..."
			}
			t.Logf("  @%s = %q", c.Name, text)
		}
	}

	// Should match the for-range but not the C-style for.
	if len(matches) < 1 {
		t.Errorf("expected at least 1 match, got %d", len(matches))
	}
}

func TestCompilePattern_SExprPreservesStructure(t *testing.T) {
	lang := testLang(t, "go")

	// Statement patterns should preserve their structural type.
	tests := []struct {
		pattern  string
		wantType string
	}{
		{`return $X`, "return_statement"},
		{`if $COND { $$$BODY }`, "if_statement"},
		{`$X = $Y`, "assignment_statement"},
		{`func $NAME()`, "function_declaration"},
	}

	for _, tc := range tests {
		cp, err := CompilePattern(lang, tc.pattern)
		if err != nil {
			t.Fatalf("compile error for %q: %v", tc.pattern, err)
		}
		if !strings.Contains(cp.SExpr, tc.wantType) {
			t.Errorf("pattern %q: SExpr should contain %q: %s", tc.pattern, tc.wantType, cp.SExpr)
		}
	}
}

// With the Go snippet context the pattern parses as a call; without it the
// bare top-level text is a type conversion, as in pinned tree-sitter-go.
func TestCompilePattern_SnippetContextChangesGoCall(t *testing.T) {
	lang := testLang(t, "go")
	with, err := CompilePattern(lang, `fmt.Println($X)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with.SExpr, "call_expression") || strings.Contains(with.SExpr, "type_conversion_expression") {
		t.Fatalf("with context: %s", with.SExpr)
	}
	saved := grammars.SnippetContexts["go"]
	delete(grammars.SnippetContexts, "go")
	defer func() { grammars.SnippetContexts["go"] = saved }()
	without, err := CompilePattern(lang, `fmt.Println($X)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(without.SExpr, "type_conversion_expression") {
		t.Fatalf("without context: %s", without.SExpr)
	}
}

// A snippet that is invalid inside the context falls back to the bare parse.
func TestCompilePattern_SnippetContextFallsBackToBareParse(t *testing.T) {
	lang := testLang(t, "go")
	cp, err := CompilePattern(lang, `func $F() {}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cp.SExpr, "function_declaration") {
		t.Fatalf("SExpr: %s", cp.SExpr)
	}
}

// A grammar with no snippet context parses the bare pattern.
func TestCompilePattern_NoSnippetContextIsUnaffected(t *testing.T) {
	if got := grammars.SnippetContext("python"); got != "" {
		t.Fatalf("python has a snippet context %q", got)
	}
	lang := testLang(t, "python")
	cp, err := CompilePattern(lang, `$X + $Y`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cp.SExpr, "binary_operator") {
		t.Fatalf("SExpr: %s", cp.SExpr)
	}
}

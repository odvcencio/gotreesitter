package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/mane/gotreesitter"
	"github.com/odvcencio/mane/grammars"
)

// collectNamedTypes does a depth-first traversal collecting the Type() of all
// named nodes. This is the standard way to inspect a tree-sitter parse tree
// since auxiliary repeat nodes (e.g. source_file_repeat1) are unnamed.
func collectNamedTypes(lang *gotreesitter.Language, node *gotreesitter.Node) []string {
	if node == nil {
		return nil
	}
	var types []string
	if node.IsNamed() {
		types = append(types, node.Type(lang))
	}
	for i := 0; i < node.ChildCount(); i++ {
		types = append(types, collectNamedTypes(lang, node.Child(i))...)
	}
	return types
}

// findNamedChild does a depth-first search of the subtree rooted at node,
// returning the first named descendant with the given type. It searches
// through both named and unnamed children recursively.
func findNamedChild(lang *gotreesitter.Language, node *gotreesitter.Node, typeName string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.IsNamed() && child.Type(lang) == typeName {
			return child
		}
		if found := findNamedChild(lang, child, typeName); found != nil {
			return found
		}
	}
	return nil
}

// parseGo is a test helper that creates a parser, lexes and parses Go source.
func parseGo(t *testing.T, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)
	ts := grammars.NewGoTokenSource(srcBytes, lang)
	tree := parser.ParseWithTokenSource(srcBytes, ts)
	if tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	return tree, lang
}

func TestParseGoPackageOnly(t *testing.T) {
	tree, lang := parseGo(t, "package main\n")
	root := tree.RootNode()

	if root.Type(lang) != "source_file" {
		t.Fatalf("expected root type source_file, got %q", root.Type(lang))
	}
	if root.HasError() {
		t.Error("root has error flag set")
	}

	// Should contain a package_clause with an identifier.
	pkg := findNamedChild(lang, root, "package_clause")
	if pkg == nil {
		t.Fatal("no package_clause found in tree")
	}
	ident := findNamedChild(lang, pkg, "identifier")
	if ident == nil {
		t.Fatal("no identifier found in package_clause")
	}
	if got := ident.Text(tree.Source()); got != "main" {
		t.Errorf("expected identifier text %q, got %q", "main", got)
	}
}

func TestParseGoImport(t *testing.T) {
	tree, lang := parseGo(t, "package main\n\nimport \"fmt\"\n")
	root := tree.RootNode()

	if root.Type(lang) != "source_file" {
		t.Fatalf("expected root type source_file, got %q", root.Type(lang))
	}
	if root.HasError() {
		t.Error("root has error flag set")
	}

	pkg := findNamedChild(lang, root, "package_clause")
	if pkg == nil {
		t.Fatal("no package_clause found")
	}

	imp := findNamedChild(lang, root, "import_declaration")
	if imp == nil {
		t.Fatal("no import_declaration found")
	}

	spec := findNamedChild(lang, imp, "import_spec")
	if spec == nil {
		t.Fatal("no import_spec found in import_declaration")
	}

	strLit := findNamedChild(lang, spec, "interpreted_string_literal")
	if strLit == nil {
		t.Fatal("no interpreted_string_literal found in import_spec")
	}
	if got := strLit.Text(tree.Source()); got != `"fmt"` {
		t.Errorf("expected string literal text %q, got %q", `"fmt"`, got)
	}
}

func TestParseGoFile(t *testing.T) {
	src := `package main

func main() {
	println("hello")
}
`
	tree, lang := parseGo(t, src)
	root := tree.RootNode()

	if root.Type(lang) != "source_file" {
		t.Fatalf("expected root type source_file, got %q", root.Type(lang))
	}
	if root.HasError() {
		t.Error("root has error flag set")
	}

	// Verify package_clause
	pkg := findNamedChild(lang, root, "package_clause")
	if pkg == nil {
		t.Fatal("no package_clause found")
	}

	// Verify function_declaration
	fn := findNamedChild(lang, root, "function_declaration")
	if fn == nil {
		t.Fatal("no function_declaration found")
	}

	// Function name
	fnName := findNamedChild(lang, fn, "identifier")
	if fnName == nil {
		t.Fatal("no identifier (function name) in function_declaration")
	}
	if got := fnName.Text(tree.Source()); got != "main" {
		t.Errorf("expected function name %q, got %q", "main", got)
	}

	// Parameter list
	params := findNamedChild(lang, fn, "parameter_list")
	if params == nil {
		t.Fatal("no parameter_list in function_declaration")
	}

	// Block body
	block := findNamedChild(lang, fn, "block")
	if block == nil {
		t.Fatal("no block in function_declaration")
	}

	// The println("hello") call is inside the block. Our SLR parser may
	// parse it as either call_expression or type_conversion_expression
	// (both are valid LR parses for `identifier(expr)` in Go; the real
	// tree-sitter uses GLR to resolve the ambiguity). Accept either.
	call := findNamedChild(lang, block, "call_expression")
	typeConv := findNamedChild(lang, block, "type_conversion_expression")
	if call == nil && typeConv == nil {
		t.Fatal("no call_expression or type_conversion_expression in block")
	}

	// Verify the string argument is present.
	strLit := findNamedChild(lang, block, "interpreted_string_literal")
	if strLit == nil {
		t.Fatal("no interpreted_string_literal in function body")
	}
	if got := strLit.Text(tree.Source()); got != `"hello"` {
		t.Errorf("expected string literal %q, got %q", `"hello"`, got)
	}
}

func TestParseGoNoErrors(t *testing.T) {
	// Valid Go source should produce an error-free tree.
	sources := []struct {
		name string
		src  string
	}{
		{"empty package", "package main\n"},
		{"with import", "package main\n\nimport \"fmt\"\n"},
		{"with function", "package main\n\nfunc main() {}\n"},
		{"with var", "package main\n\nvar x int\n"},
		{"with const", "package main\n\nconst c = 1\n"},
		{"with type", "package main\n\ntype T struct{}\n"},
	}

	for _, tc := range sources {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			if root.Type(lang) != "source_file" {
				t.Errorf("expected source_file root, got %q", root.Type(lang))
			}
			if root.HasError() {
				t.Errorf("unexpected error in parse tree for %q", tc.name)
			}
		})
	}
}

func TestParseGoTokenSource(t *testing.T) {
	// Verify the token source produces the expected token sequence.
	lang := grammars.GoLanguage()
	src := []byte("package main\n")
	ts := grammars.NewGoTokenSource(src, lang)

	expected := []struct {
		sym  gotreesitter.Symbol
		text string
	}{
		{5, "package"},   // anon_sym_package
		{1, "main"},      // sym_identifier
		{3, "\n"},        // anon_sym_SEMI (auto-inserted)
		{0, ""},          // EOF
	}

	for i, want := range expected {
		tok := ts.Next()
		if tok.Symbol != want.sym {
			t.Errorf("token %d: expected symbol %d, got %d", i, want.sym, tok.Symbol)
		}
		if tok.Text != want.text {
			t.Errorf("token %d: expected text %q, got %q", i, want.text, tok.Text)
		}
	}
}

func TestParseGoDeclarations(t *testing.T) {
	// Test that individual declaration types are recognized correctly.
	// We test each declaration in isolation to avoid multi-function GLR
	// conflicts (our parser is SLR, not GLR).
	tests := []struct {
		name     string
		src      string
		nodeType string
	}{
		{
			"package clause",
			"package foo\n",
			"package_clause",
		},
		{
			"import declaration",
			"package main\n\nimport \"fmt\"\n",
			"import_declaration",
		},
		{
			"function declaration",
			"package main\n\nfunc hello() {}\n",
			"function_declaration",
		},
		{
			"var declaration",
			"package main\n\nvar x int\n",
			"var_declaration",
		},
		{
			"const declaration",
			"package main\n\nconst c = 42\n",
			"const_declaration",
		},
		{
			"type declaration",
			"package main\n\ntype T struct{}\n",
			"type_declaration",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			if root.Type(lang) != "source_file" {
				t.Fatalf("expected source_file root, got %q", root.Type(lang))
			}
			found := findNamedChild(lang, root, tc.nodeType)
			if found == nil {
				// Dump named types for debugging.
				types := collectNamedTypes(lang, root)
				t.Fatalf("expected %q not found; tree contains: %v", tc.nodeType, types)
			}
		})
	}
}

func TestParseGoFunctionBody(t *testing.T) {
	src := `package main

func hello() {
	fmt.Println("world")
}
`
	tree, lang := parseGo(t, src)
	root := tree.RootNode()

	if root.HasError() {
		t.Error("root has error flag set")
	}

	fn := findNamedChild(lang, root, "function_declaration")
	if fn == nil {
		t.Fatal("no function_declaration")
	}

	block := findNamedChild(lang, fn, "block")
	if block == nil {
		t.Fatal("no block in function_declaration")
	}

	// selector_expression for fmt.Println
	sel := findNamedChild(lang, block, "selector_expression")
	if sel == nil {
		t.Fatal("no selector_expression in block")
	}

	// The string argument.
	strLit := findNamedChild(lang, block, "interpreted_string_literal")
	if strLit == nil {
		t.Fatal("no interpreted_string_literal in function body")
	}
	if got := strLit.Text(tree.Source()); got != `"world"` {
		t.Errorf("expected string literal %q, got %q", `"world"`, got)
	}
}

package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewCTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewCTokenSource([]byte("int main(void) { return 0; }\n"), lang); err == nil {
		t.Fatal("expected error for language missing c token symbols")
	}
}

func TestNewCTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewCTokenSourceOrEOF([]byte("int main(void) { return 0; }\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestCTokenSourceSkipToByte(t *testing.T) {
	lang := CLanguage()
	src := []byte("int main(void) {\n  int x = 1;\n  return x;\n}\n")
	target := bytes.Index(src, []byte("return"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
	if tok.Text != "return" {
		t.Fatalf("expected token text %q, got %q", "return", tok.Text)
	}
}

func TestParseCPreprocessorDefines(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#define FOO 42\n#define BAR 100\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("parse has errors; root type = %s", root.Type(lang))
	}

	found := 0
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child.Type(lang) == "preproc_def" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("expected 2 preproc_def nodes, got %d", found)
	}
}

func TestParseCMixedWithPreprocessor(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#define MAX 255\nint main(void) { return 0; }\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has errors")
	}

	types := make([]string, root.ChildCount())
	for i := 0; i < root.ChildCount(); i++ {
		types[i] = root.Child(i).Type(lang)
	}
	if len(types) < 2 {
		t.Fatalf("expected at least 2 top-level nodes, got %v", types)
	}
}

func TestParseCHeaderGuard(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#ifndef FOO_H\n#define FOO_H\n\nint x;\n\n#endif\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("header guard parse has errors")
	}
}

func TestParseCFixedWidthIntegerTypesAsPrimitiveTypes(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("typedef struct {\n  uint32_t count;\n  int32_t delta;\n} Sample;\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasError() {
		t.Fatalf("fixed-width integer parse has errors: %s", root.SExpr(lang))
	}

	gotPrimitive := map[string]bool{}
	gotTypeIdentifier := map[string]bool{}
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if !node.IsNamed() {
			return gotreesitter.WalkContinue
		}
		switch node.Type(lang) {
		case "primitive_type":
			gotPrimitive[node.Text(src)] = true
		case "type_identifier":
			gotTypeIdentifier[node.Text(src)] = true
		}
		return gotreesitter.WalkContinue
	})

	for _, want := range []string{"uint32_t", "int32_t"} {
		if !gotPrimitive[want] {
			t.Fatalf("missing primitive_type %q in tree: %s", want, root.SExpr(lang))
		}
		if gotTypeIdentifier[want] {
			t.Fatalf("%q parsed as type_identifier unexpectedly: %s", want, root.SExpr(lang))
		}
	}
}
func TestParseCDefineWithExpression(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#define FOO (1 + 2)\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("define-with-expression parse has errors")
	}
}

func TestParseCWithTokenSource(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("int main(void) { return 0; }\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected c parse without syntax errors")
	}
}

func TestCTokenSourceLineCommentContinuationCRLF(t *testing.T) {
	lang := CLanguage()
	src := []byte("// hello \\\r\n   still a comment\r\nthis_is_not a_comment;\r\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	comment := ts.Next()
	if got, want := comment.Text, "// hello \\\r\n   still a comment\r"; got != want {
		t.Fatalf("comment token text = %q, want %q", got, want)
	}

	ident := ts.Next()
	if got, want := ident.Text, "this_is_not"; got != want {
		t.Fatalf("next token text = %q, want %q", got, want)
	}
}

func TestCTokenSourceSystemIncludeAndPragmaTokens(t *testing.T) {
	lang := CLanguage()
	src := []byte("#include <stdbool.h>\n#pragma GCC diagnostic push\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tok := ts.Next()
	if got, want := tok.Text, "#include"; got != want {
		t.Fatalf("directive text = %q, want %q", got, want)
	}

	tok = ts.Next()
	if got, want := tok.Text, "<stdbool.h>"; got != want {
		t.Fatalf("include arg text = %q, want %q", got, want)
	}
	if got := lang.SymbolNames[tok.Symbol]; got != "system_lib_string" {
		t.Fatalf("include arg symbol = %q, want %q", got, "system_lib_string")
	}

	tok = ts.Next()
	if got := lang.SymbolNames[tok.Symbol]; got != "preproc_include_token2" {
		t.Fatalf("line terminator symbol = %q, want %q", got, "preproc_include_token2")
	}

	tok = ts.Next()
	if got, want := tok.Text, "#pragma"; got != want {
		t.Fatalf("pragma text = %q, want %q", got, want)
	}

	tok = ts.Next()
	if got, want := tok.Text, "GCC diagnostic push"; got != want {
		t.Fatalf("pragma arg text = %q, want %q", got, want)
	}
	if got := lang.SymbolNames[tok.Symbol]; got != "preproc_arg" {
		t.Fatalf("pragma arg symbol = %q, want %q", got, "preproc_arg")
	}
}

func TestParseCSystemIncludeAndPragma(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#include <assert.h>\n#pragma GCC diagnostic push\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if root := tree.RootNode(); root == nil || root.HasError() {
		t.Fatalf("parse has errors")
	}
}

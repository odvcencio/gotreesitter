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

func TestCTokenSourceFunctionLikeMacroTokenSequence(t *testing.T) {
	lang := CLanguage()
	src := []byte("#define LOG(...) fprintf(stderr, __VA_ARGS__)\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	var got []string
	for {
		tok := ts.Next()
		if tok.Symbol == 0 {
			break
		}
		got = append(got, lang.SymbolNames[tok.Symbol])
	}

	want := []string{"#define", "identifier", "(", "...", ")", "preproc_arg", "preproc_include_token2"}
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}

func TestParseCFunctionLikeMacro(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#define LOG(...) fprintf(stderr, __VA_ARGS__)\n")
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
		t.Fatalf("function-like macro parse has errors; root type = %s", root.Type(lang))
	}

	found := false
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.Type(lang) == "preproc_function_def" {
			found = true
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if !found {
		t.Fatalf("expected preproc_function_def in tree, got %s", root.SExpr(lang))
	}
}

func TestParseCMultilineFunctionLikeMacro(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#define LOG(...) \\\n  fprintf(stderr, __VA_ARGS__)\n")
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
		t.Fatalf("multiline function-like macro parse has errors; root type = %s", root.Type(lang))
	}

	found := false
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.Type(lang) == "preproc_function_def" {
			found = true
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if !found {
		t.Fatalf("expected preproc_function_def in tree, got %s", root.SExpr(lang))
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

func TestParseCSignedIntegerLiteralsAsNumberLiterals(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("int f(int a) {\n  switch (a) {\n    case -1:\n      return +2;\n    default:\n      return 0;\n  }\n}\n")
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
		t.Fatalf("signed integer parse has errors: %s", root.SExpr(lang))
	}

	gotNumber := map[string]bool{}
	gotUnary := map[string]bool{}
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if !node.IsNamed() {
			return gotreesitter.WalkContinue
		}
		switch node.Type(lang) {
		case "number_literal":
			gotNumber[node.Text(src)] = true
		case "unary_expression":
			gotUnary[node.Text(src)] = true
		}
		return gotreesitter.WalkContinue
	})

	for _, want := range []string{"-1", "+2"} {
		if !gotNumber[want] {
			t.Fatalf("missing number_literal %q in tree: %s", want, root.SExpr(lang))
		}
		if gotUnary[want] {
			t.Fatalf("%q parsed as unary_expression unexpectedly: %s", want, root.SExpr(lang))
		}
	}
}

func TestParseCSubtractionKeepsBinaryExpression(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("int f(int a) {\n  return a - 1;\n}\n")
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
		t.Fatalf("subtraction parse has errors: %s", root.SExpr(lang))
	}

	foundBinary := false
	foundSignedLiteral := false
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if !node.IsNamed() {
			return gotreesitter.WalkContinue
		}
		switch node.Type(lang) {
		case "binary_expression":
			foundBinary = true
		case "number_literal":
			if node.Text(src) == "-1" {
				foundSignedLiteral = true
			}
		}
		return gotreesitter.WalkContinue
	})

	if !foundBinary {
		t.Fatalf("expected binary_expression in tree: %s", root.SExpr(lang))
	}
	if foundSignedLiteral {
		t.Fatalf("unexpected signed number_literal in subtraction tree: %s", root.SExpr(lang))
	}
}

func TestCTokenSourceEmitsGenericEndifInsideLinkageSpecification(t *testing.T) {
	lang := CLanguage()
	src := []byte("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\nint x;\n\n#ifdef __cplusplus\n}\n#endif\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	ts.cur = newSourceCursor(src)
	ts.cur.advanceBytes(32)
	ts.SetParserState(35)

	tok := ts.Next()
	if got, want := lang.SymbolNames[tok.Symbol], "preproc_directive"; got != want {
		t.Fatalf("directive token = %q, want %q", got, want)
	}
	if got, want := tok.Text, "#endif"; got != want {
		t.Fatalf("directive text = %q, want %q", got, want)
	}

	tok = ts.Next()
	if got, want := lang.SymbolNames[tok.Symbol], "preproc_include_token2"; got != want {
		t.Fatalf("line terminator token = %q, want %q", got, want)
	}
}

func TestCTokenSourceInsertsMissingEndifBeforeBraceWrappedClose(t *testing.T) {
	lang := CLanguage()
	src := []byte("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\nint x;\n\n#ifdef __cplusplus\n}\n#endif\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	ts.cur = newSourceCursor(src)
	ts.cur.advanceBytes(66)
	ts.SetParserState(10)

	tok := ts.Next()
	if got, want := lang.SymbolNames[tok.Symbol], "#endif"; got != want {
		t.Fatalf("synthetic token = %q, want %q", got, want)
	}
	if got, want := tok.StartByte, uint32(66); got != want {
		t.Fatalf("synthetic token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(66); got != want {
		t.Fatalf("synthetic token end = %d, want %d", got, want)
	}
	if tok.Text != "" {
		t.Fatalf("synthetic token text = %q, want empty", tok.Text)
	}
}

func TestCTokenSourceConditionalExprEmitsLineTerminator(t *testing.T) {
	lang := CLanguage()
	src := []byte("#elif defined(__GNUC__) || defined(__clang__)\n#pragma GCC diagnostic push\n")
	ts, err := NewCTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}

	var got []string
	for {
		tok := ts.Next()
		if tok.Symbol == 0 {
			break
		}
		got = append(got, lang.SymbolNames[tok.Symbol])
	}

	pragmaIndex := -1
	lineTermIndex := -1
	for i, sym := range got {
		if sym == "\n" && lineTermIndex < 0 {
			lineTermIndex = i
		}
		if (sym == "#pragma" || sym == "preproc_directive") && pragmaIndex < 0 {
			pragmaIndex = i
		}
	}
	if lineTermIndex < 0 {
		t.Fatalf("missing conditional newline token in token stream: %v", got)
	}
	if pragmaIndex < 0 {
		t.Fatalf("missing directive after conditional expression: %v", got)
	}
	if got, want := lineTermIndex, pragmaIndex-1; got != want {
		t.Fatalf("line terminator index = %d, want %d; tokens=%v", got, want, got)
	}
}

func TestParseCPreprocConditionalPragmasWithTokenSource(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#ifdef _MSC_VER\n#pragma warning(push)\n#pragma warning(disable : 4101)\n#elif defined(__GNUC__) || defined(__clang__)\n#pragma GCC diagnostic push\n#pragma GCC diagnostic ignored \"-Wunused-variable\"\n#endif\n")
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
		t.Fatalf("conditional pragma parse has errors: %s", root.SExpr(lang))
	}

	preprocElifCount := 0
	preprocCallCount := 0
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		switch node.Type(lang) {
		case "preproc_elif":
			preprocElifCount++
		case "preproc_call":
			preprocCallCount++
		}
		return gotreesitter.WalkContinue
	})
	if got, want := preprocElifCount, 1; got != want {
		t.Fatalf("preproc_elif count = %d, want %d: %s", got, want, root.SExpr(lang))
	}
	if got, want := preprocCallCount, 4; got != want {
		t.Fatalf("preproc_call count = %d, want %d: %s", got, want, root.SExpr(lang))
	}
}

func TestParseCExternCWrapperWithTokenSource(t *testing.T) {
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\nint x;\n\n#ifdef __cplusplus\n}\n#endif\n")
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
	if !root.HasError() {
		t.Fatalf("extern wrapper parse should preserve the missing conditional close: %s", root.SExpr(lang))
	}
	if got, want := root.NamedChildCount(), 1; got != want {
		t.Fatalf("root NamedChildCount = %d, want %d: %s", got, want, root.SExpr(lang))
	}

	foundLinkage := false
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.Type(lang) == "linkage_specification" {
			foundLinkage = true
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if !foundLinkage {
		t.Fatalf("expected linkage_specification in tree, got %s", root.SExpr(lang))
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

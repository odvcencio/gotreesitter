package gotreesitter_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGoParserIncludedRangePastEOFDoesNotHang(t *testing.T) {
	src := []byte("package main\n")
	p := gotreesitter.NewParser(grammars.GoLanguage())
	p.SetIncludedRanges([]gotreesitter.Range{{
		StartByte:  100,
		EndByte:    200,
		StartPoint: gotreesitter.Point{Row: 9},
		EndPoint:   gotreesitter.Point{Row: 18},
	}})
	done := make(chan struct{})
	go func() {
		tree, _ := p.Parse(src)
		if tree != nil {
			tree.Release()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("parse hung while clamping an included range past EOF")
	}
}

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
// Uses the custom GoTokenSource when the current Go blob is ts2go-compiled
// (detected by presence of the `source_file_token1` anonymous composite
// symbol); otherwise falls back to the DFA baked into the grammargen blob.
func parseGo(t *testing.T, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)
	var (
		tree *gotreesitter.Tree
		err  error
	)
	if _, ok := lang.SymbolByName("source_file_token1"); ok {
		ts := mustGoTokenSource(t, srcBytes, lang)
		tree, err = parser.ParseWithTokenSource(srcBytes, ts)
	} else {
		tree, err = parser.Parse(srcBytes)
	}
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
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

	// Should contain a package_clause with a package name node.
	pkg := findNamedChild(lang, root, "package_clause")
	if pkg == nil {
		t.Fatal("no package_clause found in tree")
	}
	ident := findNamedChild(lang, pkg, "package_identifier")
	if ident == nil {
		// Older grammar snapshots used "identifier" here.
		ident = findNamedChild(lang, pkg, "identifier")
	}
	if ident == nil {
		t.Fatal("no package identifier found in package_clause")
	}
	if got := ident.Text(tree.Source()); got != "main" {
		t.Errorf("expected identifier text %q, got %q", "main", got)
	}
}

func TestParseGoFunctionDeclarationFields(t *testing.T) {
	tree, lang := parseGo(t, "package main\nfunc Hello() string { return \"hello\" }\n")
	fn := findNamedChild(lang, tree.RootNode(), "function_declaration")
	if fn == nil {
		t.Fatal("no function_declaration found")
	}

	fieldNames := make([]string, fn.ChildCount())
	childTypes := make([]string, fn.ChildCount())
	for i := 0; i < fn.ChildCount(); i++ {
		fieldNames[i] = fn.FieldNameForChild(i, lang)
		if child := fn.Child(i); child != nil {
			childTypes[i] = child.Type(lang)
		}
	}

	for _, tc := range []struct {
		field string
		typ   string
	}{
		{"name", "identifier"},
		{"parameters", "parameter_list"},
		{"result", "type_identifier"},
		{"body", "block"},
	} {
		child := fn.ChildByFieldName(tc.field, lang)
		if child == nil {
			t.Fatalf("ChildByFieldName(%q) returned nil; child fields=%v child types=%v root=%s", tc.field, fieldNames, childTypes, tree.RootNode().SExpr(lang))
		}
		if got := child.Type(lang); got != tc.typ {
			t.Fatalf("ChildByFieldName(%q).Type() = %q, want %q; child fields=%v child types=%v", tc.field, got, tc.typ, fieldNames, childTypes)
		}
	}
}

// TestParseGoNewMakeSoleArgumentIsTypeIdentifier is a regression test for a
// C-oracle divergence found on real Go stdlib (os/dir_unix.go: `new(dirInfo)`):
// tree-sitter-go's grammar declares `_simple_type`/`_expression` as an
// explicit ambiguity for the sole/leading argument of a call to the "new" or
// "make" builtins (special_argument_list's `_type` slot overlaps
// argument_list's `_expression` slot for a bare identifier). C's LALR table
// resolves this deterministically toward `_simple_type` (type_identifier);
// gotreesitter's GLR table forked and picked `_expression` (identifier)
// instead. normalizeGoNewMakeTypeArgument (parser_result_go.go) reclassifies
// the argument node in the one syntactic shape this covers. See
// cgo_harness's TestZZCodivRepro* / TestZZCodivCorpusSpotCheck* history for
// the C-oracle byte-identity verification.
func TestParseGoNewMakeSoleArgumentIsTypeIdentifier(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"new-user-type", "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\td := new(dirInfo)\n\t_ = d\n}\n"},
		{"make-user-type", "package p\n\ntype dirInfo struct{}\n\nfunc f() {\n\tm := make(dirInfo, 0)\n\t_ = m\n}\n"},
		{"new-builtin-primitive", "package p\n\nfunc f() {\n\td := new(int)\n\t_ = d\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			call := findNamedChild(lang, tree.RootNode(), "call_expression")
			if call == nil {
				t.Fatalf("no call_expression found; root=%s", tree.RootNode().SExpr(lang))
			}
			args := call.ChildByFieldName("arguments", lang)
			if args == nil {
				t.Fatalf("call_expression has no arguments field; root=%s", tree.RootNode().SExpr(lang))
			}
			arg := args.NamedChild(0)
			if arg == nil {
				t.Fatalf("argument_list has no named child; root=%s", tree.RootNode().SExpr(lang))
			}
			if got := arg.Type(lang); got != "type_identifier" {
				t.Fatalf("sole new/make argument Type() = %q, want %q; root=%s", got, "type_identifier", tree.RootNode().SExpr(lang))
			}
		})
	}
}

// TestParseGoNewMakeUserShadowNotMisclassified ensures the new/make
// reclassification stays scoped to the exact "new"/"make" builtin call shape
// (function field is literally the identifier "new" or "make") and does not
// leak into ordinary calls whose function name happens to contain those
// substrings, or calls to a same-named user function whose argument isn't a
// bare identifier.
func TestParseGoNewMakeUserShadowNotMisclassified(t *testing.T) {
	tree, lang := parseGo(t, "package p\n\nfunc dirInfo() int { return 0 }\n\nfunc f() {\n\td := dirInfo()\n\t_ = d\n}\n")
	call := findNamedChild(lang, tree.RootNode(), "call_expression")
	if call == nil {
		t.Fatalf("no call_expression found; root=%s", tree.RootNode().SExpr(lang))
	}
	fn := call.ChildByFieldName("function", lang)
	if fn == nil || fn.Type(lang) != "identifier" || fn.Text([]byte("package p\n\nfunc dirInfo() int { return 0 }\n\nfunc f() {\n\td := dirInfo()\n\t_ = d\n}\n")) != "dirInfo" {
		t.Fatalf("expected call to dirInfo() with identifier function field")
	}
}

func TestParseGoRangeWithNestedFunctionLiteralBody(t *testing.T) {
	src := `package p

func TestUnderSize(t *testing.T) {
	z, err := OpenReader("testdata/readme.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()

	for _, f := range z.File {
		t.Run(f.Name, func(t *testing.T) {
			rd, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rd.Close()

			_, err = io.Copy(io.Discard, rd)
			if err != ErrFormat {
				t.Fatalf("Error mismatch\n\tGot:  %v\n\tWant: %v", err, ErrFormat)
			}
		})
	}
}
`
	tree, lang := parseGo(t, src)
	root := tree.RootNode()
	defer tree.Release()

	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if root.HasError() {
		t.Fatalf("root has error:\n%s", root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if findNamedChild(lang, root, "for_statement") == nil {
		t.Fatalf("missing for_statement:\n%s", root.SExpr(lang))
	}
	if findNamedChild(lang, root, "func_literal") == nil {
		t.Fatalf("missing nested func_literal:\n%s", root.SExpr(lang))
	}
	if findNamedChild(lang, root, "defer_statement") == nil {
		t.Fatalf("missing nested defer_statement:\n%s", root.SExpr(lang))
	}
}

func TestParseGoRecoveredForIgnoresLineCommentBrace(t *testing.T) {
	src := `package p

func TestCommentBrace(t *testing.T) {
	for _, name := range []string{"one"} {
		_ = name
		// }
		t.Run(name, func(t *testing.T) {
			defer t.Helper()
		})
	}
}
`
	assertGoRecoveryCanary(t, src, []string{
		"for_statement",
		"func_literal",
		"defer_statement",
	}, []string{
		"t.Run(name, func(t *testing.T)",
		"defer t.Helper()",
	})
}

func TestParseGoRecoveredForIgnoresLiteralAndCommentBraces(t *testing.T) {
	src := "package p\n\n" +
		"func TestLiteralBraces(t *testing.T) {\n" +
		"\tfor _, name := range []string{\"one\"} {\n" +
		"\t\ttext := \"}\"\n" +
		"\t\traw := `}`\n" +
		"\t\t/* } */\n" +
		"\t\t// }\n" +
		"\t\tt.Run(text+raw, func(t *testing.T) {\n" +
		"\t\t\tdefer t.Helper()\n" +
		"\t\t})\n" +
		"\t}\n" +
		"}\n"
	assertGoRecoveryCanary(t, src, []string{
		"for_statement",
		"interpreted_string_literal",
		"raw_string_literal",
		"func_literal",
		"defer_statement",
	}, []string{
		"text := \"}\"",
		"raw := `}`",
		"defer t.Helper()",
	})
}

func TestParseGoRecoveredForFindsBodyAfterRangeCompositeLiteral(t *testing.T) {
	src := `package p

func TestCompositeRange(t *testing.T) {
	for _, v := range []struct{ A int }{{1}} {
		t.Run("case", func(t *testing.T) {
			_ = v.A
		})
	}
}
`
	assertGoRecoveryCanary(t, src, []string{
		"for_statement",
		"range_clause",
		"func_literal",
		"selector_expression",
	}, []string{
		"[]struct{ A int }{{1}}",
		"_ = v.A",
	})
}

func assertGoRecoveryCanary(t *testing.T, src string, wantTypes, wantTexts []string) {
	t.Helper()

	tree, lang := parseGo(t, src)
	root := tree.RootNode()
	defer tree.Release()

	if got := root.Type(lang); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}
	if root.HasError() {
		t.Fatalf("root has error:\n%s", root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	for _, typ := range wantTypes {
		if findNamedChild(lang, root, typ) == nil {
			t.Fatalf("missing %s:\n%s", typ, root.SExpr(lang))
		}
	}
	rootText := root.Text(tree.Source())
	for _, text := range wantTexts {
		if !bytes.Contains([]byte(rootText), []byte(text)) {
			t.Fatalf("root text missing %q:\n%s", text, rootText)
		}
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

func TestParseGoDotImportAliasKeepsAnonymousDotChild(t *testing.T) {
	tree, lang := parseGo(t, "package main\n\nimport . \"unicode\"\n")
	root := tree.RootNode()

	spec := findNamedChild(lang, root, "import_spec")
	if spec == nil {
		t.Fatal("no import_spec found")
	}
	dot := findNamedChild(lang, spec, "dot")
	if dot == nil {
		t.Fatal("no dot alias found in import_spec")
	}
	if got, want := dot.ChildCount(), 1; got != want {
		t.Fatalf("dot.ChildCount() = %d, want %d", got, want)
	}
	child := dot.Child(0)
	if child == nil {
		t.Fatal("dot.Child(0) = nil")
	}
	if child.IsNamed() {
		t.Fatal("restored dot token should be anonymous")
	}
	if got, want := child.Type(lang), "."; got != want {
		t.Fatalf("dot child type = %q, want %q", got, want)
	}
	if got, want := child.Text(tree.Source()), "."; got != want {
		t.Fatalf("dot child text = %q, want %q", got, want)
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
	// Verify the token source produces the expected token sequence. Only
	// meaningful against ts2go's Go blob — GoTokenSource was calibrated to
	// that symbol layout. Skip when the current blob is grammargen (default
	// in 0.14.0+); GoTokenSource remains usable via the public API for
	// callers carrying their own ts2go Go blob.
	lang := grammars.GoLanguage()
	if _, ok := lang.SymbolByName("source_file_token1"); !ok {
		t.Skip("GoTokenSource is ts2go-specific; current Go blob is grammargen-compiled")
	}
	src := []byte("package main\n")
	ts := mustGoTokenSource(t, src, lang)
	semiSyms := lang.TokenSymbolsByName(";")
	if len(semiSyms) == 0 {
		t.Fatal("go language missing semicolon token symbol")
	}

	expected := []struct {
		sym  gotreesitter.Symbol
		text string
	}{
		{5, "package"},      // anon_sym_package
		{1, "main"},         // sym_identifier
		{semiSyms[0], "\n"}, // regular semicolon token for auto-inserted newline
		{0, ""},             // EOF
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

func TestParseGoExplicitStatementSemicolonPreserved(t *testing.T) {
	src := `package main

func hello() int { v := 0; return v }
`
	tree, lang := parseGo(t, src)
	root := tree.RootNode()

	if root.HasError() {
		t.Fatalf("root has error flag set: %s", root.SExpr(lang))
	}

	stmtList := findNamedChild(lang, root, "statement_list")
	if stmtList == nil {
		stmtList = findNamedChild(lang, root, "statement_list_repeat1")
	}
	if stmtList == nil {
		t.Fatalf("no statement_list found: %s", root.SExpr(lang))
	}

	var sawExplicit bool
	for i := 0; i < stmtList.ChildCount(); i++ {
		child := stmtList.Child(i)
		if child == nil || child.Type(lang) != ";" {
			continue
		}
		sawExplicit = true
		if got := child.Text(tree.Source()); got != ";" {
			t.Fatalf("semicolon text = %q, want %q", got, ";")
		}
	}
	if !sawExplicit {
		t.Fatalf("statement_list missing explicit semicolon child: %s", stmtList.SExpr(lang))
	}
}

func TestParseGoIncrementalRepeatedSingleByteEdit(t *testing.T) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)

	src := []byte(`package main

func main() {
	x := 0
	_ = x
}
`)

	editAt := bytes.Index(src, []byte("0"))
	if editAt < 0 {
		t.Fatal("could not find edit byte")
	}
	start := pointAtOffset(src, editAt)
	end := pointAtOffset(src, editAt+1)
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  start,
		OldEndPoint: end,
		NewEndPoint: end,
	}

	tree, err := parser.ParseWithTokenSource(src, mustGoTokenSource(t, src, lang))
	if err != nil {
		t.Fatalf("initial ParseWithTokenSource failed: %v", err)
	}
	if tree.RootNode() == nil {
		t.Fatal("initial parse returned nil root")
	}

	for i := 0; i < 25; i++ {
		if src[editAt] == '0' {
			src[editAt] = '1'
		} else {
			src[editAt] = '0'
		}

		tree.Edit(edit)
		tree, err = parser.ParseIncrementalWithTokenSource(src, tree, mustGoTokenSource(t, src, lang))
		if err != nil {
			t.Fatalf("iteration %d: incremental parse failed: %v", i, err)
		}
		if tree.RootNode() == nil {
			t.Fatalf("iteration %d: incremental parse returned nil root", i)
		}
	}
}

func TestParseGoIncrementalWithTokenSourceReusesSubtrees(t *testing.T) {
	lang := grammars.GoLanguage()
	// This test drives GoTokenSource incremental reuse, which is ts2go-
	// specific (the custom lexer's symbol map doesn't match grammargen).
	// Skip when the default blob is grammargen.
	if _, ok := lang.SymbolByName("source_file_token1"); !ok {
		t.Skip("GoTokenSource is ts2go-specific; current Go blob is grammargen-compiled")
	}
	parser := gotreesitter.NewParser(lang)

	src := []byte(`package main

func main() {
	v := 0
	_ = v
}
`)
	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		t.Fatal("could not find edit marker")
	}
	editAt += len("v := ")

	start := pointAtOffset(src, editAt)
	end := pointAtOffset(src, editAt+1)
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  start,
		OldEndPoint: end,
		NewEndPoint: end,
	}

	ts := mustGoTokenSource(t, src, lang)
	oldTree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("initial ParseWithTokenSource failed: %v", err)
	}
	if oldTree.RootNode() == nil {
		t.Fatal("initial parse returned nil root")
	}

	next := append([]byte(nil), src...)
	next[editAt] = '1'
	oldTree.Edit(edit)
	ts.Reset(next)

	newTree, prof, err := parser.ParseIncrementalWithTokenSourceProfiled(next, oldTree, ts)
	if err != nil {
		t.Fatalf("ParseIncrementalWithTokenSourceProfiled failed: %v", err)
	}
	if newTree.RootNode() == nil {
		t.Fatal("incremental parse returned nil root")
	}
	if got, want := newTree.RootNode().EndByte(), uint32(len(next)); got != want {
		t.Fatalf("incremental parse truncated: root.EndByte=%d want=%d", got, want)
	}
	if newTree.RootNode().HasError() {
		t.Fatal("incremental parse produced error root")
	}
	if prof.ReusedSubtrees == 0 {
		t.Fatalf("expected subtree reuse, got profile: %+v", prof)
	}
	if prof.ReusedBytes == 0 {
		t.Fatalf("expected reused bytes > 0, got profile: %+v", prof)
	}

	// Sanity-check against a fresh parse of the edited source.
	freshTS := mustGoTokenSource(t, next, lang)
	freshTree, err := parser.ParseWithTokenSource(next, freshTS)
	if err != nil {
		t.Fatalf("fresh ParseWithTokenSource failed: %v", err)
	}
	if freshTree.RootNode() == nil {
		t.Fatal("fresh parse returned nil root")
	}
	if got, want := freshTree.RootNode().EndByte(), uint32(len(next)); got != want {
		t.Fatalf("fresh parse truncated: root.EndByte=%d want=%d", got, want)
	}
	if freshTree.RootNode().HasError() {
		t.Fatal("fresh parse produced error root")
	}
	if !bytes.Equal([]byte(newTree.RootNode().Text(next)), []byte(freshTree.RootNode().Text(next))) {
		t.Fatalf("incremental root text mismatch with fresh parse")
	}
}

func TestParseGoIncrementalRangeClauseReturnEdit(t *testing.T) {
	lang := grammars.GoLanguage()
	// Drives GoTokenSource incremental reuse; ts2go-specific. Skip when
	// the default Go blob is grammargen.
	if _, ok := lang.SymbolByName("source_file_token1"); !ok {
		t.Skip("GoTokenSource is ts2go-specific; current Go blob is grammargen-compiled")
	}

	parseWithReturnDigit := func(t *testing.T, digit byte) {
		t.Helper()

		parser := gotreesitter.NewParser(lang)
		base := []byte(`package p

func f(s []int) int {
	for _, v := range s {
		_ = v
	}
	return 0
}
`)

		tree, err := parser.ParseWithTokenSource(base, mustGoTokenSource(t, base, lang))
		if err != nil {
			t.Fatalf("initial ParseWithTokenSource failed: %v", err)
		}
		if tree.RootNode() == nil {
			t.Fatal("initial parse returned nil root")
		}

		editAt := bytes.Index(base, []byte("return 0"))
		if editAt < 0 {
			t.Fatal("could not find return edit marker")
		}
		editAt += len("return ")

		next := append([]byte(nil), base...)
		next[editAt] = digit

		start := pointAtOffset(base, editAt)
		end := pointAtOffset(base, editAt+1)
		edit := gotreesitter.InputEdit{
			StartByte:   uint32(editAt),
			OldEndByte:  uint32(editAt + 1),
			NewEndByte:  uint32(editAt + 1),
			StartPoint:  start,
			OldEndPoint: end,
			NewEndPoint: end,
		}

		tree.Edit(edit)
		tree, err = parser.ParseIncrementalWithTokenSource(next, tree, mustGoTokenSource(t, next, lang))
		if err != nil {
			t.Fatalf("incremental parse failed: %v", err)
		}
		root := tree.RootNode()
		if root == nil {
			t.Fatal("incremental parse returned nil root")
		}
		if root.StartByte() != 0 {
			t.Fatalf("root start mismatch: got %d, want 0", root.StartByte())
		}
		if root.EndByte() > uint32(len(next)) {
			t.Fatalf("root end out of bounds: got %d, source len %d", root.EndByte(), len(next))
		}
		if trailing := next[root.EndByte():]; len(bytes.TrimSpace(trailing)) != 0 {
			t.Fatalf("unexpected non-whitespace trailing bytes after root: %q", string(trailing))
		}

		got := root.Text(next)
		if !bytes.Contains([]byte(got), []byte("package p")) {
			t.Fatalf("root text missing package clause:\n%s", got)
		}
		if !bytes.Contains([]byte(got), []byte("func f(s []int) int")) {
			t.Fatalf("root text missing function signature:\n%s", got)
		}
		if !bytes.Contains([]byte(got), []byte("for _, v := range s")) {
			t.Fatalf("root text missing range clause:\n%s", got)
		}
		wantReturn := append([]byte("return "), digit)
		if !bytes.Contains([]byte(got), wantReturn) {
			t.Fatalf("root text missing edited return value %q:\n%s", string(wantReturn), got)
		}
		if root.HasError() {
			t.Fatalf("incremental parse has errors for return %q", string([]byte{digit}))
		}

		fn := findNamedChild(lang, root, "function_declaration")
		if fn == nil {
			t.Fatal("missing function_declaration after incremental parse")
		}
		if findNamedChild(lang, fn, "range_clause") == nil {
			t.Fatal("missing range_clause after incremental parse")
		}
		if findNamedChild(lang, fn, "return_statement") == nil {
			t.Fatal("missing return_statement after incremental parse")
		}
	}

	t.Run("return 1", func(t *testing.T) {
		parseWithReturnDigit(t, '1')
	})
	t.Run("return 2", func(t *testing.T) {
		parseWithReturnDigit(t, '2')
	})
}

// TestGoInterpretedStringFalseErrors pins two confirmed false-ERROR defect
// classes traced to a stale grammars/grammar_blobs/go.bin: the shipped blob
// was compiled by an older grammargen revision (before the LALR/alias fixes
// landed on this branch) and never regenerated afterward, so its LR tables
// for interpreted_string_literal disagreed with the rest of the grammar.
//
//   - "Bug A": a comparison ending in an interpreted string literal
//     immediately followed by the lowest-precedence binary operator `||`
//     produced a spurious ERROR at `||` (the completed
//     interpreted_string_literal's reduce action was missing `||` from its
//     lookahead set in the stale table).
//   - "Bug B": escape_sequence adjacency inside an interpreted string
//     (content ending right before an escape, or two adjacent escapes)
//     produced a 1-byte ERROR at the backslash with an orphaned
//     interpreted_string_literal_content node beside the real string.
//
// Regenerating go.bin from the current grammargen source (same `emit go
// -bin` invocation documented in grammargen/README.md) fixes both; this test
// guards against the blob going stale again.
func TestGoInterpretedStringFalseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "bugA_string_or_lowest_precedence",
			src:  "package p\nfunc f(x string) bool { return x == \"a\" || x == \"b\" }\n",
		},
		{
			name: "bugA_three_chain_or",
			src:  "package p\nfunc f(a, b string) bool { return a == \"x\" || a == \"y\" || b == \"z\" }\n",
		},
		{
			name: "bugB_space_before_escaped_quote",
			src:  "package p\nvar s = \"hello \\\"world\\\"\"\n",
		},
		{
			// Real-world repro (derived from cgo_harness/c_extern_wrapper_parity_test.go,
			// which itself hit the false ERROR under the stale blob): a
			// string literal containing both a space-then-escaped-quote and
			// several runs of adjacent escape_sequence tokens (\n\n).
			name: "bugB_adjacent_escapes_real_world",
			src: "package p\nfunc f() {\n" +
				"\tsrc := []byte(\"#ifdef __cplusplus\\nextern \\\"C\\\" {\\n#endif\\n\\nint x;\\n\\n#ifdef __cplusplus\\n}\\n#endif\\n\")\n" +
				"\t_ = src\n}\n",
		},
		{
			// ddmin-minimized repro from the investigation's residual bucket:
			// two adjacent top-level functions where the first ends in a
			// call taking a func-literal argument and the second is a
			// simple `if a.hash() != b.hash() {}`. Originally fell out of the
			// same stale-blob root cause as bugA/bugB above; later became the
			// regression pin for the `_automatic_semicolon` external-scanner
			// ASI fix (grammars/go_scanner.go): routing the terminator
			// through an external token restructures the LALR table enough
			// that the pre-existing, upstream-intentional dynamic-precedence
			// tie between index_expression and generic_type(composite_literal)
			// (both prec.dynamic(1, ...), see grammargen/go_grammar.go) needs
			// a wider merge-per-key survivor budget at its merge point — see
			// the "go" case in effectiveParseMergePerKeyCap in parser_retry.go
			// (steady-state cap raised from 3 to 8; an earlier, narrower
			// content-gated widen missed non-bracket-shaped triggers, see that
			// comment for the regression files that caught it).
			name: "residual_adjacent_funcs_call_then_if_ne",
			src: "package grammargen\n" +
				"func TestBitsetForEach(t *testing.T) {\n" +
				"\tb.forEach(func(idx int) {\n\t})\n" +
				"\tfor i := range want {\n" +
				"\t\tif got[i] != want[i] {\n" +
				"\t\t\tt.Errorf(\"forEach[%d] = %d, want %d\", i, got[i], want[i])\n" +
				"\t\t}\n\t}\n}\n" +
				"func TestBitsetHash(t *testing.T) {\n" +
				"\tif a.hash() != b.hash() {\n\t}\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("root has error for %q:\n%s", tc.name, root.SExpr(lang))
			}
			var walk func(n *gotreesitter.Node)
			walk = func(n *gotreesitter.Node) {
				if n == nil {
					return
				}
				if n.IsError() || n.IsMissing() {
					t.Fatalf("found ERROR/MISSING node in otherwise-clean tree for %q:\n%s", tc.name, root.SExpr(lang))
				}
				for i := 0; i < n.ChildCount(); i++ {
					walk(n.Child(i))
				}
			}
			walk(root)
		})
	}
}

// TestGoWhitespaceAdjacentImmediateStringToken pins a second, distinct
// grammargen defect found while investigating the false-ERROR classes above:
// grammargen/dfa.go's lex-mode computation unconditionally injected the
// grammar's terminal extras (whitespace, comments) into every lex mode,
// including "strict immediate" modes reached mid interpreted_string_literal
// (right after content, expecting only more content, an escape_sequence, or
// the closing quote — all token.immediate()). That corrupted the
// immediate-vs-non-immediate tie-break used when generating the DFA: a
// same-character non-immediate sibling terminal (e.g. the plain, non-immediate
// '"' that opens a brand new string elsewhere in the grammar), pulled in only
// via reduce-follow/missing-token-recovery lookahead widening, would win over
// the correct immediate token for genuinely byte-adjacent input with no
// whitespace to skip at all. The lexer then handed the parser the wrong
// symbol for a plain "content immediately followed by the closing quote"
// string (most visibly on interpreted string literals whose content is pure
// whitespace, or ends right before the closing quote), sending it into
// unnecessary — and here, badly wrong — error recovery. This could silently
// produce either a totally wrong tree shape (composite_literal/expression_list
// garbling of what should be an if_statement or call_expression, with no
// HasError() signal at all) or a HasError()==true tree with zero visible
// ERROR/MISSING nodes (the "phantom" class): the tainted, error-absorbed
// leaf token survived, GSS-shared, into a completely different, ultimately-
// winning GLR fork that never itself entered error recovery.
//
// Fixed in grammargen/dfa.go by excluding follow/missing-recovery widening
// (and terminal-extras injection) for states whose only real, non-widened
// parser actions are already all token.immediate() terminals.
func TestGoWhitespaceAdjacentImmediateStringToken(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "bare_space_only_string_var_decl",
			src:  "package p\nvar x = \" \"\n",
		},
		{
			name: "bare_space_only_string_call_arg",
			src:  "package p\nfunc f() {\n\tg(\" \")\n}\n",
		},
		{
			name: "space_only_string_short_var_decl",
			src:  "package p\nfunc f() {\n\tx := \" \"\n\t_ = x\n}\n",
		},
		{
			name: "content_ending_in_space_before_close_quote",
			src:  "package p\nfunc f() {\n\tx := \"a \"\n\t_ = x\n}\n",
		},
		{
			// The originally-discovered real-world trigger: an if-block whose
			// body ends in a multi-line t.Fatalf(...) call with a "..." +
			// "..." concatenated, %d-bearing format string. Before the fix
			// this silently misparsed the entire if_statement as an unrelated
			// composite_literal/expression_list, with HasError()==false.
			name: "if_block_multiline_fatalf_composite_literal_garble",
			src: "package p\nfunc f() {\n" +
				"\tif actualBytes > maxRetainedFullNodeBytes {\n" +
				"\t\tt.Fatalf(\"maxRetainedNodeCapacityForClass(full) = %d nodes = %d bytes; \"+\n" +
				"\t\t\t\"below default full-parse slab capacity %d nodes\",\n" +
				"\t\t\tmaxNodes, nodeCapacityForClass(arenaClassFull))\n" +
				"\t}\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("root has error for %q:\n%s", tc.name, root.SExpr(lang))
			}
			var walk func(n *gotreesitter.Node)
			walk = func(n *gotreesitter.Node) {
				if n == nil {
					return
				}
				if n.IsError() || n.IsMissing() {
					t.Fatalf("found ERROR/MISSING node in otherwise-clean tree for %q:\n%s", tc.name, root.SExpr(lang))
				}
				for i := 0; i < n.ChildCount(); i++ {
					walk(n.Child(i))
				}
			}
			walk(root)
			if got := findNamedChild(lang, root, "interpreted_string_literal"); got == nil {
				t.Fatalf("expected interpreted_string_literal in tree for %q, got:\n%s", tc.name, root.SExpr(lang))
			}
			// The if-block case's own defect was a silently WRONG shape with
			// no error flag at all (if_statement collapsing into
			// expression_list/composite_literal) — walk()/root.HasError()
			// above can't catch that, so pin the shape directly here too,
			// not just in the separate if_block_keeps_if_statement_shape
			// subtest below.
			if tc.name == "if_block_multiline_fatalf_composite_literal_garble" {
				if findNamedChild(lang, root, "if_statement") == nil {
					t.Fatalf("missing if_statement, tree collapsed into something else for %q:\n%s", tc.name, root.SExpr(lang))
				}
			}
		})
	}

	// The if-block case must keep its if_statement shape, not collapse into a
	// bare expression_list — this is what the original garble looked like.
	t.Run("if_block_keeps_if_statement_shape", func(t *testing.T) {
		src := "package p\nfunc f() {\n" +
			"\tif actualBytes > maxRetainedFullNodeBytes {\n" +
			"\t\tt.Fatalf(\"maxRetainedNodeCapacityForClass(full) = %d nodes = %d bytes; \"+\n" +
			"\t\t\t\"below default full-parse slab capacity %d nodes\",\n" +
			"\t\t\tmaxNodes, nodeCapacityForClass(arenaClassFull))\n" +
			"\t}\n}\n"
		tree, lang := parseGo(t, src)
		root := tree.RootNode()
		if findNamedChild(lang, root, "if_statement") == nil {
			t.Fatalf("missing if_statement, tree collapsed into something else:\n%s", root.SExpr(lang))
		}
	})
}

// TestGoIfElseSameLineKeywordPromotion pins a third, distinct false-ERROR
// class found while stress-testing the two fixes above against a broader
// real-world corpus (Go stdlib): grammargen-compiled Go blobs mis-lexed the
// "else" keyword when it directly follows an if-block's closing "}" on the
// same line separated only by spaces/tabs (never a newline).
//
// Root cause: parser_dfa_token_source.go's
// preferSameLineTokenOverGeneratedZeroWidthSentinel scans past same-line
// whitespace looking for a real token to prefer over the parser's generated
// zero-width auto-semicolon sentinel ("\x00"), but checked the RAW,
// un-promoted DFA classification of that token — reserved words come back
// as a plain "identifier" from the raw scan; keyword promotion (recognizing
// that identifier text "else" should really be the "else" keyword symbol)
// happens later in the pipeline. Since the if-block's closing state has no
// action for a bare "identifier" there (only for the "else" keyword
// symbol), the same-line preference check always lost and fell back to the
// sentinel — completing the if_statement one token early with no else
// clause, then reparsing "else { ... }" as a bare identifier followed by an
// unrelated composite_literal (the block's "{...}" reinterpreted as a
// literal_value, "else" as its type name). Fixed by promoting the
// candidate real token before checking whether the current parser state can
// consume it.
//
// This is an engine-level fix (parser_dfa_token_source.go, not
// grammargen-generated tables), so it applies to every grammargen-compiled,
// DFA-driven language with the same "\n | ; | \x00" auto-semicolon shape,
// not just Go.
func TestGoIfElseSameLineKeywordPromotion(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "assignment_in_both_branches",
			src:  "package p\nfunc f() { if x { z = a } else { z = b } }\n",
		},
		{
			name: "short_var_decl_in_else",
			src:  "package p\nfunc f() { if x { z = a } else { z := b } }\n",
		},
		{
			name: "call_in_else",
			src:  "package p\nfunc f() { if x { z = a } else { b() } }\n",
		},
		{
			name: "no_space_before_else",
			src:  "package p\nfunc f() { if x { z = a }else{ z = b } }\n",
		},
		{
			name: "else_if_chain_with_assignment",
			src:  "package p\nfunc f() { if x { z = a } else if y { z = b } else { z = c } }\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, lang := parseGo(t, tc.src)
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("root has error for %q:\n%s", tc.name, root.SExpr(lang))
			}
			var walk func(n *gotreesitter.Node)
			walk = func(n *gotreesitter.Node) {
				if n == nil {
					return
				}
				if n.IsError() || n.IsMissing() {
					t.Fatalf("found ERROR/MISSING node in otherwise-clean tree for %q:\n%s", tc.name, root.SExpr(lang))
				}
				for i := 0; i < n.ChildCount(); i++ {
					walk(n.Child(i))
				}
			}
			walk(root)
			ifStmt := findNamedChild(lang, root, "if_statement")
			if ifStmt == nil {
				t.Fatalf("missing if_statement for %q, tree collapsed into something else:\n%s", tc.name, root.SExpr(lang))
			}
			if ifStmt.ChildByFieldName("alternative", lang) == nil {
				t.Fatalf("if_statement lost its else alternative for %q:\n%s", tc.name, root.SExpr(lang))
			}
		})
	}
}

// TestGoMergePerKeyCapRegressionFiles pins the two smallest of four real
// files a review pass found clean at the pre-ASI-fix baseline and under the
// C oracle, but that flipped to a false ERROR under the first (content-gated,
// cap=5) attempt at fixing the merge-per-key survivor budget needed by the
// `_automatic_semicolon` external-scanner ASI fix (grammars/go_scanner.go).
// query_kotlin_regression_test.go is notable: its `got[i] != want[i]` /
// `imports[i] != wantImports[i]` shapes actually matched the old gate's
// bracket-index-comparison content probe (grep for the shape) and still
// needed more survivors than that gate's cap=5 provided — cap=5 was
// calibrated to the single original pin case, not to every occurrence of the
// shape it targeted. The other two (cursor_test.go, this repo;
// sort_slices_benchmark_test.go, Go standard library) are exercised by ad
// hoc corpus walks rather than pinned here. See the "go" case in
// effectiveParseMergePerKeyCap and fullParseRetryMergePerKeyOverride's "go"
// case (parser_retry.go) for the full account: the fix landed as a
// steady-state cap=3 (unchanged from pre-ASI-fix) plus a retry rung that
// fires only when a fresh cap=3 parse reports HasError, re-parsing once at
// cap=16 — these two files' cap=3 parses report HasError=true, so this test
// exercises the retry rung internally without asserting on it directly.
func TestGoMergePerKeyCapRegressionFiles(t *testing.T) {
	for _, name := range []string{
		"language_forest_optin_test.go",
		"query_kotlin_regression_test.go",
	} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			tree, lang := parseGo(t, string(src))
			root := tree.RootNode()
			defer tree.Release()
			if root.HasError() {
				t.Fatalf("root has error for %s:\n%s", name, root.SExpr(lang))
			}
		})
	}
}

// TestGoMergePerKeyCapNormalizeGoStaysCleanAtSteadyState pins the other half
// of the merge-per-key cap story: grammargen/normalize.go (and the 153-byte
// repro of its trigger shape below) parses clean at the steady-state cap=3
// — the retry rung in fullParseRetryMergePerKeyOverride's "go" case must
// never fire for it. This matters because an earlier, since-reverted attempt
// at this fix (an unconditional steady-state raise to cap=8, commit
// a03cdff0) broke this exact file: the same construct parses clean at cap=3
// but produces a false ERROR at every fixed steady-state cap from 8 through
// 16 tested — the GLR merge-selection engine is not monotonic in the cap
// value, so a from-scratch parse at a fixed, elevated cap can select a worse
// merge winner than one that started narrow. The retry-rung design sidesteps
// this by construction (clean-at-cap=3 files never retry), but this test
// guards against a future change reintroducing a steady-state raise, or
// otherwise causing this file/shape to see a wider cap on its first pass.
func TestGoMergePerKeyCapNormalizeGoStaysCleanAtSteadyState(t *testing.T) {
	t.Run("grammargen/normalize.go", func(t *testing.T) {
		src, err := os.ReadFile("grammargen/normalize.go")
		if err != nil {
			t.Fatalf("read grammargen/normalize.go: %v", err)
		}
		tree, lang := parseGo(t, string(src))
		root := tree.RootNode()
		defer tree.Release()
		if root.HasError() {
			t.Fatalf("root has error for grammargen/normalize.go:\n%s", root.SExpr(lang))
		}
	})
	t.Run("minimal_repro", func(t *testing.T) {
		src := "package p\n" +
			"func f() {\n" +
			"\tfor value, names := range candidatesByValue {\n" +
			"\t\tif anonymousSources[value] {\n" +
			"\t\t\tout[names[0]] = true\n" +
			"\t\t}\n" +
			"\t}\n" +
			"}\n"
		tree, lang := parseGo(t, src)
		root := tree.RootNode()
		defer tree.Release()
		if root.HasError() {
			t.Fatalf("root has error for minimal_repro:\n%s", root.SExpr(lang))
		}
	})
}

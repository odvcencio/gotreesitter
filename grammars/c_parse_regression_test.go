package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestParseFileCSizeofIdentifierKeepsExpressionBranch(t *testing.T) {
	assertCSizeofIdentifierExpression(t, []byte("void f(void) { g(sizeof(TSExternalTokenState)); }\n"), "TSExternalTokenState")
}

func TestParseFileCSizeofUnknownHeaderTypeKeepsExpressionBranch(t *testing.T) {
	assertCSizeofIdentifierExpression(t, []byte("void f(void) { g(sizeof(clusterState)); }\n"), "clusterState")
}

func TestParseFileCSizeofLocalVariableKeepsExpressionBranch(t *testing.T) {
	assertCSizeofIdentifierExpression(t, []byte("void f(void) { char buf[8]; g(sizeof(buf)); }\n"), "buf")
}

func TestParseFileCUnknownHeaderTypeCastKeepsCallBranch(t *testing.T) {
	assertCCastUnknownTypeIsCallExpression(t, []byte("void f(void) { x = (clusterState)(y); }\n"), "clusterState")
}

// The existing engine turns `(local_t)(y)` into a cast because local_t is a
// typedef in the file. Pinned C parses it as a call, and ParseFile gives C's
// tree because the C certificate lists the table-v1 provider. This test checks
// the existing engine's rewrite, with the registered C token source.
func TestParseFileCLocalTypedefCastStaysCastExpression(t *testing.T) {
	src := []byte("typedef int local_t;\nvoid f(void) { x = (local_t)(y); }\n")
	assertCCastLocalTypedefStaysCast(t, parseCExistingEngine(t, src), src, "local_t")
}

func TestParseFileCHeaderTypeCastWithoutArgumentListStaysCastExpression(t *testing.T) {
	src := []byte("void f(int content_size) { x = (off_t)content_size; }\n")
	bt, err := ParseFile("parser.c", src)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	assertCCastLocalTypedefStaysCast(t, bt, src, "off_t")
}

// parseCExistingEngine parses src as ParseFile did before the core served C
// registry requests: the registered C token source on the existing engine.
func parseCExistingEngine(t *testing.T, src []byte) *gotreesitter.BoundTree {
	t.Helper()
	lang := CLanguage()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.ParseWithTokenSource(src, NewCTokenSourceOrEOF(src, lang))
	if err != nil || tree == nil {
		t.Fatalf("ParseWithTokenSource: tree %v, error %v", tree != nil, err)
	}
	return gotreesitter.Bind(tree)
}

func TestParseFileCCommentFollowedByDeclarationKeepsDeclarationBounds(t *testing.T) {
	src := []byte("// hello \\\n   this is still a comment\nthis_is_not a_comment;\n")
	bt, err := ParseFile("parser.c", src)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	root := bt.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("expected error-free C parse tree, got %v", root)
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}

	decl := root.Child(1)
	if decl == nil {
		t.Fatal("root child[1] is nil, want declaration")
	}
	if got := bt.NodeType(decl); got != "declaration" {
		t.Fatalf("root child[1] type = %q, want declaration", got)
	}
	wantStart := uint32(bytes.Index(src, []byte("this_is_not")))
	wantEnd := uint32(len(src) - 1)
	if got := decl.StartByte(); got != wantStart {
		t.Fatalf("declaration start = %d, want %d", got, wantStart)
	}
	if got := decl.EndByte(); got != wantEnd {
		t.Fatalf("declaration end = %d, want %d", got, wantEnd)
	}
}

func TestParseFileCDefineValueParensStaysPreprocDef(t *testing.T) {
	src := []byte("#define SIZE_ALIGN (4*sizeof(size_t))\n")
	bt, err := ParseFile("parser.c", src)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	root := bt.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("expected error-free C parse tree, got %v", root)
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}

	macro := root.Child(0)
	if macro == nil {
		t.Fatal("root child[0] is nil, want preproc_def")
	}
	if got := bt.NodeType(macro); got != "preproc_def" {
		t.Fatalf("root child[0] type = %q, want preproc_def", got)
	}
	if got, want := macro.ChildCount(), 3; got != want {
		t.Fatalf("preproc_def child count = %d, want %d", got, want)
	}

	value := macro.Child(2)
	wantStart := uint32(bytes.Index(src, []byte("(4*sizeof(size_t))")))
	wantEnd := uint32(len(src) - 1)
	if value == nil {
		t.Fatal("preproc value child is nil, want preproc_arg")
	}
	if got := bt.NodeType(value); got != "preproc_arg" {
		t.Fatalf("preproc value child type = %q, want preproc_arg", got)
	}
	if got := value.StartByte(); got != wantStart {
		t.Fatalf("preproc_arg start = %d, want %d", got, wantStart)
	}
	if got := value.EndByte(); got != wantEnd {
		t.Fatalf("preproc_arg end = %d, want %d", got, wantEnd)
	}
}

func TestParseFileCMultilineDefineKeepsBodyInsidePreprocArg(t *testing.T) {
	src := []byte("#define LOG(...) \\\n  if (flag) { \\\n    work(); \\\n  }\n\nint z;\n")
	bt, err := ParseFile("parser.c", src)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	root := bt.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("expected error-free C parse tree, got %v", root)
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	for i := 0; i < root.ChildCount(); i++ {
		if got := bt.NodeType(root.Child(i)); got == "if_statement" {
			t.Fatalf("root leaked macro body as top-level if_statement: %s", root.SExpr(CLanguage()))
		}
	}

	macro := root.Child(0)
	if macro == nil {
		t.Fatal("root child[0] is nil, want preproc_function_def")
	}
	if got := bt.NodeType(macro); got != "preproc_function_def" {
		t.Fatalf("root child[0] type = %q, want preproc_function_def", got)
	}
	arg := macro.Child(macro.ChildCount() - 1)
	if arg == nil {
		t.Fatal("macro value child is nil, want preproc_arg")
	}
	if got := bt.NodeType(arg); got != "preproc_arg" {
		t.Fatalf("macro value child type = %q, want preproc_arg", got)
	}
	wantStart := uint32(bytes.Index(src, []byte("if (flag)")))
	wantEnd := uint32(bytes.Index(src, []byte("\n\nint z;")))
	if got := arg.StartByte(); got != wantStart {
		t.Fatalf("preproc_arg start = %d, want %d", got, wantStart)
	}
	if got := arg.EndByte(); got != wantEnd {
		t.Fatalf("preproc_arg end = %d, want %d", got, wantEnd)
	}
}

func assertCSizeofIdentifierExpression(t *testing.T, src []byte, wantIdent string) {
	t.Helper()
	bt, err := ParseFile("parser.c", src)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	lang := CLanguage()
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFile returned nil root for C sizeof expression")
	}
	if root.HasError() {
		t.Fatalf("expected error-free C parse tree, got %s", root.SExpr(lang))
	}

	var sizeofExpr *gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if bt.NodeType(node) == "sizeof_expression" {
			sizeofExpr = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if sizeofExpr == nil {
		t.Fatalf("missing sizeof_expression in tree: %s", root.SExpr(lang))
	}

	var parenExpr *gotreesitter.Node
	parenExprIndex := -1
	for i := 0; i < sizeofExpr.ChildCount(); i++ {
		child := sizeofExpr.Child(i)
		if child == nil {
			continue
		}
		switch bt.NodeType(child) {
		case "parenthesized_expression":
			parenExpr = child
			parenExprIndex = i
		case "type_descriptor":
			t.Fatalf("sizeof(identifier) collapsed to type_descriptor: %s", root.SExpr(lang))
		}
	}
	if parenExpr == nil {
		t.Fatalf("sizeof_expression missing parenthesized_expression: %s", root.SExpr(lang))
	}
	if got, want := sizeofExpr.FieldNameForChild(parenExprIndex, lang), "value"; got != want {
		t.Fatalf("sizeof_expression field = %q, want %q", got, want)
	}

	var identifier *gotreesitter.Node
	gotreesitter.Walk(parenExpr, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if bt.NodeType(node) == "identifier" {
			identifier = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if identifier == nil {
		t.Fatalf("parenthesized_expression missing identifier: %s", root.SExpr(lang))
	}
	if got, want := identifier.Text(src), wantIdent; got != want {
		t.Fatalf("sizeof identifier = %q, want %q", got, want)
	}
}

func assertCCastUnknownTypeIsCallExpression(t *testing.T, src []byte, wantIdent string) {
	t.Helper()
	bt, err := ParseFile("parser.c", src)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	lang := CLanguage()
	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFile returned nil root for C cast/call expression")
	}
	if root.HasError() {
		t.Fatalf("expected error-free C parse tree, got %s", root.SExpr(lang))
	}

	var callExpr *gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		switch bt.NodeType(node) {
		case "cast_expression":
			t.Fatalf("unknown header type collapsed to cast_expression: %s", root.SExpr(lang))
		case "call_expression":
			callExpr = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if callExpr == nil {
		t.Fatalf("missing call_expression in tree: %s", root.SExpr(lang))
	}
	if got, want := callExpr.FieldNameForChild(0, lang), "function"; got != want {
		t.Fatalf("call function field = %q, want %q", got, want)
	}
	if got, want := callExpr.FieldNameForChild(1, lang), "arguments"; got != want {
		t.Fatalf("call arguments field = %q, want %q", got, want)
	}

	function := callExpr.Child(0)
	if function == nil || bt.NodeType(function) != "parenthesized_expression" {
		t.Fatalf("call_expression missing parenthesized function: %s", root.SExpr(lang))
	}
	arguments := callExpr.Child(1)
	if arguments == nil || bt.NodeType(arguments) != "argument_list" {
		t.Fatalf("call_expression missing argument_list: %s", root.SExpr(lang))
	}
	var identifier *gotreesitter.Node
	gotreesitter.Walk(function, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if bt.NodeType(node) == "identifier" {
			identifier = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if identifier == nil {
		t.Fatalf("parenthesized function missing identifier: %s", root.SExpr(lang))
	}
	if got, want := identifier.Text(src), wantIdent; got != want {
		t.Fatalf("call identifier = %q, want %q", got, want)
	}
}

func assertCCastLocalTypedefStaysCast(t *testing.T, bt *gotreesitter.BoundTree, src []byte, wantIdent string) {
	t.Helper()
	defer bt.Release()

	lang := CLanguage()
	root := bt.RootNode()
	if root == nil {
		t.Fatal("nil root for C cast expression")
	}
	if root.HasError() {
		t.Fatalf("expected error-free C parse tree, got %s", root.SExpr(lang))
	}

	var castExpr *gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		switch bt.NodeType(node) {
		case "call_expression":
			t.Fatalf("local typedef cast collapsed to call_expression: %s", root.SExpr(lang))
		case "cast_expression":
			castExpr = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if castExpr == nil {
		t.Fatalf("missing cast_expression in tree: %s", root.SExpr(lang))
	}
	if got, want := castExpr.FieldNameForChild(1, lang), "type"; got != want {
		t.Fatalf("cast type field = %q, want %q", got, want)
	}
	if got, want := castExpr.FieldNameForChild(3, lang), "value"; got != want {
		t.Fatalf("cast value field = %q, want %q", got, want)
	}

	typeDescriptor := castExpr.Child(1)
	if typeDescriptor == nil || bt.NodeType(typeDescriptor) != "type_descriptor" {
		t.Fatalf("cast_expression missing type_descriptor: %s", root.SExpr(lang))
	}
	typeIdent := typeDescriptor.Child(0)
	if typeIdent == nil || bt.NodeType(typeIdent) != "type_identifier" {
		t.Fatalf("type_descriptor missing type_identifier: %s", root.SExpr(lang))
	}
	if got, want := typeIdent.Text(src), wantIdent; got != want {
		t.Fatalf("cast type identifier = %q, want %q", got, want)
	}
}

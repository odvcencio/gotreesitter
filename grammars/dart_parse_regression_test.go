package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// TestDartLibraryDirectiveWithoutNameParsesCleanly pins the C-exact shape of a
// library directive that carries no name. Grammar be07cf7118d3 accepts the
// directive, so the parse no longer inserts a missing identifier and no longer
// sets the error flag. The locked C oracle returns the same tree.
func TestDartLibraryDirectiveWithoutNameParsesCleanly(t *testing.T) {
	src := []byte("library;\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected a clean library directive, got %s", root.SExpr(DartLanguage()))
	}
	if got := root.NamedChildCount(); got != 1 {
		t.Fatalf("named child count = %d, want 1; tree=%s", got, root.SExpr(DartLanguage()))
	}
	libraryName := root.NamedChild(0)
	if libraryName == nil || libraryName.Type(DartLanguage()) != "library_name" {
		t.Fatalf("first named child = %v, want library_name; tree=%s", libraryName, root.SExpr(DartLanguage()))
	}
	if got := libraryName.NamedChildCount(); got != 0 {
		t.Fatalf("library_name named child count = %d, want 0; tree=%s", got, root.SExpr(DartLanguage()))
	}
	if got, want := root.SExpr(DartLanguage()), "(program (library_name))"; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}
}

func TestDartGetterCallWithIfNullArgumentParsesWithoutError(t *testing.T) {
	src := []byte("base class Parser implements Finalizable {\n  List<int> get contents => f(_contents ?? '');\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected getter call with ?? argument to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	if got, want := root.Type(DartLanguage()), "program"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if got, want := root.NamedChildCount(), 1; got != want {
		t.Fatalf("named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
}

func TestDartNestedTypeArgumentsBeforeArgumentsParseAsSelectorCall(t *testing.T) {
	src := []byte("base class Parser implements Finalizable {\n  late final c = z.lookup<T<U>>(arg);\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected generic call canary to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
	body := classDef.ChildByFieldName("body", DartLanguage())
	if body == nil {
		t.Fatalf("class body missing; tree=%s", root.SExpr(DartLanguage()))
	}
	if body.NamedChildCount() == 0 {
		t.Fatalf("class body has no named children; tree=%s", root.SExpr(DartLanguage()))
	}
	decl := body.NamedChild(0)
	if decl == nil {
		t.Fatalf("class declaration missing; tree=%s", root.SExpr(DartLanguage()))
	}
	initList := decl.NamedChild(1)
	if initList == nil || initList.Type(DartLanguage()) != "initialized_identifier_list" {
		t.Fatalf("initialized list = %v; tree=%s", initList, root.SExpr(DartLanguage()))
	}
	init := initList.NamedChild(0)
	if init == nil || init.Type(DartLanguage()) != "initialized_identifier" {
		t.Fatalf("initialized identifier = %v; tree=%s", init, root.SExpr(DartLanguage()))
	}
	if got, want := init.NamedChildCount(), 4; got != want {
		t.Fatalf("initialized identifier named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	propertySel := init.NamedChild(2)
	if propertySel == nil || propertySel.Type(DartLanguage()) != "selector" {
		t.Fatalf("property selector = %v, want selector; tree=%s", propertySel, root.SExpr(DartLanguage()))
	}
	selector := init.NamedChild(3)
	if selector == nil || selector.Type(DartLanguage()) != "selector" {
		t.Fatalf("call selector = %v, want selector; tree=%s", selector, root.SExpr(DartLanguage()))
	}
	if got, want := selector.NamedChildCount(), 1; got != want {
		t.Fatalf("call selector named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	argPart := selector.NamedChild(0)
	if argPart == nil || argPart.Type(DartLanguage()) != "argument_part" {
		t.Fatalf("argument part = %v, want argument_part; tree=%s", argPart, root.SExpr(DartLanguage()))
	}
}

// TestDartSingleTypeArgumentFreeCallParsesAsSelectorCall pins the C-exact
// shape of a free call with one type argument. Upstream issue 103 landed in
// grammar be07cf7118d3, so `calloc<Size>(1)` now reduces to a selector with an
// argument_part. The three retired compat rewrites used to reshape this tree
// into a relational_expression chain, which the C oracle never produces.
func TestDartSingleTypeArgumentFreeCallParsesAsSelectorCall(t *testing.T) {
	src := []byte("class CancelToken {\n  final _token = calloc<Size>(1);\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected single-type free call to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
	body := classDef.ChildByFieldName("body", DartLanguage())
	if body == nil || body.NamedChildCount() == 0 {
		t.Fatalf("class body missing; tree=%s", root.SExpr(DartLanguage()))
	}
	decl := body.NamedChild(0)
	if decl == nil {
		t.Fatalf("class declaration missing; tree=%s", root.SExpr(DartLanguage()))
	}
	initList := decl.NamedChild(1)
	if initList == nil || initList.Type(DartLanguage()) != "initialized_identifier_list" {
		t.Fatalf("initialized list = %v; tree=%s", initList, root.SExpr(DartLanguage()))
	}
	init := initList.NamedChild(0)
	if init == nil || init.Type(DartLanguage()) != "initialized_identifier" {
		t.Fatalf("initialized identifier = %v; tree=%s", init, root.SExpr(DartLanguage()))
	}
	if got, want := init.NamedChildCount(), 3; got != want {
		t.Fatalf("initialized identifier named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	callee := init.NamedChild(1)
	if callee == nil || callee.Type(DartLanguage()) != "identifier" {
		t.Fatalf("callee = %v, want identifier; tree=%s", callee, root.SExpr(DartLanguage()))
	}
	selector := init.NamedChild(2)
	if selector == nil || selector.Type(DartLanguage()) != "selector" {
		t.Fatalf("call selector = %v, want selector; tree=%s", selector, root.SExpr(DartLanguage()))
	}
	if got, want := selector.NamedChildCount(), 1; got != want {
		t.Fatalf("call selector named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	argPart := selector.NamedChild(0)
	if argPart == nil || argPart.Type(DartLanguage()) != "argument_part" {
		t.Fatalf("argument part = %v, want argument_part; tree=%s", argPart, root.SExpr(DartLanguage()))
	}
	if got, want := argPart.NamedChildCount(), 2; got != want {
		t.Fatalf("argument part named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	typeArgs := argPart.NamedChild(0)
	if typeArgs == nil || typeArgs.Type(DartLanguage()) != "type_arguments" {
		t.Fatalf("type arguments = %v, want type_arguments; tree=%s", typeArgs, root.SExpr(DartLanguage()))
	}
	if got, want := typeArgs.NamedChildCount(), 1; got != want {
		t.Fatalf("type arguments named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	typeName := typeArgs.NamedChild(0)
	if typeName == nil || typeName.Type(DartLanguage()) != "type_identifier" {
		t.Fatalf("type argument child = %v, want type_identifier; tree=%s", typeName, root.SExpr(DartLanguage()))
	}
	args := argPart.NamedChild(1)
	if args == nil || args.Type(DartLanguage()) != "arguments" {
		t.Fatalf("arguments = %v, want arguments; tree=%s", args, root.SExpr(DartLanguage()))
	}
	if got, want := args.NamedChildCount(), 1; got != want {
		t.Fatalf("arguments named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	arg := args.NamedChild(0)
	if arg == nil || arg.Type(DartLanguage()) != "argument" {
		t.Fatalf("argument = %v, want argument; tree=%s", arg, root.SExpr(DartLanguage()))
	}
	literal := arg.NamedChild(0)
	if literal == nil || literal.Type(DartLanguage()) != "decimal_integer_literal" {
		t.Fatalf("argument value = %v, want decimal_integer_literal; tree=%s", literal, root.SExpr(DartLanguage()))
	}
}

// TestDartComplexVoidFunctionTypeArgumentFreeCallParsesAsSelectorCall pins the
// C-exact shape of a free call whose single type argument holds a function
// type. Grammar be07cf7118d3 reduces it to a selector with an argument_part.
func TestDartComplexVoidFunctionTypeArgumentFreeCallParsesAsSelectorCall(t *testing.T) {
	src := []byte("base class Parser implements Finalizable {\n  late final p = _lookup<ffi.NativeFunction<ffi.Void Function(ffi.Pointer<TSParser>, TSLogger)>>('ts_parser_set_logger');\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected complex void function-type call to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
	body := classDef.ChildByFieldName("body", DartLanguage())
	if body == nil || body.NamedChildCount() == 0 {
		t.Fatalf("class body missing; tree=%s", root.SExpr(DartLanguage()))
	}
	decl := body.NamedChild(0)
	if decl == nil {
		t.Fatalf("class declaration missing; tree=%s", root.SExpr(DartLanguage()))
	}
	initList := decl.NamedChild(1)
	if initList == nil || initList.Type(DartLanguage()) != "initialized_identifier_list" {
		t.Fatalf("initialized list = %v; tree=%s", initList, root.SExpr(DartLanguage()))
	}
	init := initList.NamedChild(0)
	if init == nil || init.Type(DartLanguage()) != "initialized_identifier" {
		t.Fatalf("initialized identifier = %v; tree=%s", init, root.SExpr(DartLanguage()))
	}
	assertDartFunctionTypeArgumentSelectorCall(t, root, init)
}

// TestDartNestedFunctionTypeArgumentFreeCallParsesAsSelectorCall pins the
// C-exact shape of a free call whose type argument nests a function type with
// four parameters.
func TestDartNestedFunctionTypeArgumentFreeCallParsesAsSelectorCall(t *testing.T) {
	src := []byte("base class Parser implements Finalizable {\n  late final p = _lookup<ffi.NativeFunction<TSSymbol Function(ffi.Pointer<TSLanguage>, ffi.Pointer<ffi.Char>, ffi.Uint32, ffi.Bool)>>('ts_language_symbol_for_name');\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected nested function-type call to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
	body := classDef.ChildByFieldName("body", DartLanguage())
	if body == nil || body.NamedChildCount() == 0 {
		t.Fatalf("class body missing; tree=%s", root.SExpr(DartLanguage()))
	}
	decl := body.NamedChild(0)
	if decl == nil {
		t.Fatalf("class declaration missing; tree=%s", root.SExpr(DartLanguage()))
	}
	initList := decl.NamedChild(1)
	if initList == nil || initList.Type(DartLanguage()) != "initialized_identifier_list" {
		t.Fatalf("initialized list = %v; tree=%s", initList, root.SExpr(DartLanguage()))
	}
	init := initList.NamedChild(0)
	if init == nil || init.Type(DartLanguage()) != "initialized_identifier" {
		t.Fatalf("initialized identifier = %v; tree=%s", init, root.SExpr(DartLanguage()))
	}
	assertDartFunctionTypeArgumentSelectorCall(t, root, init)
}

// TestDartComplexGenericReturnTypeArgumentFreeCallParsesAsSelectorCall pins the
// C-exact shape of a free call whose function-type argument returns a generic
// type.
func TestDartComplexGenericReturnTypeArgumentFreeCallParsesAsSelectorCall(t *testing.T) {
	src := []byte("base class Parser implements Finalizable {\n  late final p = _lookup<ffi.NativeFunction<ffi.Pointer<TSLanguage> Function(ffi.Pointer<TSParser>)>>('ts_parser_language');\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected complex generic-return call to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
	body := classDef.ChildByFieldName("body", DartLanguage())
	if body == nil || body.NamedChildCount() == 0 {
		t.Fatalf("class body missing; tree=%s", root.SExpr(DartLanguage()))
	}
	decl := body.NamedChild(0)
	if decl == nil {
		t.Fatalf("class declaration missing; tree=%s", root.SExpr(DartLanguage()))
	}
	initList := decl.NamedChild(1)
	if initList == nil || initList.Type(DartLanguage()) != "initialized_identifier_list" {
		t.Fatalf("initialized list = %v; tree=%s", initList, root.SExpr(DartLanguage()))
	}
	init := initList.NamedChild(0)
	if init == nil || init.Type(DartLanguage()) != "initialized_identifier" {
		t.Fatalf("initialized identifier = %v; tree=%s", init, root.SExpr(DartLanguage()))
	}
	if got, want := init.NamedChildCount(), 3; got != want {
		t.Fatalf("initialized identifier named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	callee := init.NamedChild(1)
	if callee == nil || callee.Type(DartLanguage()) != "identifier" {
		t.Fatalf("callee = %v, want identifier; tree=%s", callee, root.SExpr(DartLanguage()))
	}
	selector := init.NamedChild(2)
	if selector == nil || selector.Type(DartLanguage()) != "selector" {
		t.Fatalf("call selector = %v, want selector; tree=%s", selector, root.SExpr(DartLanguage()))
	}
	if got, want := selector.NamedChildCount(), 1; got != want {
		t.Fatalf("selector named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
}

func TestDartConstructorNamedLikeClassBuildsConstructorSignature(t *testing.T) {
	src := []byte("base class QueryCursor {\n  QueryCursor() {}\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected constructor to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
	body := classDef.ChildByFieldName("body", DartLanguage())
	if body == nil || body.NamedChildCount() == 0 {
		t.Fatalf("class body missing; tree=%s", root.SExpr(DartLanguage()))
	}
	methodSig := body.NamedChild(0)
	if methodSig == nil || methodSig.Type(DartLanguage()) != "method_signature" {
		t.Fatalf("method signature = %v; tree=%s", methodSig, root.SExpr(DartLanguage()))
	}
	sig := methodSig.NamedChild(0)
	if sig == nil || sig.Type(DartLanguage()) != "constructor_signature" {
		t.Fatalf("signature = %v, want constructor_signature; tree=%s", sig, root.SExpr(DartLanguage()))
	}
}

func TestDartPrivateConstructorDeclarationBuildsConstructorSignature(t *testing.T) {
	src := []byte("class _SymbolAddresses {\n  final TreeSitter _library;\n  _SymbolAddresses(this._library);\n}\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected private constructor to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(DartLanguage()) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(DartLanguage()))
	}
	body := classDef.ChildByFieldName("body", DartLanguage())
	if body == nil || body.NamedChildCount() < 2 {
		t.Fatalf("class body missing constructor declaration; tree=%s", root.SExpr(DartLanguage()))
	}
	decl := body.NamedChild(1)
	if decl == nil || decl.Type(DartLanguage()) != "declaration" {
		t.Fatalf("constructor declaration = %v, want declaration; tree=%s", decl, root.SExpr(DartLanguage()))
	}
	sig := decl.NamedChild(0)
	if sig == nil || sig.Type(DartLanguage()) != "constructor_signature" {
		t.Fatalf("signature = %v, want constructor_signature; tree=%s", sig, root.SExpr(DartLanguage()))
	}
}

func TestDartEnhancedEnumUnnamedConstructorBuildsConstructorSignature(t *testing.T) {
	src := []byte("enum LogPriority { warning; LogPriority(this.priority, this.prefix); final int priority; final String prefix; }\n")
	parser := ts.NewParser(DartLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected enhanced enum constructor to parse cleanly, got %s", root.SExpr(DartLanguage()))
	}
	enumDecl := root.NamedChild(0)
	if enumDecl == nil || enumDecl.Type(DartLanguage()) != "enum_declaration" {
		t.Fatalf("first named child = %v, want enum_declaration; tree=%s", enumDecl, root.SExpr(DartLanguage()))
	}
	body := enumDecl.ChildByFieldName("body", DartLanguage())
	if body == nil || body.NamedChildCount() < 2 {
		t.Fatalf("enum body missing constructor declaration; tree=%s", root.SExpr(DartLanguage()))
	}
	decl := body.NamedChild(1)
	if decl == nil || decl.Type(DartLanguage()) != "declaration" {
		t.Fatalf("enum constructor declaration = %v, want declaration; tree=%s", decl, root.SExpr(DartLanguage()))
	}
	sig := decl.NamedChild(0)
	if sig == nil || sig.Type(DartLanguage()) != "constructor_signature" {
		t.Fatalf("signature = %v, want constructor_signature; tree=%s", sig, root.SExpr(DartLanguage()))
	}
}

func TestDartNullableTypeAndNullLiteralRestoreAnonymousChildren(t *testing.T) {
	lang := DartLanguage()
	if sym, ok := lang.SymbolByName("?"); !ok {
		t.Fatal("DartLanguage missing anonymous ? symbol")
	} else if int(sym) >= len(lang.SymbolMetadata) || lang.SymbolMetadata[sym].Named {
		t.Fatalf("Dart ? symbol metadata = %+v, want anonymous", lang.SymbolMetadata[sym])
	}
	if sym, ok := lang.SymbolByName("null"); !ok {
		t.Fatal("DartLanguage missing anonymous null symbol")
	} else if int(sym) >= len(lang.SymbolMetadata) || lang.SymbolMetadata[sym].Named {
		t.Fatalf("Dart null symbol metadata = %+v, want anonymous", lang.SymbolMetadata[sym])
	}

	parser := ts.NewParser(lang)
	src := []byte(`
class Parser {
  Tree parse(String program, {int? encoding}) {
    if (program == null) {
      return Tree();
    }
    return Tree();
  }
  String? _contents;
}
`)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	var nullableNodes []*ts.Node
	var nullNodes []*ts.Node
	var walk func(*ts.Node)
	walk = func(n *ts.Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "nullable_type" {
			nullableNodes = append(nullableNodes, n)
		}
		if n.Type(lang) == "null_literal" {
			nullNodes = append(nullNodes, n)
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if got, want := len(nullableNodes), 2; got != want {
		t.Fatalf("nullable_type count = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	for _, nullable := range nullableNodes {
		if got := nullable.ChildCount(); got != 1 {
			t.Fatalf("nullable_type child count = %d, want 1; node=%s tree=%s", got, nullable.SExpr(lang), root.SExpr(lang))
		}
		child := nullable.Child(0)
		if child == nil {
			t.Fatalf("nullable_type missing question child; node=%s", nullable.SExpr(lang))
		}
		if child.Type(lang) != "?" || child.IsNamed() {
			t.Fatalf("nullable child type/named = %q/%v, want ?/false; node=%s", child.Type(lang), child.IsNamed(), nullable.SExpr(lang))
		}
	}
	if got, want := len(nullNodes), 1; got != want {
		t.Fatalf("null_literal count = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	nullLiteral := nullNodes[0]
	if got := nullLiteral.ChildCount(); got != 1 {
		t.Fatalf("null_literal child count = %d, want 1; node=%s tree=%s", got, nullLiteral.SExpr(lang), root.SExpr(lang))
	}
	nullChild := nullLiteral.Child(0)
	if nullChild == nil {
		t.Fatalf("null_literal missing null child; node=%s", nullLiteral.SExpr(lang))
	}
	if nullChild.Type(lang) != "null" || nullChild.IsNamed() {
		t.Fatalf("null child type/named = %q/%v, want null/false; node=%s", nullChild.Type(lang), nullChild.IsNamed(), nullLiteral.SExpr(lang))
	}
}

// TestDartBaseModifierKeepsAnonymousChildViaEngine proves that the dart `base`
// class modifier keeps its same-named anonymous token child at the engine
// layer (Parser.wrapsSameNamedAnonymousToken's per-language carve-out), not
// via a post-hoc AST patch. `base`'s anon token shifts into only one parse
// state in the generated grammar tables (its 3 grammar call sites merge onto
// a single LR state), so without the carve-out it would collapse to a
// childless named leaf, same as go's `nil`/`true`/`false`/`iota`.
func TestDartBaseModifierKeepsAnonymousChildViaEngine(t *testing.T) {
	lang := DartLanguage()
	parser := ts.NewParser(lang)
	src := []byte("base class Foo {}\n")
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected base class modifier to parse cleanly, got %s", root.SExpr(lang))
	}
	classDef := root.NamedChild(0)
	if classDef == nil || classDef.Type(lang) != "class_definition" {
		t.Fatalf("first named child = %v, want class_definition; tree=%s", classDef, root.SExpr(lang))
	}
	baseNode := classDef.Child(0)
	if baseNode == nil || baseNode.Type(lang) != "base" {
		t.Fatalf("first child = %v, want base; tree=%s", baseNode, root.SExpr(lang))
	}
	if !baseNode.IsNamed() {
		t.Fatalf("base node should be named; tree=%s", root.SExpr(lang))
	}
	if got := baseNode.ChildCount(); got != 1 {
		t.Fatalf("base child count = %d, want 1; tree=%s", got, root.SExpr(lang))
	}
	child := baseNode.Child(0)
	if child == nil {
		t.Fatalf("base node missing anonymous token child; node=%s", baseNode.SExpr(lang))
	}
	if child.Type(lang) != "base" || child.IsNamed() {
		t.Fatalf("base child type/named = %q/%v, want base/false; node=%s", child.Type(lang), child.IsNamed(), baseNode.SExpr(lang))
	}
	if child.StartByte() != baseNode.StartByte() || child.EndByte() != baseNode.EndByte() {
		t.Fatalf("base child byte range = [%d,%d), want [%d,%d) to match parent", child.StartByte(), child.EndByte(), baseNode.StartByte(), baseNode.EndByte())
	}
}

// assertDartFunctionTypeArgumentSelectorCall checks the C-exact selector shape
// of a free call that carries one function-type argument and one string
// argument.
func assertDartFunctionTypeArgumentSelectorCall(t *testing.T, root, init *ts.Node) {
	t.Helper()
	if got, want := init.NamedChildCount(), 3; got != want {
		t.Fatalf("initialized identifier named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	callee := init.NamedChild(1)
	if callee == nil || callee.Type(DartLanguage()) != "identifier" {
		t.Fatalf("callee = %v, want identifier; tree=%s", callee, root.SExpr(DartLanguage()))
	}
	selector := init.NamedChild(2)
	if selector == nil || selector.Type(DartLanguage()) != "selector" {
		t.Fatalf("call selector = %v, want selector; tree=%s", selector, root.SExpr(DartLanguage()))
	}
	if got, want := selector.NamedChildCount(), 1; got != want {
		t.Fatalf("call selector named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	argPart := selector.NamedChild(0)
	if argPart == nil || argPart.Type(DartLanguage()) != "argument_part" {
		t.Fatalf("argument part = %v, want argument_part; tree=%s", argPart, root.SExpr(DartLanguage()))
	}
	if got, want := argPart.NamedChildCount(), 2; got != want {
		t.Fatalf("argument part named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	typeArgs := argPart.NamedChild(0)
	if typeArgs == nil || typeArgs.Type(DartLanguage()) != "type_arguments" {
		t.Fatalf("type arguments = %v, want type_arguments; tree=%s", typeArgs, root.SExpr(DartLanguage()))
	}
	if got, want := typeArgs.NamedChildCount(), 3; got != want {
		t.Fatalf("type arguments named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	nested := typeArgs.NamedChild(2)
	if nested == nil || nested.Type(DartLanguage()) != "type_arguments" {
		t.Fatalf("nested type arguments = %v, want type_arguments; tree=%s", nested, root.SExpr(DartLanguage()))
	}
	fnType := nested.NamedChild(0)
	if fnType == nil || fnType.Type(DartLanguage()) != "function_type" {
		t.Fatalf("function type = %v, want function_type; tree=%s", fnType, root.SExpr(DartLanguage()))
	}
	args := argPart.NamedChild(1)
	if args == nil || args.Type(DartLanguage()) != "arguments" {
		t.Fatalf("arguments = %v, want arguments; tree=%s", args, root.SExpr(DartLanguage()))
	}
	if got, want := args.NamedChildCount(), 1; got != want {
		t.Fatalf("arguments named child count = %d, want %d; tree=%s", got, want, root.SExpr(DartLanguage()))
	}
	arg := args.NamedChild(0)
	if arg == nil || arg.Type(DartLanguage()) != "argument" {
		t.Fatalf("argument = %v, want argument; tree=%s", arg, root.SExpr(DartLanguage()))
	}
	literal := arg.NamedChild(0)
	if literal == nil || literal.Type(DartLanguage()) != "string_literal" {
		t.Fatalf("argument value = %v, want string_literal; tree=%s", literal, root.SExpr(DartLanguage()))
	}
}

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestFieldProjectionNativeRetiredDispatchArms(t *testing.T) {
	tests := fieldProjectionRetirementCases()
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(test.language).ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			test.assert(t, tree.RootNode(), test.language)
		})
	}
}

func TestFieldProjectionRetiredDispatchArmRoutes(t *testing.T) {
	tests := fieldProjectionRetirementCases()
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if test.name == "haskell_root_sections" || test.name == "erlang_root_forms" {
				assertFieldProjectionProductionIncrementalRoutes(t, test)
				return
			}
			for _, receipt := range retiredDispatchRouteReceipts(t, test.language, test.source) {
				t.Run(receipt.name, func(t *testing.T) {
					test.assert(t, receipt.tree.RootNode(), test.language)
					if receipt.name == "incremental" {
						profile := receipt.incrementalProfile
						if test.reuseUnsupportedReason != "" {
							if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != test.reuseUnsupportedReason {
								t.Fatalf("incremental reuse status = %+v", profile)
							}
						} else if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
							t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
						}
					}
				})
			}
		})
	}
}

func assertFieldProjectionProductionIncrementalRoutes(
	t *testing.T,
	test fieldProjectionRetirementCase,
) {
	t.Helper()
	source := append(append([]byte(nil), test.source...), '\n')
	parser := gotreesitter.NewParser(test.language)
	parser.SetAdmissionCandidateRoute(false)
	production, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	test.assert(t, production.RootNode(), test.language)

	oldTree, err := parser.Parse(test.source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	endPoint := retiredDispatchPointAtByte(test.source, len(test.source))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(test.source)),
		OldEndByte:  uint32(len(test.source)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  endPoint,
		OldEndPoint: endPoint,
		NewEndPoint: gotreesitter.Point{Row: endPoint.Row + 1},
	})
	incremental, profile, err := parser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	test.assert(t, incremental.RootNode(), test.language)
	if test.reuseUnsupportedReason != "" {
		if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != test.reuseUnsupportedReason {
			t.Fatalf("incremental reuse status = %+v", profile)
		}
		return
	}
	if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
	}
}

type fieldProjectionRetirementCase struct {
	name     string
	source   []byte
	language *gotreesitter.Language
	assert   func(*testing.T, *gotreesitter.Node, *gotreesitter.Language)
	// reuseUnsupportedReason records a locked grammar limitation.
	reuseUnsupportedReason string
}

func fieldProjectionRetirementCases() []fieldProjectionRetirementCase {
	return []fieldProjectionRetirementCase{
		{
			name:                   "lua_local_declarations",
			source:                 []byte("local function foo() end\nlocal x = 1"),
			language:               LuaLanguage(),
			assert:                 assertLuaLocalDeclarationFields,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:     "make_conditional_consequence",
			source:   []byte("ifneq (a,b)\n\t@echo x\nelse\n\t@echo y\nendif"),
			language: MakeLanguage(),
			assert:   assertMakeConsequenceFields,
		},
		{
			name:     "zig_initializer_list",
			source:   []byte("const x = .{};\nconst y = .{\"x\"};"),
			language: ZigLanguage(),
			assert:   assertZigInitializerListFields,
		},
		{
			name:                   "haskell_root_sections",
			source:                 []byte("{-# LANGUAGE OverloadedStrings #-}\n-- | module docs\nmodule Main where\nimport Data.List\n\nx = 1"),
			language:               HaskellLanguage(),
			assert:                 assertHaskellRootSectionFields,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:     "erlang_root_forms",
			source:   []byte("% file comment\n-module(test).\n-export([main/0]).\nmain() ->\n  ok % inner comment\n  ."),
			language: ErlangLanguage(),
			assert:   assertErlangRootFormFields,
		},
		{
			name:     "dart_switch_expression_body",
			source:   []byte("int f(Object value) => switch (value) { int n => n, _ => 0 };\n"),
			language: DartLanguage(),
			assert:   assertDartSwitchExpressionBodyFields,
		},
		{
			name:     "elixir_nested_call_target",
			source:   []byte("def unquote(f)(x), do: nil\n"),
			language: ElixirLanguage(),
			assert:   assertElixirNestedCallTargetField,
		},
		{
			name: "scala_inherited_field_provenance",
			source: []byte(`import foo.bar.Baz
object Outer {
  private def search(value: Int): Int =
    if value == 0 then 1 else 2
}
`),
			language:               ScalaLanguage(),
			assert:                 assertScalaInheritedFieldProvenance,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name: "sql_into_field_provenance",
			source: []byte(`SELECT (SELECT 1), a
FROM (SELECT a FROM table) AS b;
SELECT a INTO b;
`),
			language: SqlLanguage(),
			assert:   assertSQLIntoFieldProvenance,
		},
	}
}

func assertLuaLocalDeclarationFields(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	if root == nil || root.Type(lang) != "chunk" || root.ChildCount() != 2 {
		t.Fatalf("unexpected Lua root: %v", root)
	}
	for index := 0; index < root.ChildCount(); index++ {
		if got := root.FieldNameForChild(index, lang); got != "local_declaration" {
			t.Fatalf("Lua child %d field = %q, want local_declaration", index, got)
		}
	}
}

func assertMakeConsequenceFields(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	conditional := findFirstNamedDescendantWhere(root, lang, "conditional", func(*gotreesitter.Node) bool { return true })
	if conditional == nil || conditional.ChildCount() < 4 {
		t.Fatalf("missing Make conditional: %s", root.SExpr(lang))
	}
	assertFieldName(t, conditional, lang, 1, "\t", "consequence")
	assertFieldName(t, conditional, lang, 2, "recipe_line", "consequence")

	elseDirective := conditional.Child(3)
	if elseDirective == nil || elseDirective.Type(lang) != "else_directive" || elseDirective.ChildCount() < 3 {
		t.Fatalf("missing Make else directive: %s", root.SExpr(lang))
	}
	assertFieldName(t, elseDirective, lang, 1, "\t", "consequence")
	assertFieldName(t, elseDirective, lang, 2, "recipe_line", "consequence")
}

func assertZigInitializerListFields(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	count := 0
	var visit func(*gotreesitter.Node)
	visit = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.Type(lang) == "anonymous_struct_initializer" {
			count++
			assertFieldName(t, node, lang, 1, "initializer_list", "")
		}
		for index := 0; index < node.ChildCount(); index++ {
			visit(node.Child(index))
		}
	}
	visit(root)
	if count != 2 {
		t.Fatalf("Zig initializer count = %d, want 2: %s", count, root.SExpr(lang))
	}
}

func assertHaskellRootSectionFields(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	if root == nil || root.HasError() || root.Type(lang) != "haskell" {
		t.Fatalf("unexpected Haskell root: %v", root)
	}
	wantFields := map[string]string{
		"imports":      "imports",
		"declarations": "declarations",
	}
	for index := 0; index < root.ChildCount(); index++ {
		want := wantFields[root.Child(index).Type(lang)]
		if got := root.FieldNameForChild(index, lang); got != want {
			t.Fatalf("Haskell child %d field = %q, want %q", index, got, want)
		}
	}
}

func assertErlangRootFormFields(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	if root == nil || root.HasError() || root.Type(lang) != "source_file" {
		t.Fatalf("unexpected Erlang root: %v", root)
	}
	for index := 0; index < root.ChildCount(); index++ {
		want := "forms_only"
		if root.Child(index).IsExtra() {
			want = ""
		}
		if got := root.FieldNameForChild(index, lang); got != want {
			t.Fatalf("Erlang child %d field = %q, want %q", index, got, want)
		}
	}
}

func assertDartSwitchExpressionBodyFields(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	switchExpression := findFirstNamedDescendantWhere(
		root,
		lang,
		"switch_expression",
		func(*gotreesitter.Node) bool { return true },
	)
	if switchExpression == nil {
		t.Fatalf("missing Dart switch expression: %s", root.SExpr(lang))
	}
	bodyFields := 0
	bodyStarted := false
	for index := 0; index < switchExpression.ChildCount(); index++ {
		child := switchExpression.Child(index)
		if child == nil {
			continue
		}
		field := switchExpression.FieldNameForChild(index, lang)
		if field == "body" {
			bodyFields++
			bodyStarted = true
			continue
		}
		if bodyStarted {
			t.Fatalf("Dart switch child %d field = %q after body start", index, field)
		}
	}
	if bodyFields < 2 {
		t.Fatalf("Dart switch body field count = %d, want at least 2", bodyFields)
	}
}

func assertElixirNestedCallTargetField(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	nested := findFirstNamedDescendantWhere(
		root,
		lang,
		"call",
		func(node *gotreesitter.Node) bool {
			return node.ChildCount() >= 2 &&
				node.Child(0) != nil &&
				node.Child(0).Type(lang) == "call" &&
				node.Child(1) != nil &&
				node.Child(1).Type(lang) == "arguments"
		},
	)
	if nested == nil {
		t.Fatalf("missing Elixir nested call: %s", root.SExpr(lang))
	}
	if got := nested.FieldNameForChild(0, lang); got != "target" {
		t.Fatalf("Elixir nested call target field = %q, want target", got)
	}
}

func assertScalaInheritedFieldProvenance(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected Scala root: %v", root)
	}
	importDeclaration := findFirstNamedDescendantWhere(
		root,
		lang,
		"import_declaration",
		func(*gotreesitter.Node) bool { return true },
	)
	if importDeclaration == nil {
		t.Fatalf("missing Scala import declaration: %s", root.SExpr(lang))
	}
	for index := 1; index < importDeclaration.ChildCount(); index++ {
		child := importDeclaration.Child(index)
		if child == nil {
			continue
		}
		if typ := child.Type(lang); typ != "identifier" && typ != "." {
			continue
		}
		if got := importDeclaration.FieldNameForChild(index, lang); got != "path" {
			t.Fatalf("Scala import child %d field = %q, want path", index, got)
		}
	}

	objectDefinition := findFirstNamedDescendantWhere(
		root,
		lang,
		"object_definition",
		func(*gotreesitter.Node) bool { return true },
	)
	if objectDefinition == nil {
		t.Fatalf("missing Scala object definition: %s", root.SExpr(lang))
	}
	var sawName, sawBody bool
	for index := 0; index < objectDefinition.ChildCount(); index++ {
		child := objectDefinition.Child(index)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "identifier":
			sawName = objectDefinition.FieldNameForChild(index, lang) == "name"
		case "template_body":
			sawBody = objectDefinition.FieldNameForChild(index, lang) == "body"
		}
	}
	if !sawName || !sawBody {
		t.Fatalf("Scala object fields are incomplete: %s", objectDefinition.SExpr(lang))
	}

	functionDefinition := findFirstNamedDescendantWhere(
		objectDefinition,
		lang,
		"function_definition",
		func(*gotreesitter.Node) bool { return true },
	)
	if functionDefinition == nil {
		t.Fatalf("missing Scala function definition: %s", objectDefinition.SExpr(lang))
	}
	wantFields := map[string]string{
		"identifier":      "name",
		"parameters":      "parameters",
		"type_identifier": "return_type",
	}
	sawFunctionBody := false
	for index := 0; index < functionDefinition.ChildCount(); index++ {
		child := functionDefinition.Child(index)
		if child == nil {
			continue
		}
		if child.Type(lang) == "modifiers" {
			if got := functionDefinition.FieldNameForChild(index, lang); got != "" {
				t.Fatalf("Scala modifiers field = %q, want empty", got)
			}
			continue
		}
		if functionDefinition.FieldNameForChild(index, lang) == "body" {
			sawFunctionBody = true
		}
		want, ok := wantFields[child.Type(lang)]
		if !ok {
			continue
		}
		if got := functionDefinition.FieldNameForChild(index, lang); got != want {
			t.Fatalf("Scala %s field = %q, want %q", child.Type(lang), got, want)
		}
		delete(wantFields, child.Type(lang))
	}
	if len(wantFields) != 0 || !sawFunctionBody {
		t.Fatalf("Scala function fields are incomplete: fields=%v body=%t", wantFields, sawFunctionBody)
	}

	ifExpression := findFirstNamedDescendantWhere(
		functionDefinition,
		lang,
		"if_expression",
		func(*gotreesitter.Node) bool { return true },
	)
	if ifExpression == nil {
		t.Fatalf("missing Scala if expression: %s", functionDefinition.SExpr(lang))
	}
	wantIfFields := map[string]bool{
		"condition":   false,
		"consequence": false,
		"alternative": false,
	}
	for index := 0; index < ifExpression.ChildCount(); index++ {
		field := ifExpression.FieldNameForChild(index, lang)
		if _, ok := wantIfFields[field]; ok {
			wantIfFields[field] = true
		}
	}
	for field, found := range wantIfFields {
		if !found {
			t.Fatalf("Scala if expression lacks %s: %s", field, ifExpression.SExpr(lang))
		}
	}
}

func assertSQLIntoFieldProvenance(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected SQL root: %v", root)
	}
	var bodies int
	var explicitInto int
	var visit func(*gotreesitter.Node)
	visit = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.Type(lang) == "select_clause_body" {
			bodies++
			hasIntoKeyword := false
			for index := 0; index < node.ChildCount(); index++ {
				child := node.Child(index)
				if child != nil && child.Type(lang) == "INTO" {
					hasIntoKeyword = true
				}
			}
			for index := 0; index < node.ChildCount(); index++ {
				if got := node.FieldNameForChild(index, lang); got == "into" {
					if !hasIntoKeyword {
						t.Fatalf("SQL body has an into field without INTO: %s", node.SExpr(lang))
					}
					explicitInto++
				}
			}
		}
		for index := 0; index < node.ChildCount(); index++ {
			visit(node.Child(index))
		}
	}
	visit(root)
	if bodies < 4 {
		t.Fatalf("SQL select body count = %d, want at least 4", bodies)
	}
	if explicitInto != 1 {
		t.Fatalf("SQL explicit into field count = %d, want 1", explicitInto)
	}
}

func assertFieldName(
	t *testing.T,
	parent *gotreesitter.Node,
	lang *gotreesitter.Language,
	index int,
	wantType string,
	wantField string,
) {
	t.Helper()
	child := parent.Child(index)
	if child == nil || child.Type(lang) != wantType {
		t.Fatalf("child %d type = %v, want %s", index, child, wantType)
	}
	if got := parent.FieldNameForChild(index, lang); got != wantField {
		t.Fatalf("child %d field = %q, want %q", index, got, wantField)
	}
}

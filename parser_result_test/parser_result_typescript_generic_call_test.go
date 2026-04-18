package parserresult_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTSXPredefinedGenericCallParsesAsCallExpression(t *testing.T) {
	const src = "const [inputValue, setInputValue] = useState<string>(values[0].toString())\n"
	tree, lang := parseByLanguageName(t, "tsx", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected tsx parse error: %s", root.SExpr(lang))
	}

	call := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "useState<string>(values[0].toString())"
	})
	if call == nil {
		t.Fatalf("tsx generic call missing call_expression: %s", root.SExpr(lang))
	}

	typeArgs := firstNode(call, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_arguments"
	})
	if typeArgs == nil {
		t.Fatalf("tsx generic call missing type_arguments: %s", call.SExpr(lang))
	}
	predefined := firstNode(typeArgs, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "predefined_type"
	})
	if predefined == nil || predefined.Text([]byte(src)) != "string" {
		t.Fatalf("tsx generic call missing predefined string type: %s", call.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "useState<string>(values[0].toString())"
	}); bad != nil {
		t.Fatalf("tsx generic call still parsed as binary_expression: %s", bad.SExpr(lang))
	}
}

func TestTSXCustomGenericCallStillParsesAsCallExpression(t *testing.T) {
	const src = "const x = foo<Bar>(baz)\n"
	tree, lang := parseByLanguageName(t, "tsx", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected tsx parse error: %s", root.SExpr(lang))
	}
	call := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "foo<Bar>(baz)"
	})
	if call == nil {
		t.Fatalf("tsx custom generic call missing call_expression: %s", root.SExpr(lang))
	}
	typeArgs := firstNode(call, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_arguments"
	})
	if typeArgs == nil {
		t.Fatalf("tsx custom generic call missing type_arguments: %s", call.SExpr(lang))
	}
	typeIdent := firstNode(typeArgs, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_identifier"
	})
	if typeIdent == nil || typeIdent.Text([]byte(src)) != "Bar" {
		t.Fatalf("tsx custom generic call lost type_identifier: %s", call.SExpr(lang))
	}
}

func TestTypeScriptCustomGenericCallStillParsesAsCallExpression(t *testing.T) {
	const src = "const x = createMissingNode<Identifier>(token(), reportAtCurrentPosition, diagnosticMessage)\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}

	call := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "createMissingNode<Identifier>(token(), reportAtCurrentPosition, diagnosticMessage)"
	})
	if call == nil {
		t.Fatalf("typescript custom generic call missing call_expression: %s", root.SExpr(lang))
	}
	typeArgs := firstNode(call, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_arguments"
	})
	if typeArgs == nil {
		t.Fatalf("typescript custom generic call missing type_arguments: %s", call.SExpr(lang))
	}
	typeIdent := firstNode(typeArgs, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_identifier"
	})
	if typeIdent == nil || typeIdent.Text([]byte(src)) != "Identifier" {
		t.Fatalf("typescript custom generic call lost type_identifier: %s", call.SExpr(lang))
	}
	args := firstNode(call, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "arguments"
	})
	if args == nil || args.ChildCount() != 7 {
		t.Fatalf("typescript custom generic call arguments child count = %d, want 7: %s", args.ChildCount(), call.SExpr(lang))
	}
}

func TestTSXNestedGenericCallStillParsesAsCallExpression(t *testing.T) {
	const src = "const [inputRef, setInputRef] = useState<React.RefObject<HTMLInputElement>>()\n"
	tree, lang := parseByLanguageName(t, "tsx", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected tsx parse error: %s", root.SExpr(lang))
	}

	call := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "useState<React.RefObject<HTMLInputElement>>()"
	})
	if call == nil {
		t.Fatalf("tsx nested generic call missing call_expression: %s", root.SExpr(lang))
	}
	typeArgs := firstNode(call, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_arguments"
	})
	if typeArgs == nil {
		t.Fatalf("tsx nested generic call missing type_arguments: %s", call.SExpr(lang))
	}
	if got := countNodes(call, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_arguments"
	}); got < 2 {
		t.Fatalf("tsx nested generic call type_arguments count = %d, want at least 2: %s", got, call.SExpr(lang))
	}
}

func TestTSXShiftExpressionStillUsesRightShiftToken(t *testing.T) {
	const src = "const x = foo >> bar\n"
	tree, lang := parseByLanguageName(t, "tsx", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected tsx parse error: %s", root.SExpr(lang))
	}
	shift := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "foo >> bar"
	})
	if shift == nil {
		t.Fatalf("tsx shift expression missing binary_expression: %s", root.SExpr(lang))
	}
}

func TestTypeScriptShiftExpressionStillUsesRightShiftToken(t *testing.T) {
	const src = "const x = foo >> bar\nconst y = foo>>bar\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	for _, expr := range []string{"foo >> bar", "foo>>bar"} {
		shift := firstNode(root, func(n *gotreesitter.Node) bool {
			return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == expr
		})
		if shift == nil {
			t.Fatalf("typescript shift expression missing binary_expression for %q: %s", expr, root.SExpr(lang))
		}
	}
}

func TestTypeScriptTypeAssertionOverTernaryParsesWithoutError(t *testing.T) {
	const src = "namespace ts {\n    function createNodeArray<T extends Node>(elements: T[], pos: number, end?: number): NodeArray<T> {\n        const length = elements.length;\n        const array = <MutableNodeArray<T>>(length >= 1 && length <= 4 ? elements.slice() : elements);\n        return array;\n    }\n}\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}

	typeAssertion := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_assertion" && n.Text([]byte(src)) == "<MutableNodeArray<T>>(length >= 1 && length <= 4 ? elements.slice() : elements)"
	})
	if typeAssertion == nil {
		t.Fatalf("missing type_assertion for compact close angles: %s", root.SExpr(lang))
	}
	ternary := firstNode(typeAssertion, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "ternary_expression"
	})
	if ternary == nil {
		t.Fatalf("type_assertion missing ternary_expression payload: %s", typeAssertion.SExpr(lang))
	}
}

func TestTypeScriptTypeAssertionOverCallExpressionParsesWithoutError(t *testing.T) {
	const src = "namespace ts {\n    function parseNamedImportsOrExports(kind: SyntaxKind) {\n        const node = createNode(kind);\n        node.elements = <NodeArray<ImportSpecifier> | NodeArray<ExportSpecifier>>parseBracketedList(ParsingContext.ImportOrExportSpecifiers,\n            kind === SyntaxKind.NamedImports ? parseImportSpecifier : parseExportSpecifier,\n            SyntaxKind.OpenBraceToken, SyntaxKind.CloseBraceToken);\n        return finishNode(node);\n    }\n}\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}

	typeAssertion := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "type_assertion" && n.Text([]byte(src)) == "<NodeArray<ImportSpecifier> | NodeArray<ExportSpecifier>>parseBracketedList(ParsingContext.ImportOrExportSpecifiers,\n            kind === SyntaxKind.NamedImports ? parseImportSpecifier : parseExportSpecifier,\n            SyntaxKind.OpenBraceToken, SyntaxKind.CloseBraceToken)"
	})
	if typeAssertion == nil {
		t.Fatalf("missing type_assertion for compact close-angle call: %s", root.SExpr(lang))
	}
	call := firstNode(typeAssertion, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression"
	})
	if call == nil {
		t.Fatalf("type_assertion missing call_expression payload: %s", typeAssertion.SExpr(lang))
	}
}

func TestTSXEnumAssignmentsDoNotInheritNameFieldFromEnumBody(t *testing.T) {
	const src = "export enum MaterialsInputType {\n  ELEMENTS = 'elements',\n}\n"
	tree, lang := parseByLanguageName(t, "tsx", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected tsx parse error: %s", root.SExpr(lang))
	}
	enumBody := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "enum_body"
	})
	if enumBody == nil {
		t.Fatalf("missing enum_body: %s", root.SExpr(lang))
	}
	assignment := firstNode(enumBody, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "enum_assignment"
	})
	if assignment == nil {
		t.Fatalf("missing enum_assignment: %s", enumBody.SExpr(lang))
	}
	foundIdx := -1
	for i := 0; i < enumBody.ChildCount(); i++ {
		if enumBody.Child(i) == assignment {
			foundIdx = i
			break
		}
	}
	if foundIdx < 0 {
		t.Fatalf("enum_assignment not found as direct enum_body child: %s", enumBody.SExpr(lang))
	}
	if got := enumBody.FieldNameForChild(foundIdx, lang); got != "" {
		t.Fatalf("enum_body.FieldNameForChild(%d) = %q, want empty", foundIdx, got)
	}
}

func TestTypeScriptUnaryCallPrecedenceStillWrapsCallExpression(t *testing.T) {
	const src = "!isNodeKind(kind)\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	unary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "unary_expression" && n.Text([]byte(src)) == "!isNodeKind(kind)"
	})
	if unary == nil {
		t.Fatalf("missing unary_expression for negated call: %s", root.SExpr(lang))
	}
	innerCall := firstNode(unary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "isNodeKind(kind)"
	})
	if innerCall == nil {
		t.Fatalf("unary_expression missing inner call_expression: %s", unary.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "!isNodeKind(kind)"
	}); bad != nil {
		t.Fatalf("negated call still parsed as call_expression: %s", bad.SExpr(lang))
	}
}

func TestTypeScriptLogicalAndCallPrecedenceStillKeepsBinaryExpression(t *testing.T) {
	const src = "node && cbNode(node)\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	binary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "node && cbNode(node)"
	})
	if binary == nil {
		t.Fatalf("missing binary_expression for logical-and call precedence: %s", root.SExpr(lang))
	}
	rightCall := firstNode(binary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "cbNode(node)"
	})
	if rightCall == nil {
		t.Fatalf("binary_expression missing rhs call_expression: %s", binary.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "node && cbNode(node)"
	}); bad != nil {
		t.Fatalf("logical-and expression still parsed as call_expression: %s", bad.SExpr(lang))
	}
}

func TestTypeScriptLogicalOrBetweenCallsStillKeepsBinaryExpression(t *testing.T) {
	const src = "visitNodes(cbNode, cbNodes, node.decorators) || visitNodes(cbNode, cbNodes, node.modifiers)\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	binary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "visitNodes(cbNode, cbNodes, node.decorators) || visitNodes(cbNode, cbNodes, node.modifiers)"
	})
	if binary == nil {
		t.Fatalf("missing binary_expression for logical-or call precedence: %s", root.SExpr(lang))
	}
	if got := countNodes(binary, func(n *gotreesitter.Node) bool { return n.Type(lang) == "call_expression" }); got < 2 {
		t.Fatalf("logical-or expression call count = %d, want at least 2: %s", got, binary.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "visitNodes(cbNode, cbNodes, node.decorators) || visitNodes(cbNode, cbNodes, node.modifiers)"
	}); bad != nil {
		t.Fatalf("logical-or expression still parsed as call_expression: %s", bad.SExpr(lang))
	}
}

func TestJavaScriptUnaryCallPrecedenceStillWrapsCallExpression(t *testing.T) {
	const src = "!isNodeKind(kind)\n"
	tree, lang := parseByLanguageName(t, "javascript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected javascript parse error: %s", root.SExpr(lang))
	}
	unary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "unary_expression" && n.Text([]byte(src)) == "!isNodeKind(kind)"
	})
	if unary == nil {
		t.Fatalf("missing unary_expression for negated JS call: %s", root.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "!isNodeKind(kind)"
	}); bad != nil {
		t.Fatalf("negated JS call still parsed as call_expression: %s", bad.SExpr(lang))
	}
}

func TestTypeScriptEqualityVsLogicalOrStillKeepsEqualityOnRight(t *testing.T) {
	const src = "token() === SyntaxKind.CloseBraceToken || token() === SyntaxKind.EndOfFileToken\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	binary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "token() === SyntaxKind.CloseBraceToken || token() === SyntaxKind.EndOfFileToken"
	})
	if binary == nil {
		t.Fatalf("missing binary_expression for equality/logical-or precedence: %s", root.SExpr(lang))
	}
	if got := countNodes(binary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && (n.Text([]byte(src)) == "token() === SyntaxKind.CloseBraceToken" || n.Text([]byte(src)) == "token() === SyntaxKind.EndOfFileToken")
	}); got < 2 {
		t.Fatalf("binary_expression equality count = %d, want at least 2: %s", got, binary.SExpr(lang))
	}
}

func TestTypeScriptLogicalOrChainStillKeepsEqualityOperands(t *testing.T) {
	const src = "tokenIsIdentifierOrKeyword(token()) || token() === SyntaxKind.StringLiteral || token() === SyntaxKind.NumericLiteral\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	binary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "tokenIsIdentifierOrKeyword(token()) || token() === SyntaxKind.StringLiteral || token() === SyntaxKind.NumericLiteral"
	})
	if binary == nil {
		t.Fatalf("missing binary_expression for logical-or/equality chain: %s", root.SExpr(lang))
	}
	if got := countNodes(binary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && (n.Text([]byte(src)) == "token() === SyntaxKind.StringLiteral" || n.Text([]byte(src)) == "token() === SyntaxKind.NumericLiteral")
	}); got < 2 {
		t.Fatalf("logical-or chain equality count = %d, want at least 2: %s", got, binary.SExpr(lang))
	}
}

func TestTypeScriptUnaryVsLogicalAndStillKeepsBinaryExpression(t *testing.T) {
	const src = "!noConditionalTypes && !scanner.hasPrecedingLineBreak()\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	binary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "!noConditionalTypes && !scanner.hasPrecedingLineBreak()"
	})
	if binary == nil {
		t.Fatalf("missing binary_expression for unary/logical-and precedence: %s", root.SExpr(lang))
	}
	if got := countNodes(binary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "unary_expression"
	}); got < 2 {
		t.Fatalf("unary/logical-and unary count = %d, want at least 2: %s", got, binary.SExpr(lang))
	}
}

func TestTypeScriptParenthesizedUnaryVsLogicalAndStillKeepsBinaryExpression(t *testing.T) {
	const src = "!(token() === SyntaxKind.SemicolonToken && inErrorRecovery) && isStartOfStatement()\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	binary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "!(token() === SyntaxKind.SemicolonToken && inErrorRecovery) && isStartOfStatement()"
	})
	if binary == nil {
		t.Fatalf("missing binary_expression for parenthesized unary/logical-and precedence: %s", root.SExpr(lang))
	}
	unary := firstNode(binary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "unary_expression" && n.Text([]byte(src)) == "!(token() === SyntaxKind.SemicolonToken && inErrorRecovery)"
	})
	if unary == nil {
		t.Fatalf("binary_expression missing unary lhs: %s", binary.SExpr(lang))
	}
}

func TestTypeScriptAssignmentRHSAsExpressionStillStaysOnRight(t *testing.T) {
	const src = "(result as Identifier).escapedText = \"\" as __String\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	assign := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "assignment_expression" && n.Text([]byte(src)) == "(result as Identifier).escapedText = \"\" as __String"
	})
	if assign == nil {
		t.Fatalf("missing assignment_expression for rhs as-expression: %s", root.SExpr(lang))
	}
	rhs := firstNode(assign, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "\"\" as __String"
	})
	if rhs == nil {
		t.Fatalf("assignment_expression missing rhs as_expression: %s", assign.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "(result as Identifier).escapedText = \"\" as __String"
	}); bad != nil {
		t.Fatalf("assignment rhs still wrapped by outer as_expression: %s", bad.SExpr(lang))
	}
}

func TestTypeScriptCallAssignmentRHSAsExpressionStillStaysOnRight(t *testing.T) {
	const src = "unaryMinusExpression = createNode(SyntaxKind.PrefixUnaryExpression) as PrefixUnaryExpression\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	assign := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "assignment_expression" && n.Text([]byte(src)) == "unaryMinusExpression = createNode(SyntaxKind.PrefixUnaryExpression) as PrefixUnaryExpression"
	})
	if assign == nil {
		t.Fatalf("missing assignment_expression for call rhs as-expression: %s", root.SExpr(lang))
	}
	rhs := firstNode(assign, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "createNode(SyntaxKind.PrefixUnaryExpression) as PrefixUnaryExpression"
	})
	if rhs == nil {
		t.Fatalf("assignment_expression missing rhs call as_expression: %s", assign.SExpr(lang))
	}
}

func TestTypeScriptTernaryFalseArmAsExpressionStillStaysOnFalseArm(t *testing.T) {
	const src = "token() === SyntaxKind.TrueKeyword || token() === SyntaxKind.FalseKeyword ? parseTokenNode<BooleanLiteral>() : parseLiteralLikeNode(token()) as LiteralExpression\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	ternary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "ternary_expression" && n.Text([]byte(src)) == "token() === SyntaxKind.TrueKeyword || token() === SyntaxKind.FalseKeyword ? parseTokenNode<BooleanLiteral>() : parseLiteralLikeNode(token()) as LiteralExpression"
	})
	if ternary == nil {
		t.Fatalf("missing ternary_expression for false-arm as-expression: %s", root.SExpr(lang))
	}
	trueCall := firstNode(ternary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "parseTokenNode<BooleanLiteral>()"
	})
	if trueCall == nil {
		t.Fatalf("ternary_expression missing generic true-arm call_expression: %s", ternary.SExpr(lang))
	}
	if typeArgs := firstNode(trueCall, func(n *gotreesitter.Node) bool { return n.Type(lang) == "type_arguments" }); typeArgs == nil {
		t.Fatalf("generic true-arm call_expression missing type_arguments: %s", trueCall.SExpr(lang))
	}
	falseArm := firstNode(ternary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "parseLiteralLikeNode(token()) as LiteralExpression"
	})
	if falseArm == nil {
		t.Fatalf("ternary_expression missing false-arm as_expression: %s", ternary.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "token() === SyntaxKind.TrueKeyword || token() === SyntaxKind.FalseKeyword ? parseTokenNode<BooleanLiteral>() : parseLiteralLikeNode(token()) as LiteralExpression"
	}); bad != nil {
		t.Fatalf("ternary still wrapped by outer as_expression: %s", bad.SExpr(lang))
	}
}

func TestTypeScriptAsUnionTypeStillBuildsTypeSide(t *testing.T) {
	const src = "createNode(kind) as JSDocVariadicType | JSDocNonNullableType\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	asExpr := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "createNode(kind) as JSDocVariadicType | JSDocNonNullableType"
	})
	if asExpr == nil {
		t.Fatalf("missing as_expression for union type: %s", root.SExpr(lang))
	}
	union := firstNode(asExpr, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "union_type"
	})
	if union == nil {
		t.Fatalf("as_expression missing union_type: %s", asExpr.SExpr(lang))
	}
	if got := countNodes(union, func(n *gotreesitter.Node) bool { return n.Type(lang) == "type_identifier" }); got < 2 {
		t.Fatalf("union_type type_identifier count = %d, want at least 2: %s", got, union.SExpr(lang))
	}
}

func TestTypeScriptAsUnionTypeChainStillBuildsNestedTypeSide(t *testing.T) {
	const src = "createNode(kind, type.pos) as JSDocOptionalType | JSDocNonNullableType | JSDocNullableType\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	asExpr := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "createNode(kind, type.pos) as JSDocOptionalType | JSDocNonNullableType | JSDocNullableType"
	})
	if asExpr == nil {
		t.Fatalf("missing as_expression for union chain: %s", root.SExpr(lang))
	}
	if got := countNodes(asExpr, func(n *gotreesitter.Node) bool { return n.Type(lang) == "union_type" }); got < 2 {
		t.Fatalf("union chain union_type count = %d, want at least 2: %s", got, asExpr.SExpr(lang))
	}
}

func TestTypeScriptAsIntersectionObjectTypeStillBuildsTypeSide(t *testing.T) {
	const src = "createNode(SyntaxKind.ExpressionWithTypeArguments) as ExpressionWithTypeArguments & { expression: Identifier | PropertyAccessEntityNameExpression }\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	asExpr := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "as_expression" && n.Text([]byte(src)) == "createNode(SyntaxKind.ExpressionWithTypeArguments) as ExpressionWithTypeArguments & { expression: Identifier | PropertyAccessEntityNameExpression }"
	})
	if asExpr == nil {
		t.Fatalf("missing as_expression for intersection object type: %s", root.SExpr(lang))
	}
	if firstNode(asExpr, func(n *gotreesitter.Node) bool { return n.Type(lang) == "intersection_type" }) == nil {
		t.Fatalf("as_expression missing intersection_type: %s", asExpr.SExpr(lang))
	}
	objectType := firstNode(asExpr, func(n *gotreesitter.Node) bool { return n.Type(lang) == "object_type" })
	if objectType == nil {
		t.Fatalf("as_expression missing object_type: %s", asExpr.SExpr(lang))
	}
	if firstNode(objectType, func(n *gotreesitter.Node) bool { return n.Type(lang) == "property_signature" }) == nil {
		t.Fatalf("object_type missing property_signature: %s", objectType.SExpr(lang))
	}
	if firstNode(objectType, func(n *gotreesitter.Node) bool { return n.Type(lang) == "type_annotation" }) == nil {
		t.Fatalf("object_type missing type_annotation: %s", objectType.SExpr(lang))
	}
	if firstNode(objectType, func(n *gotreesitter.Node) bool { return n.Type(lang) == "union_type" }) == nil {
		t.Fatalf("object_type missing nested union_type: %s", objectType.SExpr(lang))
	}
}

func TestTypeScriptCommentedLogicalOrCallChainStillKeepsBinaryExpression(t *testing.T) {
	const src = "identifier || // import id\n                token() === SyntaxKind.AsteriskToken || // import *\n                token() === SyntaxKind.OpenBraceToken\n"
	tree, lang := parseByLanguageName(t, "typescript", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected typescript parse error: %s", root.SExpr(lang))
	}
	binary := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "binary_expression" && n.Text([]byte(src)) == "identifier || // import id\n                token() === SyntaxKind.AsteriskToken || // import *\n                token() === SyntaxKind.OpenBraceToken"
	})
	if binary == nil {
		t.Fatalf("missing binary_expression for commented logical-or call chain: %s", root.SExpr(lang))
	}
	if got := countNodes(binary, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "token()"
	}); got < 2 {
		t.Fatalf("commented logical-or call count = %d, want at least 2: %s", got, binary.SExpr(lang))
	}
	if bad := firstNode(root, func(n *gotreesitter.Node) bool {
		return n.Type(lang) == "call_expression" && n.Text([]byte(src)) == "identifier || // import id\n                token() === SyntaxKind.AsteriskToken || // import *\n                token() === SyntaxKind.OpenBraceToken"
	}); bad != nil {
		t.Fatalf("commented logical-or chain still parsed as call_expression: %s", bad.SExpr(lang))
	}
}

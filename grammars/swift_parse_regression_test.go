package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestSwiftLineCommentsStayExtraComments(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte("// header\n//\n// body\nlet x = 1\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse swift comments: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("swift comment fixture has parse errors: %s", root.SExpr(lang))
	}
	if got, want := root.NamedChildCount(), 4; got != want {
		t.Fatalf("named child count = %d, want %d; tree: %s", got, want, root.SExpr(lang))
	}
	expectedCommentSpans := [][2]uint32{
		{0, 9},
		{10, 12},
		{13, 20},
	}
	for i, span := range expectedCommentSpans {
		child := root.NamedChild(i)
		if got := child.Type(lang); got != "comment" {
			t.Fatalf("named child %d type = %q, want comment; tree: %s", i, got, root.SExpr(lang))
		}
		if !child.IsExtra() {
			t.Fatalf("named child %d is not extra; tree: %s", i, root.SExpr(lang))
		}
		if got, want := child.StartByte(), span[0]; got != want {
			t.Fatalf("comment %d start = %d, want %d; tree: %s", i, got, want, root.SExpr(lang))
		}
		if got, want := child.EndByte(), span[1]; got != want {
			t.Fatalf("comment %d end = %d, want %d; tree: %s", i, got, want, root.SExpr(lang))
		}
	}
}

func TestSwiftMemberKeywordSelfAfterDotStaysNavigable(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte("let element = Element.self\nlet storage = _HashNode.Storage.self\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse swift member self: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("swift member self fixture has parse errors: %s", root.SExpr(lang))
	}
	sexpr := root.SExpr(lang)
	if got, want := strings.Count(sexpr, "(navigation_suffix (simple_identifier))"), 3; got != want {
		t.Fatalf("navigation suffix count = %d, want %d; tree: %s", got, want, sexpr)
	}
}

func TestSwiftImportThenClassParsesAsTopLevelDeclarations(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte("import Foundation\nclass Foo {}\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse swift import/class: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("swift import/class fixture has parse errors: %s", root.SExpr(lang))
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d; tree: %s", got, want, root.SExpr(lang))
	}
	sexpr := root.SExpr(lang)
	for _, want := range []string{"(import_declaration", "(class_declaration"} {
		if !strings.Contains(sexpr, want) {
			t.Fatalf("missing %s in tree: %s", want, sexpr)
		}
	}
}

func TestSwiftLicenseHeaderImportThenClassParses(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte(`//
//  Foo.swift
//
//  Copyright (c) 2025 Foo Foundation (http://example.org/)
//
//  Permission is hereby granted, free of charge, to any person obtaining a copy
//  of this software and associated documentation files (the "Software"), to deal

import Foundation
class Foo {}
`)
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse swift license header: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("swift license header fixture has parse errors: %s", root.SExpr(lang))
	}
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d; tree: %s", got, want, root.SExpr(lang))
	}
	sexpr := root.SExpr(lang)
	if strings.Count(sexpr, "(comment)") < 7 {
		t.Fatalf("license header comments were not preserved as comments: %s", sexpr)
	}
	for _, want := range []string{"(import_declaration", "(class_declaration"} {
		if !strings.Contains(sexpr, want) {
			t.Fatalf("missing %s in tree: %s", want, sexpr)
		}
	}
}

// countSwiftNodeType walks the tree and counts nodes of the given type.
func countSwiftNodeType(lang *gotreesitter.Language, n *gotreesitter.Node, typ string) int {
	if n == nil {
		return 0
	}
	count := 0
	if n.Type(lang) == typ {
		count++
	}
	for i := 0; i < n.ChildCount(); i++ {
		count += countSwiftNodeType(lang, n.Child(i), typ)
	}
	return count
}

// TestSwiftComparisonInConditionRecoversFunction is the regression test for
// issue #118: a comparison operator (< / > / ==) in an if/while condition used
// to make the body brace be consumed as a trailing closure, collapsing the
// whole function into ERROR nodes with no recoverable function_declaration.
func TestSwiftComparisonInConditionRecoversFunction(t *testing.T) {
	lang := SwiftLanguage()
	cases := []struct {
		name string
		src  string
	}{
		{"if-greater", "func a() { if x > 0 { foo() } }"},
		{"if-less", "func a() { if x < 0 { foo() } }"},
		{"if-equal", "func a() { if x == 0 { foo() } }"},
		{"while-greater", "func a() { while x > 0 { foo() } }"},
		{"compound", "func a() { if x > 0 && y < 1 { foo() } }"},
		{"nested", "func a() { if x > 0 { if y < 2 { foo() } } }"},
		{"if-else", "func a() { if x > 0 { a() } else { b() } }"},
		{"class-methods", "class C {\n  func a() { if x > 0 { foo() } }\n  func b() { bar() }\n}"},
		{"struct-method", "struct S {\n  func a() { if x > 0 { foo() } }\n}"},
		{"extension-method", "extension S {\n  func a() { if x > 0 { foo() } }\n}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("recovered tree still reports error: %s", root.SExpr(lang))
			}
			if got, want := root.EndByte(), uint32(len(src)); got != want {
				t.Fatalf("root end = %d, want %d (span not byte-faithful): %s", got, want, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "function_declaration"); got < 1 {
				t.Fatalf("function_declaration count = %d, want >= 1: %s", got, root.SExpr(lang))
			}
		})
	}
}

// TestSwiftComparisonConditionTreeIsFaithful checks that the recovered tree is
// structurally correct: a proper if_statement whose condition is the bare
// comparison_expression (the synthetic parenthesis used during recovery is
// stripped) with byte-faithful spans.
func TestSwiftComparisonConditionTreeIsFaithful(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte("func a() { if x > 0 { foo() } }")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	for _, want := range []string{"(function_declaration", "(if_statement", "(comparison_expression"} {
		if !strings.Contains(sexpr, want) {
			t.Fatalf("missing %s in recovered tree: %s", want, sexpr)
		}
	}
	// The synthetic parenthesis must not survive as a tuple_expression condition.
	if countSwiftNodeType(lang, root, "tuple_expression") != 0 {
		t.Fatalf("synthetic parenthesis leaked as tuple_expression: %s", sexpr)
	}
	if countSwiftNodeType(lang, root, "lambda_literal") != 0 {
		t.Fatalf("if body misparsed as trailing-closure lambda_literal: %s", sexpr)
	}
}

// issue #123: a `for…in` loop whose iterable is a range (`0..<n`, `0...n`) or a
// call expression (`stride(from:to:by:)`) used to make the loop body brace be
// consumed as a trailing closure, silently collapsing the enclosing function to
// _modifierless_function_declaration_no_body (with no ERROR node) and spilling
// the body statements out as siblings.
func TestSwiftForRangeIterableRecoversFunction(t *testing.T) {
	lang := SwiftLanguage()
	cases := []struct {
		name string
		src  string
	}{
		{"half-open-range", "func f(n: Int) -> Int {\n  var t = 0\n  for i in 0..<n { t += i }\n  return t\n}"},
		{"closed-range", "func f(n: Int) -> Int {\n  var t = 0\n  for i in 0...n { t += i }\n  return t\n}"},
		{"spaced-range", "func f(n: Int) -> Int {\n  var t = 0\n  for i in 0 ..< n { t += i }\n  return t\n}"},
		{"stride-call", "func f(n: Int) -> Int {\n  var t = 0\n  for i in stride(from: 0, to: n, by: 1) { t += i }\n  return t\n}"},
		{"class-method", "class C {\n  func f(n: Int) -> Int {\n    var t = 0\n    for i in 0..<n { t += i }\n    return t\n  }\n}"},
		{"struct-method", "struct S {\n  func f(n: Int) {\n    for i in 0...n { print(i) }\n  }\n}"},
		{"destructuring", "func f() { for (a, b) in zip(xs, ys) { print(a, b) } }"},
		// The loop variable is a backtick-escaped keyword, not the `in` separator.
		{"backtick-var", "func f(n: Int) { for `in` in 0..<n { print(`in`) } }"},
		// A Unicode loop variable must not be split at the `in` substring boundary.
		{"unicode-var", "func f(n: Int) { for π in 0..<n { print(π) } }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("recovered tree still reports error: %s", root.SExpr(lang))
			}
			if got, want := root.EndByte(), uint32(len(src)); got != want {
				t.Fatalf("root end = %d, want %d (span not byte-faithful): %s", got, want, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "function_declaration"); got < 1 {
				t.Fatalf("function_declaration count = %d, want >= 1: %s", got, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "for_statement"); got < 1 {
				t.Fatalf("for_statement count = %d, want >= 1: %s", got, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "_modifierless_function_declaration_no_body"); got != 0 {
				t.Fatalf("function collapsed to _modifierless_function_declaration_no_body: %s", root.SExpr(lang))
			}
			// The synthetic parenthesis used during recovery must be unwrapped, not
			// left as a tuple_expression iterable.
			if countSwiftNodeType(lang, root, "tuple_expression") != 0 {
				t.Fatalf("synthetic parenthesis leaked as tuple_expression: %s", root.SExpr(lang))
			}
		})
	}
}

// TestSwiftForBareIdentifierUnaffected guards against the recovery pass disturbing
// a for…in over a bare identifier, which already parses correctly.
func TestSwiftForBareIdentifierUnaffected(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte("func f(xs: [Int]) -> Int {\n  var t = 0\n  for x in xs { t += x }\n  return t\n}")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("clean for…in source reported error: %s", root.SExpr(lang))
	}
	if countSwiftNodeType(lang, root, "for_statement") != 1 {
		t.Fatalf("expected exactly one for_statement: %s", root.SExpr(lang))
	}
	if countSwiftNodeType(lang, root, "tuple_expression") != 0 {
		t.Fatalf("recovery pass wrapped a clean iterable in tuple_expression: %s", root.SExpr(lang))
	}
}

// TestSwiftNormalTrailingClosureUnaffected guards against the recovery pass
// disturbing a legitimate trailing closure (which must stay a lambda_literal).
func TestSwiftNormalTrailingClosureUnaffected(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte("func a() { items.map { x in x } }")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("clean trailing-closure source reported error: %s", root.SExpr(lang))
	}
	if countSwiftNodeType(lang, root, "lambda_literal") != 1 {
		t.Fatalf("expected exactly one lambda_literal: %s", root.SExpr(lang))
	}
}

// issue #131: an `if … else if …` chain used to collapse the enclosing function
// to _modifierless_function_declaration_no_body (with no ERROR node, like #123).
// The trailing-closure ambiguity recovery only found the first `if` token — the
// chained `if` keyword is swallowed into an ERROR node — so it bracketed only the
// first condition, leaving the else-if's body brace absorbed as a trailing closure
// and the function silently truncated. Following the chain through the source and
// requiring a byte-faithful reparse recovers every condition.
func TestSwiftElseIfChainRecoversFunction(t *testing.T) {
	lang := SwiftLanguage()
	cases := []struct {
		name string
		src  string
	}{
		{"else-if-trailing-return", "func f(_ x: Int) -> Int {\n    if x > 0 {\n        return 1\n    } else if x < 0 {\n        return 2\n    }\n    return 3\n}\n"},
		{"else-if-else", "func f(_ x: Int) -> Int {\n    if x > 0 {\n        return 1\n    } else if x < 0 {\n        return 2\n    } else {\n        return 3\n    }\n}\n"},
		{"three-way-chain", "func f(_ x: Int) -> Int {\n    if x > 0 {\n        return 1\n    } else if x < 0 {\n        return 2\n    } else if x == 5 {\n        return 5\n    } else {\n        return 3\n    }\n}\n"},
		{"oneline-chain", "func f(_ x: Int) -> Int { if x > 0 { return 1 } else if x < 0 { return 2 } else { return 3 } }"},
		{"else-if-no-return", "func f(_ x: Int) {\n    if x > 0 {\n        a()\n    } else if x < 0 {\n        b()\n    }\n}\n"},
		{"class-method-chain", "class C {\n  func f(_ x: Int) -> Int {\n    if x > 0 { return 1 } else if x < 0 { return 2 }\n    return 3\n  }\n}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("recovered tree still reports error: %s", root.SExpr(lang))
			}
			if got, want := root.EndByte(), uint32(len(src)); got != want {
				t.Fatalf("root end = %d, want %d (span not byte-faithful): %s", got, want, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "function_declaration"); got < 1 {
				t.Fatalf("function_declaration count = %d, want >= 1: %s", got, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "_modifierless_function_declaration_no_body"); got != 0 {
				t.Fatalf("function collapsed to _modifierless_function_declaration_no_body: %s", root.SExpr(lang))
			}
			// Each `else if` continuation must form a nested if_statement, not be
			// absorbed as a trailing-closure lambda_literal.
			if got := countSwiftNodeType(lang, root, "if_statement"); got < 2 {
				t.Fatalf("if_statement count = %d, want >= 2 (chain not nested): %s", got, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "lambda_literal"); got != 0 {
				t.Fatalf("else-if body misparsed as trailing-closure lambda_literal: %s", root.SExpr(lang))
			}
			// The synthetic parens injected during recovery must be unwrapped.
			if got := countSwiftNodeType(lang, root, "tuple_expression"); got != 0 {
				t.Fatalf("synthetic parenthesis leaked as tuple_expression: %s", root.SExpr(lang))
			}
		})
	}
}

// TestSwiftTernaryExpressionRecovers is the regression test for issue #135: the
// conditional operator `cond ? a : b` never fired the ternary_expression
// reduction on the runtime blob, dropping `? a : b` into an ERROR node in every
// position and collapsing any enclosing function. The recovery normalizer now
// reconstructs the ternary_expression in place.
func TestSwiftTernaryExpressionRecovers(t *testing.T) {
	lang := SwiftLanguage()
	cases := []struct {
		name string
		src  string
	}{
		{"top-level-let", "let y = 3 > 0 ? 1 : 2\n"},
		{"return-no-param", "func f() -> Int { return true ? 1 : 2 }\n"},
		{"return-with-param", "func f(x: Int) -> Int { return x > 0 ? 1 : 2 }\n"},
		{"call-argument", "func f(x: Int) { print(x > 0 ? 1 : 2) }\n"},
		{"string-operands", "let a = cond ? \"x\" : \"y\"\n"},
		{"nested-in-parens", "let z = a + (cond ? 1 : 2) + b\n"},
		{"parenthesised-condition", "let availableRules = (context == nil) ? [.noContextRule] : []\n"},
		{"enum-dot-operands", "let result: MyEnumType = condition ? .someEnumCase : .someOtherEnum\n"},
		{"multi-argument", "let m = foo(a, b > 0 ? 1 : 2)\n"},
		{"as-if-condition", "if a ? b : c == d {\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("recovered tree still reports error: %s", root.SExpr(lang))
			}
			if got, want := root.EndByte(), uint32(len(src)); got != want {
				t.Fatalf("root end = %d, want %d (span not byte-faithful): %s", got, want, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "ternary_expression"); got < 1 {
				t.Fatalf("ternary_expression count = %d, want >= 1: %s", got, root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "ERROR"); got != 0 {
				t.Fatalf("ERROR node present after recovery: %s", root.SExpr(lang))
			}
			if got := countSwiftNodeType(lang, root, "_modifierless_function_declaration_no_body"); got != 0 {
				t.Fatalf("function collapsed to _modifierless_function_declaration_no_body: %s", root.SExpr(lang))
			}
		})
	}
}

// TestSwiftTernaryFieldsAndShape checks the reconstructed ternary_expression has
// the exact upstream child layout: condition/if_true/if_false fields around the
// `?` and `:` tokens.
func TestSwiftTernaryFieldsAndShape(t *testing.T) {
	lang := SwiftLanguage()
	src := []byte("let y = a ? b : c\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("ternary tree reports error: %s", root.SExpr(lang))
	}
	var tern *gotreesitter.Node
	var find func(n *gotreesitter.Node)
	find = func(n *gotreesitter.Node) {
		if n == nil || tern != nil {
			return
		}
		if n.Type(lang) == "ternary_expression" {
			tern = n
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			find(n.Child(i))
		}
	}
	find(root)
	if tern == nil {
		t.Fatalf("no ternary_expression: %s", root.SExpr(lang))
	}
	for _, field := range []string{"condition", "if_true", "if_false"} {
		c := tern.ChildByFieldName(field, lang)
		if c == nil {
			t.Fatalf("ternary_expression missing %q field: %s", field, root.SExpr(lang))
		}
	}
	if got, want := tern.StartByte(), uint32(8); got != want {
		t.Fatalf("ternary start = %d, want %d", got, want)
	}
	if got, want := tern.EndByte(), uint32(17); got != want {
		t.Fatalf("ternary end = %d, want %d", got, want)
	}
}

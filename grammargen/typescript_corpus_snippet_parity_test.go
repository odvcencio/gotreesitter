package grammargen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

func TestTypeScriptCorpusSnippetParity(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight TypeScript parity generation under -race; non-race coverage keeps the generated-vs-reference check")
	}

	genLang, refLang := loadImportedParityLanguages(t, "typescript")
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "generic_call",
			src:  "f<T>(x)\n",
		},
		{
			name: "optional_chained_generic_call",
			src:  "A?.<B>();\n",
		},
		{
			name: "member_generic_call_and_nested_type_args",
			src:  "a.b<[C]>();\na<C.D[]>();\n",
		},
		{
			name: "generic_call_statement_no_arguments",
			src:  "function f() {\n  identifiers = createMap<string>();\n}\n",
		},
		{
			name: "import_alias_assignment",
			src:  "import r = X.N;\n",
		},
		{
			name: "module_identifier_expression_statement",
			src:  "var module;\nmodule;\n",
		},
		{
			name: "async_arrow_identifier",
			src:  "const x = async => async;\n",
		},
		{
			name: "unary_call_precedence",
			src:  "!isNodeKind(kind)\n",
		},
		{
			name: "logical_and_call_precedence",
			src:  "node && cbNode(node)\n",
		},
		{
			name: "logical_or_between_calls",
			src:  "visitNodes(cbNode, cbNodes, node.decorators) || visitNodes(cbNode, cbNodes, node.modifiers)\n",
		},
		{
			name: "equality_vs_logical_or_precedence",
			src:  "token() === SyntaxKind.CloseBraceToken || token() === SyntaxKind.EndOfFileToken\n",
		},
		{
			name: "logical_or_chain_with_equalities",
			src:  "tokenIsIdentifierOrKeyword(token()) || token() === SyntaxKind.StringLiteral || token() === SyntaxKind.NumericLiteral\n",
		},
		{
			name: "unary_vs_logical_and_precedence",
			src:  "!noConditionalTypes && !scanner.hasPrecedingLineBreak()\n",
		},
		{
			name: "parenthesized_unary_vs_logical_and_precedence",
			src:  "!(token() === SyntaxKind.SemicolonToken && inErrorRecovery) && isStartOfStatement()\n",
		},
		{
			name: "assignment_rhs_as_expression",
			src:  "(result as Identifier).escapedText = \"\" as __String\n",
		},
		{
			name: "assignment_rhs_call_as_expression",
			src:  "unaryMinusExpression = createNode(SyntaxKind.PrefixUnaryExpression) as PrefixUnaryExpression\n",
		},
		{
			name: "ternary_false_arm_as_expression",
			src:  "token() === SyntaxKind.TrueKeyword || token() === SyntaxKind.FalseKeyword ? parseTokenNode<BooleanLiteral>() : parseLiteralLikeNode(token()) as LiteralExpression\n",
		},
		{
			name: "as_union_type",
			src:  "createNode(kind) as JSDocVariadicType | JSDocNonNullableType\n",
		},
		{
			name: "as_union_type_chain",
			src:  "createNode(kind, type.pos) as JSDocOptionalType | JSDocNonNullableType | JSDocNullableType\n",
		},
		{
			name: "as_intersection_object_type",
			src:  "createNode(SyntaxKind.ExpressionWithTypeArguments) as ExpressionWithTypeArguments & { expression: Identifier | PropertyAccessEntityNameExpression }\n",
		},
		{
			name: "commented_logical_or_call_chain",
			src:  "identifier || // import id\n                token() === SyntaxKind.AsteriskToken || // import *\n                token() === SyntaxKind.OpenBraceToken\n",
		},
		{
			name: "if_statement_set_computed_subscript",
			src:  "if ( foo ) {\n\tset[ 1 ]\n}\n",
		},
		{
			name: "if_statement_set_computed_member_call",
			src:  "if ( foo ) {\n\tset[ 1 ].apply()\n}\n",
		},
		{
			name: "destructured_function_type_parameter",
			src:  "let foo: ({a}: Foo) => number\n",
		},
		{
			name: "const_type_parameters_function",
			src:  "function foo<const T, const U extends string>(x: T, y: U) {\n\n}\n",
		},
		{
			name: "template_literal_types",
			src:  "type A<B, C> = `${B}${C}`;\ntype A = `${B[0]}-foo-${C}-bar-${D<U, D>}`\n",
		},
		{
			name: "class_method_with_accessibility_and_string_concat",
			src:  "class A extends B {\n    constructor(x: number, y: number) {\n        super(x);\n    }\n    public toString() {\n        return super.toString() + \" y=\" + this.y;\n    }\n}\n",
		},
		{
			name: "functions_typed_parameters_corpus_block_exact",
			src:  "function greeter(person: string) {\n  return \"Hello, \" + person;\n}\n\nfunction foo<T>(x: T): T {\n\n}\n\nfunction foo<T, U>(a: T[], f: (x: T) => U): U[] {\n\n}\n\nfunction foo<T, U>(this: T[]): U[] {\n  return []\n}\n\nfunction foo<const T, const U extends string>(x: T, y: U) {\n\n}\n",
		},
		{
			name: "template_literal_types_corpus_block_exact",
			src:  "type A<B, C> = `${B}${C}`;\ntype A = `${B[0]}-foo-${C}-bar-${D<U, D>}`\ntype A = `[${'a'}${0}]`\ntype A<B, C> = B extends C\n  ? C extends string\n    ? `${C}${\"\" extends C ? \"\" : \".\"}${B}`\n    : never\n  : never\ntype Trim<S extends string> = S extends `${infer R}` ? Trim<R>  : S;\ntype A = `${true & ('foo' | false)}`;\ntype StringToNumber<S extends string> = S extends `${infer N extends number}` ? N : never;\n",
		},
		{
			name: "object_types_mapped_index_corpus_block_exact",
			src:  "let person: {name: string, age: number};\nlet thing: { [type: string]: string };\ntype T = { [K in keyof T]-?: any }\ntype T = { [K in keyof T]?: any }\ntype T = { -readonly [K in keyof T]: any }\n",
		},
		{
			name: "enum_declarations_corpus_block_exact",
			src:  "enum Test {\n    A,\n    'B',\n    'C' = Math.floor(Math.random() * 1000),\n    D = 10,\n    E\n}\n\nenum Style {\n    None = 0,\n    Bold = 1,\n    Italic = 2,\n    Underline = 4,\n    Emphasis = Bold | Italic,\n    Hyperlink = Bold | Underline\n}\n",
		},
		{
			name: "super_class_corpus_block_exact",
			src:  "class A extends B {\n    constructor(x: number, y: number) {\n        super(x);\n    }\n    public toString() {\n        return super.toString() + \" y=\" + this.y;\n    }\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertGeneratedAndReferenceParity(t, genLang, refLang, tt.src)
		})
	}
}

func TestTSXCorpusSnippetParity(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight TypeScript parity generation under -race; non-race coverage keeps the generated-vs-reference check")
	}

	genLang, refLang := loadImportedParityLanguages(t, "tsx")
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "generic_call",
			src:  "f<T>(x)\n",
		},
		{
			name: "optional_chained_generic_call",
			src:  "A?.<B>();\n",
		},
		{
			name: "member_generic_call_and_nested_type_args",
			src:  "a.b<[C]>();\na<C.D[]>();\n",
		},
		{
			name: "import_alias_assignment",
			src:  "import r = X.N;\n",
		},
		{
			name: "module_identifier_expression_statement",
			src:  "var module;\nmodule;\n",
		},
		{
			name: "async_arrow_identifier",
			src:  "const x = async => async;\n",
		},
		{
			name: "jsx_generic_ambiguity_from_functions_corpus",
			src:  "<A>(amount, interestRate, duration): number => 2\n\nfunction* foo<A>(amount, interestRate, duration): number {\n\tyield amount * interestRate * duration / 12\n}\n\n(module: any): number => 2\n",
		},
		{
			name: "if_statement_set_computed_subscript",
			src:  "if ( foo ) {\n\tset[ 1 ]\n}\n",
		},
		{
			name: "if_statement_set_computed_member_call",
			src:  "if ( foo ) {\n\tset[ 1 ].apply()\n}\n",
		},
		{
			name: "destructured_function_type_parameter",
			src:  "let foo: ({a}: Foo) => number\n",
		},
		{
			name: "const_type_parameters_function",
			src:  "function foo<const T, const U extends string>(x: T, y: U) {\n\n}\n",
		},
		{
			name: "template_literal_types",
			src:  "type A<B, C> = `${B}${C}`;\ntype A = `${B[0]}-foo-${C}-bar-${D<U, D>}`\n",
		},
		{
			name: "class_method_with_accessibility_and_string_concat",
			src:  "class A extends B {\n    constructor(x: number, y: number) {\n        super(x);\n    }\n    public toString() {\n        return super.toString() + \" y=\" + this.y;\n    }\n}\n",
		},
		{
			name: "functions_typed_parameters_corpus_block_exact",
			src:  "function greeter(person: string) {\n  return \"Hello, \" + person;\n}\n\nfunction foo<T>(x: T): T {\n\n}\n\nfunction foo<T, U>(a: T[], f: (x: T) => U): U[] {\n\n}\n\nfunction foo<T, U>(this: T[]): U[] {\n  return []\n}\n\nfunction foo<const T, const U extends string>(x: T, y: U) {\n\n}\n",
		},
		{
			name: "template_literal_types_corpus_block_exact",
			src:  "type A<B, C> = `${B}${C}`;\ntype A = `${B[0]}-foo-${C}-bar-${D<U, D>}`\ntype A = `[${'a'}${0}]`\ntype A<B, C> = B extends C\n  ? C extends string\n    ? `${C}${\"\" extends C ? \"\" : \".\"}${B}`\n    : never\n  : never\ntype Trim<S extends string> = S extends `${infer R}` ? Trim<R>  : S;\ntype A = `${true & ('foo' | false)}`;\ntype StringToNumber<S extends string> = S extends `${infer N extends number}` ? N : never;\n",
		},
		{
			name: "enum_declarations_corpus_block_exact",
			src:  "enum Test {\n    A,\n    'B',\n    'C' = Math.floor(Math.random() * 1000),\n    D = 10,\n    E\n}\n\nenum Style {\n    None = 0,\n    Bold = 1,\n    Italic = 2,\n    Underline = 4,\n    Emphasis = Bold | Italic,\n    Hyperlink = Bold | Underline\n}\n",
		},
		{
			name: "super_class_corpus_block_exact",
			src:  "class A extends B {\n    constructor(x: number, y: number) {\n        super(x);\n    }\n    public toString() {\n        return super.toString() + \" y=\" + this.y;\n    }\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertGeneratedAndReferenceParity(t, genLang, refLang, tt.src)
		})
	}
}

func TestTypeScriptDirectCRegressionDeepParity(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight TypeScript parity generation under -race; non-race coverage keeps the generated-vs-reference check")
	}

	assertImportedDeepParityCases(t, "typescript", []struct {
		name string
		src  string
	}{
		{name: "import_alias_assignment", src: "import r = X.N;\n"},
		{name: "module_identifier_expression_statement", src: "var module;\nmodule;\n"},
		{name: "async_arrow_identifier", src: "const x = async => async;\n"},
		{name: "generic_arrow_expression_statement", src: "<A>(amount, interestRate, duration): number => 2\n"},
		{name: "decorated_class_declaration", src: "@baz @bam class Foo {\n    @foo static 2: string;\n    @bar.buzz(grue) public static 2: string = 'string';\n    @readonly readonly 'hello'?: int = 'string';\n    @readonly fooBar(@required param: any, @optional param2?: any) {\n    }\n}\n"},
		{name: "decorator_call_type_arguments_in_class", src: "class Foo {\n  @bar<T>()\n  method() {\n  }\n}\n"},
		{name: "namespace_constructor_type_variable", src: "namespace ts {\n    const enum SignatureFlags {\n        None = 0,\n    }\n    let NodeConstructor: new (kind: SyntaxKind, pos: number, end: number) => Node;\n}\n"},
		{name: "commented_namespace_constructor_type_variable", src: "/// <reference path=\"utilities.ts\"/>\n/// <reference path=\"scanner.ts\"/>\n\nnamespace ts {\n    const enum SignatureFlags {\n        None = 0,\n    }\n    let NodeConstructor: new (kind: SyntaxKind, pos: number, end: number) => Node;\n}\n"},
		{name: "namespace_export_function_default_optional_parameter", src: "namespace ts {\n    export function createSourceFile(fileName: string, sourceText: string, languageVersion: ScriptTarget, setParentNodes = false, scriptKind?: ScriptKind): SourceFile {\n        return result;\n    }\n}\n"},
		{name: "function_default_optional_parameter", src: "function createSourceFile(fileName: string, sourceText: string, languageVersion: ScriptTarget, setParentNodes = false, scriptKind?: ScriptKind): SourceFile {\n    return result;\n}\n"},
		{name: "logical_and_unary_call_chain", src: "if (!noConditionalTypes && !scanner.hasPrecedingLineBreak() && parseOptional(SyntaxKind.ExtendsKeyword)) {\n}\n"},
		{name: "logical_and_nested_member_equality", src: "if (!ts.isIdentifier(a) && !ts.isIdentifier(b) && a.right.escapedText === b.right.escapedText) {\n}\n"},
		{name: "if_statement_set_computed_subscript", src: "if ( foo ) {\n\tset[ 1 ]\n}\n"},
		{name: "if_statement_set_computed_member_call", src: "if ( foo ) {\n\tset[ 1 ].apply()\n}\n"},
		{name: "destructured_function_type_parameter", src: "let foo: ({a}: Foo) => number\n"},
	})
}

func TestTypeScriptLargeParserCorpusDeepParity(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight TypeScript parity generation under -race; non-race coverage keeps the generated-vs-reference check")
	}

	path := filepath.Join("..", "cgo_harness", "corpus_real", "typescript", "large__parser.ts")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("large TypeScript corpus sample not found: %s", path)
	}
	if err != nil {
		t.Fatalf("read large TypeScript corpus sample: %v", err)
	}

	genLang, refLang := loadImportedParityLanguages(t, "typescript")
	genTree, err := gotreesitter.NewParser(genLang).Parse(data)
	if err != nil {
		t.Fatalf("generated parse: %v", err)
	}
	refTree, err := gotreesitter.NewParser(refLang).Parse(data)
	if err != nil {
		t.Fatalf("reference parse: %v", err)
	}

	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()
	if genRoot.HasError() != refRoot.HasError() {
		t.Logf("generated runtime: %s", genTree.ParseRuntime().Summary())
		t.Logf("reference runtime: %s", refTree.ParseRuntime().Summary())
		logFirstProblemNode(t, "gen", genRoot, genLang, data)
		logFirstProblemNode(t, "ref", refRoot, refLang, data)
		t.Fatalf("error mismatch: gen=%v ref=%v", genRoot.HasError(), refRoot.HasError())
	}
	divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10)
	if len(divs) > 0 {
		t.Fatalf("deep mismatch: %v", divs)
	}
}

func TestTSXDirectCRegressionDeepParity(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight TypeScript parity generation under -race; non-race coverage keeps the generated-vs-reference check")
	}

	assertImportedDeepParityCases(t, "tsx", []struct {
		name string
		src  string
	}{
		{name: "import_alias_assignment", src: "import r = X.N;\n"},
		{name: "module_identifier_expression_statement", src: "var module;\nmodule;\n"},
		{name: "async_arrow_identifier", src: "const x = async => async;\n"},
		{name: "decorated_class_declaration", src: "@baz @bam class Foo {\n    @foo static 2: string;\n    @bar.buzz(grue) public static 2: string = 'string';\n    @readonly readonly 'hello'?: int = 'string';\n    @readonly fooBar(@required param: any, @optional param2?: any) {\n    }\n}\n"},
		{name: "if_statement_set_computed_subscript", src: "if ( foo ) {\n\tset[ 1 ]\n}\n"},
		{name: "if_statement_set_computed_member_call", src: "if ( foo ) {\n\tset[ 1 ].apply()\n}\n"},
		{name: "destructured_function_type_parameter", src: "let foo: ({a}: Foo) => number\n"},
	})
}

func loadImportedParityLanguages(t *testing.T, grammarName string) (*gotreesitter.Language, *gotreesitter.Language) {
	t.Helper()

	var grammarSpec importParityGrammar
	for _, g := range importParityGrammars {
		if g.name == grammarName {
			grammarSpec = g
			break
		}
	}
	if grammarSpec.name == "" {
		t.Fatalf("%s import parity grammar not found", grammarName)
	}
	if grammarSpec.jsonPath != "" {
		grammarSpec.jsonPath = fallbackParitySeedPath(grammarSpec.jsonPath)
		if _, err := os.Stat(grammarSpec.jsonPath); err != nil {
			t.Skipf("%s grammar.json not available: %v", grammarName, err)
		}
	} else if grammarSpec.path != "" {
		grammarSpec.path = fallbackParitySeedPath(grammarSpec.path)
		if _, err := os.Stat(grammarSpec.path); err != nil {
			t.Skipf("%s grammar.js not available: %v", grammarName, err)
		}
	}

	gram, err := importParityGrammarSource(grammarSpec)
	if err != nil {
		t.Fatalf("import %s grammar: %v", grammarName, err)
	}

	timeout := grammarSpec.genTimeout
	if timeout == 0 {
		timeout = 180 * time.Second
	}
	genLang, err := generateWithTimeout(gram, timeout)
	if err != nil {
		t.Fatalf("generate %s language: %v", grammarName, err)
	}
	refLang := grammarSpec.blobFunc()
	adaptExternalScanner(refLang, genLang)
	return genLang, refLang
}

func assertGeneratedAndReferenceParity(t *testing.T, genLang, refLang *gotreesitter.Language, src string) {
	t.Helper()

	data := []byte(src)
	genTree, err := gotreesitter.NewParser(genLang).Parse(data)
	if err != nil {
		t.Fatalf("generated parse: %v", err)
	}
	refTree, err := gotreesitter.NewParser(refLang).Parse(data)
	if err != nil {
		t.Fatalf("reference parse: %v", err)
	}

	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()
	genSExpr := safeSExpr(genRoot, genLang, 256)
	refSExpr := safeSExpr(refRoot, refLang, 256)

	if genRoot.HasError() != refRoot.HasError() {
		if os.Getenv("DIAG_TS_CORPUS_SNIPPET") == "1" {
			logCorpusSnippetDiag(t, "gen", genLang, data)
			logCorpusSnippetDiag(t, "ref", refLang, data)
		}
		t.Fatalf("error mismatch: gen=%v ref=%v\nGEN: %s\nREF: %s", genRoot.HasError(), refRoot.HasError(), genSExpr, refSExpr)
	}
	if genSExpr != refSExpr {
		divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10)
		if os.Getenv("DIAG_TS_CORPUS_SNIPPET") == "1" {
			logCorpusSnippetDiag(t, "gen", genLang, data)
			logCorpusSnippetDiag(t, "ref", refLang, data)
		}
		if len(divs) == 0 {
			return
		}
		t.Fatalf("sexpr mismatch\nGEN: %s\nREF: %s\nDIVS: %v", genSExpr, refSExpr, divs)
	}
}

func assertImportedDeepParityCases(t *testing.T, grammarName string, cases []struct {
	name string
	src  string
}) {
	t.Helper()
	genLang, refLang := loadImportedParityLanguages(t, grammarName)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func assertGeneratedAndReferenceDeepParity(t *testing.T, genLang, refLang *gotreesitter.Language, src string) {
	t.Helper()

	data := []byte(src)
	genTree, err := gotreesitter.NewParser(genLang).Parse(data)
	if err != nil {
		t.Fatalf("generated parse: %v", err)
	}
	refTree, err := gotreesitter.NewParser(refLang).Parse(data)
	if err != nil {
		t.Fatalf("reference parse: %v", err)
	}

	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()
	if genRoot.HasError() != refRoot.HasError() {
		if os.Getenv("DIAG_TS_CORPUS_SNIPPET") == "1" {
			logCorpusSnippetDiag(t, "gen", genLang, data)
			logCorpusSnippetDiag(t, "ref", refLang, data)
		}
		t.Fatalf("error mismatch: gen=%v ref=%v\nGEN: %s\nREF: %s", genRoot.HasError(), refRoot.HasError(), safeSExpr(genRoot, genLang, 256), safeSExpr(refRoot, refLang, 256))
	}
	divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10)
	if len(divs) > 0 {
		if os.Getenv("DIAG_TS_CORPUS_SNIPPET") == "1" {
			logCorpusSnippetDiag(t, "gen", genLang, data)
			logCorpusSnippetDiag(t, "ref", refLang, data)
		}
		t.Fatalf("deep mismatch\nGEN: %s\nREF: %s\nDIVS: %v", safeSExpr(genRoot, genLang, 256), safeSExpr(refRoot, refLang, 256), divs)
	}
}

func logCorpusSnippetDiag(t *testing.T, label string, lang *gotreesitter.Language, src []byte) {
	t.Helper()

	parser := gotreesitter.NewParser(lang)
	parser.SetGLRTrace(true)
	parser.SetLogger(func(kind gotreesitter.ParserLogType, msg string) {
		switch kind {
		case gotreesitter.ParserLogLex:
			var sym, start, end int
			if _, err := fmt.Sscanf(msg, "token sym=%d start=%d end=%d", &sym, &start, &end); err == nil &&
				sym >= 0 && sym < len(lang.SymbolNames) && start >= 0 && end >= start && end <= len(src) {
				t.Logf("[%s][lex] sym=%d raw=%q text=%q start=%d end=%d", label, sym, lang.SymbolNames[sym], string(src[start:end]), start, end)
				return
			}
			t.Logf("[%s][lex] %s", label, msg)
		case gotreesitter.ParserLogParse:
			t.Logf("[%s][parse] %s", label, msg)
		}
	})

	tree, err := parser.Parse(src)
	if err != nil {
		t.Logf("[%s] parse error: %v", label, err)
		return
	}
	t.Logf("[%s] hasError=%v sexpr=%s", label, tree.RootNode().HasError(), safeSExpr(tree.RootNode(), lang, 256))
	logCorpusSnippetTreeOutline(t, label, tree.RootNode(), lang, src, 0, 6)
}

func logCorpusSnippetTreeOutline(t *testing.T, label string, node *gotreesitter.Node, lang *gotreesitter.Language, src []byte, depth, maxDepth int) {
	t.Helper()
	if node == nil || lang == nil || depth > maxDepth {
		return
	}
	text := ""
	if node.ChildCount() == 0 && node.EndByte() >= node.StartByte() && int(node.EndByte()) <= len(src) {
		text = string(src[node.StartByte():node.EndByte()])
	}
	t.Logf("[%s][tree] %s%s named=%v extra=%v error=%v range=%d..%d text=%q",
		label, strings.Repeat("  ", depth), node.Type(lang), node.IsNamed(), node.IsExtra(), node.HasError(), node.StartByte(), node.EndByte(), text)
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		field := node.FieldNameForChild(i, lang)
		if field != "" {
			t.Logf("[%s][tree] %sfield[%d]=%s", label, strings.Repeat("  ", depth+1), i, field)
		}
		logCorpusSnippetTreeOutline(t, label, child, lang, src, depth+1, maxDepth)
	}
}

func logFirstProblemNode(t *testing.T, label string, root *gotreesitter.Node, lang *gotreesitter.Language, src []byte) {
	t.Helper()
	node := firstProblemNode(root)
	if node == nil {
		t.Logf("[%s] no ERROR/MISSING node found", label)
		return
	}
	sexpr := safeSExpr(node, lang, 32)
	if len(sexpr) > 600 {
		sexpr = sexpr[:600] + "..."
	}
	t.Logf("[%s] first problem type=%s range=%d..%d point=%d:%d..%d:%d childCount=%d sexpr=%s",
		label,
		node.Type(lang),
		node.StartByte(),
		node.EndByte(),
		node.StartPoint().Row,
		node.StartPoint().Column,
		node.EndPoint().Row,
		node.EndPoint().Column,
		node.ChildCount(),
		sexpr)
	t.Logf("[%s] source window: %q", label, sourceWindow(src, node.StartByte(), node.EndByte(), 180))
	logProblemAncestry(t, label, node, lang)
}

func firstProblemNode(node *gotreesitter.Node) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	if !node.HasError() {
		return nil
	}
	for i := 0; i < node.ChildCount(); i++ {
		if child := firstProblemNode(node.Child(i)); child != nil {
			return child
		}
	}
	if node.IsError() || node.IsMissing() {
		return node
	}
	return nil
}

func sourceWindow(src []byte, start, end uint32, context int) string {
	if int(start) > len(src) {
		start = uint32(len(src))
	}
	if int(end) > len(src) {
		end = uint32(len(src))
	}
	if end < start {
		end = start
	}
	windowStart := int(start)
	if windowStart > context {
		windowStart -= context
	} else {
		windowStart = 0
	}
	windowEnd := int(end) + context
	if windowEnd > len(src) {
		windowEnd = len(src)
	}
	return string(src[windowStart:windowEnd])
}

func logProblemAncestry(t *testing.T, label string, node *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	for depth, cur := 0, node; cur != nil && depth < 8; depth, cur = depth+1, cur.Parent() {
		t.Logf("[%s] ancestor[%d] type=%s range=%d..%d childCount=%d named=%v error=%v missing=%v",
			label, depth, cur.Type(lang), cur.StartByte(), cur.EndByte(), cur.ChildCount(), cur.IsNamed(), cur.HasError(), cur.IsMissing())
		if cur.Parent() != nil {
			if prev := cur.PrevSibling(); prev != nil {
				t.Logf("[%s] ancestor[%d] prev=%s range=%d..%d error=%v", label, depth, prev.Type(lang), prev.StartByte(), prev.EndByte(), prev.HasError())
			}
			if next := cur.NextSibling(); next != nil {
				t.Logf("[%s] ancestor[%d] next=%s range=%d..%d error=%v", label, depth, next.Type(lang), next.StartByte(), next.EndByte(), next.HasError())
			}
		}
	}
}

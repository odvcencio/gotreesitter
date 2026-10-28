package gotreesitter_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseLanguageSample(t *testing.T, name, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()

	var entry grammars.LangEntry
	var report grammars.ParseSupport
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s language entry not found", name)
	}
	found = false
	for _, r := range grammars.AuditParseSupport() {
		if r.Name == name {
			report = r
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s parse support entry not found", name)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)

	var (
		tree *gotreesitter.Tree
		err  error
	)
	switch report.Backend {
	case grammars.ParseBackendTokenSource:
		tree, err = parser.ParseWithTokenSource(srcBytes, entry.TokenSourceFactory(srcBytes, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(srcBytes)
	default:
		t.Fatalf("unsupported %s backend: %s", name, report.Backend)
	}
	if err != nil {
		t.Fatalf("%s parse failed: %v", name, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s parse returned nil tree/root", name)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("%s parse has error: %s", name, tree.ParseRuntime().Summary())
	}
	return tree, lang
}

func TestParseSmokeSampleLeafTokenType(t *testing.T) {
	tests := []struct {
		name       string
		lang       string
		start, end uint32
		subject    string
		wantType   string
	}{
		{"AsmImmediateIntStaysInt", "asm", 19, 20, "immediate", "int"},
		{"FennelImmediateNumberStaysNumber", "fennel", 8, 9, "number", "number"},
		{"ForthBuiltinOperatorBeatsWord", "forth", 13, 14, "operator", "operator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := grammars.ParseSmokeSample(tt.lang)
			tree, lang := parseLanguageSample(t, tt.lang, src)
			t.Cleanup(tree.Release)

			node := tree.RootNode().NamedDescendantForByteRange(tt.start, tt.end)
			if node == nil {
				t.Fatalf("missing named descendant for %s %s", tt.lang, tt.subject)
			}
			if got, want := node.Type(lang), tt.wantType; got != want {
				t.Fatalf("%s %s type = %q, want %q", tt.lang, tt.subject, got, want)
			}
		})
	}
}

func TestParseRustRecoveredTopLevelImplItem(t *testing.T) {
	src := `
pub type ExplicitSelf = Spanned<SelfKind>;

impl Arg {
    pub fn to_self(&self) -> Option<ExplicitSelf> {
        if let PatKind::Ident(BindingMode::ByValue(mutbl), ident, _) = self.pat.node {
            if ident.node.name == keywords::SelfValue.name() {
                return match self.ty.node {
                    TyKind::ImplicitSelf => Some(respan(self.pat.span, SelfKind::Value(mutbl))),
                    _ => None,
                };
            }
        }
        None
    }
}
`
	tree, lang := parseLanguageSample(t, "rust", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.Type(lang), "source_file"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if root.HasError() {
		t.Fatalf("rust impl recovery left root with errors: %s", root.SExpr(lang))
	}
	if impl := findNamedChild(lang, root, "impl_item"); impl == nil {
		t.Fatalf("expected recovered impl_item, got %s", root.SExpr(lang))
	}
}

func TestParseMesonCommandArgumentPrefersVariableunit(t *testing.T) {
	src := grammars.ParseSmokeSample("meson")
	tree, lang := parseLanguageSample(t, "meson", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("meson root child count = %d, want %d", got, want)
	}
	cmd := root.Child(0)
	if cmd == nil {
		t.Fatal("meson root child is nil")
	}
	if got, want := cmd.Type(lang), "normal_command"; got != want {
		t.Fatalf("meson root child type = %q, want %q", got, want)
	}
	arg := cmd.Child(2)
	if arg == nil {
		t.Fatal("meson command argument child is nil")
	}
	if got, want := arg.Type(lang), "variableunit"; got != want {
		t.Fatalf("meson command argument type = %q, want %q", got, want)
	}
}

func TestParseJavaCollapsedModifierAndWildcardChildren(t *testing.T) {
	src := "package p;\n\nimport com.example.*;\n\nclass X { private X() {} }\n"
	tree, lang := parseLanguageSample(t, "java", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	modifiers := firstNodeByTypeAndText(root, lang, []byte(src), "modifiers", "private")
	if modifiers == nil {
		t.Fatalf("missing Java private modifiers node: %s", root.SExpr(lang))
	}
	if got, want := modifiers.ChildCount(), 1; got != want {
		t.Fatalf("modifiers.ChildCount() = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if child := modifiers.Child(0); child == nil || child.Type(lang) != "private" {
		if child == nil {
			t.Fatalf("modifiers child = nil; root=%s", root.SExpr(lang))
		}
		t.Fatalf("modifiers child type = %q, want private; root=%s", child.Type(lang), root.SExpr(lang))
	}

	asterisk := firstNodeByTypeAndText(root, lang, []byte(src), "asterisk", "*")
	if asterisk == nil {
		t.Fatalf("missing Java asterisk node: %s", root.SExpr(lang))
	}
	if got, want := asterisk.ChildCount(), 1; got != want {
		t.Fatalf("asterisk.ChildCount() = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if child := asterisk.Child(0); child == nil || child.Type(lang) != "*" {
		if child == nil {
			t.Fatalf("asterisk child = nil; root=%s", root.SExpr(lang))
		}
		t.Fatalf("asterisk child type = %q, want *; root=%s", child.Type(lang), root.SExpr(lang))
	}
}

func TestParseJavaDottedFieldAssignmentStatement(t *testing.T) {
	src := `class X {
  @Generated("com.github.javaparser.generator.metamodel.MetaModelGenerator")
  private static void initializePropertyMetaModels() {
    nodeMetaModel.commentPropertyMetaModel = new PropertyMetaModel(
        nodeMetaModel,
        "comment",
        com.github.javaparser.ast.comments.Comment.class,
        Optional.of(commentMetaModel),
        true,
        false,
        false,
        false);
  }

  void f() {
    nodeMetaModel.commentPropertyMetaModel = new PropertyMetaModel(nodeMetaModel, "comment");
  }
}
`
	entry := grammars.DetectLanguage("Test.java")
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.ParseWithTokenSource([]byte(src), entry.TokenSourceFactory([]byte(src), lang))
	if err != nil {
		t.Fatalf("java parse failed: %v", err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	t.Logf("runtime: %s", tree.ParseRuntime().Summary())
	sexpr := root.SExpr(lang)
	if root.HasError() {
		t.Fatalf("java parse has error: %s root=%s", tree.ParseRuntime().Summary(), sexpr)
	}
	if !strings.Contains(sexpr, "(expression_statement (assignment_expression (field_access") {
		t.Fatalf("missing dotted field assignment expression; root=%s", sexpr)
	}
	if strings.Contains(sexpr, "local_variable_declaration") {
		t.Fatalf("dotted field assignment parsed as local variable declaration; root=%s", sexpr)
	}
}

func TestParsePythonCollapsedWildcardImportChild(t *testing.T) {
	src := "from os import *\n"
	tree, lang := parseLanguageSample(t, "python", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	wildcard := firstNodeByTypeAndText(root, lang, []byte(src), "wildcard_import", "*")
	if wildcard == nil {
		t.Fatalf("missing Python wildcard_import node: %s", root.SExpr(lang))
	}
	if got, want := wildcard.ChildCount(), 1; got != want {
		t.Fatalf("wildcard_import.ChildCount() = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if child := wildcard.Child(0); child == nil || child.Type(lang) != "*" {
		if child == nil {
			t.Fatalf("wildcard_import child = nil; root=%s", root.SExpr(lang))
		}
		t.Fatalf("wildcard_import child type = %q, want *; root=%s", child.Type(lang), root.SExpr(lang))
	}
}

func TestParsePythonCollapsedAsPatternTargetIdentifier(t *testing.T) {
	src := "with manager() as target:\n    pass\n"
	tree, lang := parseLanguageSample(t, "python", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	target := firstNodeByTypeAndText(root, lang, []byte(src), "as_pattern_target", "target")
	if target == nil {
		t.Fatalf("missing Python as_pattern_target node: %s", root.SExpr(lang))
	}
	if got, want := target.ChildCount(), 1; got != want {
		t.Fatalf("as_pattern_target.ChildCount() = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
	if child := target.Child(0); child == nil || child.Type(lang) != "identifier" {
		if child == nil {
			t.Fatalf("as_pattern_target child = nil; root=%s", root.SExpr(lang))
		}
		t.Fatalf("as_pattern_target child type = %q, want identifier; root=%s", child.Type(lang), root.SExpr(lang))
	}
}

func firstNodeByTypeAndText(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte, typ, text string) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.Type(lang) == typ && root.Text(source) == text {
		return root
	}
	for _, child := range root.Children() {
		if got := firstNodeByTypeAndText(child, lang, source, typ, text); got != nil {
			return got
		}
	}
	return nil
}

func TestParseJSXSingleStatementRoot(t *testing.T) {
	tests := []struct {
		name string
		lang string
		src  string
	}{
		{"JavaScriptSelfClosingAttributeExpression", "javascript", "const el = <Avatar userId={foo.creatorId} />\n"},
		{"JavaScriptNamespacedSpreadChildren", "javascript", "const el = <Foo:Bar bar={}>{...children}</Foo:Bar>\n"},
		{"TSXSelfClosingAttributeExpression", "tsx", "const el = <Avatar userId={foo.creatorId} />\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, lang := parseLanguageSample(t, tt.lang, tt.src)
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			if got, want := root.ChildCount(), 1; got != want {
				t.Fatalf("%s root child count = %d, want %d", tt.lang, got, want)
			}
			stmt := root.Child(0)
			if stmt == nil {
				t.Fatalf("%s root child is nil", tt.lang)
			}
			if got, want := stmt.Type(lang), "lexical_declaration"; got != want {
				t.Fatalf("%s root child type = %q, want %q", tt.lang, got, want)
			}
		})
	}
}

func TestParseTSXGenericCallUnionTypeArgument(t *testing.T) {
	for _, src := range []string{
		"const [error, setError] = useState<string | null>(null);\n",
		"const [value, setValue] = useState<string | undefined>(() => undefined);\n",
	} {
		t.Run(src, func(t *testing.T) {
			tree, lang := parseLanguageSample(t, "tsx", src)
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			pos := strings.Index(src, "useState")
			if pos < 0 {
				t.Fatal("useState not found in sample")
			}
			node := root.NamedDescendantForByteRange(uint32(pos), uint32(pos+len("useState")))
			if node == nil {
				t.Fatal("missing useState descendant")
			}
			var call *gotreesitter.Node
			for cur := node; cur != nil; cur = cur.Parent() {
				if cur.Type(lang) == "call_expression" {
					call = cur
					break
				}
			}
			if call == nil {
				t.Fatalf("missing call_expression around useState: %s", root.SExpr(lang))
			}
			sexpr := call.SExpr(lang)
			if !strings.Contains(sexpr, "type_arguments") || !strings.Contains(sexpr, "union_type") || !strings.Contains(sexpr, "literal_type") {
				t.Fatalf("useState call did not preserve union type arguments: %s", sexpr)
			}
		})
	}
}

func TestParseTSXOptionalChainKeepsTokenChild(t *testing.T) {
	src := "const value = elements?.concat(wildcards);\n"
	tree, lang := parseLanguageSample(t, "tsx", src)
	t.Cleanup(tree.Release)

	pos := strings.Index(src, "?.")
	if pos < 0 {
		t.Fatal("optional chain token not found in sample")
	}
	node := tree.RootNode().NamedDescendantForByteRange(uint32(pos), uint32(pos+2))
	if node == nil {
		t.Fatal("missing optional_chain descendant")
	}
	for node != nil && node.Type(lang) != "optional_chain" {
		node = node.Parent()
	}
	if node == nil {
		t.Fatalf("missing optional_chain node: %s", tree.RootNode().SExpr(lang))
	}
	// C tree-sitter emits (optional_chain (?.)) — the visible "?." anonymous
	// token is a child, so childCount==1. The Go parser must match.
	if got, want := node.ChildCount(), 1; got != want {
		t.Fatalf("optional_chain child count = %d, want %d; root=%s", got, want, tree.RootNode().SExpr(lang))
	}
	if got, want := node.Child(0).Type(lang), "?."; got != want {
		t.Fatalf("optional_chain child type = %q, want %q; root=%s", got, want, tree.RootNode().SExpr(lang))
	}
}

func TestParseJavaScriptTypeScriptDynamicImportKeepsKeywordChild(t *testing.T) {
	for _, tc := range []struct {
		lang string
		src  string
	}{
		{lang: "javascript", src: "async function load(name) { return import(`./${name}.js`); }\n"},
		{lang: "typescript", src: "async function load(name: string) { return import(\"fs\"); }\n"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			tree, lang := parseLanguageSample(t, tc.lang, tc.src)
			t.Cleanup(tree.Release)

			node := firstNodeByTypeAndText(tree.RootNode(), lang, []byte(tc.src), "import", "import")
			if node == nil {
				t.Fatalf("missing dynamic import node: %s", tree.RootNode().SExpr(lang))
			}
			if got, want := node.ChildCount(), 1; got != want {
				t.Fatalf("dynamic import child count = %d, want %d; root=%s", got, want, tree.RootNode().SExpr(lang))
			}
			child := node.Child(0)
			if child == nil {
				t.Fatal("dynamic import keyword child is nil")
			}
			if got, want := child.Type(lang), "import"; got != want {
				t.Fatalf("dynamic import child type = %q, want %q", got, want)
			}
			if child.IsNamed() {
				t.Fatal("dynamic import keyword child is named, want anonymous")
			}
		})
	}
}

// TestParseTypeScriptFamilyShapes parses each sample and checks the root is a
// clean "program" whose S-expression holds every want substring and none of
// the absent ones.
func TestParseTypeScriptFamilyShapes(t *testing.T) {
	tests := []struct {
		name   string
		lang   string
		src    string
		want   []string
		absent []string
	}{
		{
			name: "TSXTypedArrowParameters",
			lang: "tsx",
			src:  "export const renderTrack = (values: number[], domain: number[], colors: string[]) => { return null; };\n",
			want: []string{"arrow_function", "formal_parameters"},
		},
		{
			name: "TypeScriptTypedArrowParameters",
			lang: "typescript",
			src:  "const f = (str: string) => str;\n",
			want: []string{"arrow_function", "formal_parameters"},
		},
		// The TypeScriptBareDefaultParameter case guards against a v0.43.0 regression
		// where "function f(a = 1) {}" collapsed to a top-level ERROR. The locked C
		// tree-sitter-typescript oracle parses this as
		// "(required_parameter pattern: (identifier) value: (number))"; our GLR
		// engine's steady-state cap-one merge budget was discarding the
		// pattern-reduction derivation in favor of a same-state, lower-precedence
		// assignment_expression derivation before the parse could complete. Fixed by
		// the structure-aware cap-two steady state (typeScriptSteadyStateMergeCap in
		// parser_retry.go), which subsumed the source-text detector originally added
		// for this shape.
		{
			name:   "TypeScriptBareDefaultParameter",
			lang:   "typescript",
			src:    "function f(a = 1) {}\n",
			want:   []string{"function_declaration", "required_parameter"},
			absent: []string{"ERROR"},
		},
		{
			name:   "TSXBareDefaultParameter",
			lang:   "tsx",
			src:    "function f(a = 1) {}\n",
			want:   []string{"function_declaration", "required_parameter"},
			absent: []string{"ERROR"},
		},
		// The TypeScriptArrayElementDefaultParameter case guards the array-element
		// default parameter shape ("[a = 1]") that the bare-default-parameter
		// detector was widened to cover: unlike a whole-pattern default
		// ("{a} = {}"), the identifier here sits directly before '=', so it hits the
		// same required_parameter/assignment_expression fork as "function f(a = 1)".
		// This is the React-hook tuple pattern ("const [state, setState] = ...")
		// with an initial-value default on the first slot.
		{
			name: "TypeScriptArrayElementDefaultParameter",
			lang: "typescript",
			src:  "function f([a = 1]) {}\n",
			want: []string{"required_parameter", "array_pattern", "assignment_pattern"},
		},
		{
			name: "TSXArrayElementDefaultParameter",
			lang: "tsx",
			src:  "function f([a = 1]) {}\n",
			want: []string{"required_parameter", "array_pattern", "assignment_pattern"},
		},
		// The TypeScriptReactHookTupleDefaultParameter case is the flagship
		// real-world shape motivating the array-element widening: a destructured
		// tuple parameter with a default on the state slot, as commonly authored in
		// custom React hook wrappers, e.g. "function useToggle([state = false,
		// setState]) {}".
		{
			name: "TypeScriptReactHookTupleDefaultParameter",
			lang: "typescript",
			src:  "function h([state = 0, setState]) {}\n",
			want: []string{"(array_pattern (assignment_pattern (identifier) (number)) (identifier))"},
		},
		// The TypeScriptRenamedPropertyDefaultParameter case guards the renamed
		// destructured-property default shape ("{a: b = 1}"): the preceding-context
		// gate widened to accept ':' immediately before the bound identifier for
		// this case.
		{
			name: "TypeScriptRenamedPropertyDefaultParameter",
			lang: "typescript",
			src:  "function f({a: b = 1}) {}\n",
			want: []string{"required_parameter", "object_pattern", "pair_pattern", "assignment_pattern"},
		},
		// The TypeScriptUnicodeIdentifierDefaultParameter case guards the bare
		// default parameter shape when the parameter name is a multi-byte UTF-8
		// identifier (a case the retired source-text detector's unicode-aware byte
		// scan used to require; the structure-aware cap-two steady state has no such
		// requirement, but the shape stays pinned here).
		{
			name: "TypeScriptUnicodeIdentifierDefaultParameter",
			lang: "typescript",
			src:  "function f(café = 1) {}\n",
			want: []string{"required_parameter"},
		},
		// The TypeScriptCommentAdjacentDefaultParameter case guards a block comment
		// sitting between the opening '(' and the defaulted identifier: the gate's
		// backward scan must skip over a complete "/* ... */" comment (in addition
		// to whitespace) to still see the '(' context character.
		{
			name: "TypeScriptCommentAdjacentDefaultParameter",
			lang: "typescript",
			src:  "function f(/* c */ a = 1) {}\n",
			want: []string{"comment", "required_parameter"},
		},
		// The TypeScriptArrayDestructuringDeclarationDefaultValue case documents
		// current behavior for a destructuring DECLARATION (not a parameter list)
		// with an array-element default: "const [a = 1] = arr;". This shares the
		// same required_parameter-shaped fork as the parameter-list case -- '[' is
		// now an accepted preceding-context gate character -- so the widened
		// detector incidentally also fixes this declaration shape even though this
		// pass only targeted parameter lists. This test asserts the actual observed
		// behavior (no error) rather than assuming it is unfixed; if a future change
		// to the detector's gate narrows it back to parameter-list-only contexts,
		// this test will fail and should be revisited rather than silently loosened.
		{
			name: "TypeScriptArrayDestructuringDeclarationDefaultValue",
			lang: "typescript",
			src:  "const [a = 1] = arr;\n",
			want: []string{"array_pattern", "assignment_pattern"},
		},
		// The TypeScriptExportArrowReturnTypeAnnotation case covers the "export
		// const" variant of issue #402: the export wrapper must not change whether
		// the widening detector fires.
		{
			name: "TypeScriptExportArrowReturnTypeAnnotation",
			lang: "typescript",
			src:  "export const f = (a: A): B => { return a; };\n",
			want: []string{"export_statement", "lexical_declaration", "arrow_function", "type_annotation"},
		},
		// The TypeScriptArrowParenthesizedReturnType case guards a sibling shape of
		// issue #402 found in PR review: a typed-parameter arrow whose return-type
		// annotation is itself parenthesized ("(a: A): (B) => a"). The original
		// fix's backward colon-scan stopped at the first ')' it met, so it never
		// walked past the return type's own closing paren to find the true
		// return-type colon. The scan now balances parens while walking backward so
		// it only accepts a colon seen at paren depth zero.
		{
			name: "TypeScriptArrowParenthesizedReturnType",
			lang: "typescript",
			src:  "const f = (a: A): (B) => a;\n",
			want: []string{"arrow_function", "formal_parameters", "required_parameter", "parenthesized_type"},
		},
		// The TypeScriptArrowUnionInParensReturnType case covers a union type inside
		// a parenthesized return-type annotation ("(a: A): (string | number) =>
		// a"), the shape that motivated the balanced-paren backward scan: the union
		// members and the "|" separator sit between the return type's own
		// parentheses, well within the scan's 512-byte window, and must not be
		// mistaken for a top-level colon boundary.
		{
			name: "TypeScriptArrowUnionInParensReturnType",
			lang: "typescript",
			src:  "const f = (a: A): (string | number) => a;\n",
			want: []string{"arrow_function", "parenthesized_type", "union_type"},
		},
		// The TypeScriptArrowFunctionTypeReturnType case covers a nested
		// function-type return annotation ("(a: A): (() => B) => a"): the return
		// type itself contains an inner "=>" and its own parenthesized empty
		// parameter list. The detector's outer loop tries every "=>" occurrence in
		// source order, so a non-matching inner "=>" (its own backward scan lands
		// on the outer arrow's parameter-list colon, which is not immediately
		// preceded by ')') falls through cleanly to the real, outer arrow.
		{
			name: "TypeScriptArrowFunctionTypeReturnType",
			lang: "typescript",
			src:  "const f = (a: A): (() => B) => a;\n",
			want: []string{"arrow_function", "parenthesized_type", "function_type"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, lang := parseLanguageSample(t, tt.lang, tt.src)
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			sexpr := root.SExpr(lang)
			if root.Type(lang) != "program" || root.HasError() {
				t.Fatalf("%s root = %s hasError=%v; tree=%s", tt.name, root.Type(lang), root.HasError(), sexpr)
			}
			for _, want := range tt.want {
				if !strings.Contains(sexpr, want) {
					t.Fatalf("%s missing %s: %s", tt.name, want, sexpr)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(sexpr, absent) {
					t.Fatalf("%s retained %s: %s", tt.name, absent, sexpr)
				}
			}
		})
	}
}

// TestParseTypeScriptMultipleBareDefaultParameters guards the multi-parameter
// shape of the same regression: each default-valued parameter forks and must
// independently reconverge without losing its required_parameter reduction.
func TestParseTypeScriptMultipleBareDefaultParameters(t *testing.T) {
	src := "function f(a = 1, b = 2) {}\n"
	tree, lang := parseLanguageSample(t, "typescript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("typescript multiple default parameters root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), root.SExpr(lang))
	}
	sexpr := root.SExpr(lang)
	if got, want := strings.Count(sexpr, "required_parameter"), 2; got != want {
		t.Fatalf("typescript multiple default parameters required_parameter count = %d, want %d: %s", got, want, sexpr)
	}
}

// TestParseTypeScriptArrowAndMethodBareDefaultParameter covers the two other
// formal_parameters call sites (arrow functions and class methods) that share
// the same required_parameter production and were equally affected.
func TestParseTypeScriptArrowAndMethodBareDefaultParameter(t *testing.T) {
	cases := []string{
		"const g = (a = 1) => a;\n",
		"class C { m(a = 1) {} }\n",
	}
	for _, src := range cases {
		tree, lang := parseLanguageSample(t, "typescript", src)
		root := tree.RootNode()
		if root.Type(lang) != "program" || root.HasError() {
			tree.Release()
			t.Fatalf("typescript %q root = %s hasError=%v; tree=%s", src, root.Type(lang), root.HasError(), root.SExpr(lang))
		}
		if sexpr := root.SExpr(lang); !strings.Contains(sexpr, "required_parameter") {
			tree.Release()
			t.Fatalf("typescript %q did not preserve required_parameter: %s", src, sexpr)
		}
		tree.Release()
	}
}

func TestParseTypeScriptNestedDestructuringArrayPattern(t *testing.T) {
	src := "const { value: [dirPath, { dirName, options, fileNames }] } = result;\n"
	tree, lang := parseLanguageSample(t, "typescript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("nested TypeScript destructuring root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), root.SExpr(lang))
	}
	sexpr := root.SExpr(lang)
	for _, want := range []string{"array_pattern", "object_pattern", "shorthand_property_identifier_pattern"} {
		if !strings.Contains(sexpr, want) {
			t.Fatalf("nested TypeScript destructuring missing %s: %s", want, sexpr)
		}
	}
	if strings.Contains(sexpr, "non_null_expression") {
		t.Fatalf("nested TypeScript destructuring retained non_null_expression: %s", sexpr)
	}
}

func TestParseTypeScriptDestructuringRefreshPreservesMissingError(t *testing.T) {
	// The second statement is a genuine missing-token recovery (switch case
	// with no expression before ':') confirmed against the C tree-sitter
	// oracle to synthesize a MISSING identifier node. An earlier revision of
	// this fixture used "const broken = ;" expecting a missing node there,
	// but the C oracle recovers that shape via a plain ERROR wrapper with no
	// MISSING node at all, so it could never exercise the HasError-refresh
	// invariant this test guards (that normalizing the unrelated
	// destructuring pattern in the first statement must not clear the
	// HasError bit on a real missing node elsewhere in the tree).
	src := "const { value: [dirPath, { dirName, options, fileNames }] } = result;\nswitch (x) { case: }\n"
	lang := grammars.TypescriptLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("typescript parse failed: %v", err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root.Type(lang) != "program" || !root.HasError() {
		t.Fatalf("destructuring with missing token root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), root.SExpr(lang))
	}
	sexpr := root.SExpr(lang)
	if !strings.Contains(sexpr, "array_pattern") || !strings.Contains(sexpr, "object_pattern") {
		t.Fatalf("destructuring normalization did not run: %s", sexpr)
	}
	foundMissing := false
	gotreesitter.Walk(root, func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if node.IsMissing() {
			foundMissing = true
			if !node.HasError() {
				t.Fatalf("missing node %s did not preserve HasError; tree=%s", node.Type(lang), root.SExpr(lang))
			}
		}
		return gotreesitter.WalkContinue
	})
	if !foundMissing {
		t.Fatalf("missing-token regression did not produce a missing node: %s", root.SExpr(lang))
	}
}

func TestParseTypeScriptDestructuredArrowReturnTypeCallArgument(t *testing.T) {
	src := "const remainingPaths = arrayFrom(allFileNames.entries(), ([fileName, { isRedirect, isInNodeModules }]): ModulePath => ({ path: fileName, isRedirect, isInNodeModules }));\n"
	tree, lang := parseLanguageSample(t, "typescript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("destructured TypeScript arrow call root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), root.SExpr(lang))
	}
	if sexpr := root.SExpr(lang); !strings.Contains(sexpr, "arrow_function") || !strings.Contains(sexpr, "array_pattern") || !strings.Contains(sexpr, "type_annotation") {
		t.Fatalf("destructured TypeScript arrow call did not preserve arrow/type shape: %s", sexpr)
	}
}

// TestParseTypeScriptArrowReturnTypeAnnotation guards issue #402: an arrow
// function with a typed parameter AND an explicit return-type annotation
// ("(a: A): B => ...") collapsed to a top-level ERROR when it was the value
// of a const/let declaration. The locked C tree-sitter-typescript oracle's
// _call_signature production (formal_parameters return_type?) competes, at
// the identical post-')' state, with reducing the parenthesized parameter
// list as a plain parenthesized_expression once a top-level ':' follows the
// ')'; the pattern-reduction derivation was being discarded before the
// return-type/arrow tokens confirmed it, under the steady-state cap-one
// merge budget. Fixed by the structure-aware cap-two steady state
// (typeScriptSteadyStateMergeCap in parser_retry.go), which subsumed the
// source-text detector originally added for this shape. TSX already parsed
// this correctly because its wider JSX conflict set keeps a second survivor
// alive through the fork.
func TestParseTypeScriptArrowReturnTypeAnnotation(t *testing.T) {
	src := "const f = (a: A): B => { return a; };\n"
	tree, lang := parseLanguageSample(t, "typescript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("typescript arrow return-type annotation root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), sexpr)
	}
	for _, want := range []string{"lexical_declaration", "arrow_function", "formal_parameters", "required_parameter", "type_annotation", "statement_block"} {
		if !strings.Contains(sexpr, want) {
			t.Fatalf("typescript arrow return-type annotation missing %s: %s", want, sexpr)
		}
	}
	if strings.Contains(sexpr, "ERROR") {
		t.Fatalf("typescript arrow return-type annotation retained an ERROR node: %s", sexpr)
	}
}

// TestParseTSXArrowReturnTypeAnnotation is the TSX-side companion to
// TestParseTypeScriptArrowReturnTypeAnnotation: TSX already parsed this
// shape correctly before the fix, so this pins the s-expression shape to
// prevent a regression while confirming TypeScript now matches it.
func TestParseTSXArrowReturnTypeAnnotation(t *testing.T) {
	src := "const f = (a: A): B => { return a; };\n"
	tree, lang := parseLanguageSample(t, "tsx", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("tsx arrow return-type annotation root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), sexpr)
	}
	want := "(program (lexical_declaration (variable_declarator (identifier) (arrow_function (formal_parameters (required_parameter (identifier) (type_annotation (type_identifier)))) (type_annotation (type_identifier)) (statement_block (return_statement (identifier)))))))"
	if sexpr != want {
		t.Fatalf("tsx arrow return-type annotation sexpr = %s, want %s", sexpr, want)
	}
}

// TestParseTypeScriptArrowNoReturnTypeAnnotationControl is the negative
// control for issue #402: a typed-parameter arrow with no return-type
// annotation was never affected (the existing typed-arrow-parameters
// widening already covers it) and must keep parsing cleanly.
func TestParseTypeScriptArrowNoReturnTypeAnnotationControl(t *testing.T) {
	src := "const f = (a: A) => { return a; };\n"
	tree, lang := parseLanguageSample(t, "typescript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("typescript arrow no-return-type control root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), sexpr)
	}
	if strings.Contains(sexpr, "type_annotation") && strings.Count(sexpr, "type_annotation") != 1 {
		t.Fatalf("typescript arrow no-return-type control unexpected type_annotation count: %s", sexpr)
	}
}

// TestParseTypeScriptArrowReturnTypeAnnotationLongerFixture reproduces the
// reporter's bisect fixture shape for issue #402: several preceding
// top-level declarations (import, interface, type alias, enum, two classes)
// followed by a trailing "export const make = (id: ID): User => {...}". The
// reporter noted this shape happened to parse on v0.21.0 in some
// larger-file contexts before the v0.22.0 conflict-policy change made it
// fail consistently, so this fixture guards the fix at realistic file
// scale, not just the minimal one-liner.
func TestParseTypeScriptArrowReturnTypeAnnotationLongerFixture(t *testing.T) {
	src := `import { z } from "zod";

interface Config {
	name: string;
}

type ID = string;

enum Status {
	Active,
	Inactive,
}

class Base {
	id: ID;
}

class UserStore extends Base {
	items: Record<ID, unknown> = {};
}

export const make = (id: ID): User => {
	return { id, name: "" };
};
`
	tree, lang := parseLanguageSample(t, "typescript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("typescript arrow return-type annotation longer fixture root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), sexpr)
	}
	for _, want := range []string{"export_statement", "arrow_function", "formal_parameters", "required_parameter", "statement_block"} {
		if !strings.Contains(sexpr, want) {
			t.Fatalf("typescript arrow return-type annotation longer fixture missing %s: %s", want, sexpr)
		}
	}
}

// TestParseTypeScriptTypeAnnotationArrowValueControl is a negative-control
// pin: "const x: (a: A) => B = foo" is a variable's own type annotation (a
// function_type in type position), not an arrow_function value expression.
// This shape was never affected by issue #402 (no
// required_parameter/parenthesized_expression fork here); it exercises the
// exact false-positive risk the retired source-text detector's balanced-paren
// scan had to avoid, and the structure-aware cap-two steady state
// (typeScriptSteadyStateMergeCap in parser_retry.go) that subsumed it must
// still parse it correctly.
func TestParseTypeScriptTypeAnnotationArrowValueControl(t *testing.T) {
	src := "const x: (a: A) => B = foo;\n"
	tree, lang := parseLanguageSample(t, "typescript", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	if root.Type(lang) != "program" || root.HasError() {
		t.Fatalf("typescript type-annotation arrow-value control root = %s hasError=%v; tree=%s", root.Type(lang), root.HasError(), sexpr)
	}
	if strings.Contains(sexpr, "arrow_function") {
		t.Fatalf("typescript type-annotation arrow-value control unexpectedly parsed a value arrow_function: %s", sexpr)
	}
	if !strings.Contains(sexpr, "function_type") {
		t.Fatalf("typescript type-annotation arrow-value control missing function_type: %s", sexpr)
	}
}

func TestParseJSXMultipleAttributesAfterExpression(t *testing.T) {
	src := "const el = <Foo bar=\"string\" baz={2} data-i8n=\"dialogs.welcome.heading\" bam />\n"
	for _, langName := range []string{"javascript", "tsx"} {
		t.Run(langName, func(t *testing.T) {
			tree, lang := parseLanguageSample(t, langName, src)
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			if got, want := root.ChildCount(), 1; got != want {
				t.Fatalf("%s root child count = %d, want %d", langName, got, want)
			}
			stmt := root.Child(0)
			if stmt == nil {
				t.Fatalf("%s root child is nil", langName)
			}
			if got, want := stmt.Type(lang), "lexical_declaration"; got != want {
				t.Fatalf("%s root child type = %q, want %q", langName, got, want)
			}
			attrPos := strings.Index(src, "data-i8n")
			if attrPos < 0 {
				t.Fatal("data-i8n attribute not found in sample")
			}
			node := root.NamedDescendantForByteRange(uint32(attrPos), uint32(attrPos+len("data-i8n")))
			if node == nil {
				t.Fatalf("%s data-i8n descendant is nil", langName)
			}
			if got, want := node.Type(lang), "property_identifier"; got != want {
				t.Fatalf("%s data-i8n type = %q, want %q", langName, got, want)
			}
		})
	}
}

func TestParseJSXStatementBoundaryAfterClosingElement(t *testing.T) {
	src := "var a = <Foo></Foo>\n" +
		"b = <Foo.Bar></Foo.Bar>\n"
	for _, langName := range []string{"javascript", "tsx"} {
		t.Run(langName, func(t *testing.T) {
			tree, lang := parseLanguageSample(t, langName, src)
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			if got, want := root.NamedChildCount(), 2; got != want {
				t.Fatalf("%s root named child count = %d, want %d", langName, got, want)
			}
			if stmt := root.NamedChild(0); stmt == nil || stmt.Type(lang) != "variable_declaration" {
				if stmt == nil {
					t.Fatalf("%s first statement is nil", langName)
				}
				t.Fatalf("%s first statement type = %q, want %q", langName, stmt.Type(lang), "variable_declaration")
			}
			if stmt := root.NamedChild(1); stmt == nil || stmt.Type(lang) != "expression_statement" {
				if stmt == nil {
					t.Fatalf("%s second statement is nil", langName)
				}
				t.Fatalf("%s second statement type = %q, want %q", langName, stmt.Type(lang), "expression_statement")
			}
		})
	}
}

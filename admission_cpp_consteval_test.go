package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// C++ has no production certificate. With a scratch certificate the core route
// keeps the whole `if consteval` input, as pinned C does (issue #1633).
// cgo_harness/ccore_cpp_consteval_c_test.go checks the same inputs against C.
func TestCcoreCppConstevalKeepsLaterDefinitions(t *testing.T) {
	defer ccoreCertifyAll()()
	lang := ccoreRouteLang(t, "cpp")
	const ce = "constexpr int ce(int x) {\n    if consteval {\n        return 1;\n    } else {\n        return x;\n    }\n}\n"
	for _, c := range []struct {
		name, src, want string
	}{
		{"if_consteval", "void f() { if consteval { } }",
			"(translation_unit (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement (ERROR) (consteval_block_declaration (compound_statement)))))"},
		{"if_not_consteval", "void f() { if !consteval { } }",
			"(translation_unit (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement (ERROR) (consteval_block_declaration (compound_statement)))))"},
		{"bracket_then_bar", "[c]|",
			"(translation_unit (ERROR (structured_binding_declarator (identifier))))"},
		{"definitions_after_consteval", "void before() {}\n" + ce + "void after1() { f(); }\nvoid after2() {}\n",
			"(translation_unit (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement)) (function_definition (type_qualifier) (primitive_type) (function_declarator (identifier) (parameter_list (parameter_declaration (primitive_type) (identifier)))) (compound_statement (ERROR (consteval_block_declaration (compound_statement (return_statement (number_literal)))) (compound_literal_expression (type_identifier) (initializer_list (identifier) (ERROR (identifier))))))) (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement (expression_statement (call_expression (identifier) (argument_list))))) (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement)))"},
	} {
		t.Run(c.name, func(t *testing.T) {
			gts.CcoreTestReset()
			p := ccoreRouteParser(lang)
			tree, err := p.Parse([]byte(c.src))
			if err != nil || tree == nil {
				t.Fatalf("Parse: %v", err)
			}
			defer tree.Release()
			if !gts.CcoreTestFromCore(tree) {
				decl, _ := gts.CcoreTestCounters()
				t.Fatalf("the core did not serve the parse; declines %v", decl)
			}
			if got := tree.ParseStopReason(); got != gts.ParseStopAccepted {
				t.Fatalf("stop reason %v, want accepted", got)
			}
			if end := tree.RootNode().EndByte(); int(end) != len(c.src) {
				t.Fatalf("root ends at %d of %d", end, len(c.src))
			}
			if !tree.RootNode().HasError() {
				t.Fatal("HasError is false; the grammar has no rule for this input and C reports an error")
			}
			if got := ccoreSExpr(tree, lang); got != c.want {
				t.Fatalf("tree\n got %s\nwant %s", got, c.want)
			}
		})
	}
}

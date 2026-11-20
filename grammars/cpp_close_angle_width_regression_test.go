package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// When the shared-token election hands every live stack a single `>` but one
// stack's own lex mode reads `>>`, C keeps that stack alive by lexing each
// version separately. The expected trees are the C runtime's, without field
// names.
func TestCppShiftAfterSubscriptSurvivesCloseAngleElection(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{
			src:  "int f(){ v = (d[c] >> 8) | ((d[c] & 0x80) << 1); }",
			want: "(translation_unit (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement (expression_statement (assignment_expression (identifier) (binary_expression (parenthesized_expression (binary_expression (subscript_expression (identifier) (subscript_argument_list (identifier))) (number_literal))) (parenthesized_expression (binary_expression (parenthesized_expression (binary_expression (subscript_expression (identifier) (subscript_argument_list (identifier))) (number_literal))) (number_literal)))))))))",
		},
		{
			src:  "x = i[] >> 8;",
			want: "(translation_unit (expression_statement (assignment_expression (identifier) (binary_expression (subscript_expression (identifier) (subscript_argument_list)) (number_literal)))))",
		},
		{src: "", want: ""},
	}
	lang := CppLanguage()
	for _, tc := range cases {
		parser := ts.NewParser(lang)
		parser.SetTimeoutMicros(2_000_000)
		tree, err := parser.Parse([]byte(tc.src))
		if err != nil {
			t.Fatalf("%q: parse failed: %v", tc.src, err)
		}
		root := tree.RootNode()
		if got := tree.ParseStopReason(); got != ts.ParseStopAccepted {
			t.Errorf("%q: stop reason %q, want accepted; runtime=%s", tc.src, got, tree.ParseRuntime().Summary())
		}
		if end := root.EndByte(); end != uint32(len(tc.src)) {
			t.Errorf("%q: root ends at %d, want %d", tc.src, end, len(tc.src))
		}
		if tc.want == "" {
			continue
		}
		if root.HasError() {
			t.Errorf("%q: unexpected error: %s", tc.src, root.SExpr(lang))
		}
		if got := root.SExpr(lang); got != tc.want {
			t.Errorf("%q:\n got: %s\nwant: %s", tc.src, got, tc.want)
		}
	}
}

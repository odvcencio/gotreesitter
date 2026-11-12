package parse_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// jstsCompatCleanParseGoldenSExprJS / jstsCompatCleanParseGoldenSExprTS pin
// the exact normalized shape of testJSTSCompatCleanParseSourceJS/TS (if/while
// statement-keyword normalization, empty_statement semicolon drop, and
// call/unary/binary precedence rewrites — everything the fused walk
// touches), captured before the 2026-07-13 jstswalk-containment-gap fix and
// re-verified identical after.
const jstsCompatCleanParseGoldenSExprJS = "(program (if_statement (parenthesized_expression (identifier)) (statement_block (expression_statement (call_expression (identifier) (arguments)))) (else_clause (statement_block (expression_statement (call_expression (identifier) (arguments)))))) (while_statement (parenthesized_expression (identifier)) (statement_block (expression_statement (call_expression (identifier) (arguments))))) (lexical_declaration (variable_declarator (identifier) (binary_expression (unary_expression (identifier)) (binary_expression (unary_expression (identifier)) (identifier))))) (empty_statement) (expression_statement (call_expression (identifier) (arguments (call_expression (identifier) (arguments (identifier)))))))"
const jstsCompatCleanParseGoldenSExprTS = "(program (if_statement (parenthesized_expression (identifier)) (statement_block (expression_statement (call_expression (identifier) (arguments)))) (else_clause (statement_block (expression_statement (call_expression (identifier) (arguments)))))) (while_statement (parenthesized_expression (identifier)) (statement_block (expression_statement (call_expression (identifier) (arguments))))) (lexical_declaration (variable_declarator (identifier) (type_annotation (predefined_type)) (binary_expression (unary_expression (identifier)) (binary_expression (unary_expression (identifier)) (identifier))))) (empty_statement) (expression_statement (call_expression (identifier) (type_arguments (predefined_type)) (arguments (call_expression (identifier) (arguments (identifier)))))))"

const testJSTSCompatCleanParseSourceJS = "if (a) { b(); } else { c(); }\nwhile (x) { y(); }\nlet z = !a + -b * c;\n;\nfoo(bar(baz));\n"
const testJSTSCompatCleanParseSourceTS = "if (a) { b(); } else { c(); }\nwhile (x) { y(); }\nlet z: number = !a + -b * c;\n;\nfoo<string>(bar(baz));\n"

// TestJSTSCompatCleanParseUnaffectedByMemoryBudgetFix is an explicit
// byte-identity check for the 2026-07-13 jstswalk-containment-gap fix
// (parseStopPoller.memoryBudgetParser / javaScriptTypeScriptCompatMemoryBudgetStopReason
// / the shared compatMemoryBudgetTripped sticky latch): a clean,
// comfortably-within-any-reasonable-budget JS/TS parse must produce the
// exact same normalized tree shape as before the fix — the new code paths
// only ever change behavior when a memory budget is actually configured AND
// actually trips, which never happens here.
func TestJSTSCompatCleanParseUnaffectedByMemoryBudgetFix(t *testing.T) {
	cases := []struct {
		lang   string
		src    string
		golden string
	}{
		{lang: "javascript", src: testJSTSCompatCleanParseSourceJS, golden: jstsCompatCleanParseGoldenSExprJS},
		{lang: "typescript", src: testJSTSCompatCleanParseSourceTS, golden: jstsCompatCleanParseGoldenSExprTS},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			for _, label := range []string{"default", "budget=4096MB"} {
				t.Run(label, func(t *testing.T) {
					if label == "budget=4096MB" {
						t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "4096")
						gotreesitter.ResetParseEnvConfigCacheForTests()
						defer gotreesitter.ResetParseEnvConfigCacheForTests()
					}
					tree, lang := parseLanguageSample(t, tc.lang, tc.src)
					t.Cleanup(tree.Release)

					if got, want := tree.ParseStopReason(), gotreesitter.ParseStopAccepted; got != want {
						t.Fatalf("ParseStopReason() = %q, want %q", got, want)
					}
					if tree.ParseStoppedEarly() {
						t.Fatal("ParseStoppedEarly() = true, want false for a clean small parse")
					}
					root := tree.RootNode()
					if root == nil {
						t.Fatal("RootNode() = nil")
					}
					if root.HasError() {
						t.Fatalf("clean %s source reported an error: %s", tc.lang, root.SExpr(lang))
					}
					if got := root.SExpr(lang); got != tc.golden {
						t.Fatalf("SExpr drift:\n got:  %s\n want: %s", got, tc.golden)
					}
				})
			}
		})
	}
}

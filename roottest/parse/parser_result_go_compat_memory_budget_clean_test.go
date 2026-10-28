package parse_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// goCompatCleanParseGoldenSExpr pins the exact normalized shape of
// testGoCompatCleanParseSource (const-block semicolon drop + switch/case
// sibling-boundary normalization), captured before the memory-budget fix and
// re-verified identical after.
const goCompatCleanParseGoldenSExpr = "(source_file (package_clause (package_identifier)) (const_declaration (const_spec (identifier) (expression_list (int_literal))) (const_spec (identifier) (expression_list (int_literal)))) (function_declaration (identifier) (parameter_list) (type_identifier) (block (statement_list (expression_switch_statement (expression_case (expression_list (true)) (statement_list (return_statement (expression_list (int_literal))))) (default_case (statement_list (return_statement (expression_list (int_literal))))))))))"

const testGoCompatCleanParseSource = "package p\n\nconst (\n\tA = 1\n\tB = 2\n)\n\nfunc F() int {\n\tswitch {\n\tcase true:\n\t\treturn 1\n\tdefault:\n\t\treturn 0\n\t}\n}\n"

// TestGoCompatCleanParseUnaffectedByMemoryBudgetFix is an explicit
// byte-identity check for the 2026-07-12 gocompat-walk-containment-gap fix
// (parseStopPoller.memoryBudgetParser / (*Parser).goCompatMemoryBudgetStopReason
// / compatMemoryBudgetTripped): a clean, comfortably-within-any-reasonable-budget
// Go parse must produce the exact same normalized tree shape as before the fix
// — the new code paths only ever change behavior when a memory budget is
// actually configured AND actually trips, which never happens here. This
// complements TestGoZerrorsNormalizerByteIdentity (parser_result_go_compat_byte_identity_test.go),
// which pins a large real-corpus golden; this one is a small, self-contained
// pin exercising the const-block semicolon-drop and switch-statement
// sibling-boundary normalizers directly.
func TestGoCompatCleanParseUnaffectedByMemoryBudgetFix(t *testing.T) {
	src := []byte(testGoCompatCleanParseSource)
	lang := grammars.GoLanguage()

	// Two parses of the identical clean source: the default configuration,
	// and one with an explicit, generously large memory budget (env-gated,
	// like GOT_PARSE_MEMORY_BUDGET_MB in TestParserMemoryBudgetStopsParse).
	// Both must produce byte-identical normalized trees — the new
	// memory-budget polling in the Go compat walk (parseStopPoller.memoryBudgetParser)
	// and stage-boundary checks (goCompatMemoryBudgetStopReason) are additive
	// and must never fire, let alone alter output, on a parse this small.
	for _, label := range []string{"default", "budget=4096MB"} {
		t.Run(label, func(t *testing.T) {
			if label == "budget=4096MB" {
				t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "4096")
				gotreesitter.ResetParseEnvConfigCacheForTests()
				defer gotreesitter.ResetParseEnvConfigCacheForTests()
			}
			tree, err := gotreesitter.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			defer tree.Release()

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
				t.Fatalf("clean Go source reported an error: %s", root.SExpr(lang))
			}
			if got := root.EndByte(); got != uint32(len(src)) {
				t.Fatalf("root.EndByte() = %d, want %d (full source coverage)", got, len(src))
			}
			if got := root.SExpr(lang); got != goCompatCleanParseGoldenSExpr {
				t.Fatalf("SExpr drift:\n got:  %s\n want: %s", got, goCompatCleanParseGoldenSExpr)
			}
		})
	}
}

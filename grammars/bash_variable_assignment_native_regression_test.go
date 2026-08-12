//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// bashVariableAssignmentWrapperCases pins the native (no result-compatibility)
// shape for two or more consecutive Bash assignments with no intervening
// command. tree-sitter-bash's variable_assignments rule
// (seq($.variable_assignment, repeat1($.variable_assignment))) is a normal
// visible rule, not a hidden one, so the C parser always keeps it as a real
// wrapper node wherever the grammar's hidden _terminated_statement /
// _statement rules admit it — at the program root, inside a subshell, and
// inside an if_statement's condition slot alike. tree-sitter-bash
// test/corpus/literals.txt (commit a06c2e44, "Concatenation in strings")
// pins the same wrapper for two top-level assignments.
//
// Materialization now leaves this wrapper in place instead of splicing its
// children into the enclosing node, so raw and production digests agree for
// every case here and no follow-up field repair is needed.
var bashVariableAssignmentWrapperCases = []struct {
	name   string
	source string
}{
	{name: "program_level", source: "a=1 b=2 c=3"},
	{name: "subshell", source: "(a=1 b=2)"},
	{name: "if_condition", source: "if a=1 b=2; then\n  echo hi\nfi\n"},
	// tree-sitter-bash test/corpus/literals.txt, commit a06c2e44.
	{name: "corpus_expansion_values", source: "component_type=\"${1}\" item_name=\"${2?}\"\n"},
}

func TestBashVariableAssignmentWrapperNeedsNoResultCompatibility(t *testing.T) {
	for _, test := range bashVariableAssignmentWrapperCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			language := BashLanguage()

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)

			rawDigest := bashVariableAssignmentTreeDigest(t, raw, language)
			productionDigest := bashVariableAssignmentTreeDigest(t, production, language)
			if rawDigest != productionDigest {
				t.Fatalf("production digest = %s, want raw digest %s", productionDigest, rawDigest)
			}
			assertBashKeepsVariableAssignmentsWrapper(t, production.RootNode(), language)
		})
	}
}

func TestBashVariableAssignmentWrapperRoutes(t *testing.T) {
	for _, test := range bashVariableAssignmentWrapperCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			gotreesitter.SetGLRForestEnabled(false)
			t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(true) })

			receipts := retiredDispatchRouteReceiptsAllowCompactFallback(
				t,
				BashLanguage(),
				[]byte(test.source),
			)
			for _, receipt := range receipts {
				assertBashKeepsVariableAssignmentsWrapper(t, receipt.tree.RootNode(), BashLanguage())
				if receipt.name != "incremental" {
					continue
				}
				if receipt.incrementalProfile.ReuseUnsupported {
					if receipt.incrementalProfile.ReuseUnsupportedReason == "" {
						t.Fatalf("incremental reuse status = %+v", receipt.incrementalProfile)
					}
					return
				}
				if !receipt.incrementalProfile.OldTreeReuseRoute ||
					receipt.incrementalProfile.ReusedSubtrees == 0 ||
					receipt.incrementalProfile.ReusedBytes == 0 {
					t.Fatalf("incremental route did not reuse the old tree: %+v", receipt.incrementalProfile)
				}
			}
		})
	}
}

// TestBashIfConditionKeepsNativeFieldWithoutAWrapper covers the
// if-condition-field-projection witness that has no embedded assignment at
// all (materialization_subpass_census_test.go's "bash_if_condition_field"):
// the "condition" field on a multi-child if_statement condition (a
// test_command plus its terminating ";") is already correct straight out of
// raw materialization, with nothing to project.
func TestBashIfConditionKeepsNativeFieldWithoutAWrapper(t *testing.T) {
	source := []byte("if [ $ret -eq 0 ]; then\n  echo hi\nfi\n")
	language := BashLanguage()

	tree, err := gotreesitter.NewParser(language).ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)

	ifStatement := findFirstNamedDescendantWhere(tree.RootNode(), language, "if_statement", func(*gotreesitter.Node) bool { return true })
	if ifStatement == nil {
		t.Fatalf("missing if_statement: %s", tree.RootNode().SExpr(language))
	}
	conditionFields := 0
	for i := 0; i < ifStatement.ChildCount(); i++ {
		child := ifStatement.Child(i)
		if child == nil || child.Type(language) == "then" {
			break
		}
		if i == 0 {
			continue // the "if" keyword itself
		}
		if got := ifStatement.FieldNameForChild(i, language); got != "condition" {
			t.Fatalf("if_statement child %d (%s) field = %q, want condition", i, child.Type(language), got)
		}
		conditionFields++
	}
	if conditionFields < 2 {
		t.Fatalf("condition field count = %d, want at least 2 (test_command and its terminator)", conditionFields)
	}
}

// assertBashKeepsVariableAssignmentsWrapper asserts that root contains at
// least one native variable_assignments wrapper, that its own field (when
// it sits directly under an if_statement's condition slot) is "condition",
// and that every child underneath it is a variable_assignment.
func assertBashKeepsVariableAssignmentsWrapper(t *testing.T, root *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	wrapper := findFirstNamedDescendantWhere(root, lang, "variable_assignments", func(*gotreesitter.Node) bool { return true })
	if wrapper == nil {
		t.Fatalf("expected a native variable_assignments wrapper: %s", root.SExpr(lang))
	}
	if wrapper.ChildCount() < 2 {
		t.Fatalf("variable_assignments wrapper has %d children, want at least 2", wrapper.ChildCount())
	}
	for i := 0; i < wrapper.ChildCount(); i++ {
		if got := wrapper.Child(i).Type(lang); got != "variable_assignment" {
			t.Fatalf("variable_assignments child %d = %q, want variable_assignment", i, got)
		}
	}
	parent := wrapper.Parent()
	if parent != nil && parent.Type(lang) == "if_statement" {
		if got := parent.FieldNameForChild(bashChildIndex(parent, wrapper), lang); got != "condition" {
			t.Fatalf("variable_assignments field under if_statement = %q, want condition", got)
		}
	}
}

func bashChildIndex(parent, child *gotreesitter.Node) int {
	for i := 0; i < parent.ChildCount(); i++ {
		if parent.Child(i) == child {
			return i
		}
	}
	return -1
}

func bashVariableAssignmentTreeDigest(
	t *testing.T,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

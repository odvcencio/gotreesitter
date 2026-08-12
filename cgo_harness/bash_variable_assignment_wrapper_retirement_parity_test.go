//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// bashVariableAssignmentWrapperParityCases mirrors
// grammars.bashVariableAssignmentWrapperCases: consecutive top-level Bash
// assignments with no intervening command keep their native
// variable_assignments wrapper (tree-sitter-bash's variable_assignments rule
// is a normal visible rule, not a hidden one), matching the C oracle
// wherever the wrapper can appear.
var bashVariableAssignmentWrapperParityCases = []struct {
	name   string
	source string
}{
	{name: "program_level", source: "a=1 b=2 c=3"},
	{name: "subshell", source: "(a=1 b=2)"},
	{name: "if_condition", source: "if a=1 b=2; then\n  echo hi\nfi\n"},
	// tree-sitter-bash test/corpus/literals.txt, commit a06c2e44.
	{name: "corpus_expansion_values", source: "component_type=\"${1}\" item_name=\"${2?}\"\n"},
}

func TestBashVariableAssignmentWrapperRetirementCOracleParity(t *testing.T) {
	entry, ok := parityEntriesByName["bash"]
	if !ok {
		t.Fatal("missing Bash grammar entry")
	}
	language := entry.Language()
	cLanguage, err := ParityCLanguage("bash")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range bashVariableAssignmentWrapperParityCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)

			goParser := gotreesitter.NewParser(language)
			goParser.SetAdmissionCandidateRoute(false)
			goTree, err := goParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			defer cTree.Close()

			var divergences []string
			compareNodes(
				goTree.RootNode(),
				language,
				cTree.RootNode(),
				"root",
				&divergences,
			)
			if len(divergences) != 0 {
				t.Fatalf("C-oracle divergences:\n%s", strings.Join(divergences, "\n"))
			}
		})
	}
}

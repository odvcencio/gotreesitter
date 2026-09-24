//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPythonEscapeSequenceIssue1275COracleParity(t *testing.T) {
	goLang := grammars.PythonLanguage()
	cLang, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		source string
	}{
		{"triple_line_end_one_slash", "s = \"\"\"" + `\` + "\n\"\"\""},
		{"triple_line_end_two_slashes", "s = \"\"\"" + `\\` + "\n\"\"\""},
		{"triple_line_end_three_slashes", "s = \"\"\"" + `\\\` + "\n\"\"\""},
		{"triple_line_end_four_slashes", "s = \"\"\"" + `\\\\` + "\n\"\"\""},
		{"triple_line_end_crlf", "s = \"\"\"" + `\\` + "\r\n\"\"\""},
		{"single_line_end_one_slash", "s = \"a" + `\` + "\nb\""},
		{"single_line_end_three_slashes", "s = \"a" + `\\\` + "\nb\""},
		{"raw_triple_line_end", "s = r\"\"\"" + `\\` + "\n\"\"\""},
		{"bytes_triple_line_end", "s = b\"\"\"" + `\\` + "\n\"\"\""},
		{"f_triple_line_end", "s = f\"\"\"" + `\\` + "\n\"\"\""},
		{"raw_single_line_end", "s = r\"a" + `\` + "\nb\""},
		{"bytes_single_line_end", "s = b\"a" + `\` + "\nb\""},
		{"f_single_line_end", "s = f\"a" + `\` + "\nb\""},
		{"docstring_line_end", "def f():\n    \"\"\"text " + `\\\\` + "\n    more\"\"\"\n"},
	}
	for pairs := 1; pairs <= 4; pairs++ {
		cases = append(cases, struct {
			name   string
			source string
		}{fmt.Sprintf("triple_%d_escape_pairs", pairs), "s = \"\"\"" + strings.Repeat(`\`, pairs*2) + "\"\"\""})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse([]byte(tc.source), nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned no tree")
			}
			defer cTree.Close()

			for _, candidate := range []bool{false, true} {
				name := "production"
				if candidate {
					name = "candidate"
				}
				t.Run(name, func(t *testing.T) {
					parser := gotreesitter.NewParser(goLang)
					parser.SetAdmissionCandidateRoute(candidate)
					tree, err := parser.Parse([]byte(tc.source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertPythonIssue1275ExactCNode(t, tree.RootNode(), goLang, cTree.RootNode(), "root")
				})
			}
		})
	}
}

func assertPythonIssue1275ExactCNode(t *testing.T, goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string) {
	t.Helper()
	gs, cs := snapshotGo(goNode, goLang), snapshotC(cNode)
	if gs != cs {
		t.Fatalf("%s differs from the C oracle: go=%+v c=%+v", path, gs, cs)
	}
	for i := 0; i < gs.ChildCount; i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		if goField, cField := goNode.FieldNameForChild(i, goLang), cNode.FieldNameForChild(uint32(i)); goField != cField {
			t.Fatalf("%s field differs from the C oracle: go=%q c=%q", childPath, goField, cField)
		}
		assertPythonIssue1275ExactCNode(t, goNode.Child(i), goLang, cNode.Child(uint(i)), childPath)
	}
}

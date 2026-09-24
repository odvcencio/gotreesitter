package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPythonEscapeSequenceIssue1275SiblingSpans(t *testing.T) {
	lang := grammars.PythonLanguage()
	cases := []struct {
		name   string
		source string
	}{
		{"triple_line_end_one_slash", "s = \"\"\"" + `\` + "\n\"\"\""},
		{"triple_line_end_two_slashes", "s = \"\"\"" + `\\` + "\n\"\"\""},
		{"triple_line_end_three_slashes", "s = \"\"\"" + `\\\` + "\n\"\"\""},
		{"triple_line_end_four_slashes", "s = \"\"\"" + `\\\\` + "\n\"\"\""},
		{"single_line_end", "s = \"a" + `\` + "\nb\""},
		{"raw_triple_line_end", "s = r\"\"\"" + `\\` + "\n\"\"\""},
		{"bytes_triple_line_end", "s = b\"\"\"" + `\\` + "\n\"\"\""},
		{"f_triple_line_end", "s = f\"\"\"" + `\\` + "\n\"\"\""},
	}
	for pairs := 1; pairs <= 4; pairs++ {
		cases = append(cases, struct {
			name   string
			source string
		}{fmt.Sprintf("triple_%d_escape_pairs", pairs), "s = \"\"\"" + strings.Repeat(`\`, pairs*2) + "\"\"\""})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(candidate)
				tree, err := parser.Parse([]byte(tc.source))
				if err != nil {
					t.Fatal(err)
				}
				assertPythonIssue1275NoSiblingOverlap(t, tree.RootNode(), lang)
				if tc.name == "triple_line_end_four_slashes" {
					var escapes [][2]uint32
					var visit func(*gotreesitter.Node)
					visit = func(node *gotreesitter.Node) {
						if node.Type(lang) == "escape_sequence" {
							escapes = append(escapes, [2]uint32{node.StartByte(), node.EndByte()})
						}
						for i := 0; i < node.ChildCount(); i++ {
							visit(node.Child(i))
						}
					}
					visit(tree.RootNode())
					if len(escapes) != 2 || escapes[0] != [2]uint32{7, 9} || escapes[1] != [2]uint32{9, 11} {
						t.Fatalf("candidate=%t escapes=%v, want [7..9 9..11]", candidate, escapes)
					}
				}
				tree.Release()
			}
		})
	}
}

func assertPythonIssue1275NoSiblingOverlap(t *testing.T, node *gotreesitter.Node, lang *gotreesitter.Language) {
	t.Helper()
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if i > 0 {
			previous := node.Child(i - 1)
			if previous.EndByte() > child.StartByte() {
				t.Fatalf("%s siblings overlap: %s %d..%d and %s %d..%d", node.Type(lang),
					previous.Type(lang), previous.StartByte(), previous.EndByte(),
					child.Type(lang), child.StartByte(), child.EndByte())
			}
		}
		assertPythonIssue1275NoSiblingOverlap(t, child, lang)
	}
}

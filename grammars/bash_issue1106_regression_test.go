package grammars_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestIssue1106BashEmptyAssignments(t *testing.T) {
	data, err := os.ReadFile("testdata/bash_empty_assignments.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string
		Source      string
		Assignments map[string]string
		Command     string
		Invalid     bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	lang := grammars.BashLanguage()
	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			source := []byte(test.Source)
			tree, err := gotreesitter.NewParser(lang).ParseStrict(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root == nil {
				t.Fatal("parse returned no root")
			}
			if root.HasErrorOrMissing() != test.Invalid {
				t.Fatalf("invalid = %v, want %v: %s", root.HasErrorOrMissing(), test.Invalid, root.SExpr(lang))
			}
			if test.Invalid {
				return
			}
			if root.EndByte() != uint32(len(source)) {
				t.Fatalf("root ends at %d, want %d", root.EndByte(), len(source))
			}
			assignments := make(map[string]bool)
			foundCommand := test.Command == ""
			gotreesitter.Walk(root, func(node *gotreesitter.Node, _ int) gotreesitter.WalkAction {
				switch node.Type(lang) {
				case "variable_assignment":
					name := node.ChildByFieldName("name", lang)
					if name == nil {
						t.Fatal("assignment has no name")
					}
					key := name.Text(source)
					want, ok := test.Assignments[key]
					if !ok || assignments[key] {
						t.Fatalf("unexpected or duplicate assignment %q", key)
					}
					assignments[key] = true
					value := node.ChildByFieldName("value", lang)
					if want == "" {
						if value != nil {
							t.Fatalf("%s= has a value node: %s", key, value.SExpr(lang))
						}
					} else if value == nil || value.Text(source) != want {
						t.Fatalf("%s value does not match %q: %s", key, want, node.SExpr(lang))
					}
					start := strings.Index(test.Source, key+"=")
					if node.StartByte() != uint32(start) || node.EndByte() != uint32(start+len(key)+1+len(want)) {
						t.Fatalf("%s assignment span = [%d,%d)", key, node.StartByte(), node.EndByte())
					}
				case "command":
					name := node.ChildByFieldName("name", lang)
					if name != nil && name.Text(source) == test.Command {
						foundCommand = true
					}
				}
				return gotreesitter.WalkContinue
			})
			if len(assignments) != len(test.Assignments) || !foundCommand {
				t.Fatalf("assignments = %v, command found = %v: %s", assignments, foundCommand, root.SExpr(lang))
			}
		})
	}
}

package grammars_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// +spec=`Bare builtin names in delimited references remain variables; arguments select function calls.`
// +case:id=make_function_variables,desc=`Builtin names as variables and calls`,expect=`Correct node kinds without errors`
// +link=component://grammars/patches/README.md
func TestMakeFunctionVariables(t *testing.T) {
	data, err := os.ReadFile("testdata/make_function_variables.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Source                          string
		Variables, Functions, Shells, Escapes int
		Error                                 bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	lang := grammars.MakeLanguage()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(tc.Source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root == nil {
				t.Fatal("missing root")
			}
			if root.HasError() != tc.Error {
				t.Fatalf("HasError = %v, want %v: %s", root.HasError(), tc.Error, root.SExpr(lang))
			}
			if root.EndByte() != uint32(len(tc.Source)) {
				t.Fatalf("root ends at %d, want %d", root.EndByte(), len(tc.Source))
			}
			if tc.Error {
				return
			}
			sexpr := root.SExpr(lang)
			for kind, want := range map[string]int{"variable_reference": tc.Variables, "function_call": tc.Functions, "shell_function": tc.Shells, "rule": 1, "escape": tc.Escapes} {
				if got := strings.Count(sexpr, "("+kind+" ") + strings.Count(sexpr, "("+kind+")"); got != want {
					t.Errorf("%s count = %d, want %d: %s", kind, got, want, sexpr)
				}
			}
		})
	}
}

// +spec=`A backslash before a comma stays inside a word in names, prerequisites and lists.`
// +case:id=make_escaped_comma,desc=`Escaped comma inside a word`,expect=`One word per name and no error, as the C parser gives`
func TestMakeEscapedCommaStaysInWord(t *testing.T) {
	lang := grammars.MakeLanguage()
	for _, tc := range []struct{ name, source, want string }{
		{"prerequisite", "a: b\\,c\n", "(makefile (rule (targets (word)) (prerequisites (word))))"},
		{"target", "a\\,b: c\n", "(makefile (rule (targets (word)) (prerequisites (word))))"},
		{"include", "include a\\,b\n", "(makefile (include_directive (list (word))))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("unexpected error: %s", root.SExpr(lang))
			}
			if root.EndByte() != uint32(len(tc.source)) {
				t.Fatalf("root ends at %d, want %d", root.EndByte(), len(tc.source))
			}
			if got := root.SExpr(lang); got != tc.want {
				t.Fatalf("tree = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestMakeFunctionVariableEditsMatchFresh(t *testing.T) {
	lang := grammars.MakeLanguage()
	parser := gotreesitter.NewParser(lang)
	before := []byte("X := $(file)\nfmt:\n\t@echo ok\n")
	tree, err := parser.Parse(before)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { tree.Release() }()
	for _, expression := range []string{"$(dir)", "$(dir src/a.go)", "${file}", "$(foreach file,a b,$(file))", "$(file", "$(file)"} {
		after := []byte("X := " + expression + "\nfmt:\n\t@echo ok\n")
		tree.Edit(spliceTestEdit(before, after))
		incremental, err := parser.ParseIncremental(after, tree)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := gotreesitter.NewParser(lang).Parse(after)
		if err != nil {
			incremental.Release()
			t.Fatal(err)
		}
		diff := spliceTreeDiff(incremental.RootNode(), fresh.RootNode(), lang, "")
		fresh.Release()
		tree.Release()
		tree, before = incremental, after
		if diff != "" {
			t.Fatalf("edit to %q: %s", expression, diff)
		}
	}
}

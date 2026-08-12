package query_test

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCMakeBlockDefHighlightCaptures(t *testing.T) {
	src := []byte("block()\nendblock()\n\nblock(VARIABLES PROPAGATE)\nendblock()\n")
	captures, _ := cmakeQueryCaptures(t, src, `
(block_def
  (block_command
    (block) @function.builtin
    (argument_list
      (argument
        (unquoted_argument) @constant
        (#any-of? @constant "SCOPE_FOR" "POLICIES" "VARIABLES" "PROPAGATE")
      )*
    )?
  )
  (endblock_command
    (endblock) @function.builtin))
`)
	want := []string{
		"function.builtin:block",
		"function.builtin:endblock",
		"function.builtin:block",
		"constant:VARIABLES",
		"constant:PROPAGATE",
		"function.builtin:endblock",
	}
	if !reflect.DeepEqual(captures, want) {
		t.Fatalf("captures = %#v, want %#v", captures, want)
	}
}

func TestCMakeSetCacheArgumentHighlightCaptures(t *testing.T) {
	src := []byte(`set(TREE_SITTER_ABI_VERSION 15 CACHE STRING "Tree-sitter ABI version")`)
	captures, sexpr := cmakeQueryCaptures(t, src, `
(normal_command
  (identifier) @_function
  (#match? @_function "^[sS][eE][tT]$")
  (argument_list
    .
    (argument)
    ((argument) @_cache @keyword.modifier
      .
      (argument) @_type @type
      (#any-of? @_cache "CACHE")
      (#any-of? @_type "BOOL" "FILEPATH" "PATH" "STRING" "INTERNAL"))))
`)
	want := []string{
		"_function:set",
		"_cache:CACHE",
		"keyword.modifier:CACHE",
		"_type:STRING",
		"type:STRING",
	}
	if !reflect.DeepEqual(captures, want) {
		t.Fatalf("captures = %#v, want %#v\nsexpr: %s", captures, want, sexpr)
	}
}

func cmakeQueryCaptures(t *testing.T, src []byte, queryText string) ([]string, string) {
	t.Helper()
	lang := grammars.CmakeLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	if tree.RootNode() == nil {
		t.Fatal("Parse returned nil root")
	}
	query, err := gotreesitter.NewQuery(queryText, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	matches := query.Execute(tree)
	var captures []string
	for _, match := range matches {
		for _, capture := range match.Captures {
			captures = append(captures, capture.Name+":"+capture.Text(src))
		}
	}
	return captures, tree.RootNode().SExpr(lang)
}

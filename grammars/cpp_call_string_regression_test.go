package grammars_test

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	runtime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

func TestCppQualifiedCallArguments(t *testing.T) {
	call := "(call_expression (identifier) (argument_list (call_expression (qualified_identifier (namespace_identifier) (identifier)) (argument_list)) "
	cases := []struct{ source, want string }{
		{`n(){for(;;){o(s::b(),"");}}`, "(translation_unit (function_definition (function_declarator (identifier) (parameter_list)) (compound_statement (for_statement (compound_statement (expression_statement " + call + "(string_literal)))))))))"},
		{`void n(){o(s::b(),"");}`, "(translation_unit (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement (expression_statement " + call + "(string_literal)))))))"},
		{`void n(){o(s::b(),"text");}`, "(translation_unit (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement (expression_statement " + call + "(string_literal (string_content))))))))"},
		{`void n(){o(s::b(), 1);}`, "(translation_unit (function_definition (primitive_type) (function_declarator (identifier) (parameter_list)) (compound_statement (expression_statement " + call + "(number_literal)))))))"},
	}
	for _, c := range cases {
		for _, tokenSource := range []bool{false, true} {
			name := c.source + "/DFA"
			if tokenSource {
				name = c.source + "/TokenSource"
			}
			t.Run(name, func(t *testing.T) {
				lang := grammars.CppLanguage()
				parser := ts.NewParser(lang)
				src := []byte(c.source)
				var tree *ts.Tree
				var err error
				if tokenSource {
					factory := runtime.TokenSourceFactory("cpp")
					if factory == nil {
						t.Fatal("missing C++ token source")
					}
					tree, err = parser.ParseWithTokenSource(src, factory(src, lang))
				} else {
					tree, err = parser.Parse(src)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				root := tree.RootNode()
				got := root.SExpr(lang)
				if root.HasError() || got != c.want {
					t.Fatalf("got %s; want %s", got, c.want)
				}
				allocations := testing.AllocsPerRun(100, func() {
					next, err := parser.ParseIncremental(src, tree)
					if err != nil {
						panic(err)
					}
					next.Release()
				})
				if allocations != 0 {
					t.Fatalf("no-edit reparse allocated %.2f times per run", allocations)
				}
				if root.EndByte() != uint32(len(src)) {
					t.Fatalf("root ends at %d", root.EndByte())
				}
			})
		}
	}
}

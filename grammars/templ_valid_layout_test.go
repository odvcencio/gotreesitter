package grammars_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTemplValidLayoutMatchesC(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "element text after component import",
			src:  "templ t(){@e(){>}}",
			want: "(source_file (component_declaration (component_identifier) (parameter_list) (component_block (component_import (component_identifier) (argument_list) (component_block (element_text))))))",
		},
		{
			name: "empty component import argument string",
			src:  "templ r(){@n(\"\")}\ntempl l(){}",
			want: "(source_file (component_declaration (component_identifier) (parameter_list) (component_block (component_import (component_identifier) (argument_list (interpreted_string_literal))))) (component_declaration (component_identifier) (parameter_list) (component_block)))",
		},
	}

	entry := grammars.DetectLanguageByName("templ")
	if entry == nil || entry.Language() == nil {
		t.Fatal("templ language is not registered")
	}
	lang := entry.Language()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.src, err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root == nil {
				t.Fatalf("Parse(%q) returned nil root", tc.src)
			}
			if got := root.SExpr(lang); got != tc.want {
				t.Errorf("SExpr mismatch:\n got  %s\n want %s", got, tc.want)
			}
			if root.HasError() {
				t.Errorf("Parse(%q) produced a root with errors: %s", tc.src, root.SExpr(lang))
			}
		})
	}
}

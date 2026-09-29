package grammars_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestFirrtlValidLayoutMatchesC(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "nested when at end of input",
			src:  "circuit s:\n module E:\n  when eq(n):\n   when lt(n):\n    x<=n",
			want: "(source_file (circuit (identifier) (module (identifier) (when (primitive_operation (primop) (identifier)) (suite (when (primitive_operation (primop) (identifier)) (suite (connection (identifier) (identifier)))))))))",
		},
	}

	entry := grammars.DetectLanguageByName("firrtl")
	if entry == nil || entry.Language() == nil {
		t.Fatal("firrtl language is not registered")
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

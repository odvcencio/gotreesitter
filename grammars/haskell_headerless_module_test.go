package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHaskellHeaderlessModuleKeepsAllTopLevelItems(t *testing.T) {
	entry := grammars.DetectLanguageByName("haskell")
	if entry == nil {
		t.Fatal("haskell grammar is not registered")
	}
	lang := entry.Language()
	for _, tc := range []struct {
		name, source, want string
	}{
		{
			name:   "two imports",
			source: "import T\nimport qualified D\n",
			want:   "(haskell (imports (import (module (module_id))) (import (module (module_id)))))",
		},
		{
			name:   "top splice",
			source: "t\n",
			want:   "(haskell (declarations (top_splice (variable))))",
		},
		{
			name:   "explicit module",
			source: "module M where\nimport T\nimport qualified D\n\nf = 1\n",
			want:   "(haskell (header (module (module_id))) (imports (import (module (module_id))) (import (module (module_id)))) (declarations (bind (variable) (match (literal (integer))))))",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if got := root.SExpr(lang); got != tc.want {
				t.Errorf("SExpr = %s, want %s", got, tc.want)
			}
			if root.HasError() {
				t.Errorf("unexpected parse error: %s", root.SExpr(lang))
			}
			if got, want := root.EndByte(), uint32(len(tc.source)); got != want {
				t.Errorf("root ends at byte %d, want %d", got, want)
			}
		})
	}
}

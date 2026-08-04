package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestIssue660AnonymousCommaHasNoField(t *testing.T) {
	lang := grammars.PythonLanguage()
	cases := []string{
		"from a import b, c\n",
		"import a, b\n",
		"x[a, b]\n",
	}

	for _, useCandidate := range []bool{false, true} {
		for _, source := range cases {
			name := "production"
			if useCandidate {
				name = "candidate"
			}
			t.Run(name+"/"+source, func(t *testing.T) {
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(useCandidate)
				tree, err := parser.Parse([]byte(source))
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				defer tree.Release()
				assertAnonymousCommasUnfielded(t, tree.RootNode(), lang, []byte(source))
			})
		}
	}
}

func assertAnonymousCommasUnfielded(t *testing.T, parent *gotreesitter.Node, lang *gotreesitter.Language, source []byte) {
	t.Helper()
	if parent == nil {
		return
	}
	for i := 0; i < parent.ChildCount(); i++ {
		child := parent.Child(i)
		if child == nil {
			continue
		}
		if child.Text(source) == "," && parent.FieldNameForChild(i, lang) != "" {
			t.Fatalf("anonymous comma at [%d,%d) has field %q in parent %s: %s",
				child.StartByte(), child.EndByte(), parent.FieldNameForChild(i, lang), parent.Type(lang), parent.SExpr(lang))
		}
		assertAnonymousCommasUnfielded(t, child, lang, source)
	}
}

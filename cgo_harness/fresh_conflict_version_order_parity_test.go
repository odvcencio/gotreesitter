//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestFreshConflictVersionOrderMatchesLockedC(t *testing.T) {
	// Minimized from the authenticated top-20 fresh corpus. C retains the last
	// action in the original version when an action cell has several choices.
	cases := []struct {
		grammar string
		sources []string
	}{
		{"typescript", []string{"5<u,t>('',(x)=>'')", "f<A,B>('',(x)=>'')"}},
		{"cpp", []string{"e<>(){N=sizeof(R);(t);}", "void f(){N=sizeof(int);}"}},
		{"c_sharp", []string{`namespace s{public class t{s(){s=[""];}}}`, `class C{void M(){s=a?[0];}}`, "var x = [ y, ];\n"}},
		{"dart", []string{"s({Function()?r}){e().e();}", "s({String?r}){e().e();}"}},
	}
	for _, tc := range cases {
		t.Run(tc.grammar, func(t *testing.T) {
			cLang, err := COracleLanguage(tc.grammar)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			for _, source := range tc.sources {
				cTree := cParser.Parse([]byte(source), nil)
				if cTree == nil || cTree.RootNode() == nil {
					t.Fatal("locked C parser returned no tree")
				}
				t.Cleanup(cTree.Close)
				for _, candidate := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/compact=%t", source, candidate), func(t *testing.T) {
						tree, lang, err := parseWithGo(parityCase{
							name: tc.grammar, source: source, candidateRoute: &candidate,
						}, []byte(source), nil)
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { releaseGoTree(tree) })
						assertLockedCTreeExact(t, "fresh conflict version order", tree, lang, cTree)
					})
				}
			}
		})
	}
}

// Recovery must retain every closed packed history until its C dispatch turn.
func TestFreshRecoveryHistoriesMatchLockedC(t *testing.T) {
	for _, tc := range []struct{ grammar, source string }{
		{"typescript", "const n = 1e+;\n"},
		{"dart", "x//\nimport system;\n'"},
	} {
		t.Run(tc.grammar, func(t *testing.T) {
			cLang, err := COracleLanguage(tc.grammar)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse([]byte(tc.source), nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			for _, candidate := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", candidate), func(t *testing.T) {
					tree, lang, err := parseWithGo(parityCase{name: tc.grammar, source: tc.source, candidateRoute: &candidate}, []byte(tc.source), nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { releaseGoTree(tree) })
					assertLockedCTreeExactWithErrors(t, "packed recovery histories", tree, lang, cTree)
				})
			}
		})
	}
}

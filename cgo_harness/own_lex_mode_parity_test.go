//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Several GLR stacks read the shared lookahead with their own lex modes, from
// the position where C lexes each version. These inputs pin that against the
// locked C parser on the harness route (cpp: the registered token source) and
// on the plain DFA route.
func TestOwnLexModeReadMatchesLockedC(t *testing.T) {
	for _, tc := range []struct{ lang, src string }{
		{"julia", "h = (UInt === UInt64) ? 0x94cb2bb20a28ce96 : 0x1f60a087\n"},
		{"cpp", "using X = A<B<T> >;\nvoid after() {}\n"},
		{"cpp", "A<B<T>> x;\nvoid after() {}\n"},
		{"cpp", "template <typename T>\nstruct S {\n    using X = typename A<B<T>>::type;\n    using Y =\n        A<B<T> >::type;\n    void m() {}\n};\nvoid after() {}\n"},
		{"cpp", "class A {\n  H<I<J>> method() {\n    typename K::template L<M<N>>::type variable2;\n    typename K::template L<M<N>>::template O<P>::type variable3;\n  }\n};\n"},
	} {
		cTree := parseLockedC(t, tc.lang, []byte(tc.src))
		for _, route := range []string{"harness", "dfa"} {
			t.Run(tc.lang+"/"+route+"/"+tc.src, func(t *testing.T) {
				tree, lang := parseOwnLexRoute(t, tc.lang, route, []byte(tc.src))
				defer releaseGoTree(tree)
				assertLockedCTreeExact(t, tc.lang, tree, lang, cTree)
			})
		}
	}
}

// The fixtures behind the invariant gate's counter ledger: the root keeps C's
// error flag, end byte and child count. Earthfile and F# differ from C deeper
// in the tree on the unchanged runtime too, so only their roots are compared.
func TestOwnLexModeFixturesKeepLockedCRoot(t *testing.T) {
	for _, name := range []string{"julia", "earthfile", "fsharp"} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile("../internal/benchfixtures/testdata/real/" + name)
			if err != nil {
				t.Fatal(err)
			}
			cRoot := parseLockedC(t, name, src).RootNode()
			tree, _ := parseOwnLexRoute(t, name, "harness", src)
			defer releaseGoTree(tree)
			goRoot := tree.RootNode()
			if goRoot.HasError() != cRoot.HasError() || goRoot.EndByte() != uint32(cRoot.EndByte()) || goRoot.ChildCount() != int(cRoot.ChildCount()) {
				t.Fatalf("root: Go HasError=%v end=%d children=%d, C HasError=%v end=%d children=%d; runtime=%s",
					goRoot.HasError(), goRoot.EndByte(), goRoot.ChildCount(), cRoot.HasError(), cRoot.EndByte(), cRoot.ChildCount(), tree.ParseRuntime().Summary())
			}
		})
	}
}

func parseLockedC(t *testing.T, name string, src []byte) *sitter.Tree {
	t.Helper()
	cLang, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("locked C parser returned no tree")
	}
	t.Cleanup(tree.Close)
	return tree
}

func parseOwnLexRoute(t *testing.T, name, route string, src []byte) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	if route == "harness" {
		tree, lang, err := parseWithGo(parityCase{name: name, source: string(src)}, src, nil)
		if err != nil {
			t.Fatal(err)
		}
		return tree, lang
	}
	lang := parityEntriesByName[name].Language()
	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return tree, lang
}

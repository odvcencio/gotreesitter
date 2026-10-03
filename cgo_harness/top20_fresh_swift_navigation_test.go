//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTop20FreshSwiftNavigation(t *testing.T) {
	cLang, err := COracleLanguage("swift")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}

	// Shrunk from the locked Utils.swift and Logger.swift fresh failures.
	// The identifier before '.' remains an expression, rather than a user_type.
	for _, source := range []string{
		"func g(h:S)throws->[U]{r.r((t))}",
		"{{{e=\"\\(s.d(\"\"))\"}}}",
	} {
		src := []byte(source)
		cTree := cParser.Parse(src, nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("locked C parser returned no tree")
		}
		t.Cleanup(cTree.Close)
		for _, candidate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/compact=%t", source, candidate), func(t *testing.T) {
				goTree, lang, err := parseWithGo(parityCase{
					name: "swift", source: source, candidateRoute: &candidate,
				}, src, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { releaseGoTree(goTree) })
				assertLockedCTreeExact(t, "Swift expression navigation", goTree, lang, cTree)
			})
		}
	}
}

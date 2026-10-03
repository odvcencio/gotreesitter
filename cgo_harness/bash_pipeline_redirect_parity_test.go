//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestBashPipelineRedirectConvergenceMatchesLockedC(t *testing.T) {
	cLang, err := COracleLanguage("bash")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}

	// The first source was minimized from the locked lib-midx.sh failure.
	// The redirect owns the entire pipeline before the && list boundary.
	for _, source := range []string{
		"t(){x|e|t>t&&m|x|t>i\n}",
		"x|e|t>t\n",
		"x|e|t>t&&m|x|t>i\n",
	} {
		cTree := cParser.Parse([]byte(source), nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("locked C parser returned no tree")
		}
		t.Cleanup(cTree.Close)
		for _, candidate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/compact=%t", source, candidate), func(t *testing.T) {
				tree, lang, err := parseWithGo(parityCase{
					name: "bash", source: source, candidateRoute: &candidate,
				}, []byte(source), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { releaseGoTree(tree) })
				assertLockedCTreeExact(t, "Bash pipeline redirect", tree, lang, cTree)
			})
		}
	}
}

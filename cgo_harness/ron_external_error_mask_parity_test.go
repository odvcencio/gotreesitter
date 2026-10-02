//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestRonExternalErrorModeRejectedResultMatchesLockedC(t *testing.T) {
	// Minimized from fresh-C edit-session passing steps lost when external
	// scanners began participating in ERROR-mode lexing. A rejected numeric
	// scan must not enable string content by masking its result.
	for _, tc := range []struct {
		grammar string
		sources []string
	}{
		{"ron", []string{`d 1"`, `1`, `"x"`}},
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
			for i, source := range tc.sources {
				cTree := cParser.Parse([]byte(source), nil)
				if cTree == nil || cTree.RootNode() == nil {
					t.Fatal("locked C parser returned no tree")
				}
				t.Cleanup(cTree.Close)
				for _, candidate := range []bool{false, true} {
					t.Run(fmt.Sprintf("case-%d/compact=%t", i, candidate), func(t *testing.T) {
						tree, lang, err := parseWithGo(parityCase{
							name: tc.grammar, source: source, candidateRoute: &candidate,
						}, []byte(source), nil)
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { releaseGoTree(tree) })
						assertLockedCTreeExactWithErrors(t, "rejected ERROR scanner result", tree, lang, cTree)
					})
				}
			}
		})
	}
}

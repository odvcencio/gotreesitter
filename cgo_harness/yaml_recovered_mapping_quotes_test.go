//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestYAMLRecoveredMappingQuotesLockedC(t *testing.T) {
	cl, err := COracleLanguage("yaml")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	lang := grammars.YamlLanguage()
	for _, source := range []string{
		"a: [1, 2\nb: 3 # '\n",
		"a: [1, 2\nb: 'ok'\n",
		"a: [1, 2\nb: 'it''s ok'\n",
		"a: [1, 2\nb: 'unfinished\n",
	} {
		t.Run(source, func(t *testing.T) {
			ct := cp.Parse([]byte(source), nil)
			defer ct.Close()
			for _, candidate := range []bool{false, true} {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(candidate)
				gt, err := p.Parse([]byte(source))
				if err != nil {
					t.Fatal(err)
				}
				defer gt.Release()
				assertLockedCTreeExactWithErrors(t, "recovered mapping", gt, lang, ct)
			}
		})
	}
}

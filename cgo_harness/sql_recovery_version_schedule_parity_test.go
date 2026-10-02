//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestSQLRecoveryMissingVersionScheduleMatchesLockedC(t *testing.T) {
	cLang, err := COracleLanguage("sql")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"SELECT''p ORDERBYa<->'2",
		"SELECT''O ORDERBYa<->'2",
		"SELECT 1 ORDER BY a < -2",
	} {
		cTree := cParser.Parse([]byte(source), nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("locked C parser returned no tree")
		}
		t.Cleanup(cTree.Close)
		for _, candidate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/compact=%t", source, candidate), func(t *testing.T) {
				tree, lang, err := parseWithGo(parityCase{name: "sql", source: source, candidateRoute: &candidate}, []byte(source), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { releaseGoTree(tree) })
				assertLockedCTreeExactWithErrors(t, "SQL recovery version schedule", tree, lang, cTree)
			})
		}
	}
}

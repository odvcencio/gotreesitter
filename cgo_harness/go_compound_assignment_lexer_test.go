//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoConditionalCompoundAssignmentLockedC(t *testing.T) {
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, operator := range []string{"|=", "&=", "+=", "<<=", ">>=", "&^="} {
		t.Run(operator, func(t *testing.T) {
			source := []byte("package p\nfunc f(){ if tr.Skip { v " + operator + " x } }\n")
			lang := grammars.GoLanguage()
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(true)
			routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			routed, fallback := gts.AdmissionCandidateCounters()
			oracle := cp.Parse(source, nil)
			if oracle == nil {
				t.Fatal("C returned no tree")
			}
			defer oracle.Close()
			assertLockedCTreeExact(t, operator, tree, lang, oracle)
			if routed != routedBefore+1 || fallback != fallbackBefore {
				t.Fatalf("compact admission failed: %q", gts.AdmissionCandidateLastFallbackReason())
			}
		})
	}
}

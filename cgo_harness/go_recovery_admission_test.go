//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoMissingOperandCompactAdmissionLockedC(t *testing.T) {
	source := []byte("package main\n\nfunc () {\n\tvar x = \n}\n")
	lang := grammars.GoLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
	tree, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	oracle := cp.Parse(source, nil)
	if oracle == nil {
		t.Fatal("C returned no tree")
	}
	defer oracle.Close()
	assertG18LockedCExact(t, "missing operand", tree, lang, oracle)
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != routedBefore+1 || fallback != fallbackBefore {
		t.Fatalf("compact recovery declined: %q", gts.AdmissionCandidateLastFallbackReason())
	}
}

//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoExplicitEOFRecoveryLockedC(t *testing.T) {
	source := []byte("package p\nfunc f(){x=y")
	lang := grammars.GoLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	before, failed := gts.AdmissionCandidateCounters()
	tree, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	oracle := cp.Parse(source, nil)
	if oracle == nil {
		t.Fatal("C tree is nil")
	}
	defer oracle.Close()
	assertG18LockedCExact(t, "physical EOF terminator", tree, lang, oracle)
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != before+1 || fallback != failed {
		t.Fatalf("physical EOF fell back: %s", gts.AdmissionCandidateLastFallbackReason())
	}
}

func TestGoRecoveryPublicationLockedC(t *testing.T) {
	for _, source := range []string{
		"package/p\nfunc a() { aa := 12; _ = aa + 1 }\nfunc b() { _ = 2 }\n",
		"package p\nfunc a() { aa := 12; _ = aa + 1'}\nfunc b() { _ = 2 }\n",
		"package p\nfunc a() { aa := 12; _ = aa + 1 }\nfunc b() { _ = 2'}\n",
	} {
		t.Run(source, func(t *testing.T) {
			lang := grammars.GoLanguage()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			cl, err := ParityCLanguage("go")
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			before, failed := gts.AdmissionCandidateCounters()
			tree, err := p.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			oracle := cp.Parse([]byte(source), nil)
			if oracle == nil {
				t.Fatal("C tree is nil")
			}
			defer oracle.Close()
			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != before+1 || fallback != failed {
				t.Fatalf("recovery publication fell back: %s", gts.AdmissionCandidateLastFallbackReason())
			}
			assertG18LockedCExact(t, "recovery publication", tree, lang, oracle)
		})
	}
}

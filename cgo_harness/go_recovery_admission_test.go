//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoMissingOperandCompactAdmissionLockedC(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"missing_operand", "package main\n\nfunc () {\n\tvar x = \n}\n"},
		{"missing_brace_census", "package p\nfunc f() {\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testGoRecoveryAdmissionLockedC(t, []byte(tc.source))
		})
	}
}

func testGoRecoveryAdmissionLockedC(t *testing.T, source []byte) {
	t.Helper()
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
	assertG18LockedCExact(t, "compact recovery", tree, lang, oracle)
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != routedBefore+1 || fallback != fallbackBefore {
		t.Fatalf("compact recovery declined: %q", gts.AdmissionCandidateLastFallbackReason())
	}
}

func TestGoIncompleteFunctionAdmissionLockedC(t *testing.T) {
	source := []byte("package p\nfunc")
	lang := grammars.GoLanguage()
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
	for _, compact := range []bool{false, true} {
		name := "production"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
			tree, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			assertG18LockedCExact(t, name, tree, lang, oracle)
			if !tree.RootNode().HasError() {
				t.Fatal("incomplete function lost its error flag")
			}
			routed, fallback := gts.AdmissionCandidateCounters()
			wantRouted := routedBefore
			if compact {
				wantRouted++
			}
			if routed != wantRouted || fallback != fallbackBefore {
				t.Fatalf("unexpected route: routed=%d fallback=%d, want %d/%d", routed, fallback, wantRouted, fallbackBefore)
			}
		})
	}
}

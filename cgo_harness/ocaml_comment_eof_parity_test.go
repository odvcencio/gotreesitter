//go:build cgo && treesitter_c_parity

package cgoharness

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"testing"
)

func TestOCamlCommentEOFLockedCParity(t *testing.T) {
	for _, source := range []string{
		"(**)", "(**)\n", "(**)\t", " (* outer (* inner *) *) \n",
		"(**) (**)\n", "(**)\n(**)\n", "\n(**)\n",
		"let x = 1\n(**)\n", "(* a *)\nlet x = 1\n(* b *)\n",
	} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				runParityCase(t, parityCase{name: "ocaml", candidateRoute: &candidate}, "comment-eof", []byte(source))
			}
		})
	}
}

func TestOCamlCommentEOFErrorVerdictMatchesLockedC(t *testing.T) {
	lang := grammars.DetectLanguageByName("ocaml").Language()
	cLang, err := COracleLanguage("ocaml")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	for _, control := range []struct {
		source    string
		wantError bool
	}{{"^(**)\n", true}, {"(**)^", true}, {"(* unfinished", false}} {
		source := control.source
		t.Run(source, func(t *testing.T) {
			ct := cp.Parse([]byte(source), nil)
			if ct == nil {
				t.Fatal("nil C tree")
			}
			defer ct.Close()
			if ct.RootNode().HasError() != control.wantError {
				t.Fatalf("C error=%t, want %t", ct.RootNode().HasError(), control.wantError)
			}
			for _, candidate := range []bool{false, true} {
				p := gotreesitter.NewParser(lang)
				p.SetAdmissionCandidateRoute(candidate)
				gt, err := p.Parse([]byte(source))
				if err != nil {
					t.Fatal(err)
				}
				defer gt.Release()
				if gt.RootNode().HasError() != ct.RootNode().HasError() {
					t.Fatalf("candidate=%t error=%t, want C error=%t", candidate, gt.RootNode().HasError(), ct.RootNode().HasError())
				}
				if !control.wantError {
					if diff := FirstDivergenceDumpV1(gt.RootNode(), lang, ct.RootNode()); diff != nil {
						t.Fatalf("candidate=%t unterminated-comment divergence=%+v", candidate, diff)
					}
				}
			}
		})
	}
}

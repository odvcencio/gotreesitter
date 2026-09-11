//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestHTMLRecoverySelectionLockedC(t *testing.T) {
	testHTMLRecoverySelectionLockedC(t, []byte("<html><bodyHello</body></html>\n"))
}

func TestHTMLRecoveryTrailingTextLockedC(t *testing.T) {
	testHTMLRecoverySelectionLockedC(t, []byte("<html><body>Hello /bod></html>\n"))
}

func TestHTMLRecoveryCommentTailLockedC(t *testing.T) {
	for _, source := range []string{"<!--c-->>", "<!-- c -->>", "<!--c--> >", "<!--c-->>>", "<!--c--><!--d-->>"} {
		t.Run(source, func(t *testing.T) {
			testHTMLRecoverySelectionLockedC(t, []byte(source))
		})
	}
}

func testHTMLRecoverySelectionLockedC(t *testing.T, source []byte) {
	t.Helper()
	lang := grammars.HtmlLanguage()
	cl, err := ParityCLanguage("html")
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
	t.Logf("C tree: %s", oracle.RootNode().ToSexp())
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
			t.Logf("fallback: %s", gts.AdmissionCandidateLastFallbackReason())
			t.Logf("Go tree: %s", tree.RootNode().SExpr(lang))
			assertG18LockedCExact(t, name, tree, lang, oracle)
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

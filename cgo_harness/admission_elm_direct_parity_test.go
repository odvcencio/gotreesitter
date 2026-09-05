//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestAdmissionCandidateElmHighlightCOracle(t *testing.T) {
	const sourceSHA256 = "8fca87bd8cc2735e83704acd8d06ffbc6cf04e386505de45596218d7fb72642c"
	const treeSHA256 = "f9e33776ce39fa7ebed6e3e8a845a3b5d4dd87e03e09b6f6fa8fc1a02db852c8"
	source, err := os.ReadFile("../testdata/admission_direct/elm_highlight_basic.elm")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != sourceSHA256 {
		t.Fatalf("Elm fixture SHA-256=%s, want %s", got, sourceSHA256)
	}
	cLanguage, err := ParityCLanguage("elm")
	if err != nil {
		t.Fatal(err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	if cDigest != treeSHA256 {
		t.Fatalf("locked C digest=%s, want %s", cDigest, treeSHA256)
	}
	language := grammars.ElmLanguage()
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	for _, route := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact_%t", route), func(t *testing.T) {
			parser := gotreesitter.NewParser(language)
			parser.SetAdmissionCandidateRoute(route)
			beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
			tree, err := parser.Parse(source)
			if err != nil || tree == nil {
				t.Fatalf("parse failed: %v", err)
			}
			defer tree.Release()
			afterRouted, afterFallback := gotreesitter.AdmissionCandidateCounters()
			wantRouted := uint64(0)
			if route {
				wantRouted = 1
			}
			if afterRouted-beforeRouted != wantRouted || afterFallback != beforeFallback {
				t.Fatalf("route counters=%d/%d, want %d/0", afterRouted-beforeRouted, afterFallback-beforeFallback, wantRouted)
			}
			runtime := tree.ParseRuntime()
			if runtime.Truncated || runtime.TokenSourceEOFEarly || tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
				t.Fatalf("incomplete Elm parse: %s", runtime.Summary())
			}
			inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			if inspection.SHA256 != cDigest {
				t.Fatalf("Go digest=%s, locked C=%s", inspection.SHA256, cDigest)
			}
		})
	}
}

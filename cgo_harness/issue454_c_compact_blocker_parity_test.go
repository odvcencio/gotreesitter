//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestIssue454COneKiBLockedCParity compares the malformed deletion with locked C.
// Check both route settings, including the error flags that previously differed.
// A compact decline must report its fallback instead of claiming native admission.
func TestIssue454COneKiBLockedCParity(t *testing.T) {
	source := append([]byte(nil), benchfixtures.Issue454CSource()[:1024]...)
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C edit marker is absent")
	}
	edited := append(append([]byte(nil), source[:site]...), source[site+1:]...)

	cLang, err := ParityCLanguage("c")
	if err != nil {
		t.Fatalf("load locked C grammar: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set locked C grammar: %v", err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C parser returned no tree")
	}
	defer cTree.Close()

	if got, want := cTree.RootNode().HasError(), true; got != want {
		t.Fatalf("locked C root HasError() = %v, want %v", got, want)
	}

	for _, compact := range []bool{false, true} {
		name := "production"
		if compact {
			name = "compact_requested"
		}
		t.Run(name, func(t *testing.T) {
			goLang := grammars.CLanguage()
			parser := gts.NewParser(goLang)
			parser.SetAdmissionCandidateRoute(compact)
			beforeRoute, beforeFallback := gts.AdmissionCandidateCounters()
			goTree, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("parse edited C witness with Go: %v", err)
			}
			defer releaseGoTree(goTree)
			afterRoute, afterFallback := gts.AdmissionCandidateCounters()
			routed, fallback := afterRoute-beforeRoute, afterFallback-beforeFallback
			reason := gts.AdmissionCandidateLastFallbackReason()
			if compact {
				if !((routed == 1 && fallback == 0) || (routed == 0 && fallback == 1 && reason != "")) {
					t.Fatalf("compact route=%d/%d: %q", routed, fallback, reason)
				}
				t.Logf("compact route=%d/%d fallback=%q", routed, fallback, reason)
			} else if routed != 0 || fallback != 0 {
				t.Fatalf("production parse entered compact admission: %d/%d", routed, fallback)
			}
			if goTree.ParseStoppedEarly() || !goTree.RootNode().HasError() {
				t.Fatalf("malformed source lost its complete error tree: %s", goTree.ParseRuntime().Summary())
			}
			assertG18LockedCExact(t, name, goTree, goLang, cTree)
		})
	}
}

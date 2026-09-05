//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"strings"
	"testing"
)

func TestCompactRecoveryProductionAliasDoesNotRequireOwnedCertificate(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	source := []byte("package p\nfunc f(){}\nfunc g(){}+")
	tree, routed, reason := p.tryCompactFullParseRoute(source)
	if !routed || tree == nil {
		t.Fatalf("owned recovery declined: %s", reason)
	}
	want := tree.RootNode().SExpr(p.language)
	tree.Release()
	runner := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	for _, mode := range []string{"default_finalization", "uncertified_language"} {
		t.Run(mode, func(t *testing.T) {
			language := *p.language
			finalization := diagnosticParserCoreFinalizeOwnedRecovery
			if mode == "default_finalization" {
				finalization = diagnosticParserCoreFinalizeDefault
			} else {
				language.CompactOwnedEOFRecoveryCertified = false
			}
			var scratch parserCoreRunnerScratch
			tree, err := materializeDiagnosticParserCoreAcceptedSelectionWithRootFinalization(
				runner.compact, runner.scheduler.acceptedHead, runner.scheduler.acceptedPayloads,
				NewParser(&language), source, &scratch, false, true, finalization,
			)
			if mode == "default_finalization" {
				if tree != nil {
					tree.Release()
				}
				if err == nil || !strings.Contains(err.Error(), "accepted compact root carries an error-bearing trailing extra payload") {
					t.Fatalf("default finalization must retain its trailing-error guard: %v", err)
				}
				return
			}
			if err != nil || tree == nil {
				t.Fatalf("authenticated production alias materialization err=%v", err)
			}
			defer tree.Release()
			if got := tree.RootNode().SExpr(&language); got != want {
				t.Fatalf("alias projection=%s, want %s", got, want)
			}
		})
	}
}

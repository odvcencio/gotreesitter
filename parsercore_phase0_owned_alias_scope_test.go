//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"strings"
	"testing"
)

func TestCompactRecoveryProductionAliasScope(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	source := []byte("package p\nfunc f(){}\nfunc g(){}+")
	tree, routed, reason := p.tryCompactFullParseRoute(source)
	if !routed || tree == nil {
		t.Fatalf("owned recovery declined: %s", reason)
	}
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
			if tree != nil {
				tree.Release()
			}
			if err == nil || !strings.Contains(err.Error(), "leaves do not tile") {
				t.Fatalf("uncertified production alias materialization err=%v", err)
			}
		})
	}
}

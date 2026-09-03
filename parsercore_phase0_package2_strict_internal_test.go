//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	_ "embed"
	"testing"
)

// Keep this internal test in package gotreesitter without importing the
// grammars package, which would create an import cycle.
//
//go:embed grammars/grammar_blobs/scala.bin
var package2ScalaBlob []byte

// package2ScalaStrictRunner enables the complete package-two capability bundle
// on a private language copy. The production profile stays unchanged until
// the locked Scala witnesses pass this receipt test.
func package2ScalaStrictRunner(t *testing.T) *parserCoreFreshFullRunner {
	t.Helper()
	lang, err := LoadLanguage(package2ScalaBlob)
	if err != nil {
		t.Fatalf("decode Scala grammar: %v", err)
	}
	lang.Name = "scala"
	lang.CompactStrategy2ErrorRegionCertified = true
	lang.CompactMissingTokenInsertionCertified = true
	lang.CompactStackSummaryRecoveryCertified = false
	lang.CompactPrimaryAcceptanceDerivationCertified = true
	lang.CompactAcceptanceStructuralElectionCertified = true
	lang.CompactRecoveryTrailingLineageRetirementCertified = true
	lang.CompactRecoveryErrorModeKeywordCaptureCertified = true
	parser := NewParser(lang)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		t.Fatalf("new Scala package-two runner: %v", err)
	}
	t.Cleanup(func() {
		if runner.compact != nil {
			if err := runner.compact.ResetReleasingRetention(); err != nil {
				t.Errorf("reset Scala package-two runner: %v", err)
			}
		}
	})
	return runner
}

// TestPackage2ScalaStrictReceipt locks the compact S5 receipt for the
// smallest witness. It requires exact routing and C's five physical merges.
func TestPackage2ScalaStrictReceipt(t *testing.T) {
	runner := package2ScalaStrictRunner(t)
	const source = "(y; "
	_, _, runErr := runner.executeSchedulerOpenWithObserverAndErrorRuns(
		[]byte(source), runner.compact, true, diagnosticParserCoreSeedObserver{}, true,
	)
	generic := runner.scheduler.receipt
	if runErr != nil {
		if generic != nil {
			t.Fatalf("compact route declined: %v; stop=%+v work=%+v", runErr, generic.Stop, generic.Stop.Work)
		}
		t.Fatalf("compact route declined before receipt: %v", runErr)
	}
	if generic == nil {
		t.Fatal("compact route returned no scheduler receipt")
	}
	if generic.Stop.Boundary != "" {
		t.Fatalf("compact receipt stopped at %s: %s", generic.Stop.Boundary, generic.Stop.Detail)
	}
	acceptance := generic.Acceptance
	if acceptance == nil {
		t.Fatalf("compact receipt has no acceptance: %+v", *generic)
	}
	work := acceptance.Work
	if work.ActionLookups == 0 || work.Dispatches == 0 || work.Elections == 0 {
		t.Fatalf("S5 receipt lacks action/output/trial work: %+v", work)
	}
	if runner.scheduler.s5MissingInsertions != 1 {
		t.Fatalf("S5 insertion commits=%d, want one", runner.scheduler.s5MissingInsertions)
	}
	if work.StackSummaryRecoveryForks != 0 || work.RecoverEOFAccepts != 0 || work.RecoveryVersionCapDrops != 0 {
		t.Fatalf("S5 receipt used an unsupported recovery or ceiling path: %+v", work)
	}
	if work.RecoveryLineageSelections == 0 {
		t.Fatalf("S5 receipt did not record recovery-lineage selection: %+v", work)
	}
	coreWork := acceptance.CoreWork
	if got, want := coreWork.PhysicalHeadMergeAttempts, uint64(5); got != want {
		t.Fatalf("physical merge attempts=%d, want %d", got, want)
	}
	if got, want := coreWork.PhysicalHeadMergeSuccesses, uint64(5); got != want {
		t.Fatalf("physical merge successes=%d, want %d", got, want)
	}
	if got, want := coreWork.PhysicalHeadMergeInputLinks, uint64(5); got != want {
		t.Fatalf("physical merge input links=%d, want %d", got, want)
	}
	if got, want := coreWork.PredecessorLinkUnionAlternateAppended, uint64(5); got != want {
		t.Fatalf("alternate links=%d, want %d", got, want)
	}
	if coreWork.PredecessorLinkUnionRecursiveChanged != 0 {
		t.Fatalf("recursive link changes=%d, want zero", coreWork.PredecessorLinkUnionRecursiveChanged)
	}
	t.Logf("Scala package-two strict receipt: acceptance=%+v work=%+v core=%+v", acceptance.Header.Header, work, coreWork)
}

// TestPackage2ScalaStrictReceiptComposition locks the shorter merge topology
// in the owned-lexer composition witness.
func TestPackage2ScalaStrictReceiptComposition(t *testing.T) {
	runner := package2ScalaStrictRunner(t)
	const source = "((y)->; "
	_, _, runErr := runner.executeSchedulerOpenWithObserverAndErrorRuns(
		[]byte(source), runner.compact, true, diagnosticParserCoreSeedObserver{}, true,
	)
	generic := runner.scheduler.receipt
	if runErr != nil {
		if generic != nil {
			t.Fatalf("compact composition route declined: %v; stop=%+v work=%+v", runErr, generic.Stop, generic.Stop.Work)
		}
		t.Fatalf("compact composition route declined before receipt: %v", runErr)
	}
	if generic == nil || generic.Acceptance == nil {
		t.Fatalf("compact composition receipt has no acceptance: %+v", generic)
	}
	work := generic.Acceptance.Work
	if work.ActionLookups == 0 || work.Dispatches == 0 || work.Elections == 0 {
		t.Fatalf("composition S5 receipt lacks action/output/trial work: %+v", work)
	}
	if runner.scheduler.s5MissingInsertions != 1 || work.StackSummaryRecoveryForks != 0 || work.RecoverEOFAccepts != 0 || work.RecoveryVersionCapDrops != 0 {
		t.Fatalf("composition S5 receipt has wrong fork or ceiling telemetry: insertions=%d work=%+v", runner.scheduler.s5MissingInsertions, work)
	}
	if work.RecoveryLineageSelections == 0 {
		t.Fatalf("composition S5 receipt did not select a recovery lineage: %+v", work)
	}
	coreWork := generic.Acceptance.CoreWork
	for name, got := range map[string]uint64{
		"attempts":        coreWork.PhysicalHeadMergeAttempts,
		"successes":       coreWork.PhysicalHeadMergeSuccesses,
		"input_links":     coreWork.PhysicalHeadMergeInputLinks,
		"alternate_links": coreWork.PredecessorLinkUnionAlternateAppended,
	} {
		if got != 3 {
			t.Fatalf("composition %s=%d, want 3; acceptance=%+v core=%+v", name, got, generic.Acceptance.Header.Header, coreWork)
		}
	}
	if coreWork.PredecessorLinkUnionRecursiveChanged != 0 {
		t.Fatalf("composition recursive link changes=%d, want zero", coreWork.PredecessorLinkUnionRecursiveChanged)
	}
	t.Logf("Scala package-two composition strict receipt: work=%+v core=%+v", work, coreWork)
}

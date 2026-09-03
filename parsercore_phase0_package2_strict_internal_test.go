//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	_ "embed"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Keep this internal test in package gotreesitter without importing the
// grammars package, which would create an import cycle.
//
//go:embed grammars/grammar_blobs/scala.bin
var package2ScalaBlob []byte

// package2ScalaStrictRunner enables the complete package-two capability bundle
// on a private language copy. The exact-blob profile uses the same flags only
// after the locked Scala witnesses pass this receipt test.
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
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	t.Cleanup(func() {
		if runner.compact != nil {
			if err := runner.compact.ResetReleasingRetention(); err != nil {
				t.Errorf("reset Scala package-two runner: %v", err)
			}
		}
	})
	return runner
}

func requirePackage2ScalaTree(
	t *testing.T,
	runner *parserCoreFreshFullRunner,
	source string,
	wantSExpr string,
	wantDigest string,
	wantMissingByte uint32,
) {
	t.Helper()
	tree, err := runner.materializeSelection([]byte(source), runner.compact, &runner.scheduler)
	if err != nil {
		t.Fatalf("materialize compact selection: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("materialized compact selection has no root")
	}
	if got := root.SExpr(runner.lang); got != wantSExpr {
		t.Fatalf("S-expression=%q, want %q", got, wantSExpr)
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) ||
		root.IsError() || !root.HasError() {
		t.Fatalf("root=%d..%d error=%t has_error=%t, want 0..%d non-error with error content",
			root.StartByte(), root.EndByte(), root.IsError(), root.HasError(), len(source))
	}
	missingCount := 0
	walkResultTree(root, func(node *Node) {
		if !node.IsMissing() {
			return
		}
		missingCount++
		if node.StartByte() != wantMissingByte || node.EndByte() != wantMissingByte || !node.HasError() {
			t.Errorf("missing node=%d..%d has_error=%t, want %d..%d with error content",
				node.StartByte(), node.EndByte(), node.HasError(), wantMissingByte, wantMissingByte)
		}
	})
	if missingCount != 1 {
		t.Fatalf("materialized missing nodes=%d, want one", missingCount)
	}
	if got := requireDiagnosticParserCoreCanonicalTreeDigest(t, tree, runner.lang); got != wantDigest {
		t.Fatalf("canonical tree digest=%s, want %s", got, wantDigest)
	}
	cost, err := runner.compact.RecoveryStoredErrorCost(runner.scheduler.acceptedHead)
	if err != nil {
		t.Fatalf("read selected recovery cost: %v", err)
	}
	if cost != uint32(core.RecoveryCostPerMissingTree+core.RecoveryCostPerRecovery) {
		t.Fatalf("selected stored recovery cost=%d, want 610", cost)
	}
}

func requirePackage2ScalaOwnedEOFRequest(
	t *testing.T,
	runner *parserCoreFreshFullRunner,
	state StateID,
	beforeByte uint32,
	eofByte uint32,
	wantPublicCount int,
) {
	t.Helper()
	if runner == nil || runner.scheduler.receipt == nil {
		t.Fatal("Scala package-two owned lexer receipt is unavailable")
	}
	requests := runner.scheduler.versionLexerRequests
	if len(requests) != 1 {
		t.Fatalf("Scala package-two owned lexer requests=%d, want one", len(requests))
	}
	request := requests[0]
	if request.state != state || request.token.Symbol != 0 ||
		request.token.StartByte != eofByte || request.token.EndByte != eofByte ||
		request.token.Missing || request.token.NoLookahead || request.token.ExternalScannerToken ||
		request.token.lexerInternalDFALexed {
		t.Fatalf("Scala package-two owned lexer request=%+v, want state %d authenticated EOF at %d", request, state, eofByte)
	}
	if request.before == nil || request.after == nil ||
		request.before.dfa.lexerPos != int(beforeByte) || request.after.dfa.lexerPos != int(eofByte) {
		t.Fatalf("Scala package-two owned lexer cursors=%+v/%+v, want %d..%d", request.before, request.after, beforeByte, eofByte)
	}
	public := runner.scheduler.receipt.VersionLexerRequests
	if len(public) != wantPublicCount {
		t.Fatalf("Scala package-two public owned lexer receipts=%d, want %d: %+v", len(public), wantPublicCount, public)
	}
	eof := public[len(public)-1]
	if eof.State != state || eof.Token.Symbol != 0 ||
		eof.Token.StartByte != eofByte || eof.Token.EndByte != eofByte ||
		eof.InternalDFAToken {
		t.Fatalf("Scala package-two public owned lexer receipt=%+v", public)
	}
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
	if work.PotentialReductionActions != 9 || work.PotentialReductionOutputs != 9 ||
		work.ReductionPromotions != 4 || work.MissingTokenTrials != 1 ||
		work.MissingTokenCommits != 1 || work.RecoveryDiscontinuityMerges != 5 ||
		work.RecoveryLineageSelections != 1 || work.RecoveryCondensePasses != 4 {
		t.Fatalf("S5 receipt has wrong exact recovery work: %+v", work)
	}
	if runner.scheduler.s5MissingInsertions != 1 {
		t.Fatalf("S5 insertion commits=%d, want one", runner.scheduler.s5MissingInsertions)
	}
	if work.StackSummaryRecoveryForks != 0 || work.RecoverEOFAccepts != 0 ||
		work.RecoveryVersionCapDrops != 0 || work.RecoveryLineageRetirements != 0 ||
		work.RecoveryAmbiguityForks != 0 || work.RecoveryCeilingDeclines != 0 {
		t.Fatalf("S5 receipt used an unsupported recovery or ceiling path: %+v", work)
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
	requirePackage2ScalaOwnedEOFRequest(t, runner, 429, 3, 4, 1)
	requirePackage2ScalaTree(
		t, runner, source,
		"(compilation_unit (parenthesized_expression (identifier)))",
		"06d2d9f6b02599ec5be0808330bf1c62e49b8cb0b146d094ceaa28b9185c79b6",
		2,
	)
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
	if work.PotentialReductionActions != 7 || work.PotentialReductionOutputs != 7 ||
		work.ReductionPromotions != 4 || work.MissingTokenTrials != 1 ||
		work.MissingTokenCommits != 1 || work.RecoveryDiscontinuityMerges != 3 ||
		work.RecoveryLineageSelections != 1 || work.RecoveryCondensePasses != 4 {
		t.Fatalf("composition S5 receipt has wrong exact recovery work: %+v", work)
	}
	if runner.scheduler.s5MissingInsertions != 1 || work.StackSummaryRecoveryForks != 0 ||
		work.RecoverEOFAccepts != 0 || work.RecoveryVersionCapDrops != 0 ||
		work.RecoveryLineageRetirements != 0 || work.RecoveryAmbiguityForks != 0 ||
		work.RecoveryCeilingDeclines != 0 {
		t.Fatalf("composition S5 receipt has wrong fork or ceiling telemetry: insertions=%d work=%+v", runner.scheduler.s5MissingInsertions, work)
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
	requirePackage2ScalaOwnedEOFRequest(t, runner, 429, 7, 8, 3)
	public := runner.scheduler.receipt.VersionLexerRequests
	if public[0].ElectionIndex != 4 || public[0].HeaderCreationSeq != 0 ||
		public[0].State != 10712 || public[0].Token.Symbol != 79 ||
		public[0].Token.StartByte != 4 || public[0].Token.EndByte != 6 ||
		public[0].Token.Text != "->" || !public[0].InternalDFAToken ||
		public[1].ElectionIndex != 4 || public[1].HeaderCreationSeq != 1 ||
		public[1].State != 18766 || public[1].Token.Symbol != 23 ||
		public[1].Token.StartByte != 4 || public[1].Token.EndByte != 5 ||
		public[1].Token.Text != "-" || !public[1].InternalDFAToken {
		t.Fatalf("composition owned lexer fork receipts=%+v", public[:2])
	}
	requirePackage2ScalaTree(
		t, runner, source,
		"(compilation_unit (parenthesized_expression (postfix_expression (parenthesized_expression (identifier)) (operator_identifier))))",
		"c0de79617c3e1bdac71c1686e5d6a3d4b2fb375aca9f69902aee974c5ca9f852",
		6,
	)
	t.Logf("Scala package-two composition strict receipt: work=%+v core=%+v", work, coreWork)
}

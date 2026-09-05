//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactRecoveryVersionTurnFinishedCostIsStrict(t *testing.T) {
	// A paused head adds 500 recovery cost. The finished span supplies the tie.
	for _, tc := range []struct {
		finished, paused uint32
		want             bool
	}{{501, 1, false}, {500, 1, true}, {502, 1, false}} {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("finished=%d/paused=%d/reversed=%t", tc.finished, tc.paused, reversed), func(t *testing.T) {
				scheduler := newStage3RecoveryCondenseScheduler(t, []uint32{tc.finished, tc.paused})
				scheduler.options.materializationSource = []byte(strings.Repeat("x", 502))
				for i := range scheduler.headers {
					header := &scheduler.headers[i]
					region := header.recoveryRegion()
					// Price the real ERROR spans; do not simulate cost through the cache.
					err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
						var err error
						header.head, _, err = scheduler.compact.RecoverEOFAcceptWithOpenRegionAndCostOwned(owner, header.head, region.startByte, region.endByte, region.children,
							func(core.NodeID, core.SubtreeID) (uint32, error) { return 0, nil })
						return err
					})
					if err != nil {
						t.Fatal(err)
					}
					header.closeRecoveryRegion()
					header.markRecoveryCosted()
				}
				scheduler.headers[0].accepted = true
				scheduler.headers[1].paused = true
				if reversed {
					scheduler.headers[0], scheduler.headers[1] = scheduler.headers[1], scheduler.headers[0]
				}
				got, err := scheduler.finishedRecoveryTreeBeatsPausedFrontier()
				if err != nil || got != tc.want {
					t.Fatalf("finished cost gate=%t err=%v, want %t", got, err, tc.want)
				}
			})
		}
	}
}

func TestCompactRecoveryVersionTurnUsesCOrder(t *testing.T) {
	entries := []diagnosticParserCoreRecoveryCondenseEntry{
		{key: diagnosticParserCoreRecoveryCondenseKey{recoveryGroup: 1}, status: core.RecoveryErrorStatus{Cost: 100, IsInError: true}},
		{key: diagnosticParserCoreRecoveryCondenseKey{missingGroup: 1}, status: core.RecoveryErrorStatus{Cost: 100}},
	}
	legacy := diagnosticParserCoreRecoveryCondensePairwise(entries, []int{0, 1})
	owned := diagnosticParserCoreRecoveryCondensePairwiseMode(entries, []int{0, 1}, true)
	if !reflect.DeepEqual(legacy, []int{0, 1}) || !reflect.DeepEqual(owned, []int{1, 0}) {
		t.Fatalf("recovery order legacy=%v owned=%v, want legacy absorber-first and owned C preference", legacy, owned)
	}
}

func TestCompactRecoveryVersionTurnCancellationPreservesFork(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil || !handled || len(scheduler.headers) != 2 {
		t.Fatalf("create recovery fork: handled=%t err=%v headers=%d", handled, err, len(scheduler.headers))
	}
	scheduler.options.allowCompactRecoveryVersionTurns = true
	scheduler.recoveryTurns = diagnosticParserCoreRecoveryTurns{active: true, lastByte: 2}
	cancelled := uint32(1)
	parser := NewParser(scheduler.tokenSource.language)
	parser.SetCancellationFlag(&cancelled)
	scheduler.options.stopControlParser = parser
	beforeTurns := scheduler.recoveryTurns
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	beforeRequests := append([]diagnosticParserCoreVersionLexerRequest(nil), scheduler.versionLexerRequests...)
	beforeOwnership := scheduler.versionLexerOwnershipActive
	stop, err := scheduler.dispatchRecoveryVersionTurn()
	if err == nil || stop != nil {
		t.Fatalf("cancelled recovery turn: stop=%+v err=%v", stop, err)
	}
	if scheduler.recoveryTurns != beforeTurns || scheduler.versionLexerOwnershipActive != beforeOwnership ||
		!reflect.DeepEqual(scheduler.headers, beforeHeaders) || !reflect.DeepEqual(scheduler.versionLexerRequests, beforeRequests) {
		t.Fatal("cancelled recovery turn changed the fork or lexer ownership")
	}
}

func newCompactRecoveryVersionTurnEOFScheduler(t *testing.T, table core.TableView) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	scheduler := newRecoveryLineageForkSchedulerWithTable(t, table, true)
	scheduler.options.materializationSource = []byte("a+")
	scheduler.options.MaxDispatches = 16
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil || !handled || len(scheduler.headers) != 2 {
		t.Fatalf("create recovery fork: handled=%t err=%v", handled, err)
	}
	language := scheduler.tokenSource.language
	absorberCursor := newDiagnosticParserCoreOwnedLexerSnapshot(t, scheduler.compact, language, 2)
	siblingCursor := newDiagnosticParserCoreOwnedLexerSnapshot(t, scheduler.compact, language, 1)
	scheduler.versionLexerRequests = []diagnosticParserCoreVersionLexerRequest{
		newDiagnosticParserCoreOwnedLexerRequest(scheduler.electionIndex, 0, Token{StartByte: 2, EndByte: 2}, absorberCursor, absorberCursor),
		newDiagnosticParserCoreOwnedLexerRequest(scheduler.electionIndex, 3, Token{Symbol: 4, Text: "+", StartByte: 1, EndByte: 2}, siblingCursor, absorberCursor),
	}
	for i, cursor := range []*diagnosticParserCoreVersionLexerSnapshot{absorberCursor, siblingCursor} {
		header := &scheduler.headers[i]
		baseline, set := header.recoveryNodeBaseline()
		header.publishVersionState(header.recoveryRegion(), cursor, uint32(i+1), header.recoveryGroupIdentity(), header.recoveryMissingGroupIdentity(), baseline, set)
		header.shifted = false
	}
	scheduler.options.allowCompactRecoveryVersionTurns = true
	scheduler.versionLexerOwnershipActive = true
	scheduler.recoveryTurns = diagnosticParserCoreRecoveryTurns{active: true, lastByte: 2}
	return scheduler
}

func TestCompactRecoveryVersionTurnAbsorberEOFPreservesSibling(t *testing.T) {
	scheduler := newCompactRecoveryVersionTurnEOFScheduler(t, recoveryLineageForkTable{})
	sibling := scheduler.headers[1]
	siblingRequest := scheduler.versionLexerRequests[1]
	stop, err := scheduler.dispatchRecoveryVersionTurn()
	if err != nil || stop != nil {
		t.Fatalf("absorber EOF turn: stop=%+v err=%v", stop, err)
	}
	if !scheduler.headers[0].accepted || scheduler.work.RecoverEOFAccepts != 1 {
		t.Fatal("absorber did not process its owned EOF")
	}
	if scheduler.headers[1] != sibling || !reflect.DeepEqual(scheduler.versionLexerRequests[1], siblingRequest) || sibling.versionState.relexSnapshot.dfa.lexerPos != 1 {
		t.Fatal("absorber EOF advanced the sibling or changed its snapshot")
	}
}

type compactRecoveryVersionTurnCapTable struct{ recoveryLineageForkTable }

func (table compactRecoveryVersionTurnCapTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	if state == 1 && symbol == 0 {
		return core.NewActionRow([]core.Action{{Type: core.ActionAccept}}, false), nil
	}
	return table.recoveryLineageForkTable.Actions(state, symbol)
}

func TestCompactRecoveryVersionTurnCapDeclinesBeforeMutation(t *testing.T) {
	scheduler := newCompactRecoveryVersionTurnEOFScheduler(t, compactRecoveryVersionTurnCapTable{})
	for len(scheduler.headers) < 6 {
		header := scheduler.headers[1]
		header.creationSeq = uint64(len(scheduler.headers) + 10)
		scheduler.headers = append(scheduler.headers, header)
	}
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	beforeRequests := append([]diagnosticParserCoreVersionLexerRequest(nil), scheduler.versionLexerRequests...)
	beforeTurns, beforeWork, beforeSeq := scheduler.recoveryTurns, scheduler.compact.Work(), scheduler.nextSeq
	stop, err := scheduler.dispatchRecoveryVersionTurn()
	if err != nil || stop == nil || !strings.Contains(stop.detail, "version-cap") {
		t.Fatalf("six-version EOF recovery: stop=%+v err=%v", stop, err)
	}
	if scheduler.recoveryTurns != beforeTurns || scheduler.compact.Work() != beforeWork || scheduler.nextSeq != beforeSeq ||
		!reflect.DeepEqual(scheduler.headers, beforeHeaders) || !reflect.DeepEqual(scheduler.versionLexerRequests, beforeRequests) {
		t.Fatal("version-cap decline changed the fork before rejecting it")
	}
}

func newCompactRecoveryVersionTurnGoParser(t *testing.T) *Parser {
	t.Helper()
	p := newAdmissionCandidateGoParser(t)
	lang := p.language
	if !lang.CompactOwnedEOFRecoveryCertified {
		t.Fatal("Go artifact has no owned EOF recovery profile")
	}
	p.SetAdmissionCandidateRoute(true)
	return p
}

func TestCompactRecoveryGoVersionTurnProfiledFallback(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	source := []byte("func f(){}\nfunc g(){}\n")
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	edited, edit := compactExecutionEdit(source, 21, 22, "+")
	old.Edit(edit)
	runner := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	legacyBefore := runner.legacyParseRuns
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	old.Release()
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if runner.legacyParseRuns != legacyBefore {
		t.Fatal("full compact recovery invoked legacy parsing")
	}
	if profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 || !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "compact_incremental_full_recovery" {
		t.Fatalf("full compact recovery misreported reuse: %+v", profile)
	}
	runtime := next.ParseRuntime()
	if !runtime.CompactIncrementalFullRecoveryRoute || runtime.CompactIncrementalReuseRoute || runtime.CompactIncrementalReusedSubtrees != 0 || runtime.CompactIncrementalReusedBytes != 0 || runtime.NormalizationPassesRun != 0 {
		t.Fatalf("full compact recovery runtime=%+v", runtime)
	}
	if !next.compactMaterialized || next.RootNode().ChildCount() != 3 {
		t.Fatal("full recovery did not publish the compact tree")
	}
	for i := 0; i < next.RootNode().ChildCount(); i++ {
		if next.RootNode().Child(i).Parent() != next.RootNode() {
			t.Fatal("old tree release damaged new parent links")
		}
	}
}

func TestCompactRecoveryGoVersionTurnRejectsLexicalReentry(t *testing.T) {
	for _, suffix := range []string{"+@", "+@+"} {
		p := newCompactRecoveryVersionTurnGoParser(t)
		tree, routed, reason := p.tryCompactFullParseRoute([]byte("func f(){}\nfunc g(){}" + suffix))
		if tree != nil {
			tree.Release()
		}
		if routed {
			t.Fatalf("owned recovery admitted unsupported lexical reentry %q", suffix)
		}
		if reason == "" {
			t.Fatal("lexical reentry declined without a reason")
		}
	}
}

func TestCompactRecoveryGoVersionTurnPackageAlias(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	tree, routed, reason := p.tryCompactFullParseRoute([]byte("package p\nfunc f(){}\nfunc g(){}+"))
	if tree != nil {
		defer tree.Release()
	}
	if !routed {
		t.Fatalf("package alias recovery declined: %s", reason)
	}
	root := tree.RootNode()
	if !tree.compactMaterialized || !root.HasError() || root.ChildCount() != 4 {
		t.Fatal("compact recovery lost its package declaration")
	}
	identifier := root.Child(0).Child(1)
	if identifier.Type(p.language) != "package_identifier" || identifier.StartByte() != 8 || identifier.EndByte() != 9 || identifier.HasError() {
		t.Fatal("compact recovery lost the authenticated package identifier alias")
	}
}

func TestCompactRecoveryGoVersionTurnAdmission(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	lang := p.language
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runner.compact.ResetReleasingRetention(); err != nil {
			t.Error(err)
		}
	}()
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	if !runner.options.allowCompactRecoveryVersionTurns {
		t.Fatal("Go artifact did not enable owned recovery turns")
	}
	source := []byte("func f(){}\nfunc g(){}+")
	_, _, err = runner.executeSchedulerOpenWithObserverAndErrorRuns(source, runner.compact, true, diagnosticParserCoreSeedObserver{}, true)
	if err != nil {
		t.Fatalf("owned recovery declined: %v; stop=%+v", err, runner.scheduler.receipt.Stop)
	}
	requests := runner.scheduler.receipt.VersionLexerRequests
	if len(requests) < 2 {
		t.Fatal("recovery did not request both owned lookaheads")
	}
	if requests[0].State != 0 || requests[0].Token.Symbol != 0 || requests[0].Token.StartByte != 22 ||
		requests[1].Token.Text != "+" || requests[1].Token.StartByte != 21 || requests[1].Token.EndByte != 22 ||
		requests[0].HeaderCreationSeq == requests[1].HeaderCreationSeq {
		t.Fatalf("absorber EOF did not precede the sibling '+': %+v", requests)
	}
	tree, err := runner.materializeSelection(source, runner.compact, &runner.scheduler)
	if err != nil {
		t.Fatalf("owned recovery did not materialize: %v; acceptance=%+v", err, runner.scheduler.receipt.Acceptance)
	}
	defer tree.Release()
	root := tree.RootNode()
	if !tree.compactMaterialized || !root.HasError() || root.IsError() || root.ChildCount() != 3 {
		t.Fatal("owned recovery result is not the compact source_file with local ERROR")
	}
	for i := 0; i < 2; i++ {
		if root.Child(i).Type(lang) != "function_declaration" || root.Child(i).HasError() {
			t.Errorf("recovery contaminated declaration %d", i)
		}
	}
	tail := root.Child(2)
	if !tail.IsError() || !tail.IsExtra() || tail.StartByte() != 21 || tail.EndByte() != 22 || tail.ChildCount() != 1 || tail.Child(0).HasError() {
		t.Error("recovery did not preserve the local tail ERROR and clean '+' leaf")
	}
}

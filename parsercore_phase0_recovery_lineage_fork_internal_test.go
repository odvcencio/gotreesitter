//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type recoveryLineageForkTable struct{}

type recoveryLineageForkFaultTable struct {
	recoveryLineageForkTable
	stateTwoLookaheadCalls int
	returnError            error
	panicValue             any
}

type recoveryLineageForkLaterReduceTable struct{ recoveryLineageForkTable }

func (table recoveryLineageForkLaterReduceTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	if state == 2 && symbol == 4 {
		return core.NewActionRow([]core.Action{
			{Type: core.ActionShift, State: 4},
			{Type: core.ActionReduce, Symbol: 5, ChildCount: 1},
		}, false), nil
	}
	return table.recoveryLineageForkTable.Actions(state, symbol)
}

func (table *recoveryLineageForkFaultTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	if state == 2 && symbol == 4 {
		table.stateTwoLookaheadCalls++
		if table.stateTwoLookaheadCalls == 2 {
			if table.panicValue != nil {
				panic(table.panicValue)
			}
			if table.returnError != nil {
				return core.ActionRow{}, table.returnError
			}
		}
	}
	return table.recoveryLineageForkTable.Actions(state, symbol)
}

func (recoveryLineageForkTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	var actions []core.Action
	switch {
	case state == 1 && symbol == 2:
		actions = []core.Action{{Type: core.ActionShift, State: 2}}
	case state == 1 && symbol == 3:
		actions = []core.Action{{Type: core.ActionShift, Extra: true}}
	case state == 2 && symbol == 4:
		actions = []core.Action{{Type: core.ActionReduce, Symbol: 5, ChildCount: 1}}
	case state == 3 && symbol == 4:
		actions = []core.Action{{Type: core.ActionShift, State: 4}}
	}
	return core.NewActionRow(actions, false), nil
}

func (recoveryLineageForkTable) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	if state == 1 && symbol == 5 {
		return 3, nil
	}
	return 0, nil
}

func (recoveryLineageForkTable) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}

func (recoveryLineageForkTable) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

func newRecoveryLineageForkScheduler(t *testing.T, armed bool) *diagnosticParserCoreGenericScheduler {
	return newRecoveryLineageForkSchedulerWithTable(t, recoveryLineageForkTable{}, armed)
}

func newRecoveryLineageForkSchedulerWithTable(
	t *testing.T,
	table core.TableView,
	armed bool,
) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(core.StateID(1), 1)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	lang := &Language{TokenCount: 5, SymbolCount: 5}
	tokenSource := &dfaTokenSource{
		language: lang,
		lexer:    &Lexer{source: []byte("a?"), pos: 2, col: 2},
	}
	return &diagnosticParserCoreGenericScheduler{
		compact:                      compact,
		tokenSource:                  tokenSource,
		headers:                      []diagnosticParserCoreHeader{{head: seed, creationSeq: 3}},
		token:                        Token{Symbol: 4, StartByte: 1, EndByte: 2},
		nextSeq:                      10,
		versionLexerBefore:           dfaRelexSnapshot{lexerPos: 1, lexerCol: 1},
		versionLexerBeforeValid:      true,
		versionLexerBeforeElection:   0,
		versionLexerBeforeCheckpoint: 0,
		options: DiagnosticParserCorePrefixOptions{
			Recovery:                             true,
			allowCompactStrategy2ErrorRegion:     true,
			allowCompactMissingTokenInsertion:    true,
			allowCompactFaithfulS5Recovery:       true,
			allowCompactRecoveryLineageSelection: armed,
		},
	}
}

func requireS5ForkRollback(
	t *testing.T,
	scheduler *diagnosticParserCoreGenericScheduler,
	original diagnosticParserCoreHeader,
	beforeStats core.Stats,
	beforeWork core.Work,
	receipt *DiagnosticParserCoreGenericScheduler,
) {
	t.Helper()
	if len(scheduler.headers) != 1 || scheduler.headers[0] != original {
		t.Fatalf("S5 rollback changed the frontier: %+v", scheduler.headers)
	}
	afterStats, err := scheduler.compact.Stats(original.head)
	if err != nil || afterStats != beforeStats || scheduler.compact.Work() != beforeWork {
		t.Fatalf("S5 rollback changed the core: stats=%+v/%+v work=%+v/%+v err=%v",
			afterStats, beforeStats, scheduler.compact.Work(), beforeWork, err)
	}
	if scheduler.receipt != receipt || scheduler.receipt.Tokens != 7 {
		t.Fatalf("S5 rollback changed the receipt pointer or value: pointer=%p/%p receipt=%+v",
			scheduler.receipt, receipt, scheduler.receipt)
	}
	if scheduler.verifierHeaderPtr != nil || scheduler.verifierBound != 0 {
		t.Fatalf("S5 rollback retained a stale verifier binding: pointer=%p bound=%d",
			scheduler.verifierHeaderPtr, scheduler.verifierBound)
	}
	if scheduler.nextSeq != 10 || scheduler.s5MissingInsertions != 0 || scheduler.recoveryIsolation {
		t.Fatalf("S5 rollback changed scheduler state: next=%d insertions=%d isolated=%t",
			scheduler.nextSeq, scheduler.s5MissingInsertions, scheduler.recoveryIsolation)
	}
}

func TestS5MissingInsertionForkPublishesBothRecoveryLineages(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if !handled {
		t.Fatal("the viable missing-token candidate did not fork")
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("headers=%d, want two recovery lineages", len(scheduler.headers))
	}
	if !scheduler.headers[0].isRecoveryLineage() || !scheduler.headers[1].isRecoveryLineage() {
		t.Fatal("the fork did not mark both recovery lineages")
	}
	if !scheduler.headers[0].isRecoveryCosted() || !scheduler.headers[1].isRecoveryCosted() {
		t.Fatal("the fork did not preserve recovery cost provenance")
	}
	if !scheduler.recoveryIsolation {
		t.Fatal("the fork did not activate recovery isolation")
	}
	if scheduler.headers[1].shifted || scheduler.headers[1].recoveryRegion() != nil {
		t.Fatalf("missing lineage consumed the real token: %+v", scheduler.headers[1])
	}
	if !scheduler.headers[0].shifted || scheduler.headers[0].recoveryRegion() == nil {
		t.Fatalf("absorb lineage did not consume the real token: %+v", scheduler.headers[0])
	}
	if scheduler.headers[0].creationSeq != 3 || scheduler.headers[1].creationSeq != 11 || scheduler.nextSeq != 12 {
		t.Fatalf("creation sequence=%d/%d next=%d, want 3/11 next 12",
			scheduler.headers[0].creationSeq, scheduler.headers[1].creationSeq, scheduler.nextSeq)
	}
	if scheduler.headers[0].recoveryGroupIdentity() != 10 ||
		scheduler.headers[1].recoveryMissingGroupIdentity() != 10 {
		t.Fatalf("S5 recovery groups=%d/%d, want absorb/missing group 10",
			scheduler.headers[0].recoveryGroupIdentity(), scheduler.headers[1].recoveryMissingGroupIdentity())
	}
	if baseline, set := scheduler.headers[0].recoveryNodeBaseline(); !set || baseline != 0 {
		t.Fatalf("S5 absorb baseline=%d/%t, want 0/true", baseline, set)
	}
	if baseline, set := scheduler.headers[1].recoveryNodeBaseline(); !set || baseline != 0 {
		t.Fatalf("S5 missing baseline=%d/%t, want 0/true", baseline, set)
	}
	if scheduler.s5MissingInsertions != 1 {
		t.Fatalf("missing insertion count=%d, want 1", scheduler.s5MissingInsertions)
	}
	if !scheduler.s3RegionOpened {
		t.Fatal("the S5 absorb lineage did not record its S3 region")
	}

	derivations, err := scheduler.compact.Derivations(scheduler.headers[1].head)
	if err != nil {
		t.Fatalf("missing derivation: %v", err)
	}
	if len(derivations) != 1 || len(derivations[0].Payloads) != 1 {
		t.Fatalf("missing derivations=%+v, want one payload", derivations)
	}
	reduced, err := scheduler.compact.MaterializationView(derivations[0].Payloads[0])
	if err != nil {
		t.Fatalf("reduced missing payload: %v", err)
	}
	if reduced.Symbol != 5 || len(reduced.Children) != 1 || !reduced.Fragile {
		t.Fatalf("reduced missing payload=%+v, want symbol 5 with one child", reduced)
	}
	missing, err := scheduler.compact.MaterializationView(reduced.Children[0])
	if err != nil {
		t.Fatalf("missing payload: %v", err)
	}
	if !missing.Missing || missing.Symbol != 2 || missing.StartByte != 1 || missing.EndByte != 1 {
		t.Fatalf("missing payload=%+v, want symbol 2 at byte 1", missing)
	}

	region := scheduler.headers[0].recoveryRegion()
	if len(region.children) != 1 || region.startByte != 1 || region.endByte != 2 {
		t.Fatalf("absorb region=%+v, want one child over bytes 1..2", region)
	}
	absorbed, err := scheduler.compact.MaterializationView(region.children[0])
	if err != nil {
		t.Fatalf("absorbed payload: %v", err)
	}
	if absorbed.Symbol != 4 || absorbed.StartByte != 1 || absorbed.EndByte != 2 {
		t.Fatalf("absorbed payload=%+v, want symbol 4 over bytes 1..2", absorbed)
	}
}

func TestS5AbsorberPreservesMixedParserStatesForRecoverySelection(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	other, err := scheduler.compact.Seed(core.StateID(3), 1)
	if err != nil {
		t.Fatal(err)
	}
	headers := []diagnosticParserCoreHeader{
		scheduler.headers[0],
		{head: other, creationSeq: 4},
	}
	staged := diagnosticParserCoreS5Work{}
	var absorb diagnosticParserCoreHeader
	err = scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var absorbErr error
		absorb, absorbErr = scheduler.s5AppendAndMergeAbsorberOwned(owner, headers, 0, 1, &staged)
		return absorbErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if absorb.head.Node == 0 {
		t.Fatal("mixed-state absorber did not publish a recovery marker")
	}
	stats, err := scheduler.compact.Stats(absorb.head)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CurrentExactPaths != 2 || staged.recoveryDiscontinuityMerges != 1 {
		t.Fatalf("mixed-state absorber stats=%+v work=%+v, want two paths and one marker merge", stats, staged)
	}
	region := absorb.recoveryRegion()
	if region == nil || region.state != 1 {
		t.Fatalf("mixed-state recovery region=%+v, want first recovery state 1", region)
	}
}

func TestS5AbsorberPreservesLexerGapBeforeErrorRegion(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.token.StartByte = 2
	scheduler.token.EndByte = 3
	staged := diagnosticParserCoreS5Work{}
	var absorb diagnosticParserCoreHeader
	err := scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var absorbErr error
		absorb, absorbErr = scheduler.s5AppendAndMergeAbsorberOwned(owner, scheduler.headers, 0, 1, &staged)
		return absorbErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if absorb.head.Node == 0 {
		t.Fatal("lexer-gap absorber did not publish a recovery marker")
	}
	state, byteOffset, err := scheduler.compact.Boundary(absorb.head)
	if err != nil {
		t.Fatal(err)
	}
	if state != 0 || byteOffset != 1 {
		t.Fatalf("lexer-gap marker=%d@%d, want ERROR_STATE at byte 1", state, byteOffset)
	}
	region := absorb.recoveryRegion()
	if region == nil || region.state != 1 || region.startByte != 2 || region.endByte != 3 || len(region.children) != 1 {
		t.Fatalf("lexer-gap region=%+v, want state 1 over bytes 2..3", region)
	}
}

func TestS5MissingInsertionClampsInheritedBaselineBeforeFork(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.headers[0].publishRecoveryCondenseState(0, 0, 9, true)
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if !handled || len(scheduler.headers) != 2 {
		t.Fatalf("S5 clamp fixture did not fork: handled=%t headers=%d", handled, len(scheduler.headers))
	}
	for index := range scheduler.headers {
		if baseline, set := scheduler.headers[index].recoveryNodeBaseline(); !set || baseline != 0 {
			t.Fatalf("S5 header %d baseline=%d/%t, want clamped 0/true", index, baseline, set)
		}
	}
}

func TestS5MissingInsertionUsesFixedRecoveryPosition(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	extraHead, err := scheduler.compact.Shift(
		scheduler.headers[0].head, core.Symbol(3), 0,
		core.Token{Symbol: 3, StartByte: 1, EndByte: 3, Extra: true}, core.ForkOrder{},
	)
	if err != nil {
		t.Fatalf("shift trailing extra: %v", err)
	}
	if _, byteOffset, boundaryErr := scheduler.compact.Boundary(extraHead); boundaryErr != nil || byteOffset != 3 {
		t.Fatalf("trailing-extra boundary=%d err=%v, want byte 3", byteOffset, boundaryErr)
	}

	var trial []diagnosticParserCoreHeader
	err = scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var viable bool
		trial, viable, _, err = scheduler.s5TryMissingCandidateOwned(
			owner,
			[]diagnosticParserCoreHeader{{head: extraHead, creationSeq: 3}},
			0, core.Symbol(2), core.StateID(2), 2, &diagnosticParserCoreS5Work{},
		)
		if err != nil {
			return err
		}
		if !viable {
			return errors.New("fixed-position missing candidate was not viable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(trial) == 0 {
		t.Fatal("fixed-position missing trial returned no headers")
	}
	derivations, err := scheduler.compact.Derivations(trial[0].head)
	if err != nil || len(derivations) != 1 {
		t.Fatalf("trial derivations=%+v err=%v", derivations, err)
	}
	stack := append([]core.SubtreeID(nil), derivations[0].Payloads...)
	found := false
	for len(stack) != 0 {
		last := len(stack) - 1
		payload := stack[last]
		stack = stack[:last]
		view, viewErr := scheduler.compact.MaterializationView(payload)
		if viewErr != nil {
			t.Fatal(viewErr)
		}
		if view.Missing {
			found = true
			if view.StartByte != 2 || view.EndByte != 2 {
				t.Fatalf("missing leaf range=%d..%d, want fixed recovery position 2..2", view.StartByte, view.EndByte)
			}
		}
		stack = append(stack, view.Children...)
	}
	if !found {
		t.Fatal("trial did not retain the missing leaf")
	}
}

func TestS3SecondIndependentErrorRegionDeclines(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.s3RegionOpened = true
	original := scheduler.headers[0]

	handled, err := scheduler.s3TryOpenErrorRegionWithAlternatives(0, true)
	if err == nil || !strings.Contains(err.Error(), "one error region per parse") {
		t.Fatalf("s3TryOpenErrorRegionWithAlternatives error=%v, want typed single-region decline", err)
	}
	if handled || scheduler.headers[0] != original {
		t.Fatalf("second region changed the frontier: handled=%t header=%+v", handled, scheduler.headers[0])
	}
	if !scheduler.s3RegionOpened {
		t.Fatal("the second-region decline cleared the first-region marker")
	}
}

func TestS3RegionMarkerAllowsClosureOnlyRedispatch(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	shifted, err := scheduler.compact.Shift(
		scheduler.headers[0].head,
		core.Symbol(2),
		0,
		core.Token{Symbol: 2, StartByte: 1, EndByte: 1},
		core.ForkOrder{},
	)
	if err != nil {
		t.Fatalf("shift closure input: %v", err)
	}
	scheduler.headers[0].head = shifted
	scheduler.s3RegionOpened = true

	handled, err := scheduler.s3TryOpenErrorRegion(0)
	if err != nil {
		t.Fatalf("s3TryOpenErrorRegion: %v", err)
	}
	if !handled || scheduler.headers[0].recoveryRegion() != nil {
		t.Fatalf("closure-only redispatch handled=%t region=%+v", handled, scheduler.headers[0].recoveryRegion())
	}
	state, _, err := scheduler.compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatalf("closed boundary: %v", err)
	}
	if state != 3 || !scheduler.s3RegionOpened {
		t.Fatalf("closure-only state=%d marker=%t, want 3/true", state, scheduler.s3RegionOpened)
	}
	if scheduler.headers[0].isRecoveryCosted() {
		t.Fatal("closure-only redispatch gained recovery cost provenance")
	}
}

func TestS3AbsorbCostProvenanceSurvivesResumeAndRegionClose(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	clean := scheduler.headers[0]

	handled, err := scheduler.s3TryOpenErrorRegionWithAlternatives(0, true)
	if err != nil {
		t.Fatalf("s3TryOpenErrorRegionWithAlternatives: %v", err)
	}
	if !handled {
		t.Fatal("standalone S3 absorb did not open a region")
	}
	recovered := &scheduler.headers[0]
	region := recovered.recoveryRegion()
	if region == nil || !recovered.isRecoveryCosted() || recovered.isRecoveryLineage() {
		t.Fatalf("opened standalone S3 header=%+v region=%+v", *recovered, region)
	}

	resumed, err := scheduler.compact.ErrorRegionResume(
		recovered.head, region.state, region.startByte, region.endByte, region.children,
	)
	if err != nil {
		t.Fatalf("ErrorRegionResume: %v", err)
	}
	recovered.head = resumed
	recovered.closeRecoveryRegion()
	if recovered.recoveryRegion() != nil || recovered.versionState != nil || !recovered.isRecoveryCosted() {
		t.Fatalf("closed standalone S3 header lost cost provenance: %+v", *recovered)
	}

	recoveredHeader := *recovered
	scheduler.headers = []diagnosticParserCoreHeader{clean, recoveredHeader}
	if candidates := scheduler.collectCondenseCandidates(0); len(candidates) != 0 {
		t.Fatalf("recovery-costed candidate entered clean physical merge: %+v", candidates)
	}
	scheduler.headers[0], scheduler.headers[1] = scheduler.headers[1], scheduler.headers[0]
	if candidates := scheduler.collectCondenseCandidates(0); len(candidates) != 0 {
		t.Fatalf("recovery-costed source entered clean physical merge: %+v", candidates)
	}
}

func TestS5MissingInsertionForkRequiresAcceptanceSelection(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, false)
	original := scheduler.headers[0]
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if handled || len(scheduler.headers) != 1 || scheduler.headers[0] != original {
		t.Fatalf("unarmed fork changed the frontier: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.s5MissingInsertions != 0 || scheduler.nextSeq != 10 || scheduler.recoveryIsolation {
		t.Fatalf("unarmed fork changed state: insertions=%d next=%d isolated=%t",
			scheduler.s5MissingInsertions, scheduler.nextSeq, scheduler.recoveryIsolation)
	}
}

func TestS5MissingInsertionForkDeclinesUnrepresentableTokenCount(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.tokenSource.language.TokenCount = uint32(^Symbol(0)) + 1
	original := scheduler.headers[0]

	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if handled || len(scheduler.headers) != 1 || scheduler.headers[0] != original {
		t.Fatalf("wide token table changed the frontier: handled=%t headers=%+v", handled, scheduler.headers)
	}
}

func TestS5MissingInsertionForkRequiresLeadingReduceAction(t *testing.T) {
	scheduler := newRecoveryLineageForkSchedulerWithTable(t, recoveryLineageForkLaterReduceTable{}, true)
	original := scheduler.headers[0]

	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if handled || len(scheduler.headers) != 1 || scheduler.headers[0] != original {
		t.Fatalf("later reduce action admitted a missing candidate: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.s5MissingInsertions != 0 || scheduler.nextSeq != 10 || scheduler.recoveryIsolation {
		t.Fatalf("later reduce action changed state: insertions=%d next=%d isolated=%t",
			scheduler.s5MissingInsertions, scheduler.nextSeq, scheduler.recoveryIsolation)
	}
}

func TestS5UpdatedReductionFreshnessTargetsAdoptedSibling(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	head := scheduler.headers[0].head
	scheduler.headers = []diagnosticParserCoreHeader{
		{head: head, creationSeq: 1},
		{head: head, creationSeq: 2},
		{head: head, creationSeq: 3},
	}
	slot, err := scheduler.s5UpdatedReductionSiblingIndex(0, head)
	if err != nil || slot != 1 {
		t.Fatalf("S5 adoption slot=%d err=%v, want first exact sibling 1", slot, err)
	}
	err = scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		adopted, adoptErr := scheduler.adoptUpdatedReductionSiblingOwned(
			owner, 0, head, core.CleanPathRankNotApplicable, 0,
			core.AlternativeSet{}, false, core.DropCohortRefSet{}, false, false,
			core.DropCohortProducerSiblingAdoption,
		)
		if adoptErr != nil {
			return adoptErr
		}
		if !adopted {
			return errors.New("S5 exact sibling was not adopted")
		}
		scheduler.headers[slot].freshness = core.ReductionUpdated
		return nil
	})
	if err != nil {
		t.Fatalf("adopt S5 sibling: %v", err)
	}
	if scheduler.headers[1].freshness != core.ReductionUpdated ||
		scheduler.headers[2].freshness != core.ReductionFreshness(0) {
		t.Fatalf("S5 freshness targeted the wrong sibling: %+v", scheduler.headers)
	}
}

func TestS5MissingInsertionForkAcceptsHiddenFirstCandidate(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.tokenSource.language.SymbolMetadata = make([]SymbolMetadata, 5)
	for index := range scheduler.tokenSource.language.SymbolMetadata {
		scheduler.tokenSource.language.SymbolMetadata[index].Visible = true
	}
	scheduler.tokenSource.language.SymbolMetadata[2].Visible = false
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if !handled || len(scheduler.headers) != 2 {
		t.Fatalf("hidden candidate did not fork: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.s5MissingInsertions != 1 || scheduler.nextSeq != 12 || !scheduler.recoveryIsolation {
		t.Fatalf("hidden candidate state: insertions=%d next=%d isolated=%t",
			scheduler.s5MissingInsertions, scheduler.nextSeq, scheduler.recoveryIsolation)
	}
	derivations, err := scheduler.compact.Derivations(scheduler.headers[1].head)
	if err != nil || len(derivations) != 1 || len(derivations[0].Payloads) != 1 {
		t.Fatalf("hidden missing derivations=%+v err=%v", derivations, err)
	}
	reduced, err := scheduler.compact.MaterializationView(derivations[0].Payloads[0])
	if err != nil || reduced.Symbol != 5 || len(reduced.Children) != 1 {
		t.Fatalf("hidden reduced payload=%+v err=%v", reduced, err)
	}
	missing, err := scheduler.compact.MaterializationView(reduced.Children[0])
	if err != nil || !missing.Missing || missing.Symbol != 2 {
		t.Fatalf("hidden missing payload=%+v err=%v", missing, err)
	}
}

func TestS5MissingInsertionForkAcceptsZeroOffsetAbsorb(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	seed, err := scheduler.compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	scheduler.headers[0].head = seed
	scheduler.token = Token{Symbol: 4, StartByte: 0, EndByte: 1}
	scheduler.tokenSource.lexer.source = []byte("?")
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if !handled || len(scheduler.headers) != 2 {
		t.Fatalf("zero-offset absorb did not fork: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.s5MissingInsertions != 1 || scheduler.nextSeq != 12 || !scheduler.recoveryIsolation {
		t.Fatalf("zero-offset absorb state: insertions=%d next=%d isolated=%t",
			scheduler.s5MissingInsertions, scheduler.nextSeq, scheduler.recoveryIsolation)
	}
	region := scheduler.headers[0].recoveryRegion()
	if region == nil || region.startByte != 0 || region.endByte != 1 {
		t.Fatalf("zero-offset region=%+v", region)
	}
}

func TestS5MissingInsertionForkRestoresFullTransactionOnDecline(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.token = Token{Symbol: 3, StartByte: 1, EndByte: 2}
	receipt := &DiagnosticParserCoreGenericScheduler{Tokens: 7}
	scheduler.receipt = receipt
	scheduler.verifierHeaderPtr = &scheduler.headers[0]
	scheduler.verifierBound = len(scheduler.headers)
	original := scheduler.headers[0]
	beforeStats, err := scheduler.compact.Stats(original.head)
	if err != nil {
		t.Fatalf("read pre-decline stats: %v", err)
	}
	beforeWork := scheduler.compact.Work()

	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil || handled {
		t.Fatalf("S5 decline handled=%t err=%v", handled, err)
	}
	requireS5ForkRollback(t, scheduler, original, beforeStats, beforeWork, receipt)
}

func TestS5MissingInsertionForkRestoresFullTransactionOnError(t *testing.T) {
	sentinel := errors.New("S5 action fault")
	table := &recoveryLineageForkFaultTable{returnError: sentinel}
	scheduler := newRecoveryLineageForkSchedulerWithTable(t, table, true)
	receipt := &DiagnosticParserCoreGenericScheduler{Tokens: 7}
	scheduler.receipt = receipt
	scheduler.verifierHeaderPtr = &scheduler.headers[0]
	scheduler.verifierBound = len(scheduler.headers)
	original := scheduler.headers[0]
	beforeStats, err := scheduler.compact.Stats(original.head)
	if err != nil {
		t.Fatalf("read pre-error stats: %v", err)
	}
	beforeWork := scheduler.compact.Work()

	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if handled || !errors.Is(err, sentinel) {
		t.Fatalf("S5 error handled=%t err=%v, want sentinel", handled, err)
	}
	requireS5ForkRollback(t, scheduler, original, beforeStats, beforeWork, receipt)
}

func TestS5MissingInsertionForkRestoresFullTransactionOnPanic(t *testing.T) {
	const sentinel = "S5 action panic"
	table := &recoveryLineageForkFaultTable{panicValue: sentinel}
	scheduler := newRecoveryLineageForkSchedulerWithTable(t, table, true)
	receipt := &DiagnosticParserCoreGenericScheduler{Tokens: 7}
	scheduler.receipt = receipt
	scheduler.verifierHeaderPtr = &scheduler.headers[0]
	scheduler.verifierBound = len(scheduler.headers)
	original := scheduler.headers[0]
	beforeStats, err := scheduler.compact.Stats(original.head)
	if err != nil {
		t.Fatalf("read pre-panic stats: %v", err)
	}
	beforeWork := scheduler.compact.Work()

	func() {
		defer func() {
			if recovered := recover(); recovered != sentinel {
				t.Fatalf("S5 recovered panic=%v, want %q", recovered, sentinel)
			}
		}()
		_, _ = scheduler.s5TryMissingTokenInsertion(0)
	}()
	requireS5ForkRollback(t, scheduler, original, beforeStats, beforeWork, receipt)
}

func TestRecoveryCompetitionDoesNotUseOrdinaryNoActionDrop(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.receipt = &DiagnosticParserCoreGenericScheduler{}
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil || !handled {
		t.Fatalf("fork: handled=%t err=%v", handled, err)
	}
	scheduler.token = Token{Symbol: 3, StartByte: 1, EndByte: 2}
	scheduler.epochProgress = true

	unsupported, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatalf("dispatchPass: %v", err)
	}
	if unsupported != nil {
		t.Fatalf("unsupported=%+v, want owned recovery dispatch", unsupported)
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("ordinary no-action logic dropped a recovery version: headers=%d", len(scheduler.headers))
	}
	if !scheduler.versionLexerOwnershipActive || len(scheduler.versionLexerRequests) != 1 {
		t.Fatalf("owned recovery dispatch was not armed: active=%t requests=%d",
			scheduler.versionLexerOwnershipActive, len(scheduler.versionLexerRequests))
	}
}

func TestRecoveryCompetitionDeclinesAfterOrdinaryAmbiguity(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil || !handled {
		t.Fatalf("fork: handled=%t err=%v", handled, err)
	}
	scheduler.headers[1].clearRecoveryLineage()

	unsupported, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatalf("dispatchPass: %v", err)
	}
	if unsupported == nil || unsupported.boundary != DiagnosticParserCoreRecovery {
		t.Fatalf("unsupported=%+v, want a recovery decline", unsupported)
	}
	if unsupported.detail != "recovery competition crossed ordinary grammar ambiguity" {
		t.Fatalf("detail=%q", unsupported.detail)
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("decline changed the recovery frontier: headers=%d", len(scheduler.headers))
	}
}

func TestRecoveryCanonicalizationKeepsMarkedOwnersSeparate(t *testing.T) {
	compact, err := core.New(lineageSelectionTable{}, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	leftPayload, err := compact.ErrorRegionLeaf(core.Symbol(3), 0, 1, false)
	if err != nil {
		t.Fatalf("left payload: %v", err)
	}
	rightPayload, err := compact.MissingLeaf(core.Symbol(4), 1)
	if err != nil {
		t.Fatalf("right payload: %v", err)
	}
	var left, right core.Head
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var publishErr error
		left, publishErr = compact.ErrorRegionResumeWithLiveCondenseCandidatesOwned(
			owner, nil, seed, core.StateID(2), 0, 1, []core.SubtreeID{leftPayload},
		)
		if publishErr != nil {
			return publishErr
		}
		right, publishErr = compact.ErrorRegionResumeWithLiveCondenseCandidatesOwned(
			owner, nil, seed, core.StateID(2), 0, 1, []core.SubtreeID{rightPayload},
		)
		return publishErr
	})
	if err != nil {
		t.Fatalf("publish isolated heads: %v", err)
	}
	if left == right {
		t.Fatal("isolated recovery publications returned one compact head")
	}

	headers := []diagnosticParserCoreHeader{
		{head: left, creationSeq: 1},
		{head: right, creationSeq: 2},
	}
	for index := range headers {
		headers[index].markRecoveryLineage()
	}
	var scratch diagnosticParserCoreCanonicalScratch
	isolated, err := scratch.canonicalizeRecovery(compact, headers)
	if err != nil {
		t.Fatalf("canonicalize marked: %v", err)
	}
	if len(isolated) != 2 || isolated[0].head == isolated[1].head {
		t.Fatalf("marked recovery owners reconverged: %+v", isolated)
	}

	for index := range headers {
		headers[index].clearRecoveryLineage()
		headers[index].shifted = true
	}
	ordinary, err := scratch.canonicalize(compact, headers)
	if err != nil {
		t.Fatalf("canonicalize ordinary: %v", err)
	}
	if len(ordinary) != 1 {
		t.Fatalf("ordinary canonicalization retained %d heads, want 1", len(ordinary))
	}
}

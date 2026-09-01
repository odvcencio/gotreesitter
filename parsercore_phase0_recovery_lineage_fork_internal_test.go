//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type recoveryLineageForkTable struct{}

func (recoveryLineageForkTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	var actions []core.Action
	switch {
	case state == 1 && symbol == 2:
		actions = []core.Action{{Type: core.ActionShift, State: 2}}
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
	t.Helper()
	compact, err := core.New(recoveryLineageForkTable{}, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(core.StateID(1), 1)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	lang := &Language{TokenCount: 5}
	return &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			language: lang,
			lexer:    &Lexer{source: []byte("a?")},
		},
		headers: []diagnosticParserCoreHeader{{head: seed, creationSeq: 3}},
		token:   Token{Symbol: 4, StartByte: 1, EndByte: 2},
		nextSeq: 10,
		options: DiagnosticParserCorePrefixOptions{
			Recovery:                             true,
			allowCompactStrategy2ErrorRegion:     true,
			allowCompactMissingTokenInsertion:    true,
			allowCompactRecoveryLineageSelection: armed,
		},
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
	if scheduler.headers[0].creationSeq != 3 || scheduler.headers[1].creationSeq != 10 || scheduler.nextSeq != 11 {
		t.Fatalf("creation sequence=%d/%d next=%d, want 3/10 next 11",
			scheduler.headers[0].creationSeq, scheduler.headers[1].creationSeq, scheduler.nextSeq)
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
	missing, err := scheduler.compact.MaterializationView(derivations[0].Payloads[0])
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

func TestS5MissingInsertionForkDeclinesHiddenFirstCandidate(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	scheduler.tokenSource.language.SymbolMetadata = make([]SymbolMetadata, 5)
	for index := range scheduler.tokenSource.language.SymbolMetadata {
		scheduler.tokenSource.language.SymbolMetadata[index].Visible = true
	}
	scheduler.tokenSource.language.SymbolMetadata[2].Visible = false
	original := scheduler.headers[0]

	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if handled || len(scheduler.headers) != 1 || scheduler.headers[0] != original {
		t.Fatalf("hidden candidate changed the frontier: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.s5MissingInsertions != 0 || scheduler.nextSeq != 10 || scheduler.recoveryIsolation {
		t.Fatalf("hidden candidate changed state: insertions=%d next=%d isolated=%t",
			scheduler.s5MissingInsertions, scheduler.nextSeq, scheduler.recoveryIsolation)
	}
}

func TestS5MissingInsertionForkRestoresFrontierWhenAbsorbDeclines(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
	seed, err := scheduler.compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	scheduler.headers[0].head = seed
	scheduler.token = Token{Symbol: 4, StartByte: 0, EndByte: 1}
	scheduler.tokenSource.lexer.source = []byte("?")
	original := scheduler.headers[0]

	handled, err := scheduler.s5TryMissingTokenInsertion(0)
	if err != nil {
		t.Fatalf("s5TryMissingTokenInsertion: %v", err)
	}
	if handled || len(scheduler.headers) != 1 || scheduler.headers[0] != original {
		t.Fatalf("declined absorb changed the frontier: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.s5MissingInsertions != 0 || scheduler.nextSeq != 10 || scheduler.recoveryIsolation {
		t.Fatalf("declined absorb changed state: insertions=%d next=%d isolated=%t",
			scheduler.s5MissingInsertions, scheduler.nextSeq, scheduler.recoveryIsolation)
	}
}

func TestRecoveryCompetitionDoesNotUseOrdinaryNoActionDrop(t *testing.T) {
	scheduler := newRecoveryLineageForkScheduler(t, true)
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
	if unsupported == nil || unsupported.boundary != DiagnosticParserCoreRecovery {
		t.Fatalf("unsupported=%+v, want a recovery decline", unsupported)
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("ordinary no-action logic dropped a recovery version: headers=%d", len(scheduler.headers))
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

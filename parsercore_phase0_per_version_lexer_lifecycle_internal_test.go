//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type diagnosticParserCoreOwnedLexerPanicRestoreScanner struct{}

func (diagnosticParserCoreOwnedLexerPanicRestoreScanner) Create() any { return new(byte) }
func (diagnosticParserCoreOwnedLexerPanicRestoreScanner) Destroy(any) {}
func (diagnosticParserCoreOwnedLexerPanicRestoreScanner) Serialize(any, []byte) int {
	return 1
}
func (diagnosticParserCoreOwnedLexerPanicRestoreScanner) Deserialize(any, []byte) {
	panic("owned scanner restore panic")
}
func (diagnosticParserCoreOwnedLexerPanicRestoreScanner) Scan(any, *ExternalLexer, []bool) bool {
	return false
}

func newDiagnosticParserCoreOwnedLexerSnapshot(
	t *testing.T,
	compact *core.Core,
	language *Language,
	lexerPosition int,
) *diagnosticParserCoreVersionLexerSnapshot {
	t.Helper()
	var snapshot *diagnosticParserCoreVersionLexerSnapshot
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var snapshotErr error
		snapshot, snapshotErr = newDiagnosticParserCoreVersionLexerSnapshot(
			compact,
			language,
			owner,
			dfaRelexSnapshot{lexerPos: lexerPosition},
			0,
			0,
		)
		return snapshotErr
	})
	if err != nil {
		t.Fatalf("construct owned lexer snapshot: %v", err)
	}
	return snapshot
}

func newDiagnosticParserCoreOwnedLexerRequest(
	electionIndex int,
	state StateID,
	token Token,
	before *diagnosticParserCoreVersionLexerSnapshot,
	after *diagnosticParserCoreVersionLexerSnapshot,
) diagnosticParserCoreVersionLexerRequest {
	return diagnosticParserCoreVersionLexerRequest{
		electionIndex: electionIndex,
		state:         state,
		token:         token,
		before:        before,
		after:         after,
		valid:         true,
	}
}

func TestDiagnosticParserCoreOwnedLexerSeedingSkipsAcceptedHeader(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	acceptedHead, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	liveHead, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "owned-lexer-accepted-seed-test"}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: newDFATokenSourceDirect(NewLexer(nil, nil), language, nil, nil, nil, nil),
		headers: []diagnosticParserCoreHeader{
			{head: acceptedHead, accepted: true},
			{head: liveHead},
		},
		versionLexerBefore:           dfaRelexSnapshot{},
		versionLexerBeforeValid:      true,
		versionLexerBeforeElection:   5,
		versionLexerBeforeCheckpoint: 0,
		checkpointBeforeID:           0,
		checkpointID:                 0,
		electionIndex:                5,
	}
	if err := scheduler.seedVersionLexerOwnership(); err != nil {
		t.Fatalf("seed owned lexer frontier: %v", err)
	}
	if scheduler.headers[0].versionState != nil || scheduler.headers[0].versionLexerSnapshot() != nil ||
		scheduler.headers[0].versionLexerRequestReference() != 0 {
		t.Fatalf("seeding populated accepted header: %+v", scheduler.headers[0])
	}
	if scheduler.headers[1].versionLexerSnapshot() == nil || scheduler.headers[1].versionLexerRequestReference() != 0 {
		t.Fatalf("seeding did not publish the live header cursor: %+v", scheduler.headers[1])
	}
	if scheduler.work.PerVersionLexPublications != 1 {
		t.Fatalf("seed publications=%d, want one live header", scheduler.work.PerVersionLexPublications)
	}
}

func TestDiagnosticParserCoreOwnedLexerActivationRejectsOpenRecoveryRegion(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	recoveryHead, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryHead, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "owned-lexer-open-recovery-activation-test"}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: newDFATokenSourceDirect(NewLexer(nil, nil), language, nil, nil, nil, nil),
		headers: []diagnosticParserCoreHeader{
			{
				head: recoveryHead,
				versionState: &diagnosticParserCoreVersionState{
					s3Region: &diagnosticParserCoreS3Region{state: 1},
				},
			},
			{head: ordinaryHead},
		},
		versionLexerBefore:           dfaRelexSnapshot{},
		versionLexerBeforeValid:      true,
		versionLexerBeforeElection:   5,
		versionLexerBeforeCheckpoint: 0,
		checkpointBeforeID:           0,
		checkpointID:                 0,
		electionIndex:                5,
		receipt:                      &DiagnosticParserCoreGenericScheduler{},
	}
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	stop, err := scheduler.activateVersionLexerOwnershipAtRagged(1)
	if err != nil {
		t.Fatalf("activate owned lexer frontier: %v", err)
	}
	if stop == nil || stop.boundary != DiagnosticParserCoreRoute ||
		!strings.Contains(stop.detail, "open recovery region") || stop.headerIndex != 0 {
		t.Fatalf("open recovery activation stop=%+v", stop)
	}
	if scheduler.versionLexerOwnershipActive || len(scheduler.versionLexerRequests) != 0 ||
		!reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.work != (DiagnosticParserCoreGenericWork{}) {
		t.Fatalf("blocked activation mutated state: active=%t requests=%d headers=%+v work=%+v",
			scheduler.versionLexerOwnershipActive,
			len(scheduler.versionLexerRequests),
			scheduler.headers,
			scheduler.work,
		)
	}
}

func TestDiagnosticParserCoreOwnedLexerBindingRestoresAfterErrorAndPanic(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 9}: {{Type: core.ActionShift, State: 2}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "owned-lexer-binding-test"}
	before := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	after := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 1)
	token := Token{Symbol: 9, StartByte: 0, EndByte: 1}
	request := newDiagnosticParserCoreOwnedLexerRequest(4, 1, token, before, after)
	header := diagnosticParserCoreHeader{
		head: head,
		versionState: &diagnosticParserCoreVersionState{
			relexSnapshot: before,
			lexerRequest:  1,
		},
	}
	boundary, err := compact.ClassifyBoundary(head, 9)
	if err != nil {
		t.Fatal(err)
	}
	priorToken := Token{Symbol: 7, StartByte: 3, EndByte: 4}
	priorElection := DiagnosticParserCoreElection{Token: priorToken}
	priorCell := diagnosticParserCoreTokenCell{token: priorToken, state: 7, valid: true}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			lexer:    &Lexer{source: []byte("x")},
			language: language,
		},
		headers:              []diagnosticParserCoreHeader{header},
		token:                priorToken,
		currentElection:      priorElection,
		tokenCell:            priorCell,
		electionIndex:        4,
		versionLexerRequests: []diagnosticParserCoreVersionLexerRequest{request},
	}
	cell := diagnosticParserCoreGenericCell{headerIndex: 0, boundary: boundary, versionLexerRequest: 1}
	callbackErr := errors.New("owned callback failed")
	if err := scheduler.withVersionLexerRequest(cell, func() error {
		if scheduler.token != token || scheduler.currentElection.Token != token {
			t.Fatalf("owned request was not bound: token=%+v election=%+v", scheduler.token, scheduler.currentElection)
		}
		return callbackErr
	}); !errors.Is(err, callbackErr) {
		t.Fatalf("callback error=%v, want %v", err, callbackErr)
	}
	assertRestored := func() {
		t.Helper()
		if scheduler.token != priorToken || !reflect.DeepEqual(scheduler.currentElection, priorElection) || scheduler.tokenCell != priorCell {
			t.Fatalf("request binding leaked scheduler state: token=%+v election=%+v cell=%+v", scheduler.token, scheduler.currentElection, scheduler.tokenCell)
		}
		_, start, end, exact := compact.PhaseScannerCheckpoints()
		if start != 0 || end != 0 || exact {
			t.Fatalf("request binding leaked scanner phase: start=%d end=%d exact=%t", start, end, exact)
		}
	}
	assertRestored()

	func() {
		defer func() {
			if recovered := recover(); recovered != "owned callback panic" {
				t.Fatalf("panic=%v, want original callback panic", recovered)
			}
		}()
		_ = scheduler.withVersionLexerRequest(cell, func() error {
			panic("owned callback panic")
		})
	}()
	assertRestored()
}

func TestDiagnosticParserCoreOwnedLexerRejoinRestoresTokenSourceOnPhaseFailure(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "owned-lexer-rejoin-test"}
	snapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 1)
	tokenSource := &dfaTokenSource{
		lexer:     &Lexer{source: []byte("12345678"), pos: 7, row: 2, col: 3},
		language:  language,
		state:     9,
		glrStates: []StateID{9, 10},
	}
	priorDFA := tokenSource.snapshotRelexState()
	priorGLRStates := append([]StateID(nil), tokenSource.glrStates...)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: tokenSource,
		headers: []diagnosticParserCoreHeader{{
			head:    head,
			shifted: true,
			versionState: &diagnosticParserCoreVersionState{
				relexSnapshot: snapshot,
			},
		}},
		versionLexerOwnershipActive: true,
	}
	err = compact.ApplySchedulerAtomic(func(core.SchedulerTransactionToken) error {
		return scheduler.rejoinSharedLexerFromOwnedHeader()
	})
	if err == nil || !strings.Contains(err.Error(), "set checkpoint during active transaction") {
		t.Fatalf("rejoin phase error=%v", err)
	}
	if got := tokenSource.snapshotRelexState(); !got.equal(priorDFA) || tokenSource.state != 9 ||
		!reflect.DeepEqual(tokenSource.glrStates, priorGLRStates) {
		t.Fatalf("rejoin failure leaked token source: dfa=%+v state=%d glr=%v", got, tokenSource.state, tokenSource.glrStates)
	}
	if len(scheduler.headers) != 1 || scheduler.headers[0].versionLexerSnapshot() != snapshot ||
		!scheduler.versionLexerOwnershipActive {
		t.Fatalf("rejoin failure mutated owned frontier: headers=%+v active=%t", scheduler.headers, scheduler.versionLexerOwnershipActive)
	}
}

func TestDiagnosticParserCoreOwnedLexerRejoinRejectsMissingTokenSource(t *testing.T) {
	scheduler := &diagnosticParserCoreGenericScheduler{
		headers:                     []diagnosticParserCoreHeader{{shifted: true}},
		versionLexerOwnershipActive: true,
	}
	if err := scheduler.rejoinSharedLexerFromOwnedHeader(); err == nil ||
		!strings.Contains(err.Error(), "token source is unavailable") {
		t.Fatalf("missing token source error=%v", err)
	}
	scheduler.tokenSource = &dfaTokenSource{}
	if err := scheduler.rejoinSharedLexerFromOwnedHeader(); err == nil ||
		!strings.Contains(err.Error(), "token source is unavailable") {
		t.Fatalf("missing lexer error=%v", err)
	}
}

func TestDiagnosticParserCoreOwnedLexerActivationRollsBackAfterPanic(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := compact.InternCheckpoint([]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.SetPhaseCheckpoint(checkpoint); err != nil {
		t.Fatal(err)
	}
	firstHead, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondHead, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	scanner := diagnosticParserCoreOwnedLexerPanicRestoreScanner{}
	language := &Language{Name: "owned-lexer-activation-panic-test", ExternalScanner: scanner}
	tokenSource := &dfaTokenSource{
		lexer:              &Lexer{},
		language:           language,
		hasExternalScanner: true,
		externalPayload:    scanner.Create(),
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: tokenSource,
		headers: []diagnosticParserCoreHeader{
			{head: firstHead},
			{head: secondHead},
		},
		checkpointBeforeID:           checkpoint,
		electionIndex:                5,
		versionLexerBefore:           dfaRelexSnapshot{externalScannerPresent: true, externalPayload: []byte{1}},
		versionLexerBeforeValid:      true,
		versionLexerBeforeElection:   5,
		versionLexerBeforeCheckpoint: checkpoint,
		receipt:                      &DiagnosticParserCoreGenericScheduler{},
	}
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	func() {
		defer func() {
			if recovered := recover(); recovered != "owned scanner restore panic" {
				t.Fatalf("activation panic=%v", recovered)
			}
		}()
		_, _ = scheduler.activateVersionLexerOwnershipAtRagged(0)
	}()
	if !reflect.DeepEqual(scheduler.headers, beforeHeaders) || len(scheduler.versionLexerRequests) != 0 ||
		len(scheduler.receipt.VersionLexerRequests) != 0 || scheduler.versionLexerOwnershipActive ||
		scheduler.headerRollbackScratch.busy || scheduler.work != (DiagnosticParserCoreGenericWork{}) {
		t.Fatalf("activation panic rollback drifted: headers=%+v requests=%d receiptRequests=%d active=%t scratchBusy=%t work=%+v",
			scheduler.headers,
			len(scheduler.versionLexerRequests),
			len(scheduler.receipt.VersionLexerRequests),
			scheduler.versionLexerOwnershipActive,
			scheduler.headerRollbackScratch.busy,
			scheduler.work,
		)
	}
}

func TestDiagnosticParserCoreOwnedLexerDropsAuthenticatedPausedVersion(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	pausedHead, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	shiftedHead, err := compact.Seed(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "owned-lexer-paused-drop-test"}
	pausedSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	shiftedSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 1)
	token := Token{Symbol: 9, StartByte: 0, EndByte: 1}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			lexer:    &Lexer{source: []byte("x")},
			language: language,
		},
		headers: []diagnosticParserCoreHeader{
			{
				head:   pausedHead,
				paused: true,
				versionState: &diagnosticParserCoreVersionState{
					relexSnapshot: pausedSnapshot,
					lexerRequest:  1,
				},
			},
			{
				head:    shiftedHead,
				shifted: true,
				versionState: &diagnosticParserCoreVersionState{
					relexSnapshot: shiftedSnapshot,
				},
			},
		},
		token:                       token,
		electionIndex:               6,
		epochProgress:               true,
		versionLexerOwnershipActive: true,
		versionLexerRequests: []diagnosticParserCoreVersionLexerRequest{
			newDiagnosticParserCoreOwnedLexerRequest(6, 1, token, pausedSnapshot, pausedSnapshot),
			newDiagnosticParserCoreOwnedLexerRequest(6, 2, token, pausedSnapshot, shiftedSnapshot),
		},
		options: DiagnosticParserCorePrefixOptions{
			ReceiptMode:   DiagnosticParserCoreReceiptSummary,
			MaxDispatches: 10,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil {
		t.Fatalf("paused owned drop stop=%+v err=%v", stop, err)
	}
	if len(scheduler.headers) != 1 || scheduler.headers[0].head != shiftedHead || !scheduler.headers[0].shifted {
		t.Fatalf("paused owned drop frontier=%+v, want shifted survivor", scheduler.headers)
	}
	if scheduler.work.NoActionDrops != 1 || scheduler.work.PerVersionLexViabilityDrops != 1 {
		t.Fatalf("paused owned drop work=%+v", scheduler.work)
	}
}

func TestDiagnosticParserCoreOwnedLexerNoLookaheadRequiresOneRunnableHead(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 0}: {{Type: core.ActionReduce, Symbol: 3}},
		{state: 2, symbol: 0}: {{Type: core.ActionReduce, Symbol: 3}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	firstHead, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondHead, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "owned-lexer-no-lookahead-test"}
	firstSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	secondSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	token := Token{NoLookahead: true}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			lexer:    &Lexer{},
			language: language,
		},
		headers: []diagnosticParserCoreHeader{
			{head: firstHead, versionState: &diagnosticParserCoreVersionState{relexSnapshot: firstSnapshot, lexerRequest: 1}},
			{head: secondHead, versionState: &diagnosticParserCoreVersionState{relexSnapshot: secondSnapshot, lexerRequest: 2}},
		},
		token:                       token,
		electionIndex:               2,
		versionLexerOwnershipActive: true,
		versionLexerRequests: []diagnosticParserCoreVersionLexerRequest{
			newDiagnosticParserCoreOwnedLexerRequest(2, 1, token, firstSnapshot, firstSnapshot),
			newDiagnosticParserCoreOwnedLexerRequest(2, 2, token, secondSnapshot, secondSnapshot),
		},
		options: DiagnosticParserCorePrefixOptions{
			ReceiptMode:              DiagnosticParserCoreReceiptSummary,
			MaxDispatches:            10,
			hasNoLookaheadRootSymbol: true,
			noLookaheadRootSymbol:    3,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatal(err)
	}
	if stop == nil || stop.boundary != DiagnosticParserCoreRoute ||
		!strings.Contains(stop.detail, "requires one runnable head") {
		t.Fatalf("owned no-lookahead stop=%+v", stop)
	}
	if scheduler.dispatches != 0 || scheduler.work.Reductions != 0 {
		t.Fatalf("owned no-lookahead mutated scheduler: dispatches=%d work=%+v", scheduler.dispatches, scheduler.work)
	}
}

func TestDiagnosticParserCoreOwnedLexerElectionEnforcesPostReductionEOF(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	firstHead, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondHead, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{
		Name: "owned-lexer-post-reduction-eof-test",
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: 'x', Hi: 'x', NextState: 1},
				},
			},
			{AcceptToken: 9, Default: -1, EOF: -1},
		},
	}
	firstSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	secondSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			lexer:    NewLexer(language.LexStates, []byte("x")),
			language: language,
		},
		headers: []diagnosticParserCoreHeader{
			{head: firstHead, shifted: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: firstSnapshot}},
			{head: secondHead, shifted: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: secondSnapshot}},
		},
		electionIndex:                 7,
		versionLexerOwnershipActive:   true,
		requireEOFPostNoLookaheadRoot: true,
		receipt:                       &DiagnosticParserCoreGenericScheduler{},
	}
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	err = scheduler.beginNextVersionLexerElection()
	var decline *diagnosticParserCoreDecline
	if !errors.As(err, &decline) || decline.boundary != DiagnosticParserCoreRoute ||
		!strings.Contains(decline.detail, "not followed by authenticated EOF") {
		t.Fatalf("post-reduction EOF error=%v", err)
	}
	if scheduler.electionIndex != 7 || !scheduler.requireEOFPostNoLookaheadRoot ||
		len(scheduler.versionLexerRequests) != 0 || !reflect.DeepEqual(scheduler.headers, beforeHeaders) {
		t.Fatalf("post-reduction EOF rollback drifted: election=%d requireEOF=%t requests=%d headers=%+v",
			scheduler.electionIndex,
			scheduler.requireEOFPostNoLookaheadRoot,
			len(scheduler.versionLexerRequests),
			scheduler.headers,
		)
	}
}

func TestDiagnosticParserCoreOwnedLexerElectionEnforcesNoLookaheadCap(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	firstHead, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondHead, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{
		Name: "owned-lexer-no-lookahead-cap-test",
		LexModes: []LexMode{
			{LexStateID: noLookaheadLexState},
			{LexStateID: noLookaheadLexState},
			{LexStateID: noLookaheadLexState},
		},
	}
	firstSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	secondSnapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 0)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			lexer:    NewLexer(nil, nil),
			language: language,
		},
		headers: []diagnosticParserCoreHeader{
			{head: firstHead, shifted: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: firstSnapshot}},
			{head: secondHead, shifted: true, versionState: &diagnosticParserCoreVersionState{relexSnapshot: secondSnapshot}},
		},
		electionIndex:               11,
		noLookaheadSteps:            maxDiagnosticParserCoreNoLookaheadSteps,
		versionLexerOwnershipActive: true,
		receipt:                     &DiagnosticParserCoreGenericScheduler{},
	}
	beforeHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	err = scheduler.beginNextVersionLexerElection()
	var decline *diagnosticParserCoreDecline
	if !errors.As(err, &decline) || decline.boundary != DiagnosticParserCoreCap ||
		!strings.Contains(decline.detail, "no-lookahead re-election cap") {
		t.Fatalf("no-lookahead cap error=%v", err)
	}
	if scheduler.electionIndex != 11 || scheduler.noLookaheadSteps != maxDiagnosticParserCoreNoLookaheadSteps ||
		len(scheduler.versionLexerRequests) != 0 || !reflect.DeepEqual(scheduler.headers, beforeHeaders) {
		t.Fatalf("no-lookahead cap rollback drifted: election=%d steps=%d requests=%d headers=%+v",
			scheduler.electionIndex,
			scheduler.noLookaheadSteps,
			len(scheduler.versionLexerRequests),
			scheduler.headers,
		)
	}
}

func TestDiagnosticParserCoreOwnedLexerZeroWidthExtraRequiresProgress(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 9}: {{Type: core.ActionShift, Extra: true}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(5, 10)
	if err != nil {
		t.Fatal(err)
	}
	beforeStats, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{Name: "owned-lexer-zero-width-extra-test"}
	snapshot := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 10)
	token := Token{Symbol: 9, StartByte: 10, EndByte: 10, ExternalScannerToken: true}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			lexer:    &Lexer{},
			language: language,
		},
		headers: []diagnosticParserCoreHeader{{
			head: head,
			versionState: &diagnosticParserCoreVersionState{
				relexSnapshot: snapshot,
				lexerRequest:  1,
			},
		}},
		token:                       token,
		electionIndex:               3,
		versionLexerOwnershipActive: true,
		versionLexerRequests: []diagnosticParserCoreVersionLexerRequest{
			newDiagnosticParserCoreOwnedLexerRequest(3, 5, token, snapshot, snapshot),
		},
		options: DiagnosticParserCorePrefixOptions{
			ReceiptMode:   DiagnosticParserCoreReceiptSummary,
			MaxDispatches: 10,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatal(err)
	}
	if stop == nil || stop.boundary != DiagnosticParserCoreRoute ||
		stop.detail != "generic scheduler zero-width extra shift has no scanner or parser-state progress" {
		t.Fatalf("owned zero-width extra stop=%+v", stop)
	}
	afterStats, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterStats, beforeStats) || scheduler.dispatches != 0 {
		t.Fatalf("owned zero-width extra mutated state: before=%+v after=%+v dispatches=%d", beforeStats, afterStats, scheduler.dispatches)
	}
}

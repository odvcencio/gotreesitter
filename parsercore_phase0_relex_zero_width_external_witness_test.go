//go:build gts_parsercorephase0

package gotreesitter

import "testing"

// newCompactZeroWidthRelexWitnessScheduler wires perlNonassocWitnessLanguage
// (parser_relex_zero_width_external_witness_test.go, the production perl
// `_NONASSOC` witness fixture) up to a minimal
// diagnosticParserCoreGenericScheduler, mirroring
// TestDiagnosticParserCoreExternalVersionRelexOwnsCheckpoint's own
// direct-construction pattern (parsercore_phase0_state_relex_internal_test.go)
// closely enough to reuse the exact same source, shared token, and grammar
// production's own zero-width witness test uses.
//
// state is the starved header's OWN current parse state (1 in the shared
// fixture grammar: no action for `number` until it shifts `_nonassoc`).
// s.token is set to tok so relexZeroWidthExternalTokenForState's
// shared-election-identity guard passes, matching a real starved header
// still sharing the current dispatch pass's shared token.
func newCompactZeroWidthRelexWitnessScheduler(t *testing.T, lang *Language, state StateID) (*diagnosticParserCoreGenericScheduler, Token) {
	t.Helper()
	p := NewParser(lang)
	source := []byte("foo(1, 2;\n")
	tok := Token{Symbol: 1, StartByte: 4, EndByte: 5, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 5}}
	tokenSource := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, p.lookupActionIndex, nil, nil, nil)
	t.Cleanup(tokenSource.Close)
	tokenSource.SetParserState(state)

	// captureSharedElectionSnapshot's own doc (parsercore_phase0_driver.go)
	// says elect() captures this before tokenSource.Next() produces the
	// shared token every election, regardless of checkpoint support -- the
	// exact proof relexZeroWidthExternalTokenForState reuses. Capture it here
	// the same way, directly, since this fixture drives the probe without a
	// real election loop.
	before := tokenSource.snapshotRelexState()

	scheduler := &diagnosticParserCoreGenericScheduler{
		tokenSource:             tokenSource,
		versionLexerBefore:      before,
		versionLexerBeforeValid: true,
		token:                   tok,
	}
	return scheduler, tok
}

// TestRelexZeroWidthExternalTokenForStateAdmitsPerlNonassocWitness proves the
// compact-route probe reaches the same verdict production's
// relexZeroWidthExternalTokenForStackLexState reaches on the identical perl
// `_NONASSOC` witness grammar (parser_relex_zero_width_external_witness_test.go):
// a starved header with no action for the shared `number` token admits a
// zero-width external marker at the shared token's own start byte, using only
// the election-start payload every election already captures -- no checkpoint
// identity involved, matching the design decision recorded on
// relexZeroWidthExternalTokenForState's own doc comment.
func TestRelexZeroWidthExternalTokenForStateAdmitsPerlNonassocWitness(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)

	if scheduler.tokenSource.stateHasActionForSymbol(1, tok.Symbol) {
		t.Fatal("precondition: starved state must have no action for the shared token before the rescue")
	}

	scheduler.zeroWidthRelexBudget = maxConsecutiveZeroWidthTokens
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex

	got, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok)
	if !ok {
		t.Fatal("compact zero-width external rescue declined the perl _NONASSOC witness")
	}
	if got.Symbol != 2 || got.StartByte != tok.StartByte || got.EndByte != tok.StartByte || !got.ExternalScannerToken {
		t.Fatalf("admitted token = %+v, want zero-width symbol=2 at byte %d", got, tok.StartByte)
	}
	if !scheduler.tokenSource.stateHasActionForSymbol(3, tok.Symbol) {
		t.Fatal("postcondition: the post-shift state (3) must have a real action for the shared token")
	}
	if scheduler.zeroWidthRelexBudget != maxConsecutiveZeroWidthTokens-1 {
		t.Fatalf("zeroWidthRelexBudget = %d, want %d after one admission", scheduler.zeroWidthRelexBudget, maxConsecutiveZeroWidthTokens-1)
	}
}

// TestRelexZeroWidthExternalTokenForStateAdmitsWhereIdentityGateDeclines is
// the design-decision witness this task's port turns on: relexExternalTokenForState
// declines outright for perlNonassocZeroWidthExternalScanner (no checkpoint
// support, not declared stateless), yet relexZeroWidthExternalTokenForState
// admits the same starved header on the same shared token using only the
// election-start payload every election already captures. The compact route
// does not need a new identity proof for this narrower rescue -- it reuses
// the one production uses (externalPreScanPayload / here,
// versionLexerBefore.externalPayload, captured per election regardless of
// checkpoint support).
func TestRelexZeroWidthExternalTokenForStateAdmitsWhereIdentityGateDeclines(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)

	if _, ok := scheduler.relexExternalTokenForState(1, tok); ok {
		t.Fatal("precondition: relexExternalTokenForState must decline a non-checkpoint, non-stateless scanner")
	}

	scheduler.zeroWidthRelexBudget = maxConsecutiveZeroWidthTokens
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex
	if _, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok); !ok {
		t.Fatal("relexZeroWidthExternalTokenForState declined where it should reuse the non-identity election-start proof")
	}
}

// TestRelexZeroWidthExternalTokenForStateRequiresSharedToken proves the
// compact equivalent of production's byte-continuity guard
// (realTokenAttachmentGapIsParserPadding): every live, not-yet-shifted header
// shares one byte position within a dispatch pass by construction, so this
// probe only ever rescues the literal current shared election token, never a
// derived one (for example an S3 error-region-adjusted token) that might sit
// at a different byte.
func TestRelexZeroWidthExternalTokenForStateRequiresSharedToken(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)
	scheduler.zeroWidthRelexBudget = maxConsecutiveZeroWidthTokens
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex

	drifted := tok
	drifted.StartByte++
	drifted.EndByte++
	if _, ok := scheduler.relexZeroWidthExternalTokenForState(1, drifted); ok {
		t.Fatal("rescue admitted a token that is not the scheduler's own current shared election token")
	}
}

// TestRelexZeroWidthExternalTokenForStateDeclinesOnceOwnershipIsActive proves
// the rescue declines once the scheduler has already switched to independent
// per-header owned lexing: at that point every live header runs its own
// Next() and this probe's premise -- one shared election every live header
// still shares -- no longer holds.
func TestRelexZeroWidthExternalTokenForStateDeclinesOnceOwnershipIsActive(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)
	scheduler.zeroWidthRelexBudget = maxConsecutiveZeroWidthTokens
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex
	scheduler.versionLexerOwnershipActive = true

	if _, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok); ok {
		t.Fatal("rescue admitted a token after ragged ownership already activated")
	}
}

// TestRelexZeroWidthExternalTokenForStateRequiresForwardProgress is the
// compact equivalent of production's B1/B3 termination-proof witness
// (TestRelexZeroWidthExternalTokenRequiresForwardProgress): a post-shift
// state with no action for the shared token must not be admitted, since a
// rescue that could commit there and starve again has no proof it ever
// terminates.
func TestRelexZeroWidthExternalTokenForStateRequiresForwardProgress(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ParseTable[3][1] = 0 // state 3 no longer has an action for `number`
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)
	scheduler.zeroWidthRelexBudget = maxConsecutiveZeroWidthTokens
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex

	if _, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok); ok {
		t.Fatal("rescue admitted a shift into a state with no forward progress on the shared token")
	}
}

// TestRelexZeroWidthExternalTokenForStateRequiresSingleShiftAction is the
// compact equivalent of production's TestRelexZeroWidthExternalTokenRequiresSingleShiftAction:
// a conflicted or non-shift action cell for the probed symbol is out of this
// rescue's scope.
func TestRelexZeroWidthExternalTokenForStateRequiresSingleShiftAction(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	lang.ParseActions[1].Actions = []ParseAction{
		{Type: ParseActionShift, State: 3},
		{Type: ParseActionReduce, Symbol: 1, ChildCount: 0},
	}
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)
	scheduler.zeroWidthRelexBudget = maxConsecutiveZeroWidthTokens
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex

	if _, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok); ok {
		t.Fatal("rescue admitted a shift from a conflicted action cell")
	}
}

// TestRelexZeroWidthExternalTokenForStateRespectsBudget is the compact
// equivalent of production's TestRelexZeroWidthExternalTokenRespectsRescueBudget:
// an exhausted per-election budget declines even though every other guard
// still holds.
func TestRelexZeroWidthExternalTokenForStateRespectsBudget(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)
	scheduler.zeroWidthRelexBudget = 0
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex

	if _, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok); ok {
		t.Fatal("rescue admitted a token with an exhausted budget")
	}
}

// TestRelexZeroWidthExternalTokenForStateBudgetResetsPerElection proves the
// lazy per-election reset: a budget exhausted under one electionIndex is
// restored to the full allowance once electionIndex advances, mirroring
// zeroWidthRescueBudget's own per-shared-token reset in production's
// dispatch loop (parser.go).
func TestRelexZeroWidthExternalTokenForStateBudgetResetsPerElection(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)
	scheduler.zeroWidthRelexBudget = 0
	scheduler.zeroWidthRelexBudgetElection = scheduler.electionIndex
	scheduler.electionIndex++

	got, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok)
	if !ok {
		t.Fatal("rescue declined after its budget's owning election advanced")
	}
	if got.Symbol != 2 {
		t.Fatalf("admitted token = %+v, want symbol 2", got)
	}
	if scheduler.zeroWidthRelexBudget != maxConsecutiveZeroWidthTokens-1 {
		t.Fatalf("zeroWidthRelexBudget = %d, want %d after the reset and one admission", scheduler.zeroWidthRelexBudget, maxConsecutiveZeroWidthTokens-1)
	}
}

// TestRelexZeroWidthExternalTokenForStateBudgetIsNotDeadOnElectionZero is the
// regression test for the sentinel defect in zeroWidthRelexBudgetElection:
// electionIndex reaches 0 after the very first elect() call
// (parsercore_phase0_driver.go), the same value zeroWidthRelexBudgetElection
// itself defaults to. Without initializeDiagnosticParserCoreGenericScheduler
// setting zeroWidthRelexBudgetElection to -1 (mirroring electionIndex's own
// -1 start), the two zero values collide on a scheduler's first election:
// the lazy reset in relexZeroWidthExternalTokenForState sees
// zeroWidthRelexBudgetElection == electionIndex and believes the budget was
// already reset for this election, when it was never touched at all,
// leaving zeroWidthRelexBudget dead at its own zero value for the scheduler's
// entire first election.
func TestRelexZeroWidthExternalTokenForStateBudgetIsNotDeadOnElectionZero(t *testing.T) {
	lang := perlNonassocWitnessLanguage()
	scheduler, tok := newCompactZeroWidthRelexWitnessScheduler(t, lang, 1)
	// A freshly initialized scheduler (initializeDiagnosticParserCoreGenericScheduler)
	// reaches electionIndex 0 after its first elect() call, with
	// zeroWidthRelexBudget and zeroWidthRelexBudgetElection never touched by
	// this rescue before. newCompactZeroWidthRelexWitnessScheduler's own
	// struct literal leaves electionIndex at its own Go zero value (0),
	// matching that exact shape without any manual pre-seeding.
	if scheduler.electionIndex != 0 {
		t.Fatalf("test precondition: scheduler.electionIndex = %d, want 0 (unset)", scheduler.electionIndex)
	}
	scheduler.zeroWidthRelexBudgetElection = -1

	got, ok := scheduler.relexZeroWidthExternalTokenForState(1, tok)
	if !ok {
		t.Fatal("rescue declined on the scheduler's first election (electionIndex 0); the lazy budget reset did not fire")
	}
	if got.Symbol != 2 {
		t.Fatalf("admitted token = %+v, want symbol 2", got)
	}
	if scheduler.zeroWidthRelexBudget != maxConsecutiveZeroWidthTokens-1 {
		t.Fatalf("zeroWidthRelexBudget = %d, want %d after the first election's own reset and one admission", scheduler.zeroWidthRelexBudget, maxConsecutiveZeroWidthTokens-1)
	}
}

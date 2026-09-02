//go:build gts_parsercorephase0

package gotreesitter

import "testing"

type phase0OwnedExternalScanner struct {
	sameSymbol bool
	fail       bool
}

type phase0OwnedExternalScannerState struct {
	value byte
}

func (phase0OwnedExternalScanner) Create() any {
	return &phase0OwnedExternalScannerState{}
}

func (phase0OwnedExternalScanner) Destroy(any) {}

func (phase0OwnedExternalScanner) Serialize(payload any, buf []byte) int {
	if len(buf) == 0 {
		return 0
	}
	buf[0] = payload.(*phase0OwnedExternalScannerState).value
	return 1
}

func (phase0OwnedExternalScanner) Deserialize(payload any, buf []byte) {
	state := payload.(*phase0OwnedExternalScannerState)
	if len(buf) == 0 {
		state.value = 0
		return
	}
	state.value = buf[0]
}

func (phase0OwnedExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func (phase0OwnedExternalScanner) CheckpointIdentity() (ExternalScannerCheckpointIdentity, bool) {
	return ExternalScannerCheckpointIdentity{
		Scanner: []byte("phase0-owned-external-scanner-v1"),
		Grammar: []byte("phase0-owned-external-grammar-v1"),
	}, true
}

func (scanner phase0OwnedExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	if lexer.Lookahead() != 'x' {
		return false
	}
	state := payload.(*phase0OwnedExternalScannerState)
	if scanner.fail {
		state.value = 9
		return false
	}
	switch {
	case len(valid) > 0 && valid[0]:
		state.value = 1
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(Symbol(11))
		return true
	case len(valid) > 1 && valid[1]:
		state.value = 2
		lexer.Advance(false)
		lexer.MarkEnd()
		if scanner.sameSymbol {
			lexer.SetResultSymbol(Symbol(11))
		} else {
			lexer.SetResultSymbol(Symbol(12))
		}
		return true
	default:
		return false
	}
}

func phase0OwnedExternalLanguage() *Language {
	return phase0OwnedExternalLanguageWithScanner(phase0OwnedExternalScanner{})
}

func phase0OwnedExternalLanguageWithScanner(scanner phase0OwnedExternalScanner) *Language {
	return &Language{
		Name:               "phase0-owned-external",
		SymbolCount:        13,
		TokenCount:         13,
		ExternalTokenCount: 2,
		SymbolNames:        make([]string, 13),
		LexModes: []LexMode{
			{ExternalLexState: 1},
			{ExternalLexState: 2},
		},
		LexStates:       []LexState{{}},
		ExternalScanner: scanner,
		ExternalSymbols: []Symbol{11, 12},
		ExternalLexStates: [][]bool{
			{false, false},
			{true, false},
			{false, true},
		},
	}
}

func TestDiagnosticParserCoreExternalVersionRelexOwnsCheckpoint(t *testing.T) {
	lang := phase0OwnedExternalLanguage()
	source := []byte("x")
	lookup := func(StateID, Symbol) uint16 { return 1 }
	tokenSource := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, lookup, nil, nil, nil)
	defer tokenSource.Close()
	tokenSource.SetParserState(0)
	before := tokenSource.snapshotRelexState()
	shared := tokenSource.Next()
	if shared.Symbol != Symbol(11) || !shared.ExternalScannerToken || shared.StartByte != 0 || shared.EndByte != 1 {
		t.Fatalf("shared external token = %+v, want symbol 11 at 0..1", shared)
	}
	sharedState := tokenSource.externalPayload.(*phase0OwnedExternalScannerState).value
	if sharedState != 1 {
		t.Fatalf("shared scanner state = %d, want 1", sharedState)
	}

	scheduler := &diagnosticParserCoreGenericScheduler{
		tokenSource:             tokenSource,
		versionLexerBefore:      before,
		versionLexerBeforeValid: true,
	}
	candidate, ok := scheduler.relexTokenForState(1, shared)
	if !ok {
		t.Fatal("external per-version relex = false, want a divergent scanner token")
	}
	if candidate.Symbol != Symbol(12) || !candidate.ExternalScannerToken || candidate.StartByte != 0 || candidate.EndByte != 1 {
		t.Fatalf("candidate external token = %+v, want symbol 12 at 0..1", candidate)
	}
	if got := tokenSource.externalPayload.(*phase0OwnedExternalScannerState).value; got != sharedState {
		t.Fatalf("shared scanner payload after probe = %d, want restored %d", got, sharedState)
	}
	if got := tokenSource.lexer.pos; got != 1 {
		t.Fatalf("shared lexer position after probe = %d, want 1", got)
	}
}

func TestDiagnosticParserCoreExternalVersionRelexDetectsStateOnlyDivergence(t *testing.T) {
	lang := phase0OwnedExternalLanguageWithScanner(phase0OwnedExternalScanner{sameSymbol: true})
	source := []byte("x")
	lookup := func(StateID, Symbol) uint16 { return 1 }
	tokenSource := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, lookup, nil, nil, nil)
	defer tokenSource.Close()
	tokenSource.SetParserState(0)
	before := tokenSource.snapshotRelexState()
	shared := tokenSource.Next()
	if shared.Symbol != Symbol(11) || !shared.ExternalScannerToken || shared.StartByte != 0 || shared.EndByte != 1 {
		t.Fatalf("shared external token = %+v, want symbol 11 at 0..1", shared)
	}
	sharedState := tokenSource.externalPayload.(*phase0OwnedExternalScannerState).value
	candidate, ok := (&diagnosticParserCoreGenericScheduler{
		tokenSource:             tokenSource,
		versionLexerBefore:      before,
		versionLexerBeforeValid: true,
	}).relexTokenForState(1, shared)
	if !ok {
		t.Fatal("external per-version relex = false, want a scanner-state witness")
	}
	if candidate.Symbol != shared.Symbol || candidate.StartByte != shared.StartByte || candidate.EndByte != shared.EndByte {
		t.Fatalf("state-only candidate = %+v, want same symbol and span as shared %+v", candidate, shared)
	}
	if got := tokenSource.externalPayload.(*phase0OwnedExternalScannerState).value; got != sharedState {
		t.Fatalf("shared scanner payload after state-only probe = %d, want restored %d", got, sharedState)
	}
	if got := tokenSource.lexer.pos; got != 1 {
		t.Fatalf("shared lexer position after state-only probe = %d, want 1", got)
	}
}

func TestDiagnosticParserCoreExternalVersionRelexRestoresAfterFailedScan(t *testing.T) {
	lang := phase0OwnedExternalLanguage()
	source := []byte("x")
	lookup := func(StateID, Symbol) uint16 { return 1 }
	tokenSource := newDFATokenSourceDirect(NewLexer(lang.LexStates, source), lang, lookup, nil, nil, nil)
	defer tokenSource.Close()
	tokenSource.SetParserState(0)
	before := tokenSource.snapshotRelexState()
	shared := tokenSource.Next()
	if shared.Symbol != Symbol(11) {
		t.Fatalf("successful shared token = %+v, want symbol 11", shared)
	}
	lang.ExternalScanner = phase0OwnedExternalScanner{fail: true}
	// Keep the token source's payload and switch only the scanner behavior. The
	// failing implementation mutates the payload before it returns false.
	sharedState := tokenSource.externalPayload.(*phase0OwnedExternalScannerState).value
	candidate, ok := (&diagnosticParserCoreGenericScheduler{
		tokenSource:             tokenSource,
		versionLexerBefore:      before,
		versionLexerBeforeValid: true,
	}).relexTokenForState(1, shared)
	if ok || candidate != shared {
		t.Fatalf("failed external probe = %+v/%t, want shared token/false", candidate, ok)
	}
	if got := tokenSource.externalPayload.(*phase0OwnedExternalScannerState).value; got != sharedState {
		t.Fatalf("shared scanner payload after failed probe = %d, want restored %d", got, sharedState)
	}
	if got := tokenSource.lexer.pos; got != 1 {
		t.Fatalf("shared lexer position after failed probe = %d, want 1", got)
	}
}

func TestDiagnosticParserCoreSameSpanRelexReconstructsExactToken(t *testing.T) {
	shared := Token{
		Symbol:                   3,
		Text:                     "token",
		StartByte:                7,
		EndByte:                  12,
		StartPoint:               Point{Row: 2, Column: 4},
		EndPoint:                 Point{Row: 2, Column: 9},
		ExternalScannerToken:     true,
		ExternalScannerStartByte: 5,
	}
	relexed := shared
	relexed.Symbol = 8
	relexed.ExternalScannerToken = false
	relexed.ExternalScannerStartByte = 0

	verified, ok := diagnosticParserCoreSameSpanRelex(shared, relexed)
	if !ok || verified != relexed {
		t.Fatalf("verified token = %+v/%t, want %+v/true", verified, ok, relexed)
	}
	cell := diagnosticParserCoreGenericCell{relexedSymbol: verified.Symbol}
	if got := cell.dispatchToken(shared); got != relexed {
		t.Fatalf("dispatched token = %+v, want %+v", got, relexed)
	}
}

// TestDispatchTokenClearsIsKeywordOnRelexedSymbolOverride pins finding F7:
// when relexedSymbol overrides the shared token's symbol, dispatchToken must
// clear isKeyword alongside the external-scanner fields. isKeyword records
// that the ORIGINAL symbol was reached through the keyword-adoption
// promotion path; that fact does not carry over to an unrelated relexed
// symbol replacing it, so a dispatch view built from a promoted keyword
// token must not still claim the relexed token is a keyword adoption.
func TestDispatchTokenClearsIsKeywordOnRelexedSymbolOverride(t *testing.T) {
	shared := Token{
		Symbol:                   2,
		Text:                     "if",
		StartByte:                0,
		EndByte:                  2,
		EndPoint:                 Point{Column: 2},
		ExternalScannerToken:     true,
		ExternalScannerStartByte: 1,
		isKeyword:                true,
	}
	cell := diagnosticParserCoreGenericCell{relexedSymbol: 9}

	got := cell.dispatchToken(shared)
	if got.isKeyword {
		t.Fatal("dispatchToken with a relexedSymbol override: isKeyword = true, want false")
	}
	if got.Symbol != 9 {
		t.Fatalf("dispatchToken symbol = %d, want 9 (the relexed symbol)", got.Symbol)
	}
	if got.ExternalScannerToken || got.ExternalScannerStartByte != 0 {
		t.Fatalf("dispatchToken external-scanner fields = %v/%d, want false/0",
			got.ExternalScannerToken, got.ExternalScannerStartByte)
	}
}

// TestDispatchTokenPreservesIsKeywordWithoutRelexedSymbolOverride pins the
// other half of finding F7: dispatchToken must leave isKeyword untouched
// when there is no relexedSymbol override at all (cell.relexedSymbol == 0),
// so the clear above is scoped to the override branch only.
func TestDispatchTokenPreservesIsKeywordWithoutRelexedSymbolOverride(t *testing.T) {
	shared := Token{Symbol: 2, isKeyword: true}
	cell := diagnosticParserCoreGenericCell{}

	got := cell.dispatchToken(shared)
	if !got.isKeyword {
		t.Fatal("dispatchToken without a relexedSymbol override: isKeyword = false, want true (unmodified)")
	}
	if got != shared {
		t.Fatalf("dispatchToken without an override = %+v, want unmodified %+v", got, shared)
	}
}

func TestDiagnosticParserCoreSameSpanRelexRejectsTokenFieldChanges(t *testing.T) {
	shared := Token{
		Symbol:                   3,
		Text:                     "token",
		StartByte:                7,
		EndByte:                  12,
		StartPoint:               Point{Row: 2, Column: 4},
		EndPoint:                 Point{Row: 2, Column: 9},
		ExternalScannerToken:     true,
		ExternalScannerStartByte: 5,
	}
	exact := shared
	exact.Symbol = 8
	exact.ExternalScannerToken = false
	exact.ExternalScannerStartByte = 0

	tests := []struct {
		name   string
		change func(*Token)
	}{
		{name: "zero symbol", change: func(token *Token) { token.Symbol = 0 }},
		{name: "shared symbol", change: func(token *Token) { token.Symbol = shared.Symbol }},
		{name: "text", change: func(token *Token) { token.Text = "other" }},
		{name: "start byte", change: func(token *Token) { token.StartByte++ }},
		{name: "end byte", change: func(token *Token) { token.EndByte++ }},
		{name: "start row", change: func(token *Token) { token.StartPoint.Row++ }},
		{name: "start column", change: func(token *Token) { token.StartPoint.Column++ }},
		{name: "end row", change: func(token *Token) { token.EndPoint.Row++ }},
		{name: "end column", change: func(token *Token) { token.EndPoint.Column++ }},
		{name: "missing", change: func(token *Token) { token.Missing = true }},
		{name: "no lookahead", change: func(token *Token) { token.NoLookahead = true }},
		{name: "external token", change: func(token *Token) { token.ExternalScannerToken = true }},
		{name: "external start", change: func(token *Token) { token.ExternalScannerStartByte = 5 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relexed := exact
			test.change(&relexed)
			if verified, ok := diagnosticParserCoreSameSpanRelex(shared, relexed); ok || verified.Symbol != 0 {
				t.Fatalf("verified token = %+v/%t, want zero/false; token=%+v", verified, ok, relexed)
			}
		})
	}
}

// DiagnosticParserCoreSameSpanRelexForTest exposes diagnosticParserCoreSameSpanRelex
// directly so the external test package can pin the same-span verifier's
// own contract (F3) against a real, per-header relex a real grammar's raw
// DFA produced (via RelexTokenForStateForTest), not just synthetic
// hand-built tokens.
func DiagnosticParserCoreSameSpanRelexForTest(shared, relexed Token) (Token, bool) {
	return diagnosticParserCoreSameSpanRelex(shared, relexed)
}

// RelexTokenForStateForTest exposes relexTokenForState directly so the
// external test package can pin the probe's own contract (D2-1 Phase 1
// item 2) against a real witness, independent of dispatchPassActive's own
// caller-side ragged-end decline (covered separately by
// RunStateDependentRelexSchedulerForTest). checkpointLength only needs to be
// non-zero for the internal-DFA fallback. The external-scanner path requires
// a real election snapshot and an identity-bearing checkpointed scanner;
// direct tests that omit that state remain on the legacy internal-DFA probe.
func RelexTokenForStateForTest(
	lang *Language, source []byte, checkpointLength int,
	options DiagnosticParserCorePrefixOptions, state StateID, tok Token,
) (Token, bool) {
	parser := NewParser(lang)
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return Token{}, false
	}
	defer tokenSource.Close()
	scheduler := &diagnosticParserCoreGenericScheduler{
		tokenSource: tokenSource,
		checkpoint:  DiagnosticParserCoreScannerCheckpoint{Length: checkpointLength},
		options:     options,
	}
	return scheduler.relexTokenForState(state, tok)
}

func RunStateDependentRelexSchedulerForTest(lang *Language, source []byte) (DiagnosticParserCoreGenericScheduler, error) {
	return runStateDependentRelexSchedulerForTest(lang, source, false)
}

// StateDependentRecoveryProvenanceForTest records the recovery lifecycle that
// produced one forced error-mode scheduler receipt.
type StateDependentRecoveryProvenanceForTest struct {
	ResumeState        StateID
	ResumeSymbol       Symbol
	ResumeCount        uint32
	MissingInsertions  uint32
	LineageSelections  uint64
	LineageRetirements uint64
	SelectedAbsorb     bool
	NoActionDrops      uint64
}

// RunStateDependentRecoveryRelexProvenanceForTest runs the forced error-mode
// compact attempt that follows a certified plain-first decline.
func RunStateDependentRecoveryRelexProvenanceForTest(lang *Language, source []byte) (DiagnosticParserCoreGenericScheduler, StateDependentRecoveryProvenanceForTest, error) {
	return runStateDependentRecoveryRelexSchedulerForTest(lang, source)
}

func ErrorModeRelexForTest(lang *Language, source []byte, startByte uint32) (Token, bool) {
	parser := NewParser(lang)
	tokenSource := parser.acquireParserDFATokenSourceWithErrorRuns(source, true)
	if tokenSource == nil {
		return Token{}, false
	}
	defer tokenSource.Close()
	scheduler := diagnosticParserCoreGenericScheduler{
		tokenSource: tokenSource,
		options: DiagnosticParserCorePrefixOptions{
			allowCompactRecoveryErrorModeKeywordCapture: lang.CompactRecoveryErrorModeKeywordCaptureCertified,
		},
	}
	return scheduler.s3ErrorModeRelex(startByte)
}

func runStateDependentRecoveryRelexSchedulerForTest(lang *Language, source []byte) (DiagnosticParserCoreGenericScheduler, StateDependentRecoveryProvenanceForTest, error) {
	parser := NewParser(lang)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		return DiagnosticParserCoreGenericScheduler{}, StateDependentRecoveryProvenanceForTest{}, err
	}
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	// executeSchedulerOpen normally binds this context. This test seam calls
	// the scheduler directly, so bind the same production materialization and
	// recovery-pricing inputs here.
	runner.options.materializationParser = parser
	runner.options.materializationSource = source
	runner.options.materializationForceReplayParseStates = true
	runner.options.materializationContextSet = true
	tokenSource := parser.acquireParserDFATokenSourceWithErrorRuns(source, true)
	if tokenSource == nil {
		return DiagnosticParserCoreGenericScheduler{}, StateDependentRecoveryProvenanceForTest{}, nil
	}
	defer tokenSource.Close()
	scheduler, runErr := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		runner.compact,
		tokenSource,
		&runner.scannerScratch,
		lang.InitialState,
		runner.options,
		diagnosticParserCoreSeedObserver{},
	)
	if scheduler == nil || scheduler.receipt == nil {
		return DiagnosticParserCoreGenericScheduler{}, StateDependentRecoveryProvenanceForTest{}, runErr
	}
	provenance := StateDependentRecoveryProvenanceForTest{
		ResumeState:        scheduler.s3ResumeState,
		ResumeSymbol:       scheduler.s3ResumeSymbol,
		ResumeCount:        scheduler.s3ResumeCount,
		MissingInsertions:  scheduler.s5MissingInsertions,
		LineageSelections:  scheduler.work.RecoveryLineageSelections,
		LineageRetirements: scheduler.work.RecoveryLineageRetirements,
		SelectedAbsorb:     scheduler.selectedRecoveryAbsorbLineage,
		NoActionDrops:      scheduler.work.NoActionDrops,
	}
	return *scheduler.receipt, provenance, runErr
}

// RunStateDependentRelexSchedulerWithSpanUnlockedRelexDisabledForTest is
// RunStateDependentRelexSchedulerForTest with
// DisablePerHeaderSpanUnlockedRelex forced on, for the D2-1 gate test
// (Phase 1 item 5: "when disabled, restore the span-locked behavior
// exactly").
func RunStateDependentRelexSchedulerWithSpanUnlockedRelexDisabledForTest(lang *Language, source []byte) (DiagnosticParserCoreGenericScheduler, error) {
	return runStateDependentRelexSchedulerForTest(lang, source, true)
}

func runStateDependentRelexSchedulerForTest(lang *Language, source []byte, disablePerHeaderSpanUnlockedRelex bool) (DiagnosticParserCoreGenericScheduler, error) {
	parser := NewParser(lang)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		return DiagnosticParserCoreGenericScheduler{}, err
	}
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	runner.options.DisablePerHeaderSpanUnlockedRelex = disablePerHeaderSpanUnlockedRelex
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return DiagnosticParserCoreGenericScheduler{}, nil
	}
	defer tokenSource.Close()
	scheduler, runErr := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		runner.compact,
		tokenSource,
		&runner.scannerScratch,
		lang.InitialState,
		runner.options,
		diagnosticParserCoreSeedObserver{},
	)
	if scheduler == nil || scheduler.receipt == nil {
		return DiagnosticParserCoreGenericScheduler{}, runErr
	}
	return *scheduler.receipt, runErr
}

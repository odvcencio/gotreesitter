//go:build gts_parsercorephase0

package gotreesitter

import "testing"

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
// RunStateDependentRelexSchedulerForTest). checkpointLength only needs to
// be non-zero for a language with an external scanner: the probe reads
// nothing else from the checkpoint (Lexer.scan only needs DFA fields), so
// any non-zero value satisfies the "this header owns checkpoint identity"
// guard without a real serialized scanner snapshot.
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
	return runStateDependentRelexSchedulerForTest(lang, source, false, false)
}

// RunStateDependentRelexSchedulerWithSpanUnlockedRelexDisabledForTest is
// RunStateDependentRelexSchedulerForTest with
// DisablePerHeaderSpanUnlockedRelex forced on, for the D2-1 gate test
// (Phase 1 item 5: "when disabled, restore the span-locked behavior
// exactly").
func RunStateDependentRelexSchedulerWithSpanUnlockedRelexDisabledForTest(lang *Language, source []byte) (DiagnosticParserCoreGenericScheduler, error) {
	return runStateDependentRelexSchedulerForTest(lang, source, true, false)
}

// RunStateDependentRelexSchedulerWithUnanimousRelexAdoptionDisabledForTest is
// RunStateDependentRelexSchedulerForTest with DisableUnanimousRelexAdoption
// forced on, for the D2-2a negative-arm test: restoring the D2-1 ragged-end
// decline exactly in place of adoption.
func RunStateDependentRelexSchedulerWithUnanimousRelexAdoptionDisabledForTest(lang *Language, source []byte) (DiagnosticParserCoreGenericScheduler, error) {
	return runStateDependentRelexSchedulerForTest(lang, source, false, true)
}

// TestD2AdoptionEligibleGuards pins each D2-2a unanimous-relex-adoption
// guard (G1-G5, diagnosticParserCoreUnanimousRelexAdoptionEligible's own doc
// comment) directly, against hand-built candidates. The baseline case is
// eligible; every other case breaks exactly one guard and must decline.
// G4 and G5 each cover two independent checks, so each has two arms: a
// mutation deleting either check alone still has a dedicated arm to catch
// it.
func TestD2AdoptionEligibleGuards(t *testing.T) {
	validToken := Token{Symbol: 9, StartByte: 3, EndByte: 4}
	disagreeingToken := Token{Symbol: 11, StartByte: 3, EndByte: 4}
	externalToken := Token{Symbol: 9, StartByte: 3, EndByte: 4, ExternalScannerToken: true}
	missingToken := Token{Symbol: 9, StartByte: 3, EndByte: 4, Missing: true}
	noLookaheadToken := Token{Symbol: 9, StartByte: 3, EndByte: 4, NoLookahead: true}

	tests := []struct {
		name           string
		candidates     []diagnosticParserCoreUnanimousRelexAdoptionCandidate
		raggedCount    int
		noActionCount  int
		headerCount    int
		checkpointSame bool
		wantEligible   bool
	}{
		{
			name: "baseline is eligible",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: validToken, hasAction: true},
				{token: validToken, hasAction: true},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 2, checkpointSame: true,
			wantEligible: true,
		},
		{
			name: "G1 ragged count does not cover every no-action head",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: validToken, hasAction: true},
				{token: validToken, hasAction: true},
			},
			raggedCount: 1, noActionCount: 2, headerCount: 2, checkpointSame: true,
			wantEligible: false,
		},
		{
			name: "G1 no-action heads do not cover every header",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: validToken, hasAction: true},
				{token: validToken, hasAction: true},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 3, checkpointSame: true,
			wantEligible: false,
		},
		{
			name: "G2 candidates disagree on the relexed token",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: validToken, hasAction: true},
				{token: disagreeingToken, hasAction: true},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 2, checkpointSame: true,
			wantEligible: false,
		},
		{
			name: "G3 one candidate has no live action",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: validToken, hasAction: true},
				{token: validToken, hasAction: false},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 2, checkpointSame: true,
			wantEligible: false,
		},
		{
			name: "G4 scanner checkpoint changed",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: validToken, hasAction: true},
				{token: validToken, hasAction: true},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 2, checkpointSame: false,
			wantEligible: false,
		},
		{
			name: "G4 relexed token is an external-scanner token",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: externalToken, hasAction: true},
				{token: externalToken, hasAction: true},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 2, checkpointSame: true,
			wantEligible: false,
		},
		{
			name: "G5 relexed token is missing",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: missingToken, hasAction: true},
				{token: missingToken, hasAction: true},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 2, checkpointSame: true,
			wantEligible: false,
		},
		{
			name: "G5 relexed token has no lookahead",
			candidates: []diagnosticParserCoreUnanimousRelexAdoptionCandidate{
				{token: noLookaheadToken, hasAction: true},
				{token: noLookaheadToken, hasAction: true},
			},
			raggedCount: 2, noActionCount: 2, headerCount: 2, checkpointSame: true,
			wantEligible: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, eligible := diagnosticParserCoreUnanimousRelexAdoptionEligible(
				test.candidates, test.raggedCount, test.noActionCount, test.headerCount, test.checkpointSame,
			)
			if eligible != test.wantEligible {
				t.Fatalf("eligible = %v, want %v (token=%+v)", eligible, test.wantEligible, token)
			}
			if eligible && token != test.candidates[0].token {
				t.Fatalf("adopted token = %+v, want the candidates' own shared token %+v", token, test.candidates[0].token)
			}
		})
	}
}

func runStateDependentRelexSchedulerForTest(
	lang *Language, source []byte, disablePerHeaderSpanUnlockedRelex, disableUnanimousRelexAdoption bool,
) (DiagnosticParserCoreGenericScheduler, error) {
	parser := NewParser(lang)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		return DiagnosticParserCoreGenericScheduler{}, err
	}
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	runner.options.DisablePerHeaderSpanUnlockedRelex = disablePerHeaderSpanUnlockedRelex
	runner.options.DisableUnanimousRelexAdoption = disableUnanimousRelexAdoption
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

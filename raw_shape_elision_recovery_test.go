package gotreesitter

import "testing"

// buildArithmeticRecoveryGarbageLanguage is a small, C-recovery-eligible
// grammar (ts2go convention: InitialState=1, nonterminal GOTO columns hold
// raw state IDs, CertifyCRecoveryCostCompetition run at construction) used
// to validate the recovery elision fix (see cHandleError's everForked latch
// in parser_recover_c.go). It parses "NUMBER (+ NUMBER)*" and additionally
// lexes a garbage "#" token (symbol HASH) that has NO parse action anywhere
// in the table, including the initial state — so no opportunistic/legal
// top-level resync heuristic can short-circuit the error, and the faithful
// C-recovery port (cHandleError) is what actually handles it.
//
//	expression -> NUMBER                    (production 0, 1 child)
//	expression -> expression + NUMBER        (production 1, 3 children)
//
// Feeding "1 #" shifts and reduces NUMBER -> expression deterministically
// while singleStackMode is true (a genuine single-stack prefix, a candidate
// for elision), then hits a true no-action dead end on "#" that only
// cHandleError's version-spawning recovery machinery resolves.
func buildArithmeticRecoveryGarbageLanguage() *Language {
	lang := &Language{
		Name:               "arithmetic_recovery_garbage",
		SymbolCount:        5,
		TokenCount:         4,
		ExternalTokenCount: 0,
		StateCount:         6,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  2,
		InitialState:       1,

		// Terminals first (ts2go convention: TokenCount counts leading
		// terminal symbols; nonterminal GOTO columns hold raw state IDs).
		SymbolNames: []string{"EOF", "NUMBER", "+", "HASH", "expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "NUMBER", Visible: true, Named: true},
			{Name: "+", Visible: true, Named: false},
			{Name: "HASH", Visible: true, Named: true},
			{Name: "expression", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		ParseActions: []ParseActionEntry{
			{Actions: nil}, // 0
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},                                   // 1: S1 col NUMBER
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 0}}}, // 2: S2 reduce expr->NUMBER
			{Actions: []ParseAction{{Type: ParseActionAccept}}},                                            // 3: S3 col EOF
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},                                   // 4: S3 col +
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},                                   // 5: S4 col NUMBER
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 3, ProductionID: 1}}}, // 6: S5 reduce expr->expr+NUMBER
		},

		// Columns: EOF(0), NUMBER(1), +(2), HASH(3), expression(4, GOTO=raw state)
		ParseTable: [][]uint16{
			{0, 0, 0, 0, 0}, // state 0: unused placeholder
			{0, 1, 0, 0, 3}, // state 1: start; NUMBER->shift(idx1); expression->goto state 3
			{2, 2, 2, 2, 0}, // state 2: reduce regardless of lookahead
			{3, 0, 4, 0, 0}, // state 3: accept at EOF, shift + ; NO action for HASH (real no-action dead end)
			{0, 5, 0, 0, 0}, // state 4
			{6, 6, 6, 6, 0}, // state 5
		},

		LexModes: []LexMode{
			{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0},
		},

		LexStates: []LexState{
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
					{Lo: '+', Hi: '+', NextState: 2},
					{Lo: '#', Hi: '#', NextState: 4},
					{Lo: ' ', Hi: ' ', NextState: 3},
				},
			},
			{AcceptToken: 1, Skip: false, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '0', Hi: '9', NextState: 1}}},
			{AcceptToken: 2, Skip: false, Default: -1, EOF: -1},
			{AcceptToken: 0, Skip: true, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ' ', Hi: ' ', NextState: 3}}},
			{AcceptToken: 3, Skip: false, Default: -1, EOF: -1},
		},
	}
	CertifyCRecoveryCostCompetition(lang)
	return lang
}

// TestArithmeticRecoveryGarbageLanguageActuallyEntersCRecovery instruments
// that "1 #" genuinely reaches the faithful C-recovery port (cHandleError),
// not some earlier heuristic (opportunistic top-level resync, plain
// absorb-into-error-leaf, etc.) that would make the recovery differential
// below pass vacuously.
func TestArithmeticRecoveryGarbageLanguageActuallyEntersCRecovery(t *testing.T) {
	lang := buildArithmeticRecoveryGarbageLanguage()
	if !lang.CRecoveryCostCompetitionCapable || !lang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatalf("language is not C-recovery eligible: capable=%v enabledByDefault=%v (DiagnoseCRecoveryGate=%+v)",
			lang.CRecoveryCostCompetitionCapable, lang.CRecoveryCostCompetitionEnabledByDefault, DiagnoseCRecoveryGate(lang))
	}
	parser := NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree := mustParse(t, parser, []byte("1 #"))
	defer tree.Release()

	rt := tree.ParseRuntime()
	if !rt.CRecoveryEnteredErrorState {
		t.Fatal("CRecoveryEnteredErrorState = false, want true (test input must actually reach cHandleError)")
	}
	if !tree.RootNode().HasError() {
		t.Fatal("HasError() = false, want true (garbage HASH token must surface as an error)")
	}
}

// TestRawShapeElisionDifferentialRecoveryFromSingleStackPrefix is the
// recovery-path differential that validates the cHandleError everForked
// latch (parser_recover_c.go): a single-stack prefix ("1" shifts and reduces
// to "expression" deterministically, a candidate for elision) followed by a
// malformed "#" that drives the parse into the faithful C-recovery port,
// including transient recovery versions, counted before dispatch completes. The
// selected tree must be identical whether the elision gate is on or forced
// off.
func TestRawShapeElisionDifferentialRecoveryFromSingleStackPrefix(t *testing.T) {
	lang := buildArithmeticRecoveryGarbageLanguage()
	source := []byte("1 #")
	telemetryBefore := recoveryRuntimeTelemetryEnabled
	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(telemetryBefore) })
	parserOn, parserOff := NewParser(lang), NewParser(lang)
	parserOn.SetAdmissionCandidateRoute(false)
	parserOff.SetAdmissionCandidateRoute(false)

	SetRawShapeElisionDisabledForDiagnostics(false)
	gateOn := mustParse(t, parserOn, source)
	defer gateOn.Release()
	gateOnRuntime := gateOn.ParseRuntime()

	SetRawShapeElisionDisabledForDiagnostics(true)
	defer SetRawShapeElisionDisabledForDiagnostics(false)
	gateOff := mustParse(t, parserOff, source)
	defer gateOff.Release()
	gateOffRuntime := gateOff.ParseRuntime()

	if !gateOnRuntime.CRecoveryEnteredErrorState || !gateOffRuntime.CRecoveryEnteredErrorState {
		t.Fatalf("CRecoveryEnteredErrorState gate-on=%v gate-off=%v, want both true (recovery precondition)",
			gateOnRuntime.CRecoveryEnteredErrorState, gateOffRuntime.CRecoveryEnteredErrorState)
	}
	// Outer-loop stack counts can miss forks resolved within one dispatch.
	for name, parser := range map[string]*Parser{"gate-on": parserOn, "gate-off": parserOff} {
		stats := parser.DebugRecoveryRuntimeStats()
		if stats.RecoveryEntryCount == 0 || stats.PeakLiveVersionCount <= 1 {
			t.Fatalf("%s recovery entries=%d peak live versions=%d, want recovery with competing versions",
				name, stats.RecoveryEntryCount, stats.PeakLiveVersionCount)
		}
	}
	if got, want := gateOn.RootNode().SExpr(lang), gateOff.RootNode().SExpr(lang); got != want {
		t.Fatalf("gate-on tree = %s, gate-off (elision disabled) tree = %s, want identical", got, want)
	}
}

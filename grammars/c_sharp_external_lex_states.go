//go:build !grammar_subset || grammar_subset_c_sharp

// c_sharp precise ExternalLexStates — DEFAULT since the 2026-07 cliff
// campaign (previously staged behind -tags csharp_precise_els).
//
// The embedded c_sharp grammar blob preserves LexModes[state].ExternalLexState
// (histogram matches C exactly: 10 distinct states) but shipped an EMPTY
// ExternalLexStates validity table (ts2go dropped it). With the table empty,
// dfaTokenSource (parser_dfa_token_source.go) cannot use its precise per-state
// external path and instead unions externalValidMaskByState across all active
// GLR stacks. Under multi-stack pressure that union over-approximates
// external-token validity and corrupts the STATEFUL interpolated-string
// scanner (grammars/csharp_scanner.go): at an interpolation hole's close it
// reported external-lex-state 2 (expression start) instead of 5 (string
// content), the scanner's interpolation stack never popped (measured: 27
// pushes, 0 pops), and pass-1 on DeclaredTypeManager.cs died no_stacks_alive
// at byte 31888 behind a ~1,000-pass retry cascade.
//
// The table below is copied VERBATIM from tree-sitter-c-sharp src/parser.c
// (ts_external_scanner_states[10][12], pinned commit in languages.lock).
// Registering it (a) restores the precise per-state external path, and
// (b) satisfies the C-recovery election gate (DiagnoseCRecoveryGate), so
// c_sharp runs the faithful C error-recovery cost competition by default.
//
// The two engine blockers that kept this staged were fixed in the same
// campaign:
//   - parser_recover_c.go cCondenseAndResume: the C MAX_VERSION_COUNT window
//     now bounds DISTINCT merge keys (C merges same-key versions via
//     ts_stack_merge before its cap; the old raw positional trim dropped
//     whole grammar interpretations).
//   - parser.go usesPrimaryExternalScannerStateForGLR: the c_sharp
//     primary-stack-only pin now applies only while ExternalLexStates is
//     empty; with the precise table the scored per-row path sees all stack
//     states, so interpolation-hole stacks receive interpolation_close_brace.
//
// Validation: grammars/csharp_external_lex_states_regression_test.go.
package grammars

// cSharpExternalLexStates mirrors tree-sitter-c-sharp src/parser.c
// ts_external_scanner_states[10][12]. Columns are the C# external token indices,
// in the language's ExternalSymbols order:
//
//	 0 _optional_semi              6 interpolation_open_brace
//	 1 interpolation_regular_start 7 interpolation_close_brace
//	 2 interpolation_verbatim_start 8 interpolation_string_content
//	 3 interpolation_raw_start     9 raw_string_start
//	 4 interpolation_start_quote  10 raw_string_end
//	 5 interpolation_end_quote    11 raw_string_content
var cSharpExternalLexStates = [][]bool{
	/*  0 */ {false, false, false, false, false, false, false, false, false, false, false, false},
	/*  1 */ {true, true, true, true, true, true, true, true, true, true, true, true},
	/*  2 */ {false, true, true, true, false, false, false, false, false, true, false, false},
	/*  3 */ {false, true, true, true, false, false, false, true, false, true, false, false},
	/*  4 */ {false, false, false, false, false, false, false, true, false, false, false, false},
	/*  5 */ {false, false, false, false, false, true, true, false, true, false, false, false},
	/*  6 */ {false, false, false, false, false, false, false, false, false, false, true, false},
	/*  7 */ {true, false, false, false, false, false, false, false, false, false, false, false},
	/*  8 */ {false, false, false, false, true, false, false, false, false, false, false, false},
	/*  9 */ {false, false, false, false, false, false, false, false, false, false, false, true},
}

func init() {
	RegisterExternalLexStates("c_sharp", cSharpExternalLexStates)
}

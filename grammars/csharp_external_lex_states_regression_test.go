//go:build !grammar_subset || grammar_subset_c_sharp

package grammars

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestCSharpExternalLexStatesRegression guards the c_sharp ExternalLexStates
// table. The embedded c_sharp grammar blob preserves LexModes[state].ExternalLexState
// but ships an EMPTY ExternalLexStates validity table. Without the table, the GLR
// token source falls back to a union-of-active-stacks external-validity mask, which
// over-approximates valid external tokens and corrupts the stateful interpolated-
// string external scanner (the interpolation stack never pops, so pass-1 diverges
// at an interpolated string and ends no_stacks_alive, driving a ~1000x retry
// cascade). The sidecar (grammars/runtime/c_sharp_external_lex_states_gen.go)
// restores the precise table extracted verbatim from tree-sitter-c-sharp
// src/parser.c (ts_external_scanner_states) at the commit pinned in
// grammars/languages.lock. Regenerate the sidecar with:
//
//	go run ./cmd/ts2go -lexstates-only -only c_sharp \
//	  -manifest grammars/languages.lock -outdir grammars/runtime \
//	  -package grammarruntime
//
// The precise table also gates the c_sharp C-recovery election, so a missing or
// stale sidecar silently drops the faithful C error-recovery cost competition.
//
// The 9150f7d56bb4 refresh appended the _lambda_paren_open external, so the
// table grew from 10 rows by 12 columns to 12 rows by 13 columns.
func TestCSharpExternalLexStatesRegression(t *testing.T) {
	lang := CSharpLanguage()

	// External token indices, matching lang.ExternalSymbols order (== C enum order):
	//   0 _optional_semi                7 interpolation_close_brace
	//   1 interpolation_regular_start   8 interpolation_string_content
	//   2 interpolation_verbatim_start  9 raw_string_start
	//   3 interpolation_raw_start      10 raw_string_end
	//   4 interpolation_start_quote    11 raw_string_content
	//   5 interpolation_end_quote      12 _lambda_paren_open
	//   6 interpolation_open_brace
	want := [][]bool{
		/*  0 */ {false, false, false, false, false, false, false, false, false, false, false, false, false},
		/*  1 */ {true, true, true, true, true, true, true, true, true, true, true, true, true},
		/*  2 */ {false, true, true, true, false, false, false, false, false, true, false, false, true},
		/*  3 */ {false, true, true, true, false, false, false, true, false, true, false, false, true},
		/*  4 */ {false, false, false, false, false, false, false, false, false, false, false, false, true},
		/*  5 */ {false, false, false, false, false, false, false, true, false, false, false, false, true},
		/*  6 */ {false, false, false, false, false, false, false, true, false, false, false, false, false},
		/*  7 */ {false, false, false, false, false, true, true, false, true, false, false, false, false},
		/*  8 */ {false, false, false, false, true, false, false, false, false, false, false, false, false},
		/*  9 */ {false, false, false, false, false, false, false, false, false, false, false, true, false},
		/* 10 */ {true, false, false, false, false, false, false, false, false, false, false, false, false},
		/* 11 */ {false, false, false, false, false, false, false, false, false, false, true, false, false},
	}

	if got := len(lang.ExternalLexStates); got != len(want) {
		t.Fatalf("c_sharp ExternalLexStates rows = %d, want %d (missing sidecar => union-mask fallback corrupts interpolation scanner)", got, len(want))
	}
	for i, wantRow := range want {
		gotRow := lang.ExternalLexStates[i]
		if len(gotRow) != len(wantRow) {
			t.Fatalf("ExternalLexStates[%d] len = %d, want %d", i, len(gotRow), len(wantRow))
		}
		for j := range wantRow {
			if gotRow[j] != wantRow[j] {
				t.Fatalf("ExternalLexStates[%d][%d] = %v, want %v (row must match tree-sitter-c-sharp ts_external_scanner_states)", i, j, gotRow[j], wantRow[j])
			}
		}
	}

	// LexModes must still reference the table (external_lex_state indices 0..11).
	maxELS := 0
	for _, lm := range lang.LexModes {
		if int(lm.ExternalLexState) > maxELS {
			maxELS = int(lm.ExternalLexState)
		}
	}
	if maxELS >= len(lang.ExternalLexStates) {
		t.Fatalf("LexModes reference external_lex_state %d but table only has %d rows", maxELS, len(lang.ExternalLexStates))
	}

	// The precise table is also the C-recovery election gate for c_sharp:
	// with it registered (plus the attached scanner), the faithful C
	// error-recovery cost competition must be ELECTED by default.
	diag := gotreesitter.DiagnoseCRecoveryGate(lang)
	if !diag.Supported {
		t.Fatalf("DiagnoseCRecoveryGate not supported: %s", diag.Reason)
	}
	if !lang.CRecoveryCostCompetitionCapable || !lang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatalf("c_sharp not elected for C recovery: capable=%v default=%v",
			lang.CRecoveryCostCompetitionCapable, lang.CRecoveryCostCompetitionEnabledByDefault)
	}
}

// TestCSharpInterpolatedStringFirstPassNoStacksAlive is the end-to-end regression
// guard for the interpolated-string first-pass failure. It parses the corpus file
// DeclaredTypeManager.cs (skipped when unavailable) and asserts pass-1 reaches EOF
// without ending no_stacks_alive. Before the ExternalLexStates fix, pass-1 died at
// the file's second interpolated string (byte 31888) with StopReason=no_stacks_alive
// and Truncated=true, feeding a 1,076-pass retry cascade.
func TestCSharpInterpolatedStringFirstPassNoStacksAlive(t *testing.T) {
	const corpusPath = "/home/draco/work/gotreesitter-corpora/corpus_sources/c_sharp/src/Bicep.Core/TypeSystem/DeclaredTypeManager.cs"
	src, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Skipf("corpus file unavailable: %v", err)
	}

	lang := CSharpLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rt := tree.ParseRuntime()
	if rt.StopReason == gotreesitter.ParseStopNoStacksAlive {
		t.Fatalf("pass-1 ended no_stacks_alive (interpolation-scanner corruption regressed): %s", rt.Summary())
	}
	if rt.Truncated {
		t.Fatalf("pass-1 truncated before EOF: %s", rt.Summary())
	}
	if got, want := tree.RootNode().EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (%s)", got, want, rt.Summary())
	}
}

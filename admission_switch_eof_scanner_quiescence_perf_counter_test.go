//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerfCounterExcludesEOFScannerQuiescenceProbeLexing pins finding F8: the
// compact end-of-input scanner quiescence probe
// (proveCompactEOFScannerQuiescence, parsercore_phase0_eof_scanner_quiescence.go)
// re-runs the external scanner outside the parse, once per head state, and
// that lexing must not inflate the parse's own perf-counter lexed count.
//
// gts.CompactEOFScannerQuiescenceProbeWindowForTest brackets the probe's own
// per-state Next() loop with two callbacks, so the test reads the LexTokens
// and ProbeLexTokens delta the loop alone produced, unconfounded by whatever
// the rest of the parse itself lexes (the two engines the admission switch
// chooses between do not call Next() the same number of times over the whole
// parse, so comparing their totals would not isolate the probe's own
// contribution).
//
// Without the -tags perf build every counter reads zero, so this test then
// skips its assertions, matching TestRustTokenTreeForkCount's pattern
// (rust_token_tree_fork_reduction_test.go).
func TestPerfCounterExcludesEOFScannerQuiescenceProbeLexing(t *testing.T) {
	lang := grammars.ScalaLanguage()
	if lang.ExternalScanner == nil {
		t.Fatal("scala lost its external scanner, so this witness no longer exercises the probe")
	}
	source := []byte(scalaEOFScannerQuiescenceWitnesses[0].source)

	gts.ResetPerfCounters()

	var lexTokensBefore, lexTokensAfter, probeLexTokensBefore, probeLexTokensAfter uint64
	windows := 0
	restore := gts.CompactEOFScannerQuiescenceProbeWindowForTest(
		func() {
			windows++
			snap := gts.PerfCountersSnapshot()
			lexTokensBefore = snap.LexTokens
			probeLexTokensBefore = snap.ProbeLexTokens
		},
		func() {
			snap := gts.PerfCountersSnapshot()
			lexTokensAfter = snap.LexTokens
			probeLexTokensAfter = snap.ProbeLexTokens
		},
	)
	defer restore()

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	defer candidateTree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf(
			"candidate route counters = %d/%d, want 1/0; reason=%s",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason(),
		)
	}
	if windows != 1 {
		t.Fatalf("probe window fired %d times, want exactly 1 (once per parse attempt)", windows)
	}
	if lexTokensBefore == 0 {
		t.Skip("perf counters disabled (build with -tags perf to measure lexed tokens)")
	}

	probeDelta := probeLexTokensAfter - probeLexTokensBefore
	if probeDelta == 0 {
		t.Fatal("probe lexed-token counter did not move, so this witness did not exercise the probe")
	}
	if lexTokensAfter != lexTokensBefore {
		t.Fatalf(
			"parse lexed-token counter moved by %d during the probe window, want 0 (probe delta was %d)",
			lexTokensAfter-lexTokensBefore, probeDelta,
		)
	}
}

//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strconv"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// The compact-route decline census sub-classifies a recovery-boundary decline
// by the C recovery mechanism that owns the input at that point
// (admissionCensusRecoveryShape, admission_census.go). These tests pin that
// classification.
//
// Every witness below is a short literal source over an embedded grammar, not
// a corpus file. cgo_harness/corpus_real is gitignored, so a corpus-backed
// gate silently skips and reports ok on a checkout that never staged the
// corpus; these witnesses cannot skip.

// enableAdmissionCensus turns the decline census on for the calling test,
// using the pattern the existing census tests already established
// (admission_switch_kotlin_certification_test.go): set the environment
// variable, then clear the sync.Once that caches it so the new value takes
// effect regardless of what an earlier test in this binary already read.
// t.Setenv restores the variable, and the cleanup clears the cache again so
// later tests re-read rather than inherit this test's value.
func enableAdmissionCensus(t *testing.T) {
	t.Helper()
	t.Setenv("GTS_ADMISSION_CENSUS", "1")
	t.Setenv("GTS_ADMISSION_CENSUS_RECOVERY_SHAPE", "1")
	gts.ResetAdmissionCensusEnabledForTest()
	t.Cleanup(gts.ResetAdmissionCensusEnabledForTest)
}

// disableAdmissionCensus is enableAdmissionCensus's counterpart, proving the
// census-off path rather than assuming it.
func disableAdmissionCensus(t *testing.T) {
	t.Helper()
	t.Setenv("GTS_ADMISSION_CENSUS", "0")
	t.Setenv("GTS_ADMISSION_CENSUS_RECOVERY_SHAPE", "0")
	gts.ResetAdmissionCensusEnabledForTest()
	t.Cleanup(gts.ResetAdmissionCensusEnabledForTest)
}

// admissionCensusRecoveryShapeWitness is one pinned (source, mechanism) pair.
type admissionCensusRecoveryShapeWitness struct {
	language string
	source   string
	// mechanism is the c-mechanism value the census must report.
	mechanism string
	// admitted marks the first live recover_eof witness. It now serves through
	// the certified compact route, so it has no decline classification.
	admitted bool
	// candidates, when non-zero, additionally pins the reported
	// missing-token candidate population. It is asserted only where the
	// count is a structural claim worth pinning (exactly one candidate
	// terminal, so C's ascending-order "first surviving candidate" scan is
	// unambiguous), not for every witness.
	candidates int
}

func admissionCensusRecoveryShapeWitnesses() []admissionCensusRecoveryShapeWitness {
	return []admissionCensusRecoveryShapeWitness{
		// C ts_parser__handle_error step 2 (parser.c:2154-2230): a terminal
		// shifts from the declining state into a state whose leading action
		// for the elected token is a reduce, so C inserts a zero-width
		// MISSING leaf and keeps parsing. B3 stage S5 owns this.
		{language: "go", source: "package p\nfunc f() {\n", mechanism: "missing-token-insertion", candidates: 1},
		{language: "python", source: "def f(:\n    pass\n", mechanism: "missing-token-insertion", candidates: 1},
		{language: "ini", source: "[a\nb=c\n", mechanism: "missing-token-insertion", candidates: 1},
		{language: "c", source: "int x = ;", mechanism: "missing-token-insertion"},

		// C ts_parser__recover strategy 1 (cRecoverStrategy1Election): no
		// missing-token opportunity, but an ancestor boundary within
		// cRecoverMaxSummaryDepth has an action for the elected token, so C
		// recovers to that state. B3 stage S4 owns this.
		{language: "json", source: `{"a": 1`, mechanism: "stack-summary-resume"},
		{language: "c", source: "int main() { return 0; ", mechanism: "stack-summary-resume"},
		{language: "lua", source: "function f( end", mechanism: "stack-summary-resume"},

		// C ts_parser__recover strategy 2 (cAbsorbTokenIntoError): neither
		// earlier mechanism applies, so C absorbs the elected token into an
		// open error region. B3 stage S3 owns this.
		{language: "sql", source: "SELECT a b c FROM;", mechanism: "error-region-absorb"},

		// C ts_parser__recover recover_eof: the elected token is
		// authenticated end-of-file and no earlier mechanism applies, so C
		// wraps the remaining stack in one ERROR root.
		{language: "yaml", source: "[\n", mechanism: "recover-eof-wrap", admitted: true},
	}
}

// admissionCensusDeclineReason drives the compact route's own accept-or-decline
// seam and returns the decline reason, or "" when the route admitted the
// source.
//
// It uses TryCompactFullParseRouteForTest rather than
// SetAdmissionCandidateRoute plus Parse plus the counter triple. That seam
// exists precisely to expose the decline detail without the public-Parse
// eligibility gates, and it avoids mutating the process-global admission
// counters that other tests in this package read.
func admissionCensusDeclineReason(t *testing.T, language, source string) string {
	t.Helper()
	var entry grammars.LangEntry
	found := false
	for _, candidate := range grammars.AllLanguages() {
		if candidate.Name == language {
			entry, found = candidate, true
			break
		}
	}
	if !found {
		// Deliberately fatal, not a skip. The whole point of using embedded
		// grammars instead of the gitignored corpus was that a corpus gate
		// silently skips and reports ok; skipping here would reproduce that
		// exact failure mode on a build with a trimmed registry.
		t.Fatalf("grammar %q is not registered in this build", language)
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatalf("grammar %q has no loadable language", language)
	}
	tree, ok, reason := gts.TryCompactFullParseRouteForTest(gts.NewParser(lang), []byte(source))
	if tree != nil {
		defer tree.Release()
	}
	if ok {
		return ""
	}
	if reason == "" {
		t.Fatalf("%s: compact route declined %q with no reason", language, source)
	}
	return reason
}

// TestAdmissionCensusRecoveryShapeClassification pins the C-mechanism
// sub-class the census reports for each recovery-boundary decline.
func TestAdmissionCensusRecoveryShapeClassification(t *testing.T) {
	enableAdmissionCensus(t)

	for _, witness := range admissionCensusRecoveryShapeWitnesses() {
		t.Run(witness.language+"/"+witness.mechanism, func(t *testing.T) {
			reason := admissionCensusDeclineReason(t, witness.language, witness.source)
			if witness.admitted {
				if reason != "" {
					t.Fatalf("certified compact route declined %q: %s", witness.source, reason)
				}
				return
			}
			if reason == "" {
				t.Fatalf("compact route admitted %q; expected a recovery decline", witness.source)
			}
			want := "[c-mechanism=" + witness.mechanism
			if !strings.Contains(reason, want) {
				t.Fatalf("source %q: decline reason %q does not report %q", witness.source, reason, want)
			}
			if witness.candidates != 0 {
				wantCandidates := want + " candidates=" + strconv.Itoa(witness.candidates) + " "
				if !strings.Contains(reason, wantCandidates) {
					t.Fatalf("source %q: decline reason %q does not report %q", witness.source, reason, wantCandidates)
				}
			}
		})
	}
}

// TestAdmissionCensusRecoveryShapeIsDiagnosticOnly proves the classification
// changes decline TEXT only: with the census disabled every witness still
// declines, still declines for the same recorded reason, and carries no
// c-mechanism tag at all.
func TestAdmissionCensusRecoveryShapeIsDiagnosticOnly(t *testing.T) {
	for _, witness := range admissionCensusRecoveryShapeWitnesses() {
		t.Run(witness.language, func(t *testing.T) {
			off := func() string {
				disableAdmissionCensus(t)
				return admissionCensusDeclineReason(t, witness.language, witness.source)
			}()
			if witness.admitted {
				if off != "" {
					t.Fatalf("certified compact route declined %q with the census disabled: %s", witness.source, off)
				}
				enableAdmissionCensus(t)
				if on := admissionCensusDeclineReason(t, witness.language, witness.source); on != "" {
					t.Fatalf("certified compact route declined %q with the census enabled: %s", witness.source, on)
				}
				return
			}
			if off == "" {
				t.Fatalf("compact route admitted %q with the census disabled", witness.source)
			}
			if strings.Contains(off, "c-mechanism") {
				t.Fatalf("census-disabled decline reason leaked a classification: %q", off)
			}

			enableAdmissionCensus(t)
			on := admissionCensusDeclineReason(t, witness.language, witness.source)
			if on == "" {
				t.Fatalf("compact route admitted %q with the census enabled", witness.source)
			}
			// State the relationship precisely. The census does NOT append to
			// the census-off text: with the census disabled the runner reports
			// its FOLDED message ("did not accept EOF"), and enabling the
			// census replaces that with the scheduler's own fine-grained
			// boundary and detail. That replacement is pre-existing behavior.
			// What this tranche adds is only the trailing tag, so the property
			// to prove is that stripping the tag leaves the census detail the
			// census already produced, intact.
			tagIndex := strings.Index(on, " [c-mechanism=")
			if tagIndex < 0 {
				t.Fatalf("census-enabled decline reason carries no classification: %q", on)
			}
			base, tag := on[:tagIndex], on[tagIndex:]
			if !strings.HasSuffix(tag, "]") || strings.Count(tag, "[c-mechanism=") != 1 {
				t.Fatalf("census-enabled reason appended %q, want exactly one [c-mechanism=...] tag", tag)
			}
			// The untagged remainder must still be the census's own
			// classification of this decline, unaltered.
			if !strings.Contains(base, "mechanism=recovery-entered") ||
				!strings.Contains(base, gts.DiagnosticParserCoreNoTableActionDetailForTest()) {
				t.Fatalf("the tag displaced part of the census detail; remainder = %q", base)
			}
			// And the routing outcome is identical either way: both declined.
			if off == "" || on == "" {
				t.Fatalf("census toggling changed the routing outcome")
			}
		})
	}
}

// TestAdmissionCensusRecoveryShapeVocabulary proves every classification the
// census can emit comes from the closed, documented set. A new mechanism must
// be added here deliberately, not leak out as free text.
func TestAdmissionCensusRecoveryShapeVocabulary(t *testing.T) {
	enableAdmissionCensus(t)

	// Derived from the shipped constants, not retyped: renaming a constant
	// must not leave this test asserting the old vocabulary.
	known := map[string]bool{}
	for _, shape := range gts.AdmissionCensusRecoveryShapesForTest() {
		known[shape] = true
	}
	if len(known) != 4 {
		t.Fatalf("census exposes %d recovery shapes, want the 4 documented ones", len(known))
	}
	for _, witness := range admissionCensusRecoveryShapeWitnesses() {
		if witness.admitted {
			continue
		}
		if !known[witness.mechanism] {
			t.Fatalf("witness pins mechanism %q outside the documented vocabulary", witness.mechanism)
		}
		reason := admissionCensusDeclineReason(t, witness.language, witness.source)
		index := strings.Index(reason, "[c-mechanism=")
		if index < 0 {
			t.Fatalf("witness %q emitted no classification tag: %q", witness.source, reason)
		}
		tail := reason[index+len("[c-mechanism="):]
		end := strings.IndexAny(tail, " ]")
		if end < 0 {
			t.Fatalf("malformed classification tag in %q", reason)
		}
		if !known[tail[:end]] {
			t.Fatalf("census reported mechanism %q outside the documented vocabulary (%q)", tail[:end], reason)
		}
	}
}

// TestAdmissionCensusRecoveryShapeNeedsItsOwnOptIn proves the separation that
// keeps cgo_harness byte-identical: with the ordinary census enabled but the
// recovery sub-classification NOT requested, no decline carries the tag.
//
// cgo_harness/testmain_cgo_test.go sets GTS_ADMISSION_CENSUS=1 for its whole
// package, and six tests there pin the recovery decline reason by exact
// equality. Those tests are container-only, so a regression here would not
// surface on a host run; this gate stands in for them.
func TestAdmissionCensusRecoveryShapeNeedsItsOwnOptIn(t *testing.T) {
	t.Setenv("GTS_ADMISSION_CENSUS", "1")
	t.Setenv("GTS_ADMISSION_CENSUS_RECOVERY_SHAPE", "")
	gts.ResetAdmissionCensusEnabledForTest()
	t.Cleanup(gts.ResetAdmissionCensusEnabledForTest)

	for _, witness := range admissionCensusRecoveryShapeWitnesses() {
		if witness.admitted {
			continue
		}
		reason := admissionCensusDeclineReason(t, witness.language, witness.source)
		if reason == "" {
			t.Fatalf("compact route admitted %q", witness.source)
		}
		if strings.Contains(reason, "c-mechanism") {
			t.Fatalf("the ordinary census leaked a recovery sub-classification: %q", reason)
		}
		// The census's own coarse classification must still be present, so
		// this proves a narrower gate, not a disabled census.
		if !strings.Contains(reason, "mechanism=") {
			t.Fatalf("the ordinary census stopped classifying entirely: %q", reason)
		}
	}
}

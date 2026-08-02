//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// This file gates spec.campaign.v7 tranche A5/C1 (compat-tail elision):
// admission_switch_candidate.go's tryCompactFullParseRoute skips the
// result-compatibility walk, the C-recovery-swallow resolver, and the final-
// tree compaction pass for a registry-eligible language (see
// result_compat_elision.go and docs/compat-tail-elision.md). Every test here
// parses the SAME source through the SAME compact route twice -- once with
// elision forced off (SetResultCompatibilityElisionForceDisabledForTest),
// once with it left at its registry-computed default -- and requires a
// byte-identical gts-deep-tree-v1 digest (benchfixtures.InspectGoTree). ANY
// divergence is a hard failure: per the campaign's stop rule, a tree
// difference kills the elision for that language rather than being papered
// over here.

// compatTailElisionOutcome classifies one (language, source) probe.
type compatTailElisionOutcome string

const (
	elisionOutcomeEqual       compatTailElisionOutcome = "EQUAL"
	elisionOutcomeDiverge     compatTailElisionOutcome = "DIVERGE"
	elisionOutcomeNotEligible compatTailElisionOutcome = "NOT_ELIGIBLE"
	elisionOutcomeNotRouted   compatTailElisionOutcome = "NOT_ROUTED"
	elisionOutcomeAsymmetric  compatTailElisionOutcome = "ASYMMETRIC_ROUTE"
	elisionOutcomeError       compatTailElisionOutcome = "ERROR"
)

type compatTailElisionRow struct {
	language string
	outcome  compatTailElisionOutcome
	detail   string
}

// probeCompatTailElisionEquivalence parses source with lang twice through the
// public compact-candidate route: once with elision force-disabled, once with
// it left at the registry default. It reports elisionOutcomeNotEligible
// immediately (without parsing) when the registry does not mark lang
// eligible, and elisionOutcomeNotRouted when the unelided control parse never
// actually took the compact route (nothing to compare -- an ordinary,
// expected fallback for a source or grammar shape the compact scheduler
// declines).
func probeCompatTailElisionEquivalence(entry grammars.LangEntry, source []byte) compatTailElisionRow {
	row := compatTailElisionRow{language: entry.Name, outcome: elisionOutcomeError}
	defer func() {
		if r := recover(); r != nil {
			row.outcome = elisionOutcomeError
			row.detail = fmt.Sprintf("panic: %v", r)
		}
	}()

	lang := entry.Language()
	if lang == nil {
		row.detail = "nil language"
		return row
	}
	if !gts.ResultCompatibilityElisionEligibleForTest(lang) {
		row.outcome = elisionOutcomeNotEligible
		return row
	}
	support := grammars.EvaluateParseSupport(entry, lang)
	if support.Backend != grammars.ParseBackendDFA {
		row.outcome = elisionOutcomeNotRouted
		row.detail = "not DFA-routable: " + support.Reason
		return row
	}

	unelidedTree, unelidedRouted, err := parseThroughCompactRoute(lang, source, true)
	if err != nil {
		row.detail = "unelided parse: " + err.Error()
		return row
	}
	if unelidedTree == nil || !unelidedRouted {
		row.outcome = elisionOutcomeNotRouted
		row.detail = "unelided control parse did not take the compact route: " + gts.AdmissionCandidateLastFallbackReason()
		if unelidedTree != nil {
			unelidedTree.Release()
		}
		return row
	}
	defer unelidedTree.Release()

	elidedTree, elidedRouted, err := parseThroughCompactRoute(lang, source, false)
	if err != nil {
		row.detail = "elided parse: " + err.Error()
		return row
	}
	if elidedTree != nil {
		defer elidedTree.Release()
	}
	if !elidedRouted {
		// The unelided control run proved this exact source routes compact for
		// this language; the only difference between the two runs is the
		// elision flag itself, so a route flip here is a genuine bug, not an
		// ordinary decline.
		row.outcome = elisionOutcomeAsymmetric
		row.detail = "elided run fell back to production while the unelided control routed compact: " + gts.AdmissionCandidateLastFallbackReason()
		return row
	}

	unelidedInspection, err := benchfixtures.InspectGoTree(unelidedTree.RootNode(), lang)
	if err != nil {
		row.detail = "unelided digest: " + err.Error()
		return row
	}
	elidedInspection, err := benchfixtures.InspectGoTree(elidedTree.RootNode(), lang)
	if err != nil {
		row.detail = "elided digest: " + err.Error()
		return row
	}
	if unelidedInspection.SHA256 != elidedInspection.SHA256 {
		row.outcome = elisionOutcomeDiverge
		first := firstAdmissionTreeDivergence(elidedTree.RootNode(), unelidedTree.RootNode(), lang, "root")
		if first == "" {
			first = "digest mismatch without a visible node mismatch"
		}
		row.detail = fmt.Sprintf(
			"elided=%s unelided=%s first=%s",
			elidedInspection.SHA256[:12],
			unelidedInspection.SHA256[:12],
			first,
		)
		return row
	}
	row.outcome = elisionOutcomeEqual
	row.detail = "digest " + elidedInspection.SHA256[:12]
	return row
}

// parseThroughCompactRoute parses source with a fresh Parser pinned to the
// compact candidate route, forcing the compat-tail elision flag to
// forceElisionOff for the duration of the call. It reports whether the parse
// actually routed compact (as opposed to falling back to production), since
// only a routed parse exercises tryCompactFullParseRoute at all.
func parseThroughCompactRoute(lang *gts.Language, source []byte, forceElisionOff bool) (*gts.Tree, bool, error) {
	restore := gts.SetResultCompatibilityElisionForceDisabledForTest(forceElisionOff)
	defer restore()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	gts.ResetAdmissionCandidateCountersForTest()
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, false, err
	}
	routed, _ := gts.AdmissionCandidateCounters()
	return tree, routed == 1, nil
}

// summarizeCompatTailElisionRows logs a per-outcome tally and fails the test
// on any DIVERGE or ERROR. NOT_ELIGIBLE, NOT_ROUTED, and ASYMMETRIC_ROUTE
// each get their own count; ASYMMETRIC_ROUTE is also a hard failure (see
// probeCompatTailElisionEquivalence).
func summarizeCompatTailElisionRows(t *testing.T, label string, rows []compatTailElisionRow) {
	t.Helper()
	sort.Slice(rows, func(i, j int) bool { return rows[i].language < rows[j].language })
	counts := map[compatTailElisionOutcome]int{}
	for _, row := range rows {
		counts[row.outcome]++
		t.Logf("%-16s %-16s %s", row.language, row.outcome, row.detail)
	}
	t.Logf(
		"--- %s: EQUAL=%d DIVERGE=%d NOT_ELIGIBLE=%d NOT_ROUTED=%d ASYMMETRIC_ROUTE=%d ERROR=%d total=%d ---",
		label, counts[elisionOutcomeEqual], counts[elisionOutcomeDiverge], counts[elisionOutcomeNotEligible],
		counts[elisionOutcomeNotRouted], counts[elisionOutcomeAsymmetric], counts[elisionOutcomeError], len(rows),
	)
	if counts[elisionOutcomeEqual] == 0 {
		t.Fatalf("%s: zero eligible sources actually routed compact on both sides; the gate proved nothing", label)
	}
	for _, row := range rows {
		if row.outcome == elisionOutcomeDiverge || row.outcome == elisionOutcomeAsymmetric || row.outcome == elisionOutcomeError {
			t.Errorf("%s: %s %s: %s", label, row.language, row.outcome, row.detail)
		}
	}
}

// TestCompatTailElisionEquivalenceSmoke runs the elided-vs-unelided
// equivalence probe across every registered language's committed smoke
// sample, for every language the registry currently marks elision-eligible.
// It is unconditional (no env gate): smoke samples are tiny and committed, so
// this is the always-on floor of the correctness gate.
func TestCompatTailElisionEquivalenceSmoke(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entries := grammars.AllLanguages()
	rows := make([]compatTailElisionRow, 0, len(entries))
	for _, entry := range entries {
		source := []byte(grammars.ParseSmokeSample(entry.Name))
		if len(source) == 0 {
			continue
		}
		rows = append(rows, probeCompatTailElisionEquivalence(entry, source))
	}
	summarizeCompatTailElisionRows(t, "smoke", rows)
}

// compatTailElisionCorpusManifest mirrors admissionRealCorpusManifest
// (admission_real_corpus_matrix_test.go): the same
// cgo_harness/corpus_real/manifest.json this repo's other real-corpus gate
// reads.
type compatTailElisionCorpusManifest struct {
	Entries []compatTailElisionCorpusEntry `json:"entries"`
}

type compatTailElisionCorpusEntry struct {
	Language   string `json:"language"`
	Bucket     string `json:"bucket"`
	Bytes      int    `json:"bytes"`
	OutputPath string `json:"output_path"`
}

// TestCompatTailElisionEquivalenceRealCorpus extends the smoke gate to real,
// larger source files for elision-eligible languages, when
// cgo_harness/corpus_real is present on this host (it is git-ignored, so it
// is not always available -- see admission_real_corpus_matrix_test.go for the
// same convention this test follows).
func TestCompatTailElisionEquivalenceRealCorpus(t *testing.T) {
	if os.Getenv("GTS_ADMISSION_REAL_CORPUS") != "1" {
		t.Skip("set GTS_ADMISSION_REAL_CORPUS=1 to run the compat-tail elision real-corpus gate")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	manifestPath := filepath.Join("cgo_harness", "corpus_real", "manifest.json")
	manifestSource, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Skipf("real corpus manifest unavailable: %v", err)
	}
	var manifest compatTailElisionCorpusManifest
	if err := json.Unmarshal(manifestSource, &manifest); err != nil {
		t.Fatal(err)
	}

	languages := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		languages[entry.Name] = entry
	}

	rows := make([]compatTailElisionRow, 0, len(manifest.Entries))
	for _, corpus := range manifest.Entries {
		entry, ok := languages[corpus.Language]
		if !ok || !gts.ResultCompatibilityElisionEligibleForTest(entry.Language()) {
			continue
		}
		path := admissionRealCorpusPath(manifestPath, corpus.OutputPath)
		source, err := os.ReadFile(path)
		if err != nil {
			rows = append(rows, compatTailElisionRow{language: corpus.Language, outcome: elisionOutcomeError, detail: "read corpus: " + err.Error()})
			continue
		}
		row := probeCompatTailElisionEquivalence(entry, source)
		row.detail = fmt.Sprintf("%s (%s, %d bytes)", row.detail, corpus.Bucket, len(source))
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		t.Skip("no elision-eligible language has a real-corpus entry present")
	}
	summarizeCompatTailElisionRows(t, "real-corpus", rows)
}

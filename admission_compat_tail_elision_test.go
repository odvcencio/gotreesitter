package gotreesitter_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// This file gates spec.campaign.v7 tranche A5/C1 (compat-tail elision):
// admission_switch_candidate.go's tryCompactFullParseRoute skips the
// C-recovery-swallow resolver and the final-tree compaction pass for a
// registry-eligible language (see result_compat_elision.go and
// the compat-tail elision record (Hyphae space m31labs/gotreesitter)). Every test here parses the SAME source
// through the SAME compact route twice -- once with elision forced off
// (SetResultCompatibilityElisionForceDisabledForTest), once with it left at
// its registry-computed default -- and requires a byte-identical
// gts-deep-tree-v1 digest (benchfixtures.InspectGoTree). ANY divergence is a
// hard failure: per the campaign's stop rule, a tree difference kills the
// elision for that language rather than being papered over here.
//
// Deliberately untagged (no gts_parsercorephase0 build tag): the production
// code this file gates, admission_switch_candidate.go, ships in the DEFAULT
// build (it is excluded only by the opt-out tag gts_no_parsercorephase0), so
// this gate must compile and run in the same default build -- including the
// required, untagged `go test . -race` root-package lane
// (.github/scripts/root_race_shard.sh enumerates targets with `go test -race
// . -list`, which cannot see a tagged file). This file therefore defines its
// own small local copies of the two helpers it needs from tagged sibling
// files (firstAdmissionTreeDivergence in admission_scorecard_test.go,
// admissionRealCorpusPath in admission_real_corpus_matrix_test.go) instead
// of depending on them directly, under different names to avoid a duplicate-
// symbol conflict on a build that compiles both this file and those.

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
		first := compatTailElisionFirstTreeDivergence(elidedTree.RootNode(), unelidedTree.RootNode(), lang, "root")
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

// compatTailElisionSmokeLanguages is a curated, diverse subset of
// elision-eligible, compact-routable languages for
// TestCompatTailElisionEquivalenceSmoke's always-on run: retired-arm
// languages (html, erlang, ocaml) and languages that never had an arm at all
// (css, json5, toml). Bounded deliberately, and kept small -- see that
// test's doc comment for why this is not the full registry.
var compatTailElisionSmokeLanguages = []string{
	"html", "erlang", "ocaml", "css", "json5", "toml",
}

// TestCompatTailElisionEquivalenceSmoke runs the elided-vs-unelided
// equivalence probe across a curated, bounded subset of registered
// languages' committed smoke samples (compatTailElisionSmokeLanguages). It
// is unconditional (no env gate): this is the always-on floor of the
// correctness gate, including in the untagged `go test . -race` root-package
// lane (see the file doc comment above).
//
// Bounded rather than exhaustive deliberately: an earlier version of this
// test iterated all 206 registered languages unconditionally. Loading and
// parsing every registered grammar inflates process heap enough to disturb
// the whole-process TestArenaGCRetentionAfterRelease gate elsewhere in this
// package -- the exact interaction admissionScorecardRequiredCompactPasses's
// own test (TestAdmissionCandidateScorecard206, admission_scorecard_test.go)
// already documents and avoids by gating its own 206-language sweep behind
// an explicit opt-in. TestCompatTailElisionEquivalenceSmokeExhaustive below
// is that same opt-in for this gate; this test is the bounded, safe default.
func TestCompatTailElisionEquivalenceSmoke(t *testing.T) {
	t.Cleanup(func() {
		grammars.PurgeEmbeddedLanguageCache()
		gts.DrainArenaPools()
		runtime.GC()
	})
	languages := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		languages[entry.Name] = entry
	}
	rows := make([]compatTailElisionRow, 0, len(compatTailElisionSmokeLanguages))
	for _, name := range compatTailElisionSmokeLanguages {
		entry, ok := languages[name]
		if !ok {
			t.Fatalf("curated smoke language %q is not registered", name)
		}
		source := []byte(grammars.ParseSmokeSample(name))
		if len(source) == 0 {
			t.Fatalf("curated smoke language %q has no smoke sample", name)
		}
		rows = append(rows, probeCompatTailElisionEquivalence(entry, source))
	}
	summarizeCompatTailElisionRows(t, "smoke", rows)
}

// TestCompatTailElisionEquivalenceSmokeExhaustive is
// TestCompatTailElisionEquivalenceSmoke's full-registry counterpart: every
// registered language's committed smoke sample, not just the curated
// subset. Opt-in (GTS_COMPAT_TAIL_ELISION_EXHAUSTIVE=1) for the same reason
// TestAdmissionCandidateScorecard206 (admission_scorecard_test.go) gates its
// own 206-language sweep behind GTS_ADMISSION_SCORECARD: loading every
// registered grammar in one process disturbs
// TestArenaGCRetentionAfterRelease elsewhere in this package's default
// suite.
func TestCompatTailElisionEquivalenceSmokeExhaustive(t *testing.T) {
	if os.Getenv("GTS_COMPAT_TAIL_ELISION_EXHAUSTIVE") != "1" {
		t.Skip("set GTS_COMPAT_TAIL_ELISION_EXHAUSTIVE=1 to run the exhaustive, all-registered-language compat-tail elision smoke gate")
	}
	t.Cleanup(func() {
		grammars.PurgeEmbeddedLanguageCache()
		gts.DrainArenaPools()
		runtime.GC()
	})
	entries := grammars.AllLanguages()
	rows := make([]compatTailElisionRow, 0, len(entries))
	for _, entry := range entries {
		source := []byte(grammars.ParseSmokeSample(entry.Name))
		if len(source) == 0 {
			continue
		}
		rows = append(rows, probeCompatTailElisionEquivalence(entry, source))
	}
	summarizeCompatTailElisionRows(t, "smoke-exhaustive", rows)
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
//
// This gate does NOT run in CI: cgo_harness/corpus_real is git-ignored and no
// CI job provisions it, so GTS_ADMISSION_REAL_CORPUS is never set there (see
// the CI gate coverage record (Hyphae space m31labs/gotreesitter)). A prior PR cited this test's results as merge
// evidence despite that -- both skip paths below print directly to stderr in
// addition to t.Skip's own message, so the reason is captured verbatim by
// `go test -json` (which race_root_shards/race_root_isolated already run
// this untagged file under) and by any -v invocation, not only visible to
// someone who happens to pass -v by hand, precisely so that mistake cannot
// repeat silently.
func TestCompatTailElisionEquivalenceRealCorpus(t *testing.T) {
	if os.Getenv("GTS_ADMISSION_REAL_CORPUS") != "1" {
		reason := "SKIP " + t.Name() + ": GTS_ADMISSION_REAL_CORPUS is not set to 1; this gate requires cgo_harness/corpus_real, which is git-ignored and NOT provisioned by any CI job (see the CI gate coverage record (Hyphae space m31labs/gotreesitter)) -- its results are not CI evidence"
		fmt.Fprintln(os.Stderr, reason)
		t.Skip(reason)
	}
	t.Cleanup(func() {
		grammars.PurgeEmbeddedLanguageCache()
		gts.DrainArenaPools()
		runtime.GC()
	})

	manifestPath := filepath.Join("cgo_harness", "corpus_real", "manifest.json")
	manifestSource, err := os.ReadFile(manifestPath)
	if err != nil {
		reason := fmt.Sprintf("SKIP %s: real corpus manifest unavailable at %s: %v", t.Name(), manifestPath, err)
		fmt.Fprintln(os.Stderr, reason)
		t.Skip(reason)
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
		path := compatTailElisionCorpusPath(manifestPath, corpus.OutputPath)
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

// compatTailElisionRaceLanguages are elision-eligible smoke-sample-bearing
// languages used by TestCompatTailElisionSurvivesConcurrentCancellation.
// Picked for route diversity (retired arms and never-had-an-arm languages),
// not exhaustively -- the race property under test does not depend on
// per-language dispatcher history, only on the shared tail control flow, so
// a small set kept the memory footprint of this always-on test bounded (see
// TestCompatTailElisionEquivalenceSmoke's doc comment for why bounded
// footprint matters here).
var compatTailElisionRaceLanguages = []string{"html", "zig"}

// TestCompatTailElisionSurvivesConcurrentCancellation reproduces the
// concurrent-cancellation regression an earlier version of
// finalizeCompactReturnedTreeForParse (admission_switch_candidate.go) let
// through: hoisting the terminal-stop-reason check ahead of
// normalizeReturnedTreeForParse's own "!tree.resultCompatibilityApplied"
// guard let an unrelated cancellation, observed strictly AFTER the compact
// route already produced a complete, accepted tree, discard that good tree
// instead of returning it. See the compat-tail elision record's "Corrected
// finding" section.
//
// Every trial races a background goroutine's cancellation-flag flip against
// a full Parser.Parse call. Whenever AdmissionCandidateCounters reports this
// exact call routed the compact route (a routed delta of exactly one), the
// compact scheduler already held a complete, accepted derivation for this
// call by definition; the returned tree must never report a terminal
// (early-stopped/cancelled) reason in that case.
func TestCompatTailElisionSurvivesConcurrentCancellation(t *testing.T) {
	t.Cleanup(func() {
		grammars.PurgeEmbeddedLanguageCache()
		gts.DrainArenaPools()
		runtime.GC()
	})
	const trials = 150

	languages := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		languages[entry.Name] = entry
	}

	for _, langName := range compatTailElisionRaceLanguages {
		langName := langName
		t.Run(langName, func(t *testing.T) {
			entry, ok := languages[langName]
			if !ok {
				t.Fatalf("language %q not found in registry", langName)
			}
			lang := entry.Language()
			if lang == nil || !gts.ResultCompatibilityElisionEligibleForTest(lang) {
				t.Fatalf("language %q must be elision-eligible for this probe to be meaningful", langName)
			}
			source := []byte(grammars.ParseSmokeSample(langName))
			if len(source) == 0 {
				t.Fatalf("language %q has no smoke sample", langName)
			}

			routedTrials := 0
			routedCancelled := 0
			for i := 0; i < trials; i++ {
				var flag uint32
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(true)
				parser.SetCancellationFlag(&flag)
				gts.ResetAdmissionCandidateCountersForTest()

				done := make(chan struct{})
				go func() {
					defer close(done)
					atomic.StoreUint32(&flag, 1)
				}()

				tree, err := parser.Parse(source)
				<-done
				if err != nil {
					continue
				}
				routed, _ := gts.AdmissionCandidateCounters()
				if routed != 1 {
					if tree != nil {
						tree.Release()
					}
					continue
				}
				routedTrials++
				if tree == nil {
					t.Fatalf("trial %d: routed=1 but Parse returned a nil tree", i)
					continue
				}
				if tree.ParseStoppedEarly() {
					routedCancelled++
				}
				tree.Release()
			}
			t.Logf("%s: %d/%d trials routed compact, %d reported cancelled after routing", langName, routedTrials, trials, routedCancelled)
			if routedCancelled != 0 {
				t.Fatalf("%s: %d/%d routed parses reported a terminal stop reason after the compact route already completed them (want 0)", langName, routedCancelled, trials)
			}
		})
	}
}

// compatTailElisionFirstTreeDivergence is a local copy of
// firstAdmissionTreeDivergence (admission_scorecard_test.go, a tagged
// sibling file this untagged file cannot depend on -- see the file doc
// comment above). Reports the first structural difference between two trees
// built from the same source, or "" if they are identical.
func compatTailElisionFirstTreeDivergence(candidate, production *gts.Node, lang *gts.Language, path string) string {
	if candidate == nil || production == nil {
		return fmt.Sprintf("%s nil candidate=%t production=%t", path, candidate == nil, production == nil)
	}
	candidateStart, productionStart := candidate.StartPoint(), production.StartPoint()
	candidateEnd, productionEnd := candidate.EndPoint(), production.EndPoint()
	if candidate.Type(lang) != production.Type(lang) ||
		candidate.StartByte() != production.StartByte() ||
		candidate.EndByte() != production.EndByte() ||
		candidateStart != productionStart ||
		candidateEnd != productionEnd ||
		candidate.IsNamed() != production.IsNamed() ||
		candidate.IsExtra() != production.IsExtra() ||
		candidate.IsMissing() != production.IsMissing() ||
		candidate.IsError() != production.IsError() ||
		candidate.ChildCount() != production.ChildCount() {
		return fmt.Sprintf(
			"%s candidate=%s[%d:%d] children=%d flags=%t/%t/%t/%t production=%s[%d:%d] children=%d flags=%t/%t/%t/%t",
			path,
			candidate.Type(lang),
			candidate.StartByte(),
			candidate.EndByte(),
			candidate.ChildCount(),
			candidate.IsNamed(),
			candidate.IsExtra(),
			candidate.IsMissing(),
			candidate.IsError(),
			production.Type(lang),
			production.StartByte(),
			production.EndByte(),
			production.ChildCount(),
			production.IsNamed(),
			production.IsExtra(),
			production.IsMissing(),
			production.IsError(),
		)
	}
	for index := 0; index < candidate.ChildCount(); index++ {
		candidateField := candidate.FieldNameForChild(index, lang)
		productionField := production.FieldNameForChild(index, lang)
		if candidateField != productionField {
			return fmt.Sprintf(
				"%s/%d field candidate=%q production=%q",
				path,
				index,
				candidateField,
				productionField,
			)
		}
		childPath := fmt.Sprintf("%s/%s[%d]", path, candidate.Child(index).Type(lang), index)
		if diff := compatTailElisionFirstTreeDivergence(candidate.Child(index), production.Child(index), lang, childPath); diff != "" {
			return diff
		}
	}
	if candidate.HasError() != production.HasError() {
		return fmt.Sprintf(
			"%s has_error candidate=%t production=%t",
			path,
			candidate.HasError(),
			production.HasError(),
		)
	}
	return ""
}

// compatTailElisionCorpusPath is a local copy of admissionRealCorpusPath
// (admission_real_corpus_matrix_test.go, a tagged sibling file this untagged
// file cannot depend on -- see the file doc comment above).
func compatTailElisionCorpusPath(manifestPath, outputPath string) string {
	if filepath.IsAbs(outputPath) {
		return outputPath
	}
	if _, err := os.Stat(outputPath); err == nil {
		return outputPath
	}
	if path := filepath.Join("cgo_harness", outputPath); path != outputPath {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(filepath.Dir(manifestPath), outputPath)
}

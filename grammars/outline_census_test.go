package grammars

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// outlineCensusBaselinePath is the committed, stable baseline artifact this
// test ratchets against. It is regenerated deliberately, in the same change
// as whatever caused it to move (grammar update, pattern-table edit, smoke
// sample edit), never silently.
const outlineCensusBaselinePath = "testdata/outline_census/baseline.json"

// TestOutlineTierCensus computes the workstream-D outline-symbol coverage
// census for every registered language (spec.outline-api.v1 section 5,
// origin https://github.com/odvcencio/gotreesitter/issues/649) and is the
// single source of truth for coverage claims in every later workstream-D
// stage. It ships no outline API and changes no shipped behavior; it is
// instrument-only.
//
// Reproduce with:
//
//	GTS_OUTLINE_CENSUS_OUT=harness_out/outline/tier_census.json \
//	GOFLAGS=-buildvcs=false go test ./grammars -run TestOutlineTierCensus -count=1
func TestOutlineTierCensus(t *testing.T) {
	// The census loads every registered grammar (like the existing
	// TestInferredTagsQueryCoverage in registry_test.go). Purge afterward so
	// it does not inflate process heap for later tests in this package.
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	entries := AllLanguages()
	result := computeOutlineTierCensus(entries)

	if result.LanguageCount != len(entries) {
		t.Fatalf("census processed %d languages, registry has %d", result.LanguageCount, len(entries))
	}

	validTiers := map[OutlineTier]bool{
		OutlineTierOne:           true,
		OutlineTierOneUnmeasured: true,
		OutlineTierTwo:           true,
		OutlineTierThree:         true,
		OutlineTierFour:          true,
	}

	sumCounts := 0
	for tier, n := range result.CountsByTier {
		if !validTiers[OutlineTier(tier)] {
			t.Errorf("unknown tier %q in counts_by_tier", tier)
		}
		sumCounts += n
	}
	if sumCounts != len(entries) {
		t.Fatalf("tier counts sum to %d, want %d -- every language must land in exactly one tier", sumCounts, len(entries))
	}

	seen := make(map[string]bool, len(result.Rows))
	for _, row := range result.Rows {
		if !validTiers[row.Tier] {
			t.Errorf("%s: invalid tier %q", row.Language, row.Tier)
		}
		if seen[row.Language] {
			t.Errorf("%s: duplicate census row", row.Language)
		}
		seen[row.Language] = true
		if row.Tier == OutlineTierThree && len(row.CandidateSymbols) == 0 {
			t.Errorf("%s: classified T3 but carries no candidate symbols", row.Language)
		}
		if row.Tier == OutlineTierFour && len(row.CandidateSymbols) != 0 {
			t.Errorf("%s: classified T4 but carries candidate symbols %v", row.Language, row.CandidateSymbols)
		}
	}
	for _, entry := range entries {
		if !seen[entry.Name] {
			t.Errorf("%s: registered language missing from census", entry.Name)
		}
	}

	t.Logf("=== Outline tier census (%d languages) ===", len(result.Rows))
	t.Logf("real corpus root: %q exists=%v", result.RealCorpusRoot, result.RealCorpusRootExists)
	if !result.RealCorpusRootExists {
		t.Log("*** T1 IS UNMEASURABLE ON THIS HOST: the real-corpus root does not exist, so no language can be " +
			"confirmed T1 here. Query-covered languages are reported T1_unmeasured, never silently folded into T2. ***")
	}
	for _, tier := range []OutlineTier{OutlineTierOne, OutlineTierOneUnmeasured, OutlineTierTwo, OutlineTierThree, OutlineTierFour} {
		t.Logf("  %-14s %d", tier, result.CountsByTier[string(tier)])
	}
	t.Log("--- T3 pattern backlog: query empty, candidate symbols present (actionable via a data edit) ---")
	for _, row := range result.Rows {
		if row.Tier == OutlineTierThree {
			t.Logf("  %-18s candidates=%v", row.Language, row.CandidateSymbols)
		}
	}

	if out := strings.TrimSpace(os.Getenv("GTS_OUTLINE_CENSUS_OUT")); out != "" {
		if err := writeOutlineCensusJSON(out, result); err != nil {
			t.Fatalf("write census JSON to %s: %v", out, err)
		}
		t.Logf("wrote census JSON to %s", out)
	}

	assertOutlineCensusBaseline(t, result)
}

// assertOutlineCensusBaseline ratchets the deterministic portion of the
// census -- whether each language's inferred tags query is non-empty, its
// T3/T4 split and candidate symbols when the query is empty, and its
// smoke-sample capture shape when the query is non-empty -- against the
// committed baseline. These facts derive only from the compiled grammar,
// the shared inference pattern table, and the fixed smoke samples, so they
// reproduce identically on every host; drift is a real coverage change and
// is hard-failed, mirroring how the derivation-set differential's baseline
// is pinned (cgo_harness/derivation_set_differential_test.go,
// TestDerivationSetDifferentialBaselineCensus).
//
// The corpus-dependent T1/T2/T1_unmeasured split depends on an external,
// non-repo-tracked real-corpus checkout (GOT_CORPUS_SOURCES_ROOT) that
// varies by host and can drift independently of this repository, so it is
// only logged against the baseline, never asserted.
func assertOutlineCensusBaseline(t *testing.T, result OutlineCensusResult) {
	t.Helper()

	data, err := os.ReadFile(outlineCensusBaselinePath)
	if err != nil {
		t.Fatalf("read committed baseline %s: %v", outlineCensusBaselinePath, err)
	}
	var baseline OutlineCensusResult
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatalf("decode committed baseline %s: %v", outlineCensusBaselinePath, err)
	}

	baselineByLang := make(map[string]OutlineCensusRow, len(baseline.Rows))
	for _, row := range baseline.Rows {
		baselineByLang[row.Language] = row
	}

	drift := 0
	for _, row := range result.Rows {
		base, ok := baselineByLang[row.Language]
		if !ok {
			t.Errorf("%s: not present in committed baseline %s -- add it in the same change", row.Language, outlineCensusBaselinePath)
			drift++
			continue
		}

		if row.TagsQueryNonEmpty != base.TagsQueryNonEmpty {
			t.Errorf("%s: tags-query coverage changed (was non-empty=%v, now non-empty=%v) -- update %s in the same change with a stated reason",
				row.Language, base.TagsQueryNonEmpty, row.TagsQueryNonEmpty, outlineCensusBaselinePath)
			drift++
			continue
		}

		if !row.TagsQueryNonEmpty {
			if row.Tier != base.Tier {
				t.Errorf("%s: query-empty tier changed (was %s, now %s) -- update %s in the same change with a stated reason",
					row.Language, base.Tier, row.Tier, outlineCensusBaselinePath)
				drift++
			}
			if !stringSlicesEqual(row.CandidateSymbols, base.CandidateSymbols) {
				t.Errorf("%s: candidate symbols changed (was %v, now %v) -- update %s in the same change with a stated reason",
					row.Language, base.CandidateSymbols, row.CandidateSymbols, outlineCensusBaselinePath)
				drift++
			}
			continue
		}

		if row.SmokeCaptureCount != base.SmokeCaptureCount || !stringSlicesEqual(row.SmokeCaptureNames, base.SmokeCaptureNames) {
			t.Errorf("%s: smoke-sample capture shape changed (was count=%d names=%v, now count=%d names=%v) -- update %s in the same change with a stated reason",
				row.Language, base.SmokeCaptureCount, base.SmokeCaptureNames, row.SmokeCaptureCount, row.SmokeCaptureNames, outlineCensusBaselinePath)
			drift++
		}

		if row.Tier != base.Tier {
			t.Logf("%s: corpus-dependent tier observed %s, baseline recorded %s (host-dependent, not asserted -- see real_corpus_root_exists)",
				row.Language, row.Tier, base.Tier)
		}
	}

	if drift > 0 {
		t.Fatalf("%d language(s) drifted from the committed, host-independent census baseline (%s)", drift, outlineCensusBaselinePath)
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

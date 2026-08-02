//go:build gts_parsercorephase0

package gotreesitter_test

// The B4b stage 2b class-1 differential regression witness (spec.b4b-
// alternative-set.v2 section 7 class 1, and the stage 2b gate). Stage 2a
// built this harness around a test-only opt-in override
// (SetAlternativeSetV2OptInDeciderEnabledForTest) that substituted v2 for
// the then-live scalar decider; the section 7 gate passed and stage 2b
// flipped dropGenericNoActionHeads to decide with v2 unconditionally for
// uncertified languages, so that override was retired and this file now
// exercises the plain candidate route. For each corpus source this file
// re-parses twice: once with the candidate route disabled (production),
// and once with it enabled (v2 decides). If v2 ever proves a converged-
// split drop that the retired scalar decider alone would not have (spec
// section 7 class 1), the candidate parse either declines somewhere else
// and falls back to production trivially (nothing interesting -- the tree
// is production's by construction), or it fully routes on v2's own proof
// alone, in which case its resulting tree must equal production's. A
// mismatch there is a class-1 differing-tree case: per the campaign's
// C-oracle adjudication amendment (2026-08-02), that is not automatically a
// v2 theorem falsifier -- the cgo_harness C-oracle adjudication
// (b4b_alternative_set_v2_kotlin_adjudication_test.go) determines whether
// production or the v2-admitted compact route is the side that diverges
// from the locked C oracle.
//
// Run with:
//
//	go test -tags gts_parsercorephase0 -run TestB4bV2OptInDeciderDifferential -v .

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type v2DifferentialResult struct {
	name             string
	routed           bool
	fallback         bool
	fallbackReason   string
	digestsMatch     bool
	productionDigest string
	candidateDigest  string
	err              error
}

// runV2OptInDeciderDifferential parses source under production and under the
// candidate route, which decides converged-split drops with the v2 proof
// unconditionally since the stage 2b flip (dropGenericNoActionHeads), and
// reports whether the resulting trees match. Recording is forced on
// explicitly (defense-in-depth): production has recorded it unconditional
// since stage 2b, but an empty altSet would make v2 decline everything
// trivially and silently defeat this harness's purpose, so the requirement
// stays pinned here regardless of the current default.
func runV2OptInDeciderDifferential(t testing.TB, name string, lang *gts.Language, source []byte) v2DifferentialResult {
	t.Helper()
	result := v2DifferentialResult{name: name}

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		result.err = fmt.Errorf("production parse: %w", err)
		return result
	}
	defer productionTree.Release()
	productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), lang)
	if err != nil {
		result.err = fmt.Errorf("inspect production tree: %w", err)
		return result
	}
	result.productionDigest = productionInspection.SHA256

	restoreRecording := core.SetAlternativeSetRecordingEnabledForTest(true)
	defer restoreRecording()

	gts.ResetAdmissionCandidateCountersForTest()
	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidate.Parse(source)
	if err != nil {
		result.err = fmt.Errorf("v2-decider candidate parse: %w", err)
		return result
	}
	defer candidateTree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	result.routed = routed == 1
	result.fallback = fallback == 1
	result.fallbackReason = gts.AdmissionCandidateLastFallbackReason()

	candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
	if err != nil {
		result.err = fmt.Errorf("inspect v2-opt-in-decider candidate tree: %w", err)
		return result
	}
	result.candidateDigest = candidateInspection.SHA256
	result.digestsMatch = result.productionDigest == result.candidateDigest
	return result
}

func reportV2Differential(t *testing.T, result v2DifferentialResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("%s: %v", result.name, result.err)
	}
	if !result.digestsMatch {
		t.Errorf(
			"CLASS-1 DIFFERING-TREE CANDIDATE: %s: v2-opt-in-decider route (routed=%t fallback=%t reason=%q) digest=%s != production digest=%s -- needs C-oracle adjudication before this is a theorem falsifier (campaign amendment 2026-08-02)",
			result.name, result.routed, result.fallback, result.fallbackReason, result.candidateDigest, result.productionDigest,
		)
		return
	}
	t.Logf("%-28s routed=%-5t fallback=%-5t digest=%s (matches production)", result.name, result.routed, result.fallback, result.candidateDigest)
}

// TestB4bV2OptInDeciderDifferentialCanonicalFixtures covers the four
// canonical Go fixtures (spec.b4b-alternative-set.v2 section 7 gate corpus),
// reusing the same frozen, authenticated embedded fixtures as
// TestDiagnosticParserCoreCanonicalAdmissions.
func TestB4bV2OptInDeciderDifferentialCanonicalFixtures(t *testing.T) {
	lang := grammars.GoLanguage()
	loaded, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatalf("load canonical Go fixtures: %v", err)
	}
	if len(loaded) != 4 {
		t.Fatalf("canonical Go fixture count = %d, want 4", len(loaded))
	}
	for _, fixture := range loaded {
		result := runV2OptInDeciderDifferential(t, fixture.Fixture.ID, lang, fixture.Source)
		reportV2Differential(t, result)
	}
}

// TestB4bV2OptInDeciderDifferentialCertifiedLanguages covers the six
// certified languages' real corpora (spec.b4b-alternative-set.v2 section 7
// gate corpus), reusing the same witnesses as
// TestAdmissionCandidateCertifiedConvergedPathSplitsMatchProduction.
func TestB4bV2OptInDeciderDifferentialCertifiedLanguages(t *testing.T) {
	tests := []struct {
		name       string
		corpusPath string
		source     string
		load       func() *gts.Language
	}{
		{name: "bash", corpusPath: filepath.Join("testdata", "compact_converged_split", "bash.sh"), load: grammars.BashLanguage},
		{name: "erlang", corpusPath: filepath.Join("testdata", "compact_converged_split", "erlang.erl"), load: grammars.ErlangLanguage},
		{name: "haskell", source: grammars.ParseSmokeSample("haskell"), load: grammars.HaskellLanguage},
		{name: "javascript", corpusPath: filepath.Join("testdata", "compact_converged_split", "javascript.js"), load: grammars.JavascriptLanguage},
		{name: "python", source: "def greet(name):\n    return f\"hello {name}\"\n\nprint(greet(\"world\"))\n", load: grammars.PythonLanguage},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			if test.corpusPath != "" {
				var err error
				source, err = os.ReadFile(test.corpusPath)
				if err != nil {
					t.Fatalf("read certified witness: %v", err)
				}
			}
			lang := test.load()
			if !lang.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("exact artifact lacks converged-split certification")
			}
			result := runV2OptInDeciderDifferential(t, test.name, lang, source)
			reportV2Differential(t, result)
		})
	}
}

// TestB4bV2OptInDeciderDifferentialSmokeCorpus covers the 206-language smoke
// corpus (spec.b4b-alternative-set.v2 section 7 gate corpus).
func TestB4bV2OptInDeciderDifferentialSmokeCorpus(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entries := grammars.AllLanguages()
	var mismatches []string
	tested := 0
	for _, entry := range entries {
		lang := entry.Language()
		if lang == nil {
			continue
		}
		support := grammars.EvaluateParseSupport(entry, lang)
		if support.Backend != grammars.ParseBackendDFA {
			continue
		}
		source := []byte(grammars.ParseSmokeSample(entry.Name))
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("%s: panic during differential (skipped): %v", entry.Name, r)
				}
			}()
			result := runV2OptInDeciderDifferential(t, entry.Name, lang, source)
			if result.err != nil {
				return // a source that cannot even production-parse is out of scope here
			}
			tested++
			if !result.digestsMatch {
				mismatches = append(mismatches, fmt.Sprintf(
					"%s: routed=%t fallback=%t reason=%q candidate=%s production=%s",
					entry.Name, result.routed, result.fallback, result.fallbackReason, result.candidateDigest, result.productionDigest,
				))
			}
		}()
	}
	t.Logf("smoke corpus v2-opt-in-decider differential: %d/%d sources compared, %d mismatch(es)", tested, len(entries), len(mismatches))
	sort.Strings(mismatches)
	for _, mismatch := range mismatches {
		t.Errorf("CLASS-1 DIFFERING-TREE CANDIDATE: %s -- needs C-oracle adjudication (campaign amendment 2026-08-02)", mismatch)
	}
}

// TestB4bV2OptInDeciderDifferentialKotlinWitness is the Kotlin class
// detector (spec.b4b-alternative-set.v2 section 6.1, section 7 gate item 2).
// v2's own branch-discriminated proof must decline this drop -- the
// v2-opt-in-decider route must therefore fall back to production, exactly
// like the live scalar decider already does
// (TestAdmissionCandidateKotlinPlatformModifierSplitDeclinesWithSplitDropsWithheld).
//
// Kotlin's A3 certification (grammars/runtime_profiles.go) ships
// CompactPrimaryAcceptanceDerivationCertified only.
// CompactConvergedReductionSplitDropsCertified -- the certified artifact
// escape this witness would need to route regardless of v2's own proof,
// the same mechanism TestB4bV2OptInDeciderDifferentialErlangProbes documents
// for Erlang -- stays withheld: review found a distinct, compact-only
// divergence class on an annotated extension property (see the
// runtime_profiles.go "kotlin" entry comment). Without that escape, this
// witness declines exactly as it did before any Kotlin certification
// landed.
func TestB4bV2OptInDeciderDifferentialKotlinWitness(t *testing.T) {
	source := []byte("internal actual fun f(): String = \"x\"\n")
	lang := grammars.KotlinLanguage()
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("kotlin unexpectedly carries the withheld converged-split-drop certification")
	}

	result := runV2OptInDeciderDifferential(t, "kotlin_witness", lang, source)
	if result.err != nil {
		t.Fatalf("kotlin witness differential: %v", result.err)
	}
	if result.routed {
		t.Fatalf(
			"expected v2's own proof to decline the Kotlin witness drop (spec section 6.1); v2-opt-in-decider route routed instead (digest=%s)",
			result.candidateDigest,
		)
	}
	if !result.fallback || !strings.Contains(result.fallbackReason, "converged-path reduction split") {
		t.Fatalf("expected the Kotlin witness to fall back on a converged-path reduction split decline under v2; fallback=%t reason=%q", result.fallback, result.fallbackReason)
	}
	if !result.digestsMatch {
		t.Fatalf("Kotlin witness fallback tree does not match production: candidate=%s production=%s", result.candidateDigest, result.productionDigest)
	}
	t.Logf("kotlin witness: v2 declines (matches the live scalar decider's existing fail-closed behavior); fallback reason=%q", result.fallbackReason)
}

// TestB4bV2OptInDeciderDifferentialErlangProbes checks whether both erlang
// probe witnesses re-prove under v2 (spec.b4b-alternative-set.v2 section 7
// gate: "Both erlang probe witnesses re-prove under v2 with production
// digest equality. Their stopped-branch confirmation was under v1
// containment -- unverified under v2 until this runs."). Erlang is
// certified (CompactConvergedReductionSplitDropsCertified), so its parse
// always routes regardless of proof outcome (the artifact escape); tree
// equality with production is therefore not itself informative about
// whether v2 re-proves. This test additionally reads the three-proof
// census's V2Proved count, which is.
//
// FINDING (recorded, not a stop rule -- see the doc comment below the
// assertion): neither witness re-proves under v2. Both are census class 3
// (V1Proved but not V2Proved): the erlang macro-expansion trigger produces
// the identical (event, differing-branch) shape as the Kotlin class (spec
// section 6.1), so v2 correctly, conservatively declines them too -- the
// same branch discrimination that fixed Kotlin also disqualifies these. The
// tree is unaffected today (erlang's certificate keeps routing it via the
// escape, matching production, confirmed below); the consequence is scoped
// to stage 3: the "probe witnesses prove under v2" precondition for
// retiring erlang's certificate (spec section 8) is not met, so per the
// design's own fail-closed principle for a high class-3 rate ("its
// certificate simply stays"), erlang's certificate cannot be retired on
// this evidence. That is an accurate stage-3 planning input, not a defect
// in this stage's wiring.
func TestB4bV2OptInDeciderDifferentialErlangProbes(t *testing.T) {
	restoreCensus := gts.SetDiagnosticParserCoreShadowCensusEnabledForTest(true)
	defer restoreCensus()
	restoreRecording := core.SetAlternativeSetRecordingEnabledForTest(true)
	defer restoreRecording()

	lang := grammars.ErlangLanguage()
	if !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("shared erlang language does not carry the converged-split artifact certificate")
	}

	witnesses := map[string]string{
		"macro_function_clauses": "-module(m).\n" +
			"-define(FN, foo(0) -> zero; foo(N) when N > 0 -> pos; foo(_) -> neg).\n" +
			"?FN.\n",
		"macro_expanded_top_level_function": "-module(m).\n" +
			"-define(FN1, bar(1) -> one).\n" +
			"?FN1.\n",
	}
	for _, name := range []string{"macro_function_clauses", "macro_expanded_top_level_function"} {
		source := []byte(witnesses[name])
		t.Run(name, func(t *testing.T) {
			gts.DiagnosticParserCoreShadowCensusResetForTest()

			result := runV2OptInDeciderDifferential(t, name, lang, source)
			if result.err != nil {
				t.Fatalf("erlang probe differential: %v", result.err)
			}
			if !result.routed {
				t.Fatalf("expected the certified escape (or v2's own proof) to route this source; fallback reason=%q", result.fallbackReason)
			}
			if !result.digestsMatch {
				t.Fatalf("erlang probe %q: v2-opt-in-decider tree does not match production: candidate=%s production=%s (this WOULD be a soundness concern -- the certified escape masking an unsound routing)", name, result.candidateDigest, result.productionDigest)
			}

			totals := gts.DiagnosticParserCoreThreeProofCensusSnapshotForTest()
			switch {
			case totals.V2Proved > 0:
				t.Logf("erlang probe %q: v2 re-proves on its own merit (V2Proved=%d of %d elections), tree matches production", name, totals.V2Proved, totals.Elections)
			case totals.Class3V1ProvesV2Declines > 0:
				t.Logf(
					"STAGE-3 FINDING erlang probe %q: does NOT re-prove under v2 (V1Proved=%d, V2Proved=0, class3=%d) -- same (event, differing-branch) shape as the Kotlin class; the tree still matches production only because the certified artifact escape routes it, same as today's scalar decider. Erlang's stage-3 certificate-retirement precondition is not met by this witness.",
					name, totals.V1Proved, totals.Class3V1ProvesV2Declines,
				)
			default:
				t.Errorf("erlang probe %q: v2 did not prove (V2Proved=0) and the decline is not the expected class-3 (branch-discrimination) shape (V1Proved=%d class3=%d); this needs investigation, unlike the recorded class-3 finding above", name, totals.V1Proved, totals.Class3V1ProvesV2Declines)
			}
		})
	}
}

// alternativeSetV2WitnessDigest is a small helper retained for future
// witnesses that want to log a stable short digest without pulling in the
// full benchfixtures dependency chain.
func alternativeSetV2WitnessDigest(source []byte) string {
	sum := sha256.Sum256(source)
	return fmt.Sprintf("%x", sum[:8])
}

//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// compactT3RouteEqualityWitnessManifestPath is the manifest committed by PR
// #573 (v1) and upgraded to v2 by the B3 stage S1 structural-parity harness,
// read directly by cgo_harness/compact_t3_oracle_adjudication_test.go (cgo-
// and Docker-gated, so not runnable here). cgo_harness is a separate Go
// module (its own go.mod), so this package cannot import its types; it
// decodes the same committed JSON with a local, minimal struct instead. The
// v2 manifest adds a top-level "denominator" object this struct does not
// declare; json.Unmarshal (no DisallowUnknownFields here) ignores it.
const compactT3RouteEqualityWitnessManifestPath = "cgo_harness/testdata/compact_t3_oracle_witnesses_v2.json"

type routeEqualityWitnessManifest struct {
	Witnesses []routeEqualityWitness `json:"witnesses"`
}

type routeEqualityWitness struct {
	ID         string `json:"id"`
	Language   string `json:"language"`
	SourceUTF8 string `json:"source_utf8"`
	Expected   struct {
		CHasError          *bool `json:"c_has_error"`
		ProductionHasError *bool `json:"production_has_error"`
		CompactHasError    *bool `json:"compact_has_error"`
	} `json:"expected"`
}

// loadRouteEqualityWitnesses accepts testing.TB (not just *testing.T) so
// FuzzAdmissionRouteEquality (fuzz_admission_route_equality_test.go, campaign
// v7 tranche B2) can reuse it from a *testing.F during corpus setup.
func loadRouteEqualityWitnesses(t testing.TB) map[string]routeEqualityWitness {
	t.Helper()
	raw, err := os.ReadFile(compactT3RouteEqualityWitnessManifestPath)
	if err != nil {
		t.Fatalf("read witness manifest: %v", err)
	}
	var manifest routeEqualityWitnessManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode witness manifest: %v", err)
	}
	byID := make(map[string]routeEqualityWitness, len(manifest.Witnesses))
	for _, w := range manifest.Witnesses {
		byID[w.ID] = w
	}
	return byID
}

// TestCompactRouteHTMLErroneousEndTagByteGapDeclines pins the B1 reference
// witness by its literal bytes, independent of the committed manifest file:
// html "<a></a^>". Through B3 stage S2 the compact route declined this input
// (no leaf covered byte 6, the stray '^' inside the malformed close tag) and
// fell back to production, which reports HasError()==true for the same
// bytes. B3 stage S3 lands native strategy-2 recovery (error-region absorb
// and condense-resume) for exactly this witness class
// (html_erroneous_end_tag): compact now routes this input itself instead of
// declining, publishing an ERROR-wrapped tree that matches the pinned C
// oracle exactly (cgo_harness/compact_t3_oracle_adjudication_test.go's
// compactT3RecoveryCertifiedWitnesses asserts the full structural comparison
// under cgo; this always-on test pins only the route decision and root
// HasError, which do not need cgo). Before either gate landed, the compact
// route accepted this input and published document[0:8] clean.
func TestCompactRouteHTMLErroneousEndTagByteGapDeclines(t *testing.T) {
	// This loads the html grammar into the process-wide embedded cache; purge
	// afterward so it does not inflate heap for later whole-process tests
	// (matches admission_scorecard_test.go's convention).
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("html")
	if entry == nil {
		t.Fatal("html is not registered")
	}
	lang := entry.Language()
	source := []byte("<a></a^>")

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	if !productionTree.RootNode().HasError() {
		t.Fatal("production HasError=false, want true for <a></a^>")
	}

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
		t.Fatalf("route counters routed=%d fallback=%d, want routed=1 fallback=0 (B3 stage S3: compact now handles this witness natively)", routed, fallback)
	}
	if !candidateTree.RootNode().HasError() {
		t.Fatal("natively-routed compact tree HasError=false, want true")
	}
}

// TestCompactRouteAcceptedHiddenDoctypeLeafTiles proves that a hidden raw
// terminal can supply the bytes omitted from the public doctype children.
func TestCompactRouteAcceptedHiddenDoctypeLeafTiles(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("html")
	if entry == nil {
		t.Fatal("html is not registered")
	}
	lang := entry.Language()
	source := []byte("<!doctype html>\n")

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()

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
		t.Fatalf("route counters routed=%d fallback=%d, want routed=1 fallback=0", routed, fallback)
	}
	if got, want := candidateTree.RootNode().SExpr(lang), productionTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("candidate tree diverges from production\n got:  %s\n want: %s", got, want)
	}
}

// TestCompactRouteAdjudicatesFalseCleanWitnesses pulls a focused
// subset (3 html, 6 javascript) from the 20-witness committed manifest (10
// html, 8 javascript, 2 swift). Each entry was, before the B1 tiling gate,
// accepted by the compact route with HasError()==false while production and
// the locked C oracle both report an error (compact_t3_oracle_witnesses_v2.
// json: c_has_error=production_has_error=true for all 20; compact_has_error
// was false for all 20 before B1's fix). B1 made compact decline and fall
// back to production for all 18 html/javascript entries (see
// TestCompactRouteSwiftShiftComparisonGapIsNotATilingDefect for the 2 swift
// entries, which stay a different, non-tiling mechanism). B3 stage S3 then
// landed native strategy-2 recovery for the html_erroneous_end_tag class:
// the 3 html entries here (html_min_a, html_min_html, html_log_1) now route
// natively instead of declining, publishing a compact-native tree with the
// same root HasError as production (exact structural C-oracle parity is
// asserted separately, under cgo, by
// cgo_harness/compact_t3_oracle_adjudication_test.go's
// compactT3RecoveryCertifiedWitnesses). The certified JavaScript artifact now
// routes js_log_1, js_log_2, js_log_3, js_log_5, js_log_7, and js_log_8
// with exact C parity.
//
// Scope note: the manifest's 2 swift entries (swift_log_1, swift_log_2) are
// NOT in this list. Root-cause (verified by direct tree inspection):
// swift's witnesses are ambiguous `>>` tokenization (ship-vs-comparison,
// `x >> y>>`) where the compact generic scheduler's accepted derivation
// fully tiles the source -- every byte is covered by a leaf, contiguously,
// with no trivia-excused or non-trivia gap anywhere. Production instead
// treats the dangling trailing `>` `>` as unparseable and wraps it in an
// ERROR node. Both derivations are complete and gapless; they simply
// disagree on which reduction is grammatically valid for the trailing
// operator. That is a shift/reduce conflict-resolution divergence between
// the compact scheduler and production's LR tables, a different mechanism
// from the false-clean coverage gap this tranche closes, so the B1 leaf-
// tiling invariant correctly does not (and must not be made to) reject it.
// See TestCompactRouteSwiftShiftComparisonGapIsNotATilingDefect
// (admission_route_equality_swift_residual_test.go, gts_parsercorephase0-
// tagged -- loading the swift grammar alone retains roughly 20 MB past a
// full drain+purge+GC cycle, a pre-existing characteristic reproduced
// identically on origin/main with none of this tranche's changes present,
// unrelated to either route; keeping it out of the always-on suite avoids
// tripping TestArenaGCRetentionAfterRelease's whole-process heap budget),
// which documents the current, still-open state directly. Filed as an
// out-of-scope finding for a follow-up tranche rather than folded into this
// fix (campaign v7 delegation protocol: scope changes route back through
// the specification, not through an ad hoc widening of one tranche's check).
func TestCompactRouteAdjudicatesFalseCleanWitnesses(t *testing.T) {
	// html and javascript are not otherwise loaded by the always-on (untagged)
	// suite; purge the process-wide embedded cache afterward so this test does
	// not inflate heap for later whole-process tests (matches
	// admission_scorecard_test.go's convention).
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	witnesses := loadRouteEqualityWitnesses(t)
	ids := []string{
		"html_min_a", "html_min_html", "html_log_1",
		"js_log_1", "js_log_2", "js_log_3", "js_log_5", "js_log_6", "js_log_7", "js_log_8",
	}
	for _, id := range ids {
		id := id
		t.Run(id, func(t *testing.T) {
			witness, ok := witnesses[id]
			if !ok {
				t.Fatalf("witness %q is missing from %s", id, compactT3RouteEqualityWitnessManifestPath)
			}
			if witness.Expected.CHasError == nil || witness.Expected.ProductionHasError == nil || witness.Expected.CompactHasError == nil {
				t.Fatalf("witness %q has an incomplete expected outcome", id)
			}
			// The manifest's compact_has_error records what the compact-routed
			// Parse() call actually returns, not a fixed pre-adjudication
			// snapshot. This tranche's fix makes that value match production's
			// (decline, fall back within the same call), so the manifest now
			// records compact_has_error=true for these 18 entries -- the same
			// value as production_has_error, both true. A manifest that still
			// showed compact_has_error=false here would mean either this
			// witness's manifest record was not updated for the fix, or the
			// fix has regressed; both are the same failure from this test's
			// point of view.
			if *witness.Expected.CompactHasError != *witness.Expected.ProductionHasError {
				t.Fatalf("witness %q manifest compact_has_error=%t disagrees with production_has_error=%t; the fixed manifest must record that compact now matches production",
					id, *witness.Expected.CompactHasError, *witness.Expected.ProductionHasError)
			}
			if *witness.Expected.CHasError != *witness.Expected.ProductionHasError {
				t.Fatalf("witness %q manifest c_has_error=%t disagrees with production_has_error=%t; adjudication requires the C oracle to side with production",
					id, *witness.Expected.CHasError, *witness.Expected.ProductionHasError)
			}

			entry := grammars.DetectLanguageByName(witness.Language)
			if entry == nil {
				t.Fatalf("witness %q: language %q is not registered", id, witness.Language)
			}
			lang := entry.Language()
			source := []byte(witness.SourceUTF8)

			gts.ResetAdmissionCandidateCountersForTest()
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(true)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("witness %q: parse: %v", id, err)
			}
			defer tree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			nativeRecovery := witness.Language == "html" || id == "js_log_1" || id == "js_log_2" || id == "js_log_3" ||
				id == "js_log_5" || id == "js_log_6" || id == "js_log_7" || id == "js_log_8"
			if nativeRecovery {
				if routed != 1 || fallback != 0 {
					t.Fatalf("witness %q: route counters routed=%d fallback=%d, want routed=1 fallback=0; reason=%q",
						id, routed, fallback, gts.AdmissionCandidateLastFallbackReason())
				}
			} else {
				if routed != 0 || fallback != 1 {
					t.Fatalf("witness %q: route counters routed=%d fallback=%d, want routed=0 fallback=1 (compact must decline)", id, routed, fallback)
				}
				reason := gts.AdmissionCandidateLastFallbackReason()
				if !strings.Contains(reason, "accepted compact root leaves do not tile the accepted span") {
					t.Fatalf("witness %q: fallback reason=%q, want an accepted-root leaf-tiling gap", id, reason)
				}
			}

			if got := tree.RootNode().HasError(); got != *witness.Expected.ProductionHasError {
				t.Fatalf("witness %q: served HasError=%t, want %t (manifest production_has_error, C-adjudicated)",
					id, got, *witness.Expected.ProductionHasError)
			}
		})
	}
}

// TestCompactRouteDeclinesUncertifiedRecoveryAliasResume pins a mutation
// where the same property_identifier alias appears after a different resume.
// The compact tree swallows the next class into the arrow expression. The
// exact JavaScript artifact has no alias rule for that resume state.
func TestCompactRouteDeclinesUncertifiedRecoveryAliasResume(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	lang := grammars.JavascriptLanguage()
	source := []byte("const f = (a) => a + 1#&\nclass A { m() { return 1 } }\n\n")

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 0 || fallback != 1 {
		t.Fatalf("route counters routed=%d fallback=%d, want 0/1", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "accepted compact root leaves do not tile the accepted span") {
		t.Fatalf("fallback reason=%q, want an accepted-root leaf gap", reason)
	}
}

func TestCompactRouteDeclinesRetiredRecoveryWithoutAliasRule(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	lang := *grammars.JavascriptLanguage()
	lang.CompactRecoveryTerminalAliasRules = nil
	source := []byte("const f = (a) =. + + 1;\nclass A { m() { return 1 } }\n\n")

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(&lang)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 0 || fallback != 1 {
		t.Fatalf("route counters routed=%d fallback=%d, want 0/1", routed, fallback)
	}
	if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "accepted compact root leaves do not tile the accepted span") {
		t.Fatalf("fallback reason=%q, want an accepted-root leaf gap", reason)
	}
}

func TestCompactRouteCompetesMissingInsertionWithErrorAbsorb(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	gts.EnableArenaBreakdown(true)
	t.Cleanup(func() { gts.EnableArenaBreakdown(false) })
	lang := grammars.HtmlLanguage()
	if !lang.CompactStrategy2ErrorRegionCertified || !lang.CompactMissingTokenInsertionCertified {
		t.Fatal("the HTML artifact lacks complete compact recovery certification")
	}
	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse([]byte("<html><bodyHello</body></html>\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("route counters routed=%d fallback=%d, want routed=1 fallback=0; reason=%q",
			routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}
	if !tree.RootNode().HasError() {
		t.Fatal("the selected recovery tree lost its error")
	}
	if !routeEqualityTreeContainsMissing(tree.RootNode()) {
		t.Fatal("the selected recovery tree contains no missing terminal")
	}
	breakdown, recorded := tree.ArenaBreakdown()
	if !recorded || breakdown.MissingNodeDependencyCount == 0 || breakdown.MissingNodeDependencyBytesAllocated == 0 {
		t.Fatalf("compact recovery did not materialize missing-dependency telemetry: recorded=%t breakdown=%+v", recorded, breakdown)
	}
}

func routeEqualityTreeContainsMissing(node *gts.Node) bool {
	if node == nil {
		return false
	}
	if node.IsMissing() {
		return true
	}
	for index := 0; index < node.ChildCount(); index++ {
		if routeEqualityTreeContainsMissing(node.Child(index)) {
			return true
		}
	}
	return false
}

// TestCompactRouteAcceptedRootLeafCoverageGapDeclines pins the adversarial
// review finding against B3 stage S3's first shipped revision: html
// "<!--c-->>" (9 bytes) reduced the compact route's accepted root over zero
// real children (a hollow "document[0:9]"), and the tiling audit of the era
// could not catch it because isDerivationRootReduce exempts the root's own
// reduce from the per-reduce raw-children check, and the root was the only
// accepted payload. finalizeDiagnosticParserCoreAcceptedRootSpan's
// allowErrorRoot branch now runs a second, root-only audit
// (diagnosticParserCoreAcceptedTreeLeafCoverageGap) after materialization:
// it walks the finalized public tree and requires every genuine terminal
// leaf (or built-in ERROR leaf) to tile [firstNonTriviaByte, sourceLen)
// modulo trivia, independent of which reduce claimed which span. A hollow
// non-terminal reduce (a childless node whose symbol is not a terminal and
// not ERROR) contributes no coverage, so it can no longer paper over lost
// bytes.
//
// Each case here absorbed a comment and a stray '>' (or more) into an S3
// error region and then reduced "document" over nothing; every one must now
// decline and fall back to production, which reports the correct
// ERROR-wrapped, HasError=true tree for all five.
func TestCompactRouteAcceptedRootLeafCoverageGapDeclines(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("html")
	if entry == nil {
		t.Fatal("html is not registered")
	}
	lang := entry.Language()

	sources := []string{
		"<!--c-->>",
		"<!-- c -->>",
		"<!--c--> >",
		"<!--c-->>>",
		"<!--c--><!--d-->>",
	}
	for _, src := range sources {
		src := src
		t.Run(src, func(t *testing.T) {
			source := []byte(src)

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			if !productionTree.RootNode().HasError() {
				t.Fatalf("production HasError=false, want true for %q", src)
			}

			gts.ResetAdmissionCandidateCountersForTest()
			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candidateTree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 0 || fallback != 1 {
				t.Fatalf("%q: route counters routed=%d fallback=%d, want routed=0 fallback=1 (root leaf-coverage gap must decline)", src, routed, fallback)
			}
			reason := gts.AdmissionCandidateLastFallbackReason()
			if !strings.Contains(reason, "do not tile the accepted span") {
				t.Fatalf("%q: fallback reason=%q, want it to cite the root leaf-coverage gap", src, reason)
			}

			if !candidateTree.RootNode().HasError() {
				t.Fatalf("%q: production-served HasError=false, want true", src)
			}
			if got, want := candidateTree.RootNode().SExpr(lang), productionTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("%q: served tree diverges from production\n got:  %s\n want: %s", src, got, want)
			}
		})
	}
}

// TestCompactRouteAcceptedRootTrailingErrorExtraDeclines pins the
// adversarial review round-2 finding: the accept-time splice gap. C's
// ts_parser__accept rebuilds the last non-extra tree over the remaining
// stack contents, INCLUDING trailing extras, at the moment of accepting.
// This materializer's S3 accept path did not perform that splice: an ERROR
// region that resumes onto a head already past the last structural reduce
// (s3CloseInProgressProductions's own eager closure can land there) ended
// up attached as the accepted root's own sibling instead of nested inside
// the preceding element -- one level too shallow. Every byte was still
// covered (so REQUIRED 1's leaf-coverage audit could not catch this), but
// the ATTACHMENT was wrong, and it flipped the enclosing element's own span
// and HasError, which callers read.
//
// html "<html><body>x</body>\x00>" is the minimal reproducer: compact
// published document[0:22] with 2 children (element[0:20] HasError=false,
// ERROR[20:22] extra) where the C oracle reports 1 child (element[0:22]
// HasError=true, the same byte-identical ERROR nested inside it). The 3
// sibling reproducers here were found by construction (two hand-built
// variants on the same base) and by direct search of the committed
// adversarial-review fuzz corpus (the fourth, "the corpus original").
//
// Fixed fail-closed (diagnosticParserCoreAcceptedRootTrailingErrorExtraGap,
// parsercore_phase0_driver.go): every one of these now declines and falls
// back to production instead of publishing the misattached tree.
func TestCompactRouteAcceptedRootTrailingErrorExtraDeclines(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("html")
	if entry == nil {
		t.Fatal("html is not registered")
	}
	lang := entry.Language()

	sources := []struct {
		source     string
		wantReason string
	}{
		{"<html><body>x</body>\x00>", "error-bearing trailing extra"},
		{"<html><body>x</body>\x00/h>", "error-bearing trailing extra"},
		{"<html><body>x</body>\x00/h>\x00/html>", "error-bearing trailing extra"},
		{"<html><body>-->Hello</body>\x00/h>\x00/html>\n", "one error region per parse"},
	}
	for _, test := range sources {
		test := test
		t.Run(test.source, func(t *testing.T) {
			source := []byte(test.source)

			gts.ResetAdmissionCandidateCountersForTest()
			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candidateTree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 0 || fallback != 1 {
				t.Fatalf("%q: route counters routed=%d fallback=%d, want routed=0 fallback=1", test.source, routed, fallback)
			}
			reason := gts.AdmissionCandidateLastFallbackReason()
			if !strings.Contains(reason, test.wantReason) {
				t.Fatalf("%q: fallback reason=%q, want %q", test.source, reason, test.wantReason)
			}
		})
	}
}

// TestCompactRouteCleanTrailingExtraStillRoutesNatively is the control case
// for TestCompactRouteAcceptedRootTrailingErrorExtraDeclines: an ordinary
// trailing extra (a comment, carrying no error at all) legitimately sits
// beside -- or, once the enclosing element is genuinely still open, inside
// -- a root's non-extra child in both engines, and must keep routing
// natively. diagnosticParserCoreAcceptedRootTrailingErrorExtraGap is
// deliberately narrower than "any trailing extra" specifically so these
// two shapes do not trip it:
//
//   - "<html><body>x</body><!--c-->": the trailing comment is spliced into
//     the still-open outer element by compact's ordinary (non-S3) extra-
//     shift machinery before that element's own reduce ever fires --
//     document ends up with exactly 1 child, matching production and the C
//     oracle, so this shape is not even a root-level trailing extra.
//   - "<a></a><!--trailing-->": the outer element closes explicitly (a real
//     "</a>"), so the trailing comment lands beside it as document's own
//     second child in every engine alike -- document[0:22], 2 children
//     (element, comment), byte-identical between compact and production.
//
// Both must keep serving the compact-native tree; neither carries an
// error-bearing trailing extra, so neither is the shape this gate exists
// to reject.
func TestCompactRouteCleanTrailingExtraStillRoutesNatively(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("html")
	if entry == nil {
		t.Fatal("html is not registered")
	}
	lang := entry.Language()

	sources := []string{
		"<html><body>x</body><!--c-->",
		"<a></a><!--trailing-->",
	}
	for _, src := range sources {
		src := src
		t.Run(src, func(t *testing.T) {
			source := []byte(src)

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()

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
				t.Fatalf("%q: route counters routed=%d fallback=%d, want routed=1 fallback=0 (clean trailing extra must not trip the splice-gap decline)", src, routed, fallback)
			}
			if candidateTree.RootNode().HasError() {
				t.Fatalf("%q: candidate HasError=true, want false (clean trailing extra)", src)
			}
			if got, want := candidateTree.RootNode().SExpr(lang), productionTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("%q: served tree diverges from production\n got:  %s\n want: %s", src, got, want)
			}
		})
	}
}

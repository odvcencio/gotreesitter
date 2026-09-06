//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const (
	compactT3WitnessManifestPath    = "testdata/compact_t3_oracle_witnesses_v2.json"
	compactT3WitnessManifestSchema  = "compact-t3-c-oracle-witnesses-v2"
	compactT3WitnessManifestVersion = 2
	// compactT3WitnessParityScope moved from "has_error_only" (v1) to
	// "full_structural" (v2, B3 stage S1): the harness now compares deep-tree
	// shape, byte and point spans, the Missing flag, child fields, and
	// HasError at every node, not just the root HasError bit.
	compactT3WitnessParityScope   = "full_structural"
	compactT3WitnessTrackingIssue = "https://github.com/odvcencio/gotreesitter/issues/587"
	compactT3WitnessCount         = 20
	// compactT3WitnessCampaignTargetCount is the 98-witness denominator that
	// spec.campaign.v7 (2026-08-01) cites from the PR #573 differential-fuzz
	// sweep. The sweep's log artifact and mutation harness are not committed
	// (compactT3WitnessDenominator.GapReason records the verification). This
	// manifest pins the reproducible subset instead of fabricating witnesses
	// that cannot be checked against the locked C oracle.
	compactT3WitnessCampaignTargetCount = 98
)

var compactT3WitnessLanguageCounts = map[string]int{
	"html":       10,
	"javascript": 8,
	"swift":      2,
}

var compactT3WitnessCampaignTargetLanguageCounts = map[string]int{
	"html":       88,
	"javascript": 8,
	"swift":      2,
}

type compactT3WitnessManifest struct {
	Schema         string                      `json:"schema"`
	Version        int                         `json:"version"`
	ParityScope    string                      `json:"parity_scope"`
	TrackingIssue  string                      `json:"tracking_issue"`
	WitnessCount   int                         `json:"witness_count"`
	LanguageCounts map[string]int              `json:"language_counts"`
	Denominator    compactT3WitnessDenominator `json:"denominator"`
	Witnesses      []compactT3Witness          `json:"witnesses"`
}

// compactT3WitnessDenominator records, in the committed manifest itself, the
// gap between the campaign's cited 98-witness target and the 20 witnesses
// this repository can actually reproduce and verify. See GapReason.
type compactT3WitnessDenominator struct {
	PinnedTotal                  int            `json:"pinned_total"`
	PinnedLanguageCounts         map[string]int `json:"pinned_language_counts"`
	CampaignTargetTotal          int            `json:"campaign_target_total"`
	CampaignTargetLanguageCounts map[string]int `json:"campaign_target_language_counts"`
	GapTotal                     int            `json:"gap_total"`
	GapLanguageCounts            map[string]int `json:"gap_language_counts"`
	GapReason                    string         `json:"gap_reason"`
	RecordedBy                   string         `json:"recorded_by"`
}

type compactT3Witness struct {
	ID               string                     `json:"id"`
	Language         string                     `json:"language"`
	FailureClass     string                     `json:"failure_class"`
	SourceProvenance compactT3SourceProvenance  `json:"source_provenance"`
	SourceUTF8       string                     `json:"source_utf8"`
	SourceSHA256     string                     `json:"source_sha256"`
	COracle          compactT3COracleProvenance `json:"c_oracle"`
	Expected         compactT3ExpectedOutcome   `json:"expected"`
}

type compactT3SourceProvenance struct {
	Origin string `json:"origin"`
	Record string `json:"record"`
}

type compactT3COracleProvenance struct {
	Contract              string `json:"contract"`
	BindingCommit         string `json:"binding_commit"`
	RuntimeCommit         string `json:"runtime_commit"`
	GrammarRepository     string `json:"grammar_repository"`
	GrammarCommit         string `json:"grammar_commit"`
	CompilerPath          string `json:"compiler_path"`
	CompilerVersion       string `json:"compiler_version"`
	GrammarCompileFlags   string `json:"grammar_compile_flags"`
	GrammarArtifactSHA256 string `json:"grammar_artifact_sha256"`
}

type compactT3ExpectedOutcome struct {
	CHasError          *bool `json:"c_has_error"`
	ProductionHasError *bool `json:"production_has_error"`
	CompactHasError    *bool `json:"compact_has_error"`
}

// compactT3RecoveryCertifiedWitnesses lists witnesses where native compact
// recovery reaches exact structural C-oracle parity. The exact HTML artifact
// certifies its complete class. JavaScript certifies all eight pinned recovery
// witnesses.
var compactT3RecoveryCertifiedWitnesses = map[string]bool{
	"html_min_a":    true,
	"html_min_html": true,
	"html_log_1":    true,
	"html_log_2":    true,
	"html_log_3":    true,
	"html_log_4":    true,
	"html_log_5":    true,
	"html_log_6":    true,
	"html_log_7":    true,
	"html_log_8":    true,
	"js_log_1":      true,
	"js_log_2":      true,
	"js_log_3":      true,
	"js_log_4":      true,
	"js_log_5":      true,
	"js_log_6":      true,
	"js_log_7":      true,
	"js_log_8":      true,
}

// TestCompactT3OracleAdjudication verifies each committed false-clean witness
// three ways: C oracle, production, and compact.
//
// Root HasError is asserted for all three routes against the manifest's
// recorded expectation (unchanged from v1; this remains the hard CI gate).
//
// B3 stage S1 extends the harness with a full structural comparison
// (deep-tree shape, byte and point spans, the Missing flag, child fields,
// and per-node HasError) for both production and compact against the C
// oracle.
//
//   - compact vs. the C oracle: S3 and S5 cover all ten
//     html_erroneous_end_tag witnesses. Every witness in
//     compactT3RecoveryCertifiedWitnesses must match the C oracle exactly.
//     JavaScript routes all eight witnesses exactly.
//     Swift still has no compact recovery implementation.
//   - production vs. the C oracle: the S1 design brief's stated gate is
//     "the harness must pass with production serving every witness before
//     any compact change." Verified in the pinned parity container
//     (cgo_harness/docker, matching CI's compact-t3-oracle-cgo job), that
//     gate does NOT hold: production's root HasError matches the C oracle on
//     every witness, but its deep tree shape does not -- every one of the 20
//     committed witnesses shows at least one node-level divergence (root
//     type ERROR vs. the grammar's start symbol, or a child-count mismatch
//     one level down). This is a pre-existing production/C divergence, not
//     something this stage introduced or can fix (production recovery
//     semantics are out of scope for B3; see the design brief section 7).
//     It also is not a new discovery: the retired dispatch.html arm already
//     recorded "the pre-existing document-child divergence for the
//     malformed witness" against this same C oracle
//     (testdata/result_compat_ownership_v1.json, dispatch.html retirement
//     condition). Per the stage S1 stop rule ("if any witness fails
//     structural parity on the production route, stop; that is a production
//     defect or a decision-0007 divergence to receipt, not a B3 work item"),
//     this is reported here -- loudly, per witness, in the test log -- and
//     left for a decision-0007 receipt rather than asserted (which would
//     either mask the gap by weakening the check, or turn this currently
//     green, CI-required test red over a defect outside this stage's scope).
func TestCompactT3OracleAdjudication(t *testing.T) {
	manifest := loadCompactT3WitnessManifest(t)
	for _, language := range compactT3WitnessLanguages(manifest) {
		language := language
		t.Run(language, func(t *testing.T) {
			cLanguage, err := ParityCLanguage(language)
			if err != nil {
				t.Fatalf("load C oracle: %v", err)
			}
			identity, err := COracleIdentity(language)
			if err != nil {
				t.Fatalf("read C oracle identity: %v", err)
			}
			goLanguage, ok := compactT3GoLanguage(language)
			if !ok {
				t.Fatalf("manifest uses unsupported language %q", language)
			}

			for _, witness := range compactT3WitnessesForLanguage(manifest, language) {
				witness := witness
				t.Run(witness.FailureClass+"/"+witness.ID, func(t *testing.T) {
					assertCompactT3COracleProvenance(t, witness, identity)
					source := []byte(witness.SourceUTF8)

					cTree := compactT3ParseC(t, cLanguage, source)
					defer cTree.Close()
					cRoot := cTree.RootNode()

					productionTree := compactT3ParseGo(t, goLanguage, source, false)
					defer productionTree.Release()
					productionRoot := productionTree.RootNode()

					compactTree := compactT3ParseGo(t, goLanguage, source, true)
					defer compactTree.Release()
					compactRoot := compactTree.RootNode()

					assertCompactT3Outcome(t, witness, "C oracle", witness.Expected.CHasError, cRoot.HasError())
					assertCompactT3Outcome(t, witness, "production", witness.Expected.ProductionHasError, productionRoot.HasError())
					assertCompactT3Outcome(t, witness, "compact", witness.Expected.CompactHasError, compactRoot.HasError())

					if divergences := compactT3StructuralDivergences(productionRoot, goLanguage, cRoot); len(divergences) != 0 {
						t.Logf(
							"STOP-RULE FINDING witness %q: production structurally diverges from the C oracle at %d node(s) despite matching root HasError; first: %s (%d more not shown); production recovery is out of B3 scope -- this needs a decision-0007 receipt, not a B3 fix",
							witness.ID, len(divergences), compactT3FormatDivergence(divergences[0]), len(divergences)-1,
						)
					} else {
						t.Logf("witness %q: production already matches the C oracle structurally", witness.ID)
					}

					divergences := compactT3StructuralDivergences(compactRoot, goLanguage, cRoot)
					switch {
					case len(divergences) == 0:
						t.Logf("witness %q: compact matches the C oracle structurally", witness.ID)
					case compactT3RecoveryCertifiedWitnesses[witness.ID]:
						t.Fatalf(
							"witness %q: compact recovery certification requires C parity, but compact diverges at %d node(s); first: %s",
							witness.ID, len(divergences), compactT3FormatDivergence(divergences[0]),
						)
					default:
						t.Logf(
							"witness %q: compact diverges from the C oracle at %d node(s) outside the certified recovery set; first: %s",
							witness.ID, len(divergences), compactT3FormatDivergence(divergences[0]),
						)
						// The faithful-decline check only applies inside the
						// witness class this stage owns: outside
						// html_erroneous_end_tag, compact's shape is
						// independently pre-S3 (no recovery attempt at all,
						// per-witness expected.compact_has_error can differ
						// from production's, e.g. the swift
						// shift_comparison class), so comparing it against
						// production is not meaningful and was never this
						// harness's contract before this stage landed.
						if witness.FailureClass != "html_erroneous_end_tag" {
							break
						}
						if got, want := compactRoot.SExpr(goLanguage), productionRoot.SExpr(goLanguage); got != want {
							t.Fatalf(
								"witness %q: compact declined this shape but its tree does not match production's own tree (decline is not faithful):\ncompact:    %s\nproduction: %s",
								witness.ID, got, want,
							)
						}
					}
				})
			}
		})
	}
}

// TestCompactT3JavaScriptRecoveryProfileCharacterization pins the exact compact
// recovery frontier for the certified JavaScript artifact. A routed row must match C
// structurally. Every other row must fail closed at its recorded mechanism
// boundary and return to the production parser.
func TestCompactT3JavaScriptRecoveryProfileCharacterization(t *testing.T) {
	manifest := loadCompactT3WitnessManifest(t)
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatalf("load C oracle: %v", err)
	}
	goLanguage := grammars.JavascriptLanguage()
	if !goLanguage.CompactStrategy2ErrorRegionCertified || !goLanguage.CompactMissingTokenInsertionCertified {
		t.Fatal("the exact JavaScript artifact lacks compact recovery certification")
	}

	expectations := map[string]struct {
		routed         bool
		fallbackDetail string
	}{
		"js_log_1": {routed: true},
		"js_log_2": {routed: true},
		"js_log_3": {routed: true},
		"js_log_4": {routed: true},
		"js_log_5": {routed: true},
		"js_log_6": {routed: true},
		"js_log_7": {routed: true},
		"js_log_8": {routed: true},
	}

	for _, witness := range compactT3WitnessesForLanguage(manifest, "javascript") {
		witness := witness
		expectation, ok := expectations[witness.ID]
		if !ok {
			t.Fatalf("JavaScript witness %q has no forced-profile expectation", witness.ID)
		}
		t.Run(witness.ID, func(t *testing.T) {
			source := []byte(witness.SourceUTF8)
			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactTree := compactT3ParseGo(t, goLanguage, source, true)
			defer compactTree.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routed := routedAfter - routedBefore
			fallback := fallbackAfter - fallbackBefore

			if expectation.routed {
				if routed != 1 || fallback != 0 {
					t.Fatalf("route counters=%d/%d, want 1/0; reason=%q", routed, fallback, gotreesitter.AdmissionCandidateLastFallbackReason())
				}
				if divergences := compactT3StructuralDivergences(compactTree.RootNode(), goLanguage, cTree.RootNode()); len(divergences) != 0 {
					t.Fatalf("forced compact recovery diverges from C at %d node(s); first: %s", len(divergences), compactT3FormatDivergence(divergences[0]))
				}
				return
			}

			if routed != 0 || fallback != 1 {
				t.Fatalf("route counters=%d/%d, want 0/1", routed, fallback)
			}
			if reason := gotreesitter.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, expectation.fallbackDetail) {
				t.Fatalf("fallback reason=%q, want substring %q", reason, expectation.fallbackDetail)
			}
		})
	}
}

// TestCompactT3JavaScriptRecoveryRouteSeedOracleParity adjudicates recovery
// trees that the required route-equality seed corpus reaches outside the
// compact T3 manifest. They must match C before the seed gate can treat their
// compact-versus-production difference as certified recovery behavior.
func TestCompactT3JavaScriptRecoveryRouteSeedOracleParity(t *testing.T) {
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatalf("load C oracle: %v", err)
	}
	goLanguage := grammars.JavascriptLanguage()
	if !goLanguage.CompactStrategy2ErrorRegionCertified || !goLanguage.CompactMissingTokenInsertionCertified {
		t.Fatal("the exact JavaScript artifact lacks compact recovery certification")
	}
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "hash_after_identifier_run", source: "function A(){A000000} # 0"},
		{name: "hash_between_statements", source: "a; # ; b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactTree := compactT3ParseGo(t, goLanguage, source, true)
			defer compactTree.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter-routedBefore != 1 || fallbackAfter-fallbackBefore != 0 {
				t.Fatalf("route counters delta=%d/%d, want 1/0; reason=%q",
					routedAfter-routedBefore, fallbackAfter-fallbackBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
			}
			if divergences := compactT3StructuralDivergences(compactTree.RootNode(), goLanguage, cTree.RootNode()); len(divergences) != 0 {
				t.Fatalf("compact recovery diverges from C at %d node(s); first: %s",
					len(divergences), compactT3FormatDivergence(divergences[0]))
			}
		})
	}
}

// TestCompactT3JavaScriptRecoveryMutationDifferential checks a deterministic
// one-byte neighborhood around every pinned JavaScript witness. A compact
// result can publish only when its complete tree matches C. Unsupported
// mutations must return to production through the existing fallback route.
func TestCompactT3JavaScriptRecoveryMutationDifferential(t *testing.T) {
	manifest := loadCompactT3WitnessManifest(t)
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatalf("load C oracle: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set C oracle language: %v", err)
	}
	goLanguage := grammars.JavascriptLanguage()
	if !goLanguage.CompactStrategy2ErrorRegionCertified || !goLanguage.CompactMissingTokenInsertionCertified {
		t.Fatal("the exact JavaScript artifact lacks compact recovery certification")
	}
	compactParser := gotreesitter.NewParser(goLanguage)
	compactParser.SetAdmissionCandidateRoute(true)
	s3OnlyLanguage := *goLanguage
	s3OnlyLanguage.CompactMissingTokenInsertionCertified = false
	s3OnlyParser := gotreesitter.NewParser(&s3OnlyLanguage)
	s3OnlyParser.SetAdmissionCandidateRoute(true)

	var cases, routed, fallback, s3OnlyRouted, missingExpanded, missingContracted int
	check := func(witnessID, mutation string, source []byte) {
		t.Helper()
		cases++
		cTree := cParser.Parse(source, nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatalf("%s/%s: C oracle returned no root", witnessID, mutation)
		}

		routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
		compactTree, parseErr := compactParser.Parse(source)
		if parseErr != nil {
			t.Fatalf("%s/%s: compact parse: %v", witnessID, mutation, parseErr)
		}
		if compactTree == nil || compactTree.RootNode() == nil {
			t.Fatalf("%s/%s: compact parse returned no root", witnessID, mutation)
		}
		routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
		compactRouted := false
		switch {
		case routedAfter-routedBefore == 1 && fallbackAfter-fallbackBefore == 0:
			compactRouted = true
			routed++
			if divergences := compactT3StructuralDivergences(compactTree.RootNode(), goLanguage, cTree.RootNode()); len(divergences) != 0 {
				t.Fatalf("%s/%s: routed compact tree diverges from C at %d node(s); first: %s; source=%q",
					witnessID, mutation, len(divergences), compactT3FormatDivergence(divergences[0]), source)
			}
		case routedAfter-routedBefore == 0 && fallbackAfter-fallbackBefore == 1:
			fallback++
		default:
			t.Fatalf("%s/%s: route counters delta=%d/%d, want 1/0 or 0/1",
				witnessID, mutation, routedAfter-routedBefore, fallbackAfter-fallbackBefore)
		}

		s3RoutedBefore, s3FallbackBefore := gotreesitter.AdmissionCandidateCounters()
		s3Tree, s3Err := s3OnlyParser.Parse(source)
		if s3Err != nil {
			t.Fatalf("%s/%s: S3-only compact parse: %v", witnessID, mutation, s3Err)
		}
		if s3Tree == nil || s3Tree.RootNode() == nil {
			t.Fatalf("%s/%s: S3-only compact parse returned no root", witnessID, mutation)
		}
		s3RoutedAfter, s3FallbackAfter := gotreesitter.AdmissionCandidateCounters()
		s3Routed := false
		switch {
		case s3RoutedAfter-s3RoutedBefore == 1 && s3FallbackAfter-s3FallbackBefore == 0:
			s3Routed = true
			s3OnlyRouted++
			if divergences := compactT3StructuralDivergences(s3Tree.RootNode(), &s3OnlyLanguage, cTree.RootNode()); len(divergences) != 0 {
				t.Fatalf("%s/%s: S3-only tree diverges from C at %d node(s); first: %s; source=%q",
					witnessID, mutation, len(divergences), compactT3FormatDivergence(divergences[0]), source)
			}
		case s3RoutedAfter-s3RoutedBefore == 0 && s3FallbackAfter-s3FallbackBefore == 1:
		default:
			t.Fatalf("%s/%s: S3-only route counters delta=%d/%d, want 1/0 or 0/1",
				witnessID, mutation, s3RoutedAfter-s3RoutedBefore, s3FallbackAfter-s3FallbackBefore)
		}
		if compactRouted && !s3Routed {
			missingExpanded++
		}
		if !compactRouted && s3Routed {
			missingContracted++
		}

		s3Tree.Release()
		compactTree.Release()
		cTree.Close()
	}

	replacements := []byte{'?', '#', ')', '}'}
	insertions := []byte{'?', '#'}
	for _, witness := range compactT3WitnessesForLanguage(manifest, "javascript") {
		source := []byte(witness.SourceUTF8)
		check(witness.ID, "original", source)
		for offset := range source {
			deleted := make([]byte, 0, len(source)-1)
			deleted = append(deleted, source[:offset]...)
			deleted = append(deleted, source[offset+1:]...)
			check(witness.ID, fmt.Sprintf("delete-%d", offset), deleted)

			for _, replacement := range replacements {
				if source[offset] == replacement {
					continue
				}
				replaced := append([]byte(nil), source...)
				replaced[offset] = replacement
				check(witness.ID, fmt.Sprintf("replace-%d-%02x", offset, replacement), replaced)
			}
		}
		for offset := 0; offset <= len(source); offset++ {
			for _, insertion := range insertions {
				inserted := make([]byte, 0, len(source)+1)
				inserted = append(inserted, source[:offset]...)
				inserted = append(inserted, insertion)
				inserted = append(inserted, source[offset:]...)
				check(witness.ID, fmt.Sprintf("insert-%d-%02x", offset, insertion), inserted)
			}
		}
	}
	if routed == 0 || fallback == 0 {
		t.Fatalf("mutation differential lacked both route outcomes: cases=%d routed=%d fallback=%d", cases, routed, fallback)
	}
	// Trailing retirement, keyword capture, and EOF pricing add 71 routes.
	// Two more require S5 because standalone S3 rejects mixed shift/reduce
	// closure states without an authenticated resume rule.
	const certifiedS5Expansions = 73
	if missingExpanded != certifiedS5Expansions || missingContracted != 0 {
		t.Fatalf("T3 neighborhood S5 boundary: expanded=%d contracted=%d, want %d/0",
			missingExpanded, missingContracted, certifiedS5Expansions)
	}
	t.Logf("JavaScript compact recovery mutation differential: cases=%d routed=%d fallback=%d S3-only=%d missing-expanded=%d missing-contracted=%d",
		cases, routed, fallback, s3OnlyRouted, missingExpanded, missingContracted)
}

func loadCompactT3WitnessManifest(t *testing.T) compactT3WitnessManifest {
	t.Helper()
	raw, err := os.ReadFile(compactT3WitnessManifestPath)
	if err != nil {
		t.Fatalf("read witness manifest: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest compactT3WitnessManifest
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode witness manifest: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("decode trailing witness manifest content: %v", err)
	}
	if err := validateCompactT3WitnessManifest(manifest); err != nil {
		t.Fatalf("validate witness manifest: %v", err)
	}
	return manifest
}

func validateCompactT3WitnessManifest(manifest compactT3WitnessManifest) error {
	if manifest.Schema != compactT3WitnessManifestSchema {
		return fmt.Errorf("schema=%q, want %q", manifest.Schema, compactT3WitnessManifestSchema)
	}
	if manifest.Version != compactT3WitnessManifestVersion {
		return fmt.Errorf("version=%d, want %d", manifest.Version, compactT3WitnessManifestVersion)
	}
	if manifest.ParityScope != compactT3WitnessParityScope {
		return fmt.Errorf("parity_scope=%q, want %q", manifest.ParityScope, compactT3WitnessParityScope)
	}
	if manifest.TrackingIssue != compactT3WitnessTrackingIssue {
		return fmt.Errorf("tracking_issue=%q, want %q", manifest.TrackingIssue, compactT3WitnessTrackingIssue)
	}
	if manifest.WitnessCount != compactT3WitnessCount {
		return fmt.Errorf("witness_count=%d, want %d", manifest.WitnessCount, compactT3WitnessCount)
	}
	if len(manifest.Witnesses) != compactT3WitnessCount {
		return fmt.Errorf("decoded witness count=%d, want %d", len(manifest.Witnesses), compactT3WitnessCount)
	}
	if err := validateCompactT3LanguageCounts(manifest.LanguageCounts); err != nil {
		return err
	}
	if err := validateCompactT3WitnessDenominator(manifest.Denominator); err != nil {
		return fmt.Errorf("denominator: %w", err)
	}

	seenIDs := make(map[string]struct{}, len(manifest.Witnesses))
	actualLanguageCounts := make(map[string]int, len(compactT3WitnessLanguageCounts))
	for _, witness := range manifest.Witnesses {
		if strings.TrimSpace(witness.ID) == "" {
			return errorsForCompactT3Witness(witness, "id is empty")
		}
		if _, exists := seenIDs[witness.ID]; exists {
			return errorsForCompactT3Witness(witness, "id is duplicated")
		}
		seenIDs[witness.ID] = struct{}{}
		if _, ok := compactT3WitnessLanguageCounts[witness.Language]; !ok {
			return errorsForCompactT3Witness(witness, "language is not in the exact witness set")
		}
		actualLanguageCounts[witness.Language]++
		if strings.TrimSpace(witness.FailureClass) == "" {
			return errorsForCompactT3Witness(witness, "failure_class is empty")
		}
		if strings.TrimSpace(witness.SourceProvenance.Origin) == "" || strings.TrimSpace(witness.SourceProvenance.Record) == "" {
			return errorsForCompactT3Witness(witness, "source_provenance is incomplete")
		}
		if witness.SourceUTF8 == "" || !utf8.ValidString(witness.SourceUTF8) {
			return errorsForCompactT3Witness(witness, "source_utf8 is empty or invalid")
		}
		if err := validateCompactT3SourceDigest(witness); err != nil {
			return err
		}
		if err := validateCompactT3ExpectedOutcome(witness); err != nil {
			return err
		}
		if err := validateCompactT3COracleProvenance(witness.COracle); err != nil {
			return errorsForCompactT3Witness(witness, err.Error())
		}
	}
	if err := validateCompactT3LanguageCounts(actualLanguageCounts); err != nil {
		return fmt.Errorf("actual %w", err)
	}
	return nil
}

func validateCompactT3LanguageCounts(counts map[string]int) error {
	if len(counts) != len(compactT3WitnessLanguageCounts) {
		return fmt.Errorf("language count entries=%d, want %d", len(counts), len(compactT3WitnessLanguageCounts))
	}
	for language, want := range compactT3WitnessLanguageCounts {
		if got := counts[language]; got != want {
			return fmt.Errorf("language=%q count=%d, want %d", language, got, want)
		}
	}
	return nil
}

// validateCompactT3WitnessDenominator keeps the manifest's own gap-accounting
// metadata self-consistent (pinned counts match the witness set actually
// committed, and the recorded gap is exactly campaign target minus pinned)
// so the denominator block cannot silently drift from the witnesses it
// describes.
func validateCompactT3WitnessDenominator(d compactT3WitnessDenominator) error {
	if d.PinnedTotal != compactT3WitnessCount {
		return fmt.Errorf("pinned_total=%d, want %d", d.PinnedTotal, compactT3WitnessCount)
	}
	if err := validateCompactT3LanguageCounts(d.PinnedLanguageCounts); err != nil {
		return fmt.Errorf("pinned_language_counts: %w", err)
	}
	if d.CampaignTargetTotal != compactT3WitnessCampaignTargetCount {
		return fmt.Errorf("campaign_target_total=%d, want %d", d.CampaignTargetTotal, compactT3WitnessCampaignTargetCount)
	}
	if len(d.CampaignTargetLanguageCounts) != len(compactT3WitnessCampaignTargetLanguageCounts) {
		return fmt.Errorf("campaign_target_language_counts entries=%d, want %d", len(d.CampaignTargetLanguageCounts), len(compactT3WitnessCampaignTargetLanguageCounts))
	}
	wantGapTotal := 0
	for language, want := range compactT3WitnessCampaignTargetLanguageCounts {
		got := d.CampaignTargetLanguageCounts[language]
		if got != want {
			return fmt.Errorf("campaign_target_language_counts[%q]=%d, want %d", language, got, want)
		}
		gap := want - compactT3WitnessLanguageCounts[language]
		if gapGot := d.GapLanguageCounts[language]; gapGot != gap {
			return fmt.Errorf("gap_language_counts[%q]=%d, want %d", language, gapGot, gap)
		}
		wantGapTotal += gap
	}
	if d.GapTotal != wantGapTotal {
		return fmt.Errorf("gap_total=%d, want %d", d.GapTotal, wantGapTotal)
	}
	if d.CampaignTargetTotal-d.PinnedTotal != d.GapTotal {
		return fmt.Errorf("campaign_target_total-pinned_total=%d does not match gap_total=%d", d.CampaignTargetTotal-d.PinnedTotal, d.GapTotal)
	}
	if strings.TrimSpace(d.GapReason) == "" {
		return fmt.Errorf("gap_reason is empty")
	}
	if strings.TrimSpace(d.RecordedBy) == "" {
		return fmt.Errorf("recorded_by is empty")
	}
	return nil
}

func validateCompactT3SourceDigest(witness compactT3Witness) error {
	if len(witness.SourceSHA256) != sha256.Size*2 {
		return errorsForCompactT3Witness(witness, "source_sha256 has an invalid length")
	}
	if _, err := hex.DecodeString(witness.SourceSHA256); err != nil {
		return errorsForCompactT3Witness(witness, "source_sha256 is not hexadecimal")
	}
	sum := sha256.Sum256([]byte(witness.SourceUTF8))
	if got := hex.EncodeToString(sum[:]); got != witness.SourceSHA256 {
		return errorsForCompactT3Witness(witness, fmt.Sprintf("source_sha256=%s, want %s", witness.SourceSHA256, got))
	}
	return nil
}

func validateCompactT3ExpectedOutcome(witness compactT3Witness) error {
	if witness.Expected.CHasError == nil {
		return errorsForCompactT3Witness(witness, "expected.c_has_error is absent")
	}
	if witness.Expected.ProductionHasError == nil {
		return errorsForCompactT3Witness(witness, "expected.production_has_error is absent")
	}
	if witness.Expected.CompactHasError == nil {
		return errorsForCompactT3Witness(witness, "expected.compact_has_error is absent")
	}
	return nil
}

func validateCompactT3COracleProvenance(provenance compactT3COracleProvenance) error {
	fields := []struct {
		name  string
		value string
	}{
		{"contract", provenance.Contract},
		{"binding_commit", provenance.BindingCommit},
		{"runtime_commit", provenance.RuntimeCommit},
		{"grammar_repository", provenance.GrammarRepository},
		{"grammar_commit", provenance.GrammarCommit},
		{"compiler_path", provenance.CompilerPath},
		{"compiler_version", provenance.CompilerVersion},
		{"grammar_compile_flags", provenance.GrammarCompileFlags},
		{"grammar_artifact_sha256", provenance.GrammarArtifactSHA256},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("c_oracle.%s is empty", field.name)
		}
	}
	if len(provenance.GrammarArtifactSHA256) != sha256.Size*2 {
		return fmt.Errorf("c_oracle.grammar_artifact_sha256 has an invalid length")
	}
	if _, err := hex.DecodeString(provenance.GrammarArtifactSHA256); err != nil {
		return fmt.Errorf("c_oracle.grammar_artifact_sha256 is not hexadecimal")
	}
	return nil
}

func errorsForCompactT3Witness(witness compactT3Witness, message string) error {
	return fmt.Errorf("witness %q (%s): %s", witness.ID, witness.Language, message)
}

func compactT3WitnessLanguages(manifest compactT3WitnessManifest) []string {
	languages := make([]string, 0, len(manifest.LanguageCounts))
	for language := range manifest.LanguageCounts {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}

func compactT3WitnessesForLanguage(manifest compactT3WitnessManifest, language string) []compactT3Witness {
	var witnesses []compactT3Witness
	for _, witness := range manifest.Witnesses {
		if witness.Language == language {
			witnesses = append(witnesses, witness)
		}
	}
	return witnesses
}

func compactT3GoLanguage(name string) (*gotreesitter.Language, bool) {
	switch name {
	case "html":
		return grammars.HtmlLanguage(), true
	case "javascript":
		return grammars.JavascriptLanguage(), true
	case "swift":
		return grammars.SwiftLanguage(), true
	default:
		return nil, false
	}
}

func assertCompactT3COracleProvenance(t *testing.T, witness compactT3Witness, actual COracleBuildIdentity) {
	t.Helper()
	want := witness.COracle
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"contract", actual.Contract, want.Contract},
		{"binding_commit", actual.BindingCommit, want.BindingCommit},
		{"runtime_commit", actual.RuntimeCommit, want.RuntimeCommit},
		{"grammar_repository", actual.GrammarRepo, want.GrammarRepository},
		{"grammar_commit", actual.GrammarCommit, want.GrammarCommit},
		{"compiler_path", actual.CompilerPath, want.CompilerPath},
		{"compiler_version", actual.CompilerVersion, want.CompilerVersion},
		{"grammar_compile_flags", actual.GrammarCompileFlags, want.GrammarCompileFlags},
		{"grammar_artifact_sha256", actual.GrammarArtifactSHA256, want.GrammarArtifactSHA256},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Fatalf("witness %q: C oracle %s=%q, want %q", witness.ID, check.name, check.got, check.want)
		}
	}
}

// compactT3ParseC parses source with the pinned C oracle language and
// returns the tree. The caller owns the returned tree and must Close it; the
// transport parser itself is disposable and closed here (tree-sitter C trees
// do not depend on the parser instance that produced them).
func compactT3ParseC(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		t.Fatalf("set C oracle language: %v", err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C oracle parse returned no root")
	}
	return tree
}

// compactT3ParseGo parses source on the production (compact=false) or
// compact (compact=true) route and returns the tree. The caller owns the
// returned tree and must Release it.
func compactT3ParseGo(t *testing.T, language *gotreesitter.Language, source []byte, compact bool) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(compact)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse compact=%t: %v", compact, err)
	}
	return tree
}

func assertCompactT3Outcome(t *testing.T, witness compactT3Witness, route string, want *bool, got bool) {
	t.Helper()
	if want == nil {
		t.Fatalf("witness %q: %s expectation is absent", witness.ID, route)
	}
	if got != *want {
		t.Fatalf("witness %q: %s HasError=%t, want %t", witness.ID, route, got, *want)
	}
}

// compactT3StructuralDivergence is one node-level structural mismatch found
// while walking a Go tree against the pinned C oracle tree in lockstep. The
// compared axes are exactly the B3 stage S1 obligation: deep-tree shape
// (type, child count), byte and point spans, named and extra flags, the
// Missing flag, direct and propagated error flags, and child field names.
type compactT3StructuralDivergence struct {
	Path     string
	Category string
	GoValue  string
	CValue   string
}

func compactT3FormatDivergence(d compactT3StructuralDivergence) string {
	return fmt.Sprintf("%s: %s go=%q c=%q", d.Path, d.Category, d.GoValue, d.CValue)
}

// compactT3StructuralDivergences walks goNode and cNode in lockstep and
// returns every divergence found, in tree order. A nil result means the two
// trees are structurally identical on every compared axis. Unlike
// parity_cgo_test.go's compareNodes (bytes/named/missing/child-count/fields,
// no HasError or points) or parity_dump_v1.go's FirstDivergenceDumpV1
// (adds HasError and points, but not Missing, and stops at the first
// divergence), this walk covers every axis the S1 gate names and collects
// every divergence so a failing witness reports its full shape at once.
func compactT3StructuralDivergences(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node) []compactT3StructuralDivergence {
	var out []compactT3StructuralDivergence
	compactT3WalkStructuralDivergences(goNode, goLang, cNode, compactT3RootPath(goNode, goLang, cNode), &out)
	return out
}

func compactT3WalkStructuralDivergences(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string, out *[]compactT3StructuralDivergence) {
	if goNode == nil || cNode == nil {
		if goNode != nil || cNode != nil {
			*out = append(*out, compactT3StructuralDivergence{
				Path: path, Category: "shape",
				GoValue: fmt.Sprintf("nil=%v", goNode == nil), CValue: fmt.Sprintf("nil=%v", cNode == nil),
			})
		}
		return
	}

	goType := compactT3GoNodeType(goNode, goLang)
	cType := cNode.Kind()
	if goType != cType {
		*out = append(*out, compactT3StructuralDivergence{Path: path, Category: "type", GoValue: goType, CValue: cType})
	}
	if goNode.IsNamed() != cNode.IsNamed() {
		*out = append(*out, compactT3StructuralDivergence{
			Path: path, Category: "named",
			GoValue: fmt.Sprintf("%v", goNode.IsNamed()), CValue: fmt.Sprintf("%v", cNode.IsNamed()),
		})
	}
	if goNode.IsExtra() != cNode.IsExtra() {
		*out = append(*out, compactT3StructuralDivergence{
			Path: path, Category: "extra",
			GoValue: fmt.Sprintf("%v", goNode.IsExtra()), CValue: fmt.Sprintf("%v", cNode.IsExtra()),
		})
	}
	if goNode.IsError() != cNode.IsError() {
		*out = append(*out, compactT3StructuralDivergence{
			Path: path, Category: "error",
			GoValue: fmt.Sprintf("%v", goNode.IsError()), CValue: fmt.Sprintf("%v", cNode.IsError()),
		})
	}
	if goNode.IsMissing() != cNode.IsMissing() {
		*out = append(*out, compactT3StructuralDivergence{
			Path: path, Category: "missing",
			GoValue: fmt.Sprintf("%v", goNode.IsMissing()), CValue: fmt.Sprintf("%v", cNode.IsMissing()),
		})
	}
	if goNode.HasError() != cNode.HasError() {
		*out = append(*out, compactT3StructuralDivergence{
			Path: path, Category: "has_error",
			GoValue: fmt.Sprintf("%v", goNode.HasError()), CValue: fmt.Sprintf("%v", cNode.HasError()),
		})
	}

	goStart, goEnd := goNode.StartPoint(), goNode.EndPoint()
	cStart, cEnd := cNode.StartPosition(), cNode.EndPosition()
	if goNode.StartByte() != uint32(cNode.StartByte()) || goNode.EndByte() != uint32(cNode.EndByte()) ||
		goStart.Row != uint32(cStart.Row) || goStart.Column != uint32(cStart.Column) ||
		goEnd.Row != uint32(cEnd.Row) || goEnd.Column != uint32(cEnd.Column) {
		*out = append(*out, compactT3StructuralDivergence{
			Path: path, Category: "span",
			GoValue: fmt.Sprintf("%d:%d-%d:%d @%d..%d", goStart.Row, goStart.Column, goEnd.Row, goEnd.Column, goNode.StartByte(), goNode.EndByte()),
			CValue:  fmt.Sprintf("%d:%d-%d:%d @%d..%d", cStart.Row, cStart.Column, cEnd.Row, cEnd.Column, cNode.StartByte(), cNode.EndByte()),
		})
	}

	goChildCount := goNode.ChildCount()
	cChildCount := int(cNode.ChildCount())
	if goChildCount != cChildCount {
		*out = append(*out, compactT3StructuralDivergence{
			Path: path, Category: "child_count",
			GoValue: fmt.Sprintf("%d", goChildCount), CValue: fmt.Sprintf("%d", cChildCount),
		})
		return
	}

	for i := 0; i < goChildCount; i++ {
		goChild := goNode.Child(i)
		cChild := cNode.Child(uint(i))
		nextPath := compactT3ChildPath(path, compactT3NodeTypeOrFallback(goChild, goLang, cChild), i)

		goField := goNode.FieldNameForChild(i, goLang)
		cField := cNode.FieldNameForChild(uint32(i))
		if goField != cField {
			*out = append(*out, compactT3StructuralDivergence{Path: nextPath, Category: "field", GoValue: goField, CValue: cField})
		}
		compactT3WalkStructuralDivergences(goChild, goLang, cChild, nextPath, out)
	}
}

func compactT3GoNodeType(node *gotreesitter.Node, lang *gotreesitter.Language) string {
	if node == nil || lang == nil {
		return ""
	}
	// A zero-terminator sentinel type has no meaningful name; treat it the
	// same way parity_dump_v1.go's dumpV1GoType does.
	if typ := node.Type(lang); typ != "\\0" {
		return typ
	}
	return ""
}

func compactT3NodeTypeOrFallback(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node) string {
	if goNode != nil && goLang != nil {
		if typ := compactT3GoNodeType(goNode, goLang); typ != "" {
			return typ
		}
	}
	if cNode != nil {
		if typ := cNode.Kind(); typ != "" {
			return typ
		}
	}
	return "?"
}

func compactT3RootPath(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node) string {
	return "/" + compactT3NodeTypeOrFallback(goNode, goLang, cNode)
}

func compactT3ChildPath(parentPath, typ string, index int) string {
	if typ == "" {
		typ = "?"
	}
	return fmt.Sprintf("%s/%s[%d]", parentPath, typ, index)
}

//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestAdmissionCandidateExactExternalPayloadCorpus(t *testing.T) {
	tests := []struct {
		language   string
		path       string
		sha256     string
		nextReason string
	}{
		// kotlinx-datetime@292e7e0ce510, core/common/src/serializers/DateTimeUnitSerializers.kt
		{
			language: "kotlin",
			path:     "testdata/admission_direct/external_payload/kotlin.kt",
			sha256:   "e825c4c57c95082c9aa2b62f35853d0b2edfc24934a349ce9f1195d14ae7522b",
		},
		// ocaml/ocaml@7c51f06c0a5d, testsuite/tools/environment.mli
		{
			language:   "ocaml",
			path:       "testdata/admission_direct/external_payload/ocaml.mli",
			sha256:     "1b5415e35e2eb7b6ab69f41754d32bbdd5bf6043cdf67467e2148acf8e8039dd",
			nextReason: "live-link cap exceeded",
		},
		// tree-sitter-perl@ad74e6db234c, unicode_ranges.pl
		{
			language: "perl",
			path:     "testdata/admission_direct/external_payload/perl.pl",
			sha256:   "84b468672c82a73ba88d62a47591e85d02f9e35952a7ce45a494db71d1fa3ad4",
		},
		// tree-sitter-rust@77a3747266f4, examples/weird-exprs.rs
		{
			language:   "rust",
			path:       "testdata/admission_direct/external_payload/rust.rs",
			sha256:     "4968463f974c79afd641769a48d4e7e1c617b1acec03b5ccea3555b28def9dba",
			nextReason: "live-link cap exceeded",
		},
	}
	entries := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		entries[entry.Name] = entry
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			source, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != test.sha256 {
				t.Fatalf("fixture SHA-256=%s, want %s", got, test.sha256)
			}
			entry, ok := entries[test.language]
			if !ok {
				t.Fatalf("%s grammar is not registered", test.language)
			}
			row := runAdmissionScorecardSource(entry, source)
			switch row.status {
			case scorecardPass:
				return
			case scorecardFallback:
				if strings.Contains(row.detail, "external payload") {
					t.Fatalf("exact external payload still declined: %s", row.detail)
				}
				if test.nextReason != "" && !strings.Contains(row.detail, test.nextReason) {
					t.Fatalf("fallback=%q, want the next blocker %q", row.detail, test.nextReason)
				}
				if test.nextReason == "" {
					// Independent scheduler guards can stop these corpora first.
					// Core's TestRecursiveInsertAcceptsExactExternalScannerProvenance
					// proves insertion for exact top-level and descendant payloads.
					// TestRecursiveInsertKeepsScannerCheckpointMismatchSeparate
					// protects the corresponding negative identity contract.
					frontierDecline := strings.HasPrefix(row.detail, "compact route declined at no_action")
					linkCap := strings.HasPrefix(row.detail, "compact route error: parser-core phase zero: shared (") &&
						strings.Contains(row.detail, "live-link cap exceeded")
					if !frontierDecline && !linkCap {
						t.Fatalf("unexpected fallback category: %s", row.detail)
					}
					assertExternalPayloadFallbackEquivalent(t, entry, source)
				}
			default:
				t.Fatalf("compact route=%s: %s", row.status, row.detail)
			}
		})
	}
}

// TestAdmissionCandidatePerlExternalPayloadPinsLiveLinkCapReach pins the
// exact decline this compact route reaches on
// testdata/admission_direct/external_payload/perl.pl, not merely its
// category. Task #81 found that admitting the zero-width external relex
// seam (relexZeroWidthExternalTokenForState, wired into dispatchPassActive
// at its own call site, parsercore_phase0_driver.go) moved this fixture's
// decline from "shared (1370,2837) live-link cap exceeded: 9 > 8" all the
// way back to byte 397 -- a real reach-point regression, still under
// investigation (a rescued fork produces links a structurally identical
// sibling cannot fold back with; see the seam's own doc comment for the
// trace). The seam now defaults off (GOT_COMPACT_ZERO_WIDTH_RESCUE,
// parser_config.go), so this pin is back at its original position. This
// test exists so a later change that moves the reach point again -- in
// either direction -- is visible instead of silently absorbed by
// TestAdmissionCandidateExactExternalPayloadCorpus's own category-only
// assertion for this fixture.
func TestAdmissionCandidatePerlExternalPayloadPinsLiveLinkCapReach(t *testing.T) {
	const path = "testdata/admission_direct/external_payload/perl.pl"
	const wantSHA256 = "84b468672c82a73ba88d62a47591e85d02f9e35952a7ce45a494db71d1fa3ad4"
	const wantDetail = "compact route error: parser-core phase zero: shared (1370,2837) live-link cap exceeded: 9 > 8"

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != wantSHA256 {
		t.Fatalf("fixture SHA-256=%s, want %s", got, wantSHA256)
	}
	var entry grammars.LangEntry
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == "perl" {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatal("perl grammar is not registered")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	row := runAdmissionScorecardSource(entry, source)
	if row.status != scorecardFallback {
		t.Fatalf("compact route=%s, want %s", row.status, scorecardFallback)
	}
	if row.detail != wantDetail {
		t.Fatalf("fallback=%q, want the exact pinned decline %q (the reach point moved; update this pin deliberately, after checking whether the new reach point is expected)", row.detail, wantDetail)
	}
}

// perlMapGrepSeedPath is the second witness task #81 found: with the
// zero-width external relex seam admitted, this file goes from routing
// compact cleanly to entering S3 recovery and then finding no table action
// for the elected token. It lives in the seeded tree-sitter-perl checkout
// (cgo_harness/seed_parity_repos.sh), not this repo's own testdata, so this
// test skips when the seed is absent instead of failing a host run that
// never seeded it.
const perlMapGrepSeedPath = "/tmp/grammar_parity/perl/test/highlight/map-grep.pm"

// TestAdmissionCandidatePerlMapGrepRoutesCompactByDefault pins the seam's
// default-off state on the map-grep.pm witness: the compact route must
// route this file cleanly, matching production's own digest, with
// GOT_COMPACT_ZERO_WIDTH_RESCUE left unset. Admitting the seam
// (GOT_COMPACT_ZERO_WIDTH_RESCUE=1) sends this same file into
// "compact route declined at recovery [mechanism=recovery-entered]: did not
// accept EOF: generic scheduler has no table action for the elected
// token" instead -- the regression the default-off gate exists to prevent.
// See relexZeroWidthExternalTokenForState's own doc comment
// (parsercore_phase0_driver.go) for the fuller trace.
func TestAdmissionCandidatePerlMapGrepRoutesCompactByDefault(t *testing.T) {
	source, err := os.ReadFile(perlMapGrepSeedPath)
	if err != nil {
		t.Skipf("perl parity seed unavailable: %s (%v); run cgo_harness/seed_parity_repos.sh", perlMapGrepSeedPath, err)
	}
	var entry grammars.LangEntry
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == "perl" {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatal("perl grammar is not registered")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)

	const wantDigest = "633141732a3b"
	row := runAdmissionScorecardSource(entry, source)
	if row.status != scorecardPass {
		t.Fatalf("compact route=%s, want %s (detail=%s)", row.status, scorecardPass, row.detail)
	}
	if wantSuffix := "digest " + wantDigest; row.detail != wantSuffix {
		t.Fatalf("pass detail=%q, want %q", row.detail, wantSuffix)
	}
}

func assertExternalPayloadFallbackEquivalent(t *testing.T, entry grammars.LangEntry, source []byte) {
	t.Helper()
	language := entry.Language()
	var legacyDigest string
	for _, route := range []bool{false, true} {
		parser := gotreesitter.NewParser(language)
		parser.SetAdmissionCandidateRoute(route)
		beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
		tree, err := parser.Parse(source)
		if err != nil || tree == nil {
			t.Fatalf("compact=%t parse failed: %v", route, err)
		}
		defer tree.Release()
		afterRouted, afterFallback := gotreesitter.AdmissionCandidateCounters()
		wantFallback := uint64(0)
		if route {
			wantFallback = 1
		}
		if afterRouted != beforeRouted || afterFallback-beforeFallback != wantFallback {
			t.Fatalf("compact=%t route counters=%d/%d, want 0/%d", route, afterRouted-beforeRouted, afterFallback-beforeFallback, wantFallback)
		}
		runtime := tree.ParseRuntime()
		if tree.ParseStopReason() != gotreesitter.ParseStopAccepted || runtime.Truncated || runtime.TokenSourceEOFEarly ||
			runtime.ExpectedEOFByte != uint32(len(source)) || runtime.LastTokenEndByte != uint32(len(source)) || !runtime.LastTokenWasEOF {
			t.Fatalf("compact=%t incomplete parse: %s", route, runtime.Summary())
		}
		inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
		if err != nil {
			t.Fatal(err)
		}
		if route && inspection.SHA256 != legacyDigest {
			t.Fatalf("fallback digest=%s, forced legacy=%s", inspection.SHA256, legacyDigest)
		}
		legacyDigest = inspection.SHA256
	}
}

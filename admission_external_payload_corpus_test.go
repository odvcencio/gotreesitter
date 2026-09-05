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

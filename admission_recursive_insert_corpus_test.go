//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

func TestAdmissionCandidateBoundedRecursiveInsertionCorpus(t *testing.T) {
	tests := []struct {
		language   string
		path       string
		sha256     string
		nextReason string
	}{
		// tree-sitter-c-sharp@88366631d598, test/highlight/variableDeclarations.cs
		{
			language:   "c_sharp",
			path:       "testdata/admission_direct/recursive_insert/c_sharp.cs",
			sha256:     "d532120abe52b3af477aa079e33a6998ef6b1a4370cff257277d319cd1912dd1",
			nextReason: "did not accept EOF",
		},
		// tree-sitter-elixir@7937d3b4d65f, test/highlight/module.ex
		{
			language:   "elixir",
			path:       "testdata/admission_direct/recursive_insert/elixir.ex",
			sha256:     "7977e259f7e718177e568eedbb82ba8df72a149095718f3c563de5ec06db285c",
			nextReason: "lacks alternative-set coverage by one non-blended survivor",
		},
		// tree-sitter-perl@ad74e6db234c, test/highlight/statements.pm
		{
			language:   "perl",
			path:       "testdata/admission_direct/recursive_insert/perl.pm",
			sha256:     "a00caed758e0a58db30454892ef404518228255e34c2e7f2eccc27ffc648f51b",
			nextReason: "converged-path reduction split drops",
		},
		// tree-sitter-scala@97aead18d977, test/tags/basics.scala
		{
			language:   "scala",
			path:       "testdata/admission_direct/recursive_insert/scala.scala",
			sha256:     "d53a16c04dd1ad917104a5f9ab6bf45c4c779188bafe7044d2fec06217a3b7d9",
			nextReason: "lacks alternative-set coverage by one non-blended survivor",
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
				if strings.Contains(row.detail, "recursive insertion") {
					t.Fatalf("bounded recursive insertion still declined: %s", row.detail)
				}
				if !strings.Contains(row.detail, test.nextReason) {
					t.Fatalf("fallback=%q, want the next blocker %q", row.detail, test.nextReason)
				}
			default:
				t.Fatalf("compact route=%s: %s", row.status, row.detail)
			}
		})
	}
}

//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCompactDeclineProfileReportsSelectedTreeReuse(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	for _, test := range loadCanonicalGoIncrementalCases(t) {
		if test.spec.Name != "same_line_length_change" {
			continue
		}
		lang := canonicalIncrementalGoLanguage(t, "go")
		cParser := sitter.NewParser()
		t.Cleanup(cParser.Close)
		if err := cParser.SetLanguage(loadCanonicalGoCLanguage(t)); err != nil {
			t.Fatal(err)
		}
		cTree := cParser.Parse(test.edited, nil)
		if cTree == nil {
			t.Fatal("C parser returned no tree")
		}
		t.Cleanup(cTree.Close)
		var profiles [2]gts.IncrementalParseProfile
		var digests [2]string
		for index, compactAttempt := range []bool{true, false} {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(true)
			oldTree, err := parser.Parse(test.source)
			requireCanonicalGoIncrementalTree(t, oldTree, test.source, "initial compact", err)
			parser.SetAdmissionCandidateRoute(compactAttempt)
			oldTree.Edit(test.forward)
			next, profile, err := parser.ParseIncrementalProfiled(test.edited, oldTree)
			requireCanonicalGoIncrementalTree(t, next, test.edited, "incremental", err)
			digests[index] = canonicalGoTreeDigest(t, next, lang, "incremental")
			assertLockedCTreeExact(t, "profiled compact decline", next, lang, cTree)
			runtime := next.ParseRuntime()
			if next != oldTree {
				oldTree.Release()
			}
			next.Release()
			if profile.ReuseUnsupported || !profile.OldTreeReuseRoute {
				t.Fatalf("compactAttempt=%t lost old-tree reuse", compactAttempt)
			}
			if compactAttempt && runtime.CompactIncrementalFallbackReason == "" {
				t.Fatal("fixture did not exercise a compact decline")
			}
			profiles[index] = profile
		}
		if digests[0] != digests[1] {
			t.Fatalf("compact decline digest=%s, direct digest=%s", digests[0], digests[1])
		}
		attempted, direct := profiles[0], profiles[1]
		if attempted.ReusedSubtrees != direct.ReusedSubtrees || attempted.ReusedBytes != direct.ReusedBytes {
			t.Errorf("discarded compact reuse leaked into result: attempted=%d/%d direct=%d/%d", attempted.ReusedSubtrees, attempted.ReusedBytes, direct.ReusedSubtrees, direct.ReusedBytes)
		}
		if attempted.TokensConsumed <= direct.TokensConsumed {
			t.Errorf("discarded compact work omitted: attempted tokens=%d direct=%d", attempted.TokensConsumed, direct.TokensConsumed)
		}
		return
	}
	t.Fatal("missing same-line length fixture")
}

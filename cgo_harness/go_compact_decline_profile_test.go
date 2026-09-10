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
	for _, tc := range loadCanonicalGoIncrementalCases(t) {
		if tc.spec.Name != "recovery_deletion" {
			continue
		}
		lang := canonicalIncrementalGoLanguage(t, "go")
		cp := sitter.NewParser()
		defer cp.Close()
		if err := cp.SetLanguage(canonicalIncrementalCLanguage(t, "go")); err != nil {
			t.Fatal(err)
		}
		oracle := cp.Parse(tc.edited, nil)
		requireCanonicalCIncrementalTree(t, oracle, tc.edited, "fresh C")
		defer oracle.Close()
		want := canonicalCTreeDigest(t, oracle, "fresh C")
		var profiles [2]gts.IncrementalParseProfile
		for i, compactAttempt := range []bool{true, false} {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			old, err := p.Parse(tc.source)
			requireCanonicalGoIncrementalTree(t, old, tc.source, "initial compact", err)
			p.SetAdmissionCandidateRoute(compactAttempt)
			old.Edit(tc.forward)
			next, profile, err := p.ParseIncrementalProfiled(tc.edited, old)
			requireCanonicalGoIncrementalTree(t, next, tc.edited, "incremental", err)
			got := canonicalGoTreeDigest(t, next, lang, "incremental")
			receipt := next.ParseRuntime()
			if next != old {
				old.Release()
			}
			next.Release()
			if got != want {
				t.Fatalf("compactAttempt=%t digest=%s want=%s", compactAttempt, got, want)
			}
			if profile.ReuseUnsupported || !profile.OldTreeReuseRoute {
				t.Fatalf("compactAttempt=%t lost old-tree reuse", compactAttempt)
			}
			if compactAttempt && receipt.CompactIncrementalFallbackReason == "" {
				t.Fatal("fixture did not exercise a compact decline")
			}
			profiles[i] = profile
		}
		attempted, direct := profiles[0], profiles[1]
		if attempted.ReusedSubtrees != direct.ReusedSubtrees || attempted.ReusedBytes != direct.ReusedBytes {
			t.Errorf("discarded compact reuse leaked into result: attempted=%d/%d direct=%d/%d", attempted.ReusedSubtrees, attempted.ReusedBytes, direct.ReusedSubtrees, direct.ReusedBytes)
		}
		if attempted.TokensConsumed <= direct.TokensConsumed {
			t.Errorf("discarded compact work omitted: attempted tokens=%d direct=%d", attempted.TokensConsumed, direct.TokensConsumed)
		}
		t.Logf("selected reuse attempted=%d/%d direct=%d/%d; total tokens attempted=%d direct=%d", attempted.ReusedSubtrees, attempted.ReusedBytes, direct.ReusedSubtrees, direct.ReusedBytes, attempted.TokensConsumed, direct.TokensConsumed)
		return
	}
	t.Fatal("missing recovery deletion fixture")
}

//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"testing"
)

func TestGoRepeatedCanonicalIncrementalLockedC(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	cases := loadCanonicalGoIncrementalCases(t)
	for _, tc := range cases {
		if tc.spec.Name == "early_newline" {
			// This function-boundary prefix exposes a clean call/conversion mismatch
			// on the first reverse edit, which error recovery cannot detect.
			tc.spec.Name = "newline_prefix_call_conversion"
			tc.source = tc.source[:65953]
			tc.edited = append(append(append([]byte(nil), tc.source[:19]...), '\n'), tc.source[19:]...)
			tc.forward = canonicalGoInputEdit(tc.source, tc.edited, 19, 19, 20)
			tc.reverse = canonicalGoInputEdit(tc.edited, tc.source, 19, 20, 19)
			cases = append(cases, tc)
			break
		}
	}
	for _, tc := range cases {
		if tc.spec.Language != "go" || tc.spec.Role != "representative" {
			continue
		}
		t.Run(tc.spec.Name, func(t *testing.T) {
			lang := canonicalIncrementalGoLanguage(t, "go")
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			tree, err := p.Parse(tc.source)
			requireCanonicalGoIncrementalTree(t, tree, tc.source, "initial", err)
			defer func() { releaseCanonicalGoTree(tree) }()
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(canonicalIncrementalCLanguage(t, "go")); err != nil {
				t.Fatal(err)
			}
			dirs := tc.directions()
			for i := 0; i < 4; i++ {
				direction := dirs[i%2]
				tree.Edit(direction.goEdit)
				next, profile, err := p.ParseIncrementalProfiled(direction.to, tree)
				requireCanonicalGoIncrementalTree(t, next, direction.to, "repeated", err)
				if next != tree {
					tree.Release()
				}
				tree = next
				oracle := cp.Parse(direction.to, nil)
				requireCanonicalCIncrementalTree(t, oracle, direction.to, "fresh C")
				got := canonicalGoTreeDigest(t, tree, lang, "repeated")
				want := canonicalCTreeDigest(t, oracle, "fresh C")
				oracle.Close()
				if got != want {
					t.Errorf("cycle=%d %s parity mismatch: got=%s want=%s", i, direction.name, got, want)
				}
				receipt := tree.ParseRuntime()
				t.Logf("cycle=%d direction=%s unsupported=%t reason=%q compact_reuse=%t compact_reason=%q reused_bytes=%d new_nodes=%d", i, direction.name, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, receipt.CompactIncrementalReuseRoute, receipt.CompactIncrementalFallbackReason, profile.ReusedBytes, profile.NewNodesAllocated)
				if profile.ReuseUnsupported {
					t.Errorf("cycle=%d %s lost incremental reuse: %s", i, direction.name, profile.ReuseUnsupportedReason)
				}
			}
		})
	}
}

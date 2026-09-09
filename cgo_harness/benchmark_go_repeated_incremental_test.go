//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	gts "github.com/odvcencio/gotreesitter"
	"testing"
)

// BenchmarkGoCanonicalFourEditHistory includes initial tree creation and final
// release. Each operation has exactly four edits, so calibration cannot change
// the share of first-edit work relative to later reuse.
func BenchmarkGoCanonicalFourEditHistory(b *testing.B) {
	for _, tc := range loadCanonicalGoIncrementalCases(b) {
		if tc.spec.Language != "go" || tc.spec.Role != "representative" {
			continue
		}
		b.Run(tc.spec.Name, func(b *testing.B) {
			lang := canonicalIncrementalGoLanguage(b, "go")
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			dirs := tc.directions()
			var nodes, tokens, reused, unsupported uint64
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				tree, err := p.Parse(tc.source)
				if err != nil || tree == nil {
					b.Fatalf("initial parse: %v", err)
				}
				for i := 0; i < 4; i++ {
					d := dirs[i%2]
					tree.Edit(d.goEdit)
					next, profile, err := p.ParseIncrementalProfiled(d.to, tree)
					if err != nil || next == nil {
						tree.Release()
						b.Fatalf("edit %d: %v", i, err)
					}
					if next != tree {
						tree.Release()
					}
					tree = next
					nodes += profile.NewNodesAllocated
					tokens += profile.TokensConsumed
					reused += profile.ReusedBytes
					if profile.ReuseUnsupported {
						unsupported++
					}
				}
				tree.Release()
			}
			b.StopTimer()
			b.ReportMetric(float64(nodes)/float64(b.N), "edit_nodes/history")
			b.ReportMetric(float64(tokens)/float64(b.N), "edit_tokens/history")
			b.ReportMetric(float64(reused)/float64(b.N), "reused_B/history")
			b.ReportMetric(float64(unsupported)/float64(b.N), "fallbacks/history")
		})
	}
}

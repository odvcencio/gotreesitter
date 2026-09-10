//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// Each operation includes initial compact parsing, four alternating edits,
// and all tree releases. Parser and grammar construction remain outside timing.
func BenchmarkGoCompactFourEditLifecycle(b *testing.B) {
	for _, tc := range compactNativeCanonicalCases(b) {
		b.Run(tc.spec.Name, func(b *testing.B) {
			for _, lane := range []string{"compact", "legacy"} {
				b.Run(lane, func(b *testing.B) {
					p := gts.NewParser(canonicalIncrementalGoLanguage(b, "go"))
					dirs := tc.directions()
					var tokens, nodes, reusedBytes, initialTokens, initialNodes uint64
					b.ReportAllocs()
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						var before uint64
						if n != 0 {
							before = compactNativeLegacyEntries(b, p)
						}
						p.SetAdmissionCandidateRoute(true)
						tree, err := p.Parse(tc.source)
						if err != nil || tree == nil {
							b.Fatalf("initial parse: %v", err)
						}
						if compactNativeLegacyEntries(b, p) != before {
							tree.Release()
							b.Fatal("initial parse entered legacy")
						}
						initial := tree.ParseRuntime()
						initialTokens += initial.TokensConsumed
						initialNodes += uint64(initial.NodesAllocated)
						p.SetAdmissionCandidateRoute(lane == "compact")
						for i := 0; i < 4; i++ {
							d := dirs[i%2]
							before = compactNativeLegacyEntries(b, p)
							tree.Edit(d.goEdit)
							next, profile, err := p.ParseIncrementalProfiled(d.to, tree)
							if next != tree {
								tree.Release()
							}
							tree = next
							if err != nil || tree == nil {
								releaseCanonicalGoTree(tree)
								b.Fatalf("edit %d: %v", i, err)
							}
							entries := compactNativeLegacyEntries(b, p) - before
							if lane == "compact" {
								if entries != 0 || !tree.ParseRuntime().CompactIncrementalReuseRoute ||
									profile.ReuseUnsupported || profile.ReusedBytes == 0 {
									tree.Release()
									b.Fatal("compact history left native reuse")
								}
							} else if entries != 1 {
								tree.Release()
								b.Fatal("legacy history did not enter legacy exactly once")
							}
							tokens += profile.TokensConsumed
							nodes += profile.NewNodesAllocated
							reusedBytes += profile.ReusedBytes
						}
						tree.Release()
					}
					b.StopTimer()
					operations := float64(b.N)
					b.ReportMetric(float64(tokens)/operations, "edit_tokens/history")
					b.ReportMetric(float64(nodes)/operations, "edit_nodes/history")
					b.ReportMetric(float64(reusedBytes)/operations, "reused_B/history")
					b.ReportMetric(float64(initialTokens)/operations, "initial_tokens/history")
					b.ReportMetric(float64(initialNodes)/operations, "initial_nodes/history")
				})
			}
		})
	}
}

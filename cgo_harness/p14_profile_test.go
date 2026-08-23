//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func benchmarkP14EarlyNewlineDirection(b *testing.B, reverse bool) {
	b.Helper()
	cases := loadCanonicalGoIncrementalCases(b)
	var tc canonicalGoIncrementalCase
	for _, candidate := range cases {
		if candidate.spec.Name == "early_newline" {
			tc = candidate
			break
		}
	}
	if tc.spec.Name == "" {
		b.Fatal("canonical early_newline case is missing")
	}
	var from, target []byte
	var edit gotreesitter.InputEdit
	if reverse {
		from, target, edit = tc.edited, tc.source, tc.reverse
	} else {
		from, target, edit = tc.source, tc.edited, tc.forward
	}
	parser := gotreesitter.NewParser(canonicalIncrementalGoLanguage(b, "go"))
	tree, err := parser.Parse(from)
	requireCanonicalGoIncrementalTree(b, tree, from, "P14 profile initial", err)
	defer releaseCanonicalGoTree(tree)

	b.ReportAllocs()
	b.SetBytes(int64((len(tc.source) + len(tc.edited)) / 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 {
			b.StopTimer()
			releaseCanonicalGoTree(tree)
			tree, err = parser.Parse(from)
			requireCanonicalGoIncrementalTree(b, tree, from, "P14 profile reset", err)
			b.StartTimer()
		}
		tree.Edit(edit)
		newTree, _, err := parser.ParseIncrementalProfiled(target, tree)
		requireCanonicalGoIncrementalTree(b, newTree, target, "P14 profile direction", err)
		if newTree != tree {
			tree.Release()
		}
		tree = newTree
	}
	b.StopTimer()
}

func BenchmarkP14EarlyNewlineForward(b *testing.B) {
	benchmarkP14EarlyNewlineDirection(b, false)
}

func BenchmarkP14EarlyNewlineReverse(b *testing.B) {
	benchmarkP14EarlyNewlineDirection(b, true)
}

//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import "testing"

// The complete operation includes Tree.Edit, parsing, result checks, and
// releasing the old handle. Each process runs an explicit Go-C-C-Go cycle.
func BenchmarkAccountingCompleteGoCCGo(b *testing.B) {
	for _, tc := range loadCanonicalGoIncrementalCases(b) {
		if tc.spec.Name != "same_line_length_change" {
			continue
		}
		goLang := canonicalIncrementalGoLanguage(b, "go")
		cLang := canonicalIncrementalCLanguage(b, "go")
		admitCanonicalGoIncrementalCase(b, tc, goLang, cLang)
		b.Run("GoBefore", func(b *testing.B) { benchmarkCanonicalGoIncremental(b, tc, goLang) })
		b.Run("CBefore", func(b *testing.B) { benchmarkCanonicalCIncremental(b, tc, cLang) })
		b.Run("CAfter", func(b *testing.B) { benchmarkCanonicalCIncremental(b, tc, cLang) })
		b.Run("GoAfter", func(b *testing.B) { benchmarkCanonicalGoIncremental(b, tc, goLang) })
		return
	}
	b.Fatal("missing fixed workload")
}

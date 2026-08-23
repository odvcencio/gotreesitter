//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// BenchmarkP13CanonicalGoIncremental keeps only the four representative Go
// cases. The locked matrix's controls remain outside this disposition.
func BenchmarkP13CanonicalGoIncremental(b *testing.B) {
	for _, tc := range loadCanonicalGoIncrementalCases(b) {
		if tc.spec.Role != "representative" || tc.spec.Language != "go" {
			continue
		}
		tc := tc
		b.Run(tc.spec.Name, func(b *testing.B) {
			goLang := canonicalIncrementalGoLanguage(b, tc.spec.Language)
			cLang := canonicalIncrementalCLanguage(b, tc.spec.Language)
			admitCanonicalGoIncrementalCase(b, tc, goLang, cLang)
			b.Run("gotreesitter", func(b *testing.B) {
				benchmarkCanonicalGoIncremental(b, tc, goLang)
			})
			b.Run("tree-sitter-c", func(b *testing.B) {
				benchmarkCanonicalCIncremental(b, tc, cLang)
			})
		})
	}
}

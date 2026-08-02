//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestSelectCompactAcceptanceDerivation(t *testing.T) {
	primary := core.Derivation{Score: 4}
	secondary := core.Derivation{Score: 4, BranchOrder: 1, HasBranchOrder: true}

	tests := []struct {
		name  string
		paths []core.Derivation
		allow bool
		want  bool
	}{
		{name: "sole", paths: []core.Derivation{primary}, want: true},
		{name: "uncertified", paths: []core.Derivation{primary, secondary}},
		{name: "certified-primary", paths: []core.Derivation{primary, secondary}, allow: true, want: true},
		{
			name:  "higher-secondary",
			paths: []core.Derivation{primary, {Score: 5, BranchOrder: 1, HasBranchOrder: true}},
			allow: true,
		},
		{
			name:  "two-primaries",
			paths: []core.Derivation{primary, {Score: 4}},
			allow: true,
		},
		{
			name:  "no-primary",
			paths: []core.Derivation{secondary, {Score: 4, BranchOrder: 2, HasBranchOrder: true}},
			allow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := selectCompactAcceptanceDerivation(tt.paths, tt.allow)
			if ok != tt.want {
				t.Fatalf("selected=%t, want %t", ok, tt.want)
			}
			if ok && (got.Score != primary.Score || got.HasBranchOrder ||
				got.BranchOrder != 0 || len(got.Payloads) != 0) {
				t.Fatalf("selected=%+v, want primary %+v", got, primary)
			}
		})
	}
}

// CompactAcceptanceDerivationTreesEqualForTest exposes
// compactAcceptanceDerivationTreesEqual to the external gotreesitter_test
// package, which can import grammars to build real parsed trees (an
// internal test file in this package cannot: grammars imports gotreesitter,
// so that would be an import cycle).
func CompactAcceptanceDerivationTreesEqualForTest(lang *Language, a, b *Node) bool {
	return compactAcceptanceDerivationTreesEqual(lang, a, b)
}

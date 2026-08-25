//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"reflect"
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

func TestSelectCompactAcceptanceDerivationWithMateriality(t *testing.T) {
	paths := []core.Derivation{
		{Payloads: []core.SubtreeID{1}, Score: 1, BranchOrder: 12, HasBranchOrder: true},
		{Payloads: []core.SubtreeID{1}, Score: 1, BranchOrder: 13, HasBranchOrder: true},
		{Payloads: []core.SubtreeID{1}, Score: 1, BranchOrder: 14, HasBranchOrder: true},
	}
	path, selected, materiality := selectCompactAcceptanceDerivationWithMateriality(paths, false)
	if !selected || !materiality || !reflect.DeepEqual(path, paths[0]) {
		t.Fatalf("all-branch-ordered materiality reference = (%+v, %t, %t), want first path, selected, materiality", path, selected, materiality)
	}

	primary := core.Derivation{Score: 4}
	secondary := core.Derivation{Score: 4, BranchOrder: 1, HasBranchOrder: true}
	path, selected, materiality = selectCompactAcceptanceDerivationWithMateriality([]core.Derivation{primary, secondary}, true)
	if !selected || materiality || !reflect.DeepEqual(path, primary) {
		t.Fatalf("certified primary selection = (%+v, %t, %t), want ordinary primary selection", path, selected, materiality)
	}

	higherSecondary := core.Derivation{Score: 5, BranchOrder: 1, HasBranchOrder: true}
	path, selected, materiality = selectCompactAcceptanceDerivationWithMateriality([]core.Derivation{primary, higherSecondary}, true)
	if selected || materiality || !reflect.DeepEqual(path, core.Derivation{}) {
		t.Fatalf("higher secondary selection = (%+v, %t, %t), want the existing decline", path, selected, materiality)
	}
}

func TestCompactAcceptanceElectionMaterialityDeclineDetail(t *testing.T) {
	paths := make([]core.Derivation, compactAcceptanceElectionMaxLiveDerivations+1)
	if got := compactAcceptanceElectionMaterialityDeclineDetail(paths, true); got != compactAcceptanceElectionCandidateCapDetail {
		t.Fatalf("candidate-cap detail = %q, want %q", got, compactAcceptanceElectionCandidateCapDetail)
	}
	paths = paths[:2]
	if got := compactAcceptanceElectionMaterialityDeclineDetail(paths, false); got != compactAcceptanceElectionNoContextDetail {
		t.Fatalf("no-context detail = %q, want %q", got, compactAcceptanceElectionNoContextDetail)
	}
	if got := compactAcceptanceElectionMaterialityDeclineDetail(paths, true); got != "" {
		t.Fatalf("ready materiality detail = %q, want empty", got)
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

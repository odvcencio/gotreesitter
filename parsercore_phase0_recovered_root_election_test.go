//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestRecoveredRootElectionCOrder(t *testing.T) {
	s := newLineageSelectionScheduler(t, true)
	missingHead := s.headers[0].head
	s.headers = s.headers[1:]
	s.s3RegionOpened = true
	missing, err := s.compact.Derivations(missingHead)
	if err != nil {
		t.Fatal(err)
	}
	absorbed, err := s.compact.Derivations(s.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || len(absorbed) != 1 {
		t.Fatalf("missing paths=%d absorbed paths=%d, want one each", len(missing), len(absorbed))
	}
	source, err := newDiagnosticParserCoreRecoveryCostSource(s.compact, s.options.materializationSource)
	if err != nil {
		t.Fatal(err)
	}
	var memo core.RecoveryCostMemo
	missingCost, err := diagnosticParserCoreDerivationErrorCost(s.recoverySymbolPolicy(), source, &memo, missing[0])
	if err != nil {
		t.Fatal(err)
	}
	absorbedCost, err := diagnosticParserCoreDerivationErrorCost(s.recoverySymbolPolicy(), source, &memo, absorbed[0])
	if err != nil {
		t.Fatal(err)
	}
	if missingCost != 610 || absorbedCost != 609 {
		t.Fatalf("PHP recovery costs=%d/%d, want missing 610 and absorb 609", missingCost, absorbedCost)
	}
	tests := []struct {
		name  string
		paths []core.Derivation
		want  int
	}{
		{"lower cost", []core.Derivation{missing[0], absorbed[0]}, 1},
		{"higher precedence", []core.Derivation{absorbed[0], {Payloads: absorbed[0].Payloads, Score: 1}}, 1},
		{"positive tie takes later", []core.Derivation{absorbed[0], absorbed[0]}, 1},
		{"cost before precedence", []core.Derivation{{Payloads: missing[0].Payloads, Score: 10}, absorbed[0]}, 1},
		{"complete payload list", []core.Derivation{{Payloads: []core.SubtreeID{missing[0].Payloads[0], absorbed[0].Payloads[0]}}, absorbed[0]}, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			winner, supported, err := s.electRecoveredRoot(s.headers[0], test.paths)
			if err != nil || !supported || winner != test.want {
				t.Fatalf("winner=%d supported=%t err=%v, want %d", winner, supported, err, test.want)
			}
		})
	}

	t.Run("missing source declines", func(t *testing.T) {
		source := s.options.materializationSource
		s.options.materializationSource = nil
		defer func() { s.options.materializationSource = source }()
		_, supported, err := s.electRecoveredRoot(s.headers[0], []core.Derivation{absorbed[0], absorbed[0]})
		if supported || err != nil {
			t.Fatalf("supported=%t err=%v", supported, err)
		}
	})
	t.Run("path cap declines", func(t *testing.T) {
		paths := make([]core.Derivation, compactAcceptanceElectionMaxLiveDerivations+1)
		for index := range paths {
			paths[index] = absorbed[0]
		}
		_, supported, err := s.electRecoveredRoot(s.headers[0], paths)
		if supported || err != nil {
			t.Fatalf("supported=%t err=%v", supported, err)
		}
	})
	t.Run("historical marker declines", func(t *testing.T) {
		s.s3RegionOpened = false
		defer func() { s.s3RegionOpened = true }()
		_, supported, err := s.electRecoveredRoot(s.headers[0], []core.Derivation{absorbed[0], absorbed[0]})
		if supported || err != nil {
			t.Fatalf("supported=%t err=%v", supported, err)
		}
	})
	t.Run("clean fork has no authority", func(t *testing.T) {
		s.headers[0].clearRecoveryLineage()
		_, supported, err := s.electRecoveredRoot(s.headers[0], []core.Derivation{absorbed[0], absorbed[0]})
		if supported || err != nil {
			t.Fatalf("supported=%t err=%v", supported, err)
		}
	})
}

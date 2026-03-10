//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"
)

func TestMergeGrammargenCGOFloorsPreservesRatchetDirection(t *testing.T) {
	existing := map[string]grammargenCGOFloorEntry{
		"c": {
			Eligible:    10,
			NoError:     7,
			TreeParity:  6,
			Divergences: 11,
		},
		"scala": {
			Eligible:    10,
			NoError:     0,
			TreeParity:  0,
			Divergences: 25,
		},
	}
	observed := map[string]grammargenCGOFloorEntry{
		"c": {
			Eligible:    9,
			NoError:     8,
			TreeParity:  5,
			Divergences: 14,
		},
		"json": {
			Eligible:    10,
			NoError:     10,
			TreeParity:  10,
			Divergences: 0,
		},
	}

	merged := mergeGrammargenCGOFloors(existing, observed)

	if got, want := merged["c"].Eligible, 10; got != want {
		t.Fatalf("c eligible = %d, want %d", got, want)
	}
	if got, want := merged["c"].NoError, 8; got != want {
		t.Fatalf("c no_error = %d, want %d", got, want)
	}
	if got, want := merged["c"].TreeParity, 6; got != want {
		t.Fatalf("c tree_parity = %d, want %d", got, want)
	}
	if got, want := merged["c"].Divergences, 11; got != want {
		t.Fatalf("c divergences = %d, want %d", got, want)
	}
	if got, want := merged["scala"], existing["scala"]; got != want {
		t.Fatalf("scala entry = %+v, want %+v", got, want)
	}
	if got, want := merged["json"], observed["json"]; got != want {
		t.Fatalf("json entry = %+v, want %+v", got, want)
	}
}

func TestGrammargenCGORatchetRegressionsIncludesDivergences(t *testing.T) {
	msgs := grammargenCGORatchetRegressions("c",
		grammargenCGOFloorEntry{
			Eligible:    10,
			NoError:     7,
			TreeParity:  6,
			Divergences: 11,
		},
		grammargenCGOFloorEntry{
			Eligible:    10,
			NoError:     7,
			TreeParity:  6,
			Divergences: 12,
		},
	)

	if len(msgs) != 1 {
		t.Fatalf("regressions = %v, want one divergence regression", msgs)
	}
	if !strings.Contains(msgs[0], "divergences") {
		t.Fatalf("regression message %q does not mention divergences", msgs[0])
	}
}

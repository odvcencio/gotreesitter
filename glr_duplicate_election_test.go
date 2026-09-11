package gotreesitter

import (
	"fmt"
	"testing"
)

func TestMergeDuplicateElectionAcrossAlgorithms(t *testing.T) {
	algorithms := []struct {
		name string
		run  func([]glrStack, *glrMergeScratch) []glrStack
	}{
		{"small", func(s []glrStack, m *glrMergeScratch) []glrStack { return mergeStacksSmallForLanguage(s, m, nil) }},
		{"small_deferred", func(s []glrStack, m *glrMergeScratch) []glrStack {
			m.perKeyCap = 1
			return mergeStacksSmallDeferExact(s, m, nil)
		}},
		{"hash", mergeStacksWithScratch},
		{"hash_deferred", func(s []glrStack, m *glrMergeScratch) []glrStack { return mergeStacksWithScratchDeferExact(s, m, 1) }},
		{"large_cap", func(s []glrStack, m *glrMergeScratch) []glrStack {
			return mergeStacksWithScratchLargeCap(s, m, maxStacksPerMergeKey+1)
		}},
	}
	for _, algorithm := range algorithms {
		for _, cEnabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/c=%v", algorithm.name, cEnabled), func(t *testing.T) {
				incumbent := makeRetentionTestStack(9, 3, true, 12)
				incumbent.branchOrder = 5
				candidate := incumbent
				candidate.branchOrder = 1
				stacks := []glrStack{incumbent, candidate}
				// Five inputs force the normal dispatcher through its hash path.
				for state := StateID(20); state < 23; state++ {
					stacks = append(stacks, makeRetentionTestStack(state, 3, true, 12))
				}
				scratch := &glrMergeScratch{parser: &Parser{errorCostCompetition: cEnabled}, perKeyCap: 2}
				defer scratch.reset()
				if !stackEquivalentForMergeState(scratch, nil, 9, &incumbent, &candidate) {
					t.Fatal("fixture does not prove duplicate equivalence")
				}
				result := algorithm.run(stacks, scratch)
				if len(result) != 4 {
					t.Fatalf("survivors=%d, want 4", len(result))
				}
				want := candidate.branchOrder
				if cEnabled {
					want = incumbent.branchOrder
				}
				found := false
				for _, survivor := range result {
					if survivor.top().state == 9 {
						found = true
						if survivor.branchOrder != want {
							t.Fatalf("survivor branch=%d, want %d", survivor.branchOrder, want)
						}
					}
				}
				if !found {
					t.Fatal("duplicate key has no survivor")
				}
			})
		}
	}
}

func TestMergeDuplicatePreservesCPhysicalIncumbent(t *testing.T) {
	incumbent := makeRetentionTestStack(9, 3, true, 12)
	incumbent.branchOrder = 5
	candidate := incumbent
	candidate.branchOrder = 1
	scratch := &glrMergeScratch{parser: &Parser{errorCostCompetition: true}}
	if stackMergeDuplicateShouldReplace(scratch, &candidate, &incumbent) {
		t.Fatal("grammar branch rank replaced the C physical incumbent")
	}
	scratch.parser.errorCostCompetition = false
	if !stackMergeDuplicateShouldReplace(scratch, &candidate, &incumbent) {
		t.Fatal("legacy duplicate election lost its branch rank")
	}
	scratch.parser.errorCostCompetition = true
	candidate.score++
	if !stackMergeDuplicateShouldReplace(scratch, &candidate, &incumbent) {
		t.Fatal("higher dynamic precedence did not replace the incumbent")
	}
	candidate.score = incumbent.score
	candidate.shifted = false
	if !stackMergeDuplicateShouldReplace(scratch, &candidate, &incumbent) {
		t.Fatal("pending token work did not replace the shifted incumbent")
	}
}

func TestMergeDuplicateElectionUsesActiveParserLifetime(t *testing.T) {
	incumbent := makeRetentionTestStack(9, 3, true, 12)
	incumbent.branchOrder = 5
	candidate := incumbent
	candidate.branchOrder = 1
	p := &Parser{errorCostCompetition: true, noTreeBenchmarkOnly: true}
	scratch := &glrMergeScratch{parser: p}
	if !stackMergeDuplicateShouldReplace(scratch, &candidate, &incumbent) {
		t.Fatal("no-tree benchmark changed duplicate election")
	}
	p.noTreeBenchmarkOnly = false
	scratch.reset()
	if scratch.parser != nil || !stackMergeDuplicateShouldReplace(scratch, &candidate, &incumbent) {
		t.Fatal("pooled scratch retained the previous parser's election")
	}
}

package parsercorephase0

import (
	"errors"
	"testing"
)

type rejectRecoveryContainerPlan struct{}

func (rejectRecoveryContainerPlan) ReductionPlan(uint16, int) (ReductionPlan, error) {
	return ReductionPlan{}, errors.New("synthetic ERROR requested a grammar production")
}

func TestRecoveryErrorMaterializesWithoutGrammarProduction(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	c.plans = rejectRecoveryContainerPlan{}
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	children := make([]SubtreeID, 5)
	for i := range children {
		children[i], err = c.ErrorRegionLeaf(1, uint32(i), uint32(i+1), false)
		if err != nil {
			t.Fatal(err)
		}
	}
	head, err := c.ErrorRegionResume(seed, 1, 0, 5, children)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := c.Derivations(head)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.MaterializationOrder(paths[0].Payloads, nil); err != nil {
		t.Fatal(err)
	}
	root := paths[0].Payloads[0]
	for name, mutate := range map[string]func(*subtreeRecord){
		"production": func(r *subtreeRecord) { r.productionID = 1 },
		"precedence": func(r *subtreeRecord) { r.dynamicPrecedence = 1 },
		"missing":    func(r *subtreeRecord) { r.missing = true },
		"fields":     func(r *subtreeRecord) { r.fieldCount = 1 },
		"aliases":    func(r *subtreeRecord) { r.aliasCount = 1 },
	} {
		record := c.subtrees[root-1]
		mutate(&record)
		if err := c.validateGenericMaterializationMetadata(root, record); err == nil {
			t.Fatalf("ERROR with forged %s passed validation", name)
		}
	}
	duplicate, err := c.ErrorRegionResume(seed, 2, 0, 1, []SubtreeID{children[0], children[0]})
	if err != nil {
		t.Fatal(err)
	}
	paths, err = c.Derivations(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.MaterializationOrder(paths[0].Payloads, nil); err == nil {
		t.Fatal("ERROR with repeated child ownership passed validation")
	}
}

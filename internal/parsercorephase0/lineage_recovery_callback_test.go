package parsercorephase0

import "testing"

type recoveryCostPlanCallbackTable struct {
	fakeTable
	onPlan func() error
	onGoto func()
}

func (t *recoveryCostPlanCallbackTable) ReductionPlan(id uint16, count int) (ReductionPlan, error) {
	if t.onPlan != nil {
		if err := t.onPlan(); err != nil {
			return ReductionPlan{}, err
		}
	}
	return NewReductionPlan(id, count, nil, nil)
}
func (t *recoveryCostPlanCallbackTable) Goto(state StateID, symbol Symbol) (StateID, error) {
	if t.onGoto != nil {
		t.onGoto()
	}
	return t.fakeTable.Goto(state, symbol)
}
func TestLineageRecoveryCostReadAfterPlanCallback(t *testing.T) {
	tables := &recoveryCostPlanCallbackTable{fakeTable: fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}}
	c, err := New(tables, Limits{MaxPopPaths: 8, MaxDerivations: 8})
	if err != nil {
		t.Fatal(err)
	}
	planCalls, gotoCalls := 0, 0
	err = c.RunFreshSchedulerSession(func(parent SchedulerTransactionToken) error {
		return c.ApplySchedulerSpeculation(parent, func(owner SchedulerTransactionToken) (bool, error) {
			seed, err := c.Seed(1, 0)
			if err != nil {
				return false, err
			}
			var head Head
			for _, symbol := range []Symbol{7, 8} {
				head, err = c.appendDiagnosticPayload(seed, 3, Token{Symbol: symbol, EndByte: 1}, pathMeta{})
				if err != nil {
					return false, err
				}
			}
			paths, err := c.popPaths(head.Node, 1)
			if err != nil {
				return false, err
			}
			if len(paths) != 2 {
				t.Fatalf("fixture has %d paths, want two", len(paths))
			}
			tables.onPlan = func() error {
				planCalls++
				return c.RecordHeadStoredErrorCostOwned(owner, head, 17)
			}
			tables.onGoto = func() {
				gotoCalls++
				if c.reductionScratch.selectCleanChildren {
					t.Error("clean-child selection used recovery cost from before the plan callback")
				}
			}
			boundary, err := c.ClassifyBoundary(head, 9)
			if err != nil {
				return false, err
			}
			c.reductionScratch.pressureSeen = true
			_, err = c.ReduceOutputsClassifiedIntoOwned(owner, nil, boundary, 0, ForkOrder{})
			return true, err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if planCalls != 1 || gotoCalls != 2 {
		t.Fatalf("plan/goto calls=%d/%d, want 1/2", planCalls, gotoCalls)
	}
}

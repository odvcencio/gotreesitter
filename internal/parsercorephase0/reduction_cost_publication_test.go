package parsercorephase0

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func newReductionCostPublicationFixture(t *testing.T) (*Core, ClassifiedBoundary) {
	t.Helper()
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}
	compact, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.ShiftMissingLeaf(seed, 3, 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := compact.ClassifyBoundary(head, 9)
	if err != nil {
		t.Fatal(err)
	}
	return compact, boundary
}

func TestReductionCostCallbackPublishesCumulativeMissingCost(t *testing.T) {
	compact, boundary := newReductionCostPublicationFixture(t)
	var outputs []ReductionOutput
	callbackCalls := 0
	err := compact.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		var err error
		outputs, err = compact.ReduceOutputsClassifiedIntoWithCostOwned(
			owner, nil, boundary, 0, ForkOrder{},
			func(prev NodeID, payload SubtreeID) (uint32, error) {
				callbackCalls++
				view, viewErr := compact.Subtree(payload)
				if viewErr != nil {
					return 0, viewErr
				}
				if len(view.Children) != 1 {
					return 0, errors.New("reduction callback received an unexpected parent shape")
				}
				child, childErr := compact.Subtree(view.Children[0])
				if childErr != nil {
					return 0, childErr
				}
				if !child.Missing {
					return 0, errors.New("reduction callback lost the missing child")
				}
				prefix, prefixErr := compact.RecoveryStoredErrorCost(Head{Node: prev})
				if prefixErr != nil {
					return 0, prefixErr
				}
				return prefix + compactMissingLeafStoredErrorCost, nil
			},
		)
		return err
	})
	if err != nil {
		t.Fatalf("cost-aware reduction: %v", err)
	}
	if callbackCalls != 1 || len(outputs) != 1 {
		t.Fatalf("callback calls=%d outputs=%d, want one callback and one output", callbackCalls, len(outputs))
	}
	cost, err := compact.RecoveryStoredErrorCost(outputs[0].Head)
	if err != nil {
		t.Fatal(err)
	}
	if cost != compactMissingLeafStoredErrorCost {
		t.Fatalf("reduction output stored cost=%d, want %d before any boundary merge", cost, compactMissingLeafStoredErrorCost)
	}
}

func TestCostAwareReductionRejectsNilCallbackBeforeMutation(t *testing.T) {
	for _, mode := range []string{"generic", "generic-live", "corridor"} {
		t.Run(mode, func(t *testing.T) {
			compact, boundary := newReductionCostPublicationFixture(t)
			before := captureSchedulerTransactionState(compact)
			err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				if mode == "corridor" {
					_, _ = compact.ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesAndCostOwned(
						owner, nil, nil, boundary, ForkOrder{}, nil,
					)
					return nil
				}
				if mode == "generic" {
					_, _ = compact.ReduceOutputsClassifiedIntoWithCostOwned(
						owner, nil, boundary, 0, ForkOrder{}, nil,
					)
					return nil
				}
				_, _ = compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesAndCostOwned(
					owner, nil, nil, boundary, 0, ForkOrder{}, nil,
				)
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), "cost callback is required") {
				t.Fatalf("nil cost callback error=%v", err)
			}
			if got := captureSchedulerTransactionState(compact); !reflect.DeepEqual(got, before) {
				t.Fatalf("nil cost callback mutated compact state: got=%+v before=%+v", got, before)
			}
		})
	}
}

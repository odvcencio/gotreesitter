package parsercorephase0

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func TestSingleVersionOneChildPopPreservesPathAndFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Core, Head)
	}{
		{"ordered", func(*Core, Head) {}},
		{"absent-order", func(c *Core, _ Head) { c.links[0].flags &^= linkFlagHasOrder }},
		{"pop-cap", func(c *Core, _ Head) { c.limits.MaxPopPaths = 0 }},
		{"boundary-cap", func(c *Core, _ Head) { c.limits.MaxLinksPerBoundary = 0 }},
		{"missing-adjacency", func(c *Core, h Head) { c.nodes[h.Node-1].firstLink = 0 }},
		{"bad-adjacency", func(c *Core, h Head) { c.nodes[h.Node-1].firstLink = uint32(len(c.links) + 1) }},
		{"adjacency-cycle", func(c *Core, _ Head) { c.links[0].next = 1 }},
		{"predecessor-cycle", func(c *Core, h Head) { c.links[0].prev = h.Node }},
		{"missing-payload", func(c *Core, _ Head) { c.links[0].payload = 0 }},
		{"extra-fallback", func(c *Core, _ Head) { c.subtrees[0].extra = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, head := newSingleVersionReductionFixture(t, 1, 0)
			test.mutate(compact, head)
			compact.condenseScopeActive = true
			want, wantErr := compact.popPaths(head.Node, 1)
			want = slices.Clone(want)
			for index := range want {
				want[index].children = slices.Clone(want[index].children)
				want[index].trailing = slices.Clone(want[index].trailing)
			}
			got, gotErr := compact.reductionPopPaths(head.Node, 1)
			if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || !reflect.DeepEqual(got, want) {
				t.Fatalf("one-child pop drifted: got=%+v err=%v want=%+v err=%v", got, gotErr, want, wantErr)
			}
		})
	}
}

func TestSingleVersionReductionMatchesAggregatedOutput(t *testing.T) {
	for _, childCount := range []uint8{0, 1, 2, 3, 4} {
		for _, extras := range []int{0, 2} {
			t.Run(fmt.Sprintf("children=%d/extras=%d", childCount, extras), func(t *testing.T) {
				var want []ReductionOutput
				var wantState schedulerTransactionState
				var wantPaths []Derivation
				for _, single := range []bool{false, true} {
					compact, head := newSingleVersionReductionFixture(t, childCount, extras)
					var outputs []ReductionOutput
					err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
						boundary, err := compact.ClassifyBoundary(head, 1)
						if err != nil {
							return err
						}
						dst := make([]ReductionOutput, 0, 1)
						if single {
							outputs, err = compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, dst, boundary, 0, ForkOrder{})
						} else {
							outputs, err = compact.ReduceOutputsClassifiedIntoOwned(owner, dst, boundary, 0, ForkOrder{})
						}
						return err
					})
					if err != nil || len(outputs) != 1 {
						t.Fatalf("single=%t outputs=%+v err=%v", single, outputs, err)
					}
					paths, err := compact.Derivations(outputs[0].Head)
					if err != nil {
						t.Fatal(err)
					}
					state := captureSchedulerTransactionState(compact)
					if !single {
						want, wantState, wantPaths = outputs, state, paths
						continue
					}
					if !reflect.DeepEqual(outputs, want) || !reflect.DeepEqual(state, wantState) || !reflect.DeepEqual(paths, wantPaths) {
						t.Fatalf("direct output differs from aggregation: got=%+v want=%+v state=%+v wantState=%+v paths=%+v wantPaths=%+v", outputs, want, state, wantState, paths, wantPaths)
					}
					if cap(compact.reductionScratch.boundaries) != 0 {
						t.Fatal("sole output allocated boundary aggregation storage")
					}
				}
			})
		}
	}
}

func TestSingleVersionReductionPreservesHistoricalBoundary(t *testing.T) {
	compact, head := newSingleVersionReductionFixture(t, 1, 0)
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		for repeat := 0; repeat < 2; repeat++ {
			boundary, err := compact.ClassifyBoundary(head, 1)
			if err != nil {
				return err
			}
			outputs, err := compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, nil, boundary, 0, ForkOrder{})
			if err != nil {
				return err
			}
			if repeat == 1 && (len(outputs) != 1 || outputs[0].HistoricalBoundaryProvenance != HistoricalBoundaryDeterministic) {
				t.Fatalf("historical provenance lost: %+v", outputs)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSingleVersionReductionRollsBack(t *testing.T) {
	compact, head := newSingleVersionReductionFixture(t, 1, 2)
	before := captureSchedulerTransactionState(compact)
	reject := errors.New("reject after sole reduction")
	err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		boundary, err := compact.ClassifyBoundary(head, 1)
		if err != nil {
			return err
		}
		if _, err := compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, nil, boundary, 0, ForkOrder{}); err != nil {
			return err
		}
		return reject
	})
	if !errors.Is(err, reject) || !reflect.DeepEqual(captureSchedulerTransactionState(compact), before) {
		t.Fatalf("sole reduction rollback failed: err=%v", err)
	}
}

func newSingleVersionReductionFixture(t *testing.T, childCount uint8, extras int) (*Core, Head) {
	t.Helper()
	tables := diagnosticReduceTable(childCount, 3)
	if childCount == 0 {
		tables.gotos[tableCell{state: 3, symbol: 2}] = 4
	}
	compact, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < int(childCount)+extras; index++ {
		head, err = compact.appendDiagnosticPayload(head, 3, Token{
			Symbol: Symbol(10 + index), StartByte: uint32(index), EndByte: uint32(index + 1), Extra: index >= int(childCount),
		}, pathMeta{ScoreDelta: int64(index + 1), BranchOrder: ForkOrder{Value: uint64(index + 1), Present: true}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if childCount == 0 && extras == 0 {
		tables.actions[tableCell{state: 1, symbol: 1}] = tables.actions[tableCell{state: 3, symbol: 1}]
	}
	return compact, head
}

func TestShortSingleVersionPopMatchesGeneric(t *testing.T) {
	for _, count := range []uint8{2, 3, 4} {
		for _, mutate := range []func(*Core){
			func(*Core) {},
			func(c *Core) { c.links[0].flags &^= linkFlagHasOrder },
			func(c *Core) { c.links[0].scoreDelta = 1<<63 - 1 },
			func(c *Core) { c.links[0].next = 1 },
			func(c *Core) { c.subtrees[0].extra = true },
			func(c *Core) { c.limits.MaxPopPaths = 0 },
		} {
			c, h := newSingleVersionReductionFixture(t, count, 0)
			mutate(c)
			c.condenseScopeActive = true
			want, we := c.popPaths(h.Node, int(count))
			want = slices.Clone(want)
			for i := range want {
				want[i].children = slices.Clone(want[i].children)
				want[i].trailing = slices.Clone(want[i].trailing)
			}
			got, ge := c.reductionPopPaths(h.Node, int(count))
			if fmt.Sprint(ge) != fmt.Sprint(we) || !reflect.DeepEqual(got, want) {
				t.Fatalf("count=%d got=%+v err=%v want=%+v err=%v", count, got, ge, want, we)
			}
		}
	}
}

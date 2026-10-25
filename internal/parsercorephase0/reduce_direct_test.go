package parsercorephase0

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type directReductionCountingTable struct {
	*fakeTable
	calls int
}

func (c *directReductionCountingTable) Goto(state StateID, symbol Symbol) (StateID, error) {
	c.calls++
	return c.fakeTable.Goto(state, symbol)
}

func TestDirectReductionFallbackReusesGoto(t *testing.T) {
	c, boundary := directReductionFixture(t, 1, false)
	if _, err := c.ReduceOutputs(boundary.head, 10, 0, ForkOrder{}); err != nil {
		t.Fatal(err)
	}
	boundary, err := c.ClassifyBoundary(boundary.head, 10)
	if err != nil {
		t.Fatal(err)
	}
	table := &directReductionCountingTable{fakeTable: c.tables.(*fakeTable)}
	c.tables = table
	if err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		_, handled, err := c.TryReduceDirectOwned(owner, &boundary, ShiftHeadLineage{Owner: 3})
		if handled || err != nil {
			t.Fatalf("historical boundary: handled=%t err=%v", handled, err)
		}
		_, err = c.ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, nil, boundary, ForkOrder{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if table.calls != 1 {
		t.Fatalf("fallback made %d GOTO lookups, want one", table.calls)
	}
}

func TestDirectReductionFailurePoisonsFreshOwner(t *testing.T) {
	if phase0AEnabled {
		return // Diagnostic builds always use the general reducer.
	}
	c, boundary := directReductionFixture(t, 1, false)
	c.limits.MaxSubtrees = uint32(c.subtrees.Len())
	err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		_, handled, err := c.TryReduceDirectOwned(owner, &boundary, ShiftHeadLineage{Owner: 3})
		if handled || err == nil || !strings.Contains(err.Error(), "subtree arena cap") {
			t.Fatalf("cap result: handled=%t err=%v", handled, err)
		}
		return nil // Ignoring the operation error must still discard the session.
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned fresh scheduler session") || c.nodes.Len() != 0 || c.subtrees.Len() != 0 || c.Work() != (Work{}) {
		t.Fatalf("ignored error escaped the owner: err=%v work=%+v", err, c.Work())
	}
}

func TestDirectReductionObservesCallbackRecoveryCost(t *testing.T) {
	if phase0AEnabled {
		return
	}
	for _, stage := range []string{"plan", "goto"} {
		t.Run(stage, func(t *testing.T) {
			c, boundary := directReductionFixture(t, 1, false)
			table := &recoveryCostPlanCallbackTable{fakeTable: *c.tables.(*fakeTable)}
			c.tables, c.plans = table, table
			before := captureSchedulerTransactionState(c)
			if err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
				publish := func() error { return c.publishInheritedStoredErrorCost(boundary.head, 17) }
				if stage == "plan" {
					table.onPlan = publish
				} else {
					table.onGoto = func() {
						if err := publish(); err != nil {
							t.Fatal(err)
						}
					}
				}
				_, handled, err := c.TryReduceDirectOwned(owner, &boundary, ShiftHeadLineage{Owner: 3})
				if handled || err != nil {
					t.Fatalf("callback recovery: handled=%t err=%v", handled, err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, captureSchedulerTransactionState(c)) {
				t.Fatal("callback recovery published a direct reduction")
			}
			if cost, err := c.RecoveryStoredErrorCost(boundary.head); err != nil || cost != 17 {
				t.Fatalf("callback cost=%d err=%v", cost, err)
			}
		})
	}
}

func directReductionFixture(t *testing.T, count int, extra bool) (*Core, ClassifiedBoundary) {
	t.Helper()
	tables := &fakeTable{actions: map[tableCell][]Action{}, gotos: map[tableCell]StateID{{state: 1, symbol: 80}: 400}, fields: map[uint16][]FieldMapEntry{}, aliases: map[productionKey][]Symbol{}}
	for i := 0; i < count; i++ {
		tables.actions[tableCell{state: StateID(i + 1), symbol: 9}] = []Action{{Type: ActionShift, State: StateID(i + 2), Extra: extra && i == count-1}}
	}
	tables.actions[tableCell{state: StateID(count + 1), symbol: 10}] = []Action{{Type: ActionReduce, Symbol: 80, ChildCount: uint8(count), ProductionID: 7, DynamicPrecedence: -3}}
	if count > 0 {
		tables.fields[7] = []FieldMapEntry{{FieldID: 1, ChildIndex: uint8(count - 1)}}
		aliases := make([]Symbol, count)
		aliases[0] = 11
		tables.aliases[productionKey{productionID: 7, childCount: count}] = aliases
	}
	c, err := New(tables, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		head, err = c.Shift(head, 9, 0, Token{Symbol: 9, StartByte: uint32(i), EndByte: uint32(i + 1), Extra: extra && i == count-1}, ForkOrder{Present: i%2 == 0, Value: uint64(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	boundary, err := c.ClassifyBoundary(head, 10)
	if err != nil {
		t.Fatal(err)
	}
	return c, boundary
}

func TestDirectReductionMatchesGeneralPublication(t *testing.T) {
	for _, count := range []int{0, 1, 2, 8, 32} {
		for _, converged := range []bool{false, true} {
			t.Run(fmt.Sprintf("children=%d/converged=%t", count, converged), func(t *testing.T) {
				before, oldBoundary := directReductionFixture(t, count, false)
				after, newBoundary := directReductionFixture(t, count, false)
				if count == 2 {
					// The generic pop uses the first forward child's end when
					// the final child leaves the reverse extent at zero.
					for _, c := range []*Core{before, after} {
						last := c.subtrees.At(1)
						last.startByte, last.endByte = 0, 0
					}
				}
				publication := ShiftHeadLineage{Owner: 3, PublishLineage: converged, Set: NewAlternativeSetMember(2, 0), Lineage: 2, Rank: CleanPathRankSelected, SetDirty: true}
				var oldHead, newHead Head
				if err := before.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
					outputs, err := before.ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, nil, oldBoundary, ForkOrder{})
					if err != nil {
						return err
					}
					if len(outputs) != 1 || outputs[0].Freshness != ReductionNew || outputs[0].HistoricalBoundaryProvenance != HistoricalBoundaryNone {
						t.Fatalf("unexpected control output: %+v", outputs)
					}
					oldHead = outputs[0].Head
					if err := before.RecordHeadOwnerOwned(owner, oldHead, publication.Owner); err != nil {
						return err
					}
					if converged {
						return before.publishShiftLineage(oldHead, publication)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if err := after.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
					var handled bool
					var err error
					newHead, handled, err = after.TryReduceDirectOwned(owner, &newBoundary, publication)
					if phase0AEnabled {
						if handled || err != nil {
							t.Fatal("diagnostic reducer did not retain general path")
						}
						return nil
					}
					if !handled && err == nil {
						t.Fatal("deterministic reduction declined")
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if phase0AEnabled {
					return
				}
				for name, pair := range map[string][2]any{
					"head": {oldHead, newHead}, "state": {captureSchedulerTransactionState(before), captureSchedulerTransactionState(after)},
					"nodes": {before.nodes.snapshotForTest(), after.nodes.snapshotForTest()}, "lineages": {before.nodeLineages, after.nodeLineages},
					"links": {before.links.Values(), after.links.Values()}, "subtrees": {before.subtrees.snapshotForTest(), after.subtrees.snapshotForTest()},
					"children": {before.children, after.children}, "fields": {before.fields, after.fields}, "aliases": {before.aliases, after.aliases},
				} {
					if !reflect.DeepEqual(pair[0], pair[1]) {
						t.Errorf("%s differs: before=%+v after=%+v", name, pair[0], pair[1])
					}
				}
			})
		}
	}
}

func TestDirectReductionDeclinesWithoutPublication(t *testing.T) {
	for _, tc := range []struct {
		name    string
		count   int
		extra   bool
		prepare func(*Core, ClassifiedBoundary)
	}{
		{name: "wide", count: 33},
		{name: "extra", count: 2, extra: true},
		{name: "recovery", count: 1, prepare: func(c *Core, b ClassifiedBoundary) {
			if err := c.publishInheritedStoredErrorCost(b.head, 1); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "historical", count: 1, prepare: func(c *Core, b ClassifiedBoundary) {
			if _, err := c.ReduceOutputs(b.head, 10, 0, ForkOrder{}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, boundary := directReductionFixture(t, tc.count, tc.extra)
			if tc.prepare != nil {
				tc.prepare(c, boundary)
			}
			// A previous standalone operation changes classification identity.
			var err error
			boundary, err = c.ClassifyBoundary(boundary.head, 10)
			if err != nil {
				t.Fatal(err)
			}
			before := captureSchedulerTransactionState(c)
			if err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
				_, handled, err := c.TryReduceDirectOwned(owner, &boundary, ShiftHeadLineage{Owner: 3})
				if handled || err != nil {
					t.Fatalf("refusal: handled=%t err=%v", handled, err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, captureSchedulerTransactionState(c)) {
				t.Fatal("refusal changed graph or counters")
			}
		})
	}
}

func TestDirectReductionGotoMemoDoesNotCrossFrontier(t *testing.T) {
	if phase0AEnabled {
		t.Skip("diagnostic build uses the general reducer")
	}
	c, boundary := directReductionFixture(t, 1, false)
	if _, err := c.ReduceOutputs(boundary.head, 10, 0, ForkOrder{}); err != nil {
		t.Fatal(err)
	}
	boundary, err := c.ClassifyBoundary(boundary.head, 10)
	if err != nil {
		t.Fatal(err)
	}
	table := &directReductionCountingTable{fakeTable: c.tables.(*fakeTable)}
	c.tables = table
	if err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		if _, handled, err := c.TryReduceDirectOwned(owner, &boundary, ShiftHeadLineage{Owner: 3}); handled || err != nil {
			t.Fatalf("historical boundary: handled=%t err=%v", handled, err)
		}
		if err := c.BeginFrontier(); err != nil {
			return err
		}
		next, err := c.ClassifyBoundary(boundary.head, 10)
		if err != nil {
			return err
		}
		_, err = c.ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, nil, next, ForkOrder{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if table.calls != 2 {
		t.Fatalf("new frontier reused stale GOTO: calls=%d, want two", table.calls)
	}
}

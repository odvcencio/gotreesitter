package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

type singleVersionTestAdapter struct {
	table         *fakeTable
	before, after int
	stop          int
}

func (a *singleVersionTestAdapter) SingleVersionRow(state StateID, symbol Symbol) (ActionRow, bool) {
	row, err := a.table.Actions(state, symbol)
	return row, err == nil && row.Len() == 1
}
func (a *singleVersionTestAdapter) BeforeSingleVersion(ActionType) error {
	a.before++
	if a.before == a.stop {
		return errors.New("cancel single version")
	}
	return nil
}
func (a *singleVersionTestAdapter) AfterSingleVersion(Head, ActionType, bool) error {
	a.after++
	return nil
}
func singleVersionFixture(t *testing.T) (*Core, Head, *singleVersionTestAdapter) {
	c, b := directReductionFixture(t, 1, false)
	table := c.tables.(*fakeTable)
	table.actions[tableCell{state: 400, symbol: 10}] = []Action{{Type: ActionReduce, Symbol: 81, ChildCount: 1}}
	table.actions[tableCell{state: 500, symbol: 10}] = []Action{{Type: ActionShift, State: 600}}
	table.gotos[tableCell{state: 1, symbol: 81}] = 500
	return c, b.head, &singleVersionTestAdapter{table: table}
}
func TestSingleVersionMatchesSeparateOperations(t *testing.T) {
	if phase0AEnabled {
		return
	}
	for _, limit := range []uint32{0, 1, 2, 3, 20} {
		t.Run(string(rune('a'+limit)), func(t *testing.T) {
			before, oldHead, _ := singleVersionFixture(t)
			after, newHead, adapter := singleVersionFixture(t)
			pub := ShiftHeadLineage{Owner: 3, PublishLineage: true, Set: NewAlternativeSetMember(2, 0), Lineage: 2, Rank: CleanPathRankSelected, SetDirty: true}
			tok := Token{Symbol: 10, StartByte: 1, EndByte: 2}
			want := min(limit, 3)
			err := before.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
				for i := uint32(0); i < want; i++ {
					var err error
					if i < 2 {
						b, e := before.ClassifyBoundary(oldHead, 10)
						if e != nil {
							return e
						}
						o, e := before.ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesOwned(owner, nil, nil, b, ForkOrder{})
						if e != nil {
							return e
						}
						oldHead = o[0].Head
						err = before.RecordHeadOwnerOwned(owner, oldHead, pub.Owner)
						if err == nil {
							err = before.publishShiftLineage(oldHead, pub)
						}
					} else {
						oldHead, _, err = before.ShiftDirectAndPublishLineageOwned(owner, nil, oldHead, 10, 600, false, tok, ForkOrder{}, pub)
					}
					if err != nil {
						return err
					}
					pub.PreviousHead = oldHead
					pub.SetDirty = false
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			pub.PreviousHead = Head{}
			pub.SetDirty = true
			err = after.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
				result, e := after.RunSingleVersionOwned(owner, newHead, tok, pub, limit, adapter, adapter)
				newHead = result.Head
				if e == nil && (result.Steps != want || result.Shifted != (want == 3) || result.ReductionFallback) {
					t.Fatalf("result=%+v", result)
				}
				return e
			})
			if err != nil {
				t.Fatal(err)
			}
			for name, pair := range map[string][2]any{
				"head": {oldHead, newHead}, "work": {before.Work(), after.Work()}, "nodes": {before.nodes.snapshotForTest(), after.nodes.snapshotForTest()}, "lineage": {before.nodeLineages, after.nodeLineages}, "links": {before.links.Values(), after.links.Values()}, "subtrees": {before.subtrees.snapshotForTest(), after.subtrees.snapshotForTest()}, "children": {before.children, after.children},
			} {
				if !reflect.DeepEqual(pair[0], pair[1]) {
					t.Errorf("%s differs: %v vs %v", name, pair[0], pair[1])
				}
			}
			if adapter.after != int(want) || adapter.before != int(want) {
				t.Fatalf("observer before=%d after=%d", adapter.before, adapter.after)
			}
		})
	}
}
func TestSingleVersionExternalShiftKeepsCheckpoint(t *testing.T) {
	if phase0AEnabled {
		return
	}
	for _, missing := range []bool{false, true} {
		c, h, a := singleVersionFixture(t)
		c.EnableTerminalScannerCheckpointProvenance()
		before, e := c.InternCheckpoint([]byte("before"))
		if e != nil {
			t.Fatal(e)
		}
		after, e := c.InternCheckpoint([]byte("after"))
		if e != nil {
			t.Fatal(e)
		}
		if !missing {
			if e = c.SetPhaseExternalTokenScannerCheckpoints(before, after); e != nil {
				t.Fatal(e)
			}
		}
		var last Head
		var result SingleVersionResult
		err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
			r, e := c.RunSingleVersionOwned(owner, h, Token{Symbol: 10, StartByte: 1, EndByte: 2, External: true}, ShiftHeadLineage{Owner: 3}, 20, a, a)
			last, result = r.Head, r
			if !missing && e == nil && (r.Steps != 3 || !r.Shifted || r.ReductionFallback) {
				t.Fatalf("external result=%+v", r)
			}
			return e
		})
		if missing {
			if err != nil || result.Steps != 2 || result.Shifted || a.before != 2 || a.after != 2 {
				t.Fatalf("unbound external shift was consumed: %+v err=%v", result, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		payload, e := c.SoleHeadPayload(last)
		if e != nil {
			t.Fatal(e)
		}
		view, e := c.MaterializationView(payload)
		if e != nil {
			t.Fatal(e)
		}
		if !view.ExternalScannerCheckpointExact || view.ExternalScannerCheckpointStart != before || view.ExternalScannerCheckpointEnd != after {
			t.Fatalf("checkpoint lost: %+v", view)
		}
		if a.before != 3 || a.after != 3 {
			t.Fatal("external shift missed observer work")
		}
	}
}
func TestSingleVersionCancelPoisonsSession(t *testing.T) {
	if phase0AEnabled {
		return
	}
	c, h, a := singleVersionFixture(t)
	a.stop = 2
	err := c.RunFreshSchedulerSession(func(owner SchedulerTransactionToken) error {
		r, e := c.RunSingleVersionOwned(owner, h, Token{Symbol: 10, EndByte: 2}, ShiftHeadLineage{Owner: 3}, 20, a, a)
		if e == nil || r.Steps != 1 {
			t.Fatalf("cancel=%+v err=%v", r, e)
		}
		return nil
	})
	if err == nil || c.nodes.Len() != 0 || c.Work() != (Work{}) {
		t.Fatal("ignored cancellation escaped fresh owner")
	}
}

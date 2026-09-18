package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

func flattenRecoveryRepeats(t *testing.T, c *Core, children []SubtreeID) []SubtreeID {
	t.Helper()
	var out []SubtreeID
	for _, id := range children {
		view, err := c.MaterializationView(id)
		if err != nil {
			t.Fatal(err)
		}
		if view.Symbol == RecoveryErrorRepeatSymbol {
			out = append(out, flattenRecoveryRepeats(t, c, view.Children)...)
		} else {
			out = append(out, id)
		}
	}
	return out
}

func TestRecoveryErrorRepeatOwnershipCostAndRollback(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	c.plans = rejectRecoveryContainerPlan{}
	lexical := appendAncestorRecoveryPayload(t, c, ErrorRegionSymbol, 0, 1, false)
	second := appendAncestorRecoveryPayload(t, c, 1, 1, 2, false)
	third := appendAncestorRecoveryPayload(t, c, 1, 2, 3, false)
	children := []SubtreeID{lexical, second, third}
	for n := 1; n <= len(children); n++ {
		var repeat SubtreeID
		err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			var err error
			repeat, err = c.RecoveryErrorRepeatOwned(owner, children[:n])
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		order, err := c.MaterializationOrder([]SubtreeID{repeat}, nil)
		if err != nil || len(order) != n*3-1 {
			t.Fatalf("n=%d order=%v err=%v", n, order, err)
		}
		if got := flattenRecoveryRepeats(t, c, []SubtreeID{repeat}); !reflect.DeepEqual(got, children[:n]) {
			t.Fatalf("children=%v", got)
		}
		src := fakeRecoveryCostSource{}
		for _, id := range order {
			v, err := c.MaterializationView(id)
			if err != nil {
				t.Fatal(err)
			}
			src[id] = RecoveryCostNode{Symbol: v.Symbol, Extra: v.Extra, StartByte: v.StartByte, EndByte: v.EndByte, Children: v.Children}
		}
		symbols := []SelectedSymbolPolicy{{}, {Visible: true}}
		got, err := RecoveryNodeErrorCost(symbols, src, repeat)
		want := []uint32{501, 702, 803}[n-1]
		if err != nil || got != want {
			t.Fatalf("repeat n=%d cost=%d want=%d err=%v", n, got, want, err)
		}
		got, err = RecoveryErrorRegionCost(symbols, src, nil, 0, 0, uint32(n), 0, []SubtreeID{repeat})
		if want = uint32(500 + n + 100*n); err != nil || got != want {
			t.Fatalf("ERROR n=%d cost=%d want=%d err=%v", n, got, want, err)
		}
		for name, mutate := range map[string]func(*subtreeRecord){
			"production": func(r *subtreeRecord) { r.productionID = 1 }, "extra": func(r *subtreeRecord) { r.extra = true },
			"missing": func(r *subtreeRecord) { r.missing = true }, "terminal": func(r *subtreeRecord) { r.terminal = true },
			"precedence": func(r *subtreeRecord) { r.dynamicPrecedence = 1 }, "empty": func(r *subtreeRecord) { r.childCount = 0 },
			"fields": func(r *subtreeRecord) { r.fieldCount = 1 }, "aliases": func(r *subtreeRecord) { r.aliasCount = 1 },
		} {
			r := c.subtrees[repeat-1]
			mutate(&r)
			if c.validateGenericMaterializationMetadata(repeat, r) == nil {
				t.Fatalf("forged %s accepted", name)
			}
		}
	}
	before := len(c.subtrees)
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if _, err := c.RecoveryErrorRepeatOwned(owner, children); err != nil {
			return err
		}
		return errors.New("abort")
	})
	if err == nil || len(c.subtrees) != before {
		t.Fatal("repeat survived rollback")
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _ = c.RecoveryErrorRepeatOwned(owner, []SubtreeID{lexical, lexical})
		return nil
	})
	if err == nil || len(c.subtrees) != before {
		t.Fatal("swallowed repeat error committed")
	}
}

func TestRecoveryErrorRepeatSelectedMaterialization(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	children := []SubtreeID{
		appendAncestorRecoveryPayload(t, c, 1, 0, 1, false),
		appendAncestorRecoveryPayload(t, c, 1, 1, 2, false),
		appendAncestorRecoveryPayload(t, c, 1, 2, 3, false),
	}
	var repeat SubtreeID
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		repeat, err = c.RecoveryErrorRepeatOwned(owner, children)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 3}, []SubtreeID{repeat}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := c.BuildSelectedStore([]SubtreeID{root}, selectedStoreTestPolicy(t, 2, 1), []byte("abc"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.NodeCount() != 4 {
		t.Fatalf("hidden repeats remained: nodes=%d", store.NodeCount())
	}
	r, _ := store.Record(store.Root())
	for i := 0; i < 3; i++ {
		id, ok := store.Child(r, uint32(i))
		if !ok {
			t.Fatal("missing child")
		}
		child, ok := store.Record(id)
		if !ok || child.Symbol != 1 || child.StartByte != uint32(i) {
			t.Fatalf("child=%+v valid=%v", child, ok)
		}
	}
}

func TestRecoveryErrorRepeatPushCheckpointCostAndRollback(t *testing.T) {
	c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
	checkpoint := mustInternCheckpoint(t, c, []byte("source"))
	if err := c.SetPhaseCheckpoint(checkpoint); err != nil {
		t.Fatal(err)
	}
	seed, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var marker Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		marker, err = c.AppendRecoveryDiscontinuityOwned(owner, seed, RecoveryDiscontinuityContext{ByteOffset: 0, Checkpoint: checkpoint})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	children := []SubtreeID{appendAncestorRecoveryPayload(t, c, 1, 0, 1, false), appendAncestorRecoveryPayload(t, c, 1, 1, 2, false)}
	other := mustInternCheckpoint(t, c, []byte("other"))
	if err := c.SetPhaseCheckpoint(other); err != nil {
		t.Fatal(err)
	}
	var output Head
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		output, err = c.PushRecoveryErrorRepeatOwned(owner, marker, children, func(prev NodeID, id SubtreeID) (uint32, error) {
			if prev != marker.Node {
				t.Fatal("wrong prefix")
			}
			v, err := c.MaterializationView(id)
			if err != nil {
				return 0, err
			}
			if v.Symbol != RecoveryErrorRepeatSymbol {
				t.Fatal("missing repeat")
			}
			return 702, nil
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := c.nodeScannerCheckpoint(output.Node); got != checkpoint {
		t.Fatalf("checkpoint=%d want=%d", got, checkpoint)
	}
	if got, err := c.RecoveryStoredErrorCost(output); err != nil || got != 702 {
		t.Fatalf("cost=%d err=%v", got, err)
	}
	paths, err := c.Derivations(output)
	if err != nil || len(paths) != 1 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	if got := flattenRecoveryRepeats(t, c, paths[0].Payloads); !reflect.DeepEqual(got, children) {
		t.Fatalf("payloads=%v", got)
	}
	before := len(c.subtrees)
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _ = c.PushRecoveryErrorRepeatOwned(owner, marker, children, func(NodeID, SubtreeID) (uint32, error) { return 0, errors.New("cost failed") })
		return nil
	})
	if err == nil || len(c.subtrees) != before {
		t.Fatal("failed repeat push committed")
	}
}

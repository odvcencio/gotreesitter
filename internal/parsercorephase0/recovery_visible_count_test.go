package parsercorephase0

import (
	"errors"
	"math"
	"testing"
)

type recoveryVisibleMaterializationSource struct{ compact *Core }

func (s recoveryVisibleMaterializationSource) RecoveryCostNode(id SubtreeID) (RecoveryCostNode, error) {
	view, err := s.compact.MaterializationView(id)
	if err != nil {
		return RecoveryCostNode{}, err
	}
	return RecoveryCostNode{
		Symbol: view.Symbol, Extra: view.Extra, Missing: view.Missing,
		Children: append([]SubtreeID(nil), view.Children...),
		Aliases:  append([]Symbol(nil), view.Aliases...),
	}, nil
}

func appendRecoveryVisibleFixture(t *testing.T, c *Core, symbol Symbol, extra, missing bool, children []SubtreeID, aliases []Symbol) SubtreeID {
	t.Helper()
	id, err := c.appendSubtree(subtreeRecord{symbol: symbol, extra: extra, missing: missing}, children, nil, aliases)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newRecoveryVisibleCore(t *testing.T) *Core {
	t.Helper()
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCoreRecoveryVisibleCountPreservesOccurrenceContexts(t *testing.T) {
	for _, parentFirst := range []bool{false, true} {
		c := newRecoveryVisibleCore(t)
		symbols := visibleSymbols()
		leaf := appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, true, nil, nil)
		hidden := appendRecoveryVisibleFixture(t, c, 6, false, false, []SubtreeID{leaf}, nil)
		extra := appendRecoveryVisibleFixture(t, c, 6, true, false, []SubtreeID{leaf}, nil)
		repeat := appendRecoveryVisibleFixture(t, c, RecoveryErrorRepeatSymbol, false, false, []SubtreeID{leaf}, nil)
		parent := appendRecoveryVisibleFixture(t, c, RecoveryErrorSymbol, false, false,
			[]SubtreeID{hidden, hidden, extra, repeat}, []Symbol{ordinarySymbol, 0, ordinarySymbol, 0})
		ids := []SubtreeID{leaf, hidden, extra, repeat, parent}
		if parentFirst {
			ids = []SubtreeID{parent, repeat, extra, hidden, leaf}
		}
		for pass := 0; pass < 2; pass++ {
			for _, id := range ids {
				want, referenceErr := RecoveryNodeVisibleSubtreeCount(symbols, recoveryVisibleMaterializationSource{c}, id)
				got, err := c.CachedVisibleSubtreeCount(symbols, id)
				if err != nil || referenceErr != nil || got != want {
					t.Fatalf("parent_first=%t pass=%d id=%d count=%d/%v, reference=%d/%v", parentFirst, pass, id, got, err, want, referenceErr)
				}
			}
		}
		if got, err := c.CachedVisibleSubtreeCount(symbols, parent); err != nil || got != 6 {
			t.Fatalf("parent count=%d/%v, want 6", got, err)
		}
		c.markSubtreeFragile(parent)
		if got, err := c.CachedVisibleSubtreeCount(symbols, parent); err != nil || got != 6 {
			t.Fatalf("fragile parent count=%d/%v, want 6", got, err)
		}
	}
}

func TestCoreRecoveryVisibleCountMemoSuppressesRepeatedDescendants(t *testing.T) {
	c := newRecoveryVisibleCore(t)
	symbols := visibleSymbols()
	root := appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, nil, nil)
	// Thirty shared parents represent over two billion visible occurrences.
	// Each compact record must be traversed once, regardless of repeated edges.
	for depth := 0; depth < 30; depth++ {
		root = appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, []SubtreeID{root, root}, nil)
	}
	const want = uint32(1<<31 - 1)
	if got, err := c.CachedVisibleSubtreeCount(symbols, root); err != nil || got != want {
		t.Fatalf("shared count=%d/%v, want %d", got, err, want)
	}
	allocations := testing.AllocsPerRun(50, func() {
		if got, err := c.CachedVisibleSubtreeCount(symbols, root); err != nil || got != want {
			t.Fatalf("repeated count=%d/%v, want %d", got, err, want)
		}
	})
	if allocations != 0 {
		t.Fatalf("warm count allocations=%g, want zero", allocations)
	}
	root = appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, []SubtreeID{root, root}, nil)
	if got, err := c.CachedVisibleSubtreeCount(symbols, root); err != nil || got != math.MaxUint32 {
		t.Fatalf("maximum count=%d/%v, want %d", got, err, uint32(math.MaxUint32))
	}
	hidden := appendRecoveryVisibleFixture(t, c, 6, false, false, []SubtreeID{root}, nil)
	if got, err := c.CachedVisibleSubtreeCount(symbols, hidden); err != nil || got != math.MaxUint32 {
		t.Fatalf("hidden maximum count=%d/%v, want %d", got, err, uint32(math.MaxUint32))
	}
	aliased := appendRecoveryVisibleFixture(t, c, 6, false, false, []SubtreeID{hidden}, []Symbol{ordinarySymbol})
	if _, err := c.CachedVisibleSubtreeCount(symbols, aliased); err == nil {
		t.Fatal("alias adjustment overflow succeeded")
	}
	repeat := appendRecoveryVisibleFixture(t, c, RecoveryErrorRepeatSymbol, false, false, []SubtreeID{root}, nil)
	if _, err := c.CachedVisibleSubtreeCount(symbols, repeat); err == nil {
		t.Fatal("ERROR_REPEAT root adjustment overflow succeeded")
	}
	nestedRepeat := appendRecoveryVisibleFixture(t, c, 6, false, false, []SubtreeID{repeat}, nil)
	if got, err := c.CachedVisibleSubtreeCount(symbols, nestedRepeat); err != nil || got != math.MaxUint32 {
		t.Fatalf("nested ERROR_REPEAT count=%d/%v, want %d", got, err, uint32(math.MaxUint32))
	}
	root = appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, []SubtreeID{root}, nil)
	if _, err := c.CachedVisibleSubtreeCount(symbols, root); err == nil {
		t.Fatal("overflowing count succeeded")
	}
}

func TestCoreRecoveryVisibleCountRebindsMutablePolicy(t *testing.T) {
	c := newRecoveryVisibleCore(t)
	leaf := appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, nil, nil)
	symbols := visibleSymbols()
	for _, visible := range []bool{true, false, true} {
		symbols[ordinarySymbol].Visible = visible
		want := uint32(0)
		if visible {
			want = 1
		}
		if got, err := c.CachedVisibleSubtreeCount(symbols, leaf); err != nil || got != want {
			t.Fatalf("visible=%t count=%d/%v, want %d", visible, got, err, want)
		}
	}
	if got, err := c.CachedVisibleSubtreeCount(nil, leaf); err != nil || got != 0 {
		t.Fatalf("empty policy count=%d/%v, want zero", got, err)
	}
}

func TestCoreRecoveryVisibleCountRollbackInvalidatesReusedID(t *testing.T) {
	c := newRecoveryVisibleCore(t)
	symbols := visibleSymbols()
	keep := appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, nil, nil)
	if _, err := c.CachedVisibleSubtreeCount(symbols, keep); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("visible count rollback")
	var discarded SubtreeID
	err := c.ApplySchedulerAtomic(func(SchedulerTransactionToken) error {
		discarded = appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, []SubtreeID{keep}, nil)
		if got, err := c.CachedVisibleSubtreeCount(symbols, discarded); err != nil || got != 2 {
			t.Fatalf("provisional count=%d/%v, want 2", got, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error=%v", err)
	}
	if _, err := c.CachedVisibleSubtreeCount(symbols, discarded); err == nil {
		t.Fatal("discarded ID remained readable")
	}
	replacement := appendRecoveryVisibleFixture(t, c, 6, false, false, nil, nil)
	if replacement != discarded {
		t.Fatalf("replacement ID=%d, want %d", replacement, discarded)
	}
	if got, err := c.CachedVisibleSubtreeCount(symbols, replacement); err != nil || got != 0 {
		t.Fatalf("replacement count=%d/%v, want zero", got, err)
	}
	if got, err := c.CachedVisibleSubtreeCount(symbols, keep); err != nil || got != 1 {
		t.Fatalf("retained count=%d/%v, want one", got, err)
	}
}

func TestCoreRecoveryVisibleCountResetAndAccounting(t *testing.T) {
	c := newRecoveryVisibleCore(t)
	symbols := visibleSymbols()
	leaf := appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, nil, nil)
	storageBefore, footprintBefore := c.StorageBytes(), c.FootprintBytes()
	if _, err := c.CachedVisibleSubtreeCount(symbols, leaf); err != nil {
		t.Fatal(err)
	}
	wantStorage := uint64(len(c.recoveryVisibleCounts))*coreRecoveryVisibleCountBytes + uint64(len(c.recoveryVisibleSymbols))*coreBoolBytes
	wantFootprint := uint64(cap(c.recoveryVisibleCounts))*coreRecoveryVisibleCountBytes + uint64(cap(c.recoveryVisibleSymbols))*coreBoolBytes
	if got := c.StorageBytes() - storageBefore; got != wantStorage {
		t.Fatalf("storage increase=%d, want %d", got, wantStorage)
	}
	if got := c.FootprintBytes() - footprintBefore; got != wantFootprint {
		t.Fatalf("footprint increase=%d, want %d", got, wantFootprint)
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if c.StorageBytes() != 0 || len(c.recoveryVisibleCounts) != 0 || len(c.recoveryVisibleSymbols) != 0 ||
		cap(c.recoveryVisibleCounts) == 0 || c.FootprintBytes() < wantFootprint {
		t.Fatalf("reset storage=%d footprint=%d counts=%d/%d policy=%d", c.StorageBytes(), c.FootprintBytes(), len(c.recoveryVisibleCounts), cap(c.recoveryVisibleCounts), len(c.recoveryVisibleSymbols))
	}
	leaf = appendRecoveryVisibleFixture(t, c, 6, false, false, nil, nil)
	if got, err := c.CachedVisibleSubtreeCount(symbols, leaf); err != nil || got != 0 {
		t.Fatalf("reset replacement count=%d/%v, want zero", got, err)
	}
	if err := c.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	if c.recoveryVisibleCounts != nil || c.recoveryVisibleSymbols != nil {
		t.Fatal("decline reset retained visible-count storage")
	}
}

func TestCoreRecoveryVisibleCountMatchesReusedMaterialization(t *testing.T) {
	c, head, reused := reusedFixture(t)
	var payload SubtreeID
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) (err error) {
		_, payload, err = c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]SelectedSymbolPolicy, int(reused.Symbol)+1)
	symbols[reused.Symbol].Visible = true
	for pass := 0; pass < 2; pass++ {
		want, referenceErr := RecoveryNodeVisibleSubtreeCount(symbols, recoveryVisibleMaterializationSource{c}, payload)
		got, err := c.CachedVisibleSubtreeCount(symbols, payload)
		if err != nil || referenceErr != nil || got != want || got != 1 {
			t.Fatalf("reused count=%d/%v, reference=%d/%v", got, err, want, referenceErr)
		}
	}
}

func TestCoreRecoveryVisibleCountRejectsMalformedMetadata(t *testing.T) {
	for _, malformed := range []string{"alias width", "child range", "alias range", "cycle"} {
		t.Run(malformed, func(t *testing.T) {
			c := newRecoveryVisibleCore(t)
			leaf := appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, nil, nil)
			root := appendRecoveryVisibleFixture(t, c, ordinarySymbol, false, false, []SubtreeID{leaf}, nil)
			switch malformed {
			case "alias width":
				c.aliases = append(c.aliases, 5, 5)
				c.subtrees[root-1].aliasCount = 2
			case "child range":
				c.subtrees[root-1].childCount++
			case "alias range":
				c.subtrees[root-1].firstAlias = 1
			case "cycle":
				c.children[0] = root
			}
			if _, err := c.CachedVisibleSubtreeCount(visibleSymbols(), root); err == nil {
				t.Fatal("malformed metadata was counted")
			}
		})
	}
}

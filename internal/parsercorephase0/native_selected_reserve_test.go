package parsercorephase0

import (
	"errors"
	"testing"
)

func TestNativeSelectedPublicReserveHonorsBudgetAndCancellation(t *testing.T) {
	for _, mode := range []string{"reserve", "tight", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c, _, _, scratch := nativeSelectedFixture(t)
			store := c.acquireSelectedStoreWithSources(0, 0, true)
			defer store.Release()
			scratch.store = store
			cancelled := errors.New("cancelled reserve")
			poll := func() error { return nil }
			if mode == "tight" {
				c.limits.MaxSelectedBytes = 1
			}
			if mode == "cancel" {
				poll = func() error { return cancelled }
			}
			err := scratch.reservePublicCapacity(c, poll)
			if mode == "cancel" {
				if !errors.Is(err, cancelled) {
					t.Fatal("reserve ignored cancellation")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if mode == "reserve" {
				if cap(store.records) < 2 || cap(store.children) < 1 || cap(scratch.replay) < 2 {
					t.Fatal("reserve omitted a result plane")
				}
			} else if cap(store.records) != 0 || cap(store.children) != 0 || cap(scratch.replay) != 0 {
				t.Fatal("tight or cancelled hint allocated result storage")
			}
		})
	}
}

func TestNativeSelectedPublicReserveDoesNotCountEmptyAliasSlots(t *testing.T) {
	table := &nativeSelectedIdentityTable{selectedStorePolicyTable: selectedStorePolicyTable{policy: selectedStoreTestPolicy(t, 2, 1)}, identity: [32]byte{1}}
	c, err := New(table, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var children []SubtreeID
	for i := uint32(0); i < 3; i++ {
		child, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, startByte: i, endByte: i + 1}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
	}
	if _, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 3}, children, nil, []Symbol{0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	store := c.acquireSelectedStoreWithSources(0, 0, true)
	defer store.Release()
	scratch := NativeSelectedScratch{store: store}
	if err := scratch.reservePublicCapacity(c, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Four real records plus one optional sibling root; empty alias columns
	// cannot cause the three extra reservations that inflated large-file RSS.
	if cap(store.records) != 5 || cap(scratch.replay) != 5 {
		t.Fatalf("empty aliases inflated reserve: records=%d replay=%d", cap(store.records), cap(scratch.replay))
	}
}

func TestNativeSelectedPublicReserveCountsAliasOccurrences(t *testing.T) {
	for _, tc := range []struct {
		name              string
		visible, retained bool
		alias             Symbol
		want              int
	}{
		{"hidden-terminal", false, false, 0, 2},
		{"visible-rename", true, false, 3, 3},
		{"hidden-rename", false, false, 3, 3},
		{"retained-wrapper", true, true, 3, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := selectedStoreTestPolicy(t, 2, 3)
			policy.Symbols[1].Visible = tc.visible
			if tc.retained {
				policy.SetRetainedAliasChildren([]SelectedAliasChildPair{{Alias: 3, Child: 1}})
			}
			table := &nativeSelectedIdentityTable{selectedStorePolicyTable: selectedStorePolicyTable{policy: policy}, identity: [32]byte{1}}
			c, err := New(table, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			child, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{child}, nil, []Symbol{tc.alias}); err != nil {
				t.Fatal(err)
			}
			store := c.acquireSelectedStoreWithSources(0, 0, true)
			defer store.Release()
			scratch := NativeSelectedScratch{store: store}
			if err = scratch.reservePublicCapacity(c, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if cap(store.records) != tc.want || cap(scratch.replay) != tc.want {
				t.Fatalf("reserve = %d records / %d replay; want %d", cap(store.records), cap(scratch.replay), tc.want)
			}
		})
	}
}

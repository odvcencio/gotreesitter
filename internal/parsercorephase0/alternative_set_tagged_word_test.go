package parsercorephase0

import (
	"errors"
	"slices"
	"testing"
	"unsafe"
)

func TestAlternativeSetTaggedWordExactWidthAndPayload(t *testing.T) {
	if unsafe.Sizeof(AlternativeSet{}) != 8 || unsafe.Sizeof(nodeLineageRecord{}) != 28 {
		t.Fatal("unexpected widths")
	}
	c := newTinyCore(t, 64)
	s := NewAlternativeSetMember(65535, 65534)
	want := packAlternativeSetMember(65535, 65534)
	if got, ok := c.AlternativeSetMembers(s); !ok || !slices.Equal(got, []uint32{want}) {
		t.Fatal("full-width inline identity changed")
	}
	if !c.alternativeSetInsert(&s, packAlternativeSetMember(1, 0)) || !s.spilled() {
		t.Fatal("spill transition absent")
	}
	if got, ok := c.AlternativeSetMembers(s); !ok || !slices.Equal(got, []uint32{packAlternativeSetMember(1, 0), want}) {
		t.Fatal("spill lost full-width identity")
	}
}
func TestAlternativeSetTaggedWordNestedRollback(t *testing.T) {
	c := newTinyCore(t, 64)
	h, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplySchedulerAtomic(func(o SchedulerTransactionToken) error {
		return c.RecordHeadLineageSetOwned(o, h, NewAlternativeSetMember(9, 0), false)
	}); err != nil {
		t.Fatal(err)
	}
	old := c.nodeLineages[h.Node-1]
	stop := errors.New("rollback tagged set")
	err = c.ApplyAtomic(func() error {
		if err := c.recordNodeLineageSet(h, NewAlternativeSetMember(2, 0), false); err != nil {
			return err
		}
		if err := c.ApplyAtomic(func() error { return c.recordNodeLineageSet(h, NewAlternativeSetMember(1, 0), false) }); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) || c.nodeLineages[h.Node-1] != old {
		t.Fatalf("nested spill commit/outer rollback changed exact lineage: err=%v got=%+v want=%+v", err, c.nodeLineages[h.Node-1], old)
	}
	if got, ok := c.AlternativeSetMembers(c.nodeLineages[h.Node-1].set); !ok || !slices.Equal(got, []uint32{packAlternativeSetMember(9, 0)}) {
		t.Fatal("rollback restored spill offset as member")
	}
}

// The public accessor must inline through consumers; a forwarding wrapper can
// cross the compiler budget and allocate a copied inline set even when read only.
func TestAlternativeSetMembersConsumedWithoutAllocations(t *testing.T) {
	c := newTinyCore(t, 64)
	spilled := NewAlternativeSetMember(1, 0)
	c.alternativeSetInsert(&spilled, packAlternativeSetMember(2, 0))
	var consumed uint32
	for _, tc := range []struct {
		name string
		set  AlternativeSet
	}{{"empty", AlternativeSet{}}, {"inline", NewAlternativeSetMember(65535, 65535)}, {"spilled", spilled}} {
		t.Run(tc.name, func(t *testing.T) {
			allocations := testing.AllocsPerRun(100, func() {
				members, ok := c.AlternativeSetMembers(tc.set)
				if !ok {
					panic("invalid member view")
				}
				for _, member := range members {
					consumed ^= member
				}
			})
			if allocations != 0 {
				t.Fatalf("member consumption allocations=%v, want zero", allocations)
			}
		})
	}
	_ = consumed
}

package parsercorephase0

import (
	"errors"
	"math"
	"slices"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestDropCohortRefArenaCopiesRetainExactMembership(t *testing.T) {
	c := newTinyCore(t, 8)
	first, second, earlier := g18Ref(1, 1, 2, 0), g18Ref(1, 1, 3, 0), g18Ref(1, 1, 1, 0)
	prefix := g18RefSetFrom(t, c, first)
	extended := prefix
	if !c.AddDropCohortRef(&extended, second) {
		t.Fatal("tail extension failed")
	}
	if extended.Spill != prefix.Spill || extended.endIdentity == prefix.endIdentity {
		t.Fatal("tail extension did not share its prefix with a new end identity")
	}
	inserted := extended
	if !c.AddDropCohortRef(&inserted, earlier) {
		t.Fatal("sorted insertion failed")
	}
	for _, test := range []struct {
		set  DropCohortRefSet
		want []DropCohortRef
	}{
		{prefix, []DropCohortRef{first}},
		{extended, []DropCohortRef{first, second}},
		{inserted, []DropCohortRef{earlier, first, second}},
	} {
		if got := g18Members(t, c, test.set); !slices.Equal(got, test.want) {
			t.Fatalf("copied view changed: %v, want %v", got, test.want)
		}
	}
	before := c.FootprintBytes()
	var shared DropCohortRefSet
	if !c.UnionDropCohortRefs(&shared, prefix) || shared != prefix || c.FootprintBytes() != before {
		t.Fatal("publishing an existing immutable set duplicated storage")
	}
}

func TestDropCohortRefArenaRejectsForeignAndRecycledViews(t *testing.T) {
	requireRejected := func(t *testing.T, c *Core, stale DropCohortRefSet) {
		t.Helper()
		if _, ok := c.DropCohortRefAt(stale, 0); ok {
			t.Fatal("stale or foreign view resolved to a current reference")
		}
		before := slices.Clone(c.dropCohortRefSpill)
		var destination DropCohortRefSet
		if _, err := c.UnionDropCohortRefsChecked(&destination, stale); err == nil || !destination.Empty() || !slices.Equal(before, c.dropCohortRefSpill) {
			t.Fatalf("invalid union changed its destination or arena: %v", err)
		}
	}
	t.Run("foreign-core-same-index", func(t *testing.T) {
		left, right := newTinyCore(t, 8), newTinyCore(t, 8)
		ref := g18Ref(1, 1, 1, 0)
		foreign := g18RefSetFrom(t, left, ref)
		local := g18RefSetFrom(t, right, ref)
		if foreign.Spill != local.Spill || foreign.Count != local.Count {
			t.Fatal("fixture did not reuse the same arena range")
		}
		requireRejected(t, right, foreign)
	})
	t.Run("reset-same-index", func(t *testing.T) {
		c := newTinyCore(t, 8)
		stale := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0))
		if err := c.Reset(); err != nil {
			t.Fatal(err)
		}
		local := g18RefSetFrom(t, c, g18Ref(2, 2, 2, 0))
		if stale.Spill != local.Spill {
			t.Fatal("fixture did not reuse the same arena range")
		}
		requireRejected(t, c, stale)
	})
	t.Run("rolled-back-tail", func(t *testing.T) {
		c := newTinyCore(t, 8)
		first := g18Ref(1, 1, 1, 0)
		prefix := g18RefSetFrom(t, c, first)
		stale := prefix
		sentinel := errors.New("rollback reference extension")
		if err := c.ApplyAtomic(func() error {
			if !c.AddDropCohortRef(&stale, g18Ref(1, 1, 2, 0)) {
				t.Fatal("speculative extension failed")
			}
			return sentinel
		}); !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
		local := prefix
		if !c.AddDropCohortRef(&local, g18Ref(1, 1, 3, 0)) || local.Spill != stale.Spill || local.Count != stale.Count {
			t.Fatal("fixture did not replace the rolled-back suffix")
		}
		requireRejected(t, c, stale)
		if got := g18Members(t, c, prefix); !slices.Equal(got, []DropCohortRef{first}) {
			t.Fatal("rollback invalidated the surviving prefix")
		}
	})
}

func TestDropCohortRefArenaIdentityOverflowIsAtomic(t *testing.T) {
	var counter atomic.Uint64
	counter.Store(math.MaxUint64 - 1)
	if _, err := reserveDropCohortRefIdentities(&counter, 2); err == nil || counter.Load() != math.MaxUint64-1 {
		t.Fatal("over-limit reservation changed the counter")
	}
	if first, err := reserveDropCohortRefIdentities(&counter, 1); err != nil || first != math.MaxUint64 {
		t.Fatalf("last identity=%d err=%v", first, err)
	}
	if _, err := reserveDropCohortRefIdentities(&counter, 1); err == nil || counter.Load() != math.MaxUint64 {
		t.Fatal("exhausted identity space wrapped")
	}
}

func TestDropCohortRefArenaChargesIdentityStorage(t *testing.T) {
	if got := unsafe.Sizeof(dropCohortRefRecord{}); got != 40 {
		t.Fatalf("stored reference size=%d, want 40", got)
	}
	c := newTinyCoreWithLimits(t, Limits{MaxDropCohortRefs: 2, MaxDropCohortRefBytes: 39})
	var set DropCohortRefSet
	before := c.FootprintBytes()
	if c.AddDropCohortRef(&set, g18Ref(1, 1, 1, 0)) || set != (DropCohortRefSet{}) || c.FootprintBytes() != before {
		t.Fatal("publication exceeded the byte budget or partially changed storage")
	}
}

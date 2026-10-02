package parsercorephase0

import (
	"errors"
	"testing"
)

func TestNodeDropCohortReferencesAreImmutableAndRolledBack(t *testing.T) {
	c := newTinyCore(t, 8)
	first := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0))
	id, changed, err := c.unionNodeDropCohortRefs(0, first)
	if err != nil || !changed || id == 0 {
		t.Fatalf("publish first set: %d %t %v", id, changed, err)
	}
	before := c.nodeDropCohortRefSet(id)
	count, footprint := len(c.nodeDropCohortRefs), c.FootprintBytes()
	decline := errors.New("test rollback")
	if err := c.ApplySchedulerAtomic(func(SchedulerTransactionToken) error {
		next, changed, err := c.unionNodeDropCohortRefs(id, g18RefSetFrom(t, c, g18Ref(1, 1, 2, 0)))
		if err != nil || !changed || next == id || c.nodeDropCohortRefSet(next).Len() != 2 {
			t.Fatalf("union publication: %d %t %v", next, changed, err)
		}
		if c.nodeDropCohortRefSet(id) != before {
			t.Fatal("union changed the shared old value")
		}
		return decline
	}); !errors.Is(err, decline) {
		t.Fatalf("rollback error: %v", err)
	}
	if len(c.nodeDropCohortRefs) != count || c.nodeDropCohortRefSet(id) != before {
		t.Fatal("rollback retained a new set or changed the old set")
	}
	if c.FootprintBytes() < footprint {
		t.Fatal("rollback hid retained capacity from the memory budget")
	}
	if err := c.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	if c.nodeDropCohortRefs != nil {
		t.Fatal("reset retained the reference arena")
	}
}

func TestNodeDropCohortReferencesEnforceMetadataCapacity(t *testing.T) {
	c := newTinyCore(t, 8)
	c.limits.MaxMetadata = 1
	set := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0))
	id, _, err := c.unionNodeDropCohortRefs(0, set)
	if err != nil {
		t.Fatal(err)
	}
	if next, changed, err := c.unionNodeDropCohortRefs(id, set); err != nil || changed || next != id {
		t.Fatalf("duplicate allocated metadata: %d %t %v", next, changed, err)
	}
	if _, _, err := c.unionNodeDropCohortRefs(id, g18RefSetFrom(t, c, g18Ref(1, 1, 2, 0))); err == nil {
		t.Fatal("metadata cap was bypassed")
	}
	if len(c.nodeDropCohortRefs) != 1 || c.nodeDropCohortRefSet(id) != set {
		t.Fatal("capacity decline mutated published references")
	}
}

func TestNodeDropCohortCapacityDeclineDoesNotPublishSpill(t *testing.T) {
	c := newTinyCore(t, 8)
	c.limits.MaxMetadata = 64
	set := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0), g18Ref(1, 1, 2, 0), g18Ref(1, 1, 3, 0), g18Ref(1, 1, 4, 0))
	id, _, err := c.unionNodeDropCohortRefs(0, set)
	if err != nil {
		t.Fatal(err)
	}
	before, spill := c.nodeDropCohortRefSet(id), len(c.dropCohortRefSpill)
	c.limits.MaxMetadata = 1
	if _, _, err := c.unionNodeDropCohortRefs(id, g18RefSetFrom(t, c, g18Ref(1, 1, 5, 0))); err == nil {
		t.Fatal("metadata cap admitted a new spilled value")
	}
	if c.nodeDropCohortRefSet(id) != before || len(c.dropCohortRefSpill) != spill {
		t.Fatal("capacity decline published a spill or changed a shared value")
	}
}

package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"
)

func mustLineageRefs(t *testing.T, c *Core, index uint32) DropCohortRefSet {
	t.Helper()
	refs, err := c.nodeLineageRefs(index)
	if err != nil {
		t.Fatal(err)
	}
	return refs
}

func TestNodeLineageRefsLazyStorageAndIdempotentUpdate(t *testing.T) {
	c := newTinyCore(t, 8)
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.recordNodeLineageRefs(head, DropCohortRefSet{}); err != nil {
		t.Fatal(err)
	}
	if c.nodeLineageRefSets != nil {
		t.Fatal("empty references allocated storage")
	}
	refs := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0))
	if err := c.recordNodeLineageRefs(head, refs); err != nil {
		t.Fatal(err)
	}
	index := c.nodeLineages[head.Node-1].dropCohortRefIndex
	before := c.FootprintBytes()
	if err := c.recordNodeLineageRefs(head, refs); err != nil {
		t.Fatal(err)
	}
	if c.nodeLineages[head.Node-1].dropCohortRefIndex != index || len(c.nodeLineageRefSets) != 1 || c.FootprintBytes() != before {
		t.Fatal("duplicate references allocated or changed their index")
	}
	more := g18RefSetFrom(t, c, g18Ref(1, 1, 2, 0))
	if err := c.recordNodeLineageRefs(head, more); err != nil {
		t.Fatal(err)
	}
	if got := mustLineageRefs(t, c, index); got != refs {
		t.Fatalf("published snapshot changed: %+v, want %+v", got, refs)
	}
	if got, err := c.NodeLineageDropCohortRefs(head.Node); err != nil || got.Len() != 2 {
		t.Fatalf("updated refs=%+v err=%v", got, err)
	}
}

func TestNodeLineageRefsNestedRollbackRestoresStorage(t *testing.T) {
	c := newTinyCore(t, 8)
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0))
	if err := c.recordNodeLineageRefs(head, first); err != nil {
		t.Fatal(err)
	}
	before := c.nodeLineages[head.Node-1]
	beforeArena := c.nodeLineageRefSets
	sentinel := errors.New("rollback references")
	err = c.ApplyAtomic(func() error {
		second := g18RefSetFrom(t, c, g18Ref(1, 1, 2, 0))
		if err := c.recordNodeLineageRefs(head, second); err != nil {
			return err
		}
		outer := c.nodeLineages[head.Node-1]
		outerArena := c.nodeLineageRefSets
		if err := c.ApplyAtomic(func() error {
			third := g18RefSetFrom(t, c, g18Ref(1, 1, 3, 0))
			if err := c.recordNodeLineageRefs(head, third); err != nil {
				return err
			}
			return sentinel
		}); !errors.Is(err, sentinel) {
			t.Fatalf("inner rollback=%v", err)
		}
		if c.nodeLineages[head.Node-1] != outer || !reflect.DeepEqual(c.nodeLineageRefSets, outerArena) || cap(c.nodeLineageRefSets) != cap(outerArena) {
			t.Fatal("inner rollback did not restore the outer reference snapshot")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("outer rollback=%v", err)
	}
	if c.nodeLineages[head.Node-1] != before || !reflect.DeepEqual(c.nodeLineageRefSets, beforeArena) || cap(c.nodeLineageRefSets) != cap(beforeArena) {
		t.Fatal("outer rollback did not restore references and their storage")
	}

}

func TestNodeLineageRefsMergeSharesEqualSets(t *testing.T) {
	c := newTinyCore(t, 8)
	heads := make([]Head, 3)
	for i := range heads {
		var err error
		heads[i], err = c.Seed(StateID(i+1), 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	refs := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0))
	for _, head := range heads[:2] {
		if err := c.recordNodeLineageRefs(head, refs); err != nil {
			t.Fatal(err)
		}
	}
	before := c.nodeLineages[heads[0].Node-1]
	bytes := c.FootprintBytes()
	if err := c.mergeNodeLineageMetadata(heads[0].Node, heads[1].Node, heads[0].Node); err != nil {
		t.Fatal(err)
	}
	if c.nodeLineages[heads[0].Node-1] != before || c.FootprintBytes() != bytes {
		t.Fatal("equal sets at different indices changed the incumbent")
	}
	if err := c.mergeNodeLineageMetadata(heads[0].Node, heads[1].Node, heads[2].Node); err != nil {
		t.Fatal(err)
	}
	if c.nodeLineages[heads[2].Node-1].dropCohortRefIndex != before.dropCohortRefIndex || c.FootprintBytes() != bytes {
		t.Fatal("new lineage did not share the immutable reference set")
	}
}

func TestNodeLineageRefsFlagsAccountingResetAndRelease(t *testing.T) {
	c := newTinyCore(t, 8)
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := c.FootprintBytes()
	flags := DropCohortRefSet{Flags: dropCohortRefFlagOverflowed | dropCohortRefFlagBlended}
	if err := c.recordNodeLineageRefs(head, flags); err != nil {
		t.Fatal(err)
	}
	if got, err := c.NodeLineageDropCohortRefs(head.Node); err != nil || got != flags {
		t.Fatalf("flags=%+v err=%v", got, err)
	}
	want := uint64(cap(c.nodeLineageRefSets)) * uint64(unsafe.Sizeof(DropCohortRefSet{}))
	if c.FootprintBytes()-before != want {
		t.Fatal("reference storage is missing from footprint")
	}
	if _, err := c.nodeLineageRefs(2); err == nil {
		t.Fatal("invalid reference index accepted")
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if len(c.nodeLineageRefSets) != 0 {
		t.Fatal("reset retained live references")
	}
	head, err = c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if refs, err := c.NodeLineageDropCohortRefs(head.Node); err != nil || refs != (DropCohortRefSet{}) {
		t.Fatalf("reused node retained refs=%+v err=%v", refs, err)
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	c.releaseRecordArenaReserve()
	if c.nodeLineageRefSets != nil {
		t.Fatal("declined core retained reference storage")
	}
}

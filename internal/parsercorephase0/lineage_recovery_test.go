package parsercorephase0

import (
	"errors"
	"testing"
	"unsafe"
)

func TestLineageRecoverySidecarImplicitZeroAndReset(t *testing.T) {
	c := newTinyCore(t, 8)
	heads := make([]Head, 3)
	for i := range heads {
		var err error
		heads[i], err = c.Seed(StateID(i+1), 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	before := c.FootprintBytes()
	for _, h := range heads {
		if got, err := c.RecoveryStoredErrorCost(h); err != nil || got != 0 {
			t.Fatal(got, err)
		}
		if err := c.recordNodeStoredErrorCost(h, 0); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.nodeLineageRecovery) != 0 || cap(c.nodeLineageRecovery) != 0 || c.FootprintBytes() != before {
		t.Fatal("zero recovery state allocated")
	}
	c.writeNodeLineageRecovery(heads[0].Node, nodeLineageRecovery{storedErrorCost: 17, dropCohortRefIndex: 23})
	c.writeNodeLineageRecovery(heads[2].Node, nodeLineageRecovery{storedErrorCost: 29})
	if c.nodeLineageRecoveryAt(heads[1].Node) != (nodeLineageRecovery{}) {
		t.Fatal("extension did not initialize implicit zeros")
	}
	retained := cap(c.nodeLineageRecovery)
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	for i := range heads {
		var err error
		heads[i], err = c.Seed(StateID(i+1), 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	if cap(c.nodeLineageRecovery) != retained || len(c.nodeLineageRecovery) != 0 {
		t.Fatal("reset changed retention contract")
	}
	for _, h := range heads {
		if got, err := c.RecoveryStoredErrorCost(h); err != nil || got != 0 {
			t.Fatal("stale cost after reset", got, err)
		}
	}
	c.writeNodeLineageRecovery(heads[2].Node, nodeLineageRecovery{storedErrorCost: 31})
	if c.nodeLineageRecoveryAt(heads[0].Node) != (nodeLineageRecovery{}) || c.nodeLineageRecoveryAt(heads[1].Node) != (nodeLineageRecovery{}) {
		t.Fatal("extension exposed stale retained values")
	}
	for _, id := range []NodeID{0, 4} {
		if _, err := c.RecoveryStoredErrorCost(Head{Node: id}); err == nil {
			t.Fatal("invalid node accepted", id)
		}
	}
	if unsafe.Sizeof(nodeLineageHot{}) != 16 || unsafe.Sizeof(nodeLineageRecovery{}) != 8 {
		t.Fatal("physical widths changed")
	}
	c.releaseRecordArenaReserve()
	if cap(c.nodeLineageRecovery) != 0 {
		t.Fatal("release retained recovery sidecar")
	}
}

func TestLineageRecoverySidecarNestedRollback(t *testing.T) {
	c := newTinyCore(t, 8)
	var head Head
	if err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		head, err = c.Seed(1, 0)
		if err != nil {
			return err
		}
		return c.RecordHeadStoredErrorCostOwned(owner, head, 17)
	}); err != nil {
		t.Fatal(err)
	}
	before := c.testLineageAt(int(head.Node - 1))
	originalLen, originalCap := len(c.nodeLineageRecovery), cap(c.nodeLineageRecovery)
	refs := g18RefSetFrom(t, c, g18Ref(1, 1, 1, 0))
	innerRefs := g18RefSetFrom(t, c, g18Ref(2, 1, 1, 0))
	abort := errors.New("abort recovery sidecar")
	err := c.ApplyAtomic(func() error {
		if err := c.recordNodeLineageRefs(head, refs); err != nil {
			return err
		}
		outer := c.testLineageAt(int(head.Node - 1))
		if err := c.ApplyAtomic(func() error {
			if err := c.recordNodeLineageRefs(head, innerRefs); err != nil {
				return err
			}
			return abort
		}); !errors.Is(err, abort) {
			t.Fatal(err)
		}
		if c.testLineageAt(int(head.Node-1)) != outer {
			t.Fatal("inner rollback lost recovery record")
		}
		if err := c.ApplyAtomic(func() error { return c.recordNodeLineageRefs(head, innerRefs) }); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) || c.testLineageAt(int(head.Node-1)) != before || len(c.nodeLineageRecovery) != originalLen || cap(c.nodeLineageRecovery) < originalCap {
		t.Fatal("outer rollback did not restore complete record and length", err)
	}
	// Rolling back the first nonzero write must restore an absent sidecar too.
	d := newTinyCore(t, 8)
	h, err := d.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := g18RefSetFrom(t, d, g18Ref(3, 1, 1, 0))
	if err := d.ApplyAtomic(func() error {
		if err := d.recordNodeLineageRefs(h, r); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if len(d.nodeLineageRecovery) != 0 || cap(d.nodeLineageRecovery) == 0 || d.testLineageAt(int(h.Node-1)).dropCohortRefIndex != 0 {
		t.Fatal("rollback lost its length mark or retained-capacity accounting")
	}
}

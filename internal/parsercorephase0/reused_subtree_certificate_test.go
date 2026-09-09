package parsercorephase0

import (
	"errors"
	"testing"
)

func TestReusedSubtreeCertificateIgnoresInactiveAmbiguity(t *testing.T) {
	c, head, reused := reusedFixture(t)
	other, err := c.Seed(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if other.Node == head.Node {
		t.Fatal("fixture reused the active seed")
	}
	c.nodes[other.Node-1].pathCount = 2
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatalf("inactive ambiguity blocked clean head: %v", err)
	}
	reused.Key++
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwned(owner, other, reused)
		return err
	})
	if err == nil {
		t.Fatal("inactive-node certificate admitted the ambiguous head")
	}
}

func TestReusedSubtreeCertificateAcceptsCleanConvergence(t *testing.T) {
	c, head, reused := reusedFixture(t)
	c.nodeLineages[head.Node-1].converged = true
	c.nodeLineages[head.Node-1].blended = true
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatalf("clean convergence blocked reuse: %v", err)
	}
}

func TestReusedSubtreeCertificateRollbackAndStorage(t *testing.T) {
	c, head, reused := reusedFixture(t)
	stop := errors.New("rollback")
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if _, _, err := c.PushReusedSubtreeOwned(owner, head, reused); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) || len(c.reuseCertifiedNodes) != 0 || c.reuseProof.nodes != 0 {
		t.Fatalf("rollback retained certificate: err=%v length=%d cursor=%d", err, len(c.reuseCertifiedNodes), c.reuseProof.nodes)
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	certificate := c.reuseCertifiedNodes
	if len(certificate) == 0 || len(certificate) != int(c.reuseProof.nodes) {
		t.Fatal("certificate and proof cursor differ")
	}
	used, retained := c.StorageBytes(), c.FootprintBytes()
	c.reuseCertifiedNodes = nil
	usedWithout, retainedWithout := c.StorageBytes(), c.FootprintBytes()
	c.reuseCertifiedNodes = certificate
	if used-usedWithout != uint64(len(certificate))*coreBoolBytes || retained-retainedWithout != uint64(cap(certificate))*coreBoolBytes {
		t.Fatalf("certificate accounting used=%d retained=%d", used-usedWithout, retained-retainedWithout)
	}
	if err := c.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	if len(c.reuseCertifiedNodes) != 0 || cap(c.reuseCertifiedNodes) != 0 {
		t.Fatal("release retained certificate storage")
	}
}

func TestReusedSubtreeCertificateVisitsNewRecordsWithInactiveBranches(t *testing.T) {
	for _, count := range []uint32{512, 1024} {
		c, head, reused := reusedFixture(t)
		c.tables.(*fakeTable).gotos[tableCell{state: 1, symbol: 100}] = 1
		polls := 0
		err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
			for i := uint32(0); i < count; i++ {
				inactive, err := c.Seed(3, i)
				if err != nil {
					return err
				}
				c.nodes[inactive.Node-1].pathCount = 2
				reused.Key, reused.State = i+1, 1
				reused.StartByte, reused.EndByte = i, i+1
				head, _, err = c.PushReusedSubtreeOwnedWithPoll(owner, head, reused, func() error { polls++; return nil })
				if err != nil {
					return err
				}
				if c.reuseCertifiedNodes[inactive.Node-1] {
					t.Fatal("inactive ambiguity received a valid certificate")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// Each borrow sees only a few new records. Rescanning old records would
		// trigger additional polls at the validation loop's 128-work-unit interval.
		if polls != 2*int(count) {
			t.Fatalf("count=%d polls=%d want=%d", count, polls, 2*count)
		}
	}
}

package parsercorephase0

import "testing"

func TestSoleHeadPayloadAuthenticatesBoundary(t *testing.T) {
	c, seed, reused := reusedFixture(t)
	if _, err := c.SoleHeadPayload(seed); err == nil {
		t.Fatal("seed has no payload")
	}
	var head Head
	var payload SubtreeID
	if err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) (err error) {
		head, payload, err = c.PushReusedSubtreeOwned(owner, seed, reused)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.SoleHeadPayload(head); err != nil || got != payload {
		t.Fatalf("payload=%d error=%v, want %d", got, err, payload)
	}
	n := c.nodes.At(int(head.Node - 1))
	setNodePathCountForTest(c, head.Node, 2)
	if _, err := c.SoleHeadPayload(head); err == nil {
		t.Fatal("ambiguous frontier was authenticated")
	}
	setNodePathCountForTest(c, head.Node, 1)
	{
		linkStorageMutation := c.links.At(int(n.firstLink - 1))
		linkStorageMutation.payload = 0
		c.links.Set(int(n.firstLink-1), linkStorageMutation)
	}
	if _, err := c.SoleHeadPayload(head); err == nil {
		t.Fatal("absent payload was authenticated")
	}
}

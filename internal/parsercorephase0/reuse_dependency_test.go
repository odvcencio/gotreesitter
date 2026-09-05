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
	n := &c.nodes[head.Node-1]
	n.pathCount = 2
	if _, err := c.SoleHeadPayload(head); err == nil {
		t.Fatal("ambiguous frontier was authenticated")
	}
	n.pathCount = 1
	c.links[n.firstLink-1].payload = 0
	if _, err := c.SoleHeadPayload(head); err == nil {
		t.Fatal("absent payload was authenticated")
	}
}

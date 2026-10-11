package parsercorephase0

import "testing"

func TestSubtreeGeometryRejectsInvalidAndStalePayloads(t *testing.T) {
	c, head, reused := reusedFixture(t)
	var payload SubtreeID
	if err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) (err error) {
		_, payload, err = c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := c.SubtreeGeometry(payload)
	if err != nil || got.StartByte != 2 || got.EndByte != 10 || got.Symbol != 100 || got.Terminal || got.Missing {
		t.Fatalf("borrowed geometry=%+v, err=%v", got, err)
	}
	for _, id := range []SubtreeID{0, payload + 1} {
		if _, err := c.SubtreeGeometry(id); err == nil {
			t.Fatalf("invalid payload %d accepted", id)
		}
	}
	c.reusedSubtrees[0].descriptor.EndByte++
	if _, err := c.SubtreeGeometry(payload); err == nil {
		t.Fatal("stale borrowed geometry authenticated a projection")
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SubtreeGeometry(payload); err == nil {
		t.Fatal("reset payload remained readable")
	}
}

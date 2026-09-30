package queryexec

import "testing"

func TestOutputStorageBound(t *testing.T) {
	b := NewBudget(100)
	if !b.Retain(MaxRetainedCaptures) || b.Remaining() != 100 {
		t.Fatal("output changed work accounting")
	}
	if b.Retain(1) || !b.Exceeded() {
		t.Fatal("capture storage was not bounded")
	}
	b = NewBudget(100)
	for i := 0; i < MaxActiveStates; i++ {
		if !b.Retain(0) {
			t.Fatal("early output exhaustion")
		}
	}
	if b.Retain(0) || !b.Exceeded() {
		t.Fatal("pending output states were not bounded")
	}
}

func TestBudgetWorkAndActiveStates(t *testing.T) {
	b := NewBudget(2)
	if !b.Charge() || !b.Charge() || b.Remaining() != 0 || b.Exceeded() {
		t.Fatal("exactly consuming the work allowance must not exceed it")
	}
	if b.Charge() || !b.Exceeded() || b.Enter() {
		t.Fatal("work exhaustion must stay latched across nested states")
	}
	b = NewBudget(1_000_000)
	for i := 0; i < MaxActiveStates; i++ {
		if !b.Enter() {
			t.Fatalf("state %d rejected before the active-state bound", i)
		}
	}
	if b.Enter() || !b.Exceeded() || b.Remaining() != 1_000_000 {
		t.Fatal("active-state exhaustion must be independent of remaining work")
	}
	for i := 0; i < MaxActiveStates; i++ {
		b.Leave()
	}
	if b.active != 0 || b.Enter() {
		t.Fatal("unwinding must release states without clearing exhaustion")
	}
	var unlimited *Budget
	if !unlimited.Enter() || !unlimited.Charge() || unlimited.Exceeded() || unlimited.Remaining() != -1 {
		t.Fatal("nil budget must remain an explicit unlimited opt-out")
	}
	unlimited.Leave()
}

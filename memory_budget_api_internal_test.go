package gotreesitter

import "testing"

func TestSetMemoryBudgetBytesZeroDoesNotAllocateColdState(t *testing.T) {
	p := &Parser{}
	p.SetMemoryBudgetBytes(0)
	if p.forestDeclineMemo != nil {
		t.Fatal("SetMemoryBudgetBytes(0) allocated the cold state")
	}
	if got := p.MemoryBudgetBytes(); got != 0 {
		t.Fatalf("MemoryBudgetBytes() = %d, want 0", got)
	}
}

func TestParseMemoryHardCeilingScalesWithConfiguredBudget(t *testing.T) {
	defaultCeiling := parseMemoryHardCeilingBytes()
	if defaultCeiling <= 0 {
		t.Skip("hard ceiling is off in this environment")
	}
	p := &Parser{}
	if got := parseMemoryHardCeilingBytesForParser(p); got != defaultCeiling {
		t.Fatalf("default ceiling = %d, want %d", got, defaultCeiling)
	}
	p.SetMemoryBudgetBytes(defaultCeiling)
	if got, want := parseMemoryHardCeilingBytesForParser(p), 4*defaultCeiling; got != want {
		t.Fatalf("ceiling for a large budget = %d, want %d", got, want)
	}
	p.SetMemoryBudgetBytes(1 << 20)
	if got := parseMemoryHardCeilingBytesForParser(p); got != defaultCeiling {
		t.Fatalf("ceiling for a small budget = %d, want the default %d", got, defaultCeiling)
	}
	p.SetMemoryBudgetBytes(-1)
	if got := parseMemoryHardCeilingBytesForParser(p); got != defaultCeiling {
		t.Fatalf("ceiling with the budget off = %d, want the default %d", got, defaultCeiling)
	}
	if got := parseMemoryBudgetForParser(p, 10); got != 0 {
		t.Fatalf("budget off: parseMemoryBudgetForParser = %d, want 0", got)
	}
}

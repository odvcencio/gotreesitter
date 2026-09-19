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

func TestParseMemoryBudgetScalesWithSourceSize(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "")
	t.Setenv("GOT_PARSE_MEMORY_HARD_CEILING_MB", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	const mib = int64(1) << 20
	const large = 16 << 20 // 16 MiB of source
	scaled := int64(large) * parseMemoryBudgetBytesPerSourceByte
	if got := parseMemoryBudget(10); got != 512*mib {
		t.Fatalf("small source budget = %d, want the 512 MiB floor", got)
	}
	if got := parseMemoryBudget(large); got != scaled {
		t.Fatalf("large source budget = %d, want %d", got, scaled)
	}

	p := &Parser{}
	if got := parseMemoryHardCeilingBytesForParse(p, 10); got != 2048*mib {
		t.Fatalf("small source ceiling = %d, want the 2 GiB floor", got)
	}
	if got, want := parseMemoryHardCeilingBytesForParse(p, large), parseMemoryHardCeilingBudgetMultiple*scaled; got != want {
		t.Fatalf("large source ceiling = %d, want %d", got, want)
	}

	p.SetMemoryBudgetBytes(4096 * mib)
	if got := parseMemoryBudgetForParser(p, large); got != 4096*mib {
		t.Fatalf("fixed budget = %d, want 4 GiB for every source size", got)
	}
	if got, want := parseMemoryHardCeilingBytesForParse(p, 10), parseMemoryHardCeilingBudgetMultiple*4096*mib; got != want {
		t.Fatalf("ceiling for a fixed budget = %d, want %d", got, want)
	}

	p.SetMemoryBudgetBytes(-1)
	if got := parseMemoryBudgetForParser(p, large); got != 0 {
		t.Fatalf("budget off: parseMemoryBudgetForParser = %d, want 0", got)
	}
	if got := parseMemoryHardCeilingBytesForParse(p, large); got != 2048*mib {
		t.Fatalf("budget off: ceiling = %d, want the 2 GiB default", got)
	}
}

func TestParseMemoryBudgetEnvironmentValuesStayFixed(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "64")
	t.Setenv("GOT_PARSE_MEMORY_HARD_CEILING_MB", "100")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	const mib = int64(1) << 20
	if got := parseMemoryBudget(16 << 20); got != 64*mib {
		t.Fatalf("environment budget = %d, want a fixed 64 MiB", got)
	}
	p := &Parser{}
	p.SetMemoryBudgetBytes(4096 * mib)
	if got := parseMemoryHardCeilingBytesForParse(p, 16<<20); got != 100*mib {
		t.Fatalf("environment ceiling = %d, want a fixed 100 MiB", got)
	}
}

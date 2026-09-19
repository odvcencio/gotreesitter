package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func memoryBudgetWitnessSource() []byte {
	var buf bytes.Buffer
	buf.WriteString("package p\n\nfunc f() {\n")
	for i := 0; i < 5000; i++ {
		buf.WriteString("\tvar x = 1\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes()
}

func parseStopWithMemoryBudget(t *testing.T, parse func([]byte) (*gts.Tree, error), source []byte) gts.ParseStopReason {
	t.Helper()
	// A warm arena pool can let one large batch pass the budget between
	// checks. Start each parse from a cold pool.
	gts.DrainArenaPools()
	tree, err := parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	return tree.ParseStopReason()
}

// TestParserSetMemoryBudgetBytesOverridesDefault checks each setter value
// against a 1 MiB default budget.
func TestParserSetMemoryBudgetBytesOverridesDefault(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gts.ResetParseEnvConfigCacheForTests()
	defer gts.ResetParseEnvConfigCacheForTests()

	source := memoryBudgetWitnessSource()
	parser := gts.NewParser(grammars.GoLanguage())
	if got := parseStopWithMemoryBudget(t, parser.Parse, source); got != gts.ParseStopMemoryBudget {
		t.Fatalf("default budget: stop = %q, want %q", got, gts.ParseStopMemoryBudget)
	}

	parser.SetMemoryBudgetBytes(256 << 20)
	if got := parser.MemoryBudgetBytes(); got != 256<<20 {
		t.Fatalf("MemoryBudgetBytes() = %d, want %d", got, 256<<20)
	}
	if got := parseStopWithMemoryBudget(t, parser.Parse, source); got != gts.ParseStopAccepted {
		t.Fatalf("larger budget: stop = %q, want %q", got, gts.ParseStopAccepted)
	}

	parser.SetMemoryBudgetBytes(-5)
	if got := parser.MemoryBudgetBytes(); got != -1 {
		t.Fatalf("MemoryBudgetBytes() after a negative value = %d, want -1", got)
	}
	if got := parseStopWithMemoryBudget(t, parser.Parse, source); got != gts.ParseStopAccepted {
		t.Fatalf("budget off: stop = %q, want %q", got, gts.ParseStopAccepted)
	}

	parser.SetMemoryBudgetBytes(0)
	if got := parseStopWithMemoryBudget(t, parser.Parse, source); got != gts.ParseStopMemoryBudget {
		t.Fatalf("restored default: stop = %q, want %q", got, gts.ParseStopMemoryBudget)
	}
}

// TestParserPoolMemoryBudgetBytesOption checks that pooled parsers get the
// configured budget on each checkout.
func TestParserPoolMemoryBudgetBytesOption(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "1")
	gts.ResetParseEnvConfigCacheForTests()
	defer gts.ResetParseEnvConfigCacheForTests()

	source := memoryBudgetWitnessSource()
	lang := grammars.GoLanguage()
	defaultPool := gts.NewParserPool(lang)
	if got := parseStopWithMemoryBudget(t, defaultPool.Parse, source); got != gts.ParseStopMemoryBudget {
		t.Fatalf("default pool: stop = %q, want %q", got, gts.ParseStopMemoryBudget)
	}
	largePool := gts.NewParserPool(lang, gts.WithParserPoolMemoryBudgetBytes(256<<20))
	for i := 0; i < 2; i++ {
		if got := parseStopWithMemoryBudget(t, largePool.Parse, source); got != gts.ParseStopAccepted {
			t.Fatalf("pool with budget, parse %d: stop = %q, want %q", i, got, gts.ParseStopAccepted)
		}
	}
}

// TestDefaultMemoryBudgetScalesWithInput checks the default budget end to
// end. With a 1 MiB floor, a fixed 1 MiB budget stops this input, but the
// size-scaled default must let it finish.
func TestDefaultMemoryBudgetScalesWithInput(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "")
	gts.ResetParseEnvConfigCacheForTests()
	defer gts.ResetParseEnvConfigCacheForTests()
	restore := gts.SetParseMemoryBudgetFloorMBForTest(1)
	defer restore()

	source := memoryBudgetWitnessSource()
	parser := gts.NewParser(grammars.GoLanguage())
	if got := parseStopWithMemoryBudget(t, parser.Parse, source); got != gts.ParseStopAccepted {
		t.Fatalf("size-scaled default budget: stop = %q, want %q", got, gts.ParseStopAccepted)
	}
	// A parser keeps its arena slabs, and the budget counts growth beyond
	// them. Use a new parser so the fixed budget applies to all memory.
	fixed := gts.NewParser(grammars.GoLanguage())
	fixed.SetMemoryBudgetBytes(1 << 20)
	if got := parseStopWithMemoryBudget(t, fixed.Parse, source); got != gts.ParseStopMemoryBudget {
		t.Fatalf("fixed 1 MiB budget: stop = %q, want %q", got, gts.ParseStopMemoryBudget)
	}
}

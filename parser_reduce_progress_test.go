package gotreesitter

import "testing"

func TestReductionProgressGuardAllowsShrinkingStack(t *testing.T) {
	guard := reductionProgressGuard{minimumDepth: maxConsecutivePrimaryReduces * 3}
	for depth := guard.minimumDepth - 1; depth > 0; depth-- {
		if !guard.advance(depth) {
			t.Fatalf("stopped a shrinking stack at depth %d", depth)
		}
	}
}

func TestReductionProgressGuardBoundsStalledStack(t *testing.T) {
	for _, mode := range []string{"unchanged", "growing", "repeated_growth_and_shrink"} {
		t.Run(mode, func(t *testing.T) {
			guard := reductionProgressGuard{minimumDepth: 2}
			for step := 1; step <= maxConsecutivePrimaryReduces+1; step++ {
				depth := 2
				switch mode {
				case "growing":
					depth += step
				case "repeated_growth_and_shrink":
					depth += step % 2
				}
				if got, want := guard.advance(depth), step <= maxConsecutivePrimaryReduces; got != want {
					t.Fatalf("step %d at depth %d: continue=%t, want %t", step, depth, got, want)
				}
			}
		})
	}
}

func TestReductionProgressGuardRenewsBudgetOnlyBelowMinimum(t *testing.T) {
	guard := reductionProgressGuard{minimumDepth: 3}
	for step := 0; step < maxConsecutivePrimaryReduces; step++ {
		if !guard.advance(3) {
			t.Fatal("stopped before the limit")
		}
	}
	if !guard.advance(2) {
		t.Fatal("a new minimum did not renew the budget")
	}
	for step := 0; step < maxConsecutivePrimaryReduces; step++ {
		if !guard.advance(2) {
			t.Fatal("stopped before the renewed limit")
		}
	}
	if guard.advance(2) {
		t.Fatal("unchanged depth bypassed the renewed limit")
	}
}

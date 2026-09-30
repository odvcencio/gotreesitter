package retrybudget

import (
	"math"
	"testing"
)

func TestFirstPassAllowance(t *testing.T) {
	var b Budget
	b.Seed(100, 3, 8)
	if b.Remaining != 600 || !b.TakePass(24) {
		t.Fatalf("first retry: %+v", b)
	}
	b.Charge(100, 4)
	b.Seed(1000, 7, 8)
	if b.Remaining != 200 || !b.TakePass(24) {
		t.Fatalf("budget renewed: %+v", b)
	}
	b.Charge(100, 4)
	if !b.Exhausted(24) || b.TakePass(24) || b.Passes != 2 {
		t.Fatalf("exhausted budget: %+v", b)
	}
}

func TestStackCeilingSuppressesRetries(t *testing.T) {
	for _, stacks := range []int{8, 12} {
		var b Budget
		b.Seed(100, stacks, 8)
		if b.TakePass(24) {
			t.Fatalf("stack pressure granted work: %+v", b)
		}
	}
}

func TestPassLimitWithoutWorkPolicy(t *testing.T) {
	var b Budget
	for i := 0; i < 24; i++ {
		if !b.TakePass(24) {
			t.Fatalf("retry %d denied", i)
		}
	}
	if b.TakePass(24) {
		t.Fatal("pass ceiling did not hold")
	}
}

func TestWorkOverflowCannotRenewBudget(t *testing.T) {
	var b Budget
	b.Seed(math.MaxUint64, 7, 8)
	if b.Remaining != math.MaxUint64 {
		t.Fatalf("overflowed allowance: %+v", b)
	}
	b.Charge(math.MaxUint64, 2)
	if !b.Exhausted(24) {
		t.Fatalf("overflowed charge: %+v", b)
	}
}

//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactRecoveryMemoMemoryBudget(t *testing.T) {
	compact, source := newRecoveryCostFixture(t, "abc")
	child, err := compact.ErrorRegionLeaf(5, 0, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.ErrorRegionResume(seed, 1, 0, 3, []core.SubtreeID{child})
	if err != nil {
		t.Fatal(err)
	}
	derivations, err := compact.Derivations(head)
	if err != nil || len(derivations) != 1 || len(derivations[0].Payloads) != 1 {
		t.Fatalf("recovery fixture: derivations=%v err=%v", derivations, err)
	}
	for _, ceiling := range []bool{false, true} {
		t.Run(map[bool]string{false: "budget", true: "ceiling"}[ceiling], func(t *testing.T) {
			var scheduler diagnosticParserCoreGenericScheduler
			before := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
			limit := int64((before + 1) * uint64(stopControlFootprintChurnRatio))
			if ceiling {
				scheduler.options.stopControlHardCeilingBytes = limit
			} else {
				scheduler.options.stopControlMemoryBudgetBytes = limit
			}
			if got := scheduler.stopControlMemoryBudgetReason(); got != ParseStopNone {
				t.Fatalf("empty memo stop reason = %v", got)
			}
			if _, err := core.RecoveryNodeErrorCostMemo(visibleSymbols(8), source, &scheduler.recoveryCostMemo, derivations[0].Payloads[0]); err != nil {
				t.Fatal(err)
			}
			if scheduler.recoveryCostMemo.Len() == 0 {
				t.Fatal("recovery pricing did not allocate the memo")
			}
			after := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
			want := uint64(scheduler.recoveryCostMemo.Len()) * uint64(unsafe.Sizeof(uint32(0))+unsafe.Sizeof(false))
			if got := after - before; got != want {
				t.Fatalf("memo footprint growth = %d, want %d", got, want)
			}
			if got := scheduler.stopControlMemoryBudgetReason(); got != ParseStopMemoryBudget {
				t.Fatalf("populated memo stop reason = %v, want memory budget", got)
			}
			if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
				t.Fatal(err)
			}
			if got := diagnosticParserCoreSchedulerFootprintBytes(&scheduler); got != after {
				t.Fatalf("reset footprint = %d, want retained footprint %d", got, after)
			}
		})
	}
}

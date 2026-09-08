//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"
	"unsafe"
)

func TestCompactMemoryBudgetDetectsGrowthAfterSmallPoll(t *testing.T) {
	for _, ceiling := range []bool{false, true} {
		t.Run(map[bool]string{false: "budget", true: "ceiling"}[ceiling], func(t *testing.T) {
			var scheduler diagnosticParserCoreGenericScheduler
			before := diagnosticParserCoreSchedulerFootprintBytes(&scheduler)
			limit := int64((before + 1) * uint64(stopControlFootprintChurnRatio) * 4)
			if ceiling {
				scheduler.options.stopControlHardCeilingBytes = limit
			} else {
				scheduler.options.stopControlMemoryBudgetBytes = limit
			}
			if got := scheduler.stopControlMemoryBudgetReason(); got != ParseStopNone {
				t.Fatalf("initial poll=%v", got)
			}
			scheduler.headers = make([]diagnosticParserCoreHeader, 0, int(uint64(limit)/uint64(unsafe.Sizeof(diagnosticParserCoreHeader{})))+1)
			if got := scheduler.stopControlMemoryBudgetReason(); got != ParseStopMemoryBudget {
				t.Fatalf("growth poll=%v; want memory stop", got)
			}
		})
	}
}

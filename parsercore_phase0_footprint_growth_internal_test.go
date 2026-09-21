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

// TestDiagnosticParserCoreSchedulerFootprintBytesCountsZeroWidthCatchUpState
// is the regression test for the review finding that
// diagnosticParserCoreSchedulerFootprintBytes omitted zeroWidthCatchUp (the
// per-header, per-election catch-up budget sidecar map,
// parsercore_phase0_driver.go) and relexZeroWidthPreScanScratch (the borrow
// scratch relexZeroWidthExternalTokenForState uses). Either omission would
// let the stop-control memory budget undercount a scheduler that has grown
// one of these two retained buffers.
func TestDiagnosticParserCoreSchedulerFootprintBytesCountsZeroWidthCatchUpState(t *testing.T) {
	var withoutMap diagnosticParserCoreGenericScheduler
	before := diagnosticParserCoreSchedulerFootprintBytes(&withoutMap)

	withMap := withoutMap
	withMap.zeroWidthCatchUp = map[uint64]diagnosticParserCoreZeroWidthCatchUpState{
		1: {budget: 4, election: 1},
		2: {budget: 3, election: 1},
	}
	if got := diagnosticParserCoreSchedulerFootprintBytes(&withMap); got <= before {
		t.Fatalf("footprint with a populated zeroWidthCatchUp = %d, want more than the empty scheduler's %d", got, before)
	}

	withScratch := withoutMap
	withScratch.relexZeroWidthPreScanScratch = make([]byte, 0, 4096)
	if got := diagnosticParserCoreSchedulerFootprintBytes(&withScratch); got <= before {
		t.Fatalf("footprint with a grown relexZeroWidthPreScanScratch = %d, want more than the empty scheduler's %d", got, before)
	}
}

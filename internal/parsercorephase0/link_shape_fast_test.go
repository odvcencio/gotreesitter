package parsercorephase0

import (
	"math"
	"testing"
)

func TestLinkShapeFastPathPreservesValidationAndErrorOrder(t *testing.T) {
	flags := []uint32{0, linkFlagHasOrder, linkFlagRecoveryDiscontinuity, linkKnownFlags, math.MaxUint32}
	for bit := uint(2); bit < 32; bit++ {
		flags = append(flags, 1<<bit, 1<<bit|linkFlagHasOrder, 1<<bit|linkFlagRecoveryDiscontinuity)
	}
	for _, flag := range flags {
		for _, payload := range []SubtreeID{0, 1, math.MaxUint32} {
			for _, score := range []int64{0, 1, -1, math.MinInt64, math.MaxInt64} {
				for _, order := range []uint64{0, 1, math.MaxUint64} {
					link := linkRecord{flags: flag, payload: payload, scoreDelta: score, order: order, prev: math.MaxUint32, next: math.MaxUint32}
					fast, original := link.validateShape(), link.validateUncommonShape()
					if (fast == nil) != (original == nil) {
						t.Fatalf("shape=%+v fast=%v original=%v", link, fast, original)
					}
					if fast != nil && fast.Error() != original.Error() {
						t.Fatalf("error order changed: fast=%v original=%v", fast, original)
					}
				}
			}
		}
	}
}

// Package recoverymerge defines the cost constraint for immutable closed
// error histories retained in a native recovery version.
package recoverymerge

// EqualCosts preserves C's prefix cost constraint when packing closed errors.
func EqualCosts(a, b uint32) bool { return a == b }

// ClosedHistoriesCertified keeps checkpoint-backed scanners on their established
// recovery path until physical recovery order, not just conflict order, is proven.
func ClosedHistoriesCertified(conflictOrder, recoveryOrder, scannerCheckpoints bool) bool {
	return conflictOrder && (!scannerCheckpoints || recoveryOrder)
}

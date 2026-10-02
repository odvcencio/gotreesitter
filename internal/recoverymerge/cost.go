// Package recoverymerge defines the cost constraint for immutable closed
// error histories retained in a native recovery version.
package recoverymerge

// EqualCosts preserves C's prefix cost constraint when packing closed errors.
func EqualCosts(a, b uint32) bool { return a == b }

package recoveryturn

// MissingVersionTurnsForIncludedRanges narrows an existing certification only
// when an authenticated artifact's grammar owner records a mode exclusion.
// Missing identity or an absent hook preserves the caller's existing policy;
// this function cannot grant certification when certified is false.
func MissingVersionTurnsForIncludedRanges(certified bool, identity [32]byte, identityKnown bool, scanner any) bool {
	if !certified {
		return false
	}
	if !identityKnown {
		return true
	}
	exclusion, ok := scanner.(interface {
		ExcludesMissingVersionTurnsForIncludedRanges([32]byte) bool
	})
	return !ok || !exclusion.ExcludesMissingVersionTurnsForIncludedRanges(identity)
}

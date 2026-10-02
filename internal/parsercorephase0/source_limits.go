package parsercorephase0

import "math"

// SourceRecordLimits gives larger inputs the same record capacity per byte as
// the first capacity tier. These arena bounds limit storage independently of
// the scheduler's memory budget, which must still be polled before publication.
// Limits on live branches, derivations, and pop paths do not scale with input.
func SourceRecordLimits(base Limits, sourceBytes int) Limits {
	if sourceBytes <= 0 || uint64(sourceBytes) < uint64(base.MaxNodes) || base.MaxNodes == 0 {
		return base
	}
	tiers := uint64(sourceBytes)/uint64(base.MaxNodes) + 1
	scale := func(value uint32) uint32 {
		if value != 0 && tiers > uint64(math.MaxUint32)/uint64(value) {
			return math.MaxUint32
		}
		return uint32(uint64(value) * tiers)
	}
	base.MaxNodes = scale(base.MaxNodes)
	base.MaxLinks = scale(base.MaxLinks)
	base.MaxSubtrees = scale(base.MaxSubtrees)
	base.MaxChildren = scale(base.MaxChildren)
	base.MaxMetadata = scale(base.MaxMetadata)
	return base
}

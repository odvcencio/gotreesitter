package gotreesitter

import "sync/atomic"

func parseIterations(sourceLen int) int {
	return max(10_000, sourceLen*30)
}

// parseStackDepth returns the stack depth limit scaled to input size.
func parseStackDepth(sourceLen int) int {
	return max(1_000, sourceLen*2)
}

// parseNodeLimit returns the maximum number of Node allocations allowed.
// This is the hard ceiling that prevents OOM regardless of iteration count.
func parseNodeLimit(sourceLen int) int {
	// Keep the default budget high enough for large full-parse corpora so
	// correctness gates can run without relying on external scale overrides.
	// The 300k floor avoids premature truncation on small/medium inputs
	// during short-lived ambiguity spikes and malformed-input recovery.
	// The sourceLen*52 budget avoids first-pass node-limit retries on the
	// default synthetic Go full-parse workload while staying materially below
	// the diagnostic 2x override used for deeper corpus investigations.
	limit := max(300_000, sourceLen*52)
	scale := parseNodeLimitScaleFactor()
	if scale <= 1 {
		return limit
	}
	maxInt := int(^uint(0) >> 1)
	if limit > maxInt/scale {
		return maxInt
	}
	return limit * scale
}

func parseMemoryBudget(sourceLen int) int64 {
	mb := parseMemoryBudgetMB()
	if mb <= 0 {
		return 0
	}
	// Keep the budget source-length aware so callers can lower it to zero for
	// tests without introducing an unused-parameter path here.
	if sourceLen < 0 {
		sourceLen = 0
	}
	return int64(mb) * 1024 * 1024
}

func parseFullArenaNodeCapacity(sourceLen, hint int) int {
	target := parseFullArenaInitialNodeCapacity(sourceLen)
	if hint <= 0 || hint < target {
		return target
	}
	limit := parseFullArenaHintLimit(sourceLen)
	if hint > limit {
		return max(target, limit)
	}
	return max(target, hint)
}

func parseFullArenaHintLimit(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	// Hints are learned from previous parses on the same Parser. In a ParserPool,
	// a parser that just handled a large file can later be checked out for a much
	// smaller file. Cap the reusable hint by the current source size so normal
	// concurrent full parses do not inherit a stale large-file preallocation.
	limit := sourceLen
	retainedFullNodes := nodeCapacityForBytes(maxRetainedFullNodeBytes)
	if limit > retainedFullNodes {
		limit = retainedFullNodes
	}
	return max(base, limit)
}

func parseFullArenaInitialNodeCapacity(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	// First-pass sizing when no adaptive hint exists yet. Empirically our Go
	// grammar consumes ~1 node per 5-10 input bytes, so sourceLen/4 gives
	// comfortable headroom without front-loading arena memory that never
	// becomes live. Any shortfall is absorbed by overflow slabs and the
	// adaptive hint on the next parse trims to observed peak + 25%.
	estimate := sourceLen / 4
	const maxPreallocNodes = 1_500_000
	if estimate > maxPreallocNodes {
		estimate = maxPreallocNodes
	}
	return max(base, estimate)
}

func (p *Parser) fullArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.fullArenaHint))
}

func (p *Parser) incrementalArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.incrementalArenaHint))
}

func (p *Parser) incrementalGSSHintCapacity() int {
	if p == nil {
		return defaultGSSNodeSlabCap
	}
	hint := int(atomic.LoadUint32(&p.incrementalGSSHint))
	if hint < defaultGSSNodeSlabCap {
		return defaultGSSNodeSlabCap
	}
	return hint
}

func (p *Parser) fullGSSHintCapacity() int {
	if p == nil {
		return fullParseGSSNodeSlabCap
	}
	hint := int(atomic.LoadUint32(&p.fullGSSHint))
	if hint < fullParseGSSNodeSlabCap {
		return fullParseGSSNodeSlabCap
	}
	return hint
}

func (p *Parser) recordFullArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/4 // keep 25% headroom above observed peak.
	base := nodeCapacityForClass(arenaClassFull)
	if target < base {
		target = base
	}
	const maxHintNodes = 2_000_000
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.fullArenaHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < base {
				blended = base
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.fullArenaHint, old, next) {
			return
		}
	}
}

func (p *Parser) recordIncrementalArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/8 // keep 12.5% headroom above observed peak.
	base := nodeCapacityForClass(arenaClassIncremental)
	if target < base {
		target = base
	}
	const maxHintNodes = 1_000_000
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.incrementalArenaHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < base {
				blended = base
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.incrementalArenaHint, old, next) {
			return
		}
	}
}

func (p *Parser) recordIncrementalGSSUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/8 // keep 12.5% headroom above observed peak.
	if target < defaultGSSNodeSlabCap {
		target = defaultGSSNodeSlabCap
	}
	const maxHintNodes = 512 * 1024
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.incrementalGSSHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < defaultGSSNodeSlabCap {
				blended = defaultGSSNodeSlabCap
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.incrementalGSSHint, old, next) {
			return
		}
	}
}

func (p *Parser) recordFullGSSUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/4 // keep 25% headroom above observed peak.
	if target < fullParseGSSNodeSlabCap {
		target = fullParseGSSNodeSlabCap
	}
	const maxHintNodes = 1_024 * 1_024
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.fullGSSHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < fullParseGSSNodeSlabCap {
				blended = fullParseGSSNodeSlabCap
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.fullGSSHint, old, next) {
			return
		}
	}
}

func parseFullEntryScratchCapacity(sourceLen int) int {
	if sourceLen <= 0 {
		return defaultStackEntrySlabCap
	}
	estimate := sourceLen * 12
	if estimate < defaultStackEntrySlabCap {
		estimate = defaultStackEntrySlabCap
	}
	// Keep initial scratch growth bounded; larger capacities are still
	// reached on demand and retained up to maxRetainedStackEntryCap.
	const maxPreallocEntries = 768 * 1024
	if estimate > maxPreallocEntries {
		estimate = maxPreallocEntries
	}
	return estimate
}

func tuneIncrementalGLRCaps(maxStacks, mergePerKeyCap int) (int, int) {
	if maxStacks > 2 {
		maxStacks = 2
	}
	if mergePerKeyCap > 2 {
		mergePerKeyCap = 2
	}
	return maxStacks, mergePerKeyCap
}

func parseIncrementalArenaNodeCapacity(sourceLen, hint int) int {
	base := nodeCapacityForClass(arenaClassIncremental)
	target := base
	if sourceLen > 0 {
		estimate := sourceLen * 4
		const maxPreallocNodes = 512 * 1024
		if estimate > maxPreallocNodes {
			estimate = maxPreallocNodes
		}
		target = max(base, estimate)
	}
	if hint <= 0 || hint < target {
		return target
	}
	limit := parseNodeLimit(sourceLen)
	if hint > limit {
		return max(base, limit)
	}
	return max(base, hint)
}

func parseIncrementalEntryScratchCapacity(sourceLen int) int {
	if sourceLen <= 0 {
		return defaultStackEntrySlabCap
	}
	estimate := sourceLen * 8
	if estimate < defaultStackEntrySlabCap {
		estimate = defaultStackEntrySlabCap
	}
	const maxPreallocEntries = 256 * 1024
	if estimate > maxPreallocEntries {
		estimate = maxPreallocEntries
	}
	return estimate
}

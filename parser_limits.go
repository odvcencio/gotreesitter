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
	return parseNodeLimitForLanguage(sourceLen, nil)
}

// parseNodeLimitForLanguage is parseNodeLimit with a per-language budget tuned
// for grammars that allocate unusually many nodes per input byte. The default
// sourceLen*52 budget is calibrated against the synthetic Go full-parse
// workload; grammars with heavy GLR fanout (notably tree-sitter-markdown's
// inline code-span / emphasis / latex-span / strikethrough external scanner
// and its ambiguous block/inline split) can legitimately consume 150+ nodes
// per byte on dense real-world inputs. Raising only the per-byte factor for
// those grammars avoids forcing two full-parse retries on every small doc
// while preserving the OOM ceiling for other languages.
func parseNodeLimitForLanguage(sourceLen int, lang *Language) int {
	perByte := 52
	if lang != nil {
		switch lang.Name {
		case "markdown", "markdown_inline":
			// Measured on the mdpp zero-cgo-parsing.mdpp corpus: 11 KB input
			// drove ~1.9M node allocations (~170/byte). 200/byte keeps the
			// first parse inside the ceiling without retry churn and still
			// bounds pathological inputs.
			perByte = 200
		}
	}
	limit := max(300_000, sourceLen*perByte)
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

func parseMemoryBudgetForParser(p *Parser, sourceLen int) int64 {
	budget := parseMemoryBudget(sourceLen)
	if p == nil || !p.skipRecoveryReparse || p.language == nil {
		return budget
	}
	if p.language.Name != "c_sharp" {
		return budget
	}
	const csharpRecoveryBudget = int64(64 * 1024 * 1024)
	if budget == 0 || budget > csharpRecoveryBudget {
		return csharpRecoveryBudget
	}
	return budget
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
	// sourceLen (bytes) is used directly as a loose upper bound on node count —
	// grammars produce well under 1 node per byte, so this is a conservative
	// ceiling, intentionally roomier than parseFullArenaInitialNodeCapacity
	// (sourceLen/4) so useful same-size hints fall between initial and limit.
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
	// First-pass sizing when no adaptive hint exists yet. Small and medium
	// sources use conservative headroom so ordinary parses do not front-load
	// arena memory they never touch. Very large generated sources, especially
	// Python/SWIG output, commonly approach one node per input byte; giving
	// those parses a larger primary arena avoids repeated overflow-slab growth
	// that is thrown away immediately by the large-arena retention guard.
	estimate := sourceLen / 4
	if sourceLen >= 1024*1024 {
		estimate = sourceLen
	} else if sourceLen >= 128*1024 {
		// Medium synthetic/full-parse workloads can need well over sourceLen/4
		// nodes. Preallocating closer to the steady-state footprint avoids
		// geometric overflow slabs that raise peak RSS during the first parse.
		estimate = sourceLen * 2 / 3
	}
	const maxPreallocNodes = 1_500_000
	if estimate > maxPreallocNodes {
		estimate = maxPreallocNodes
	}
	return max(base, estimate)
}

func parsePendingFullArenaNodeCapacity(sourceLen, hint int) int {
	target := parsePendingFullArenaInitialNodeCapacity(sourceLen)
	if hint <= 0 {
		return target
	}
	limit := parsePendingFullArenaHintLimit(sourceLen)
	if hint > limit {
		hint = limit
	}
	base := nodeCapacityForClass(arenaClassFull)
	if hint < base {
		hint = base
	}
	if hint < target && hint < target*3/4 {
		return target
	}
	return hint
}

func parsePendingFullArenaHintLimit(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	limit := sourceLen
	const maxPendingFullArenaHintNodes = 2_000_000
	if limit > maxPendingFullArenaHintNodes {
		limit = maxPendingFullArenaHintNodes
	}
	return max(base, limit)
}

func parsePendingFullArenaInitialNodeCapacity(sourceLen int) int {
	target := parseFullArenaInitialNodeCapacity(sourceLen)
	if sourceLen < 1024*1024 {
		return target
	}
	estimate := sourceLen / 2
	const maxPendingFullParentPreallocNodes = 1_050_000
	if estimate > maxPendingFullParentPreallocNodes {
		estimate = maxPendingFullParentPreallocNodes
	}
	base := nodeCapacityForClass(arenaClassFull)
	pendingTarget := max(base, estimate)
	if pendingTarget < target {
		return pendingTarget
	}
	return target
}

func parseCompactFullArenaNodeCapacity(sourceLen, hint int) int {
	target := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	if hint <= 0 {
		return target
	}
	limit := parseCompactFullArenaHintLimit(sourceLen)
	if hint > limit {
		hint = limit
	}
	base := nodeCapacityForClass(arenaClassFull)
	if hint < base {
		hint = base
	}
	if hint < target/2 {
		return target
	}
	return hint
}

func parseCompactFullArenaHintLimit(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	limit := sourceLen / 2
	const maxCompactFullArenaHintNodes = 1_000_000
	if limit > maxCompactFullArenaHintNodes {
		limit = maxCompactFullArenaHintNodes
	}
	return max(base, limit)
}

func parseCompactFullArenaInitialNodeCapacity(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	estimate := sourceLen / 4
	const maxCompactFullArenaPreallocNodes = 750_000
	if estimate > maxCompactFullArenaPreallocNodes {
		estimate = maxCompactFullArenaPreallocNodes
	}
	return max(base, estimate)
}

func parseFinalChildRefArenaNodeCapacity(sourceLen, hint int) int {
	target := parseFinalChildRefArenaInitialNodeCapacity(sourceLen)
	if hint <= 0 {
		return target
	}
	limit := parseFinalChildRefArenaHintLimit(sourceLen)
	if hint > limit {
		hint = limit
	}
	base := nodeCapacityForClass(arenaClassFull)
	if hint < base {
		hint = base
	}
	if hint < target/2 {
		return target
	}
	return hint
}

func parseFinalChildRefArenaHintLimit(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	limit := parseFinalChildRefArenaInitialNodeCapacity(sourceLen) * 4
	const maxFinalChildRefArenaHintNodes = 512 * 1024
	if limit > maxFinalChildRefArenaHintNodes {
		limit = maxFinalChildRefArenaHintNodes
	}
	return max(base, limit)
}

func parseFinalChildRefArenaInitialNodeCapacity(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	estimate := sourceLen / 384
	const minFinalChildRefArenaPreallocNodes = 64 * 1024
	if estimate < minFinalChildRefArenaPreallocNodes {
		estimate = minFinalChildRefArenaPreallocNodes
	}
	const maxFinalChildRefArenaPreallocNodes = 256 * 1024
	if estimate > maxFinalChildRefArenaPreallocNodes {
		estimate = maxFinalChildRefArenaPreallocNodes
	}
	return max(base, estimate)
}

func parseFullExternalScannerCheckpointCapacity(sourceLen, nodeCapacity int) int {
	if sourceLen < 256*1024 || nodeCapacity <= 0 {
		return 0
	}
	sourceSized := sourceLen * 3 / 8
	if sourceSized <= 0 || sourceSized > nodeCapacity {
		return nodeCapacity
	}
	return sourceSized
}

func parseNoTreeArenaNodeCapacity(sourceLen int) int {
	if !parseShouldCompactNoTreeShiftLeaves(sourceLen) {
		return parseNoTreeFullLeafArenaNodeCapacity(sourceLen)
	}
	// Large no-tree parses keep shifted leaves and reductions in compact
	// noTreeNode slabs, so full Node capacity is needed only for the returned
	// placeholder root and unusual recovery/error fallbacks.
	return nodeCapacityForClass(arenaClassFull)
}

func parseShouldCompactNoTreeShiftLeaves(sourceLen int) bool {
	return sourceLen >= 256*1024
}

func parseShouldUseCompactFullShiftLeaves(p *Parser, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	if p == nil {
		return false
	}
	compactConfigured, compactEnabled := parseCompactFullLeavesEnv()
	if compactConfigured && !compactEnabled {
		return false
	}
	defaultPythonLargeNoCompat := !compactConfigured &&
		p.noResultCompatibilityBenchmarkOnly &&
		p.language != nil &&
		p.language.Name == "python"
	return p != nil &&
		(compactEnabled || defaultPythonLargeNoCompat) &&
		!p.noTreeBenchmarkOnly &&
		p.noResultCompatibilityBenchmarkOnly &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		len(source) >= 256*1024
}

func parseShouldUsePendingFullParents(p *Parser, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	if p == nil {
		return false
	}
	pendingConfigured, pendingEnabled := parsePendingParentsEnv()
	if pendingConfigured && !pendingEnabled {
		return false
	}
	compactLeafParentBoundary := parseCompactFullLeavesEnabled() && p.noResultCompatibilityBenchmarkOnly
	defaultPythonLargeNoCompat := !pendingConfigured &&
		p.noResultCompatibilityBenchmarkOnly &&
		p.language != nil &&
		p.language.Name == "python"
	return p != nil &&
		(pendingEnabled || compactLeafParentBoundary || defaultPythonLargeNoCompat) &&
		!p.noTreeBenchmarkOnly &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		len(source) >= 256*1024
}

func parseShouldUseFinalChildRefs(p *Parser, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	if p == nil {
		return false
	}
	finalConfigured, finalEnabled := parseFinalChildRefsEnv()
	if finalConfigured && !finalEnabled {
		return false
	}
	defaultPythonLargeNoCompat := !finalConfigured &&
		p.pendingFullParents &&
		p.noResultCompatibilityBenchmarkOnly &&
		p.language != nil &&
		p.language.Name == "python"
	return p != nil &&
		(finalEnabled || defaultPythonLargeNoCompat) &&
		p.pendingFullParents &&
		p.noResultCompatibilityBenchmarkOnly &&
		!p.noTreeBenchmarkOnly &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		len(source) >= 256*1024
}

func parseShouldSkipInvisibleFullLeafCheckpoints(p *Parser, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	return p != nil &&
		p.noResultCompatibilityBenchmarkOnly &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		p.language != nil &&
		p.language.Name == "python" &&
		len(source) >= 256*1024
}

func parseShouldCaptureFullMaterializationTiming(p *Parser, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	return p != nil &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		p.language != nil &&
		p.language.Name == "python" &&
		len(source) >= 256*1024
}

func parseShouldCaptureMaterializationTiming(p *Parser, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	if p == nil || p.noTreeBenchmarkOnly {
		return false
	}
	if parsePhaseTimingEnabled() {
		return true
	}
	return parseShouldCaptureFullMaterializationTiming(p, source, reuse, oldTree, arenaClass)
}

func parseShouldUseTransientReduceScratchNoAlias(sourceLen int) bool {
	return sourceLen >= 256*1024
}

func parseNoTreeFullLeafArenaNodeCapacity(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if sourceLen <= 0 {
		return base
	}
	// Small no-tree parses still use real shifted leaves; keep enough full Node
	// capacity for that mode.
	estimate := sourceLen / 2
	if sourceLen >= 1024*1024 {
		estimate = sourceLen / 3
	}
	const maxPreallocNodes = 1_000_000
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

func (p *Parser) pendingFullArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.pendingFullArenaHint))
}

func (p *Parser) compactFullArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.compactFullArenaHint))
}

func (p *Parser) finalChildRefArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.finalChildRefArenaHint))
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
	p.recordArenaUsageHint(&p.fullArenaHint, used, 2_000_000, parseFullArenaHintHeadroom)
}

func (p *Parser) recordPendingFullArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	p.recordArenaUsageHint(&p.pendingFullArenaHint, used, 2_000_000, parsePendingFullArenaHintHeadroom)
}

func (p *Parser) recordCompactFullArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	p.recordArenaUsageHint(&p.compactFullArenaHint, used, 1_000_000, parseCompactFullArenaHintHeadroom)
}

func (p *Parser) recordFinalChildRefArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	p.recordArenaUsageHint(&p.finalChildRefArenaHint, used, 512*1024, parseFinalChildRefArenaHintHeadroom)
}

func (p *Parser) recordArenaUsageHint(slot *uint32, used int, maxHintNodes int, headroom func(int) int) {
	if p == nil || slot == nil || used <= 0 {
		return
	}
	target := used
	if headroom != nil {
		target += headroom(used)
	}
	base := nodeCapacityForClass(arenaClassFull)
	if target < base {
		target = base
	}
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(slot)
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
		if old == next || atomic.CompareAndSwapUint32(slot, old, next) {
			return
		}
	}
}

func parseFullArenaHintHeadroom(used int) int {
	if used <= 0 {
		return 0
	}
	if used < 256*1024 {
		return used / 16
	}
	headroom := used / 16
	const maxLargeFullArenaHintHeadroom = 64 * 1024
	if headroom > maxLargeFullArenaHintHeadroom {
		return maxLargeFullArenaHintHeadroom
	}
	return headroom
}

func parsePendingFullArenaHintHeadroom(used int) int {
	if used <= 0 {
		return 0
	}
	if used < 256*1024 {
		return used / 4
	}
	headroom := used / 32
	const maxLargePendingFullArenaHintHeadroom = 32 * 1024
	if headroom > maxLargePendingFullArenaHintHeadroom {
		return maxLargePendingFullArenaHintHeadroom
	}
	return headroom
}

func parseCompactFullArenaHintHeadroom(used int) int {
	if used <= 0 {
		return 0
	}
	if used < 256*1024 {
		return used / 4
	}
	headroom := used / 16
	const maxLargeCompactFullArenaHintHeadroom = 32 * 1024
	if headroom > maxLargeCompactFullArenaHintHeadroom {
		return maxLargeCompactFullArenaHintHeadroom
	}
	return headroom
}

func parseFinalChildRefArenaHintHeadroom(used int) int {
	if used <= 0 {
		return 0
	}
	if used < 64*1024 {
		return used / 4
	}
	headroom := used / 16
	const maxFinalChildRefArenaHintHeadroom = 16 * 1024
	if headroom > maxFinalChildRefArenaHintHeadroom {
		return maxFinalChildRefArenaHintHeadroom
	}
	return headroom
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
	const maxPreallocEntries = 64 * 1024
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
		// Incremental reparses should rebuild only the dirty frontier. Keep the
		// cold-start arena modest and let observed parser hints or overflow slabs
		// handle the rarer wide invalidation case.
		estimate := sourceLen / 8
		const maxPreallocNodes = 64 * 1024
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

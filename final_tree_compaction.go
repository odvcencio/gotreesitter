package gotreesitter

import (
	"math"
	"unsafe"
)

const (
	// Returned-tree compaction is reserved for exceptional full parses. The
	// final-tree walk is itself O(nodes), so ordinary parses must fail the cheap
	// retained-arena check before doing any extra work.
	minFinalTreeCompactionArenaBytes = 512 * 1024 * 1024
	minFinalTreeCompactionReclaim    = 256 * 1024 * 1024
	finalTreeCompactionFixedHeadroom = 64 * 1024 * 1024
)

// maybeCompactReturnedFullTree copies a completed full-parse result into a
// right-sized arena when discarded GLR alternatives otherwise keep hundreds
// of megabytes of Node and slice slabs alive with the returned tree.
//
// Keep this at the outer Parser.Parse boundary, after retry selection,
// normalization, and recovery-result replacement. parseInternal results can
// still be discarded by those stages.
func (p *Parser) maybeCompactReturnedFullTree(tree *Tree, source []byte) *Tree {
	return p.maybeCompactReturnedFullTreeWithBudget(tree, source, parseMemoryBudgetForParser(p, len(source)))
}

func (p *Parser) maybeCompactReturnedFullTreeWithBudget(tree *Tree, source []byte, memoryBudget int64) *Tree {
	if !eligibleForFinalTreeCompaction(p, tree, source) {
		return tree
	}

	arena := tree.arena
	retainedBytes := arena.allocatedBytes
	if retainedBytes < minFinalTreeCompactionArenaBytes {
		return tree
	}

	stats := collectMaterializedFinalTreeStorageStats(tree.root)
	estimatedBytes, ok := estimateCompactedFinalTreeBytes(stats)
	if !ok || !finalTreeCompactionWorthwhile(retainedBytes, estimatedBytes, memoryBudget) {
		return tree
	}

	destination := acquireNodeArena(arenaClassFull)
	if !prepareArenaForFinalTreeClone(destination, stats) {
		destination.Release()
		return tree
	}
	root := cloneTreeNodesIntoArena(tree.root, destination)
	if root == nil {
		destination.Release()
		return tree
	}

	// The estimate is deliberately conservative, but keep the old tree if a
	// pooled destination happened to retain enough backing storage to erase the
	// expected win.
	if !finalTreeCompactionWorthwhile(retainedBytes, destination.allocatedBytes, memoryBudget) {
		destination.Release()
		return tree
	}

	tree.root = root
	tree.arena = destination
	tree.lastEditedLeaf = nil
	arena.Release()
	return tree
}

func eligibleForFinalTreeCompaction(p *Parser, tree *Tree, source []byte) bool {
	if p == nil || tree == nil || tree.released || tree.root == nil || tree.arena == nil {
		return false
	}
	if p.noTreeBenchmarkOnly || p.noResultCompatibilityBenchmarkOnly || p.crecoverySwallowedErrorCheckActive {
		return false
	}
	if tree.sourceEncoding != InputEncodingUTF8 || tree.sourceUTF16 != nil || len(source) != len(tree.source) {
		return false
	}
	if tree.rawParseStopReason() != ParseStopAccepted || tree.rawParseRuntime().Truncated || tree.forestFastPath || tree.incrementalReuseDisabled {
		return false
	}
	if tree.hasDeferredResultCompatibility() || tree.externalScannerCheckpointsDeferred || len(tree.includedRanges) != 0 || len(tree.edits) != 0 {
		return false
	}
	if len(tree.borrowedArena) != 0 || tree.arena.class != arenaClassFull || tree.arena.refs.Load() != 1 {
		return false
	}
	if tree.root.ownerArena != tree.arena || tree.arena.finalChildRefs {
		return false
	}
	return true
}

func collectMaterializedFinalTreeStorageStats(root *Node) finalTreeMaterializationStats {
	var stats finalTreeMaterializationStats
	if root == nil {
		return stats
	}
	var local [128]*Node
	stack := local[:0]
	stack = append(stack, root)
	for len(stack) > 0 {
		last := len(stack) - 1
		node := stack[last]
		stack = stack[:last]
		if node == nil {
			continue
		}
		stats.nodes++
		if metadata := node.fieldMetadata; metadata != nil {
			stats.nodeFieldMetadata++
			stats.fieldIDElements += uint64(len(metadata.ids))
			stats.fieldSourceElements += uint64(len(metadata.sources))
		}
		childCount := len(node.children)
		if childCount == 0 {
			stats.leafNodes++
			continue
		}
		stats.parentNodes++
		stats.childSlices++
		stats.childPointers += uint64(childCount)
		for i := childCount - 1; i >= 0; i-- {
			stack = append(stack, node.children[i])
		}
	}
	return stats
}

func prepareArenaForFinalTreeClone(arena *nodeArena, stats finalTreeMaterializationStats) bool {
	if arena == nil || arena.used != 0 {
		return false
	}
	maxInt := uint64(^uint(0) >> 1)
	counts := [...]uint64{
		stats.nodes,
		stats.childPointers,
		stats.fieldIDElements,
		stats.fieldSourceElements,
		stats.nodeFieldMetadata,
	}
	for _, count := range counts {
		if count > maxInt {
			return false
		}
	}

	arena.ensureExactNodeCapacity(int(stats.nodes))
	if stats.childPointers > 0 {
		arena.childSlabs = []childSliceSlab{{data: make([]*Node, int(stats.childPointers))}}
	} else {
		arena.childSlabs = nil
	}
	if stats.fieldIDElements > 0 {
		arena.fieldSlabs = []fieldSliceSlab{{data: make([]FieldID, int(stats.fieldIDElements))}}
	} else {
		arena.fieldSlabs = nil
	}
	if stats.fieldSourceElements > 0 {
		arena.fieldSourceSlabs = []fieldSourceSliceSlab{{data: make([]uint8, int(stats.fieldSourceElements))}}
	} else {
		arena.fieldSourceSlabs = nil
	}
	if stats.nodeFieldMetadata > 0 {
		arena.nodeFieldMetadataSlabs = []nodeFieldMetadataSlab{{data: make([]nodeFieldMetadata, int(stats.nodeFieldMetadata))}}
	} else {
		arena.nodeFieldMetadataSlabs = nil
	}
	arena.childSlabCursor = 0
	arena.fieldSlabCursor = 0
	arena.fieldSourceSlabCursor = 0
	arena.nodeFieldMetadataSlabCursor = 0
	arena.recomputeAllocatedBytes()
	return true
}

func estimateCompactedFinalTreeBytes(stats finalTreeMaterializationStats) (int64, bool) {
	total := uint64(finalTreeCompactionFixedHeadroom)
	parts := [...]struct {
		count uint64
		size  uintptr
	}{
		{stats.nodes, unsafe.Sizeof(Node{})},
		{stats.childPointers, unsafe.Sizeof((*Node)(nil))},
		{stats.nodeFieldMetadata, unsafe.Sizeof(nodeFieldMetadata{})},
		{stats.fieldIDElements, unsafe.Sizeof(FieldID(0))},
		{stats.fieldSourceElements, unsafe.Sizeof(uint8(0))},
	}
	for _, part := range parts {
		size := uint64(part.size)
		if size != 0 && part.count > (math.MaxUint64-total)/size {
			return 0, false
		}
		total += part.count * size
	}
	if total > math.MaxInt64 {
		return 0, false
	}
	return int64(total), true
}

func finalTreeCompactionWorthwhile(retainedBytes, compactedBytes, memoryBudget int64) bool {
	if retainedBytes <= 0 || compactedBytes <= 0 || memoryBudget <= 0 || compactedBytes >= retainedBytes {
		return false
	}
	reclaim := retainedBytes - compactedBytes
	if reclaim < minFinalTreeCompactionReclaim {
		return false
	}
	// Require at least 30% retained-arena reduction.
	if reclaim > math.MaxInt64/10 || retainedBytes > math.MaxInt64/3 {
		return false
	}
	if reclaim*10 < retainedBytes*3 {
		return false
	}
	// Leave one quarter of the parser memory budget for non-arena heap,
	// scratch still alive at the outer return boundary, and GC pacing while
	// both arenas coexist.
	peakAllowance := memoryBudget - memoryBudget/4
	if retainedBytes > peakAllowance || compactedBytes > peakAllowance-retainedBytes {
		return false
	}
	return true
}

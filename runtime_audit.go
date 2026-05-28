package gotreesitter

type runtimeAuditNodeKind uint8

const (
	runtimeAuditNodeKindLeaf runtimeAuditNodeKind = iota + 1
	runtimeAuditNodeKindParent
)

const stackEquivMismatchDepthBucketCount = 8

type runtimeAuditNodeInfo struct {
	gen                 uint32
	kind                runtimeAuditNodeKind
	reduceChildPath     reduceChildPath
	reduceChildPointers uint32
}

type runtimeAuditEquivStateInfo struct {
	state                                 StateID
	stackEquivCalls                       uint64
	stackEquivTrue                        uint64
	stackEquivDepthMismatch               uint64
	stackEquivHashMismatch                uint64
	stackEquivStateMismatch               uint64
	stackEquivPayloadMismatch             uint64
	stackEquivEntryCompares               uint64
	stackEquivStateMismatchDepthSum       uint64
	stackEquivStateMismatchMaxDepth       uint32
	stackEquivStateMismatchDepthBuckets   [stackEquivMismatchDepthBucketCount]uint64
	stackEquivPayloadMismatchDepthSum     uint64
	stackEquivPayloadMismatchMaxDepth     uint32
	stackEquivPayloadMismatchDepthBuckets [stackEquivMismatchDepthBucketCount]uint64
	stackEquivPayloadHeaderSigDiff        uint64
	stackEquivPayloadHeaderSigSame        uint64
	stackEquivPayloadShallowSigDiff       uint64
	stackEquivPayloadShallowSigSame       uint64
	stackEquivPairKeyed                   uint64
	stackEquivPairUnkeyed                 uint64
	stackEquivPairRepeats                 uint64
	stackEquivPairRepeatTrue              uint64
	stackEquivPairRepeatFalse             uint64
	stackEquivPairRepeatMismatch          uint64
	stackEquivPairStores                  uint64
	mergeHeaderEqTotal                    uint64
	mergeDeepTrue                         uint64
	mergeDeepFalse                        uint64
	mergeHeaderDeepDivergent              uint64
	equivCacheLookups                     uint64
	equivCacheHits                        uint64
	equivCacheStores                      uint64
	equivCacheMisses                      uint64
	equivCacheTrueHits                    uint64
	equivCacheFalseHits                   uint64
	equivCacheEpochMisses                 uint64
	equivCacheKeyMisses                   uint64
	equivCacheVersionMisses               uint64
	equivSkipError                        uint64
	equivSkipLeaf                         uint64
	equivSkipFieldMismatch                uint64
	equivExactCalls                       uint64
	equivExactTrue                        uint64
	equivExactPointerTrue                 uint64
	equivExactNilMismatch                 uint64
	equivExactHeaderMismatch              uint64
	equivExactChildMismatch               uint64
	equivExactTerminalCalls               uint64
	equivExactTerminalTrue                uint64
	equivExactTerminalFalse               uint64
	equivFrontierCalls                    uint64
	equivFrontierTrue                     uint64
	equivExactChildCompares               uint64
	equivFrontierChildScans               uint64
	equivFrontierCandidateCompares        uint64
}

type runtimeAuditStackEquivPairKey struct {
	a     uintptr
	b     uintptr
	depth uint32
}

type runtimeAudit struct {
	enabled              bool
	equivEnabled         bool
	currentTokenGen      uint32
	tokenActive          bool
	currentEquivState    StateID
	currentEquivStateSet bool

	gssGen     map[*gssNode]uint32
	nodeInfo   map[*Node]runtimeAuditNodeInfo
	seenGSS    map[*gssNode]struct{}
	seenNode   map[*Node]struct{}
	equivState map[StateID]*runtimeAuditEquivStateInfo
	equivPairs map[runtimeAuditStackEquivPairKey]bool

	currentGSSAllocated                 uint64
	currentGSSRetained                  uint64
	currentParentAllocated              uint64
	currentParentRetained               uint64
	currentLeafAllocated                uint64
	currentLeafRetained                 uint64
	currentChildSlicesAllocated         uint64
	currentChildSlicesRetained          uint64
	currentChildPointersAllocated       uint64
	currentChildPointersRetained        uint64
	currentReduceChildSlicesAllocated   [reduceChildPathCount]uint64
	currentReduceChildSlicesRetained    [reduceChildPathCount]uint64
	currentReduceChildPointersAllocated [reduceChildPathCount]uint64
	currentReduceChildPointersRetained  [reduceChildPathCount]uint64

	totalGSSAllocated                 uint64
	totalGSSRetained                  uint64
	totalGSSDropped                   uint64
	totalParentAllocated              uint64
	totalParentRetained               uint64
	totalParentDropped                uint64
	totalLeafAllocated                uint64
	totalLeafRetained                 uint64
	totalLeafDropped                  uint64
	totalChildSlicesAllocated         uint64
	totalChildSlicesRetained          uint64
	totalChildSlicesDropped           uint64
	totalChildPointersAllocated       uint64
	totalChildPointersRetained        uint64
	totalChildPointersDropped         uint64
	totalReduceChildSlicesAllocated   [reduceChildPathCount]uint64
	totalReduceChildSlicesRetained    [reduceChildPathCount]uint64
	totalReduceChildSlicesDropped     [reduceChildPathCount]uint64
	totalReduceChildPointersAllocated [reduceChildPathCount]uint64
	totalReduceChildPointersRetained  [reduceChildPathCount]uint64
	totalReduceChildPointersDropped   [reduceChildPathCount]uint64

	mergeStacksIn       uint64
	mergeStacksOut      uint64
	mergeSlotsUsed      uint64
	globalCullStacksIn  uint64
	globalCullStacksOut uint64

	stackEquivCalls                       uint64
	stackEquivTrue                        uint64
	stackEquivDepthMismatch               uint64
	stackEquivHashMismatch                uint64
	stackEquivStateMismatch               uint64
	stackEquivPayloadMismatch             uint64
	stackEquivEntryCompares               uint64
	stackEquivStateMismatchDepthSum       uint64
	stackEquivStateMismatchMaxDepth       uint32
	stackEquivStateMismatchDepthBuckets   [stackEquivMismatchDepthBucketCount]uint64
	stackEquivPayloadMismatchDepthSum     uint64
	stackEquivPayloadMismatchMaxDepth     uint32
	stackEquivPayloadMismatchDepthBuckets [stackEquivMismatchDepthBucketCount]uint64
	stackEquivPayloadHeaderSigDiff        uint64
	stackEquivPayloadHeaderSigSame        uint64
	stackEquivPayloadShallowSigDiff       uint64
	stackEquivPayloadShallowSigSame       uint64
	stackEquivPairKeyed                   uint64
	stackEquivPairUnkeyed                 uint64
	stackEquivPairRepeats                 uint64
	stackEquivPairRepeatTrue              uint64
	stackEquivPairRepeatFalse             uint64
	stackEquivPairRepeatMismatch          uint64
	stackEquivPairStores                  uint64
	mergeHeaderEqTotal                    uint64
	mergeDeepTrue                         uint64
	mergeDeepFalse                        uint64
	mergeHeaderDeepDivergent              uint64
	equivCacheLookups                     uint64
	equivCacheHits                        uint64
	equivCacheStores                      uint64
	equivCacheMisses                      uint64
	equivCacheTrueHits                    uint64
	equivCacheFalseHits                   uint64
	equivCacheEpochMisses                 uint64
	equivCacheKeyMisses                   uint64
	equivCacheVersionMisses               uint64
	equivSkipError                        uint64
	equivSkipLeaf                         uint64
	equivSkipFieldMismatch                uint64
	equivExactCalls                       uint64
	equivExactTrue                        uint64
	equivExactPointerTrue                 uint64
	equivExactNilMismatch                 uint64
	equivExactHeaderMismatch              uint64
	equivExactChildMismatch               uint64
	equivExactTerminalCalls               uint64
	equivExactTerminalTrue                uint64
	equivExactTerminalFalse               uint64
	equivFrontierCalls                    uint64
	equivFrontierTrue                     uint64
	equivExactChildCompares               uint64
	equivFrontierChildScans               uint64
	equivFrontierCandidateCompares        uint64
}

var runtimeAuditEnabled bool
var runtimeEquivAuditEnabled bool

// EnableRuntimeAudit toggles per-parse survivor instrumentation.
// This debug hook is intended for single-threaded benchmark/profiling runs.
func EnableRuntimeAudit(enabled bool) {
	runtimeAuditEnabled = enabled
}

// EnableGLREquivAudit toggles lightweight GLR equivalence attribution.
// This is intended for parser gap diagnostics and avoids the heavier survivor
// maps used by EnableRuntimeAudit.
func EnableGLREquivAudit(enabled bool) {
	runtimeEquivAuditEnabled = enabled
}

func (a *runtimeAudit) beginParse() {
	equivEnabled := runtimeEquivAuditEnabled
	if !runtimeAuditEnabled && !equivEnabled {
		a.reset()
		return
	}
	a.enabled = runtimeAuditEnabled
	a.equivEnabled = equivEnabled
	a.currentTokenGen = 0
	a.tokenActive = false
	a.currentEquivState = 0
	a.currentEquivStateSet = false
	a.currentGSSAllocated = 0
	a.currentGSSRetained = 0
	a.currentParentAllocated = 0
	a.currentParentRetained = 0
	a.currentLeafAllocated = 0
	a.currentLeafRetained = 0
	a.currentChildSlicesAllocated = 0
	a.currentChildSlicesRetained = 0
	a.currentChildPointersAllocated = 0
	a.currentChildPointersRetained = 0
	a.currentReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalGSSAllocated = 0
	a.totalGSSRetained = 0
	a.totalGSSDropped = 0
	a.totalParentAllocated = 0
	a.totalParentRetained = 0
	a.totalParentDropped = 0
	a.totalLeafAllocated = 0
	a.totalLeafRetained = 0
	a.totalLeafDropped = 0
	a.totalChildSlicesAllocated = 0
	a.totalChildSlicesRetained = 0
	a.totalChildSlicesDropped = 0
	a.totalChildPointersAllocated = 0
	a.totalChildPointersRetained = 0
	a.totalChildPointersDropped = 0
	a.totalReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesDropped = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersDropped = [reduceChildPathCount]uint64{}
	a.mergeStacksIn = 0
	a.mergeStacksOut = 0
	a.mergeSlotsUsed = 0
	a.globalCullStacksIn = 0
	a.globalCullStacksOut = 0
	a.stackEquivCalls = 0
	a.stackEquivTrue = 0
	a.stackEquivDepthMismatch = 0
	a.stackEquivHashMismatch = 0
	a.stackEquivStateMismatch = 0
	a.stackEquivPayloadMismatch = 0
	a.stackEquivEntryCompares = 0
	a.stackEquivStateMismatchDepthSum = 0
	a.stackEquivStateMismatchMaxDepth = 0
	a.stackEquivStateMismatchDepthBuckets = [stackEquivMismatchDepthBucketCount]uint64{}
	a.stackEquivPayloadMismatchDepthSum = 0
	a.stackEquivPayloadMismatchMaxDepth = 0
	a.stackEquivPayloadMismatchDepthBuckets = [stackEquivMismatchDepthBucketCount]uint64{}
	a.stackEquivPayloadHeaderSigDiff = 0
	a.stackEquivPayloadHeaderSigSame = 0
	a.stackEquivPayloadShallowSigDiff = 0
	a.stackEquivPayloadShallowSigSame = 0
	a.stackEquivPairKeyed = 0
	a.stackEquivPairUnkeyed = 0
	a.stackEquivPairRepeats = 0
	a.stackEquivPairRepeatTrue = 0
	a.stackEquivPairRepeatFalse = 0
	a.stackEquivPairRepeatMismatch = 0
	a.stackEquivPairStores = 0
	a.mergeHeaderEqTotal = 0
	a.mergeDeepTrue = 0
	a.mergeDeepFalse = 0
	a.mergeHeaderDeepDivergent = 0
	a.equivCacheLookups = 0
	a.equivCacheHits = 0
	a.equivCacheStores = 0
	a.equivCacheMisses = 0
	a.equivCacheTrueHits = 0
	a.equivCacheFalseHits = 0
	a.equivCacheEpochMisses = 0
	a.equivCacheKeyMisses = 0
	a.equivCacheVersionMisses = 0
	a.equivSkipError = 0
	a.equivSkipLeaf = 0
	a.equivSkipFieldMismatch = 0
	a.equivExactCalls = 0
	a.equivExactTrue = 0
	a.equivExactPointerTrue = 0
	a.equivExactNilMismatch = 0
	a.equivExactHeaderMismatch = 0
	a.equivExactChildMismatch = 0
	a.equivExactTerminalCalls = 0
	a.equivExactTerminalTrue = 0
	a.equivExactTerminalFalse = 0
	a.equivFrontierCalls = 0
	a.equivFrontierTrue = 0
	a.equivExactChildCompares = 0
	a.equivFrontierChildScans = 0
	a.equivFrontierCandidateCompares = 0
	if a.equivState != nil {
		clearRuntimeAuditEquivStateMap(a.equivState)
	}
	if a.equivEnabled {
		if a.equivPairs == nil {
			a.equivPairs = make(map[runtimeAuditStackEquivPairKey]bool)
		} else {
			clearRuntimeAuditStackEquivPairMap(a.equivPairs)
		}
	} else if a.equivPairs != nil {
		clearRuntimeAuditStackEquivPairMap(a.equivPairs)
	}
	if !a.enabled {
		return
	}
	if a.gssGen == nil {
		a.gssGen = make(map[*gssNode]uint32)
	} else {
		clearRuntimeAuditGSSMap(a.gssGen)
	}
	if a.nodeInfo == nil {
		a.nodeInfo = make(map[*Node]runtimeAuditNodeInfo)
	} else {
		clearRuntimeAuditNodeMap(a.nodeInfo)
	}
	if a.seenGSS == nil {
		a.seenGSS = make(map[*gssNode]struct{})
	} else {
		clearRuntimeAuditSeenGSSMap(a.seenGSS)
	}
	if a.seenNode == nil {
		a.seenNode = make(map[*Node]struct{})
	} else {
		clearRuntimeAuditSeenNodeMap(a.seenNode)
	}
}

func (a *runtimeAudit) reset() {
	a.enabled = false
	a.equivEnabled = false
	a.currentTokenGen = 0
	a.tokenActive = false
	a.currentEquivState = 0
	a.currentEquivStateSet = false
	a.currentGSSAllocated = 0
	a.currentGSSRetained = 0
	a.currentParentAllocated = 0
	a.currentParentRetained = 0
	a.currentLeafAllocated = 0
	a.currentLeafRetained = 0
	a.currentChildSlicesAllocated = 0
	a.currentChildSlicesRetained = 0
	a.currentChildPointersAllocated = 0
	a.currentChildPointersRetained = 0
	a.currentReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalGSSAllocated = 0
	a.totalGSSRetained = 0
	a.totalGSSDropped = 0
	a.totalParentAllocated = 0
	a.totalParentRetained = 0
	a.totalParentDropped = 0
	a.totalLeafAllocated = 0
	a.totalLeafRetained = 0
	a.totalLeafDropped = 0
	a.totalChildSlicesAllocated = 0
	a.totalChildSlicesRetained = 0
	a.totalChildSlicesDropped = 0
	a.totalChildPointersAllocated = 0
	a.totalChildPointersRetained = 0
	a.totalChildPointersDropped = 0
	a.totalReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildSlicesDropped = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersRetained = [reduceChildPathCount]uint64{}
	a.totalReduceChildPointersDropped = [reduceChildPathCount]uint64{}
	a.mergeStacksIn = 0
	a.mergeStacksOut = 0
	a.mergeSlotsUsed = 0
	a.globalCullStacksIn = 0
	a.globalCullStacksOut = 0
	a.stackEquivCalls = 0
	a.stackEquivTrue = 0
	a.stackEquivDepthMismatch = 0
	a.stackEquivHashMismatch = 0
	a.stackEquivStateMismatch = 0
	a.stackEquivPayloadMismatch = 0
	a.stackEquivEntryCompares = 0
	a.stackEquivStateMismatchDepthSum = 0
	a.stackEquivStateMismatchMaxDepth = 0
	a.stackEquivStateMismatchDepthBuckets = [stackEquivMismatchDepthBucketCount]uint64{}
	a.stackEquivPayloadMismatchDepthSum = 0
	a.stackEquivPayloadMismatchMaxDepth = 0
	a.stackEquivPayloadMismatchDepthBuckets = [stackEquivMismatchDepthBucketCount]uint64{}
	a.stackEquivPayloadHeaderSigDiff = 0
	a.stackEquivPayloadHeaderSigSame = 0
	a.stackEquivPayloadShallowSigDiff = 0
	a.stackEquivPayloadShallowSigSame = 0
	a.stackEquivPairKeyed = 0
	a.stackEquivPairUnkeyed = 0
	a.stackEquivPairRepeats = 0
	a.stackEquivPairRepeatTrue = 0
	a.stackEquivPairRepeatFalse = 0
	a.stackEquivPairRepeatMismatch = 0
	a.stackEquivPairStores = 0
	a.mergeHeaderEqTotal = 0
	a.mergeDeepTrue = 0
	a.mergeDeepFalse = 0
	a.mergeHeaderDeepDivergent = 0
	a.equivCacheLookups = 0
	a.equivCacheHits = 0
	a.equivCacheStores = 0
	a.equivCacheMisses = 0
	a.equivCacheTrueHits = 0
	a.equivCacheFalseHits = 0
	a.equivCacheEpochMisses = 0
	a.equivCacheKeyMisses = 0
	a.equivCacheVersionMisses = 0
	a.equivSkipError = 0
	a.equivSkipLeaf = 0
	a.equivSkipFieldMismatch = 0
	a.equivExactCalls = 0
	a.equivExactTrue = 0
	a.equivExactPointerTrue = 0
	a.equivExactNilMismatch = 0
	a.equivExactHeaderMismatch = 0
	a.equivExactChildMismatch = 0
	a.equivExactTerminalCalls = 0
	a.equivExactTerminalTrue = 0
	a.equivExactTerminalFalse = 0
	a.equivFrontierCalls = 0
	a.equivFrontierTrue = 0
	a.equivExactChildCompares = 0
	a.equivFrontierChildScans = 0
	a.equivFrontierCandidateCompares = 0
	if a.gssGen != nil {
		clearRuntimeAuditGSSMap(a.gssGen)
	}
	if a.nodeInfo != nil {
		clearRuntimeAuditNodeMap(a.nodeInfo)
	}
	if a.seenGSS != nil {
		clearRuntimeAuditSeenGSSMap(a.seenGSS)
	}
	if a.seenNode != nil {
		clearRuntimeAuditSeenNodeMap(a.seenNode)
	}
	if a.equivState != nil {
		clearRuntimeAuditEquivStateMap(a.equivState)
	}
	if a.equivPairs != nil {
		clearRuntimeAuditStackEquivPairMap(a.equivPairs)
	}
}

func (a *runtimeAudit) startToken(stacks []glrStack) {
	if a == nil || !a.enabled {
		return
	}
	if a.tokenActive {
		a.observeFrontier(stacks)
		a.finishToken()
	}
	a.currentTokenGen++
	a.tokenActive = true
	a.currentGSSAllocated = 0
	a.currentGSSRetained = 0
	a.currentParentAllocated = 0
	a.currentParentRetained = 0
	a.currentLeafAllocated = 0
	a.currentLeafRetained = 0
	a.currentChildSlicesAllocated = 0
	a.currentChildSlicesRetained = 0
	a.currentChildPointersAllocated = 0
	a.currentChildPointersRetained = 0
	a.currentReduceChildSlicesAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildSlicesRetained = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersAllocated = [reduceChildPathCount]uint64{}
	a.currentReduceChildPointersRetained = [reduceChildPathCount]uint64{}
}

func (a *runtimeAudit) finishParse(stacks []glrStack) {
	if a == nil || !a.enabled || !a.tokenActive {
		return
	}
	a.observeFrontier(stacks)
	a.finishToken()
}

func (a *runtimeAudit) finishToken() {
	if a == nil || !a.enabled || !a.tokenActive {
		return
	}
	a.totalGSSAllocated += a.currentGSSAllocated
	a.totalGSSRetained += a.currentGSSRetained
	if a.currentGSSAllocated > a.currentGSSRetained {
		a.totalGSSDropped += a.currentGSSAllocated - a.currentGSSRetained
	}
	a.totalParentAllocated += a.currentParentAllocated
	a.totalParentRetained += a.currentParentRetained
	if a.currentParentAllocated > a.currentParentRetained {
		a.totalParentDropped += a.currentParentAllocated - a.currentParentRetained
	}
	a.totalLeafAllocated += a.currentLeafAllocated
	a.totalLeafRetained += a.currentLeafRetained
	if a.currentLeafAllocated > a.currentLeafRetained {
		a.totalLeafDropped += a.currentLeafAllocated - a.currentLeafRetained
	}
	a.totalChildSlicesAllocated += a.currentChildSlicesAllocated
	a.totalChildSlicesRetained += a.currentChildSlicesRetained
	if a.currentChildSlicesAllocated > a.currentChildSlicesRetained {
		a.totalChildSlicesDropped += a.currentChildSlicesAllocated - a.currentChildSlicesRetained
	}
	a.totalChildPointersAllocated += a.currentChildPointersAllocated
	a.totalChildPointersRetained += a.currentChildPointersRetained
	if a.currentChildPointersAllocated > a.currentChildPointersRetained {
		a.totalChildPointersDropped += a.currentChildPointersAllocated - a.currentChildPointersRetained
	}
	for path := reduceChildPath(1); path < reduceChildPathCount; path++ {
		a.totalReduceChildSlicesAllocated[path] += a.currentReduceChildSlicesAllocated[path]
		a.totalReduceChildSlicesRetained[path] += a.currentReduceChildSlicesRetained[path]
		if a.currentReduceChildSlicesAllocated[path] > a.currentReduceChildSlicesRetained[path] {
			a.totalReduceChildSlicesDropped[path] += a.currentReduceChildSlicesAllocated[path] - a.currentReduceChildSlicesRetained[path]
		}
		a.totalReduceChildPointersAllocated[path] += a.currentReduceChildPointersAllocated[path]
		a.totalReduceChildPointersRetained[path] += a.currentReduceChildPointersRetained[path]
		if a.currentReduceChildPointersAllocated[path] > a.currentReduceChildPointersRetained[path] {
			a.totalReduceChildPointersDropped[path] += a.currentReduceChildPointersAllocated[path] - a.currentReduceChildPointersRetained[path]
		}
	}
	a.tokenActive = false
}

func (a *runtimeAudit) recordGSSAlloc(n *gssNode) {
	if a == nil || !a.enabled || !a.tokenActive || n == nil {
		return
	}
	a.currentGSSAllocated++
	a.gssGen[n] = a.currentTokenGen
}

func (a *runtimeAudit) recordNodeAlloc(n *Node, kind runtimeAuditNodeKind) {
	if a == nil || !a.enabled || !a.tokenActive || n == nil {
		return
	}
	switch kind {
	case runtimeAuditNodeKindParent:
		a.currentParentAllocated++
		if childCount := len(n.children); childCount > 0 {
			a.currentChildSlicesAllocated++
			a.currentChildPointersAllocated += uint64(childCount)
		}
	case runtimeAuditNodeKindLeaf:
		a.currentLeafAllocated++
	default:
		return
	}
	a.nodeInfo[n] = runtimeAuditNodeInfo{gen: a.currentTokenGen, kind: kind}
}

func (a *runtimeAudit) recordReduceParentChildPath(n *Node, path reduceChildPath, childCount int) {
	if a == nil || !a.enabled || !a.tokenActive || n == nil || !path.valid() || childCount <= 0 {
		return
	}
	info, ok := a.nodeInfo[n]
	if !ok || info.gen != a.currentTokenGen || info.kind != runtimeAuditNodeKindParent {
		return
	}
	info.reduceChildPath = path
	info.reduceChildPointers = uint32(childCount)
	a.nodeInfo[n] = info
	a.currentReduceChildSlicesAllocated[path]++
	a.currentReduceChildPointersAllocated[path] += uint64(childCount)
}

func (a *runtimeAudit) recordMerge(in, out, slots int) {
	if a == nil || !a.enabled {
		return
	}
	a.mergeStacksIn += uint64(in)
	a.mergeStacksOut += uint64(out)
	a.mergeSlotsUsed += uint64(slots)
}

func (a *runtimeAudit) recordGlobalCull(in, out int) {
	if a == nil || !a.enabled {
		return
	}
	a.globalCullStacksIn += uint64(in)
	a.globalCullStacksOut += uint64(out)
}

func (a *runtimeAudit) recordStackEquivCall() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivCalls++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivCalls++
	}
}

func (a *runtimeAudit) recordStackEquivTrue() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivTrue++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivTrue++
	}
}

func (a *runtimeAudit) recordStackEquivDepthMismatch() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivDepthMismatch++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivDepthMismatch++
	}
}

func (a *runtimeAudit) recordStackEquivHashMismatch() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivHashMismatch++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivHashMismatch++
	}
}

func (a *runtimeAudit) recordStackEquivStateMismatch() {
	a.recordStackEquivStateMismatchAt(-1)
}

func (a *runtimeAudit) recordStackEquivStateMismatchAt(depthFromTop int) {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivStateMismatch++
	recordStackEquivMismatchDepth(&a.stackEquivStateMismatchDepthSum, &a.stackEquivStateMismatchMaxDepth, &a.stackEquivStateMismatchDepthBuckets, depthFromTop)
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivStateMismatch++
		recordStackEquivMismatchDepth(&state.stackEquivStateMismatchDepthSum, &state.stackEquivStateMismatchMaxDepth, &state.stackEquivStateMismatchDepthBuckets, depthFromTop)
	}
}

func (a *runtimeAudit) recordStackEquivPayloadMismatch() {
	a.recordStackEquivPayloadMismatchAt(-1)
}

func (a *runtimeAudit) recordStackEquivPayloadMismatchAt(depthFromTop int) {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivPayloadMismatch++
	recordStackEquivMismatchDepth(&a.stackEquivPayloadMismatchDepthSum, &a.stackEquivPayloadMismatchMaxDepth, &a.stackEquivPayloadMismatchDepthBuckets, depthFromTop)
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivPayloadMismatch++
		recordStackEquivMismatchDepth(&state.stackEquivPayloadMismatchDepthSum, &state.stackEquivPayloadMismatchMaxDepth, &state.stackEquivPayloadMismatchDepthBuckets, depthFromTop)
	}
}

func (a *runtimeAudit) recordStackEquivPayloadMismatchSignatures(left, right stackEntry) {
	if a == nil || !a.equivEnabled {
		return
	}
	headerDiff := stackEntryExactHeaderSignature(left) != stackEntryExactHeaderSignature(right)
	shallowDiff := stackEntryExactShallowSignature(left) != stackEntryExactShallowSignature(right)
	if headerDiff {
		a.stackEquivPayloadHeaderSigDiff++
	} else {
		a.stackEquivPayloadHeaderSigSame++
	}
	if shallowDiff {
		a.stackEquivPayloadShallowSigDiff++
	} else {
		a.stackEquivPayloadShallowSigSame++
	}
	if state := a.currentEquivStateInfo(); state != nil {
		if headerDiff {
			state.stackEquivPayloadHeaderSigDiff++
		} else {
			state.stackEquivPayloadHeaderSigSame++
		}
		if shallowDiff {
			state.stackEquivPayloadShallowSigDiff++
		} else {
			state.stackEquivPayloadShallowSigSame++
		}
	}
}

func (a *runtimeAudit) recordStackEquivEntryCompare() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivEntryCompares++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivEntryCompares++
	}
}

func (a *runtimeAudit) recordStackEquivPairUnkeyed() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.stackEquivPairUnkeyed++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivPairUnkeyed++
	}
}

func (a *runtimeAudit) lookupStackEquivPair(key runtimeAuditStackEquivPairKey) (bool, bool) {
	if a == nil || !a.equivEnabled {
		return false, false
	}
	a.stackEquivPairKeyed++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivPairKeyed++
	}
	if a.equivPairs == nil {
		a.equivPairs = make(map[runtimeAuditStackEquivPairKey]bool)
		return false, false
	}
	previous, ok := a.equivPairs[key]
	if !ok {
		return false, false
	}
	a.stackEquivPairRepeats++
	if previous {
		a.stackEquivPairRepeatTrue++
	} else {
		a.stackEquivPairRepeatFalse++
	}
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivPairRepeats++
		if previous {
			state.stackEquivPairRepeatTrue++
		} else {
			state.stackEquivPairRepeatFalse++
		}
	}
	return previous, true
}

func (a *runtimeAudit) storeStackEquivPair(key runtimeAuditStackEquivPairKey, previous bool, hit bool, result bool) {
	if a == nil || !a.equivEnabled {
		return
	}
	if hit {
		if previous != result {
			a.stackEquivPairRepeatMismatch++
			if state := a.currentEquivStateInfo(); state != nil {
				state.stackEquivPairRepeatMismatch++
			}
		}
		return
	}
	if a.equivPairs == nil {
		a.equivPairs = make(map[runtimeAuditStackEquivPairKey]bool)
	}
	a.equivPairs[key] = result
	a.stackEquivPairStores++
	if state := a.currentEquivStateInfo(); state != nil {
		state.stackEquivPairStores++
	}
}

func recordStackEquivMismatchDepth(sum *uint64, maxDepth *uint32, buckets *[stackEquivMismatchDepthBucketCount]uint64, depthFromTop int) {
	if depthFromTop < 0 {
		return
	}
	depth := uint64(depthFromTop)
	*sum += depth
	if depth > uint64(*maxDepth) {
		*maxDepth = uint32(depth)
	}
	buckets[stackEquivMismatchDepthBucket(depthFromTop)]++
}

func stackEquivMismatchDepthBucket(depthFromTop int) int {
	switch {
	case depthFromTop <= 0:
		return 0
	case depthFromTop == 1:
		return 1
	case depthFromTop == 2:
		return 2
	case depthFromTop == 3:
		return 3
	case depthFromTop < 8:
		return 4
	case depthFromTop < 16:
		return 5
	case depthFromTop < 32:
		return 6
	default:
		return 7
	}
}

func (a *runtimeAudit) recordEquivCacheLookup() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheLookups++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivCacheLookups++
	}
}

func (a *runtimeAudit) recordEquivCacheHit() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheHits++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivCacheHits++
	}
}

func (a *runtimeAudit) recordEquivCacheResultHit(result bool) {
	if a == nil || !a.equivEnabled {
		return
	}
	if result {
		a.equivCacheTrueHits++
		if state := a.currentEquivStateInfo(); state != nil {
			state.equivCacheTrueHits++
		}
		return
	}
	a.equivCacheFalseHits++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivCacheFalseHits++
	}
}

func (a *runtimeAudit) recordEquivCacheStore() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheStores++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivCacheStores++
	}
}

func (a *runtimeAudit) recordEquivCacheEpochMiss() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheMisses++
	a.equivCacheEpochMisses++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivCacheMisses++
		state.equivCacheEpochMisses++
	}
}

func (a *runtimeAudit) recordEquivCacheKeyMiss() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheMisses++
	a.equivCacheKeyMisses++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivCacheMisses++
		state.equivCacheKeyMisses++
	}
}

func (a *runtimeAudit) recordEquivCacheVersionMiss() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivCacheMisses++
	a.equivCacheVersionMisses++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivCacheMisses++
		state.equivCacheVersionMisses++
	}
}

func (a *runtimeAudit) recordEquivSkipError() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivSkipError++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivSkipError++
	}
}

func (a *runtimeAudit) recordEquivSkipLeaf() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivSkipLeaf++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivSkipLeaf++
	}
}

func (a *runtimeAudit) recordEquivSkipFieldMismatch() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivSkipFieldMismatch++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivSkipFieldMismatch++
	}
}

func (a *runtimeAudit) recordEquivExactCall() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactCalls++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactCalls++
	}
}

func (a *runtimeAudit) recordEquivExactTrue() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactTrue++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactTrue++
	}
}

func (a *runtimeAudit) recordEquivExactPointerTrue() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactPointerTrue++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactPointerTrue++
	}
}

func (a *runtimeAudit) recordEquivExactNilMismatch() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactNilMismatch++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactNilMismatch++
	}
}

func (a *runtimeAudit) recordEquivExactHeaderMismatch() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactHeaderMismatch++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactHeaderMismatch++
	}
}

func (a *runtimeAudit) recordEquivExactChildMismatch() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactChildMismatch++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactChildMismatch++
	}
}

func (a *runtimeAudit) recordEquivExactTerminalCall() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactTerminalCalls++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactTerminalCalls++
	}
}

func (a *runtimeAudit) recordEquivExactTerminalTrue() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactTerminalTrue++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactTerminalTrue++
	}
}

func (a *runtimeAudit) recordEquivExactTerminalFalse() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactTerminalFalse++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactTerminalFalse++
	}
}

func (a *runtimeAudit) recordEquivFrontierCall() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivFrontierCalls++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivFrontierCalls++
	}
}

func (a *runtimeAudit) recordEquivFrontierTrue() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivFrontierTrue++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivFrontierTrue++
	}
}

func (a *runtimeAudit) recordEquivExactChildCompare() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivExactChildCompares++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivExactChildCompares++
	}
}

func (a *runtimeAudit) recordEquivFrontierChildScan() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivFrontierChildScans++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivFrontierChildScans++
	}
}

func (a *runtimeAudit) recordEquivFrontierCandidateCompare() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.equivFrontierCandidateCompares++
	if state := a.currentEquivStateInfo(); state != nil {
		state.equivFrontierCandidateCompares++
	}
}

func (a *runtimeAudit) setEquivState(state StateID) {
	if a == nil || !a.equivEnabled {
		return
	}
	a.currentEquivState = state
	a.currentEquivStateSet = true
}

func (a *runtimeAudit) clearEquivState() {
	if a == nil || !a.equivEnabled {
		return
	}
	a.currentEquivState = 0
	a.currentEquivStateSet = false
}

func (a *runtimeAudit) currentEquivStateInfo() *runtimeAuditEquivStateInfo {
	if a == nil || !a.equivEnabled || !a.currentEquivStateSet {
		return nil
	}
	if a.equivState == nil {
		a.equivState = make(map[StateID]*runtimeAuditEquivStateInfo)
	}
	info := a.equivState[a.currentEquivState]
	if info == nil {
		info = &runtimeAuditEquivStateInfo{state: a.currentEquivState}
		a.equivState[a.currentEquivState] = info
	}
	return info
}

func (a *runtimeAudit) equivStateStats() []ParseEquivStateRuntime {
	if a == nil || len(a.equivState) == 0 {
		return nil
	}
	out := make([]ParseEquivStateRuntime, 0, len(a.equivState))
	for _, info := range a.equivState {
		out = append(out, ParseEquivStateRuntime{
			State:                                 info.state,
			StackEquivCalls:                       info.stackEquivCalls,
			StackEquivTrue:                        info.stackEquivTrue,
			StackEquivDepthMismatch:               info.stackEquivDepthMismatch,
			StackEquivHashMismatch:                info.stackEquivHashMismatch,
			StackEquivStateMismatch:               info.stackEquivStateMismatch,
			StackEquivPayloadMismatch:             info.stackEquivPayloadMismatch,
			StackEquivEntryCompares:               info.stackEquivEntryCompares,
			StackEquivStateMismatchDepthSum:       info.stackEquivStateMismatchDepthSum,
			StackEquivStateMismatchMaxDepth:       info.stackEquivStateMismatchMaxDepth,
			StackEquivStateMismatchDepthBuckets:   info.stackEquivStateMismatchDepthBuckets,
			StackEquivPayloadMismatchDepthSum:     info.stackEquivPayloadMismatchDepthSum,
			StackEquivPayloadMismatchMaxDepth:     info.stackEquivPayloadMismatchMaxDepth,
			StackEquivPayloadMismatchDepthBuckets: info.stackEquivPayloadMismatchDepthBuckets,
			StackEquivPayloadHeaderSigDiff:        info.stackEquivPayloadHeaderSigDiff,
			StackEquivPayloadHeaderSigSame:        info.stackEquivPayloadHeaderSigSame,
			StackEquivPayloadShallowSigDiff:       info.stackEquivPayloadShallowSigDiff,
			StackEquivPayloadShallowSigSame:       info.stackEquivPayloadShallowSigSame,
			StackEquivPairKeyed:                   info.stackEquivPairKeyed,
			StackEquivPairUnkeyed:                 info.stackEquivPairUnkeyed,
			StackEquivPairRepeats:                 info.stackEquivPairRepeats,
			StackEquivPairRepeatTrue:              info.stackEquivPairRepeatTrue,
			StackEquivPairRepeatFalse:             info.stackEquivPairRepeatFalse,
			StackEquivPairRepeatMismatch:          info.stackEquivPairRepeatMismatch,
			StackEquivPairStores:                  info.stackEquivPairStores,
			EquivCacheLookups:                     info.equivCacheLookups,
			EquivCacheHits:                        info.equivCacheHits,
			EquivCacheStores:                      info.equivCacheStores,
			EquivCacheMisses:                      info.equivCacheMisses,
			EquivCacheTrueHits:                    info.equivCacheTrueHits,
			EquivCacheFalseHits:                   info.equivCacheFalseHits,
			EquivCacheEpochMisses:                 info.equivCacheEpochMisses,
			EquivCacheKeyMisses:                   info.equivCacheKeyMisses,
			EquivCacheVersionMisses:               info.equivCacheVersionMisses,
			EquivSkipError:                        info.equivSkipError,
			EquivSkipLeaf:                         info.equivSkipLeaf,
			EquivSkipFieldMismatch:                info.equivSkipFieldMismatch,
			EquivExactCalls:                       info.equivExactCalls,
			EquivExactTrue:                        info.equivExactTrue,
			EquivExactPointerTrue:                 info.equivExactPointerTrue,
			EquivExactNilMismatch:                 info.equivExactNilMismatch,
			EquivExactHeaderMismatch:              info.equivExactHeaderMismatch,
			EquivExactChildMismatch:               info.equivExactChildMismatch,
			EquivExactTerminalCalls:               info.equivExactTerminalCalls,
			EquivExactTerminalTrue:                info.equivExactTerminalTrue,
			EquivExactTerminalFalse:               info.equivExactTerminalFalse,
			EquivFrontierCalls:                    info.equivFrontierCalls,
			EquivFrontierTrue:                     info.equivFrontierTrue,
			EquivExactChildCompares:               info.equivExactChildCompares,
			EquivFrontierChildScans:               info.equivFrontierChildScans,
			EquivFrontierCandidateCompares:        info.equivFrontierCandidateCompares,
		})
	}
	return out
}

func (a *runtimeAudit) observeFrontier(stacks []glrStack) {
	if a == nil || !a.enabled || !a.tokenActive {
		return
	}
	clearRuntimeAuditSeenGSSMap(a.seenGSS)
	clearRuntimeAuditSeenNodeMap(a.seenNode)
	var gssRetained uint64
	var parentRetained uint64
	var leafRetained uint64
	var childSlicesRetained uint64
	var childPointersRetained uint64
	for i := range stacks {
		if stacks[i].dead {
			continue
		}
		if stacks[i].gss.head != nil {
			a.observeGSSChain(stacks[i].gss.head, &gssRetained, &parentRetained, &leafRetained, &childSlicesRetained, &childPointersRetained)
			continue
		}
		a.observeEntries(stacks[i].entries, &parentRetained, &leafRetained, &childSlicesRetained, &childPointersRetained)
	}
	a.currentGSSRetained = gssRetained
	a.currentParentRetained = parentRetained
	a.currentLeafRetained = leafRetained
	a.currentChildSlicesRetained = childSlicesRetained
	a.currentChildPointersRetained = childPointersRetained
}

func (a *runtimeAudit) observeGSSChain(head *gssNode, gssRetained, parentRetained, leafRetained, childSlicesRetained, childPointersRetained *uint64) {
	if a == nil || head == nil {
		return
	}
	for n := head; n != nil; n = n.prev {
		gen, ok := a.gssGen[n]
		if !ok || gen != a.currentTokenGen {
			break
		}
		if _, seen := a.seenGSS[n]; !seen {
			a.seenGSS[n] = struct{}{}
			*gssRetained = *gssRetained + 1
		}
		a.observeNode(stackEntryNode(n.entry), parentRetained, leafRetained, childSlicesRetained, childPointersRetained)
	}
}

func (a *runtimeAudit) observeEntries(entries []stackEntry, parentRetained, leafRetained, childSlicesRetained, childPointersRetained *uint64) {
	if a == nil || len(entries) == 0 {
		return
	}
	for i := len(entries) - 1; i >= 0; i-- {
		node := stackEntryNode(entries[i])
		if node == nil && stackEntryNoTreeNode(entries[i]) != nil {
			continue
		}
		info, ok := a.nodeInfo[node]
		if !ok || info.gen != a.currentTokenGen {
			break
		}
		a.observeNode(node, parentRetained, leafRetained, childSlicesRetained, childPointersRetained)
	}
}

func (a *runtimeAudit) observeNode(node *Node, parentRetained, leafRetained, childSlicesRetained, childPointersRetained *uint64) {
	if a == nil || node == nil {
		return
	}
	info, ok := a.nodeInfo[node]
	if !ok || info.gen != a.currentTokenGen {
		return
	}
	if _, seen := a.seenNode[node]; seen {
		return
	}
	a.seenNode[node] = struct{}{}
	switch info.kind {
	case runtimeAuditNodeKindParent:
		*parentRetained = *parentRetained + 1
		if childCount := len(node.children); childCount > 0 {
			*childSlicesRetained = *childSlicesRetained + 1
			*childPointersRetained = *childPointersRetained + uint64(childCount)
		}
		if info.reduceChildPath.valid() && info.reduceChildPointers > 0 {
			a.currentReduceChildSlicesRetained[info.reduceChildPath]++
			a.currentReduceChildPointersRetained[info.reduceChildPath] += uint64(info.reduceChildPointers)
		}
	case runtimeAuditNodeKindLeaf:
		*leafRetained = *leafRetained + 1
	}
}

func (a *runtimeAudit) reduceChildPathRuntime(path reduceChildPath) ReduceChildPathRuntime {
	if a == nil || !path.valid() {
		return ReduceChildPathRuntime{}
	}
	return ReduceChildPathRuntime{
		SlicesAllocated:   a.totalReduceChildSlicesAllocated[path],
		SlicesRetained:    a.totalReduceChildSlicesRetained[path],
		SlicesDropped:     a.totalReduceChildSlicesDropped[path],
		PointersAllocated: a.totalReduceChildPointersAllocated[path],
		PointersRetained:  a.totalReduceChildPointersRetained[path],
		PointersDropped:   a.totalReduceChildPointersDropped[path],
	}
}

func clearRuntimeAuditGSSMap(m map[*gssNode]uint32) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditNodeMap(m map[*Node]runtimeAuditNodeInfo) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditSeenGSSMap(m map[*gssNode]struct{}) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditSeenNodeMap(m map[*Node]struct{}) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditEquivStateMap(m map[StateID]*runtimeAuditEquivStateInfo) {
	for k := range m {
		delete(m, k)
	}
}

func clearRuntimeAuditStackEquivPairMap(m map[runtimeAuditStackEquivPairKey]bool) {
	for k := range m {
		delete(m, k)
	}
}

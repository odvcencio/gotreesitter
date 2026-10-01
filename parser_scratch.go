package gotreesitter

import (
	"sync"
	"unsafe"
)

// Keep enough compatibility traversal storage for ordinary generated files
// without retaining multi-megabyte backing arrays after unusually wide trees.
const maxRetainedGoCompatFrames = 16 * 1024

type parserScratch struct {
	reuseState               parseReuseState
	advancePages             []*[syntheticRootReplayAdvancePageFrames]syntheticRootReplayFrame
	closePages               []*[syntheticRootReplayClosePageFrames]syntheticRootReplayFrame
	advancePageUsed          uint16
	closePageUsed            uint16
	advanceFrames            uint32
	closeFrames              uint32
	merge                    glrMergeScratch
	entries                  glrEntryScratch
	gss                      gssScratch
	audit                    runtimeAudit
	tmpEntries               []stackEntry
	glrStates                []StateID
	nodeLinks                []*Node
	stackPick                []int
	stackKeep                []bool
	stackCull                []stackCullKey
	goCompatFrames           []goCompatSubtreeFrame
	reduce                   reduceBuildScratch
	transientChildren        transientChildScratch
	transientParents         transientParentScratch
	transientCheckpointBytes int64
	transientCheckpoints     uint64
	relexSnapshotBuffer      []byte
	relexSnapshotInUse       bool
	trackChildErrors         bool
	materializeStopPollCount uint32
	budgetBytes              int64
	budgetBaselineBytes      int64
	gssBaselineBytes         int64
}

var parserScratchPool = sync.Pool{
	New: func() any {
		return &parserScratch{}
	},
}

func acquireParserScratch() *parserScratch {
	return parserScratchPool.Get().(*parserScratch)
}

func (s *parserScratch) setBudget(bytes int64) {
	if s == nil {
		return
	}
	s.budgetBytes = bytes
	s.budgetBaselineBytes = s.allocatedBytes()
	s.gssBaselineBytes = s.gss.allocatedBytes
	s.gss.summaryBudgetOwner = s
	s.merge.budgetBytes = bytes
}

func (s *parserScratch) clearBudget() {
	if s == nil {
		return
	}
	s.budgetBytes = 0
	s.budgetBaselineBytes = 0
	s.gssBaselineBytes = 0
	s.merge.budgetBytes = 0
	s.gss.summaryBudgetOwner = nil
}

func (s *parserScratch) summaryBudgetState() (int64, int64, int64) {
	if s == nil {
		return 0, 0, 0
	}
	return s.budgetBytes, s.budgetBaselineBytes, s.allocatedBytes()
}

func (s *parserScratch) allocatedBytes() int64 {
	if s == nil {
		return 0
	}
	return int64(cap(s.reuseState.arenaRefs)+cap(s.reuseState.arenaWalk))*int64(unsafe.Sizeof(uintptr(0))) + s.entries.allocatedBytes + s.gss.allocatedBytes + s.merge.allocatedBytes() + s.transientChildren.allocatedBytes + s.transientParents.allocatedBytes + int64(len(s.advancePages))*int64(unsafe.Sizeof([syntheticRootReplayAdvancePageFrames]syntheticRootReplayFrame{})) + int64(len(s.closePages))*int64(unsafe.Sizeof([syntheticRootReplayClosePageFrames]syntheticRootReplayFrame{}))
}

func (s *parserScratch) resetReplayPages() {
	if s == nil {
		return
	}
	if cap(s.advancePages) > 1 {
		clear(s.advancePages[:cap(s.advancePages)][1:])
	}
	if cap(s.closePages) > 1 {
		clear(s.closePages[:cap(s.closePages)][1:])
	}
	if len(s.advancePages) > 0 {
		s.advancePages = s.advancePages[:1:1]
	}
	if len(s.closePages) > 0 {
		s.closePages = s.closePages[:1:1]
	}
	s.advancePageUsed, s.closePageUsed = 0, 0
	s.advanceFrames, s.closeFrames = 0, 0
}

func (s *parserScratch) budgetExhausted() bool {
	if s == nil || s.budgetBytes <= 0 {
		return false
	}
	used := s.allocatedBytes() - s.budgetBaselineBytes
	if used < 0 {
		used = 0
	}
	return used >= s.budgetBytes
}

func (s *parserScratch) snapshotDFARelexState(d *dfaTokenSource) (dfaRelexSnapshot, bool) {
	if s == nil || s.relexSnapshotInUse {
		return d.snapshotRelexState(), false
	}
	s.relexSnapshotInUse = true
	snapshot, buf := d.snapshotRelexStateWithExternalBuffer(s.relexSnapshotBuffer)
	s.relexSnapshotBuffer = buf[:0]
	return snapshot, true
}

func (s *parserScratch) releaseDFARelexSnapshot(retained bool) {
	if s == nil || !retained {
		return
	}
	if cap(s.relexSnapshotBuffer) > 0 {
		clear(s.relexSnapshotBuffer[:cap(s.relexSnapshotBuffer)])
	}
	if cap(s.relexSnapshotBuffer) == externalScannerSerializationBufferSize {
		s.relexSnapshotBuffer = s.relexSnapshotBuffer[:0]
	} else {
		s.relexSnapshotBuffer = nil
	}
	s.relexSnapshotInUse = false
}

func releaseParserScratch(s *parserScratch, skipGSSClear bool) {
	if s == nil {
		return
	}
	s.merge.reset()
	if cap(s.tmpEntries) > 0 {
		buf := s.tmpEntries[:cap(s.tmpEntries)]
		clear(buf)
		if cap(buf) > maxRetainedStackEntryCap {
			s.tmpEntries = nil
		} else {
			s.tmpEntries = buf[:0]
		}
	}
	if cap(s.glrStates) > maxGLRStacks {
		s.glrStates = nil
	} else if len(s.glrStates) > 0 {
		s.glrStates = s.glrStates[:0]
	}
	const maxRetainedNodeLinkStack = 256 * 1024
	s.reuseState.reusedAny = false
	if cap(s.reuseState.arenaWalk) > maxRetainedNodeLinkStack {
		s.reuseState.arenaWalk = nil
	} else {
		clear(s.reuseState.arenaWalk[:cap(s.reuseState.arenaWalk)])
		s.reuseState.arenaWalk = s.reuseState.arenaWalk[:0]
	}
	if cap(s.reuseState.arenaRefs) > maxGLRStacks {
		s.reuseState.arenaRefs = nil
	} else {
		clear(s.reuseState.arenaRefs[:cap(s.reuseState.arenaRefs)])
		s.reuseState.arenaRefs = s.reuseState.arenaRefs[:0]
	}

	if cap(s.nodeLinks) > maxRetainedNodeLinkStack {
		s.nodeLinks = nil
	} else if cap(s.nodeLinks) > 0 {
		// Clear the full capacity, not just [:len]. wireParentLinksWithScratch
		// returns the scratch slice as stack[:0], so len=0 but cap>0 with live
		// *Node pointers in the backing array. Clearing [:len] is a no-op here.
		clear(s.nodeLinks[:cap(s.nodeLinks)])
		s.nodeLinks = s.nodeLinks[:0]
	}
	const maxRetainedStackCullScratch = 256
	if cap(s.stackPick) > maxRetainedStackCullScratch {
		s.stackPick = nil
	} else if len(s.stackPick) > 0 {
		s.stackPick = s.stackPick[:0]
	}
	if cap(s.stackKeep) > maxRetainedStackCullScratch {
		s.stackKeep = nil
	} else if len(s.stackKeep) > 0 {
		s.stackKeep = s.stackKeep[:0]
	}
	if cap(s.stackCull) > maxRetainedStackCullScratch {
		s.stackCull = nil
	} else if len(s.stackCull) > 0 {
		s.stackCull = s.stackCull[:0]
	}
	if cap(s.goCompatFrames) > maxRetainedGoCompatFrames {
		s.goCompatFrames = nil
	} else if cap(s.goCompatFrames) > 0 {
		clear(s.goCompatFrames[:cap(s.goCompatFrames)])
		s.goCompatFrames = s.goCompatFrames[:0]
	}
	const maxRetainedReduceBuildScratch = 256 * 1024
	if cap(s.reduce.nodes) > maxRetainedReduceBuildScratch {
		s.reduce.nodes = nil
		s.reduce.fieldIDs = nil
		s.reduce.fieldSources = nil
		s.reduce.repeatStamp = nil
		s.reduce.repeatCount = nil
		s.reduce.repeatSource = nil
		s.reduce.repeatTouched = nil
		s.reduce.trackFields = false
		s.reduce.repeatEpoch = 0
		s.reduce.transientParents = nil
		s.reduce.transientChildren = nil
	} else {
		s.reduce.reset()
		s.reduce.transientParents = nil
		s.reduce.transientChildren = nil
	}
	s.transientChildren.resetForRelease()
	s.transientParents.resetForRelease()
	s.transientCheckpointBytes = 0
	s.transientCheckpoints = 0
	if cap(s.relexSnapshotBuffer) > 0 {
		clear(s.relexSnapshotBuffer[:cap(s.relexSnapshotBuffer)])
	}
	if cap(s.relexSnapshotBuffer) != externalScannerSerializationBufferSize {
		s.relexSnapshotBuffer = nil
	} else {
		s.relexSnapshotBuffer = s.relexSnapshotBuffer[:0]
	}
	s.relexSnapshotInUse = false
	s.trackChildErrors = false
	s.materializeStopPollCount = 0
	s.resetReplayPages()
	s.entries.reset()
	s.gss.skipClear = skipGSSClear
	s.gss.audit = nil
	s.gss.reset()
	s.audit.reset()
	s.clearBudget()
	parserScratchPool.Put(s)
}

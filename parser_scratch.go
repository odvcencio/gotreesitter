package gotreesitter

import "sync"

type parserScratch struct {
	merge       glrMergeScratch
	entries     glrEntryScratch
	gss         gssScratch
	audit       runtimeAudit
	tmpEntries  []stackEntry
	glrStates   []StateID
	nodeLinks   []*Node
	stackPick   []int
	stackKeep   []bool
	stackCull   []stackCullKey
	stateKeep   []StateID
	reduce      reduceBuildScratch
	budgetBytes int64
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
}

func (s *parserScratch) clearBudget() {
	if s == nil {
		return
	}
	s.budgetBytes = 0
}

func (s *parserScratch) allocatedBytes() int64 {
	if s == nil {
		return 0
	}
	return s.entries.allocatedBytes + s.gss.allocatedBytes
}

func (s *parserScratch) budgetExhausted() bool {
	if s == nil || s.budgetBytes <= 0 {
		return false
	}
	return s.allocatedBytes() >= s.budgetBytes
}

func releaseParserScratch(s *parserScratch, skipGSSClear bool) {
	if s == nil {
		return
	}
	if len(s.merge.result) > 0 {
		clear(s.merge.result)
	}
	s.merge.result = s.merge.result[:0]
	if len(s.merge.slots) > 0 {
		s.merge.slots = s.merge.slots[:0]
	}
	s.merge.perKeyCap = 0
	s.merge.audit = nil
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
	if cap(s.stateKeep) > maxRetainedStackCullScratch {
		s.stateKeep = nil
	} else if len(s.stateKeep) > 0 {
		s.stateKeep = s.stateKeep[:0]
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
	} else {
		s.reduce.reset()
	}
	s.entries.reset()
	s.gss.skipClear = skipGSSClear
	s.gss.audit = nil
	s.gss.reset()
	s.audit.reset()
	s.clearBudget()
	parserScratchPool.Put(s)
}

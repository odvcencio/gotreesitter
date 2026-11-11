package gotreesitter

import (
	"sync"
	"unsafe"
)

// Keep enough compatibility traversal storage for ordinary generated files
// without retaining multi-megabyte backing arrays after unusually wide trees.
const maxRetainedGoCompatFrames = 16 * 1024

// Keep large parses from handing their working set to small parses. Heavy
// small parses use separate pools, so matching parsers can reuse their working
// storage without charging it to unrelated small parses.
const largeParserScratchSourceBytes = 64 << 10
const heavySmallParserScratchBytes = 16 << 20

type parserScratch struct {
	largePool                bool
	sourceBytes              int
	heavyGrammar             scratchGrammarKey
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

var largeParserScratchPool sync.Pool
var heavySmallParserScratchPool sync.Pool

// Scratch is reset storage, not a parse result. Exact blob identity is useful
// where available; geometry provides a bounded reuse preference for custom
// grammars without retaining their Language or depending on its lifetime.
type scratchGrammarKey struct {
	hash     [32]byte
	geometry [6]uint32
	known    bool
	scanner  bool
}

func scratchKeyForLanguage(lang *Language) (scratchGrammarKey, bool) {
	if lang == nil {
		return scratchGrammarKey{}, false
	}
	key := scratchGrammarKey{scanner: lang.ExternalScanner != nil}
	key.hash, key.known = lang.GrammarBlobSHA256()
	if !key.known {
		key.geometry = [6]uint32{lang.StateCount, lang.SymbolCount, lang.TokenCount, lang.ExternalTokenCount, uint32(len(lang.LexStates)), uint32(len(lang.ParseActions))}
	}
	return key, true
}

func acquireParserScratch() *parserScratch {
	return parserScratchPool.Get().(*parserScratch)
}

func parserHeavySmallScratchFits(s *parserScratch, p *Parser, sourceBytes int) bool {
	if s == nil || p == nil || p.language == nil || s.sourceBytes <= 0 {
		return false
	}
	if sourceBytes < (s.sourceBytes+1)/2 || sourceBytes > s.sourceBytes*2 {
		return false
	}
	key, ok := scratchKeyForLanguage(p.language)
	return ok && key == s.heavyGrammar
}

func reclaimLargeScratchForSmallParse(s *parserScratch) {
	if s != nil && s.allocatedBytes() > 32<<20 {
		s.transientParents = transientParentScratch{}
		s.transientChildren = transientChildScratch{}
	}
}

func acquireParserScratchForSource(sourceBytes int, parser *Parser) *parserScratch {
	if sourceBytes < tinyParserScratchSourceBytes {
		return acquireTinyParserScratch(sourceBytes, parser)
	}
	var s *parserScratch
	if sourceBytes >= largeParserScratchSourceBytes {
		if cached := largeParserScratchPool.Get(); cached != nil {
			s = cached.(*parserScratch)
		} else {
			s = &parserScratch{}
		}
		s.largePool = true
	} else {
		// A small parse can coexist with an idle large scratch. Keep the
		// large parse's GSS and merge caches, but reclaim large transient
		// materialization slabs so both classes do not retain their peaks.
		// Pool.Get gives exclusive ownership of this idle scratch; release
		// already cleared the slabs before putting it in the pool.
		if cached := largeParserScratchPool.Get(); cached != nil {
			large := cached.(*parserScratch)
			reclaimLargeScratchForSmallParse(large)
			largeParserScratchPool.Put(large)
		}
		if parser != nil {
			if _, ok := scratchKeyForLanguage(parser.language); ok {
				if cached := heavySmallParserScratchPool.Get(); cached != nil {
					heavy := cached.(*parserScratch)
					if parserHeavySmallScratchFits(heavy, parser, sourceBytes) {
						s = heavy
					}
					// A mismatched idle candidate is discarded. Putting it back
					// ahead of a new heavy parse can make this bucket write-only.
				}
			}
		}
		if s == nil {
			s = acquireParserScratch()
		}
	}
	s.sourceBytes = sourceBytes
	return s
}

// Hand-out bound for tiny sources. A parse of fewer than
// tinyParserScratchSourceBytes bytes (the node count of one default GSS slab;
// a policy cutoff, not a derived memory limit) takes a pooled object only if
// the object's accounted capacity (allocatedBytes: entries, GSS, merge,
// transient slabs and replay pages) plus tinyParserScratchGrowthBytes is at
// most tinyParserScratchCeilingBytes, the scratch that roottest/parse
// TestParseScratchIsolationSmallLargeSmall allows a tiny parse to hold. An
// object that does not fit is neither used nor discarded: it returns to the
// pool it came from after the parse has its scratch, and at most
// maxScratchHandOutTries shared-pool objects are tried before the parse
// allocates a fresh one.
//
// This bounds accounted capacity at hand-out for that size class: at most
// tinyParserScratchCeilingBytes - tinyParserScratchGrowthBytes (7 MiB). It does
// not bound memory outside allocatedBytes (nodeLinks, tmpEntries, reduction
// scratch), what a parse grows to after it has its scratch (the memory budgets
// govern that growth; one fresh full-parse GSS slab alone is 2 MiB on 64-bit
// targets), or sources of tinyParserScratchSourceBytes and above, which take
// any object through the unchanged path. What each pool keeps at release is
// unchanged.
//
// Two cases lose reuse compared with taking the next object unchecked:
//   - A burst of tiny parses that meets oversized shared objects allocates
//     fresh scratch where an unchecked hand-out would reuse them, because each
//     parse tries at most maxScratchHandOutTries objects.
//   - A tiny input that keeps more than 7 MiB of accounted capacity after a
//     parse is rejected for its own next parse and rebuilds its scratch each
//     time.
const (
	tinyParserScratchSourceBytes  = 4 << 10
	tinyParserScratchCeilingBytes = 8 << 20
	tinyParserScratchGrowthBytes  = 1 << 20
	maxScratchHandOutTries        = 3
)

// scratchPool is the part of sync.Pool that tiny hand-out uses.
type scratchPool interface {
	Get() any
	Put(any)
}

type tinyScratchPoolSet struct {
	shared, large, heavy scratchPool
}

// tinyScratchPools is read only by acquireTinyParserScratch. Tests replace it
// with deterministic stacks; larger sources and release use the pool variables
// directly.
var tinyScratchPools = &tinyScratchPoolSet{
	shared: &parserScratchPool,
	large:  &largeParserScratchPool,
	heavy:  &heavySmallParserScratchPool,
}

// parserScratchFitsHandOut reports whether a pooled object may be given to a
// parse of sourceBytes: always for sources of tinyParserScratchSourceBytes and
// above, otherwise only within the tiny hand-out bound.
func parserScratchFitsHandOut(s *parserScratch, sourceBytes int) bool {
	if sourceBytes >= tinyParserScratchSourceBytes {
		return true
	}
	return s.allocatedBytes()+tinyParserScratchGrowthBytes <= tinyParserScratchCeilingBytes
}

// acquireTinyParserScratch is acquireParserScratchForSource for sources below
// tinyParserScratchSourceBytes, with the hand-out bound applied to every
// candidate. Objects that match but exceed the bound are held locally and go
// back to the pool they came from once this parse has its scratch.
//
//go:noinline
func acquireTinyParserScratch(sourceBytes int, parser *Parser) *parserScratch {
	pools := tinyScratchPools
	// Reclaim the transient slabs of an idle large scratch, as for any small
	// parse; the large object is not borrowed.
	if cached := pools.large.Get(); cached != nil {
		large := cached.(*parserScratch)
		reclaimLargeScratchForSmallParse(large)
		pools.large.Put(large)
	}
	var s *parserScratch
	var rejected [maxScratchHandOutTries]*parserScratch
	rejectedCount := 0
	var rejectedHeavy *parserScratch
	if parser != nil {
		if _, ok := scratchKeyForLanguage(parser.language); ok {
			if cached := pools.heavy.Get(); cached != nil {
				heavy := cached.(*parserScratch)
				if parserHeavySmallScratchFits(heavy, parser, sourceBytes) {
					if parserScratchFitsHandOut(heavy, sourceBytes) {
						s = heavy
					} else {
						rejectedHeavy = heavy
					}
				}
				// A candidate for another grammar or source size is
				// discarded, as on the path for larger sources.
			}
		}
	}
	for tries := 0; s == nil && tries < maxScratchHandOutTries; tries++ {
		cached := pools.shared.Get()
		if cached == nil {
			break
		}
		candidate := cached.(*parserScratch)
		if parserScratchFitsHandOut(candidate, sourceBytes) {
			s = candidate
		} else {
			rejected[rejectedCount] = candidate
			rejectedCount++
		}
	}
	if s == nil {
		s = &parserScratch{}
	}
	if rejectedHeavy != nil {
		pools.heavy.Put(rejectedHeavy)
	}
	for i := 0; i < rejectedCount; i++ {
		pools.shared.Put(rejected[i])
	}
	s.sourceBytes = sourceBytes
	return s
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
	return int64(cap(s.reuseState.arenaRefs)+cap(s.reuseState.arenaWalk))*int64(unsafe.Sizeof(uintptr(0))) + s.entries.allocatedBytes + s.gss.allocatedBytes + s.merge.allocatedBytes() + s.transientChildren.allocatedBytes + s.transientParents.allocatedBytes + s.transientParents.seen.Bytes() + int64(len(s.advancePages))*int64(unsafe.Sizeof([syntheticRootReplayAdvancePageFrames]syntheticRootReplayFrame{})) + int64(len(s.closePages))*int64(unsafe.Sizeof([syntheticRootReplayClosePageFrames]syntheticRootReplayFrame{}))
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
	releaseParserScratchForParser(s, skipGSSClear, nil)
}

func releaseParserScratchForParser(s *parserScratch, skipGSSClear bool, parser *Parser) {
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
	if s.largePool {
		s.heavyGrammar = scratchGrammarKey{}
		largeParserScratchPool.Put(s)
	} else {
		heavy := s.allocatedBytes() > heavySmallParserScratchBytes
		if heavy && parser != nil && parser.language != nil {
			s.heavyGrammar, _ = scratchKeyForLanguage(parser.language)
			heavySmallParserScratchPool.Put(s)
		} else if !heavy {
			s.heavyGrammar = scratchGrammarKey{}
			parserScratchPool.Put(s)
		}
	}
}

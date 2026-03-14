package gotreesitter

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Parser reads parse tables from a Language and produces a syntax tree.
// It supports GLR parsing: when a (state, symbol) pair maps to multiple
// actions, the parser forks the stack and explores all alternatives in
// parallel while preserving distinct parse paths. Duplicate stack
// versions are collapsed and ambiguities are resolved at selection time.
//
// Parser is not safe for concurrent use. Use one parser per goroutine, a
// ParserPool, or guard shared parser instances with external synchronization.
type Parser struct {
	language          *Language
	reuseCursor       reuseCursor
	reuseScratch      reuseScratch
	reuseMu           sync.Mutex
	fullArenaHint     uint32
	rootSymbol        Symbol
	hasRootSymbol     bool
	hasRecoverState   []bool
	hasRecoverSymbol  []bool
	recoverByState    [][]recoverSymbolAction
	hasKeywordState   []bool
	forceRawSpanAll   bool
	forceRawSpanTable []bool
	included          []Range
	logger            ParserLogger
	glrTrace          bool // temporary: verbose GLR stack tracing
	maxConflictWidth  int  // widest N-way conflict in the parse table
	timeoutMicros     uint64
	cancellationFlag  *uint32
	denseLimit        int
	smallBase         int
	smallLookup       [][]smallActionPair
	reduceAliasSeq    [][]Symbol
	reduceHasFields   []bool
}

type smallActionPair struct {
	sym uint16
	val uint16
}

type recoverSymbolAction struct {
	sym    uint16
	action ParseAction
}

const (
	// maxForkCloneDepth limits GLR stack cloning for pathological ambiguity.
	// Above this depth, we execute only the first action to avoid runaway work.
	maxForkCloneDepth = 4 * 1024
	// maxConsecutivePrimaryReduces prevents infinite reduce loops on the
	// primary stack when no token advancement occurs.
	maxConsecutivePrimaryReduces = 256
	// Allow a small temporary oversubscription on full parses before
	// triggering expensive global stack culling, mirroring the C runtime's
	// version overflow window.
	fullParseGLRStackOverflow = 4
)

// IncrementalParseProfile attributes incremental parse time into coarse buckets.
//
// ReuseCursorNanos includes reuse-cursor setup and subtree-candidate checks.
// ReparseNanos includes the remainder of incremental parsing/rebuild work.
type IncrementalParseProfile struct {
	ReuseCursorNanos   int64
	ReparseNanos       int64
	ReusedSubtrees     uint64
	ReusedBytes        uint64
	NewNodesAllocated  uint64
	RecoverSearches    uint64
	RecoverStateChecks uint64
	RecoverStateSkips  uint64
	RecoverSymbolSkips uint64
	RecoverLookups     uint64
	RecoverHits        uint64
	MaxStacksSeen      int
	EntryScratchPeak   uint64
}

type incrementalParseTiming struct {
	totalNanos         int64
	reuseNanos         int64
	reusedSubtrees     uint64
	reusedBytes        uint64
	newNodes           uint64
	recoverSearches    uint64
	recoverStateChecks uint64
	recoverStateSkips  uint64
	recoverSymbolSkips uint64
	recoverLookups     uint64
	recoverHits        uint64
	maxStacksSeen      int
	entryScratchPeak   uint64
}

type parseReuseState struct {
	reusedAny bool
	arenaRefs []*nodeArena
}

// NewParser creates a new Parser for the given language.
func NewParser(lang *Language) *Parser {
	p := &Parser{language: lang}
	if lang != nil {
		p.forceRawSpanAll = lang.Name == "yaml"
		for i, name := range lang.SymbolNames {
			if name != "statement_list" {
				continue
			}
			if p.forceRawSpanTable == nil {
				p.forceRawSpanTable = make([]bool, len(lang.SymbolNames))
			}
			p.forceRawSpanTable[i] = true
		}
		if lang.LargeStateCount > 0 {
			p.denseLimit = int(lang.LargeStateCount)
		} else {
			p.denseLimit = len(lang.ParseTable)
		}
		p.smallBase = int(lang.LargeStateCount)
		if len(lang.SmallParseTableMap) > 0 && len(lang.SmallParseTable) > 0 {
			p.smallLookup = buildSmallLookup(lang)
		}
		p.reduceAliasSeq = buildReduceAliasSequences(lang)
		p.reduceHasFields = buildReduceFieldPresence(lang)
		p.recoverByState, p.hasRecoverState, p.hasRecoverSymbol = buildRecoverActionsByState(lang)
		p.hasKeywordState = buildKeywordStates(lang)
		p.rootSymbol, p.hasRootSymbol = p.inferRootSymbol()
		p.maxConflictWidth = computeMaxConflictWidth(lang)
	}
	return p
}

func buildReduceAliasSequences(lang *Language) [][]Symbol {
	if lang == nil || len(lang.AliasSequences) == 0 {
		return nil
	}
	out := make([][]Symbol, len(lang.AliasSequences))
	for i, seq := range lang.AliasSequences {
		for j := range seq {
			if seq[j] != 0 {
				out[i] = seq
				break
			}
		}
	}
	return out
}

func buildReduceFieldPresence(lang *Language) []bool {
	if lang == nil || len(lang.FieldMapSlices) == 0 {
		return nil
	}
	out := make([]bool, len(lang.FieldMapSlices))
	for i, fm := range lang.FieldMapSlices {
		out[i] = fm[1] != 0
	}
	return out
}

// InferredRootSymbol returns the root symbol inferred during parser
// construction, and whether inference succeeded.
func (p *Parser) InferredRootSymbol() (Symbol, bool) {
	return p.rootSymbol, p.hasRootSymbol
}

// computeMaxConflictWidth scans the parse action table and returns the
// widest N-way conflict (largest len(entry.Actions)). This determines the
// minimum GLR stack cap needed to keep all fork paths alive.
func computeMaxConflictWidth(lang *Language) int {
	maxWidth := 1
	for i := range lang.ParseActions {
		if n := len(lang.ParseActions[i].Actions); n > maxWidth {
			maxWidth = n
		}
	}
	return maxWidth
}

func (p *Parser) inferRootSymbol() (Symbol, bool) {
	if p == nil || p.language == nil {
		return 0, false
	}
	lang := p.language
	if lang.SymbolCount == 0 || lang.TokenCount >= lang.SymbolCount {
		return 0, false
	}
	// ts2go grammars use InitialState=1 (tree-sitter convention). Hand-built
	// test grammars often leave InitialState=0 and may not have a unique
	// start-symbol shape; skip inference there.
	if lang.InitialState == 0 {
		return 0, false
	}
	initial := lang.InitialState
	var candidate Symbol
	found := false
	for sym := Symbol(lang.TokenCount); uint32(sym) < lang.SymbolCount; sym++ {
		gotoState := p.lookupGoto(initial, sym)
		if gotoState == 0 {
			continue
		}
		if !p.stateHasAcceptOnEOF(gotoState) {
			continue
		}
		if !found {
			candidate = sym
			found = true
			continue
		}
		if p.preferRootSymbol(sym, candidate) {
			candidate = sym
		}
	}
	return candidate, found
}

func (p *Parser) stateHasAcceptOnEOF(state StateID) bool {
	if p == nil || p.language == nil {
		return false
	}
	idx := p.lookupActionIndex(state, 0)
	if idx == 0 || int(idx) >= len(p.language.ParseActions) {
		return false
	}
	actions := p.language.ParseActions[idx].Actions
	for i := range actions {
		if actions[i].Type == ParseActionAccept {
			return true
		}
	}
	return false
}

func (p *Parser) preferRootSymbol(candidate, current Symbol) bool {
	score := func(sym Symbol) int {
		s := 0
		if p != nil && p.language != nil && int(sym) < len(p.language.SymbolMetadata) {
			meta := p.language.SymbolMetadata[sym]
			if meta.Visible {
				s += 2
			}
			if meta.Named {
				s++
			}
		}
		if p != nil && p.language != nil && int(sym) < len(p.language.SymbolNames) {
			switch p.language.SymbolNames[sym] {
			case "source_file", "program", "module", "document", "file":
				s += 3
			}
		}
		return s
	}
	candidateScore := score(candidate)
	currentScore := score(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	return candidate < current
}

func (p *Parser) canFinalizeNoActionEOF(s *glrStack) bool {
	if s == nil || s.dead {
		return false
	}
	top := s.top()
	if top.node == nil {
		return true
	}

	tokenCount := uint32(0)
	if p != nil && p.language != nil {
		tokenCount = p.language.TokenCount
	}

	// Without an inferred root, the legacy behavior is still appropriate:
	// a single nonterminal at the top can serve as the final tree root.
	if p == nil || !p.hasRootSymbol {
		return p != nil && p.language != nil && uint32(top.node.symbol) >= tokenCount
	}

	nonExtraCount, onlyNonExtra := countNonExtraStackNodes(s)
	if nonExtraCount == 0 {
		return true
	}
	if nonExtraCount != 1 || onlyNonExtra == nil || onlyNonExtra.symbol == errorSymbol {
		return false
	}
	return uint32(onlyNonExtra.symbol) >= tokenCount
}

func (p *Parser) acceptNoActionEOF(s *glrStack, tok Token) bool {
	if tok.Symbol != 0 || tok.StartByte != tok.EndByte {
		return false
	}
	if s == nil || s.dead {
		return false
	}
	if s.top().node == nil && s.byteOffset != tok.EndByte {
		return false
	}
	if !p.canFinalizeNoActionEOF(s) {
		return false
	}
	s.accepted = true
	return true
}

func (p *Parser) acceptExpectedRootEOF(s *glrStack, tok Token) bool {
	if tok.Symbol != 0 || tok.StartByte != tok.EndByte || p == nil || !p.hasRootSymbol || s == nil || s.dead {
		return false
	}
	expectedStart := p.expectedRootStartByte()
	top := s.top()
	if top.node != nil &&
		top.node.symbol == p.rootSymbol &&
		top.node.symbol != errorSymbol &&
		top.node.startByte == expectedStart &&
		top.node.endByte == tok.EndByte &&
		s.byteOffset == tok.EndByte {
		s.accepted = true
		return true
	}
	if s.byteOffset != tok.EndByte {
		return false
	}
	if stackHasExpectedRootEOFSuffix(s, p.rootSymbol, expectedStart, tok.EndByte) {
		s.accepted = true
		return true
	}
	return false
}

func (p *Parser) expectedRootStartByte() uint32 {
	if p == nil || len(p.included) == 0 {
		return 0
	}
	return p.included[0].StartByte
}

func (p *Parser) expectedRootEndByte(source []byte) uint32 {
	if p != nil && len(p.included) > 0 {
		return p.included[len(p.included)-1].EndByte
	}
	return uint32(len(source))
}

func trimExpectedRootEOFSuffixNodes(nodes []*Node, expectedRoot Symbol, expectedStart, eofByte uint32) ([]*Node, bool) {
	if len(nodes) < 2 {
		return nodes, false
	}

	suffixStart := len(nodes)
	for suffixStart > 0 {
		n := nodes[suffixStart-1]
		if n == nil || n.isExtra {
			break
		}
		if n.symbol != expectedRoot || n.startByte != eofByte || n.endByte != eofByte {
			break
		}
		suffixStart--
	}
	if suffixStart == len(nodes) || suffixStart == 0 {
		return nodes, false
	}

	prefixCount := 0
	firstStart := uint32(0)
	lastEnd := uint32(0)
	for i := 0; i < suffixStart; i++ {
		n := nodes[i]
		if n == nil || n.isExtra {
			continue
		}
		if prefixCount == 0 {
			firstStart = n.startByte
		}
		lastEnd = n.endByte
		if n.hasError {
			return nodes, false
		}
		prefixCount++
	}
	if prefixCount == 0 || firstStart != expectedStart || lastEnd != eofByte {
		return nodes, false
	}
	return nodes[:suffixStart], true
}

func stackHasExpectedRootEOFSuffix(s *glrStack, expectedRoot Symbol, expectedStart, eofByte uint32) bool {
	if s == nil {
		return false
	}
	if len(s.entries) > 0 {
		nodes := make([]*Node, 0, len(s.entries)-1)
		for i := range s.entries {
			if s.entries[i].node != nil {
				nodes = append(nodes, s.entries[i].node)
			}
		}
		_, ok := trimExpectedRootEOFSuffixNodes(nodes, expectedRoot, expectedStart, eofByte)
		return ok
	}
	_, ok := trimExpectedRootEOFSuffixNodes(nodesFromGSS(s.gss), expectedRoot, expectedStart, eofByte)
	return ok
}

func countNonExtraStackNodes(s *glrStack) (int, *Node) {
	if s == nil {
		return 0, nil
	}
	nonExtraCount := 0
	onlyNonExtra := (*Node)(nil)
	countNode := func(n *Node) bool {
		if n == nil || n.isExtra {
			return false
		}
		nonExtraCount++
		onlyNonExtra = n
		return nonExtraCount > 1
	}

	if len(s.entries) > 0 {
		for i := range s.entries {
			if countNode(s.entries[i].node) {
				return nonExtraCount, onlyNonExtra
			}
		}
		return nonExtraCount, onlyNonExtra
	}
	for n := s.gss.head; n != nil; n = n.prev {
		if countNode(n.entry.node) {
			break
		}
	}
	return nonExtraCount, onlyNonExtra
}

func (p *Parser) parseIncrementalInternal(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming) *Tree {
	// Fast path: unchanged source and no recorded edits.
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree
	}

	// Subtree reuse is safe for DFA token sources without external scanners
	// and for custom token sources that explicitly opt in.
	if !tokenSourceSupportsIncrementalReuse(ts) {
		arenaClass := incrementalArenaClassForSource(source)
		// Keep parse-time memory behavior consistent with incremental parses.
		return p.parseInternal(source, ts, nil, nil, arenaClass, timing, 0, 0, 0, false)
	}

	p.reuseMu.Lock()
	defer p.reuseMu.Unlock()

	var reuse *reuseCursor
	if timing != nil {
		reuseStart := time.Now()
		reuse = p.reuseCursor.reset(oldTree, source, &p.reuseScratch)
		timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
	} else {
		reuse = p.reuseCursor.reset(oldTree, source, &p.reuseScratch)
	}
	arenaClass := incrementalArenaClassForSource(source)
	tree := p.parseInternal(source, ts, reuse, oldTree, arenaClass, timing, 0, 0, 0, false)
	if reuse != nil {
		if timing != nil {
			reuseStart := time.Now()
			reuse.commitScratch(&p.reuseScratch)
			timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
		} else {
			reuse.commitScratch(&p.reuseScratch)
		}
	}
	return tree
}

func tokenSourceSupportsIncrementalReuse(ts TokenSource) bool {
	if ts == nil {
		return false
	}
	if dts, ok := ts.(*dfaTokenSource); ok {
		return dts.language == nil || dts.language.ExternalScanner == nil
	}
	if reusable, ok := ts.(IncrementalReuseTokenSource); ok {
		return reusable.SupportsIncrementalReuse()
	}
	return false
}

func incrementalArenaClassForSource(source []byte) arenaClass {
	arenaClass := arenaClassIncremental
	// Very large files can outgrow incremental defaults and trigger repeated
	// fallback allocations; use full-parse slab sizing only beyond this point.
	const incrementalUseFullArenaThreshold = 1 * 1024 * 1024
	if len(source) >= incrementalUseFullArenaThreshold {
		arenaClass = arenaClassFull
	}
	return arenaClass
}

func canReuseUnchangedTree(source []byte, oldTree *Tree, lang *Language) bool {
	if oldTree == nil || oldTree.language != lang || len(oldTree.edits) != 0 {
		return false
	}
	oldSource := oldTree.source
	if len(oldSource) != len(source) {
		return false
	}
	if len(source) == 0 {
		return true
	}
	// Common incremental no-edit case: caller passes the same source slice.
	// Pointer equality avoids memcmp on hot no-op reparses.
	if &oldSource[0] == &source[0] {
		return true
	}
	return bytes.Equal(oldSource, source)
}

func (p *Parser) logf(kind ParserLogType, format string, args ...any) {
	if p == nil || p.logger == nil {
		return
	}
	p.logger(kind, fmt.Sprintf(format, args...))
}

// parseInternal is the core GLR parsing loop shared by Parse and
// ParseWithTokenSource.
//
// It maintains a set of parse stacks. For unambiguous grammars (single
// action per table entry), there is exactly one stack and the algorithm
// reduces to standard LR parsing. When multiple actions exist for a
// (state, symbol) pair, the parser forks: one stack per alternative.
// Stacks that error out are dropped. Only duplicate stack versions are
// merged; distinct alternatives are preserved.
func (p *Parser) parseInternal(source []byte, ts TokenSource, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass, timing *incrementalParseTiming, maxStacksOverride int, maxNodesOverride int, maxMergePerKeyOverride int, deterministicExternalConflicts bool) *Tree {
	parseStart := time.Now()
	if p.logger != nil {
		p.logf(ParserLogParse, "start len=%d incremental=%t", len(source), reuse != nil || oldTree != nil)
	}
	deferParentLinks := reuse == nil && oldTree == nil
	if closer, ok := ts.(interface{ Close() }); ok {
		defer closer.Close()
	}
	scratch := acquireParserScratch()
	if deferParentLinks {
		scratch.gss.initialCap = fullParseGSSNodeSlabCap
	} else {
		scratch.gss.initialCap = defaultGSSNodeSlabCap
	}
	defer releaseParserScratch(scratch, deferParentLinks)
	trackChildErrors := !deferParentLinks

	arena := acquireNodeArena(arenaClass)
	arena.skipChildClear = reuse == nil && oldTree == nil
	if timing != nil {
		startUsed := arena.used
		defer func() {
			timing.totalNanos += time.Since(parseStart).Nanoseconds()
			if arena.used >= startUsed {
				timing.newNodes += uint64(arena.used - startUsed)
			}
			peak := uint64(scratch.entries.peakEntriesUsed())
			if peak > timing.entryScratchPeak {
				timing.entryScratchPeak = peak
			}
		}()
	}
	if arenaClass == arenaClassFull {
		defer func() {
			p.recordFullArenaUsage(arena.used)
		}()
	}
	switch arenaClass {
	case arenaClassFull:
		arena.ensureNodeCapacity(parseFullArenaNodeCapacity(len(source), p.fullArenaHintCapacity()))
		scratch.entries.ensureInitialCap(parseFullEntryScratchCapacity(len(source)))
	case arenaClassIncremental:
		arena.ensureNodeCapacity(parseIncrementalArenaNodeCapacity(len(source)))
		scratch.entries.ensureInitialCap(parseIncrementalEntryScratchCapacity(len(source)))
	}
	var reuseState parseReuseState
	nodeCount := 0
	iterationsUsed := 0
	peakStackDepth := 0
	maxStacksSeen := 0
	var perfTokensConsumed uint64
	var lastTokenEndByte uint32
	var lastTokenSymbol Symbol
	var lastTokenWasEOF bool
	tokenSourceEOFEarly := false
	expectedEOFByte := uint32(len(source))
	if len(p.included) > 0 {
		expectedEOFByte = p.included[len(p.included)-1].EndByte
	}
	parseRuntime := ParseRuntime{
		StopReason:      ParseStopNone,
		SourceLen:       uint32(len(source)),
		ExpectedEOFByte: expectedEOFByte,
	}
	finalizeTree := func(tree *Tree, stopReason ParseStopReason) *Tree {
		if tokenSourceEOFEarly && (stopReason == ParseStopAccepted || stopReason == ParseStopNone) {
			stopReason = ParseStopTokenSourceEOF
		}
		parseRuntime.StopReason = stopReason
		parseRuntime.Iterations = iterationsUsed
		parseRuntime.NodesAllocated = nodeCount
		parseRuntime.PeakStackDepth = peakStackDepth
		parseRuntime.MaxStacksSeen = maxStacksSeen
		parseRuntime.TokensConsumed = perfTokensConsumed
		parseRuntime.LastTokenEndByte = lastTokenEndByte
		parseRuntime.LastTokenSymbol = lastTokenSymbol
		parseRuntime.LastTokenWasEOF = lastTokenWasEOF
		parseRuntime.TokenSourceEOFEarly = tokenSourceEOFEarly
		parseRuntime.RootEndByte = 0
		parseRuntime.Truncated = false
		if tree != nil && tree.RootNode() != nil {
			parseRuntime.RootEndByte = tree.RootNode().EndByte()
			parseRuntime.Truncated = parseRuntime.RootEndByte < expectedEOFByte
		}
		if tree != nil {
			tree.setParseRuntime(parseRuntime)
		}
		if p.logger != nil {
			p.logf(
				ParserLogParse,
				"stop reason=%s truncated=%t tokens=%d max_stacks=%d",
				parseRuntime.StopReason,
				parseRuntime.Truncated,
				parseRuntime.TokensConsumed,
				parseRuntime.MaxStacksSeen,
			)
		}
		return tree
	}
	finalize := func(stacks []glrStack, stopReason ParseStopReason) *Tree {
		tree := p.buildResultFromGLR(stacks, source, arena, oldTree, &reuseState, &scratch.nodeLinks)
		return finalizeTree(tree, stopReason)
	}
	finalizeErrorTree := func(stopReason ParseStopReason) *Tree {
		arena.Release()
		return finalizeTree(parseErrorTreeWithExpectedRoot(source, p.language, p.rootSymbol, p.hasRootSymbol), stopReason)
	}

	var stacksBuf [4]glrStack
	stacks := stacksBuf[:1]
	initialStackCap := 64 * 1024
	if reuse != nil {
		// Incremental reparses often borrow scratch slabs from an earlier full
		// parse. Preallocating that full retained capacity forces large memclr
		// work on every edit; keep incremental stack preallocation modest.
		initialStackCap = defaultStackEntrySlabCap
	}
	stacks[0] = newGLRStackWithScratchCap(p.language.InitialState, &scratch.entries, initialStackCap)
	stacks[0].recoverabilityKnown = true
	stacks[0].mayRecover = p.stateCanRecover(p.language.InitialState)
	maxStacksSeen = len(stacks)
	if timing != nil && timing.maxStacksSeen < len(stacks) {
		timing.maxStacksSeen = len(stacks)
	}
	maxStacks := parseMaxGLRStacksValue()
	if maxStacksOverride > 0 && maxStacksOverride > maxStacks {
		maxStacks = maxStacksOverride
	}
	// Ensure the stack cap is at least as wide as the grammar's widest
	// N-way conflict. Without this, retainTopStacks silently drops correct
	// parse paths, producing wrong trees instead of triggering error recovery.
	if p.maxConflictWidth > maxStacks {
		maxStacks = p.maxConflictWidth
	}
	mergePerKeyCap := maxStacksPerMergeKey
	if maxMergePerKeyOverride > mergePerKeyCap {
		mergePerKeyCap = maxMergePerKeyOverride
	}
	if mergePerKeyCap > maxStacksPerMergeKeyCeiling {
		mergePerKeyCap = maxStacksPerMergeKeyCeiling
	}
	if reuse == nil && p.language != nil && p.language.Name == "bash" {
		if maxStacks < 256 {
			maxStacks = 256
		}
		if mergePerKeyCap < 256 {
			mergePerKeyCap = 256
		}
	}
	if reuse == nil && p.language != nil && p.language.Name == "c_sharp" {
		// C# member-access vs qualified-name ambiguity needs a slightly
		// wider per-key survivor budget on full parses to match the C runtime.
		if mergePerKeyCap < 16 {
			mergePerKeyCap = 16
		}
	}
	if reuse != nil {
		// Incremental reparses benefit from tighter GLR retention because
		// edits are localized and we prioritize latency over broad ambiguity fanout.
		if maxStacks > 32 {
			maxStacks = 32
		}
		if mergePerKeyCap > 4 {
			mergePerKeyCap = 4
		}
	}
	scratch.merge.perKeyCap = mergePerKeyCap
	langName := ""
	if p.language != nil {
		langName = p.language.Name
	}
	maxStackCullTrigger := glrStackCullTrigger(maxStacks, arenaClass, langName)

	maxIter := parseIterations(len(source))
	maxDepth := parseStackDepth(len(source))
	maxNodes := parseNodeLimit(len(source))
	if maxNodesOverride > maxNodes {
		maxNodes = maxNodesOverride
	}
	parseRuntime.IterationLimit = maxIter
	parseRuntime.StackDepthLimit = maxDepth
	parseRuntime.NodeLimit = maxNodes

	needToken := true
	var tok Token
	var nextBranchOrder uint64 = 1

	// Per-primary-stack infinite-reduce detection.
	var lastReduceState StateID
	lastReduceDepth := -1
	var consecutiveReduces int

	for iter := 0; iter < maxIter; iter++ {
		if p.timeoutMicros > 0 {
			// Timeout is checked inside the parse loop so long-running parses
			// can terminate predictably under caller-configured limits.
			if time.Since(parseStart) > time.Duration(p.timeoutMicros)*time.Microsecond {
				return finalize(stacks, ParseStopTimeout)
			}
		}
		if flag := p.cancellationFlag; flag != nil && atomic.LoadUint32(flag) != 0 {
			return finalize(stacks, ParseStopCancelled)
		}
		iterationsUsed = iter + 1
		if perfCountersEnabled {
			perfRecordMaxConcurrentStacks(len(stacks))
		}
		if timing != nil && len(stacks) > timing.maxStacksSeen {
			timing.maxStacksSeen = len(stacks)
		}
		if len(stacks) > maxStacksSeen {
			maxStacksSeen = len(stacks)
		}
		// Fast-path the overwhelmingly common non-GLR case with one live stack.
		if len(stacks) == 1 {
			if stacks[0].dead {
				return finalize(stacks, ParseStopNoStacksAlive)
			}
		} else {
			allDead := true
			for i := range stacks {
				if !stacks[i].dead {
					allDead = false
					break
				}
			}
			if allDead {
				return finalize(stacks, ParseStopNoStacksAlive)
			}
			// Prune dead stacks and collapse only truly duplicate stack versions.
			scratch.merge.language = p.language
			stacks = mergeStacksWithScratch(stacks, &scratch.merge)
			if len(stacks) == 0 {
				return finalizeErrorTree(ParseStopNoStacksAlive)
			}
		}
		// Cap the number of parallel stacks to prevent combinatorial explosion.
		// Keep the most promising stacks instead of truncating by insertion
		// order, which can discard viable parses on highly-ambiguous inputs.
		if len(stacks) > maxStackCullTrigger {
			if p.glrTrace {
				fmt.Printf("[GLR] CAP CULL: %d stacks → keep %d (trigger=%d)\n", len(stacks), maxStacks, maxStackCullTrigger)
				for ci := range stacks {
					fmt.Printf("  pre-cull[%d]: st=%d dead=%v shift=%v dep=%d score=%d byte=%d\n",
						ci, stacks[ci].top().state, stacks[ci].dead, stacks[ci].shifted, stacks[ci].depth(), stacks[ci].score, stacks[ci].byteOffset)
				}
			}
			if perfCountersEnabled {
				perfRecordGlobalCapCull(len(stacks), maxStacks)
			}
			if arenaClass == arenaClassFull {
				stacks = retainTopStacksForLanguageWithScratch(stacks, maxStacks, p.language, &scratch.stackPick, &scratch.stackKeep, &scratch.stackCull, &scratch.stateKeep)
			} else {
				stacks = retainTopStacksForLanguageWithScratch(stacks, maxStacks, p.language, &scratch.stackPick, &scratch.stackKeep, nil, nil)
			}
			if p.glrTrace {
				fmt.Printf("[GLR] after cull:\n")
				for ci := range stacks {
					fmt.Printf("  kept[%d]: st=%d dead=%v shift=%v dep=%d score=%d byte=%d\n",
						ci, stacks[ci].top().state, stacks[ci].dead, stacks[ci].shifted, stacks[ci].depth(), stacks[ci].score, stacks[ci].byteOffset)
				}
			}
		}

		// Keep the most promising stack in slot 0 because several parser
		// heuristics (lex-mode selection, reduce-loop detection, depth cap)
		// currently key off the primary stack.
		if len(stacks) > 1 {
			p.promotePrimaryStack(stacks)
		}
		for i := range stacks {
			stacks[i].cacheEntries = false
			if stacks[i].gss.head != nil {
				stacks[i].entries = nil
			}
		}

		// Safety: if the primary stack has grown beyond the depth cap,
		// or we've allocated too many nodes, return what we have.
		primaryDepth := stacks[0].depth()
		if primaryDepth > peakStackDepth {
			peakStackDepth = primaryDepth
		}
		if primaryDepth > maxDepth {
			return finalize(stacks, ParseStopStackDepthLimit)
		}
		if nodeCount > maxNodes {
			return finalize(stacks, ParseStopNodeLimit)
		}

		// Use the primary (first) stack's state for DFA lex mode selection.
		// Pass all active GLR stack states so external scanner valid symbols
		// are computed as the union across all stacks.
		if stateful, ok := ts.(parserStateTokenSource); ok {
			stateful.SetParserState(stacks[0].top().state)
			if len(stacks) > 1 {
				if p.language != nil && p.language.Name == "yaml" && p.language.ExternalScanner != nil {
					// External scanners are stateful. Until scanner state is
					// tracked per GLR stack, drive tokenization from the primary
					// stack state only to avoid over-admitting tokens from state unions.
					if len(scratch.glrStates) > 0 {
						scratch.glrStates = scratch.glrStates[:0]
					}
					stateful.SetGLRStates(nil)
				} else {
					glrBuf := scratch.glrStates[:0]
					if cap(glrBuf) < len(stacks) {
						glrBuf = make([]StateID, 0, len(stacks))
					}
					for si := range stacks {
						if !stacks[si].dead {
							glrBuf = append(glrBuf, stacks[si].top().state)
						}
					}
					scratch.glrStates = glrBuf
					stateful.SetGLRStates(glrBuf)
				}
			} else {
				if len(scratch.glrStates) > 0 {
					scratch.glrStates = scratch.glrStates[:0]
				}
				stateful.SetGLRStates(nil)
			}
		}

		// --- Token acquisition and incremental reuse ---
		if needToken {
			tok = ts.Next()
			if p.logger != nil {
				p.logf(ParserLogLex, "token sym=%d start=%d end=%d", tok.Symbol, tok.StartByte, tok.EndByte)
			}
			perfTokensConsumed++
			lastTokenEndByte = tok.EndByte
			lastTokenSymbol = tok.Symbol
			lastTokenWasEOF = tok.Symbol == 0 && tok.StartByte == tok.EndByte && !tok.NoLookahead
			if lastTokenWasEOF && tok.EndByte < expectedEOFByte {
				tokenSourceEOFEarly = true
			}
			// Clear per-stack shifted flags so all stacks process the
			// new token.
			for si := range stacks {
				stacks[si].shifted = false
			}
		}

		// Incremental parsing fast-path: when there is a single active stack,
		// try to reuse an unchanged subtree starting at the current token.
		if reuse != nil && len(stacks) == 1 && !stacks[0].dead && tok.Symbol != 0 {
			if timing != nil {
				reuseStart := time.Now()
				nextTok, reusedBytes, ok := p.tryReuseSubtree(&stacks[0], tok, ts, reuse, &scratch.entries, &scratch.gss)
				timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
				if ok {
					timing.reusedSubtrees++
					timing.reusedBytes += uint64(reusedBytes)
					reuseState.markReused(stacks[0].top().node, arena)
					tok = nextTok
					needToken = false
					consecutiveReduces = 0
					continue
				}
			} else {
				if nextTok, _, ok := p.tryReuseSubtree(&stacks[0], tok, ts, reuse, &scratch.entries, &scratch.gss); ok {
					reuseState.markReused(stacks[0].top().node, arena)
					tok = nextTok
					needToken = false
					consecutiveReduces = 0
					continue
				}
			}
		}

		// --- Action application for all alive stacks ---
		// Process all alive stacks for this token.
		// We iterate by index because forks may append to `stacks`.
		numStacks := len(stacks)
		anyReduced := false

		if p.glrTrace {
			symName := "?"
			if int(tok.Symbol) < len(p.language.SymbolNames) {
				symName = p.language.SymbolNames[tok.Symbol]
			}
			fmt.Printf("[GLR] iter=%d tok=%s(%d)[%d-%d] stacks=%d needTok=%v\n",
				iter, symName, tok.Symbol, tok.StartByte, tok.EndByte, len(stacks), needToken)
			for si := range stacks {
				fmt.Printf("  s[%d]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
					si, stacks[si].top().state, stacks[si].dead, stacks[si].shifted, stacks[si].depth(), stacks[si].byteOffset)
			}
		}

		parseActions := p.language.ParseActions
		for si := 0; si < numStacks; si++ {
			s := &stacks[si]
			if s.dead || s.shifted {
				continue
			}

			currentState := s.top().state
			actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
			var actions []ParseAction
			if actionIdx != 0 && int(actionIdx) < len(parseActions) {
				actions = parseActions[actionIdx].Actions
			}
			if p.glrTrace {
				fmt.Printf("  stack[%d] state=%d actionIdx=%d actions=%d\n", si, currentState, actionIdx, len(actions))
				for ai, a := range actions {
					fmt.Printf("    action[%d]: type=%d state=%d sym=%d cnt=%d prec=%d\n",
						ai, a.Type, a.State, a.Symbol, a.ChildCount, a.DynamicPrecedence)
				}
			}

			if p.acceptExpectedRootEOF(s, tok) {
				if len(stacks) == 1 {
					return finalize(stacks, ParseStopAccepted)
				}
				continue
			}

			// --- Extra token handling (comments, whitespace) ---
			if len(actions) == 1 &&
				actions[0].Type == ParseActionShift && actions[0].Extra {
				p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, nil, false, nil)
				needToken = true
				continue
			}

			// --- No action: error handling ---
			if len(actions) == 0 {
				if tok.Symbol == 0 {
					if tok.StartByte == tok.EndByte {
						// True EOF. A stack that already represents a complete root
						// should survive even if sibling GLR branches are incomplete.
						if p.acceptNoActionEOF(s, tok) {
							if len(stacks) == 1 {
								return finalize(stacks, ParseStopAccepted)
							}
							continue
						}
						s.dead = true
						continue
					}
					// Zero-symbol width token: skip.
					needToken = true
					continue
				}

				// When multiple alternatives exist, drop no-action stacks
				// immediately instead of running deep recovery scans.
				if len(stacks) > 1 {
					if p.glrTrace {
						fmt.Printf("  stack[%d] KILLED: no action for sym=%d in state=%d (multiple stacks)\n", si, tok.Symbol, currentState)
					}
					s.dead = true
					continue
				}

				// Try grammar-directed recovery by searching the stack for
				// the nearest state that can recover on this lookahead.
				if depth, recoverAct, ok := p.findRecoverActionOnStack(s, tok.Symbol, timing); ok {
					if !s.truncate(depth + 1) {
						s.dead = true
						continue
					}
					p.applyAction(s, recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					needToken = true
					continue
				}

				// Only stack: error recovery — wrap token in error node.
				if s.depth() == 0 {
					return finalize(stacks, ParseStopNoStacksAlive)
				}
				p.pushOrExtendErrorNode(s, currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &trackChildErrors)
				needToken = true
				continue
			}

			// --- GLR: fork for multiple actions ---
			// For single-action entries (the common case), no fork occurs.
			// For multi-action entries, clone the stack for each alternative.
			if len(actions) > 1 {
				if reuse == nil && p.language != nil && p.language.Name == "go" && maxStacksSeen > 1 && currentState == 3 && tok.Symbol == 15 {
					if chosen, ok := repetitionShiftConflictChoice(actions); ok {
						p.applyAction(s, chosen, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
						continue
					}
				}
				// Current external-scanner integration shares one scanner payload
				// across all GLR stacks. Forking stacks while mutating shared
				// scanner state can diverge from C runtime behavior. Until
				// per-stack scanner state is modeled, keep external-scanner
				// parses deterministic at conflicts.
				if deterministicExternalConflicts && p.language != nil && p.language.Name == "yaml" && p.language.ExternalScanner != nil {
					chosen := actions[0]
					for ai := 1; ai < len(actions); ai++ {
						cand := actions[ai]
						if cand.Type == ParseActionShift {
							chosen = cand
							break
						}
						if chosen.Type == ParseActionReduce && cand.Type == ParseActionReduce &&
							cand.DynamicPrecedence > chosen.DynamicPrecedence {
							chosen = cand
						}
					}
					p.applyAction(s, chosen, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					continue
				}
				if perfCountersEnabled {
					rrConflict, rsConflict := classifyConflictShape(actions)
					switch {
					case rrConflict:
						perfRecordConflictRR()
					case rsConflict:
						perfRecordConflictRS()
					default:
						perfRecordConflictOther()
					}
				}
				if perfCountersEnabled {
					perfRecordFork(len(actions), perfTokensConsumed)
				}
				// Deep-stack GLR forks can trigger pathological clone volumes on
				// very large inputs. At extreme depths, take the primary action
				// to keep parsing bounded.
				if s.depth() > maxForkCloneDepth {
					act := actions[0]
					p.applyAction(s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					continue
				}
				// Copy the current stack value before appending forks.
				// Appending can reallocate `stacks`, which would invalidate `s`.
				base := *s
				if p.glrTrace {
					fmt.Printf("[GLR] FORK: %d actions from state=%d\n", len(actions), currentState)
					for ai, a := range actions {
						symName := "?"
						if int(a.Symbol) < len(p.language.SymbolNames) {
							symName = p.language.SymbolNames[a.Symbol]
						}
						fmt.Printf("  action[%d]: type=%d state=%d sym=%s(%d) cnt=%d prec=%d\n",
							ai, a.Type, a.State, symName, a.Symbol, a.ChildCount, a.DynamicPrecedence)
					}
				}
				for ai := 1; ai < len(actions); ai++ {
					fork := base.cloneWithScratch(&scratch.gss)
					fork.branchOrder = nextBranchOrder
					nextBranchOrder++
					act := actions[ai]
					p.applyAction(&fork, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					if p.glrTrace {
						fmt.Printf("[GLR] fork[%d] after action[%d]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
							len(stacks), ai, fork.top().state, fork.dead, fork.shifted, fork.depth(), fork.byteOffset)
					}
					stacks = append(stacks, fork)
				}
				// Re-acquire the pointer after possible reallocation.
				s = &stacks[si]
				act := actions[0]
				p.applyAction(s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
				if p.glrTrace {
					fmt.Printf("[GLR] orig[%d] after action[0]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
						si, s.top().state, s.dead, s.shifted, s.depth(), s.byteOffset)
				}
			} else {
				act := actions[0]
				disableBashReduceChain := p.language != nil && p.language.Name == "bash" && s.gss.head != nil
				if act.Type == ParseActionReduce && !disableBashReduceChain {
					p.applyActionWithReduceChain(s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
				} else {
					p.applyAction(s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
				}
			}
		}

		// GLR all-dead recovery: when multiple stacks exist and ALL of
		// them die on the current token, resurrect the best one and do
		// error recovery instead of abandoning the parse entirely. This
		// handles grammars where a reduce/shift conflict produces forks
		// that all converge to a state without an action for the next
		// token (e.g., trailing commas in jq objects).
		//
		// Only activate during retry passes (maxStacksOverride > 0) to
		// avoid suppressing the first-pass → retry escalation path. On
		// the first pass, letting all stacks die triggers a retry at a
		// higher stack cap, which often produces cleaner trees.
		if numStacks > 1 && maxStacksOverride > 0 {
			allDead := true
			for si := 0; si < len(stacks); si++ {
				if !stacks[si].dead {
					allDead = false
					break
				}
			}
			if allDead {
				// Find the best stack to resurrect.
				bestIdx := 0
				for si := 1; si < len(stacks); si++ {
					if stacks[si].score > stacks[bestIdx].score {
						bestIdx = si
					} else if stacks[si].score == stacks[bestIdx].score && stacks[si].depth() < stacks[bestIdx].depth() {
						bestIdx = si
					}
				}
				s := &stacks[bestIdx]
				s.dead = false

				// Collapse to single stack so subsequent iterations use
				// single-stack error recovery paths.
				stacks[0] = *s
				stacks = stacks[:1]

				if p.glrTrace {
					fmt.Printf("[GLR] ALL-DEAD RECOVERY: resurrect stack (was [%d]) st=%d dep=%d byte=%d\n",
						bestIdx, stacks[0].top().state, stacks[0].depth(), stacks[0].byteOffset)
				}

				currentState := stacks[0].top().state
				// Try grammar-directed recovery first.
				if depth, recoverAct, ok := p.findRecoverActionOnStack(&stacks[0], tok.Symbol, timing); ok {
					if stacks[0].truncate(depth + 1) {
						p.applyAction(&stacks[0], recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
						needToken = true
					} else {
						stacks[0].dead = true
					}
				} else if stacks[0].depth() > 0 {
					// Wrap the problematic token in an error node.
					p.pushOrExtendErrorNode(&stacks[0], currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &trackChildErrors)
					needToken = true
				}
			}
		}

		// After processing all stacks: determine whether to advance the
		// token. If any stack reduced, reuse the same token (the reducing
		// stacks have new top states and need to re-check the action for
		// the current lookahead). Otherwise, advance to next token.
		if anyReduced {
			needToken = tok.NoLookahead

			// Infinite-reduce detection (for the primary stack).
			if !tok.NoLookahead && len(stacks) > 0 && !stacks[0].dead {
				topState := stacks[0].top().state
				topDepth := stacks[0].depth()
				if topState == lastReduceState && topDepth == lastReduceDepth {
					consecutiveReduces++
				} else {
					lastReduceState = topState
					lastReduceDepth = topDepth
					consecutiveReduces = 1
				}
				if consecutiveReduces > maxConsecutivePrimaryReduces {
					needToken = true
					lastReduceDepth = -1
					consecutiveReduces = 0
				}
			} else if tok.NoLookahead {
				lastReduceDepth = -1
				consecutiveReduces = 0
			}
		} else {
			needToken = true
			lastReduceDepth = -1
			consecutiveReduces = 0
		}

		// Check for accept on any stack. Keep all accepted branches so the
		// final GLR chooser can rank them instead of short-circuiting on the
		// first accepted stack encountered.
		if accepted := compactAcceptedStacks(stacks); len(accepted) > 0 {
			return finalize(accepted, ParseStopAccepted)
		}
	}

	// Iteration limit reached.
	return finalize(stacks, ParseStopIterationLimit)
}

func repetitionShiftConflictChoice(actions []ParseAction) (ParseAction, bool) {
	if len(actions) < 2 {
		return ParseAction{}, false
	}
	var shift ParseAction
	shiftFound := false
	reduceFound := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			if !act.Repetition || shiftFound {
				return ParseAction{}, false
			}
			shift = act
			shiftFound = true
		case ParseActionReduce:
			reduceFound = true
		default:
			return ParseAction{}, false
		}
	}
	if !shiftFound || !reduceFound {
		return ParseAction{}, false
	}
	return shift, true
}

func compactAcceptedStacks(stacks []glrStack) []glrStack {
	acceptedCount := 0
	for i := range stacks {
		if stacks[i].accepted {
			stacks[acceptedCount] = stacks[i]
			acceptedCount++
		}
	}
	return stacks[:acceptedCount]
}

func glrStackCullTrigger(maxStacks int, class arenaClass, langName string) int {
	if maxStacks <= 0 {
		return maxStacks
	}
	if class != arenaClassFull || langName == "c_sharp" {
		return maxStacks
	}
	maxInt := int(^uint(0) >> 1)
	if maxStacks > maxInt-fullParseGLRStackOverflow {
		return maxInt
	}
	return maxStacks + fullParseGLRStackOverflow
}

func (p *Parser) promotePrimaryStack(stacks []glrStack) {
	if len(stacks) <= 1 {
		return
	}
	best := 0
	for i := 1; i < len(stacks); i++ {
		if stackComparePtr(&stacks[i], &stacks[best]) > 0 {
			best = i
		}
	}
	if best != 0 {
		stacks[0], stacks[best] = stacks[best], stacks[0]
	}
}

func stackCompareForCull(lang *Language, a, b *glrStack) int {
	if lang == nil || lang.Name != "c_sharp" {
		return stackComparePtr(a, b)
	}
	if a.dead != b.dead {
		if a.dead {
			return -1
		}
		return 1
	}
	if a.accepted != b.accepted {
		if a.accepted {
			return 1
		}
		return -1
	}
	if aErr, bErr := stackErrorRank(a), stackErrorRank(b); aErr != bErr {
		if aErr < bErr {
			return 1
		}
		return -1
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	aDepth := a.depth()
	bDepth := b.depth()
	if aDepth != bDepth {
		if aDepth > bDepth {
			return 1
		}
		return -1
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if a.shifted != b.shifted {
		if !a.shifted {
			return 1
		}
		return -1
	}
	aHash := stackHash(*a)
	bHash := stackHash(*b)
	if aHash > bHash {
		return 1
	}
	if aHash < bHash {
		return -1
	}
	return 0
}

type stackCullKey struct {
	state      StateID
	byteOffset uint32
	score      int
	hash       uint64
	depth      int
	errorRank  uint8
	flags      uint8
}

const (
	stackCullDeadFlag = 1 << iota
	stackCullAcceptedFlag
	stackCullShiftedFlag
)

func buildStackCullKeys(stacks []glrStack, lang *Language, buf *[]stackCullKey) []stackCullKey {
	if len(stacks) == 0 {
		if buf != nil {
			*buf = (*buf)[:0]
		}
		return nil
	}
	var keys []stackCullKey
	if buf != nil {
		if cap(*buf) < len(stacks) {
			*buf = make([]stackCullKey, len(stacks))
		}
		keys = (*buf)[:len(stacks)]
	} else {
		keys = make([]stackCullKey, len(stacks))
	}
	needHash := lang != nil && (lang.Name == "c_sharp" || lang.Name == "bash")
	for i := range stacks {
		s := &stacks[i]
		top := s.top()
		flags := uint8(0)
		if s.dead {
			flags |= stackCullDeadFlag
		}
		if s.accepted {
			flags |= stackCullAcceptedFlag
		}
		if s.shifted {
			flags |= stackCullShiftedFlag
		}
		errorRank := uint8(0)
		if top.node != nil && top.node.hasError {
			errorRank = 1
		}
		keys[i] = stackCullKey{
			state:      top.state,
			byteOffset: s.byteOffset,
			score:      s.score,
			depth:      s.depth(),
			errorRank:  errorRank,
			flags:      flags,
		}
		if needHash {
			keys[i].hash = stackHash(*s)
		}
	}
	return keys
}

func compareStackCullKeys(lang *Language, a, b stackCullKey) int {
	useHashTieBreak := lang != nil && (lang.Name == "c_sharp" || lang.Name == "bash")
	aDead := a.flags&stackCullDeadFlag != 0
	bDead := b.flags&stackCullDeadFlag != 0
	if aDead != bDead {
		if aDead {
			return -1
		}
		return 1
	}
	aAccepted := a.flags&stackCullAcceptedFlag != 0
	bAccepted := b.flags&stackCullAcceptedFlag != 0
	if aAccepted != bAccepted {
		if aAccepted {
			return 1
		}
		return -1
	}
	if a.errorRank != b.errorRank {
		if a.errorRank < b.errorRank {
			return 1
		}
		return -1
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	if !useHashTieBreak {
		aShifted := a.flags&stackCullShiftedFlag != 0
		bShifted := b.flags&stackCullShiftedFlag != 0
		if aShifted != bShifted {
			if !aShifted {
				return 1
			}
			return -1
		}
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if a.depth != b.depth {
		if lang != nil && lang.Name == "bash" {
			if a.depth < b.depth {
				return 1
			}
			return -1
		}
		if a.depth > b.depth {
			return 1
		}
		return -1
	}
	if useHashTieBreak {
		aShifted := a.flags&stackCullShiftedFlag != 0
		bShifted := b.flags&stackCullShiftedFlag != 0
		if aShifted != bShifted {
			if !aShifted {
				return 1
			}
			return -1
		}
		if a.hash > b.hash {
			return 1
		}
		if a.hash < b.hash {
			return -1
		}
	}
	return 0
}

func retainTopStacks(stacks []glrStack, keep int) []glrStack {
	return retainTopStacksForLanguage(stacks, keep, nil)
}

func retainTopStacksForLanguage(stacks []glrStack, keep int, lang *Language) []glrStack {
	return retainTopStacksForLanguageWithScratch(stacks, keep, lang, nil, nil, nil, nil)
}

func retainTopStacksForLanguageWithScratch(stacks []glrStack, keep int, lang *Language, selectedBuf *[]int, chosenBuf *[]bool, keyBuf *[]stackCullKey, stateBuf *[]StateID) []glrStack {
	if keep <= 0 {
		return stacks[:0]
	}
	if len(stacks) <= keep {
		return stacks
	}
	if keyBuf == nil || stateBuf == nil {
		return retainTopStacksForLanguageWithScratchLegacy(stacks, keep, lang, selectedBuf, chosenBuf)
	}
	keys := buildStackCullKeys(stacks, lang, keyBuf)

	// Preserve one strong representative per top state before filling the
	// remaining cap. Otherwise a burst of near-duplicate stacks from one state
	// can crowd out a shallower but semantically distinct branch.
	var selected []int
	if selectedBuf != nil {
		if cap(*selectedBuf) < len(stacks) {
			*selectedBuf = make([]int, 0, len(stacks))
		}
		selected = (*selectedBuf)[:0]
	} else {
		selected = make([]int, 0, len(stacks))
	}
	var states []StateID
	if stateBuf != nil {
		if cap(*stateBuf) < len(stacks) {
			*stateBuf = make([]StateID, 0, len(stacks))
		}
		states = (*stateBuf)[:0]
	} else {
		states = make([]StateID, 0, len(stacks))
	}
	for i := range stacks {
		state := keys[i].state
		bestIdx := -1
		for j := range states {
			if states[j] == state {
				bestIdx = j
				break
			}
		}
		if bestIdx >= 0 {
			if compareStackCullKeys(lang, keys[i], keys[selected[bestIdx]]) > 0 {
				selected[bestIdx] = i
			}
			continue
		}
		states = append(states, state)
		selected = append(selected, i)
	}
	for i := 0; i < len(selected); i++ {
		best := i
		for j := i + 1; j < len(selected); j++ {
			if compareStackCullKeys(lang, keys[selected[j]], keys[selected[best]]) > 0 {
				best = j
			}
		}
		if best != i {
			selected[i], selected[best] = selected[best], selected[i]
			states[i], states[best] = states[best], states[i]
		}
	}
	if len(selected) > keep {
		selected = selected[:keep]
		states = states[:keep]
	}

	var chosen []bool
	if chosenBuf != nil {
		if cap(*chosenBuf) < len(stacks) {
			*chosenBuf = make([]bool, len(stacks))
		}
		chosen = (*chosenBuf)[:len(stacks)]
		clear(chosen)
	} else {
		chosen = make([]bool, len(stacks))
	}
	for _, idx := range selected {
		chosen[idx] = true
	}
	for len(selected) < keep {
		best := -1
		for i := range stacks {
			if chosen[i] {
				continue
			}
			if best < 0 || compareStackCullKeys(lang, keys[i], keys[best]) > 0 {
				best = i
			}
		}
		if best < 0 {
			break
		}
		chosen[best] = true
		selected = append(selected, best)
	}
	for i := 0; i < len(selected); i++ {
		idx := selected[i]
		if idx == i {
			continue
		}
		stacks[i], stacks[idx] = stacks[idx], stacks[i]
		keys[i], keys[idx] = keys[idx], keys[i]
		for j := i + 1; j < len(selected); j++ {
			if selected[j] == i {
				selected[j] = idx
				break
			}
		}
	}
	return stacks[:len(selected)]
}

func retainTopStacksForLanguageWithScratchLegacy(stacks []glrStack, keep int, lang *Language, selectedBuf *[]int, chosenBuf *[]bool) []glrStack {
	if keep <= 0 {
		return stacks[:0]
	}
	if len(stacks) <= keep {
		return stacks
	}

	var selected []int
	if selectedBuf != nil {
		if cap(*selectedBuf) < len(stacks) {
			*selectedBuf = make([]int, 0, len(stacks))
		}
		selected = (*selectedBuf)[:0]
	} else {
		selected = make([]int, 0, len(stacks))
	}
	for i := range stacks {
		state := stacks[i].top().state
		seen := false
		for j := 0; j < i; j++ {
			if stacks[j].top().state == state {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		best := i
		for j := i + 1; j < len(stacks); j++ {
			if stacks[j].top().state != state {
				continue
			}
			if stackCompareForCull(lang, &stacks[j], &stacks[best]) > 0 {
				best = j
			}
		}
		selected = append(selected, best)
	}
	for i := 0; i < len(selected); i++ {
		best := i
		for j := i + 1; j < len(selected); j++ {
			if stackCompareForCull(lang, &stacks[selected[j]], &stacks[selected[best]]) > 0 {
				best = j
			}
		}
		if best != i {
			selected[i], selected[best] = selected[best], selected[i]
		}
	}
	if len(selected) > keep {
		selected = selected[:keep]
	}

	var chosen []bool
	if chosenBuf != nil {
		if cap(*chosenBuf) < len(stacks) {
			*chosenBuf = make([]bool, len(stacks))
		}
		chosen = (*chosenBuf)[:len(stacks)]
		clear(chosen)
	} else {
		chosen = make([]bool, len(stacks))
	}
	for _, idx := range selected {
		chosen[idx] = true
	}
	for len(selected) < keep {
		best := -1
		for i := range stacks {
			if chosen[i] {
				continue
			}
			if best < 0 || stackCompareForCull(lang, &stacks[i], &stacks[best]) > 0 {
				best = i
			}
		}
		if best < 0 {
			break
		}
		chosen[best] = true
		selected = append(selected, best)
	}
	for i := 0; i < len(selected); i++ {
		idx := selected[i]
		if idx == i {
			continue
		}
		stacks[i], stacks[idx] = stacks[idx], stacks[i]
		for j := i + 1; j < len(selected); j++ {
			if selected[j] == i {
				selected[j] = idx
				break
			}
		}
	}
	return stacks[:len(selected)]
}

func classifyConflictShape(actions []ParseAction) (rrConflict, rsConflict bool) {
	if len(actions) < 2 {
		return false, false
	}
	reduceCount := 0
	hasShift := false
	hasOther := false
	for i := range actions {
		switch actions[i].Type {
		case ParseActionReduce:
			reduceCount++
		case ParseActionShift:
			hasShift = true
		default:
			hasOther = true
		}
	}
	if hasOther || reduceCount == 0 {
		return false, false
	}
	if hasShift {
		return false, true
	}
	return reduceCount >= 2, false
}

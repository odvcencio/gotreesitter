package gotreesitter

import (
	"bytes"
	"fmt"
	"strings"
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
	language                            *Language
	reuseCursor                         reuseCursor
	reuseScratch                        reuseScratch
	reuseMu                             sync.Mutex
	reparseFactory                      normalizationTokenSourceFactory
	recoveryParser                      *Parser
	skipRecoveryReparse                 bool
	fullArenaHint                       uint32
	incrementalArenaHint                uint32
	fullGSSHint                         uint32
	incrementalGSSHint                  uint32
	rootSymbol                          Symbol
	hasRootSymbol                       bool
	hasRecoverState                     []bool
	hasRecoverSymbol                    []bool
	recoverByState                      [][]recoverSymbolAction
	hasKeywordState                     []bool
	forceRawSpanAll                     bool
	forceRawSpanTable                   []bool
	included                            []Range
	logger                              ParserLogger
	glrTrace                            bool // verbose GLR stack tracing
	maxConflictWidth                    int  // widest N-way conflict in the parse table
	timeoutMicros                       uint64
	cancellationFlag                    *uint32
	denseLimit                          int
	smallBase                           int
	smallLookup                         [][]smallActionPair
	smallTokenLookup                    [][]uint16
	reduceAliasSeq                      [][]Symbol
	aliasTargetSymbol                   []bool
	singleTokenWrapperSymbol            []bool
	reduceHasFields                     []bool
	fieldInheritedScratch               []bool
	fieldConflictedScratch              []bool
	reduceScratch                       *reduceBuildScratch
	currentExternalTokenCheckpoint      externalScannerCheckpoint
	currentExternalTokenCheckpointStart uint32
	currentExternalTokenCheckpointEnd   uint32
	currentExternalTokenCheckpointValid bool
}

var snippetParserPools sync.Map

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
	// maxConsecutiveMissingSingleShifts prevents single-stack recovery from
	// cycling forever by repeatedly inserting the same missing token before
	// the same lookahead without advancing the parse position.
	maxConsecutiveMissingSingleShifts = 16
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
	ReuseCursorNanos                   int64
	ReparseNanos                       int64
	ReusedSubtrees                     uint64
	ReusedBytes                        uint64
	NewNodesAllocated                  uint64
	ReuseUnsupported                   bool
	ReuseUnsupportedReason             string
	ReuseRejectDirty                   uint64
	ReuseRejectAncestorDirtyBeforeEdit uint64
	ReuseRejectHasError                uint64
	ReuseRejectInvalidSpan             uint64
	ReuseRejectOutOfBounds             uint64
	ReuseRejectRootNonLeafChanged      uint64
	ReuseRejectLargeNonLeaf            uint64
	RecoverSearches                    uint64
	RecoverStateChecks                 uint64
	RecoverStateSkips                  uint64
	RecoverSymbolSkips                 uint64
	RecoverLookups                     uint64
	RecoverHits                        uint64
	MaxStacksSeen                      int
	EntryScratchPeak                   uint64
	StopReason                         ParseStopReason
	TokensConsumed                     uint64
	LastTokenEndByte                   uint32
	ExpectedEOFByte                    uint32
	ArenaBytesAllocated                int64
	ScratchBytesAllocated              int64
	EntryScratchBytesAllocated         int64
	GSSBytesAllocated                  int64
	SingleStackIterations              int
	MultiStackIterations               int
	SingleStackTokens                  uint64
	MultiStackTokens                   uint64
	SingleStackGSSNodes                uint64
	MultiStackGSSNodes                 uint64
	GSSNodesAllocated                  uint64
	GSSNodesRetained                   uint64
	GSSNodesDroppedSameToken           uint64
	ParentNodesAllocated               uint64
	ParentNodesRetained                uint64
	ParentNodesDroppedSameToken        uint64
	LeafNodesAllocated                 uint64
	LeafNodesRetained                  uint64
	LeafNodesDroppedSameToken          uint64
	MergeStacksIn                      uint64
	MergeStacksOut                     uint64
	MergeSlotsUsed                     uint64
	GlobalCullStacksIn                 uint64
	GlobalCullStacksOut                uint64
}

type incrementalParseTiming struct {
	totalNanos                         int64
	reuseNanos                         int64
	reusedSubtrees                     uint64
	reusedBytes                        uint64
	newNodes                           uint64
	reuseUnsupported                   bool
	reuseUnsupportedReason             string
	reuseRejectDirty                   uint64
	reuseRejectAncestorDirtyBeforeEdit uint64
	reuseRejectHasError                uint64
	reuseRejectInvalidSpan             uint64
	reuseRejectOutOfBounds             uint64
	reuseRejectRootNonLeafChanged      uint64
	reuseRejectLargeNonLeaf            uint64
	recoverSearches                    uint64
	recoverStateChecks                 uint64
	recoverStateSkips                  uint64
	recoverSymbolSkips                 uint64
	recoverLookups                     uint64
	recoverHits                        uint64
	maxStacksSeen                      int
	entryScratchPeak                   uint64
	stopReason                         ParseStopReason
	tokensConsumed                     uint64
	lastTokenEndByte                   uint32
	expectedEOFByte                    uint32
	arenaBytesAllocated                int64
	scratchBytesAllocated              int64
	entryScratchBytesAllocated         uint64
	gssBytesAllocated                  uint64
	singleStackIterations              int
	multiStackIterations               int
	singleStackTokens                  uint64
	multiStackTokens                   uint64
	singleStackGSSNodes                uint64
	multiStackGSSNodes                 uint64
	gssNodesAllocated                  uint64
	gssNodesRetained                   uint64
	gssNodesDroppedSameToken           uint64
	parentNodesAllocated               uint64
	parentNodesRetained                uint64
	parentNodesDroppedSameToken        uint64
	leafNodesAllocated                 uint64
	leafNodesRetained                  uint64
	leafNodesDroppedSameToken          uint64
	mergeStacksIn                      uint64
	mergeStacksOut                     uint64
	mergeSlotsUsed                     uint64
	globalCullStacksIn                 uint64
	globalCullStacksOut                uint64
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
		if lang.Name == "cobol" || lang.Name == "COBOL" {
			for i, name := range lang.SymbolNames {
				if !strings.HasSuffix(name, "_division") &&
					!strings.Contains(name, "_statement") &&
					!strings.HasSuffix(name, "_option") &&
					!strings.HasSuffix(name, "_clause") &&
					!strings.HasSuffix(name, "_section") &&
					!strings.HasSuffix(name, "_paragraph") {
					continue
				}
				if p.forceRawSpanTable == nil {
					p.forceRawSpanTable = make([]bool, len(lang.SymbolNames))
				}
				p.forceRawSpanTable[i] = true
			}
		}
		if lang.LargeStateCount > 0 {
			p.denseLimit = int(lang.LargeStateCount)
		} else {
			p.denseLimit = len(lang.ParseTable)
		}
		p.smallBase = int(lang.LargeStateCount)
		if len(lang.SmallParseTableMap) > 0 && len(lang.SmallParseTable) > 0 {
			p.smallTokenLookup = buildSmallTokenLookup(lang)
			p.smallLookup = buildSmallLookup(lang, p.smallTokenLookup)
		}
		p.reduceAliasSeq = buildReduceAliasSequences(lang)
		p.aliasTargetSymbol = buildAliasTargetSymbols(lang)
		p.singleTokenWrapperSymbol = buildSingleTokenWrapperSymbols(lang)
		p.reduceHasFields = buildReduceFieldPresence(lang)
		p.recoverByState, p.hasRecoverState, p.hasRecoverSymbol = buildRecoverActionsByState(lang)
		p.hasKeywordState = buildKeywordStates(lang)
		p.rootSymbol, p.hasRootSymbol = p.inferRootSymbol()
		p.maxConflictWidth = computeMaxConflictWidth(lang)
	}
	return p
}

func snippetParserPool(lang *Language) *sync.Pool {
	if lang == nil {
		return nil
	}
	if pool, ok := snippetParserPools.Load(lang); ok {
		return pool.(*sync.Pool)
	}
	pool := &sync.Pool{
		New: func() any {
			return NewParser(lang)
		},
	}
	actual, _ := snippetParserPools.LoadOrStore(lang, pool)
	return actual.(*sync.Pool)
}

func acquireSnippetParser(lang *Language) *Parser {
	pool := snippetParserPool(lang)
	if pool == nil {
		return nil
	}
	parser, _ := pool.Get().(*Parser)
	if parser == nil {
		parser = NewParser(lang)
	}
	resetSnippetParser(parser)
	return parser
}

func releaseSnippetParser(parser *Parser) {
	if parser == nil || parser.language == nil {
		return
	}
	resetSnippetParser(parser)
	if pool := snippetParserPool(parser.language); pool != nil {
		pool.Put(parser)
	}
}

func resetSnippetParser(parser *Parser) {
	if parser == nil {
		return
	}
	parser.reparseFactory = nil
	parser.recoveryParser = nil
	parser.skipRecoveryReparse = false
	parser.fullArenaHint = 0
	parser.incrementalArenaHint = 0
	parser.fullGSSHint = 0
	parser.incrementalGSSHint = 0
	parser.included = nil
	parser.logger = nil
	parser.glrTrace = false
	parser.timeoutMicros = 0
	parser.cancellationFlag = nil
	// Release *Node refs so the arenas from the last incremental parse can be
	// collected by the GC. Without this, a Parser sitting in a sync.Pool keeps
	// its reuseCursor.topLevel/*Node alive, preventing arena reclamation.
	parser.reuseCursor.releaseNodeRefs()
	parser.reuseScratch.releaseNodeRefs()
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

// tryRelexBroadDFA attempts to re-lex the current token position using the
// layout fallback lex state (state 0's DFA), which includes ALL terminal
// symbols. If it produces a different token that has valid actions in the
// current parser state, return it. This handles cases where the per-state
// lex mode's IMMTOKEN catch-all consumed input meant for a keyword.
func (p *Parser) tryRelexBroadDFA(tok Token, parserState StateID, ts TokenSource) (Token, bool) {
	if p == nil || p.language == nil || ts == nil {
		return Token{}, false
	}
	dts, ok := ts.(*dfaTokenSource)
	if !ok || dts == nil || dts.lexer == nil {
		return Token{}, false
	}
	// Only try if the token is an immediate token
	if int(tok.Symbol) >= len(p.language.ImmediateTokens) || !p.language.ImmediateTokens[tok.Symbol] {
		return Token{}, false
	}
	// Get the broad lex state (state 0's lex mode)
	if len(p.language.LexModes) == 0 {
		return Token{}, false
	}
	broadLS := p.language.LexModes[0].LexState

	// Save lexer state
	savedPos, savedRow, savedCol := dts.lexer.pos, dts.lexer.row, dts.lexer.col

	// Re-lex from token start with broad DFA
	dts.lexer.pos = int(tok.StartByte)
	tok2 := dts.lexer.Next(broadLS)

	if tok2.Symbol > 0 && tok2.Symbol != tok.Symbol {
		// Check if the new token has actions in the current parser state
		actionIdx := p.lookupActionIndex(parserState, tok2.Symbol)
		if actionIdx != 0 && int(actionIdx) < len(p.language.ParseActions) &&
			len(p.language.ParseActions[actionIdx].Actions) > 0 {
			if p.glrTrace {
				fmt.Printf("  RELEX: %s(%d) → %s(%d) in state=%d\n",
					p.language.SymbolNames[tok.Symbol], tok.Symbol,
					p.language.SymbolNames[tok2.Symbol], tok2.Symbol,
					parserState)
			}
			return tok2, true
		}
	}

	// Restore lexer state
	dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
	return Token{}, false
}

// tryRelexCurrentStateDFA re-lexes from the current token start using the
// current parser state's DFA lex mode. This helps when a lookahead token was
// originally lexed under a pre-reduce state, but a reduce chain changes the
// parser state before the token is consumed.
func (p *Parser) tryRelexCurrentStateDFA(tok Token, parserState StateID, ts TokenSource) (Token, bool) {
	if p == nil || p.language == nil || ts == nil || tok.NoLookahead {
		return Token{}, false
	}
	dts, ok := ts.(*dfaTokenSource)
	if !ok || dts == nil || dts.lexer == nil || dts.language == nil {
		return Token{}, false
	}
	if tok.Symbol == 0 && tok.StartByte >= uint32(len(dts.lexer.source)) {
		return Token{}, false
	}
	if int(parserState) >= len(p.language.LexModes) {
		return Token{}, false
	}
	if dts.language.ExternalScanner != nil && p.language.LexModes[parserState].ExternalLexState != 0 {
		return Token{}, false
	}
	savedPos, savedRow, savedCol := dts.lexer.pos, dts.lexer.row, dts.lexer.col
	dts.lexer.pos = int(tok.StartByte)
	dts.lexer.row = tok.StartPoint.Row
	dts.lexer.col = tok.StartPoint.Column
	tok2, endPos, endRow, endCol := dts.scanPreferredTokenForState(parserState)
	if tok2.Symbol == 0 {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
		return Token{}, false
	}
	if tok2.Symbol == tok.Symbol && tok2.StartByte == tok.StartByte && tok2.EndByte == tok.EndByte {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
		return Token{}, false
	}
	actionIdx := p.lookupActionIndex(parserState, tok2.Symbol)
	if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) || len(p.language.ParseActions[actionIdx].Actions) == 0 {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
		return Token{}, false
	}
	dts.lexer.pos, dts.lexer.row, dts.lexer.col = endPos, endRow, endCol
	return tok2, true
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
				return false
			}
		}
	} else {
		for n := s.gss.head; n != nil; n = n.prev {
			if countNode(n.entry.node) {
				return false
			}
		}
	}

	if nonExtraCount == 0 {
		return true
	}
	if onlyNonExtra == nil || onlyNonExtra.symbol == errorSymbol {
		return false
	}
	return uint32(onlyNonExtra.symbol) >= tokenCount
}

func (p *Parser) tryInsertMissingSingleShift(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) bool {
	if p == nil || p.language == nil || s == nil || s.dead || tok.NoLookahead {
		return false
	}
	if tok.Symbol == 0 || uint32(tok.Symbol) >= p.language.TokenCount {
		return false
	}

	state := s.top().state
	var (
		candidateSym Symbol
		candidateAct ParseAction
		candidateCnt int
	)
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if sym == 0 || sym == tok.Symbol || uint32(sym) >= p.language.TokenCount {
			return true
		}
		if int(sym) >= len(p.language.SymbolMetadata) {
			return true
		}
		meta := p.language.SymbolMetadata[sym]
		if !meta.Visible || !meta.Named {
			return true
		}
		if int(idx) >= len(p.language.ParseActions) {
			return true
		}
		actions := p.language.ParseActions[idx].Actions
		if len(actions) != 1 {
			return true
		}
		act := actions[0]
		if act.Type != ParseActionShift || act.Extra {
			return true
		}
		if p.lookupActionIndex(act.State, tok.Symbol) == 0 {
			return true
		}
		candidateSym = sym
		candidateAct = act
		candidateCnt++
		return candidateCnt < 2
	})
	if candidateCnt != 1 {
		return false
	}

	missingTok := Token{
		Symbol:     candidateSym,
		StartByte:  tok.StartByte,
		EndByte:    tok.StartByte,
		StartPoint: tok.StartPoint,
		EndPoint:   tok.StartPoint,
		Missing:    true,
	}
	if top := s.top().node; top != nil && top.endByte <= tok.StartByte {
		missingTok.StartByte = top.endByte
		missingTok.EndByte = top.endByte
		missingTok.StartPoint = top.endPoint
		missingTok.EndPoint = top.endPoint
	}
	p.applyAction(s, candidateAct, missingTok, new(bool), nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
	s.shifted = false
	return true
}

func nodesFromStack(stack glrStack) []*Node {
	if len(stack.entries) > 0 {
		nodes := make([]*Node, 0, len(stack.entries))
		for _, entry := range stack.entries {
			if entry.node != nil {
				nodes = append(nodes, entry.node)
			}
		}
		return nodes
	}
	return nodesFromGSS(stack.gss)
}

func trimTrailingRecoveryEOFErrors(nodes []*Node, eofByte uint32) []*Node {
	end := len(nodes)
	for end > 0 {
		n := nodes[end-1]
		if n == nil || n.symbol != errorSymbol || n.startByte != eofByte || n.endByte != eofByte {
			break
		}
		if len(n.children) == 0 {
			end--
			continue
		}
		if len(n.children) != 1 {
			break
		}
		child := n.children[0]
		if child == nil || child.symbol != errorSymbol || child.startByte != eofByte || child.endByte != eofByte {
			break
		}
		end--
	}
	return nodes[:end]
}

func trimRecoveryWhitespaceTail(n *Node, source []byte) {
	if n == nil {
		return
	}
	for _, child := range n.children {
		trimRecoveryWhitespaceTail(child, source)
	}
	if len(n.children) == 0 {
		return
	}
	last := n.children[len(n.children)-1]
	if last == nil || n.endByte <= last.endByte || int(n.endByte) > len(source) || int(last.endByte) > len(source) {
		return
	}
	if len(bytes.TrimSpace(source[last.endByte:n.endByte])) != 0 {
		return
	}
	n.endByte = last.endByte
	n.endPoint = last.endPoint
}

func findVisibleSymbolByName(lang *Language, name string, named bool) (Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	for i, symName := range lang.SymbolNames {
		if symName != name || i >= len(lang.SymbolMetadata) {
			continue
		}
		meta := lang.SymbolMetadata[i]
		if !meta.Visible || meta.Named != named {
			continue
		}
		return Symbol(i), true
	}
	return 0, false
}

func normalizeSQLRecoveredMissingNull(root *Node, arena *nodeArena, lang *Language) {
	if root == nil || arena == nil || lang == nil || lang.Name != "sql" {
		return
	}
	nullParentSym, ok := findVisibleSymbolByName(lang, "NULL", true)
	if !ok {
		return
	}
	nullLeafSym, ok := findVisibleSymbolByName(lang, "NULL", false)
	if !ok {
		return
	}
	numberSym, ok := findVisibleSymbolByName(lang, "number", true)
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(parent *Node) {
		if parent == nil {
			return
		}
		for i, child := range parent.children {
			if child == nil {
				continue
			}
			walk(child)
			if parent.Type(lang) != "select_clause_body" || !child.isMissing || child.symbol != numberSym {
				continue
			}
			leaf := newLeafNodeInArena(arena, nullLeafSym, false, child.startByte, child.endByte, child.startPoint, child.endPoint)
			leaf.isMissing = true
			leaf.hasError = true
			repl := newParentNodeInArena(arena, nullParentSym, true, []*Node{leaf}, nil, 0)
			repl.hasError = true
			parent.children[i] = repl
		}
	}
	walk(root)
}

func (p *Parser) tryAdvanceEOFOnSingleStack(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry) bool {
	if p == nil || p.language == nil || s == nil || s.dead || s.depth() == 0 {
		return false
	}
	parseActions := p.language.ParseActions
	anyReduced := false
	const maxEOFRecoverySteps = 256
	for steps := 0; steps < maxEOFRecoverySteps; steps++ {
		if s.accepted {
			return true
		}
		actionIdx := p.lookupActionIndex(s.top().state, 0)
		if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
			return p.canFinalizeNoActionEOF(s)
		}
		actions := parseActions[actionIdx].Actions
		if len(actions) != 1 {
			return false
		}
		act := actions[0]
		switch act.Type {
		case ParseActionReduce:
			p.applyAction(s, act, tok, &anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, false, nil)
			if s.dead {
				return false
			}
		case ParseActionAccept:
			s.accepted = true
			return true
		default:
			return false
		}
	}
	return false
}

func (p *Parser) tryInsertMissingSingleShiftAtEOF(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch) bool {
	if p == nil || p.language == nil || s == nil || s.dead || tok.NoLookahead || tok.Symbol != 0 || tok.StartByte != tok.EndByte {
		return false
	}

	state := s.top().state
	var (
		candidateSym Symbol
		candidateAct ParseAction
		candidateCnt int
	)
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if sym == 0 || uint32(sym) >= p.language.TokenCount {
			return true
		}
		if int(sym) >= len(p.language.SymbolMetadata) {
			return true
		}
		meta := p.language.SymbolMetadata[sym]
		if !meta.Visible || !meta.Named {
			return true
		}
		if int(idx) >= len(p.language.ParseActions) {
			return true
		}
		actions := p.language.ParseActions[idx].Actions
		if len(actions) != 1 {
			return true
		}
		act := actions[0]
		if act.Type != ParseActionShift || act.Extra {
			return true
		}
		if p.lookupActionIndex(act.State, 0) == 0 {
			return true
		}
		candidateSym = sym
		candidateAct = act
		candidateCnt++
		return candidateCnt < 2
	})
	if candidateCnt != 1 {
		return false
	}

	missingTok := Token{
		Symbol:     candidateSym,
		StartByte:  tok.StartByte,
		EndByte:    tok.StartByte,
		StartPoint: tok.StartPoint,
		EndPoint:   tok.StartPoint,
		Missing:    true,
	}
	p.applyAction(s, candidateAct, missingTok, new(bool), nodeCount, arena, entryScratch, gssScratch, nil, false, nil)
	s.shifted = false
	return true
}

func (p *Parser) tryRecoverTrailingEOFSuffix(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, source []byte) ([]*Node, bool) {
	if p == nil || s == nil || s.dead || tok.Symbol != 0 || tok.StartByte != tok.EndByte {
		return nil, false
	}
	entries := s.entries
	borrowed := false
	if entries == nil {
		tmp := []stackEntry(nil)
		if tmpEntries != nil {
			tmp = *tmpEntries
		}
		entries, borrowed = s.entriesForRead(tmp)
	}
	if borrowed && tmpEntries != nil {
		defer func() {
			*tmpEntries = entries[:0]
		}()
	}
	if len(entries) == 0 {
		return nil, false
	}

	for firstDrop := len(entries) - 1; firstDrop >= 0; firstDrop-- {
		node := entries[firstDrop].node
		if node == nil || node.isExtra {
			continue
		}
		if firstDrop == 0 {
			continue
		}
		cuts := []int{firstDrop}
		if !node.isNamed {
			cut := firstDrop + 1
			for cut < len(entries) {
				trailing := entries[cut].node
				if trailing == nil || !trailing.isExtra {
					break
				}
				cut++
			}
			if cut > firstDrop && cut <= len(entries) {
				cuts = append(cuts, cut)
			}
		}
		for _, cut := range cuts {
			prefix := s.cloneWithScratch(gssScratch)
			if !prefix.truncate(cut) {
				continue
			}
			prefixEOF := tok
			switch {
			case cut > 0 && entries[cut-1].node != nil:
				last := entries[cut-1].node
				prefixEOF.StartByte = last.endByte
				prefixEOF.EndByte = last.endByte
				prefixEOF.StartPoint = last.endPoint
				prefixEOF.EndPoint = last.endPoint
			default:
				prefixEOF.StartByte = node.startByte
				prefixEOF.EndByte = node.startByte
				prefixEOF.StartPoint = node.startPoint
				prefixEOF.EndPoint = node.startPoint
			}
			insertedMissing := false
			if !p.tryAdvanceEOFOnSingleStack(&prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch, tmpEntries) {
				if !p.tryInsertMissingSingleShiftAtEOF(&prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch) {
					continue
				}
				insertedMissing = true
				if !p.tryAdvanceEOFOnSingleStack(&prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch, tmpEntries) {
					continue
				}
			}

			nodes := nodesFromStack(prefix)
			if p.hasRootSymbol && len(nodes) == 1 && nodes[0] != nil && nodes[0].symbol == p.rootSymbol {
				nodes = append([]*Node(nil), nodes[0].children...)
			}
			if insertedMissing || cut > firstDrop {
				nodes = trimTrailingRecoveryEOFErrors(nodes, tok.StartByte)
				for _, n := range nodes {
					trimRecoveryWhitespaceTail(n, source)
				}
			}
			recovered := false
			for i := cut; i < len(entries); i++ {
				trailing := entries[i].node
				if trailing == nil {
					continue
				}
				if trailing.symbol == errorSymbol && trailing.startByte == tok.StartByte && trailing.endByte == tok.EndByte {
					continue
				}
				if !recovered && !trailing.isExtra {
					errNode := newParentNodeInArena(arena, errorSymbol, true, []*Node{trailing}, nil, 0)
					errNode.hasError = true
					errNode.isExtra = true
					nodes = append(nodes, errNode)
					recovered = true
					if nodeCount != nil {
						*nodeCount = *nodeCount + 1
					}
					continue
				}
				nodes = append(nodes, trailing)
			}
			if recovered || insertedMissing || cut > firstDrop {
				return nodes, true
			}
		}
	}
	return nil, false
}

func (p *Parser) parseIncrementalInternal(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming) *Tree {
	// Fast path: unchanged source and no recorded edits.
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree
	}
	if tree, ok := p.tryTokenInvariantLeafEdit(source, oldTree, ts, timing); ok {
		return tree
	}

	// Subtree reuse is safe for DFA token sources without external scanners
	// and for custom token sources that explicitly opt in.
	if !tokenSourceSupportsIncrementalReuse(ts) {
		if timing != nil {
			timing.reuseUnsupported = true
			timing.reuseUnsupportedReason = incrementalReuseUnavailableReason(ts)
		}
		// When subtree reuse is unavailable, incremental reparses should behave
		// like ordinary full parses, including retry widening. This keeps
		// conservative fallback paths for external-scanner languages on the same
		// correctness footing as Parse.
		deterministicExternalConflicts := fullParseUsesDeterministicExternalConflicts(p.language)
		initialMaxStacks := fullParseInitialMaxStacks(p.language, p.maxConflictWidth)
		tree := p.parseInternal(source, ts, nil, nil, arenaClassFull, timing, initialMaxStacks, 0, 0, deterministicExternalConflicts)
		tree = p.retryFullParseWithTokenSource(source, ts, initialMaxStacks, deterministicExternalConflicts, tree)
		if shouldRepeatExternalScannerFullParse(p.language, tree) {
			tree = p.retryFullParseWithTokenSource(source, ts, initialMaxStacks, deterministicExternalConflicts, tree)
		}
		return tree
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
			timing.reuseRejectDirty += reuse.rejectDirty
			timing.reuseRejectAncestorDirtyBeforeEdit += reuse.rejectAncestorDirtyBeforeEdit
			timing.reuseRejectHasError += reuse.rejectHasError
			timing.reuseRejectInvalidSpan += reuse.rejectInvalidSpan
			timing.reuseRejectOutOfBounds += reuse.rejectOutOfBounds
			timing.reuseRejectRootNonLeafChanged += reuse.rejectRootNonLeafChanged
			timing.reuseRejectLargeNonLeaf += reuse.rejectLargeNonLeaf
		}
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
		return languageSupportsIncrementalReuse(dts.language)
	}
	if reusable, ok := ts.(IncrementalReuseTokenSource); ok {
		return reusable.SupportsIncrementalReuse()
	}
	return false
}

func languageSupportsIncrementalReuse(lang *Language) bool {
	if lang == nil {
		return false
	}
	if lang.ExternalScanner == nil {
		return true
	}
	if reusable, ok := lang.ExternalScanner.(IncrementalReuseExternalScanner); ok {
		return reusable.SupportsIncrementalReuse()
	}
	return false
}

func incrementalReuseUnavailableReason(ts TokenSource) string {
	if ts == nil {
		return "token_source_nil"
	}
	if dts, ok := ts.(*dfaTokenSource); ok {
		if dts.language == nil {
			return "dfa_token_source_no_language"
		}
		if languageSupportsIncrementalReuse(dts.language) {
			return ""
		}
		if dts.language.ExternalScanner != nil {
			return "external_scanner_unsupported"
		}
		return "dfa_token_source_no_reuse"
	}
	if _, ok := ts.(IncrementalReuseTokenSource); ok {
		return ""
	}
	return "token_source_no_incremental_reuse"
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

func (p *Parser) clearCurrentExternalTokenCheckpoint() {
	if p == nil {
		return
	}
	p.currentExternalTokenCheckpoint = externalScannerCheckpoint{}
	p.currentExternalTokenCheckpointStart = 0
	p.currentExternalTokenCheckpointEnd = 0
	p.currentExternalTokenCheckpointValid = false
}

func (p *Parser) updateCurrentExternalTokenCheckpoint(ts TokenSource, tok Token) {
	if p == nil {
		return
	}
	p.clearCurrentExternalTokenCheckpoint()
	cp, startByte, endByte, ok := currentExternalScannerCheckpoint(ts)
	if !ok || tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return
	}
	if startByte != tok.StartByte || endByte != tok.EndByte {
		return
	}
	p.currentExternalTokenCheckpoint = cp
	p.currentExternalTokenCheckpointStart = startByte
	p.currentExternalTokenCheckpointEnd = endByte
	p.currentExternalTokenCheckpointValid = true
}

func (p *Parser) recordCurrentExternalLeafCheckpoint(node *Node, tok Token) {
	if p == nil || node == nil || !p.currentExternalTokenCheckpointValid {
		return
	}
	if tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return
	}
	if node.ownerArena == nil {
		return
	}
	if node.StartByte() != p.currentExternalTokenCheckpointStart || node.EndByte() != p.currentExternalTokenCheckpointEnd {
		return
	}
	node.ownerArena.recordExternalScannerLeafCheckpoint(node, p.currentExternalTokenCheckpoint.start, p.currentExternalTokenCheckpoint.end)
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

func resolveParseMaxStacks(configuredMaxStacks, maxStacksOverride, conflictWidth int) (maxStacks int, retryPass bool) {
	maxStacks = configuredMaxStacks
	if maxStacks <= 0 {
		maxStacks = maxGLRStacks
	}
	if maxStacksOverride > 0 {
		maxStacks = maxStacksOverride
		retryPass = maxStacksOverride > configuredMaxStacks
	}
	if conflictWidth > maxStacks {
		maxStacks = conflictWidth
	}
	return maxStacks, retryPass
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
	p.clearCurrentExternalTokenCheckpoint()
	if p.logger != nil {
		p.logf(ParserLogParse, "start len=%d incremental=%t", len(source), reuse != nil || oldTree != nil)
	}
	deferParentLinks := reuse == nil && oldTree == nil
	scratch := acquireParserScratch()
	scratch.merge.beginEquivEpoch()
	if deferParentLinks {
		scratch.gss.initialCap = p.fullGSSHintCapacity()
	} else {
		scratch.gss.initialCap = p.incrementalGSSHintCapacity()
	}
	defer releaseParserScratch(scratch, deferParentLinks)
	p.reduceScratch = &scratch.reduce
	defer func() {
		p.reduceScratch = nil
	}()
	scratch.audit.beginParse()
	scratch.merge.audit = nil
	scratch.gss.audit = nil
	trackChildErrors := !deferParentLinks

	arena := acquireNodeArena(arenaClass)
	arena.skipChildClear = reuse == nil && oldTree == nil
	arena.audit = nil
	if scratch.audit.enabled {
		scratch.merge.audit = &scratch.audit
		scratch.gss.audit = &scratch.audit
		arena.audit = &scratch.audit
	}
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
			p.recordFullGSSUsage(scratch.gss.usedTotal)
		}()
	} else {
		defer func() {
			p.recordIncrementalArenaUsage(arena.used)
			p.recordIncrementalGSSUsage(scratch.gss.usedTotal)
		}()
	}
	switch arenaClass {
	case arenaClassFull:
		target := parseFullArenaNodeCapacity(len(source), p.fullArenaHintCapacity())
		arena.ensureNodeCapacity(target)
		scratch.entries.ensureInitialCap(parseFullEntryScratchCapacity(len(source)))
	case arenaClassIncremental:
		target := parseIncrementalArenaNodeCapacity(len(source), p.incrementalArenaHintCapacity())
		arena.ensureNodeCapacity(target)
		scratch.entries.ensureInitialCap(parseIncrementalEntryScratchCapacity(len(source)))
	}
	arena.setBudget(parseMemoryBudget(len(source)))
	scratch.setBudget(parseMemoryBudget(len(source)))
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
	singleStackIterations := 0
	multiStackIterations := 0
	var singleStackTokens uint64
	var multiStackTokens uint64
	expectedEOFByte := uint32(len(source))
	if len(p.included) > 0 {
		expectedEOFByte = p.included[len(p.included)-1].EndByte
	}
	var stacks []glrStack
	parseRuntime := ParseRuntime{
		StopReason:        ParseStopNone,
		SourceLen:         uint32(len(source)),
		ExpectedEOFByte:   expectedEOFByte,
		MemoryBudgetBytes: arena.budgetBytes,
	}
	arenaStatsCaptured := false
	captureArenaStats := func() {
		if arenaStatsCaptured || arena == nil {
			return
		}
		parseRuntime.ArenaBytesAllocated = arena.allocatedBytes
		parseRuntime.MemoryBudgetBytes = arena.budgetBytes
		arenaStatsCaptured = true
	}
	scratchStatsCaptured := false
	captureScratchStats := func() {
		if scratchStatsCaptured || scratch == nil {
			return
		}
		parseRuntime.ScratchBytesAllocated = scratch.allocatedBytes()
		parseRuntime.EntryScratchBytesAllocated = scratch.entries.allocatedBytes
		parseRuntime.GSSBytesAllocated = scratch.gss.allocatedBytes
		scratchStatsCaptured = true
	}
	finalizeTree := func(tree *Tree, stopReason ParseStopReason) *Tree {
		scratch.audit.finishParse(stacks)
		captureArenaStats()
		captureScratchStats()
		if tokenSourceEOFEarly && (stopReason == ParseStopAccepted || stopReason == ParseStopNone) {
			stopReason = ParseStopTokenSourceEOF
		}
		parseRuntime.StopReason = stopReason
		parseRuntime.Iterations = iterationsUsed
		parseRuntime.NodesAllocated = nodeCount
		parseRuntime.PeakStackDepth = peakStackDepth
		parseRuntime.MaxStacksSeen = maxStacksSeen
		parseRuntime.SingleStackIterations = singleStackIterations
		parseRuntime.MultiStackIterations = multiStackIterations
		parseRuntime.SingleStackTokens = singleStackTokens
		parseRuntime.MultiStackTokens = multiStackTokens
		parseRuntime.SingleStackGSSNodes = scratch.gss.singleStackAllocs
		parseRuntime.MultiStackGSSNodes = scratch.gss.multiStackAllocs
		parseRuntime.GSSNodesAllocated = scratch.audit.totalGSSAllocated
		parseRuntime.GSSNodesRetained = scratch.audit.totalGSSRetained
		parseRuntime.GSSNodesDroppedSameToken = scratch.audit.totalGSSDropped
		parseRuntime.ParentNodesAllocated = scratch.audit.totalParentAllocated
		parseRuntime.ParentNodesRetained = scratch.audit.totalParentRetained
		parseRuntime.ParentNodesDroppedSameToken = scratch.audit.totalParentDropped
		parseRuntime.LeafNodesAllocated = scratch.audit.totalLeafAllocated
		parseRuntime.LeafNodesRetained = scratch.audit.totalLeafRetained
		parseRuntime.LeafNodesDroppedSameToken = scratch.audit.totalLeafDropped
		parseRuntime.MergeStacksIn = scratch.audit.mergeStacksIn
		parseRuntime.MergeStacksOut = scratch.audit.mergeStacksOut
		parseRuntime.MergeSlotsUsed = scratch.audit.mergeSlotsUsed
		parseRuntime.GlobalCullStacksIn = scratch.audit.globalCullStacksIn
		parseRuntime.GlobalCullStacksOut = scratch.audit.globalCullStacksOut
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
		if timing != nil {
			timing.stopReason = parseRuntime.StopReason
			timing.tokensConsumed = parseRuntime.TokensConsumed
			timing.lastTokenEndByte = parseRuntime.LastTokenEndByte
			timing.expectedEOFByte = parseRuntime.ExpectedEOFByte
			timing.arenaBytesAllocated = parseRuntime.ArenaBytesAllocated
			timing.scratchBytesAllocated = parseRuntime.ScratchBytesAllocated
			timing.entryScratchBytesAllocated = uint64(parseRuntime.EntryScratchBytesAllocated)
			timing.gssBytesAllocated = uint64(parseRuntime.GSSBytesAllocated)
			timing.singleStackIterations = parseRuntime.SingleStackIterations
			timing.multiStackIterations = parseRuntime.MultiStackIterations
			timing.singleStackTokens = parseRuntime.SingleStackTokens
			timing.multiStackTokens = parseRuntime.MultiStackTokens
			timing.singleStackGSSNodes = parseRuntime.SingleStackGSSNodes
			timing.multiStackGSSNodes = parseRuntime.MultiStackGSSNodes
			timing.gssNodesAllocated = parseRuntime.GSSNodesAllocated
			timing.gssNodesRetained = parseRuntime.GSSNodesRetained
			timing.gssNodesDroppedSameToken = parseRuntime.GSSNodesDroppedSameToken
			timing.parentNodesAllocated = parseRuntime.ParentNodesAllocated
			timing.parentNodesRetained = parseRuntime.ParentNodesRetained
			timing.parentNodesDroppedSameToken = parseRuntime.ParentNodesDroppedSameToken
			timing.leafNodesAllocated = parseRuntime.LeafNodesAllocated
			timing.leafNodesRetained = parseRuntime.LeafNodesRetained
			timing.leafNodesDroppedSameToken = parseRuntime.LeafNodesDroppedSameToken
			timing.mergeStacksIn = parseRuntime.MergeStacksIn
			timing.mergeStacksOut = parseRuntime.MergeStacksOut
			timing.mergeSlotsUsed = parseRuntime.MergeSlotsUsed
			timing.globalCullStacksIn = parseRuntime.GlobalCullStacksIn
			timing.globalCullStacksOut = parseRuntime.GlobalCullStacksOut
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
		captureArenaStats()
		tree := p.buildResultFromGLR(stacks, source, arena, oldTree, &reuseState, &scratch.nodeLinks)
		return finalizeTree(tree, stopReason)
	}
	finalizeErrorTree := func(stopReason ParseStopReason) *Tree {
		captureArenaStats()
		arena.Release()
		return finalizeTree(parseErrorTree(source, p.language), stopReason)
	}
	finalizeRecoveredNodes := func(nodes []*Node) *Tree {
		captureArenaStats()
		tree := p.buildResultFromNodes(nodes, source, arena, oldTree, &reuseState, &scratch.nodeLinks)
		if root := tree.RootNode(); root != nil {
			normalizeSQLRecoveredMissingNull(root, arena, p.language)
			for _, child := range root.children {
				trimRecoveryWhitespaceTail(child, source)
			}
			wireParentLinksWithScratch(root, &scratch.nodeLinks)
		}
		return finalizeTree(tree, ParseStopAccepted)
	}
	tryFinalizeTrailingEOFSuffix := func(s *glrStack, tok Token) (*Tree, bool) {
		if nodes, ok := p.tryRecoverTrailingEOFSuffix(s, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, source); ok {
			return finalizeRecoveredNodes(nodes), true
		}
		return nil, false
	}

	var stacksBuf [4]glrStack
	stacks = stacksBuf[:1]
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
	maxStacks, retryPass := resolveParseMaxStacks(parseMaxGLRStacksValue(), maxStacksOverride, p.maxConflictWidth)
	mergePerKeyCap := parseMaxMergePerKeyValue()
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
		maxStacks, mergePerKeyCap = tuneIncrementalGLRCaps(maxStacks, mergePerKeyCap)
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
	parseRuntime.MemoryBudgetBytes = arena.budgetBytes

	needToken := true
	var tok Token
	var nextBranchOrder uint64 = 1

	// Per-primary-stack infinite-reduce detection.
	var lastReduceState StateID
	lastReduceDepth := -1
	var consecutiveReduces int
	var lastMissingShiftState StateID
	lastMissingShiftDepth := -1
	var lastMissingShiftSymbol Symbol
	var lastMissingShiftStartByte uint32
	var lastMissingShiftEndByte uint32
	var consecutiveMissingShifts int
	tryMissingSingleShift := func(stackIndex int, s *glrStack, currentState StateID) bool {
		missingShiftDepth := s.depth()
		if lastMissingShiftState == currentState &&
			lastMissingShiftDepth == missingShiftDepth &&
			lastMissingShiftSymbol == tok.Symbol &&
			lastMissingShiftStartByte == tok.StartByte &&
			lastMissingShiftEndByte == tok.EndByte &&
			consecutiveMissingShifts >= maxConsecutiveMissingSingleShifts {
			if p.glrTrace {
				fmt.Printf("  stack[%d] SKIP missing-shift cycle state=%d sym=%d byte=%d..%d count=%d\n",
					stackIndex, currentState, tok.Symbol, tok.StartByte, tok.EndByte, consecutiveMissingShifts)
			}
			return false
		}
		if !p.tryInsertMissingSingleShift(s, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &trackChildErrors) {
			return false
		}
		if lastMissingShiftState == currentState &&
			lastMissingShiftDepth == missingShiftDepth &&
			lastMissingShiftSymbol == tok.Symbol &&
			lastMissingShiftStartByte == tok.StartByte &&
			lastMissingShiftEndByte == tok.EndByte {
			consecutiveMissingShifts++
		} else {
			lastMissingShiftState = currentState
			lastMissingShiftDepth = missingShiftDepth
			lastMissingShiftSymbol = tok.Symbol
			lastMissingShiftStartByte = tok.StartByte
			lastMissingShiftEndByte = tok.EndByte
			consecutiveMissingShifts = 1
		}
		return true
	}

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
			cullIn := len(stacks)
			cullLang := stackCullLanguageForArena(p.language, arenaClass)
			stacks = retainTopStacksForLanguageWithScratch(stacks, maxStacks, cullLang, &scratch.stackPick, &scratch.stackKeep, &scratch.stackCull, &scratch.stateKeep)
			scratch.audit.recordGlobalCull(cullIn, len(stacks))
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
		scratch.gss.singleStackMode = len(stacks) == 1
		if scratch.gss.singleStackMode {
			singleStackIterations++
		} else {
			multiStackIterations++
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
		if arena.budgetExhausted() {
			return finalize(stacks, ParseStopMemoryBudget)
		}
		if scratch.budgetExhausted() {
			return finalize(stacks, ParseStopMemoryBudget)
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
			scratch.audit.startToken(stacks)
			if len(stacks) == 1 {
				singleStackTokens++
			} else {
				multiStackTokens++
			}
			tok = ts.Next()
			p.updateCurrentExternalTokenCheckpoint(ts, tok)
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
			lastMissingShiftDepth = -1
			consecutiveMissingShifts = 0
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
		forceAdvanceAfterReduce := false

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
		retryAction:
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
			// --- Extra token handling (comments, whitespace) ---
			if len(actions) > 0 &&
				actions[0].Type == ParseActionShift && actions[0].Extra {
				named := p.isNamedSymbol(tok.Symbol)
				leaf := newLeafNodeInArena(arena, tok.Symbol, named,
					tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
				if tok.Missing {
					leaf.isMissing = true
					leaf.hasError = true
				}
				leaf.isExtra = true
				leaf.preGotoState = currentState
				leaf.parseState = extraShiftTargetState(currentState, actions[0])
				p.recordCurrentExternalLeafCheckpoint(leaf, tok)
				p.pushStackNode(s, leaf.parseState, leaf, &scratch.entries, &scratch.gss)
				nodeCount++
				needToken = true
				continue
			}

			// --- No action: error handling ---
			if len(actions) == 0 {
				sameState := len(stacks) == 1
				if !sameState {
					sameState = true
					for sj := range stacks {
						if stacks[sj].dead {
							continue
						}
						if stacks[sj].top().state != currentState {
							sameState = false
							break
						}
					}
				}
				if tok.Symbol == 0 {
					// A stale DFA lookahead can surface as a zero-width EOF token
					// after a reduce chain changes the current state. Re-lex once
					// before accepting EOF so multiline constructs can continue.
					if sameState {
						if reTok, ok := p.tryRelexCurrentStateDFA(tok, currentState, ts); ok {
							tok = reTok
							needToken = false
							goto retryAction
						}
					}
					if tok.StartByte == tok.EndByte {
						// True EOF. If this is the only stack, return result when
						// the stack is in a state that can represent a complete root.
						if len(stacks) == 1 {
							if p.canFinalizeNoActionEOF(s) {
								return finalize(stacks, ParseStopAccepted)
							}
							if tree, ok := tryFinalizeTrailingEOFSuffix(s, tok); ok {
								return tree
							}
							s.dead = true
							continue
						}
						// Multiple stacks at EOF: this one is done.
						// Mark dead so merge picks the best remaining.
						s.dead = true
						continue
					}
					// Zero-symbol width token: skip.
					needToken = true
					continue
				}
				if tok.StartByte == tok.EndByte {
					// Layout and recovery helpers can emit zero-width internal
					// tokens (for example indentation markers). If the current
					// state cannot act on one, skipping it matches tree-sitter
					// better than materializing a zero-width error node that can
					// later corrupt reduce-child counting.
					needToken = true
					continue
				}
				// A token can be lexed under a pre-reduce DFA mode and become
				// invalid after all live stacks converge on the same reduced state.
				// Re-lex once under that current state before broader recovery.
				if sameState {
					if reTok, ok := p.tryRelexCurrentStateDFA(tok, currentState, ts); ok {
						tok = reTok
						needToken = false
						goto retryAction
					}
				}

				// Before killing a stack, try re-lexing with the broad
				// (state-0) lex mode. If the current lex mode's DFA
				// produced a token that's not valid in this state (e.g.,
				// an IMMTOKEN catch-all consumed a keyword), the broad
				// DFA may produce the correct token.
				if len(stacks) <= 1 {
					if reTok, ok := p.tryRelexBroadDFA(tok, currentState, ts); ok {
						tok = reTok
						needToken = false
						goto retryAction
					}
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
				if tryMissingSingleShift(si, s, currentState) {
					anyReduced = true
					needToken = false
					consecutiveReduces = 0
					continue
				}
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
					if p.applyActionWithReduceChain(s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors) {
						forceAdvanceAfterReduce = true
					}
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
		if numStacks > 1 && retryPass {
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
				if tryMissingSingleShift(bestIdx, &stacks[0], currentState) {
					anyReduced = true
					needToken = false
					consecutiveReduces = 0
				} else
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
			needToken = tok.NoLookahead || forceAdvanceAfterReduce

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
					if tok.Symbol == 0 && tok.StartByte == tok.EndByte && !tok.NoLookahead && len(stacks) == 1 {
						if tree, ok := tryFinalizeTrailingEOFSuffix(&stacks[0], tok); ok {
							return tree
						}
						if p.canFinalizeNoActionEOF(&stacks[0]) {
							return finalize(stacks, ParseStopAccepted)
						}
						return finalize(stacks, ParseStopNoStacksAlive)
					}
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

func stackCullLanguageForArena(lang *Language, class arenaClass) *Language {
	if class != arenaClassFull && lang != nil && lang.Name == "bash" {
		// Incremental culling historically used the generic stack comparator
		// for Bash. Keep that tie-break order while still reusing scratch.
		return nil
	}
	return lang
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

type stackCullKey struct {
	state      StateID
	byteOffset uint32
	score      int
	hash       uint64
	depth      int
	branch     uint64
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
			branch:     s.branchOrder,
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
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
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
	if a.branch != b.branch {
		if a.branch < b.branch {
			return 1
		}
		return -1
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
	compareLang := lang
	if keyBuf == nil || stateBuf == nil {
		// Preserve the former no-key fallback semantics. That path used the
		// C#-specific comparator, but all other languages followed the generic
		// stack comparator even if the keyed full-parse path has language
		// tie-breakers.
		if compareLang == nil || compareLang.Name != "c_sharp" {
			compareLang = nil
		}
	}
	keys := buildStackCullKeys(stacks, compareLang, keyBuf)
	return retainTopStacksByKeys(stacks, keep, compareLang, keys, selectedBuf, chosenBuf, stateBuf)
}

func retainTopStacksByKeys(stacks []glrStack, keep int, lang *Language, keys []stackCullKey, selectedBuf *[]int, chosenBuf *[]bool, stateBuf *[]StateID) []glrStack {
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

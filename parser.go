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
	reparseFactory                      TokenSourceFactory
	recoveryParser                      *Parser
	skipRecoveryReparse                 bool
	fullArenaHint                       uint32
	pendingFullArenaHint                uint32
	compactFullArenaHint                uint32
	finalChildRefArenaHint              uint32
	incrementalArenaHint                uint32
	fullGSSHint                         uint32
	incrementalGSSHint                  uint32
	rootSymbol                          Symbol
	hasRootSymbol                       bool
	hasRecoverState                     []bool
	hasRecoverSymbol                    []bool
	recoverByState                      [][]recoverSymbolAction
	hasKeywordState                     []bool
	typeScriptPropertyIdentifierSymbol  Symbol
	typeScriptIdentifierSymbol          Symbol
	typeScriptHasPropertyIdentifier     bool
	typeScriptHasIdentifier             bool
	typeScriptContextualPropertyKeyword map[string]Symbol
	forceRawSpanAll                     bool
	forceRawSpanTable                   []bool
	included                            []Range
	logger                              ParserLogger
	glrTrace                            bool // verbose GLR stack tracing
	ambiguityProfile                    *AmbiguityProfile
	maxConflictWidth                    int // widest N-way conflict in the parse table
	timeoutMicros                       uint64
	cancellationFlag                    *uint32
	denseLimit                          int
	smallBase                           int
	smallLookup                         [][]smallActionPair
	smallTokenLookup                    [][]uint16
	externalValidByState                [][]uint16
	classifiedActions                   []classifiedParseAction
	reduceChainHints                    []reduceChainHint
	reduceChainHintByState              []int
	reduceAliasSeq                      [][]Symbol
	aliasTargetSymbol                   []bool
	singleTokenWrapperSymbol            []bool
	reduceHasFields                     []bool
	fieldIDScratch                      []FieldID
	fieldInheritedScratch               []bool
	fieldConflictedScratch              []bool
	reduceScratch                       *reduceBuildScratch
	noTreeBenchmarkOnly                 bool
	noTreeCheckpointBenchmarkOnly       bool
	compactNoTreeShiftLeaves            bool
	compactFullShiftLeaves              bool
	pendingFullParents                  bool
	finalChildRefs                      bool
	skipInvisibleFullLeafCheckpoints    bool
	transientReduceChildren             bool
	transientReduceScratchNoAlias       bool
	transientChildren                   *transientChildScratch
	noResultCompatibilityBenchmarkOnly  bool
	currentExternalTokenCheckpoint      externalScannerCheckpoint
	currentExternalTokenCheckpointStart uint32
	currentExternalTokenCheckpointEnd   uint32
	currentExternalTokenCheckpointValid bool
	normalizationStats                  normalizationStats
	materializationTiming               *parseMaterializationTiming
	reduceTiming                        *parseMaterializationTiming
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
	ReuseCursorNanos                    int64
	ReparseNanos                        int64
	ReusedSubtrees                      uint64
	ReusedBytes                         uint64
	NewNodesAllocated                   uint64
	ReuseUnsupported                    bool
	ReuseUnsupportedReason              string
	ReuseRejectDirty                    uint64
	ReuseRejectAncestorDirtyBeforeEdit  uint64
	ReuseRejectHasError                 uint64
	ReuseRejectInvalidSpan              uint64
	ReuseRejectOutOfBounds              uint64
	ReuseRejectRootNonLeafChanged       uint64
	ReuseRejectLargeNonLeaf             uint64
	RecoverSearches                     uint64
	RecoverStateChecks                  uint64
	RecoverStateSkips                   uint64
	RecoverSymbolSkips                  uint64
	RecoverLookups                      uint64
	RecoverHits                         uint64
	MaxStacksSeen                       int
	EntryScratchPeak                    uint64
	StopReason                          ParseStopReason
	TokensConsumed                      uint64
	LastTokenEndByte                    uint32
	ExpectedEOFByte                     uint32
	ArenaBytesAllocated                 int64
	ScratchBytesAllocated               int64
	EntryScratchBytesAllocated          int64
	GSSBytesAllocated                   int64
	SingleStackIterations               int
	MultiStackIterations                int
	SingleStackTokens                   uint64
	MultiStackTokens                    uint64
	SingleStackGSSNodes                 uint64
	MultiStackGSSNodes                  uint64
	GSSNodesAllocated                   uint64
	GSSNodesRetained                    uint64
	GSSNodesDroppedSameToken            uint64
	ParentNodesAllocated                uint64
	ParentNodesRetained                 uint64
	ParentNodesDroppedSameToken         uint64
	LeafNodesAllocated                  uint64
	LeafNodesRetained                   uint64
	LeafNodesDroppedSameToken           uint64
	MergeStacksIn                       uint64
	MergeStacksOut                      uint64
	MergeSlotsUsed                      uint64
	GlobalCullStacksIn                  uint64
	GlobalCullStacksOut                 uint64
	ParserLoopNanos                     int64
	TokenNextNanos                      int64
	ActionDispatchNanos                 int64
	ActionLookupNanos                   int64
	GLRMergeNanos                       int64
	GLRCullNanos                        int64
	ResultSelectionNanos                int64
	TransientParentMaterializationNanos int64
	ResultTreeBuildNanos                int64
	TransientChildMaterializationNanos  int64
	ResultPythonKeywordRepairNanos      int64
	ResultPythonRootRepairNanos         int64
	ResultFinalizeRootNanos             int64
	ResultExtendTrailingNanos           int64
	ResultNormalizeRootStartNanos       int64
	ResultCompatibilityNanos            int64
	ResultParentLinkNanos               int64
	ReduceRangeNanos                    int64
	ReducePendingParentNanos            int64
	ReduceChildBuildNanos               int64
	ReduceParentBuildNanos              int64
	ReduceSpanNanos                     int64
	ReduceStackPushNanos                int64
	ReduceNoTreeBuildNanos              int64
	ActionExtraShiftNanos               int64
	ActionNoActionNanos                 int64
	ActionNoActionRelexNanos            int64
	ActionNoActionMissingNanos          int64
	ActionNoActionRecoverNanos          int64
	ActionNoActionErrorNanos            int64
	ActionConflictChoiceNanos           int64
	ActionConflictForkNanos             int64
	ActionSingleShiftNanos              int64
	ActionSingleReduceNanos             int64
	ActionSingleAcceptNanos             int64
	ActionSingleRecoverNanos            int64
	ActionSingleOtherNanos              int64
	NormalizationNanos                  int64
}

type incrementalParseTiming struct {
	totalNanos                          int64
	reuseNanos                          int64
	reusedSubtrees                      uint64
	reusedBytes                         uint64
	newNodes                            uint64
	reuseUnsupported                    bool
	reuseUnsupportedReason              string
	reuseRejectDirty                    uint64
	reuseRejectAncestorDirtyBeforeEdit  uint64
	reuseRejectHasError                 uint64
	reuseRejectInvalidSpan              uint64
	reuseRejectOutOfBounds              uint64
	reuseRejectRootNonLeafChanged       uint64
	reuseRejectLargeNonLeaf             uint64
	recoverSearches                     uint64
	recoverStateChecks                  uint64
	recoverStateSkips                   uint64
	recoverSymbolSkips                  uint64
	recoverLookups                      uint64
	recoverHits                         uint64
	maxStacksSeen                       int
	entryScratchPeak                    uint64
	stopReason                          ParseStopReason
	tokensConsumed                      uint64
	lastTokenEndByte                    uint32
	expectedEOFByte                     uint32
	arenaBytesAllocated                 int64
	scratchBytesAllocated               int64
	entryScratchBytesAllocated          uint64
	gssBytesAllocated                   uint64
	singleStackIterations               int
	multiStackIterations                int
	singleStackTokens                   uint64
	multiStackTokens                    uint64
	singleStackGSSNodes                 uint64
	multiStackGSSNodes                  uint64
	gssNodesAllocated                   uint64
	gssNodesRetained                    uint64
	gssNodesDroppedSameToken            uint64
	parentNodesAllocated                uint64
	parentNodesRetained                 uint64
	parentNodesDroppedSameToken         uint64
	leafNodesAllocated                  uint64
	leafNodesRetained                   uint64
	leafNodesDroppedSameToken           uint64
	mergeStacksIn                       uint64
	mergeStacksOut                      uint64
	mergeSlotsUsed                      uint64
	globalCullStacksIn                  uint64
	globalCullStacksOut                 uint64
	parserLoopNanos                     int64
	tokenNextNanos                      int64
	actionDispatchNanos                 int64
	actionLookupNanos                   int64
	glrMergeNanos                       int64
	glrCullNanos                        int64
	resultSelectionNanos                int64
	transientParentMaterializationNanos int64
	resultTreeBuildNanos                int64
	transientChildMaterializationNanos  int64
	resultPythonKeywordRepairNanos      int64
	resultPythonRootRepairNanos         int64
	resultFinalizeRootNanos             int64
	resultExtendTrailingNanos           int64
	resultNormalizeRootStartNanos       int64
	resultCompatibilityNanos            int64
	resultParentLinkNanos               int64
	reduceRangeNanos                    int64
	reducePendingParentNanos            int64
	reduceChildBuildNanos               int64
	reduceParentBuildNanos              int64
	reduceSpanNanos                     int64
	reduceStackPushNanos                int64
	reduceNoTreeBuildNanos              int64
	actionExtraShiftNanos               int64
	actionNoActionNanos                 int64
	actionNoActionRelexNanos            int64
	actionNoActionMissingNanos          int64
	actionNoActionRecoverNanos          int64
	actionNoActionErrorNanos            int64
	actionConflictChoiceNanos           int64
	actionConflictForkNanos             int64
	actionSingleShiftNanos              int64
	actionSingleReduceNanos             int64
	actionSingleAcceptNanos             int64
	actionSingleRecoverNanos            int64
	actionSingleOtherNanos              int64
	normalizationNanos                  int64
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
		if isCobolLanguage(lang) {
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
		p.externalValidByState = p.buildExternalValidByState()
		p.classifiedActions = buildClassifiedParseActions(lang)
		p.reduceChainHints = buildReduceChainHints(lang)
		p.reduceChainHintByState = buildReduceChainHintIndex(p.reduceChainHints)
		p.reduceAliasSeq = buildReduceAliasSequences(lang)
		p.aliasTargetSymbol = buildAliasTargetSymbols(lang)
		p.singleTokenWrapperSymbol = buildSingleTokenWrapperSymbols(lang)
		p.reduceHasFields = buildReduceFieldPresence(lang)
		p.recoverByState, p.hasRecoverState, p.hasRecoverSymbol = buildRecoverActionsByState(lang)
		p.hasKeywordState = buildKeywordStates(lang)
		p.initTypeScriptContextualKeywordSymbols(lang)
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
	parser.pendingFullArenaHint = 0
	parser.compactFullArenaHint = 0
	parser.finalChildRefArenaHint = 0
	parser.incrementalArenaHint = 0
	parser.fullGSSHint = 0
	parser.incrementalGSSHint = 0
	parser.included = nil
	parser.logger = nil
	parser.glrTrace = false
	parser.ambiguityProfile = nil
	parser.noTreeBenchmarkOnly = false
	parser.noTreeCheckpointBenchmarkOnly = false
	parser.compactNoTreeShiftLeaves = false
	parser.compactFullShiftLeaves = false
	parser.pendingFullParents = false
	parser.finalChildRefs = false
	parser.skipInvisibleFullLeafCheckpoints = false
	parser.noResultCompatibilityBenchmarkOnly = false
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
// lex mode's catch-all consumed input meant for a keyword/comment after
// reductions changed the parser state.
func (p *Parser) tryRelexBroadDFA(tok Token, parserState StateID, ts TokenSource) (Token, bool) {
	if p == nil || p.language == nil || ts == nil {
		return Token{}, false
	}
	dts, ok := ts.(*dfaTokenSource)
	if !ok || dts == nil || dts.lexer == nil {
		return Token{}, false
	}
	// Get the broad lex state (state 0's lex mode)
	if len(p.language.LexModes) == 0 {
		return Token{}, false
	}
	broadLS := p.language.LexModes[0].LexStateIndex()

	// Save lexer state
	savedPos, savedRow, savedCol := dts.lexer.pos, dts.lexer.row, dts.lexer.col

	type broadRelexCandidate struct {
		token      Token
		extraShift bool
	}

	tryAt := func(pos int, row, col uint32) (broadRelexCandidate, bool) {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = pos, row, col
		tok2 := dts.lexer.Next(broadLS)
		if tok2.Symbol == 0 {
			return broadRelexCandidate{}, false
		}
		// The broad lexer can skip extras before returning a token. Relexing is
		// only safe when any intentional layout skip is explicit at the call site.
		if int(tok2.StartByte) != pos {
			return broadRelexCandidate{}, false
		}
		actionIdx := p.lookupActionIndex(parserState, tok2.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) ||
			len(p.language.ParseActions[actionIdx].Actions) == 0 {
			return broadRelexCandidate{}, false
		}
		extraShift := false
		for _, action := range p.language.ParseActions[actionIdx].Actions {
			if action.Type == ParseActionShift && action.Extra {
				extraShift = true
				break
			}
		}
		if p.glrTrace {
			fmt.Printf("  RELEX: %s(%d) → %s(%d) in state=%d\n",
				p.language.SymbolNames[tok.Symbol], tok.Symbol,
				p.language.SymbolNames[tok2.Symbol], tok2.Symbol,
				parserState)
		}
		return broadRelexCandidate{token: tok2, extraShift: extraShift}, true
	}

	isImmediate := int(tok.Symbol) < len(p.language.ImmediateTokens) && p.language.ImmediateTokens[tok.Symbol]
	if isImmediate {
		if cand, ok := tryAt(int(tok.StartByte), tok.StartPoint.Row, tok.StartPoint.Column); ok {
			return cand.token, true
		}
	} else {
		if cand, ok := tryAt(int(tok.StartByte), tok.StartPoint.Row, tok.StartPoint.Column); ok && cand.extraShift {
			return cand.token, true
		}
	}

	skipPos, skipCol := int(tok.StartByte), tok.StartPoint.Column
	for skipPos < len(dts.lexer.source) &&
		(dts.lexer.source[skipPos] == ' ' || dts.lexer.source[skipPos] == '\t') {
		skipPos++
		skipCol++
	}
	if skipPos > int(tok.StartByte) {
		if cand, ok := tryAt(skipPos, tok.StartPoint.Row, skipCol); ok && (isImmediate || cand.extraShift) {
			return cand.token, true
		}
	}

	isStringContent := int(tok.Symbol) < len(p.language.SymbolNames) && p.language.SymbolNames[tok.Symbol] == "string_content"
	if isStringContent {
		skipPos, skipRow, skipCol := int(tok.StartByte), tok.StartPoint.Row, tok.StartPoint.Column
		for skipPos < len(dts.lexer.source) {
			b := dts.lexer.source[skipPos]
			if b == ' ' || b == '\t' {
				skipPos++
				skipCol++
				continue
			}
			if b == '\n' {
				skipPos++
				skipRow++
				skipCol = 0
				continue
			}
			if b == '\r' {
				skipPos++
				skipCol = 0
				continue
			}
			break
		}
		if skipPos > int(tok.StartByte) {
			if cand, ok := tryAt(skipPos, skipRow, skipCol); ok &&
				p.allowStringContentWhitespaceBroadRelex(cand.token.Symbol) {
				return cand.token, true
			}
		}
	}

	// Restore lexer state
	dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
	return Token{}, false
}

func (p *Parser) allowStringContentWhitespaceBroadRelex(candidate Symbol) bool {
	if p == nil || p.language == nil || int(candidate) >= len(p.language.SymbolNames) {
		return false
	}
	candidateName := p.language.SymbolNames[candidate]
	if candidateName != "b" && candidateName != "x" {
		return false
	}
	hasEscBlob, hasHexBlob := false, false
	for _, name := range p.language.SymbolNames {
		switch name {
		case "esc_blob":
			hasEscBlob = true
		case "hex_blob":
			hasHexBlob = true
		}
	}
	return hasEscBlob && hasHexBlob
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
	if dts.language.ExternalScanner != nil && p.language.LexModes[parserState].ExternalLexState != 0 &&
		!p.canRelexExternalTokenWithCurrentStateDFA(tok) {
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

func (p *Parser) canRelexExternalTokenWithCurrentStateDFA(tok Token) bool {
	if p == nil || p.language == nil || int(tok.Symbol) >= len(p.language.SymbolNames) {
		return false
	}
	if p.language.Name != "kotlin" {
		return false
	}
	// Kotlin's LALR table reuses states between package headers and import
	// lists. The external scanner can therefore produce import-only tokens
	// before reductions reveal that the current branch needs an ordinary DFA
	// token such as "." or "import".
	switch p.language.SymbolNames[tok.Symbol] {
	case "_import_dot", "_import_list_delimiter":
		return true
	default:
		return false
	}
}

func (p *Parser) canFinalizeNoActionEOF(s *glrStack) bool {
	if s == nil || s.dead {
		return false
	}
	top := s.top()
	if !stackEntryHasNode(top) {
		return true
	}

	tokenCount := uint32(0)
	if p != nil && p.language != nil {
		tokenCount = p.language.TokenCount
	}

	// Without an inferred root, the legacy behavior is still appropriate:
	// a single nonterminal at the top can serve as the final tree root.
	if p == nil || !p.hasRootSymbol {
		return p != nil && p.language != nil && uint32(stackEntryNodeSymbol(top)) >= tokenCount
	}

	nonExtraCount := 0
	onlyNonExtraSymbol := Symbol(0)
	countEntry := func(e stackEntry) bool {
		if !stackEntryHasNode(e) || stackEntryNodeIsExtra(e) {
			return false
		}
		nonExtraCount++
		onlyNonExtraSymbol = stackEntryNodeSymbol(e)
		return nonExtraCount > 1
	}

	if len(s.entries) > 0 {
		for i := range s.entries {
			if countEntry(s.entries[i]) {
				return false
			}
		}
	} else {
		for n := s.gss.head; n != nil; n = n.prev {
			if countEntry(n.entry) {
				return false
			}
		}
	}

	if nonExtraCount == 0 {
		return true
	}
	if onlyNonExtraSymbol == errorSymbol {
		return false
	}
	return uint32(onlyNonExtraSymbol) >= tokenCount
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
	if top := s.top(); stackEntryHasNode(top) && stackEntryNodeEndByte(top) <= tok.StartByte {
		missingTok.StartByte = stackEntryNodeEndByte(top)
		missingTok.EndByte = stackEntryNodeEndByte(top)
		missingTok.StartPoint = stackEntryNodeEndPoint(top)
		missingTok.EndPoint = stackEntryNodeEndPoint(top)
	}
	p.applyAction(s, candidateAct, missingTok, new(bool), nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
	s.shifted = false
	return true
}

func nodesFromStack(stack glrStack) []*Node {
	if len(stack.entries) > 0 {
		nodes := make([]*Node, 0, len(stack.entries))
		for _, entry := range stack.entries {
			if node := stackEntryNode(entry); node != nil {
				nodes = append(nodes, node)
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
	return lang.visibleSymbolByNameAndNamed(name, named)
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
			if parent.Type(lang) != "select_clause_body" || !child.isMissing() || child.symbol != numberSym {
				continue
			}
			leaf := newLeafNodeInArena(arena, nullLeafSym, false, child.startByte, child.endByte, child.startPoint, child.endPoint)
			leaf.setMissing(true)
			leaf.setHasError(true)
			repl := newParentNodeInArena(arena, nullParentSym, true, []*Node{leaf}, nil, 0)
			repl.setHasError(true)
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
		node := stackEntryNode(entries[firstDrop])
		if node == nil || node.isExtra() {
			continue
		}
		if firstDrop == 0 {
			continue
		}
		for _, cut := range trailingEOFSuffixCuts(entries, firstDrop, node) {
			prefix := s.cloneWithScratch(gssScratch)
			if !prefix.truncate(cut) {
				continue
			}
			prefixEOF := eofTokenForTrailingCut(tok, entries, cut, node)
			insertedMissing, advanced := p.advanceTrailingEOFPrefix(&prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch, tmpEntries)
			if !advanced {
				continue
			}

			nodes := p.trailingEOFNodesFromPrefix(prefix)
			if insertedMissing || cut > firstDrop {
				nodes = trimTrailingRecoveryEOFErrors(nodes, tok.StartByte)
				for _, n := range nodes {
					trimRecoveryWhitespaceTail(n, source)
				}
			}
			nodes, recovered := appendTrailingEOFRecoveryNodes(nodes, entries, cut, tok, arena, nodeCount)
			if recovered || insertedMissing || cut > firstDrop {
				return nodes, true
			}
		}
	}
	return nil, false
}

func trailingEOFSuffixCuts(entries []stackEntry, firstDrop int, node *Node) []int {
	cuts := []int{firstDrop}
	if node == nil || node.isNamed() {
		return cuts
	}
	cut := firstDrop + 1
	for cut < len(entries) {
		trailing := stackEntryNode(entries[cut])
		if trailing == nil || !trailing.isExtra() {
			break
		}
		cut++
	}
	if cut > firstDrop && cut <= len(entries) {
		cuts = append(cuts, cut)
	}
	return cuts
}

func eofTokenForTrailingCut(tok Token, entries []stackEntry, cut int, fallback *Node) Token {
	prefixEOF := tok
	if cut > 0 {
		if last := stackEntryNode(entries[cut-1]); last != nil {
			prefixEOF.StartByte = last.endByte
			prefixEOF.EndByte = last.endByte
			prefixEOF.StartPoint = last.endPoint
			prefixEOF.EndPoint = last.endPoint
			return prefixEOF
		}
	}
	prefixEOF.StartByte = fallback.startByte
	prefixEOF.EndByte = fallback.startByte
	prefixEOF.StartPoint = fallback.startPoint
	prefixEOF.EndPoint = fallback.startPoint
	return prefixEOF
}

func (p *Parser) advanceTrailingEOFPrefix(prefix *glrStack, prefixEOF Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry) (bool, bool) {
	if p.tryAdvanceEOFOnSingleStack(prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch, tmpEntries) {
		return false, true
	}
	if !p.tryInsertMissingSingleShiftAtEOF(prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch) {
		return false, false
	}
	if !p.tryAdvanceEOFOnSingleStack(prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch, tmpEntries) {
		return false, false
	}
	return true, true
}

func (p *Parser) trailingEOFNodesFromPrefix(prefix glrStack) []*Node {
	nodes := nodesFromStack(prefix)
	if p.hasRootSymbol && len(nodes) == 1 && nodes[0] != nil && nodes[0].symbol == p.rootSymbol {
		return append([]*Node(nil), nodes[0].children...)
	}
	return nodes
}

func appendTrailingEOFRecoveryNodes(nodes []*Node, entries []stackEntry, cut int, tok Token, arena *nodeArena, nodeCount *int) ([]*Node, bool) {
	recovered := false
	for i := cut; i < len(entries); i++ {
		trailing := stackEntryNode(entries[i])
		if trailing == nil {
			continue
		}
		if trailing.symbol == errorSymbol && trailing.startByte == tok.StartByte && trailing.endByte == tok.EndByte {
			continue
		}
		if !recovered && !trailing.isExtra() {
			errNode := newParentNodeInArena(arena, errorSymbol, true, []*Node{trailing}, nil, 0)
			errNode.setHasError(true)
			errNode.setExtra(true)
			nodes = append(nodes, errNode)
			recovered = true
			if nodeCount != nil {
				*nodeCount = *nodeCount + 1
			}
			continue
		}
		nodes = append(nodes, trailing)
	}
	return nodes, recovered
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
	if oldTree != nil {
		oldTree.ensureParentLinks()
	}
	if oldTree != nil {
		if languageUsesExternalScannerCheckpoints(p.language) {
			oldTree.ensureExternalScannerCheckpoints()
		}
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
	if p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly {
		return
	}
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
	if p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly {
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
	arena := node.ownerArena
	if p.skipInvisibleFullLeafCheckpoints && !p.isVisibleSymbol(tok.Symbol) {
		return
	}
	if !arena.recordExternalScannerLeafCheckpoint(node, p.currentExternalTokenCheckpoint.start, p.currentExternalTokenCheckpoint.end) {
		return
	}
	arena.externalScannerCheckpointLeafNodes++
}

func (p *Parser) currentExternalNoTreeLeafCheckpointRef(arena *nodeArena, tok Token) (externalScannerCheckpointRef, bool) {
	if p == nil || arena == nil || !p.currentExternalTokenCheckpointValid {
		return externalScannerCheckpointRef{}, false
	}
	if !p.noTreeCheckpointBenchmarkOnly {
		return externalScannerCheckpointRef{}, false
	}
	if tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return externalScannerCheckpointRef{}, false
	}
	if tok.StartByte != p.currentExternalTokenCheckpointStart || tok.EndByte != p.currentExternalTokenCheckpointEnd {
		return externalScannerCheckpointRef{}, false
	}
	cp := arena.recordExternalScannerCompactCheckpoint(
		p.currentExternalTokenCheckpoint.start,
		p.currentExternalTokenCheckpoint.end,
	)
	arena.compactFullLeafCreated++
	arena.checkpointLeafFullNodesAvoided++
	return cp, true
}

func (p *Parser) currentExternalCompactFullLeafCheckpointRef(arena *nodeArena, tok Token) (externalScannerCheckpointRef, bool) {
	if p == nil || arena == nil || !p.currentExternalTokenCheckpointValid {
		return externalScannerCheckpointRef{}, false
	}
	if tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return externalScannerCheckpointRef{}, false
	}
	if tok.StartByte != p.currentExternalTokenCheckpointStart || tok.EndByte != p.currentExternalTokenCheckpointEnd {
		return externalScannerCheckpointRef{}, false
	}
	if p.skipInvisibleFullLeafCheckpoints && !p.isVisibleSymbol(tok.Symbol) {
		return externalScannerCheckpointRef{}, false
	}
	cp := arena.recordExternalScannerCompactCheckpoint(
		p.currentExternalTokenCheckpoint.start,
		p.currentExternalTokenCheckpoint.end,
	)
	arena.checkpointLeafFullNodesAvoided++
	return cp, true
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

func captureParseArenaStats(parseRuntime *ParseRuntime, arena *nodeArena, arenaBreakdown **ArenaBreakdown, preMaterializationFieldRejectCandidates, preMaterializationFieldRejectSameKeyCandidates, preMaterializationFieldRejectOverflowCandidates uint64) bool {
	if parseRuntime == nil || arena == nil {
		return false
	}
	arena.finalizeCompactFullLeafDropped()
	arena.finalizePendingParentDropped()
	parseRuntime.ArenaBytesAllocated = arena.allocatedBytes
	parseRuntime.MemoryBudgetBytes = arena.budgetBytes
	parseRuntime.ExternalScannerCheckpointRecords = arena.externalScannerCheckpointRecords
	parseRuntime.ExternalScannerCheckpointSlotsAllocated = arena.externalScannerCheckpointSlotsAllocated()
	parseRuntime.ExternalScannerCheckpointBytesAllocated = arena.externalScannerCheckpointBytesAllocated()
	parseRuntime.ExternalScannerSnapshotBytesAllocated = arena.externalScannerSnapshotPayloadBytes
	parseRuntime.ExternalScannerCheckpointLeafNodes = arena.externalScannerCheckpointLeafNodes
	parseRuntime.CompactFullLeafCreated = arena.compactFullLeafCreated
	parseRuntime.CompactFullLeafMaterialized = arena.compactFullLeafMaterialized
	parseRuntime.CompactFullLeafMaterializedForParentReduce = arena.compactFullLeafMaterializedForParentReduce
	parseRuntime.CompactFullLeafMaterializedForParentReject = arena.compactFullLeafMaterializedForParentReject
	parseRuntime.CompactFullLeafMaterializedForFinalTree = arena.compactFullLeafMaterializedForFinalTree
	parseRuntime.CompactFullLeafMaterializedForNormalization = arena.compactFullLeafMaterializedForNormalization
	parseRuntime.CompactFullLeafMaterializedForRecovery = arena.compactFullLeafMaterializedForRecovery
	parseRuntime.CompactFullLeafMaterializedForQuery = arena.compactFullLeafMaterializedForQuery
	parseRuntime.CompactFullLeafMaterializedForCursor = arena.compactFullLeafMaterializedForCursor
	parseRuntime.CompactFullLeafMaterializedForParentAPI = arena.compactFullLeafMaterializedForParentAPI
	parseRuntime.CompactFullLeafMaterializedForEdit = arena.compactFullLeafMaterializedForEdit
	parseRuntime.CompactFullLeafMaterializedForCheckpointRebuild = arena.compactFullLeafMaterializedForCheckpointRebuild
	parseRuntime.CompactFullLeafMaterializedForFieldRejectPayload = arena.compactFullLeafMaterializedForFieldRejectPayload
	parseRuntime.CompactFullLeafDropped = arena.compactFullLeafDropped
	parseRuntime.PendingParentCreated = arena.pendingParentCreated
	parseRuntime.PendingParentMaterialized = arena.pendingParentMaterialized
	parseRuntime.PendingParentMaterializedForParentReduce = arena.pendingParentMaterializedForParentReduce
	parseRuntime.PendingParentMaterializedForParentReject = arena.pendingParentMaterializedForParentReject
	parseRuntime.PendingParentMaterializedForFieldReject = arena.pendingParentMaterializedForFieldReject
	parseRuntime.PendingParentMaterializedForFieldRejectPayload = arena.pendingParentMaterializedForFieldRejectPayload
	parseRuntime.PendingParentMaterializedForFinalTree = arena.pendingParentMaterializedForFinalTree
	parseRuntime.PendingParentMaterializedForNormalization = arena.pendingParentMaterializedForNormalization
	parseRuntime.PendingParentMaterializedForRecovery = arena.pendingParentMaterializedForRecovery
	parseRuntime.PendingParentMaterializedForQuery = arena.pendingParentMaterializedForQuery
	parseRuntime.PendingParentMaterializedForCursor = arena.pendingParentMaterializedForCursor
	parseRuntime.PendingParentMaterializedForParentAPI = arena.pendingParentMaterializedForParentAPI
	parseRuntime.PendingParentMaterializedForEdit = arena.pendingParentMaterializedForEdit
	parseRuntime.PendingParentMaterializedForCheckpointRebuild = arena.pendingParentMaterializedForCheckpointRebuild
	parseRuntime.PendingParentDropped = arena.pendingParentDropped
	parseRuntime.PendingParentsFlattened = arena.pendingParentsFlattened
	parseRuntime.PendingChildRefsFlattened = arena.pendingChildRefsFlattened
	parseRuntime.PendingChildEntriesAllocated = arena.pendingChildEntriesAllocated
	parseRuntime.PendingChildEntryCapacity = arena.pendingChildEntryCapacity()
	parseRuntime.PendingChildEntryWaste = arena.pendingChildEntryWaste()
	parseRuntime.PendingParentCandidates = arena.pendingParentCandidates
	parseRuntime.PendingParentRejectedEmpty = arena.pendingParentRejectedEmpty
	parseRuntime.PendingParentRejectedChildLimit = arena.pendingParentRejectedChildLimit
	parseRuntime.PendingParentRejectedAlias = arena.pendingParentRejectedAlias
	parseRuntime.PendingParentRejectedRawSpan = arena.pendingParentRejectedRawSpan
	parseRuntime.PendingParentRejectedFields = arena.pendingParentRejectedFields
	parseRuntime.PendingParentRejectedFieldsParentHidden = arena.pendingParentRejectedFieldsParentHidden
	parseRuntime.PendingParentRejectedFieldsNoIDs = arena.pendingParentRejectedFieldsNoIDs
	parseRuntime.PendingParentRejectedFieldsInherited = arena.pendingParentRejectedFieldsInherited
	parseRuntime.PendingParentRejectedFieldsHiddenChild = arena.pendingParentRejectedFieldsHiddenChild
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlain = arena.pendingParentRejectedFieldsHiddenChildPlain
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlainEmpty = arena.pendingParentRejectedFieldsHiddenChildPlainEmpty
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlainOne = arena.pendingParentRejectedFieldsHiddenChildPlainOne
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlainMany = arena.pendingParentRejectedFieldsHiddenChildPlainMany
	parseRuntime.PendingParentRejectedFieldsHiddenChildWithFields = arena.pendingParentRejectedFieldsHiddenChildWithFields
	parseRuntime.PendingParentRejectedFieldsChild = arena.pendingParentRejectedFieldsChild
	parseRuntime.PendingParentRejectedFieldsAllVisibleDirect = arena.pendingParentRejectedFieldsAllVisibleDirect
	parseRuntime.PendingParentRejectedChild = arena.pendingParentRejectedChild
	parseRuntime.PendingParentRejectedSpan = arena.pendingParentRejectedSpan
	parseRuntime.PendingParentRejectedFill = arena.pendingParentRejectedFill
	parseRuntime.FinalChildRefParents = arena.finalChildRefParents
	parseRuntime.FinalChildRefs = arena.finalChildRefsCreated
	parseRuntime.FinalChildRefMaterializedParents = arena.finalChildRefsMaterializedParents
	parseRuntime.FinalChildRefMaterializedChildren = arena.finalChildRefsMaterializedChildren
	parseRuntime.FinalChildRefSingleChildAccesses = arena.finalChildRefsSingleChildAccesses
	parseRuntime.FinalChildRefSingleChildMaterializedChildren = arena.finalChildRefsSingleChildMaterializedChildren
	parseRuntime.PreMaterializationFieldRejectCandidates = preMaterializationFieldRejectCandidates
	parseRuntime.PreMaterializationFieldRejectSameKeyCandidates = preMaterializationFieldRejectSameKeyCandidates
	parseRuntime.PreMaterializationFieldRejectOverflowCandidates = preMaterializationFieldRejectOverflowCandidates
	parseRuntime.CheckpointLeafFullNodesAvoided = arena.checkpointLeafFullNodesAvoided
	parseRuntime.LeafNodesConstructed = arena.leafNodesConstructed
	parseRuntime.ParentNodesConstructed = arena.parentNodesConstructed
	parseRuntime.NoTreeReduceNodesConstructed = arena.noTreeReduceNodesConstructed
	parseRuntime.NoTreeLeafNodesConstructed = arena.noTreeLeafNodesConstructed
	if arena.breakdownEnabled && arenaBreakdown != nil {
		*arenaBreakdown = arena.collectArenaBreakdown()
	}
	return true
}

func captureParseScratchStats(parseRuntime *ParseRuntime, scratch *parserScratch, arena *nodeArena, arenaBreakdown **ArenaBreakdown) bool {
	if parseRuntime == nil || scratch == nil {
		return false
	}
	parseRuntime.ScratchBytesAllocated = scratch.allocatedBytes()
	parseRuntime.EntryScratchBytesAllocated = scratch.entries.allocatedBytes
	parseRuntime.GSSBytesAllocated = scratch.gss.allocatedBytes
	parseRuntime.TransientChildSlicesAllocated = scratch.transientChildren.slicesAllocated
	parseRuntime.TransientChildPointersAllocated = scratch.transientChildren.pointersAllocated
	parseRuntime.TransientChildSlicesMaterialized = scratch.transientChildren.slicesMaterialized
	parseRuntime.TransientChildPointersMaterialized = scratch.transientChildren.pointersMaterialized
	parseRuntime.TransientParentNodesAllocated = scratch.transientParents.nodesAllocated
	parseRuntime.TransientParentNodesMaterialized = scratch.transientParents.nodesMaterialized
	if arena != nil && arena.breakdownEnabled && arenaBreakdown != nil {
		if *arenaBreakdown == nil {
			*arenaBreakdown = &ArenaBreakdown{}
		}
		(*arenaBreakdown).MergeScratchBytesAllocated = scratch.merge.allocatedBytes()
	}
	return true
}

func parseStopReasonWithTokenSourceEOF(stopReason ParseStopReason, tokenSourceEOFEarly bool) ParseStopReason {
	if tokenSourceEOFEarly && (stopReason == ParseStopAccepted || stopReason == ParseStopNone) {
		return ParseStopTokenSourceEOF
	}
	return stopReason
}

func recordParseRuntimeLoopStats(parseRuntime *ParseRuntime, scratch *parserScratch, iterationsUsed, nodeCount, peakStackDepth, maxStacksSeen, singleStackIterations, multiStackIterations int, singleStackTokens, multiStackTokens uint64) {
	if parseRuntime == nil {
		return
	}
	parseRuntime.Iterations = iterationsUsed
	parseRuntime.NodesAllocated = nodeCount
	parseRuntime.PeakStackDepth = peakStackDepth
	parseRuntime.MaxStacksSeen = maxStacksSeen
	parseRuntime.SingleStackIterations = singleStackIterations
	parseRuntime.MultiStackIterations = multiStackIterations
	parseRuntime.SingleStackTokens = singleStackTokens
	parseRuntime.MultiStackTokens = multiStackTokens
	if scratch == nil {
		return
	}
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
	parseRuntime.ChildSlicesAllocated = scratch.audit.totalChildSlicesAllocated
	parseRuntime.ChildSlicesRetained = scratch.audit.totalChildSlicesRetained
	parseRuntime.ChildSlicesDroppedSameToken = scratch.audit.totalChildSlicesDropped
	parseRuntime.ChildPointersAllocated = scratch.audit.totalChildPointersAllocated
	parseRuntime.ChildPointersRetained = scratch.audit.totalChildPointersRetained
	parseRuntime.ChildPointersDroppedSameToken = scratch.audit.totalChildPointersDropped
	parseRuntime.ReduceChildFastGSS = scratch.audit.reduceChildPathRuntime(reduceChildPathFastGSS)
	parseRuntime.ReduceChildAllVisible = scratch.audit.reduceChildPathRuntime(reduceChildPathAllVisible)
	parseRuntime.ReduceChildNoAlias = scratch.audit.reduceChildPathRuntime(reduceChildPathNoAlias)
	parseRuntime.ReduceChildScratchGeneral = scratch.audit.reduceChildPathRuntime(reduceChildPathScratchGeneral)
	parseRuntime.ReduceChildScratchNoAlias = scratch.audit.reduceChildPathRuntime(reduceChildPathScratchNoAlias)
	parseRuntime.MergeStacksIn = scratch.audit.mergeStacksIn
	parseRuntime.MergeStacksOut = scratch.audit.mergeStacksOut
	parseRuntime.MergeSlotsUsed = scratch.audit.mergeSlotsUsed
	parseRuntime.GlobalCullStacksIn = scratch.audit.globalCullStacksIn
	parseRuntime.GlobalCullStacksOut = scratch.audit.globalCullStacksOut
	parseRuntime.StackEquivCalls = scratch.audit.stackEquivCalls
	parseRuntime.StackEquivTrue = scratch.audit.stackEquivTrue
	parseRuntime.StackEquivDepthMismatch = scratch.audit.stackEquivDepthMismatch
	parseRuntime.StackEquivHashMismatch = scratch.audit.stackEquivHashMismatch
	parseRuntime.StackEquivStateMismatch = scratch.audit.stackEquivStateMismatch
	parseRuntime.StackEquivPayloadMismatch = scratch.audit.stackEquivPayloadMismatch
	parseRuntime.StackEquivEntryCompares = scratch.audit.stackEquivEntryCompares
	parseRuntime.StackEquivStateMismatchDepthSum = scratch.audit.stackEquivStateMismatchDepthSum
	parseRuntime.StackEquivStateMismatchMaxDepth = scratch.audit.stackEquivStateMismatchMaxDepth
	parseRuntime.StackEquivStateMismatchDepthBuckets = scratch.audit.stackEquivStateMismatchDepthBuckets
	parseRuntime.StackEquivPayloadMismatchDepthSum = scratch.audit.stackEquivPayloadMismatchDepthSum
	parseRuntime.StackEquivPayloadMismatchMaxDepth = scratch.audit.stackEquivPayloadMismatchMaxDepth
	parseRuntime.StackEquivPayloadMismatchDepthBuckets = scratch.audit.stackEquivPayloadMismatchDepthBuckets
	parseRuntime.StackEquivPayloadHeaderSigDiff = scratch.audit.stackEquivPayloadHeaderSigDiff
	parseRuntime.StackEquivPayloadHeaderSigSame = scratch.audit.stackEquivPayloadHeaderSigSame
	parseRuntime.StackEquivPayloadShallowSigDiff = scratch.audit.stackEquivPayloadShallowSigDiff
	parseRuntime.StackEquivPayloadShallowSigSame = scratch.audit.stackEquivPayloadShallowSigSame
	parseRuntime.StackEquivPairKeyed = scratch.audit.stackEquivPairKeyed
	parseRuntime.StackEquivPairUnkeyed = scratch.audit.stackEquivPairUnkeyed
	parseRuntime.StackEquivPairRepeats = scratch.audit.stackEquivPairRepeats
	parseRuntime.StackEquivPairRepeatTrue = scratch.audit.stackEquivPairRepeatTrue
	parseRuntime.StackEquivPairRepeatFalse = scratch.audit.stackEquivPairRepeatFalse
	parseRuntime.StackEquivPairRepeatMismatch = scratch.audit.stackEquivPairRepeatMismatch
	parseRuntime.StackEquivPairStores = scratch.audit.stackEquivPairStores
	parseRuntime.MergeHeaderEqTotal = scratch.audit.mergeHeaderEqTotal
	parseRuntime.MergeDeepTrue = scratch.audit.mergeDeepTrue
	parseRuntime.MergeDeepFalse = scratch.audit.mergeDeepFalse
	parseRuntime.MergeHeaderDeepDivergent = scratch.audit.mergeHeaderDeepDivergent
	parseRuntime.EquivCacheLookups = scratch.audit.equivCacheLookups
	parseRuntime.EquivCacheHits = scratch.audit.equivCacheHits
	parseRuntime.EquivCacheStores = scratch.audit.equivCacheStores
	parseRuntime.EquivCacheMisses = scratch.audit.equivCacheMisses
	parseRuntime.EquivCacheTrueHits = scratch.audit.equivCacheTrueHits
	parseRuntime.EquivCacheFalseHits = scratch.audit.equivCacheFalseHits
	parseRuntime.EquivCacheEpochMisses = scratch.audit.equivCacheEpochMisses
	parseRuntime.EquivCacheKeyMisses = scratch.audit.equivCacheKeyMisses
	parseRuntime.EquivCacheVersionMisses = scratch.audit.equivCacheVersionMisses
	parseRuntime.EquivSkipError = scratch.audit.equivSkipError
	parseRuntime.EquivSkipLeaf = scratch.audit.equivSkipLeaf
	parseRuntime.EquivSkipFieldMismatch = scratch.audit.equivSkipFieldMismatch
	parseRuntime.EquivExactCalls = scratch.audit.equivExactCalls
	parseRuntime.EquivExactTrue = scratch.audit.equivExactTrue
	parseRuntime.EquivExactPointerTrue = scratch.audit.equivExactPointerTrue
	parseRuntime.EquivExactNilMismatch = scratch.audit.equivExactNilMismatch
	parseRuntime.EquivExactHeaderMismatch = scratch.audit.equivExactHeaderMismatch
	parseRuntime.EquivExactChildMismatch = scratch.audit.equivExactChildMismatch
	parseRuntime.EquivExactTerminalCalls = scratch.audit.equivExactTerminalCalls
	parseRuntime.EquivExactTerminalTrue = scratch.audit.equivExactTerminalTrue
	parseRuntime.EquivExactTerminalFalse = scratch.audit.equivExactTerminalFalse
	parseRuntime.EquivFrontierCalls = scratch.audit.equivFrontierCalls
	parseRuntime.EquivFrontierTrue = scratch.audit.equivFrontierTrue
	parseRuntime.EquivExactChildCompares = scratch.audit.equivExactChildCompares
	parseRuntime.EquivFrontierChildScans = scratch.audit.equivFrontierChildScans
	parseRuntime.EquivFrontierCandidateCompares = scratch.audit.equivFrontierCandidateCompares
	parseRuntime.EquivStateStats = scratch.audit.equivStateStats()
}

func recordParseRuntimeMaterializationTiming(parseRuntime *ParseRuntime, timingRef *parseMaterializationTiming, timing parseMaterializationTiming) {
	if parseRuntime == nil || timingRef == nil {
		return
	}
	parseRuntime.ResultSelectionNanos = timing.resultSelectionNanos
	parseRuntime.TransientParentMaterializationNanos = timing.transientParentMaterializeNanos
	parseRuntime.ResultTreeBuildNanos = timing.resultTreeBuildNanos
	parseRuntime.TransientChildMaterializationNanos = timing.transientChildMaterializationNanos
	parseRuntime.ResultPythonKeywordRepairNanos = timing.pythonKeywordRepairNanos
	parseRuntime.ResultPythonRootRepairNanos = timing.pythonRootRepairNanos
	parseRuntime.ResultFinalizeRootNanos = timing.resultFinalizeRootNanos
	parseRuntime.ResultExtendTrailingNanos = timing.resultExtendTrailingNanos
	parseRuntime.ResultNormalizeRootStartNanos = timing.resultNormalizeRootStartNanos
	parseRuntime.ResultCompatibilityNanos = timing.resultCompatibilityNanos
	parseRuntime.ResultParentLinkNanos = timing.resultParentLinkNanos
	if timing.reduceRangeNanos != 0 ||
		timing.reducePendingParentNanos != 0 ||
		timing.reduceChildBuildNanos != 0 ||
		timing.reduceParentBuildNanos != 0 ||
		timing.reduceSpanNanos != 0 ||
		timing.reduceStackPushNanos != 0 ||
		timing.reduceNoTreeBuildNanos != 0 {
		parseRuntime.ReduceTiming = &ParseReduceTiming{
			RangeNanos:         timing.reduceRangeNanos,
			PendingParentNanos: timing.reducePendingParentNanos,
			ChildBuildNanos:    timing.reduceChildBuildNanos,
			ParentBuildNanos:   timing.reduceParentBuildNanos,
			SpanNanos:          timing.reduceSpanNanos,
			StackPushNanos:     timing.reduceStackPushNanos,
			NoTreeBuildNanos:   timing.reduceNoTreeBuildNanos,
		}
	}
	if timing.actionExtraShiftNanos != 0 ||
		timing.actionNoActionNanos != 0 ||
		timing.actionNoActionRelexNanos != 0 ||
		timing.actionNoActionMissingNanos != 0 ||
		timing.actionNoActionRecoverNanos != 0 ||
		timing.actionNoActionErrorNanos != 0 ||
		timing.actionConflictChoiceNanos != 0 ||
		timing.actionConflictForkNanos != 0 ||
		timing.actionSingleShiftNanos != 0 ||
		timing.actionSingleReduceNanos != 0 ||
		timing.actionSingleAcceptNanos != 0 ||
		timing.actionSingleRecoverNanos != 0 ||
		timing.actionSingleOtherNanos != 0 {
		parseRuntime.ActionTiming = &ParseActionTiming{
			ExtraShiftNanos:      timing.actionExtraShiftNanos,
			NoActionNanos:        timing.actionNoActionNanos,
			NoActionRelexNanos:   timing.actionNoActionRelexNanos,
			NoActionMissingNanos: timing.actionNoActionMissingNanos,
			NoActionRecoverNanos: timing.actionNoActionRecoverNanos,
			NoActionErrorNanos:   timing.actionNoActionErrorNanos,
			ConflictChoiceNanos:  timing.actionConflictChoiceNanos,
			ConflictForkNanos:    timing.actionConflictForkNanos,
			SingleShiftNanos:     timing.actionSingleShiftNanos,
			SingleReduceNanos:    timing.actionSingleReduceNanos,
			SingleAcceptNanos:    timing.actionSingleAcceptNanos,
			SingleRecoverNanos:   timing.actionSingleRecoverNanos,
			SingleOtherNanos:     timing.actionSingleOtherNanos,
		}
	}
}

func recordParseRuntimePhaseTiming(parseRuntime *ParseRuntime, timingRef *parseMaterializationTiming, parseStart time.Time, parserLoopNanos, tokenNextNanos, actionDispatchNanos, actionLookupNanos, glrMergeNanos, glrCullNanos int64) {
	if parseRuntime == nil || timingRef == nil {
		return
	}
	parseRuntime.ParseWallNanos = time.Since(parseStart).Nanoseconds()
	parseRuntime.ParserLoopNanos = parserLoopNanos
	parseRuntime.TokenNextNanos = tokenNextNanos
	parseRuntime.ActionDispatchNanos = actionDispatchNanos
	parseRuntime.ActionLookupNanos = actionLookupNanos
	parseRuntime.GLRMergeNanos = glrMergeNanos
	parseRuntime.GLRCullNanos = glrCullNanos
}

func recordParseRuntimeTokenStats(parseRuntime *ParseRuntime, tokensConsumed uint64, lastTokenEndByte uint32, lastTokenSymbol Symbol, lastTokenWasEOF, tokenSourceEOFEarly bool) {
	if parseRuntime == nil {
		return
	}
	parseRuntime.TokensConsumed = tokensConsumed
	parseRuntime.LastTokenEndByte = lastTokenEndByte
	parseRuntime.LastTokenSymbol = lastTokenSymbol
	parseRuntime.LastTokenWasEOF = lastTokenWasEOF
	parseRuntime.TokenSourceEOFEarly = tokenSourceEOFEarly
}

func recordParseRuntimeRootStats(parseRuntime *ParseRuntime, tree *Tree, expectedEOFByte uint32, collectFinalStats bool, lang *Language) {
	if parseRuntime == nil {
		return
	}
	parseRuntime.RootEndByte = 0
	parseRuntime.Truncated = false
	root := rawRootOrNil(tree)
	if root == nil {
		return
	}
	parseRuntime.RootEndByte = root.EndByte()
	parseRuntime.Truncated = parseRuntime.RootEndByte < expectedEOFByte
	if !collectFinalStats {
		return
	}
	root = tree.RootNode()
	if root == nil {
		return
	}
	finalStats := collectFinalTreeMaterializationStats(root, lang)
	parseRuntime.FinalNodes = finalStats.nodes
	parseRuntime.FinalParentNodes = finalStats.parentNodes
	parseRuntime.FinalLeafNodes = finalStats.leafNodes
	parseRuntime.FinalFieldedParentNodes = finalStats.fieldedParentNodes
	parseRuntime.FinalUnfieldedParentNodes = finalStats.unfieldedParentNodes
	parseRuntime.FinalVisibleParentNodes = finalStats.visibleParentNodes
	parseRuntime.FinalHiddenParentNodes = finalStats.hiddenParentNodes
	parseRuntime.FinalCheckpointLeafNodes = finalStats.checkpointLeafNodes
	parseRuntime.FinalChildSlices = finalStats.childSlices
	parseRuntime.FinalChildPointers = finalStats.childPointers
	parseRuntime.FinalFieldIDElements = finalStats.fieldIDElements
	parseRuntime.FinalFieldSourceElements = finalStats.fieldSourceElements
}

func copyParseRuntimeToTiming(timing *incrementalParseTiming, parseRuntime ParseRuntime) {
	if timing == nil {
		return
	}
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
	timing.parserLoopNanos = parseRuntime.ParserLoopNanos
	timing.tokenNextNanos = parseRuntime.TokenNextNanos
	timing.actionDispatchNanos = parseRuntime.ActionDispatchNanos
	timing.actionLookupNanos = parseRuntime.ActionLookupNanos
	timing.glrMergeNanos = parseRuntime.GLRMergeNanos
	timing.glrCullNanos = parseRuntime.GLRCullNanos
	timing.resultSelectionNanos = parseRuntime.ResultSelectionNanos
	timing.transientParentMaterializationNanos = parseRuntime.TransientParentMaterializationNanos
	timing.resultTreeBuildNanos = parseRuntime.ResultTreeBuildNanos
	timing.transientChildMaterializationNanos = parseRuntime.TransientChildMaterializationNanos
	timing.resultPythonKeywordRepairNanos = parseRuntime.ResultPythonKeywordRepairNanos
	timing.resultPythonRootRepairNanos = parseRuntime.ResultPythonRootRepairNanos
	timing.resultFinalizeRootNanos = parseRuntime.ResultFinalizeRootNanos
	timing.resultExtendTrailingNanos = parseRuntime.ResultExtendTrailingNanos
	timing.resultNormalizeRootStartNanos = parseRuntime.ResultNormalizeRootStartNanos
	timing.resultCompatibilityNanos = parseRuntime.ResultCompatibilityNanos
	timing.resultParentLinkNanos = parseRuntime.ResultParentLinkNanos
	if reduceTiming := parseRuntime.ReduceTiming; reduceTiming != nil {
		timing.reduceRangeNanos = reduceTiming.RangeNanos
		timing.reducePendingParentNanos = reduceTiming.PendingParentNanos
		timing.reduceChildBuildNanos = reduceTiming.ChildBuildNanos
		timing.reduceParentBuildNanos = reduceTiming.ParentBuildNanos
		timing.reduceSpanNanos = reduceTiming.SpanNanos
		timing.reduceStackPushNanos = reduceTiming.StackPushNanos
		timing.reduceNoTreeBuildNanos = reduceTiming.NoTreeBuildNanos
	}
	if actionTiming := parseRuntime.ActionTiming; actionTiming != nil {
		timing.actionExtraShiftNanos = actionTiming.ExtraShiftNanos
		timing.actionNoActionNanos = actionTiming.NoActionNanos
		timing.actionNoActionRelexNanos = actionTiming.NoActionRelexNanos
		timing.actionNoActionMissingNanos = actionTiming.NoActionMissingNanos
		timing.actionNoActionRecoverNanos = actionTiming.NoActionRecoverNanos
		timing.actionNoActionErrorNanos = actionTiming.NoActionErrorNanos
		timing.actionConflictChoiceNanos = actionTiming.ConflictChoiceNanos
		timing.actionConflictForkNanos = actionTiming.ConflictForkNanos
		timing.actionSingleShiftNanos = actionTiming.SingleShiftNanos
		timing.actionSingleReduceNanos = actionTiming.SingleReduceNanos
		timing.actionSingleAcceptNanos = actionTiming.SingleAcceptNanos
		timing.actionSingleRecoverNanos = actionTiming.SingleRecoverNanos
		timing.actionSingleOtherNanos = actionTiming.SingleOtherNanos
	}
	timing.normalizationNanos = parseRuntime.NormalizationNanos
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
	parseFlags := p.applyParseModeFlags(source, reuse, oldTree, arenaClass)
	defer p.restoreParseModeFlags(parseFlags)
	p.clearCurrentExternalTokenCheckpoint()
	p.resetNormalizationStats()
	if p.logger != nil {
		p.logf(ParserLogParse, "start len=%d incremental=%t", len(source), reuse != nil || oldTree != nil)
	}
	deferParentLinks := reuse == nil && oldTree == nil
	scratch := acquireParserScratch()
	transientReduceParents := p.configureParseScratch(scratch, source, reuse, oldTree, arenaClass, deferParentLinks)
	defer releaseParserScratch(scratch, deferParentLinks)
	p.reduceScratch = &scratch.reduce
	if transientReduceParents {
		p.reduceScratch.transientParents = &scratch.transientParents
		p.reduceScratch.transientChildren = &scratch.transientChildren
	}
	defer func() {
		p.reduceScratch = nil
	}()
	scratch.audit.beginParse()
	scratch.merge.audit = nil
	scratch.gss.audit = nil
	trackChildErrors := !deferParentLinks

	arena := acquireNodeArena(arenaClass)
	arena.skipChildClear = reuse == nil && oldTree == nil
	arena.finalChildRefs = p.finalChildRefs
	arena.audit = nil
	if scratch.audit.enabled || scratch.audit.equivEnabled {
		scratch.merge.audit = &scratch.audit
	}
	if scratch.audit.enabled {
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
	var materializationTiming parseMaterializationTiming
	var materializationTimingRef *parseMaterializationTiming
	if timing != nil || parseShouldCaptureMaterializationTiming(p, source, reuse, oldTree, arenaClass) || (p != nil && p.noTreeBenchmarkOnly && (parseReduceTimingEnabled() || parseActionTimingEnabled())) {
		materializationTimingRef = &materializationTiming
	}
	phaseTiming := materializationTimingRef != nil
	var actionTiming *parseMaterializationTiming
	if materializationTimingRef != nil && parseActionTimingEnabled() {
		actionTiming = materializationTimingRef
	}
	recordActionTiming := func(state StateID, lookahead Symbol, actions []ParseAction, kind ambiguityActionTimingKind, nanos int64) {
		if nanos <= 0 || p == nil || p.ambiguityProfile == nil {
			return
		}
		p.ambiguityProfile.recordActionTiming(state, lookahead, actions, kind, nanos)
	}
	var parserLoopNanos int64
	var tokenNextNanos int64
	var actionDispatchNanos int64
	var actionLookupNanos int64
	var glrMergeNanos int64
	var glrCullNanos int64
	prevMaterializationTiming := p.materializationTiming
	prevReduceTiming := p.reduceTiming
	p.materializationTiming = materializationTimingRef
	if materializationTimingRef != nil && parseReduceTimingEnabled() {
		p.reduceTiming = materializationTimingRef
	} else {
		p.reduceTiming = nil
	}
	defer func() {
		p.materializationTiming = prevMaterializationTiming
		p.reduceTiming = prevReduceTiming
	}()
	defer p.recordParseArenaUsageOnReturn(arenaClass, arena, scratch)()
	p.ensureParseInitialCapacity(source, arenaClass, arena, scratch)
	memoryBudget := parseMemoryBudgetForParser(p, len(source))
	arena.setBudget(memoryBudget)
	scratch.setBudget(memoryBudget)
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
	preMaterializationDiag := parsePreMaterializationDiagEnabled()
	var preMaterializationFieldRejectCandidates uint64
	var preMaterializationFieldRejectSameKeyCandidates uint64
	var preMaterializationFieldRejectOverflowCandidates uint64
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
	var arenaBreakdown *ArenaBreakdown
	scratchStatsCaptured := false
	captureArenaStats := func() {
		if arenaStatsCaptured {
			return
		}
		if captureParseArenaStats(&parseRuntime, arena, &arenaBreakdown, preMaterializationFieldRejectCandidates, preMaterializationFieldRejectSameKeyCandidates, preMaterializationFieldRejectOverflowCandidates) {
			arenaStatsCaptured = true
		}
	}
	captureScratchStats := func() {
		if scratchStatsCaptured {
			return
		}
		if captureParseScratchStats(&parseRuntime, scratch, arena, &arenaBreakdown) {
			scratchStatsCaptured = true
		}
	}
	finalizeTree := func(tree *Tree, stopReason ParseStopReason) *Tree {
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		if p.transientReduceChildren && tree != nil {
			materializeStart := time.Time{}
			if materializationTimingRef != nil {
				materializeStart = time.Now()
			}
			scratch.transientChildren.materializeNode(tree.RootNode(), arena, &scratch.nodeLinks)
			if materializationTimingRef != nil {
				materializationTimingRef.transientChildMaterializationNanos += time.Since(materializeStart).Nanoseconds()
			}
		}
		scratch.audit.finishParse(stacks)
		captureArenaStats()
		captureScratchStats()
		parseRuntime.StopReason = parseStopReasonWithTokenSourceEOF(stopReason, tokenSourceEOFEarly)
		recordParseRuntimeLoopStats(&parseRuntime, scratch, iterationsUsed, nodeCount, peakStackDepth, maxStacksSeen, singleStackIterations, multiStackIterations, singleStackTokens, multiStackTokens)
		recordParseRuntimePhaseTiming(&parseRuntime, materializationTimingRef, parseStart, parserLoopNanos, tokenNextNanos, actionDispatchNanos, actionLookupNanos, glrMergeNanos, glrCullNanos)
		recordParseRuntimeMaterializationTiming(&parseRuntime, materializationTimingRef, materializationTiming)
		recordParseRuntimeTokenStats(&parseRuntime, perfTokensConsumed, lastTokenEndByte, lastTokenSymbol, lastTokenWasEOF, tokenSourceEOFEarly)
		recordParseRuntimeRootStats(&parseRuntime, tree, expectedEOFByte, scratch.audit.enabled || (arena != nil && arena.breakdownEnabled), p.language)
		p.copyNormalizationStats(&parseRuntime)
		if tree != nil {
			tree.setParseRuntime(parseRuntime)
			if arenaBreakdown != nil {
				tree.setArenaBreakdown(arenaBreakdown)
			}
		}
		copyParseRuntimeToTiming(timing, parseRuntime)
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
	finalize := func(treeStacks []glrStack, stopReason ParseStopReason) *Tree {
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		if p.noTreeBenchmarkOnly {
			rootEndByte := expectedEOFByte
			if stopReason != ParseStopAccepted && stopReason != ParseStopNone {
				rootEndByte = lastTokenEndByte
			}
			tree := p.buildNoTreeBenchmarkResult(source, arena, rootEndByte)
			return finalizeTree(tree, stopReason)
		}
		if len(treeStacks) == 0 {
			captureArenaStats()
		}
		tree := p.buildResultFromGLR(
			treeStacks,
			source,
			arena,
			oldTree,
			&reuseState,
			&scratch.nodeLinks,
			scratch.reduce.transientParents,
			scratch.reduce.transientChildren,
			!trackChildErrors,
			materializationTimingRef,
		)
		return finalizeTree(tree, stopReason)
	}
	finalizeErrorTree := func(stopReason ParseStopReason) *Tree {
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		captureArenaStats()
		arena.Release()
		return finalizeTree(parseErrorTree(source, p.language), stopReason)
	}
	finalizeRecoveredNodes := func(nodes []*Node) *Tree {
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		materializeTransientParentNodes(nodes, arena, scratch.reduce.transientParents, scratch.reduce.transientChildren)
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
		if p.noTreeBenchmarkOnly {
			return nil, false
		}
		nodes, ok := p.tryRecoverTrailingEOFSuffix(s, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, source)
		if !ok {
			return nil, false
		}
		return finalizeRecoveredNodes(nodes), true
	}

	stacks, maxStacksSeen = p.newInitialParseStacks(scratch, reuse, timing)
	caps := p.configureParseCaps(source, reuse, arenaClass, scratch, maxStacksOverride, maxNodesOverride, maxMergePerKeyOverride)
	maxStacks := caps.maxStacks
	retryPass := caps.retryPass
	mergePerKeyCap := caps.mergePerKeyCap
	maxStackCullTrigger := caps.maxStackCullTrigger
	maxIter := caps.maxIter
	maxDepth := caps.maxDepth
	maxNodes := caps.maxNodes
	parseRuntime.IterationLimit = maxIter
	parseRuntime.StackDepthLimit = maxDepth
	parseRuntime.NodeLimit = maxNodes
	parseRuntime.MemoryBudgetBytes = arena.budgetBytes

	needToken := true
	var tok Token
	var nextBranchOrder uint64 = 1

	var lastReduceState StateID
	lastReduceDepth := -1
	var consecutiveReduces int
	missingShift := parseMissingShiftTracker{lastDepth: -1}
	tryMissingSingleShift := func(stackIndex int, s *glrStack, currentState StateID) bool {
		return missingShift.tryInsert(p, stackIndex, s, currentState, tok, &nodeCount, arena, scratch, &trackChildErrors)
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
		if len(stacks) == 1 {
			if stacks[0].dead {
				return finalize(stacks, ParseStopNoStacksAlive)
			}
			scratch.gss.singleStackMode = true
			clearParseStackEntryCaches(stacks)
		} else {
			prep := p.prepareParseStacksForIteration(stacks, scratch, arena, arenaClass, maxStacks, maxStackCullTrigger, phaseTiming, &glrMergeNanos, &glrCullNanos)
			stacks = prep.stacks
			if prep.stopped {
				if prep.errorTree {
					return finalizeErrorTree(prep.stopReason)
				}
				return finalize(stacks, prep.stopReason)
			}
		}
		if scratch.gss.singleStackMode {
			singleStackIterations++
		} else {
			multiStackIterations++
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

		p.updateParserStateTokenSource(ts, stacks, scratch)

		// --- Token acquisition and incremental reuse ---
		if needToken {
			scratch.audit.startToken(stacks)
			if len(stacks) == 1 {
				singleStackTokens++
			} else {
				multiStackTokens++
			}
			if phaseTiming {
				tokenStart := time.Now()
				tok = ts.Next()
				tokenNextNanos += time.Since(tokenStart).Nanoseconds()
			} else {
				tok = ts.Next()
			}
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
			for si := range stacks {
				stacks[si].shifted = false
			}
			missingShift.resetForToken()
		}

		if reuse != nil && len(stacks) == 1 && !stacks[0].dead && tok.Symbol != 0 {
			if nextTok, ok := p.tryReuseCurrentParseSubtree(&stacks[0], tok, ts, reuse, scratch, arena, &reuseState, timing); ok {
				tok = nextTok
				needToken = false
				consecutiveReduces = 0
				continue
			}
		}

		numStacks := len(stacks)
		anyReduced := false
		forceAdvanceAfterReduce := false
		dispatchStart := time.Time{}
		if phaseTiming {
			dispatchStart = time.Now()
		}
		if p.glrTrace {
			p.traceParseIteration(iter, tok, stacks, needToken)
		}
		parseActions := p.language.ParseActions
		for si := 0; si < numStacks; si++ {
			s := &stacks[si]
			if s.dead || s.shifted {
				continue
			}
			currentState := s.top().state
		retryAction:
			actionStart := time.Time{}
			if phaseTiming {
				actionStart = time.Now()
			}
			actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
			var actions []ParseAction
			if actionIdx != 0 && int(actionIdx) < len(parseActions) {
				actions = parseActions[actionIdx].Actions
			}
			if phaseTiming {
				actionLookupNanos += time.Since(actionStart).Nanoseconds()
			}
			if keywordSym, ok := p.typeScriptContextualPropertyKeywordSymbol(tok, source); ok && parseStacksShareState(stacks[:numStacks], currentState) {
				if p.typeScriptContextualPropertyKeywordHasAction(keywordSym, currentState) {
					tok.Symbol = keywordSym
					needToken = false
					goto retryAction
				}
			}
			p.traceStackActions(si, currentState, tok.Symbol, actions)
			if p.ambiguityProfile != nil {
				p.ambiguityProfile.record(currentState, tok.Symbol, actions, numStacks)
			}
			if len(actions) > 0 && actions[0].Type == ParseActionShift && actions[0].Extra {
				actionKindStart := time.Time{}
				if actionTiming != nil {
					actionKindStart = time.Now()
				}
				p.applyExtraShiftAction(s, currentState, actions[0], tok, arena, scratch)
				nodeCount++
				needToken = true
				if actionTiming != nil {
					ns := time.Since(actionKindStart).Nanoseconds()
					actionTiming.actionExtraShiftNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionExtraShift, ns)
				}
				continue
			}
			if len(actions) == 0 {
				noActionStart := time.Time{}
				if actionTiming != nil {
					noActionStart = time.Now()
				}
				recordNoActionTiming := func() int64 {
					if actionTiming == nil {
						return 0
					}
					ns := time.Since(noActionStart).Nanoseconds()
					actionTiming.actionNoActionNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionNoAction, ns)
					return ns
				}
				sameState := parseStacksShareState(stacks, currentState)
				if tok.Symbol == 0 {
					if sameState {
						if reTok, ok := p.tryRelexCurrentStateDFA(tok, currentState, ts); ok {
							tok = reTok
							needToken = false
							if actionTiming != nil {
								ns := recordNoActionTiming()
								actionTiming.actionNoActionRelexNanos += ns
							}
							goto retryAction
						}
					}
					if tok.StartByte != tok.EndByte {
						needToken = true
						if actionTiming != nil {
							recordNoActionTiming()
						}
						continue
					}
					if len(stacks) == 1 {
						if p.canFinalizeNoActionEOF(s) {
							if actionTiming != nil {
								recordNoActionTiming()
							}
							if phaseTiming {
								actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
							}
							return finalize(stacks, ParseStopAccepted)
						}
						if tree, ok := tryFinalizeTrailingEOFSuffix(s, tok); ok {
							if actionTiming != nil {
								recordNoActionTiming()
							}
							if phaseTiming {
								actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
							}
							return tree
						}
					}
					s.dead = true
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					continue
				}
				if tok.StartByte == tok.EndByte {
					needToken = true
					if actionTiming != nil {
						recordNoActionTiming()
					}
					continue
				}
				if sameState {
					if reTok, ok := p.tryRelexCurrentStateDFA(tok, currentState, ts); ok {
						tok = reTok
						needToken = false
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRelexNanos += ns
						}
						goto retryAction
					}
					if reTok, ok := p.tryRelexBroadDFA(tok, currentState, ts); ok {
						tok = reTok
						needToken = false
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRelexNanos += ns
						}
						goto retryAction
					}
				}
				if len(stacks) > 1 {
					if p.glrTrace {
						fmt.Printf("  stack[%d] KILLED: no action for sym=%d in state=%d (multiple stacks)\n", si, tok.Symbol, currentState)
					}
					s.dead = true
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					continue
				}
				if tryMissingSingleShift(si, s, currentState) {
					anyReduced = true
					needToken = false
					consecutiveReduces = 0
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionMissingNanos += ns
					}
					continue
				}
				if depth, recoverAct, ok := p.findRecoverActionOnStack(s, tok.Symbol, timing); ok {
					if !s.truncate(depth + 1) {
						s.dead = true
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionErrorNanos += ns
						}
						continue
					}
					p.applyAction(s, recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					needToken = true
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionRecoverNanos += ns
					}
					continue
				}
				if s.depth() == 0 {
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					if phaseTiming {
						actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
					}
					return finalize(stacks, ParseStopNoStacksAlive)
				}
				p.pushOrExtendErrorNode(s, currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &trackChildErrors)
				needToken = true
				if actionTiming != nil {
					ns := recordNoActionTiming()
					actionTiming.actionNoActionErrorNanos += ns
				}
				continue
			}
			if len(actions) > 1 {
				conflictStart := time.Time{}
				if actionTiming != nil {
					conflictStart = time.Now()
				}
				var chosen ParseAction
				choice := false
				if reuse == nil && p.language != nil {
					switch p.language.Name {
					case "java":
						if next, ok := p.javaSwitchArrowConflictChoice(s, tok, actions); ok {
							chosen, choice = next, true
						} else if next, ok := javaRepetitionShiftConflictChoice(p.language, source, tok, currentState, actions); ok {
							chosen, choice = next, true
						}
					case "c_sharp":
						if next, ok := csharpRepetitionShiftConflictChoice(p.language, tok, actions); ok {
							chosen, choice = next, true
						}
					case "go":
						if maxStacksSeen > 1 && currentState == 3 && tok.Symbol == 15 {
							if next, ok := repetitionShiftConflictChoice(actions); ok {
								chosen, choice = next, true
							}
						}
					case "rust":
						if !p.noTreeBenchmarkOnly {
							if next, ok := rustRepetitionShiftConflictChoice(p.language, tok, currentState, actions); ok {
								chosen, choice = next, true
							}
						}
					case "typescript":
						if next, ok := typescriptRepetitionShiftConflictChoice(p.language, tok, currentState, actions); ok {
							chosen, choice = next, true
						}
					case "tsx":
						if next, ok := tsxRepetitionReduceConflictChoice(p.language, tok, currentState, actions); ok {
							chosen, choice = next, true
						}
					case "javascript":
						if next, ok := javascriptRepetitionShiftConflictChoice(p.language, tok, currentState, actions); ok {
							chosen, choice = next, true
						}
					}
				}
				if !choice && deterministicExternalConflicts && p.language != nil && p.language.Name == "yaml" && p.language.ExternalScanner != nil {
					chosen, choice = deterministicExternalConflictAction(actions), true
				}
				if choice {
					p.applyAction(s, chosen, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					if actionTiming != nil {
						ns := time.Since(conflictStart).Nanoseconds()
						actionTiming.actionConflictChoiceNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionConflictChoice, ns)
					}
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
					perfRecordFork(len(actions), perfTokensConsumed)
				}
				if preMaterializationDiag {
					candidates, sameKey, overflow := p.observePreMaterializationFieldRejectFork(s, actions, scratch.tmpEntries, mergePerKeyCap)
					preMaterializationFieldRejectCandidates += candidates
					preMaterializationFieldRejectSameKeyCandidates += sameKey
					preMaterializationFieldRejectOverflowCandidates += overflow
				}
				if s.depth() > maxForkCloneDepth {
					p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					if actionTiming != nil {
						ns := time.Since(conflictStart).Nanoseconds()
						actionTiming.actionConflictForkNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionConflictFork, ns)
					}
					continue
				}
				base := *s
				if p.glrTrace {
					p.traceParseFork(currentState, actions)
				}
				for ai := 1; ai < len(actions); ai++ {
					fork := base.cloneWithScratch(&scratch.gss)
					fork.branchOrder = nextBranchOrder
					nextBranchOrder++
					p.applyAction(&fork, actions[ai], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					if p.glrTrace {
						fmt.Printf("[GLR] fork[%d] after action[%d]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
							len(stacks), ai, fork.top().state, fork.dead, fork.shifted, fork.depth(), fork.byteOffset)
					}
					stacks = append(stacks, fork)
				}
				s = &stacks[si]
				p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
				if p.glrTrace {
					fmt.Printf("[GLR] orig[%d] after action[0]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
						si, s.top().state, s.dead, s.shifted, s.depth(), s.byteOffset)
				}
				if actionTiming != nil {
					ns := time.Since(conflictStart).Nanoseconds()
					actionTiming.actionConflictForkNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionConflictFork, ns)
				}
				continue
			}
			act := actions[0]
			actionKindStart := time.Time{}
			if actionTiming != nil {
				actionKindStart = time.Now()
			}
			disableBashReduceChain := p.language != nil && p.language.Name == "bash" && s.gss.head != nil
			if act.Type == ParseActionReduce && !disableBashReduceChain {
				if p.applyActionWithReduceChain(s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors) {
					forceAdvanceAfterReduce = true
				}
				if actionTiming != nil {
					ns := time.Since(actionKindStart).Nanoseconds()
					actionTiming.actionSingleReduceNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleReduce, ns)
				}
			} else {
				switch act.Type {
				case ParseActionShift:
					p.applyShiftAction(s, act, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &trackChildErrors)
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleShiftNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleShift, ns)
					}
				case ParseActionAccept:
					p.applyAcceptAction(s)
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleAcceptNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleAccept, ns)
					}
				case ParseActionRecover:
					p.applyRecoverAction(s, act, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &trackChildErrors)
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleRecoverNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleRecover, ns)
					}
				default:
					p.applyAction(s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleOtherNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleOther, ns)
					}
				}
			}
		}
		if phaseTiming {
			actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
		}

		if numStacks > 1 && retryPass && allParseStacksDead(stacks) {
			bestIdx := bestRetryRecoveryStack(stacks)
			stacks[bestIdx].dead = false
			stacks[0] = stacks[bestIdx]
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
			} else if depth, recoverAct, ok := p.findRecoverActionOnStack(&stacks[0], tok.Symbol, timing); ok {
				if stacks[0].truncate(depth + 1) {
					p.applyAction(&stacks[0], recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, &trackChildErrors)
					needToken = true
				} else {
					stacks[0].dead = true
				}
			} else if stacks[0].depth() > 0 {
				p.pushOrExtendErrorNode(&stacks[0], currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &trackChildErrors)
				needToken = true
			}
		}

		// After processing all stacks: determine whether to advance the
		// token. If any stack reduced, reuse the same token (the reducing
		// stacks have new top states and need to re-check the action for
		// the current lookahead). Otherwise, advance to next token.
		if anyReduced {
			needToken = tok.NoLookahead || forceAdvanceAfterReduce
			if tok.NoLookahead {
				lastReduceDepth = -1
				consecutiveReduces = 0
			} else if len(stacks) > 0 && !stacks[0].dead {
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
					if tok.Symbol == 0 && tok.StartByte == tok.EndByte && len(stacks) == 1 {
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
			}
		} else {
			needToken = true
			lastReduceDepth = -1
			consecutiveReduces = 0
		}
		if accepted := compactAcceptedStacks(stacks); len(accepted) > 0 {
			return finalize(accepted, ParseStopAccepted)
		}
	}

	return finalize(stacks, ParseStopIterationLimit)
}

type parseModeFlags struct {
	compactNoTreeShiftLeaves         bool
	compactFullShiftLeaves           bool
	pendingFullParents               bool
	finalChildRefs                   bool
	skipInvisibleFullLeafCheckpoints bool
	transientReduceChildren          bool
	transientReduceScratchNoAlias    bool
	transientChildren                *transientChildScratch
}

func (p *Parser) applyParseModeFlags(source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) parseModeFlags {
	prev := parseModeFlags{
		compactNoTreeShiftLeaves:         p.compactNoTreeShiftLeaves,
		compactFullShiftLeaves:           p.compactFullShiftLeaves,
		pendingFullParents:               p.pendingFullParents,
		finalChildRefs:                   p.finalChildRefs,
		skipInvisibleFullLeafCheckpoints: p.skipInvisibleFullLeafCheckpoints,
		transientReduceChildren:          p.transientReduceChildren,
		transientReduceScratchNoAlias:    p.transientReduceScratchNoAlias,
		transientChildren:                p.transientChildren,
	}
	p.compactNoTreeShiftLeaves = p.noTreeBenchmarkOnly && parseShouldCompactNoTreeShiftLeaves(len(source))
	p.compactFullShiftLeaves = parseShouldUseCompactFullShiftLeaves(p, source, reuse, oldTree, arenaClass)
	p.pendingFullParents = parseShouldUsePendingFullParents(p, source, reuse, oldTree, arenaClass)
	p.finalChildRefs = parseShouldUseFinalChildRefs(p, source, reuse, oldTree, arenaClass)
	p.skipInvisibleFullLeafCheckpoints = parseShouldSkipInvisibleFullLeafCheckpoints(p, source, reuse, oldTree, arenaClass)
	return prev
}

func (p *Parser) restoreParseModeFlags(prev parseModeFlags) {
	p.compactNoTreeShiftLeaves = prev.compactNoTreeShiftLeaves
	p.compactFullShiftLeaves = prev.compactFullShiftLeaves
	p.pendingFullParents = prev.pendingFullParents
	p.finalChildRefs = prev.finalChildRefs
	p.skipInvisibleFullLeafCheckpoints = prev.skipInvisibleFullLeafCheckpoints
	p.transientReduceChildren = prev.transientReduceChildren
	p.transientReduceScratchNoAlias = prev.transientReduceScratchNoAlias
	p.transientChildren = prev.transientChildren
}

func (p *Parser) configureParseScratch(scratch *parserScratch, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass, deferParentLinks bool) bool {
	p.transientReduceChildren = p.shouldUseTransientReduceChildren(source, reuse, oldTree, arenaClass)
	if p.transientReduceChildren {
		p.transientChildren = &scratch.transientChildren
	} else {
		p.transientChildren = nil
	}
	scratch.merge.beginEquivEpoch()
	transientReduceParents := p.shouldUseTransientReduceParents(source, reuse, oldTree, arenaClass)
	p.transientReduceScratchNoAlias = p.transientReduceChildren && transientReduceParents && parseShouldUseTransientReduceScratchNoAlias(len(source))
	scratch.merge.pythonShallow = p.language != nil && p.language.Name == "python" && len(source) <= 512*1024
	if deferParentLinks {
		scratch.gss.initialCap = p.fullGSSHintCapacity()
	} else {
		scratch.gss.initialCap = p.incrementalGSSHintCapacity()
	}
	return transientReduceParents
}

func (p *Parser) recordParseArenaUsageOnReturn(arenaClass arenaClass, arena *nodeArena, scratch *parserScratch) func() {
	if arenaClass == arenaClassFull {
		return func() {
			if !p.noTreeBenchmarkOnly {
				switch {
				case p.finalChildRefs:
					p.recordFinalChildRefArenaUsage(arena.used)
				case p.compactFullShiftLeaves:
					p.recordCompactFullArenaUsage(arena.used)
				case p.pendingFullParents:
					p.recordPendingFullArenaUsage(arena.used)
				default:
					p.recordFullArenaUsage(arena.used)
				}
			}
			p.recordFullGSSUsage(scratch.gss.usedTotal)
		}
	}
	return func() {
		p.recordIncrementalArenaUsage(arena.used)
		p.recordIncrementalGSSUsage(scratch.gss.usedTotal)
	}
}

func (p *Parser) ensureParseInitialCapacity(source []byte, arenaClass arenaClass, arena *nodeArena, scratch *parserScratch) {
	switch arenaClass {
	case arenaClassFull:
		p.ensureFullParseInitialCapacity(source, arena, scratch)
	case arenaClassIncremental:
		target := parseIncrementalArenaNodeCapacity(len(source), p.incrementalArenaHintCapacity())
		arena.ensureNodeCapacity(target)
		scratch.entries.ensureInitialCap(parseIncrementalEntryScratchCapacity(len(source)))
	}
}

func (p *Parser) ensureFullParseInitialCapacity(source []byte, arena *nodeArena, scratch *parserScratch) {
	target := parseFullArenaNodeCapacity(len(source), p.fullArenaHintCapacity())
	checkpointCapacityTarget := target
	switch {
	case p.finalChildRefs:
		target = parseFinalChildRefArenaNodeCapacity(len(source), p.finalChildRefArenaHintCapacity())
		checkpointCapacityTarget = parseFullArenaInitialNodeCapacity(len(source))
	case p.compactFullShiftLeaves:
		target = parseCompactFullArenaNodeCapacity(len(source), p.compactFullArenaHintCapacity())
		checkpointCapacityTarget = parseFullArenaInitialNodeCapacity(len(source))
	case p.pendingFullParents:
		target = parsePendingFullArenaNodeCapacity(len(source), p.pendingFullArenaHintCapacity())
		checkpointCapacityTarget = target
	}
	if p.noTreeBenchmarkOnly {
		target = parseNoTreeArenaNodeCapacity(len(source))
		checkpointCapacityTarget = target
	}
	arena.ensureExactNodeCapacity(target)
	if !p.noTreeBenchmarkOnly && languageUsesExternalScannerCheckpoints(p.language) {
		arena.ensureExternalScannerCheckpointCapacity(parseFullExternalScannerCheckpointCapacity(len(source), checkpointCapacityTarget))
	}
	scratch.entries.ensureInitialCap(parseFullEntryScratchCapacity(len(source)))
}

func (p *Parser) newInitialParseStacks(scratch *parserScratch, reuse *reuseCursor, timing *incrementalParseTiming) ([]glrStack, int) {
	var stacksBuf [4]glrStack
	stacks := stacksBuf[:1]
	initialStackCap := 64 * 1024
	if reuse != nil {
		initialStackCap = defaultStackEntrySlabCap
	}
	stacks[0] = newGLRStackWithScratchCap(p.language.InitialState, &scratch.entries, initialStackCap)
	stacks[0].recoverabilityKnown = true
	stacks[0].mayRecover = p.stateCanRecover(p.language.InitialState)
	if timing != nil && timing.maxStacksSeen < len(stacks) {
		timing.maxStacksSeen = len(stacks)
	}
	return stacks, len(stacks)
}

type parseCaps struct {
	maxStacks           int
	retryPass           bool
	mergePerKeyCap      int
	maxStackCullTrigger int
	maxIter             int
	maxDepth            int
	maxNodes            int
}

func (p *Parser) configureParseCaps(source []byte, reuse *reuseCursor, arenaClass arenaClass, scratch *parserScratch, maxStacksOverride, maxNodesOverride, maxMergePerKeyOverride int) parseCaps {
	maxStacks, retryPass := resolveParseMaxStacks(parseMaxGLRStacksValue(), maxStacksOverride, p.maxConflictWidth)
	mergePerKeyCap := effectiveParseMergePerKeyCap(p.language, parseMaxMergePerKeyValue(), reuse != nil, len(source))
	if tsxFullParseNeedsTypedArrowMergeWidth(p.language, source, reuse) && mergePerKeyCap < 2 {
		mergePerKeyCap = 2
	}
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(p.language, source, reuse) && mergePerKeyCap < maxStacksPerMergeKey {
		mergePerKeyCap = maxStacksPerMergeKey
	}
	if maxMergePerKeyOverride > mergePerKeyCap {
		mergePerKeyCap = maxMergePerKeyOverride
	}
	if mergePerKeyCap > maxStacksPerMergeKeyCeiling {
		mergePerKeyCap = maxStacksPerMergeKeyCeiling
	}
	maxStacks, mergePerKeyCap = p.tuneParseGLRCaps(maxStacks, mergePerKeyCap, reuse)
	scratch.merge.perKeyCap = mergePerKeyCap

	maxNodes := parseNodeLimitForLanguage(len(source), p.language)
	if maxNodesOverride > maxNodes {
		maxNodes = maxNodesOverride
	}
	return parseCaps{
		maxStacks:           maxStacks,
		retryPass:           retryPass,
		mergePerKeyCap:      mergePerKeyCap,
		maxStackCullTrigger: glrStackCullTrigger(maxStacks, arenaClass, languageName(p.language)),
		maxIter:             parseIterations(len(source)),
		maxDepth:            parseStackDepth(len(source)),
		maxNodes:            maxNodes,
	}
}

func javaFullParseNeedsAnnotationDeclarationMergeWidth(lang *Language, source []byte, reuse *reuseCursor) bool {
	return lang != nil &&
		lang.Name == "java" &&
		reuse == nil &&
		!parseMaxMergePerKeyEnvConfigured() &&
		bytes.Contains(source, []byte("@interface"))
}

func (p *Parser) tuneParseGLRCaps(maxStacks, mergePerKeyCap int, reuse *reuseCursor) (int, int) {
	if reuse == nil && p.language != nil && p.language.Name == "bash" {
		if maxStacks < 256 {
			maxStacks = 256
		}
		if mergePerKeyCap < 256 {
			mergePerKeyCap = 256
		}
	}
	if reuse == nil && p.language != nil && p.language.Name == "c_sharp" && mergePerKeyCap < 16 {
		mergePerKeyCap = 16
	}
	if reuse != nil {
		maxStacks, mergePerKeyCap = tuneIncrementalGLRCaps(maxStacks, mergePerKeyCap)
	}
	return maxStacks, mergePerKeyCap
}

func languageName(lang *Language) string {
	if lang == nil {
		return ""
	}
	return lang.Name
}

type parseStackPrepResult struct {
	stacks     []glrStack
	stopReason ParseStopReason
	stopped    bool
	errorTree  bool
}

func (p *Parser) prepareParseStacksForIteration(stacks []glrStack, scratch *parserScratch, arena *nodeArena, arenaClass arenaClass, maxStacks, maxStackCullTrigger int, phaseTiming bool, glrMergeNanos, glrCullNanos *int64) parseStackPrepResult {
	result := parseStackPrepResult{stacks: stacks}
	if len(stacks) == 1 {
		if stacks[0].dead {
			result.stop(ParseStopNoStacksAlive, false)
			return result
		}
		scratch.gss.singleStackMode = true
		clearParseStackEntryCaches(stacks)
		return result
	}
	if arena.budgetExhausted() || scratch.budgetExhausted() {
		result.stop(ParseStopMemoryBudget, false)
		return result
	}
	if allParseStacksDead(stacks) {
		result.stop(ParseStopNoStacksAlive, false)
		return result
	}
	scratch.merge.language = p.language
	scratch.merge.deferExactDedupe = !p.noTreeBenchmarkOnly && p.language != nil && (p.language.Name == "typescript" || p.language.Name == "tsx")
	if p.ambiguityProfile != nil {
		p.ambiguityProfile.recordMergeBefore(stacks)
	}
	if phaseTiming && glrMergeNanos != nil {
		mergeStart := time.Now()
		result.stacks = mergeStacksWithScratch(stacks, &scratch.merge)
		*glrMergeNanos += time.Since(mergeStart).Nanoseconds()
	} else {
		result.stacks = mergeStacksWithScratch(stacks, &scratch.merge)
	}
	if p.ambiguityProfile != nil {
		p.ambiguityProfile.recordMergeAfter(result.stacks)
	}
	if len(result.stacks) == 0 {
		result.stop(ParseStopNoStacksAlive, true)
		return result
	}
	result.stacks = p.cullParseStacksForIteration(result.stacks, scratch, arenaClass, maxStacks, maxStackCullTrigger, phaseTiming, glrCullNanos)
	if len(result.stacks) > 1 {
		p.promotePrimaryStack(result.stacks)
	}
	scratch.gss.singleStackMode = len(result.stacks) == 1
	clearParseStackEntryCaches(result.stacks)
	return result
}

func (r *parseStackPrepResult) stop(reason ParseStopReason, errorTree bool) {
	r.stopReason = reason
	r.stopped = true
	r.errorTree = errorTree
}

func allParseStacksDead(stacks []glrStack) bool {
	for i := range stacks {
		if !stacks[i].dead {
			return false
		}
	}
	return true
}

func (p *Parser) cullParseStacksForIteration(stacks []glrStack, scratch *parserScratch, arenaClass arenaClass, maxStacks, maxStackCullTrigger int, phaseTiming bool, glrCullNanos *int64) []glrStack {
	if len(stacks) <= maxStackCullTrigger {
		return stacks
	}
	if p.glrTrace {
		p.traceParseStackCull("pre-cull", stacks, maxStacks, maxStackCullTrigger)
	}
	if perfCountersEnabled {
		perfRecordGlobalCapCull(len(stacks), maxStacks)
	}
	cullIn := len(stacks)
	cullLang := stackCullLanguageForArena(p.language, arenaClass)
	if phaseTiming && glrCullNanos != nil {
		cullStart := time.Now()
		stacks = retainTopStacksForLanguageWithScratch(stacks, maxStacks, cullLang, &scratch.stackPick, &scratch.stackKeep, &scratch.stackCull)
		*glrCullNanos += time.Since(cullStart).Nanoseconds()
	} else {
		stacks = retainTopStacksForLanguageWithScratch(stacks, maxStacks, cullLang, &scratch.stackPick, &scratch.stackKeep, &scratch.stackCull)
	}
	scratch.audit.recordGlobalCull(cullIn, len(stacks))
	if p.glrTrace {
		p.traceParseStackCull("kept", stacks, maxStacks, maxStackCullTrigger)
	}
	return stacks
}

func (p *Parser) traceParseStackCull(label string, stacks []glrStack, maxStacks, trigger int) {
	if label == "pre-cull" {
		fmt.Printf("[GLR] CAP CULL: %d stacks -> keep %d (trigger=%d)\n", len(stacks), maxStacks, trigger)
	} else {
		fmt.Printf("[GLR] after cull:\n")
	}
	for ci := range stacks {
		fmt.Printf("  %s[%d]: st=%d dead=%v shift=%v dep=%d score=%d byte=%d\n",
			label, ci, stacks[ci].top().state, stacks[ci].dead, stacks[ci].shifted, stacks[ci].depth(), stacks[ci].score, stacks[ci].byteOffset)
	}
}

func clearParseStackEntryCaches(stacks []glrStack) {
	for i := range stacks {
		stacks[i].cacheEntries = false
		if stacks[i].gss.head != nil {
			stacks[i].entries = nil
		}
	}
}

type parseMissingShiftTracker struct {
	lastState     StateID
	lastDepth     int
	lastSymbol    Symbol
	lastStartByte uint32
	lastEndByte   uint32
	consecutive   int
}

func (t *parseMissingShiftTracker) resetForToken() {
	t.lastDepth = -1
	t.consecutive = 0
}

func (t *parseMissingShiftTracker) tryInsert(p *Parser, stackIndex int, s *glrStack, currentState StateID, tok Token, nodeCount *int, arena *nodeArena, scratch *parserScratch, trackChildErrors *bool) bool {
	missingShiftDepth := s.depth()
	if t.matches(currentState, missingShiftDepth, tok) && t.consecutive >= maxConsecutiveMissingSingleShifts {
		if p.glrTrace {
			fmt.Printf("  stack[%d] SKIP missing-shift cycle state=%d sym=%d byte=%d..%d count=%d\n",
				stackIndex, currentState, tok.Symbol, tok.StartByte, tok.EndByte, t.consecutive)
		}
		return false
	}
	if !p.tryInsertMissingSingleShift(s, tok, nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
		return false
	}
	if t.matches(currentState, missingShiftDepth, tok) {
		t.consecutive++
		return true
	}
	t.lastState = currentState
	t.lastDepth = missingShiftDepth
	t.lastSymbol = tok.Symbol
	t.lastStartByte = tok.StartByte
	t.lastEndByte = tok.EndByte
	t.consecutive = 1
	return true
}

func (t *parseMissingShiftTracker) matches(state StateID, depth int, tok Token) bool {
	return t.lastState == state &&
		t.lastDepth == depth &&
		t.lastSymbol == tok.Symbol &&
		t.lastStartByte == tok.StartByte &&
		t.lastEndByte == tok.EndByte
}

func (p *Parser) tryReuseCurrentParseSubtree(s *glrStack, tok Token, ts TokenSource, reuse *reuseCursor, scratch *parserScratch, arena *nodeArena, reuseState *parseReuseState, timing *incrementalParseTiming) (Token, bool) {
	if timing == nil {
		nextTok, _, ok := p.tryReuseSubtree(s, tok, ts, reuse, &scratch.entries, &scratch.gss)
		if ok {
			reuseState.markReused(stackEntryNode(s.top()), arena)
		}
		return nextTok, ok
	}
	reuseStart := time.Now()
	nextTok, reusedBytes, ok := p.tryReuseSubtree(s, tok, ts, reuse, &scratch.entries, &scratch.gss)
	timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
	if !ok {
		return nextTok, false
	}
	timing.reusedSubtrees++
	timing.reusedBytes += uint64(reusedBytes)
	reuseState.markReused(stackEntryNode(s.top()), arena)
	return nextTok, true
}

func (p *Parser) traceParseIteration(iter int, tok Token, stacks []glrStack, needToken bool) {
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

func parseStacksShareState(stacks []glrStack, state StateID) bool {
	if len(stacks) == 1 {
		return true
	}
	for i := range stacks {
		if stacks[i].dead {
			continue
		}
		if stacks[i].top().state != state {
			return false
		}
	}
	return true
}

func (p *Parser) actionsForParseState(state StateID, symbol Symbol, parseActions []ParseActionEntry) []ParseAction {
	actionIdx := p.lookupActionIndex(state, symbol)
	if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
		return nil
	}
	return parseActions[actionIdx].Actions
}

func (p *Parser) traceStackActions(stackIndex int, state StateID, symbol Symbol, actions []ParseAction) {
	if !p.glrTrace {
		return
	}
	actionIdx := p.lookupActionIndex(state, symbol)
	fmt.Printf("  stack[%d] state=%d actionIdx=%d actions=%d\n", stackIndex, state, actionIdx, len(actions))
	for ai, action := range actions {
		fmt.Printf("    action[%d]: type=%d state=%d sym=%d cnt=%d prec=%d\n",
			ai, action.Type, action.State, action.Symbol, action.ChildCount, action.DynamicPrecedence)
	}
}

func deterministicExternalConflictAction(actions []ParseAction) ParseAction {
	chosen := actions[0]
	for ai := 1; ai < len(actions); ai++ {
		cand := actions[ai]
		if cand.Type == ParseActionShift {
			return cand
		}
		if chosen.Type == ParseActionReduce && cand.Type == ParseActionReduce && cand.DynamicPrecedence > chosen.DynamicPrecedence {
			chosen = cand
		}
	}
	return chosen
}

func (p *Parser) traceParseFork(currentState StateID, actions []ParseAction) {
	fmt.Printf("[GLR] FORK: %d actions from state=%d\n", len(actions), currentState)
	for ai, action := range actions {
		symName := "?"
		if int(action.Symbol) < len(p.language.SymbolNames) {
			symName = p.language.SymbolNames[action.Symbol]
		}
		fmt.Printf("  action[%d]: type=%d state=%d sym=%s(%d) cnt=%d prec=%d\n",
			ai, action.Type, action.State, symName, action.Symbol, action.ChildCount, action.DynamicPrecedence)
	}
}

func bestRetryRecoveryStack(stacks []glrStack) int {
	bestIdx := 0
	for si := 1; si < len(stacks); si++ {
		if stacks[si].score > stacks[bestIdx].score {
			bestIdx = si
			continue
		}
		if stacks[si].score == stacks[bestIdx].score && stacks[si].depth() < stacks[bestIdx].depth() {
			bestIdx = si
		}
	}
	return bestIdx
}

func (p *Parser) updateParserStateTokenSource(ts TokenSource, stacks []glrStack, scratch *parserScratch) {
	stateful, ok := ts.(parserStateTokenSource)
	if !ok || len(stacks) == 0 {
		return
	}
	stateful.SetParserState(stacks[0].top().state)
	if len(stacks) == 1 || p.usesPrimaryExternalScannerStateForGLR() {
		clearGLRStateTokenSource(stateful, scratch)
		return
	}
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

func (p *Parser) usesPrimaryExternalScannerStateForGLR() bool {
	return p != nil &&
		p.language != nil &&
		(p.language.Name == "yaml" || p.language.Name == "c_sharp") &&
		p.language.ExternalScanner != nil
}

func clearGLRStateTokenSource(stateful parserStateTokenSource, scratch *parserScratch) {
	if scratch != nil && len(scratch.glrStates) > 0 {
		scratch.glrStates = scratch.glrStates[:0]
	}
	stateful.SetGLRStates(nil)
}

func (p *Parser) applyExtraShiftAction(s *glrStack, currentState StateID, act ParseAction, tok Token, arena *nodeArena, scratch *parserScratch) {
	named := p.isNamedSymbol(tok.Symbol)
	targetState := extraShiftTargetState(currentState, act)
	if p.useCompactNoTreeShiftLeaf() && !p.shiftTokenIsMissingError(tok) {
		p.applyCompactExtraShiftAction(s, currentState, targetState, tok, named, arena, scratch)
		return
	}
	leaf := newLeafNodeInArena(arena, tok.Symbol, named, tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	if tok.Missing {
		leaf.setMissing(true)
		leaf.setHasError(true)
	}
	leaf.setExtra(true)
	leaf.preGotoState = currentState
	leaf.parseState = targetState
	p.recordCurrentExternalLeafCheckpoint(leaf, tok)
	p.pushStackNode(s, targetState, leaf, &scratch.entries, &scratch.gss)
}

func (p *Parser) applyCompactExtraShiftAction(s *glrStack, currentState, targetState StateID, tok Token, named bool, arena *nodeArena, scratch *parserScratch) {
	if cp, ok := p.currentExternalNoTreeLeafCheckpointRef(arena, tok); ok {
		leaf := newCompactCheckpointLeafInArena(arena, tok.Symbol, named, tok.StartByte, tok.EndByte, cp)
		leaf.setExtra(true)
		leaf.preGotoState = currentState
		leaf.parseState = targetState
		p.pushStackCompactCheckpointLeaf(s, targetState, leaf, &scratch.entries, &scratch.gss)
		return
	}
	leaf := newNoTreeLeafNodeInArena(arena, tok.Symbol, named, tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	leaf.setExtra(true)
	leaf.preGotoState = currentState
	leaf.parseState = targetState
	p.pushStackNoTreeNode(s, targetState, leaf, &scratch.entries, &scratch.gss)
}

func (p *Parser) shouldUseTransientReduceChildren(source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	return parseTransientReduceChildrenEnabled() &&
		p != nil &&
		p.language != nil &&
		parseTransientReduceChildrenLanguageEnabled(p.language) &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		!p.noTreeBenchmarkOnly &&
		!p.noTreeCheckpointBenchmarkOnly &&
		len(source) > 0
}

func (p *Parser) shouldUseTransientReduceParents(source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	return parseTransientReduceParentsEnabled() &&
		p != nil &&
		p.language != nil &&
		parseTransientReduceParentsLanguageEnabled(p.language) &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		!p.noTreeBenchmarkOnly &&
		!p.noTreeCheckpointBenchmarkOnly &&
		len(source) > 0
}

func (p *Parser) shouldUseTransientReduceScratchNoAlias() bool {
	return p != nil &&
		p.transientReduceScratchNoAlias &&
		p.transientChildren != nil
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

func (p *Parser) javaSwitchArrowConflictChoice(s *glrStack, tok Token, actions []ParseAction) (ParseAction, bool) {
	if p == nil || p.language == nil || p.language.Name != "java" || s == nil || !symbolHasName(p.language, tok.Symbol, "->") {
		return ParseAction{}, false
	}
	var primaryReduce ParseAction
	var reduceFound bool
	var shiftFound bool
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			shiftFound = true
		case ParseActionReduce:
			if act.ChildCount == 1 && symbolHasName(p.language, act.Symbol, "primary_expression") {
				if reduceFound {
					return ParseAction{}, false
				}
				primaryReduce = act
				reduceFound = true
			}
		default:
			return ParseAction{}, false
		}
	}
	if !shiftFound || !reduceFound {
		return ParseAction{}, false
	}
	// In switch rules, `case A ->` must reduce `A` as the label expression
	// before the arrow. Lambda parameters use the same state but have no
	// post-reduce goto that can consume `->`, so this keeps lambdas intact.
	predecessor, ok := reducePredecessorStateForStack(s, int(primaryReduce.ChildCount))
	if !ok {
		return ParseAction{}, false
	}
	gotoState := p.lookupGoto(predecessor, primaryReduce.Symbol)
	if gotoState == 0 || p.lookupActionIndex(gotoState, tok.Symbol) == 0 {
		return ParseAction{}, false
	}
	return primaryReduce, true
}

func reducePredecessorStateForStack(s *glrStack, childCount int) (StateID, bool) {
	if s == nil || childCount < 0 {
		return 0, false
	}
	if childCount == 0 {
		return s.top().state, true
	}
	if s.gss.head != nil {
		nonExtraFound := 0
		for n := s.gss.head; n != nil; n = n.prev {
			if !stackEntryHasNode(n.entry) || stackEntryNodeIsExtra(n.entry) {
				continue
			}
			nonExtraFound++
			if nonExtraFound != childCount {
				continue
			}
			if n.prev == nil {
				return 0, false
			}
			return n.prev.entry.state, true
		}
		return 0, false
	}
	rr, ok := computeReduceRangePayload(s.entries, childCount)
	if !ok {
		return 0, false
	}
	return rr.topState, true
}

func csharpRepetitionShiftConflictChoice(lang *Language, tok Token, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	kind := ""
	for _, act := range actions {
		if act.Type != ParseActionReduce {
			continue
		}
		name := ""
		if int(act.Symbol) < len(lang.SymbolNames) {
			name = lang.SymbolNames[act.Symbol]
		}
		nextKind := csharpRepeatConflictKind(name)
		if nextKind == "" || (kind != "" && kind != nextKind) {
			return ParseAction{}, false
		}
		kind = nextKind
	}
	switch kind {
	case "block":
		if !csharpCanShiftBlockRepetitionToken(lang, tok) {
			return ParseAction{}, false
		}
	case "declaration_list":
		if !csharpCanShiftDeclarationListRepetitionToken(lang, tok) {
			return ParseAction{}, false
		}
	default:
		return ParseAction{}, false
	}
	return repetitionShiftConflictChoice(actions)
}

func javaRepetitionShiftConflictChoice(lang *Language, source []byte, tok Token, state StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	switch state {
	case 1104:
		if !symbolHasName(lang, tok.Symbol, ",") || !javaArrayInitializerCommaHasFollowingElement(source, tok.EndByte) {
			return ParseAction{}, false
		}
	case 983:
		switch {
		case symbolHasName(lang, tok.Symbol, "escape_sequence"):
		case symbolHasName(lang, tok.Symbol, "string_fragment"):
		default:
			return ParseAction{}, false
		}
	case 935:
		if !symbolHasName(lang, tok.Symbol, "case") {
			return ParseAction{}, false
		}
	case 7:
		switch {
		case symbolHasName(lang, tok.Symbol, "identifier"):
		case symbolHasName(lang, tok.Symbol, "if"):
		case symbolHasName(lang, tok.Symbol, "final"):
		case symbolHasName(lang, tok.Symbol, "for"):
		default:
			return ParseAction{}, false
		}
	case 412:
		switch {
		case symbolHasName(lang, tok.Symbol, "public"):
		case symbolHasName(lang, tok.Symbol, "private"):
		case symbolHasName(lang, tok.Symbol, "protected"):
		case symbolHasName(lang, tok.Symbol, "@"):
		default:
			return ParseAction{}, false
		}
	case 2:
		if !symbolHasName(lang, tok.Symbol, "import") {
			return ParseAction{}, false
		}
	default:
		return ParseAction{}, false
	}
	return repetitionShiftConflictChoice(actions)
}

func typescriptRepetitionShiftConflictChoice(lang *Language, tok Token, state StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	switch state {
	case 9:
		switch {
		case symbolHasName(lang, tok.Symbol, "function"):
		case symbolHasName(lang, tok.Symbol, "identifier"):
		case symbolHasName(lang, tok.Symbol, "const"):
		case symbolHasName(lang, tok.Symbol, "return"):
		case symbolHasName(lang, tok.Symbol, "if"):
		case symbolHasName(lang, tok.Symbol, "export"):
		default:
			return ParseAction{}, false
		}
	case 3817:
		if !symbolHasName(lang, tok.Symbol, "case") {
			return ParseAction{}, false
		}
	default:
		return ParseAction{}, false
	}
	return repetitionShiftConflictChoice(actions)
}

// javascriptRepetitionShiftConflictChoice resolves the statement/member-list
// reduce/shift conflicts where the grammar accepts both "reduce <list>_repeat1"
// and "shift the next list-element starter". The repetition shift always
// continues the list, matching how the C runtime walks these boundaries
// without forking — tree-sitter resolves the equivalent states with a
// single-action (count=1) shift, gating the repeat reduce onto the separate
// _automatic_semicolon lookahead. Profiling text-editor-component.js showed
// these two states alone account for ~95% of all JS GLR forks (state 9 on
// `this`, state 985 on class-member starters), so collapsing them to the
// C-equivalent shift removes the dominant source of fork pressure. Tokens
// are listed explicitly to stay conservative; each is a token C deterministic-
// shifts on at the corresponding state.
func javascriptRepetitionShiftConflictChoice(lang *Language, tok Token, state StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	switch state {
	case 9:
		// Top-level program_repeat1 boundary: statement-starter tokens.
		switch {
		case symbolHasName(lang, tok.Symbol, "identifier"):
		case symbolHasName(lang, tok.Symbol, "this"):
		case symbolHasName(lang, tok.Symbol, "function"):
		case symbolHasName(lang, tok.Symbol, "var"):
		case symbolHasName(lang, tok.Symbol, "const"):
		case symbolHasName(lang, tok.Symbol, "let"):
		case symbolHasName(lang, tok.Symbol, "return"):
		case symbolHasName(lang, tok.Symbol, "if"):
		case symbolHasName(lang, tok.Symbol, "export"):
		default:
			return ParseAction{}, false
		}
	case 985:
		// class_body_repeat1 boundary: class-member-name starter tokens.
		switch {
		case symbolHasName(lang, tok.Symbol, "identifier"):
		default:
			return ParseAction{}, false
		}
	case 1335:
		// variable_declaration_repeat1 boundary: `var a, b, c` — the `,`
		// separator continues the declarator list (repetition shift).
		if !symbolHasName(lang, tok.Symbol, ",") {
			return ParseAction{}, false
		}
	case 1417:
		// object_repeat1 boundary: `{a, b, c}` — the `,` separator
		// continues the object-member list (repetition shift).
		if !symbolHasName(lang, tok.Symbol, ",") {
			return ParseAction{}, false
		}
	default:
		return ParseAction{}, false
	}
	return repetitionShiftConflictChoice(actions)
}

func singleReduceAgainstRepetitionShiftConflictChoice(actions []ParseAction) (ParseAction, bool) {
	if len(actions) < 2 {
		return ParseAction{}, false
	}
	var reduce ParseAction
	reduceFound := false
	shiftFound := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionReduce:
			if reduceFound {
				return ParseAction{}, false
			}
			reduce = act
			reduceFound = true
		case ParseActionShift:
			if !act.Repetition || shiftFound {
				return ParseAction{}, false
			}
			shiftFound = true
		default:
			return ParseAction{}, false
		}
	}
	if !reduceFound || !shiftFound {
		return ParseAction{}, false
	}
	return reduce, true
}

func tsxRepetitionReduceConflictChoice(lang *Language, tok Token, state StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	reduceSymbol := ""
	switch state {
	case 9:
		switch {
		case symbolHasName(lang, tok.Symbol, "function"):
		case symbolHasName(lang, tok.Symbol, "identifier"):
		case symbolHasName(lang, tok.Symbol, "const"):
		case symbolHasName(lang, tok.Symbol, "return"):
		case symbolHasName(lang, tok.Symbol, "if"):
		case symbolHasName(lang, tok.Symbol, "export"):
		case symbolHasName(lang, tok.Symbol, "let"):
		default:
			return ParseAction{}, false
		}
		reduceSymbol = "program_repeat1"
	case 3468:
		if !symbolHasName(lang, tok.Symbol, "identifier") {
			return ParseAction{}, false
		}
		reduceSymbol = "_jsx_start_opening_element_repeat1"
	case 3885:
		if !symbolHasName(lang, tok.Symbol, ";") {
			return ParseAction{}, false
		}
		reduceSymbol = "object_type_repeat1"
	default:
		return ParseAction{}, false
	}
	reduce, ok := singleReduceAgainstRepetitionShiftConflictChoice(actions)
	if !ok || !symbolHasName(lang, reduce.Symbol, reduceSymbol) {
		return ParseAction{}, false
	}
	return reduce, true
}

func (p *Parser) initTypeScriptContextualKeywordSymbols(lang *Language) {
	if p == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "typescript", "tsx":
	default:
		return
	}
	p.typeScriptPropertyIdentifierSymbol, p.typeScriptHasPropertyIdentifier = lang.SymbolByName("property_identifier")
	p.typeScriptIdentifierSymbol, p.typeScriptHasIdentifier = lang.SymbolByName("identifier")
	keywords := make(map[string]Symbol, len(typeScriptContextualPropertyKeywordNames))
	for _, name := range typeScriptContextualPropertyKeywordNames {
		if sym, ok := lang.SymbolByName(name); ok {
			keywords[name] = sym
		}
	}
	if len(keywords) != 0 {
		p.typeScriptContextualPropertyKeyword = keywords
	}
}

func (p *Parser) typeScriptContextualPropertyKeywordSymbol(tok Token, source []byte) (Symbol, bool) {
	if p == nil || len(p.typeScriptContextualPropertyKeyword) == 0 || tok.Text == "" {
		return 0, false
	}
	if !(p.typeScriptHasPropertyIdentifier && tok.Symbol == p.typeScriptPropertyIdentifierSymbol) &&
		!(tok.Text == "readonly" && p.typeScriptHasIdentifier && tok.Symbol == p.typeScriptIdentifierSymbol) {
		return 0, false
	}
	keywordSym, ok := p.typeScriptContextualPropertyKeyword[tok.Text]
	if !ok || keywordSym == tok.Symbol {
		return 0, false
	}
	if !typeScriptContextualKeywordHasFollowingOperand(tok, source) {
		return 0, false
	}
	return keywordSym, true
}

func (p *Parser) typeScriptContextualPropertyKeywordHasAction(keywordSym Symbol, state StateID) bool {
	if p == nil || p.language == nil {
		return false
	}
	actionIdx := p.lookupActionIndex(state, keywordSym)
	if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) || len(p.language.ParseActions[actionIdx].Actions) == 0 {
		return false
	}
	return true
}

var typeScriptContextualPropertyKeywordNames = [...]string{
	"abstract", "accessor", "any", "as", "bigint", "boolean", "class", "const",
	"declare", "enum", "export", "extends", "function", "import", "in", "infer",
	"interface", "keyof", "let", "module", "namespace", "never", "new", "number",
	"object", "override", "private", "protected", "public", "readonly", "static",
	"string", "symbol", "type", "typeof", "undefined", "unknown", "void",
}

func typeScriptContextualKeywordHasFollowingOperand(tok Token, source []byte) bool {
	pos := int(tok.EndByte)
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		}
		break
	}
	if pos >= len(source) {
		return false
	}
	switch source[pos] {
	case '(', ')', '{', '}', ';', ',', ':':
		return false
	default:
		return true
	}
}

func rustRepetitionShiftConflictChoice(lang *Language, tok Token, state StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	switch state {
	case 7:
		switch {
		case symbolHasName(lang, tok.Symbol, "pub"):
		case symbolHasName(lang, tok.Symbol, "#"):
		case symbolHasName(lang, tok.Symbol, "impl"):
		case symbolHasName(lang, tok.Symbol, "fn"):
		case symbolHasName(lang, tok.Symbol, "mod"):
		case symbolHasName(lang, tok.Symbol, "use"):
		default:
			return ParseAction{}, false
		}
	case 83:
		switch {
		case symbolHasName(lang, tok.Symbol, "identifier"):
		case symbolHasName(lang, tok.Symbol, ","):
		case symbolHasName(lang, tok.Symbol, "("):
		case symbolHasName(lang, tok.Symbol, "primitive_type"):
		case symbolHasName(lang, tok.Symbol, "::"):
		case symbolHasName(lang, tok.Symbol, "."):
		case symbolHasName(lang, tok.Symbol, ";"):
		default:
			return ParseAction{}, false
		}
	default:
		return ParseAction{}, false
	}
	return repetitionShiftConflictChoice(actions)
}

func javaArrayInitializerCommaHasFollowingElement(source []byte, offset uint32) bool {
	i := int(offset)
	for i < len(source) {
		switch source[i] {
		case ' ', '\t', '\n', '\r', '\f':
			i++
			continue
		case '/':
			if i+1 >= len(source) {
				return true
			}
			switch source[i+1] {
			case '/':
				i += 2
				for i < len(source) && source[i] != '\n' && source[i] != '\r' {
					i++
				}
				continue
			case '*':
				i += 2
				for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= len(source) {
					return false
				}
				i += 2
				continue
			}
			return true
		case '}':
			return false
		default:
			return true
		}
	}
	return false
}

func symbolHasName(lang *Language, sym Symbol, name string) bool {
	return lang != nil && int(sym) >= 0 && int(sym) < len(lang.SymbolNames) && lang.SymbolNames[sym] == name
}

func csharpRepeatConflictKind(name string) string {
	switch {
	case strings.HasSuffix(name, "block_repeat1"):
		return "block"
	case strings.HasSuffix(name, "declaration_list_repeat1"):
		return "declaration_list"
	default:
		return ""
	}
}

func csharpCanShiftBlockRepetitionToken(lang *Language, tok Token) bool {
	if int(tok.Symbol) >= len(lang.SymbolNames) {
		return false
	}
	switch lang.SymbolNames[tok.Symbol] {
	case "this", "base":
		return true
	case "identifier":
		return tok.Text != "scoped"
	default:
		return false
	}
}

func csharpCanShiftDeclarationListRepetitionToken(lang *Language, tok Token) bool {
	if int(tok.Symbol) >= len(lang.SymbolNames) {
		return false
	}
	switch lang.SymbolNames[tok.Symbol] {
	case "abstract", "class", "const", "delegate", "enum", "event", "extern", "file",
		"fixed", "global", "implicit", "interface", "internal", "namespace", "new",
		"operator", "override", "partial", "private", "protected", "public", "readonly",
		"record", "ref", "sealed", "static", "struct", "unsafe", "using", "virtual",
		"volatile":
		return true
	default:
		return false
	}
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
		if stackEntryNodeHasError(top) {
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
		if lang != nil && (lang.Name == "bash" || lang.Name == "python") {
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

type preMaterializationFieldRejectKey struct {
	state      StateID
	byteOffset uint32
}

func (p *Parser) observePreMaterializationFieldRejectFork(s *glrStack, actions []ParseAction, tmp []stackEntry, perKeyCap int) (candidates, sameKeyCandidates, overflowCandidates uint64) {
	if p == nil || s == nil || p.noTreeBenchmarkOnly || !p.usePendingFullParents() || len(actions) <= 1 {
		return 0, 0, 0
	}
	if perKeyCap <= 0 {
		perKeyCap = maxStacksPerMergeKey
	}
	type keyCount struct {
		key   preMaterializationFieldRejectKey
		count int
	}
	var fixed [8]keyCount
	keys := fixed[:0]
	for _, act := range actions {
		if act.Type != ParseActionReduce || !p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, nil) {
			continue
		}
		key, ok := p.preMaterializationFieldRejectKey(s, act, tmp)
		if !ok {
			continue
		}
		candidates++
		found := false
		for i := range keys {
			if keys[i].key == key {
				keys[i].count++
				found = true
				break
			}
		}
		if found {
			continue
		}
		if len(keys) == cap(keys) {
			next := make([]keyCount, len(keys), len(keys)*2)
			copy(next, keys)
			keys = next
		}
		keys = append(keys, keyCount{key: key, count: 1})
	}
	for i := range keys {
		if keys[i].count <= 1 {
			continue
		}
		sameKeyCandidates += uint64(keys[i].count)
		if keys[i].count > perKeyCap {
			overflowCandidates += uint64(keys[i].count - perKeyCap)
		}
	}
	return candidates, sameKeyCandidates, overflowCandidates
}

func (p *Parser) preMaterializationFieldRejectKey(s *glrStack, act ParseAction, tmp []stackEntry) (preMaterializationFieldRejectKey, bool) {
	if p == nil || s == nil {
		return preMaterializationFieldRejectKey{}, false
	}
	entries, _ := s.entriesForRead(tmp)
	window, ok := computeReduceRangeForFullPayloads(entries, int(act.ChildCount), p.usePendingFullParents())
	if !ok {
		return preMaterializationFieldRejectKey{}, false
	}
	targetState := window.topState
	if gotoState := p.lookupGoto(window.topState, act.Symbol); gotoState != 0 {
		targetState = gotoState
	}
	byteOffset := s.byteOffset
	if window.actualEnd > window.start {
		byteOffset = stackEntryNodeEndByte(entries[window.actualEnd-1])
	}
	return preMaterializationFieldRejectKey{
		state:      targetState,
		byteOffset: byteOffset,
	}, true
}

func retainTopStacks(stacks []glrStack, keep int) []glrStack {
	return retainTopStacksForLanguage(stacks, keep, nil)
}

func retainTopStacksForLanguage(stacks []glrStack, keep int, lang *Language) []glrStack {
	return retainTopStacksForLanguageWithScratch(stacks, keep, lang, nil, nil, nil)
}

func retainTopStacksForLanguageWithScratch(stacks []glrStack, keep int, lang *Language, selectedBuf *[]int, chosenBuf *[]bool, keyBuf *[]stackCullKey) []glrStack {
	if keep <= 0 {
		return stacks[:0]
	}
	if len(stacks) <= keep {
		return stacks
	}
	compareLang := lang
	if keyBuf == nil {
		// Preserve the former no-key fallback semantics. That path used the
		// C#-specific comparator, but all other languages followed the generic
		// stack comparator even if the keyed full-parse path has language
		// tie-breakers.
		if compareLang == nil || compareLang.Name != "c_sharp" {
			compareLang = nil
		}
	}
	keys := buildStackCullKeys(stacks, compareLang, keyBuf)
	return retainTopStacksByKeys(stacks, keep, compareLang, keys, selectedBuf, chosenBuf)
}

func retainTopStacksByKeys(stacks []glrStack, keep int, lang *Language, keys []stackCullKey, selectedBuf *[]int, chosenBuf *[]bool) []glrStack {
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
	for i := range stacks {
		state := keys[i].state
		bestIdx := -1
		for j, selectedIdx := range selected {
			if keys[selectedIdx].state == state {
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

package gotreesitter

type resultRootBuild struct {
	parser                *Parser
	source                []byte
	arena                 *nodeArena
	reuseState            *parseReuseState
	linkScratch           *[]*Node
	lang                  *Language
	expectedRootSymbol    Symbol
	hasExpectedRoot       bool
	shouldWireParentLinks bool
	borrowedResolved      bool
	borrowed              []*nodeArena
	replayStack           syntheticRootReplayStackStore
}

func newResultRootBuild(p *Parser, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState, linkScratch *[]*Node) resultRootBuild {
	build := resultRootBuild{
		parser:                p,
		source:                source,
		arena:                 arena,
		reuseState:            reuseState,
		linkScratch:           linkScratch,
		shouldWireParentLinks: oldTree == nil,
	}
	if p != nil {
		build.lang = p.language
		if p.hasRootSymbol {
			build.expectedRootSymbol = p.rootSymbol
			build.hasExpectedRoot = true
		}
	}
	if oldTree != nil && oldTree.RootNode() != nil {
		build.expectedRootSymbol = oldTree.RootNode().symbol
		build.hasExpectedRoot = true
	}
	return build
}

func (b *resultRootBuild) prepareRootNodes(nodes []*Node) []*Node {
	if b.isLanguage("python") {
		nodes = b.repairPythonKeywordNodes(nodes)
		nodes = collapsePythonRootFragments(nodes, b.arena, b.lang)
	}
	if b.hasExpectedRoot &&
		len(nodes) > 1 &&
		!fieldedExpectedRootPresent(nodes, b.expectedRootSymbol) {
		nodes = flattenRootSelfFragments(nodes, b.arena, b.expectedRootSymbol)
	}
	return nodes
}

func fieldedExpectedRootPresent(nodes []*Node, expectedRoot Symbol) bool {
	for _, node := range nodes {
		if node != nil &&
			node.symbol == expectedRoot &&
			fieldIDSliceHasAny(node.fieldIDs()) {
			return true
		}
	}
	return false
}

func (b *resultRootBuild) buildSingleRootTree(candidate *Node) *Tree {
	candidate = flattenInvisibleRootChildren(candidate, b.arena, b.lang)
	candidate = b.repairPythonKeywordNode(candidate)
	if tree := b.tryBuildExpectedRootFromSingleError(candidate); tree != nil {
		return tree
	}
	candidate = b.repairPythonRoot(candidate)
	if !b.hasExpectedRoot || candidate.symbol == b.expectedRootSymbol {
		return b.finishTree(candidate, b.shouldWireParentLinks, true)
	}
	return b.buildExpectedRootWrapperTree(candidate)
}

func (b *resultRootBuild) tryBuildExpectedRootFromSingleError(candidate *Node) *Tree {
	if b == nil || candidate == nil || !b.hasExpectedRoot || candidate.symbol != errorSymbol || resultChildCount(candidate) == 0 {
		return nil
	}
	if b.isLanguage("hurl") {
		return nil
	}
	rootChildren := resultChildSliceForMutation(candidate)
	rootChildren = filterZeroWidthExtras(rootChildren, b.arena)
	rootChildren = b.repairPythonKeywordNodes(rootChildren)
	if len(rootChildren) == 0 || !b.expectedRootCanFrameRecoveredFragments(rootChildren) {
		return nil
	}
	root := newParentNodeInArena(b.arena, b.expectedRootSymbol, true, rootChildren, nil, 0)
	if candidate.hasError() || resultNodesHaveError(rootChildren) {
		root.setHasError(true)
	}
	root = b.repairPythonRoot(root)
	return b.finishTree(root, b.shouldWireParentLinks, true)
}

func (b *resultRootBuild) buildExpectedRootWrapperTree(child *Node) *Tree {
	root := newParentNodeInArena(b.arena, b.expectedRootSymbol, true, b.singleChildSlice(child), nil, 0)
	return b.finishTree(root, b.shouldWireParentLinks, true)
}

func (b *resultRootBuild) tryBuildRealRootTree(nodes []*Node) *Tree {
	extraSplit := splitResultRootExtras(nodes, b.lang)
	realRoot := extraSplit.realRoot
	if realRoot == nil {
		return nil
	}
	returnRealRoot := !b.hasExpectedRoot || realRoot.symbol == b.expectedRootSymbol
	if b.reuseState != nil && b.reuseState.reusedAny {
		realRoot = cloneNodeInArena(b.arena, realRoot)
		realRoot.parent = nil
		realRoot.childIndex = -1
	}
	if returnRealRoot {
		attachResultRootExtraSplit(realRoot, extraSplit, b.arena)
	}
	realRoot = b.repairPythonRoot(realRoot)
	extendTrailing := returnRealRoot || !realRoot.hasError()
	if !returnRealRoot {
		// realRoot's symbol is not the grammar's root symbol, so it will be
		// wrapped as a CHILD of a synthetic root by buildSyntheticRootTree.
		// Apply only subtree compatibility normalization here — NOT the root-span
		// mutations (normalizeRootSourceStart sets startByte=0; trailing-whitespace
		// extension). Those belong to the actual wrapper root; applying them to a
		// soon-to-be child stretches it backward over leading comments and forward
		// over trailing whitespace, diverging from tree-sitter C (the wrapper root
		// correctly absorbs that trivia instead).
		b.finalizeWrappedSubtree(realRoot)
		return nil
	}
	realRoot = flattenInvisibleRootChildren(realRoot, b.arena, b.lang)
	return b.finishTree(realRoot, b.shouldWireParentLinks, extendTrailing)
}

func (b *resultRootBuild) buildSyntheticRootTree(nodes []*Node) *Tree {
	rootChildren := filterZeroWidthExtras(nodes, b.arena)
	rootChildren = b.repairPythonKeywordNodes(rootChildren)
	rootHasError := resultNodesHaveError(rootChildren)
	rootSymbol := b.syntheticRootSymbol(nodes, rootChildren, rootHasError)
	root := newParentNodeInArena(b.arena, rootSymbol, true, rootChildren, nil, 0)
	if rootHasError {
		root.setHasError(true)
	}
	root = b.repairPythonRoot(root)
	return b.finishTree(root, b.shouldWireParentLinks, true)
}

func (b *resultRootBuild) syntheticRootSymbol(originalNodes, rootChildren []*Node, rootHasError bool) Symbol {
	rootSymbol := rootChildren[len(rootChildren)-1].symbol
	if !b.hasExpectedRoot {
		return rootSymbol
	}
	if !rootHasError {
		return b.expectedRootSymbol
	}
	if b.isLanguage("dart") && dartProgramChildrenLookComplete(originalNodes, b.lang) {
		return b.expectedRootSymbol
	}
	if b.isLanguage("proto") && protoSourceFileChildrenLookComplete(rootChildren, b.lang) {
		return b.expectedRootSymbol
	}
	if b.expectedRootCanFrameRecoveredFragments(rootChildren) {
		return b.expectedRootSymbol
	}
	if b.isLanguage("sql") {
		return b.expectedRootSymbol
	}
	if b.isLanguage("swift") {
		return b.expectedRootSymbol
	}
	if b.isLanguage("gomod") {
		return b.expectedRootSymbol
	}
	if b.isLanguage("make") {
		// tree-sitter make keeps `makefile` as the root and embeds ERROR nodes
		// as children; keep that expected root while preserving HasError.
		return b.expectedRootSymbol
	}
	// cpon's start rule is document = _value (a single value). A file with
	// multiple top-level values (e.g. the Sublime syntax-test corpus) cannot
	// reduce to one document, so the synthetic-root path runs with errors.
	// tree-sitter C still labels the root `document` and nests the recovered
	// spans as ERROR children — the root never becomes ERROR. Match that
	// invariant here, mirroring the sql/swift cases above.
	if b.isLanguage("cpon") {
		return b.expectedRootSymbol
	}
	// elisp's start rule is source_file = repeat(_sexp), the same shape as
	// make: tree-sitter C keeps `source_file` as the root and nests recovery
	// ERRORs as children (verified vs the C oracle on scrape-elpa.el, whose
	// unlexable `#` becomes an inner ERROR leaf under a source_file root).
	if b.isLanguage("elisp") {
		return b.expectedRootSymbol
	}
	return errorSymbol
}

// syntheticRootReplayFrame identifies an immutable, interned LR stack. Shared
// prefixes make push, pop, and equality independent of total stack depth.
type syntheticRootReplayFrame struct {
	top uint32
}

type syntheticRootReplayStackNode struct {
	prev  uint32
	state StateID
	depth uint32
}

type syntheticRootReplayStackKey struct {
	prev  uint32
	state StateID
}

type syntheticRootReplayCloseKey struct {
	top       uint32
	lookahead Symbol
}

type syntheticRootReplayStackStore struct {
	nodes     []syntheticRootReplayStackNode
	intern    map[syntheticRootReplayStackKey]uint32
	closeMemo map[syntheticRootReplayCloseKey][]syntheticRootReplayFrame
}

const syntheticRootReplayMaxFrontier = 128
const syntheticRootReplayMaxGapBytes = 4096
const syntheticRootReplayMaxGapTokens = 64

func (b *resultRootBuild) syntheticRootReplayPush(frame syntheticRootReplayFrame, state StateID) syntheticRootReplayFrame {
	if b == nil {
		return syntheticRootReplayFrame{}
	}
	store := &b.replayStack
	if len(store.nodes) == 0 {
		store.nodes = append(store.nodes, syntheticRootReplayStackNode{})
	}
	if uint64(frame.top) >= uint64(len(store.nodes)) {
		return syntheticRootReplayFrame{}
	}
	key := syntheticRootReplayStackKey{prev: frame.top, state: state}
	if store.intern == nil {
		store.intern = make(map[syntheticRootReplayStackKey]uint32, 256)
	} else if top, ok := store.intern[key]; ok {
		return syntheticRootReplayFrame{top: top}
	}
	if uint64(len(store.nodes)) >= uint64(1)<<32 {
		return syntheticRootReplayFrame{}
	}
	depth := uint32(1)
	if frame.top != 0 {
		if store.nodes[frame.top].depth == ^uint32(0) {
			return syntheticRootReplayFrame{}
		}
		depth += store.nodes[frame.top].depth
	}
	top := uint32(len(store.nodes))
	store.nodes = append(store.nodes, syntheticRootReplayStackNode{prev: frame.top, state: state, depth: depth})
	store.intern[key] = top
	return syntheticRootReplayFrame{top: top}
}

func (b *resultRootBuild) syntheticRootReplayTopState(frame syntheticRootReplayFrame) (StateID, bool) {
	if b == nil || frame.top == 0 || uint64(frame.top) >= uint64(len(b.replayStack.nodes)) {
		return 0, false
	}
	return b.replayStack.nodes[frame.top].state, true
}

func (b *resultRootBuild) syntheticRootReplayPop(frame syntheticRootReplayFrame, count int) (syntheticRootReplayFrame, bool) {
	if b == nil || count < 0 || frame.top == 0 || uint64(frame.top) >= uint64(len(b.replayStack.nodes)) {
		return syntheticRootReplayFrame{}, false
	}
	node := b.replayStack.nodes[frame.top]
	if uint64(count) >= uint64(node.depth) {
		return syntheticRootReplayFrame{}, false
	}
	top := frame.top
	for i := 0; i < count; i++ {
		top = b.replayStack.nodes[top].prev
	}
	return syntheticRootReplayFrame{top: top}, true
}

func (b *resultRootBuild) expectedRootCanFrameRecoveredFragments(rootChildren []*Node) bool {
	if b == nil || b.parser == nil || b.lang == nil || !b.hasExpectedRoot || len(rootChildren) == 0 {
		return false
	}
	if b.isLanguage("hurl") {
		return false
	}
	if b.lang.InitialState == 0 {
		return false
	}
	initial := b.syntheticRootReplayPush(syntheticRootReplayFrame{}, b.lang.InitialState)
	if initial.top == 0 {
		return false
	}
	frontier := []syntheticRootReplayFrame{initial}
	consumedNonError := false
	sawRecovery := false
	gapStartByte := uint32(0)
	gapStartPoint := Point{}
	haveGapStart := false
	for _, child := range rootChildren {
		if child == nil {
			continue
		}
		if b.syntheticRootReplaySkipsChild(child) {
			if child.endByte > child.startByte && (!haveGapStart || child.endByte > gapStartByte) {
				gapStartByte = child.endByte
				gapStartPoint = child.endPoint
				haveGapStart = true
			}
			continue
		}
		if child.IsError() || child.HasError() {
			sawRecovery = true
			gapStartByte = child.endByte
			gapStartPoint = child.endPoint
			haveGapStart = true
			continue
		}
		next := b.syntheticRootReplayAdvance(frontier, child)
		if len(next) == 0 && haveGapStart && child.startByte >= gapStartByte {
			bridged := b.syntheticRootReplayBridgeGap(frontier, gapStartByte, gapStartPoint, child.startByte)
			if len(bridged) > 0 {
				next = b.syntheticRootReplayAdvance(bridged, child)
			}
		}
		if len(next) == 0 {
			return false
		}
		frontier = next
		consumedNonError = true
		gapStartByte = child.endByte
		gapStartPoint = child.endPoint
		haveGapStart = true
	}
	if consumedNonError {
		return b.syntheticRootReplayFrontierAcceptsEOF(frontier)
	}
	return sawRecovery && b.expectedRootEmptyFrameAcceptsEOF()
}

func (b *resultRootBuild) syntheticRootReplayAdvance(frontier []syntheticRootReplayFrame, child *Node) []syntheticRootReplayFrame {
	if len(frontier) == 0 || child == nil {
		return nil
	}
	frontier = b.syntheticRootReplayCloseBeforeChild(frontier, child)
	advanced := make([]syntheticRootReplayFrame, 0, len(frontier))
	for _, frame := range frontier {
		state, ok := b.syntheticRootReplayTopState(frame)
		if !ok {
			continue
		}
		next, ok := b.syntheticRootReplayChild(state, child)
		if !ok {
			continue
		}
		advanced = appendSyntheticRootReplayFrame(advanced, b.syntheticRootReplayPush(frame, next))
	}
	if len(advanced) == 0 {
		return nil
	}
	return b.syntheticRootReplayCloseEOF(advanced)
}

func (b *resultRootBuild) syntheticRootReplayCloseBeforeChild(frontier []syntheticRootReplayFrame, child *Node) []syntheticRootReplayFrame {
	if len(frontier) == 0 || child == nil {
		return nil
	}
	if b.lang != nil && b.lang.TokenCount > 0 && uint32(child.symbol) < b.lang.TokenCount {
		return b.syntheticRootReplayCloseLookahead(frontier, child.symbol)
	}
	closed := make([]syntheticRootReplayFrame, 0, len(frontier))
	var lexMemo [syntheticRootReplayMaxFrontier]syntheticRootReplayLexMemo
	lexMemoLen := 0
	for _, frame := range frontier {
		state, ok := b.syntheticRootReplayTopState(frame)
		if !ok {
			continue
		}
		var tok Token
		lexOK := false
		memoized := false
		for i := 0; i < lexMemoLen; i++ {
			if lexMemo[i].state == state {
				tok, lexOK, memoized = lexMemo[i].tok, lexMemo[i].ok, true
				break
			}
		}
		if !memoized {
			tok, lexOK = b.syntheticRootReplayLexGapTokenForState(state, child.startByte, child.startPoint, child.endByte)
			// Every replay frontier is capped at syntheticRootReplayMaxFrontier.
			// Keep the guard so a future direct caller with a larger frontier
			// degrades to repeated lexing instead of indexing past the memo.
			if lexMemoLen < len(lexMemo) {
				lexMemo[lexMemoLen] = syntheticRootReplayLexMemo{state: state, tok: tok, ok: lexOK}
				lexMemoLen++
			}
		}
		if lexOK {
			for _, reduced := range b.syntheticRootReplayCloseLookahead([]syntheticRootReplayFrame{frame}, tok.Symbol) {
				closed = appendSyntheticRootReplayFrame(closed, reduced)
			}
			continue
		}
		closed = appendSyntheticRootReplayFrame(closed, frame)
	}
	return closed
}

type syntheticRootReplayGapCursor struct {
	frame syntheticRootReplayFrame
	byte  uint32
	point Point
}

type syntheticRootReplayLexMemo struct {
	state StateID
	tok   Token
	ok    bool
}

func (b *resultRootBuild) syntheticRootReplayBridgeGap(frontier []syntheticRootReplayFrame, startByte uint32, startPoint Point, endByte uint32) []syntheticRootReplayFrame {
	if len(frontier) == 0 {
		return nil
	}
	if startByte == endByte {
		return frontier
	}
	if startByte > endByte || endByte > uint32(len(b.source)) || endByte-startByte > syntheticRootReplayMaxGapBytes {
		return nil
	}
	cursors := make([]syntheticRootReplayGapCursor, 0, len(frontier))
	for _, frame := range frontier {
		cursors = appendSyntheticRootReplayGapCursor(cursors, frame, startByte, startPoint)
	}
	for step := 0; step < syntheticRootReplayMaxGapTokens; step++ {
		allAtEnd := true
		nextCursors := make([]syntheticRootReplayGapCursor, 0, len(cursors))
		for _, cursor := range cursors {
			if cursor.byte == endByte {
				nextCursors = appendSyntheticRootReplayGapCursor(nextCursors, cursor.frame, cursor.byte, cursor.point)
				continue
			}
			allAtEnd = false
			if cursor.byte > endByte {
				continue
			}
			tok, ok := b.syntheticRootReplayLexGapToken(cursor.frame, cursor.byte, cursor.point, endByte)
			if !ok {
				if syntheticRootReplayCanSkipGapByte(b.source[cursor.byte]) {
					nextByte := cursor.byte + 1
					nextPoint := advancePointByBytes(cursor.point, b.source[cursor.byte:nextByte])
					nextCursors = appendSyntheticRootReplayGapCursor(nextCursors, cursor.frame, nextByte, nextPoint)
				}
				continue
			}
			advanced := b.syntheticRootReplayAdvanceToken([]syntheticRootReplayFrame{cursor.frame}, tok)
			if len(advanced) == 0 {
				if syntheticRootReplayCanSkipGapByte(b.source[cursor.byte]) {
					nextByte := cursor.byte + 1
					nextPoint := advancePointByBytes(cursor.point, b.source[cursor.byte:nextByte])
					nextCursors = appendSyntheticRootReplayGapCursor(nextCursors, cursor.frame, nextByte, nextPoint)
				}
				continue
			}
			for _, frame := range advanced {
				nextCursors = appendSyntheticRootReplayGapCursor(nextCursors, frame, tok.EndByte, tok.EndPoint)
				if tok.EndByte == cursor.byte && cursor.byte < endByte {
					nextByte := cursor.byte + 1
					nextPoint := advancePointByBytes(cursor.point, b.source[cursor.byte:nextByte])
					nextCursors = appendSyntheticRootReplayGapCursor(nextCursors, frame, nextByte, nextPoint)
				}
			}
		}
		if allAtEnd {
			return syntheticRootReplayGapCursorFrames(cursors)
		}
		if len(nextCursors) == 0 {
			return nil
		}
		cursors = nextCursors
	}
	return nil
}

func syntheticRootReplayCanSkipGapByte(ch byte) bool {
	switch ch {
	case 32, 9, 10, 13, 12:
		return true
	default:
		return false
	}
}

func (b *resultRootBuild) syntheticRootReplayLexGapToken(frame syntheticRootReplayFrame, startByte uint32, startPoint Point, endByte uint32) (Token, bool) {
	if b == nil || b.parser == nil || b.lang == nil || len(b.lang.LexStates) == 0 {
		return Token{}, false
	}
	state, ok := b.syntheticRootReplayTopState(frame)
	if !ok {
		return Token{}, false
	}
	return b.syntheticRootReplayLexGapTokenForState(state, startByte, startPoint, endByte)
}

func (b *resultRootBuild) syntheticRootReplayLexGapTokenForState(state StateID, startByte uint32, startPoint Point, endByte uint32) (Token, bool) {
	if b == nil || b.parser == nil || b.lang == nil || len(b.lang.LexStates) == 0 {
		return Token{}, false
	}
	if startByte >= endByte || endByte > uint32(len(b.source)) {
		return Token{}, false
	}
	lexer := NewLexer(b.lang.LexStates, b.source)
	// Gate this gap-token lexer on the language-level C-recovery answer rather
	// than the live b.parser.errorCostCompetition flag. resolveCRecoverySwallowedError
	// (parser_api.go) temporarily forces p.errorCostCompetition = false around its
	// fallback comparison re-parse, so reading the parser flag here would flip the
	// gap-token error-mode lexing inside that window. b.lang is always
	// b.parser.language (see newResultRootBuild) and errorCostCompetitionLanguage is
	// a pure function of the language (p.errorCostCompetition is initialized from it
	// at NewParser), so it is the stable constant this replay needs — identical to
	// the parser flag outside the fallback window, and exactly the behavior this
	// path had before it was switched to read the mutable cached flag. Gap-token
	// replay only runs during synthetic-root reconstruction, off the steady-state
	// token loop, so the DiagnoseCRecoveryGate scan behind it stays out of the hot
	// path.
	ts := newDFATokenSourceDirectWithCRecovery(lexer, b.lang, b.parser.lookupActionIndex, b.parser.hasKeywordState, b.parser.externalValidByState, b.parser.externalValidMaskByState, errorCostCompetitionLanguage(b.lang))
	defer ts.Close()
	ts.SetParserState(state)
	ts.SeekTokenFrontier(startByte, startPoint)
	tok := ts.Next()
	if tok.Symbol == 0 || tok.NoLookahead {
		return Token{}, false
	}
	if tok.StartByte != startByte || tok.EndByte < tok.StartByte || tok.EndByte > endByte {
		return Token{}, false
	}
	return tok, true
}

func (b *resultRootBuild) syntheticRootReplayAdvanceToken(frontier []syntheticRootReplayFrame, tok Token) []syntheticRootReplayFrame {
	if len(frontier) == 0 || tok.Symbol == 0 {
		return nil
	}
	closed := b.syntheticRootReplayCloseLookahead(frontier, tok.Symbol)
	advanced := make([]syntheticRootReplayFrame, 0, len(closed))
	for _, frame := range closed {
		top, ok := b.syntheticRootReplayTopState(frame)
		if !ok {
			continue
		}
		for _, act := range b.syntheticRootReplayActions(frame, tok.Symbol) {
			if act.Type != ParseActionShift {
				continue
			}
			if act.Extra {
				advanced = appendSyntheticRootReplayFrame(advanced, frame)
				continue
			}
			target := extraShiftTargetState(top, act)
			if target == 0 {
				continue
			}
			advanced = appendSyntheticRootReplayFrame(advanced, b.syntheticRootReplayPush(frame, target))
		}
	}
	if len(advanced) == 0 {
		return nil
	}
	return b.syntheticRootReplayCloseEOF(advanced)
}

func appendSyntheticRootReplayGapCursor(cursors []syntheticRootReplayGapCursor, frame syntheticRootReplayFrame, pos uint32, point Point) []syntheticRootReplayGapCursor {
	if frame.top == 0 || len(cursors) >= syntheticRootReplayMaxFrontier {
		return cursors
	}
	for _, cursor := range cursors {
		if cursor.byte == pos && cursor.point == point && cursor.frame.top == frame.top {
			return cursors
		}
	}
	return append(cursors, syntheticRootReplayGapCursor{frame: frame, byte: pos, point: point})
}

func syntheticRootReplayGapCursorFrames(cursors []syntheticRootReplayGapCursor) []syntheticRootReplayFrame {
	frames := make([]syntheticRootReplayFrame, 0, len(cursors))
	for _, cursor := range cursors {
		frames = appendSyntheticRootReplayFrame(frames, cursor.frame)
	}
	return frames
}

func (b *resultRootBuild) syntheticRootReplayCloseEOF(frontier []syntheticRootReplayFrame) []syntheticRootReplayFrame {
	return b.syntheticRootReplayCloseLookahead(frontier, 0)
}

func (b *resultRootBuild) syntheticRootReplayCloseLookahead(frontier []syntheticRootReplayFrame, lookahead Symbol) []syntheticRootReplayFrame {
	if len(frontier) == 1 && frontier[0].top != 0 {
		key := syntheticRootReplayCloseKey{top: frontier[0].top, lookahead: lookahead}
		if cached, ok := b.replayStack.closeMemo[key]; ok {
			return cached
		}
		closed := b.syntheticRootReplayCloseLookaheadUncached(frontier, lookahead)
		if b.replayStack.closeMemo == nil {
			b.replayStack.closeMemo = make(map[syntheticRootReplayCloseKey][]syntheticRootReplayFrame, 64)
		}
		// Replay frames and their canonical stack nodes are immutable for the
		// lifetime of a result build, so the cached slice is read-only and safe
		// to share with the replay callers.
		closed = closed[:len(closed):len(closed)]
		b.replayStack.closeMemo[key] = closed
		return closed
	}
	return b.syntheticRootReplayCloseLookaheadUncached(frontier, lookahead)
}

func (b *resultRootBuild) syntheticRootReplayCloseLookaheadUncached(frontier []syntheticRootReplayFrame, lookahead Symbol) []syntheticRootReplayFrame {
	closed := make([]syntheticRootReplayFrame, 0, len(frontier))
	for _, frame := range frontier {
		closed = appendSyntheticRootReplayFrame(closed, frame)
	}
	for i := 0; i < len(closed); i++ {
		for _, act := range b.syntheticRootReplayActions(closed[i], lookahead) {
			if act.Type != ParseActionReduce {
				continue
			}
			next, ok := b.syntheticRootReplayReduce(closed[i], act)
			if !ok {
				continue
			}
			closed = appendSyntheticRootReplayFrame(closed, next)
		}
	}
	return closed
}

func (b *resultRootBuild) syntheticRootReplayReduce(frame syntheticRootReplayFrame, act ParseAction) (syntheticRootReplayFrame, bool) {
	childCount := int(act.ChildCount)
	predecessorFrame, ok := b.syntheticRootReplayPop(frame, childCount)
	if !ok {
		return syntheticRootReplayFrame{}, false
	}
	predecessor, ok := b.syntheticRootReplayTopState(predecessorFrame)
	if !ok {
		return syntheticRootReplayFrame{}, false
	}
	next := b.parser.lookupGoto(predecessor, act.Symbol)
	if next == 0 {
		return syntheticRootReplayFrame{}, false
	}
	return b.syntheticRootReplayPush(predecessorFrame, next), true
}

func (b *resultRootBuild) syntheticRootReplayActions(frame syntheticRootReplayFrame, lookahead Symbol) []ParseAction {
	if b == nil || b.parser == nil || b.lang == nil {
		return nil
	}
	state, ok := b.syntheticRootReplayTopState(frame)
	if !ok {
		return nil
	}
	idx := b.parser.lookupActionIndex(state, lookahead)
	if idx == 0 || int(idx) >= len(b.lang.ParseActions) {
		return nil
	}
	return b.lang.ParseActions[idx].Actions
}

func (b *resultRootBuild) syntheticRootReplayFrontierAcceptsEOF(frontier []syntheticRootReplayFrame) bool {
	for _, frame := range frontier {
		state, ok := b.syntheticRootReplayTopState(frame)
		if !ok {
			continue
		}
		if b.parser.stateHasAcceptOnEOF(state) {
			return true
		}
	}
	return false
}

func appendSyntheticRootReplayFrame(frames []syntheticRootReplayFrame, frame syntheticRootReplayFrame) []syntheticRootReplayFrame {
	if frame.top == 0 || len(frames) >= syntheticRootReplayMaxFrontier {
		return frames
	}
	for _, existing := range frames {
		if existing.top == frame.top {
			return frames
		}
	}
	return append(frames, frame)
}

func (b *resultRootBuild) syntheticRootReplaySkipsChild(child *Node) bool {
	return child == nil || child.isExtra()
}

func (b *resultRootBuild) syntheticRootReplayChild(state StateID, child *Node) (StateID, bool) {
	if b == nil || b.parser == nil || b.lang == nil || child == nil {
		return 0, false
	}
	if b.lang.TokenCount > 0 && uint32(child.symbol) < b.lang.TokenCount {
		return b.parser.shiftTargetForStateSymbol(state, child.symbol)
	}
	next := b.parser.lookupGoto(state, child.symbol)
	return next, next != 0
}

func (b *resultRootBuild) expectedRootEmptyFrameAcceptsEOF() bool {
	if b == nil || b.parser == nil || b.lang == nil {
		return false
	}
	next := b.parser.lookupGoto(b.lang.InitialState, b.expectedRootSymbol)
	return next != 0 && b.parser.stateHasAcceptOnEOF(next)
}

func (b *resultRootBuild) repairPythonKeywordNode(node *Node) *Node {
	timing := b.parser.currentMaterializationTiming()
	start := materializationTimingStart(timing)
	node = repairPythonKeywordErrorNode(node, b.source, b.arena, b.lang)
	timing.addPythonKeywordRepair(start)
	return node
}

func (b *resultRootBuild) repairPythonKeywordNodes(nodes []*Node) []*Node {
	timing := b.parser.currentMaterializationTiming()
	start := materializationTimingStart(timing)
	nodes, _ = repairPythonKeywordErrorNodes(nodes, b.source, b.arena, b.lang)
	timing.addPythonKeywordRepair(start)
	return nodes
}

func (b *resultRootBuild) repairPythonRoot(root *Node) *Node {
	timing := b.parser.currentMaterializationTiming()
	start := materializationTimingStart(timing)
	root = repairPythonRootNode(root, b.arena, b.lang)
	timing.addPythonRootRepair(start)
	return root
}

func (b *resultRootBuild) singleChildSlice(child *Node) []*Node {
	if b.arena != nil {
		children := b.arena.allocNodeSlice(1)
		children[0] = child
		return children
	}
	return []*Node{child}
}

func (b *resultRootBuild) finishTree(root *Node, wireParentLinks, extendTrailing bool) *Tree {
	errorSummary, compatibilityApplied := b.finalizeRoot(root, wireParentLinks, extendTrailing)
	borrowed := b.borrowedArenas()
	if b.parser != nil {
		borrowed = append(borrowed, b.parser.takeCompatibilityBorrowedArenas()...)
	}
	tree := newTreeWithArenas(root, b.source, b.lang, b.arena, borrowed)
	tree.resultErrorSummary = errorSummary
	tree.resultCompatibilityApplied = compatibilityApplied
	if b.parser.shouldDeferResultCompatibility(root) {
		tree.deferResultCompatibility()
	}
	return tree
}

func (b *resultRootBuild) finalizeRoot(root *Node, wireParentLinks, extendTrailing bool) (resultErrorSummary, bool) {
	return b.parser.finalizeResultRoot(root, b.source, b.linkScratch, wireParentLinks, extendTrailing, b.incrementalResultCompatibilityRanges(root))
}

// incrementalResultCompatibilityRanges returns the reparsed byte spans that
// range-limit result normalization for THIS parse, or nil when the language is
// not range-limit-eligible or no subtree was reused. A fresh parse reuses
// nothing, so it always gets nil and keeps its unchanged full-tree walk.
func (b *resultRootBuild) incrementalResultCompatibilityRanges(root *Node) []Range {
	if b.parser == nil || b.lang == nil || b.parser.forceFullResultNormalizationWalk || !languageUsesRangeLimitedResultCompatibility(b.lang) {
		return nil
	}
	hasReuse := b.reuseState != nil && b.reuseState.reusedAny
	return incrementalReparsedTopLevelRanges(root, b.arena, hasReuse)
}

// finalizeWrappedSubtree applies subtree compatibility normalization to a node
// that is about to become a CHILD of a synthetic wrapper root. It deliberately
// omits the root-span mutations that finalizeResultRoot performs
// (normalizeRootSourceStart / extendNodeToTrailingWhitespace) because those are
// only correct for the actual root — the wrapper root absorbs the leading/trailing
// trivia. The compatibility guard mirrors finalizeResultRoot exactly.
func (b *resultRootBuild) finalizeWrappedSubtree(root *Node) {
	p := b.parser
	if reason := p.resultMaterializationStopReason(root.ownerArena); resultMaterializationShouldStop(reason) {
		return
	}
	if p == nil || (!p.noResultCompatibilityBenchmarkOnly && !p.shouldDeferResultCompatibility(root)) {
		if compat := normalizeResultCompatibility(root, b.source, p, nil); parseStopReasonIsActive(compat.stopReason) && p != nil {
			p.markActiveParseStopped(compat.stopReason)
		}
	}
}

func (b *resultRootBuild) borrowedArenas() []*nodeArena {
	if b.borrowedResolved {
		return b.borrowed
	}
	b.borrowed = b.reuseState.retainBorrowed(b.arena)
	b.borrowedResolved = true
	return b.borrowed
}

func (b *resultRootBuild) isLanguage(name string) bool {
	return b.lang != nil && b.lang.Name == name
}

func resultNodesHaveError(nodes []*Node) bool {
	for _, node := range nodes {
		if node != nil && (node.IsError() || node.HasError()) {
			return true
		}
	}
	return false
}

func retagResultRoot(root *Node, sym Symbol, named bool) {
	if root == nil {
		return
	}
	root.symbol = sym
	root.setNamed(named)
}

func retagResultRootAndRefreshError(root *Node, sym Symbol, named bool) {
	retagResultRoot(root, sym, named)
	refreshResultRootError(root)
}

func refreshResultRootError(root *Node) {
	if root == nil {
		return
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && (child.IsError() || child.HasError()) {
			root.setHasError(true)
			return
		}
	}
	root.setHasError(false)
}

type resultRootExtraSplit struct {
	realRoot      *Node
	allExtras     []*Node
	visibleExtras []*Node
}

func splitResultRootExtras(nodes []*Node, lang *Language) resultRootExtraSplit {
	var split resultRootExtraSplit
	for _, n := range nodes {
		if n.isExtra() {
			split.allExtras = append(split.allExtras, n)
			if symbolIsVisible(lang, n.symbol) && n.endByte > n.startByte {
				split.visibleExtras = append(split.visibleExtras, n)
			}
			continue
		}
		if split.realRoot != nil {
			split.realRoot = nil
			return split
		}
		split.realRoot = n
	}
	return split
}

func (s resultRootExtraSplit) canFoldVisibleExtras() bool {
	if len(s.visibleExtras) == 0 {
		return false
	}
	for _, extra := range s.allExtras {
		if extra != nil && (extra.IsError() || extra.HasError()) {
			return false
		}
	}
	return true
}

func classifyResultRootExtras(extras []*Node, lang *Language) resultRootExtraSplit {
	split := resultRootExtraSplit{allExtras: extras}
	for _, extra := range extras {
		if extra != nil && symbolIsVisible(lang, extra.symbol) && extra.endByte > extra.startByte {
			split.visibleExtras = append(split.visibleExtras, extra)
		}
	}
	return split
}

// attachResultRootExtraSplit publishes only visible, positive-width extras as
// root children. All extras still contribute their source range. This keeps
// production and forest materialization aligned for scanner synchronization
// tokens that carry state but no source bytes.
func attachResultRootExtraSplit(root *Node, split resultRootExtraSplit, arena *nodeArena) {
	if root == nil || len(split.allExtras) == 0 {
		return
	}
	if split.canFoldVisibleExtras() {
		foldResultRootExtras(root, split.visibleExtras, arena)
	}
	extendResultRootRangeToExtras(root, split.allExtras)
}

func foldResultRootExtras(root *Node, extras []*Node, arena *nodeArena) {
	if root == nil || len(extras) == 0 {
		return
	}
	var leadingExtras []*Node
	var trailingExtras []*Node
	for _, extra := range extras {
		if extra.startByte <= root.startByte {
			leadingExtras = append(leadingExtras, extra)
		} else {
			trailingExtras = append(trailingExtras, extra)
		}
	}
	if resultMutableChildrenForMutation(root).SurroundFinalRefs(leadingExtras, trailingExtras) {
		extendResultRootRangeToExtras(root, extras)
		return
	}
	rootChildren := resultChildSliceForMutation(root)
	merged := make([]*Node, 0, len(extras)+len(rootChildren))
	leadingCount := 0
	for _, extra := range leadingExtras {
		merged = append(merged, extra)
		leadingCount++
	}
	merged = append(merged, rootChildren...)
	merged = append(merged, trailingExtras...)
	if arena != nil {
		out := arena.allocNodeSlice(len(merged))
		copy(out, merged)
		merged = out
	}
	root.children = merged

	rootFieldIDs := root.fieldIDs()
	if len(rootFieldIDs) > 0 {
		trailingCount := len(extras) - leadingCount
		padded := make([]FieldID, leadingCount+len(rootFieldIDs)+trailingCount)
		copy(padded[leadingCount:], rootFieldIDs)
		paddedSources := root.fieldSources()
		if len(paddedSources) > 0 {
			rootFieldSources := paddedSources
			paddedSources = make([]uint8, len(padded))
			copy(paddedSources[leadingCount:], rootFieldSources)
		}
		root.setFieldMetadata(padded, paddedSources)
	}
	extendResultRootRangeToExtras(root, extras)
}

func extendResultRootRangeToExtras(root *Node, extras []*Node) {
	if root == nil {
		return
	}
	for _, extra := range extras {
		if extra == nil {
			continue
		}
		if extra.startByte < root.startByte {
			root.startByte = extra.startByte
			root.startPoint = extra.startPoint
		}
		if extra.endByte > root.endByte {
			root.endByte = extra.endByte
			root.endPoint = extra.endPoint
		}
	}
}

func (p *Parser) finalizeResultRoot(root *Node, source []byte, linkScratch *[]*Node, wireParentLinks, extendTrailing bool, incrementalRanges []Range) (resultErrorSummary, bool) {
	errorSummary := resultErrorSummaryUnknown
	compatibilityApplied := false
	if root == nil {
		return errorSummary, compatibilityApplied
	}
	timing := p.currentMaterializationTiming()
	finalizeStart := materializationTimingStart(timing)
	defer timing.addResultFinalizeRoot(finalizeStart)
	if reason := p.resultMaterializationStopReason(root.ownerArena); resultMaterializationShouldStop(reason) {
		return errorSummary, compatibilityApplied
	}
	hasHiddenRootTrivia := false
	if p != nil {
		root, hasHiddenRootTrivia = flattenInvisibleRootChildrenWithTriviaHint(
			root,
			root.ownerArena,
			p.language,
			source,
		)
	}
	widenNodeSpanToRetainedChildren(root)
	if reason := p.resultMaterializationStopReason(root.ownerArena); resultMaterializationShouldStop(reason) {
		return errorSummary, compatibilityApplied
	}
	if extendTrailing {
		start := materializationTimingStart(timing)
		extendNodeToTrailingWhitespace(root, source)
		timing.addResultExtendTrailing(start)
	}
	if reason := p.resultMaterializationStopReason(root.ownerArena); resultMaterializationShouldStop(reason) {
		return errorSummary, compatibilityApplied
	}
	start := materializationTimingStart(timing)
	p.normalizeRootSourceStart(root, source)
	timing.addResultNormalizeRootStart(start)
	if reason := p.resultMaterializationStopReason(root.ownerArena); resultMaterializationShouldStop(reason) {
		return errorSummary, compatibilityApplied
	}
	// A clean root owns hidden whitespace extras as span coverage. It does not
	// publish them as children. Apply this rule to every result route. Error
	// roots retain the extras as recovery evidence.
	if p != nil && p.language != nil && !root.hasError() && hasHiddenRootTrivia {
		filterHiddenExtraTriviaRoot(root, source, p.language)
		if extendTrailing {
			extendNodeToTrailingWhitespace(root, source)
		}
	}
	summarizeDuringParentLinks := false
	if p == nil || (!p.noResultCompatibilityBenchmarkOnly && !p.shouldDeferResultCompatibility(root)) {
		start = materializationTimingStart(timing)
		summarizeDuringParentLinks = p != nil &&
			wireParentLinks &&
			!p.shouldDeferResultParentLinks(root)
		var compat resultCompatibilityResult
		if summarizeDuringParentLinks {
			compat = applyResultCompatibilityForParentLinkSummary(root, source, p, incrementalRanges)
		} else {
			compat = normalizeResultCompatibility(root, source, p, incrementalRanges)
		}
		errorSummary = compat.errorSummary
		// resultMaterializationShouldStop (not parseStopReasonIsActive): Go's
		// normalizer can now report ParseStopMemoryBudget (see
		// normalizeGoReturnedTreeCompatibility), which parseStopReasonIsActive
		// deliberately excludes. A budget-stopped compat pass did not apply
		// cleanly and must not be treated as "compatibility applied" any more
		// than a timeout/cancellation-stopped one is.
		compatibilityApplied = p != nil && p.language != nil && !resultMaterializationShouldStop(compat.stopReason)
		if parseStopReasonIsActive(compat.stopReason) && p != nil {
			p.markActiveParseStopped(compat.stopReason)
		}
		timing.addResultCompatibility(start)
		// A compatibility pass can shrink the root below the source end.
		// Re-extend the root so it keeps trailing whitespace as span coverage.
		if extendTrailing {
			extendNodeToTrailingWhitespace(root, source)
		}
	}
	if reason := p.resultMaterializationStopReason(root.ownerArena); resultMaterializationShouldStop(reason) {
		return errorSummary, compatibilityApplied
	}
	if wireParentLinks {
		start = materializationTimingStart(timing)
		if p != nil && p.shouldDeferResultParentLinks(root) {
			root.ownerArena.deferParentLinks(root)
		} else {
			var summaryTarget *resultErrorSummary
			if summarizeDuringParentLinks {
				summaryTarget = &errorSummary
			}
			if !wireParentLinksWithScratchUntil(root, linkScratch, p, summaryTarget) && root.ownerArena != nil {
				root.ownerArena.deferParentLinks(root)
			}
		}
		timing.addResultParentLink(start)
	}
	return errorSummary, compatibilityApplied
}

func (p *Parser) shouldDeferResultCompatibility(root *Node) bool {
	if p == nil || p.language == nil || root == nil || p.noResultCompatibilityBenchmarkOnly || p.noTreeBenchmarkOnly {
		return false
	}
	if !parseTypeScriptLazyResultCompatibilityEnabled() {
		return false
	}
	switch p.language.Name {
	case "typescript", "tsx":
		return true
	default:
		return false
	}
}

func (p *Parser) shouldDeferResultParentLinks(root *Node) bool {
	if p == nil || p.language == nil || root == nil || root.ownerArena == nil {
		return false
	}
	if p.noResultCompatibilityBenchmarkOnly && !p.noTreeBenchmarkOnly {
		return true
	}
	if p.noTreeBenchmarkOnly {
		return false
	}
	switch p.language.Name {
	case "java", "python", "typescript", "tsx":
		return true
	default:
		return false
	}
}

func (p *Parser) normalizeRootSourceStart(root *Node, source []byte) {
	if root == nil || len(source) == 0 {
		return
	}
	// Included-range parses intentionally preserve range-local root spans.
	if p != nil && len(p.included) > 0 {
		return
	}
	// C excludes leading token padding from the root span. Keep a prefix only
	// when a retained child owns it. This rule covers production, compact,
	// forest, deferred, and incremental result materialization.
	firstSource := firstNonTriviaByteStart(source)
	if firstSource == root.startByte {
		return
	}
	if firstSource < root.startByte {
		// A root can start after a dropped leading extra. Pull it back to the
		// first source token so the root still covers the lost token.
		root.startByte = firstSource
		root.startPoint = advancePointByBytes(Point{}, source[:firstSource])
		return
	}
	if firstSource == 0 || resultChildCount(root) == 0 {
		return
	}
	firstChild := resultChildAt(root, 0)
	if firstChild == nil || firstChild.startByte != firstSource {
		// A retained child can own the prefix through error recovery or an
		// explicit extra. Do not move the root past that ownership.
		return
	}
	root.startByte = firstSource
	root.startPoint = firstChild.startPoint
}

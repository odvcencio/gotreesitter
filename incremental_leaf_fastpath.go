package gotreesitter

import "time"

func (p *Parser) tryTokenInvariantLeafEdit(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming) (*Tree, bool) {
	if p == nil || oldTree == nil || oldTree.RootNode() == nil || oldTree.language != p.language {
		return nil, false
	}
	if len(oldTree.edits) != 1 {
		return nil, false
	}
	edit := oldTree.edits[0]
	if edit.NewEndByte-edit.StartByte != edit.OldEndByte-edit.StartByte {
		return nil, false
	}
	if edit.NewEndPoint != edit.OldEndPoint || edit.OldEndByte <= edit.StartByte {
		return nil, false
	}
	if len(source) != len(oldTree.source) {
		return nil, false
	}
	root := oldTree.RootNode()
	leaf := oldTree.lastEditedLeaf
	if leaf == nil || !leaf.containsByteRange(edit.StartByte, edit.OldEndByte) {
		leaf = root.DescendantForByteRange(edit.StartByte, edit.OldEndByte)
	}
	if leaf == nil || leaf.ChildCount() != 0 || leaf.hasError() || leaf.isMissing() {
		return nil, false
	}
	start := time.Time{}
	if timing != nil {
		start = time.Now()
	}
	tok, ok := p.scanTokenInvariantEditedLeaf(source, ts, leaf)
	if !ok || tok.Symbol != leaf.symbol || tok.StartByte != leaf.startByte || tok.EndByte != leaf.endByte {
		return nil, false
	}
	tree := reuseTreeWithNewSource(oldTree, source, leaf)
	if tree == nil || tree.root == nil {
		return nil, false
	}
	tree.setParseRuntime(ParseRuntime{
		StopReason:       ParseStopAccepted,
		SourceLen:        uint32(len(source)),
		TokensConsumed:   1,
		LastTokenEndByte: tok.EndByte,
		LastTokenSymbol:  tok.Symbol,
		ExpectedEOFByte:  uint32(len(source)),
		RootEndByte:      tree.root.EndByte(),
		MaxStacksSeen:    1,
	})
	if timing != nil {
		timing.reuseNanos += time.Since(start).Nanoseconds()
		timing.reusedSubtrees++
		timing.reusedBytes += uint64(len(source))
		timing.maxStacksSeen = 1
		timing.stopReason = ParseStopAccepted
		timing.tokensConsumed = 1
		timing.lastTokenEndByte = tok.EndByte
		timing.expectedEOFByte = uint32(len(source))
		timing.singleStackIterations = 1
		timing.singleStackTokens = 1
	}
	return tree, true
}

func (p *Parser) tokenInvariantLeafEditCandidate(source []byte, oldTree *Tree) (*Node, InputEdit, bool) {
	if p == nil || oldTree == nil || oldTree.RootNode() == nil || oldTree.language != p.language {
		return nil, InputEdit{}, false
	}
	if len(oldTree.edits) != 1 {
		return nil, InputEdit{}, false
	}
	edit := oldTree.edits[0]
	if !inputEditPreservesTokenExtent(edit) {
		return nil, InputEdit{}, false
	}
	if len(source) != len(oldTree.source) {
		return nil, InputEdit{}, false
	}
	return oldTree.RootNode(), edit, true
}

func inputEditPreservesTokenExtent(edit InputEdit) bool {
	if edit.NewEndByte-edit.StartByte != edit.OldEndByte-edit.StartByte {
		return false
	}
	return edit.NewEndPoint == edit.OldEndPoint && edit.OldEndByte > edit.StartByte
}

func tokenInvariantEditedLeaf(root *Node, oldTree *Tree, edit InputEdit) *Node {
	if root == nil || oldTree == nil {
		return nil
	}
	leaf := oldTree.lastEditedLeaf
	if leaf == nil || !leaf.containsByteRange(edit.StartByte, edit.OldEndByte) {
		leaf = root.DescendantForByteRange(edit.StartByte, edit.OldEndByte)
	}
	if !tokenInvariantLeafReusable(leaf) {
		return nil
	}
	return leaf
}

func tokenInvariantLeafReusable(leaf *Node) bool {
	return leaf != nil && leaf.ChildCount() == 0 && !leaf.hasError() && !leaf.isMissing()
}

func tokenInvariantReuseStart(timing *incrementalParseTiming) time.Time {
	if timing != nil {
		return time.Now()
	}
	return time.Time{}
}

func (p *Parser) scanTokenInvariantEditedLeaf(source []byte, ts TokenSource, leaf *Node) (Token, bool) {
	tok, ok := scanLeafTokenWithoutMutatingSource(ts, leaf)
	if ok {
		return tok, true
	}
	if tok, ok = p.scanLeafTokenWithFreshSource(source, leaf); ok {
		return tok, true
	}

	restoreState, ok := snapshotTokenSourceState(ts)
	if !ok {
		return Token{}, false
	}
	defer restoreState()

	stateful, ok := ts.(parserStateTokenSource)
	if !ok {
		return Token{}, false
	}
	stateful.SetParserState(leaf.preGotoState)
	stateful.SetGLRStates(nil)
	return skipTokenSourceToLeaf(ts, leaf)
}

func (p *Parser) scanLeafTokenWithFreshSource(source []byte, leaf *Node) (Token, bool) {
	if p == nil || p.reparseFactory == nil || leaf == nil {
		return Token{}, false
	}
	fresh, err := p.reparseFactory(source)
	if err != nil || fresh == nil {
		return Token{}, false
	}
	release := manageTokenSourceLifetime(fresh)
	defer release()

	ts := p.wrapIncludedRanges(fresh)
	if stateful, ok := ts.(parserStateTokenSource); ok {
		stateful.SetParserState(leaf.preGotoState)
		stateful.SetGLRStates(nil)
	}
	return skipTokenSourceToLeaf(ts, leaf)
}

func skipTokenSourceToLeaf(ts TokenSource, leaf *Node) (Token, bool) {
	if skipper, ok := ts.(PointSkippableTokenSource); ok {
		return skipper.SkipToByteWithPoint(leaf.startByte, leaf.startPoint), true
	}
	if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		return skipper.SkipToByte(leaf.startByte), true
	}
	return Token{}, false
}

func tokenMatchesLeaf(tok Token, leaf *Node) bool {
	return leaf != nil && tok.Symbol == leaf.symbol && tok.StartByte == leaf.startByte && tok.EndByte == leaf.endByte
}

func setTokenInvariantReuseRuntime(tree *Tree, source []byte, tok Token) {
	tree.setParseRuntime(ParseRuntime{
		StopReason:       ParseStopAccepted,
		SourceLen:        uint32(len(source)),
		TokensConsumed:   1,
		LastTokenEndByte: tok.EndByte,
		LastTokenSymbol:  tok.Symbol,
		ExpectedEOFByte:  uint32(len(source)),
		RootEndByte:      tree.root.EndByte(),
		MaxStacksSeen:    1,
	})
}

func recordTokenInvariantReuseTiming(timing *incrementalParseTiming, source []byte, tok Token, start time.Time) {
	if timing != nil {
		timing.reuseNanos += time.Since(start).Nanoseconds()
		timing.reusedSubtrees++
		timing.reusedBytes += uint64(len(source))
		timing.maxStacksSeen = 1
		timing.stopReason = ParseStopAccepted
		timing.tokensConsumed = 1
		timing.lastTokenEndByte = tok.EndByte
		timing.expectedEOFByte = uint32(len(source))
		timing.singleStackIterations = 1
		timing.singleStackTokens = 1
	}
}

func scanLeafTokenWithoutMutatingSource(ts TokenSource, leaf *Node) (Token, bool) {
	if leaf == nil {
		return Token{}, false
	}
	switch typed := ts.(type) {
	case *dfaTokenSource:
		return scanDFALeafTokenWithoutMutatingSource(typed, leaf)
	case *includedRangeTokenSource:
		if typed == nil {
			return Token{}, false
		}
		base, ok := typed.base.(*dfaTokenSource)
		if !ok {
			return Token{}, false
		}
		snapshot, ok := prepareDFALeafScan(base, leaf)
		if !ok {
			return Token{}, false
		}
		idx := typed.idx
		tok := typed.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
		restoreDFALeafScan(base, snapshot)
		typed.idx = idx
		return tok, true
	default:
		return Token{}, false
	}
}

func scanDFALeafTokenWithoutMutatingSource(dts *dfaTokenSource, leaf *Node) (Token, bool) {
	if dts != nil && languageUsesExternalScannerCheckpoints(dts.language) {
		return scanDFALeafTokenWithExternalCheckpoint(dts, leaf)
	}
	snapshot, ok := prepareDFALeafScan(dts, leaf)
	if !ok {
		return Token{}, false
	}
	tok := dts.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
	restoreDFALeafScan(dts, snapshot)
	return tok, true
}

func scanDFALeafTokenWithExternalCheckpoint(dts *dfaTokenSource, leaf *Node) (Token, bool) {
	if dts == nil || dts.lexer == nil || leaf == nil {
		return Token{}, false
	}
	cp, ok := externalScannerCheckpointForNode(leaf)
	if !ok {
		return Token{}, false
	}
	snapshot, ok := snapshotDFATokenSourceState(dts)
	if !ok {
		return Token{}, false
	}
	defer restoreDFATokenSourceState(dts, snapshot)

	dts.state = leaf.preGotoState
	dts.glrStates = nil
	dts.restoreExternalScannerState(cp.start)
	tok := dts.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
	if tok.Symbol != leaf.symbol || tok.StartByte != leaf.startByte || tok.EndByte != leaf.endByte {
		return Token{}, false
	}
	if !dts.externalScannerStateMatches(cp.end) {
		return Token{}, false
	}
	return tok, true
}

type dfaLeafScanSnapshot struct {
	state                  StateID
	glrStates              []StateID
	lexer                  Lexer
	lastExternalTokenStart uint32
	lastExternalTokenEnd   uint32
	lastExternalTokenValid bool
	extZeroPos             int
	extZeroState           StateID
	zeroWidthPos           int
	zeroWidthCount         int
}

func prepareDFALeafScan(dts *dfaTokenSource, leaf *Node) (dfaLeafScanSnapshot, bool) {
	if dts == nil || dts.lexer == nil || dts.language == nil || leaf == nil {
		return dfaLeafScanSnapshot{}, false
	}
	if dts.language.ExternalScanner != nil || len(dts.language.ExternalSymbols) != 0 {
		return dfaLeafScanSnapshot{}, false
	}
	snapshot := dfaLeafScanSnapshot{
		state:                  dts.state,
		glrStates:              dts.glrStates,
		lexer:                  *dts.lexer,
		lastExternalTokenStart: dts.lastExternalTokenStartByte,
		lastExternalTokenEnd:   dts.lastExternalTokenEndByte,
		lastExternalTokenValid: dts.lastExternalTokenValid,
		extZeroPos:             dts.extZeroPos,
		extZeroState:           dts.extZeroState,
		zeroWidthPos:           dts.zeroWidthPos,
		zeroWidthCount:         dts.zeroWidthCount,
	}
	dts.state = leaf.preGotoState
	dts.glrStates = nil
	return snapshot, true
}

func restoreDFALeafScan(dts *dfaTokenSource, snapshot dfaLeafScanSnapshot) {
	dts.state = snapshot.state
	dts.glrStates = snapshot.glrStates
	*dts.lexer = snapshot.lexer
	dts.lastExternalTokenStartByte = snapshot.lastExternalTokenStart
	dts.lastExternalTokenEndByte = snapshot.lastExternalTokenEnd
	dts.lastExternalTokenValid = snapshot.lastExternalTokenValid
	dts.extZeroPos = snapshot.extZeroPos
	dts.extZeroState = snapshot.extZeroState
	dts.zeroWidthPos = snapshot.zeroWidthPos
	dts.zeroWidthCount = snapshot.zeroWidthCount
}

type dfaTokenSourceStateSnapshot struct {
	state                  StateID
	glrStates              []StateID
	lexer                  Lexer
	hasLexer               bool
	externalValid          []bool
	extZeroTried           []bool
	externalTokenStart     []byte
	externalTokenEnd       []byte
	externalSnapshot       []byte
	externalRetrySnap      []byte
	externalCompare        []byte
	externalScannerState   []byte
	externalLexer          ExternalLexer
	externalRetryLexer     ExternalLexer
	lastExternalTokenStart uint32
	lastExternalTokenEnd   uint32
	lastExternalTokenValid bool
	extZeroPos             int
	extZeroState           StateID
	zeroWidthPos           int
	zeroWidthCount         int
}

func snapshotTokenSourceState(ts TokenSource) (func(), bool) {
	switch typed := ts.(type) {
	case *dfaTokenSource:
		snapshot, ok := snapshotDFATokenSourceState(typed)
		if !ok {
			return nil, false
		}
		return func() {
			restoreDFATokenSourceState(typed, snapshot)
		}, true
	case *includedRangeTokenSource:
		restoreBase, ok := snapshotTokenSourceState(typed.base)
		if !ok {
			return nil, false
		}
		idx := typed.idx
		return func() {
			restoreBase()
			typed.idx = idx
		}, true
	default:
		return nil, false
	}
}

func snapshotDFATokenSourceState(dts *dfaTokenSource) (dfaTokenSourceStateSnapshot, bool) {
	if dts == nil {
		return dfaTokenSourceStateSnapshot{}, false
	}
	state := dfaTokenSourceStateSnapshot{
		state:                  dts.state,
		glrStates:              append([]StateID(nil), dts.glrStates...),
		externalValid:          append([]bool(nil), dts.externalValid...),
		extZeroTried:           append([]bool(nil), dts.extZeroTried...),
		externalTokenStart:     append([]byte(nil), dts.externalTokenStart...),
		externalTokenEnd:       append([]byte(nil), dts.externalTokenEnd...),
		externalSnapshot:       append([]byte(nil), dts.externalSnapshot...),
		externalRetrySnap:      append([]byte(nil), dts.externalRetrySnap...),
		externalCompare:        append([]byte(nil), dts.externalCompare...),
		externalLexer:          dts.externalLexer,
		externalRetryLexer:     dts.externalRetryLexer,
		lastExternalTokenStart: dts.lastExternalTokenStartByte,
		lastExternalTokenEnd:   dts.lastExternalTokenEndByte,
		lastExternalTokenValid: dts.lastExternalTokenValid,
		extZeroPos:             dts.extZeroPos,
		extZeroState:           dts.extZeroState,
		zeroWidthPos:           dts.zeroWidthPos,
		zeroWidthCount:         dts.zeroWidthCount,
	}
	if dts.language != nil && dts.language.ExternalScanner != nil {
		buf := make([]byte, 0, externalScannerSerializationBufferSize)
		state.externalScannerState = append([]byte(nil), dts.captureExternalScannerStateInto(&buf)...)
	}
	if dts.lexer != nil {
		state.lexer = *dts.lexer
		state.hasLexer = true
	}
	return state, true
}

func restoreDFATokenSourceState(dts *dfaTokenSource, state dfaTokenSourceStateSnapshot) {
	if dts == nil {
		return
	}
	dts.state = state.state
	dts.glrStates = append(dts.glrStates[:0], state.glrStates...)
	if dts.lexer == nil {
		dts.lexer = &Lexer{}
	}
	if state.hasLexer {
		*dts.lexer = state.lexer
	} else {
		dts.lexer = nil
	}
	dts.externalValid = append(dts.externalValid[:0], state.externalValid...)
	dts.extZeroTried = append(dts.extZeroTried[:0], state.extZeroTried...)
	dts.externalTokenStart = append(dts.externalTokenStart[:0], state.externalTokenStart...)
	dts.externalTokenEnd = append(dts.externalTokenEnd[:0], state.externalTokenEnd...)
	dts.externalSnapshot = append(dts.externalSnapshot[:0], state.externalSnapshot...)
	dts.externalRetrySnap = append(dts.externalRetrySnap[:0], state.externalRetrySnap...)
	dts.externalCompare = append(dts.externalCompare[:0], state.externalCompare...)
	dts.externalLexer = state.externalLexer
	dts.externalRetryLexer = state.externalRetryLexer
	dts.lastExternalTokenStartByte = state.lastExternalTokenStart
	dts.lastExternalTokenEndByte = state.lastExternalTokenEnd
	dts.lastExternalTokenValid = state.lastExternalTokenValid
	dts.extZeroPos = state.extZeroPos
	dts.extZeroState = state.extZeroState
	dts.zeroWidthPos = state.zeroWidthPos
	dts.zeroWidthCount = state.zeroWidthCount
	if dts.language != nil && dts.language.ExternalScanner != nil {
		dts.language.ExternalScanner.Deserialize(dts.externalPayload, state.externalScannerState)
	}
}

func reuseTreeWithNewSource(oldTree *Tree, source []byte, dirtyLeaf *Node) *Tree {
	if oldTree == nil || oldTree.root == nil {
		return nil
	}
	arena := oldTree.arena
	if arena != nil {
		arena.Retain()
	}
	borrowed := retainBorrowedArenasForReusedTree(oldTree, arena)
	clearDirtyPathToRoot(dirtyLeaf)
	return newTreeWithUniqueArenas(oldTree.root, source, oldTree.language, arena, borrowed)
}

func clearDirtyPathToRoot(n *Node) {
	for n != nil {
		n.setDirty(false)
		n = n.parent
	}
}

func retainBorrowedArenasForReusedTree(oldTree *Tree, primary *nodeArena) []*nodeArena {
	if oldTree == nil || len(oldTree.borrowedArena) == 0 {
		return nil
	}
	var borrowed []*nodeArena
	for _, arena := range oldTree.borrowedArena {
		if arena == nil || arena == primary {
			continue
		}
		duplicate := false
		for _, existing := range borrowed {
			if existing == arena {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		arena.Retain()
		borrowed = append(borrowed, arena)
	}
	return borrowed
}

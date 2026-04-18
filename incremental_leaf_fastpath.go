package gotreesitter

import "time"

func (p *Parser) tryTokenInvariantLeafEdit(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming) (*Tree, bool) {
	if p == nil || oldTree == nil || oldTree.RootNode() == nil || oldTree.language != p.language {
		return nil, false
	}
	if len(oldTree.edits) != 1 || languageUsesExternalScannerCheckpoints(p.language) {
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
	restoreState, ok := snapshotTokenSourceState(ts)
	if !ok {
		return nil, false
	}
	defer restoreState()
	root := oldTree.RootNode()
	leaf := oldTree.lastEditedLeaf
	if leaf == nil || !leaf.containsByteRange(edit.StartByte, edit.OldEndByte) {
		leaf = root.DescendantForByteRange(edit.StartByte, edit.OldEndByte)
	}
	if leaf == nil || leaf.ChildCount() != 0 || leaf.hasError || leaf.isMissing || leaf.isExtra {
		return nil, false
	}
	stateful, ok := ts.(parserStateTokenSource)
	if !ok {
		return nil, false
	}
	start := time.Time{}
	if timing != nil {
		start = time.Now()
	}
	stateful.SetParserState(leaf.preGotoState)
	stateful.SetGLRStates(nil)
	var tok Token
	if skipper, ok := ts.(PointSkippableTokenSource); ok {
		tok = skipper.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
	} else if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		tok = skipper.SkipToByte(leaf.startByte)
	} else {
		return nil, false
	}
	if tok.Symbol != leaf.symbol || tok.StartByte != leaf.startByte || tok.EndByte != leaf.endByte {
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
	borrowed := make([]*nodeArena, 0, len(oldTree.borrowedArena)+1)
	if oldTree.arena != nil {
		oldTree.arena.Retain()
		borrowed = append(borrowed, oldTree.arena)
	}
	for _, a := range oldTree.borrowedArena {
		if a == nil {
			continue
		}
		a.Retain()
		borrowed = append(borrowed, a)
	}
	clearDirtyPathToRoot(dirtyLeaf)
	return newTreeWithArenas(oldTree.root, source, oldTree.language, nil, borrowed)
}

func clearDirtyPathToRoot(n *Node) {
	for n != nil {
		n.dirty = false
		n = n.parent
	}
}

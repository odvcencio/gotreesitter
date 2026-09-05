//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"time"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func (p *Parser) recordLegacyParserEntry() {
	if runner, ok := p.admissionCandidateRunner.(*parserCoreFreshFullRunner); ok {
		runner.legacyParseRuns++
	}
}

// compactIncrementalReuseSession owns references for one incremental attempt.
// Keys start at one. The Core stores keys, never public node pointers.
type compactIncrementalReuseSession struct {
	oldTree        *Tree
	nodes          []*Node
	reuseState     parseReuseState
	projection     compactBorrowedProjectionScratch
	scheduler      *diagnosticParserCoreGenericScheduler
	timing         *incrementalParseTiming
	cursor         reuseCursor
	reusedSubtrees uint64
	reusedBytes    uint64
}

func (p *Parser) attemptCompactIncrementalParse(source []byte, oldTree *Tree, timing *incrementalParseTiming) (*Tree, string) {
	if oldTree == nil || oldTree.language != p.language || !oldTree.compactMaterialized ||
		oldTree.incrementalReuseDisabled || len(oldTree.edits) == 0 ||
		oldTree.root == nil || oldTree.root.HasError() || p.recoveryInitialOnly ||
		!p.admissionCandidateFullParseEligible(nil, true) {
		return nil, ""
	}
	// Preserve the existing token-invariant fast path for same-width leaf edits.
	// Width-changing edits cannot use that optimization.
	if len(oldTree.edits) == 1 && oldTree.edits[0].OldEndByte == oldTree.edits[0].NewEndByte {
		return nil, ""
	}
	if p.language.ExternalScanner != nil {
		stateless, ok := p.language.ExternalScanner.(StatelessExternalScanner)
		if !ok || !stateless.ExternalScannerIsStateless() {
			return nil, ""
		}
	}
	started := time.Now()
	if timing != nil {
		defer func() { timing.totalNanos += time.Since(started).Nanoseconds() }()
	}
	endBudget := p.enterParseBudget()
	defer endBudget()
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		return nil, err.Error()
	}
	oldTree.ensureParentLinks()
	p.reuseMu.Lock()
	defer p.reuseMu.Unlock()
	session := &compactIncrementalReuseSession{oldTree: oldTree, timing: timing, scheduler: &runner.scheduler}
	session.cursor.disableLeadingSplice = p.disableLeadingRunSplice
	session.cursor.reset(oldTree, source, &p.reuseScratch)
	runner.options.compactIncrementalReuse = session
	runner.scratch.incrementalReuse = session
	defer func() {
		runner.options.compactIncrementalReuse = nil
		runner.scheduler.options.compactIncrementalReuse = nil
		runner.scratch.incrementalReuse = nil
		session.cursor.commitScratch(&p.reuseScratch)
		session.cursor.releaseNodeRefs()
		clear(session.nodes)
		session.projection.reset()
	}()
	// Recovery and raw ambiguity selection require the original derivation.
	// An opaque borrowed payload cannot supply it, so this attempt stays clean.
	runner.scheduler.tokens = 0
	tree, err := runner.parseWithObserverAndErrorRuns(source, diagnosticParserCoreSeedObserver{}, false, false)
	if timing != nil {
		timing.reusedSubtrees = session.reusedSubtrees
		timing.reusedBytes = session.reusedBytes
		timing.tokensConsumed = runner.scheduler.tokens
	}
	if err != nil || tree == nil || session.reusedSubtrees == 0 {
		if tree != nil {
			tree.Release()
		}
		resetErr := runner.compact.ResetReleasingRetention()
		if err == nil {
			err = errors.New("compact incremental parse found no authenticated subtree")
		}
		return nil, errors.Join(err, resetErr).Error()
	}
	// Publish parent links only after every decline check has passed.
	// Borrowed nodes consult their old arena, so deferred new-arena links are insufficient.
	tree.ensureParentLinks()
	if timing != nil {
		timing.oldTreeReuseRoute = true
		timing.newNodes = uint64(tree.parseRuntime.NodesAllocated)
		timing.selectResult(tree)
	}
	return tree, ""
}

// tryCompactIncrementalReuse runs before ordinary dispatch, after each reduction.
// It never consumes a candidate while the current token still requires reduction.
func (s *diagnosticParserCoreGenericScheduler) tryCompactIncrementalReuse() (bool, error) {
	session := s.options.compactIncrementalReuse
	if session == nil {
		return false, nil
	}
	if session.timing != nil {
		started := time.Now()
		defer func() { session.timing.reuseNanos += time.Since(started).Nanoseconds() }()
	}
	if len(s.headers) != 1 || s.versionLexerOwnershipActive || s.recoveryIsolation {
		return false, errors.New("compact incremental reuse requires one clean shared-lexer version")
	}
	header := &s.headers[0]
	if header.isRecoveryLineage() || header.recoveryRegion() != nil || header.paused {
		return false, errors.New("compact incremental reuse cannot enter recovery")
	}
	if header.shifted || header.accepted || s.token.Symbol == 0 || s.token.NoLookahead || s.token.Missing {
		return false, nil
	}
	state, offset, err := s.compact.Boundary(header.head)
	if err != nil {
		return false, err
	}
	row := s.options.materializationParser.lookupAction(StateID(state), s.token.Symbol)
	if row == nil || len(row.Actions) != 1 || row.Actions[0].Type != ParseActionShift || row.Actions[0].Extra {
		return false, nil
	}
	p := s.options.materializationParser
	for _, node := range session.cursor.candidates(s.token.StartByte) {
		if node == nil || node.ChildCount() == 0 || node.IsExtra() || node.HasError() ||
			node.dirty() || node.isFragile() || !compactNodeMayBeReused(node) ||
			!compactNodeStateProofAvailable(node) || node.PreGotoState() != StateID(state) ||
			!session.cursor.topLevelSiblingBlockSpliceEligible(node) ||
			!session.cursor.nodeBytesUnchanged(node.StartByte(), node.EndByte()) ||
			!reuseSubtreeGapIsParserPadding(session.cursor.newSource, offset, node.StartByte(), p.lineContinuationEscapeByte()) {
			continue
		}
		next, ok := p.reuseTargetState(StateID(state), node, s.token)
		if !ok || next != node.parseState || s.freshSessionOwner == nil ||
			s.tokenSource == nil || s.tokenSource.lexer == nil ||
			!s.tokenSource.externalScannerQuiescent() ||
			int(node.EndByte()) < s.tokenSource.lexer.pos || node.EndByte() > uint32(len(session.cursor.newSource)) {
			continue
		}
		key := uint32(len(session.nodes) + 1)
		if key == 0 {
			return false, errors.New("compact incremental subtree keys exhausted")
		}
		head, _, err := s.compact.PushReusedSubtreeOwnedWithPoll(*s.freshSessionOwner, header.head, core.ReusedSubtree{
			Key: key, Symbol: core.Symbol(node.Symbol()), PreGotoState: state, State: core.StateID(next),
			StartByte: node.StartByte(), EndByte: node.EndByte(), DynamicPrecedence: node.dynamicPrecedence,
		}, s.pollStopControl)
		if err != nil {
			return false, err
		}
		session.nodes = append(session.nodes, node)
		session.reusedSubtrees++
		session.reusedBytes += uint64(node.EndByte() - node.StartByte())
		header.head = head
		header.shifted = true
		s.epochProgress = true
		// Match SkipToByteWithPoint positioning without consuming the next token.
		// The next scheduler election owns its state and scanner checkpoint.
		lexer := s.tokenSource.lexer
		lexer.pos = int(node.EndByte())
		lexer.row = node.EndPoint().Row
		lexer.col = node.EndPoint().Column
		lexer.includedRangeIdx = 0
		lexer.normalizeIncludedPosition()
		return true, nil
	}
	return false, nil
}

func (s *compactIncrementalReuseSession) footprintBytes() uint64 {
	if s == nil {
		return 0
	}
	return uint64(unsafe.Sizeof(*s)) +
		uint64(cap(s.nodes)+cap(s.cursor.cached)+cap(s.reuseState.arenaWalk))*uint64(unsafe.Sizeof((*Node)(nil))) +
		uint64(cap(s.cursor.stack))*uint64(unsafe.Sizeof(reuseFrame{})) +
		uint64(cap(s.reuseState.arenaRefs))*uint64(unsafe.Sizeof((*nodeArena)(nil))) +
		s.projection.footprintBytes()
}

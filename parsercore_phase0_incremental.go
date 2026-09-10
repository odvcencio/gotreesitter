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
	suffixReady, suffixValid       bool
	suffixNewStart, suffixOldStart uint32
	oldTree                        *Tree
	nodes                          []*Node
	reuseState                     parseReuseState
	projection                     compactBorrowedProjectionScratch
	scheduler                      *diagnosticParserCoreGenericScheduler
	timing                         *incrementalParseTiming
	cursor                         reuseCursor
	reusedSubtrees                 uint64
	reusedBytes                    uint64
	// unauthenticatedTopLevelCandidates counts reuse boundaries where a clean,
	// byte-unchanged in-scope candidate (a top-level sibling or a nested
	// child of the edited item) was available and the compact route could
	// not authenticate it. See compactIncrementalReuseCandidateLimit.
	unauthenticatedTopLevelCandidates uint32
	// editEndByte is the end of the last pending edit in new-source bytes.
	// See compactIncrementalReuseDeclineWindowBytes.
	editEndByte uint32
}

// compactIncrementalReuseDeclineWindowBytes bounds a compact incremental
// attempt that has reused nothing and finds no in-scope candidate at all, for
// example a JSON document whose single top-level array holds every edited
// item. Once the shared token has advanced this far past the last edit with
// zero reuse, the attempt declines so the production incremental path runs
// instead of a whole-file compact reparse that is then discarded. This is a
// bound on wasted work, not a reuse gate: an attempt that reuses one subtree
// before the window closes is never cut short.
const compactIncrementalReuseDeclineWindowBytes = 32 << 10

var errCompactIncrementalReuseWindowExhausted = errors.New(
	"compact incremental reuse declined: no subtree reused within the decline window past the edit")

// compactIncrementalReuseCandidateLimit bounds a compact incremental attempt
// that reuses nothing. Once this many clean in-scope candidates have been
// offered and declined without a single reuse, the attempt declines so the
// production incremental path, which reuses them under its established
// compatible-goto contract, runs at once instead of after a whole-file
// compact reparse that is then discarded (issue #454: INI and JSON paid a
// full compact parse per keystroke before this decline).
const compactIncrementalReuseCandidateLimit = 8

// compactIncrementalReuseCommitBytes is the reused-byte count below which an
// attempt still counts as reusing nothing for the decline bounds above. A
// single tiny borrowed subtree must not commit the attempt to a whole-file
// compact reparse that production reuse would serve in a fraction of the time.
const compactIncrementalReuseCommitBytes = 4 << 10

var errCompactIncrementalReuseUnauthenticatedCandidates = errors.New(
	"compact incremental reuse declined: in-scope candidates were offered but not authenticated")

func (p *Parser) attemptCompactIncrementalParse(source []byte, oldTree *Tree, timing *incrementalParseTiming) (*Tree, string, bool) {
	if oldTree == nil || p.oldTreeReuseContextMismatch(oldTree) != "" || !oldTree.compactMaterialized ||
		oldTree.incrementalReuseDisabled || len(oldTree.edits) == 0 ||
		oldTree.root == nil || oldTree.root.HasError() || p.recoveryInitialOnly ||
		!p.admissionCandidateFullParseEligible(nil, true) {
		return nil, "", false
	}
	p.fullParseRetryPassesTaken = 0
	// Preserve the existing token-invariant fast path for same-width leaf edits.
	// Reparse on the compact engine when the token proof fails.
	if len(oldTree.edits) == 1 && oldTree.edits[0].OldEndByte == oldTree.edits[0].NewEndByte {
		var probeStarted time.Time
		if timing != nil {
			probeStarted = time.Now()
		}
		if tree, ok := p.tryTokenInvariantReuseWithDFA(source, oldTree, timing); ok {
			return tree, "", false
		}
		if timing != nil {
			probeNanos := time.Since(probeStarted).Nanoseconds()
			timing.totalNanos += probeNanos
			timing.reuseNanos += probeNanos
		}
	}
	if p.language.ExternalScanner != nil {
		stateless, ok := p.language.ExternalScanner.(StatelessExternalScanner)
		if !ok || !stateless.ExternalScannerIsStateless() {
			return nil, "", false
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
		return nil, err.Error(), false
	}
	oldTree.ensureParentLinks()
	p.reuseMu.Lock()
	defer p.reuseMu.Unlock()
	session := &compactIncrementalReuseSession{oldTree: oldTree, timing: timing, scheduler: &runner.scheduler}
	for _, edit := range oldTree.edits {
		if edit.NewEndByte > session.editEndByte {
			session.editEndByte = edit.NewEndByte
		}
	}
	session.cursor.disableLeadingSplice = p.disableLeadingRunSplice
	session.cursor.reset(oldTree, source, &p.reuseScratch)
	session.cursor.allowFragileAncestors = true
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
		recoveryDeclined := err != nil && runner.scheduler.options.compactIncrementalReuse == session &&
			runner.scheduler.receipt != nil && runner.scheduler.receipt.Stop.Boundary == DiagnosticParserCoreRecovery
		if tree != nil {
			tree.Release()
		}
		resetErr := runner.compact.ResetReleasingRetention()
		if err == nil {
			err = errors.New("compact incremental parse found no authenticated subtree")
		}
		return nil, errors.Join(err, resetErr).Error(), recoveryDeclined && resetErr == nil
	}
	// Publish parent links only after every decline check has passed.
	// Borrowed nodes consult their old arena, so deferred new-arena links are insufficient.
	tree.ensureParentLinks()
	if timing != nil {
		timing.oldTreeReuseRoute = true
		timing.newNodes = uint64(tree.parseRuntime.NodesAllocated)
		timing.selectResult(tree)
	}
	return tree, "", false
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
	if session.reusedBytes < compactIncrementalReuseCommitBytes && s.token.StartByte > session.editEndByte &&
		s.token.StartByte-session.editEndByte > compactIncrementalReuseDeclineWindowBytes {
		return false, errCompactIncrementalReuseWindowExhausted
	}
	if len(s.headers) > 1 && !s.versionLexerOwnershipActive && !s.recoveryIsolation && !s.options.Recovery {
		// Deferral does not borrow or consume. With recovery disabled, ordinary
		// dispatch can remove a duplicate-reduction pause after sibling progress.
		// A paused alternative cannot supply reuse: borrowing still requires one
		// unpaused head and an active-ancestry certificate below.
		for i := range s.headers {
			header := &s.headers[i]
			if header.isRecoveryLineage() || header.recoveryRegion() != nil {
				return false, errors.New("compact incremental reuse cannot enter recovery")
			}
		}
		return false, nil
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
	if err := session.prepareSuffixProof(s.pollStopControl); err != nil {
		return false, err
	}
	unauthenticatedTopLevel := false
	for _, node := range session.cursor.candidates(s.token.StartByte) {
		next, ok := session.candidateState(p, node, StateID(state), offset, s.token)
		if !ok || s.freshSessionOwner == nil ||
			s.tokenSource == nil || s.tokenSource.lexer == nil ||
			!s.tokenSource.externalScannerQuiescent() ||
			int(node.EndByte()) < s.tokenSource.lexer.pos || node.EndByte() > uint32(len(session.cursor.newSource)) {
			if !ok && session.candidateInScope(p, node, s.token) {
				unauthenticatedTopLevel = true
			}
			continue
		}
		key := uint32(len(session.nodes) + 1)
		if key == 0 {
			return false, errors.New("compact incremental subtree keys exhausted")
		}
		head, payload, err := s.compact.PushReusedSubtreeOwnedWithPoll(*s.freshSessionOwner, header.head, core.ReusedSubtree{
			Key: key, Symbol: core.Symbol(node.Symbol()), PreGotoState: state, State: core.StateID(next),
			StartByte: node.StartByte(), EndByte: node.EndByte(), DynamicPrecedence: node.dynamicPrecedence,
		}, s.pollStopControl)
		if err != nil {
			if errors.Is(err, core.ErrReusedHeadUncertified) &&
				!session.cursor.topLevelSiblingBlockSpliceEligible(node) && session.suffixNodeEligible(p, node) {
				return false, nil
			}
			return false, err
		}
		session.nodes = append(session.nodes, node)
		session.reusedSubtrees++
		session.reusedBytes += uint64(node.EndByte() - node.StartByte())
		header.head = head
		if err := s.importCompactReuseDependency(payload, node); err != nil {
			return false, err
		}
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
	if unauthenticatedTopLevel && session.reusedBytes < compactIncrementalReuseCommitBytes {
		session.unauthenticatedTopLevelCandidates++
		if session.unauthenticatedTopLevelCandidates >= compactIncrementalReuseCandidateLimit {
			return false, errCompactIncrementalReuseUnauthenticatedCandidates
		}
	}
	return false, nil
}

// candidateInScope reports whether node is a clean, byte-unchanged candidate
// that candidateState would splice if it could authenticate its state. It
// separates structural eligibility from state authentication so the attempt
// can count authentication failures (compactIncrementalReuseCandidateLimit).
func (s *compactIncrementalReuseSession) candidateInScope(p *Parser, node *Node, lookahead Token) bool {
	return node != nil && node.ChildCount() > 0 && !node.IsExtra() && !node.HasError() &&
		!node.dirty() && !node.isFragile() &&
		(s.cursor.topLevelSiblingBlockSpliceEligible(node) ||
			(node.parent != nil && s.oldTree != nil && node.parent.parent == s.oldTree.root && node.parent.dirty() && s.nestedCandidateScopeEligible(p, node, lookahead))) &&
		s.cursor.nodeBytesUnchanged(node.StartByte(), node.EndByte())
}

func (s *compactIncrementalReuseSession) candidateState(p *Parser, node *Node, state StateID, offset uint32, lookahead Token) (StateID, bool) {
	if node == nil || node.ChildCount() == 0 || node.IsExtra() || node.HasError() ||
		node.dirty() || node.isFragile() || !s.nodeMayBeReused(p, node) ||
		!compactNodeStateProofAvailable(node) || node.PreGotoState() != state ||
		(!s.cursor.topLevelSiblingBlockSpliceEligible(node) && !s.nestedCandidateScopeEligible(p, node, lookahead)) {
		return 0, false
	}
	// prepareSuffixProof has already compared the complete common suffix.
	// Rechecking each suffix node would turn one O(source) proof into O(nodes)
	// duplicate scans. Keep the per-node comparison for nodes outside that
	// authenticated suffix.
	if !s.suffixNodeEligible(p, node) &&
		!s.cursor.nodeBytesUnchanged(node.StartByte(), node.EndByte()) {
		return 0, false
	}
	if !reuseSubtreeGapIsParserPadding(s.cursor.newSource, offset, node.StartByte(), p.lineContinuationEscapeByte()) {
		return 0, false
	}
	next, ok := p.reuseTargetState(state, node, lookahead)
	return next, ok && next == node.parseState
}

// The fresh token proves the left boundary. A retained dependency permits
// direct children of the edited top-level item. An authenticated suffix permits
// deeper children and covers all forward lookahead through EOF.
// Materialization authenticates ownership and rejects changed projections.
func (s *compactIncrementalReuseSession) nestedCandidateScopeEligible(p *Parser, node *Node, lookahead Token) bool {
	if s.oldTree == nil || node == nil || node.parent == nil || !node.isCompactMaterialized() ||
		uint32(node.symbol) < p.language.TokenCount || !p.isVisibleSymbol(node.symbol) ||
		s.cursor.rightBoundaryTouchedByEdit(node.EndByte()) {
		return false
	}
	if !s.suffixNodeEligible(p, node) &&
		(node.parent.parent != s.oldTree.root || !node.parent.dirty() || !s.nestedDependencyUnchanged(node)) {
		return false
	}
	leaf := leftmostLeaf(node)
	return leaf != nil && uint32(leaf.symbol) < p.language.TokenCount &&
		leaf.symbol == lookahead.Symbol && leaf.StartByte() == lookahead.StartByte &&
		leaf.EndByte() == lookahead.EndByte
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

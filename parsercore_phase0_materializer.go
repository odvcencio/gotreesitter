package gotreesitter

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreEagerMaterializationEnabled gates eager public-node construction
// during the compact scheduler run (issue #454, compact clean-path kernel).
// The gate is off by default: on the Go 137 KiB witness the eager lane builds
// every node before acceptance but costs about ten percent more wall time
// than the postorder pass, because construction interleaved with dispatch
// loses the locality of the batch pass while the compact core still writes
// every record. GTS_COMPACT_EAGER=1 (or true/on/yes) turns the lane on. The
// lane is the construction half of the single-head kernel, which stops
// writing compact records for subtrees that already own a public node.
var (
	parserCoreEagerMaterializationOnce sync.Once
	parserCoreEagerMaterializationVal  bool
)

func parserCoreEagerMaterializationEnabled() bool {
	parserCoreEagerMaterializationOnce.Do(func() {
		switch os.Getenv("GTS_COMPACT_EAGER") {
		case "1", "true", "TRUE", "True", "on", "ON", "yes", "YES":
			parserCoreEagerMaterializationVal = true
		}
	})
	return parserCoreEagerMaterializationVal
}

// SetParserCoreEagerMaterializationEnabledForTest overrides the eager gate
// for one test process. Restore the previous value (the returned func) when
// done.
func SetParserCoreEagerMaterializationEnabledForTest(on bool) func() {
	previous := parserCoreEagerMaterializationEnabled()
	parserCoreEagerMaterializationVal = on
	return func() { parserCoreEagerMaterializationVal = previous }
}

// compactMaterializer builds the public nodes of one compact derivation.
//
// Two drivers call visit. The accepted-tree materialization pass drives it
// in postorder over the accepted payloads after the scheduler returns. The
// scheduler also drives it eagerly: after each shift and each in-place
// reduction on a single header, the new subtree becomes a public node at
// once, so the postorder pass skips that subtree (VisitMaterializationPostorderPrebuilt).
// Both drivers produce the same node for the same subtree: visit reads only
// the subtree view, the point index, and the nodes of the subtree's children.
//
// The eager driver runs only for a plain fresh parse: no recovery, no
// incremental reuse, no accepted-leaf coverage. The accepted-tree pass adopts
// the eager state when its own flags match that shape, and otherwise releases
// the eager arena and builds the tree from scratch.
type compactMaterializer struct {
	compact                *core.Core
	parser                 *Parser
	source                 []byte
	arena                  *nodeArena
	scratch                *parserCoreRunnerScratch
	incrementalReuse       *compactIncrementalReuseSession
	budgetScheduler        *diagnosticParserCoreGenericScheduler
	materializationScratch *diagnosticParserCoreMaterializationScratch
	acceptedLeaves         *diagnosticParserCoreAcceptedLeafCoverageScratch
	replayTransition       core.MaterializationReplayTransition
	points                 diagnosticParserCorePointIndex
	// view and replayView are the eager driver's visit arguments. frames is
	// the pending-subtree traversal stack.
	view       core.MaterializationSubtreeView
	replayView core.MaterializationReplayView
	frames     []compactEagerFrame
	// nodesByID proves unique ownership: every populated compact ID owns
	// exactly one public node in this tree. It is a transient child-build
	// table, not a memoization or sharing mechanism.
	nodesByID    []*Node
	hasErrorByID []bool
	// eagerNodes counts subtrees the scheduler built during its run.
	// eagerSkipped counts pushes the eager driver declined. reset copies
	// both into the last* fields, which tests and benchmarks read.
	eagerNodes       uint64
	eagerSkipped     uint64
	lastEagerNodes   uint64
	lastEagerSkipped uint64
	lastAdopted      bool

	replayRootPre                   core.StateID
	recoveryTerminalAlias           Symbol
	rootFinalization                diagnosticParserCoreRootFinalization
	allowErrorRoot                  bool
	allowLexerSkippedPrefix         bool
	replayEnabled                   bool
	usesScannerCheckpoints          bool
	scannerProvenanceTransferProven bool
	// eager is set while the scheduler may call eagerPush. owned is set
	// while the materializer owns the arena; the result tree takes the arena
	// over at the end of the accepted-tree pass.
	eager              bool
	owned              bool
	allocationRecorded bool
}

// begin acquires the arena and builds the point index for one derivation.
// It is the shared entry for the eager driver and the accepted-tree pass.
func (m *compactMaterializer) begin(
	compact *core.Core, parser *Parser, source []byte, scratch *parserCoreRunnerScratch,
	incrementalReuse *compactIncrementalReuseSession, replayEnabled bool,
) error {
	if compact == nil || parser == nil || parser.language == nil {
		return errors.New("parser-core phase zero: materializer requires a compact core and a parser")
	}
	m.compact, m.parser, m.source, m.scratch = compact, parser, source, scratch
	m.incrementalReuse = incrementalReuse
	m.replayEnabled = replayEnabled
	m.replayTransition = nil
	if replayEnabled {
		if !compact.TableIdentityMatches() {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreIdentity,
				detail:   "compact parser table identity does not match the materialization parser",
			}
		}
		m.replayRootPre = core.StateID(parser.replayRootPreGotoState())
		m.replayTransition = func(pre core.StateID, view core.MaterializationReplayView) (core.StateID, bool, error) {
			state, known, err := parser.replayCompactMaterializationTransition(StateID(pre), view)
			return core.StateID(state), known, err
		}
	}
	m.arena = acquireNodeArena(arenaClassFull)
	m.owned = true
	m.allocationRecorded = false
	// Compact external-token provenance is transferred into this arena by
	// visit. Set the language identity before publishing the first
	// checkpoint so the incremental reuse gate can authenticate the copied
	// snapshots.
	m.scannerProvenanceTransferProven = true
	m.usesScannerCheckpoints = languageUsesExternalScannerCheckpoints(parser.language)
	if m.usesScannerCheckpoints {
		_, identityRequired, identityValid := externalScannerCheckpointIdentityStatus(parser.language)
		if identityRequired {
			m.scannerProvenanceTransferProven = identityValid && m.arena.setExternalScannerCheckpointIdentityForLanguage(parser.language)
		}
	}
	var lineStartsBuf []uint32
	if scratch != nil {
		lineStartsBuf = scratch.lineStarts
	}
	points, err := newDiagnosticParserCorePointIndexInto(source, m.poll, lineStartsBuf)
	if err != nil {
		return err
	}
	m.points = points
	if scratch != nil {
		scratch.lineStarts = points.lineStarts
	}
	return nil
}

// beginEager arms the eager driver for one plain fresh scheduler run.
func (m *compactMaterializer) beginEager(
	compact *core.Core, parser *Parser, source []byte, scratch *parserCoreRunnerScratch, replayEnabled bool,
) error {
	if m.eager || m.owned {
		return errors.New("parser-core phase zero: eager materializer is already armed")
	}
	if parser == nil || parser.language == nil || parser.language.CompactLexerSkippedPrefixTilingCertified {
		return errors.New("parser-core phase zero: eager materialization needs plain leaf coverage")
	}
	m.eagerNodes, m.eagerSkipped = 0, 0
	m.prepareTables(0)
	if err := m.begin(compact, parser, source, scratch, nil, replayEnabled); err != nil {
		m.releaseOwned()
		return err
	}
	m.acceptedLeaves = nil
	m.allowErrorRoot = false
	m.allowLexerSkippedPrefix = false
	m.recoveryTerminalAlias = 0
	m.rootFinalization = diagnosticParserCoreFinalizeDefault
	m.budgetScheduler = nil
	if scratch != nil {
		m.materializationScratch = &scratch.materialization
	}
	m.eager = true
	return nil
}

// eagerAdoptable reports whether the accepted-tree pass can keep the nodes
// the eager driver built. The pass flags must match the plain shape the
// eager driver assumed.
func (m *compactMaterializer) eagerAdoptable(
	compact *core.Core, parser *Parser, source []byte,
	incrementalReuse *compactIncrementalReuseSession, allowErrorRoot bool,
	rootFinalization diagnosticParserCoreRootFinalization, replayEnabled bool,
) bool {
	return m.eager && m.owned && m.compact == compact && m.parser == parser &&
		len(m.source) == len(source) && incrementalReuse == nil && !allowErrorRoot &&
		rootFinalization == diagnosticParserCoreFinalizeDefault && m.replayEnabled == replayEnabled
}

// abandonEager drops the eager state after a declined run, or when the
// accepted-tree pass cannot adopt it. The nodes built so far are released
// with the arena and charged to the attempt's work.
func (m *compactMaterializer) abandonEager() {
	if !m.eager && !m.owned {
		return
	}
	m.lastEagerNodes, m.lastEagerSkipped, m.lastAdopted = m.eagerNodes, m.eagerSkipped, false
	m.releaseOwned()
	m.clearState()
}

func (m *compactMaterializer) releaseOwned() {
	m.recordAllocation()
	if m.owned && m.arena != nil {
		m.arena.Release()
	}
	m.owned = false
	m.arena = nil
}

// recordAllocation charges the arena's node count to the attempt once.
func (m *compactMaterializer) recordAllocation() {
	if m.allocationRecorded || m.arena == nil {
		m.allocationRecorded = true
		return
	}
	if m.incrementalReuse != nil && m.incrementalReuse.timing != nil {
		m.incrementalReuse.timing.newNodes += uint64(m.arena.used)
	}
	if m.scratch != nil && m.scratch.freshAttemptWork != nil {
		m.scratch.freshAttemptWork.allocatedNodes += uint64(m.arena.used)
	}
	m.allocationRecorded = true
}

// clear drops every per-derivation reference while it retains the node
// tables for the next parse.
func (m *compactMaterializer) clearState() {
	nodesByID, hasErrorByID, frames := m.nodesByID, m.hasErrorByID, m.frames
	lastNodes, lastSkipped, lastAdopted := m.lastEagerNodes, m.lastEagerSkipped, m.lastAdopted
	*m = compactMaterializer{
		nodesByID: clearNodeScratch(nodesByID), hasErrorByID: hasErrorByID, frames: frames[:0],
		lastEagerNodes: lastNodes, lastEagerSkipped: lastSkipped, lastAdopted: lastAdopted,
	}
	if cap(m.hasErrorByID) > parserCoreMaxRetainedNodeScratch {
		m.hasErrorByID = nil
	} else {
		clear(m.hasErrorByID)
		m.hasErrorByID = m.hasErrorByID[:0]
	}
}

// reset runs after the accepted-tree pass. It releases the arena when no
// tree took it over and clears the per-derivation state.
func (m *compactMaterializer) reset() {
	if m == nil {
		return
	}
	m.lastEagerNodes, m.lastEagerSkipped = m.eagerNodes, m.eagerSkipped
	m.releaseOwned()
	m.clearState()
}

// prepareTables sizes the node tables to exactly n cleared entries.
func (m *compactMaterializer) prepareTables(n int) {
	m.nodesByID = parserCoreNodeSlice(m.nodesByID, n)
	if cap(m.hasErrorByID) < n {
		m.hasErrorByID = make([]bool, n)
	} else {
		m.hasErrorByID = m.hasErrorByID[:n]
		clear(m.hasErrorByID)
	}
}

// ensureTables grows the node tables so index id is addressable. Growth is
// amortized because the eager driver sees subtree ids in creation order.
func (m *compactMaterializer) ensureTables(n int) {
	if len(m.nodesByID) >= n {
		return
	}
	if cap(m.nodesByID) >= n {
		old := len(m.nodesByID)
		m.nodesByID = m.nodesByID[:n]
		clear(m.nodesByID[old:])
	} else {
		grown := make([]*Node, n, max(n, 2*cap(m.nodesByID), 1024))
		copy(grown, m.nodesByID)
		m.nodesByID = grown
	}
	if cap(m.hasErrorByID) >= n {
		old := len(m.hasErrorByID)
		m.hasErrorByID = m.hasErrorByID[:n]
		clear(m.hasErrorByID[old:])
	} else {
		grown := make([]bool, n, max(n, 2*cap(m.hasErrorByID), 1024))
		copy(grown, m.hasErrorByID)
		m.hasErrorByID = grown
	}
}

// prebuilt reports whether a subtree already owns a public node.
func (m *compactMaterializer) prebuilt(id core.SubtreeID) bool {
	return uint64(id) < uint64(len(m.nodesByID)) && m.nodesByID[id] != nil
}

// compactEagerFrame is one pending subtree on the eager build stack. cursor
// is the replay state after the children built so far, which is the next
// child's pre-goto state; pre is the frame's own pre-goto state.
type compactEagerFrame struct {
	id     core.SubtreeID
	next   uint32
	pre    core.StateID
	cursor core.StateID
}

// eagerPush builds the subtree the scheduler just pushed onto a single
// exact frontier. It declines quietly when the frontier has more than one
// link or when the record needs recovery construction; the postorder pass
// builds those subtrees after acceptance. Children the scheduler pushed
// during a multi-header phase are built on demand first, in the same
// postorder and with the same replay chain the accepted-tree pass uses.
func (m *compactMaterializer) eagerPush(head core.Head) error {
	payload, pre, ok := m.compact.PushedFrontier(head)
	if !ok {
		m.eagerSkipped++
		return nil
	}
	if m.prebuilt(payload) {
		return nil
	}
	view := &m.view
	if err := m.compact.FillMaterializationView(payload, view, &m.replayView); err != nil {
		return err
	}
	for _, child := range view.Children {
		if !m.prebuilt(child) {
			return m.buildPending(payload, pre)
		}
	}
	return m.visitEager(payload, view, pre)
}

// visitEager stamps the replay states from pre and visits one subtree whose
// children are built. The caller filled m.view and m.replayView.
func (m *compactMaterializer) visitEager(id core.SubtreeID, view *core.MaterializationSubtreeView, pre core.StateID) error {
	if !m.eagerRecordable(view) {
		m.eagerSkipped++
		return nil
	}
	m.ensureTables(int(id) + 1)
	if m.replayEnabled {
		state, known, err := m.replayTransition(pre, m.replayView)
		if err != nil {
			return err
		}
		view.ReplayPreGotoState, view.ReplayParseState = pre, state
		view.ReplayPreGotoKnown, view.ReplayParseStateKnown = known && !view.Extra, known
	}
	if err := m.visit(id, view); err != nil {
		return err
	}
	m.eagerNodes++
	return nil
}

// eagerRecordable reports whether the eager driver builds this record.
// Recovery-shaped records keep the postorder pass.
func (m *compactMaterializer) eagerRecordable(view *core.MaterializationSubtreeView) bool {
	return !view.Missing && view.ReusedKey == 0 && Symbol(view.Symbol) != errorSymbol
}

// buildPending builds subtree root and every unbuilt descendant in postorder.
// A prebuilt descendant advances the replay cursor the way the accepted-tree
// pass does, so every state matches that pass.
func (m *compactMaterializer) buildPending(root core.SubtreeID, pre core.StateID) error {
	frames := append(m.frames[:0], compactEagerFrame{id: root, pre: pre, cursor: pre})
	defer func() { m.frames = frames[:0] }()
	for len(frames) != 0 {
		top := &frames[len(frames)-1]
		children, err := m.compact.SubtreeChildren(top.id)
		if err != nil {
			return err
		}
		if int(top.next) < len(children) {
			child := children[top.next]
			top.next++
			if m.prebuilt(child) {
				if m.replayEnabled {
					replay, err := m.compact.MaterializationReplayView(child)
					if err != nil {
						return err
					}
					// The replay advances the parent's cursor by the child's
					// result even when the tables held no transition.
					state, _, err := m.replayTransition(top.cursor, replay)
					if err != nil {
						return err
					}
					top.cursor = state
				}
				continue
			}
			frames = append(frames, compactEagerFrame{id: child, pre: top.cursor, cursor: top.cursor})
			continue
		}
		view := &m.view
		if err := m.compact.FillMaterializationView(top.id, view, &m.replayView); err != nil {
			return err
		}
		if !m.eagerRecordable(view) {
			// A recovery-shaped record cannot be built here, and neither can
			// its ancestors. Leave the whole pending chain to the postorder
			// pass.
			m.eagerSkipped++
			return nil
		}
		framePre := top.pre
		if err := m.visitEager(top.id, view, framePre); err != nil {
			return err
		}
		state := view.ReplayParseState
		frames = frames[:len(frames)-1]
		if len(frames) != 0 {
			frames[len(frames)-1].cursor = state
		}
	}
	return nil
}

// restampRoot re-applies the replay stamp of one accepted root payload with
// the root rule the postorder pass uses: every root starts from the root
// pre-goto state. The eager driver stamped a root that follows another root
// (a trailing extra) from the state after that root instead.
func (m *compactMaterializer) restampRoot(id core.SubtreeID) error {
	if !m.replayEnabled || !m.prebuilt(id) {
		return nil
	}
	view := &m.view
	if err := m.compact.FillMaterializationView(id, view, &m.replayView); err != nil {
		return err
	}
	state, known, err := m.replayTransition(m.replayRootPre, m.replayView)
	if err != nil {
		return err
	}
	view.ReplayPreGotoState, view.ReplayParseState = m.replayRootPre, state
	view.ReplayPreGotoKnown, view.ReplayParseStateKnown = known && !view.Extra, known
	m.stampReplay(m.nodesByID[id], view)
	return nil
}

// poll checks the parser's stop reasons and the scheduler's memory budget,
// charging the arena and the coverage scratch to the budget.
func (m *compactMaterializer) poll() error {
	reason := m.parser.resultMaterializationStopReason(m.arena)
	if !resultMaterializationShouldStop(reason) && m.budgetScheduler != nil {
		additional := arenaAllocatedVolume(m.arena)
		coverageBytes := m.acceptedLeaves.footprintBytes()
		if math.MaxUint64-additional < coverageBytes {
			additional = math.MaxUint64
		} else {
			additional += coverageBytes
		}
		reason = m.budgetScheduler.stopControlMemoryBudgetReasonWithAdditionalBytes(additional)
	}
	if !resultMaterializationShouldStop(reason) {
		return nil
	}
	return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "accepted-tree materialization stopped: " + string(reason)}
}

// stamp records the node that materializes derivation id and applies the
// replay stamp. For a unary collapse chain the driver visits the ids
// inner-to-outer (postorder) and reuses one node object, so the last
// (outermost) stamp wins -- mirroring production's collapse, which
// overwrites parseState = goto(topState, outerSymbol) as each wrapper
// reduce fires.
func (m *compactMaterializer) stamp(id core.SubtreeID, node *Node, view *core.MaterializationSubtreeView) {
	m.stampReplay(node, view)
	m.nodesByID[id] = node
}

// stampReplay applies the reconstructed parse states to one node.
//
// The replay reports known=false when the top-down replay could not find a
// table transition for this id (an extra/comment leaf whose floated stack
// position does not match a live shift, or any node whose production shape
// is not a plain shift/goto of its visible symbol). In that case the
// reconstructed state is NOT authoritative, so we ABSTAIN: leave
// parseState/preGotoState at their zero value. Downstream, a zero parseState
// is the "unknown -> recompute" sentinel (incremental self-healing), which is
// strictly safer than stamping a known-wrong but trusted non-zero state
// (Phase-3 Lane 3 review amendment 1).
func (m *compactMaterializer) stampReplay(node *Node, view *core.MaterializationSubtreeView) {
	if node == nil {
		return
	}
	// A visible node can represent several compact ids when unary
	// reductions collapse during materialization. Clear every stamped
	// field before applying the outermost id, so an inner proof cannot
	// survive an outer abstention.
	node.setCompactMaterialized(true)
	node.setCompactParseStateProof(false)
	node.setCompactPreGotoStateProof(false)
	node.parseState = 0
	node.preGotoState = 0
	if !m.replayEnabled {
		return
	}
	if view.ReplayParseStateKnown {
		node.parseState = StateID(view.ReplayParseState)
		node.setCompactParseStateProof(true)
	}
	if view.ReplayPreGotoKnown {
		node.preGotoState = StateID(view.ReplayPreGotoState)
		node.setCompactPreGotoStateProof(true)
	}
}

// markFragile threads the compact record's ambiguity bit (subtreeRecord
// .fragile, exposed on MaterializationSubtreeView.Fragile) onto the public
// node so Lane-1's isFragile() reuse gate sees compact-materialized trees
// the same as production-built ones (Phase-3 Lane 3 review amendment 7). The
// compact record collapses production's fragileLeft/fragileRight into one
// conservative flag, so both edges are set. Set-only (never clears), matching
// the record's monotone contract on shared/deduped records.
func (m *compactMaterializer) markFragile(node *Node, fragile bool) {
	if node == nil || !fragile {
		return
	}
	node.setFragileLeft(true)
	node.setFragileRight(true)
}

func (m *compactMaterializer) makeParent(symbol Symbol, named bool, children []*Node, fields []FieldID, fieldSources []uint8, productionID uint16) *Node {
	if m.incrementalReuse != nil {
		return newParentNodeInArenaNoLinksWithFieldSources(m.arena, symbol, named, children, fields, fieldSources, productionID, true)
	}
	return newParentNodeInArenaWithFieldSources(m.arena, symbol, named, children, fields, fieldSources, productionID)
}

// visit builds the public node for one subtree whose children are built.
func (m *compactMaterializer) visit(id core.SubtreeID, view *core.MaterializationSubtreeView) error {
	parser, arena, source := m.parser, m.arena, m.source
	nodesByID, hasErrorByID, acceptedLeaves := m.nodesByID, m.hasErrorByID, m.acceptedLeaves
	if view.EndByte < view.StartByte || view.EndByte > uint32(len(source)) {
		return errors.New("parser-core phase zero: compact subtree extent is outside source")
	}
	if view.ReusedKey != 0 {
		node, err := m.incrementalReuse.materializeBorrowed(parser, id, *view, &m.points)
		if err != nil {
			return err
		}
		if err := acceptedLeaves.appendBorrowed(id, node, uint32(len(source))); err != nil {
			return err
		}
		if m.usesScannerCheckpoints {
			m.scannerProvenanceTransferProven = false
		}
		m.incrementalReuse.reuseState.markReused(node, arena)
		nodesByID[id] = node
		return nil
	}
	if acceptedLeaves != nil && view.Terminal {
		if m.allowLexerSkippedPrefix {
			acceptedLeaves.recordLexerSkippedPrefix(id, *view)
		}
		if m.allowErrorRoot || m.incrementalReuse != nil {
			hidden := !parser.isVisibleSymbol(Symbol(view.Symbol))
			if view.Symbol == core.RecoveryErrorSymbol {
				hidden = false
			}
			if err := acceptedLeaves.append(id, *view, uint32(len(source)), hidden); err != nil {
				return err
			}
		}
	}
	named := parser.isNamedSymbol(Symbol(view.Symbol))
	// B3 stage S3: the built-in ERROR symbol (65535) sits outside
	// every real grammar's SymbolMetadata table, so isNamedSymbol's
	// bounds check above always reads false for it. Tree-sitter
	// treats ERROR as named unconditionally (visible in
	// S-expressions and named-child traversal, matching the pinned
	// C oracle's own "(ERROR ...)"/"(ERROR (UNEXPECTED 'x'))"
	// rendering for both the container and a raw unlexable-byte
	// leaf) -- force it here rather than teach the shared,
	// grammar-table-driven isNamedSymbol about a symbol that is
	// never a real grammar table entry.
	if Symbol(view.Symbol) == errorSymbol {
		named = true
	}
	if view.Terminal {
		node := newLeafNodeInArena(
			arena, Symbol(view.Symbol), named, view.StartByte, view.EndByte,
			m.points.point(view.StartByte), m.points.point(view.EndByte),
		)
		if m.usesScannerCheckpoints && view.Terminal &&
			!materializeCompactExternalScannerCheckpoint(m.compact, arena, node, *view) {
			m.scannerProvenanceTransferProven = false
		}
		node.setExtra(view.Extra)
		node.setExternalScannerToken(view.External)
		// S5 recovery: a recovery-inserted MISSING terminal
		// (core.MissingLeaf) carries both public bits, matching the
		// pinned C oracle and production's own port
		// (parser.go's missing-shift path sets exactly this pair).
		// has-error belongs on the missing node ITSELF, not only on
		// its ancestors: C defines ts_node_has_error as
		// error_cost > 0 (node.c:520-522), and ts_subtree_error_cost
		// short-circuits on the missing bit to return
		// ERROR_COST_PER_MISSING_TREE + ERROR_COST_PER_RECOVERY
		// (subtree.h:331-337), which is 610, so C reports has-error
		// true on the leaf. For a VISIBLE missing leaf, ordinary
		// ancestor propagation (populateParentNode, tree.go) then ORs
		// the flag up through every enclosing reduce with no
		// additional code.
		//
		// Hidden missing leaves can disappear during parent construction.
		// hasErrorByID carries their error state through that collapse. This
		// matches production's explicit trackChildErrors signal.
		if view.Missing {
			node.setMissing(true)
			node.setHasError(true)
			if !materializeCompactMissingNodeDependency(arena, node, *view) {
				return fmt.Errorf("parser-core phase zero: missing leaf dependency transfer failed: node=%d@%+v dependency=%+v", node.startByte, node.startPoint, view.MissingDependency)
			}
		}
		hasErrorByID[id] = view.Missing || Symbol(view.Symbol) == errorSymbol
		// No markFragile here: fragile is a reduce/conflict-arm property
		// (subtreeRecord.fragile is only ever set on reductions), so a
		// terminal record is never fragile. The reduce branches below
		// carry the bit.
		m.stamp(id, node, view)
		return nil
	}

	entries := m.materializationScratch.entriesFor(len(view.Children))
	subtreeHasError := Symbol(view.Symbol) == errorSymbol
	structuralChildren := 0
	for index, childID := range view.Children {
		if uint64(childID) >= uint64(len(nodesByID)) || nodesByID[childID] == nil {
			return errors.New("parser-core phase zero: compact materialization traversal omitted a child")
		}
		child := nodesByID[childID]
		if hasErrorByID[childID] {
			subtreeHasError = true
		}
		entries[index] = newStackEntryNode(0, child)
		if !child.isExtra() {
			structuralChildren++
		}
	}
	if m.incrementalReuse != nil {
		if err := validateCompactBorrowedReduceInputs(parser, entries, view.ProductionID, arena); err != nil {
			return err
		}
	}
	// isDerivationRootReduce is true only for the one reduce, per parse,
	// whose symbol is this language's own inferred grammar root symbol
	// (parser.rootSymbol / hasRootSymbol -- inferRootSymbol, parser.go: a
	// grammar-derived property, computed from the language's own tables,
	// not a per-language name check). It is exempted from this reduce's
	// OWN tiling requirement for the same reason
	// finalizeDiagnosticParserCoreAcceptedRootSpan already treats the
	// root-to-sourceLen boundary as a separately governed special case
	// (extendRootToAcceptedCleanTail, with its own, more lenient rule): the
	// root reduce is the one construct with no enclosing reduce to ever
	// re-validate its own declared span from the outside, so an over-wide
	// root span (still exactly [expectedStart, sourceLen), already pinned
	// by finalizeDiagnosticParserCoreAcceptedRootSpan's own checks) is a
	// materially different, narrower risk than an internal gap anywhere
	// below it, which every enclosing reduce's own tiling check still
	// catches. This closes a jsdoc residual the retired
	// bytesAreSingleByteDecorationTrivia predicate used to leave standing:
	// javadoc/doxygen-style comments that close with "*/" (no leading
	// space) put the decoration marker
	// on the trailing edge of the root reduce's own gap, indistinguishable
	// in isolation from a genuine drop. The exemption stays limited to the
	// grammar root. Every internal reduce still passes the ordinary tiling
	// check before materialization can publish it.
	isDerivationRootReduce := m.rootFinalization == diagnosticParserCoreFinalizeDefault &&
		parser.hasRootSymbol && Symbol(view.Symbol) == parser.rootSymbol
	if m.allowLexerSkippedPrefix {
		acceptedLeaves.propagateLeadingLexerSkippedPrefix(id, view.StartByte, view.Children, nodesByID)
	}
	if gapStart, gapEnd, gapped := diagnosticParserCoreReduceChildrenTilingGapWithLexerProvenance(
		view.StartByte, view.EndByte, entries, view.Children, source, acceptedLeaves, nodesByID, m.allowLexerSkippedPrefix,
	); !isDerivationRootReduce && gapped {
		return &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreAccept,
			detail: fmt.Sprintf(
				"accepted-leaf-tiling-gap: compact subtree symbol=%d span=%d..%d has an unaccounted byte range %d..%d not covered by any child",
				view.Symbol, view.StartByte, view.EndByte, gapStart, gapEnd,
			),
		}
	}
	// B3 stage S3: an ERROR-symbol reduce is a native recovery region
	// (s3TryOpenErrorRegion/ErrorRegionResume), never a real grammar
	// production. It always bypasses unary self-reduction collapse
	// (errorSymbol's huge numeric value falls outside every real
	// grammar's SymbolMetadata table, so the collapse checks below
	// would either safely no-op or -- for the one case they would
	// not, a childless absorbed leaf sharing the ERROR symbol itself
	// -- wrongly elide the wrapper the C oracle keeps; skip them
	// outright instead of relying on that bound check), matching
	// production's own recovery construction (newRecoveryParentNodeInArena,
	// parser_recover_c.go), which never goes through the shared
	// collapsibleRawUnarySelfReduction/collapsibleUnarySelfReduction
	// path either.
	if Symbol(view.Symbol) == errorSymbol {
		children, fieldIDs, fieldSources, _ := parser.buildReduceChildrenWithPath(
			entries, 0, len(entries), structuralChildren,
			Symbol(view.Symbol), view.ProductionID, arena,
		)
		if m.recoveryTerminalAlias != 0 {
			acceptedLeaves.authenticateDirectTerminalAliases(
				parser, entries, children, view.ProductionID, m.recoveryTerminalAlias, nodesByID,
			)
		}
		parent := m.makeParent(
			Symbol(view.Symbol), named, children, fieldIDs, fieldSources, view.ProductionID,
		)
		parent.dynamicPrecedence += int32(view.DynamicPrecedence)
		parent.startByte = view.StartByte
		parent.endByte = view.EndByte
		parent.startPoint = m.points.point(view.StartByte)
		parent.endPoint = m.points.point(view.EndByte)
		parent.setExtra(view.Extra)
		// The ERROR container's own HasError is always true,
		// regardless of what populateParentNode's children-OR
		// propagation computed: matching the pinned C oracle, an
		// absorbed leaf's own HasError stays false even when the
		// leaf is itself an unlexable byte (ErrorRegionLeaf's doc
		// comment; finding production-recovery-structural-divergence),
		// so this explicit set is the only place HasError=true
		// originates for the whole region. Every enclosing ordinary
		// reduce above this one propagates it up for free through
		// populateParentNode's existing, unmodified OR-of-children
		// walk (tree.go) -- no further HasError code is needed
		// anywhere else in this file.
		parent.setHasError(true)
		hasErrorByID[id] = true
		m.markFragile(parent, view.Fragile)
		m.stamp(id, parent, view)
		return nil
	}
	action := ParseAction{
		Type: ParseActionReduce, Symbol: Symbol(view.Symbol), ChildCount: uint8(structuralChildren),
		DynamicPrecedence: int16(view.DynamicPrecedence), ProductionID: view.ProductionID,
	}
	if child := parser.collapsibleRawUnarySelfReduction(action, Token{}, arena, entries, 0, len(entries)); child != nil {
		child.productionID = view.ProductionID
		child.dynamicPrecedence += int32(view.DynamicPrecedence)
		m.markFragile(child, view.Fragile)
		if subtreeHasError {
			child.setHasError(true)
		}
		hasErrorByID[id] = subtreeHasError
		m.stamp(id, child, view)
		return nil
	}
	children, fieldIDs, fieldSources, _ := parser.buildReduceChildrenWithPath(
		entries, 0, len(entries), structuralChildren,
		Symbol(view.Symbol), view.ProductionID, arena,
	)
	if m.incrementalReuse != nil {
		if err := validateCompactBorrowedReduceProjectionWithScratch(parser, entries, children, arena, &m.incrementalReuse.projection, m.poll); err != nil {
			return fmt.Errorf("reduce symbol=%d production=%d: %w", view.Symbol, view.ProductionID, err)
		}
	}
	// Authenticate terminal aliases at their exact grammar reduction.
	// Shared recovery needs the same raw-terminal and clone proof.
	if m.allowErrorRoot {
		acceptedLeaves.authenticateDirectTerminalAliases(
			parser, entries, children, view.ProductionID, 0, nodesByID,
		)
	}
	if child := parser.collapsibleUnarySelfReduction(action, Token{}, arena, entries, 0, len(entries), children, fieldIDs); child != nil {
		child.productionID = view.ProductionID
		child.dynamicPrecedence += int32(view.DynamicPrecedence)
		m.markFragile(child, view.Fragile)
		if subtreeHasError {
			child.setHasError(true)
		}
		hasErrorByID[id] = subtreeHasError
		m.stamp(id, child, view)
		return nil
	}
	parent := m.makeParent(
		Symbol(view.Symbol), named, children, fieldIDs, fieldSources, view.ProductionID,
	)
	parent.dynamicPrecedence += int32(view.DynamicPrecedence)
	parent.startByte = view.StartByte
	parent.endByte = view.EndByte
	parent.startPoint = m.points.point(view.StartByte)
	parent.endPoint = m.points.point(view.EndByte)
	parent.setExtra(view.Extra)
	if subtreeHasError {
		parent.setHasError(true)
	}
	hasErrorByID[id] = subtreeHasError
	m.markFragile(parent, view.Fragile)
	m.stamp(id, parent, view)
	return nil
}

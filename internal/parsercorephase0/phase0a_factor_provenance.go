//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"math"
	"unsafe"
)

type Phase0AExpressionID uint64
type Phase0ASelectorID uint64

type Phase0AExpressionKind uint8

const (
	Phase0AExpressionDirect Phase0AExpressionKind = iota + 1
	Phase0AExpressionFactorChoice
)

type Phase0ATransitionKind uint8
type phase0ATransitionKind = Phase0ATransitionKind

const (
	Phase0ATransitionDirectPublication Phase0ATransitionKind = iota + 1
	Phase0ATransitionDuplicateDrop
	Phase0ATransitionPrecedenceDrop
	Phase0ATransitionPrecedenceReplacement
	Phase0ATransitionAlternateAppend
	Phase0ATransitionPhysicalCopy
	Phase0ATransitionCanonicalRedirect
	Phase0ATransitionFactorChoice
)

const (
	phase0ATransitionDuplicateDrop         = Phase0ATransitionDuplicateDrop
	phase0ATransitionPrecedenceDrop        = Phase0ATransitionPrecedenceDrop
	phase0ATransitionPrecedenceReplacement = Phase0ATransitionPrecedenceReplacement
	phase0ATransitionAlternateAppend       = Phase0ATransitionAlternateAppend
)

type Phase0ASelectorBranch uint8

const (
	Phase0ASelectorLeft Phase0ASelectorBranch = iota + 1
	Phase0ASelectorRight
)

type Phase0ACandidateRecord struct {
	TransactionID       uint64
	Occurrence          ConstructionOccurrenceKey
	Edge                IncomingEdgeKey
	Boundary            Phase0ABoundaryInput
	Payload             SubtreeID
	Predecessor         NodeID
	ScoreDelta          int64
	Order               uint64
	HasOrder            bool
	Claimed             bool
	Resolved            bool
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

type Phase0AExpressionRecord struct {
	ID                  Phase0AExpressionID
	Kind                Phase0AExpressionKind
	TransactionID       uint64
	Occurrence          ConstructionOccurrenceKey
	Edge                IncomingEdgeKey
	Selector            Phase0ASelectorID
	Left                Phase0AExpressionID
	Right               Phase0AExpressionID
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

// Phase0ALinkBindingRecord is append-only. LinkIDs are arena ordinals and may
// be reused after rollback, so consumers must select the latest non-tombstoned
// row rather than treating LinkID as globally unique.
type Phase0ALinkBindingRecord struct {
	Serial              uint64
	TransactionID       uint64
	Link                LinkID
	Expression          Phase0AExpressionID
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

type Phase0ATransitionRecord struct {
	Kind                Phase0ATransitionKind
	TransactionID       uint64
	Candidate           IncomingEdgeKey
	FromLink            LinkID
	ToLink              LinkID
	FromNode            NodeID
	ToNode              NodeID
	Expression          Phase0AExpressionID
	Selector            Phase0ASelectorID
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

type Phase0ASelectorRecord struct {
	ID                  Phase0ASelectorID
	TransactionID       uint64
	MergedPredecessor   NodeID
	FirstRoute          uint64
	RouteCount          uint32
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

type Phase0ASelectorRouteRecord struct {
	Selector            Phase0ASelectorID
	TransactionID       uint64
	SelectedLowerLink   LinkID
	SourceLowerLink     LinkID
	Branch              Phase0ASelectorBranch
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

type phase0AFactorObserver struct {
	candidates     []Phase0ACandidateRecord
	expressions    []Phase0AExpressionRecord
	bindings       []Phase0ALinkBindingRecord
	transitions    []Phase0ATransitionRecord
	selectors      []Phase0ASelectorRecord
	selectorRoutes []Phase0ASelectorRouteRecord
	candidateUndos []phase0ACandidateUndo
	nextExpression uint64
	nextBinding    uint64
	nextSelector   uint64
	sidecarFrames  []phase0ASidecarFrame
	mergePlans     []phase0AMergePlan
	replacements   []phase0AReplacementScope
}

type phase0ASidecarFrame struct {
	candidateStart, expressionStart, bindingStart, transitionStart uint64
	selectorStart, routeStart, undoStart, mergeDepth               uint64
	replacementDepth                                               uint64
}

type phase0ACandidateUndo struct {
	index                       uint64
	priorClaimed, priorResolved bool
}

type phase0AMergePlan struct {
	transaction  uint64
	left         NodeID
	entries      []phase0AMergeEntry
	rightIDs     []LinkID
	rightIndex   int
	makeSelector bool
	factorEdge   IncomingEdgeKey
}

type phase0AMergeEntry struct {
	expression Phase0AExpressionID
	source     LinkID
	branch     Phase0ASelectorBranch
	kind       Phase0ATransitionKind
	selector   Phase0ASelectorID
}

type phase0AReplacementScope struct {
	transaction uint64
	key         Phase0ABoundaryInput
	in          linkInput
	old         NodeID
	candidate   int
	direct      Phase0AExpressionID
	edge        IncomingEdgeKey
	oldIDs      []LinkID
}

const (
	phase0ACandidateBytes     = uint64(unsafe.Sizeof(Phase0ACandidateRecord{}))
	phase0AExpressionBytes    = uint64(unsafe.Sizeof(Phase0AExpressionRecord{}))
	phase0ABindingBytes       = uint64(unsafe.Sizeof(Phase0ALinkBindingRecord{}))
	phase0ATransitionBytes    = uint64(unsafe.Sizeof(Phase0ATransitionRecord{}))
	phase0ASelectorBytes      = uint64(unsafe.Sizeof(Phase0ASelectorRecord{}))
	phase0ARouteBytes         = uint64(unsafe.Sizeof(Phase0ASelectorRouteRecord{}))
	phase0ACandidateUndoBytes = uint64(unsafe.Sizeof(phase0ACandidateUndo{}))
	phase0ASidecarFrameBytes  = uint64(unsafe.Sizeof(phase0ASidecarFrame{}))
)

func phase0AFactorMarkLocked(observer *phase0AObserver) {
	observer.factor.sidecarFrames = append(observer.factor.sidecarFrames, phase0ASidecarFrame{
		candidateStart: uint64(len(observer.factor.candidates)), expressionStart: uint64(len(observer.factor.expressions)),
		bindingStart: uint64(len(observer.factor.bindings)), transitionStart: uint64(len(observer.factor.transitions)),
		selectorStart: uint64(len(observer.factor.selectors)), routeStart: uint64(len(observer.factor.selectorRoutes)),
		undoStart:  uint64(len(observer.factor.candidateUndos)),
		mergeDepth: uint64(len(observer.factor.mergePlans)), replacementDepth: uint64(len(observer.factor.replacements)),
	})
}

func phase0AFactorRollbackLocked(observer *phase0AObserver, transaction uint64, cause Phase0ARollbackCause) {
	if len(observer.factor.sidecarFrames) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "factor rollback frame mismatch"})
		return
	}
	frame := observer.factor.sidecarFrames[len(observer.factor.sidecarFrames)-1]
	for index := len(observer.factor.candidateUndos); index > int(frame.undoStart); index-- {
		undo := observer.factor.candidateUndos[index-1]
		if undo.index >= uint64(len(observer.factor.candidates)) {
			phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "candidate undo index is unavailable"})
			break
		}
		candidate := &observer.factor.candidates[undo.index]
		candidate.Claimed, candidate.Resolved = undo.priorClaimed, undo.priorResolved
	}
	clear(observer.factor.candidateUndos[frame.undoStart:])
	observer.factor.candidateUndos = observer.factor.candidateUndos[:frame.undoStart]
	for index := frame.candidateStart; index < uint64(len(observer.factor.candidates)); index++ {
		row := &observer.factor.candidates[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.expressionStart; index < uint64(len(observer.factor.expressions)); index++ {
		row := &observer.factor.expressions[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.bindingStart; index < uint64(len(observer.factor.bindings)); index++ {
		row := &observer.factor.bindings[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.transitionStart; index < uint64(len(observer.factor.transitions)); index++ {
		row := &observer.factor.transitions[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.selectorStart; index < uint64(len(observer.factor.selectors)); index++ {
		row := &observer.factor.selectors[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.routeStart; index < uint64(len(observer.factor.selectorRoutes)); index++ {
		row := &observer.factor.selectorRoutes[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	if frame.mergeDepth <= uint64(len(observer.factor.mergePlans)) {
		observer.factor.mergePlans = observer.factor.mergePlans[:frame.mergeDepth]
	}
	if frame.replacementDepth <= uint64(len(observer.factor.replacements)) {
		observer.factor.replacements = observer.factor.replacements[:frame.replacementDepth]
	}
	observer.factor.sidecarFrames = observer.factor.sidecarFrames[:len(observer.factor.sidecarFrames)-1]
}

func phase0AFactorCommitLocked(observer *phase0AObserver) {
	if len(observer.factor.sidecarFrames) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "factor commit frame mismatch"})
		return
	}
	frame := observer.factor.sidecarFrames[len(observer.factor.sidecarFrames)-1]
	if uint64(len(observer.factor.mergePlans)) != frame.mergeDepth || uint64(len(observer.factor.replacements)) != frame.replacementDepth {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "factor commit with leaked scope"})
		if frame.mergeDepth <= uint64(len(observer.factor.mergePlans)) {
			observer.factor.mergePlans = observer.factor.mergePlans[:frame.mergeDepth]
		}
		if frame.replacementDepth <= uint64(len(observer.factor.replacements)) {
			observer.factor.replacements = observer.factor.replacements[:frame.replacementDepth]
		}
	}
	if len(observer.factor.sidecarFrames) == 1 {
		clear(observer.factor.candidateUndos[frame.undoStart:])
		observer.factor.candidateUndos = observer.factor.candidateUndos[:frame.undoStart]
	}
	observer.factor.sidecarFrames = observer.factor.sidecarFrames[:len(observer.factor.sidecarFrames)-1]
}

func phase0AAppendCandidateUndoPrechargedLocked(observer *phase0AObserver, index int) bool {
	if index < 0 || index >= len(observer.factor.candidates) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "candidate undo index is unavailable"})
		return false
	}
	candidate := observer.factor.candidates[index]
	observer.factor.candidateUndos = append(observer.factor.candidateUndos, phase0ACandidateUndo{index: uint64(index), priorClaimed: candidate.Claimed, priorResolved: candidate.Resolved})
	return true
}

func phase0AReserveFactorRowsLocked(observer *phase0AObserver, candidates, expressions, bindings, transitions, selectors, routes, undos uint64) error {
	if err := phase0ACheckFactorRowsLocked(observer, candidates, expressions, bindings, transitions, selectors, routes); err != nil {
		return err
	}
	records := candidates + expressions + bindings + transitions + selectors + routes + undos
	bytes := candidates*phase0ACandidateBytes + expressions*phase0AExpressionBytes + bindings*phase0ABindingBytes + transitions*phase0ATransitionBytes + selectors*phase0ASelectorBytes + routes*phase0ARouteBytes + undos*phase0ACandidateUndoBytes
	if err := phase0AChargeManyLocked(records, bytes); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	return nil
}

func phase0ACheckFactorRowsLocked(observer *phase0AObserver, candidates, expressions, bindings, transitions, selectors, routes uint64) error {
	checks := []struct {
		current, add, limit uint64
	}{
		{uint64(len(observer.factor.candidates)), candidates, phase0AObservers.limits.MaxCandidates},
		{uint64(len(observer.factor.expressions)), expressions, phase0AObservers.limits.MaxExpressions},
		{uint64(len(observer.factor.bindings)), bindings, phase0AObservers.limits.MaxBindings},
		{uint64(len(observer.factor.transitions)), transitions, phase0AObservers.limits.MaxTransitions},
		{uint64(len(observer.factor.selectors)), selectors, phase0AObservers.limits.MaxSelectors},
		{uint64(len(observer.factor.selectorRoutes)), routes, phase0AObservers.limits.MaxSelectorRoutes},
	}
	for _, check := range checks {
		limit := check.limit
		if limit == 0 {
			limit = phase0AObservers.limits.MaxRecords
		}
		if check.current > limit || check.add > limit-check.current {
			return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "factor provenance row cap"})
		}
	}
	return nil
}

func phase0AAppendCandidatePrechargedLocked(observer *phase0AObserver, occurrence ConstructionOccurrenceKey, edge IncomingEdgeKey, boundary boundaryKey, in linkInput) {
	observer.factor.candidates = append(observer.factor.candidates, Phase0ACandidateRecord{
		TransactionID: phase0ACurrentTransaction(observer), Occurrence: occurrence, Edge: edge,
		Boundary: phase0ABoundaryInput(boundary), Payload: in.payload, Predecessor: in.prev,
		ScoreDelta: in.scoreDelta, Order: in.order.Value, HasOrder: in.order.Present,
	})
}

func phase0AFindCandidateLocked(observer *phase0AObserver, key boundaryKey, in linkInput) (int, Phase0ACandidateRecord, bool) {
	boundary := phase0ABoundaryInput(key)
	for index := range observer.factor.candidates {
		candidate := &observer.factor.candidates[index]
		if candidate.Claimed || candidate.RolledBack || !phase0ATransactionActiveLocked(observer, candidate.TransactionID) || candidate.Boundary != boundary || candidate.Payload != in.payload || candidate.Predecessor != in.prev || candidate.ScoreDelta != in.scoreDelta || candidate.HasOrder != in.order.Present || (candidate.HasOrder && candidate.Order != in.order.Value) {
			continue
		}
		return index, *candidate, true
	}
	phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "missing exact pending candidate"})
	return -1, Phase0ACandidateRecord{}, false
}

func phase0ATransactionActiveLocked(observer *phase0AObserver, transaction uint64) bool {
	if transaction == 0 {
		return len(observer.frames) == 0
	}
	for index := len(observer.frames) - 1; index >= 0; index-- {
		if observer.frames[index].TransactionID == transaction {
			return true
		}
	}
	return false
}

func phase0ATransitionLocked(observer *phase0AObserver, transition Phase0ATransitionRecord) bool {
	if err := phase0AReserveFactorRowsLocked(observer, 0, 0, 0, 1, 0, 0, 0); err != nil {
		return false
	}
	transition.TransactionID = phase0ACurrentTransaction(observer)
	observer.factor.transitions = append(observer.factor.transitions, transition)
	return true
}

func phase0AResolveCandidateLocked(observer *phase0AObserver, edge IncomingEdgeKey) {
	for index := len(observer.factor.candidates) - 1; index >= 0; index-- {
		candidate := &observer.factor.candidates[index]
		if candidate.Edge == edge && candidate.Claimed && !candidate.RolledBack {
			if !phase0AAppendCandidateUndoPrechargedLocked(observer, index) {
				return
			}
			candidate.Resolved = true
			return
		}
	}
	phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "resolved candidate is unavailable"})
}

func phase0AStableLinkIDs(core *Core, id NodeID) ([]LinkID, bool) {
	node, err := core.node(id)
	if err != nil || node.linkCount == 0 || uint64(node.linkCount) > uint64(len(core.links)) {
		return nil, false
	}
	ids := make([]LinkID, node.linkCount)
	next := LinkID(node.firstLink)
	for index := len(ids) - 1; index >= 0; index-- {
		if next == 0 || uint64(next) > uint64(len(core.links)) {
			return nil, false
		}
		if err := core.links[next-1].validateShape(); err != nil {
			return nil, false
		}
		ids[index] = next
		next = core.links[next-1].next
	}
	if next != 0 {
		return nil, false
	}
	return ids, true
}

func phase0ALiveExpressionLocked(observer *phase0AObserver, link LinkID) (Phase0AExpressionID, bool) {
	for index := len(observer.factor.bindings) - 1; index >= 0; index-- {
		binding := observer.factor.bindings[index]
		if binding.Link == link && !binding.RolledBack {
			return binding.Expression, true
		}
	}
	phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "physical link has no live provenance"})
	return 0, false
}

func phase0AObserveCandidateDrop(core *Core, key boundaryKey, in linkInput, old NodeID, index int, kind Phase0ATransitionKind) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	pendingIndex, candidate, ok := phase0AFindCandidateLocked(observer, key, in)
	if !ok {
		return
	}
	ids, valid := phase0AStableLinkIDs(core, old)
	if !valid || index < 0 || index >= len(ids) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "drop incumbent physical link is unavailable"})
		return
	}
	if err := phase0AReserveFactorRowsLocked(observer, 0, 0, 0, 1, 0, 0, 2); err != nil {
		return
	}
	if !phase0AAppendCandidateUndoPrechargedLocked(observer, pendingIndex) {
		return
	}
	observer.factor.candidates[pendingIndex].Claimed = true
	observer.factor.transitions = append(observer.factor.transitions, Phase0ATransitionRecord{Kind: kind, TransactionID: phase0ACurrentTransaction(observer), Candidate: candidate.Edge, FromLink: ids[index]})
	phase0AResolveCandidateLocked(observer, candidate.Edge)
}

func phase0AObserveDirectPublication(core *Core, key boundaryKey, in linkInput, link LinkID, node NodeID, old NodeID) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	pendingIndex, candidate, ok := phase0AFindCandidateLocked(observer, key, in)
	if !ok {
		return
	}
	transitionCount := uint64(1)
	if old != 0 {
		transitionCount++
	}
	if observer.factor.nextExpression == math.MaxUint64 || observer.factor.nextBinding == math.MaxUint64 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "direct publication serial"})
		return
	}
	if err := phase0AReserveFactorRowsLocked(observer, 0, 1, 1, transitionCount, 0, 0, 2); err != nil {
		return
	}
	if !phase0AAppendCandidateUndoPrechargedLocked(observer, pendingIndex) {
		return
	}
	observer.factor.candidates[pendingIndex].Claimed = true
	observer.factor.nextExpression++
	expression := Phase0AExpressionID(observer.factor.nextExpression)
	transaction := phase0ACurrentTransaction(observer)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: expression, Kind: Phase0AExpressionDirect, TransactionID: transaction, Occurrence: candidate.Occurrence, Edge: candidate.Edge})
	observer.factor.nextBinding++
	observer.factor.bindings = append(observer.factor.bindings, Phase0ALinkBindingRecord{Serial: observer.factor.nextBinding, TransactionID: transaction, Link: link, Expression: expression})
	kind := Phase0ATransitionDirectPublication
	if old != 0 {
		kind = Phase0ATransitionAlternateAppend
	}
	observer.factor.transitions = append(observer.factor.transitions, Phase0ATransitionRecord{Kind: kind, TransactionID: transaction, Candidate: candidate.Edge, ToLink: link, FromNode: old, ToNode: node, Expression: expression})
	if old != 0 {
		observer.factor.transitions = append(observer.factor.transitions, Phase0ATransitionRecord{Kind: Phase0ATransitionCanonicalRedirect, TransactionID: transaction, Candidate: candidate.Edge, FromNode: old, ToNode: node})
	}
	phase0AResolveCandidateLocked(observer, candidate.Edge)
}

func phase0AObservePrivatePublication(core *Core, state StateID, byteOffset uint32, in linkInput, link LinkID, node NodeID) {
	phase0AObserveDirectPublication(core, boundaryKey{frontier: core.frontier, state: state, byteOffset: byteOffset, checkpoint: core.checkpoint}, in, link, node, 0)
}

func phase0ABeginReplacement(core *Core, key boundaryKey, in linkInput, old NodeID, candidate int) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	candidateIndex, row, ok := phase0AFindCandidateLocked(observer, key, in)
	if !ok {
		return
	}
	ids, ok := phase0AStableLinkIDs(core, old)
	if !ok || candidate < 0 || candidate >= len(ids) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "replacement source adjacency is unavailable"})
		return
	}
	for _, id := range ids {
		if _, ok := phase0ALiveExpressionLocked(observer, id); !ok {
			return
		}
	}
	if observer.factor.nextExpression == math.MaxUint64 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "replacement expression serial"})
		return
	}
	bytes := uint64(unsafe.Sizeof(phase0AReplacementScope{})) + uint64(len(ids))*uint64(unsafe.Sizeof(LinkID(0)))
	limit := phase0AObservers.limits.MaxFrames
	if limit == 0 {
		limit = phase0AObservers.limits.MaxRecords
	}
	if uint64(len(observer.factor.replacements)) >= limit {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorFrameCap, Namespace: observer.run, Detail: "replacement scope cap"})
		return
	}
	if err := phase0ACheckFactorRowsLocked(observer, 0, 1, 0, 0, 0, 0); err != nil {
		return
	}
	if err := phase0AChargeManyLocked(3, phase0AExpressionBytes+phase0ACandidateUndoBytes+bytes); err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	if !phase0AAppendCandidateUndoPrechargedLocked(observer, candidateIndex) {
		return
	}
	observer.factor.candidates[candidateIndex].Claimed = true
	observer.factor.nextExpression++
	direct := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: direct, Kind: Phase0AExpressionDirect, TransactionID: phase0ACurrentTransaction(observer), Occurrence: row.Occurrence, Edge: row.Edge})
	observer.factor.replacements = append(observer.factor.replacements, phase0AReplacementScope{
		transaction: phase0ACurrentTransaction(observer), key: phase0ABoundaryInput(key), in: in,
		old: old, candidate: candidate, direct: direct, edge: row.Edge, oldIDs: ids,
	})
}

func phase0AObserveReplacementPublished(core *Core, key boundaryKey, in linkInput, node NodeID, first LinkID, count int, candidate int) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	if len(observer.factor.replacements) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "replacement publication has no scope"})
		return
	}
	index := len(observer.factor.replacements) - 1
	scope := observer.factor.replacements[index]
	observer.factor.replacements = observer.factor.replacements[:index]
	if scope.transaction != phase0ACurrentTransaction(observer) || scope.key != phase0ABoundaryInput(key) || scope.in != in || scope.candidate != candidate || len(scope.oldIDs) != count || first == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "replacement publication scope mismatch"})
		return
	}
	if observer.factor.nextBinding > math.MaxUint64-uint64(count) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "binding serial"})
		return
	}
	copyExpressions := make([]Phase0AExpressionID, len(scope.oldIDs))
	for slot, oldLink := range scope.oldIDs {
		if slot == candidate {
			copyExpressions[slot] = scope.direct
			continue
		}
		expression, ok := phase0ALiveExpressionLocked(observer, oldLink)
		if !ok {
			return
		}
		copyExpressions[slot] = expression
	}
	if err := phase0AReserveFactorRowsLocked(observer, 0, 0, uint64(count), uint64(count+1), 0, 0, 1); err != nil {
		return
	}
	for slot, oldLink := range scope.oldIDs {
		newLink := first + LinkID(slot)
		expression := copyExpressions[slot]
		kind := Phase0ATransitionPrecedenceReplacement
		if slot != candidate {
			kind = Phase0ATransitionPhysicalCopy
		}
		observer.factor.nextBinding++
		observer.factor.bindings = append(observer.factor.bindings, Phase0ALinkBindingRecord{Serial: observer.factor.nextBinding, TransactionID: scope.transaction, Link: newLink, Expression: expression})
		observer.factor.transitions = append(observer.factor.transitions, Phase0ATransitionRecord{Kind: kind, TransactionID: scope.transaction, Candidate: scope.edge, FromLink: oldLink, ToLink: newLink, FromNode: scope.old, ToNode: node, Expression: expression})
	}
	observer.factor.transitions = append(observer.factor.transitions, Phase0ATransitionRecord{Kind: Phase0ATransitionCanonicalRedirect, TransactionID: scope.transaction, Candidate: scope.edge, FromNode: scope.old, ToNode: node})
	phase0AResolveCandidateLocked(observer, scope.edge)
}

func phase0ABeginPredecessorMerge(core *Core, left, right NodeID) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	leftIDs, leftOK := phase0AStableLinkIDs(core, left)
	rightIDs, rightOK := phase0AStableLinkIDs(core, right)
	if !leftOK || !rightOK {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "merge source adjacency is unavailable"})
		return
	}
	limit := phase0AObservers.limits.MaxFrames
	if limit == 0 {
		limit = phase0AObservers.limits.MaxRecords
	}
	if uint64(len(observer.factor.mergePlans)) >= limit {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorFrameCap, Namespace: observer.run, Detail: "merge plan cap"})
		return
	}
	for _, link := range leftIDs {
		if _, ok := phase0ALiveExpressionLocked(observer, link); !ok {
			return
		}
	}
	for _, link := range rightIDs {
		if _, ok := phase0ALiveExpressionLocked(observer, link); !ok {
			return
		}
	}
	entryCapacity := len(leftIDs) + len(rightIDs)
	bytes := uint64(unsafe.Sizeof(phase0AMergePlan{})) + uint64(entryCapacity)*uint64(unsafe.Sizeof(phase0AMergeEntry{})) + uint64(len(rightIDs))*uint64(unsafe.Sizeof(LinkID(0)))
	if err := phase0AChargeManyLocked(1, bytes); err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	plan := phase0AMergePlan{transaction: phase0ACurrentTransaction(observer), left: left, rightIDs: rightIDs, rightIndex: -1, makeSelector: true}
	plan.entries = make([]phase0AMergeEntry, len(leftIDs), entryCapacity)
	for index, link := range leftIDs {
		expression, ok := phase0ALiveExpressionLocked(observer, link)
		if !ok {
			return
		}
		plan.entries[index] = phase0AMergeEntry{expression: expression, source: link, branch: Phase0ASelectorLeft, kind: Phase0ATransitionPhysicalCopy}
	}
	observer.factor.mergePlans = append(observer.factor.mergePlans, plan)
}

func phase0AMergeDecision(core *Core, incumbent int, kind Phase0ATransitionKind) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	if len(observer.factor.mergePlans) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "merge decision has no plan"})
		return
	}
	plan := &observer.factor.mergePlans[len(observer.factor.mergePlans)-1]
	plan.rightIndex++
	if plan.rightIndex < 0 || plan.rightIndex >= len(plan.rightIDs) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "merge decision has no right input"})
		return
	}
	right := plan.rightIDs[plan.rightIndex]
	expression, ok := phase0ALiveExpressionLocked(observer, right)
	if !ok {
		return
	}
	switch kind {
	case Phase0ATransitionDuplicateDrop, Phase0ATransitionPrecedenceDrop:
		if incumbent < 0 || incumbent >= len(plan.entries) {
			phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "merge drop incumbent mismatch"})
			return
		}
		phase0ATransitionLocked(observer, Phase0ATransitionRecord{Kind: kind, FromLink: right, ToLink: plan.entries[incumbent].source, Expression: expression})
	case Phase0ATransitionPrecedenceReplacement:
		if incumbent < 0 || incumbent >= len(plan.entries) {
			phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "merge replacement incumbent mismatch"})
			return
		}
		plan.entries[incumbent] = phase0AMergeEntry{expression: expression, source: right, branch: Phase0ASelectorRight, kind: kind}
	case Phase0ATransitionAlternateAppend:
		if incumbent != -1 {
			phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "merge append incumbent mismatch"})
			return
		}
		plan.entries = append(plan.entries, phase0AMergeEntry{expression: expression, source: right, branch: Phase0ASelectorRight, kind: kind})
	default:
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "unknown merge decision"})
	}
}

func phase0AMergeRecursiveDecision(core *Core, incumbent int, merged NodeID) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	if len(observer.factor.mergePlans) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "recursive merge decision has no plan"})
		return
	}
	plan := &observer.factor.mergePlans[len(observer.factor.mergePlans)-1]
	plan.rightIndex++
	if plan.rightIndex < 0 || plan.rightIndex >= len(plan.rightIDs) || incumbent < 0 || incumbent >= len(plan.entries) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "recursive merge decision input mismatch"})
		return
	}
	rightExpression, ok := phase0ALiveExpressionLocked(observer, plan.rightIDs[plan.rightIndex])
	if !ok {
		return
	}
	left := plan.entries[incumbent]
	if _, ok := phase0ALiveExpressionLocked(observer, left.source); !ok {
		return
	}
	selectorIndex := -1
	for index := len(observer.factor.selectors) - 1; index >= 0; index-- {
		selector := observer.factor.selectors[index]
		if selector.MergedPredecessor == merged && !selector.RolledBack {
			selectorIndex = index
			break
		}
	}
	if selectorIndex < 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "recursive merge selector is unavailable"})
		return
	}
	if observer.factor.nextExpression == math.MaxUint64 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "recursive merge expression serial"})
		return
	}
	if err := phase0AReserveFactorRowsLocked(observer, 0, 1, 0, 0, 0, 0, 0); err != nil {
		return
	}
	selector := observer.factor.selectors[selectorIndex]
	transaction := phase0ACurrentTransaction(observer)
	observer.factor.nextExpression++
	expression := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{
		ID: expression, Kind: Phase0AExpressionFactorChoice, TransactionID: transaction,
		Selector: selector.ID, Left: left.expression, Right: rightExpression,
	})
	plan.entries[incumbent] = phase0AMergeEntry{
		expression: expression, source: left.source, branch: left.branch,
		kind: Phase0ATransitionFactorChoice, selector: selector.ID,
	}
}

func phase0AAbortPredecessorMerge(core *Core) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || len(observer.factor.mergePlans) == 0 {
		return
	}
	observer.factor.mergePlans = observer.factor.mergePlans[:len(observer.factor.mergePlans)-1]
}

func phase0AObserveAdjacencyPublished(core *Core, node NodeID) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	if len(observer.factor.mergePlans) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "adjacency publication has no exact plan"})
		return
	}
	published, err := core.node(node)
	if err != nil || published.linkCount == 0 || published.firstLink+1 < published.linkCount {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "published adjacency is unavailable"})
		return
	}
	count := int(published.linkCount)
	first := LinkID(published.firstLink - published.linkCount + 1)
	planIndex := len(observer.factor.mergePlans) - 1
	plan := observer.factor.mergePlans[planIndex]
	observer.factor.mergePlans = observer.factor.mergePlans[:planIndex]
	if plan.transaction != phase0ACurrentTransaction(observer) || len(plan.entries) != count || first == 0 || observer.factor.nextBinding > math.MaxUint64-uint64(count) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "adjacency publication plan mismatch"})
		return
	}
	selectorCount, routeCount := uint64(0), uint64(0)
	if plan.makeSelector {
		if observer.factor.nextSelector == math.MaxUint64 {
			phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selector serial"})
			return
		}
		selectorCount, routeCount = 1, uint64(count)
	}
	undoCount := uint64(0)
	if plan.factorEdge != (IncomingEdgeKey{}) {
		undoCount = 1
	}
	if err := phase0AReserveFactorRowsLocked(observer, 0, 0, uint64(count), uint64(count), selectorCount, routeCount, undoCount); err != nil {
		return
	}
	selector := Phase0ASelectorID(0)
	firstRoute := uint64(len(observer.factor.selectorRoutes))
	if plan.makeSelector {
		observer.factor.nextSelector++
		selector = Phase0ASelectorID(observer.factor.nextSelector)
	}
	for index, entry := range plan.entries {
		link := first + LinkID(index)
		observer.factor.nextBinding++
		observer.factor.bindings = append(observer.factor.bindings, Phase0ALinkBindingRecord{Serial: observer.factor.nextBinding, TransactionID: plan.transaction, Link: link, Expression: entry.expression})
		transitionSelector := selector
		candidate := IncomingEdgeKey{}
		if !plan.makeSelector {
			transitionSelector = entry.selector
			if entry.kind == Phase0ATransitionFactorChoice {
				candidate = plan.factorEdge
			}
		}
		observer.factor.transitions = append(observer.factor.transitions, Phase0ATransitionRecord{Kind: entry.kind, TransactionID: plan.transaction, Candidate: candidate, FromLink: entry.source, ToLink: link, FromNode: plan.left, ToNode: node, Expression: entry.expression, Selector: transitionSelector})
		if plan.makeSelector {
			observer.factor.selectorRoutes = append(observer.factor.selectorRoutes, Phase0ASelectorRouteRecord{Selector: selector, TransactionID: plan.transaction, SelectedLowerLink: link, SourceLowerLink: entry.source, Branch: entry.branch})
		}
	}
	if plan.makeSelector {
		observer.factor.selectors = append(observer.factor.selectors, Phase0ASelectorRecord{ID: selector, TransactionID: plan.transaction, MergedPredecessor: node, FirstRoute: firstRoute, RouteCount: uint32(count)})
	}
	if !plan.makeSelector && plan.factorEdge != (IncomingEdgeKey{}) {
		phase0AResolveCandidateLocked(observer, plan.factorEdge)
	}
}

func phase0AObserveFactorNoChange(core *Core, key boundaryKey, in linkInput, old NodeID, candidateIndex int) {
	phase0AObserveCandidateDrop(core, key, in, old, candidateIndex, Phase0ATransitionDuplicateDrop)
}

func phase0APrepareFactorOuter(core *Core, key boundaryKey, in linkInput, old NodeID, candidateIndex int, merged NodeID) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	pendingIndex, candidate, ok := phase0AFindCandidateLocked(observer, key, in)
	if !ok {
		return
	}
	oldIDs, ok := phase0AStableLinkIDs(core, old)
	if !ok || candidateIndex < 0 || candidateIndex >= len(oldIDs) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "factor outer adjacency is unavailable"})
		return
	}
	selectorIndex := -1
	for index := len(observer.factor.selectors) - 1; index >= 0; index-- {
		selector := observer.factor.selectors[index]
		if selector.MergedPredecessor == merged && !selector.RolledBack {
			selectorIndex = index
			break
		}
	}
	if selectorIndex < 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "factor lower selector is unavailable"})
		return
	}
	selector := observer.factor.selectors[selectorIndex]
	leftExpression, ok := phase0ALiveExpressionLocked(observer, oldIDs[candidateIndex])
	if !ok {
		return
	}
	oldExpressions := make([]Phase0AExpressionID, len(oldIDs))
	for index, oldLink := range oldIDs {
		expression, live := phase0ALiveExpressionLocked(observer, oldLink)
		if !live {
			return
		}
		oldExpressions[index] = expression
	}
	if observer.factor.nextExpression > math.MaxUint64-2 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "factor expression serial"})
		return
	}
	planBytes := uint64(unsafe.Sizeof(phase0AMergePlan{})) + uint64(len(oldIDs))*uint64(unsafe.Sizeof(phase0AMergeEntry{}))
	if err := phase0ACheckFactorRowsLocked(observer, 0, 2, 0, 0, 0, 0); err != nil {
		return
	}
	if err := phase0AChargeManyLocked(4, 2*phase0AExpressionBytes+phase0ACandidateUndoBytes+planBytes); err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	if !phase0AAppendCandidateUndoPrechargedLocked(observer, pendingIndex) {
		return
	}
	observer.factor.candidates[pendingIndex].Claimed = true
	transaction := phase0ACurrentTransaction(observer)
	observer.factor.nextExpression++
	rightExpression := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: rightExpression, Kind: Phase0AExpressionDirect, TransactionID: transaction, Occurrence: candidate.Occurrence, Edge: candidate.Edge})
	observer.factor.nextExpression++
	factorExpression := Phase0AExpressionID(observer.factor.nextExpression)
	observer.factor.expressions = append(observer.factor.expressions, Phase0AExpressionRecord{ID: factorExpression, Kind: Phase0AExpressionFactorChoice, TransactionID: transaction, Selector: selector.ID, Left: leftExpression, Right: rightExpression})
	plan := phase0AMergePlan{transaction: transaction, left: old, rightIndex: -1, factorEdge: candidate.Edge}
	plan.entries = make([]phase0AMergeEntry, len(oldIDs))
	for index, oldLink := range oldIDs {
		plan.entries[index] = phase0AMergeEntry{expression: oldExpressions[index], source: oldLink, kind: Phase0ATransitionPhysicalCopy}
	}
	plan.entries[candidateIndex] = phase0AMergeEntry{expression: factorExpression, source: oldIDs[candidateIndex], kind: Phase0ATransitionFactorChoice, selector: selector.ID}
	observer.factor.mergePlans = append(observer.factor.mergePlans, plan)
}

func phase0AObserveFactorPublished(core *Core, old, node NodeID) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	if len(observer.factor.mergePlans) != 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: "factor outer plan was not consumed"})
		return
	}
	phase0ATransitionLocked(observer, Phase0ATransitionRecord{Kind: Phase0ATransitionCanonicalRedirect, FromNode: old, ToNode: node})
}

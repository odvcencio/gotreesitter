//go:build gts_workcount

package gotreesitter

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
)

const (
	DiagnosticSemanticPhaseTraceContract          = "gts-semantic-phase-trace/v5"
	DiagnosticSemanticPhaseTraceMaxEvents         = 8192
	DiagnosticSemanticPhaseTraceMaxActionCellKeys = 4096
	DiagnosticSemanticPhaseTraceMaxBoundaryKeys   = 262144
)

// DiagnosticSemanticPhaseTrace is a bounded, diagnostic-only ledger for
// locating repeated coarse action-cell classes after the parse table lookup.
// The coarse class deliberately excludes Go pointers and physical IDs, but is
// not a canonical predecessor-spine identity and may contain collisions.
type DiagnosticSemanticPhaseTrace struct {
	Contract           string                           `json:"contract"`
	MaxEvents          uint32                           `json:"max_events"`
	EventsSeen         uint64                           `json:"events_seen"`
	EventsDropped      uint64                           `json:"events_dropped"`
	ArithmeticOverflow bool                             `json:"arithmetic_overflow"`
	Aggregates         DiagnosticSemanticPhaseTotals    `json:"aggregates"`
	CollisionAudit     DiagnosticSemanticCollisionAudit `json:"collision_audit"`
	Comparability      DiagnosticSemanticComparability  `json:"comparability"`
	Events             []DiagnosticSemanticPhaseEvent   `json:"events"`

	actionCellKeys map[uint64]string
	boundaryKeys   map[uint64]semanticPhaseBoundaryKey
}

// DiagnosticSemanticPhaseTotals survives chronological-prefix truncation and
// therefore describes the whole observed parse. Phase maps are diagnostic
// vocabularies, not a cross-engine ordering contract.
type DiagnosticSemanticPhaseTotals struct {
	ActionLookupEvents    uint64            `json:"action_lookup_events"`
	ActionExecutionEvents uint64            `json:"action_execution_events"`
	DecisionEvents        uint64            `json:"decision_events"`
	LookupEmptyCells      uint64            `json:"lookup_empty_cells"`
	LookupCandidates      uint64            `json:"lookup_candidates"`
	ExecutionByActionType [4]uint64         `json:"execution_by_action_type"`
	ExecutionPhases       map[string]uint64 `json:"execution_phases"`
	DecisionPhases        map[string]uint64 `json:"decision_phases"`
	DecisionOutcomes      map[string]uint64 `json:"decision_outcomes"`
}

// DiagnosticSemanticCollisionAudit proves that equal retained hashes always
// named equal canonical input fields during the entire observed parse. It
// does not turn the deliberately shallow boundary key into predecessor-spine
// identity.
type DiagnosticSemanticCollisionAudit struct {
	Complete                        bool   `json:"complete"`
	MaxActionCellDistinctHashes     uint32 `json:"max_action_cell_distinct_hashes"`
	ActionCellKeysObserved          uint64 `json:"action_cell_keys_observed"`
	ActionCellDistinctHashes        uint64 `json:"action_cell_distinct_hashes"`
	ActionCellHashCollisions        uint64 `json:"action_cell_hash_collisions"`
	ActionCellAuditTruncated        bool   `json:"action_cell_audit_truncated"`
	ActionCellKeysUnaudited         uint64 `json:"action_cell_keys_unaudited"`
	MaxCoarseBoundaryDistinctHashes uint32 `json:"max_coarse_boundary_distinct_hashes"`
	CoarseBoundaryKeysObserved      uint64 `json:"coarse_boundary_keys_observed"`
	CoarseBoundaryDistinctHashes    uint64 `json:"coarse_boundary_distinct_hashes"`
	CoarseBoundaryHashCollisions    uint64 `json:"coarse_boundary_hash_collisions"`
	CoarseBoundaryAuditTruncated    bool   `json:"coarse_boundary_audit_truncated"`
	CoarseBoundaryKeysUnaudited     uint64 `json:"coarse_boundary_keys_unaudited"`
}

// DiagnosticSemanticComparability is deliberately fail-closed. These states
// describe what this production trace can be paired against without treating
// a transport hash or implementation-specific physical ID as semantic proof.
type DiagnosticSemanticComparability struct {
	OrderedActionCell       string `json:"ordered_action_cell"`
	ExactScannerCheckpoint  string `json:"exact_scanner_checkpoint"`
	OrderedPredecessorSpine string `json:"ordered_predecessor_spine"`
	StaticCSemanticEvents   string `json:"static_c_semantic_events"`
}

// DiagnosticSemanticPhaseEvent identifies either an action-table lookup or a
// post-reduction convergence decision. Coarse boundary classes summarize only
// the current head; they are attribution hints, not cross-engine equality.
type DiagnosticSemanticPhaseEvent struct {
	Sequence uint64 `json:"sequence"`
	Kind     string `json:"kind"`

	TokenOrdinal uint64 `json:"token_ordinal"`
	Iteration    uint64 `json:"iteration"`
	ByteOffset   uint32 `json:"byte_offset"`
	State        uint32 `json:"state"`

	LookaheadSymbol    uint32 `json:"lookahead_symbol"`
	LookaheadStartByte uint32 `json:"lookahead_start_byte"`
	LookaheadEndByte   uint32 `json:"lookahead_end_byte"`
	LookaheadFlags     uint8  `json:"lookahead_flags"`

	LookupActionCellFingerprint    uint64 `json:"lookup_action_cell_fingerprint,omitempty"`
	ExecutionActionCellFingerprint uint64 `json:"execution_action_cell_fingerprint,omitempty"`
	ExecutionCellRecomputed        bool   `json:"execution_cell_recomputed,omitempty"`
	CoarseBoundaryClass            uint64 `json:"coarse_boundary_class"`
	CandidateBoundaryClass         uint64 `json:"candidate_coarse_boundary_class,omitempty"`
	ActionOrdinal                  int16  `json:"action_ordinal"`
	ActionType                     int16  `json:"action_type"`

	Phase                string `json:"phase"`
	Outcome              string `json:"outcome"`
	Reason               string `json:"reason,omitempty"`
	CandidateCountBefore uint32 `json:"candidate_count_before"`
	CandidateCountAfter  uint32 `json:"candidate_count_after"`

	ScannerCheckpoint DiagnosticSemanticScannerCheckpoint `json:"scanner_checkpoint"`
}

// DiagnosticSemanticScannerCheckpoint authenticates the exact start/end
// serialized scanner states associated with the current token when production
// exposes them. SHA-256 is a transport digest; equality is admitted only with
// equal lengths and equal digests under the versioned domain separator.
type DiagnosticSemanticScannerCheckpoint struct {
	State       string `json:"state"`
	Reason      string `json:"reason,omitempty"`
	StartByte   uint32 `json:"start_byte"`
	EndByte     uint32 `json:"end_byte"`
	StartLength uint32 `json:"start_length,omitempty"`
	EndLength   uint32 `json:"end_length,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

var activeDiagnosticSemanticPhaseTrace *DiagnosticSemanticPhaseTrace

func semanticPhaseTraceActive() bool {
	return activeDiagnosticSemanticPhaseTrace != nil
}

// BeginDiagnosticSemanticPhaseTrace starts a parse-local trace. The trace is
// only available in gts_workcount builds and must be paired with
// BeginDiagnosticWorkCount when convergence decisions are required.
func BeginDiagnosticSemanticPhaseTrace() {
	if activeDiagnosticSemanticPhaseTrace != nil {
		panic("gotreesitter: diagnostic semantic-phase trace already active")
	}
	activeDiagnosticSemanticPhaseTrace = &DiagnosticSemanticPhaseTrace{
		Contract:  DiagnosticSemanticPhaseTraceContract,
		MaxEvents: DiagnosticSemanticPhaseTraceMaxEvents,
		Events:    make([]DiagnosticSemanticPhaseEvent, 0, DiagnosticSemanticPhaseTraceMaxEvents),
		Aggregates: DiagnosticSemanticPhaseTotals{
			ExecutionPhases: make(map[string]uint64), DecisionPhases: make(map[string]uint64),
			DecisionOutcomes: make(map[string]uint64),
		},
		Comparability: DiagnosticSemanticComparability{
			OrderedActionCell:       "production_only_canonical_fields",
			ExactScannerCheckpoint:  "per_event_when_present",
			OrderedPredecessorSpine: "unavailable_cross_engine_no_shared_canonical_spine_contract",
			StaticCSemanticEvents:   "unavailable_locked_c_exposes_aggregate_work_counts_only",
		},
		CollisionAudit: DiagnosticSemanticCollisionAudit{
			MaxActionCellDistinctHashes:     DiagnosticSemanticPhaseTraceMaxActionCellKeys,
			MaxCoarseBoundaryDistinctHashes: DiagnosticSemanticPhaseTraceMaxBoundaryKeys,
		},
		actionCellKeys: make(map[uint64]string), boundaryKeys: make(map[uint64]semanticPhaseBoundaryKey),
	}
}

// EndDiagnosticSemanticPhaseTrace returns the current trace and disables
// recording. All retained events own value data only.
func EndDiagnosticSemanticPhaseTrace() DiagnosticSemanticPhaseTrace {
	if activeDiagnosticSemanticPhaseTrace == nil {
		panic("gotreesitter: diagnostic semantic-phase trace is not active")
	}
	out := *activeDiagnosticSemanticPhaseTrace
	out.CollisionAudit.Complete = !out.ArithmeticOverflow &&
		!out.CollisionAudit.ActionCellAuditTruncated && !out.CollisionAudit.CoarseBoundaryAuditTruncated &&
		out.CollisionAudit.ActionCellHashCollisions == 0 && out.CollisionAudit.CoarseBoundaryHashCollisions == 0
	out.CollisionAudit.ActionCellDistinctHashes = uint64(len(out.actionCellKeys))
	out.CollisionAudit.CoarseBoundaryDistinctHashes = uint64(len(out.boundaryKeys))
	out.actionCellKeys = nil
	out.boundaryKeys = nil
	activeDiagnosticSemanticPhaseTrace = nil
	return out
}

func semanticPhaseTraceRecordActionCell(p *Parser, stack *glrStack, state StateID, tok Token, actions []ParseAction) {
	trace := activeDiagnosticSemanticPhaseTrace
	if trace == nil {
		return
	}
	cellHash := semanticPhaseActionCellFingerprint(actions)
	boundaryClass := semanticPhaseCoarseBoundaryClass(stack)
	semanticPhaseAuditActionCell(actions, cellHash)
	semanticPhaseAuditBoundary(stack, boundaryClass)
	checkpoint := semanticPhaseScannerCheckpoint(p, tok)
	if len(actions) == 0 {
		event := DiagnosticSemanticPhaseEvent{
			Kind: "action_lookup", TokenOrdinal: activeWorkCountConvergence.electionOrdinal, Iteration: activeWorkCountConvergence.iteration,
			ByteOffset: semanticPhaseStackByte(stack), State: uint32(state),
			LookaheadSymbol: uint32(tok.Symbol), LookaheadStartByte: tok.StartByte, LookaheadEndByte: tok.EndByte,
			LookaheadFlags: semanticPhaseLookaheadFlags(tok), LookupActionCellFingerprint: cellHash,
			CoarseBoundaryClass: boundaryClass, ActionOrdinal: -1, ActionType: -1,
			Phase: "action_cell", Outcome: "empty", ScannerCheckpoint: checkpoint,
		}
		semanticPhaseTraceAppend(event)
		referenceAtlasTraceRecordActionLookup(event)
		return
	}
	for ordinal, action := range actions {
		ordinalValue := int16(ordinal)
		if ordinal > math.MaxInt16 {
			ordinalValue = math.MaxInt16
			trace.ArithmeticOverflow = true
		}
		event := DiagnosticSemanticPhaseEvent{
			Kind: "action_lookup", TokenOrdinal: activeWorkCountConvergence.electionOrdinal, Iteration: activeWorkCountConvergence.iteration,
			ByteOffset: semanticPhaseStackByte(stack), State: uint32(state),
			LookaheadSymbol: uint32(tok.Symbol), LookaheadStartByte: tok.StartByte, LookaheadEndByte: tok.EndByte,
			LookaheadFlags: semanticPhaseLookaheadFlags(tok), LookupActionCellFingerprint: cellHash,
			CoarseBoundaryClass: boundaryClass, ActionOrdinal: ordinalValue, ActionType: int16(action.Type),
			Phase: "action_cell", Outcome: "candidate", ScannerCheckpoint: checkpoint,
		}
		semanticPhaseTraceAppend(event)
		referenceAtlasTraceRecordActionLookup(event)
	}
}

func semanticPhaseTraceRecordActionExecution(p *Parser, stack *glrStack, tok Token, action ParseAction, actionOrdinal int, phase string, cycle bool) {
	// -2 is emitted only by the production deterministic-conflict seam. Keep
	// uniqueness reconstruction entirely inside the diagnostic build.
	if actionOrdinal == -2 && (activeDiagnosticTopology != nil || activeDiagnosticSemanticPhaseTrace != nil) && p != nil && stack != nil && stack.depth() != 0 {
		actionOrdinal = semanticPhaseUniqueActionOrdinal(semanticPhaseActionsAt(p, stack.top().state, tok.Symbol), action)
	}
	workCountTopologyRecordAction(stack, tok, action, actionOrdinal)
	if activeDiagnosticSemanticPhaseTrace == nil || p == nil || stack == nil || stack.depth() == 0 {
		return
	}
	state := stack.top().state
	actions := semanticPhaseActionsAt(p, state, tok.Symbol)
	cellHash := semanticPhaseActionCellFingerprint(actions)
	semanticPhaseAuditActionCell(actions, cellHash)
	boundaryClass := semanticPhaseCoarseBoundaryClass(stack)
	semanticPhaseAuditBoundary(stack, boundaryClass)
	ordinal := int16(actionOrdinal)
	if actionOrdinal < 0 {
		ordinal = -1
	} else if actionOrdinal > math.MaxInt16 {
		ordinal = math.MaxInt16
		activeDiagnosticSemanticPhaseTrace.ArithmeticOverflow = true
	}
	outcome := "dispatched"
	if cycle {
		outcome = "cycle_rejected"
	}
	event := DiagnosticSemanticPhaseEvent{
		Kind: "action_execution", TokenOrdinal: activeWorkCountConvergence.electionOrdinal, Iteration: activeWorkCountConvergence.iteration,
		ByteOffset: semanticPhaseStackByte(stack), State: uint32(state),
		LookaheadSymbol: uint32(tok.Symbol), LookaheadStartByte: tok.StartByte, LookaheadEndByte: tok.EndByte,
		LookaheadFlags: semanticPhaseLookaheadFlags(tok), ExecutionActionCellFingerprint: cellHash, ExecutionCellRecomputed: true,
		CoarseBoundaryClass: boundaryClass, ActionOrdinal: ordinal, ActionType: int16(action.Type),
		Phase: phase, Outcome: outcome, ScannerCheckpoint: semanticPhaseScannerCheckpoint(p, tok),
	}
	semanticPhaseTraceAppend(event)
	referenceAtlasTraceRecordActionExecution(event, action)
}

func semanticPhaseUniqueActionOrdinal(actions []ParseAction, chosen ParseAction) int {
	ordinal := -1
	for index := range actions {
		if actions[index] != chosen {
			continue
		}
		if ordinal != -1 {
			return -1
		}
		ordinal = index
	}
	return ordinal
}

func semanticPhaseActionsAt(p *Parser, state StateID, symbol Symbol) []ParseAction {
	if p == nil || p.language == nil {
		return nil
	}
	var actionIndex uint16
	if int(state) < p.denseLimit {
		if int(state) >= len(p.language.ParseTable) || int(symbol) >= len(p.language.ParseTable[state]) {
			return nil
		}
		actionIndex = p.language.ParseTable[state][symbol]
	} else {
		actionIndex = p.lookupActionIndexSmall(state, symbol)
	}
	if actionIndex == 0 || int(actionIndex) >= len(p.language.ParseActions) {
		return nil
	}
	return p.language.ParseActions[actionIndex].Actions
}

func semanticPhaseTraceRecordDecision(p *Parser, phase, outcome, reason string, target, candidate *glrStack, before, after int) {
	if activeDiagnosticSemanticPhaseTrace == nil {
		return
	}
	stack := target
	if stack == nil {
		stack = candidate
	}
	lookahead := activeWorkCountConvergence.lookahead
	beforeValue, beforeOverflow := semanticPhaseBoundedUint32(before)
	afterValue, afterOverflow := semanticPhaseBoundedUint32(after)
	if beforeOverflow || afterOverflow {
		activeDiagnosticSemanticPhaseTrace.ArithmeticOverflow = true
	}
	targetClass := semanticPhaseCoarseBoundaryClass(target)
	candidateClass := semanticPhaseCoarseBoundaryClass(candidate)
	semanticPhaseAuditBoundary(target, targetClass)
	semanticPhaseAuditBoundary(candidate, candidateClass)
	event := DiagnosticSemanticPhaseEvent{
		Kind: "decision", TokenOrdinal: activeWorkCountConvergence.electionOrdinal, Iteration: activeWorkCountConvergence.iteration,
		ByteOffset: semanticPhaseStackByte(stack), State: uint32(semanticPhaseStackState(stack)),
		LookaheadSymbol: uint32(lookahead.Symbol), LookaheadStartByte: lookahead.StartByte, LookaheadEndByte: lookahead.EndByte,
		LookaheadFlags:      semanticPhaseLookaheadFlags(lookahead),
		CoarseBoundaryClass: targetClass, CandidateBoundaryClass: candidateClass,
		ActionOrdinal: -1, ActionType: -1, Phase: phase, Outcome: outcome, Reason: reason,
		CandidateCountBefore: beforeValue, CandidateCountAfter: afterValue, ScannerCheckpoint: semanticPhaseScannerCheckpoint(p, lookahead),
	}
	semanticPhaseTraceAppend(event)
	referenceAtlasTraceRecordDecision(event, target, candidate)
}

func semanticPhaseTraceAppend(event DiagnosticSemanticPhaseEvent) {
	trace := activeDiagnosticSemanticPhaseTrace
	if trace == nil {
		return
	}
	semanticPhaseTraceAggregate(trace, event)
	if trace.EventsSeen == math.MaxUint64 {
		trace.ArithmeticOverflow = true
	} else {
		trace.EventsSeen++
	}
	event.Sequence = trace.EventsSeen
	if len(trace.Events) < int(trace.MaxEvents) {
		trace.Events = append(trace.Events, event)
		return
	}
	if trace.EventsDropped == math.MaxUint64 {
		trace.ArithmeticOverflow = true
	} else {
		trace.EventsDropped++
	}
}

func semanticPhaseTraceAggregate(trace *DiagnosticSemanticPhaseTrace, event DiagnosticSemanticPhaseEvent) {
	if trace == nil {
		return
	}
	add := func(counter *uint64) {
		if *counter == math.MaxUint64 {
			trace.ArithmeticOverflow = true
			return
		}
		(*counter)++
	}
	switch event.Kind {
	case "action_lookup":
		add(&trace.Aggregates.ActionLookupEvents)
		if event.Outcome == "empty" {
			add(&trace.Aggregates.LookupEmptyCells)
		} else {
			add(&trace.Aggregates.LookupCandidates)
		}
	case "action_execution":
		add(&trace.Aggregates.ActionExecutionEvents)
		if event.ActionType >= 0 && int(event.ActionType) < len(trace.Aggregates.ExecutionByActionType) {
			add(&trace.Aggregates.ExecutionByActionType[int(event.ActionType)])
		} else {
			trace.ArithmeticOverflow = true
		}
		semanticPhaseMapAdd(trace, trace.Aggregates.ExecutionPhases, event.Phase)
	case "decision":
		add(&trace.Aggregates.DecisionEvents)
		semanticPhaseMapAdd(trace, trace.Aggregates.DecisionPhases, event.Phase)
		semanticPhaseMapAdd(trace, trace.Aggregates.DecisionOutcomes, event.Outcome)
	}
}

func semanticPhaseMapAdd(trace *DiagnosticSemanticPhaseTrace, values map[string]uint64, key string) {
	if values[key] == math.MaxUint64 {
		trace.ArithmeticOverflow = true
		return
	}
	values[key]++
}

func semanticPhaseActionCellFingerprint(actions []ParseAction) uint64 {
	h := semanticPhaseHashWord(semanticPhaseHashSeed, uint64(len(actions)))
	for _, action := range actions {
		h = semanticPhaseHashWord(h, uint64(action.Type))
		h = semanticPhaseHashWord(h, uint64(action.State))
		h = semanticPhaseHashWord(h, uint64(action.Symbol))
		h = semanticPhaseHashWord(h, uint64(action.ChildCount))
		h = semanticPhaseHashWord(h, uint64(uint16(action.DynamicPrecedence)))
		h = semanticPhaseHashWord(h, uint64(action.ProductionID))
		var flags uint64
		if action.Extra {
			flags |= 1
		}
		if action.ExtraChain {
			flags |= 2
		}
		if action.Repetition {
			flags |= 4
		}
		h = semanticPhaseHashWord(h, flags)
	}
	return h
}

func semanticPhaseAuditActionCell(actions []ParseAction, hash uint64) {
	trace := activeDiagnosticSemanticPhaseTrace
	if trace == nil {
		return
	}
	semanticPhaseAuditAdd(trace, &trace.CollisionAudit.ActionCellKeysObserved, 1)
	if previous, ok := trace.actionCellKeys[hash]; ok {
		key := semanticPhaseActionCellKey(actions)
		if previous != key {
			semanticPhaseAuditAdd(trace, &trace.CollisionAudit.ActionCellHashCollisions, 1)
		}
		return
	}
	if uint64(len(trace.actionCellKeys)) >= uint64(trace.CollisionAudit.MaxActionCellDistinctHashes) {
		trace.CollisionAudit.ActionCellAuditTruncated = true
		semanticPhaseAuditAdd(trace, &trace.CollisionAudit.ActionCellKeysUnaudited, 1)
		return
	}
	trace.actionCellKeys[hash] = semanticPhaseActionCellKey(actions)
}

func semanticPhaseActionCellKey(actions []ParseAction) string {
	words := make([]byte, 0, 8+len(actions)*64)
	words = semanticPhaseAppendWord(words, uint64(len(actions)))
	for _, action := range actions {
		words = semanticPhaseAppendWord(words, uint64(action.Type))
		words = semanticPhaseAppendWord(words, uint64(action.State))
		words = semanticPhaseAppendWord(words, uint64(action.Symbol))
		words = semanticPhaseAppendWord(words, uint64(action.ChildCount))
		words = semanticPhaseAppendWord(words, uint64(uint16(action.DynamicPrecedence)))
		words = semanticPhaseAppendWord(words, uint64(action.ProductionID))
		var flags uint64
		if action.Extra {
			flags |= 1
		}
		if action.ExtraChain {
			flags |= 2
		}
		if action.Repetition {
			flags |= 4
		}
		words = semanticPhaseAppendWord(words, flags)
	}
	return string(words)
}

type semanticPhaseBoundaryKey struct {
	Present           bool
	ByteOffset        uint32
	Depth             uint32
	State             StateID
	Symbol            Symbol
	StartByte         uint32
	EndByte           uint32
	ChildCount        uint32
	ProductionID      uint16
	DynamicPrecedence int32
	Flags             uint8
}

func semanticPhaseBoundaryKeyOf(stack *glrStack) semanticPhaseBoundaryKey {
	if stack == nil {
		return semanticPhaseBoundaryKey{}
	}
	depth, overflow := semanticPhaseBoundedUint32(stack.depth())
	if overflow && activeDiagnosticSemanticPhaseTrace != nil {
		activeDiagnosticSemanticPhaseTrace.ArithmeticOverflow = true
	}
	key := semanticPhaseBoundaryKey{Present: true, ByteOffset: stack.byteOffset, Depth: depth}
	if stack.depth() == 0 {
		return key
	}
	entry := stack.top()
	key.State = entry.state
	key.Symbol = stackEntryNodeSymbol(entry)
	key.StartByte = stackEntryNodeStartByte(entry)
	key.EndByte = stackEntryNodeEndByte(entry)
	key.ChildCount = uint32(stackEntryNodeChildCount(entry))
	key.ProductionID = stackEntryNodeProductionID(entry)
	key.DynamicPrecedence = stackEntryDynamicPrecedence(entry)
	if stackEntryNodeIsExtra(entry) {
		key.Flags |= 1
	}
	if stackEntryNodeIsMissing(entry) {
		key.Flags |= 2
	}
	if stackEntryNodeHasError(entry) {
		key.Flags |= 4
	}
	if stack.dead {
		key.Flags |= 8
	}
	if stack.accepted {
		key.Flags |= 16
	}
	if stack.cPaused {
		key.Flags |= 32
	}
	return key
}

func semanticPhaseAuditBoundary(stack *glrStack, hash uint64) {
	trace := activeDiagnosticSemanticPhaseTrace
	if trace == nil {
		return
	}
	semanticPhaseAuditAdd(trace, &trace.CollisionAudit.CoarseBoundaryKeysObserved, 1)
	key := semanticPhaseBoundaryKeyOf(stack)
	if previous, ok := trace.boundaryKeys[hash]; ok {
		if previous != key {
			semanticPhaseAuditAdd(trace, &trace.CollisionAudit.CoarseBoundaryHashCollisions, 1)
		}
		return
	}
	if uint64(len(trace.boundaryKeys)) >= uint64(trace.CollisionAudit.MaxCoarseBoundaryDistinctHashes) {
		trace.CollisionAudit.CoarseBoundaryAuditTruncated = true
		semanticPhaseAuditAdd(trace, &trace.CollisionAudit.CoarseBoundaryKeysUnaudited, 1)
		return
	}
	trace.boundaryKeys[hash] = key
}

func semanticPhaseAuditAdd(trace *DiagnosticSemanticPhaseTrace, counter *uint64, value uint64) {
	if trace == nil {
		return
	}
	if *counter > math.MaxUint64-value {
		*counter = math.MaxUint64
		trace.ArithmeticOverflow = true
		return
	}
	*counter += value
}

func semanticPhaseCoarseBoundaryClass(stack *glrStack) uint64 {
	key := semanticPhaseBoundaryKeyOf(stack)
	if !key.Present {
		return 0
	}
	h := semanticPhaseHashWord(semanticPhaseHashSeed, uint64(key.ByteOffset))
	h = semanticPhaseHashWord(h, uint64(key.Depth))
	if key.Depth == 0 {
		return h
	}
	h = semanticPhaseHashWord(h, uint64(key.State))
	h = semanticPhaseHashWord(h, uint64(key.Symbol))
	h = semanticPhaseHashWord(h, uint64(key.StartByte)<<32|uint64(key.EndByte))
	h = semanticPhaseHashWord(h, uint64(key.ChildCount))
	h = semanticPhaseHashWord(h, uint64(key.ProductionID))
	h = semanticPhaseHashWord(h, uint64(uint32(key.DynamicPrecedence)))
	h = semanticPhaseHashWord(h, uint64(key.Flags))
	return h
}

func semanticPhaseScannerCheckpoint(p *Parser, tok Token) DiagnosticSemanticScannerCheckpoint {
	if p == nil || p.language == nil {
		return DiagnosticSemanticScannerCheckpoint{State: "unavailable", Reason: "parser_or_language_absent"}
	}
	if p.language.ExternalScanner == nil {
		return DiagnosticSemanticScannerCheckpoint{
			State: "exact_empty", StartByte: tok.StartByte, EndByte: tok.EndByte,
			SHA256: semanticPhaseCheckpointDigest(tok.StartByte, tok.EndByte, nil, nil),
		}
	}
	if !p.currentExternalTokenCheckpointValid {
		return DiagnosticSemanticScannerCheckpoint{State: "unavailable", Reason: "current_token_checkpoint_not_exposed"}
	}
	if p.currentExternalTokenCheckpointStart != tok.StartByte || p.currentExternalTokenCheckpointEnd != tok.EndByte {
		return DiagnosticSemanticScannerCheckpoint{State: "unavailable", Reason: "current_token_checkpoint_span_mismatch"}
	}
	startLength, startOverflow := semanticPhaseBoundedUint32(len(p.currentExternalTokenCheckpoint.start))
	endLength, endOverflow := semanticPhaseBoundedUint32(len(p.currentExternalTokenCheckpoint.end))
	if startOverflow || endOverflow {
		if activeDiagnosticSemanticPhaseTrace != nil {
			activeDiagnosticSemanticPhaseTrace.ArithmeticOverflow = true
		}
		return DiagnosticSemanticScannerCheckpoint{State: "unavailable", Reason: "checkpoint_length_overflow"}
	}
	return DiagnosticSemanticScannerCheckpoint{
		State: "exact", StartByte: tok.StartByte, EndByte: tok.EndByte, StartLength: startLength, EndLength: endLength,
		SHA256: semanticPhaseCheckpointDigest(tok.StartByte, tok.EndByte, p.currentExternalTokenCheckpoint.start, p.currentExternalTokenCheckpoint.end),
	}
}

func semanticPhaseCheckpointDigest(startByte, endByte uint32, start, end []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte("gts-semantic-scanner-checkpoint/v2\x00"))
	var span [8]byte
	binary.LittleEndian.PutUint32(span[:4], startByte)
	binary.LittleEndian.PutUint32(span[4:], endByte)
	_, _ = h.Write(span[:])
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(start)))
	_, _ = h.Write(length[:])
	_, _ = h.Write(start)
	binary.LittleEndian.PutUint64(length[:], uint64(len(end)))
	_, _ = h.Write(length[:])
	_, _ = h.Write(end)
	return hex.EncodeToString(h.Sum(nil))
}

func semanticPhaseAppendWord(dst []byte, value uint64) []byte {
	var word [8]byte
	binary.LittleEndian.PutUint64(word[:], value)
	return append(dst, word[:]...)
}

func semanticPhaseStackState(stack *glrStack) StateID {
	if stack == nil || stack.depth() == 0 {
		return 0
	}
	return stack.top().state
}

func semanticPhaseStackByte(stack *glrStack) uint32 {
	if stack == nil {
		return 0
	}
	return stack.byteOffset
}

func semanticPhaseLookaheadFlags(tok Token) uint8 {
	var flags uint8
	if tok.NoLookahead {
		flags |= 1
	}
	if tok.ExternalScannerToken {
		flags |= 2
	}
	return flags
}

func semanticPhaseBoundedUint32(value int) (uint32, bool) {
	if value < 0 {
		return 0, true
	}
	if uint64(value) > math.MaxUint32 {
		return math.MaxUint32, true
	}
	return uint32(value), false
}

const (
	semanticPhaseHashSeed  uint64 = 1469598103934665603
	semanticPhaseHashPrime uint64 = 1099511628211
)

func semanticPhaseHashWord(hash, value uint64) uint64 {
	for shift := uint(0); shift < 64; shift += 8 {
		hash ^= (value >> shift) & 0xff
		hash *= semanticPhaseHashPrime
	}
	return hash
}

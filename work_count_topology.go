//go:build gts_workcount

package gotreesitter

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"sync"
)

const (
	// DiagnosticTopologyReceiptSchema is the shared Go and locked-C event
	// contract. The receipt retains a chronological prefix.
	DiagnosticTopologyReceiptSchema    = "gts-topology-receipt/v1"
	DiagnosticTopologyReceiptCapacity  = 4096
	diagnosticTopologyIdentityCapacity = 64 * 1024
)

const (
	DiagnosticTopologyEventAction uint64 = iota + 1
	DiagnosticTopologyEventVersionAdd
	DiagnosticTopologyEventVersionCopy
	DiagnosticTopologyEventVersionRenumber
	DiagnosticTopologyEventMerge
	DiagnosticTopologyEventLinkInsert
	DiagnosticTopologyEventPopPath
	DiagnosticTopologyEventChildElection
	DiagnosticTopologyEventAcceptElection
)

const (
	DiagnosticTopologyFlagSuccessOrSelected uint64 = 1 << iota
	DiagnosticTopologyFlagPrimaryLink
	DiagnosticTopologyFlagInitialVersion
	DiagnosticTopologyFlagTargetReplaced
	DiagnosticTopologyFlagNoIncumbent
	DiagnosticTopologyFlagActionContextKnown
)

const diagnosticTopologyNoActionType = uint64(255)

type diagnosticTopologyStackToken struct {
	versionID uint64
}

const diagnosticTopologyStackOverhead = uintptr(8)

// DiagnosticTopologyEvent is one event in the shared topology contract.
// Every field has a fixed uint64 framing position. ActionOrdinal uses -1 when
// no exact table ordinal is available.
type DiagnosticTopologyEvent struct {
	EventID           uint64 `json:"event_id"`
	Kind              uint64 `json:"kind"`
	ActionID          uint64 `json:"action_id"`
	ActionOrdinal     int64  `json:"action_ordinal"`
	ActionType        uint64 `json:"action_type"`
	State             uint64 `json:"state"`
	LookaheadSymbol   uint64 `json:"lookahead_symbol"`
	ByteOffset        uint64 `json:"byte_offset"`
	VersionID         uint64 `json:"version_id"`
	VersionIndex      uint64 `json:"version_index"`
	SourceVersionID   uint64 `json:"source_version_id"`
	SourceIndex       uint64 `json:"source_index"`
	TargetVersionID   uint64 `json:"target_version_id"`
	TargetIndex       uint64 `json:"target_index"`
	SurvivorVersionID uint64 `json:"survivor_version_id"`
	RemovedVersionID  uint64 `json:"removed_version_id"`
	NodeID            uint64 `json:"node_id"`
	PredecessorNodeID uint64 `json:"predecessor_node_id"`
	LinkID            uint64 `json:"link_id"`
	LinkOrdinal       uint64 `json:"link_ordinal"`
	PopID             uint64 `json:"pop_id"`
	PathOrdinal       uint64 `json:"path_ordinal"`
	PopToNodeID       uint64 `json:"pop_to_node_id"`
	ElectionID        uint64 `json:"election_id"`
	IncumbentID       uint64 `json:"incumbent_id"`
	CandidateID       uint64 `json:"candidate_id"`
	SelectedID        uint64 `json:"selected_id"`
	PayloadCount      uint64 `json:"payload_count"`
	Flags             uint64 `json:"flags"`
}

// DiagnosticTopologyReceipt is a bounded physical topology receipt. A
// complete receipt has no truncation, overflow, collision, or incomplete ID.
type DiagnosticTopologyReceipt struct {
	Schema             string                    `json:"schema"`
	Capacity           uint64                    `json:"capacity"`
	EventsSeen         uint64                    `json:"events_seen"`
	EventsRetained     uint64                    `json:"events_retained"`
	EventsDropped      uint64                    `json:"events_dropped"`
	Truncated          bool                      `json:"truncated"`
	ArithmeticOverflow bool                      `json:"arithmetic_overflow"`
	IdentityCollision  bool                      `json:"identity_collision"`
	IdentityIncomplete bool                      `json:"identity_incomplete"`
	Events             []DiagnosticTopologyEvent `json:"events"`
}

// Complete reports whether the receipt retained every event and every ID.
func (r DiagnosticTopologyReceipt) Complete() bool {
	return !r.Truncated && !r.ArithmeticOverflow && !r.IdentityCollision && !r.IdentityIncomplete
}

// SHA256 returns the shared little-endian u64 framing digest.
func (r DiagnosticTopologyReceipt) SHA256() string {
	h := sha256.New()
	_, _ = h.Write([]byte(DiagnosticTopologyReceiptSchema))
	_, _ = h.Write([]byte{0})
	write := func(value uint64) {
		var encoded [8]byte
		binary.LittleEndian.PutUint64(encoded[:], value)
		_, _ = h.Write(encoded[:])
	}
	write(r.Capacity)
	write(r.EventsSeen)
	write(r.EventsRetained)
	write(r.EventsDropped)
	write(diagnosticTopologyBool(r.Truncated))
	write(diagnosticTopologyBool(r.ArithmeticOverflow))
	write(diagnosticTopologyBool(r.IdentityCollision))
	write(diagnosticTopologyBool(r.IdentityIncomplete))
	for i := range r.Events {
		event := r.Events[i]
		for _, value := range [...]uint64{
			event.EventID,
			event.Kind,
			event.ActionID,
			uint64(event.ActionOrdinal),
			event.ActionType,
			event.State,
			event.LookaheadSymbol,
			event.ByteOffset,
			event.VersionID,
			event.VersionIndex,
			event.SourceVersionID,
			event.SourceIndex,
			event.TargetVersionID,
			event.TargetIndex,
			event.SurvivorVersionID,
			event.RemovedVersionID,
			event.NodeID,
			event.PredecessorNodeID,
			event.LinkID,
			event.LinkOrdinal,
			event.PopID,
			event.PathOrdinal,
			event.PopToNodeID,
			event.ElectionID,
			event.IncumbentID,
			event.CandidateID,
			event.SelectedID,
			event.PayloadCount,
			event.Flags,
		} {
			write(value)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func diagnosticTopologyBool(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

type diagnosticTopologyLinkKey struct {
	nodeID  uint64
	ordinal uint64
}

type diagnosticTopologyMergeKey struct {
	targetID    uint64
	candidateID uint64
}

type diagnosticTopologyVersion struct {
	id            uint64
	index         uint64
	head          *gssNode
	nodeID        uint64
	entrySnapshot []stackEntry
	entryNodeIDs  []uint64
	emptyNodeID   uint64
	snapshot      glrStack
}

type diagnosticTopologyAction struct {
	id                uint64
	ordinal           int64
	typeID            uint64
	state             uint64
	lookahead         uint64
	byte              uint64
	symbol            uint64
	versionID         uint64
	index             uint64
	popID             uint64
	directPop         bool
	candidatePrepared bool
}

type diagnosticTopologyPendingCopy struct {
	sourceVersionID uint64
	sourceIndex     uint64
	targetVersionID uint64
	targetIndex     uint64
}

type diagnosticTopologyCandidate struct {
	id       uint64
	popID    uint64
	ordinal  uint64
	popTo    *gssNode
	topState StateID
	window   []stackEntry
}

type diagnosticTopologyPromotion struct {
	versionID uint64
	nodeIDs   []uint64
	next      int
}

type diagnosticTopologyState struct {
	receipt DiagnosticTopologyReceipt

	nextActionID    uint64
	nextVersionID   uint64
	nextNodeID      uint64
	nextLinkID      uint64
	nextPopID       uint64
	nextElectionID  uint64
	nextCandidateID uint64

	nodeIDs                      map[*gssNode]uint64
	linkIDs                      map[diagnosticTopologyLinkKey]uint64
	linkPrevIDs                  map[diagnosticTopologyLinkKey]uint64
	versions                     map[uint64]diagnosticTopologyVersion
	versionSlots                 []uint64
	actions                      map[*glrStack]diagnosticTopologyAction
	pendingCopies                map[*glrStack]diagnosticTopologyPendingCopy
	preRecordedMerges            map[diagnosticTopologyMergeKey]bool
	candidates                   []diagnosticTopologyCandidate
	currentAction                *diagnosticTopologyAction
	reductionActionSourceID      uint64
	reductionActionTargetID      uint64
	reductionActionAddRecorded   bool
	reductionActionAddEventIndex int
	promotion                    diagnosticTopologyPromotion
	parser                       *Parser
	arena                        *nodeArena
	trackErrors                  *bool
	acceptCurrent                diagnosticTopologyAcceptCandidate
	acceptSet                    bool
	acceptEOFNodeID              uint64
	acceptSuppressNextLinkInsert bool
}

// diagnosticTopologyReceiptOwner gives one caller exclusive receipt ownership.
// A concurrent caller waits until EndDiagnosticTopologyReceipt releases it.
var diagnosticTopologyReceiptOwner sync.Mutex
var activeDiagnosticTopology *diagnosticTopologyState

// BeginDiagnosticTopologyReceipt starts one exclusive topology receipt.
// It is available only in gts_workcount builds.
func BeginDiagnosticTopologyReceipt() {
	diagnosticTopologyReceiptOwner.Lock()
	if activeDiagnosticTopology != nil {
		diagnosticTopologyReceiptOwner.Unlock()
		panic("gotreesitter: diagnostic topology receipt already active")
	}
	activeDiagnosticTopology = &diagnosticTopologyState{
		receipt: DiagnosticTopologyReceipt{
			Schema:   DiagnosticTopologyReceiptSchema,
			Capacity: DiagnosticTopologyReceiptCapacity,
			Events:   make([]DiagnosticTopologyEvent, 0, DiagnosticTopologyReceiptCapacity),
		},
		nodeIDs:           make(map[*gssNode]uint64),
		linkIDs:           make(map[diagnosticTopologyLinkKey]uint64),
		linkPrevIDs:       make(map[diagnosticTopologyLinkKey]uint64),
		versions:          make(map[uint64]diagnosticTopologyVersion),
		actions:           make(map[*glrStack]diagnosticTopologyAction),
		pendingCopies:     make(map[*glrStack]diagnosticTopologyPendingCopy),
		preRecordedMerges: make(map[diagnosticTopologyMergeKey]bool),
	}
}

// EndDiagnosticTopologyReceipt returns the receipt and releases its owner.
func EndDiagnosticTopologyReceipt() DiagnosticTopologyReceipt {
	if activeDiagnosticTopology == nil {
		panic("gotreesitter: diagnostic topology receipt is not active")
	}
	state := activeDiagnosticTopology
	state.receipt.EventsRetained = uint64(len(state.receipt.Events))
	out := state.receipt
	activeDiagnosticTopology = nil
	diagnosticTopologyReceiptOwner.Unlock()
	return out
}

func (s *diagnosticTopologyState) nextID(slot *uint64) uint64 {
	if *slot == math.MaxUint64 {
		s.receipt.ArithmeticOverflow = true
		s.receipt.IdentityIncomplete = true
		return 0
	}
	*slot++
	return *slot
}

func (s *diagnosticTopologyState) appendEvent(event DiagnosticTopologyEvent) {
	if s == nil {
		return
	}
	if s.receipt.EventsSeen == math.MaxUint64 {
		s.receipt.ArithmeticOverflow = true
		s.receipt.IdentityIncomplete = true
		return
	}
	s.receipt.EventsSeen++
	event.EventID = s.receipt.EventsSeen
	if uint64(len(s.receipt.Events)) < s.receipt.Capacity {
		s.receipt.Events = append(s.receipt.Events, event)
		return
	}
	s.receipt.Truncated = true
	if s.receipt.EventsDropped == math.MaxUint64 {
		s.receipt.ArithmeticOverflow = true
		return
	}
	s.receipt.EventsDropped++
}

func (s *diagnosticTopologyState) identityAvailable(size int) bool {
	if size < diagnosticTopologyIdentityCapacity {
		return true
	}
	s.receipt.IdentityIncomplete = true
	return false
}

func (s *diagnosticTopologyState) nextNodeIdentity() uint64 {
	if s.nextNodeID == math.MaxUint64 {
		return s.nextID(&s.nextNodeID)
	}
	if s.nextNodeID >= diagnosticTopologyIdentityCapacity {
		s.receipt.IdentityIncomplete = true
		return 0
	}
	return s.nextID(&s.nextNodeID)
}

func (s *diagnosticTopologyState) markIdentityCollision() {
	s.receipt.IdentityCollision = true
	s.receipt.IdentityIncomplete = true
}

func workCountTopologyBeginAttempt() {
	s := activeDiagnosticTopology
	if s == nil {
		return
	}
	clear(s.versions)
	s.versionSlots = s.versionSlots[:0]
	clear(s.actions)
	clear(s.pendingCopies)
	clear(s.preRecordedMerges)
	s.candidates = s.candidates[:0]
	s.currentAction = nil
	s.reductionActionSourceID = 0
	s.reductionActionTargetID = 0
	s.reductionActionAddRecorded = false
	s.reductionActionAddEventIndex = -1
	s.promotion = diagnosticTopologyPromotion{}
	s.parser = nil
	s.arena = nil
	s.trackErrors = nil
	s.acceptCurrent = diagnosticTopologyAcceptCandidate{}
	s.acceptSet = false
	s.acceptEOFNodeID = 0
	s.acceptSuppressNextLinkInsert = false
}

func workCountTopologySetParseContext(p *Parser, arena *nodeArena, trackErrors *bool) {
	s := activeDiagnosticTopology
	if s == nil {
		return
	}
	s.parser = p
	s.arena = arena
	s.trackErrors = trackErrors
}

func workCountTopologyPreparePromotion(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil || stack.gss.head != nil || len(stack.entries) == 0 {
		return
	}
	if s.promotion.versionID != 0 {
		s.markIdentityCollision()
		return
	}
	versionID := s.versionID(stack)
	s.syncVersionEntries(versionID, stack)
	version, ok := s.versions[versionID]
	if !ok || len(version.entryNodeIDs) != len(stack.entries) {
		s.receipt.IdentityIncomplete = true
		return
	}
	s.promotion = diagnosticTopologyPromotion{
		versionID: versionID,
		nodeIDs:   append([]uint64(nil), version.entryNodeIDs...),
	}
}

func workCountTopologyCommitPromotion(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	promotion := s.promotion
	s.promotion = diagnosticTopologyPromotion{}
	if promotion.versionID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	if promotion.next != len(promotion.nodeIDs) || stack.gss.head == nil {
		s.receipt.IdentityIncomplete = true
		return
	}
	if got, want := s.nodeID(stack.gss.head), promotion.nodeIDs[len(promotion.nodeIDs)-1]; got == 0 || got != want {
		s.markIdentityCollision()
		return
	}
	s.bindVersion(stack, promotion.versionID)
}

func workCountTopologyRecordDemotion(stack *glrStack, entries []stackEntry) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil || stack.gss.head == nil || len(entries) == 0 {
		return
	}
	versionID := s.versionID(stack)
	version, ok := s.versions[versionID]
	if !ok || versionID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	nodeIDs := make([]uint64, len(entries))
	node := stack.gss.head
	for i := len(entries) - 1; i >= 0; i-- {
		if node == nil || node.entry != entries[i] {
			s.receipt.IdentityIncomplete = true
			return
		}
		nodeIDs[i] = s.nodeID(node)
		node = node.prev
	}
	if node != nil {
		s.receipt.IdentityIncomplete = true
		return
	}
	version.head = nil
	version.entrySnapshot = append(version.entrySnapshot[:0], entries...)
	version.entryNodeIDs = append(version.entryNodeIDs[:0], nodeIDs...)
	version.nodeID = nodeIDs[len(nodeIDs)-1]
	s.versions[versionID] = version
}

func workCountTopologyRecordNodeAllocation(node *gssNode) {
	s := activeDiagnosticTopology
	if s == nil || node == nil {
		return
	}
	if s.promotion.versionID != 0 {
		if s.promotion.next >= len(s.promotion.nodeIDs) {
			s.receipt.IdentityIncomplete = true
			return
		}
		id := s.promotion.nodeIDs[s.promotion.next]
		s.promotion.next++
		if id == 0 {
			s.receipt.IdentityIncomplete = true
			return
		}
		s.nodeIDs[node] = id
		return
	}
	if !s.identityAvailable(len(s.nodeIDs)) {
		delete(s.nodeIDs, node)
		return
	}
	id := s.nextNodeIdentity()
	if id == 0 {
		return
	}
	s.nodeIDs[node] = id
}

func (s *diagnosticTopologyState) nodeID(node *gssNode) uint64 {
	if node == nil {
		return 0
	}
	if id := s.nodeIDs[node]; id != 0 {
		return id
	}
	// Production GSS nodes pass through allocNode. A missing allocation event
	// means this pointer came from a synthetic or unsupported construction.
	s.receipt.IdentityIncomplete = true
	if !s.identityAvailable(len(s.nodeIDs)) {
		return 0
	}
	id := s.nextNodeIdentity()
	if id != 0 {
		s.nodeIDs[node] = id
	}
	return id
}

func diagnosticTopologyStackState(stack *glrStack) uint64 {
	if stack == nil || stack.depth() == 0 {
		return 0
	}
	return uint64(stack.top().state)
}

func (s *diagnosticTopologyState) recordVersionShift(sourceIndex int) bool {
	if sourceIndex <= 0 || sourceIndex >= len(s.versionSlots) {
		s.receipt.IdentityIncomplete = true
		return false
	}
	movedID := s.versionSlots[sourceIndex]
	replacedID := s.versionSlots[sourceIndex-1]
	moved, exists := s.versions[movedID]
	_, targetExists := s.versions[replacedID]
	// A direct lower-slot promotion leaves its source ID in the old source
	// slot until the stable erase. The target ID can therefore have another
	// authoritative index during the later-slot shift sequence.
	if movedID != 0 && movedID == replacedID {
		s.markIdentityCollision()
		return false
	}
	if !exists || !targetExists || movedID == 0 || replacedID == 0 || moved.index != uint64(sourceIndex) {
		s.receipt.IdentityIncomplete = true
		return false
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventVersionRenumber
	event.VersionID = movedID
	event.VersionIndex = uint64(sourceIndex - 1)
	event.SourceVersionID = movedID
	event.SourceIndex = uint64(sourceIndex)
	event.TargetVersionID = replacedID
	event.TargetIndex = uint64(sourceIndex - 1)
	event.NodeID = s.versionNodeID(movedID, nil)
	event.Flags = DiagnosticTopologyFlagTargetReplaced
	if action := s.currentAction; action != nil {
		s.applyActionContext(&event, action)
	}
	s.appendEvent(event)
	moved.index = uint64(sourceIndex - 1)
	s.versions[movedID] = moved
	s.versionSlots[sourceIndex-1] = movedID
	return true
}

func (s *diagnosticTopologyState) forgetVersion(id uint64) {
	_, ok := s.versions[id]
	if !ok || id == 0 {
		return
	}
	delete(s.versions, id)
	for stack, action := range s.actions {
		if action.versionID == id {
			delete(s.actions, stack)
		}
	}
	if s.currentAction != nil && s.currentAction.versionID == id {
		s.currentAction = nil
	}
	for stack, copy := range s.pendingCopies {
		if copy.targetVersionID == id {
			delete(s.pendingCopies, stack)
		}
	}
}

func (s *diagnosticTopologyState) versionHasMutationBinding(ids ...uint64) bool {
	contains := func(id uint64) bool {
		for _, candidate := range ids {
			if id != 0 && id == candidate {
				return true
			}
		}
		return false
	}
	for _, action := range s.actions {
		if contains(action.versionID) {
			return true
		}
	}
	if s.currentAction != nil && contains(s.currentAction.versionID) {
		return true
	}
	for _, copy := range s.pendingCopies {
		if contains(copy.targetVersionID) {
			return true
		}
	}
	return false
}

func (s *diagnosticTopologyState) retireVersion(id uint64) {
	version, ok := s.versions[id]
	if !ok || id == 0 || version.index >= uint64(len(s.versionSlots)) || s.versionSlots[version.index] != id {
		s.receipt.IdentityIncomplete = true
		return
	}
	removedIndex := int(version.index)
	for sourceIndex := removedIndex + 1; sourceIndex < len(s.versionSlots); sourceIndex++ {
		if !s.recordVersionShift(sourceIndex) {
			return
		}
	}
	s.versionSlots = s.versionSlots[:len(s.versionSlots)-1]
	s.forgetVersion(id)
}

func workCountTopologyRenumberVersion(source, target *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || source == nil || target == nil {
		return
	}
	sourceID := source.diagnosticTopology.versionID
	targetID := target.diagnosticTopology.versionID
	if source.accepted || target.accepted {
		return
	}
	if sourceID != 0 && sourceID == targetID {
		return
	}
	if s.versionHasMutationBinding(sourceID, targetID) {
		s.receipt.IdentityIncomplete = true
		return
	}
	sourceVersion, sourceOK := s.versions[sourceID]
	targetVersion, targetOK := s.versions[targetID]
	if !sourceOK || !targetOK || sourceID == 0 || targetID == 0 ||
		sourceVersion.index <= targetVersion.index || sourceVersion.index >= uint64(len(s.versionSlots)) ||
		s.versionSlots[sourceVersion.index] != sourceID || s.versionSlots[targetVersion.index] != targetID {
		s.receipt.IdentityIncomplete = true
		return
	}
	sourceIndex := int(sourceVersion.index)
	targetIndex := int(targetVersion.index)
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventVersionRenumber
	event.VersionID = sourceID
	event.VersionIndex = uint64(targetIndex)
	event.SourceVersionID = sourceID
	event.SourceIndex = uint64(sourceIndex)
	event.TargetVersionID = targetID
	event.TargetIndex = uint64(targetIndex)
	event.NodeID = s.versionNodeID(sourceID, source)
	event.Flags = DiagnosticTopologyFlagTargetReplaced
	if action := s.currentAction; action != nil {
		s.applyActionContext(&event, action)
	}
	s.appendEvent(event)
	sourceVersion.index = uint64(targetIndex)
	s.versions[sourceID] = sourceVersion
	s.versionSlots[targetIndex] = sourceID
	for index := sourceIndex + 1; index < len(s.versionSlots); index++ {
		if !s.recordVersionShift(index) {
			return
		}
	}
	s.versionSlots = s.versionSlots[:len(s.versionSlots)-1]
	s.forgetVersion(targetID)
	target.diagnosticTopology.versionID = 0
}

func workCountTopologyRetireVersion(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	id := stack.diagnosticTopology.versionID
	if s.acceptSet && stack.accepted && id == 0 {
		return
	}
	if id == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	if _, ok := s.versions[id]; !ok {
		return
	}
	s.retireVersion(id)
	stack.diagnosticTopology.versionID = 0
}

func workCountTopologyRetireVersionIfActive(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	id := stack.diagnosticTopology.versionID
	if id == 0 {
		return
	}
	if _, ok := s.versions[id]; !ok {
		stack.diagnosticTopology.versionID = 0
		return
	}
	s.retireVersion(id)
	stack.diagnosticTopology.versionID = 0
}

func workCountTopologyRetireVersionsIfActive(stacks []glrStack) {
	for i := range stacks {
		workCountTopologyRetireVersionIfActive(&stacks[i])
	}
}

func workCountTopologyRetireUnpublishedVersions(owned, published []glrStack) {
	for i := range owned {
		id := owned[i].diagnosticTopology.versionID
		if id == 0 {
			continue
		}
		found := false
		for j := range published {
			if published[j].diagnosticTopology.versionID == id {
				found = true
				break
			}
		}
		if !found {
			workCountTopologyRetireVersionIfActive(&owned[i])
		}
	}
}

func workCountTopologyRetireMissingVersions(before, after []glrStack) {
	s := activeDiagnosticTopology
	if s == nil {
		return
	}
	if s.acceptSet && len(after) != 0 {
		allAccepted := true
		for i := range after {
			if !after[i].accepted {
				allAccepted = false
				break
			}
		}
		if allAccepted {
			// C keeps the accepted stack and pop-all slice alive until parser
			// teardown. Final Go result compaction must not publish removals.
			return
		}
	}
	for i := range before {
		id := before[i].diagnosticTopology.versionID
		if id == 0 {
			continue
		}
		retained := false
		for j := range after {
			if after[j].diagnosticTopology.versionID == id {
				retained = true
				break
			}
		}
		if !retained {
			if _, ok := s.versions[id]; ok {
				s.retireVersion(id)
			}
		}
	}
	workCountTopologySyncVersionOrder(after)
}

// workCountTopologyReconcileVersionSelection records a swap-based survivor
// selection. The selection moves survivors before it truncates the tail, so it
// does not emit the stable-deletion renumbers used by array erasure.
func workCountTopologyReconcileVersionSelection(before, after []glrStack) {
	s := activeDiagnosticTopology
	if s == nil {
		return
	}
	beforeIDs := make(map[uint64]struct{}, len(before))
	for i := range before {
		if before[i].accepted {
			continue
		}
		id := before[i].diagnosticTopology.versionID
		if id == 0 {
			s.receipt.IdentityIncomplete = true
			return
		}
		if _, ok := s.versions[id]; !ok {
			s.receipt.IdentityIncomplete = true
			return
		}
		if _, duplicate := beforeIDs[id]; duplicate {
			s.markIdentityCollision()
			return
		}
		beforeIDs[id] = struct{}{}
	}
	if len(beforeIDs) != len(s.versionSlots) {
		s.receipt.IdentityIncomplete = true
		return
	}
	ordered := make([]uint64, 0, len(s.versionSlots))
	retained := make(map[uint64]struct{}, len(after))
	for i := range after {
		if after[i].accepted {
			continue
		}
		id := after[i].diagnosticTopology.versionID
		if id == 0 {
			s.receipt.IdentityIncomplete = true
			return
		}
		if _, ok := beforeIDs[id]; !ok {
			s.receipt.IdentityIncomplete = true
			return
		}
		if _, duplicate := retained[id]; duplicate {
			s.markIdentityCollision()
			return
		}
		retained[id] = struct{}{}
		ordered = append(ordered, id)
	}
	removed := make([]uint64, 0, len(s.versionSlots)-len(ordered))
	for _, id := range s.versionSlots {
		if _, ok := retained[id]; !ok {
			removed = append(removed, id)
		}
	}
	ordered = append(ordered, removed...)
	if len(ordered) != len(s.versionSlots) {
		s.receipt.IdentityIncomplete = true
		return
	}
	copy(s.versionSlots, ordered)
	for index, id := range s.versionSlots {
		version, ok := s.versions[id]
		if !ok {
			s.receipt.IdentityIncomplete = true
			return
		}
		version.index = uint64(index)
		s.versions[id] = version
	}
	for _, id := range removed {
		s.forgetVersion(id)
	}
	s.versionSlots = s.versionSlots[:len(retained)]
}

func workCountTopologySyncVersionOrder(stacks []glrStack) {
	s := activeDiagnosticTopology
	if s == nil {
		return
	}
	seen := make(map[uint64]struct{}, len(stacks))
	versionIndex := 0
	for i := range stacks {
		if stacks[i].accepted {
			continue
		}
		id := stacks[i].diagnosticTopology.versionID
		if id == 0 {
			s.receipt.IdentityIncomplete = true
			return
		}
		version, ok := s.versions[id]
		if !ok {
			s.receipt.IdentityIncomplete = true
			return
		}
		if _, duplicate := seen[id]; duplicate {
			s.markIdentityCollision()
			return
		}
		seen[id] = struct{}{}
		version.index = uint64(versionIndex)
		s.versions[id] = version
		if versionIndex >= len(s.versionSlots) {
			s.receipt.IdentityIncomplete = true
			return
		}
		s.versionSlots[versionIndex] = id
		versionIndex++
	}
	if versionIndex != len(s.versionSlots) {
		s.receipt.IdentityIncomplete = true
	}
}

func (s *diagnosticTopologyState) bindVersion(stack *glrStack, id uint64) {
	if stack == nil || id == 0 {
		return
	}
	version, ok := s.versions[id]
	if !ok {
		s.receipt.IdentityIncomplete = true
		return
	}
	stack.diagnosticTopology.versionID = id
	newHead := stack.gss.head
	version.head = newHead
	if newHead != nil {
		version.nodeID = s.nodeID(newHead)
	}
	s.versions[id] = version
	if newHead == nil {
		s.syncVersionEntries(id, stack)
	}
	version = s.versions[id]
	version.snapshot = *stack
	if stack.entries != nil {
		version.snapshot.entries = append([]stackEntry(nil), stack.entries...)
	}
	s.versions[id] = version
}

func diagnosticTopologyCommonEntryPrefix(a, b []stackEntry) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func (s *diagnosticTopologyState) syncVersionEntries(id uint64, stack *glrStack) {
	if id == 0 || stack == nil || stack.gss.head != nil {
		return
	}
	version, ok := s.versions[id]
	if !ok {
		s.receipt.IdentityIncomplete = true
		return
	}
	common := diagnosticTopologyCommonEntryPrefix(version.entrySnapshot, stack.entries)
	if common < len(version.entryNodeIDs) {
		version.entryNodeIDs = version.entryNodeIDs[:common]
	}
	for len(version.entryNodeIDs) < len(stack.entries) {
		id := s.nextNodeIdentity()
		if id == 0 {
			break
		}
		version.entryNodeIDs = append(version.entryNodeIDs, id)
	}
	version.entrySnapshot = append(version.entrySnapshot[:0], stack.entries...)
	if len(version.entryNodeIDs) == len(stack.entries) && len(version.entryNodeIDs) != 0 {
		version.nodeID = version.entryNodeIDs[len(version.entryNodeIDs)-1]
	} else if len(stack.entries) == 0 {
		if version.emptyNodeID == 0 {
			version.emptyNodeID = s.nextNodeIdentity()
		}
		version.nodeID = version.emptyNodeID
	} else {
		version.nodeID = 0
		s.receipt.IdentityIncomplete = true
	}
	s.versions[id] = version
}

func (s *diagnosticTopologyState) allocateVersion(stack *glrStack, sourceID uint64) uint64 {
	if stack == nil || s.nextVersionID >= diagnosticTopologyIdentityCapacity {
		s.receipt.IdentityIncomplete = true
		return 0
	}
	id := s.nextID(&s.nextVersionID)
	if id == 0 {
		return 0
	}
	version := diagnosticTopologyVersion{id: id, index: uint64(len(s.versionSlots))}
	if sourceID != 0 {
		source, ok := s.versions[sourceID]
		if !ok {
			s.receipt.IdentityIncomplete = true
		} else {
			version.nodeID = source.nodeID
			version.entrySnapshot = append([]stackEntry(nil), source.entrySnapshot...)
			version.entryNodeIDs = append([]uint64(nil), source.entryNodeIDs...)
			version.emptyNodeID = source.emptyNodeID
		}
	}
	s.versions[id] = version
	s.versionSlots = append(s.versionSlots, id)
	s.bindVersion(stack, id)
	version = s.versions[id]
	if version.nodeID == 0 {
		version.nodeID = s.nextNodeIdentity()
		s.versions[id] = version
	}
	return id
}

func (s *diagnosticTopologyState) versionNodeID(versionID uint64, stack *glrStack) uint64 {
	if stack != nil && stack.gss.head != nil {
		return s.nodeID(stack.gss.head)
	}
	version, ok := s.versions[versionID]
	if !ok || versionID == 0 {
		s.receipt.IdentityIncomplete = true
		return 0
	}
	if version.nodeID == 0 {
		version.nodeID = s.nextNodeIdentity()
		s.versions[versionID] = version
	}
	return version.nodeID
}

func (s *diagnosticTopologyState) versionID(stack *glrStack) uint64 {
	if stack == nil {
		return 0
	}
	if id := stack.diagnosticTopology.versionID; id != 0 {
		if _, ok := s.versions[id]; !ok {
			s.receipt.IdentityIncomplete = true
			return 0
		}
		s.bindVersion(stack, id)
		return id
	}
	id := s.allocateVersion(stack, 0)
	if id == 0 {
		return 0
	}
	// Every non-initial version must enter through an authenticated copy seam.
	// Keep the event prefix readable, but fail the receipt closed when a seam is
	// missing instead of presenting an invented lineage as complete evidence.
	s.receipt.IdentityIncomplete = true
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventVersionAdd
	event.VersionID = id
	event.VersionIndex = s.versions[id].index
	event.NodeID = s.versionNodeID(id, stack)
	if action := s.currentAction; action != nil {
		s.applyActionContext(&event, action)
	}
	s.appendEvent(event)
	return id
}

func diagnosticTopologyEventBase() DiagnosticTopologyEvent {
	return DiagnosticTopologyEvent{ActionOrdinal: -1, ActionType: diagnosticTopologyNoActionType}
}

func (s *diagnosticTopologyState) applyActionContext(event *DiagnosticTopologyEvent, action *diagnosticTopologyAction) {
	if event == nil || action == nil {
		return
	}
	event.ActionID = action.id
	event.ActionOrdinal = action.ordinal
	event.ActionType = action.typeID
	event.Flags |= DiagnosticTopologyFlagActionContextKnown
}

func workCountTopologyRecordInitialVersion(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	id := s.allocateVersion(stack, 0)
	if id == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventVersionAdd
	event.VersionID = id
	event.VersionIndex = s.versions[id].index
	event.NodeID = s.versionNodeID(id, stack)
	event.Flags = DiagnosticTopologyFlagInitialVersion
	s.appendEvent(event)
}

func workCountTopologyPrepareVersionCopy(source, target *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || source == nil || target == nil {
		return
	}
	sourceID := s.versionID(source)
	targetID := s.allocateVersion(target, sourceID)
	if sourceID == 0 || targetID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	if previous, ok := s.pendingCopies[target]; ok && previous.targetVersionID != targetID {
		s.markIdentityCollision()
	}
	s.pendingCopies[target] = diagnosticTopologyPendingCopy{
		sourceVersionID: sourceID,
		sourceIndex:     s.versions[sourceID].index,
		targetVersionID: targetID,
		targetIndex:     s.versions[targetID].index,
	}
}

func workCountTopologyRecordVersionCopy(source, target *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || source == nil || target == nil {
		return
	}
	sourceID := s.versionID(source)
	kind := uint64(DiagnosticTopologyEventVersionCopy)
	if s.reductionActionSourceID != 0 {
		sourceID = s.reductionActionSourceID
		kind = DiagnosticTopologyEventVersionAdd
	}
	targetID := s.allocateVersion(target, sourceID)
	copy := diagnosticTopologyPendingCopy{
		sourceVersionID: sourceID,
		sourceIndex:     s.versions[sourceID].index,
		targetVersionID: targetID,
		targetIndex:     s.versions[targetID].index,
	}
	if kind == DiagnosticTopologyEventVersionCopy {
		s.recordVersionCopyEvent(target, copy, s.currentAction)
		return
	}
	if s.currentAction == nil || s.currentAction.versionID != sourceID {
		s.receipt.IdentityIncomplete = true
		return
	}
	s.actions[target] = *s.currentAction
	event := diagnosticTopologyEventBase()
	event.Kind = kind
	event.VersionID = copy.targetVersionID
	event.VersionIndex = copy.targetIndex
	event.SourceVersionID = copy.sourceVersionID
	event.SourceIndex = copy.sourceIndex
	event.NodeID = s.versionNodeID(copy.targetVersionID, target)
	s.applyActionContext(&event, s.currentAction)
	s.appendEvent(event)
}

func workCountTopologyCommitVersion(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	id := stack.diagnosticTopology.versionID
	if id == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	s.bindVersion(stack, id)
}

func (s *diagnosticTopologyState) recordVersionCopyEvent(target *glrStack, copy diagnosticTopologyPendingCopy, action *diagnosticTopologyAction) {
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventVersionCopy
	event.VersionID = copy.targetVersionID
	event.VersionIndex = copy.targetIndex
	event.SourceVersionID = copy.sourceVersionID
	event.SourceIndex = copy.sourceIndex
	event.NodeID = s.versionNodeID(copy.targetVersionID, target)
	s.applyActionContext(&event, action)
	s.appendEvent(event)
}

func workCountTopologyRecordAction(stack *glrStack, tok Token, action ParseAction, actionOrdinal int) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	workCountTopologyRecordActionResult(stack)
	ordinal := int64(actionOrdinal)
	if actionOrdinal < 0 {
		ordinal = -1
	}
	versionID := s.versionID(stack)
	if stack.gss.head == nil {
		s.syncVersionEntries(versionID, stack)
	}
	context := diagnosticTopologyAction{
		id:        s.nextID(&s.nextActionID),
		ordinal:   ordinal,
		typeID:    uint64(action.Type),
		state:     diagnosticTopologyStackState(stack),
		lookahead: uint64(tok.Symbol),
		byte:      uint64(stack.byteOffset),
		symbol:    uint64(action.Symbol),
		versionID: versionID,
		index:     s.versions[versionID].index,
	}
	if action.Type == ParseActionReduce {
		context.popID = s.nextID(&s.nextPopID)
		if context.popID == 0 {
			return
		}
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventAction
	event.ActionID = context.id
	event.ActionOrdinal = context.ordinal
	event.ActionType = context.typeID
	event.State = context.state
	event.LookaheadSymbol = context.lookahead
	event.ByteOffset = context.byte
	event.VersionID = versionID
	event.VersionIndex = context.index
	event.Flags = DiagnosticTopologyFlagActionContextKnown
	s.appendEvent(event)
	s.actions[stack] = context
	s.currentAction = &context
	if copy, ok := s.pendingCopies[stack]; ok {
		s.recordVersionCopyEvent(stack, copy, &context)
		delete(s.pendingCopies, stack)
	}
}

// workCountTopologyRecordNoActionPendingPop mirrors the iterator identity
// that C allocates when breakdown_top_of_stack checks for a pending subtree.
// The check allocates an identity even when it finds no pop path.
func workCountTopologyRecordNoActionPendingPop() {
	s := activeDiagnosticTopology
	if s == nil {
		return
	}
	if s.nextID(&s.nextPopID) == 0 {
		s.receipt.IdentityIncomplete = true
	}
}

func workCountTopologyRecordActionResult(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	context, ok := s.actions[stack]
	if !ok {
		return
	}
	s.bindVersion(stack, context.versionID)
	if context.typeID == uint64(ParseActionShift) && (s.parser == nil || !s.parser.compactPackedGSSVersionOrderEnabled()) {
		s.recordOmittedCShiftMergeAttempts(stack, context)
	}
	if stack.accepted {
		if context.typeID == uint64(ParseActionAccept) {
			s.recordAcceptAction(stack, context)
			delete(s.actions, stack)
			if s.currentAction != nil && s.currentAction.id == context.id {
				s.currentAction = nil
			}
			return
		}
		s.retireVersion(context.versionID)
		stack.diagnosticTopology.versionID = 0
	}
	delete(s.actions, stack)
	if s.currentAction != nil && s.currentAction.id == context.id {
		s.currentAction = nil
	}
}

func (s *diagnosticTopologyState) recordOmittedCShiftMergeAttempts(stack *glrStack, action diagnosticTopologyAction) {
	if s == nil || s.parser == nil || stack == nil || action.versionID == 0 {
		return
	}
	candidateVersion, ok := s.versions[action.versionID]
	if !ok || candidateVersion.index == 0 {
		return
	}
	subtreeCostRelevant := s.trackErrors == nil || *s.trackErrors
	candidateStatus := s.parser.cCondenseVersionStatus(stack, subtreeCostRelevant)
	for index := uint64(0); index < candidateVersion.index; index++ {
		if index >= uint64(len(s.versionSlots)) {
			s.receipt.IdentityIncomplete = true
			return
		}
		targetID := s.versionSlots[index]
		targetVersion, targetOK := s.versions[targetID]
		if !targetOK || targetID == 0 || targetVersion.snapshot.dead || targetVersion.snapshot.accepted ||
			stacksHeaderEquivalent(&targetVersion.snapshot, stack) {
			continue
		}
		targetStatus := s.parser.cCondenseVersionStatus(&targetVersion.snapshot, subtreeCostRelevant)
		comparison := s.parser.cCompareCondenseVersions(targetStatus, candidateStatus, &targetVersion.snapshot, stack)
		if comparison == cErrorComparisonTakeLeft || comparison == cErrorComparisonTakeRight {
			continue
		}
		savedAction := s.currentAction
		s.currentAction = nil
		workCountTopologyRecordMerge(&targetVersion.snapshot, stack, false)
		s.currentAction = savedAction
	}
}

func workCountTopologyBeginReductionVersion(source, target *glrStack, tok Token, action ParseAction, actionOrdinal int) {
	s := activeDiagnosticTopology
	if s == nil || source == nil || target == nil {
		return
	}
	workCountTopologyRecordAction(source, tok, action, actionOrdinal)
	context, ok := s.actions[source]
	if !ok || context.versionID == 0 || s.reductionActionSourceID != 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	if context.popID == 0 {
		context.popID = s.nextID(&s.nextPopID)
		if context.popID == 0 {
			return
		}
	}
	s.actions[source] = context
	s.currentAction = &context
	s.reductionActionSourceID = context.versionID
	targetID := s.allocateVersion(target, context.versionID)
	if targetID == 0 {
		return
	}
	s.reductionActionTargetID = targetID
	s.actions[target] = context
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventVersionAdd
	event.VersionID = targetID
	event.VersionIndex = s.versions[targetID].index
	event.SourceVersionID = context.versionID
	event.SourceIndex = context.index
	event.NodeID = s.versionNodeID(targetID, target)
	s.applyActionContext(&event, &context)
	s.appendEvent(event)
	s.reductionActionAddEventIndex = len(s.receipt.Events) - 1
}

func workCountTopologyEndReductionVersion(source, target *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || source == nil || target == nil {
		return
	}
	context, sourceOK := s.actions[source]
	targetContext, targetOK := s.actions[target]
	targetID := target.diagnosticTopology.versionID
	if sourceOK && targetOK && context.id == targetContext.id && targetID == s.reductionActionTargetID && !s.reductionActionAddRecorded {
		s.recordPendingReductionVersionAdd(target, context, s.versionNodeID(targetID, target))
	}
	if !sourceOK || !targetOK || context.id != targetContext.id || targetID == 0 ||
		targetID != s.reductionActionTargetID || !s.reductionActionAddRecorded {
		s.receipt.IdentityIncomplete = true
		return
	}
	if !targetContext.candidatePrepared && s.nextID(&s.nextCandidateID) == 0 {
		return
	}
	s.bindVersion(target, targetID)
	delete(s.actions, source)
	delete(s.actions, target)
	s.reductionActionSourceID = 0
	s.reductionActionTargetID = 0
	s.reductionActionAddRecorded = false
	s.reductionActionAddEventIndex = -1
}

func workCountTopologyFinishReductionAction() {
	if s := activeDiagnosticTopology; s != nil {
		s.currentAction = nil
	}
}

func workCountTopologyRecordEntryPush(stack *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil || stack.gss.head != nil || len(stack.entries) < 2 {
		return
	}
	if s.acceptSuppressNextLinkInsert {
		s.acceptSuppressNextLinkInsert = false
		return
	}
	versionID := s.versionID(stack)
	s.syncVersionEntries(versionID, stack)
	version, ok := s.versions[versionID]
	if !ok || len(version.entryNodeIDs) != len(stack.entries) {
		s.receipt.IdentityIncomplete = true
		return
	}
	linkID := s.nextID(&s.nextLinkID)
	if linkID == 0 {
		return
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventLinkInsert
	event.State = uint64(stack.entries[len(stack.entries)-1].state)
	event.NodeID = version.entryNodeIDs[len(version.entryNodeIDs)-1]
	event.PredecessorNodeID = version.entryNodeIDs[len(version.entryNodeIDs)-2]
	event.LinkID = linkID
	event.Flags = DiagnosticTopologyFlagPrimaryLink
	if action := s.currentAction; action != nil {
		s.applyActionContext(&event, action)
	}
	s.appendEvent(event)
}

func workCountTopologyRecordLinkInsert(node, predecessor *gssNode, ordinal int, primary bool) {
	s := activeDiagnosticTopology
	if s == nil || node == nil || predecessor == nil || ordinal < 0 {
		return
	}
	if s.acceptSuppressNextLinkInsert {
		if _, exists := s.nodeIDs[node]; !exists && s.acceptEOFNodeID != 0 {
			s.nodeIDs[node] = s.acceptEOFNodeID
		}
		s.acceptSuppressNextLinkInsert = false
		return
	}
	if s.promotion.versionID != 0 {
		return
	}
	nodeID := s.nodeID(node)
	predecessorID := s.nodeID(predecessor)
	key := diagnosticTopologyLinkKey{nodeID: nodeID, ordinal: uint64(ordinal)}
	linkID := s.linkIDs[key]
	if linkID != 0 {
		if s.linkPrevIDs[key] != predecessorID {
			s.markIdentityCollision()
		}
		return
	}
	if linkID == 0 {
		if !s.identityAvailable(len(s.linkIDs)) {
			return
		}
		linkID = s.nextID(&s.nextLinkID)
		if linkID == 0 {
			return
		}
		s.linkIDs[key] = linkID
		s.linkPrevIDs[key] = predecessorID
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventLinkInsert
	event.State = uint64(node.entry.state)
	event.NodeID = nodeID
	event.PredecessorNodeID = predecessorID
	event.LinkID = linkID
	event.LinkOrdinal = uint64(ordinal)
	if primary {
		event.Flags |= DiagnosticTopologyFlagPrimaryLink
	}
	if action := s.currentAction; action != nil {
		s.applyActionContext(&event, action)
	}
	s.appendEvent(event)
}

func diagnosticTopologyPayloadCount(window []stackEntry) uint64 {
	var payloads uint64
	for i := range window {
		if stackEntryHasNode(window[i]) {
			if payloads == math.MaxUint64 {
				return math.MaxUint64
			}
			payloads++
		}
	}
	return payloads
}

func workCountTopologyRecordPopPath(stack *glrStack, window []stackEntry, popTo *gssNode, pathOrdinal uint64) {
	if s := activeDiagnosticTopology; s != nil && stack != nil {
		if action, ok := s.actions[stack]; ok && action.directPop {
			return
		}
	}
	workCountTopologyRecordPopPathWithNodeID(stack, window, popTo, 0, pathOrdinal)
}

func (s *diagnosticTopologyState) recordPendingReductionVersionAdd(stack *glrStack, action diagnosticTopologyAction, popToNodeID uint64) {
	if s == nil || stack == nil || popToNodeID == 0 || s.reductionActionSourceID == 0 {
		return
	}
	versionID := stack.diagnosticTopology.versionID
	if versionID == 0 || action.versionID != s.reductionActionSourceID {
		s.receipt.IdentityIncomplete = true
		return
	}
	version, ok := s.versions[versionID]
	if !ok {
		s.receipt.IdentityIncomplete = true
		return
	}
	eventIndex := -1
	for i := len(s.receipt.Events) - 1; i >= 0; i-- {
		event := s.receipt.Events[i]
		if event.Kind == DiagnosticTopologyEventVersionAdd && event.VersionID == versionID && event.SourceVersionID == action.versionID {
			eventIndex = i
			break
		}
	}
	if eventIndex < 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	version.nodeID = popToNodeID
	s.versions[versionID] = version
	event := &s.receipt.Events[eventIndex]
	if event.Kind != DiagnosticTopologyEventVersionAdd || event.VersionID != versionID || event.SourceVersionID != action.versionID {
		s.receipt.IdentityIncomplete = true
		return
	}
	event.NodeID = popToNodeID
	if versionID == s.reductionActionTargetID {
		s.reductionActionAddRecorded = true
	}
}

func workCountTopologyRecordPopPathWithNodeID(stack *glrStack, window []stackEntry, popTo *gssNode, popToNodeID, pathOrdinal uint64) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil {
		return
	}
	action, ok := s.actions[stack]
	if !ok {
		s.receipt.IdentityIncomplete = true
		return
	}
	if popToNodeID == 0 {
		popToNodeID = s.nodeID(popTo)
	}
	s.recordPendingReductionVersionAdd(stack, action, popToNodeID)
	if action.popID == 0 {
		action.popID = s.nextID(&s.nextPopID)
		s.actions[stack] = action
		if s.currentAction != nil && s.currentAction.id == action.id {
			s.currentAction = &action
		}
	}
	if len(s.candidates) < diagnosticTopologyIdentityCapacity {
		windowCopy := append([]stackEntry(nil), window...)
		topState := StateID(0)
		if popTo != nil {
			topState = popTo.entry.state
		}
		s.candidates = append(s.candidates, diagnosticTopologyCandidate{
			popID: action.popID, ordinal: pathOrdinal,
			popTo: popTo, topState: topState, window: windowCopy,
		})
	} else {
		s.receipt.IdentityIncomplete = true
	}
	payloads := diagnosticTopologyPayloadCount(window)
	if payloads == math.MaxUint64 {
		s.receipt.ArithmeticOverflow = true
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventPopPath
	s.applyActionContext(&event, &action)
	versionID := stack.diagnosticTopology.versionID
	version, versionOK := s.versions[versionID]
	if !versionOK || versionID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	event.VersionID = versionID
	event.VersionIndex = version.index
	event.SourceVersionID = action.versionID
	event.SourceIndex = action.index
	event.NodeID = popToNodeID
	event.PopID = action.popID
	event.PathOrdinal = pathOrdinal
	event.PopToNodeID = popToNodeID
	if event.PopToNodeID == 0 {
		s.receipt.IdentityIncomplete = true
	}
	event.PayloadCount = payloads
	s.appendEvent(event)
}

func workCountTopologyRecordDirectPop(stack *glrStack, childCount int) {
	if activeDiagnosticTopology == nil || stack == nil || childCount < 0 {
		return
	}
	if childCount == 0 {
		if stack.entries != nil && stack.gss.head == nil {
			action, ok := activeDiagnosticTopology.actions[stack]
			if !ok {
				activeDiagnosticTopology.receipt.IdentityIncomplete = true
				return
			}
			version := activeDiagnosticTopology.versions[action.versionID]
			if len(version.entryNodeIDs) != len(stack.entries) || len(version.entryNodeIDs) == 0 {
				activeDiagnosticTopology.receipt.IdentityIncomplete = true
				return
			}
			workCountTopologyRecordPopPathWithNodeID(stack, nil, nil, version.entryNodeIDs[len(version.entryNodeIDs)-1], 0)
			activeDiagnosticTopology.markDirectPop(stack)
			return
		}
		workCountTopologyRecordPopPath(stack, nil, stack.gss.head, 0)
		activeDiagnosticTopology.markDirectPop(stack)
		return
	}
	if stack.entries != nil {
		action, ok := activeDiagnosticTopology.actions[stack]
		if !ok {
			activeDiagnosticTopology.receipt.IdentityIncomplete = true
			return
		}
		version := activeDiagnosticTopology.versions[action.versionID]
		if len(version.entryNodeIDs) != len(stack.entries) {
			activeDiagnosticTopology.receipt.IdentityIncomplete = true
			return
		}
		remaining := childCount
		start := len(stack.entries)
		for i := len(stack.entries) - 1; i >= 0; i-- {
			entry := stack.entries[i]
			if !stackEntryHasNode(entry) {
				continue
			}
			start = i
			if !stackEntryNodeIsExtra(entry) {
				remaining--
				if remaining == 0 {
					popToNodeID := version.emptyNodeID
					if start > 0 {
						popToNodeID = version.entryNodeIDs[start-1]
					}
					workCountTopologyRecordPopPathWithNodeID(stack, stack.entries[start:], nil, popToNodeID, 0)
					activeDiagnosticTopology.markDirectPop(stack)
					return
				}
			}
		}
		return
	}
	if stack.gss.head == nil || !gssSpanIsLinear(stack.gss.head, childCount) {
		return
	}
	remaining := childCount
	window := make([]stackEntry, 0, childCount)
	var popTo *gssNode
	for node := stack.gss.head; node != nil; node = node.prev {
		entry := node.entry
		if !stackEntryHasNode(entry) {
			continue
		}
		window = append(window, entry)
		if !stackEntryNodeIsExtra(entry) {
			remaining--
			if remaining == 0 {
				popTo = node.prev
				break
			}
		}
	}
	if remaining == 0 {
		for left, right := 0, len(window)-1; left < right; left, right = left+1, right-1 {
			window[left], window[right] = window[right], window[left]
		}
		workCountTopologyRecordPopPath(stack, window, popTo, 0)
		activeDiagnosticTopology.markDirectPop(stack)
	}
}

func workCountTopologyRecordPackedReduceGroup(p *Parser, arena *nodeArena, stack *glrStack, act ParseAction, forks []reduceFork, pathOrdinal uint64) {
	s := activeDiagnosticTopology
	if s == nil || p == nil || stack == nil || len(forks) == 0 {
		return
	}
	for i := range forks {
		workCountTopologyRecordPopPathWithNodeID(stack, forks[i].window, forks[i].popTo, 0, pathOrdinal+uint64(i))
	}
	incumbent := forks[0]
	incumbentID := s.reduceCandidateID(incumbent)
	action := s.actions[stack]
	action.candidatePrepared = true
	s.actions[stack] = action
	if s.currentAction != nil && s.currentAction.id == action.id {
		s.currentAction = &action
	}
	for i := 1; i < len(forks); i++ {
		candidate := forks[i]
		candidateID := s.reduceCandidateID(candidate)
		preference := p.reduceForkWindowPreference(arena, act, candidate, incumbent)
		s.recordPackedChildElection(stack, incumbentID, candidateID, diagnosticTopologyPayloadCount(candidate.window), preference < 0)
		if preference < 0 {
			incumbent = candidate
			incumbentID = candidateID
		}
	}
	if stack.diagnosticTopology.versionID != s.reductionActionTargetID {
		delete(s.actions, stack)
	}
}

func (s *diagnosticTopologyState) markDirectPop(stack *glrStack) {
	if s == nil || stack == nil {
		return
	}
	action, ok := s.actions[stack]
	if !ok {
		s.receipt.IdentityIncomplete = true
		return
	}
	action.directPop = true
	s.actions[stack] = action
	if s.currentAction != nil && s.currentAction.id == action.id {
		s.currentAction = &action
	}
}

func diagnosticTopologyWindowsEqual(a, b []stackEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *diagnosticTopologyState) reduceCandidateID(fork reduceFork) uint64 {
	for i := len(s.candidates) - 1; i >= 0; i-- {
		candidate := &s.candidates[i]
		if candidate.popTo == fork.popTo && candidate.topState == fork.topState &&
			diagnosticTopologyWindowsEqual(candidate.window, fork.window) {
			if candidate.id == 0 {
				candidate.id = s.nextID(&s.nextCandidateID)
			}
			return candidate.id
		}
	}
	s.receipt.IdentityIncomplete = true
	return 0
}

func workCountTopologyRecordChildElection(stack *glrStack, incumbent, candidate reduceFork, preference int) {
	s := activeDiagnosticTopology
	if s == nil || stack == nil || (preference != -1 && preference != 1) {
		return
	}
	action, ok := s.actions[stack]
	if !ok {
		s.receipt.IdentityIncomplete = true
		return
	}
	incumbentID := s.reduceCandidateID(incumbent)
	candidateID := s.reduceCandidateID(candidate)
	if incumbentID == 0 || candidateID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	selectedID := uint64(0)
	flags := uint64(0)
	switch preference {
	case -1:
		selectedID = candidateID
		flags = DiagnosticTopologyFlagSuccessOrSelected
	case 1:
		selectedID = incumbentID
	}
	electionID := s.nextID(&s.nextElectionID)
	if electionID == 0 {
		return
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventChildElection
	s.applyActionContext(&event, &action)
	event.State = action.symbol
	event.VersionID = action.versionID
	event.VersionIndex = action.index
	event.ElectionID = electionID
	event.IncumbentID = incumbentID
	event.CandidateID = candidateID
	event.SelectedID = selectedID
	event.PayloadCount = diagnosticTopologyPayloadCount(candidate.window)
	event.Flags |= flags
	s.appendEvent(event)
}

func (s *diagnosticTopologyState) recordPackedChildElection(stack *glrStack, incumbentID, candidateID, payloadCount uint64, candidateSelected bool) {
	if s == nil || stack == nil || incumbentID == 0 || candidateID == 0 {
		if s != nil {
			s.receipt.IdentityIncomplete = true
		}
		return
	}
	action, ok := s.actions[stack]
	versionID := stack.diagnosticTopology.versionID
	version, versionOK := s.versions[versionID]
	if !ok || !versionOK || versionID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	electionID := s.nextID(&s.nextElectionID)
	if electionID == 0 {
		return
	}
	selectedID := incumbentID
	flags := uint64(0)
	if candidateSelected {
		selectedID = candidateID
		flags = DiagnosticTopologyFlagSuccessOrSelected
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventChildElection
	s.applyActionContext(&event, &action)
	event.State = action.symbol
	event.VersionID = versionID
	event.VersionIndex = version.index
	event.ElectionID = electionID
	event.IncumbentID = incumbentID
	event.CandidateID = candidateID
	event.SelectedID = selectedID
	event.PayloadCount = payloadCount
	event.Flags |= flags
	s.appendEvent(event)
}

func workCountTopologyRecordMerge(target, candidate *glrStack, merged bool) {
	workCountTopologyRecordMergeWithRetire(target, candidate, merged, merged)
}

func workCountTopologyRecordMergeBeforeMutation(target, candidate *glrStack) {
	workCountTopologyRecordMergeWithRetire(target, candidate, true, false)
}

func workCountTopologyRecordMergeWithRetire(target, candidate *glrStack, merged, retire bool) {
	s := activeDiagnosticTopology
	if s == nil || target == nil || candidate == nil {
		return
	}
	targetID := s.versionID(target)
	candidateID := s.versionID(candidate)
	key := diagnosticTopologyMergeKey{targetID: targetID, candidateID: candidateID}
	if !merged && s.preRecordedMerges[key] {
		delete(s.preRecordedMerges, key)
		return
	}
	targetVersion, targetOK := s.versions[targetID]
	candidateVersion, candidateOK := s.versions[candidateID]
	if !targetOK || !candidateOK || targetID == 0 || candidateID == 0 || targetID == candidateID || targetVersion.index >= candidateVersion.index {
		s.receipt.IdentityIncomplete = true
		return
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventMerge
	event.VersionID = targetID
	event.VersionIndex = targetVersion.index
	event.SourceVersionID = targetID
	event.SourceIndex = targetVersion.index
	event.TargetVersionID = candidateID
	event.TargetIndex = candidateVersion.index
	event.SurvivorVersionID = targetID
	event.RemovedVersionID = candidateID
	event.NodeID = s.versionNodeID(targetID, target)
	event.PredecessorNodeID = s.versionNodeID(candidateID, candidate)
	if merged {
		event.Flags |= DiagnosticTopologyFlagSuccessOrSelected
		s.bindVersion(target, targetID)
	}
	if action := s.currentAction; action != nil {
		s.applyActionContext(&event, action)
	}
	s.appendEvent(event)
	if retire {
		s.retireVersion(candidateID)
	}
}

func workCountTopologyRecordPackedReductionMergeAttempts(p *Parser, candidate *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || p == nil || candidate == nil || s.reductionActionSourceID == 0 {
		return
	}
	candidateID := candidate.diagnosticTopology.versionID
	candidateVersion, ok := s.versions[candidateID]
	if !ok || candidateID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	for index := uint64(0); index < candidateVersion.index; index++ {
		if index >= uint64(len(s.versionSlots)) {
			s.receipt.IdentityIncomplete = true
			return
		}
		targetID := s.versionSlots[index]
		if targetID == s.reductionActionSourceID {
			continue
		}
		targetVersion, targetOK := s.versions[targetID]
		if !targetOK || targetID == 0 {
			s.receipt.IdentityIncomplete = true
			return
		}
		key := diagnosticTopologyMergeKey{targetID: targetID, candidateID: candidateID}
		if s.preRecordedMerges[key] {
			continue
		}
		if gssMainCanMergeForParser(p, &targetVersion.snapshot, candidate) {
			// The parser performs this successful merge through its physical
			// packed-output target. Stop before that mutation records the event.
			return
		}
		workCountTopologyRecordMerge(&targetVersion.snapshot, candidate, false)
		s.preRecordedMerges[key] = true
	}
}

func workCountTopologyCommitMerge(candidate *glrStack) {
	s := activeDiagnosticTopology
	if s == nil || candidate == nil {
		return
	}
	id := candidate.diagnosticTopology.versionID
	if id == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	s.retireVersion(id)
	candidate.diagnosticTopology.versionID = 0
}

func workCountTopologyClearVersion(stack *glrStack) {
	if stack != nil {
		stack.diagnosticTopology.versionID = 0
	}
}

func workCountTopologyRequireMergeSuccess(merged bool) {
	if s := activeDiagnosticTopology; s != nil && !merged {
		s.receipt.IdentityIncomplete = true
	}
}

type diagnosticTopologyAcceptCandidate struct {
	action       diagnosticTopologyAction
	versionID    uint64
	stack        glrStack
	payloadCount uint64
	candidateID  uint64
}

func diagnosticTopologySnapshotAcceptStack(stack glrStack) glrStack {
	entries, _ := stack.entriesForRead(nil)
	stack.entries = append([]stackEntry(nil), entries...)
	stack.gss = gssStack{}
	stack.cacheEntries = true
	return stack
}

func diagnosticTopologyAcceptPayloadCount(stack *glrStack) uint64 {
	if stack == nil {
		return 0
	}
	entries, _ := stack.entriesForRead(nil)
	payloads := 0
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !stackEntryHasNode(entry) {
			continue
		}
		payloads++
	}
	// C pushes the EOF subtree before pop_all, so it contributes one payload.
	return uint64(payloads + 1)
}

func (s *diagnosticTopologyState) acceptPopToNodeID(stack *glrStack, sourceVersion diagnosticTopologyVersion) uint64 {
	if stack != nil && stack.gss.head != nil {
		node := stack.gss.head
		for node.prev != nil {
			node = node.prev
		}
		return s.nodeID(node)
	}
	if len(sourceVersion.entryNodeIDs) != 0 {
		return sourceVersion.entryNodeIDs[0]
	}
	return sourceVersion.emptyNodeID
}

func (s *diagnosticTopologyState) recordAcceptEOFPush(stack *glrStack, action diagnosticTopologyAction) {
	predecessorNodeID := s.versionNodeID(action.versionID, stack)
	nodeID := s.nextNodeIdentity()
	linkID := s.nextID(&s.nextLinkID)
	if predecessorNodeID == 0 || nodeID == 0 || linkID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventLinkInsert
	s.applyActionContext(&event, &action)
	event.State = 1
	event.NodeID = nodeID
	event.PredecessorNodeID = predecessorNodeID
	event.LinkID = linkID
	event.Flags |= DiagnosticTopologyFlagPrimaryLink
	s.appendEvent(event)
	s.acceptEOFNodeID = nodeID
	s.acceptSuppressNextLinkInsert = true
}

func (s *diagnosticTopologyState) recordAcceptPopPath(stack *glrStack, action diagnosticTopologyAction, sourceVersion diagnosticTopologyVersion, popID, pathOrdinal, payloadCount uint64) uint64 {
	popToNodeID := s.acceptPopToNodeID(stack, sourceVersion)
	versionID := s.nextID(&s.nextVersionID)
	if popToNodeID == 0 || versionID == 0 || popID == 0 {
		s.receipt.IdentityIncomplete = true
		return 0
	}
	version := diagnosticTopologyVersion{
		id:       versionID,
		index:    uint64(len(s.versionSlots)),
		nodeID:   popToNodeID,
		snapshot: diagnosticTopologySnapshotAcceptStack(*stack),
	}
	s.versions[versionID] = version
	s.versionSlots = append(s.versionSlots, versionID)

	add := diagnosticTopologyEventBase()
	add.Kind = DiagnosticTopologyEventVersionAdd
	s.applyActionContext(&add, &action)
	add.VersionID = versionID
	add.VersionIndex = version.index
	add.SourceVersionID = action.versionID
	add.SourceIndex = action.index
	add.NodeID = popToNodeID
	s.appendEvent(add)

	pop := diagnosticTopologyEventBase()
	pop.Kind = DiagnosticTopologyEventPopPath
	s.applyActionContext(&pop, &action)
	pop.VersionID = versionID
	pop.VersionIndex = version.index
	pop.SourceVersionID = action.versionID
	pop.SourceIndex = action.index
	pop.NodeID = popToNodeID
	pop.PopID = popID
	pop.PathOrdinal = pathOrdinal
	pop.PopToNodeID = popToNodeID
	pop.PayloadCount = payloadCount
	s.appendEvent(pop)
	return versionID
}

// detachAcceptVersions mirrors C's removal of an accepted version from the
// active version pool. C does not publish renumber events for this removal.
func (s *diagnosticTopologyState) detachAcceptVersions(versionIDs []uint64) {
	if len(versionIDs) == 0 {
		return
	}
	detached := make(map[uint64]struct{}, len(versionIDs))
	for _, id := range versionIDs {
		if id == 0 {
			s.receipt.IdentityIncomplete = true
			return
		}
		detached[id] = struct{}{}
	}
	kept := s.versionSlots[:0]
	for _, id := range s.versionSlots {
		if _, ok := detached[id]; ok {
			delete(s.versions, id)
			continue
		}
		version, ok := s.versions[id]
		if !ok {
			s.receipt.IdentityIncomplete = true
			return
		}
		version.index = uint64(len(kept))
		s.versions[id] = version
		kept = append(kept, id)
	}
	if len(kept)+len(detached) != len(s.versionSlots) {
		s.receipt.IdentityIncomplete = true
		return
	}
	s.versionSlots = kept
}

func (s *diagnosticTopologyState) recordAcceptAction(stack *glrStack, action diagnosticTopologyAction) {
	if stack == nil || action.id == 0 || action.typeID != uint64(ParseActionAccept) || action.versionID == 0 {
		s.receipt.IdentityIncomplete = true
		return
	}
	if _, ok := s.versions[action.versionID]; !ok {
		s.receipt.IdentityIncomplete = true
		return
	}
	s.recordAcceptEOFPush(stack, action)
	paths := expandPackedGSSResultPaths([]glrStack{*stack})
	popID := s.nextID(&s.nextPopID)
	sourceVersion := s.versions[action.versionID]
	skipErrorRank := false
	if s.trackErrors != nil {
		skipErrorRank = !*s.trackErrors
	}
	candidates := make([]diagnosticTopologyAcceptCandidate, 0, len(paths))
	acceptedVersionIDs := make([]uint64, 0, len(paths)+1)
	acceptedVersionIDs = append(acceptedVersionIDs, action.versionID)
	for i := range paths {
		snapshot := diagnosticTopologySnapshotAcceptStack(paths[i])
		candidate := diagnosticTopologyAcceptCandidate{
			action:       action,
			versionID:    action.versionID,
			stack:        snapshot,
			payloadCount: diagnosticTopologyAcceptPayloadCount(&snapshot),
			candidateID:  s.nextID(&s.nextCandidateID),
		}
		if candidate.candidateID == 0 {
			return
		}
		acceptedVersionIDs = append(acceptedVersionIDs, s.recordAcceptPopPath(&paths[i], action, sourceVersion, popID, uint64(i), candidate.payloadCount))
		candidates = append(candidates, candidate)
	}
	for i := range candidates {
		candidate := candidates[i]
		selected := !s.acceptSet || stackCompareForResultSelection(
			s.parser, s.arena, &candidate.stack, &s.acceptCurrent.stack, skipErrorRank,
		) > 0
		s.recordAcceptElection(candidate, selected)
		if selected {
			s.acceptCurrent = candidate
			s.acceptSet = true
		}
	}
	s.detachAcceptVersions(acceptedVersionIDs)
	stack.diagnosticTopology.versionID = 0
}

func (s *diagnosticTopologyState) recordAcceptElection(candidate diagnosticTopologyAcceptCandidate, candidateSelected bool) {
	incumbentID := uint64(0)
	if s.acceptSet {
		incumbentID = s.acceptCurrent.candidateID
	}
	selectedID := incumbentID
	if candidateSelected {
		selectedID = candidate.candidateID
	}
	if candidate.candidateID == 0 || selectedID == 0 || (s.acceptSet && incumbentID == 0) {
		s.receipt.IdentityIncomplete = true
		return
	}
	electionID := s.nextID(&s.nextElectionID)
	if electionID == 0 {
		return
	}
	event := diagnosticTopologyEventBase()
	event.Kind = DiagnosticTopologyEventAcceptElection
	s.applyActionContext(&event, &candidate.action)
	event.VersionID = candidate.versionID
	event.VersionIndex = candidate.action.index
	event.ElectionID = electionID
	event.IncumbentID = incumbentID
	event.CandidateID = candidate.candidateID
	event.SelectedID = selectedID
	event.PayloadCount = candidate.payloadCount
	if !s.acceptSet {
		event.Flags |= DiagnosticTopologyFlagNoIncumbent
	}
	if candidateSelected {
		event.Flags |= DiagnosticTopologyFlagSuccessOrSelected
	}
	s.appendEvent(event)
}

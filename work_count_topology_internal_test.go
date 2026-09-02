//go:build gts_workcount

package gotreesitter

import (
	"strings"
	"testing"
	"unsafe"
)

func TestDiagnosticTopologyReceiptBoundsChronologicalPrefix(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	for i := 0; i < DiagnosticTopologyReceiptCapacity+3; i++ {
		event := diagnosticTopologyEventBase()
		event.Kind = DiagnosticTopologyEventAction
		event.State = uint64(i)
		activeDiagnosticTopology.appendEvent(event)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if receipt.EventsSeen != DiagnosticTopologyReceiptCapacity+3 ||
		receipt.EventsRetained != DiagnosticTopologyReceiptCapacity ||
		receipt.EventsDropped != 3 || !receipt.Truncated || receipt.Complete() {
		t.Fatalf("bounded receipt = %+v", receipt)
	}
	if first, last := receipt.Events[0], receipt.Events[len(receipt.Events)-1]; first.EventID != 1 || first.State != 0 || last.EventID != DiagnosticTopologyReceiptCapacity || last.State != DiagnosticTopologyReceiptCapacity-1 {
		t.Fatalf("retained prefix endpoints = %+v / %+v", first, last)
	}
	firstDigest, secondDigest := receipt.SHA256(), receipt.SHA256()
	if firstDigest == "" || firstDigest != secondDigest {
		t.Fatal("receipt digest is empty or unstable")
	}
}

func TestDiagnosticTopologyMixedBoundaryMergeKeepsLogicalSurvivor(t *testing.T) {
	BeginDiagnosticWorkCount()
	BeginDiagnosticTopologyReceipt()
	token := workCountBeginParseAttempt(6, 1024, 6)
	if token == 0 {
		t.Fatal("zero parse-attempt token")
	}

	node := NewLeafNode(11, true, 0, 5, Point{}, Point{Column: 5})
	entries := []stackEntry{{state: 1}, newStackEntryNode(7, node)}
	incumbent := glrStack{entries: append([]stackEntry(nil), entries...), byteOffset: 5}
	candidate := glrStack{entries: append([]stackEntry(nil), entries...), byteOffset: 5}
	workCountTopologyRecordInitialVersion(&incumbent)
	workCountTopologyRecordVersionCopy(&incumbent, &candidate)
	incumbentID := incumbent.diagnosticTopology.versionID
	candidateID := candidate.diagnosticTopology.versionID
	if incumbentID == 0 || candidateID == 0 || incumbentID >= candidateID {
		t.Fatalf("logical version IDs = %d/%d, want incumbent before candidate", incumbentID, candidateID)
	}

	var owner gssScratch
	candidate.ensureGSS(&owner)
	candidate.entries = nil
	candidate.cacheEntries = false
	if candidate.gss.head == nil || candidate.entries != nil {
		t.Fatalf("candidate did not enter the packed representation: head=%p entries=%d", candidate.gss.head, len(candidate.entries))
	}
	ownerUsedBefore := owner.usedTotal
	graphLinksBefore := activeDiagnosticWorkCount.GraphLinkAdditionsProxy

	scratch := glrMergeScratch{gssOwner: &owner}
	scratch.beginEquivEpoch()
	result := []glrStack{incumbent}
	merged, attempted := tryGSSMainMergeResult(&scratch, result, 0, &candidate)
	if !attempted || !merged {
		t.Fatalf("mixed boundary merge attempted=%v merged=%v, want true/true", attempted, merged)
	}
	if got := result[0].diagnosticTopology.versionID; got != incumbentID {
		t.Fatalf("physical receiver replaced logical survivor: result version=%d, want %d", got, incumbentID)
	}
	if candidate.diagnosticTopology.versionID != 0 {
		t.Fatalf("absorbed candidate retained a retired stack token: version=%d", candidate.diagnosticTopology.versionID)
	}
	if _, ok := activeDiagnosticTopology.versions[candidateID]; ok {
		t.Fatalf("absorbed candidate version %d remains active", candidateID)
	}
	if _, ok := activeDiagnosticTopology.versions[incumbentID]; !ok {
		t.Fatalf("logical incumbent version %d was retired", incumbentID)
	}
	if got := activeDiagnosticTopology.versions[incumbentID].head; got != result[0].gss.head {
		t.Fatalf("logical incumbent version head=%p, want physical receiver=%p", got, result[0].gss.head)
	}
	if got, want := owner.usedTotal, ownerUsedBefore+len(entries); got != want {
		t.Fatalf("successful mixed promotion used=%d GSS nodes, want one flat-depth allocation: %d", got, want)
	}

	workCountResolveParseAttempt(token, 6, false, 6, 6, 1024, 1024)
	workCountBeginFinalizeParseAttempt(token)
	workCountEndFinalizeParseAttempt(token, ParseStopAccepted, nil)
	counts := EndDiagnosticWorkCount()
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("mixed boundary topology receipt is incomplete: %+v", receipt)
	}
	var mergeEvent *DiagnosticTopologyEvent
	for i := range receipt.Events {
		event := &receipt.Events[i]
		if event.Kind == DiagnosticTopologyEventMerge {
			mergeEvent = event
			break
		}
	}
	if mergeEvent == nil || mergeEvent.SourceVersionID != incumbentID || mergeEvent.TargetVersionID != candidateID || mergeEvent.RemovedVersionID != candidateID || mergeEvent.SurvivorVersionID != incumbentID {
		t.Fatalf("mixed boundary logical merge event = %+v, want incumbent %d absorbing candidate %d", mergeEvent, incumbentID, candidateID)
	}
	foundMixedTelemetry := false
	for _, event := range counts.Convergence.Events {
		if strings.Contains(event.Detail, "boundary mixed-representation merge") && event.Outcome == workCountConvergenceOutcomePacked {
			foundMixedTelemetry = true
			break
		}
	}
	if !foundMixedTelemetry {
		t.Fatalf("mixed boundary convergence telemetry missing: %+v", counts.Convergence.Events)
	}
	if got, want := counts.GraphLinkAdditionsProxy-graphLinksBefore, uint64(1); got != want {
		t.Fatalf("successful mixed promotion graph-link work=%d, want one promotion link", got)
	}
}

func TestDiagnosticTopologyMixedDistinctShapeRejectDoesNotPromote(t *testing.T) {
	BeginDiagnosticWorkCount()
	BeginDiagnosticTopologyReceipt()
	finished := false
	defer func() {
		if finished {
			return
		}
		if activeDiagnosticWorkCount != nil {
			workCountResolveParseAttempt(1, 2, false, 2, 2, 1, 1024)
			workCountBeginFinalizeParseAttempt(1)
			workCountEndFinalizeParseAttempt(1, ParseStopAccepted, nil)
			_ = EndDiagnosticWorkCount()
		}
		if activeDiagnosticTopology != nil {
			_ = EndDiagnosticTopologyReceipt()
		}
	}()
	token := workCountBeginParseAttempt(2, 1024, 2)
	if token == 0 {
		t.Fatal("zero parse-attempt token")
	}

	arena := newNodeArena(arenaClassFull)
	parser := &Parser{}
	makeShape := func(childSymbol Symbol) *Node {
		child := NewLeafNode(childSymbol, true, 0, 5, Point{}, Point{Column: 5})
		parent := NewParentNode(300, true, []*Node{child}, nil, 0)
		parent.parseState = 7
		parent.rawShape = parser.captureRawShape(
			nil,
			arena,
			parent.symbol,
			parent.productionID,
			[]stackEntry{newStackEntryNode(parent.parseState, child)},
			0,
			1,
		)
		return parent
	}
	incumbentNode := makeShape(11)
	candidateNode := makeShape(12)
	incumbentEntries := []stackEntry{{state: 1}, newStackEntryNode(7, incumbentNode)}
	candidateEntries := []stackEntry{{state: 2}, newStackEntryNode(7, candidateNode)}
	incumbent := glrStack{entries: incumbentEntries, byteOffset: 5, branchOrder: 11}
	candidate := glrStack{entries: candidateEntries, byteOffset: 5, branchOrder: 22}
	workCountTopologyRecordInitialVersion(&incumbent)
	workCountTopologyRecordVersionCopy(&incumbent, &candidate)
	incumbentID := incumbent.diagnosticTopology.versionID
	candidateID := candidate.diagnosticTopology.versionID
	if incumbentID == 0 || candidateID == 0 {
		t.Fatalf("logical version IDs = %d/%d, want nonzero IDs", incumbentID, candidateID)
	}

	var owner gssScratch
	candidate.ensureGSS(&owner)
	candidate.entries = nil
	candidate.cacheEntries = false
	candidateHead := candidate.gss.head
	if candidateHead == nil {
		t.Fatal("candidate did not enter the packed representation")
	}
	nodeCountBefore := len(activeDiagnosticTopology.nodeIDs)
	ownerUsedBefore := owner.usedTotal
	versionHeadBefore := activeDiagnosticTopology.versions[candidateID].head
	promotionBefore := activeDiagnosticTopology.promotion
	if promotionBefore.versionID != 0 {
		t.Fatalf("candidate promotion remained staged: %+v", promotionBefore)
	}

	lang := &Language{ExactStackNodeEquivalenceCertified: true}
	scratch := glrMergeScratch{gssOwner: &owner, language: lang, arena: arena}
	scratch.beginEquivEpoch()
	result := []glrStack{incumbent}
	merged, attempted := tryGSSMainMergeResult(&scratch, result, 0, &candidate)
	if merged || !attempted {
		t.Fatalf("distinct mixed shapes merge=%v attempted=%v, want false/true", merged, attempted)
	}
	if result[0].gss.head != nil || len(result[0].entries) != len(incumbentEntries) {
		t.Fatalf("flat incumbent representation changed: head=%p entries=%d", result[0].gss.head, len(result[0].entries))
	}
	for i := range incumbentEntries {
		if result[0].entries[i] != incumbentEntries[i] {
			t.Fatalf("flat incumbent entry %d changed", i)
		}
	}
	if candidate.gss.head != candidateHead || candidateHead.linkCount() != 1 {
		t.Fatalf("packed candidate changed: head=%p links=%d", candidate.gss.head, candidateHead.linkCount())
	}
	if owner.usedTotal != ownerUsedBefore {
		t.Fatalf("rejected mixed staging consumed GSS scratch nodes: used=%d/%d", owner.usedTotal, ownerUsedBefore)
	}
	if len(activeDiagnosticTopology.nodeIDs) != nodeCountBefore || activeDiagnosticTopology.promotion.versionID != 0 {
		t.Fatalf("rejected mixed staging changed topology allocation state: nodes=%d/%d promotion=%+v", len(activeDiagnosticTopology.nodeIDs), nodeCountBefore, activeDiagnosticTopology.promotion)
	}
	if activeDiagnosticTopology.versions[candidateID].head != versionHeadBefore {
		t.Fatal("rejected mixed staging rebound the candidate version head")
	}
	if incumbent.diagnosticTopology.versionID != incumbentID || candidate.diagnosticTopology.versionID != candidateID || result[0].diagnosticTopology.versionID != incumbentID {
		t.Fatalf("rejected mixed staging changed version identities: incumbent=%d candidate=%d result=%d", incumbent.diagnosticTopology.versionID, candidate.diagnosticTopology.versionID, result[0].diagnosticTopology.versionID)
	}
	if activeDiagnosticTopology.receipt.IdentityIncomplete || activeDiagnosticTopology.receipt.IdentityCollision {
		t.Fatalf("rejected mixed staging damaged topology identity: incomplete=%v collision=%v", activeDiagnosticTopology.receipt.IdentityIncomplete, activeDiagnosticTopology.receipt.IdentityCollision)
	}

	workCountResolveParseAttempt(token, 2, false, 2, 2, 1, 1024)
	workCountBeginFinalizeParseAttempt(token)
	workCountEndFinalizeParseAttempt(token, ParseStopAccepted, nil)
	_ = EndDiagnosticWorkCount()
	receipt := EndDiagnosticTopologyReceipt()
	finished = true
	if !receipt.Complete() {
		t.Fatalf("distinct-shape rejection receipt is incomplete: %+v", receipt)
	}
}

func TestDiagnosticTopologyReceiptNodeAllocationRefreshesReusedPointer(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	predecessor := &gssNode{}
	workCountTopologyRecordNodeAllocation(predecessor)
	node := &gssNode{}
	workCountTopologyRecordNodeAllocation(node)
	first := activeDiagnosticTopology.nodeID(node)
	workCountTopologyRecordLinkInsert(node, predecessor, 0, true)
	workCountTopologyRecordNodeAllocation(node)
	second := activeDiagnosticTopology.nodeID(node)
	workCountTopologyRecordLinkInsert(node, predecessor, 0, true)
	receipt := EndDiagnosticTopologyReceipt()
	if first == 0 || second == 0 || first == second {
		t.Fatalf("node IDs = %d/%d, want distinct nonzero IDs", first, second)
	}
	if receipt.IdentityCollision || receipt.IdentityIncomplete || receipt.ArithmeticOverflow {
		t.Fatalf("explicit pointer reuse invalidated receipt: %+v", receipt)
	}
	if len(receipt.Events) != 2 || receipt.Events[0].LinkID == 0 || receipt.Events[1].LinkID == 0 || receipt.Events[0].LinkID == receipt.Events[1].LinkID {
		t.Fatalf("reused-node link events = %+v", receipt.Events)
	}
}

func TestDiagnosticTopologyReceiptPreservesEntriesPrefixAcrossPromotionAndCopy(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	stack := glrStack{
		entries:     []stackEntry{{state: 1}, {state: 2}},
		branchOrder: 0,
	}
	workCountTopologyRecordInitialVersion(&stack)
	initialNodeID := activeDiagnosticTopology.versions[1].nodeID
	initialPrefixIDs := append([]uint64(nil), activeDiagnosticTopology.versions[1].entryNodeIDs...)

	var scratch gssScratch
	clone := stack.cloneWithScratch(&scratch)
	clone.branchOrder = 1
	workCountTopologyRecordVersionCopy(&stack, &clone)
	copyVersionID := clone.diagnosticTopology.versionID
	copyVersion := activeDiagnosticTopology.versions[copyVersionID]
	if got := activeDiagnosticTopology.nodeID(stack.gss.head); got != initialNodeID {
		t.Fatalf("promoted head ID = %d, want initial ID %d", got, initialNodeID)
	}
	if copyVersion.nodeID != initialNodeID {
		t.Fatalf("copied head ID = %d, want shared ID %d", copyVersion.nodeID, initialNodeID)
	}
	if len(copyVersion.entryNodeIDs) != len(initialPrefixIDs) {
		t.Fatalf("copied prefix IDs = %v, want %v", copyVersion.entryNodeIDs, initialPrefixIDs)
	}
	for i := range initialPrefixIDs {
		if copyVersion.entryNodeIDs[i] != initialPrefixIDs[i] {
			t.Fatalf("copied prefix IDs = %v, want %v", copyVersion.entryNodeIDs, initialPrefixIDs)
		}
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("promotion/copy receipt is incomplete: %+v", receipt)
	}
	var copyEvent DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventVersionCopy {
			copyEvent = event
		}
	}
	if copyEvent.VersionID != copyVersionID || copyEvent.NodeID != initialNodeID {
		t.Fatalf("copy event = %+v, want version/head %d/%d", copyEvent, copyVersionID, initialNodeID)
	}
}

func TestDiagnosticTopologyReceiptSeparatesMutatedEntriesCopiesBeforePromotion(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	source := glrStack{entries: []stackEntry{{state: 1}, {state: 2}}, branchOrder: 41}
	workCountTopologyRecordInitialVersion(&source)
	first := source.clone()
	first.branchOrder = 99
	workCountTopologyRecordVersionCopy(&source, &first)
	second := source.clone()
	second.branchOrder = 7
	workCountTopologyRecordVersionCopy(&source, &second)

	first.entries[1].state = 3
	workCountTopologyCommitVersion(&first)
	firstID := first.diagnosticTopology.versionID
	secondID := second.diagnosticTopology.versionID
	firstPrefix := append([]uint64(nil), activeDiagnosticTopology.versions[firstID].entryNodeIDs...)
	secondPrefix := append([]uint64(nil), activeDiagnosticTopology.versions[secondID].entryNodeIDs...)
	if len(firstPrefix) != 2 || len(secondPrefix) != 2 || firstPrefix[0] != secondPrefix[0] || firstPrefix[1] == secondPrefix[1] {
		t.Fatalf("copy prefix IDs = %v/%v, want one shared and one distinct slot", firstPrefix, secondPrefix)
	}

	var scratch gssScratch
	first.ensureGSS(&scratch)
	second.ensureGSS(&scratch)
	if first.gss.head == nil || first.gss.head.prev == nil || second.gss.head == nil || second.gss.head.prev == nil {
		t.Fatal("promoted copies have an incomplete graph stack")
	}
	if got := activeDiagnosticTopology.nodeID(first.gss.head.prev); got != firstPrefix[0] {
		t.Fatalf("first shared prefix node ID = %d, want %d", got, firstPrefix[0])
	}
	if got := activeDiagnosticTopology.nodeID(second.gss.head.prev); got != secondPrefix[0] {
		t.Fatalf("second shared prefix node ID = %d, want %d", got, secondPrefix[0])
	}
	if got := activeDiagnosticTopology.nodeID(first.gss.head); got != firstPrefix[1] {
		t.Fatalf("first mutated head node ID = %d, want %d", got, firstPrefix[1])
	}
	if got := activeDiagnosticTopology.nodeID(second.gss.head); got != secondPrefix[1] {
		t.Fatalf("second original head node ID = %d, want %d", got, secondPrefix[1])
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("two-copy promotion receipt is incomplete: %+v", receipt)
	}
}

func TestDiagnosticTopologyReceiptRetiresMergedSlotAndRenumbersLaterVersions(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	source := glrStack{entries: []stackEntry{{state: 1}}, branchOrder: 41}
	workCountTopologyRecordInitialVersion(&source)
	removed := source.clone()
	removed.branchOrder = 99
	workCountTopologyRecordVersionCopy(&source, &removed)
	later := source.clone()
	later.branchOrder = 7
	workCountTopologyRecordVersionCopy(&source, &later)
	removedID := removed.diagnosticTopology.versionID
	laterID := later.diagnosticTopology.versionID
	workCountTopologyRecordMerge(&source, &removed, true)

	if _, ok := activeDiagnosticTopology.versions[removedID]; ok {
		t.Fatalf("merged version %d remains bound", removedID)
	}
	if got := activeDiagnosticTopology.versions[laterID].index; got != 1 {
		t.Fatalf("later version index = %d, want 1", got)
	}
	if len(activeDiagnosticTopology.versionSlots) != 2 || activeDiagnosticTopology.versionSlots[0] != source.diagnosticTopology.versionID || activeDiagnosticTopology.versionSlots[1] != laterID {
		t.Fatalf("physical version slots = %v", activeDiagnosticTopology.versionSlots)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("merge retirement receipt is incomplete: %+v", receipt)
	}
	if len(receipt.Events) != 5 {
		t.Fatalf("merge retirement events = %d, want 5: %+v", len(receipt.Events), receipt.Events)
	}
	merge := receipt.Events[3]
	renumber := receipt.Events[4]
	if merge.Kind != DiagnosticTopologyEventMerge || merge.SourceIndex != 0 || merge.TargetIndex != 1 || merge.RemovedVersionID != removedID {
		t.Fatalf("merge event = %+v", merge)
	}
	if renumber.Kind != DiagnosticTopologyEventVersionRenumber || renumber.VersionID != laterID || renumber.SourceIndex != 2 || renumber.TargetIndex != 1 || renumber.TargetVersionID != removedID || renumber.Flags&DiagnosticTopologyFlagTargetReplaced == 0 {
		t.Fatalf("renumber event = %+v", renumber)
	}
}

func TestDiagnosticTopologyReceiptRenumbersLaterSourceOntoTargetSlot(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	initial := glrStack{entries: []stackEntry{{state: 1}}}
	workCountTopologyRecordInitialVersion(&initial)
	target := initial.clone()
	workCountTopologyRecordVersionCopy(&initial, &target)
	middle := initial.clone()
	workCountTopologyRecordVersionCopy(&initial, &middle)
	source := initial.clone()
	workCountTopologyRecordVersionCopy(&initial, &source)
	later := initial.clone()
	workCountTopologyRecordVersionCopy(&initial, &later)
	targetID := target.diagnosticTopology.versionID
	middleID := middle.diagnosticTopology.versionID
	sourceID := source.diagnosticTopology.versionID
	laterID := later.diagnosticTopology.versionID

	workCountTopologyRenumberVersion(&source, &target)
	if _, ok := activeDiagnosticTopology.versions[targetID]; ok {
		t.Fatalf("replaced target version %d remains bound", targetID)
	}
	wantSlots := []uint64{initial.diagnosticTopology.versionID, sourceID, middleID, laterID}
	if len(activeDiagnosticTopology.versionSlots) != len(wantSlots) {
		t.Fatalf("renumbered slots = %v, want %v", activeDiagnosticTopology.versionSlots, wantSlots)
	}
	for i := range wantSlots {
		if activeDiagnosticTopology.versionSlots[i] != wantSlots[i] || activeDiagnosticTopology.versions[wantSlots[i]].index != uint64(i) {
			t.Fatalf("renumbered slots = %v, want %v", activeDiagnosticTopology.versionSlots, wantSlots)
		}
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("explicit renumber receipt is incomplete: %+v", receipt)
	}
	if len(receipt.Events) != 7 {
		t.Fatalf("explicit renumber events = %d, want 7: %+v", len(receipt.Events), receipt.Events)
	}
	direct := receipt.Events[5]
	shift := receipt.Events[6]
	if direct.Kind != DiagnosticTopologyEventVersionRenumber || direct.VersionID != sourceID || direct.SourceIndex != 3 || direct.TargetIndex != 1 || direct.TargetVersionID != targetID {
		t.Fatalf("direct renumber event = %+v", direct)
	}
	if shift.Kind != DiagnosticTopologyEventVersionRenumber || shift.VersionID != laterID || shift.SourceIndex != 4 || shift.TargetIndex != 3 || shift.TargetVersionID != sourceID {
		t.Fatalf("source-slot shift event = %+v", shift)
	}
}

func TestDiagnosticTopologyReceiptRenumberRejectsAliasedMutationBindings(t *testing.T) {
	t.Run("action", func(t *testing.T) {
		BeginDiagnosticTopologyReceipt()
		initial := glrStack{entries: []stackEntry{{state: 1}}}
		workCountTopologyRecordInitialVersion(&initial)
		target := initial.clone()
		workCountTopologyRecordVersionCopy(&initial, &target)
		source := initial.clone()
		workCountTopologyRecordVersionCopy(&initial, &source)
		alias := source
		workCountTopologyRecordAction(&alias, Token{Symbol: 1}, ParseAction{Type: ParseActionShift}, 0)
		workCountTopologyRenumberVersion(&source, &target)
		slotCount := len(activeDiagnosticTopology.versionSlots)
		receipt := EndDiagnosticTopologyReceipt()
		if !receipt.IdentityIncomplete || slotCount != 3 {
			t.Fatalf("action-bound renumber state = %+v, slots=%d", receipt, slotCount)
		}
	})

	t.Run("pending-copy", func(t *testing.T) {
		BeginDiagnosticTopologyReceipt()
		initial := glrStack{entries: []stackEntry{{state: 1}}}
		workCountTopologyRecordInitialVersion(&initial)
		target := initial.clone()
		workCountTopologyRecordVersionCopy(&initial, &target)
		pending := initial.clone()
		workCountTopologyPrepareVersionCopy(&initial, &pending)
		alias := pending
		workCountTopologyRenumberVersion(&alias, &target)
		slotCount := len(activeDiagnosticTopology.versionSlots)
		receipt := EndDiagnosticTopologyReceipt()
		if !receipt.IdentityIncomplete || slotCount != 3 {
			t.Fatalf("copy-bound renumber state = %+v, slots=%d", receipt, slotCount)
		}
	})
}

func TestDiagnosticTopologyReceiptRetiresOnlyUnpublishedVersions(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	initial := glrStack{entries: []stackEntry{{state: 1}}}
	workCountTopologyRecordInitialVersion(&initial)
	omitted := initial.clone()
	workCountTopologyRecordVersionCopy(&initial, &omitted)
	published := initial.clone()
	workCountTopologyRecordVersionCopy(&initial, &published)
	omittedID := omitted.diagnosticTopology.versionID
	publishedID := published.diagnosticTopology.versionID
	workCountTopologyRetireUnpublishedVersions([]glrStack{omitted, published}, []glrStack{published})
	if _, ok := activeDiagnosticTopology.versions[omittedID]; ok {
		t.Fatalf("omitted version %d remains active", omittedID)
	}
	if slots := activeDiagnosticTopology.versionSlots; len(slots) != 2 || slots[1] != publishedID {
		t.Fatalf("published slots = %v, want initial then %d", slots, publishedID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("unpublished-version receipt is incomplete: %+v", receipt)
	}
	last := receipt.Events[len(receipt.Events)-1]
	if last.Kind != DiagnosticTopologyEventVersionRenumber || last.VersionID != publishedID || last.SourceIndex != 2 || last.TargetIndex != 1 || last.TargetVersionID != omittedID {
		t.Fatalf("published-version shift = %+v", last)
	}
}

func TestDiagnosticTopologyReceiptReplacementAndCullDoNotDoubleRenumber(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	incumbent := glrStack{entries: []stackEntry{{state: 1}}, score: 1}
	workCountTopologyRecordInitialVersion(&incumbent)
	candidate := incumbent.clone()
	candidate.score = 2
	workCountTopologyRecordVersionCopy(&incumbent, &candidate)
	culled := incumbent.clone()
	culled.entries[0].state = 2
	culled.score = 0
	workCountTopologyRecordVersionCopy(&incumbent, &culled)
	workCountTopologyCommitVersion(&culled)
	incumbentID := incumbent.diagnosticTopology.versionID
	candidateID := candidate.diagnosticTopology.versionID
	culledID := culled.diagnosticTopology.versionID
	before := []glrStack{incumbent, candidate, culled}
	topologyBefore := append([]glrStack(nil), before...)

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.beginEquivEpoch()
	merged := mergeStacksWithScratch(before, &scratch)
	if len(merged) != 2 || merged[0].diagnosticTopology.versionID != candidateID {
		t.Fatalf("replacement survivors = %+v, want candidate %d first", merged, candidateID)
	}
	kept := retainTopStacksForLanguage(merged, 1, nil)
	workCountTopologyRetireMissingVersions(topologyBefore, kept)

	if len(activeDiagnosticTopology.versionSlots) != 1 || activeDiagnosticTopology.versionSlots[0] != candidateID {
		t.Fatalf("physical version slots = %v, want [%d]", activeDiagnosticTopology.versionSlots, candidateID)
	}
	if _, ok := activeDiagnosticTopology.versions[incumbentID]; ok {
		t.Fatalf("replaced incumbent version %d remains bound", incumbentID)
	}
	if _, ok := activeDiagnosticTopology.versions[culledID]; ok {
		t.Fatalf("culled version %d remains bound", culledID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("replacement/cull receipt is incomplete: %+v", receipt)
	}
	var renumbers []DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventVersionRenumber {
			renumbers = append(renumbers, event)
		}
	}
	if len(renumbers) != 2 {
		t.Fatalf("replacement/cull renumbers = %d, want 2: %+v", len(renumbers), renumbers)
	}
	direct := renumbers[0]
	if direct.SourceVersionID != candidateID || direct.SourceIndex != 1 || direct.TargetVersionID != incumbentID || direct.TargetIndex != 0 {
		t.Fatalf("replacement renumber = %+v", direct)
	}
	shift := renumbers[1]
	if shift.SourceVersionID != culledID || shift.SourceIndex != 2 || shift.TargetVersionID != candidateID || shift.TargetIndex != 1 {
		t.Fatalf("replacement tail shift = %+v", shift)
	}
}

func TestDiagnosticTopologyReceiptReconcilesDeadRemovalBeforeReplacement(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	dead := glrStack{entries: []stackEntry{{state: 1}}, dead: true}
	workCountTopologyRecordInitialVersion(&dead)
	incumbent := dead.clone()
	incumbent.dead = false
	incumbent.score = 1
	workCountTopologyRecordVersionCopy(&dead, &incumbent)
	candidate := incumbent.clone()
	candidate.score = 2
	workCountTopologyRecordVersionCopy(&incumbent, &candidate)
	deadID := dead.diagnosticTopology.versionID
	incumbentID := incumbent.diagnosticTopology.versionID
	candidateID := candidate.diagnosticTopology.versionID
	before := []glrStack{dead, incumbent, candidate}
	topologyBefore := append([]glrStack(nil), before...)

	var scratch glrMergeScratch
	scratch.perKeyCap = 1
	scratch.beginEquivEpoch()
	result := mergeStacksWithScratch(before, &scratch)
	workCountTopologyRetireMissingVersions(topologyBefore, result)
	if len(result) != 1 || result[0].diagnosticTopology.versionID != candidateID {
		t.Fatalf("dead/replacement survivor = %+v, want candidate %d", result, candidateID)
	}
	if len(activeDiagnosticTopology.versionSlots) != 1 || activeDiagnosticTopology.versionSlots[0] != candidateID {
		t.Fatalf("dead/replacement slots = %v, want [%d]", activeDiagnosticTopology.versionSlots, candidateID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("dead/replacement receipt is incomplete: %+v", receipt)
	}
	var renumbers []DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventVersionRenumber {
			renumbers = append(renumbers, event)
		}
	}
	if len(renumbers) != 3 {
		t.Fatalf("dead/replacement renumbers = %d, want 3: %+v", len(renumbers), renumbers)
	}
	if event := renumbers[0]; event.SourceVersionID != incumbentID || event.SourceIndex != 1 || event.TargetVersionID != deadID || event.TargetIndex != 0 {
		t.Fatalf("first dead-removal shift = %+v", event)
	}
	if event := renumbers[1]; event.SourceVersionID != candidateID || event.SourceIndex != 2 || event.TargetVersionID != incumbentID || event.TargetIndex != 1 {
		t.Fatalf("second dead-removal shift = %+v", event)
	}
	if event := renumbers[2]; event.SourceVersionID != candidateID || event.SourceIndex != 1 || event.TargetVersionID != incumbentID || event.TargetIndex != 0 {
		t.Fatalf("replacement renumber = %+v", event)
	}
}

func TestDiagnosticTopologyReceiptReconcilesBudgetCullBeforeReplacement(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	incumbent := glrStack{entries: []stackEntry{{state: 1}}, score: 1}
	workCountTopologyRecordInitialVersion(&incumbent)
	candidate := incumbent.clone()
	workCountTopologyRecordVersionCopy(&incumbent, &candidate)
	otherBest := incumbent.clone()
	otherBest.entries[0].state = 2
	otherBest.score = 2
	workCountTopologyRecordVersionCopy(&incumbent, &otherBest)
	workCountTopologyCommitVersion(&otherBest)
	otherLoser := otherBest.clone()
	otherLoser.score = 0
	workCountTopologyRecordVersionCopy(&otherBest, &otherLoser)
	incumbentID := incumbent.diagnosticTopology.versionID
	candidateID := candidate.diagnosticTopology.versionID
	otherBestID := otherBest.diagnosticTopology.versionID
	otherLoserID := otherLoser.diagnosticTopology.versionID
	before := []glrStack{incumbent, candidate, otherBest, otherLoser}
	topologyBefore := append([]glrStack(nil), before...)

	perStack := int64(unsafe.Sizeof(glrStack{}) + unsafe.Sizeof(glrMergeSlot{}))
	var scratch glrMergeScratch
	scratch.budgetBytes = 3 * perStack
	scratch.beginEquivEpoch()
	if limit := mergeAliveLimitForScratch(&scratch, len(before)); limit != 3 {
		t.Fatalf("merge alive limit = %d, want 3", limit)
	}
	result := mergeStacksWithScratch(before, &scratch)
	workCountTopologyRetireMissingVersions(topologyBefore, result)
	if len(result) != 2 || result[0].diagnosticTopology.versionID != otherBestID || result[1].diagnosticTopology.versionID != candidateID {
		t.Fatalf("budget-cull survivors = %+v, want versions [%d %d]", result, otherBestID, candidateID)
	}
	if _, ok := activeDiagnosticTopology.versions[incumbentID]; ok {
		t.Fatalf("replaced incumbent version %d remains bound", incumbentID)
	}
	if _, ok := activeDiagnosticTopology.versions[otherLoserID]; ok {
		t.Fatalf("budget-culled version %d remains bound", otherLoserID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("budget-cull receipt is incomplete: %+v", receipt)
	}
	var renumbers []DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventVersionRenumber {
			renumbers = append(renumbers, event)
		}
	}
	if len(renumbers) != 1 {
		t.Fatalf("budget-cull renumbers = %d, want 1: %+v", len(renumbers), renumbers)
	}
	if event := renumbers[0]; event.SourceVersionID != candidateID || event.SourceIndex != 2 || event.TargetVersionID != incumbentID || event.TargetIndex != 1 {
		t.Fatalf("post-cull replacement renumber = %+v", event)
	}
}

func TestDiagnosticTopologyReceiptIgnoresRetiredAcceptedCopiesDuringSync(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	live := glrStack{entries: []stackEntry{{state: 1}}}
	workCountTopologyRecordInitialVersion(&live)
	accepted := live.clone()
	workCountTopologyRecordVersionCopy(&live, &accepted)
	accepted.accepted = true
	staleAccepted := accepted
	workCountTopologyRetireVersion(&accepted)
	workCountTopologySyncVersionOrder([]glrStack{staleAccepted, live})
	if len(activeDiagnosticTopology.versionSlots) != 1 || activeDiagnosticTopology.versionSlots[0] != live.diagnosticTopology.versionID {
		t.Fatalf("accepted/live slots = %v, want live version %d", activeDiagnosticTopology.versionSlots, live.diagnosticTopology.versionID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("accepted/live receipt is incomplete: %+v", receipt)
	}
}

func TestDiagnosticTopologyReceiptRetiresSamePopReductionLosers(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	low := NewLeafNode(7, true, 0, 1, Point{}, Point{Column: 1})
	low.dynamicPrecedence = 1
	target := glrStack{
		entries:    []stackEntry{{state: 1}, newStackEntryNode(2, low)},
		byteOffset: 1,
	}
	workCountTopologyRecordInitialVersion(&target)
	var scratch gssScratch
	target.ensureGSS(&scratch)
	target.entries = nil
	better := target.cloneWithScratch(&scratch)
	workCountTopologyRecordVersionCopy(&target, &better)
	high := NewLeafNode(7, true, 0, 1, Point{}, Point{Column: 1})
	high.dynamicPrecedence = 2
	better.gss = gssStack{head: target.gss.head.prev}
	better.gss.push(2, high, &scratch)
	workCountTopologyCommitVersion(&better)
	betterID := better.diagnosticTopology.versionID
	parser := &Parser{}
	collapsed, reason := parser.cTryCollapseSamePopReductionVersion(&target, &better, nil)
	if reason != ParseStopNone || !collapsed {
		t.Fatalf("better same-pop collapse = %v, reason = %v", collapsed, reason)
	}
	if target.diagnosticTopology.versionID != betterID {
		t.Fatalf("same-pop survivor version = %d, want %d", target.diagnosticTopology.versionID, betterID)
	}

	worse := target.cloneWithScratch(&scratch)
	workCountTopologyRecordVersionCopy(&target, &worse)
	worseNode := NewLeafNode(7, true, 0, 1, Point{}, Point{Column: 1})
	worseNode.dynamicPrecedence = 0
	worse.gss = gssStack{head: target.gss.head.prev}
	worse.gss.push(2, worseNode, &scratch)
	workCountTopologyCommitVersion(&worse)
	worseID := worse.diagnosticTopology.versionID
	collapsed, reason = parser.cTryCollapseSamePopReductionVersion(&target, &worse, nil)
	if reason != ParseStopNone || !collapsed {
		t.Fatalf("worse same-pop collapse = %v, reason = %v", collapsed, reason)
	}
	if _, ok := activeDiagnosticTopology.versions[worseID]; ok {
		t.Fatalf("same-pop loser version %d remains bound", worseID)
	}
	if len(activeDiagnosticTopology.versionSlots) != 1 || activeDiagnosticTopology.versionSlots[0] != betterID {
		t.Fatalf("same-pop slots = %v, want [%d]", activeDiagnosticTopology.versionSlots, betterID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("same-pop receipt is incomplete: %+v", receipt)
	}
	var renumbers []DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventVersionRenumber {
			renumbers = append(renumbers, event)
		}
	}
	if len(renumbers) != 1 || renumbers[0].SourceVersionID != betterID || renumbers[0].SourceIndex != 1 || renumbers[0].TargetIndex != 0 {
		t.Fatalf("same-pop renumbers = %+v", renumbers)
	}
}

func TestDiagnosticTopologyReceiptReusesEntriesPopTargetIdentity(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	stack := glrStack{entries: []stackEntry{{state: 1}}, branchOrder: 0}
	workCountTopologyRecordInitialVersion(&stack)
	for ordinal := 0; ordinal < 2; ordinal++ {
		workCountTopologyRecordAction(&stack, Token{Symbol: 9}, ParseAction{Type: ParseActionReduce}, ordinal)
		workCountTopologyRecordDirectPop(&stack, 0)
		workCountTopologyRecordActionResult(&stack)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("entries-pop receipt is incomplete: %+v", receipt)
	}
	var popTargets []uint64
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventPopPath {
			popTargets = append(popTargets, event.PopToNodeID)
		}
	}
	if len(popTargets) != 2 || popTargets[0] == 0 || popTargets[0] != popTargets[1] {
		t.Fatalf("pop target IDs = %v, want two equal nonzero IDs", popTargets)
	}
}

func TestDiagnosticTopologyReceiptRetiresRecoverAcceptedVersionWithoutAcceptElection(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	stack := glrStack{entries: []stackEntry{{state: 1}}, branchOrder: 0}
	workCountTopologyRecordInitialVersion(&stack)
	initialID := stack.diagnosticTopology.versionID
	later := stack.clone()
	workCountTopologyRecordVersionCopy(&stack, &later)
	laterID := later.diagnosticTopology.versionID
	workCountTopologyRecordAction(&stack, Token{Symbol: 0}, ParseAction{Type: ParseActionRecover}, 0)
	stack.accepted = true
	workCountTopologyRecordActionResult(&stack)
	if stack.diagnosticTopology.versionID != 0 {
		t.Fatalf("accepted recovery version token = %d, want zero", stack.diagnosticTopology.versionID)
	}
	if slots := activeDiagnosticTopology.versionSlots; len(slots) != 1 || slots[0] != laterID {
		t.Fatalf("accepted recovery slots = %v, want [%d]", slots, laterID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("recovery-accept receipt is incomplete: %+v", receipt)
	}
	renumbers := 0
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventAcceptElection {
			t.Fatalf("recovery accepted stack emitted an accept election: %+v", event)
		}
		if event.Kind == DiagnosticTopologyEventVersionRenumber {
			renumbers++
			if event.VersionID != laterID || event.SourceIndex != 1 || event.TargetIndex != 0 || event.TargetVersionID != initialID {
				t.Fatalf("recovery-accept shift = %+v", event)
			}
		}
	}
	if renumbers != 1 {
		t.Fatalf("recovery-accept renumbers = %d, want 1", renumbers)
	}
}

func TestDiagnosticTopologyReceiptRetiresCRecoverEOFAccept(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	stack := glrStack{entries: []stackEntry{{state: 1}}}
	workCountTopologyRecordInitialVersion(&stack)
	initialID := stack.diagnosticTopology.versionID
	later := stack.clone()
	workCountTopologyRecordVersionCopy(&stack, &later)
	laterID := later.diagnosticTopology.versionID
	parser := &Parser{}
	parser.cRecoverEOFAccept(&stack, Token{}, nil, nil, nil, nil, nil)
	if !stack.accepted || stack.diagnosticTopology.versionID != 0 {
		t.Fatalf("C EOF recovery accepted/token = %v/%d", stack.accepted, stack.diagnosticTopology.versionID)
	}
	if slots := activeDiagnosticTopology.versionSlots; len(slots) != 1 || slots[0] != laterID {
		t.Fatalf("C EOF recovery slots = %v, want [%d]", slots, laterID)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("C EOF recovery receipt is incomplete: %+v", receipt)
	}
	renumbers := 0
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventAcceptElection {
			t.Fatalf("C EOF recovery emitted an accept election: %+v", event)
		}
		if event.Kind == DiagnosticTopologyEventVersionRenumber {
			renumbers++
			if event.VersionID != laterID || event.SourceIndex != 1 || event.TargetIndex != 0 || event.TargetVersionID != initialID {
				t.Fatalf("C EOF recovery shift = %+v", event)
			}
		}
	}
	if renumbers != 1 {
		t.Fatalf("C EOF recovery renumbers = %d, want 1", renumbers)
	}
}

func TestDiagnosticTopologyReceiptKeepsEachCandidateFromOneAcceptAction(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	stack := glrStack{entries: []stackEntry{{state: 1}, {state: 2}}, branchOrder: 0}
	workCountTopologyRecordInitialVersion(&stack)
	var scratch gssScratch
	stack.ensureGSS(&scratch)
	stack.entries = nil
	stack.cacheEntries = false
	stack.gss.head.appendExtraLink(gssMainLink{
		prev:  stack.gss.head.prev,
		entry: stackEntry{state: 3},
	})
	workCountTopologyRecordLinkInsert(stack.gss.head, stack.gss.head.prev, 1, false)
	if got := len(expandPackedGSSResultPaths([]glrStack{stack})); got != 2 {
		t.Fatalf("expanded packed paths = %d, want two; links=%d", got, stack.gss.head.linkCount())
	}
	workCountTopologyRecordAction(&stack, Token{Symbol: 0}, ParseAction{Type: ParseActionAccept}, 0)
	stack.accepted = true
	workCountTopologyRecordActionResult(&stack)
	if stack.diagnosticTopology.versionID != 0 || len(activeDiagnosticTopology.versionSlots) != 0 {
		t.Fatalf("accepted versions remain active: stack=%d slots=%v", stack.diagnosticTopology.versionID, activeDiagnosticTopology.versionSlots)
	}
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("multi-candidate accept receipt is incomplete: %+v", receipt)
	}
	var acceptAction DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventAction && event.ActionType == uint64(ParseActionAccept) {
			acceptAction = event
		}
	}
	var eofLinks, adds, pops, elections []DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventAcceptElection {
			elections = append(elections, event)
		}
		if event.ActionID == acceptAction.ActionID {
			switch event.Kind {
			case DiagnosticTopologyEventLinkInsert:
				eofLinks = append(eofLinks, event)
			case DiagnosticTopologyEventVersionAdd:
				adds = append(adds, event)
			case DiagnosticTopologyEventPopPath:
				pops = append(pops, event)
			}
		}
	}
	if len(eofLinks) != 1 || len(adds) != 2 || len(pops) != 2 {
		t.Fatalf("ACCEPT topology has links/adds/pops = %d/%d/%d, want 1/2/2", len(eofLinks), len(adds), len(pops))
	}
	if len(elections) != 2 {
		t.Fatalf("accept elections = %d, want two", len(elections))
	}
	if elections[0].ActionID == 0 || elections[0].ActionID != elections[1].ActionID ||
		elections[0].CandidateID == 0 || elections[0].CandidateID == elections[1].CandidateID {
		t.Fatalf("accept elections do not preserve the shared action and distinct candidates: %+v", elections)
	}
	if elections[0].Flags&DiagnosticTopologyFlagNoIncumbent == 0 ||
		elections[1].IncumbentID != elections[0].CandidateID {
		t.Fatalf("accept election chain = %+v, want the first candidate as the second incumbent", elections)
	}
	if acceptAction.EventID == 0 || eofLinks[0].EventID != acceptAction.EventID+1 ||
		adds[0].EventID != eofLinks[0].EventID+1 || pops[0].EventID != adds[0].EventID+1 ||
		adds[1].EventID != pops[0].EventID+1 || pops[1].EventID != adds[1].EventID+1 ||
		elections[0].EventID != pops[1].EventID+1 || elections[1].EventID != elections[0].EventID+1 {
		t.Fatalf("ACCEPT action sequence is not EOF push, pop paths, then elections: %+v", receipt.Events)
	}
	if pops[0].PopID == 0 || pops[0].PopID != pops[1].PopID || pops[0].PathOrdinal != 0 || pops[1].PathOrdinal != 1 {
		t.Fatalf("ACCEPT pop paths do not share one pop: %+v", pops)
	}
	for i := range pops {
		if pops[i].PayloadCount != elections[i].PayloadCount {
			t.Fatalf("candidate %d payload count changed from pop to election: %d/%d", i, pops[i].PayloadCount, elections[i].PayloadCount)
		}
	}
}

func TestDiagnosticTopologyReceiptStopsAtEventCounterOverflow(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	activeDiagnosticTopology.receipt.EventsSeen = ^uint64(0)
	activeDiagnosticTopology.appendEvent(diagnosticTopologyEventBase())
	receipt := EndDiagnosticTopologyReceipt()
	if receipt.EventsSeen != ^uint64(0) || receipt.EventsRetained != 0 || len(receipt.Events) != 0 {
		t.Fatalf("overflow receipt recorded an event: %+v", receipt)
	}
	if !receipt.ArithmeticOverflow || !receipt.IdentityIncomplete || receipt.Complete() {
		t.Fatalf("overflow flags = %+v", receipt)
	}
}

func TestDiagnosticTopologyReceiptRecordsChildElection(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	stack := glrStack{
		entries:     []stackEntry{{state: 1}},
		branchOrder: 7,
	}
	workCountTopologyRecordInitialVersion(&stack)
	workCountTopologyRecordAction(&stack, Token{Symbol: 13}, ParseAction{Type: ParseActionReduce}, 2)
	incumbentWindow := []stackEntry{{state: 21}}
	candidateWindow := []stackEntry{{state: 22}}
	popToNodeID := activeDiagnosticTopology.versions[1].entryNodeIDs[0]
	workCountTopologyRecordPopPathWithNodeID(&stack, incumbentWindow, nil, popToNodeID, 0)
	workCountTopologyRecordPopPathWithNodeID(&stack, candidateWindow, nil, popToNodeID, 1)
	workCountTopologyRecordChildElection(
		&stack,
		reduceFork{window: incumbentWindow},
		reduceFork{window: candidateWindow},
		-1,
	)
	workCountTopologyRecordChildElection(
		&stack,
		reduceFork{window: incumbentWindow},
		reduceFork{window: candidateWindow},
		0,
	)
	workCountTopologyRecordActionResult(&stack)
	receipt := EndDiagnosticTopologyReceipt()

	var pops []DiagnosticTopologyEvent
	var election DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		switch event.Kind {
		case DiagnosticTopologyEventPopPath:
			pops = append(pops, event)
		case DiagnosticTopologyEventChildElection:
			election = event
		}
	}
	if len(pops) != 2 || pops[0].PathOrdinal != 0 || pops[1].PathOrdinal != 1 || pops[0].PopID == 0 || pops[0].PopID != pops[1].PopID {
		t.Fatalf("pop events = %+v", pops)
	}
	if election.ElectionID != 1 || election.PopID != 0 || election.IncumbentID != 1 || election.CandidateID != 2 || election.SelectedID != 2 {
		t.Fatalf("child election = %+v, pops = %+v", election, pops)
	}
	if election.Flags&(DiagnosticTopologyFlagSuccessOrSelected|DiagnosticTopologyFlagActionContextKnown) != DiagnosticTopologyFlagSuccessOrSelected|DiagnosticTopologyFlagActionContextKnown {
		t.Fatalf("child election flags = %#x", election.Flags)
	}
	if !receipt.Complete() {
		t.Fatalf("child-election receipt is incomplete: %+v", receipt)
	}
}

func TestDiagnosticTopologyReceiptMarksIdentityCollisionAndOverflow(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	source := glrStack{entries: []stackEntry{{state: 1}}, branchOrder: 0}
	target := source
	target.branchOrder = 1
	workCountTopologyRecordInitialVersion(&source)
	workCountTopologyPrepareVersionCopy(&source, &target)
	workCountTopologyPrepareVersionCopy(&source, &target)
	activeDiagnosticTopology.nextNodeID = ^uint64(0)
	workCountTopologyRecordNodeAllocation(&gssNode{})
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.IdentityCollision || !receipt.IdentityIncomplete || !receipt.ArithmeticOverflow || receipt.Complete() {
		t.Fatalf("collision and overflow flags = %+v", receipt)
	}
}

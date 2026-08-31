//go:build gts_workcount

package gotreesitter

import "testing"

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
	copyVersionID := activeDiagnosticTopology.stackVersions[&clone].versionID
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

func TestDiagnosticTopologyReceiptDoesNotCallFinalFallbackAnAcceptElection(t *testing.T) {
	BeginDiagnosticTopologyReceipt()
	stack := glrStack{entries: []stackEntry{{state: 1}}, branchOrder: 0}
	workCountTopologyRecordInitialVersion(&stack)
	workCountTopologyRecordAction(&stack, Token{Symbol: 0}, ParseAction{Type: ParseActionRecover}, 0)
	stack.accepted = true
	workCountTopologyRecordActionResult(&stack)
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("unaccepted-final receipt is incomplete: %+v", receipt)
	}
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventAcceptElection {
			t.Fatalf("unaccepted final stack emitted an accept election: %+v", event)
		}
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
	receipt := EndDiagnosticTopologyReceipt()
	if !receipt.Complete() {
		t.Fatalf("multi-candidate accept receipt is incomplete: %+v", receipt)
	}
	var elections []DiagnosticTopologyEvent
	var acceptAction DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		if event.Kind == DiagnosticTopologyEventAction && event.ActionType == uint64(ParseActionAccept) {
			acceptAction = event
		}
		if event.Kind == DiagnosticTopologyEventAcceptElection {
			elections = append(elections, event)
		}
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
	if acceptAction.EventID == 0 || elections[0].EventID != acceptAction.EventID+1 || elections[1].EventID != elections[0].EventID+1 {
		t.Fatalf("accept elections are not inside the ACCEPT action: %+v", elections)
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

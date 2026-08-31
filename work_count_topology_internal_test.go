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
	workCountTopologyRecordPopPath(&stack, incumbentWindow, nil, 0)
	workCountTopologyRecordPopPath(&stack, candidateWindow, nil, 1)
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

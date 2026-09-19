package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

type linkProvenanceFixture struct {
	core       *Core
	boundary   ClassifiedBoundary
	descriptor ActionRowDescriptor
	plan       ReductionPlan
	rangeValue LinkRange
	refs       []DropCohortRef
	action     Action
	payload    SubtreeID
	nodes      []NodeID
}

func newLinkProvenanceFixture(t *testing.T, maxLinksPerBoundary uint32) linkProvenanceFixture {
	t.Helper()
	action := Action{Type: ActionReduce, State: 3, Symbol: 9, ProductionID: 17}
	table := &fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 9}: {action}},
	}
	core, err := New(table, Limits{MaxLinksPerBoundary: maxLinksPerBoundary})
	if err != nil {
		t.Fatal(err)
	}
	firstNode, err := core.appendNode(nodeRecord{state: 3, byteOffset: 0, pathCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondNode, err := core.appendNode(nodeRecord{state: 3, byteOffset: 1, pathCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: action.ProductionID, startByte: 0, endByte: 1,
		childSymbols: []Symbol{30},
	})
	firstLink := core.appendGraphLink(linkRecord{prev: firstNode, payload: payload})
	secondLink := core.appendGraphLink(linkRecord{prev: secondNode, payload: payload, next: firstLink})
	boundary, err := core.ClassifyBoundary(Head{Node: firstNode}, 9)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewReductionPlan(action.ProductionID, int(action.ChildCount), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	handle := core.dropCohortHandle(1)
	identity := DropCohortActionIdentity{
		BoundaryState: boundary.state,
		Lookahead:     boundary.lookahead,
		ActionOrdinal: 0,
		Action:        action,
		Selection:     DropCohortSelectionNone,
	}
	core.dropCohortRecords = []dropCohortRecord{{
		handle: handle, state: DropCohortComplete, expected: 2, written: 2,
	}}
	core.dropCohortMembers = []dropCohortMember{
		{cohort: handle, head: Head{Node: firstNode}, branch: 0, action: identity},
		{cohort: handle, head: Head{Node: secondNode}, branch: 1, action: identity},
	}
	refs := []DropCohortRef{
		{Owner: handle.Owner, Epoch: handle.Epoch, Sequence: handle.Sequence, Branch: 1},
		{Owner: handle.Owner, Epoch: handle.Epoch, Sequence: handle.Sequence, Branch: 0},
	}
	core.dropCohortCertificateRefs = append(core.dropCohortCertificateRefs, refs...)
	return linkProvenanceFixture{
		core: core, boundary: boundary, descriptor: boundary.actions.Descriptor(), plan: plan,
		rangeValue: LinkRange{First: secondLink, Count: 2}, refs: refs, action: action, payload: payload,
		nodes: []NodeID{firstNode, secondNode},
	}
}

func bindFixture(t *testing.T, fixture linkProvenanceFixture) error {
	t.Helper()
	return fixture.core.BindDropCohortLinkRefs(
		fixture.boundary, fixture.descriptor, 0, fixture.plan,
		fixture.rangeValue, fixture.refs,
	)
}

func TestLinkProvenanceDefaultOffAndAccounting(t *testing.T) {
	fixture := newLinkProvenanceFixture(t, 8)
	core := fixture.core
	if core.dropCohortLinkRefIndexes != nil {
		t.Fatal("new core allocated link provenance sidecar")
	}
	want := uint64(len(core.nodes))*coreNodeRecordBytes + uint64(len(core.links))*coreLinkRecordBytes +
		uint64(len(core.subtrees))*coreSubtreeRecordBytes + uint64(len(core.children))*coreChildRecordBytes +
		uint64(len(core.dropCohortRecords))*coreDropCohortRecordBytes +
		uint64(len(core.dropCohortMembers))*coreDropCohortMemberBytes +
		uint64(len(core.dropCohortCertificateRefs))*coreDropCohortRefBytes
	if got := core.StorageBytes(); got != want {
		t.Fatalf("default storage=%d, want %d", got, want)
	}
	before := core.StorageBytes()
	if err := bindFixture(t, fixture); err != nil {
		t.Fatal(err)
	}
	if len(core.dropCohortLinkRefIndexes) != len(core.links) {
		t.Fatalf("sidecar length=%d, want %d", len(core.dropCohortLinkRefIndexes), len(core.links))
	}
	if got, want := core.StorageBytes()-before, uint64(len(core.links))*coreUint32Bytes; got != want {
		t.Fatalf("sidecar storage delta=%d, want %d", got, want)
	}
	if got := core.dropCohortLinkRefJournal; len(got) != 0 {
		t.Fatalf("committed sidecar journal length=%d, want zero", len(got))
	}
	if got := core.FootprintBytes(); got < core.StorageBytes() {
		t.Fatalf("footprint=%d is below storage=%d", got, core.StorageBytes())
	}
	for index, ref := range fixture.refs {
		link := fixture.rangeValue.First - LinkID(index)
		if got, ok := core.DropCohortLinkRef(link); !ok || got != ref {
			t.Fatalf("link %d ref=(%+v,%t), want %+v", link, got, ok, ref)
		}
	}
	if got, ok := core.DropCohortLinkRef(0); ok || got != (DropCohortRef{}) {
		t.Fatalf("zero link lookup=(%+v,%t), want empty", got, ok)
	}
}

func TestLinkProvenanceManyToOneAndExactGraphAuthentication(t *testing.T) {
	many := newLinkProvenanceFixture(t, 8)
	many.core.links[many.rangeValue.First-1].prev = many.nodes[0]
	many.core.dropCohortMembers[1].head = Head{Node: many.nodes[0]}
	many.refs[1] = many.refs[0]
	many.core.dropCohortCertificateRefs = many.core.dropCohortCertificateRefs[:1]
	if err := bindFixture(t, many); err != nil {
		t.Fatalf("many-to-one bind failed: %v", err)
	}
	if many.core.dropCohortLinkRefIndexes[0] != 1 || many.core.dropCohortLinkRefIndexes[1] != 1 {
		t.Fatalf("many-to-one sidecar=%v, want [1 1]", many.core.dropCohortLinkRefIndexes)
	}

	negative := newLinkProvenanceFixture(t, 8)
	negative.core.dropCohortMembers[0].head = Head{Node: negative.nodes[1]}
	if err := bindFixture(t, negative); err == nil {
		t.Fatal("same-action member with a different graph head was accepted")
	}
	if negative.core.dropCohortLinkRefIndexes != nil {
		t.Fatalf("failed graph authentication allocated sidecar=%v", negative.core.dropCohortLinkRefIndexes)
	}
}

func TestLinkProvenanceRejectsMalformedRangesAndReferences(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Core, LinkRange) LinkRange
	}{
		{name: "mixed first zero", edit: func(_ *Core, _ LinkRange) LinkRange { return LinkRange{Count: 1} }},
		{name: "mixed count zero", edit: func(_ *Core, _ LinkRange) LinkRange { return LinkRange{First: 1} }},
		{name: "first out of bounds", edit: func(_ *Core, _ LinkRange) LinkRange { return LinkRange{First: 99, Count: 1} }},
		{name: "noncontiguous", edit: func(core *Core, value LinkRange) LinkRange {
			core.links[value.First-1].next = 0
			return value
		}},
		{name: "chain skips", edit: func(core *Core, value LinkRange) LinkRange {
			core.links[value.First-1].next = value.First - 1
			core.links[value.First-2].next = value.First + 1
			return value
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLinkProvenanceFixture(t, 8)
			value := test.edit(fixture.core, fixture.rangeValue)
			if _, err := fixture.core.resolveDropCohortLinkRange(value); err == nil {
				t.Fatalf("range %+v was accepted", value)
			}
		})
	}

	capFixture := newLinkProvenanceFixture(t, 1)
	if _, err := capFixture.core.resolveDropCohortLinkRange(capFixture.rangeValue); err == nil {
		t.Fatal("range above boundary cap was accepted")
	}
	zero := newLinkProvenanceFixture(t, 8)
	if err := zero.core.BindDropCohortLinkRefs(zero.boundary, zero.descriptor, 0, zero.plan, LinkRange{}, nil); err != nil {
		t.Fatalf("zero range with zero refs failed: %v", err)
	}
	for _, value := range []LinkRange{{Count: 1}, {First: 1}} {
		if err := zero.core.BindDropCohortLinkRefs(zero.boundary, zero.descriptor, 0, zero.plan, value, nil); err == nil {
			t.Fatalf("mixed-empty range %+v was accepted", value)
		}
	}
	stale := newLinkProvenanceFixture(t, 8)
	stale.refs[0].Owner++
	if err := bindFixture(t, stale); err == nil || stale.core.dropCohortLinkRefIndexes != nil {
		t.Fatalf("foreign owner result err=%v sidecar=%v", err, stale.core.dropCohortLinkRefIndexes)
	}
	stale = newLinkProvenanceFixture(t, 8)
	stale.refs[0].Epoch = 0
	if err := bindFixture(t, stale); err == nil || stale.core.dropCohortLinkRefIndexes != nil {
		t.Fatalf("stale epoch result err=%v sidecar=%v", err, stale.core.dropCohortLinkRefIndexes)
	}
}

func TestLinkProvenanceRollbackRestoresContentAllocationAndLength(t *testing.T) {
	fixture := newLinkProvenanceFixture(t, 8)
	fixture.core.dropCohortLinkRefIndexes = make([]uint32, len(fixture.core.links))
	before := append([]uint32(nil), fixture.core.dropCohortLinkRefIndexes...)
	beforeLen, beforeCap := len(fixture.core.dropCohortLinkRefIndexes), cap(fixture.core.dropCohortLinkRefIndexes)
	beforePtr := &fixture.core.dropCohortLinkRefIndexes[0]
	err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := fixture.core.BindDropCohortLinkRefsOwned(owner, fixture.boundary, fixture.descriptor, 0, fixture.plan, fixture.rangeValue, fixture.refs); err != nil {
			return err
		}
		return errors.New("force sidecar rollback")
	})
	if err == nil {
		t.Fatal("forced rollback returned nil")
	}
	if !reflect.DeepEqual(fixture.core.dropCohortLinkRefIndexes, before) || len(fixture.core.dropCohortLinkRefIndexes) != beforeLen || cap(fixture.core.dropCohortLinkRefIndexes) != beforeCap || &fixture.core.dropCohortLinkRefIndexes[0] != beforePtr {
		t.Fatalf("rollback sidecar=%v len/cap=%d/%d, want %v %d/%d", fixture.core.dropCohortLinkRefIndexes, len(fixture.core.dropCohortLinkRefIndexes), cap(fixture.core.dropCohortLinkRefIndexes), before, beforeLen, beforeCap)
	}

	fresh := newLinkProvenanceFixture(t, 8)
	if err := fresh.core.BindDropCohortLinkRefs(fresh.boundary, fresh.descriptor, 0, fresh.plan, fresh.rangeValue, fresh.refs); err != nil {
		t.Fatal(err)
	}
	if err := fresh.core.Reset(); err != nil {
		t.Fatal(err)
	}
	if len(fresh.core.dropCohortLinkRefIndexes) != 0 || len(fresh.core.dropCohortLinkRefJournal) != 0 {
		t.Fatalf("reset sidecar lengths=%d/%d, want zero", len(fresh.core.dropCohortLinkRefIndexes), len(fresh.core.dropCohortLinkRefJournal))
	}
	if _, ok := fresh.core.DropCohortLinkRef(1); ok {
		t.Fatal("reset retained a link provenance reference")
	}
}

func TestLinkProvenanceRejectsAuthenticationMismatches(t *testing.T) {
	fixture := newLinkProvenanceFixture(t, 8)
	badPlan, err := NewReductionPlan(fixture.action.ProductionID+1, int(fixture.action.ChildCount), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.core.BindDropCohortLinkRefs(fixture.boundary, fixture.descriptor, 0, badPlan, fixture.rangeValue, fixture.refs); err == nil {
		t.Fatal("mismatched reduction plan was accepted")
	}
	if err := fixture.core.BindDropCohortLinkRefs(fixture.boundary, ActionRowDescriptor{}, 0, fixture.plan, fixture.rangeValue, fixture.refs); err == nil {
		t.Fatal("mismatched row descriptor was accepted")
	}
	if err := fixture.core.BindDropCohortLinkRefs(fixture.boundary, fixture.descriptor, -1, fixture.plan, fixture.rangeValue, fixture.refs); err == nil {
		t.Fatal("negative action ordinal was accepted")
	}
}

func TestReductionOutputCarriesAuthenticatedLinkChain(t *testing.T) {
	action := Action{Type: ActionReduce, State: 3, Symbol: 9, ChildCount: 1, ProductionID: 17}
	core, err := New(&fakeTable{
		actions: map[tableCell][]Action{{state: 3, symbol: 9}: {action}},
		gotos:   map[tableCell]StateID{{state: 1, symbol: 9}: 4},
	}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	head, err := core.appendDiagnosticPayload(seed, 3, Token{Symbol: 1, StartByte: 0, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := core.ClassifyBoundary(head, 9)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := core.ReduceOutputsClassifiedInto(nil, boundary, 0, ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 {
		t.Fatalf("reduction outputs=%d, want one", len(outputs))
	}
	node, err := core.node(outputs[0].Head.Node)
	if err != nil {
		t.Fatal(err)
	}
	want := LinkChainRef{First: LinkID(node.firstLink), Count: node.linkCount}
	if outputs[0].Links != want {
		t.Fatalf("output link chain=%+v, want %+v", outputs[0].Links, want)
	}
	if core.dropCohortLinkRefIndexes != nil {
		t.Fatalf("default reduction allocated sidecar=%v", core.dropCohortLinkRefIndexes)
	}
	allocs := testing.AllocsPerRun(100, func() {
		got, err := core.reductionLinkChainForHead(outputs[0].Head)
		if err != nil || got != want {
			t.Fatalf("allocation probe chain=%+v err=%v, want %+v", got, err, want)
		}
	})
	if allocs != 0 {
		t.Fatalf("reduction link chain handoff allocations=%v, want zero", allocs)
	}
}

func TestReductionLinkChainHandoffPreservesNoncontiguousIDs(t *testing.T) {
	fixture := newLinkProvenanceFixture(t, 8)
	chainLink := fixture.core.appendGraphLink(linkRecord{
		prev: fixture.nodes[0], payload: fixture.payload, next: fixture.rangeValue.First - 1,
	})
	nodeID, err := fixture.core.appendNode(nodeRecord{
		state: 3, byteOffset: 2, firstLink: uint32(chainLink), linkCount: 2, pathCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := LinkChainRef{First: chainLink, Count: 2}
	got, err := fixture.core.reductionLinkChainForHead(Head{Node: nodeID})
	if err != nil || got != want {
		t.Fatalf("noncontiguous chain=%+v err=%v, want %+v", got, err, want)
	}
	if _, err := fixture.core.LinkRangeForHead(Head{Node: nodeID}); err == nil {
		t.Fatal("sidecar LinkRange accepted a noncontiguous chain")
	}
}

func TestReductionLinkChainHandoffRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Core, NodeID, LinkChainRef)
	}{
		{name: "mixed empty", edit: func(core *Core, nodeID NodeID, _ LinkChainRef) {
			core.nodes[nodeID-1].firstLink = 0
		}},
		{name: "first out of bounds", edit: func(core *Core, nodeID NodeID, value LinkChainRef) {
			core.nodes[nodeID-1].firstLink = uint32(len(core.links) + 1)
			core.nodes[nodeID-1].linkCount = value.Count
		}},
		{name: "count exceeds arena", edit: func(core *Core, nodeID NodeID, value LinkChainRef) {
			core.nodes[nodeID-1].firstLink = uint32(value.First)
			core.nodes[nodeID-1].linkCount = uint32(len(core.links) + 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLinkProvenanceFixture(t, 8)
			nodeID, err := fixture.core.appendNode(nodeRecord{
				state: 3, byteOffset: 2, firstLink: uint32(fixture.rangeValue.First),
				linkCount: fixture.rangeValue.Count, pathCount: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			test.edit(fixture.core, nodeID, LinkChainRef{
				First: fixture.rangeValue.First, Count: fixture.rangeValue.Count,
			})
			if _, err := fixture.core.reductionLinkChainForHead(Head{Node: nodeID}); err == nil {
				t.Fatalf("malformed node metadata was accepted: %s", test.name)
			}
		})
	}
}

func TestReductionLinkChainHandoffRollbackAndReset(t *testing.T) {
	fixture := newLinkProvenanceFixture(t, 8)
	beforeNodes, beforeLinks := len(fixture.core.nodes), len(fixture.core.links)
	var escaped Head
	err := fixture.core.ApplySchedulerAtomic(func(_ SchedulerTransactionToken) error {
		id, err := fixture.core.appendNode(nodeRecord{
			state: 3, byteOffset: 2, firstLink: uint32(fixture.rangeValue.First),
			linkCount: fixture.rangeValue.Count, pathCount: 1,
		})
		if err != nil {
			return err
		}
		escaped = Head{Node: id}
		if _, err := fixture.core.reductionLinkChainForHead(escaped); err != nil {
			return err
		}
		return errors.New("force reduction handoff rollback")
	})
	if err == nil {
		t.Fatal("forced rollback returned nil")
	}
	if len(fixture.core.nodes) != beforeNodes || len(fixture.core.links) != beforeLinks {
		t.Fatalf("rollback arena lengths=%d/%d, want %d/%d", len(fixture.core.nodes), len(fixture.core.links), beforeNodes, beforeLinks)
	}
	if _, err := fixture.core.reductionLinkChainForHead(escaped); err == nil {
		t.Fatal("rolled-back reduction output head remained readable")
	}
	if _, err := fixture.core.reductionLinkChainForHead(Head{Node: escaped.Node + 100}); err == nil {
		t.Fatal("out-of-bounds reduction output head was accepted")
	}

	validID, err := fixture.core.appendNode(nodeRecord{
		state: 3, byteOffset: 2, firstLink: uint32(fixture.rangeValue.First),
		linkCount: fixture.rangeValue.Count, pathCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.core.reductionLinkChainForHead(Head{Node: validID}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.core.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.core.reductionLinkChainForHead(Head{Node: validID}); err == nil {
		t.Fatal("reset retained a reduction handoff head")
	}
}

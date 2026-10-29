package parsercorephase0

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestAppendAdjacencyNodeRejectsNonDecreasingPredecessorWithoutMutation(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, compact, shallowPayloadSpec{symbol: 20, endByte: 1})
	before, err := compact.Stats(seed)
	if err != nil {
		t.Fatal(err)
	}
	beforeWork := compact.Work()
	next := NodeID(compact.nodes.Len() + 1)
	if _, err := compact.appendAdjacencyNode(2, 1, []linkRecord{{prev: next, payload: payload}}); err == nil || !strings.Contains(err.Error(), "must be lower than new node") {
		t.Fatalf("non-decreasing predecessor error=%v", err)
	}
	after, err := compact.Stats(seed)
	if err != nil {
		t.Fatal(err)
	}
	if after != before || compact.Work() != beforeWork {
		t.Fatalf("rejected publication mutated storage/work: before=%+v after=%+v work=%+v/%+v", before, after, compact.Work(), beforeWork)
	}
}

func TestAppendNodeAuthenticatesCompleteAdjacency(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{})
	if _, err := compact.Seed(1, 0); err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, compact, shallowPayloadSpec{symbol: 20, endByte: 1})

	// appendNode is the single publication point, so even diagnostic callers
	// that inject raw arena records cannot publish an upward/self edge.
	compact.links.AppendValues(linkRecord{prev: NodeID(compact.nodes.Len() + 1), payload: payload})
	beforeNodes := compact.nodes.Len()
	if _, err := compact.appendNode(nodeInput{
		state: 2, byteOffset: 1, firstLink: uint32(compact.links.Len()), linkCount: 1, pathCount: 1,
	}); err == nil || !strings.Contains(err.Error(), "invalid node id 2") {
		t.Fatalf("raw publication error=%v", err)
	}
	if compact.nodes.Len() != beforeNodes {
		t.Fatalf("rejected raw publication changed node arena: %d -> %d", beforeNodes, compact.nodes.Len())
	}
	if _, err := compact.appendNode(nodeInput{state: 2, byteOffset: 1, firstLink: 1, pathCount: 1}); err == nil || !strings.Contains(err.Error(), "empty adjacency") {
		t.Fatalf("empty adjacency error=%v", err)
	}
}

func TestPopPathsRejectsCorruptedNonDecreasingEdgeWithoutVisitingMap(t *testing.T) {
	compact, head := newReductionLinkCollisionFixture(t, false)
	node, err := compact.node(head.Node)
	if err != nil {
		t.Fatal(err)
	}
	linkID := LinkID(node.firstLink)
	original := compact.links.At(int(linkID - 1)).prev
	{
		linkStorageMutation := compact.links.At(int(linkID - 1))
		linkStorageMutation.prev = head.Node
		compact.links.Set(int(linkID-1), linkStorageMutation)
	}
	if _, err := compact.popPaths(head.Node, 2); err == nil || !strings.Contains(err.Error(), "predecessor does not decrease") {
		t.Fatalf("corrupt graph error=%v", err)
	}
	if compact.popScratch.busy || len(compact.popScratch.rev) != 0 || len(compact.popScratch.revScores) != 0 || len(compact.popScratch.revOrders) != 0 || len(compact.popScratch.trailing) != 0 || len(compact.popScratch.paths) != 0 {
		t.Fatalf("failed graph traversal retained logical scratch: %+v", compact.popScratch)
	}
	{
		linkStorageMutation := compact.links.At(int(linkID - 1))
		linkStorageMutation.prev = original
		compact.links.Set(int(linkID-1), linkStorageMutation)
	}
	if paths, err := compact.popPaths(head.Node, 2); err != nil || len(paths) != 2 {
		t.Fatalf("clean retry paths=%+v err=%v", paths, err)
	}
}

func TestDAGInvariantSurvivesRollbackAndNodeIDReuse(t *testing.T) {
	compact := newTinyCoreWithLimits(t, Limits{})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, compact, shallowPayloadSpec{symbol: 20, endByte: 1})
	sentinel := errors.New("rollback")
	var rolledBackID NodeID
	err = compact.ApplyAtomic(func() error {
		rolledBackID, err = compact.appendAdjacencyNode(2, 1, []linkRecord{{prev: seed.Node, payload: payload}})
		if err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error=%v", err)
	}
	reusedID, err := compact.appendAdjacencyNode(2, 1, []linkRecord{{prev: seed.Node, payload: payload}})
	if err != nil {
		t.Fatal(err)
	}
	if reusedID != rolledBackID || seed.Node >= reusedID {
		t.Fatalf("node ID reuse broke DAG order: seed=%d rolled-back=%d reused=%d", seed.Node, rolledBackID, reusedID)
	}
}

func TestPublishedAdjacencySpillsBeyondInlineWidth(t *testing.T) {
	limit := uint32(inlineAdjacencyCapacity + 4)
	compact := newTinyCoreWithLimits(t, Limits{MaxLinksPerBoundary: limit})
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	links := make([]linkRecord, inlineAdjacencyCapacity+1)
	wantPayloads := make([]SubtreeID, len(links))
	for index := range links {
		payload := appendShallowPayload(t, compact, shallowPayloadSpec{
			symbol: Symbol(20 + index), productionID: uint16(index + 1), endByte: 1,
		})
		wantPayloads[index] = payload
		links[index] = linkRecord{prev: seed.Node, payload: payload}
	}
	id, err := compact.appendAdjacencyNode(2, 1, links)
	if err != nil {
		t.Fatal(err)
	}
	node, err := compact.node(id)
	if err != nil {
		t.Fatal(err)
	}
	var inline [inlineAdjacencyCapacity]linkRecord
	got, err := compact.publishedNodeLinksInto(inline[:0], *node)
	if err != nil {
		t.Fatal(err)
	}
	gotPayloads := make([]SubtreeID, len(got))
	for index := range got {
		gotPayloads[index] = got[index].payload
	}
	if !slices.Equal(gotPayloads, wantPayloads) || len(got) <= len(inline) {
		t.Fatalf("spilled adjacency payloads=%v want=%v len=%d inline=%d", gotPayloads, wantPayloads, len(got), len(inline))
	}

	key := compact.boundaryKey(2, 1)
	compact.writeBoundary(key, id)
	in := linkInput{prev: seed.Node, payload: wantPayloads[len(wantPayloads)-1]}
	exists, err := compact.condenseInputExists(key, in)
	if err != nil || !exists {
		t.Fatalf("spilled adjacency exact input exists=%t err=%v", exists, err)
	}
	outcome, err := compact.condenseWithOutcome(key, in)
	if err != nil || outcome.head.Node != id || outcome.change != condenseUnchanged {
		t.Fatalf("spilled adjacency replay=%+v err=%v", outcome, err)
	}
}

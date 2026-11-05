package parsercorephase0

import (
	"math"
	"strings"
	"testing"
)

func TestPrecedenceMaximumCertificateValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		score int64
	}{
		{name: "positive", score: 7},
		{name: "zero", score: 0},
		{name: "negative", score: -7},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := newTinyCoreWithLimits(t, Limits{})
			seed, err := core.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			seedRecord, err := core.nodePrecedenceMaximum(seed.Node)
			if err != nil {
				t.Fatal(err)
			}
			if seedRecord.value != 0 {
				t.Fatalf("seed certificate=%+v, want zero", seedRecord)
			}
			payload := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: 20, startByte: 0, endByte: 1, childSymbols: []Symbol{30},
			})
			child, err := core.appendAdjacencyNode(2, 1, []linkRecord{{
				prev: seed.Node, payload: payload, scoreDelta: test.score,
			}})
			if err != nil {
				t.Fatal(err)
			}
			record, err := core.nodePrecedenceMaximum(child)
			if err != nil {
				t.Fatal(err)
			}
			if record.value != test.score {
				t.Fatalf("certificate=%+v, want %d", record, test.score)
			}
		})
	}
}

func TestPrecedenceMaximumLeafContributionIsZero(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, startByte: 0, endByte: 1})
	child, err := core.appendAdjacencyNode(2, 1, []linkRecord{{
		prev: seed.Node, payload: payload, scoreDelta: math.MaxInt64,
	}})
	if err != nil {
		t.Fatal(err)
	}
	record, err := core.nodePrecedenceMaximum(child)
	if err != nil {
		t.Fatal(err)
	}
	if record.value != 0 {
		t.Fatalf("leaf certificate=%+v, want zero contribution", record)
	}
}

func TestPrecedenceMaximumMissingCertificateFailsClosed(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	if _, err := core.computePrecedenceMaximum(nil, precedenceMaximumWitness{}); err == nil || !strings.Contains(err.Error(), "missing precedence maximum certificate") {
		t.Fatalf("missing certificate error=%v", err)
	}
	left, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := core.mergePredecessorsBounded(left.Node, right.Node, 0, nil); err == nil || !strings.Contains(err.Error(), "missing folded precedence maximum witness") {
		t.Fatalf("missing folded witness error=%v", err)
	}
}

func TestPrecedenceMaximumOverflowFailsClosed(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, startByte: 0, endByte: 1, childSymbols: []Symbol{30}})
	maximum, err := core.appendAdjacencyNode(2, 1, []linkRecord{{
		prev: seed.Node, payload: payload, scoreDelta: math.MaxInt64,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.appendAdjacencyNode(3, 2, []linkRecord{{
		prev: maximum, payload: payload, scoreDelta: 1,
	}}); err == nil || !strings.Contains(err.Error(), "precedence maximum overflow") {
		t.Fatalf("overflow error=%v", err)
	}
}

func TestPrecedenceMaximumDiscardedIncomingEdgeIsRetained(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	root, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	incumbent := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 1, startByte: 0, endByte: 1, childSymbols: []Symbol{30},
	})
	incoming := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 2, startByte: 0, endByte: 1, childSymbols: []Symbol{31},
	})
	left, err := core.appendAdjacencyNode(7, 0, []linkRecord{{
		prev: root.Node, payload: incumbent, scoreDelta: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAdjacencyNode(7, 0, []linkRecord{{
		prev: root.Node, payload: incoming, scoreDelta: 9,
	}})
	if err != nil {
		t.Fatal(err)
	}
	incumbentTop := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 30, productionID: 1, startByte: 1, endByte: 2, childSymbols: []Symbol{40},
	})
	incomingTop := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 30, productionID: 2, startByte: 1, endByte: 2, childSymbols: []Symbol{41},
	})
	key := core.boundaryKey(8, 2)
	oldTop, err := core.condense(key, linkInput{prev: left, payload: incumbentTop, scoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := core.condense(key, linkInput{prev: right, payload: incomingTop, scoreDelta: 9})
	if err != nil {
		t.Fatal(err)
	}
	if merged == oldTop {
		t.Fatal("discarded incoming maximum did not publish a certified replacement")
	}
	oldRecord, err := core.node(oldTop.Node)
	if err != nil {
		t.Fatal(err)
	}
	newRecord, err := core.node(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	if storedNodeMaximumForTest(t, core, oldTop.Node) != 2 || storedNodeMaximumForTest(t, core, merged.Node) != 18 {
		t.Fatalf("old/new certificates=%+v/%+v, want 2/18", oldRecord, newRecord)
	}
	links, err := core.nodeLinks(*newRecord)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].prev == right {
		t.Fatalf("replacement links=%#v, want the incumbent lower edge", links)
	}
}

func TestPrecedenceMaximumReplacementThenLaterCandidateUsesOrder(t *testing.T) {
	for _, test := range []struct {
		name       string
		laterScore int64
		want       int64
	}{
		{name: "higher later candidate", laterScore: 9, want: 9},
		{name: "lower later candidate", laterScore: 3, want: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := newTinyCoreWithLimits(t, Limits{})
			root, err := core.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			incumbent := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: 20, productionID: 1, startByte: 0, endByte: 1, childSymbols: []Symbol{30},
			})
			replacement := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: 20, productionID: 2, startByte: 0, endByte: 1, childSymbols: []Symbol{31},
			})
			preReplacementSibling := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: 21, startByte: 0, endByte: 1, childSymbols: []Symbol{32},
			})
			later := appendShallowPayload(t, core, shallowPayloadSpec{
				symbol: 22, startByte: 0, endByte: 1, childSymbols: []Symbol{33},
			})
			left, err := core.appendAdjacencyNode(7, 0, []linkRecord{
				{prev: root.Node, payload: incumbent, scoreDelta: 1},
				{prev: root.Node, payload: preReplacementSibling, scoreDelta: 100},
			})
			if err != nil {
				t.Fatal(err)
			}
			right, err := core.appendAdjacencyNode(7, 0, []linkRecord{
				{prev: root.Node, payload: replacement, scoreDelta: 5},
				{prev: root.Node, payload: later, scoreDelta: test.laterScore},
			})
			if err != nil {
				t.Fatal(err)
			}
			merged, changed, err := core.mergePredecessorsBounded(left, right, 0, &precedenceMaximumWitness{})
			if err != nil {
				t.Fatal(err)
			}
			if !changed {
				t.Fatal("ordered replacement did not publish a new node")
			}
			record, err := core.nodePrecedenceMaximum(merged)
			if err != nil {
				t.Fatal(err)
			}
			if record.value != test.want {
				t.Fatalf("ordered precedence maximum=%d, want %d", record.value, test.want)
			}
		})
	}
}

func TestPrecedenceMaximumNestedOuterNegativeRecomputesFromMergedPredecessor(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	base, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	innerIncumbent := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 1, startByte: 0, endByte: 1, childSymbols: []Symbol{30},
	})
	innerReplacement := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 2, startByte: 0, endByte: 1, childSymbols: []Symbol{31},
	})
	leftPredecessor, err := core.appendAdjacencyNode(7, 1, []linkRecord{{
		prev: base.Node, payload: innerIncumbent, scoreDelta: 9,
	}})
	if err != nil {
		t.Fatal(err)
	}
	rightPredecessor, err := core.appendAdjacencyNode(7, 1, []linkRecord{{
		prev: base.Node, payload: innerReplacement, scoreDelta: 10,
	}})
	if err != nil {
		t.Fatal(err)
	}
	outerPayload := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 30, startByte: 1, endByte: 2, childSymbols: []Symbol{40},
	})
	key := core.boundaryKey(8, 2)
	old, err := core.condense(key, linkInput{
		prev: leftPredecessor, payload: outerPayload, scoreDelta: -20,
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := core.condense(key, linkInput{
		prev: rightPredecessor, payload: outerPayload, scoreDelta: -20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged == old {
		t.Fatal("nested outer merge did not publish a fresh boundary")
	}
	record, err := core.nodePrecedenceMaximum(merged.Node)
	if err != nil {
		t.Fatal(err)
	}
	if record.value != -10 {
		t.Fatalf("nested outer precedence maximum=%d, want -10", record.value)
	}
}

func TestPrecedenceMaximumReplacementThenNestedNegativeUsesOuterCandidate(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{MaxDerivations: 8, MaxPopPaths: 8})
	base, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	directIncumbent := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 1, startByte: 0, endByte: 1, childSymbols: []Symbol{30},
	})
	directReplacement := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 20, productionID: 2, startByte: 0, endByte: 1, childSymbols: []Symbol{31},
	})
	innerIncumbent := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 21, productionID: 3, startByte: 0, endByte: 1, childSymbols: []Symbol{40},
	})
	innerReplacement := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 21, productionID: 4, startByte: 0, endByte: 1, childSymbols: []Symbol{41},
	})
	leftPredecessor, err := core.appendAdjacencyNode(7, 1, []linkRecord{{
		prev: base.Node, payload: innerIncumbent, scoreDelta: 9,
	}})
	if err != nil {
		t.Fatal(err)
	}
	rightPredecessor, err := core.appendAdjacencyNode(7, 1, []linkRecord{{
		prev: base.Node, payload: innerReplacement, scoreDelta: 11,
	}})
	if err != nil {
		t.Fatal(err)
	}
	outerPayload := appendShallowPayload(t, core, shallowPayloadSpec{
		symbol: 30, startByte: 1, endByte: 2, childSymbols: []Symbol{50},
	})
	leftLinks := []linkRecord{
		{prev: base.Node, payload: directIncumbent, scoreDelta: 1},
		{prev: leftPredecessor, payload: outerPayload, scoreDelta: -20},
	}
	rightLinks := []linkRecord{
		{prev: base.Node, payload: directReplacement, scoreDelta: 5},
		{prev: rightPredecessor, payload: outerPayload, scoreDelta: -20},
	}
	left, err := core.appendAdjacencyNode(8, 2, leftLinks)
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAdjacencyNode(8, 2, rightLinks)
	if err != nil {
		t.Fatal(err)
	}
	witness := precedenceMaximumWitness{}
	merged, changed, err := core.mergePredecessorsBounded(left, right, 0, &witness)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("replacement and nested merge did not publish a fresh node")
	}
	record, err := core.nodePrecedenceMaximum(merged)
	if err != nil {
		t.Fatal(err)
	}
	if record.value != 5 {
		t.Fatalf("replacement and nested precedence maximum=%d, want 5", record.value)
	}
	if !witness.hasPostReplacement {
		t.Fatal("replacement and nested merge lost the authenticated outer post-replacement candidate")
	}
	post, err := core.linkPrecedenceMaximum(witness.postReplacement)
	if err != nil {
		t.Fatal(err)
	}
	if post.value != -9 {
		t.Fatalf("post-replacement candidate=%d, want outer candidate -9", post.value)
	}
}

func TestPrecedenceMaximumOverflowRollsBackCondense(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	seed, err := core.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload := appendShallowPayload(t, core, shallowPayloadSpec{symbol: 20, startByte: 0, endByte: 1, childSymbols: []Symbol{30}})
	first, err := core.condense(core.boundaryKey(2, 1), linkInput{
		prev: seed.Node, payload: payload, scoreDelta: math.MaxInt64,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeNodes, beforeLinks, beforeWork := core.nodes.Len(), core.links.Len(), core.Work()
	if _, err := core.condense(core.boundaryKey(3, 2), linkInput{
		prev: first.Node, payload: payload, scoreDelta: 1,
	}); err == nil || !strings.Contains(err.Error(), "precedence maximum overflow") {
		t.Fatalf("rollback overflow error=%v", err)
	}
	if core.nodes.Len() != beforeNodes || core.links.Len() != beforeLinks || core.Work() != beforeWork {
		t.Fatalf("overflow rollback changed graph: nodes=%d/%d links=%d/%d work=%+v/%+v", core.nodes.Len(), beforeNodes, core.links.Len(), beforeLinks, core.Work(), beforeWork)
	}
}

func newScannerPairCheckpoint(t *testing.T, core *Core, value byte) CheckpointID {
	t.Helper()
	return mustInternCheckpoint(t, core, []byte{value})
}

func TestScannerStatePairNonterminalRootsIgnoreDescendants(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	start := newScannerPairCheckpoint(t, core, 1)
	endLeft := newScannerPairCheckpoint(t, core, 2)
	endRight := newScannerPairCheckpoint(t, core, 3)
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, endLeft); err != nil {
		t.Fatal(err)
	}
	leftChild, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 20, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, endRight); err != nil {
		t.Fatal(err)
	}
	rightChild, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 21, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	left, err := core.appendSubtree(subtreeRecord{symbol: 30}, []SubtreeID{leftChild}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.appendSubtree(subtreeRecord{symbol: 31}, []SubtreeID{rightChild}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := core.subtreeScannerStatePairsEqual(left, right)
	if err != nil || !equal {
		t.Fatalf("nonterminal scanner state equal=%t err=%v, want empty-state equality", equal, err)
	}
}

func TestScannerStatePairDistinctPayloadsEqualEndState(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	leftStart := newScannerPairCheckpoint(t, core, 1)
	rightStart := newScannerPairCheckpoint(t, core, 2)
	end := newScannerPairCheckpoint(t, core, 3)
	if err := core.SetPhaseExternalTokenScannerCheckpoints(leftStart, end); err != nil {
		t.Fatal(err)
	}
	left, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 20, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(rightStart, end); err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 21, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := core.subtreeScannerStatePairsEqual(left, right)
	if err != nil || !equal {
		t.Fatalf("equal-end scanner state equal=%t err=%v, want equal", equal, err)
	}
}

func TestScannerStatePairUnequalEndStateDeclines(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	start := newScannerPairCheckpoint(t, core, 1)
	leftEnd := newScannerPairCheckpoint(t, core, 2)
	rightEnd := newScannerPairCheckpoint(t, core, 3)
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, leftEnd); err != nil {
		t.Fatal(err)
	}
	left, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 20, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(start, rightEnd); err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 20, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := core.subtreeScannerStatePairsEqual(left, right)
	if err != nil || equal {
		t.Fatalf("unequal-end scanner state equal=%t err=%v, want mismatch", equal, err)
	}
}

func TestScannerStatePairIgnoresStartStateDifference(t *testing.T) {
	core := newTinyCoreWithLimits(t, Limits{})
	leftStart := newScannerPairCheckpoint(t, core, 1)
	rightStart := newScannerPairCheckpoint(t, core, 2)
	end := newScannerPairCheckpoint(t, core, 3)
	if err := core.SetPhaseExternalTokenScannerCheckpoints(leftStart, end); err != nil {
		t.Fatal(err)
	}
	left, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 20, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.SetPhaseExternalTokenScannerCheckpoints(rightStart, end); err != nil {
		t.Fatal(err)
	}
	right, err := core.appendAuthenticatedTerminal(subtreeRecord{symbol: 20, external: true, terminal: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := core.subtreeScannerStatePairsEqual(left, right)
	if err != nil || !equal {
		t.Fatalf("start-only scanner difference equal=%t err=%v, want equal", equal, err)
	}
}

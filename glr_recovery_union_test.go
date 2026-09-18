package gotreesitter

import (
	"reflect"
	"testing"
)

func recoveryUnionFixture() (*glrMergeScratch, func() *gssNode) {
	p := &Parser{errorCostCompetition: true, cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize)}
	p.beginCNodeMemoEpoch()
	scratch := &glrMergeScratch{parser: p}
	missing := func() *gssNode {
		n := &Node{symbol: 1, flags: nodeFlagMissing | nodeFlagHasError}
		return &gssNode{entry: newStackEntryNode(7, n), depth: 1}
	}
	return scratch, missing
}

func TestGSSRecoveryUnionCostPaths(t *testing.T) {
	scratch, missing := recoveryUnionFixture()
	a, b := missing(), missing()
	if !gssRecoveryCostsEqual(scratch, nil, a, b) {
		t.Fatal("equal missing costs rejected")
	}
	b.prev = missing()
	if gssRecoveryCostsEqual(scratch, nil, a, b) {
		t.Fatal("unequal cumulative costs admitted")
	}
	a.appendExtraLink(gssMainLink{entry: newStackEntryNode(7, &Node{symbol: 2})})
	if gssRecoveryCostsEqual(scratch, nil, a, a) {
		t.Fatal("nonuniform paths admitted")
	}
}

func TestGSSRecoveryUnionVirtualLinkRejectsWithoutMutation(t *testing.T) {
	scratch, missing := recoveryUnionFixture()
	a, b := missing(), missing()
	beforeA, beforeB := *a, *b
	preflight := &gssMainPreflight{scratch: scratch, virtualLink: map[*gssNode][]gssMainLink{
		a: {{entry: newStackEntryNode(7, &Node{symbol: 2})}},
	}}
	if gssRecoveryCostsEqual(scratch, preflight, a, b) {
		t.Fatal("virtual zero-cost alternative admitted")
	}
	if !reflect.DeepEqual(beforeA, *a) || !reflect.DeepEqual(beforeB, *b) {
		t.Fatal("rejected proof mutated physical links")
	}
	if len(preflight.virtualLink[a]) != 1 {
		t.Fatal("rejected proof changed virtual links")
	}
}

func TestGSSRecoveryUnionProofBounds(t *testing.T) {
	scratch, missing := recoveryUnionFixture()
	t.Run("cycle", func(t *testing.T) {
		a := missing()
		a.prev = a
		if gssRecoveryCostsEqual(scratch, nil, a, missing()) {
			t.Fatal("cycle admitted")
		}
	})
	t.Run("depth", func(t *testing.T) {
		a := missing()
		for i := 0; i < 128; i++ {
			a = &gssNode{entry: stackEntry{state: 7}, prev: a}
		}
		if gssRecoveryCostsEqual(scratch, nil, a, a) {
			t.Fatal("proof beyond depth bound admitted")
		}
	})
	t.Run("uncached_payload", func(t *testing.T) {
		n := &Node{symbol: errorSymbol, flags: nodeFlagHasError}
		for i := 0; i < 1024; i++ {
			n = &Node{symbol: 2, flags: nodeFlagHasError, children: []*Node{n}}
		}
		a := &gssNode{entry: newStackEntryNode(7, n)}
		before := append([]cNodeMemoCacheEntry(nil), scratch.parser.cNodeMemoCache...)
		if gssRecoveryCostsEqual(scratch, nil, a, a) {
			t.Fatal("uncached payload admitted")
		}
		if !reflect.DeepEqual(before, scratch.parser.cNodeMemoCache) {
			t.Fatal("proof populated the memo on a miss")
		}
	})
}

func TestGSSRecoveryUnionMemoAuthentication(t *testing.T) {
	scratch, missing := recoveryUnionFixture()
	n := &Node{symbol: errorSymbol, flags: nodeFlagHasError}
	a := &gssNode{entry: newStackEntryNode(7, n)}
	slot := scratch.parser.cNodeMemoSlot(n)
	slot.hasCost, slot.cost, slot.ver = true, cErrCostPerMissingTree+cErrCostPerRecovery, n.equivVersion
	if !gssRecoveryCostsEqual(scratch, nil, a, missing()) {
		t.Fatal("authenticated equal cost rejected")
	}
	n.equivVersion++
	if gssRecoveryCostsEqual(scratch, nil, a, missing()) {
		t.Fatal("stale node version admitted")
	}
	n.equivVersion--
	scratch.parser.cNodeMemoEpoch++
	if gssRecoveryCostsEqual(scratch, nil, a, missing()) {
		t.Fatal("stale memo epoch admitted")
	}
}

func TestGSSRecoveryUnionOwnership(t *testing.T) {
	scratch, missing := recoveryUnionFixture()
	for _, name := range []string{"accepted", "paused", "open", "missing_group", "validation"} {
		t.Run(name, func(t *testing.T) {
			a := glrStack{gss: gssStack{head: missing()}}
			b := glrStack{gss: gssStack{head: missing()}}
			switch name {
			case "accepted":
				b.accepted = true
			case "paused":
				b.cPaused = true
			case "open":
				b.cRec = &cRecoverState{}
			case "missing_group":
				b.cRecoverMissingGroup = &cRecGroup{}
			case "validation":
				b.cRecoveryUnvalidatedMarker = true
			}
			beforeA, beforeB := a, b
			if gssRecoveryStacksCanMerge(scratch, &a, &b) || gssRecoveryStacksCanMerge(scratch, &b, &a) {
				t.Fatal("incompatible recovery ownership admitted")
			}
			if !reflect.DeepEqual(beforeA, a) || !reflect.DeepEqual(beforeB, b) {
				t.Fatal("rejected proof mutated stack ownership")
			}
		})
	}
}

func TestGSSRecoveryUnionKeepsDistinctPathsAndBaseline(t *testing.T) {
	scratch, missing := recoveryUnionFixture()
	scratch.parser.mergeScratch = scratch
	a := glrStack{gss: gssStack{head: missing()}, cNodeBaseline: 3, cEverErrored: true}
	b := glrStack{gss: gssStack{head: missing()}, cNodeBaseline: 7, cEverErrored: true}
	stackEntryNode(b.gss.head.entry).symbol = 2
	if !tryGSSMainMergeForParser(scratch.parser, &a, &b) {
		t.Fatal("equal recovery paths did not merge")
	}
	if a.gss.head.linkCount() != 2 || a.cNodeBaseline != 3 {
		t.Fatal("merge lost a path or changed incumbent baseline")
	}
	if b.gss.head.linkCount() != 1 {
		t.Fatal("merge mutated donor links")
	}
}

func TestGSSRecoveryUnionRejectsEdgeBudgetAndOverflow(t *testing.T) {
	scratch, missing := recoveryUnionFixture()
	remaining := 1
	if _, ok := gssUniformRecoveryCost(scratch, nil, missing(), 0, &remaining); ok {
		t.Fatal("edge bypassed exhausted budget")
	}
	n := &Node{symbol: errorSymbol, flags: nodeFlagHasError}
	slot := scratch.parser.cNodeMemoSlot(n)
	slot.hasCost, slot.cost, slot.ver = true, ^uint32(0), n.equivVersion
	a := &gssNode{entry: newStackEntryNode(7, n), prev: missing()}
	if gssRecoveryCostsEqual(scratch, nil, a, a) {
		t.Fatal("wrapped cost admitted")
	}
}

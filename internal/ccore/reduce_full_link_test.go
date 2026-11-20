package ccore

import "testing"

type fullLinkTestScanner struct{ ExtScanner }

func TestFullLinkReduction(t *testing.T) {
	const parent Symbol = 5
	const nextState = 9
	p := nodeTestParser(t, make([]SymbolMeta, 8), nil)
	child := nodeLeaf(p, 1)
	children := []Handle{child}
	p.nodes = []stackNode{
		{state: 1},
		{state: nextState, position: Length{bytes: 1, col: 1}, linkCount: maxLinkCount},
	}
	p.heads = []stackHead{{node: 0}, {node: 1}, {node: 0}}
	resident := &p.nodes[1]
	for i := range resident.links {
		resident.links[i] = stackLink{node: 0, sub: child}
	}
	check := func(label string, want bool) {
		t.Helper()
		if got := p.wouldRejectReduction(0, len(p.heads)-1, parent, nextState, children); got != want {
			t.Fatalf("%s: wouldRejectReduction = %t, want %t", label, got, want)
		}
	}
	check("all eight links have a different symbol", true)
	// An equivalent payload can update precedence or merge predecessor links
	// even when the destination is full.
	resident.links[maxLinkCount-1].sub = p.newNode(parent, children, 0)
	check("possible equivalent payload in last slot", false)
	resident.links[maxLinkCount-1].sub = child

	// An earlier compatible head with space gets the link first.
	earlier := *resident
	earlier.linkCount--
	p.nodes = append(p.nodes, earlier)
	p.heads = []stackHead{{node: 0}, {node: 2}, {node: 1}, {node: 0}}
	check("first compatible head has space", false)
	p.heads[1].lastExternal = child
	check("unexpected checkpoint on first compatible head", false)
	p.heads[1].lastExternal = 0
	p.nodes[2].state++
	check("earlier head has different state", true)

	p.heads[2].lastExternal = child
	check("unexpected checkpoint on full resident", false)
	p.heads[2].lastExternal = 0
	p.heads[3].lastExternal = child
	check("unexpected checkpoint on candidate", false)
	p.heads[3].lastExternal = 0
	p.scanner = fullLinkTestScanner{}
	check("configured scanner outside an external mode", false)
	p.scanner = nil
	p.ext.active = true
	check("external checkpoint ownership", false)
	p.ext.active = false
	p.reuseActive = true
	check("old tree", false)
	p.reuseActive = false
	p.owner = &cell{}
	check("published store", false)
	p.owner = nil
	children[0] = p.newMissingLeaf(1, 1, Length{}, 0)
	check("missing child", false)
	children[0] = child
	p.nodes[0].position.bytes = ^uint32(0)
	check("byte extent overflows", false)
}

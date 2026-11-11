package ccore

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
)

// Differential tests of the linear reduction: each case builds two parsers with
// the same stack, runs one complete reduction (and the caller's renumber) with the
// general path on one and the linear path on the other, and compares the records,
// the child lists, the version numbering, every head field, the stack graph with
// its reference counts, and the free list.

func linTables(t *testing.T) *Tables {
	t.Helper()
	raw, err := os.ReadFile("testdata/tables/json.json")
	if err != nil {
		t.Fatal(err)
	}
	tb := &Tables{}
	if err := json.Unmarshal(raw, tb); err != nil {
		t.Fatal(err)
	}
	return tb
}

type linItem struct {
	kind  string // leaf, extra, pending, error, prec, null
	state uint16
}

type linCase struct {
	name     string
	items    []linItem
	count    uint32
	dynPrec  int32
	fragile  bool
	wantFast bool
	setup    func(p *Parser) // applied to both parsers after the stack is built
}

func linBuild(t *testing.T, tb *Tables, items []linItem) *Parser {
	p := &Parser{t: tb}
	p.clear()
	base := p.nodeNew(-1, 0, false, 1)
	p.nodes[base].refCount++
	p.heads = append(p.heads, stackHead{node: base, status: headActive})
	tok := Symbol(3)
	leaf := func() Handle {
		return p.newLeaf(tok, Length{bytes: 1, col: 1}, Length{bytes: 2, col: 2}, 1, 2, false, false, false)
	}
	for i, it := range items {
		switch it.kind {
		case "leaf":
			p.push(0, leaf(), false, it.state)
		case "extra":
			p.push(0, p.setExtra(leaf(), true), false, it.state)
		case "pending":
			p.push(0, p.newNode(Symbol(tb.TokenCount), []Handle{leaf(), leaf()}, 0), true, it.state)
		case "error":
			p.push(0, p.newNode(symError, []Handle{leaf()}, 0), false, it.state)
		case "prec":
			h := p.newNode(Symbol(tb.TokenCount), []Handle{leaf()}, 0)
			p.rec(h).dynamicPrecedence = 7
			p.push(0, h, false, it.state)
		case "null":
			p.push(0, 0, false, it.state)
		default:
			t.Fatalf("item %d: kind %q", i, it.kind)
		}
	}
	return p
}

// linNodeSig is the structure under a stack node, with reference counts.
func linGraph(p *Parser, id int32, seen map[int32]bool, out *[]string) {
	if id < 0 || seen[id] {
		return
	}
	seen[id] = true
	n := p.nodes[id]
	s := fmt.Sprintf("st=%d pos=%v cost=%d cnt=%d dyn=%d ref=%d lc=%d", n.state, n.position, n.errorCost, n.nodeCount, n.dynPrec, n.refCount, n.linkCount)
	for i := 0; i < int(n.linkCount); i++ {
		s += fmt.Sprintf(" [sub=%d pend=%t]", n.links[i].sub, n.links[i].pending)
	}
	*out = append(*out, s)
	for i := 0; i < int(n.linkCount); i++ {
		linGraph(p, n.links[i].node, seen, out)
	}
}

func linCompare(t *testing.T, name string, a, b *Parser, rvA, rvB int) {
	t.Helper()
	if rvA != rvB {
		t.Fatalf("%s: return value %d (general) against %d (linear)", name, rvA, rvB)
	}
	if !reflect.DeepEqual(a.recs, b.recs) {
		t.Fatalf("%s: subtree records differ\n general %+v\n linear  %+v", name, a.recs, b.recs)
	}
	if !reflect.DeepEqual(a.kids, b.kids) {
		t.Fatalf("%s: child lists differ: %v against %v", name, a.kids, b.kids)
	}
	if len(a.heads) != len(b.heads) {
		t.Fatalf("%s: %d heads against %d", name, len(a.heads), len(b.heads))
	}
	for i := range a.heads {
		ha, hb := a.heads[i], b.heads[i]
		if ha.status != hb.status || ha.nodeCountAtLastError != hb.nodeCountAtLastError || ha.lastExternal != hb.lastExternal ||
			ha.lookaheadWhenPaused != hb.lookaheadWhenPaused || !reflect.DeepEqual(ha.summary, hb.summary) {
			t.Fatalf("%s: head %d differs: %+v against %+v", name, i, ha, hb)
		}
		var ga, gb []string
		linGraph(a, ha.node, map[int32]bool{}, &ga)
		linGraph(b, hb.node, map[int32]bool{}, &gb)
		if !reflect.DeepEqual(ga, gb) {
			t.Fatalf("%s: stack graph of head %d differs:\n general %v\n linear  %v", name, i, ga, gb)
		}
	}
	fa, fb := append([]int32(nil), a.free...), append([]int32(nil), b.free...)
	sort.Slice(fa, func(i, j int) bool { return fa[i] < fa[j] })
	sort.Slice(fb, func(i, j int) bool { return fb[i] < fb[j] })
	if len(a.nodes) != len(b.nodes) || !reflect.DeepEqual(fa, fb) {
		t.Fatalf("%s: stack node arena differs: %d nodes, free %v against %d nodes, free %v", name, len(a.nodes), fa, len(b.nodes), fb)
	}
}

func TestLinearReductionMatchesGeneral(t *testing.T) {
	tb := linTables(t)
	nt := Symbol(tb.TokenCount)
	L := func(s uint16) linItem { return linItem{"leaf", s} }
	cases := []linCase{
		{name: "plain", items: []linItem{L(2), L(3), L(4)}, count: 2, wantFast: true},
		{name: "trailing extras", items: []linItem{L(2), {"extra", 3}, L(4), {"extra", 5}, {"extra", 6}}, count: 2, wantFast: true},
		{name: "pop ends at the base node", items: []linItem{L(2), L(3)}, count: 2, wantFast: true},
		{name: "pop reaches the base early", items: []linItem{L(2)}, count: 3},
		{name: "pending link", items: []linItem{L(2), {"pending", 3}}, count: 2, wantFast: true},
		{name: "nonzero error cost", items: []linItem{L(2), {"error", 3}, L(4)}, count: 2, wantFast: true},
		{name: "dynamic precedence", items: []linItem{L(2), {"prec", 3}, L(4)}, count: 2, dynPrec: 3, wantFast: true},
		{name: "fragile", items: []linItem{L(2), L(3), L(4)}, count: 2, fragile: true, wantFast: true},
		{name: "child count 0", items: []linItem{L(2), L(3)}, count: 0},
		{name: "null marker link", items: []linItem{L(2), {"null", 3}, L(4)}, count: 3},
		{name: "second head halted", items: []linItem{L(2), L(3), L(4)}, count: 2, setup: func(p *Parser) {
			p.nodeRetain(p.heads[0].node)
			p.heads = append(p.heads, stackHead{node: p.heads[0].node, status: headHalted})
		}},
		{name: "last external and error mark on the head", items: []linItem{L(2), L(3), L(4)}, count: 2, wantFast: true, setup: func(p *Parser) {
			p.heads[0].lastExternal = p.newLeaf(Symbol(4), Length{bytes: 1}, Length{bytes: 1}, 0, 2, false, true, false)
			p.heads[0].nodeCountAtLastError = 1
		}},
		{name: "sole paused head", items: []linItem{L(2), L(3), L(4)}, count: 2, wantFast: true, setup: func(p *Parser) {
			p.heads[0].status = headPaused
		}},
		{name: "branching node on the walk", items: []linItem{L(2), L(3), L(4)}, count: 3, setup: func(p *Parser) {
			top := p.heads[0].node
			mid := p.nodes[top].links[0].node
			other := p.nodes[mid].links[0].node
			p.addLink(mid, stackLink{node: other, sub: p.newLeaf(Symbol(5), Length{bytes: 1}, Length{bytes: 1}, 0, 2, false, false, false)})
		}},
	}
	for _, c := range cases {
		a, b := linBuild(t, tb, c.items), linBuild(t, tb, c.items)
		if c.setup != nil {
			c.setup(a)
			c.setup(b)
		}
		a.noLinear = true
		run := func(p *Parser) int {
			rv := p.reduce(0, nt, c.count, c.dynPrec, 0, c.fragile, false)
			if rv >= 0 {
				p.renumber(rv, 0) // as advance does
			}
			return rv
		}
		rvA, rvB := run(a), run(b)
		linCompare(t, c.name, a, b, rvA, rvB)
		if got := b.FastReductions(); (got == 1) != c.wantFast || got > 1 {
			t.Fatalf("%s: %d fast reductions, want fast=%t", c.name, got, c.wantFast)
		}
		if a.FastReductions() != 0 {
			t.Fatalf("%s: the general parser took the fast path", c.name)
		}
	}
}

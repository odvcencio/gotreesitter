package ccore

import (
	"maps"
	"math/rand"
	"testing"
)

// findMultiBits (fresh parses) must give the same map as findMultiWalk, which
// the incremental parse uses and which marks the records in their flags word.
// Each case builds a graph in a fresh store and compares the two.

// sweepLeaf makes a heap leaf (a size of 255 bytes or more does not fit the
// inline form); sweepInline makes an inline leaf, which is a value and not a record.
func sweepLeaf(p *Parser) Handle {
	return p.newLeaf(1, Length{}, Length{bytes: 300, col: 300}, 0, 0, false, false, false)
}

func sweepInline(p *Parser) Handle {
	return p.newLeaf(1, Length{}, Length{bytes: 1, col: 1}, 0, 0, false, false, false)
}

// bothWays runs the walk with flag marks and the walk with bit marks on the same
// finished tree and returns their maps.
func bothWays(t *testing.T, p *Parser) (walk, bitsMap map[Handle]uint32) {
	t.Helper()
	p.multi = nil
	p.findMultiWalk()
	walk = p.multi
	p.multi = nil
	if !p.findMultiBits() {
		t.Fatal("findMultiBits refused a small store")
	}
	bitsMap = p.multi
	p.multi = nil
	return
}

func sameMulti(t *testing.T, name string, walk, b map[Handle]uint32) {
	t.Helper()
	if !maps.Equal(walk, b) {
		t.Errorf("%s: the walk with flag marks found %v, the walk with bit marks %v", name, walk, b)
	}
}

func TestFindMultiBitsAgreesOnTrees(t *testing.T) {
	cases := map[string]func(p *Parser) Handle{
		"a tree": func(p *Parser) Handle {
			n := p.newNode(5, []Handle{sweepLeaf(p), sweepLeaf(p)}, 0)
			return p.newNode(6, []Handle{n, sweepLeaf(p)}, 0)
		},
		"internal node in two slots": func(p *Parser) Handle {
			x := p.newNode(5, []Handle{sweepLeaf(p), sweepLeaf(p)}, 0)
			return p.newNode(6, []Handle{x, x}, 0)
		},
		"internal node under two parents": func(p *Parser) Handle {
			x := p.newNode(5, []Handle{sweepLeaf(p), sweepLeaf(p)}, 0)
			y := p.newNode(6, []Handle{x, sweepLeaf(p)}, 0)
			return p.newNode(7, []Handle{y, x}, 0)
		},
		"heap leaf in three slots": func(p *Parser) Handle {
			l := sweepLeaf(p)
			return p.newNode(6, []Handle{l, sweepLeaf(p), l, l}, 0)
		},
		"deep node reached by two paths": func(p *Parser) Handle {
			x := p.newNode(5, []Handle{sweepLeaf(p), sweepLeaf(p)}, 0)
			g := p.newNode(5, []Handle{x, sweepLeaf(p)}, 0)
			h := p.newNode(5, []Handle{g, sweepLeaf(p)}, 0)
			return p.newNode(6, []Handle{h, x}, 0)
		},
		"inline leaf in two slots": func(p *Parser) Handle {
			l := sweepInline(p)
			return p.newNode(6, []Handle{l, l, sweepLeaf(p)}, 0)
		},
		"unreachable parent": func(p *Parser) Handle {
			x := p.newNode(5, []Handle{sweepLeaf(p), sweepLeaf(p)}, 0)
			p.newNode(9, []Handle{x, sweepLeaf(p)}, 0) // a discarded alternative
			return p.newNode(6, []Handle{x, sweepLeaf(p)}, 0)
		},
		"root is an inline leaf": func(p *Parser) Handle { return sweepInline(p) },
		"no tree":                func(p *Parser) Handle { return 0 },
	}
	for name, build := range cases {
		p := newTestParser(t)
		p.finished = build(p)
		walk, b := bothWays(t, p)
		sameMulti(t, name, walk, b)
		if name == "a tree" || name == "unreachable parent" || name == "inline leaf in two slots" {
			if len(b) != 0 {
				t.Errorf("%s: %v found in a tree", name, b)
			}
		} else if name != "root is an inline leaf" && name != "no tree" && len(b) == 0 {
			t.Errorf("%s: nothing found, the case shows a record reached twice", name)
		}
	}
}

// An edge to a record of the same or a higher slot (not made by a parse) and a
// cycle must end, with the same answer as the flag walk.
func TestFindMultiBitsAgreesOnCycles(t *testing.T) {
	p := newTestParser(t)
	x := p.newNode(5, []Handle{sweepLeaf(p), sweepLeaf(p)}, 0)
	w := p.newNode(6, []Handle{x, sweepLeaf(p)}, 0)
	p.children(x)[0] = w
	p.finished = w
	walk, b := bothWays(t, p)
	sameMulti(t, "cycle through the root", walk, b)
	p.children(x)[0] = x
	walk, b = bothWays(t, p)
	sameMulti(t, "self edge", walk, b)
}

// Over the bound the function refuses and changes nothing, and findMulti then
// takes the flag walk and gives the same map.
func TestFindMultiBitsRefusesOverTheBoundAndFindMultiFallsBack(t *testing.T) {
	p := newTestParser(t)
	x := p.newNode(5, []Handle{sweepLeaf(p), sweepLeaf(p)}, 0)
	p.finished = p.newNode(6, []Handle{x, x}, 0)
	saved := maxMarkIndex
	defer func() { maxMarkIndex = saved }()
	maxMarkIndex = 2
	p.multi = nil
	if p.findMultiBits() || p.multi != nil {
		t.Fatal("findMultiBits did not refuse over the bound")
	}
	p.findMulti()
	if p.multi[x] != 1 || len(p.multi) != 1 {
		t.Fatalf("fallback found %v, want X once", p.multi)
	}
}

// The two walks agree on random graphs with random sharing over several pages
// of records, including the number of times each record is reached.
func TestFindMultiBitsAgreesOnRandomGraphs(t *testing.T) {
	rng := rand.New(rand.NewSource(75))
	for iter := 0; iter < 300; iter++ {
		p := newTestParser(t)
		var nodes, free []Handle
		n := 20 + rng.Intn(60)
		if iter%10 == 0 {
			n = 20 + rng.Intn(2*recPageLen) // the larger cases cross a page
		}
		share := rng.Intn(3) == 0
		for i := 0; i < n; i++ {
			l := sweepLeaf(p)
			free = append(free, l)
			nodes = append(nodes, l)
			if len(free) >= 2 && rng.Intn(2) == 0 {
				k := min(2+rng.Intn(3), len(free))
				kids := append([]Handle(nil), free[len(free)-k:]...)
				free = free[:len(free)-k]
				if share && rng.Intn(15) == 0 {
					kids[rng.Intn(len(kids))] = nodes[rng.Intn(len(nodes))]
				}
				h := p.newNode(5, kids, 0)
				free = append(free, h)
				nodes = append(nodes, h)
			}
		}
		for len(free) > 1 {
			k := min(3, len(free))
			kids := append([]Handle(nil), free[len(free)-k:]...)
			free = free[:len(free)-k]
			free = append(free, p.newNode(6, kids, 0))
		}
		p.finished = free[0]
		walk, b := bothWays(t, p)
		if !maps.Equal(walk, b) {
			t.Fatalf("iteration %d: flag walk %v, bit walk %v", iter, walk, b)
		}
	}
}

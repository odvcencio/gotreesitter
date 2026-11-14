package ccore

import "testing"

func TestNodeNextParseStateRules(t *testing.T) {
	tb := &Tables{TokenCount: 3, StateCount: 3, DenseLimit: 3, InitialState: 1, Meta: make([]SymbolMeta, 5), Dense: [][]uint16{nil, {0, 1, 2, 2, 0}, {0, 0, 0, 0, 0}}, Actions: []ActionEntry{
		{}, {Actions: []Action{{Type: ActionShift, State: 2}, {Type: ActionReduce}}}, {Actions: []Action{{Type: ActionReduce}, {Type: ActionShift, State: 2}}},
	}}
	p := &Parser{t: tb}
	p.Store = newStoreCore().view()
	p.beginParse(true)
	r := NodeReader{S: &p.Store, T: &NodeTables{Base: tb, SymbolCount: 5}}
	leaf := func(sym Symbol, state uint16) Node {
		return Node{H: p.newLeaf(sym, Length{}, Length{bytes: 1, col: 1}, 0, state, false, false, false)}
	}
	if got := r.NextParseState(leaf(1, 1)); got != 0 {
		t.Fatalf("last reduce: %d", got)
	}
	tok := leaf(2, 1)
	if n := testing.AllocsPerRun(100, func() { _ = r.NextParseState(tok) }); n != 0 {
		t.Fatalf("next state allocs = %v", n)
	}
	if got := r.NextParseState(tok); got != 2 {
		t.Fatalf("last shift: %d", got)
	}
	tb.Actions[2].Actions[1].Extra = true
	if got := r.NextParseState(tok); got != 1 {
		t.Fatalf("extra shift: %d", got)
	}
	tb.Actions[2].Actions[1].Extra = false
	// Alias must not change the grammar symbol used to select the action.
	tok.Alias = 1
	if got := r.NextParseState(tok); got != 2 {
		t.Fatalf("alias changed next state: %d", got)
	}
	// Symbol metadata is optional; the declared count controls bounds.
	tb.Meta = nil
	if got := r.NextParseState(tok); got != 2 {
		t.Fatalf("absent metadata: %d", got)
	}
	tb.Meta = make([]SymbolMeta, 1)
	if got := r.NextParseState(tok); got != 2 {
		t.Fatalf("short metadata: %d", got)
	}
	r.T.SymbolCount = 2
	if got := r.NextParseState(tok); got != 0 {
		t.Fatalf("out-of-range symbol: %d", got)
	}
	r.T.SymbolCount = 0
	if got := r.NextParseState(tok); got != 0 {
		t.Fatalf("unknown symbol count: %d", got)
	}
	r.T.SymbolCount = 5
	tb.Dense[1][2] = 99
	if got := r.NextParseState(tok); got != 0 {
		t.Fatalf("invalid action index: %d", got)
	}
	tb.Dense[1][2] = 0
	tb.Actions[0].Actions = []Action{{Type: ActionShift, State: 2}}
	if got := r.NextParseState(tok); got != 0 {
		t.Fatalf("zero action index: %d", got)
	}
	tb.Dense[1][2] = 2
	tb.Actions[2].Actions[1].State = 99
	if got := r.NextParseState(tok); got != 0 {
		t.Fatalf("invalid shift target: %d", got)
	}
	tb.Actions[2].Actions[1].State = 70000
	tb.StateCount = 100000
	if got := r.NextParseState(tok); got != 0 {
		t.Fatalf("unrepresentable shift target: %d", got)
	}
	tb.Actions[2].Actions[1].State = 2
	tb.LargeStateGotos = map[uint64]uint32{uint64(1)<<32 | 3: 70000}
	wide := p.newNode(3, []Handle{tok.H}, 0)
	p.rec(wide).parseState = 1
	if got := r.NextParseState(Node{H: wide}); got != 0 {
		t.Fatalf("unrepresentable goto: %d", got)
	}
	tb.LargeStateGotos = nil
	tb.StateCount = 3
	tb.Dense[1][3] = 99
	h := p.newNode(3, []Handle{tok.H}, 0)
	p.rec(h).parseState = 1
	if got := r.NextParseState(Node{H: h}); got != 0 {
		t.Fatalf("invalid goto: %d", got)
	}
	tb.Dense[1][3] = 2
	if got := (NodeReader{}).NextParseState(Node{}); got != 0 {
		t.Fatalf("null node: %d", got)
	}
	parent := p.newNode(3, []Handle{tok.H}, 0)
	p.rec(parent).parseState = 1
	if got := r.NextParseState(Node{H: parent}); got != 2 {
		t.Fatalf("nonterminal goto: %d", got)
	}
	tb.InitialState = 0
	if got := r.NextParseState(Node{H: parent}); got != 2 {
		t.Fatalf("zero initial state direct goto: %d", got)
	}
	tb.InitialState = 1
	if got := r.NextParseState(leaf(2, stateNone)); got != stateNone {
		t.Fatalf("state none: %d", got)
	}
	if got := r.NextParseState(leaf(2, 3)); got != 0 {
		t.Fatalf("out-of-range state: %d", got)
	}
	errNode := p.newNode(symError, []Handle{tok.H}, 0)
	p.rec(errNode).parseState = 1
	if got := r.NextParseState(Node{H: errNode}); got != 0 {
		t.Fatalf("error symbol: %d", got)
	}
}

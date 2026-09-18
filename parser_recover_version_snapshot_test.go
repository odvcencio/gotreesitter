package gotreesitter

import "testing"

// C advances to the version count captured before the promoted head executes.
// Earlier siblings remain available for recovery but do not run again.
func TestCDoAllPotentialReductionsSkipsEarlierSiblingsAfterPromotion(t *testing.T) {
	lang := &Language{
		TokenCount: 2, StateCount: 7, SymbolCount: 6,
		ParseTable: [][]uint16{
			nil,
			{0, 0, 0, 2, 3, 4},
			{0, 1},
			{0, 5},
			{0, 6},
			nil,
			nil,
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
				{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
			}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 5}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 5, ChildCount: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 6}}},
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: true},
			{Name: "token", Visible: true},
			{Name: "leaf", Visible: true, Named: true},
			{Name: "earlier", Visible: true, Named: true},
			{Name: "promoted", Visible: true, Named: true},
			{Name: "unvisited", Visible: true, Named: true},
		},
	}
	p := &Parser{language: lang, denseLimit: len(lang.ParseTable)}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	leaf := newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	start := newGLRStack(1)
	start.pushEntry(newStackEntryNode(2, leaf), nil, nil)
	var allocated int
	versions, canShift, reason := p.cDoAllPotentialReductions([]byte("x"), start, 0, true, Token{}, &allocated, arena, nil, nil, nil, nil, nil)
	if reason != ParseStopNone || !canShift || len(versions) != 2 {
		t.Fatalf("versions=%d canShift=%t reason=%v", len(versions), canShift, reason)
	}
	for i, want := range []struct {
		state  StateID
		symbol Symbol
	}{{4, 4}, {3, 3}} {
		top := versions[i].top()
		if top.state != want.state || !stackEntryHasNode(top) || stackEntryNodeSymbol(top) != want.symbol {
			t.Fatalf("version %d state=%d symbol=%d; want state=%d symbol=%d", i, top.state, stackEntryNodeSymbol(top), want.state, want.symbol)
		}
	}
	if allocated != 2 {
		t.Fatalf("constructed %d parents; want only the two initial reductions", allocated)
	}
}

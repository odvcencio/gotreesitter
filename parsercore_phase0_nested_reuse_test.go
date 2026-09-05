//go:build !gts_no_parsercorephase0

package gotreesitter

import "testing"

func TestCompactNestedReuseCandidateProofs(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Parser, *compactIncrementalReuseSession, *Node, *Token)
	}{
		{"clean", nil},
		{"unknown_dependency", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { clearCompactReuseDependency(n) }},
		{"dependency_beyond_source", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { setCompactReuseDependency(n, 4) }},
		{"changed_reduction_lookahead", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { setCompactReuseDependency(n, 2) }},
		{"dirty_node", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.setDirty(true) }},
		{"fragile_node", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.setFragileLeft(true) }},
		{"fragile_parent", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parent.setFragileRight(true) }},
		{"error_parent", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parent.symbol = errorSymbol }},
		{"missing_parent", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parent.setMissing(true) }},
		{"clean_parent", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parent.setDirty(false) }},
		{"deeper_parent", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parent.parent = &Node{} }},
		{"unproven_pre_state", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) {
			n.setCompactPreGotoStateProof(false)
		}},
		{"unproven_state", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) {
			n.setCompactParseStateProof(false)
		}},
		{"wrong_pre_state", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.preGotoState++ }},
		{"wrong_goto", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parseState++ }},
		{"hidden", func(p *Parser, _ *compactIncrementalReuseSession, _ *Node, _ *Token) {
			p.language.SymbolMetadata[3].Visible = false
		}},
		{"terminal", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.symbol = 1 }},
		{"childless", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.children = nil }},
		{"token_symbol", func(_ *Parser, _ *compactIncrementalReuseSession, _ *Node, token *Token) { token.Symbol++ }},
		{"token_start", func(_ *Parser, _ *compactIncrementalReuseSession, _ *Node, token *Token) { token.StartByte++ }},
		{"token_end", func(_ *Parser, _ *compactIncrementalReuseSession, _ *Node, token *Token) { token.EndByte++ }},
		{"aliased_leaf", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.children[0].symbol = 5 }},
		{"right_boundary", func(_ *Parser, s *compactIncrementalReuseSession, _ *Node, _ *Token) {
			s.cursor.edits = []InputEdit{{StartByte: 1, OldEndByte: 1, NewEndByte: 2}}
		}},
		{"changed_bytes", func(_ *Parser, s *compactIncrementalReuseSession, _ *Node, _ *Token) {
			s.cursor.newSource = []byte("b y")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, session, node, _, _, _ := compactBorrowedMaterializationFixture(t)
			p.language.InitialState, p.language.LargeStateCount = 1, 8
			p.language.StateCount = 8
			p.language.ParseTable = [][]uint16{nil, nil, nil, nil, {0, 0, 0, 7}}
			p.denseLimit = 8
			if !setCompactReuseDependency(node, 0) {
				t.Fatal("fixture dependency was not authenticated")
			}
			parent := session.oldTree.root
			parent.endByte = 3
			parent.setDirty(true)
			session.oldTree.root = newParentNodeInArena(node.ownerArena, 4, true, []*Node{parent}, nil, 0)
			session.oldTree.source = []byte("a x")
			session.oldTree.edits = []InputEdit{{StartByte: 2, OldEndByte: 3, NewEndByte: 3}}
			session.cursor.reset(session.oldTree, []byte("a y"), nil)
			token := Token{Symbol: 1, StartByte: 0, EndByte: 1}
			if test.change != nil {
				test.change(p, session, node, &token)
			}
			state, ok := session.candidateState(p, node, 4, 0, token)
			if want := test.change == nil; ok != want || (ok && state != 7) {
				t.Fatalf("candidate state=%d accepted=%t, want accepted=%t", state, ok, want)
			}
		})
	}
}

func TestCompactNestedReuseRejectsProjectionRewrite(t *testing.T) {
	p, _, node, _, _, _ := compactBorrowedMaterializationFixture(t)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	entries := []stackEntry{newStackEntryNode(0, node)}
	if err := validateCompactBorrowedReduceInputs(p, entries, 1, arena); err == nil {
		t.Fatal("an owning reduction aliased the borrowed nonterminal")
	}
	// An equal-span replacement cannot substitute for the borrowed identity.
	clone := newParentNodeInArenaNoLinksWithFieldSources(arena, node.symbol, true, node.children, nil, nil, 0, true)
	for _, output := range []*Node{clone, node.children[0]} {
		if err := validateCompactBorrowedReduceProjection(p, entries, []*Node{output}, arena); err == nil {
			t.Fatal("a clone or unary collapse replaced the borrowed projection")
		}
	}
}

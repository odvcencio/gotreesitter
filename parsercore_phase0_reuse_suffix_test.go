//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"errors"
	"testing"
)

func TestCompactSuffixReuseCandidateProofs(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Parser, *compactIncrementalReuseSession, *Node, *Token)
	}{
		{"clean", nil},
		{"dirty_node", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.setDirty(true) }},
		{"missing_node", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.setMissing(true) }},
		{"error_node", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.symbol = errorSymbol }},
		{"fragile_node", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.setFragileLeft(true) }},
		{"error_parent", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parent.symbol = errorSymbol }},
		{"missing_parent", func(_ *Parser, _ *compactIncrementalReuseSession, n *Node, _ *Token) { n.parent.setMissing(true) }},
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
			s.cursor.edits = []InputEdit{{StartByte: 2, OldEndByte: 2, NewEndByte: 3}}
		}},
		{"changed_later_lookahead", func(_ *Parser, s *compactIncrementalReuseSession, _ *Node, _ *Token) {
			s.cursor.newSource = []byte("ya?")
		}},
		{"changed_bytes", func(_ *Parser, s *compactIncrementalReuseSession, _ *Node, _ *Token) {
			s.cursor.newSource = []byte("yb!")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, session, node, _, _, _ := compactBorrowedMaterializationFixture(t)
			p.language.InitialState, p.language.LargeStateCount = 1, 8
			p.language.StateCount = 8
			p.language.ParseTable = [][]uint16{nil, nil, nil, nil, {0, 0, 0, 7}}
			p.denseLimit = 8
			parent := session.oldTree.root
			parent.setFragileRight(true)
			wrapper := newParentNodeInArena(node.ownerArena, 4, true, []*Node{parent}, nil, 0)
			session.oldTree.root = newParentNodeInArena(node.ownerArena, 4, true, []*Node{wrapper}, nil, 0)
			node.startByte, node.endByte = 1, 2
			node.children[0].startByte, node.children[0].endByte = 1, 2
			session.oldTree.source = []byte("xa!")
			session.oldTree.edits = []InputEdit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1}}
			session.cursor.reset(session.oldTree, []byte("ya!"), nil)
			session.scheduler = &diagnosticParserCoreGenericScheduler{tokenSource: &dfaTokenSource{language: p.language}}
			token := Token{Symbol: 1, StartByte: 1, EndByte: 2}
			if test.change != nil {
				test.change(p, session, node, &token)
			}
			if err := session.prepareSuffixProof(nil); err != nil {
				t.Fatal(err)
			}
			state, ok := session.candidateState(p, node, 4, 1, token)
			if want := test.change == nil; ok != want || (ok && state != 7) {
				t.Fatalf("candidate state=%d accepted=%t, want accepted=%t", state, ok, want)
			}
		})
	}
}

func TestCompactSuffixProofSourceGuards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*compactIncrementalReuseSession)
	}{
		{"old_ranges", func(s *compactIncrementalReuseSession) { s.oldTree.includedRanges = []Range{{EndByte: 1}} }},
		{"new_ranges", func(s *compactIncrementalReuseSession) { s.scheduler.options.includedRanges = []Range{{EndByte: 1}} }},
		{"utf16", func(s *compactIncrementalReuseSession) { s.oldTree.sourceEncoding = InputEncodingUTF16 }},
		{"prefix_wrapper", func(s *compactIncrementalReuseSession) { s.scheduler.tokenSource.isBash = true }},
		{"scanner", func(s *compactIncrementalReuseSession) { s.scheduler.tokenSource.hasExternalScanner = true }},
		{"zero_width", func(s *compactIncrementalReuseSession) { s.scheduler.tokenSource.hasZeroWidthSentinelSymbol = true }},
		{"angle_split", func(s *compactIncrementalReuseSession) { s.scheduler.tokenSource.language.Name = "java" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, s, _, _, _, _ := compactBorrowedMaterializationFixture(t)
			s.cursor.reset(s.oldTree, []byte("a"), nil)
			s.scheduler = &diagnosticParserCoreGenericScheduler{tokenSource: &dfaTokenSource{language: p.language}}
			tc.change(s)
			if err := s.prepareSuffixProof(nil); err != nil {
				t.Fatal(err)
			}
			if !s.suffixReady || s.suffixValid {
				t.Fatal("unsupported source received a suffix proof")
			}
		})
	}
}

func TestCompactSuffixProofMappingAndEOF(t *testing.T) {
	for _, tc := range []struct {
		name, old, new string
		start, end     uint32
		edits          []InputEdit
		want           bool
	}{
		{"same_width", "xa!", "ya!", 1, 2, []InputEdit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1}}, true},
		{"eof", "xa", "ya", 1, 2, []InputEdit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1}}, true},
		{"insert", "xa!", "yya!", 2, 3, []InputEdit{{StartByte: 0, OldEndByte: 1, NewEndByte: 2}}, true},
		{"delete", "xxa!", "ya!", 1, 2, []InputEdit{{StartByte: 0, OldEndByte: 2, NewEndByte: 1}}, true},
		{"multiple", "xxa!", "zzza!", 3, 4, []InputEdit{{StartByte: 0, OldEndByte: 1, NewEndByte: 2}, {StartByte: 0, OldEndByte: 3, NewEndByte: 3}}, true},
		{"repeated_wrong_mapping", "aaa!", "aaaa!", 2, 3, nil, false},
		{"changed_after_node", "xa!", "ya?", 1, 2, []InputEdit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1}, {StartByte: 2, OldEndByte: 3, NewEndByte: 3}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, s, n, _, _, _ := compactBorrowedMaterializationFixture(t)
			s.oldTree.source = []byte(tc.old)
			s.oldTree.edits = tc.edits
			n.startByte, n.endByte = tc.start, tc.end
			s.cursor.reset(s.oldTree, []byte(tc.new), nil)
			s.scheduler = &diagnosticParserCoreGenericScheduler{tokenSource: &dfaTokenSource{language: p.language}}
			if err := s.prepareSuffixProof(nil); err != nil {
				t.Fatal(err)
			}
			if got := s.suffixNodeEligible(p, n); got != tc.want {
				t.Fatalf("eligible=%t want=%t suffix=%d:%d", got, tc.want, s.suffixOldStart, s.suffixNewStart)
			}
		})
	}
}

func TestCompactSuffixProofCancellationAndCache(t *testing.T) {
	p, s, _, _, _, _ := compactBorrowedMaterializationFixture(t)
	s.oldTree.source = bytes.Repeat([]byte("a"), 16384)
	s.cursor.reset(s.oldTree, append([]byte(nil), s.oldTree.source...), nil)
	s.scheduler = &diagnosticParserCoreGenericScheduler{tokenSource: &dfaTokenSource{language: p.language}}
	stop := errors.New("stop suffix scan")
	calls := 0
	err := s.prepareSuffixProof(func() error {
		calls++
		if calls == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || s.suffixReady || s.suffixValid {
		t.Fatal("canceled proof was published")
	}
	calls = 0
	if err := s.prepareSuffixProof(func() error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 5 || !s.suffixValid {
		t.Fatalf("successful scan polls=%d valid=%t", calls, s.suffixValid)
	}
	if err := s.prepareSuffixProof(func() error { t.Fatal("cached proof rescanned"); return stop }); err != nil {
		t.Fatal(err)
	}
}

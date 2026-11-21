package ccore

import (
	"reflect"
	"strings"
	"testing"
)

// TestSelectChildrenBuildsNoRecord checks selectChildren against the old
// form, which built a persistent comparison node and called selectTree
// (C/parser.c:889-912 compares a scratch tree that owns no subtree). For
// competing child lists that make the candidate win and lose, by error cost,
// dynamic precedence, symbol order, child count, a difference below the first
// level, and equality, the decision and the log lines must be the same, and
// the new form must claim no record, no child list and no node count.
func TestSelectChildrenBuildsNoRecord(t *testing.T) {
	tb := linTables(t)
	nt := Symbol(tb.TokenCount) // a nonterminal
	p := linBuild(t, tb, nil)
	leaf := func(sym Symbol) Handle {
		return p.newLeaf(sym, Length{bytes: 1, col: 1}, Length{bytes: 2, col: 2}, 1, 2, false, false, false)
	}
	node := func(sym Symbol, kids ...Handle) Handle { return p.newNode(sym, kids, 0) }
	prec := func(h Handle, v int32) Handle { p.rec(h).dynamicPrecedence = v; return h }
	errNode := func() Handle { return node(symError, leaf(3)) }

	type tc struct {
		name        string
		left        func() Handle
		cand        func() []Handle
		want        bool
		wantLogKind string
	}
	cases := []tc{
		{"candidate has the larger error cost", func() Handle { return node(nt, leaf(3), leaf(4)) }, func() []Handle { return []Handle{leaf(3), errNode()} }, false, "select_smaller_error"},
		{"left has the larger error cost", func() Handle { return node(nt, leaf(3), errNode()) }, func() []Handle { return []Handle{leaf(3), leaf(4)} }, true, "select_smaller_error"},
		{"candidate has the higher precedence", func() Handle { return node(nt, leaf(3)) }, func() []Handle { return []Handle{prec(node(nt+1, leaf(3)), 5)} }, true, "select_higher_precedence"},
		{"left has the higher precedence", func() Handle { return node(nt, prec(node(nt+1, leaf(3)), 5)) }, func() []Handle { return []Handle{leaf(3)} }, false, "select_higher_precedence"},
		{"earlier symbol in the candidate", func() Handle { return node(nt, leaf(4), leaf(3)) }, func() []Handle { return []Handle{leaf(3), leaf(3)} }, true, "select_earlier"},
		{"earlier symbol in left", func() Handle { return node(nt, leaf(3), leaf(3)) }, func() []Handle { return []Handle{leaf(4), leaf(3)} }, false, "select_earlier"},
		{"fewer children in the candidate", func() Handle { return node(nt, leaf(3), leaf(3)) }, func() []Handle { return []Handle{leaf(3)} }, true, "select_earlier"},
		{"more children in the candidate", func() Handle { return node(nt, leaf(3)) }, func() []Handle { return []Handle{leaf(3), leaf(3)} }, false, "select_earlier"},
		{"difference below the first level, candidate earlier", func() Handle { return node(nt, node(nt+1, leaf(4)), leaf(3)) }, func() []Handle { return []Handle{node(nt+1, leaf(3)), leaf(3)} }, true, "select_earlier"},
		{"difference below the first level, left earlier", func() Handle { return node(nt, node(nt+1, leaf(3)), leaf(3)) }, func() []Handle { return []Handle{node(nt+1, leaf(4)), leaf(3)} }, false, "select_earlier"},
		{"equal", func() Handle { return node(nt, node(nt+1, leaf(3)), leaf(4)) }, func() []Handle { return []Handle{node(nt+1, leaf(3)), leaf(4)} }, false, "select_existing"},
	}
	var sawWin, sawLose bool
	for _, c := range cases {
		left, kids := c.left(), c.cand()
		var oldLog, newLog []string
		before := mark(p)

		// The old form: a persistent comparison node.
		p.logf = func(s string) { oldLog = append(oldLog, s) }
		scratch := p.newNode(p.symbol(left), kids, 0)
		oldGot := p.selectTree(left, scratch)
		oldDelta := mark(p).sub(before)

		// The new form.
		mid := mark(p)
		p.logf = func(s string) { newLog = append(newLog, s) }
		calls := p.selectCandidates
		newGot := p.selectChildren(left, kids)
		newDelta := mark(p).sub(mid)
		p.logf = nil

		if newGot != oldGot || newGot != c.want {
			t.Errorf("%s: decision %v, old form %v, want %v", c.name, newGot, oldGot, c.want)
		}
		if !reflect.DeepEqual(newLog, oldLog) {
			t.Errorf("%s: log %q, old form %q", c.name, newLog, oldLog)
		}
		if len(newLog) > 0 && !strings.HasPrefix(newLog[0], c.wantLogKind) {
			t.Errorf("%s: first log line %q, want kind %s", c.name, newLog[0], c.wantLogKind)
		}
		if oldDelta.nNode != 1 || oldDelta.recs != 1 {
			t.Errorf("%s: old form built %d nodes, %d records, want 1 and 1", c.name, oldDelta.nNode, oldDelta.recs)
		}
		if newDelta.nNode != 0 || newDelta.recs != 0 || newDelta.kids != 0 {
			t.Errorf("%s: selectChildren built %d nodes, %d records, %d child slots, want none", c.name, newDelta.nNode, newDelta.recs, newDelta.kids)
		}
		if p.selectCandidates != calls+1 {
			t.Errorf("%s: selectCandidates %d, want %d", c.name, p.selectCandidates, calls+1)
		}
		sawWin = sawWin || newGot
		sawLose = sawLose || !newGot
	}
	if !sawWin || !sawLose {
		t.Fatalf("the cases must cover a winner and a loser: win %v, lose %v", sawWin, sawLose)
	}
}

type selectMark struct{ nNode, recs, kids int }

func (a selectMark) sub(b selectMark) selectMark {
	return selectMark{a.nNode - b.nNode, a.recs - b.recs, a.kids - b.kids}
}

// mark reads the counters that the test compares: nodes built, records
// handed out and child slots taken.
func mark(p *Parser) selectMark {
	return selectMark{p.nNode, int(p.recWatermark()), p.nKids}
}

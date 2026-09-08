package gotreesitter

import "testing"

func cleanCLinkCollapseFixture() (*compactPackedGSSReceiptAuditFixture, *gssNode, *gssNode) {
	f := newCompactPackedGSSReceiptAuditFixture()
	a := f.entry("node", 42, 42, 1, 5, 8, 10)
	b := f.entry("node", 42, 42, 1, 5, 8, 20)
	stackEntryNode(a).children = []*Node{newLeafNodeInArena(f.arena, 10, true, 5, 6, Point{}, Point{})}
	stackEntryNode(b).children = []*Node{newLeafNodeInArena(f.arena, 20, true, 5, 6, Point{}, Point{})}
	f.language.CompactPackedGSSVersionOrderCertified = false
	f.scratch.packedGSSVersionOrderActive = false
	f.parser.errorCostCompetition = true
	return f, &gssNode{entry: a, depth: 1}, &gssNode{entry: b, depth: 1}
}

func TestCLinkCollapsePreservesPrecedence(t *testing.T) {
	for _, higher := range []bool{false, true} {
		f, a, b := cleanCLinkCollapseFixture()
		incumbent, candidate := a.entry.node, b.entry.node
		if higher {
			stackEntryNode(b.entry).dynamicPrecedence = 1
		}
		if !cGSSCompleteLinkCollapse(f.scratch, a, b) {
			t.Fatal("equal raw headers with different descendants did not collapse")
		}
		result := []glrStack{{gss: gssStack{head: a}, byteOffset: 8}}
		incoming := glrStack{gss: gssStack{head: b}, byteOffset: 8}
		if merged, _ := tryGSSMainMergeResult(f.scratch, result, 0, &incoming); !merged {
			t.Fatal("proved collapse failed")
		}
		want := incumbent
		if higher {
			want = candidate
		}
		if a.entry.node != want || a.linkCount() != 1 || b.linkCount() != 1 {
			t.Fatalf("incorrect collapse: higher=%t links=%d", higher, a.linkCount())
		}
	}
}

func TestCLinkCollapseDeclinesWithoutProof(t *testing.T) {
	for _, cause := range []string{"mode_off", "raw_missing", "foreign_arena", "offset", "scanner", "distinct", "capacity", "nested_distinct", "budget"} {
		t.Run(cause, func(t *testing.T) {
			f, a, b := cleanCLinkCollapseFixture()
			switch cause {
			case "mode_off":
				f.parser.errorCostCompetition = false
			case "raw_missing":
				stackEntryNode(b.entry).rawShape = 0
			case "foreign_arena":
				f.scratch.arena = newNodeArena(arenaClassFull)
			case "offset":
				stackEntryNode(b.entry).startByte++
			case "scanner":
				f.language.ExternalScanner = newC26lCheckpointScanner()
				for _, entry := range []stackEntry{a.entry, b.entry} {
					node := stackEntryNode(entry)
					node.rawShape = rawShapeZeroChildRef
					node.children = nil
					node.setExternalScannerToken(true)
				}
			case "distinct", "capacity":
				b.entry = f.entry("node", 43, 43, 1, 5, 8, 20)
				if cause == "capacity" {
					for a.linkCount() < maxMainLinkCount {
						a.appendExtraLink(gssMainLink{entry: a.entry})
					}
				}
			case "nested_distinct":
				a.prev = &gssNode{entry: f.entry("node", 50, 50, 1, 0, 5, 30), depth: 1}
				b.prev = &gssNode{entry: f.entry("node", 51, 51, 1, 0, 5, 40), depth: 1}
				a.depth, b.depth = 2, 2
			case "budget":
				remaining := 0
				if cGSSCompleteLinkCollapseWalk(f.scratch, a, b, 0, &remaining) {
					t.Fatal("exhausted proof succeeded")
				}
				return
			}
			beforeA, beforeB := a.entry.node, b.entry.node
			linksA, linksB := a.linkCount(), b.linkCount()
			if cGSSCompleteLinkCollapse(f.scratch, a, b) {
				t.Fatal("unproved collapse admitted")
			}
			if a.entry.node != beforeA || b.entry.node != beforeB || a.linkCount() != linksA || b.linkCount() != linksB {
				t.Fatal("declined proof changed the graph")
			}
			if cause == "distinct" || cause == "capacity" || cause == "nested_distinct" {
				result := []glrStack{{gss: gssStack{head: a}, byteOffset: 8}}
				incoming := glrStack{gss: gssStack{head: b}, byteOffset: 8}
				if merged, _ := tryGSSMainMergeResult(f.scratch, result, 0, &incoming); merged {
					t.Fatal("distinct alternative lost its separate version")
				}
			}
		})
	}
}

func TestCLinkCollapseRecursivePredecessorKeepsPayload(t *testing.T) {
	f, a, b := cleanCLinkCollapseFixture()
	a.prev = &gssNode{entry: f.entry("node", 50, 50, 1, 0, 5, 30), depth: 1}
	b.prev = &gssNode{entry: f.entry("node", 50, 50, 1, 0, 5, 40), depth: 1}
	a.depth, b.depth = 2, 2
	stackEntryNode(a.prev.entry).dynamicPrecedence = 1
	stackEntryNode(b.entry).dynamicPrecedence = 1
	incumbent := a.entry.node
	result := []glrStack{{gss: gssStack{head: a}, byteOffset: 8, score: 1}}
	incoming := glrStack{gss: gssStack{head: b}, byteOffset: 8, score: 1}
	if !cGSSCompleteLinkCollapse(f.scratch, a, b) {
		t.Fatal("recursive equivalent paths did not prove collapse")
	}
	if merged, _ := tryGSSMainMergeResult(f.scratch, result, 0, &incoming); !merged {
		t.Fatal("recursive collapse failed")
	}
	if a.entry.node != incumbent || result[0].score != 1 {
		t.Fatal("recursive merge replaced the payload or changed total precedence")
	}
}

func TestCLinkCollapseBoundsAllHelperWalks(t *testing.T) {
	f, a, b := cleanCLinkCollapseFixture()
	for i := 0; i < 129; i++ {
		a = &gssNode{entry: a.entry, prev: a, depth: a.depth + 1}
	}
	if cGSSCompleteLinkCollapse(f.scratch, a, b) {
		t.Fatal("graph beyond the shared node bound was admitted")
	}
}

func TestCLinkFreshTokenReceiptProvenance(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		p := &Parser{errorCostCompetition: enabled}
		var ref rawShapeRef
		p.stampCompactPackedGSSZeroChildReceipt(&ref)
		want := rawShapeRef(0)
		if enabled {
			want = rawShapeZeroChildRef
		}
		if ref != want {
			t.Fatalf("mode=%t receipt=%d, want %d", enabled, ref, want)
		}
	}
	f, a, _ := cleanCLinkCollapseFixture()
	unknown := newLeafNodeInArena(f.arena, 42, true, 5, 8, Point{}, Point{})
	if _, ok := cStackLinkPayloadHeader(f.scratch, newStackEntryNode(7, unknown)); ok {
		t.Fatal("an unproven physical leaf acquired a raw header")
	}
	// A collapsed unary node can have no public children and one raw child.
	node := stackEntryNode(a.entry)
	node.children = nil
	before := node.rawShape
	f.parser.stampCompactPackedGSSZeroChildReceipt(&node.rawShape)
	header, ok := cStackLinkPayloadHeader(f.scratch, a.entry)
	if node.rawShape != before || !ok || header.childCount != 1 {
		t.Fatal("token stamping overwrote a captured unary receipt")
	}
}

func TestCLinkUnaryReductionClearsElidedTokenReceipt(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	language := &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true, Named: true}}}
	p := &Parser{language: language, errorCostCompetition: true}
	gss := gssScratch{singleStackMode: true}
	base := gss.allocNode(stackEntry{state: 1}, nil, 1)
	child := newLeafNodeInArena(arena, 1, true, 0, 2, Point{}, Point{Column: 2})
	p.stampCompactPackedGSSZeroChildReceipt(&child.rawShape)
	if child.rawShape != rawShapeZeroChildRef || !gss.mayElideRawShape() {
		t.Fatal("fixture lacks a stamped token and an elidable reduction")
	}
	head := gss.allocNode(newStackEntryNode(2, child), base, 2)
	s := &glrStack{gss: gssStack{head: head}, byteOffset: 2}
	act := ParseAction{Type: ParseActionReduce, Symbol: 1, ChildCount: 1}
	var reduced bool
	if !p.tryFastUnaryCollapseFromGSS(s, act, Token{}, &reduced, arena, nil, &gss, nil) || !reduced {
		t.Fatal("unary reduction declined")
	}
	if got := stackEntryRawShapeRef(s.gss.head.entry); got != 0 {
		t.Fatalf("elided unary retained receipt %d, want unknown", got)
	}
}

func TestCLinkMixedCollapseKeepsLogicalIncumbent(t *testing.T) {
	for _, flatIncumbent := range []bool{false, true} {
		f, a, b := cleanCLinkCollapseFixture()
		owner := &gssScratch{}
		f.scratch.gssOwner = owner
		left := glrStack{gss: gssStack{head: a}, byteOffset: 8}
		right := glrStack{gss: gssStack{head: b}, byteOffset: 8}
		if flatIncumbent {
			left.gss.head, left.entries = nil, []stackEntry{a.entry}
		} else {
			right.gss.head, right.entries = nil, []stackEntry{b.entry}
		}
		result := []glrStack{left}
		if merged, _ := tryGSSMainMergeResult(f.scratch, result, 0, &right); !merged {
			t.Fatal("mixed collapse declined")
		}
		if result[0].gss.head.entry.node != a.entry.node || len(result[0].entries) != 0 {
			t.Fatalf("mixed collapse replaced the logical incumbent: flat=%t", flatIncumbent)
		}
	}
}

func TestCLinkMixedDeclineDoesNotAllocateGraphNodes(t *testing.T) {
	f, a, b := cleanCLinkCollapseFixture()
	owner := &gssScratch{}
	f.scratch.gssOwner = owner
	stackEntryNode(b.entry).rawShape = 0
	result := []glrStack{{entries: []stackEntry{a.entry}, byteOffset: 8}}
	incoming := glrStack{gss: gssStack{head: b}, byteOffset: 8}
	before := owner.usedTotal
	if merged, _ := tryGSSMainMergeResult(f.scratch, result, 0, &incoming); merged {
		t.Fatal("unproved mixed collapse admitted")
	}
	if owner.usedTotal != before || result[0].gss.head != nil || result[0].entries[0].node != a.entry.node {
		t.Fatal("declined mixed probe allocated graph nodes or changed the incumbent")
	}
}

func TestCLinkAcceptedRootsDoNotCollapse(t *testing.T) {
	f, a, b := cleanCLinkCollapseFixture()
	incumbent, candidate := a.entry.node, b.entry.node
	result := []glrStack{{gss: gssStack{head: a}, byteOffset: 8, accepted: true}}
	incoming := glrStack{gss: gssStack{head: b}, byteOffset: 8, accepted: true}
	if merged, attempted := tryGSSMainMergeResult(f.scratch, result, 0, &incoming); merged || !attempted {
		t.Fatalf("accepted roots merged=%t attempted=%t", merged, attempted)
	}
	if a.entry.node != incumbent || b.entry.node != candidate || a.linkCount() != 1 || b.linkCount() != 1 {
		t.Fatal("accepted-root rejection changed either graph")
	}
}

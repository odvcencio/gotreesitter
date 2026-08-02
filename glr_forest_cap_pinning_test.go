package gotreesitter

import "testing"

// Stage 0 of spec.forest-coalescer-dedup-generalization.v1: span-maximal
// pinning at the forest link cap, plus the cap-event instrument that backs
// it. The evidence base (cedar-d, N=11 on a regenerated javascript table) is
// exactly the scenario built here: forestMaxLinksPerNode fills a coalesced
// node with hidden, score/errorCost-tied links from different bracketing
// splits of a repeated construct, and the sole full-coverage (widest-span)
// split arrives last and used to be silently dropped by the pre-existing
// stable-incumbent tie policy.

// forestCapPinTestLanguage marks symbols 100 and 200 hidden (Visible=false)
// so hiddenSym computation in forestCapReplacementIndex has real metadata to
// read, matching a real Language rather than the "unknown language reads as
// visible" case.
func forestCapPinTestLanguage() *Language {
	meta := make([]SymbolMetadata, 201)
	meta[100] = SymbolMetadata{Name: "_hidden_a", Visible: false}
	meta[200] = SymbolMetadata{Name: "_hidden_b", Visible: false}
	return &Language{SymbolMetadata: meta}
}

// forestCapPinTestLink builds a gssLink whose subtree is a bare materialized
// Node carrying exactly the (symbol, start, end) entrySymSpan reads, with a
// prev node at (prevState, prevByte). No arena or raw shape is needed:
// forestCapReplacementIndex's hidden-tie arms (and forestCapTiePinsCandidate)
// only ever read entrySymSpan and the score/errorCost/prev fields directly.
func forestCapPinTestLink(sym Symbol, start, end uint32, prevState StateID, prevByte uint32, score, errorCost int) gssLink {
	node := &Node{symbol: sym, startByte: start, endByte: end}
	prev := &gssForestNode{state: prevState, byteOffset: prevByte}
	return gssLink{prev: prev, subtree: newStackEntryNode(0, node), score: score, errorCost: errorCost}
}

func TestForestCapTiePinsWiderArrivingHiddenCandidate(t *testing.T) {
	p := &Parser{language: forestCapPinTestLanguage()}
	// One resident: symbol 100, covers only the last split [7,10).
	incumbent := forestCapPinTestLink(100, 7, 10, 5, 7, 0, 0)
	node := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}

	// The full-coverage split arrives late: same symbol, same score/errorCost,
	// but covers the whole run [0,10) -- exactly cedar-d's N=11 shape.
	candidate := forestCapPinTestLink(100, 0, 10, 1, 0, 0, 0)

	idx, replace := forestCapReplacementIndex(p, nil, node, &candidate, 1)
	if !replace || idx != 0 {
		t.Fatalf("forestCapReplacementIndex(wider candidate) = (%d, %v), want (0, true) -- pinning must keep the span-maximal arriving candidate", idx, replace)
	}

	stats := p.ForestCapTieStats()
	if stats.HiddenTieDecisions != 1 || stats.CandidatesPinned != 1 || stats.SameSpanTies != 0 {
		t.Fatalf("ForestCapTieStats after pin = %+v, want HiddenTieDecisions=1 CandidatesPinned=1 SameSpanTies=0", stats)
	}
}

func TestForestCapTieKeepsWiderIncumbentOnHiddenTie(t *testing.T) {
	p := &Parser{language: forestCapPinTestLanguage()}
	// Resident already holds the full-coverage split.
	incumbent := forestCapPinTestLink(100, 0, 10, 1, 0, 0, 0)
	node := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}

	// A narrower, tied candidate arrives after it -- today's stable-incumbent
	// policy already protects the wider resident, and pinning must leave that
	// direction untouched (zero-delta: this is not the N=11 defect).
	candidate := forestCapPinTestLink(100, 7, 10, 5, 7, 0, 0)

	idx, replace := forestCapReplacementIndex(p, nil, node, &candidate, 1)
	if replace || idx != 0 {
		t.Fatalf("forestCapReplacementIndex(narrower candidate) = (%d, %v), want (0, false) -- must not evict an already wider-span incumbent", idx, replace)
	}

	stats := p.ForestCapTieStats()
	if stats.HiddenTieDecisions != 1 || stats.CandidatesPinned != 0 {
		t.Fatalf("ForestCapTieStats after protective no-op = %+v, want HiddenTieDecisions=1 CandidatesPinned=0", stats)
	}
}

func TestForestCapTieLeavesExactSpanTieUnpinned(t *testing.T) {
	p := &Parser{language: forestCapPinTestLanguage()}
	incumbent := forestCapPinTestLink(100, 3, 10, 5, 7, 0, 0)
	node := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}

	// Same symbol, same span, same score/errorCost: the span-maximal proxy
	// cannot distinguish these (open question in the spec), so behavior must
	// stay exactly the legacy stable-incumbent no-op.
	candidate := forestCapPinTestLink(100, 3, 10, 55, 70, 0, 0)

	idx, replace := forestCapReplacementIndex(p, nil, node, &candidate, 1)
	if replace || idx != 0 {
		t.Fatalf("forestCapReplacementIndex(exact span tie) = (%d, %v), want (0, false)", idx, replace)
	}

	stats := p.ForestCapTieStats()
	if stats.HiddenTieDecisions != 1 || stats.CandidatesPinned != 0 || stats.SameSpanTies != 1 {
		t.Fatalf("ForestCapTieStats after exact-span tie = %+v, want HiddenTieDecisions=1 CandidatesPinned=0 SameSpanTies=1", stats)
	}
}

func TestForestCapTieLeavesCrossSymbolTieUnpinned(t *testing.T) {
	p := &Parser{language: forestCapPinTestLanguage()}
	// Resident is symbol 200, narrower than the arriving symbol-100 candidate.
	incumbent := forestCapPinTestLink(200, 7, 10, 5, 7, 0, 0)
	node := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}

	candidate := forestCapPinTestLink(100, 0, 10, 1, 0, 0, 0)

	idx, replace := forestCapReplacementIndex(p, nil, node, &candidate, 1)
	if replace || idx != 0 {
		t.Fatalf("forestCapReplacementIndex(cross-symbol tie) = (%d, %v), want (0, false) -- pinning is scoped to one symbol group", idx, replace)
	}
}

// TestForestCapTieReceiptsGatedByEnv is hermetic against an ambient
// GOT_FOREST_CAP_TIE_DUMP in the test process's environment: it clears the
// variable itself before relying on "unset" rather than assuming a clean
// shell. forestCapReplacementIndex reads Parser.forestCapTieDumpActive, a
// field latched once per parse by resetForestCapTieStats (production calls
// it at the top of tryForestFastPath/parseForest); calling
// resetForestCapTieStats directly after each t.Setenv mirrors that latch
// for this unit test, which calls forestCapReplacementIndex directly and so
// never goes through either of those entry points itself.
func TestForestCapTieReceiptsGatedByEnv(t *testing.T) {
	incumbent := forestCapPinTestLink(100, 7, 10, 5, 7, 0, 0)
	candidate := forestCapPinTestLink(100, 0, 10, 1, 0, 0, 0)

	t.Setenv("GOT_FOREST_CAP_TIE_DUMP", "")
	p := &Parser{language: forestCapPinTestLanguage()}
	p.resetForestCapTieStats()
	node := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}
	if _, _ = forestCapReplacementIndex(p, nil, node, &candidate, 1); len(p.ForestCapTieStats().Receipts) != 0 {
		t.Fatalf("receipts populated with GOT_FOREST_CAP_TIE_DUMP unset: %+v", p.ForestCapTieStats().Receipts)
	}

	t.Setenv("GOT_FOREST_CAP_TIE_DUMP", "1")
	p2 := &Parser{language: forestCapPinTestLanguage()}
	p2.resetForestCapTieStats()
	node2 := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}
	if _, _ = forestCapReplacementIndex(p2, nil, node2, &candidate, 1); len(p2.ForestCapTieStats().Receipts) != 1 {
		t.Fatalf("receipts with GOT_FOREST_CAP_TIE_DUMP=1: got %d, want 1", len(p2.ForestCapTieStats().Receipts))
	}
	receipt := p2.ForestCapTieStats().Receipts[0]
	if receipt.Symbol != 100 || receipt.CandidateStart != 0 || receipt.CandidateEnd != 10 ||
		receipt.CandidatePrevState != 1 || receipt.CandidatePrevByte != 0 ||
		receipt.IncumbentStart != 7 || receipt.IncumbentEnd != 10 ||
		receipt.IncumbentPrevState != 5 || receipt.IncumbentPrevByte != 7 ||
		receipt.SameSpan || !receipt.CandidateKept {
		t.Fatalf("receipt shape = %+v, want the candidate/incumbent (symbol,span,prev.state,prev.byteOffset) tuples", receipt)
	}
}

// TestForestCapTieReceiptsAreCopiedNotAliased guards ForestCapTieStats'
// copy-on-return contract: the caller must be able to hold a returned
// Receipts slice across a later parse on the same *Parser without it being
// silently rewritten (resetForestCapTieStats truncates and reuses the same
// backing array with [:0], not a fresh allocation, every parse).
func TestForestCapTieReceiptsAreCopiedNotAliased(t *testing.T) {
	t.Setenv("GOT_FOREST_CAP_TIE_DUMP", "1")
	p := &Parser{language: forestCapPinTestLanguage()}
	p.resetForestCapTieStats()
	incumbent := forestCapPinTestLink(100, 7, 10, 5, 7, 0, 0)
	candidate := forestCapPinTestLink(100, 0, 10, 1, 0, 0, 0)
	node := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}
	if _, _ = forestCapReplacementIndex(p, nil, node, &candidate, 1); len(p.ForestCapTieStats().Receipts) != 1 {
		t.Fatal("setup did not record a receipt")
	}
	held := p.ForestCapTieStats().Receipts
	heldSymbol := held[0].Symbol

	// A later parse-equivalent reset + a differently-shaped tie must not
	// mutate the slice the caller already holds.
	p.resetForestCapTieStats()
	incumbent2 := forestCapPinTestLink(200, 70, 100, 5, 70, 0, 0)
	candidate2 := forestCapPinTestLink(200, 0, 100, 1, 0, 0, 0)
	node2 := &gssForestNode{state: 9, byteOffset: 100, links: []gssLink{incumbent2}}
	if _, _ = forestCapReplacementIndex(p, nil, node2, &candidate2, 1); len(p.ForestCapTieStats().Receipts) != 1 {
		t.Fatal("second setup did not record a receipt")
	}

	if held[0].Symbol != heldSymbol {
		t.Fatalf("held receipts slice was rewritten by a later parse: got symbol %d, want unchanged %d", held[0].Symbol, heldSymbol)
	}
}

// TestForestCapTiePinsWiderCandidateAtFullCapMultiResidentNode exercises the
// cap-full, multi-resident path for real: forestMaxLinksPerNode (8)
// raw-shape-distinct hidden links, all score/errorCost-tied, built up
// through the real insertion path (coalesceForestWithRaw) so
// cachedWorstResidentLink actually scans multiple residents instead of the
// trivial single-link node the other unit tests in this file use. A
// still-wider candidate must win regardless of which specific resident
// cachedWorstResidentLink's raw-shape/order tiebreak happens to pick as
// "worst" -- the candidate's span (smaller start, same end) is wider than
// every resident's, not merely the one selected.
func TestForestCapTiePinsWiderCandidateAtFullCapMultiResidentNode(t *testing.T) {
	var idx gssForestIndex
	idx.init(0)
	slab := &gssForestNodeSlab{}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	p := &Parser{language: forestCapPinTestLanguage()}
	makeEntry := func(childSym Symbol, prod uint16, start, end uint32) stackEntry {
		child := newLeafNodeInArena(arena, childSym, true, start, end, Point{}, Point{})
		parent := newParentNodeInArena(arena, 100, true, []*Node{child}, nil, 0)
		parent.rawShape = p.captureRawShape(nil, arena, 100, prod, []stackEntry{newStackEntryNode(StateID(childSym), child)}, 0, 1)
		return newStackEntryNode(100, parent)
	}

	var node *gssForestNode
	for i := 0; i < forestMaxLinksPerNode; i++ {
		start := uint32(200 - 10*(i+1))
		prev := &gssForestNode{state: StateID(i + 1), byteOffset: start}
		entry := makeEntry(Symbol(300+i), uint16(i+1), start, 200)
		node = coalesceForestWithRaw(p, arena, &idx, slab, 5, 200, prev, entry, 0, 0)
	}
	if len(node.links) != forestMaxLinksPerNode {
		t.Fatalf("setup links = %d, want %d", len(node.links), forestMaxLinksPerNode)
	}

	widePrev := &gssForestNode{state: 99, byteOffset: 10}
	wideEntry := makeEntry(999, 99, 10, 200)
	node = coalesceForestWithRaw(p, arena, &idx, slab, 5, 200, widePrev, wideEntry, 0, 0)

	if len(node.links) != forestMaxLinksPerNode {
		t.Fatalf("links after cap = %d, want %d", len(node.links), forestMaxLinksPerNode)
	}
	found := false
	for i := range node.links {
		if node.links[i].subtree.node == wideEntry.node {
			found = true
		}
	}
	if !found {
		t.Fatal("span-maximal candidate was dropped at a full, multi-resident cap node instead of pinning")
	}
	if stats := p.ForestCapTieStats(); stats.CandidatesPinned == 0 {
		t.Fatalf("ForestCapTieStats.CandidatesPinned = 0 after a full-cap multi-resident pin, want > 0 (stats=%+v)", stats)
	}
}

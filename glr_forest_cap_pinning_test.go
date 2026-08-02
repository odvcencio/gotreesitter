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

func TestForestCapTieReceiptsGatedByEnv(t *testing.T) {
	p := &Parser{language: forestCapPinTestLanguage()}
	incumbent := forestCapPinTestLink(100, 7, 10, 5, 7, 0, 0)
	node := &gssForestNode{state: 9, byteOffset: 10, links: []gssLink{incumbent}}
	candidate := forestCapPinTestLink(100, 0, 10, 1, 0, 0, 0)

	if _, _ = forestCapReplacementIndex(p, nil, node, &candidate, 1); len(p.ForestCapTieStats().Receipts) != 0 {
		t.Fatalf("receipts populated with GOT_FOREST_CAP_TIE_DUMP unset: %+v", p.ForestCapTieStats().Receipts)
	}

	t.Setenv("GOT_FOREST_CAP_TIE_DUMP", "1")
	p2 := &Parser{language: forestCapPinTestLanguage()}
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

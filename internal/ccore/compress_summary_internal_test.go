package ccore

import (
	"math/rand"
	"slices"
	"testing"
)

// compressReference is compress before it skipped a summary: C/subtree.c:293
// ts_subtree_compress, which summarizes every rotated tree in its second loop,
// the inner ones twice.
func compressReference(p *Parser, tree Handle, count uint32, stack *[]Handle) {
	initial := len(*stack)
	sym := p.symbol(tree)
	for i := uint32(0); i < count; i++ {
		if tree&1 != 0 || p.shared(tree) || p.childCount(tree) < 2 {
			break
		}
		child := p.children(tree)[0]
		if child&1 != 0 || p.childCount(child) < 2 || p.shared(child) || p.symbol(child) != sym {
			break
		}
		grand := p.children(child)[0]
		if grand&1 != 0 || p.childCount(grand) < 2 || p.shared(grand) || p.symbol(grand) != sym {
			break
		}
		gk := p.children(grand)
		p.children(tree)[0] = grand
		p.children(child)[0] = gk[len(gk)-1]
		gk[len(gk)-1] = child
		if len(*stack) == cap(*stack) {
			*stack = grow(&p.led, *stack, 1)
		}
		*stack = append(*stack, tree)
		tree = grand
	}
	for len(*stack) > initial {
		tree = (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		child := p.children(tree)[0]
		ck := p.children(child)
		grand := ck[len(ck)-1]
		p.summarize(grand)
		p.summarize(child)
		p.summarize(tree)
	}
}

// compressNoTreeSummary is the control: compressReference without any summary
// of tree, the outermost included. A test that cannot tell it from the
// reference cannot see a missing summary either.
func compressNoTreeSummary(p *Parser, tree Handle, count uint32, stack *[]Handle) {
	initial := len(*stack)
	sym := p.symbol(tree)
	for i := uint32(0); i < count; i++ {
		if tree&1 != 0 || p.shared(tree) || p.childCount(tree) < 2 {
			break
		}
		child := p.children(tree)[0]
		if child&1 != 0 || p.childCount(child) < 2 || p.shared(child) || p.symbol(child) != sym {
			break
		}
		grand := p.children(child)[0]
		if grand&1 != 0 || p.childCount(grand) < 2 || p.shared(grand) || p.symbol(grand) != sym {
			break
		}
		gk := p.children(grand)
		p.children(tree)[0] = grand
		p.children(child)[0] = gk[len(gk)-1]
		gk[len(gk)-1] = child
		if len(*stack) == cap(*stack) {
			*stack = grow(&p.led, *stack, 1)
		}
		*stack = append(*stack, tree)
		tree = grand
	}
	for len(*stack) > initial {
		tree = (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		child := p.children(tree)[0]
		ck := p.children(child)
		grand := ck[len(ck)-1]
		p.summarize(grand)
		p.summarize(child)
	}
}

const (
	compressRepeat  Symbol = 10 // hidden, like a grammar's repeat helper
	compressVisible Symbol = 11
	compressAliasA  Symbol = 12
	compressAliasB  Symbol = 13
)

// newCompressParser is a parser whose tables give the summaries something to
// read: visible and named tokens, alias productions and an active scanner side.
func newCompressParser() *Parser {
	meta := make([]SymbolMeta, 16)
	for s := 1; s < 8; s++ {
		meta[s] = SymbolMeta{Visible: s%2 == 1, Named: s%3 == 0}
	}
	meta[compressVisible] = SymbolMeta{Visible: true, Named: true}
	meta[compressAliasA] = SymbolMeta{Visible: true, Named: true}
	meta[compressAliasB] = SymbolMeta{Visible: true}
	alias := [][]Symbol{nil, {0, compressAliasA}, {compressAliasB, 0, compressAliasA}, {compressAliasA}}
	p := &Parser{t: &Tables{Meta: meta, TokenCount: 8, Alias: alias}}
	p.Store = newStoreCore().view()
	p.beginParse(true)
	p.ext.active = true
	return p
}

// compressBuild makes a left-leaning chain of n nodes of the symbol sym in p,
// from the seed, and returns its root and every handle it made with a record.
// The same seed makes the same records and handles in any fresh parser.
func compressBuild(p *Parser, sym Symbol, n int, seed int64) (root Handle, heap []Handle) {
	rng := rand.New(rand.NewSource(seed))
	keep := func(h Handle) Handle {
		if h&1 == 0 {
			heap = append(heap, h)
		}
		return h
	}
	length := func(multiRow bool) Length {
		l := Length{bytes: uint32(1 + rng.Intn(6))}
		l.col = l.bytes
		if multiRow {
			l.row, l.col = uint32(1+rng.Intn(2)), uint32(rng.Intn(4))
			l.bytes += 2
		}
		return l
	}
	state := func() uint16 { return uint16(1 + rng.Intn(40)) }
	var leaf func(depth int) Handle
	leaf = func(depth int) Handle {
		switch k := rng.Intn(10); {
		case k == 0: // external token, sometimes with a scanner state change
			h := keep(p.newLeaf(Symbol(1+rng.Intn(7)), length(false), length(false), uint32(rng.Intn(3)), state(), false, true, rng.Intn(2) == 0))
			if rng.Intn(2) == 0 {
				p.rec(h).flags |= heapExternalStateChange
			}
			return h
		case k == 1: // a token that spans rows, so the row tests of the summary differ
			return keep(p.newLeaf(Symbol(1+rng.Intn(7)), length(rng.Intn(2) == 0), length(true), uint32(rng.Intn(20)), state(), false, false, rng.Intn(2) == 0))
		case k == 2: // a missing token, on the heap or inline
			if rng.Intn(2) == 0 {
				return keep(p.add(rec{symbol: Symbol(1 + rng.Intn(7)), parseState: state(), flags: heapMissing | heapVisible}))
			}
			return p.newLeaf(Symbol(1+rng.Intn(7)), Length{}, Length{}, 0, state(), false, false, false) | Handle(inlMissing)
		case k == 3: // an extra: the end symbol, or a token copied with the extra flag
			if rng.Intn(2) == 0 {
				return p.newLeaf(0, length(false), length(false), 0, state(), false, false, false)
			}
			return keep(p.setExtra(p.newLeaf(Symbol(1+rng.Intn(7)), Length{bytes: 300, col: 300}, length(false), 0, state(), false, false, false), true))
		case k == 4: // an error leaf
			return keep(p.newLeaf(symError, length(false), length(false), 0, state(), false, false, false))
		case k == 5 && depth < 2: // a visible node, or an error repeat
			s := compressVisible
			if rng.Intn(3) == 0 {
				s = symErrorRepeat
			}
			return keep(p.newNode(s, []Handle{leaf(depth + 1), leaf(depth + 1)}, uint16(rng.Intn(4))))
		case k == 6 && depth < 2: // a node of the chain's symbol off the spine
			return keep(p.newNode(sym, []Handle{leaf(depth + 1), leaf(depth + 1)}, uint16(rng.Intn(4))))
		default:
			return p.newLeaf(Symbol(1+rng.Intn(7)), length(false), length(false), uint32(rng.Intn(4)), state(), rng.Intn(5) == 0, false, rng.Intn(4) == 0)
		}
	}
	kids := []Handle{leaf(0), leaf(0)}
	root = keep(p.newNode(sym, kids, uint16(rng.Intn(4))))
	for i := 1; i < n; i++ {
		kids = []Handle{root}
		for j := 1 + rng.Intn(3); j > 0; j-- {
			kids = append(kids, leaf(0))
		}
		root = keep(p.newNode(sym, kids, uint16(rng.Intn(4))))
	}
	// Flags that a summary only sets: fragile flags and parse states put on
	// the records after they were built, so a summary that runs twice could
	// carry over what a single summary would not.
	for _, h := range heap {
		r := p.rec(h)
		if rng.Intn(4) == 0 {
			r.flags |= heapFragileLeft
		}
		if rng.Intn(4) == 0 {
			r.flags |= heapFragileRight
		}
		switch rng.Intn(4) {
		case 0:
			r.parseState = stateNone
		case 1:
			r.parseState = state()
		}
	}
	return root, heap
}

// compressDiffers counts the records of heap whose fields or child handles
// differ between a and b.
func compressDiffers(a, b *Parser, heap []Handle) int {
	n := 0
	for _, h := range heap {
		if *a.rec(h) != *b.rec(h) || !slices.Equal(a.children(h), b.children(h)) {
			n++
		}
	}
	return n
}

// TestCompressSummarizesEachTreeOnce compares compress, which summarizes each
// inner rotated tree once, with compressReference, which summarizes it twice as
// C does, on random repeat chains: every field and every child handle of every
// record must be equal. The chains mix repeat, error and error-repeat symbols;
// external, missing, extra, error and multi-row leaves; alias productions; and
// fragile flags and parse states set on the records. A control without the
// summaries of tree must differ, so the comparison can fail.
func TestCompressSummarizesEachTreeOnce(t *testing.T) {
	runs, compared, differed, controlRuns, controlDiffered := 0, 0, 0, 0, 0
	for n := 1; n <= 64; n++ {
		ref, got, ctl := newCompressParser(), newCompressParser(), newCompressParser()
		var stack []Handle
		for _, sym := range []Symbol{compressRepeat, symErrorRepeat, symError} {
			for seed := int64(1); seed <= 2; seed++ {
				for _, count := range []int{1, 2, 3, 4, n / 2, n - 1, n, n + 3} {
					s := seed*1_000_003 + int64(n)*1009 + int64(count)*31 + int64(sym)
					rootRef, heap := compressBuild(ref, sym, n, s)
					rootGot, heapGot := compressBuild(got, sym, n, s)
					rootCtl, _ := compressBuild(ctl, sym, n, s)
					if rootGot != rootRef || rootCtl != rootRef || !slices.Equal(heap, heapGot) || compressDiffers(ref, got, heap) != 0 || compressDiffers(ref, ctl, heap) != 0 {
						t.Fatalf("n=%d sym=%d seed=%d count=%d: the three copies differ before compress", n, sym, seed, count)
					}
					stack = stack[:0]
					compressReference(ref, rootRef, uint32(count), &stack)
					got.compress(rootGot, uint32(count), &stack)
					compressNoTreeSummary(ctl, rootCtl, uint32(count), &stack)
					if len(stack) != 0 {
						t.Fatalf("n=%d count=%d: the stack holds %d handles after compress", n, count, len(stack))
					}
					runs++
					compared += len(heap)
					if d := compressDiffers(ref, got, heap); d != 0 {
						differed++
						t.Errorf("n=%d sym=%d seed=%d count=%d: %d records differ from the reference", n, sym, seed, count, d)
					}
					controlRuns++
					if compressDiffers(ref, ctl, heap) != 0 {
						controlDiffered++
					}
				}
			}
		}
	}
	t.Logf("%d compress runs, %d record comparisons, %d runs differed; the control differed in %d of %d runs", runs, compared, differed, controlDiffered, controlRuns)
	if controlDiffered == 0 {
		t.Fatal("the control without tree summaries matched the reference in every run: the test cannot see a missing summary")
	}
}

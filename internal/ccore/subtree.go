package ccore

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Subtree records and the Store (C/subtree.c, C/subtree.h).

type rec struct { // C SubtreeHeapData without ref_count
	padding, size          Length
	lookaheadBytes         uint32
	errorCost              uint32
	childCount             uint32
	childrenStart          uint32
	visibleChildCount      uint32
	namedChildCount        uint32
	visibleDescendantCount uint32
	dynamicPrecedence      int32
	symbol                 Symbol
	parseState             uint16
	flags                  uint16
	repeatDepth            uint16
	productionID           uint16
	firstLeafSymbol        Symbol
	firstLeafState         uint16
}

// Store owns the record arena and the child arena. Every read and write of a
// record or of a child list goes through its methods, so that a later change
// of the storage (pages, segments) touches this file and no algorithm file.
type Store struct {
	recs []rec
	kids []Handle
}

// Tree is a parse result: the root handle and the store that holds it.
type Tree struct {
	Root  Handle
	Store *Store
}

func (p *Store) clear()           { p.recs = append(p.recs[:0], rec{}); p.kids = p.kids[:0] }
func (p *Store) recCount() uint32 { return uint32(len(p.recs)) }

// add appends a record and returns its handle.
func (p *Store) add(r rec) Handle {
	p.recs = append(p.recs, r)
	return Handle(len(p.recs)-1) << 1
}

// addKids appends a child list and returns its start index.
func (p *Store) addKids(children []Handle) uint32 {
	start := uint32(len(p.kids))
	p.kids = append(p.kids, children...)
	return start
}

// copyKids appends a copy of the child list [start, start+n) and returns the copy's start.
func (p *Store) copyKids(start, n uint32) uint32 {
	at := uint32(len(p.kids))
	p.kids = append(p.kids, p.kids[start:start+n]...)
	return at
}

func (p *Store) kidSpan(start, n uint32) []Handle { return p.kids[start : start+n] }
func (p *Store) kidAt(slot int) Handle            { return p.kids[slot] }
func (p *Store) setKidAt(slot int, h Handle)      { p.kids[slot] = h }

// capBytes is the arena capacity in bytes (a statistic, not C's).
func (p *Store) capBytes() int { return cap(p.recs)*80 + cap(p.kids)*8 }

// arenaUse returns the arena bytes in use and the bytes reachable from root.
func (p *Store) arenaUse(root Handle) (used, reachable int) {
	const recBytes, kidBytes = 96, 8
	used = len(p.recs)*recBytes + len(p.kids)*kidBytes
	seen := make(map[Handle]bool)
	stack := []Handle{root}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 || seen[h] {
			continue
		}
		seen[h] = true
		r := p.rec(h)
		reachable += recBytes + int(r.childCount)*kidBytes
		stack = append(stack, p.kidSpan(r.childrenStart, r.childCount)...)
	}
	return used, reachable
}

// hashArena writes every record and child handle to w (determinism checks).
func (p *Store) hashArena(w io.Writer) {
	var b [8]byte
	for _, r := range p.recs {
		fmt.Fprintf(w, "%v", r)
	}
	for _, k := range p.kids {
		binary.LittleEndian.PutUint64(b[:], uint64(k))
		w.Write(b[:])
	}
}

// Read accessors for adapters and tests.
func (p *Store) Symbol(h Handle) Symbol     { return p.symbol(h) }
func (p *Store) ParseState(h Handle) uint16 { return p.parseState(h) }
func (p *Store) Visible(h Handle) bool      { return p.visible(h) }
func (p *Store) Named(h Handle) bool        { return p.named(h) }
func (p *Store) Extra(h Handle) bool        { return p.extra(h) }
func (p *Store) Missing(h Handle) bool      { return p.missing(h) }
func (p *Store) HasChanges(h Handle) bool   { return p.hasChanges(h) }
func (p *Store) Padding(h Handle) Length    { return p.padding(h) }
func (p *Store) Size(h Handle) Length       { return p.size(h) }
func (p *Store) ChildCount(h Handle) uint32 { return p.childCount(h) }
func (p *Store) Children(h Handle) []Handle { return p.children(h) }
func (p *Store) ErrorCost(h Handle) uint32  { return p.errorCost(h) }

func (p *Store) rec(h Handle) *rec { return &p.recs[h>>1] }

func (p *Store) symbol(h Handle) Symbol {
	if h&1 != 0 {
		return Symbol((h >> 8) & 0xff)
	}
	return p.recs[h>>1].symbol
}

func (p *Store) parseState(h Handle) uint16 {
	if h&1 != 0 {
		return uint16(h >> 16)
	}
	return p.recs[h>>1].parseState
}

func (p *Store) hasFlag(h Handle, inl uint64, heap uint16) bool {
	if h&1 != 0 {
		return uint64(h)&inl != 0
	}
	return p.recs[h>>1].flags&heap != 0
}

func (p *Store) visible(h Handle) bool { return p.hasFlag(h, inlVisible, heapVisible) }
func (p *Store) named(h Handle) bool   { return p.hasFlag(h, inlNamed, heapNamed) }
func (p *Store) extra(h Handle) bool   { return p.hasFlag(h, inlExtra, heapExtra) }
func (p *Store) missing(h Handle) bool { return p.hasFlag(h, inlMissing, heapMissing) }
func (p *Store) isKeyword(h Handle) bool {
	return p.hasFlag(h, inlKeyword, heapKeyword)
}

func (p *Store) padding(h Handle) Length { // C ts_subtree_padding
	if h&1 != 0 {
		return Length{bytes: uint32(h>>48) & 0xff, row: uint32(h>>40) & 0xf, col: uint32(h>>32) & 0xff}
	}
	return p.recs[h>>1].padding
}

func (p *Store) size(h Handle) Length { // C ts_subtree_size
	if h&1 != 0 {
		b := uint32(h>>56) & 0xff
		return Length{bytes: b, col: b}
	}
	return p.recs[h>>1].size
}

func (p *Store) totalSize(h Handle) Length { return lenAdd(p.padding(h), p.size(h)) }

func (p *Store) lookaheadBytes(h Handle) uint32 {
	if h&1 != 0 {
		return uint32(h>>44) & 0xf
	}
	return p.recs[h>>1].lookaheadBytes
}

func (p *Store) childCount(h Handle) uint32 {
	if h&1 != 0 {
		return 0
	}
	return p.recs[h>>1].childCount
}

func (p *Store) children(h Handle) []Handle {
	r := &p.recs[h>>1]
	return p.kids[r.childrenStart : r.childrenStart+r.childCount]
}

func (p *Store) errorCost(h Handle) uint32 { // C ts_subtree_error_cost
	if p.missing(h) {
		return costPerMissingTree + costPerRecovery
	}
	if h&1 != 0 {
		return 0
	}
	return p.recs[h>>1].errorCost
}

func (p *Store) dynPrec(h Handle) int32 {
	if h&1 != 0 || p.recs[h>>1].childCount == 0 {
		return 0
	}
	return p.recs[h>>1].dynamicPrecedence
}

func (p *Store) visibleDescendants(h Handle) uint32 {
	if h&1 != 0 || p.recs[h>>1].childCount == 0 {
		return 0
	}
	return p.recs[h>>1].visibleDescendantCount
}

func (p *Store) repeatDepth(h Handle) uint32 {
	if h&1 != 0 {
		return 0
	}
	return uint32(p.recs[h>>1].repeatDepth)
}

func (p *Store) leafSymbol(h Handle) Symbol { // C ts_subtree_leaf_symbol
	if h&1 != 0 {
		return p.symbol(h)
	}
	r := &p.recs[h>>1]
	if r.childCount == 0 {
		return r.symbol
	}
	return r.firstLeafSymbol
}

func (p *Store) leafParseState(h Handle) uint16 { // C ts_subtree_leaf_parse_state
	if h&1 != 0 {
		return p.parseState(h)
	}
	r := &p.recs[h>>1]
	if r.childCount == 0 {
		return r.parseState
	}
	return r.firstLeafState
}

func (p *Store) fragileLeft(h Handle) bool {
	return h&1 == 0 && p.recs[h>>1].flags&heapFragileLeft != 0
}
func (p *Store) fragileRight(h Handle) bool {
	return h&1 == 0 && p.recs[h>>1].flags&heapFragileRight != 0
}

func (p *Store) hasExternalTokens(h Handle) bool {
	return h&1 == 0 && h != 0 && p.recs[h>>1].flags&heapHasExternalTokens != 0
}

func (p *Store) hasExternalStateChange(h Handle) bool {
	return h&1 == 0 && h != 0 && p.recs[h>>1].flags&heapExternalStateChange != 0
}

func (p *Store) dependsOnColumn(h Handle) bool {
	return p.hasFlag(h, inlDependsOnColumn, heapDependsOnColumn)
}

func (p *Store) isFragile(h Handle) bool {
	return h&1 == 0 && p.recs[h>>1].flags&(heapFragileLeft|heapFragileRight) != 0
}

func (p *Store) hasChanges(h Handle) bool { return p.hasFlag(h, inlHasChanges, heapHasChanges) }

func (p *Store) errorExtentCost(size Length) uint32 {
	return costPerRecovery + size.bytes + 30*size.row
}

// ts_subtree_make_mut + ts_subtree_set_extra: a heap leaf is copied (no ref counts).
func (p *Store) setExtra(h Handle, extra bool) Handle {
	if h&1 != 0 {
		if extra {
			return h | Handle(inlExtra)
		}
		return h &^ Handle(inlExtra)
	}
	r := p.recs[h>>1]
	if extra {
		r.flags |= heapExtra
	} else {
		r.flags &^= heapExtra
	}
	p.recs = append(p.recs, r)
	return Handle(len(p.recs)-1) << 1
}

// C/subtree.c:166 ts_subtree_new_leaf (has_external_tokens and
// depends_on_column are false here: no external scanner, no get_column).
func (p *Parser) newLeaf(sym Symbol, padding, size Length, lookahead uint32, state uint16, keyword, external, dependsOnColumn bool) Handle {
	vis, nam := p.meta(sym)
	extra := sym == 0
	inline := sym <= 255 && !external && padding.bytes < 255 && padding.row < 16 && padding.col < 255 &&
		size.bytes < 255 && size.row == 0 && size.col < 255 && lookahead < 16
	if inline {
		f := inlInline
		if vis {
			f |= inlVisible
		}
		if nam {
			f |= inlNamed
		}
		if extra {
			f |= inlExtra
		}
		if keyword {
			f |= inlKeyword
		}
		if dependsOnColumn {
			f |= inlDependsOnColumn
		}
		return Handle(f | uint64(sym)<<8 | uint64(state)<<16 | uint64(padding.col)<<32 |
			uint64(padding.row)<<40 | uint64(lookahead)<<44 | uint64(padding.bytes)<<48 | uint64(size.bytes)<<56)
	}
	var fl uint16
	if vis {
		fl |= heapVisible
	}
	if nam {
		fl |= heapNamed
	}
	if extra {
		fl |= heapExtra
	}
	if keyword {
		fl |= heapKeyword
	}
	if external {
		fl |= heapHasExternalTokens
	}
	if dependsOnColumn {
		fl |= heapDependsOnColumn
	}
	return p.add(rec{padding: padding, size: size, lookaheadBytes: lookahead, symbol: sym, parseState: state, flags: fl})
}

// ts_subtree_make_mut + ts_subtree_set_symbol (C/subtree.c).
func (p *Parser) setSymbol(h Handle, sym Symbol) Handle {
	vis, nam := p.meta(sym)
	if h&1 != 0 {
		h = h&^(0xff<<8) | Handle(sym)<<8
		h &^= Handle(inlVisible | inlNamed)
		if vis {
			h |= Handle(inlVisible)
		}
		if nam {
			h |= Handle(inlNamed)
		}
		return h
	}
	r := *p.rec(h)
	r.symbol = sym
	r.flags &^= heapVisible | heapNamed
	if vis {
		r.flags |= heapVisible
	}
	if nam {
		r.flags |= heapNamed
	}
	return p.add(r)
}

// C/subtree.c:492 ts_subtree_new_node. The children are copied into the
// children arena (C appends the node data to the children allocation).
func (p *Parser) newNode(sym Symbol, children []Handle, productionID uint16) Handle {
	vis, nam := p.meta(sym)
	var fl uint16
	if vis {
		fl |= heapVisible
	}
	if nam {
		fl |= heapNamed
	}
	if sym == symError || sym == symErrorRepeat {
		fl |= heapFragileLeft | heapFragileRight
	}
	start := p.addKids(children)
	h := p.add(rec{symbol: sym, childCount: uint32(len(children)), childrenStart: start, flags: fl, productionID: productionID})
	p.summarize(h)
	return h
}

// C/subtree.c:349 ts_subtree_summarize_children.
func (p *Parser) summarize(h Handle) {
	self := p.rec(h)
	self.namedChildCount, self.visibleChildCount, self.errorCost = 0, 0, 0
	self.repeatDepth, self.visibleDescendantCount, self.dynamicPrecedence = 0, 0, 0
	self.flags &^= heapHasExternalTokens | heapDependsOnColumn | heapExternalStateChange
	structural := 0
	alias := p.aliasSeq(self.productionID)
	var lookaheadEnd uint32
	kids := p.kidSpan(self.childrenStart, self.childCount)
	for i, child := range kids {
		if p.ext.active {
			p.extSummarize(self, child, i == 0)
		}
		if i == 0 {
			self.padding = p.padding(child)
			self.size = p.size(child)
		} else {
			self.size = lenAdd(self.size, p.totalSize(child))
		}
		if end := self.padding.bytes + self.size.bytes + p.lookaheadBytes(child); end > lookaheadEnd {
			lookaheadEnd = end
		}
		grandchildren := p.childCount(child)
		csym := p.symbol(child)
		if csym == symErrorRepeat {
			self.errorCost += p.errorCost(child) - p.errorExtentCost(p.size(child))
		} else {
			self.errorCost += p.errorCost(child)
			if self.symbol == symError || self.symbol == symErrorRepeat {
				if !p.extra(child) && !(csym == symError && grandchildren == 0) {
					if p.visible(child) {
						self.errorCost += costPerSkippedTree
					} else if grandchildren > 0 {
						self.errorCost += costPerSkippedTree * p.rec(child).visibleChildCount
					}
				}
			}
		}
		self.dynamicPrecedence += p.dynPrec(child)
		self.visibleDescendantCount += p.visibleDescendants(child)
		if !p.extra(child) && csym != 0 && structural < len(alias) && alias[structural] != 0 {
			self.visibleDescendantCount++
			self.visibleChildCount++
			if _, named := p.meta(alias[structural]); named {
				self.namedChildCount++
			}
		} else if p.visible(child) {
			self.visibleDescendantCount++
			self.visibleChildCount++
			if p.named(child) {
				self.namedChildCount++
			}
		} else if grandchildren > 0 {
			c := p.rec(child)
			self.visibleChildCount += c.visibleChildCount
			self.namedChildCount += c.namedChildCount
		}
		if csym == symError {
			self.flags |= heapFragileLeft | heapFragileRight
			self.parseState = stateNone
		}
		if !p.extra(child) {
			structural++
		}
	}
	self.lookaheadBytes = lookaheadEnd - self.size.bytes - self.padding.bytes
	if self.symbol == symError || self.symbol == symErrorRepeat {
		self.errorCost += p.errorExtentCost(self.size)
	}
	if len(kids) > 0 {
		first, last := kids[0], kids[len(kids)-1]
		self.firstLeafSymbol = p.leafSymbol(first)
		self.firstLeafState = p.leafParseState(first)
		if p.fragileLeft(first) {
			self.flags |= heapFragileLeft
		}
		if p.fragileRight(last) {
			self.flags |= heapFragileRight
		}
		if len(kids) >= 2 && self.flags&(heapVisible|heapNamed) == 0 && p.symbol(first) == self.symbol {
			if p.repeatDepth(first) > p.repeatDepth(last) {
				self.repeatDepth = uint16(p.repeatDepth(first) + 1)
			} else {
				self.repeatDepth = uint16(p.repeatDepth(last) + 1)
			}
		}
	}
}

// C/subtree.c:609 ts_subtree_compare.
func (p *Parser) compare(left, right Handle) int {
	stack := append(p.scratch[:0], left, right)
	defer func() { p.scratch = stack[:0] }()
	for len(stack) > 0 {
		right, left = stack[len(stack)-1], stack[len(stack)-2]
		stack = stack[:len(stack)-2]
		ls, rs := p.symbol(left), p.symbol(right)
		lc, rc := p.childCount(left), p.childCount(right)
		switch {
		case ls < rs:
			return -1
		case rs < ls:
			return 1
		case lc < rc:
			return -1
		case rc < lc:
			return 1
		}
		for i := lc; i > 0; i-- {
			stack = append(stack, p.children(left)[i-1], p.children(right)[i-1])
		}
	}
	return 0
}

// C/subtree.c:93 ts_subtree_array_remove_trailing_extras.
func (p *Parser) removeTrailingExtras(self []Handle, dest []Handle) ([]Handle, []Handle) {
	dest = dest[:0]
	for len(self) > 0 && p.extra(self[len(self)-1]) {
		dest = append(dest, self[len(self)-1])
		self = self[:len(self)-1]
	}
	for i, j := 0, len(dest)-1; i < j; i, j = i+1, j-1 {
		dest[i], dest[j] = dest[j], dest[i]
	}
	return self, dest
}

// C/subtree.c:293 ts_subtree_compress (ref counts taken as 1).
func (p *Parser) compress(tree Handle, count uint32, stack *[]Handle) {
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

// shared is C's ref_count > 1 for the balance: a record that existed before
// this parse belongs to a tree that is still alive (the old tree).
func (p *Parser) shared(h Handle) bool { return h&1 == 0 && uint32(h>>1) < p.watermark }

// C/parser.c:1915 ts_parser__balance_subtree.
func (p *Parser) balance() {
	stack := p.scratch[:0]
	if p.childCount(p.finished) > 0 && !p.shared(p.finished) {
		stack = append(stack, p.finished)
	}
	var tmp []Handle
	for len(stack) > 0 {
		tree := stack[len(stack)-1]
		if p.rec(tree).repeatDepth > 0 {
			k := p.children(tree)
			c1, c2 := k[0], k[len(k)-1]
			delta := int64(p.repeatDepth(c1)) - int64(p.repeatDepth(c2))
			if delta > 0 {
				for i := uint32(delta) / 2; i > 0; i /= 2 {
					p.compress(tree, i, &tmp)
				}
			}
		}
		stack = stack[:len(stack)-1]
		for _, child := range p.children(tree) {
			if p.childCount(child) > 0 && !p.shared(child) {
				stack = append(stack, child)
			}
		}
	}
	p.scratch = stack[:0]
}

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

// claim takes a record slot for the running parse and returns its handle
// (lifetime.go, step 1). While a parse logs (an incremental parse) it takes from
// the free list first, sets the new bit, logs what it took and leaves an empty
// record in the slot, so that the sweep of an aborted parse finds and frees it;
// otherwise it bumps. A record that will own storage (a child list, scanner-state
// bytes) is claimed before that storage is allocated and filled afterwards (fill):
// the slot is the parse's own from the claim on, so no other allocator can take
// its place, and a refusal of the storage leaves an empty record, never storage
// that no record owns. An index from the free list is checked against the record
// limit like a bumped one.
func (s *Store) claim() Handle {
	c := s.core
	if !s.logging { // a fresh parse: bump into the current page; the store is dropped if the parse fails, so the slot is not written
		cur := &c.pc
		if cur.recSlot < recPageLen {
			idx := uint32(cur.recPage)<<recShift | uint32(cur.recSlot)
			if idx > c.recLimit.Load() {
				c.limitPanic("record index limit reached")
			}
			cur.recSlot++
			return Handle(idx) << 1
		}
		return Handle(s.bumpRec(cur)) << 1
	}
	var idx uint32
	if idx = c.freeRec; idx != 0 {
		if idx > c.recLimit.Load() {
			c.limitPanic("record index limit reached")
		}
		w := &s.refs[idx>>recShift][idx&recMask]
		c.freeRec = *w & countMask
		*w = 0 // a record taken from the free list has one owner
		c.taken = append(c.taken, idx)
	} else {
		cur := &c.pc
		if cur.recSlot == recPageLen {
			s.closeSpan()
			s.growRec(cur)
			c.spanFirst = cur.recSlot
		}
		idx = s.bumpRec(cur)
	}
	*s.recAt(idx) = rec{flags: heapNew}
	return Handle(idx) << 1
}

// fill sets the record that claim gave out.
func (s *Store) fill(h Handle, r rec) {
	if s.logging {
		r.flags |= heapNew
	}
	*s.recAt(uint32(h >> 1)) = r
}

// add is claim and fill for a record that owns no storage.
func (s *Store) add(r rec) Handle {
	h := s.claim()
	s.fill(h, r)
	return h
}

// bumpRec takes the next slot of cur's page.
func (s *Store) bumpRec(cur *arenaCur) uint32 {
	if cur.recSlot == recPageLen {
		s.growRec(cur)
	}
	idx := uint32(cur.recPage)<<recShift | uint32(cur.recSlot)
	if idx > s.core.recLimit.Load() {
		s.core.limitPanic("record index limit reached")
	}
	cur.recSlot++
	return idx
}

// addKids copies a child list into the store and returns its start (0 for no children).
func (s *Store) addKids(children []Handle) uint32 {
	n := uint32(len(children))
	if n == 0 {
		return 0
	}
	if cur := &s.core.pc; n <= 8 && !s.logging && cur.kidPage >= 0 && cur.kidOff+int(n) <= kidPageLen { // a fresh parse: lists of 1 to 8 handles have exact size classes
		o := cur.kidOff
		copy(s.kids[cur.kidPage][o:o+int(n)], children)
		cur.kidOff += int(n)
		return uint32(cur.kidPage)<<kidShift | uint32(o)
	}
	start := s.allocKids(&s.core.pc, n, s.logging)
	copy(s.kidSpan(start, n), children)
	return start
}

// copyKids copies the child list [start, start+n) and returns the copy's start.
func (s *Store) copyKids(start, n uint32) uint32 {
	if n == 0 {
		return 0
	}
	at := s.allocKids(&s.core.pc, n, s.logging)
	copy(s.kidSpan(at, n), s.kidSpan(start, n))
	return at
}

// recWatermark is the index the next bumped record gets: records below it
// existed before the parse that is starting. A page that is full has slot
// recPageLen, which is the first bit of the page number: the two are added, not
// or-ed.
func (s *Store) recWatermark() uint32 {
	if s.core == nil {
		return 0
	}
	cur := &s.core.pc
	if cur.recPage < 0 {
		return 0
	}
	return uint32(cur.recPage)<<recShift + uint32(cur.recSlot)
}

// allocatedRecords is the number of record slots the store has handed out.
func (s *Store) allocatedRecords() int {
	w := int(s.recWatermark())
	if w == 0 {
		return 0
	}
	return w - 1
}

// capBytes is the memory the pages hold, in bytes (a statistic, not C's).
func (s *Store) capBytes() int {
	if s.core == nil {
		return 0
	}
	return s.core.pageBytes()
}

// arenaUse returns the page bytes held and the bytes reachable from root.
func (s *Store) arenaUse(root Handle) (used, reachable int) {
	const kidBytes = 8
	used = s.core.pageBytes()
	seen := make(map[Handle]bool)
	stack := []Handle{root}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 || seen[h] {
			continue
		}
		seen[h] = true
		r := s.rec(h)
		reachable += recBytes + int(r.childCount)*kidBytes
		stack = append(stack, s.kidSpan(r.childrenStart, r.childCount)...)
	}
	return used, reachable
}

// hashArena writes every record, child handle and state byte the parse has
// used to w (determinism checks). The caller holds the lease and emu.
func (s *Store) hashArena(w io.Writer) {
	c := s.core
	var b [8]byte
	cur, ecur := &c.pc, &c.ec // the caller holds the lease and emu
	for pg := 0; pg < c.nextRec; pg++ {
		n := recPageLen // a page that is not the current page of an allocator is full
		if pg == cur.recPage {
			n = cur.recSlot
		} else if pg == ecur.recPage {
			n = ecur.recSlot // slots past it hold stale records of a pooled page
		}
		for i := 0; i < n; i++ {
			r := &s.recs[pg][i]
			if s.refs[pg][i]&flagFree != 0 {
				continue // a free record: its contents are stale
			}
			fmt.Fprintf(w, "%v", *r)
			if r.childCount > 0 {
				for _, k := range s.kidSpan(r.childrenStart, r.childCount) {
					binary.LittleEndian.PutUint64(b[:], uint64(k))
					w.Write(b[:])
				}
			} else if r.flags&heapHasExternalTokens != 0 && r.namedChildCount > 0 {
				w.Write(s.stateBytes(r.visibleDescendantCount, r.namedChildCount))
			}
		}
	}
}

// Read accessors for adapters and tests. They read through the view they are
// called on; a handle is valid while the tree it came from is held and not
// edited or released by the goroutine that owns it (see Tree.Reader).
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

// Children returns the child list of h. The slice points into a page: it is
// valid until the tree that the view belongs to is edited or released (see
// Tree.Reader); the caller must not write it.
func (p *Store) Children(h Handle) []Handle {
	if h&1 != 0 {
		return nil
	}
	return p.children(h)
}
func (p *Store) ErrorCost(h Handle) uint32 { return p.errorCost(h) }

// ExtState returns the scanner-state bytes of a heap leaf that has external
// tokens (C ts_subtree_external_scanner_state); nil for any other handle. The
// slice points into a page and is valid until the tree that the view belongs to
// is edited or released (see Tree.Reader); the caller must not write it.
func (p *Store) ExtState(h Handle) []byte {
	if h == 0 || h&1 != 0 {
		return nil
	}
	r := p.rec(h)
	if r.flags&heapHasExternalTokens == 0 || r.childCount != 0 {
		return nil
	}
	return p.stateBytes(r.visibleDescendantCount, r.namedChildCount)
}

func (p *Store) symbol(h Handle) Symbol {
	if h&1 != 0 {
		return Symbol((h >> 8) & 0xff)
	}
	return p.rec(h).symbol
}

func (p *Store) parseState(h Handle) uint16 {
	if h&1 != 0 {
		return uint16(h >> 16)
	}
	return p.rec(h).parseState
}

func (p *Store) hasFlag(h Handle, inl uint64, heap uint16) bool {
	if h&1 != 0 {
		return uint64(h)&inl != 0
	}
	return p.rec(h).flags&heap != 0
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
	return p.rec(h).padding
}

func (p *Store) size(h Handle) Length { // C ts_subtree_size
	if h&1 != 0 {
		b := uint32(h>>56) & 0xff
		return Length{bytes: b, col: b}
	}
	return p.rec(h).size
}

func (p *Store) totalSize(h Handle) Length { return lenAdd(p.padding(h), p.size(h)) }

func (p *Store) lookaheadBytes(h Handle) uint32 {
	if h&1 != 0 {
		return uint32(h>>44) & 0xf
	}
	return p.rec(h).lookaheadBytes
}

func (p *Store) childCount(h Handle) uint32 {
	if h&1 != 0 {
		return 0
	}
	return p.rec(h).childCount
}

func (p *Store) children(h Handle) []Handle {
	r := p.rec(h)
	return p.kidSpan(r.childrenStart, r.childCount)
}

func (p *Store) errorCost(h Handle) uint32 { // C ts_subtree_error_cost
	if p.missing(h) {
		return costPerMissingTree + costPerRecovery
	}
	if h&1 != 0 {
		return 0
	}
	return p.rec(h).errorCost
}

func (p *Store) dynPrec(h Handle) int32 {
	if h&1 != 0 || p.rec(h).childCount == 0 {
		return 0
	}
	return p.rec(h).dynamicPrecedence
}

func (p *Store) visibleDescendants(h Handle) uint32 {
	if h&1 != 0 || p.rec(h).childCount == 0 {
		return 0
	}
	return p.rec(h).visibleDescendantCount
}

// repeatDepth is C/subtree.h:307 ts_subtree_repeat_depth. C reads
// ptr->repeat_depth for every heap subtree, and for a leaf with external
// tokens that field overlays bytes 16 and 17 of the union member
// external_scanner_state (C/subtree.h:132-152, ExternalScannerState at
// subtree.h:31-37). A state of 24 bytes or fewer is stored in short_data, and
// the bytes past its length are zero because ts_subtree_new_leaf
// (C/subtree.c:201) builds the record from a compound literal. A longer state
// is stored through long_data (8 bytes), so bytes 16 and 17 stay zero.
func (p *Parser) repeatDepth(h Handle) uint32 {
	if h&1 != 0 {
		return 0
	}
	r := p.rec(h)
	if r.childCount == 0 && r.flags&heapHasExternalTokens != 0 {
		st := p.extState(h)
		var d uint32
		if len(st) <= 24 {
			if len(st) > 16 {
				d = uint32(st[16])
			}
			if len(st) > 17 {
				d |= uint32(st[17]) << 8
			}
		}
		return d
	}
	return uint32(r.repeatDepth)
}

func (p *Store) leafSymbol(h Handle) Symbol { // C ts_subtree_leaf_symbol
	if h&1 != 0 {
		return p.symbol(h)
	}
	r := p.rec(h)
	if r.childCount == 0 {
		return r.symbol
	}
	return r.firstLeafSymbol
}

func (p *Store) leafParseState(h Handle) uint16 { // C ts_subtree_leaf_parse_state
	if h&1 != 0 {
		return p.parseState(h)
	}
	r := p.rec(h)
	if r.childCount == 0 {
		return r.parseState
	}
	return r.firstLeafState
}

func (p *Store) fragileLeft(h Handle) bool {
	return h&1 == 0 && p.rec(h).flags&heapFragileLeft != 0
}
func (p *Store) fragileRight(h Handle) bool {
	return h&1 == 0 && p.rec(h).flags&heapFragileRight != 0
}

func (p *Store) hasExternalTokens(h Handle) bool {
	return h&1 == 0 && h != 0 && p.rec(h).flags&heapHasExternalTokens != 0
}

func (p *Store) hasExternalStateChange(h Handle) bool {
	return h&1 == 0 && h != 0 && p.rec(h).flags&heapExternalStateChange != 0
}

func (p *Store) dependsOnColumn(h Handle) bool {
	return p.hasFlag(h, inlDependsOnColumn, heapDependsOnColumn)
}

func (p *Store) isFragile(h Handle) bool {
	return h&1 == 0 && p.rec(h).flags&(heapFragileLeft|heapFragileRight) != 0
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
	r := *p.rec(h)
	if extra {
		r.flags |= heapExtra
	} else {
		r.flags &^= heapExtra
	}
	nh := p.claim()
	if r.childCount > 0 { // C ts_subtree_make_mut clones the child list (C/subtree.c:261-278): two records must not own one list
		r.childrenStart = p.copyKids(r.childrenStart, r.childCount)
	} else {
		p.cloneState(&r, &p.core.pc)
	}
	p.nCopy++
	p.fill(nh, r)
	return nh
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
	p.nLeaf++
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
	nh := p.claim()
	p.cloneState(&r, &p.core.pc)
	r.symbol = sym
	r.flags &^= heapVisible | heapNamed
	if vis {
		r.flags |= heapVisible
	}
	if nam {
		r.flags |= heapNamed
	}
	p.nCopy++
	p.fill(nh, r)
	return nh
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
	h := p.claim() // before the child list: a refused record strands no storage
	start := p.addKids(children)
	if p.logging {
		p.core.fire("storage") // test hook: the child list exists; the record must already be ours
	}
	p.nNode++
	p.fill(h, rec{symbol: sym, childCount: uint32(len(children)), childrenStart: start, flags: fl, productionID: productionID})
	p.summarize(h)
	if p.rec(h).size.bytes == 0 {
		p.zeroNodes++ // see findMulti
	}
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
		// One record lookup per child; an inline leaf (cr nil) has its fields in the handle.
		var cr *rec
		var cpad, csize Length
		var clook, grandchildren, cerr uint32
		var csym Symbol
		var cextra, cvis, cnamed bool
		if child&1 == 0 {
			cr = p.rec(child)
			cpad, csize, clook, grandchildren, csym = cr.padding, cr.size, cr.lookaheadBytes, cr.childCount, cr.symbol
			cextra, cvis, cnamed = cr.flags&heapExtra != 0, cr.flags&heapVisible != 0, cr.flags&heapNamed != 0
			if cr.flags&heapMissing != 0 {
				cerr = costPerMissingTree + costPerRecovery
			} else {
				cerr = cr.errorCost
			}
		} else {
			cpad, csize, clook, csym = p.padding(child), p.size(child), p.lookaheadBytes(child), p.symbol(child)
			cextra, cvis, cnamed = p.extra(child), p.visible(child), p.named(child)
			cerr = p.errorCost(child)
		}
		if i == 0 {
			self.padding = cpad
			self.size = csize
		} else {
			self.size = lenAdd(self.size, lenAdd(cpad, csize))
		}
		if end := self.padding.bytes + self.size.bytes + clook; end > lookaheadEnd {
			lookaheadEnd = end
		}
		if csym == symErrorRepeat {
			self.errorCost += cerr - p.errorExtentCost(csize)
		} else {
			self.errorCost += cerr
			if self.symbol == symError || self.symbol == symErrorRepeat {
				if !cextra && !(csym == symError && grandchildren == 0) {
					if cvis {
						self.errorCost += costPerSkippedTree
					} else if grandchildren > 0 {
						self.errorCost += costPerSkippedTree * cr.visibleChildCount
					}
				}
			}
		}
		if grandchildren > 0 { // dynPrec and visibleDescendants are 0 for a leaf
			self.dynamicPrecedence += cr.dynamicPrecedence
			self.visibleDescendantCount += cr.visibleDescendantCount
		}
		if !cextra && csym != 0 && structural < len(alias) && alias[structural] != 0 {
			self.visibleDescendantCount++
			self.visibleChildCount++
			if _, named := p.meta(alias[structural]); named {
				self.namedChildCount++
			}
		} else if cvis {
			self.visibleDescendantCount++
			self.visibleChildCount++
			if cnamed {
				self.namedChildCount++
			}
		} else if grandchildren > 0 {
			self.visibleChildCount += cr.visibleChildCount
			self.namedChildCount += cr.namedChildCount
		}
		if csym == symError {
			self.flags |= heapFragileLeft | heapFragileRight
			self.parseState = stateNone
		}
		if !cextra {
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

// shared is C's ref_count > 1 for the balance: a record that this parse did
// not allocate belongs to a tree that is still alive (the old tree). A record
// that the running parse allocated has one owner unless the finished tree
// reaches it more than once. Cached heap tokens are handed out once (lifetime.go,
// step 2), but nothing in the parser stops a new internal node from being placed
// in two slots (a node of byte size zero can be; for a node of positive size the
// tree's byte spans make it unlikely, but that is not a proof the code relies
// on). So the balance step asks findMulti, for every finished tree, for the new
// records that the tree reaches more than once, and treats them as shared, which
// is C's ref_count > 1 test. The test of the balance step does not depend on the
// size of a node.
func (p *Parser) shared(h Handle) bool {
	if h&1 != 0 {
		return false
	}
	if !p.isNew(h) {
		return true
	}
	if p.multi != nil {
		if _, twice := p.multi[h]; twice {
			p.balanceRefusals++
			return true
		}
	}
	return false
}

// findMulti walks the finished tree before it is balanced and records, for each
// new record that it reaches more than once, how many times more than once
// (p.multi: the number of owners beyond the first, the count that the reference
// word keeps). It marks the records it visits (new records only: an old record
// has only old children and is never written) and clears the marks before it
// returns. A fresh parse has no publication walk, so countFreshShared adds these
// counts to the reference words; an incremental parse's publication walk counts
// the same owners itself.
func (p *Parser) findMulti() {
	p.balanceChecks++
	p.multi = nil
	stack := append(p.scratch[:0], p.finished)
	seen := p.seen[:0]
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == 0 || h&1 != 0 || !p.isNew(h) {
			continue
		}
		r := p.rec(h)
		if r.flags&heapMark != 0 {
			if p.multi == nil {
				p.multi = map[Handle]uint32{}
			}
			p.multi[h]++
			continue
		}
		r.flags |= heapMark
		seen = append(seen, h)
		if r.childCount > 0 {
			for _, k := range p.kidSpan(r.childrenStart, r.childCount) {
				if k&1 == 0 && k != 0 {
					stack = append(stack, k)
				}
			}
		}
	}
	for _, h := range seen {
		p.rec(h).flags &^= heapMark
	}
	p.scratch, p.seen = stack[:0], seen[:0]
}

// countFreshShared gives the owners that findMulti found to the reference words
// of a fresh tree, so that releasing the tree and a later tree that also owns
// the record count it once per owner, as C does (the caller holds the lease).
func (p *Parser) countFreshShared() {
	c := p.core
	for h, n := range p.multi {
		w := &p.refs[h>>(recShift+1)][(h>>1)&recMask]
		if uint64(*w&countMask)+uint64(n) > uint64(c.countLimit.Load()) {
			c.limitPanic("reference count limit reached")
		}
		*w += n
		counters.shared.Add(int64(n))
	}
}

// cloneState gives the heap leaf copy r its own scanner-state bytes (C/subtree.c:271-275).
func (p *Store) cloneState(r *rec, cur *arenaCur) {
	if r.childCount != 0 || r.flags&heapHasExternalTokens == 0 || r.namedChildCount == 0 {
		return
	}
	var off uint32
	if cur == &p.core.ec {
		off = p.allocStateEdit(r.namedChildCount)
	} else {
		off = p.allocState(cur, r.namedChildCount, cur == &p.core.pc && p.logging)
	}
	copy(p.stateBytes(off, r.namedChildCount), p.stateBytes(r.visibleDescendantCount, r.namedChildCount))
	r.visibleDescendantCount = off
}

// C/parser.c:1915 ts_parser__balance_subtree.
func (p *Parser) balance() {
	if p.preBalance != nil {
		p.preBalance() // tests only
	}
	p.findMulti()
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

// copyHeapLeaf returns a new record with the content of the heap leaf h and
// its own copy of the scanner-state bytes (C/subtree.c:271-275).
func (p *Store) copyHeapLeaf(h Handle) Handle {
	r := *p.rec(h)
	r.flags &^= heapNew | heapMark
	nh := p.claim()
	p.cloneState(&r, &p.core.pc)
	p.nCopy++
	p.fill(nh, r)
	return nh
}

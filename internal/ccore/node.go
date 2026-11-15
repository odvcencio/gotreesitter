package ccore

// Value nodes: C's TSNode as a Go value (C/node.c, C/tree.c; the vendored copy
// is cgo_harness/internal/coracle/src).
//
// A Node is a handle, the position of its slot, its start, its alias and what
// the path to it decided (field, cursor field, supertype mask, ordinal). A
// NodeReader reads the store and the tables and holds no state.
//
// Allocation: the indexed and scalar readers (RootNode, Child, NamedChild, the
// counts and every accessor) allocate nothing. Children allocates when dst has
// too little capacity for the visible children, and when the hidden nodes
// between a visible parent and its visible children are nested more than 15
// deep (its traversal stack is a local array of 16 frames and then grows on the
// heap). A caller that needs no allocation at any depth uses Child.
//
// Lifetime: a Node belongs to the tree and the reader it was read from. An
// Edit or a Release of that tree invalidates it. Nothing detects a stale node
// (Null does not): the caller keeps the tree, and the reader's store view,
// alive and does not use a node after either call. A reparse into the same
// store does not invalidate the nodes of an old tree that is unchanged and
// still held.
//
// The functions port C/node.c: ts_node_child (:593) and ts_node_named_child
// (:597) through ts_node__child (:139), the child iterator (:57, :80),
// ts_node__is_relevant (:109), ts_node__relevant_child_count (:123),
// ts_node_field_name_for_child (:689) and ts_node_field_name_for_named_child
// (:731) with ts_node__field_name_from_language (:673).

// FieldEntry is one entry of a production's field map (C TSFieldMapEntry).
type FieldEntry struct {
	FieldID    uint16
	ChildIndex uint8
	Inherited  bool
}

// NodeTables is the language data the node readers need beyond Tables.
// The root adapter builds one per language next to the Tables it shares.
type NodeTables struct {
	// SymbolCount is the grammar's declared symbol count. A zero value means
	// unavailable; NextParseState declines rather than infer it from metadata.
	SymbolCount  uint32
	Base         *Tables     // symbol metadata and alias sequences
	FieldSlices  [][2]uint16 // [production id] -> (first entry, entry count)
	FieldEntries []FieldEntry
	SuperBit     []uint32 // [symbol] -> the bit of a hidden supertype symbol in a node's mask; 0 for any other symbol
}

// Node is C's TSNode (C/node.c:20, ts_node_new) with what the path to it decided.
//
// Lifetime: a Node belongs to the tree and the reader it was read from. An Edit or a
// Release of that tree invalidates it, and nothing detects a stale node (Null does
// not). The caller keeps the tree, and the store view that Tree.Reader returns,
// alive. A reparse into the same store does not invalidate the nodes of an old tree
// that is unchanged and still held.
//
// List and Slot name the slot that holds the node's handle in the child pages.
// List is the child-list start of the record that holds the slot (see
// Store.ChildSlots). When the list is not large the slot key is Slot alone,
// which is List plus the index in the list; when the list is large
// (LargeSlot(List)) the slot is the pair (List, Slot) and Slot is the index in
// the list. For the root, List and Slot are 0 and mean nothing: tell the root
// by Parent-less use (RootNode).
type Node struct {
	H       Handle
	List    uint32
	Slot    uint32
	Start   Length
	Alias   Symbol
	Mask    uint32 // supertype bits of the hidden nodes between the visible parent and this node
	Field   uint16 // field the node fills in its visible parent (C ts_node_field_name_for_child); 0 for none
	CField  uint16 // field of the node as the tree cursor reports it (C ts_tree_cursor_current_field_id); 0 for none
	Ordinal uint32 // index among the visible children of the visible parent
}

// Null reports whether n is C's null node.
func (n Node) Null() bool { return n.H == 0 } // C ts_node_is_null (C/node.c:497)

// NodeReader reads value nodes of one tree. It holds no state; copy it freely.
//
// Lifetime: S is the store view of one tree (Tree.Reader). The reader and every Node
// read through it are valid only while that tree is held and neither edited nor
// released (see Node); nothing detects a stale reader or node.
type NodeReader struct {
	S *Store
	T *NodeTables
}

// NewNodeReader returns a reader over a store view (Tree.Reader) and tables.
func NewNodeReader(s *Store, t *NodeTables) NodeReader { return NodeReader{S: s, T: t} }

// RootNode is C ts_tree_root_node (C/tree.c:55): the root at its padding.
// A released tree has root handle 0 and gives the null node.
func (r NodeReader) RootNode(root Handle) Node {
	if root == 0 {
		return Node{}
	}
	return Node{H: root, Start: r.S.padding(root)}
}

// ---- accessors (C/node.c ts_node_*) ----

const (
	errorSymbol       Symbol = 0xFFFF // ts_builtin_sym_error
	errorRepeatSymbol Symbol = 0xFFFE // ts_builtin_sym_error_repeat
)

// meta is C ts_language_symbol_metadata (C/language.c:215): the built-in error
// symbol is visible and named, and the error-repeat symbol is neither.
func (r NodeReader) meta(sym Symbol) SymbolMeta {
	switch sym {
	case errorSymbol:
		return SymbolMeta{Visible: true, Named: true}
	case errorRepeatSymbol:
		return SymbolMeta{}
	}
	if m := r.T.Base.Meta; int(sym) < len(m) {
		return m[sym]
	}
	return SymbolMeta{}
}

// Symbol is C ts_node_symbol (C/node.c:457) before the public symbol map: the alias, else the record's symbol.
func (r NodeReader) Symbol(n Node) Symbol {
	if n.Alias != 0 {
		return n.Alias
	}
	return r.S.symbol(n.H)
}

// GrammarSymbol is C ts_node_grammar_symbol (C/node.c:473): the record's symbol, ignoring the alias.
func (r NodeReader) GrammarSymbol(n Node) Symbol { return r.S.symbol(n.H) }

// Named is C ts_node_is_named (C/node.c:505).
func (r NodeReader) Named(n Node) bool {
	if n.Alias != 0 {
		return r.meta(n.Alias).Named
	}
	return r.S.named(n.H)
}

// Extra is C ts_node_is_extra (C/node.c:501).
func (r NodeReader) Extra(n Node) bool { return r.S.extra(n.H) }

// Missing is C ts_node_is_missing (C/node.c:512).
func (r NodeReader) Missing(n Node) bool { return r.S.missing(n.H) }

// HasChanges is C ts_node_has_changes (C/node.c:516).
func (r NodeReader) HasChanges(n Node) bool { return r.S.hasChanges(n.H) }

// HasError is C ts_node_has_error (C/node.c:520): the error cost is above 0.
func (r NodeReader) HasError(n Node) bool { return r.S.errorCost(n.H) > 0 }

// IsError is C ts_node_is_error (C/node.c:524).
func (r NodeReader) IsError(n Node) bool { return r.Symbol(n) == errorSymbol }

// ParseState is C ts_node_parse_state (C/node.c:533).
func (r NodeReader) ParseState(n Node) uint16 { return r.S.parseState(n.H) }

// NextParseState is C ts_node_next_parse_state. It uses the grammar
// symbol, not an alias, and the last parse action for a terminal.
func (r NodeReader) NextParseState(n Node) uint16 {
	if n.H == 0 || r.S == nil || r.T == nil || r.T.Base == nil {
		return 0
	}
	state := r.ParseState(n)
	if state == stateNone {
		return stateNone
	}
	t := r.T.Base
	sym := r.GrammarSymbol(n)
	if uint32(state) >= t.StateCount || sym == symError || sym == symErrorRepeat || uint32(sym) >= r.T.SymbolCount {
		return 0
	}
	if uint32(sym) >= t.TokenCount {
		gotoState := uint32(t.ActionIndex(uint32(state), sym))
		if wide := t.LargeStateGotos[uint64(state)<<32|uint64(sym)]; wide != 0 {
			gotoState = wide
		}
		if gotoState >= t.StateCount || gotoState >= uint32(stateNone) {
			return 0
		}
		return uint16(gotoState)
	}
	index := t.ActionIndex(uint32(state), sym)
	if index == 0 || int(index) >= len(t.Actions) {
		return 0
	}
	actions := t.Actions[index].Actions
	if len(actions) == 0 {
		return 0
	}
	last := actions[len(actions)-1]
	if last.Type != ActionShift {
		return 0
	}
	if last.Extra {
		return state
	}
	if last.State >= t.StateCount || last.State >= uint32(stateNone) {
		return 0
	}
	return uint16(last.State)
}

// StartByte is C ts_node_start_byte (C/node.c:39).
func (r NodeReader) StartByte(n Node) uint32 { return n.Start.bytes }

// StartPoint is C ts_node_start_point (C/node.c:43).
func (r NodeReader) StartPoint(n Node) (row, col uint32) { return n.Start.row, n.Start.col }

// End is the end of the node (C ts_node_end_byte and ts_node_end_point, C/node.c:449, :453).
func (r NodeReader) End(n Node) Length { return lenAdd(n.Start, r.S.size(n.H)) }

// EndByte is C ts_node_end_byte (C/node.c:449).
func (r NodeReader) EndByte(n Node) uint32 { return n.Start.bytes + r.S.size(n.H).bytes }

// ChildCount is C ts_node_child_count (C/node.c:786).
func (r NodeReader) ChildCount(n Node) uint32 { return r.S.VisibleChildCount(n.H) }

// NamedChildCount is C ts_node_named_child_count (C/node.c:795).
func (r NodeReader) NamedChildCount(n Node) uint32 { return r.S.NamedChildCount(n.H) }

// ---- the child iterator (C/node.c NodeChildIterator) ----

// childIter is C NodeChildIterator (C/node.c:8-15).
type childIter struct {
	r          NodeReader
	list       uint32
	count      uint32
	idx        uint32
	structural uint32
	prod       uint16
	seq        []Symbol
	pos        Length
}

// iterate is C ts_node_iterate_children (C/node.c:57).
func (r NodeReader) iterate(n Node) childIter {
	list, count := r.S.ChildSlots(n.H)
	if count == 0 {
		return childIter{r: r}
	}
	prod := r.S.ProductionID(n.H)
	it := childIter{r: r, list: list, count: count, prod: prod, pos: n.Start}
	if prod != 0 && int(prod) < len(r.T.Base.Alias) { // C ts_language_alias_sequence: NULL for production 0
		it.seq = r.T.Base.Alias[prod]
	}
	return it
}

// next is C ts_node_child_iterator_next (C/node.c:80).
func (it *childIter) next(out *Node) bool {
	if it.idx >= it.count {
		return false
	}
	s := it.r.S
	child := s.ChildHandle(it.list, it.count, it.idx)
	var alias Symbol
	if !s.extra(child) {
		if it.seq != nil && int(it.structural) < len(it.seq) {
			alias = it.seq[it.structural]
		}
		it.structural++
	}
	if it.idx > 0 {
		it.pos = lenAdd(it.pos, s.padding(child))
	}
	slot := it.idx
	if !LargeSlot(it.list) {
		slot = it.list + it.idx
	}
	*out = Node{H: child, List: it.list, Slot: slot, Start: it.pos, Alias: alias}
	it.pos = lenAdd(it.pos, s.size(child))
	it.idx++
	return true
}

// ownField is C ts_node__field_name_from_language (C/node.c:673): the field of a production at
// a structural child index, ignoring inherited entries. -1 matches nothing.
func (r NodeReader) ownField(prod uint16, structural int) uint16 {
	t := r.T
	if int(prod) >= len(t.FieldSlices) {
		return 0
	}
	sl := t.FieldSlices[prod]
	end := int(sl[0]) + int(sl[1])
	if end > len(t.FieldEntries) {
		return 0
	}
	for _, e := range t.FieldEntries[sl[0]:end] {
		if !e.Inherited && int(e.ChildIndex) == structural {
			return e.FieldID
		}
	}
	return 0
}

// relevant is C ts_node__is_relevant (C/node.c:109).
func (r NodeReader) relevant(n *Node, anon bool) bool {
	if anon {
		return r.S.visible(n.H) || n.Alias != 0
	}
	if n.Alias != 0 {
		return r.meta(n.Alias).Named
	}
	return r.S.visible(n.H) && r.S.named(n.H)
}

// relevantCount is C ts_node__relevant_child_count (C/node.c:123).
func (r NodeReader) relevantCount(n *Node, anon bool) uint32 {
	if anon {
		return r.S.VisibleChildCount(n.H)
	}
	return r.S.NamedChildCount(n.H)
}

func (r NodeReader) superBit(h Handle) uint32 {
	sym := r.S.symbol(h)
	if int(sym) < len(r.T.SuperBit) {
		return r.T.SuperBit[sym]
	}
	return 0
}

// child is C ts_node__child (C/node.c:139) with the field, cursor field, mask and
// ordinal the descent finds. The field of ts_node_field_name_for_child (:689) and
// _for_named_child (:731) keeps its inheritance across any descent. The cursor
// field, the mask and the ordinal belong to the node's visible parent, as in
// C/tree_cursor.c:664 and :506, so a descent through a visible node (the named
// walk can enter an anonymous visible node that holds named children, as C does)
// starts them again.
func (r NodeReader) child(self Node, target uint32, anon bool) (Node, bool) {
	n, ok, _ := r.childQ(self, target, anon)
	return n, ok
}

// childQ is child, and reports whether the named walk met an anonymous visible
// child that holds named children (the quirk of C/node.c:139): C then numbers
// the named children differently from the named visible children in sequence.
func (r NodeReader) childQ(self Node, target uint32, anon bool) (Node, bool, bool) {
	quirk := false
	cur := self
	var inherited, curInh uint16
	var mask, vis uint32
	for {
		it := r.iterate(cur)
		var child Node
		index := uint32(0)
		descended := false
		for it.next(&child) {
			relA := r.relevant(&child, true)
			if r.relevant(&child, anon) {
				if index == target {
					child.Mask = mask
					child.Ordinal = vis
					if !r.S.extra(child.H) {
						f := r.ownField(it.prod, int(it.structural)-1)
						child.Field, child.CField = f, f
						if f == 0 {
							child.Field, child.CField = inherited, curInh
						}
					}
					return child, true, quirk
				}
				index++
				vis++
				continue
			}
			gi := target - index
			gc := r.relevantCount(&child, anon)
			if !anon && relA && gc > 0 {
				quirk = true
			}
			if gi < gc {
				f := r.ownField(it.prod, int(it.structural)-1)
				if f != 0 {
					inherited = f
				}
				if relA { // a visible node: it is the visible parent of what follows
					curInh, mask, vis = 0, 0, 0
				} else {
					if r.S.extra(child.H) {
						curInh = 0
					} else if f != 0 {
						curInh = f
					}
					mask |= r.superBit(child.H)
				}
				cur = child
				target = gi
				descended = true
				break
			}
			index += gc
			if relA {
				vis++
			} else {
				vis += r.S.VisibleChildCount(child.H)
			}
		}
		if !descended {
			return Node{}, false, quirk
		}
	}
}

// Child is C ts_node_child (C/node.c:593): the child at index i among the visible children.
func (r NodeReader) Child(n Node, i uint32) (Node, bool) { return r.child(n, i, true) }

// NamedChildChecked follows C ts_node_named_child (C/node.c:597) and reports
// whether its walk crossed an anonymous visible child with named descendants.
func (r NodeReader) NamedChildChecked(n Node, i uint32) (Node, bool, bool) {
	return r.childQ(n, i, false)
}

// NamedChild is C ts_node_named_child (C/node.c:597).
func (r NodeReader) NamedChild(n Node, i uint32) (Node, bool) { return r.child(n, i, false) }

// Children appends every visible child of n to dst, in order, in one pass, and
// returns the extended slice. Each child carries what Child would give it.
func (r NodeReader) Children(n Node, dst []Node) []Node {
	dst, _ = r.ChildrenHidden(n, dst)
	return dst
}

// ChildrenHidden is Children, and also reports whether a hidden child with
// children passed a field down to its own children (a hidden node that the
// parent's field map names, or one that inherits a field).
func (r NodeReader) ChildrenHidden(n Node, dst []Node) ([]Node, bool) {
	type frame struct {
		it        childIter
		inherited uint16
		curInh    uint16
		mask      uint32
	}
	var stack [16]frame
	st := append(stack[:0], frame{it: r.iterate(n)})
	vis := uint32(0)
	hidden := false
	for len(st) > 0 {
		top := &st[len(st)-1]
		var child Node
		if !top.it.next(&child) {
			st = st[:len(st)-1]
			continue
		}
		if r.relevant(&child, true) {
			child.Mask = top.mask
			child.Ordinal = vis
			vis++
			if !r.S.extra(child.H) {
				f := r.ownField(top.it.prod, int(top.it.structural)-1)
				child.Field, child.CField = f, f
				if f == 0 {
					child.Field, child.CField = top.inherited, top.curInh
				}
			}
			dst = append(dst, child)
		} else if r.relevantCount(&child, true) > 0 {
			f := r.ownField(top.it.prod, int(top.it.structural)-1)
			inh := top.inherited
			if f != 0 {
				inh = f
			}
			if inh != 0 {
				hidden = true
			}
			cur := top.curInh
			if r.S.extra(child.H) {
				cur = 0
			} else if f != 0 {
				cur = f
			}
			next := frame{it: r.iterate(child), inherited: inh, curInh: cur, mask: top.mask | r.superBit(child.H)}
			st = append(st, next) // top is not used after this
		}
	}
	return dst, hidden
}

// ---- field search (C/node.c:601, ts_node_child_by_field_id) ----

type fieldFrame struct {
	h        Handle
	sub      []FieldEntry
	seq      []Symbol
	children []Handle
	ci       int   // next raw child
	k        int   // current entry of sub
	sidx     int   // structural index of the next non-extra child
	vbase    int32 // visible children before the next raw child
	offset   int32 // index of this node's first visible child among the visible children of the node the search began at (or of the visible node it entered)
	contrib  int32 // visible children of the raw child a recursive call is working on
	restore  int   // length of the path when the call was made
}

func (r NodeReader) seqOf(prod uint16) []Symbol {
	if int(prod) < len(r.T.Base.Alias) {
		return r.T.Base.Alias[prod]
	}
	return nil
}

func (r NodeReader) relevantAt(child Handle, seq []Symbol, structural int) bool {
	return r.S.visible(child) || (structural >= 0 && structural < len(seq) && seq[structural] != 0)
}

func (r NodeReader) newFieldFrame(h Handle, f uint16, offset int32) (fieldFrame, bool) {
	if f == 0 || r.S.VisibleChildCount(h) == 0 { // C/node.c:603 ts_node_child_count: the visible count
		return fieldFrame{}, false
	}
	prod := r.S.ProductionID(h)
	var sub []FieldEntry
	t := r.T
	if int(prod) < len(t.FieldSlices) {
		sl := t.FieldSlices[prod]
		if end := int(sl[0]) + int(sl[1]); end <= len(t.FieldEntries) {
			for _, e := range t.FieldEntries[sl[0]:end] {
				if e.FieldID == f {
					sub = append(sub, e)
				}
			}
		}
	}
	if len(sub) == 0 {
		return fieldFrame{}, false
	}
	return fieldFrame{h: h, sub: sub, seq: r.seqOf(prod), children: r.S.Children(h), offset: offset}, true
}

// FieldPath is C ts_node_child_by_field_id (C/node.c:601): it returns the path of
// visible-child indexes from n to the node C finds for field f, or false. The
// path names positions, never handles (an inline leaf is a value and can appear
// in several slots). The search is iterative: its depth is not limited, and the
// recursive call of C, for a hidden or visible child that inherits the field
// when it is not the last candidate, is a frame on the heap.
func (r NodeReader) FieldPath(n Node, f uint16) ([]int32, bool) {
	var path []int32
	var stack []fieldFrame
	cur, ok := r.newFieldFrame(n.H, f, 0)
	for {
		if !ok { // the current call found nothing: return to the caller, which goes on with its next candidate
			if len(stack) == 0 {
				return nil, false
			}
			cur = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			path = path[:cur.restore]
			cur.k++
			if cur.k == len(cur.sub) {
				continue // the caller has no candidate left either
			}
			cur.vbase += cur.contrib
			ok = true
		}
		found, finished := false, true
		for cur.ci < len(cur.children) {
			child := cur.children[cur.ci]
			cur.ci++
			var relA bool
			var sidx int
			extra := r.S.extra(child)
			if extra {
				sidx = -1
			} else {
				sidx = cur.sidx
				cur.sidx++
			}
			relA = r.relevantAt(child, cur.seq, sidx)
			contrib := int32(0)
			if relA {
				contrib = 1
			} else {
				contrib = int32(r.S.VisibleChildCount(child))
			}
			if extra || sidx < int(cur.sub[cur.k].ChildIndex) {
				cur.vbase += contrib
				continue
			}
			e := cur.sub[cur.k]
			switch {
			case e.Inherited:
				off := cur.offset + cur.vbase
				restore := len(path)
				if relA { // a visible node: the search continues inside it, from its own first child
					path = append(path, off)
					off = 0
				}
				nf, nok := r.newFieldFrame(child, f, off)
				if cur.k+1 == len(cur.sub) { // the last candidate: C goes on in place (a tail call)
					cur, ok = nf, nok
					finished = false
				} else {
					cur.contrib, cur.restore = contrib, restore
					stack = append(stack, cur)
					cur, ok = nf, nok
					finished = false
				}
			case relA:
				return append(path, cur.offset+cur.vbase), true
			case r.S.VisibleChildCount(child) > 0: // C/node.c:658: a node with visible children: its first one
				return append(path, cur.offset+cur.vbase), true
			default:
				cur.k++
				if cur.k < len(cur.sub) {
					cur.vbase += contrib
					continue
				}
			}
			break
		}
		_ = found
		if finished {
			ok = false
		}
	}
}

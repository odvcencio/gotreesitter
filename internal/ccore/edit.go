package ccore

// C/tree.c ts_tree_edit and C/subtree.c ts_subtree_edit.
//
// Lifetime: C copies a node on the edit path only when it is shared
// (ts_subtree_make_mut). Here every node on the edit path is copied into new
// records, with a fresh copy of its child list and its scanner-state bytes, so
// the old tree stays valid and unchanged; untouched subtrees are shared by
// handle and retained (Tree.Edit queues the retains). Published records are
// never written.

type editRange struct{ start, oldEnd, newEnd Length } // C Edit

// editor allocates for the edit: the edit allocator (edit true: used by
// Tree.Edit) or the parse's allocator (edit false).
type editor struct {
	s    *Store
	edit bool
}

func (e *editor) cur() *arenaCur {
	if e.edit {
		return &e.s.core.ec
	}
	return &e.s.core.pc
}

func (e *editor) addRec(r rec) Handle {
	if e.edit {
		return e.s.addEdit(r)
	}
	return e.s.add(r)
}

// claim takes the record slot before the caller allocates the storage that the
// record will own; fill sets the record afterwards.
func (e *editor) claim() Handle {
	if e.edit {
		return e.s.claimEdit()
	}
	return e.s.claim()
}

func (e *editor) fill(h Handle, r rec) {
	if e.edit {
		e.s.fillEdit(h, r)
		return
	}
	e.s.fill(h, r)
}

func (e *editor) copyKids(start, n uint32) uint32 {
	if n == 0 {
		return 0
	}
	var at uint32
	if e.edit {
		at = e.s.allocKidsEdit(n)
	} else {
		at = e.s.allocKids(e.cur(), n, e.s.logging)
	}
	copy(e.s.kidSpan(at, n), e.s.kidSpan(start, n))
	return at
}

// subtreeEdit is C/subtree.c ts_subtree_edit. It returns the new root and
// the records it created. A location is the root or a slot of a copied
// record's child list.
func (s *Store) subtreeEdit(ed *editor, self Handle, input editRange) (Handle, []Handle) {
	type entry struct {
		kids []Handle // nil: the root
		i    int
		edit editRange
	}
	var created []Handle
	p := s
	stack := []entry{{edit: input}}
	get := func(en entry) Handle {
		if en.kids == nil {
			return self
		}
		return en.kids[en.i]
	}
	for len(stack) > 0 {
		en := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		edit := en.edit
		tree := get(en)
		isNoop := edit.oldEnd.bytes == edit.start.bytes && edit.newEnd.bytes == edit.start.bytes
		isPureInsertion := edit.oldEnd.bytes == edit.start.bytes
		parentDependsOnColumn := p.dependsOnColumn(tree)
		columnShifted := edit.newEnd.col != edit.oldEnd.col
		size := p.size(tree)
		padding := p.padding(tree)
		totalSize := lenAdd(padding, size)
		lookahead := p.lookaheadBytes(tree)
		endByte := totalSize.bytes + lookahead
		if edit.start.bytes > endByte || (isNoop && edit.start.bytes == endByte) {
			continue
		}
		if edit.oldEnd.bytes <= padding.bytes {
			padding = lenAdd(edit.newEnd, lenSub(padding, edit.oldEnd))
		} else if edit.start.bytes < padding.bytes {
			size = lenSaturatingSub(size, lenSub(edit.oldEnd, padding))
			padding = edit.newEnd
		} else if edit.start.bytes < totalSize.bytes || (edit.start.bytes == totalSize.bytes && isPureInsertion) {
			size = lenAdd(lenSub(edit.newEnd, padding), lenSaturatingSub(totalSize, edit.oldEnd))
		}
		// ts_subtree_make_mut: always a copy here (see the file comment).
		var result Handle
		if tree&1 != 0 {
			if padding.bytes < 255 && padding.row < 16 && padding.col < 255 && size.bytes < 255 && size.row == 0 && size.col < 255 && lookahead < 16 {
				result = tree &^ Handle(uint64(0xff)<<32|uint64(0xf)<<40|uint64(0xff)<<48|uint64(0xff)<<56)
				result |= Handle(uint64(padding.col)<<32 | uint64(padding.row)<<40 | uint64(padding.bytes)<<48 | uint64(size.bytes)<<56)
				result |= Handle(inlHasChanges)
			} else {
				var fl uint16 = heapHasChanges
				if uint64(tree)&inlVisible != 0 {
					fl |= heapVisible
				}
				if uint64(tree)&inlNamed != 0 {
					fl |= heapNamed
				}
				if uint64(tree)&inlExtra != 0 {
					fl |= heapExtra
				}
				if uint64(tree)&inlDependsOnColumn != 0 {
					fl |= heapDependsOnColumn
				}
				if uint64(tree)&inlMissing != 0 {
					fl |= heapMissing
				}
				if uint64(tree)&inlKeyword != 0 {
					fl |= heapKeyword
				}
				result = ed.addRec(rec{padding: padding, size: size, lookaheadBytes: lookahead,
					symbol: p.symbol(tree), parseState: p.parseState(tree), flags: fl})
			}
		} else {
			r := *p.rec(tree)
			r.padding, r.size = padding, size
			r.flags |= heapHasChanges
			nh := ed.claim()
			if r.childCount > 0 {
				r.childrenStart = ed.copyKids(r.childrenStart, r.childCount)
			} else {
				p.cloneState(&r, ed.cur())
			}
			ed.fill(nh, r)
			result = nh
		}
		if result&1 == 0 {
			created = append(created, result)
			if ed.edit {
				p.core.fire("edit-record") // test hook: after an edited record exists
			}
		}
		if en.kids == nil {
			self = result
		} else {
			en.kids[en.i] = result
		}
		var childLeft, childRight Length
		n := p.childCount(result)
		var kids []Handle
		if n > 0 {
			kids = p.children(result)
		}
		for i := uint32(0); i < n; i++ {
			child := kids[i]
			childSize := p.totalSize(child)
			childLeft = childRight
			childRight = lenAdd(childLeft, childSize)
			if childRight.bytes+p.lookaheadBytes(child) < edit.start.bytes {
				continue
			}
			if (childLeft.bytes > edit.oldEnd.bytes || (childLeft.bytes == edit.oldEnd.bytes && childSize.bytes > 0 && i > 0)) &&
				(!parentDependsOnColumn || childLeft.row > padding.row) &&
				(!p.dependsOnColumn(child) || !columnShifted || childLeft.row > edit.oldEnd.row) {
				break
			}
			childEdit := editRange{
				start:  lenSaturatingSub(edit.start, childLeft),
				oldEnd: lenSaturatingSub(edit.oldEnd, childLeft),
				newEnd: lenSaturatingSub(edit.newEnd, childLeft),
			}
			if childRight.bytes > edit.start.bytes || (childRight.bytes == edit.start.bytes && isPureInsertion) {
				edit.newEnd = edit.start
			} else {
				childEdit.oldEnd = childEdit.start
				childEdit.newEnd = childEdit.start
			}
			stack = append(stack, entry{kids: kids, i: int(i), edit: childEdit})
		}
	}
	return self, created
}

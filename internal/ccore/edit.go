package ccore

// C/tree.c ts_tree_edit and C/subtree.c ts_subtree_edit.
//
// Lifetime without reference counts: C copies a node on the edit path only
// when it is shared (ts_subtree_make_mut). Here every node on the edit path
// is copied into new records, with a fresh copy of its children range, so
// the old tree stays valid and unchanged; untouched subtrees are shared by
// handle. Nothing is reclaimed.

type editRange struct{ start, oldEnd, newEnd Length } // C Edit

// treeEdit is C/tree.c ts_tree_edit without included ranges (the
// ts_range_edit loop over included ranges is the hook left out).
func (p *Parser) treeEdit(root Handle, e editRange) Handle {
	return p.subtreeEdit(root, e)
}

// subtreeEdit is C/subtree.c ts_subtree_edit. A slot is the root (-1) or
// an index into the children arena of a copied node.
func (p *Parser) subtreeEdit(self Handle, input editRange) Handle {
	type entry struct {
		slot int
		edit editRange
	}
	stack := []entry{{slot: -1, edit: input}}
	get := func(slot int) Handle {
		if slot < 0 {
			return self
		}
		return p.kidAt(slot)
	}
	for len(stack) > 0 {
		en := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		edit := en.edit
		tree := get(en.slot)
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
				result = p.add(rec{padding: padding, size: size, lookaheadBytes: lookahead,
					symbol: p.symbol(tree), parseState: p.parseState(tree), flags: fl})
			}
		} else {
			r := *p.rec(tree)
			r.padding, r.size = padding, size
			r.flags |= heapHasChanges
			if r.childCount > 0 {
				r.childrenStart = p.copyKids(r.childrenStart, r.childCount)
			}
			result = p.add(r)
		}
		if en.slot < 0 {
			self = result
		} else {
			p.setKidAt(en.slot, result)
		}
		var childLeft, childRight Length
		n := p.childCount(result)
		var base uint32
		if n > 0 {
			base = p.rec(result).childrenStart
		}
		for i := uint32(0); i < n; i++ {
			slot := int(base + i)
			child := p.kidAt(slot)
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
			stack = append(stack, entry{slot: slot, edit: childEdit})
		}
	}
	return self
}

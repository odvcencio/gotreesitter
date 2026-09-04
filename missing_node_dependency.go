package gotreesitter

import "unsafe"

// missingNodeDependency preserves C's padding and lookahead metadata for one
// recovery-inserted leaf. The visible node stays zero width.
type missingNodeDependency struct {
	stackByte      uint32
	stackPoint     Point
	paddingBytes   uint32
	paddingExtent  Point
	lookaheadBytes uint32
}

type missingNodeDependencyEntry struct {
	node       *Node
	dependency missingNodeDependency
}

func missingNodeDependencyEntryBytesForCap(capacity int) int64 {
	if capacity <= 0 {
		return 0
	}
	return int64(capacity) * int64(unsafe.Sizeof(missingNodeDependencyEntry{}))
}

func (d missingNodeDependency) positionedByte() (uint32, bool) {
	if ^uint32(0)-d.stackByte < d.paddingBytes {
		return 0, false
	}
	return d.stackByte + d.paddingBytes, true
}

func (d missingNodeDependency) positionedPoint() (Point, bool) {
	if d.paddingExtent.Row == 0 {
		if ^uint32(0)-d.stackPoint.Column < d.paddingExtent.Column {
			return Point{}, false
		}
		return Point{Row: d.stackPoint.Row, Column: d.stackPoint.Column + d.paddingExtent.Column}, true
	}
	if ^uint32(0)-d.stackPoint.Row < d.paddingExtent.Row {
		return Point{}, false
	}
	return Point{Row: d.stackPoint.Row + d.paddingExtent.Row, Column: d.paddingExtent.Column}, true
}

func (d missingNodeDependency) endByte() (uint32, bool) {
	positioned, ok := d.positionedByte()
	if !ok || ^uint32(0)-positioned < d.lookaheadBytes {
		return 0, false
	}
	return positioned + d.lookaheadBytes, true
}

func (a *nodeArena) setMissingNodeDependency(node *Node, dependency missingNodeDependency) bool {
	if a == nil || node == nil || node.ownerArena != a || !node.isMissing() || node.startByte != node.endByte {
		return false
	}
	positionedByte, byteOK := dependency.positionedByte()
	positionedPoint, pointOK := dependency.positionedPoint()
	if !byteOK || !pointOK || positionedByte != node.startByte || positionedPoint != node.startPoint || node.startPoint != node.endPoint {
		return false
	}
	if _, ok := dependency.endByte(); !ok {
		return false
	}
	for index := range a.missingNodeDependencies {
		if a.missingNodeDependencies[index].node == node {
			a.missingNodeDependencies[index].dependency = dependency
			return true
		}
	}
	before := cap(a.missingNodeDependencies)
	a.missingNodeDependencies = append(a.missingNodeDependencies, missingNodeDependencyEntry{node: node, dependency: dependency})
	a.allocatedBytes += missingNodeDependencyEntryBytesForCap(cap(a.missingNodeDependencies) - before)
	return true
}

func missingNodeDependencyForNode(node *Node) (missingNodeDependency, bool) {
	if node == nil || node.ownerArena == nil || !node.isMissing() {
		return missingNodeDependency{}, false
	}
	entry, present := missingNodeDependencyEntryForNode(node)
	if present {
		positionedByte, byteOK := entry.dependency.positionedByte()
		positionedPoint, pointOK := entry.dependency.positionedPoint()
		if !byteOK || !pointOK || positionedByte != node.startByte || node.startByte != node.endByte ||
			positionedPoint != node.startPoint || node.startPoint != node.endPoint {
			return missingNodeDependency{}, false
		}
		if _, ok := entry.dependency.endByte(); !ok {
			return missingNodeDependency{}, false
		}
		return entry.dependency, true
	}
	return missingNodeDependency{}, false
}

func missingNodeDependencyEntryForNode(node *Node) (missingNodeDependencyEntry, bool) {
	if node == nil || node.ownerArena == nil || !node.isMissing() {
		return missingNodeDependencyEntry{}, false
	}
	for _, entry := range node.ownerArena.missingNodeDependencies {
		if entry.node == node {
			return entry, true
		}
	}
	return missingNodeDependencyEntry{}, false
}

func nodeEndsBeforeEditDependency(node *Node, editStart uint32) bool {
	if dependency, ok := missingNodeDependencyForNode(node); ok {
		end, valid := dependency.endByte()
		return valid && end < editStart
	}
	if _, present := missingNodeDependencyEntryForNode(node); present {
		return false
	}
	if node == nil {
		return true
	}
	if node.hasError() {
		for i := 0; i < nodeChildCountNoMaterialize(node); i++ {
			child, ok := nodeChildEntryAtNoMaterialize(node, i)
			if ok && !stackEntryEndsBeforeEditDependency(node.ownerArena, child, editStart) {
				return false
			}
		}
	}
	return node.endByte <= editStart
}

// stackEntryEndsBeforeEditDependency keeps lazy final-child and pending-parent
// entries reachable when a missing descendant depends on bytes after its span.
func stackEntryEndsBeforeEditDependency(arena *nodeArena, entry stackEntry, editStart uint32) bool {
	if !stackEntryHasNode(entry) {
		return true
	}
	if node := stackEntryNode(entry); node != nil {
		if !nodeEndsBeforeEditDependency(node, editStart) {
			return false
		}
		if !node.hasError() {
			return true
		}
		for i := 0; i < nodeChildCountNoMaterialize(node); i++ {
			child, ok := nodeChildEntryAtNoMaterialize(node, i)
			if ok && !stackEntryEndsBeforeEditDependency(node.ownerArena, child, editStart) {
				return false
			}
		}
		return true
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		if parent.endByte > editStart {
			return false
		}
		if !parent.hasError() {
			return true
		}
		for i := 0; i < parent.childEntryCount(); i++ {
			child := parent.childEntry(arena, i)
			if stackEntryHasNode(child) && !stackEntryEndsBeforeEditDependency(arena, child, editStart) {
				return false
			}
		}
		return true
	}
	return stackEntryNodeEndByte(entry) <= editStart
}

// editMissingNodeDependency applies C's edit dependency boundary to one
// zero-width missing leaf. It returns true when the edit reached the padding
// or lookahead dependency and the ordinary span-only path must not skip it.
func editMissingNodeDependency(node *Node, edit InputEdit, byteDelta, rowDelta int64) bool {
	dependency, ok := missingNodeDependencyForNode(node)
	if !ok {
		if _, present := missingNodeDependencyEntryForNode(node); present {
			node.setDirty(true)
			if perfCountersEnabled {
				perfRecordNodeEditMarked()
			}
			return true
		}
		return false
	}
	dependencyEnd, _ := dependency.endByte()
	isNoop := inputEditBytesAreNoop(edit)
	if edit.StartByte > dependencyEnd || (isNoop && edit.StartByte == dependencyEnd) ||
		(edit.OldEndByte <= dependency.stackByte && edit.StartByte < dependency.stackByte) {
		return false
	}
	positionedByte, _ := dependency.positionedByte()
	positionedPoint, _ := dependency.positionedPoint()
	if edit.StartByte < dependency.stackByte && edit.OldEndByte > dependency.stackByte {
		dependency.stackByte = edit.NewEndByte
		dependency.stackPoint = edit.NewEndPoint
	}
	node.setDirty(true)
	if perfCountersEnabled {
		perfRecordNodeEditMarked()
	}

	if edit.OldEndByte <= positionedByte {
		positionedByte = addUint32Delta(positionedByte, byteDelta)
		positionedPoint = shiftPointAfterEdit(positionedPoint, edit, rowDelta)
	} else if edit.StartByte < positionedByte {
		positionedByte = edit.NewEndByte
		positionedPoint = edit.NewEndPoint
	}
	if positionedByte < dependency.stackByte {
		positionedByte = dependency.stackByte
		positionedPoint = dependency.stackPoint
	}
	dependency.paddingBytes = positionedByte - dependency.stackByte
	paddingExtent, valid := pointExtentBetween(dependency.stackPoint, positionedPoint)
	if !valid {
		paddingExtent = Point{}
		positionedPoint = dependency.stackPoint
		dependency.paddingBytes = 0
	}
	dependency.paddingExtent = paddingExtent
	node.startByte = positionedByte
	node.endByte = positionedByte
	node.startPoint = positionedPoint
	node.endPoint = positionedPoint
	if !node.ownerArena.setMissingNodeDependency(node, dependency) {
		node.setDirty(true)
	}
	return true
}

func missingNodeDependencyNoopAtEnd(node *Node, edit InputEdit) bool {
	if !inputEditBytesAreNoop(edit) {
		return false
	}
	dependency, ok := missingNodeDependencyForNode(node)
	if !ok {
		return false
	}
	end, valid := dependency.endByte()
	return valid && edit.StartByte == end
}

func inputEditBytesAreNoop(edit InputEdit) bool {
	return edit.StartByte == edit.OldEndByte && edit.OldEndByte == edit.NewEndByte
}

func copyMissingNodeDependency(dst, src *Node, offset *cloneOffset) bool {
	dependency, ok := missingNodeDependencyForNode(src)
	if !ok || dst == nil || dst.ownerArena == nil {
		return false
	}
	if offset != nil {
		dependency.stackByte = addUint32Delta(dependency.stackByte, int64(offset.byteDelta))
		dependency.stackPoint = offset.offsetPoint(dependency.stackPoint)
	}
	return dst.ownerArena.setMissingNodeDependency(dst, dependency)
}

func missingNodeDependencyFromToken(token Token) (missingNodeDependency, bool) {
	if !token.Missing || !token.missingDependencyExact || token.StartByte < token.missingStackByte {
		return missingNodeDependency{}, false
	}
	paddingExtent, ok := pointExtentBetween(token.missingStackPoint, token.StartPoint)
	lookaheadEndByte := tokenLookaheadEndByte(token)
	if !ok || lookaheadEndByte < token.missingStackByte {
		return missingNodeDependency{}, false
	}
	dependency := missingNodeDependency{
		stackByte: token.missingStackByte, stackPoint: token.missingStackPoint,
		paddingBytes: token.StartByte - token.missingStackByte, paddingExtent: paddingExtent,
		lookaheadBytes: lookaheadEndByte - token.missingStackByte,
	}
	positionedByte, byteOK := dependency.positionedByte()
	positionedPoint, pointOK := dependency.positionedPoint()
	if !byteOK || !pointOK || positionedByte != token.StartByte || positionedPoint != token.StartPoint || token.StartByte != token.EndByte {
		return missingNodeDependency{}, false
	}
	if _, ok := dependency.endByte(); !ok {
		return missingNodeDependency{}, false
	}
	return dependency, true
}

func (a *nodeArena) resetMissingNodeDependencies() {
	if a == nil {
		return
	}
	clear(a.missingNodeDependencies)
	a.missingNodeDependencies = a.missingNodeDependencies[:0]
}

func pointExtentBetween(start, end Point) (Point, bool) {
	if end.Row < start.Row || (end.Row == start.Row && end.Column < start.Column) {
		return Point{}, false
	}
	if end.Row == start.Row {
		return Point{Column: end.Column - start.Column}, true
	}
	return Point{Row: end.Row - start.Row, Column: end.Column}, true
}

// recoveryMissingNodeDependency mirrors the lexer reset that C performs
// before it inserts a missing token. A reset outside all included ranges
// snaps to the next range start. A reset inside a range keeps zero padding.
func (p *Parser) recoveryMissingNodeDependency(stackByte uint32, stackPoint Point, lookahead Token) (missingNodeDependency, bool) {
	lookaheadEndByte := tokenLookaheadEndByte(lookahead)
	if p == nil || lookaheadEndByte < stackByte {
		return missingNodeDependency{}, false
	}
	positionedByte := stackByte
	positionedPoint := stackPoint
	for _, included := range p.included {
		if stackByte >= included.StartByte && stackByte < included.EndByte {
			if stackByte == included.StartByte {
				positionedPoint = included.StartPoint
			}
			break
		}
		if stackByte < included.StartByte {
			positionedByte = included.StartByte
			positionedPoint = included.StartPoint
			break
		}
	}
	paddingExtent, ok := pointExtentBetween(stackPoint, positionedPoint)
	if !ok || positionedByte < stackByte {
		return missingNodeDependency{}, false
	}
	dependency := missingNodeDependency{
		stackByte:      stackByte,
		stackPoint:     stackPoint,
		paddingBytes:   positionedByte - stackByte,
		paddingExtent:  paddingExtent,
		lookaheadBytes: lookaheadEndByte - stackByte,
	}
	if _, ok := dependency.endByte(); !ok {
		return missingNodeDependency{}, false
	}
	return dependency, true
}

func recoveryStackPosition(source []byte, stack *glrStack, lookahead Token) (uint32, Point, bool) {
	if stack == nil {
		return 0, Point{}, false
	}
	if top := stack.top(); stackEntryHasNode(top) && stackEntryNodeEndByte(top) <= lookahead.StartByte {
		return stackEntryNodeEndByte(top), stackEntryNodeEndPoint(top), true
	}
	stackByte := stack.byteOffset
	if stackByte > lookahead.StartByte || uint64(stackByte) > uint64(len(source)) {
		return 0, Point{}, false
	}
	if stackByte == lookahead.StartByte {
		return stackByte, lookahead.StartPoint, true
	}
	return stackByte, advancePointByBytes(Point{}, source[:stackByte]), true
}

func (p *Parser) recoveryMissingToken(source []byte, stack *glrStack, symbol Symbol, lookahead Token) (Token, bool) {
	stackByte, stackPoint, ok := recoveryStackPosition(source, stack, lookahead)
	if !ok {
		return Token{}, false
	}
	dependency, exact := p.recoveryMissingNodeDependency(stackByte, stackPoint, lookahead)
	if !exact {
		return Token{}, false
	}
	positionedByte, _ := dependency.positionedByte()
	positionedPoint, _ := dependency.positionedPoint()
	return Token{
		Symbol: symbol, StartByte: positionedByte, EndByte: positionedByte,
		StartPoint: positionedPoint, EndPoint: positionedPoint, Missing: true,
		lexerLookaheadEndByte: tokenLookaheadEndByte(lookahead),
		missingStackByte:      stackByte, missingStackPoint: stackPoint, missingDependencyExact: true,
	}, true
}

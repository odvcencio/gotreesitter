package gotreesitter

import "bytes"

type reuseFrame struct {
	node       *Node
	underDirty bool
}

// reuseCursor incrementally walks reusable nodes from an old tree in
// pre-order, caching candidates for the current token start byte.
type reuseCursor struct {
	sourceLen uint32
	oldSource []byte
	newSource []byte
	minEditAt uint32
	hasEdits  bool

	stack []reuseFrame
	next  *Node

	cachedStart      uint32
	cachedStartValid bool
	cached           []*Node
}

// reuseScratch holds reusable buffers for incremental reuse traversal.
type reuseScratch struct {
	stack []reuseFrame
	cache []*Node
}

func (c *reuseCursor) reset(oldTree *Tree, source []byte, scratch *reuseScratch) *reuseCursor {
	if oldTree == nil || oldTree.RootNode() == nil {
		return nil
	}
	if scratch == nil {
		scratch = &reuseScratch{}
	}

	c.sourceLen = uint32(len(source))
	c.oldSource = oldTree.source
	c.newSource = source
	c.minEditAt = 0
	c.hasEdits = len(oldTree.edits) > 0
	if c.hasEdits {
		c.minEditAt = oldTree.edits[0].StartByte
		for i := 1; i < len(oldTree.edits); i++ {
			if oldTree.edits[i].StartByte < c.minEditAt {
				c.minEditAt = oldTree.edits[i].StartByte
			}
		}
	}
	c.stack = scratch.stack[:0]
	c.next = nil
	c.cachedStart = 0
	c.cachedStartValid = false
	c.cached = scratch.cache[:0]

	c.stack = append(c.stack, reuseFrame{node: oldTree.RootNode()})
	return c
}

func (c *reuseCursor) commitScratch(scratch *reuseScratch) {
	if scratch == nil {
		return
	}
	scratch.stack = c.stack[:0]
	scratch.cache = c.cached[:0]
}

func (c *reuseCursor) candidates(start uint32) []*Node {
	if c == nil {
		return nil
	}
	if c.cachedStartValid {
		if start == c.cachedStart {
			return c.cached
		}
		if start < c.cachedStart {
			return nil
		}
	}

	c.cached = c.cached[:0]
	c.cachedStart = start
	c.cachedStartValid = true

	for {
		n := c.peek()
		if n == nil {
			return c.cached
		}

		if n.startByte < start {
			c.pop()
			continue
		}
		if n.startByte > start {
			return c.cached
		}

		for {
			n = c.peek()
			if n == nil || n.startByte != start {
				return c.cached
			}
			c.cached = append(c.cached, c.pop())
		}
	}
}

func (c *reuseCursor) peek() *Node {
	if c.next != nil {
		return c.next
	}
	c.next = c.advance()
	return c.next
}

func (c *reuseCursor) pop() *Node {
	n := c.peek()
	c.next = nil
	return n
}

func (c *reuseCursor) advance() *Node {
	for len(c.stack) > 0 {
		last := len(c.stack) - 1
		frame := c.stack[last]
		c.stack = c.stack[:last]
		cur := frame.node
		if cur == nil {
			continue
		}

		dirtyHere := cur.dirty
		if dirtyHere {
			if nodeBytesEqual(cur.startByte, cur.endByte, c.oldSource, c.newSource) {
				// Undo edit path: unchanged bytes can be reused safely.
				cur.dirty = false
				dirtyHere = false
			}
		}

		childUnderDirty := frame.underDirty || dirtyHere

		children := cur.children
		for i := len(children) - 1; i >= 0; i-- {
			c.stack = append(c.stack, reuseFrame{
				node:       children[i],
				underDirty: childUnderDirty,
			})
		}

		if frame.underDirty && c.hasEdits && cur.endByte <= c.minEditAt {
			continue
		}
		if cur.hasError || cur.endByte <= cur.startByte || cur.endByte > c.sourceLen {
			continue
		}
		if dirtyHere {
			continue
		}
		return cur
	}
	return nil
}

func nodeBytesEqual(start, end uint32, oldSource, newSource []byte) bool {
	if end < start {
		return false
	}
	if end > uint32(len(oldSource)) || end > uint32(len(newSource)) {
		return false
	}
	return bytes.Equal(oldSource[start:end], newSource[start:end])
}

// tryReuseSubtree attempts to reuse an old subtree at the current lookahead.
// On success it appends the reused node to the stack and returns the first
// lookahead token that begins at or after the node's end byte.
func (p *Parser) tryReuseSubtree(s *glrStack, lookahead Token, ts TokenSource, idx *reuseCursor, entryScratch *glrEntryScratch) (Token, bool) {
	candidates := idx.candidates(lookahead.StartByte)
	if len(candidates) == 0 {
		return lookahead, false
	}

	state := s.top().state
	for _, n := range candidates {
		nextState, ok := p.reuseTargetState(state, n, lookahead)
		if !ok {
			continue
		}

		s.push(nextState, n, entryScratch)

		// If the reused node reaches EOF, we can synthesize EOF directly
		// instead of consuming every trailing token.
		if n.EndByte() == idx.sourceLen {
			pt := n.EndPoint()
			return Token{
				Symbol:     0,
				StartByte:  idx.sourceLen,
				EndByte:    idx.sourceLen,
				StartPoint: pt,
				EndPoint:   pt,
			}, true
		}

		if skipper, ok := ts.(ByteSkippableTokenSource); ok {
			return skipper.SkipToByte(n.EndByte()), true
		}

		tok := lookahead
		for tok.Symbol != 0 && tok.StartByte < n.EndByte() {
			tok = ts.Next()
		}
		return tok, true
	}

	return lookahead, false
}

func (p *Parser) reuseTargetState(state StateID, n *Node, lookahead Token) (StateID, bool) {
	// Leaf reuse must match the current lookahead token symbol.
	if n.ChildCount() == 0 {
		if n.Symbol() != lookahead.Symbol {
			return 0, false
		}

		action := p.lookupAction(state, n.Symbol())
		if action == nil || len(action.Actions) == 0 {
			return 0, false
		}

		// Extra-token shifts keep the parser state unchanged.
		if action.Actions[0].Type == ParseActionShift && action.Actions[0].Extra {
			return state, true
		}

		for _, act := range action.Actions {
			if act.Type == ParseActionShift {
				return act.State, true
			}
		}
		return 0, false
	}

	gotoState := p.lookupGoto(state, n.Symbol())
	if gotoState == 0 {
		return 0, false
	}
	if gotoState != n.parseState {
		return 0, false
	}
	return gotoState, true
}

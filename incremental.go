package gotreesitter

// reuseIndex groups clean subtrees from a previous tree by start byte.
type reuseIndex struct {
	byStart   map[uint32][]*Node
	offsets   []uint32
	nodes     []*Node
	sourceLen uint32
}

// reuseScratch holds reusable buffers for incremental-index construction.
type reuseScratch struct {
	counts   []uint32
	offsets  []uint32
	nodes    []*Node
	starts   []uint32
	gathered []*Node
	stack    []*Node
}

func (idx *reuseIndex) candidates(start uint32) []*Node {
	if idx == nil {
		return nil
	}
	if idx.offsets != nil {
		i := int(start)
		if i < 0 || i+1 >= len(idx.offsets) {
			return nil
		}
		lo := idx.offsets[i]
		hi := idx.offsets[i+1]
		return idx.nodes[lo:hi]
	}
	return idx.byStart[start]
}

// buildReuseIndex returns an index of reusable nodes from oldTree.
// Nodes marked with hasError are excluded because that flag is also used
// as the "dirty" marker by Tree.Edit.
func buildReuseIndex(oldTree *Tree, source []byte, scratch *reuseScratch) *reuseIndex {
	if oldTree == nil || oldTree.RootNode() == nil {
		return nil
	}

	sourceLen := uint32(len(source))

	// If no edits were recorded and the source is unchanged, the whole root
	// can be reused directly without building a full index.
	if len(oldTree.edits) == 0 && oldTree.root != nil &&
			!oldTree.root.hasError &&
			oldTree.root.startByte == 0 &&
			oldTree.root.endByte == sourceLen {
		return &reuseIndex{
			byStart:   map[uint32][]*Node{0: {oldTree.root}},
			sourceLen: sourceLen,
		}
	}

	if scratch == nil {
		scratch = &reuseScratch{}
	}

	// Most token starts are a few bytes apart, so a coarse hint reduces map
	// growth without over-allocating on large files.
	capHint := int(sourceLen / 4)
	if capHint < 64 {
		capHint = 64
	}

	root := oldTree.RootNode()
	gathered, starts := gatherReusableNodes(root, sourceLen, scratch)
	total := len(gathered)
	if total == 0 {
		return nil
	}

	// Dense packed buckets avoid map hashing for common editor-size files.
	// 256 KiB source => ~2-3 MiB temporary indexing buffers.
	const denseThreshold = 256 * 1024
	if sourceLen <= denseThreshold {
		bucketCount := int(sourceLen) + 1
		counts := ensureUint32Len(scratch.counts, bucketCount)
		for i := 0; i < bucketCount; i++ {
			counts[i] = 0
		}
		for _, start := range starts {
			counts[start]++
		}

		offsets := ensureUint32Len(scratch.offsets, bucketCount+1)
		offsets[0] = 0
		for i := 0; i < bucketCount; i++ {
			offsets[i+1] = offsets[i] + counts[i]
		}

		nodes := ensureNodeLen(scratch.nodes, total)
		// Reuse `counts` as the mutable cursor array to avoid an extra allocation.
		copy(counts, offsets[:bucketCount])
		for i, n := range gathered {
			start := starts[i]
			pos := counts[start]
			nodes[pos] = n
			counts[start] = pos + 1
		}

		scratch.counts = counts
		scratch.offsets = offsets
		scratch.nodes = nodes

		return &reuseIndex{
			offsets:   offsets[:bucketCount+1],
			nodes:     nodes[:total],
			sourceLen: sourceLen,
		}
	}

	byStart := make(map[uint32][]*Node, capHint)
	for i, n := range gathered {
		start := starts[i]
		byStart[start] = append(byStart[start], n)
	}
	return &reuseIndex{
		byStart:   byStart,
		sourceLen: sourceLen,
	}
}

func gatherReusableNodes(n *Node, sourceLen uint32, scratch *reuseScratch) ([]*Node, []uint32) {
	if n == nil {
		return nil, nil
	}

	gathered := scratch.gathered[:0]
	starts := scratch.starts[:0]

	// Explicit stack preserves pre-order traversal (parent before children),
	// which means candidates at the same start byte are already in widest-first
	// order and do not require an extra sort pass.
	stack := scratch.stack[:0]
	stack = append(stack, n)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if !cur.hasError && cur.endByte > cur.startByte && cur.endByte <= sourceLen {
			gathered = append(gathered, cur)
			starts = append(starts, cur.startByte)
		}

		children := cur.children
		if len(children) == 0 {
			continue
		}
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}

	scratch.gathered = gathered
	scratch.starts = starts
	scratch.stack = stack
	return gathered, starts
}

func ensureUint32Len(buf []uint32, n int) []uint32 {
	if cap(buf) < n {
		return make([]uint32, n)
	}
	return buf[:n]
}

func ensureNodeLen(buf []*Node, n int) []*Node {
	if cap(buf) < n {
		return make([]*Node, n)
	}
	return buf[:n]
}

// tryReuseSubtree attempts to reuse an old subtree at the current lookahead.
// On success it appends the reused node to the stack and returns the first
// lookahead token that begins at or after the node's end byte.
func (p *Parser) tryReuseSubtree(s *glrStack, lookahead Token, ts TokenSource, idx *reuseIndex) (Token, bool) {
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

		s.entries = append(s.entries, stackEntry{state: nextState, node: n})

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
	return gotoState, true
}

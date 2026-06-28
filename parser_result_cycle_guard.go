package gotreesitter

// stripResultTreeSelfCycles enforces that no node is its own descendant.
//
// The trailing-EOF error recovery clones the GSS (sharing node payload
// pointers) and re-reduces, which can leave a recovered node, e.g. a
// `source_file`, referencing itself as a child. The raw .children slice can
// look fine while the materialized child view is cyclic, so every later full
// tree walk (parent-link wiring, the compat normalizers, recovery trimming)
// recurses or loops without end: a fatal stack overflow or an indefinite hang.
//
// This pass walks the tree once with an explicit stack and a visited set, so it
// terminates even on an already-cyclic graph. For each node it materializes the
// children (clearing the arena refs so .children becomes authoritative) and
// drops any child edge that points at the node itself or an ancestor on the
// current path. It runs only on recovered node trees, so normal parses are
// untouched.
//
// The underlying cycle should ideally be prevented in the recovery clone/reduce
// itself; this guard keeps the returned tree well-formed in the meantime. See
// issue #110.
func stripResultTreeSelfCycles(root *Node) {
	if root == nil {
		return
	}
	type frame struct {
		n   *Node
		idx int
	}
	black := make(map[*Node]struct{})
	onPath := map[*Node]struct{}{root: {}}
	stack := []frame{{root, 0}}

	for len(stack) > 0 {
		i := len(stack) - 1
		n := stack[i].n
		children := nodeChildrenForReason(n, materializeForNormalization)

		descended := false
		for stack[i].idx < len(children) {
			c := children[stack[i].idx]
			if c == nil {
				stack[i].idx++
				continue
			}
			if _, ancestor := onPath[c]; ancestor {
				children = removeResultTreeChildAt(n, children, stack[i].idx)
				continue
			}
			if _, done := black[c]; done {
				stack[i].idx++
				continue
			}
			stack[i].idx++
			onPath[c] = struct{}{}
			stack = append(stack, frame{c, 0})
			descended = true
			break
		}
		if descended {
			continue
		}
		delete(onPath, n)
		black[n] = struct{}{}
		stack = stack[:len(stack)-1]
	}
}

func removeResultTreeChildAt(n *Node, children []*Node, idx int) []*Node {
	oldLen := len(children)
	n.children = append(children[:idx], children[idx+1:]...)
	if len(n.fieldIDs) == oldLen {
		n.fieldIDs = append(n.fieldIDs[:idx], n.fieldIDs[idx+1:]...)
	}
	if len(n.fieldSources) == oldLen {
		n.fieldSources = append(n.fieldSources[:idx], n.fieldSources[idx+1:]...)
	}
	return n.children
}

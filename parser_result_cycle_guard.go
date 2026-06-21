package gotreesitter

// stripResultTreeSelfCycles enforces the structural invariant that no node is
// its own descendant.
//
// On recovered input the GLR start-symbol reduce can encode a final-child-ref
// that points back at the parent node itself — e.g. a `source_file` whose
// materialized child at some index is the same `source_file` pointer. The raw
// .children slice looks fine, but the materialized child view is cyclic, so
// every subsequent full tree walk (parent-link wiring, the go-compat
// normalizers, recovery whitespace trimming, …) recurses or loops without end,
// producing a fatal stack overflow or an indefinite hang.
//
// This pass walks the tree once with an explicit stack and a visited set, so it
// terminates even on an already-cyclic graph. For every node it materializes
// the children (which clears the arena refs and makes .children authoritative),
// then drops any child edge that points at the node itself or at an ancestor on
// the current path. The result is an acyclic tree the rest of the pipeline can
// safely traverse. See issue #110.
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
			if _, ancestor := onPath[c]; c == n || ancestor {
				n.children = append(children[:stack[i].idx], children[stack[i].idx+1:]...)
				children = n.children
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

package gotreesitter

// incrementalTreesStructurallyEqual checks every public tree property used
// by the incremental parity gate. It runs only when a recovery frontier or
// a top-level state mismatch requires a fresh result check.
func incrementalTreesStructurallyEqual(a, b *Tree, lang *Language) bool {
	if a == nil || b == nil {
		return a == b
	}
	type pair struct{ a, b *Node }
	stack := []pair{{a.RootNode(), b.RootNode()}}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		left, right := current.a, current.b
		if left == nil || right == nil {
			if left != right {
				return false
			}
			continue
		}
		if left.Symbol() != right.Symbol() || left.Range() != right.Range() ||
			left.IsNamed() != right.IsNamed() || left.IsMissing() != right.IsMissing() ||
			left.IsExtra() != right.IsExtra() || left.IsError() != right.IsError() ||
			left.HasError() != right.HasError() || left.ChildCount() != right.ChildCount() {
			return false
		}
		for i := left.ChildCount() - 1; i >= 0; i-- {
			if left.FieldNameForChild(i, lang) != right.FieldNameForChild(i, lang) {
				return false
			}
			stack = append(stack, pair{left.Child(i), right.Child(i)})
		}
	}
	return true
}

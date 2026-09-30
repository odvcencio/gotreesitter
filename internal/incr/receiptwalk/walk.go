// Package receiptwalk bounds traversal while publishing incremental receipts.
package receiptwalk

// Walk visits selected nodes without retaining callbacks or node references.
// Selection also decides whether to descend. Poll runs once per 256 inspected
// nodes, including rejected nodes, and its error stops the traversal.
func Walk[T any](nodes []T, depth, maxDepth int, children func(T) []T, selectNode func(T, int) (bool, bool), visit func(T), poll func() error, inspected *int) error {
	if depth > maxDepth {
		return nil
	}
	for _, node := range nodes {
		selected, descend := selectNode(node, depth)
		if selected {
			visit(node)
		}
		*inspected++
		if *inspected&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
		if descend {
			if err := Walk(children(node), depth+1, maxDepth, children, selectNode, visit, poll, inspected); err != nil {
				return err
			}
		}
	}
	return nil
}

package recover

// ContainsMissing follows retained production children, including hidden
// tokens that were omitted from the public tree. Shared histories are visited
// once, so a merged recovery graph cannot cause repeated subtree walks.
func ContainsMissing[T comparable](root T, inspect func(T) (bool, int), childAt func(T, int) (T, bool)) bool {
	seen := make(map[T]struct{})
	pending := []T{root}
	for len(pending) > 0 {
		item := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		missing, count := inspect(item)
		if missing {
			return true
		}
		for i := 0; i < count; i++ {
			if child, ok := childAt(item, i); ok {
				pending = append(pending, child)
			}
		}
	}
	return false
}

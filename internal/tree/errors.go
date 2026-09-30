// Package tree contains source-independent output rules shared by the engines.
package tree

// ExternalPaddingExtendsEnd retains the padding consumed before a hidden,
// zero-width scanner token. C includes that padding in its reduced span.
func ExternalPaddingExtendsEnd(external bool, start, end, parentEnd uint32) bool {
	return external && start == end && end > parentEnd
}

// AnyChildHasError reads the raw reduction children, including hidden leaves
// that will be omitted from the public tree. Their errors still belong to the
// parent, as in the locked C runtime's subtree error-cost aggregation.
func AnyChildHasError[T any](children []T, start, end int, hasError func(T) bool) bool {
	start = min(len(children), max(0, start))
	end = max(start, min(len(children), end))
	for _, child := range children[start:end] {
		if hasError(child) {
			return true
		}
	}
	return false
}

package lexproof

// SameStorage authenticates a lexer's table slice against its immutable owner.
func SameStorage[T any](a, b []T) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

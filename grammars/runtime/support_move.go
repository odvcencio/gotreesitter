//go:build !grammar_subset || grammar_subset_move

package grammarruntime

// RegisterMoveSupport registers the scanner support for move.
func RegisterMoveSupport() {
	RegisterExternalScanner("move", MoveExternalScanner{})
}

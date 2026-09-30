package lexproof

// Cursor is the observable lexer state compared after a primitive probe.
// Grammar tables and source owners do not need to be copied into its result.
type Cursor struct {
	Position, FailurePosition int
	Row, Column               uint32
	FailureRow, FailureColumn uint32
}

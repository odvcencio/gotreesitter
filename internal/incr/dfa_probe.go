package incr

// DFAProbeState retains exactly the mutable coordinates compared by the
// primitive-edit proof. Each probe still scans a full configured lexer; its
// immutable tables, source and flags do not need copying back to the caller.
type DFAProbeState struct {
	Position                  int
	Row, Column               uint32
	FailurePosition           int
	FailureRow, FailureColumn uint32
}

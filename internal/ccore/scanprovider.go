package ccore

// A scan provider answers the main lex scan of Parser.scan (the generated
// ts_lex of the grammar) in place of the automaton walk. Everything else in
// C/parser.c ts_parser__lex stays in the core: the lex mode of the parse state,
// the external scanner, the error-mode retry, the error skip, the keyword
// automaton, the token cache and the leaf construction. A parse with no
// provider bound walks the automaton as before and allocates nothing for it.

// ScanProvider answers one scan request. Scan takes what scanDFA takes (the
// lex state, the byte position, and the row and column of that position) and
// returns what it returns: whether a token was accepted, the token with its raw
// symbol (keyword promotion is the core's), its spans and LookaheadEnd, and in
// fail the position where a failed attempt began and where it ended. A
// successful early builtin end sets Token.BuiltinEnd so symbol 0 is
// distinguishable from skipped whitespace. A
// provider reads the bytes of the source the parse was given. It must give the
// same answer for the same request, whatever it was asked before.
type ScanProvider interface {
	Scan(lexState uint32, pos int, row, col uint32, out *Token, fail *[2]Length) bool
}

// TableScanID names the table-backed provider.
const TableScanID = "table-v1"

// TableScan is the table-backed scan provider: it runs the automaton walk on
// the lex tables, so its answers equal the walk's by construction. It counts
// its calls, so a test can show that the core asked it.
type TableScan struct {
	states []LexDFAState
	ascii  [][128]int32
	src    []byte
	calls  int
}

// NewTableScan returns a table-backed provider over the main lex tables of t
// and the bytes src.
func NewTableScan(t *Tables, src []byte) *TableScan {
	return &TableScan{states: t.LexStates, ascii: t.LexAscii, src: src}
}

// Identity is the name a certificate lists for this provider.
func (s *TableScan) Identity() string { return TableScanID }

// Calls is the number of scans the provider answered.
func (s *TableScan) Calls() int { return s.calls }

// Scan implements ScanProvider.
func (s *TableScan) Scan(lexState uint32, pos int, row, col uint32, out *Token, fail *[2]Length) bool {
	s.calls++
	return scanDFA(s.states, s.ascii, s.src, lexState, pos, row, col, out, fail)
}

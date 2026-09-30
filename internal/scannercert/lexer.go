package scannercert

import gts "github.com/odvcencio/gotreesitter"

// LexerAPI keeps access to private lexer fields in the root's test-only
// bridges. The production API does not acquire diagnostic constructors.
type LexerAPI struct {
	New                   func([]byte, int) *gts.ExternalLexer
	Clone                 func(*gts.ExternalLexer) *gts.ExternalLexer
	Input                 func(*gts.ExternalLexer) ([]byte, int)
	Observe               func(*gts.ExternalLexer) LexerObservation
	SerializationCapacity int
}

// LexerObservation includes scanner outputs and dependencies that a checkpoint
// restore must reproduce, even if both attempts return the same token kind.
type LexerObservation struct {
	Start, Cursor, End            int
	StartPoint, Point, EndPoint   gts.Point
	Marked, HasResult, ReadColumn bool
	Symbol                        gts.Symbol
	LookaheadEnd, ExaminedEnd     uint32
}

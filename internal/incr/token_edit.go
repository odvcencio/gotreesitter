package incr

// TokenEdit projects byte and point coordinates through an ASCII edit on one
// line. Coordinates inside removed text have no authenticated projection.
type TokenEdit struct {
	Start, OldEnd, NewEnd uint32
	Row                   uint32
}

// TokenReadBound anchors a certified token's read bound at its original width.
// Run-neutral edits preserve scan endpoints before or after the edited run;
// only the net growth of that token can extend an original read. Anchoring
// prevents inverse edits from repeatedly adding conservative padding. Switching
// tokens starts a new anchor using the already authenticated current bound.
// The key refers to a token already retained by the tree, never to source text.
type TokenReadBound struct {
	Key         any
	Width, Span uint32
}

func (b TokenReadBound) Edit(key any, oldWidth, newWidth, span uint32) (TokenReadBound, uint32, bool) {
	if b.Key != key || b.Span == 0 {
		b = TokenReadBound{Key: key, Width: oldWidth, Span: span}
	}
	bound := uint64(b.Span)
	if newWidth > b.Width {
		bound += uint64(newWidth - b.Width)
	}
	return b, uint32(bound), bound > 0 && bound <= uint64(^uint32(0))
}

func (e TokenEdit) Byte(offset uint32) (uint32, bool) {
	if offset <= e.Start {
		return offset, true
	}
	if offset < e.OldEnd {
		return 0, false
	}
	next := int64(offset) + int64(e.NewEnd) - int64(e.OldEnd)
	return uint32(next), next >= 0 && next <= int64(^uint32(0))
}

func (e TokenEdit) Point(offset, row, column uint32) (uint32, uint32, bool) {
	if _, ok := e.Byte(offset); !ok {
		return 0, 0, false
	}
	if offset <= e.Start || row != e.Row {
		return row, column, true
	}
	next := int64(column) + int64(e.NewEnd) - int64(e.OldEnd)
	return row, uint32(next), next >= 0 && next <= int64(^uint32(0))
}

// LengthNeutralScannerEdit requires a scanner-owned certificate for changing
// the length of an existing ASCII run. At every reachable payload and valid-symbol set,
// the edit must preserve Scan's outcome, symbol, marks, and final payload under
// TokenEdit's coordinate projection. Its read frontier may grow by at most the
// added bytes. Column, lookbehind, counters, collected text, and fixed strings
// cannot belong to a certified class. A zero class means unknown.
//
// DFA decisions and keyword election still need independent comparison. This
// does not admit ordinary subtree reuse or certify a scanner's serialization.
func LengthNeutralScannerEdit(scanner any, oldSource, source []byte, e TokenEdit) bool {
	classes, ok := scanner.(interface{ ExternalScannerLengthNeutralASCIIClass(byte) uint8 })
	if !ok || e.Start > e.OldEnd || e.Start > e.NewEnd ||
		uint64(e.OldEnd) > uint64(len(oldSource)) || uint64(e.NewEnd) > uint64(len(source)) ||
		int64(len(source))-int64(len(oldSource)) != int64(e.NewEnd)-int64(e.OldEnd) || e.OldEnd == e.NewEnd {
		return false
	}
	class := uint8(0)
	for _, changed := range [][]byte{oldSource[e.Start:e.OldEnd], source[e.Start:e.NewEnd]} {
		for _, b := range changed {
			if b >= 128 || b == '\n' || b == '\r' {
				return false
			}
			c := classes.ExternalScannerLengthNeutralASCIIClass(b)
			if c == 0 || (class != 0 && class != c) {
				return false
			}
			class = c
		}
	}
	if class == 0 {
		return false
	}
	// A surviving adjacent byte authenticates that this is a run-length edit.
	return (e.Start > 0 && oldSource[e.Start-1] < 128 && classes.ExternalScannerLengthNeutralASCIIClass(oldSource[e.Start-1]) == class) ||
		(uint64(e.OldEnd) < uint64(len(oldSource)) && oldSource[e.OldEnd] < 128 && classes.ExternalScannerLengthNeutralASCIIClass(oldSource[e.OldEnd]) == class)
}

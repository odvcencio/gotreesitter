package incr

import "bytes"

// IdentifierEditToken finds both boundaries in the source, rather than relying
// on the edited node's start: an insertion at a node's start may move that
// node right even when the inserted bytes extend its lexer token to the left.
func IdentifierEditToken(oldSource, source []byte, edit TokenEdit) (uint32, uint32, uint32, bool) {
	start, end, ok := IdentifierEditOldToken(oldSource, edit)
	if !ok {
		return 0, 0, 0, false
	}
	projected := int64(end) + int64(edit.NewEnd) - int64(edit.OldEnd)
	if projected < 0 || projected > int64(len(source)) {
		return 0, 0, 0, false
	}
	oldEnd, ok := IdentifierEditBounds(oldSource, source, edit, start, uint32(projected))
	return start, oldEnd, uint32(projected), ok
}

// IdentifierEditOldToken is the necessary old-source condition for admission.
// Tree.Edit can use it before the new source exists; passing this filter does
// not authenticate the replacement or authorize publication.
func IdentifierEditOldToken(oldSource []byte, edit TokenEdit) (uint32, uint32, bool) {
	if edit.Start > edit.OldEnd || edit.Start > edit.NewEnd || uint64(edit.OldEnd) > uint64(len(oldSource)) {
		return 0, 0, false
	}
	part := func(b byte) bool {
		return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
	}
	start, end := edit.Start, edit.OldEnd
	for start > 0 && part(oldSource[start-1]) {
		start--
		if edit.Start-start > 256 {
			return 0, 0, false
		}
	}
	for uint64(end) < uint64(len(oldSource)) && part(oldSource[end]) {
		end++
		if end-start > 256 {
			return 0, 0, false
		}
	}
	if end-start > 256 || !identifierEditASCII(oldSource[start:end]) {
		return 0, 0, false
	}
	return start, end, true
}

// IdentifierEditBounds restricts a prospective reuse schedule to one surviving
// ASCII identifier. It is an admission filter, never publication authority:
// the caller must authenticate the actual lexer token and producer separately.
func IdentifierEditBounds(oldSource, source []byte, edit TokenEdit, start, end uint32) (uint32, bool) {
	oldEnd := int64(end) + int64(edit.OldEnd) - int64(edit.NewEnd)
	if start >= end || oldEnd <= int64(start) || oldEnd > int64(len(oldSource)) || uint64(end) > uint64(len(source)) ||
		edit.Start < start || edit.Start > edit.OldEnd || edit.Start > edit.NewEnd || edit.OldEnd > uint32(oldEnd) || edit.NewEnd > end ||
		int64(len(source))-int64(len(oldSource)) != int64(edit.NewEnd)-int64(edit.OldEnd) || end-start > 256 || oldEnd-int64(start) > 256 {
		return 0, false
	}
	if !identifierEditASCII(oldSource[start:oldEnd]) || !identifierEditASCII(source[start:end]) ||
		!bytes.Equal(oldSource[:edit.Start], source[:edit.Start]) || !bytes.Equal(oldSource[edit.OldEnd:], source[edit.NewEnd:]) {
		return 0, false
	}
	return uint32(oldEnd), true
}

func identifierEditASCII(s []byte) bool {
	for i, b := range s {
		if b != '_' && !(b >= 'a' && b <= 'z') && !(b >= 'A' && b <= 'Z') && !(i > 0 && b >= '0' && b <= '9') {
			return false
		}
	}
	return len(s) > 0
}

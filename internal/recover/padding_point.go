package recover

// PaddingStartPoint recovers the point before scanner-skipped padding.
// Same-line padding needs no earlier source bytes. After a newline, only
// the preceding line determines the old column; the earlier file prefix
// cannot affect it. Work counts examined bytes for the scaling contract.
func PaddingStartPoint(source []byte, start, end, row, column uint32) (uint32, uint32, uint64, bool) {
	if start > end || uint64(end) > uint64(len(source)) {
		return 0, 0, 0, false
	}
	var lines uint32
	var work uint64
	for _, c := range source[start:end] {
		work++
		if c == '\n' {
			lines++
		}
	}
	if lines == 0 {
		width := end - start
		if start == 0 && end >= 3 && source[0] == 0xef && source[1] == 0xbb && source[2] == 0xbf {
			width -= 3
		}
		if width > column {
			return 0, 0, work, false
		}
		return row, column - width, work, true
	}
	if lines > row {
		return 0, 0, work, false
	}
	column = 0
	for i := int(start) - 1; i >= 0; i-- {
		work++
		if source[i] == '\n' {
			break
		}
		column++
	}
	// Like the scanner lexer, exclude a leading UTF-8 byte order mark.
	if column == start && start >= 3 && source[0] == 0xef && source[1] == 0xbb && source[2] == 0xbf {
		column -= 3
	}
	return row - lines, column, work, true
}

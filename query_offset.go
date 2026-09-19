package gotreesitter

import "bytes"

// applyOffsetWithReader implements the #offset! directive:
//
//	(#offset! @capture start_row start_col end_row end_col)
//
// It shifts the reported range of every node bound to pred.leftCapture by
// the four deltas, following the nvim-treesitter definition: the deltas add
// to the capture's current start and end point (row, column), and the
// capture's byte range is recomputed from the adjusted point using source.
// Columns are byte columns, matching this package's Point convention.
//
// Node spans are immutable, so the adjusted range is recorded on the
// capture itself (see QueryCapture.hasRangeOverride) rather than on the
// node. A capture that already carries an override (from an earlier
// #offset! directive on the same capture name) is adjusted relative to that
// override, matching nvim-treesitter's cumulative behavior.
func applyOffsetWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, reader R) []C {
	for i := range captures {
		if reader.CaptureName(captures[i]) != pred.leftCapture {
			continue
		}

		// The new range comes from the adjusted points, so only the points
		// of the current range are needed here.
		var startPoint, endPoint Point
		if _, _, sp, ep, ok := reader.CaptureRangeOverride(captures[i]); ok {
			startPoint, endPoint = sp, ep
		} else {
			node := reader.CaptureNode(captures[i])
			if reader.IsNil(node) {
				continue
			}
			startPoint = pointForByte(source, reader.StartByte(node))
			endPoint = pointForByte(source, reader.EndByte(node))
		}

		newStartPoint := Point{
			Row:    addOffsetComponent(startPoint.Row, pred.offset[0]),
			Column: addOffsetComponent(startPoint.Column, pred.offset[1]),
		}
		newEndPoint := Point{
			Row:    addOffsetComponent(endPoint.Row, pred.offset[2]),
			Column: addOffsetComponent(endPoint.Column, pred.offset[3]),
		}

		newStartByte := byteForPoint(source, newStartPoint)
		newEndByte := byteForPoint(source, newEndPoint)
		if newEndByte < newStartByte {
			newEndByte = newStartByte
		}

		reader.SetCaptureRangeOverride(&captures[i], newStartByte, newEndByte, newStartPoint, newEndPoint)
	}
	return captures
}

// addOffsetComponent adds a possibly-negative delta to a point coordinate,
// clamping at zero so an offset cannot underflow the unsigned coordinate.
func addOffsetComponent(v uint32, delta int) uint32 {
	result := int64(v) + int64(delta)
	if result < 0 {
		return 0
	}
	return uint32(result)
}

// pointForByte returns the row/column position of byteOffset within source,
// using the same convention as the parser's own Point: Row counts newlines
// before byteOffset, and Column counts bytes since the last newline (or the
// start of source).
func pointForByte(source []byte, byteOffset uint32) Point {
	if int(byteOffset) > len(source) {
		byteOffset = uint32(len(source))
	}
	prefix := source[:byteOffset]
	row := uint32(bytes.Count(prefix, newlineBytes))
	column := byteOffset
	if idx := bytes.LastIndexByte(prefix, '\n'); idx >= 0 {
		column = byteOffset - uint32(idx) - 1
	}
	return Point{Row: row, Column: column}
}

var newlineBytes = []byte{'\n'}

// byteForPoint returns the byte offset in source for point, the inverse of
// pointForByte. A row beyond the available lines clamps to the end of
// source; a column beyond its line's length clamps to that line's end (the
// next newline, or the end of source).
func byteForPoint(source []byte, point Point) uint32 {
	if len(source) == 0 {
		return 0
	}

	lineStart := uint32(0)
	if point.Row > 0 {
		found := uint32(0)
		idx := 0
		for {
			rel := bytes.IndexByte(source[idx:], '\n')
			if rel < 0 {
				return uint32(len(source))
			}
			idx += rel + 1
			found++
			if found == point.Row {
				break
			}
		}
		lineStart = uint32(idx)
	}

	lineEnd := uint32(len(source))
	if rel := bytes.IndexByte(source[lineStart:], '\n'); rel >= 0 {
		lineEnd = lineStart + uint32(rel)
	}

	if point.Column > lineEnd-lineStart {
		return lineEnd
	}
	return lineStart + point.Column
}

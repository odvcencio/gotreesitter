//go:build !gts_no_parsercorephase0

package gotreesitter

import "fmt"

func compactRangeBounds(source []byte, ranges []Range) (uint32, uint32, error) {
	if uint64(len(source)) > uint64(^uint32(0)) {
		return 0, 0, fmt.Errorf("compact source exceeds byte bounds")
	}
	if len(ranges) == 0 {
		return firstNonTriviaByteStart(source), uint32(len(source)), nil
	}
	var previous uint32
	for i, r := range ranges {
		if r.StartByte > r.EndByte || uint64(r.EndByte) > uint64(len(source)) || (i > 0 && r.StartByte < previous) {
			return 0, 0, fmt.Errorf("compact included range exceeds source bounds")
		}
		previous = r.EndByte
	}
	for _, r := range ranges {
		offset := firstNonTriviaByteStart(source[r.StartByte:r.EndByte])
		if offset < r.EndByte-r.StartByte {
			return r.StartByte + offset, previous, nil
		}
	}
	return previous, previous, nil
}

// compactRangeGap checks only source bytes inside the configured ranges.
func compactRangeGap(source []byte, start, end uint32, ranges []Range, poll func() error) (bool, error) {
	if start > end || uint64(end) > uint64(len(source)) {
		return false, fmt.Errorf("compact gap exceeds source bounds")
	}
	if len(ranges) == 0 {
		return diagnosticParserCoreGapIsToleratedWithPoll(source[start:end], poll)
	}
	for _, r := range ranges {
		if poll != nil {
			if err := poll(); err != nil {
				return false, err
			}
		}
		lo, hi := max(start, r.StartByte), min(end, r.EndByte)
		if lo < hi {
			ok, err := diagnosticParserCoreGapIsToleratedWithPoll(source[lo:hi], poll)
			if err != nil || !ok {
				return ok, err
			}
		}
	}
	return true, nil
}

func compactLogicalEOF(source []byte, ranges []Range) uint32 {
	_, end, err := compactRangeBounds(source, ranges)
	if err != nil {
		return uint32(len(source))
	}
	return end
}

// compactRangePointsMatch requires coordinates from the original source.
func compactRangePointsMatch(source []byte, ranges []Range, poll func() error) error {
	var offset uint32
	var point Point
	for _, r := range ranges {
		for _, boundary := range []struct {
			offset uint32
			point  Point
		}{{r.StartByte, r.StartPoint}, {r.EndByte, r.EndPoint}} {
			for offset < boundary.offset {
				if offset&4095 == 0 && poll != nil {
					if err := poll(); err != nil {
						return err
					}
				}
				if source[offset] == '\n' {
					point.Row++
					point.Column = 0
				} else {
					point.Column++
				}
				offset++
			}
			if point != boundary.point {
				return fmt.Errorf("compact included range uses different source coordinates")
			}
		}
	}
	return nil
}

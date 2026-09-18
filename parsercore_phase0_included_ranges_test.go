//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"testing"
)

func TestCompactIncludedRangeValidation(t *testing.T) {
	source := []byte("! x !")
	for _, ranges := range [][]Range{
		{{StartByte: 4, EndByte: 3}}, {{EndByte: 6}}, {{StartByte: 1, EndByte: 4}, {StartByte: 3, EndByte: 5}},
	} {
		if _, _, err := compactRangeBounds(source, ranges); err == nil {
			t.Fatalf("invalid ranges accepted: %+v", ranges)
		}
	}
	ranges := []Range{{StartByte: 1, EndByte: 4}}
	start, end, err := compactRangeBounds(source, ranges)
	if err != nil || start != 2 || end != 4 {
		t.Fatalf("bounds=%d..%d err=%v", start, end, err)
	}
	if ok, err := compactRangeGap(source, 0, 5, ranges, nil); err != nil || ok {
		t.Fatalf("included source byte was omitted: %v %v", ok, err)
	}
	if ok, err := compactRangeGap(source, 3, 5, ranges, nil); err != nil || !ok {
		t.Fatalf("excluded source byte was included: %v %v", ok, err)
	}
	if ok, err := compactRangeGap(source, 0, 6, ranges, nil); err == nil || ok {
		t.Fatal("out-of-bounds gap accepted")
	}
	stopped := errors.New("stopped")
	if _, err := compactRangeGap(source, 0, 5, ranges, func() error { return stopped }); !errors.Is(err, stopped) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestCompactIncludedRangeCoordinates(t *testing.T) {
	source := []byte("!\n x ")
	ranges := []Range{{StartByte: 3, EndByte: 5, StartPoint: Point{Row: 1, Column: 1}, EndPoint: Point{Row: 1, Column: 3}}}
	if err := compactRangePointsMatch(source, ranges, nil); err != nil {
		t.Fatal(err)
	}
	ranges[0].StartPoint.Column = 0
	if err := compactRangePointsMatch(source, ranges, nil); err == nil {
		t.Fatal("different source coordinates accepted")
	}
	stopped := errors.New("stopped")
	if err := compactRangePointsMatch(source, ranges, func() error { return stopped }); !errors.Is(err, stopped) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

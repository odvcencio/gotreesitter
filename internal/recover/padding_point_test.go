package recover

import (
	"bytes"
	"testing"
)

func TestPaddingStartPoint(t *testing.T) {
	for _, tc := range []struct {
		name, source                                 string
		start, end, row, column, wantRow, wantColumn uint32
	}{
		{"same line", "prefix   x", 6, 9, 0, 9, 0, 6},
		{"newline", "prefix \n  x", 6, 10, 1, 2, 0, 6},
		{"multiple lines", "old\nβ \r\n\t \n x", 6, 13, 3, 1, 1, 2},
		{"BOM same line", "\ufeff x", 0, 4, 0, 1, 0, 0},
		{"BOM", "\ufeffx\n x", 4, 6, 1, 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, col, _, ok := PaddingStartPoint([]byte(tc.source), tc.start, tc.end, tc.row, tc.column)
			if !ok || row != tc.wantRow || col != tc.wantColumn {
				t.Fatalf("point=%d:%d valid=%t, want %d:%d", row, col, ok, tc.wantRow, tc.wantColumn)
			}
		})
	}
	for _, tc := range []struct{ start, end, row, column uint32 }{{2, 1, 0, 0}, {0, 4, 0, 4}, {0, 1, 0, 0}} {
		if _, _, _, ok := PaddingStartPoint([]byte(" x"), tc.start, tc.end, tc.row, tc.column); ok {
			t.Fatalf("accepted invalid bounds/point %+v", tc)
		}
	}
}

func TestPaddingStartPointWorkScalesWithInput(t *testing.T) {
	// Probe each line, after a long prefix as well as near the file start.
	// A whole-prefix point calculation would visit quadratically more bytes.
	unit := []byte("previous value \n  ")
	measure := func(lines int) uint64 {
		source := bytes.Repeat(unit, lines)
		var work uint64
		for i := 0; i < lines; i++ {
			start := uint32(i*len(unit) + 14)
			end := uint32((i + 1) * len(unit))
			row, col, n, ok := PaddingStartPoint(source, start, end, uint32(i+1), 2)
			wantColumn := uint32(14)
			if i > 0 {
				wantColumn += 2
			}
			if !ok || row != uint32(i) || col != wantColumn {
				t.Fatalf("line %d point=%d:%d valid=%t", i, row, col, ok)
			}
			work += n
		}
		return work
	}
	small, large := measure(4096), measure(8192)
	if large < small*19/10 || large > small*21/10 {
		t.Fatalf("doubling input changed point work %d→%d, want about 2x", small, large)
	}
	t.Logf("point work: %d→%d examined bytes", small, large)
}

package recover

import "testing"

func TestRelexPrefixPoint(t *testing.T) {
	for _, tc := range []struct {
		name, source            string
		prefix, start, row, col uint32
		wantRow, wantCol        uint32
		ok                      bool
	}{
		{"spaces", "a   b", 1, 4, 0, 4, 0, 1, true},
		{"newline", "a\nb\nx", 3, 4, 2, 0, 1, 1, true},
		{"crlf", "a\r\nx", 1, 3, 1, 0, 0, 1, true},
		{"utf8_column", "é\nx", 2, 3, 1, 0, 0, 2, true},
		{"bad_column", "a   b", 1, 4, 0, 2, 0, 0, false},
		{"bad_row", "a\nx", 1, 2, 0, 0, 0, 0, false},
		{"outside", "x", 0, 2, 0, 2, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, col, ok := RelexPrefixPoint([]byte(tc.source), tc.prefix, tc.start, tc.row, tc.col)
			if row != tc.wantRow || col != tc.wantCol || ok != tc.ok {
				t.Fatalf("point=(%d,%d,%t), want (%d,%d,%t)", row, col, ok, tc.wantRow, tc.wantCol, tc.ok)
			}
		})
	}
}

//go:build !grammar_subset || grammar_subset_authzed || grammar_subset_c || grammar_subset_cpp || grammar_subset_go || grammar_subset_html || grammar_subset_java || grammar_subset_json || grammar_subset_lua || grammar_subset_toml

package grammarruntime

import "testing"

func TestSourceCursorSkipsEscapedNewlineExtras(t *testing.T) {
	src := []byte(" \t\\\r\n  \\\nnext")
	cur := newSourceCursor(src)

	cur.skipSpacesTabsAndEscapedNewlines()

	if got, want := cur.offset, len(src)-len("next"); got != want {
		t.Fatalf("offset = %d, want %d", got, want)
	}
	if got, want := cur.row, uint32(2); got != want {
		t.Fatalf("row = %d, want %d", got, want)
	}
	if got, want := cur.col, uint32(0); got != want {
		t.Fatalf("column = %d, want %d", got, want)
	}
}

func TestSourceCursorRuneUsesByteColumns(t *testing.T) {
	cur := newSourceCursor([]byte("x\u2014\u00e9\n\U0001f41f\xff"))
	want := []struct {
		offset int
		row    uint32
		column uint32
	}{
		{1, 0, 1},
		{4, 0, 4},
		{6, 0, 6},
		{7, 1, 0},
		{11, 1, 4},
		{12, 1, 5},
	}
	for i, next := range want {
		cur.advanceRune()
		if cur.offset != next.offset || cur.row != next.row || cur.col != next.column {
			t.Fatalf("advance %d: offset/row/column = %d/%d/%d, want %d/%d/%d",
				i+1, cur.offset, cur.row, cur.col, next.offset, next.row, next.column)
		}
	}
}

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

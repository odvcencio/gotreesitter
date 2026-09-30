package lexproof

import (
	"bytes"
	"testing"
)

func TestMemoRequiresExactDirectedWindow(t *testing.T) {
	old := bytes.Repeat([]byte("a"), 2048)
	next := bytes.Clone(old)
	edit := Edit{Start: 1024, End: 1025, Column: 1024, EndColumn: 1025}
	next[edit.Start] = 'b'
	var memo Memo
	if !memo.Remember(old, next, edit, 4, 4) {
		t.Fatal("bounded proof not stored")
	}
	if span, ok := memo.Lookup(old, next, edit, 4); !ok || span != 4 {
		t.Fatal("exact proof not found")
	}
	if _, ok := memo.Lookup(next, old, edit, 4); ok {
		t.Fatal("inverse proof inferred")
	}
	if _, ok := memo.Lookup(old, next, edit, 5); ok {
		t.Fatal("different captured frontier accepted")
	}
	changed := edit
	changed.Row++
	if _, ok := memo.Lookup(old, next, changed, 4); ok {
		t.Fatal("different point accepted")
	}
	for _, at := range []int{0, int(edit.Start) - 8, int(edit.End) + 7} {
		mutated := bytes.Clone(next)
		mutated[at] = 'c'
		if _, ok := memo.Lookup(old, mutated, edit, 4); ok {
			t.Fatalf("changed dependency %d accepted", at)
		}
	}
	mutated := bytes.Clone(next)
	mutated[1800] = 'c'
	if _, ok := memo.Lookup(old, mutated, edit, 4); !ok {
		t.Fatal("unexamined suffix invalidated proof")
	}
	next[edit.Start] = 'c'
	if _, ok := memo.Lookup(old, next, edit, 4); ok {
		t.Fatal("memo retained mutable source alias")
	}
}

func TestMemoIncludesPreviousLineAndDeclinesWideWindows(t *testing.T) {
	old := []byte("first line\n1234\n")
	next := bytes.Clone(old)
	edit := Edit{Start: 11, End: 12, Row: 1, EndRow: 1, EndColumn: 1}
	next[11] = '2'
	var memo Memo
	if !memo.Remember(old, next, edit, 2, 2) {
		t.Fatal("small previous-line proof not stored")
	}
	next[4] = 'X'
	if _, ok := memo.Lookup(old, next, edit, 2); ok {
		t.Fatal("preceding line not authenticated")
	}
	old = append(bytes.Repeat([]byte("a"), 600), '\n', '1')
	next = bytes.Clone(old)
	next[601] = '2'
	edit.Start, edit.End = 601, 602
	if memo.Remember(old, next, edit, 2, 2) {
		t.Fatal("unbounded previous-line search cached")
	}
	if memo.Remember(old, next, edit, 512, 512) {
		t.Fatal("wide lookahead cached")
	}
}

func TestMemoDoesNotEvictOnDeclinedStore(t *testing.T) {
	var memo Memo
	edit := Edit{Start: 0, End: 1, EndColumn: 1}
	for _, pair := range [][2]string{{"1", "2"}, {"2", "1"}} {
		if !memo.Remember([]byte(pair[0]), []byte(pair[1]), edit, 2, 2) {
			t.Fatal("proof not stored")
		}
	}
	if memo.Remember([]byte("1"), []byte("2"), Edit{Start: 1, End: 0}, 2, 2) {
		t.Fatal("invalid edit cached")
	}
	for _, pair := range [][2]string{{"1", "2"}, {"2", "1"}} {
		if _, ok := memo.Lookup([]byte(pair[0]), []byte(pair[1]), edit, 2); !ok {
			t.Fatal("decline evicted proof")
		}
	}
	if !memo.Remember([]byte("3"), []byte("4"), edit, 2, 2) {
		t.Fatal("replacement not stored")
	}
	if _, ok := memo.Lookup([]byte("1"), []byte("2"), edit, 2); ok {
		t.Fatal("oldest proof not evicted")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		memo.Lookup([]byte("2"), []byte("1"), edit, 2)
	}); allocs != 0 {
		t.Fatalf("lookup allocations=%g", allocs)
	}
}

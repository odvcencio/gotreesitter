package ccore

import "testing"

// TestIsReservedLargeProductDoesNotWrap: the index set*n of the reserved-word
// table is the product of two 16-bit values and needs 32 bits. With both at
// 65,535 the product is above 2^31; isReserved must not panic and must answer
// false for a table that is far shorter. (On a 64-bit target int would not
// wrap either; the test pins the answer, and the code computes in uint64 so
// that a 32-bit target gives the same answer.)
func TestIsReservedLargeProductDoesNotWrap(t *testing.T) {
	p := &Parser{t: &Tables{
		LexModes:               []LexMode{{ReservedWordSetID: 65535}},
		MaxReservedWordSetSize: 65535,
		ReservedWords:          []Symbol{5, 6, 7},
	}}
	if p.isReserved(0, 5) {
		t.Fatal("a symbol was reported reserved from an index beyond the table")
	}
	p.t.LexModes[0].ReservedWordSetID = 1
	p.t.MaxReservedWordSetSize = 3
	if p.isReserved(0, 5) {
		t.Fatal("set 1 of size 3 starts at index 3, past the table: nothing is reserved")
	}
	p.t.LexModes[0].ReservedWordSetID = 0
	if p.isReserved(0, 5) {
		t.Fatal("set 0 reserves nothing")
	}
}

package incr

import "testing"

func TestForestAttributesRequireIndependentCompleteHistory(t *testing.T) {
	r := NewReads(8)
	r.CertifyForestAttributes()
	r.Record(0, 8)
	if r.CertifiedForestAttributes() {
		t.Fatal("unsealed history certified forest attributes")
	}
	r.Seal()
	if !r.CertifiedForestAttributes() {
		t.Fatal("complete forest history lost its attributes")
	}
	// A pooled recorder's next legacy parse cannot inherit the forest receipt.
	r.Reset(8)
	r.Record(0, 8)
	r.Seal()
	if r.CertifiedForestAttributes() {
		t.Fatal("a later legacy parse inherited forest certification")
	}
	r.Reset(8)
	r.CertifyForestAttributes()
	r.Record(0, 8)
	r.Abstain()
	r.Seal()
	if r.CertifiedForestAttributes() {
		t.Fatal("incomplete history retained forest certification")
	}
}

func TestLookaheadIncludesBoundaryOrigin(t *testing.T) {
	r := NewReads(8)
	r.Record(0, 3)
	r.Record(2, 8) // the following lookahead decides a reduction at byte 2
	r.Seal()
	if count, known := r.Lookahead(2); !known || count != 6 {
		t.Fatalf("boundary lookahead=%d known=%t", count, known)
	}
	if count, known := r.LeafLookahead(2); !known || count != 1 {
		t.Fatalf("leaf absorbed the next token's probe: lookahead=%d known=%t", count, known)
	}
}

func TestLeafLookaheadRetainsFailedEarlierProbes(t *testing.T) {
	r := NewReads(8)
	r.Record(0, 6) // failed longer token before rollback
	r.Record(0, 3)
	r.Record(2, 9) // next token examines EOF
	r.Seal()
	if count, known := r.LeafLookahead(2); !known || count != 4 {
		t.Fatalf("leaf lost the failed probe: lookahead=%d known=%t", count, known)
	}
}

func TestLookaheadIncludesFailedAndOutOfOrderReads(t *testing.T) {
	r := NewReads(12)
	r.Record(0, 3)
	r.Record(4, 8)
	r.Record(0, 10) // discarded long-match probe after lexer rollback
	r.Record(9, 13) // EOF is examined beyond the source
	r.Seal()
	for _, tc := range []struct{ end, want uint32 }{{2, 8}, {8, 2}, {12, 1}} {
		got, ok := r.Lookahead(tc.end)
		if !ok || got != tc.want {
			t.Fatalf("end=%d: %d,%t want %d", tc.end, got, ok, tc.want)
		}
	}
	r.Record(0, 3)
	if _, ok := r.Lookahead(2); ok {
		t.Fatal("mutation of sealed history retained certification")
	}
}

func TestLookaheadBudgetAbstainsBeforeAllocation(t *testing.T) {
	allocated := int64(64)
	r := NewReads(12)
	r.BindBudget(100, 0, &allocated)
	r.Record(0, 3)
	r.Seal()
	if allocated != 64 || r.Bytes() != 64 {
		t.Fatal("unfunded backing array allocated")
	}
	if _, ok := r.Lookahead(2); ok {
		t.Fatal("unrecorded reads authenticated a node")
	}
}

func TestFirstLeafCConditions(t *testing.T) {
	if !FirstLeaf(false, true, true, false, false, false, false, true, false) {
		t.Fatal("equal modes with actions did not reuse")
	}
	if FirstLeaf(true, true, true, false, false, true, false, false, true) {
		t.Fatal("no-lookahead mode reused")
	}
	if FirstLeaf(false, true, true, true, true, true, false, true, false) {
		t.Fatal("keyword capture bypassed keyword rule")
	}
	if FirstLeaf(false, true, false, false, false, true, true, false, true) {
		t.Fatal("empty non-EOF crossed lex modes")
	}
	if !FirstLeaf(false, true, false, false, false, false, false, false, true) {
		t.Fatal("reusable table entry did not reuse")
	}
	if FirstLeaf(false, true, false, false, false, false, false, true, true) {
		t.Fatal("external mode crossed lex modes")
	}
}

func TestLookaheadResetRetainsBoundedStorage(t *testing.T) {
	r := NewReads(12)
	r.Record(0, 4)
	r.Seal()
	before := r.Bytes()
	allocs := testing.AllocsPerRun(5, func() {
		r.Reset(12)
		r.Record(6, 9)
		r.Seal()
	})
	if allocs != 0 || r.Bytes() != before {
		t.Fatalf("reset allocations=%g bytes=%d want=%d", allocs, r.Bytes(), before)
	}
	if _, known := r.Lookahead(2); known {
		t.Fatal("reset kept an earlier parse's read")
	}
	if count, known := r.Lookahead(8); !known || count != 1 {
		t.Fatal("new read not retained")
	}
	r.Reset(-1)
	if r.Recording() {
		t.Fatal("released arena still records")
	}
	r.TrimCapacity(1)
	if r.Bytes() != 64 {
		t.Fatal("oversized scan buffer retained")
	}
}

func TestLookaheadIncompleteBorrowedHistoryAbstains(t *testing.T) {
	r := NewReads(12)
	r.Record(0, 4)
	r.Abstain()
	r.Record(6, 12)
	r.Seal()
	if _, known := r.Lookahead(12); known {
		t.Fatal("incomplete history authenticated a new ancestor")
	}
}

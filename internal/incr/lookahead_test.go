package incr

import (
	"math/rand"
	"testing"
)

func TestLookaheadCompactionMatchesAllProbes(t *testing.T) {
	const sourceBytes = 64
	random := rand.New(rand.NewSource(71))
	for trial := 0; trial < 100; trial++ {
		reads := NewReads(sourceBytes)
		probes := make([]read, 0, 256)
		for i := 0; i < 256; i++ {
			start := uint32(random.Intn(sourceBytes + 1))
			end := start + uint32(random.Intn(sourceBytes+5-int(start)))
			probes = append(probes, read{start, end})
			reads.Record(int(start), end)
		}
		reads.Seal()
		for _, includeBoundary := range []bool{false, true} {
			cursor := reads.Cursor(includeBoundary)
			for end := uint32(1); end <= sourceBytes; end++ {
				var frontier uint32
				found := false
				for _, probe := range probes {
					if probe.start < end || (includeBoundary && probe.start == end) {
						frontier, found = max(frontier, probe.end), true
					}
				}
				wantKnown := found && frontier >= end
				var want uint32
				if wantKnown {
					want = frontier - end
				}
				got, known := reads.Lookahead(end)
				if !includeBoundary {
					got, known = reads.LeafLookahead(end)
				}
				if got != want || known != wantKnown {
					t.Fatalf("trial=%d boundary=%t end=%d: got %d/%t, want %d/%t", trial, includeBoundary, end, got, known, want, wantKnown)
				}
				if got, known := cursor.Lookahead(end); got != want || known != wantKnown {
					t.Fatalf("cursor trial=%d boundary=%t end=%d: got %d/%t, want %d/%t", trial, includeBoundary, end, got, known, want, wantKnown)
				}
			}
		}
	}
}

func TestLookaheadContainedProbesDoNotSpendBudget(t *testing.T) {
	reads := NewReads(256)
	reads.Record(0, 257)
	allocated := reads.Bytes()
	reads.BindBudget(allocated, 0, &allocated)
	for start := 1; start <= 256; start++ {
		reads.Record(start, uint32(start+1))
	}
	reads.Seal()
	if got, known := reads.LeafLookahead(256); !known || got != 1 {
		t.Fatalf("covered probes lost the original EOF dependency: %d/%t", got, known)
	}
	if reads.Bytes() != 64+128*8 || allocated != reads.Bytes() {
		t.Fatal("covered probes grew storage or changed its budget charge")
	}
}

func TestLookaheadNearbyRestoresReuseChargedStorage(t *testing.T) {
	reads := NewReads(256)
	for start := 0; start < 128; start++ {
		reads.Record(start, uint32(start+1))
	}
	allocated := reads.Bytes()
	reads.BindBudget(allocated, 0, &allocated)
	reads.Record(120, 130) // eighth-most-recent origin, with a longer failed probe
	reads.Seal()
	if got, known := reads.Lookahead(120); !known || got != 10 {
		t.Fatalf("restored origin lost its longer probe: %d/%t", got, known)
	}
	if got, known := reads.LeafLookahead(120); !known || got != 0 {
		t.Fatalf("leaf included its boundary probe: %d/%t", got, known)
	}
	if reads.Bytes() != 64+128*8 || allocated != reads.Bytes() {
		t.Fatal("nearby replay grew storage or changed its budget charge")
	}
}

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

func TestLookaheadCursorMatchesIndependentBounds(t *testing.T) {
	r := NewReads(64)
	for _, probe := range []struct {
		start int
		end   uint32
	}{{8, 11}, {0, 2}, {24, 70}, {2, 19}, {8, 20}, {40, 55}} {
		r.Record(probe.start, probe.end)
	}
	r.Seal()
	for _, include := range []bool{false, true} {
		c := r.Cursor(include)
		check := func(end uint32) {
			got, known := c.Lookahead(end)
			want, wantKnown := r.lookahead(end, include)
			if got != want || known != wantKnown {
				t.Fatalf("boundary=%t end=%d got=(%d,%t) want=(%d,%t)", include, end, got, known, want, wantKnown)
			}
		}
		for end := uint32(0); end <= 65; end++ {
			check(end)
			check(end)
		}
		for end := uint32(65); end > 0; end-- {
			check(end)
		}
		for _, end := range []uint32{2, 40, 8, 8, 24, 64, 0, 2, 65, 64, 8} {
			check(end)
		}
	}
	c := r.Cursor(true)
	c.Lookahead(8)
	r.Abstain()
	if _, known := c.Lookahead(8); known {
		t.Fatal("cursor reused an invalidated history")
	}
	r.Reset(64)
	c = r.Cursor(true)
	if _, known := c.Lookahead(8); known {
		t.Fatal("cursor used an unsealed history")
	}
}

package incrcensus

import "testing"

func TestLostNodePartitionSubtractsAcceptedDescendants(t *testing.T) {
	r := New()
	r.Start()
	p := r.Enter("selection")
	r.Decision(1, "dirty", "reject")
	r.Decision(2, "fragile", "reject")
	r.Leave(p)
	r.Stop()
	got := r.Finish([]Node{{ID: 1}, {ID: 2, Parent: 1}, {ID: 3, Parent: 2}, {ID: 4, Parent: 1}}, map[uint64]bool{3: true})
	if got.OldNodes != 4 || got.ReusedNodes != 1 || got.LostNodes != 3 {
		t.Fatalf("invalid partition: %+v", got)
	}
	rows := map[string]Row{}
	var nanos int64
	var lost uint64
	for _, row := range got.Rows {
		rows[row.Reason] = row
		nanos += row.Nanos
		lost += row.LostNodes
	}
	if rows["dirty"].LostNodes != 2 || rows["fragile"].LostNodes != 1 || lost != got.LostNodes {
		t.Fatalf("incorrect ancestor accounting: %+v", rows)
	}
	if nanos+got.ObserveNanos != got.EditNanos {
		t.Fatalf("exclusive phases do not sum: rows=%d overhead=%d total=%d", nanos, got.ObserveNanos, got.EditNanos)
	}
}
func TestFreshFallbackOwnsLostNodesOnlyWhenNothingSurvives(t *testing.T) {
	for _, retain := range []bool{false, true} {
		r := New()
		r.Start()
		r.Decision(1, "dirty", "reject")
		r.Fallback("frontier_unproven")
		r.Stop()
		got := r.Finish([]Node{{ID: 1}, {ID: 2, Parent: 1}}, map[uint64]bool{2: retain})
		want := "frontier_unproven"
		if retain {
			want = "dirty"
		}
		var found bool
		for _, row := range got.Rows {
			if row.Reason == want && row.LostNodes == got.LostNodes {
				found = true
			}
		}
		if !found {
			t.Fatalf("wrong attribution: %+v", got)
		}
	}
}
func TestNestedPhaseIsExclusive(t *testing.T) {
	r := New()
	r.Start()
	a := r.Enter("outer")
	b := r.Enter("inner")
	r.Decision(1, "guard", "reject")
	r.Leave(b)
	r.Leave(a)
	r.Stop()
	got := r.Finish(nil, nil)
	var total int64
	for _, row := range got.Rows {
		total += row.Nanos
	}
	if total+got.ObserveNanos != got.EditNanos {
		t.Fatalf("double charged time: %+v", got)
	}
}

func TestAlternativeSkipDoesNotOverwriteRefusal(t *testing.T) {
	r := New()
	r.Start()
	r.Decision(1, "shift_state_mismatch", "reject")
	r.Decision(1, "interior_lane_requires_parent", "skip")
	r.Stop()
	got := r.Finish([]Node{{ID: 1}}, nil)
	for _, row := range got.Rows {
		if row.Reason == "shift_state_mismatch" && row.LostNodes == 1 {
			return
		}
	}
	t.Fatalf("alternative lane skip overwrote the actual refusal: %+v", got)
}

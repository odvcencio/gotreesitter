package dependency

import (
	"sync"
	"testing"
)

func TestDenseSlotsBudgetAndReset(t *testing.T) {
	var s Store
	r := Record{Start: 2, End: 4, Lookahead: 0, Column: 2}
	if _, ok := s.Set(0, r, 1); ok || s.Bytes() != 0 || s.Len() != 0 {
		t.Fatal("rejected growth published or charged storage")
	}
	for _, slot := range []int{0, pageSlots - 1, 3*pageSlots + 7, pageSlots + 2} {
		before := s.Bytes()
		cost, ok := s.Set(slot, r, -1)
		if !ok || s.Bytes() != before+cost {
			t.Fatal("growth charge differs from retained capacity")
		}
		if got, ok := s.Get(slot); !ok || got != r {
			t.Fatalf("slot %d: %v/%t", slot, got, ok)
		}
	}
	if _, ok := s.Get(2 * pageSlots); ok {
		t.Fatal("sparse gap supplied a receipt")
	}
	if _, ok := s.Get(-1); ok {
		t.Fatal("negative slot supplied a receipt")
	}
	if _, ok := s.Set(-1, r, -1); ok {
		t.Fatal("negative slot accepted a receipt")
	}
	if s.Len() != 4 {
		t.Fatalf("count=%d", s.Len())
	}
	s.Range(func(slot int, _ Record) { s.Clear(slot) })
	if s.Len() != 0 {
		t.Fatal("range invalidation retained receipts")
	}
	before := s.Bytes()
	if cost, ok := s.Set(0, r, 0); !ok || cost != 0 {
		t.Fatal("reauthentication allocated storage")
	}
	s.Reset(before)
	if s.Bytes() != before || s.Len() != 0 {
		t.Fatal("bounded reset lost storage accounting")
	}
	if _, ok := s.Get(0); ok {
		t.Fatal("reset retained a stale receipt")
	}
	if cost, ok := s.Set(0, r, 0); !ok || cost != 0 {
		t.Fatal("warm slot needed growth")
	}
	s.Reset(before - 1)
	if s.Bytes() != 0 || s.Len() != 0 {
		t.Fatal("oversized reset retained storage")
	}
	if _, ok := s.Get(0); ok {
		t.Fatal("evicted store retained a receipt")
	}
}

func TestDenseConcurrentReadsDuringGrowthAndInvalidation(t *testing.T) {
	var s Store
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for n := 0; n < 20000; n++ {
				if r, ok := s.Get(0); ok && (r.Start != r.End || r.Start != r.Lookahead || r.Start != r.Column) {
					t.Errorf("mixed publication: %+v", r)
					return
				}
			}
		}()
	}
	close(start)
	for n := uint32(0); n < 20000; n++ {
		r := Record{n, n, n, n}
		if _, ok := s.Set(0, r, -1); !ok {
			t.Fatal("publication failed")
		}
		if n%200 == 0 {
			s.Set(int(n)+pageSlots, r, -1)
		}
		s.Clear(0)
	}
	wg.Wait()
}

func TestDensePublicationAuthenticatesEveryProjection(t *testing.T) {
	for _, observations := range [][]bool{{true}, {true, true}, {true, false}, {false, true}, nil} {
		var s Store
		r := Record{Start: 2, End: 4, Lookahead: 7, Column: 2}
		s.Set(0, r, -1)
		cost, ok := s.Stage(0, Record{Start: 2, End: 4, Column: 2}, 0)
		if !ok || cost != 0 || s.Len() != 0 {
			t.Fatal("draft allocated or stayed published")
		}
		if _, ok := s.Get(0); ok {
			t.Fatal("draft authenticated a receipt")
		}
		want := len(observations) != 0
		for _, valid := range observations {
			want = want && valid
			s.Observe(0, 6, valid)
		}
		s.Finish(0)
		got, present := s.Get(0)
		if present != want || (present && got != r) {
			t.Fatalf("observations=%v receipt=%+v/%t", observations, got, present)
		}
	}
	var s Store
	s.Set(0, Record{Start: 1, End: 3, Lookahead: 100, Column: 1}, -1)
	s.Stage(0, Record{Start: 2, End: 4, Column: 2}, 0)
	s.Observe(0, 4, true)
	s.Finish(0)
	if r, ok := s.Get(0); !ok || r.Lookahead != 0 {
		t.Fatal("new geometry inherited a stale maximum")
	}
	s.Stage(0, Record{Start: 2, End: 4, Column: 2}, 0)
	s.Observe(0, 3, true)
	s.Finish(0)
	if _, ok := s.Get(0); ok {
		t.Fatal("frontier before node end authenticated a receipt")
	}
	s.Stage(0, Record{End: 1}, 0)
	s.RangeStaged(s.Finish)
	if s.IsStaged(0) || s.Len() != 0 {
		t.Fatal("unmatched draft survived finalization")
	}
}

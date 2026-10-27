package incr

import "testing"

// A default-producer grant must survive valid grouped-storage transitions but
// cannot survive an incomplete history or arena reuse.
func TestGroupedReadProducerAuthorityLifecycle(t *testing.T) {
	const n = 100000
	const key = 8195
	t.Run("paged-wide-disordered", func(t *testing.T) {
		r := NewReads(n)
		r.CertifyDefaultProducer(key)
		for i := 0; i < 18000; i++ {
			r.Record(i, uint32(i+1))
		}
		// Update an older origin out of order, with a span wide enough to promote.
		r.Record(1, 70000)
		r.Seal()
		if got := r.DefaultProducerSchedule(n); got != key {
			t.Fatalf("complete grouped history key=%d", got)
		}
		if got, ok := r.Lookahead(10); !ok || got != 69990 {
			t.Fatalf("promoted lookahead distance=%d, valid=%t", got, ok)
		}
		if got := r.DefaultProducerSchedule(n - 1); got != 0 {
			t.Fatalf("different source length retained key %d", got)
		}
		r.Reset(n)
		if got := r.DefaultProducerSchedule(n); got != 0 {
			t.Fatalf("arena reset retained key %d", got)
		}
	})
	for _, kind := range []string{"abstain", "dropped", "invalid-record", "budget"} {
		t.Run(kind, func(t *testing.T) {
			r := NewReads(n)
			r.CertifyDefaultProducer(key)
			switch kind {
			case "abstain":
				r.Abstain()
			case "dropped":
				r.CommitDroppedInput(true)
			case "invalid-record":
				r.Record(-1, 0)
			case "budget":
				var allocated int64
				r.BindBudget(1, 0, &allocated)
				r.Record(0, 1)
			}
			r.Seal()
			if got := r.DefaultProducerSchedule(n); got != 0 {
				t.Fatalf("%s incomplete history retained key %d", kind, got)
			}
		})
	}
}

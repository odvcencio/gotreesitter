package ccore

import "testing"

func TestLedgerInternalHelpersClassifyRefusal(t *testing.T) {
	helpers := []struct {
		name string
		run  func(*Ledger)
	}{
		{"push", func(l *Ledger) { _ = push(l, []byte(nil), byte(1)) }},
		{"grow", func(l *Ledger) { _ = grow(l, []byte(nil), 1) }},
		{"mk", func(l *Ledger) { _ = mk[byte](l, 1) }},
		{"mkCap", func(l *Ledger) { _ = mkCap[byte](l, 1) }},
	}
	for _, h := range helpers {
		t.Run(h.name, func(t *testing.T) {
			t.Run("poll", func(t *testing.T) {
				var l Ledger
				calls := 0
				l.SetPoll(func(n int64, _ bool) bool { calls++; return n <= 1 })
				if !l.Begin(0) {
					t.Fatal("begin")
				}
				l.Force(1)
				func() {
					defer func() {
						v, ok := recover().(ledgerStop)
						if !ok || v.reason != reservationPollRefused {
							t.Fatalf("poll reason: %#v", v)
						}
					}()
					h.run(&l)
					t.Fatal("did not refuse")
				}()
				if l.Total() != 1 || calls != 2 {
					t.Fatalf("reservation changed state total=%d calls=%d", l.Total(), calls)
				}
			})
			t.Run("invalid_without_poll", func(t *testing.T) {
				var l Ledger
				if !l.Begin(0) {
					t.Fatal("begin")
				}
				l.Force(-1)
				defer func() {
					v, ok := recover().(ledgerStop)
					if !ok || v.reason != reservationInvalid {
						t.Fatalf("invalid reason: %#v", v)
					}
				}()
				h.run(&l)
				t.Fatal("did not refuse")
			})
		})
	}
}

func TestLedgerInternalHelpersRejectNegativeSizes(t *testing.T) {
	helpers := []struct {
		name string
		run  func(*Ledger)
	}{
		{"grow", func(l *Ledger) { _ = grow(l, []byte(nil), -1) }},
		{"mk", func(l *Ledger) { _ = mk[byte](l, -1) }},
		{"mkCap", func(l *Ledger) { _ = mkCap[byte](l, -1) }},
	}
	for _, h := range helpers {
		t.Run(h.name, func(t *testing.T) {
			var l Ledger
			l.Begin(0)
			defer func() {
				v, ok := recover().(ledgerStop)
				if !ok || v.reason != reservationInvalid {
					t.Fatalf("negative size reason: %#v", v)
				}
			}()
			h.run(&l)
			t.Fatal("did not refuse")
		})
	}
}

func TestLedgerHelperExistingCapacityDoesNotPoll(t *testing.T) {
	var l Ledger
	calls := 0
	l.SetPoll(func(n int64, _ bool) bool { calls++; return n == 0 })
	if !l.Begin(0) {
		t.Fatal("begin")
	}
	l.Force(1)
	original := make([]byte, 0, 1)
	out := push(&l, original, byte(7))
	if len(out) != 1 || out[0] != 7 || calls != 1 || l.Total() != 1 {
		t.Fatalf("unexpected no-growth charge calls=%d total=%d", calls, l.Total())
	}
	defer func() {
		v, ok := recover().(ledgerStop)
		if !ok || v.reason != reservationPollRefused || calls != 2 {
			t.Fatalf("next growth reason=%#v calls=%d", v, calls)
		}
	}()
	_ = push(&l, out, byte(8))
}

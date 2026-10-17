package parsercorephase0

import "testing"

func TestNegativeLexProbeMemoDomain(t *testing.T) {
	var m NegativeLexProbeMemo
	if _, _, ok := m.Lookup(0, 0); ok {
		t.Fatal("zero value is not a result")
	}
	for _, state := range []StateID{0, 1, 7, 8, 65535, 65536, 65537, 0xffffffff} {
		m.Record(state, 123, 456, 789)
		span, produced, ok := m.Lookup(state, 123)
		if !ok || span != 456 || produced != 789 {
			t.Fatal(state, span, produced, ok)
		}
		if _, _, ok := m.Lookup(state, 124); ok {
			t.Fatal("wrong start matched")
		}
		if _, _, ok := m.Lookup(state^65536, 123); ok {
			t.Fatal("upper state bits aliased", state)
		}
		other := state ^ 16
		if _, _, ok := m.Lookup(other, 123); ok {
			t.Fatal("slot collision matched")
		}
		m.Record(other, 123, 1, 2)
		if _, _, ok := m.Lookup(state, 123); ok {
			t.Fatal("replaced slot matched")
		}
		m.Clear()
		if _, _, ok := m.Lookup(other, 123); ok {
			t.Fatal("cleared entry matched")
		}
	}
}
func TestCaptureLexProbeReadSpanExactAndPanic(t *testing.T) {
	span := uint32(400)
	value, observed := CaptureLexProbeReadSpan(&span, func() int {
		if span != 0 {
			t.Fatal("prior dependency leaked")
		}
		span = 37
		return 9
	})
	if value != 9 || observed != 37 || span != 400 {
		t.Fatal(value, observed, span)
	}
	func() {
		defer func() {
			if recover() != "probe" {
				t.Fatal("panic lost")
			}
		}()
		CaptureLexProbeReadSpan(&span, func() int { span = 500; panic("probe") })
	}()
	if span != 500 {
		t.Fatal("panic lost dependency", span)
	}
}

// Doubling the direct-index capacity refines each old bucket. Every hit the
// old eight-slot policy retains must remain a hit, even with full-width states,
// different start bytes, skipped recordings and resets.
func TestNegativeLexProbeMemoRetainsEightSlotHits(t *testing.T) {
	type referenceEntry struct {
		state                 StateID
		start, span, produced uint32
		valid                 bool
	}
	var old [8]referenceEntry
	var memo NegativeLexProbeMemo
	var random uint32 = 0x39f1a57b
	oldHits, newHits := 0, 0
	for step := 0; step < 100000; step++ {
		random ^= random << 13
		random ^= random >> 17
		random ^= random << 5
		if step%251 == 0 {
			old = [8]referenceEntry{}
			memo.Clear()
		}
		state := StateID(random & 255)
		if step%5 == 0 {
			state |= 0xffff0000
		}
		if step%7 == 0 {
			state = StateID(step & 31)
		}
		start := uint32((step / 29) % 4)
		entry := old[uint32(state)%8]
		wasHit := entry.valid && entry.state == state && entry.start == start
		span, produced, hit := memo.Lookup(state, start)
		expectedSpan := uint32(state) ^ start
		expectedProduced := uint32(state) & 7
		if wasHit {
			oldHits++
			if !hit {
				t.Fatal("larger cache lost old hit", step, state, start)
			}
		}
		if hit {
			newHits++
			if span != expectedSpan || produced != expectedProduced {
				t.Fatal("metadata mismatch")
			}
		}
		// Non-cached outcomes do not evict either policy; a missing result may
		// become cacheable later, but stored metadata depends only on its key.
		if random&3 == 0 && !wasHit && !hit {
			continue
		}
		if !wasHit {
			old[uint32(state)%8] = referenceEntry{state, start, expectedSpan, expectedProduced, true}
		}
		if !hit {
			memo.Record(state, start, expectedSpan, expectedProduced)
		}
	}
	if oldHits == 0 || newHits < oldHits {
		t.Fatal(oldHits, newHits)
	}
}

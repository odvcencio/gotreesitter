package grammargen

import "testing"

func TestBitsetAddContains(t *testing.T) {
	b := newBitset(128)
	b.add(0)
	b.add(63)
	b.add(64)
	b.add(127)

	for _, idx := range []int{0, 63, 64, 127} {
		if !b.contains(idx) {
			t.Errorf("expected contains(%d) = true", idx)
		}
	}
	for _, idx := range []int{1, 62, 65, 126} {
		if b.contains(idx) {
			t.Errorf("expected contains(%d) = false", idx)
		}
	}
}

func TestBitsetGrow(t *testing.T) {
	b := newBitset(8)
	b.add(200) // forces growth
	if !b.contains(200) {
		t.Fatal("expected contains(200) after grow")
	}
	if b.contains(199) {
		t.Fatal("unexpected contains(199)")
	}
}

func TestBitsetEmpty(t *testing.T) {
	b := newBitset(64)
	if !b.empty() {
		t.Fatal("new bitset should be empty")
	}
	b.add(5)
	if b.empty() {
		t.Fatal("bitset with element should not be empty")
	}
}

func TestBitsetUnionWith(t *testing.T) {
	a := newBitset(128)
	a.add(1)
	a.add(65)

	b := newBitset(128)
	b.add(2)
	b.add(65)

	changed := a.unionWith(&b)
	if !changed {
		t.Fatal("union should report changed")
	}
	for _, idx := range []int{1, 2, 65} {
		if !a.contains(idx) {
			t.Errorf("after union, expected contains(%d)", idx)
		}
	}

	// Union again with same data — should not change.
	changed = a.unionWith(&b)
	if changed {
		t.Fatal("second union should not change")
	}
}

func TestBitsetUnionWithGrow(t *testing.T) {
	a := newBitset(64)
	b := newBitset(256)
	b.add(200)

	changed := a.unionWith(&b)
	if !changed {
		t.Fatal("union with larger set should change")
	}
	if !a.contains(200) {
		t.Fatal("expected contains(200) after union grow")
	}
}

func TestBitsetEqual(t *testing.T) {
	a := newBitset(128)
	b := newBitset(128)
	a.add(10)
	a.add(100)
	b.add(10)
	b.add(100)

	if !a.equal(&b) {
		t.Fatal("identical bitsets should be equal")
	}

	b.add(50)
	if a.equal(&b) {
		t.Fatal("different bitsets should not be equal")
	}
}

func TestBitsetEqualDifferentLengths(t *testing.T) {
	a := newBitset(64)
	b := newBitset(256)
	a.add(10)
	b.add(10)

	if !a.equal(&b) {
		t.Fatal("bitsets with same content but different backing length should be equal")
	}

	b.add(200)
	if a.equal(&b) {
		t.Fatal("should not be equal after adding element beyond a's range")
	}
}

func TestBitsetClone(t *testing.T) {
	a := newBitset(128)
	a.add(5)
	a.add(70)

	c := a.clone()
	if !c.equal(&a) {
		t.Fatal("clone should be equal")
	}

	// Mutating clone should not affect original.
	c.add(100)
	if c.equal(&a) {
		t.Fatal("mutated clone should differ")
	}
	if a.contains(100) {
		t.Fatal("original should not be affected by clone mutation")
	}
}

func TestBitsetCount(t *testing.T) {
	b := newBitset(256)
	if b.count() != 0 {
		t.Fatal("empty bitset count should be 0")
	}
	b.add(0)
	b.add(63)
	b.add(64)
	b.add(255)
	if got := b.count(); got != 4 {
		t.Fatalf("expected count 4, got %d", got)
	}
}

func TestBitsetForEach(t *testing.T) {
	b := newBitset(256)
	want := []int{0, 1, 63, 64, 127, 200}
	for _, idx := range want {
		b.add(idx)
	}

	var got []int
	b.forEach(func(idx int) {
		got = append(got, idx)
	})

	if len(got) != len(want) {
		t.Fatalf("forEach returned %d elements, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("forEach[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestBitsetHash(t *testing.T) {
	a := newBitset(128)
	b := newBitset(128)
	a.add(10)
	a.add(64)
	b.add(10)
	b.add(64)

	if a.hash() != b.hash() {
		t.Fatal("identical bitsets should have same hash")
	}

	b.add(65)
	if a.hash() == b.hash() {
		t.Log("warning: hash collision on different bitsets (unlikely but possible)")
	}
}

package gotreesitter

import (
	"testing"
)

// TestInternTableInsertLookup exercises the basic store/lookup cycle on
// a freshly constructed table. Confirms the open-addressed probe path
// returns nil on a miss and the canonical node on a hit, with counter
// movement matching the operation.
func TestInternTableInsertLookup(t *testing.T) {
	tbl := newInternTable()

	a := &Node{symbol: 7, startByte: 10, endByte: 20}
	b := &Node{symbol: 7, startByte: 30, endByte: 40}

	keyA := buildKey(a.symbol, a.productionID, a.flags, a.startByte, a.endByte, a.children)
	keyB := buildKey(b.symbol, b.productionID, b.flags, b.startByte, b.endByte, b.children)

	if got := tbl.lookup(keyA, a.children); got != nil {
		t.Fatalf("lookup before store: got %v, want nil", got)
	}
	tbl.store(keyA, a)
	if got := tbl.lookup(keyA, a.children); got != a {
		t.Fatalf("lookup after store: got %v, want %v", got, a)
	}
	if got := tbl.lookup(keyB, b.children); got != nil {
		t.Fatalf("lookup for different key: got %v, want nil", got)
	}
	tbl.store(keyB, b)
	if got := tbl.lookup(keyB, b.children); got != b {
		t.Fatalf("lookup for second key: got %v, want %v", got, b)
	}

	s := tbl.stats()
	if s.Lookups != 4 {
		t.Errorf("Lookups = %d, want 4", s.Lookups)
	}
	if s.Hits != 2 {
		t.Errorf("Hits = %d, want 2", s.Hits)
	}
	if s.Stores != 2 {
		t.Errorf("Stores = %d, want 2", s.Stores)
	}
	if s.Occupied != 2 {
		t.Errorf("Occupied = %d, want 2", s.Occupied)
	}
}

// TestInternTableHashCollisionResolution: two keys with identical hash
// bucket should each return their own node. Forces the probe sequence
// by storing many entries with the same low bits.
func TestInternTableHashCollisionResolution(t *testing.T) {
	tbl := newInternTable()

	nodes := make([]*Node, 16)
	for i := range nodes {
		nodes[i] = &Node{symbol: Symbol(i), startByte: uint32(i * 10), endByte: uint32(i*10 + 5)}
		key := buildKey(nodes[i].symbol, 0, 0, nodes[i].startByte, nodes[i].endByte, nil)
		tbl.store(key, nodes[i])
	}

	for i, n := range nodes {
		key := buildKey(n.symbol, 0, 0, n.startByte, n.endByte, nil)
		got := tbl.lookup(key, nil)
		if got != n {
			t.Errorf("nodes[%d] lookup got %v, want %v", i, got, n)
		}
	}
}

// TestInternTableChildrenIdentity verifies that two parents with the
// same symbol/span/flags but DIFFERENT children pointer arrays do not
// collapse. This is the core invariant for parent-node interning in
// Phase 3.
func TestInternTableChildrenIdentity(t *testing.T) {
	tbl := newInternTable()

	leaf1 := &Node{symbol: 1, startByte: 0, endByte: 1}
	leaf2 := &Node{symbol: 1, startByte: 1, endByte: 2}

	parent1 := &Node{symbol: 5, startByte: 0, endByte: 2, children: []*Node{leaf1, leaf2}}
	parent2 := &Node{symbol: 5, startByte: 0, endByte: 2, children: []*Node{leaf2, leaf1}}

	key1 := buildKey(parent1.symbol, 0, 0, parent1.startByte, parent1.endByte, parent1.children)
	key2 := buildKey(parent2.symbol, 0, 0, parent2.startByte, parent2.endByte, parent2.children)

	tbl.store(key1, parent1)
	tbl.store(key2, parent2)

	if got := tbl.lookup(key1, parent1.children); got != parent1 {
		t.Errorf("parent1 lookup got %v, want %v", got, parent1)
	}
	if got := tbl.lookup(key2, parent2.children); got != parent2 {
		t.Errorf("parent2 lookup got %v, want %v", got, parent2)
	}
}

// TestInternTableGrowth confirms the table doubles capacity when load
// factor is exceeded and that previously-stored entries remain reachable
// after the rehash.
func TestInternTableGrowth(t *testing.T) {
	tbl := newInternTable()
	initialCap := len(tbl.entries)
	loadTarget := int(float64(initialCap)*internTableMaxLoadFactor) + 5

	stored := make([]*Node, loadTarget)
	for i := 0; i < loadTarget; i++ {
		stored[i] = &Node{symbol: Symbol(i % 100), startByte: uint32(i), endByte: uint32(i + 1)}
		key := buildKey(stored[i].symbol, 0, 0, stored[i].startByte, stored[i].endByte, nil)
		tbl.store(key, stored[i])
	}

	if tbl.growths == 0 {
		t.Errorf("growths = 0, expected at least one growth past load factor")
	}
	if len(tbl.entries) <= initialCap {
		t.Errorf("entries capacity after growth = %d, want > %d", len(tbl.entries), initialCap)
	}

	for i, n := range stored {
		key := buildKey(n.symbol, 0, 0, n.startByte, n.endByte, nil)
		got := tbl.lookup(key, nil)
		if got != n {
			t.Errorf("post-growth lookup[%d] got %v, want %v", i, got, n)
		}
	}
}

// TestInternTableReset clears the table between parses while preserving
// counters; future phases can read counters to evaluate hit-rate
// regressions across runs.
func TestInternTableReset(t *testing.T) {
	tbl := newInternTable()
	a := &Node{symbol: 1, startByte: 0, endByte: 1}
	key := buildKey(a.symbol, 0, 0, a.startByte, a.endByte, nil)
	tbl.store(key, a)

	if tbl.occupied != 1 {
		t.Fatalf("occupied = %d, want 1", tbl.occupied)
	}

	tbl.reset()

	if tbl.occupied != 0 {
		t.Errorf("occupied after reset = %d, want 0", tbl.occupied)
	}
	if got := tbl.lookup(key, nil); got != nil {
		t.Errorf("lookup after reset got %v, want nil", got)
	}
	// Counters persist (Lookups counter incremented by the lookup above).
	if tbl.stores != 1 {
		t.Errorf("stores after reset = %d, want 1 (counters should persist)", tbl.stores)
	}
}

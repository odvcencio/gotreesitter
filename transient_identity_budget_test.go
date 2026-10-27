package gotreesitter

import (
	"testing"
	"unsafe"
)

func TestTransientIdentitySetBudgetAndRetention(t *testing.T) {
	t.Run("denial-before-allocation", func(t *testing.T) {
		s := &parserScratch{}
		s.setBudget(1)
		p := &Parser{budgetScratch: s}
		before := s.allocatedBytes()
		if s.transientParents.addSeenNode(&Node{}, p) {
			t.Fatal("budget should deny")
		}
		if s.allocatedBytes() != before || s.transientParents.seen.Capacity() != 0 {
			t.Fatal("budget denial allocated")
		}
	})
	t.Run("peak-overlap-budget", func(t *testing.T) {
		s := &parserScratch{}
		nodes := make([]Node, 65)
		for i := 0; i < 64; i++ {
			if !s.transientParents.addSeenNode(&nodes[i], nil) {
				t.Fatal("initial fill failed")
			}
		}
		if s.transientParents.seen.Capacity() != 128 {
			t.Fatal("unexpected initial capacity")
		}
		baseline := s.allocatedBytes()
		peak := int64(256 * unsafe.Sizeof(uintptr(0)))
		s.setBudget(peak - 1)
		p := &Parser{budgetScratch: s}
		if s.transientParents.addSeenNode(&nodes[64], p) || s.allocatedBytes() != baseline || s.transientParents.seen.Len() != 64 {
			t.Fatal("peak allocation denial mutated state")
		}
		s.setBudget(peak)
		if !s.transientParents.addSeenNode(&nodes[64], p) {
			t.Fatal("exact peak budget declined")
		}
		if s.transientParents.seen.Len() != 65 || s.allocatedBytes() != baseline+peak/2 {
			t.Fatal("retained accounting mismatch")
		}
		s.transientParents.clearMaterializeScratch()
	})
	t.Run("account-clear-and-shrink", func(t *testing.T) {
		s := &parserScratch{}
		s.setBudget(8 << 20)
		p := &Parser{budgetScratch: s}
		nodes := make([]Node, 40000)
		for i := range nodes {
			if !s.transientParents.addSeenNode(&nodes[i], p) {
				t.Fatalf("insert %d", i)
			}
		}
		if s.transientParents.seen.Len() != len(nodes) || s.allocatedBytes() < s.transientParents.seen.Bytes() {
			t.Fatal("missing membership or accounting")
		}
		s.transientParents.clearMaterializeScratch()
		if s.transientParents.seen.Len() != 0 {
			t.Fatal("membership survived cleanup")
		}
		s.transientParents.trimForSource(1)
		if s.transientParents.seen.Capacity() != 0 {
			t.Fatal("large retained set survived source shrink")
		}
	})
	t.Run("one-mib-source-retention-bound", func(t *testing.T) {
		s := &parserScratch{}
		s.setBudget(32 << 20)
		p := &Parser{budgetScratch: s}
		nodes := make([]Node, 200000)
		for i := range nodes {
			if !s.transientParents.addSeenNode(&nodes[i], p) {
				t.Fatalf("insert %d", i)
			}
		}
		if got := s.transientParents.seen.Capacity(); got != maxRetainedFullSliceCap {
			t.Fatalf("capacity %d", got)
		}
		retained := s.transientParents.seen.Bytes()
		s.transientParents.clearMaterializeScratch()
		s.transientParents.trimForSource(1048591)
		if s.transientParents.seen.Bytes() != retained || s.allocatedBytes() < retained {
			t.Fatal("1 MiB retention or accounting changed")
		}
		s.transientParents.resetForRelease()
		if s.transientParents.seen.Bytes() != retained {
			t.Fatal("bounded full-source capacity was discarded")
		}
		s.transientParents.trimForSource(1)
		if s.transientParents.seen.Bytes() != 0 {
			t.Fatal("small source retained large set")
		}
	})
	t.Run("nil-parser", func(t *testing.T) {
		var s transientParentScratch
		if !s.addSeenNode(&Node{}, nil) {
			t.Fatal("nil parser unexpectedly denied")
		}
		s.clearMaterializeScratch()
	})
}

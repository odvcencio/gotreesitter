package parsercorephase0

import "testing"

func TestCanonicalHeadMask(t *testing.T) {
	var mask CanonicalHeadMask
	if mask.Add(Head{}) || mask != 0 {
		t.Fatal("zero head changed the proof")
	}
	for id := NodeID(1); id <= 64; id++ {
		if !mask.Add(Head{Node: id}) {
			t.Fatalf("distinct bucket %d rejected", id)
		}
		before := mask
		if mask.Add(Head{Node: id}) || mask != before {
			t.Fatalf("duplicate %d accepted or changed mask", id)
		}
	}
	if mask.Add(Head{Node: 65}) {
		t.Fatal("collision accepted")
	}
}

func TestCanonicalHeadMaskCollisionLeavesProofUnchanged(t *testing.T) {
	var mask CanonicalHeadMask
	if !mask.Add(Head{Node: 7}) {
		t.Fatal("first head rejected")
	}
	before := mask
	if mask.Add(Head{Node: 71}) || mask != before {
		t.Fatal("collision accepted or changed mask")
	}
	if !mask.Add(Head{Node: 8}) {
		t.Fatal("collision changed another bucket")
	}
	if !mask.Add(Head{Node: NodeID(^uint32(0))}) {
		t.Fatal("maximum head rejected")
	}
	if mask.Add(Head{Node: 63}) {
		t.Fatal("maximum-ID bucket collision accepted")
	}
}

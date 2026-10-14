package parsercorephase0

import "testing"

func TestFrozenValuePreservesPublishedMetadata(t *testing.T) {
	type contract struct {
		digest   [32]byte
		required bool
	}
	first := contract{digest: [32]byte{1}, required: true}
	a := ReuseFrozenValue[contract](nil, first)
	if b := ReuseFrozenValue(a, first); b != a {
		t.Fatal("equal metadata allocated a new holder")
	}
	changed := first
	changed.digest[0] = 2
	b := ReuseFrozenValue(a, changed)
	if b == a || a.Value() != first || b.Value() != changed {
		t.Fatal("replacement mutated published metadata")
	}
	copy := a.Value()
	copy.digest[0] = 3
	if a.Value() != first {
		t.Fatal("read exposed mutable retained metadata")
	}
	var absent *FrozenValue[contract]
	if absent.Value() != (contract{}) {
		t.Fatal("absent holder did not retain zero-value semantics")
	}
}

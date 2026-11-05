package parsercorephase0

import (
	"testing"
	"unsafe"
)

func TestSelectedUnaryProgramOwnershipAndRules(t *testing.T) {
	rows := []SelectedUnaryRow{{Self: SelectedUnaryPass}, {Default: SelectedUnaryPass, Self: SelectedUnaryPass}, {Self: SelectedUnaryPass}, {}}
	pairs := []SelectedUnaryOverride{{Parent: 2, Child: 0, Rule: SelectedUnaryRenameLeaf}}
	p, err := NewSelectedUnaryProgram(rows, pairs)
	if err != nil {
		t.Fatal(err)
	}
	rows[0].Self, pairs[0].Rule = SelectedUnaryKeep, SelectedUnaryKeep
	for _, tc := range []struct {
		parent, child Symbol
		want          SelectedUnaryRule
	}{
		{0, 0, SelectedUnaryPass}, {0, 1, SelectedUnaryKeep}, {1, 0, SelectedUnaryPass},
		{2, 0, SelectedUnaryRenameLeaf}, {2, 1, SelectedUnaryKeep}, {3, 3, SelectedUnaryKeep},
		{4, 0, SelectedUnaryKeep}, {0, 4, SelectedUnaryKeep}, {65535, 65535, SelectedUnaryKeep},
	} {
		if got := p.Rule(tc.parent, tc.child); got != tc.want {
			t.Fatalf("%d/%d: got %d want %d", tc.parent, tc.child, got, tc.want)
		}
	}
	expected := uint64(unsafe.Sizeof(p)) + uint64(cap(p.rows)) + uint64(cap(p.pairs))*uint64(unsafe.Sizeof(selectedUnaryPair{}))
	if p.Width() != 4 || p.RetainedBytes() != expected {
		t.Fatalf("width or retained storage missing: %d/%d", p.Width(), p.RetainedBytes())
	}
}

func TestSelectedUnaryProgramRejectsInvalidPairs(t *testing.T) {
	rows := []SelectedUnaryRow{{Self: SelectedUnaryPass}, {Self: SelectedUnaryPass}}
	for _, pairs := range [][]SelectedUnaryOverride{
		{{Parent: 2, Child: 0}}, {{Parent: 0, Child: 2}}, {{Parent: 0, Child: 0}},
		{{Parent: 0, Child: 1, Rule: 3}}, {{Parent: 0, Child: 1}, {Parent: 0, Child: 1}},
	} {
		if _, err := NewSelectedUnaryProgram(rows, pairs); err == nil {
			t.Fatalf("invalid pairs accepted: %+v", pairs)
		}
	}
	if _, err := NewSelectedUnaryProgram(nil, nil); err == nil {
		t.Fatal("empty program accepted")
	}
	if _, err := NewSelectedUnaryProgram([]SelectedUnaryRow{{Default: 3}}, nil); err == nil {
		t.Fatal("invalid default accepted")
	}
}

func TestSelectedUnaryProgramFullSymbolRange(t *testing.T) {
	rows := make([]SelectedUnaryRow, 1<<16)
	rows[65535] = SelectedUnaryRow{Self: SelectedUnaryPass}
	p, err := NewSelectedUnaryProgram(rows, []SelectedUnaryOverride{
		{Parent: 65535, Child: 65534, Rule: SelectedUnaryRenameLeaf},
		{Parent: 1, Child: 65535, Rule: SelectedUnaryPass},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Rule(65535, 65534) != SelectedUnaryRenameLeaf || p.Rule(1, 65535) != SelectedUnaryPass || p.Rule(65535, 65535) != SelectedUnaryPass || p.Rule(65535, 1) != SelectedUnaryKeep {
		t.Fatal("high symbol key or sorted sparse rules changed")
	}
	if _, err := NewSelectedUnaryProgram(make([]SelectedUnaryRow, (1<<16)+1), nil); err == nil {
		t.Fatal("oversized symbol space accepted")
	}
	if _, err := NewSelectedUnaryProgram([]SelectedUnaryRow{{Self: 3}}, nil); err == nil {
		t.Fatal("invalid diagonal accepted")
	}
}

package parsercorephase0

import "testing"

type selectedNameTestReader []string

func (names selectedNameTestReader) SelectedSymbolText(id Symbol) string { return names[id] }

func TestSelectedSymbolNameIndexExactByteOrderingAndDuplicates(t *testing.T) {
	names := selectedNameTestReader{"z", "", "alpha", "alpha", "é", "α", "ALPHA"}
	ids := []Symbol{0, 1, 2, 3, 4, 5, 6}
	SortSelectedSymbolNames(ids, names)
	for i := 1; i < len(ids); i++ {
		if names[ids[i-1]] > names[ids[i]] {
			t.Fatalf("index out of order: %v", ids)
		}
	}
	for _, tc := range []struct {
		name  string
		want  bool
		count int
	}{{"", true, 1}, {"alpha", true, 2}, {"ALPHA", true, 1}, {"é", true, 1}, {"absent", false, 0}} {
		pos, ok := FindSelectedSymbolName(ids, tc.name, names)
		if ok != tc.want {
			t.Fatalf("%q found=%v", tc.name, ok)
		}
		count := 0
		for i := pos; i < len(ids) && names[ids[i]] == tc.name; i++ {
			count++
		}
		if count != tc.count {
			t.Fatalf("%q count=%d", tc.name, count)
		}
		if pos > 0 && names[ids[pos-1]] >= tc.name {
			t.Fatalf("%q is not first matching/insertion position", tc.name)
		}
	}
}

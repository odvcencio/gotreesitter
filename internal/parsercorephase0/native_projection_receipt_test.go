package parsercorephase0

import "testing"

func TestNativeProjectionReceiptKeepsAliasBoundary(t *testing.T) {
	policy := selectedStoreTestPolicy(t, 1, 2)
	c, err := New(&selectedStorePolicyTable{policy: policy}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	c.selectedPolicy = &policy
	leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 3}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                 string
		raw                  Symbol
		terminal, leaf, same bool
		children             int
		aliases              bool
		want                 bool
	}{
		{"exact", 1, true, true, false, 0, false, true},
		{"hidden leaf", 3, true, true, true, 0, false, true},
		{"changed hidden leaf", 3, true, true, false, 0, false, false},
		{"visible alias", 2, true, true, true, 0, false, false},
		{"hidden unary parent", 3, false, false, false, 1, false, true},
		{"visible unary parent", 2, false, false, true, 1, false, false},
		{"hidden multi parent", 3, false, false, true, 2, false, false},
		{"hidden aliased parent", 3, false, false, true, 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			children := make([]SubtreeID, tc.children)
			for i := range children {
				children[i] = leaf
			}
			var aliases []Symbol
			if tc.aliases {
				aliases = []Symbol{1}
			}
			id, err := c.appendSubtree(subtreeRecord{symbol: tc.raw, terminal: tc.terminal, endByte: 3}, children, nil, aliases)
			if err != nil {
				t.Fatal(err)
			}
			g, ok, err := c.NativeProjectionReceiptGeometry(id, 1, tc.leaf, tc.same)
			if err != nil || ok != tc.want || g.Symbol != tc.raw || g.StartByte != 0 || g.EndByte != 3 {
				t.Fatalf("identity=%+v/%t/%v, want compatible=%t", g, ok, err, tc.want)
			}
		})
	}
	if _, _, err := c.NativeProjectionReceiptGeometry(0, 1, true, true); err == nil {
		t.Fatal("invalid source accepted")
	}
}

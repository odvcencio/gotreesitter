package gotreesitter

import (
	"slices"
	"testing"
)

func TestForestRootEndWindowPreservesDescendingMatches(t *testing.T) {
	zero := &Node{endByte: 2, startByte: 2}
	zero.setExtra(true)
	a, b, c := &Node{endByte: 1}, &Node{endByte: 2}, &Node{endByte: 3}
	for _, tc := range []struct {
		name    string
		root    Node
		ordered bool
	}{
		{"repeated end with empty extra", Node{children: []*Node{a, b, zero, c}}, true},
		{"nonmonotonic", Node{children: []*Node{a, b, c, zero}}, false},
		{"missing pointer", Node{children: []*Node{a, nil, b}}, false},
		{"indexed backing", Node{childIndex: finalChildSidecarIndexBase, children: []*Node{a, b}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kids := tc.root.children
			ordered := forestOrderedRootChildren(&tc.root)
			if (ordered != nil) != tc.ordered {
				t.Fatalf("ordered = %t, want %t", ordered != nil, tc.ordered)
			}
			for start := range kids {
				for _, target := range []uint32{0, 1, 2, 3, 4, ^uint32(0)} {
					matching := func(lo, hi int) []int {
						var ends []int
						for i := hi; i > lo; i-- {
							if kid := kids[i-1]; kid != nil && kid.endByte == target {
								ends = append(ends, i)
							}
						}
						return ends
					}
					lo, hi := forestRootEndWindow(ordered, start, len(kids), target)
					got, want := matching(lo, hi), matching(start, len(kids))
					if !slices.Equal(got, want) || (tc.ordered && hi-lo != len(want)) {
						t.Fatalf("start %d endByte %d window [%d,%d): matches %v, want %v", start, target, lo, hi, got, want)
					}
				}
			}
		})
	}
}

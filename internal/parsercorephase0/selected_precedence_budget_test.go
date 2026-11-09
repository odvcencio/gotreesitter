package parsercorephase0

import (
	"errors"
	"strings"
	"testing"
	"unsafe"
)

func TestSelectedStoreOnePassRawPrecedence(t *testing.T) {
	for _, mode := range []string{"hidden", "unary", "alias_renamed", "alias_retained", "included_extra", "trailing_extra", "synthetic_extra"} {
		t.Run(mode, func(t *testing.T) {
			policy := selectedStoreTestPolicy(t, 2, 1, 4)
			if mode == "unary" {
				policy.Unary[2*len(policy.Symbols)+1] = SelectedUnaryPass
				policy.ExpectedRoot = 1
			}
			if mode == "synthetic_extra" {
				policy.ProjectSyntheticSiblingRoot = true
			}
			if mode == "alias_retained" {
				policy.SetRetainedAliasChildren([]SelectedAliasChildPair{{Alias: 4, Child: 1}})
			}
			c, err := New(&selectedStorePolicyTable{policy: policy}, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			add := func(r subtreeRecord, children []SubtreeID, aliases []Symbol) SubtreeID {
				id, e := c.appendSubtree(r, children, nil, aliases)
				if e != nil {
					t.Fatal(e)
				}
				return id
			}
			leaf := add(subtreeRecord{symbol: 1, terminal: true, endByte: 1, dynamicPrecedence: 2}, nil, nil)
			var roots []SubtreeID
			if mode == "hidden" {
				h := add(subtreeRecord{symbol: 3, endByte: 1, dynamicPrecedence: -7}, []SubtreeID{leaf}, nil)
				roots = []SubtreeID{add(subtreeRecord{symbol: 2, endByte: 1, dynamicPrecedence: 3}, []SubtreeID{h}, nil)}
			} else {
				children := []SubtreeID{leaf}
				var aliases []Symbol
				if mode == "alias_renamed" || mode == "alias_retained" {
					aliases = []Symbol{4}
				}
				if mode == "included_extra" {
					e := add(subtreeRecord{symbol: 1, terminal: true, extra: true, endByte: 1, dynamicPrecedence: 4}, nil, nil)
					children = append(children, e)
				}
				root := add(subtreeRecord{symbol: 2, endByte: 1, dynamicPrecedence: -5}, children, aliases)
				roots = []SubtreeID{root}
				if mode == "trailing_extra" || mode == "synthetic_extra" {
					e := add(subtreeRecord{symbol: 1, terminal: true, extra: true, endByte: 1, dynamicPrecedence: 4}, nil, nil)
					roots = append(roots, e)
				}
			}
			store, e := c.buildSelectedStoreOnePass(roots, policy, []byte("x"), nil)
			if e != nil {
				t.Fatal(e)
			}
			defer store.Release()
			r := store.records[store.root-1]
			want := int32(-3)
			if mode == "hidden" {
				want = -2
			}
			if mode == "included_extra" {
				want = 1
			}
			if mode == "synthetic_extra" {
				want = 6
			}
			if r.DynamicPrecedence != want {
				t.Fatalf("mode=%s precedence=%d want=%d root=%+v", mode, r.DynamicPrecedence, want, r)
			}
			if mode == "alias_renamed" || mode == "alias_retained" {
				id := store.children[r.FirstChild]
				child := store.records[id-1]
				if child.Symbol != 4 || child.DynamicPrecedence != 2 {
					t.Fatalf("alias=%+v", child)
				}
				if mode == "alias_retained" && child.ChildCount != 1 {
					t.Fatalf("retained alias=%+v", child)
				}
			}
		})
	}
}

func TestSelectedStoreOnePassDeepScratchBudgetVisibility(t *testing.T) {
	const depth = 300
	c, err := New(&fakeTable{}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	root, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < depth; i++ {
		root, err = c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{root}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	root, err = c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{root}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := unsafe.Sizeof(selectedOccurrenceFrame{}); got != 16 {
		t.Fatalf("frame bytes=%d want=16", got)
	}
	stop := errors.New("scratch budget exceeded")
	policy := selectedStoreTestPolicy(t, 2, 1)
	peak := 0
	peakPayload, peakEdges := 0, 0
	store, err := c.buildSelectedStoreOnePass([]SubtreeID{root}, policy, []byte("x"), func() error {
		capacity := cap(c.selectedBuild.stack)
		if capacity > peak {
			peak = capacity
		}
		payloads, edges := cap(c.selectedBuild.collect), cap(c.selectedBuild.rawRoots)
		if payloads > peakPayload {
			peakPayload = payloads
		}
		if edges > peakEdges {
			peakEdges = edges
		}
		need := uint64(capacity)*uint64(unsafe.Sizeof(selectedOccurrenceFrame{})) + uint64(payloads+edges)*uint64(unsafe.Sizeof(uint32(0)))
		if c.SelectedBuildAuxiliaryBytes() < need {
			return errors.New("scratch backing undercharged")
		}
		if capacity > 64 {
			return stop
		}
		return nil
	})
	if store != nil {
		store.Release()
	}
	if !errors.Is(err, stop) || peak <= 64 {
		t.Fatalf("tight budget store=%v err=%v peak=%d", store, err, peak)
	}
	if len(c.selectedBuild.stack) != 0 {
		t.Fatalf("failed build retained %d live frames", len(c.selectedBuild.stack))
	}
	peak, peakPayload, peakEdges = 0, 0, 0
	store, err = c.buildSelectedStoreOnePass([]SubtreeID{root}, policy, []byte("x"), func() error {
		capacity := cap(c.selectedBuild.stack)
		if capacity > peak {
			peak = capacity
		}
		payloads, edges := cap(c.selectedBuild.collect), cap(c.selectedBuild.rawRoots)
		if payloads > peakPayload {
			peakPayload = payloads
		}
		if edges > peakEdges {
			peakEdges = edges
		}
		need := uint64(capacity)*uint64(unsafe.Sizeof(selectedOccurrenceFrame{})) + uint64(payloads+edges)*uint64(unsafe.Sizeof(uint32(0)))
		if c.SelectedBuildAuxiliaryBytes() < need {
			return errors.New("scratch backing undercharged")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
	if peak < 256 || peakPayload < 256 || peakEdges < 256 {
		t.Fatalf("peak capacities stack=%d payload=%d edge=%d want>=256", peak, peakPayload, peakEdges)
	}
	t.Logf("frame bytes=16 peak capacities stack=%d payload=%d edge=%d charged=%d; rejected build cleaned and same-Core retry passed", peak, peakPayload, peakEdges, c.SelectedBuildAuxiliaryBytes())
}

func TestSelectedStoreOnePassRawPrecedenceOverflowCleanup(t *testing.T) {
	const width = 32769
	c, err := New(&fakeTable{}, Limits{MaxSubtrees: 8, MaxChildren: 2*width + 2, MaxSelectedOccurrences: 2*width + 4, MaxSelectedBytes: 1 << 25})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1, dynamicPrecedence: 32767}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	children := make([]SubtreeID, width)
	for i := range children {
		children[i] = leaf
	}
	left, err := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, children, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	right, err := c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, children, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1, dynamicPrecedence: 2}, []SubtreeID{left, right}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := selectedStoreTestPolicy(t, 2, 1)
	store, err := c.buildSelectedStoreOnePass([]SubtreeID{root}, policy, []byte("x"), nil)
	if store != nil {
		store.Release()
	}
	if err == nil || !strings.Contains(err.Error(), "precedence overflow") {
		t.Fatalf("store=%v error=%v", store, err)
	}
	if len(c.selectedBuild.stack) != 0 {
		t.Fatal("overflow retained live frames")
	}
	store, err = c.buildSelectedStoreOnePass([]SubtreeID{leaf}, selectedStoreTestPolicy(t, 1), []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release()
}

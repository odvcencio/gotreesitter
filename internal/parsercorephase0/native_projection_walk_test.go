package parsercorephase0

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"unsafe"
)

type nativeWalkTestSink struct {
	records  []NativeProjectionRecord
	children [][]SelectedNodeID
}

func (s *nativeWalkTestSink) Emit(r NativeProjectionRecord, ids []SelectedNodeID) (SelectedNodeID, error) {
	s.records = append(s.records, r)
	s.children = append(s.children, append([]SelectedNodeID(nil), ids...))
	return SelectedNodeID(len(s.records)), nil
}
func (s *nativeWalkTestSink) Read(id SelectedNodeID) (NativeProjectionRecord, error) {
	return s.records[id-1], nil
}
func (s *nativeWalkTestSink) Rewrite(id SelectedNodeID, r NativeProjectionRecord) error {
	s.records[id-1] = r
	return nil
}

// Keep the former postorder coverage check independent of the fused traversal.
func nativeWalkReferenceTiling(c *Core, source []byte, root Symbol, view *MaterializationSubtreeView) error {
	if view.EndByte < view.StartByte || uint64(view.EndByte) > uint64(len(source)) {
		return errors.New("extent")
	}
	if view.Terminal {
		return nil
	}
	last := view.StartByte
	for _, id := range view.Children {
		g, err := c.SubtreeGeometry(id)
		if err != nil {
			return err
		}
		if g.EndByte < g.StartByte || uint64(g.EndByte) > uint64(len(source)) {
			return errors.New("child extent")
		}
		if view.Symbol != root && g.StartByte > last && len(bytes.TrimSpace(source[last:g.StartByte])) != 0 {
			return errors.New("gap")
		}
		if g.EndByte > last {
			last = g.EndByte
		}
	}
	if view.Symbol != root && last < view.EndByte && len(bytes.TrimSpace(source[last:view.EndByte])) != 0 {
		return errors.New("tail")
	}
	return nil
}

func TestNativeProjectionFusedCoverageMatchesReference(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		spans        [][2]uint32
		parent       [2]uint32
		root         bool
		want         bool
	}{
		{"contiguous", "ab", [][2]uint32{{0, 1}, {1, 2}}, [2]uint32{0, 2}, false, true},
		{"trivia", " a b ", [][2]uint32{{1, 2}, {3, 4}}, [2]uint32{0, 5}, false, true},
		{"leading_gap", "xab", [][2]uint32{{1, 2}, {2, 3}}, [2]uint32{0, 3}, false, false},
		{"middle_gap", "axb", [][2]uint32{{0, 1}, {2, 3}}, [2]uint32{0, 3}, false, false},
		{"trailing_gap", "abx", [][2]uint32{{0, 1}, {1, 2}}, [2]uint32{0, 3}, false, false},
		{"root_gap", "axb", [][2]uint32{{0, 1}, {2, 3}}, [2]uint32{0, 3}, true, true},
		{"overlap", "ab", [][2]uint32{{0, 2}, {1, 2}}, [2]uint32{0, 2}, false, true},
		{"zero_width", "a", [][2]uint32{{0, 0}, {0, 1}, {1, 1}}, [2]uint32{0, 1}, false, true},
		{"child_outside_source", "ab", [][2]uint32{{0, 1}, {1, 3}}, [2]uint32{0, 2}, false, false},
		{"child_reversed", "ab", [][2]uint32{{0, 1}, {2, 1}}, [2]uint32{0, 2}, false, false},
		{"parent_outside_source", "ab", [][2]uint32{{0, 1}, {1, 2}}, [2]uint32{0, 3}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(&selectedStorePolicyTable{policy: selectedStoreTestPolicy(t, 2, 1)}, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			var children []SubtreeID
			for _, span := range tc.spans {
				id, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				r, _ := c.subtree(id)
				r.startByte, r.endByte = span[0], span[1]
				children = append(children, id)
			}
			sym := Symbol(3)
			if tc.root {
				sym = 2
			}
			parent, err := c.appendSubtree(subtreeRecord{symbol: sym, endByte: 2}, children, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			r, _ := c.subtree(parent)
			r.startByte, r.endByte = tc.parent[0], tc.parent[1]
			var results [2]nativeWalkTestSink
			for mode := 0; mode < 2; mode++ {
				var scratch MaterializationPostorderScratch
				replay := NativeProjectionReplay{Scratch: &scratch, Transition: func(pre StateID, v *MaterializationReplayView) (StateID, bool, error) {
					return pre + StateID(v.Symbol) + 1, true, nil
				}}
				if mode == 0 {
					replay.VisitRaw = func(_ SubtreeID, v *MaterializationSubtreeView) error {
						return nativeWalkReferenceTiling(c, []byte(tc.source), 2, v)
					}
				} else {
					replay.Tiling = &NativeProjectionTiling{RootSymbol: 2, HasRootSymbol: true, GapIsTolerated: func(s []byte) bool { return len(bytes.TrimSpace(s)) == 0 }}
				}
				_, err := c.StreamNativeProjection([]SubtreeID{parent}, []byte(tc.source), &results[mode], replay, nil)
				if (err == nil) != tc.want {
					t.Fatalf("mode %d: %v, want accepted=%t", mode, err, tc.want)
				}
				if scratch.inUse || len(scratch.nativeFrames) != 0 || len(scratch.colors) != 0 {
					t.Fatal("traversal retained live state")
				}
			}
			if tc.want && !reflect.DeepEqual(results[0], results[1]) {
				t.Fatalf("projection or replay changed: %+v / %+v", results[0], results[1])
			}
		})
	}
}

func TestNativeProjectionFusedStackGrowthAndCancellation(t *testing.T) {
	c, err := New(&selectedStorePolicyTable{policy: selectedStoreTestPolicy(t, 2, 1)}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := leaf
	for i := 0; i < 1024; i++ {
		root, err = c.appendSubtree(subtreeRecord{symbol: 3, endByte: 1}, []SubtreeID{root}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	var scratch MaterializationPostorderScratch
	calls := 0
	replay := NativeProjectionReplay{Scratch: &scratch, Transition: func(pre StateID, v *MaterializationReplayView) (StateID, bool, error) { return pre + 1, true, nil }, VisitTerminal: func(id SubtreeID, v *MaterializationSubtreeView) error {
		if id != leaf || !v.Terminal {
			t.Fatal("terminal callback saw parent")
		}
		calls++
		return nil
	}}
	var sink nativeWalkTestSink
	if _, err = c.StreamNativeProjection([]SubtreeID{root}, []byte("x"), &sink, replay, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || cap(scratch.nativeFrames) < 1025 || cap(scratch.frames) != 0 || cap(c.selectedBuild.stack) != 0 || cap(c.selectedBuild.collect) != 0 || cap(c.selectedBuild.rawRoots) != 0 {
		t.Fatal("native walk did not use one frame stack")
	}
	want := uint64(cap(scratch.colors)) + uint64(cap(scratch.nativeFrames))*uint64(unsafe.Sizeof(nativeProjectionFrame{}))
	if scratch.RetainedBytes() != want {
		t.Fatalf("scratch accounting %d want %d", scratch.RetainedBytes(), want)
	}
	stop := fmt.Errorf("cancel projection")
	polls := 0
	if _, err = c.StreamNativeProjection([]SubtreeID{root}, []byte("x"), &nativeWalkTestSink{}, replay, func() error {
		polls++
		if polls == 2 {
			return stop
		}
		return nil
	}); !errors.Is(err, stop) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, f := range scratch.nativeFrames[:cap(scratch.nativeFrames)] {
		if f != (nativeProjectionFrame{}) {
			t.Fatal("failed traversal retained frame")
		}
	}
	if scratch.inUse {
		t.Fatal("failed traversal stayed active")
	}
	if _, err = c.StreamNativeProjection([]SubtreeID{root}, []byte("x"), &nativeWalkTestSink{}, replay, nil); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

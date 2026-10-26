package parsercorephase0

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type nativeSelectedIdentityTable struct {
	selectedStorePolicyTable
	identity [32]byte
}

func (t *nativeSelectedIdentityTable) TableIdentity() ([32]byte, bool) { return t.identity, true }
func nativeSelectedFixture(t *testing.T) (*Core, SubtreeID, NativeProjectionReplay, *NativeSelectedScratch) {
	table := &nativeSelectedIdentityTable{selectedStorePolicyTable: selectedStorePolicyTable{policy: selectedStoreTestPolicy(t, 2, 1)}, identity: [32]byte{1}}
	c, e := New(table, Limits{})
	if e != nil {
		t.Fatal(e)
	}
	leaf, e := c.appendSubtree(subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	root, e := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf}, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	replay := NativeProjectionReplay{Scratch: &MaterializationPostorderScratch{}, RootPre: 1, Transition: func(pre StateID, _ *MaterializationReplayView) (StateID, bool, error) { return pre + 1, true, nil }}
	return c, root, replay, &NativeSelectedScratch{}
}
func TestNativeSelectedOutputReaderLifetime(t *testing.T) {
	c, root, replay, scratch := nativeSelectedFixture(t)
	s, e := c.BuildNativeSelectedOutput([]SubtreeID{root}, []byte("x"), replay, scratch, nil)
	if e != nil {
		t.Fatal(e)
	}
	record, ok := s.Record(s.Root())
	if !ok || record.ChildCount != 1 || record.StartByte != 0 || record.EndByte != 1 {
		t.Fatalf("root=%+v", record)
	}
	child, _ := s.Child(record, 0)
	r, _ := s.Record(child)
	if r.Parent != s.Root() || r.Symbol != 1 {
		t.Fatalf("child=%+v", r)
	}
	if scratch.store != nil || scratch.core != nil || scratch.poll != nil || len(scratch.replay) != 0 {
		t.Fatal("builder retained live owner")
	}
	if e = c.Reset(); e != nil {
		t.Fatal(e)
	}
	again, _ := s.Record(s.Root())
	if !reflect.DeepEqual(record, again) {
		t.Fatal("reset changed published output")
	}
	s.Release()
	s.Release()
	if s.Root() != 0 || s.RetainedBytes() != 0 {
		t.Fatal("release retained tree")
	}
}
func TestNativeSelectedOutputFailureReleasesOwner(t *testing.T) {
	for _, mode := range []string{"occurrence", "bytes", "cancel", "table"} {
		t.Run(mode, func(t *testing.T) {
			c, root, replay, scratch := nativeSelectedFixture(t)
			var calls int
			stop := errors.New("cancel selected output")
			poll := func() error {
				calls++
				if mode == "cancel" && calls == 2 {
					return stop
				}
				return nil
			}
			switch mode {
			case "occurrence":
				c.limits.MaxSelectedOccurrences = 1
			case "bytes":
				c.limits.MaxSelectedBytes = 1
			case "table":
				c.tables.(*nativeSelectedIdentityTable).identity[0]++
			}
			s, e := c.BuildNativeSelectedOutput([]SubtreeID{root}, []byte("x"), replay, scratch, poll)
			if e == nil || s != nil {
				t.Fatalf("accepted %s", mode)
			}
			if mode == "cancel" && !errors.Is(e, stop) {
				t.Fatal(e)
			}
			if scratch.store != nil || scratch.core != nil || scratch.poll != nil || len(scratch.replay) != 0 {
				t.Fatal("failure retained live owner")
			}
		})
	}
}
func TestNativeSelectedOutputRejectsSharedOccurrence(t *testing.T) {
	c, root, replay, scratch := nativeSelectedFixture(t)
	raw, _ := c.subtree(root)
	leaf := c.children[raw.firstChild]
	duplicate, e := c.appendSubtree(subtreeRecord{symbol: 2, endByte: 1}, []SubtreeID{leaf, leaf}, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	s, e := c.BuildNativeSelectedOutput([]SubtreeID{duplicate}, []byte("x"), replay, scratch, nil)
	if e == nil || s != nil || !strings.Contains(e.Error(), "repeated public-tree ownership") {
		t.Fatalf("duplicate accepted: %v", e)
	}
}

package parsercorephase0

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func newSoleDerivationCore() *Core {
	return &Core{
		limits: Limits{MaxDerivations: 64},
		nodes:  []nodeRecord{{pathCount: 1}},
	}
}

// The links use insertion order, as in published adjacency chains.
func appendSoleDerivationNode(c *Core, count uint64, links ...linkRecord) Head {
	var first LinkID
	for _, link := range links {
		link.next = first
		c.links = append(c.links, link)
		first = LinkID(len(c.links))
	}
	c.nodes = append(c.nodes, nodeRecord{
		firstLink: uint32(first), linkCount: uint32(len(links)), pathCount: count,
	})
	return Head{Node: NodeID(len(c.nodes))}
}

func newSoleDerivationBranches() (*Core, Head) {
	c := newSoleDerivationCore()
	first := appendSoleDerivationNode(c, 1, linkRecord{prev: 1, payload: 11})
	second := appendSoleDerivationNode(c, 1, linkRecord{prev: 1, payload: 22})
	head := appendSoleDerivationNode(c, 2,
		linkRecord{prev: first.Node, payload: 33},
		linkRecord{prev: second.Node, payload: 44},
	)
	return c, head
}

func compareSoleDerivation(t *testing.T, c *Core, head Head) (Derivation, bool, error) {
	t.Helper()
	want, wantErr := c.Derivations(head)
	got, sole, err := c.SoleDerivation(head)
	if (err == nil) != (wantErr == nil) {
		t.Fatalf("sole error=%v, enumeration error=%v", err, wantErr)
	}
	if err != nil && err.Error() != wantErr.Error() {
		t.Fatalf("sole error=%q, enumeration error=%q", err, wantErr)
	}
	if errors.Is(err, ErrDerivationEnumerationCap) != errors.Is(wantErr, ErrDerivationEnumerationCap) {
		t.Fatalf("sole cap identity=%v, enumeration cap identity=%v", err, wantErr)
	}
	wantSole := wantErr == nil && len(want) == 1
	if sole != wantSole {
		t.Fatalf("sole=%t, enumeration count=%d error=%v", sole, len(want), wantErr)
	}
	if sole && !reflect.DeepEqual(got, want[0]) {
		t.Fatalf("sole derivation=%+v, enumeration=%+v", got, want[0])
	}
	if !sole && !reflect.DeepEqual(got, Derivation{}) {
		t.Fatalf("unsupported or failed result retained a derivation: %+v", got)
	}
	return got, sole, err
}

func TestSoleDerivationPreservesPayloadsScoresAndOrder(t *testing.T) {
	for _, saturated := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "saturated"}[saturated], func(t *testing.T) {
			c := newSoleDerivationCore()
			first := appendSoleDerivationNode(c, 1,
				linkRecord{prev: 1, payload: 11, scoreDelta: 7, flags: linkFlagHasOrder, order: 5})
			gap := appendSoleDerivationNode(c, 1,
				linkRecord{prev: first.Node, flags: linkFlagRecoveryDiscontinuity})
			head := appendSoleDerivationNode(c, 1,
				linkRecord{prev: gap.Node, payload: 22, scoreDelta: -3, flags: linkFlagHasOrder, order: math.MaxUint64})
			if saturated {
				c.nodes[head.Node-1].pathCount = math.MaxUint64
			}
			got, sole, err := compareSoleDerivation(t, c, head)
			want := Derivation{Payloads: []SubtreeID{11, 22}, Score: 4, HasBranchOrder: true, BranchOrder: math.MaxUint64}
			if err != nil || !sole || !reflect.DeepEqual(got, want) {
				t.Fatalf("derivation=%+v sole=%t error=%v, want %+v", got, sole, err, want)
			}
		})
	}
}

func TestSoleDerivationWarmSinglePathAllocation(t *testing.T) {
	c := newSoleDerivationCore()
	previous := Head{Node: 1}
	for index := 0; index < 40; index++ {
		previous = appendSoleDerivationNode(c, 1, linkRecord{
			prev: previous.Node, payload: SubtreeID(index + 1),
		})
	}

	var retained Derivation
	allocations := testing.AllocsPerRun(1000, func() {
		path, sole, err := c.SoleDerivation(previous)
		if err != nil || !sole || len(path.Payloads) != 40 {
			panic("sole derivation failed")
		}
		retained = path
	})
	if allocations != 1 {
		t.Fatalf("warm sole derivation allocations=%v, want 1 payload allocation", allocations)
	}
	if len(retained.Payloads) != 40 || retained.Payloads[0] != 1 || retained.Payloads[39] != 40 {
		t.Fatalf("retained payloads=%v", retained.Payloads)
	}
}

func TestSoleDerivationVisibleNodeCountWarmNoAllocation(t *testing.T) {
	c := newSoleDerivationCore()
	symbols := make([]SelectedSymbolPolicy, 41)
	previous := Head{Node: 1}
	for index := 0; index < 40; index++ {
		symbols[index+1].Visible = true
		c.subtrees = append(c.subtrees, subtreeRecord{symbol: Symbol(index + 1)})
		previous = appendSoleDerivationNode(c, 1, linkRecord{
			prev: previous.Node, payload: SubtreeID(index + 1),
		})
	}

	count, sole, err := c.SoleDerivationVisibleNodeCount(symbols, previous)
	if err != nil || !sole || count != 40 {
		t.Fatalf("visible count=%d sole=%t error=%v", count, sole, err)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		got, gotSole, gotErr := c.SoleDerivationVisibleNodeCount(symbols, previous)
		if gotErr != nil || !gotSole || got != 40 {
			panic("sole visible count failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm sole visible count allocations=%v, want 0", allocations)
	}
}

func TestSoleDerivationVisibleNodeCountPreservesFallbacksAndErrorOrder(t *testing.T) {
	t.Run("saturated sole path", func(t *testing.T) {
		c := newSoleDerivationCore()
		c.subtrees = []subtreeRecord{{symbol: 1}}
		head := appendSoleDerivationNode(c, math.MaxUint64, linkRecord{prev: 1, payload: 1})
		count, sole, err := c.SoleDerivationVisibleNodeCount(
			[]SelectedSymbolPolicy{{}, {Visible: true}}, head,
		)
		if err != nil || !sole || count != 1 {
			t.Fatalf("visible count=%d sole=%t error=%v", count, sole, err)
		}
	})

	t.Run("ambiguous path", func(t *testing.T) {
		c, head := newSoleDerivationBranches()
		if count, sole, err := c.SoleDerivationVisibleNodeCount(nil, head); err != nil || sole || count != 0 {
			t.Fatalf("visible count=%d sole=%t error=%v", count, sole, err)
		}
	})

	t.Run("score before payload", func(t *testing.T) {
		c := newSoleDerivationCore()
		first := appendSoleDerivationNode(c, 1, linkRecord{
			prev: 1, payload: math.MaxUint32, scoreDelta: math.MaxInt64,
		})
		head := appendSoleDerivationNode(c, 1, linkRecord{prev: first.Node, payload: 1, scoreDelta: 1})
		if _, _, err := c.SoleDerivationVisibleNodeCount(nil, head); err == nil || !strings.Contains(err.Error(), "score overflow") {
			t.Fatalf("visible count error=%v", err)
		}
	})
}

func TestSoleDerivationValidControls(t *testing.T) {
	for _, name := range []string{
		"seed", "seed_with_unused_adjacency", "zero_cap_single_path", "forward_acyclic_edge",
		"ambiguous", "saturated_ambiguous", "ambiguous_payload_id_is_not_dereferenced",
	} {
		t.Run(name, func(t *testing.T) {
			c := newSoleDerivationCore()
			head := Head{Node: 1}
			wantSole := true
			switch name {
			case "seed":
				c.limits.MaxDerivations = 0
			case "seed_with_unused_adjacency":
				c.nodes[0].firstLink = math.MaxUint32
			case "zero_cap_single_path":
				head = appendSoleDerivationNode(c, 1, linkRecord{prev: 1, payload: 11})
				c.limits.MaxDerivations = 0
			case "forward_acyclic_edge":
				head = appendSoleDerivationNode(c, 1, linkRecord{prev: 3, payload: 11})
				appendSoleDerivationNode(c, 1, linkRecord{prev: 1, payload: 22})
			case "ambiguous", "saturated_ambiguous", "ambiguous_payload_id_is_not_dereferenced":
				c, head = newSoleDerivationBranches()
				wantSole = false
				if name == "saturated_ambiguous" {
					c.nodes[head.Node-1].pathCount = math.MaxUint64
				}
				if name == "ambiguous_payload_id_is_not_dereferenced" {
					c.links[c.nodes[head.Node-1].firstLink-1].payload = math.MaxUint32
				}
			}
			_, sole, err := compareSoleDerivation(t, c, head)
			if err != nil || sole != wantSole {
				t.Fatalf("sole=%t error=%v, want sole=%t", sole, err, wantSole)
			}
		})
	}
}

func TestSoleDerivationPreservesValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Core, Head) Head
		want   string
	}{
		{"invalid_head", func(c *Core, head Head) Head { return Head{} }, "invalid node id 0"},
		{"invalid_predecessor", func(c *Core, head Head) Head {
			c.links[c.nodes[head.Node-1].firstLink-1].prev = 999
			return head
		}, "invalid node id 999"},
		{"graph_cycle", func(c *Core, head Head) Head {
			c.links[c.nodes[2].firstLink-1].prev = 3
			return head
		}, "graph cycle"},
		{"adjacency_cycle", func(c *Core, head Head) Head {
			first := c.nodes[head.Node-1].firstLink
			c.links[first-1].next = LinkID(first)
			return head
		}, "adjacency cycle"},
		{"adjacency_out_of_range", func(c *Core, head Head) Head {
			c.nodes[head.Node-1].firstLink = 999
			return head
		}, "link adjacency out of range"},
		{"adjacency_short", func(c *Core, head Head) Head {
			c.nodes[head.Node-1].linkCount++
			return head
		}, "adjacency shorter than recorded link count"},
		{"adjacency_long", func(c *Core, head Head) Head {
			c.nodes[head.Node-1].linkCount--
			return head
		}, "adjacency exceeds recorded link count"},
		{"unknown_flags", func(c *Core, head Head) Head {
			c.links[c.nodes[head.Node-1].firstLink-1].flags = math.MaxUint32
			return head
		}, "link has unknown flags"},
		{"ordinary_link_without_payload", func(c *Core, head Head) Head {
			c.links[c.nodes[head.Node-1].firstLink-1].payload = 0
			return head
		}, "ordinary link has no payload"},
		{"malformed_discontinuity", func(c *Core, head Head) Head {
			c.links[c.nodes[head.Node-1].firstLink-1].flags = linkFlagRecoveryDiscontinuity
			return head
		}, "recovery discontinuity has a payload"},
		{"malformed_seed", func(c *Core, head Head) Head {
			c.nodes[0].pathCount = 0
			return head
		}, "malformed seed path count"},
		{"child_count_mismatch", func(c *Core, head Head) Head {
			c.nodes[1].pathCount = 2
			return head
		}, "path-count mismatch: enumerated 1, recorded 2"},
		{"head_count_mismatch", func(c *Core, head Head) Head {
			c.nodes[head.Node-1].pathCount = 1
			return head
		}, "path-count mismatch: enumerated 2, recorded 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, head := newSoleDerivationBranches()
			head = tc.change(c, head)
			_, _, err := compareSoleDerivation(t, c, head)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestSoleDerivationPreservesCapAndErrorOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		cap  uint64
		want string
	}{
		{"score_overflow_before_cap", 2, "score overflow"},
		{"cap_before_score_overflow", 1, "derivation enumeration cap"},
		{"later_child_cycle_before_parent_cap", 1, "graph cycle"},
		{"nested_cap_before_parent_score", 1, "derivation enumeration cap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, head := newSoleDerivationBranches()
			c.limits.MaxDerivations = tc.cap
			c.links[c.nodes[2].firstLink-1].scoreDelta = math.MaxInt64
			c.links[c.nodes[head.Node-1].firstLink-1].scoreDelta = 1
			if tc.name == "later_child_cycle_before_parent_cap" {
				c.links[c.nodes[2].firstLink-1].prev = 3
			}
			if tc.name == "nested_cap_before_parent_score" {
				head = appendSoleDerivationNode(c, 2, linkRecord{prev: head.Node, payload: 55, scoreDelta: math.MaxInt64})
			}
			_, _, err := compareSoleDerivation(t, c, head)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

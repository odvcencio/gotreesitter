package parsercorephase0

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestSoleDerivationSharedPathsAndScoreExtrema(t *testing.T) {
	for _, saturated := range []bool{false, true} {
		t.Run(fmt.Sprintf("saturated_%t", saturated), func(t *testing.T) {
			c := newSoleDerivationCore()
			shared := appendSoleDerivationNode(c, 2,
				linkRecord{prev: 1, payload: 11, scoreDelta: -9},
				linkRecord{prev: 1, payload: 22, scoreDelta: 7})
			left := appendSoleDerivationNode(c, 2,
				linkRecord{prev: shared.Node, payload: 33, scoreDelta: 2})
			right := appendSoleDerivationNode(c, 2,
				linkRecord{prev: shared.Node, payload: 44, scoreDelta: -3})
			head := appendSoleDerivationNode(c, 6,
				linkRecord{prev: left.Node, payload: 55, scoreDelta: 1},
				linkRecord{prev: right.Node, payload: 66, scoreDelta: 4},
				linkRecord{prev: shared.Node, payload: math.MaxUint32, scoreDelta: 10})
			if saturated {
				for index := 1; index < len(c.nodes); index++ {
					c.nodes[index].pathCount = math.MaxUint64
				}
			}
			paths, err := c.Derivations(head)
			if err != nil {
				t.Fatal(err)
			}
			var scores []int64
			for _, path := range paths {
				scores = append(scores, path.Score)
			}
			if !reflect.DeepEqual(scores, []int64{-6, 10, -8, 8, 1, 17}) {
				t.Fatalf("shared paths have scores %v", scores)
			}
			for _, cap := range []uint64{0, 1, 2, 5, 6, 7, math.MaxUint64} {
				t.Run(fmt.Sprintf("cap_%d", cap), func(t *testing.T) {
					c.limits.MaxDerivations = cap
					_, sole, err := compareSoleDerivation(t, c, head)
					if cap >= 6 && (err != nil || sole) {
						t.Fatalf("shared graph: sole=%t error=%v", sole, err)
					}
				})
			}
		})
	}
}

func TestSoleDerivationSummaryPreservesErrorOrder(t *testing.T) {
	for _, name := range []string{
		"prefix_overflow_before_crossed_cap", "crossed_cap_before_prefix_overflow",
		"positive_cancellation_overflow", "negative_cancellation_overflow",
		"earlier_edge_overflow_before_later_cycle", "parent_adjacency_before_child_overflow",
	} {
		t.Run(name, func(t *testing.T) {
			c := newSoleDerivationCore()
			var head Head
			want := "score overflow"
			switch name {
			case "prefix_overflow_before_crossed_cap", "crossed_cap_before_prefix_overflow":
				first, second := int64(math.MaxInt64), int64(0)
				if name == "crossed_cap_before_prefix_overflow" {
					first, second = second, first
					want = "derivation enumeration cap"
				}
				prefix := appendSoleDerivationNode(c, 2,
					linkRecord{prev: 1, payload: 11, scoreDelta: first},
					linkRecord{prev: 1, payload: 22, scoreDelta: second})
				head = appendSoleDerivationNode(c, 3,
					linkRecord{prev: 1, payload: 33},
					linkRecord{prev: prefix.Node, payload: 44, scoreDelta: 1})
				c.limits.MaxDerivations = 2
			case "positive_cancellation_overflow", "negative_cancellation_overflow":
				limit, delta := int64(math.MaxInt64), int64(1)
				if name == "negative_cancellation_overflow" {
					limit, delta = math.MinInt64, -1
				}
				prefix := appendSoleDerivationNode(c, 2,
					linkRecord{prev: 1, payload: 11, scoreDelta: limit},
					linkRecord{prev: 1, payload: 22})
				overflow := appendSoleDerivationNode(c, 2,
					linkRecord{prev: prefix.Node, payload: 33, scoreDelta: delta})
				head = appendSoleDerivationNode(c, 2,
					linkRecord{prev: overflow.Node, payload: 44, scoreDelta: -delta})
			case "earlier_edge_overflow_before_later_cycle", "parent_adjacency_before_child_overflow":
				prefix := appendSoleDerivationNode(c, 2,
					linkRecord{prev: 1, payload: 11, scoreDelta: math.MaxInt64},
					linkRecord{prev: 1, payload: 22})
				cycle := appendSoleDerivationNode(c, 1, linkRecord{prev: 3, payload: 33})
				head = appendSoleDerivationNode(c, 3,
					linkRecord{prev: prefix.Node, payload: 44, scoreDelta: 1},
					linkRecord{prev: cycle.Node, payload: 55})
				if name == "parent_adjacency_before_child_overflow" {
					c.links[c.nodes[head.Node-1].firstLink-1].flags = math.MaxUint32
					want = "link has unknown flags"
				}
			}
			_, _, err := compareSoleDerivation(t, c, head)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
		})
	}
}

func TestSoleDerivationSummaryPreservesPublishedGraphValidation(t *testing.T) {
	for _, name := range []string{
		"forward_shared_edges", "lowered_link_limits", "saturated_child_and_head_single_path",
		"saturated_head_zero_cap", "finite_child_count_mismatch_after_shared_path",
		"discontinuity_with_score", "discontinuity_with_order",
	} {
		t.Run(name, func(t *testing.T) {
			c := newSoleDerivationCore()
			c.nodes[0].firstLink = math.MaxUint32
			var head Head
			want := ""
			switch name {
			case "forward_shared_edges":
				head = appendSoleDerivationNode(c, 2,
					linkRecord{prev: 3, payload: 11}, linkRecord{prev: 3, payload: 22})
				appendSoleDerivationNode(c, 1, linkRecord{prev: 1, payload: 33})
			case "lowered_link_limits":
				head = appendSoleDerivationNode(c, 2,
					linkRecord{prev: 1, payload: 11}, linkRecord{prev: 1, payload: 22})
				c.limits.MaxLinks = 1
				c.limits.MaxLinksPerBoundary = 1
			case "saturated_child_and_head_single_path", "saturated_head_zero_cap":
				child := appendSoleDerivationNode(c, math.MaxUint64,
					linkRecord{prev: 1, payload: 11, scoreDelta: 7, flags: linkFlagHasOrder, order: 9})
				head = appendSoleDerivationNode(c, math.MaxUint64,
					linkRecord{prev: child.Node, payload: 22, scoreDelta: -2})
				if name == "saturated_head_zero_cap" {
					c.limits.MaxDerivations = 0
					want = "derivation enumeration cap"
				}
			case "finite_child_count_mismatch_after_shared_path":
				shared := appendSoleDerivationNode(c, 1, linkRecord{prev: 1, payload: 11})
				bad := appendSoleDerivationNode(c, 2, linkRecord{prev: shared.Node, payload: 22})
				head = appendSoleDerivationNode(c, 3,
					linkRecord{prev: shared.Node, payload: 33}, linkRecord{prev: bad.Node, payload: 44})
				want = "path-count mismatch: enumerated 1, recorded 2"
			case "discontinuity_with_score", "discontinuity_with_order":
				link := linkRecord{prev: 1, flags: linkFlagRecoveryDiscontinuity}
				if name == "discontinuity_with_score" {
					link.scoreDelta = 1
					want = "recovery discontinuity has a score delta"
				} else {
					link.flags |= linkFlagHasOrder
					link.order = 7
					want = "recovery discontinuity has branch order"
				}
				head = appendSoleDerivationNode(c, 2, linkRecord{prev: 1, payload: 11}, link)
			}
			_, _, err := compareSoleDerivation(t, c, head)
			if want == "" && err != nil {
				t.Fatal(err)
			}
			if want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
				t.Fatalf("error=%v, want %q", err, want)
			}
		})
	}
}

func TestSoleDerivationSummaryHasNoPersistentState(t *testing.T) {
	c := newTinyCoreWithLimits(t, Limits{MaxDerivations: 64, MaxPopPaths: 64})
	var previousHead Head
	for generation := 0; generation < 3; generation++ {
		if generation != 0 {
			if err := c.Reset(); err != nil {
				t.Fatal(err)
			}
		}
		seed, err := c.Seed(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		child := appendSoleDerivationNode(c, 1,
			linkRecord{prev: seed.Node, payload: SubtreeID(10 + generation), scoreDelta: int64(generation)})
		head := appendSoleDerivationNode(c, 2,
			linkRecord{prev: child.Node, payload: 20}, linkRecord{prev: seed.Node, payload: 30})
		if generation != 0 && head != previousHead {
			t.Fatalf("reset did not reuse the head ID: old=%v new=%v", previousHead, head)
		}
		previousHead = head
		c.work = Work{Shifts: 7, Reductions: 11, Overflow: true}
		beforeWork := c.Work()
		beforeNodes := append([]nodeRecord(nil), c.nodes...)
		beforeLinks := append([]linkRecord(nil), c.links...)
		for repeat := 0; repeat < 2; repeat++ {
			compareSoleDerivation(t, c, head)
		}
		if c.Work() != beforeWork || !reflect.DeepEqual(c.nodes, beforeNodes) || !reflect.DeepEqual(c.links, beforeLinks) {
			t.Fatal("derivation reads changed the graph or work counters")
		}
		// Reuse the same head after changing its graph and then repairing it.
		last := c.nodes[head.Node-1].firstLink - 1
		saved := c.links[last]
		c.links[last].prev = head.Node
		_, _, err = compareSoleDerivation(t, c, head)
		if err == nil || !strings.Contains(err.Error(), "graph cycle") {
			t.Fatalf("mutated graph error=%v", err)
		}
		c.links[last] = saved
		compareSoleDerivation(t, c, head)
		c.limits.MaxDerivations = 1
		compareSoleDerivation(t, c, head)
		c.limits.MaxDerivations = 64
		compareSoleDerivation(t, c, head)
		if c.Work() != beforeWork {
			t.Fatal("repeated reads changed work counters")
		}
	}
}

func TestSoleDerivationSummaryDifferentialDAGs(t *testing.T) {
	for seed := int64(0); seed < 16; seed++ {
		for _, cap := range []uint64{0, 1, 2, 16, 64} {
			t.Run(fmt.Sprintf("seed_%d/cap_%d", seed, cap), func(t *testing.T) {
				random := rand.New(rand.NewSource(seed))
				c := newSoleDerivationCore()
				c.limits.MaxDerivations = cap
				counts := []uint64{1}
				deltas := []int64{-7, -1, 0, 1, 9}
				if seed%3 == 0 {
					deltas = append(deltas, math.MaxInt64, math.MinInt64)
				}
				for node := 0; node < 8; node++ {
					var links []linkRecord
					var count uint64
					for edge, edges := 0, 1+random.Intn(3); edge < edges; edge++ {
						previous := random.Intn(len(counts))
						count += counts[previous]
						link := linkRecord{
							prev: NodeID(previous + 1), payload: SubtreeID(100 + node*3 + edge),
							scoreDelta: deltas[random.Intn(len(deltas))],
						}
						if random.Intn(4) == 0 {
							link.payload, link.flags = 0, linkFlagRecoveryDiscontinuity
							link.scoreDelta = 0
						}
						if random.Intn(3) == 0 && !link.isRecoveryDiscontinuity() {
							link.flags |= linkFlagHasOrder
							link.order = random.Uint64()
						}
						links = append(links, link)
					}
					counts = append(counts, count)
					recorded := count
					if random.Intn(3) == 0 {
						recorded = math.MaxUint64
					}
					head := appendSoleDerivationNode(c, recorded, links...)
					t.Run(fmt.Sprintf("head_%d", head.Node), func(t *testing.T) {
						compareSoleDerivation(t, c, head)
					})
				}
			})
		}
	}
}

func TestSoleDerivationSummaryScratchGrowth(t *testing.T) {
	c := newSoleDerivationCore()
	before := c.FootprintBytes()
	previous := Head{Node: 1}
	for index := 0; index < 40; index++ {
		previous = appendSoleDerivationNode(c, 1, linkRecord{
			prev: previous.Node, payload: SubtreeID(100 + index), scoreDelta: int64(index%3) - 1,
		})
	}
	head := appendSoleDerivationNode(c, 2,
		linkRecord{prev: previous.Node, payload: 201},
		linkRecord{prev: 1, payload: 202})
	compareSoleDerivation(t, c, head)
	if scratch := &c.derivationSummaryScratch; scratch.core != nil || len(scratch.nodes) != 0 || len(scratch.nodeIndexes) != 0 || len(scratch.links) != 0 {
		t.Fatalf("summary retained logical state: %+v", scratch)
	}
	if got := c.FootprintBytes(); got <= before || c.derivationSummaryScratch.footprintBytes() == 0 {
		t.Fatalf("summary scratch footprint=%d total=%d before=%d", c.derivationSummaryScratch.footprintBytes(), got, before)
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if c.derivationSummaryScratch.footprintBytes() == 0 {
		t.Fatal("reset dropped reusable summary scratch")
	}
	c.derivationSummaryScratch.dropOversized()
	if got := c.derivationSummaryScratch.footprintBytes(); got != 0 {
		t.Fatalf("released summary scratch footprint=%d", got)
	}
}

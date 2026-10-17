package parsercorephase0

import (
	"strings"
	"testing"
)

func sharedTerminalValidationLeaf(t *testing.T, c *Core, start, end CheckpointID, external bool) SubtreeID {
	t.Helper()
	if err := c.SetPhaseExternalTokenScannerCheckpoints(start, end); err != nil {
		t.Fatal(err)
	}
	id, err := c.appendAuthenticatedTerminal(subtreeRecord{symbol: 10, terminal: true, external: external, startByte: 0, endByte: 1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func sharedTerminalValidationParent(t *testing.T, c *Core, children ...SubtreeID) SubtreeID {
	t.Helper()
	id, err := c.appendSubtree(subtreeRecord{symbol: 11, startByte: 0, endByte: 1}, children, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func sharedTerminalValidationCorrupt(c *Core, id SubtreeID, mode string) {
	for i := range c.externalProvenance {
		if c.externalProvenance[i].payload != id {
			continue
		}
		if mode == "missing" {
			copy(c.externalProvenance[i:], c.externalProvenance[i+1:])
			c.externalProvenance = c.externalProvenance[:len(c.externalProvenance)-1]
		} else if mode == "unavailable" {
			c.externalProvenance[i].end = ^CheckpointID(0)
		}
		return
	}
}

func TestSharedTerminalValidationRequiresEverySharedProof(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "unavailable", "ordinary", "opaque", "missing leaf"} {
		t.Run(mode, func(t *testing.T) {
			c := newTinyCoreWithLimits(t, Limits{})
			a := mustInternCheckpoint(t, c, []byte{1})
			b := mustInternCheckpoint(t, c, []byte{2})
			first := sharedTerminalValidationLeaf(t, c, a, b, true)
			tail := sharedTerminalValidationLeaf(t, c, a, b, mode != "ordinary")
			sharedTerminalValidationCorrupt(c, tail, mode)
			if mode == "opaque" {
				c.subtrees[tail-1].externalProvenanceState = subtreeExternalProvenanceReusedOpaque
			}
			if mode == "missing leaf" {
				c.subtrees[tail-1].missing = true
			}
			left := sharedTerminalValidationParent(t, c, first, tail)
			right := sharedTerminalValidationParent(t, c, first, tail)
			got, err := c.reductionTerminalHistoriesEqual(left, right)
			if mode == "unavailable" {
				if err == nil || !strings.Contains(err.Error(), "checkpoint is unavailable") {
					t.Fatalf("shared invalid checkpoint accepted: equal=%t err=%v", got, err)
				}
			} else if mode == "opaque" {
				if err == nil || !strings.Contains(err.Error(), "reused nonterminal provenance is stale") {
					t.Fatalf("invalid opaque terminal accepted: equal=%t err=%v", got, err)
				}
			} else if err != nil || got != (mode == "valid") {
				t.Fatalf("equal=%t err=%v", got, err)
			}
		})
	}
}

func TestSharedTerminalValidationKeepsDistinctChecks(t *testing.T) {
	for _, mode := range []string{"equal", "different scanner", "missing", "unavailable", "different prefix"} {
		t.Run(mode, func(t *testing.T) {
			c := newTinyCoreWithLimits(t, Limits{})
			a := mustInternCheckpoint(t, c, []byte{1})
			b := mustInternCheckpoint(t, c, []byte{2})
			other := mustInternCheckpoint(t, c, []byte{3})
			leftLeaf := sharedTerminalValidationLeaf(t, c, a, b, true)
			end := b
			if mode == "different scanner" {
				end = other
			}
			rightLeaf := sharedTerminalValidationLeaf(t, c, a, end, true)
			sharedTerminalValidationCorrupt(c, rightLeaf, mode)
			if mode == "different prefix" {
				c.lexerSkippedPrefixes = append(c.lexerSkippedPrefixes, lexerSkippedPrefixProvenance{payload: rightLeaf, start: 0})
			}
			left := sharedTerminalValidationParent(t, c, leftLeaf)
			right := sharedTerminalValidationParent(t, c, rightLeaf)
			got, err := c.reductionTerminalHistoriesEqual(left, right)
			if mode == "unavailable" {
				if err == nil || !strings.Contains(err.Error(), "checkpoint is unavailable") {
					t.Fatalf("distinct invalid checkpoint accepted: equal=%t err=%v", got, err)
				}
			} else if err != nil || got != (mode == "equal") {
				t.Fatalf("equal=%t err=%v", got, err)
			}
		})
	}
}

func TestSharedTerminalValidationPreservesLeftRejectionOrder(t *testing.T) {
	for _, mode := range []string{"ordinary", "missing", "unavailable", "valid"} {
		t.Run(mode, func(t *testing.T) {
			c := newTinyCoreWithLimits(t, Limits{})
			a := mustInternCheckpoint(t, c, []byte{1})
			b := mustInternCheckpoint(t, c, []byte{2})
			leftLeaf := sharedTerminalValidationLeaf(t, c, a, b, mode != "ordinary")
			rightLeaf := sharedTerminalValidationLeaf(t, c, a, b, true)
			sharedTerminalValidationCorrupt(c, leftLeaf, mode)
			left := sharedTerminalValidationParent(t, c, leftLeaf)
			right := sharedTerminalValidationParent(t, c, rightLeaf)
			c.children[c.subtrees[right-1].firstChild] = right
			equal, err := c.reductionTerminalHistoriesEqual(left, right)
			switch mode {
			case "ordinary", "missing":
				if equal || err != nil {
					t.Fatalf("right error replaced left decline: equal=%t err=%v", equal, err)
				}
			case "unavailable":
				if err == nil || !strings.Contains(err.Error(), "checkpoint is unavailable") {
					t.Fatalf("right error replaced left scanner error: %v", err)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), "child identifier is out of order") {
					t.Fatalf("right traversal skipped: %v", err)
				}
			}
		})
	}
}

// The two histories visit fourteen nodes, but their shared arena gives a
// budget of thirteen. A direct right-leaf pop must still spend its visit.
func TestSharedTerminalValidationBudgetUsesBothCursors(t *testing.T) {
	c := newTinyCoreWithLimits(t, Limits{})
	a := mustInternCheckpoint(t, c, []byte{1})
	b := mustInternCheckpoint(t, c, []byte{2})
	leaf := sharedTerminalValidationLeaf(t, c, a, b, true)
	chunk := sharedTerminalValidationParent(t, c, leaf, leaf)
	left := sharedTerminalValidationParent(t, c, chunk, chunk)
	right := sharedTerminalValidationParent(t, c, chunk, chunk)
	for i := 0; i < 3; i++ {
		sharedTerminalValidationParent(t, c)
	}
	if budget := len(c.subtrees) + len(c.children); budget != 13 {
		t.Fatalf("fixture budget=%d", budget)
	}
	equal, err := c.reductionTerminalHistoriesEqual(left, right)
	if equal || err != nil {
		t.Fatalf("budget exceeded without rejection: equal=%t err=%v", equal, err)
	}
}

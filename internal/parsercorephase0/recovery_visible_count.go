package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
)

// recoveryVisibleCount stores an unaliased count without the root ERROR_REPEAT rule.
// Aliases affect only the current root. Descendant aliases are already included.
type recoveryVisibleCount struct {
	count uint32
	flags uint8
}

const (
	recoveryVisibleCountValid uint8 = 1 << iota
	recoveryVisibleCountRootVisible
	recoveryVisibleCountExtra
	recoveryVisibleCountErrorRepeat
)

// CachedVisibleSubtreeCount counts visible occurrences in one compact subtree.
// It preserves occurrence-specific visibility and caches immutable subtree totals.
// The visibility policy is copied and checked before each call. Named bits do not affect counts.
func (c *Core) CachedVisibleSubtreeCount(symbols []SelectedSymbolPolicy, id SubtreeID) (uint32, error) {
	if id == 0 {
		return 0, nil
	}
	if c == nil {
		return 0, errors.New("parser-core phase zero: recovery visible-node count on nil core")
	}
	if uint64(id) > uint64(len(c.subtrees)) {
		return 0, fmt.Errorf("parser-core phase zero: invalid subtree id %d", id)
	}
	c.prepareRecoveryVisibleCounts(symbols)
	return c.recoveryVisibleSubtreeCount(id, false, true)
}

// SoleDerivationVisibleNodeCount returns the visible payload count only when
// head has one exact derivation. The certified path does not allocate payload storage.
func (c *Core) SoleDerivationVisibleNodeCount(symbols []SelectedSymbolPolicy, head Head) (uint32, bool, error) {
	n, err := c.node(head.Node)
	if err != nil {
		return 0, false, err
	}
	if n.pathCount == 1 {
		reverseLinks, exact, err := c.appendSingleDerivationLinks(head.Node, make([]LinkID, 0, 64))
		if err != nil {
			return 0, false, err
		}
		if exact {
			var score int64
			for reverseIndex := len(reverseLinks) - 1; reverseIndex >= 0; reverseIndex-- {
				link := c.links[reverseLinks[reverseIndex]-1]
				score, err = checkedAddScore(score, link.scoreDelta)
				if err != nil {
					return 0, false, err
				}
			}

			c.prepareRecoveryVisibleCounts(symbols)
			var total uint32
			for reverseIndex := len(reverseLinks) - 1; reverseIndex >= 0; reverseIndex-- {
				payload := c.links[reverseLinks[reverseIndex]-1].payload
				if payload == 0 {
					continue
				}
				count, err := c.recoveryVisibleSubtreeCount(payload, false, true)
				if err != nil {
					return 0, true, err
				}
				if math.MaxUint32-total < count {
					return 0, true, errors.New("parser-core phase zero: recovery visible-node count overflow")
				}
				total += count
			}
			return total, true, nil
		}
	}

	derivation, sole, err := c.SoleDerivation(head)
	if err != nil || !sole {
		return 0, sole, err
	}
	c.prepareRecoveryVisibleCounts(symbols)
	var total uint32
	for _, payload := range derivation.Payloads {
		count, err := c.recoveryVisibleSubtreeCount(payload, false, true)
		if err != nil {
			return 0, true, err
		}
		if math.MaxUint32-total < count {
			return 0, true, errors.New("parser-core phase zero: recovery visible-node count overflow")
		}
		total += count
	}
	return total, true, nil
}

func (c *Core) prepareRecoveryVisibleCounts(symbols []SelectedSymbolPolicy) {
	c.bindRecoveryVisibleSymbols(symbols)
	if len(c.recoveryVisibleCounts) < len(c.subtrees) {
		c.recoveryVisibleCounts = append(c.recoveryVisibleCounts,
			make([]recoveryVisibleCount, len(c.subtrees)-len(c.recoveryVisibleCounts))...)
	}
}

func (c *Core) bindRecoveryVisibleSymbols(symbols []SelectedSymbolPolicy) {
	// Symbol is uint16. Unaddressable policy entries cannot affect a count.
	width := min(len(symbols), math.MaxUint16+1)
	equal := len(c.recoveryVisibleSymbols) == width
	if equal {
		for index := 0; index < width; index++ {
			if c.recoveryVisibleSymbols[index] != symbols[index].Visible {
				equal = false
				break
			}
		}
	}
	if equal {
		return
	}
	clear(c.recoveryVisibleCounts)
	if cap(c.recoveryVisibleSymbols) < width {
		c.recoveryVisibleSymbols = make([]bool, width)
	} else {
		c.recoveryVisibleSymbols = c.recoveryVisibleSymbols[:width]
	}
	for index := range c.recoveryVisibleSymbols {
		c.recoveryVisibleSymbols[index] = symbols[index].Visible
	}
}

func (c *Core) recoveryVisibleSubtreeCount(id SubtreeID, hasAlias, countErrorRepeat bool) (uint32, error) {
	if id == 0 {
		return 0, nil
	}
	entry := c.recoveryVisibleCounts[id-1]
	if entry.flags&recoveryVisibleCountValid == 0 {
		record := c.subtrees[id-1]
		childEnd := uint64(record.firstChild) + uint64(record.childCount)
		aliasEnd := uint64(record.firstAlias) + uint64(record.aliasCount)
		if childEnd > uint64(len(c.children)) || aliasEnd > uint64(len(c.aliases)) {
			return 0, errors.New("parser-core phase zero: recovery visible-node metadata is outside the arena")
		}
		if record.aliasCount != 0 && record.aliasCount != record.childCount {
			return 0, fmt.Errorf("parser-core phase zero: recovery cost node %d has %d aliases for %d children",
				id, record.aliasCount, record.childCount)
		}
		entry.flags = recoveryVisibleCountValid
		if record.symbol == ErrorRegionSymbol ||
			int(record.symbol) < len(c.recoveryVisibleSymbols) && c.recoveryVisibleSymbols[record.symbol] {
			entry.count = 1
			entry.flags |= recoveryVisibleCountRootVisible
		}
		if record.extra {
			entry.flags |= recoveryVisibleCountExtra
		}
		// ERROR_REPEAT precedes ERROR in the built-in symbol range.
		if record.symbol == ErrorRegionSymbol-1 {
			entry.flags |= recoveryVisibleCountErrorRepeat
		}
		children := c.children[record.firstChild:childEnd]
		aliases := c.aliases[record.firstAlias:aliasEnd]
		for index, childID := range children {
			// Published children precede their parent. This also rejects cycles.
			if childID >= id {
				return 0, errors.New("parser-core phase zero: recovery visible-node child is not a predecessor")
			}
			childAlias := len(aliases) != 0 && aliases[index] != 0
			childCount, err := c.recoveryVisibleSubtreeCount(childID, childAlias, false)
			if err != nil {
				return 0, err
			}
			if math.MaxUint32-entry.count < childCount {
				return 0, errors.New("parser-core phase zero: recovery visible-node count overflow")
			}
			entry.count += childCount
		}
		c.recoveryVisibleCounts[id-1] = entry
	}
	if entry.flags&recoveryVisibleCountRootVisible == 0 &&
		(hasAlias && entry.flags&recoveryVisibleCountExtra == 0 ||
			countErrorRepeat && entry.flags&recoveryVisibleCountErrorRepeat != 0) {
		if entry.count == math.MaxUint32 {
			return 0, errors.New("parser-core phase zero: recovery visible-node count overflow")
		}
		return entry.count + 1, nil
	}
	return entry.count, nil
}

func (c *Core) truncateRecoveryVisibleCounts(subtrees int) {
	if subtrees < len(c.recoveryVisibleCounts) {
		clear(c.recoveryVisibleCounts[subtrees:])
		c.recoveryVisibleCounts = c.recoveryVisibleCounts[:subtrees]
	}
}

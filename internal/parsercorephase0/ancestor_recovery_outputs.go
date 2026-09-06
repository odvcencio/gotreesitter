package parsercorephase0

import "errors"

// ancestorRecoveryMaxIterators matches stack.c's live iterator limit.
const ancestorRecoveryMaxIterators = 64

// RecoverToAncestorStateOutputsWithCostOwned publishes each distinct target
// version in C pop order. Repeated slices for one target retain the first slice.
func (c *Core) RecoverToAncestorStateOutputsWithCostOwned(owner SchedulerTransactionToken, candidate StackSummaryCandidate, cost ReductionOutputCostFunc) ([]Head, error) {
	return c.recoverAncestorOutputsOwned(owner, candidate, cost, nil)
}

// RecoverToAncestorStateOutputsWithOpenRegionAndCostOwned adds the absorbed
// children to each recovered version without changing the input region.
func (c *Core) RecoverToAncestorStateOutputsWithOpenRegionAndCostOwned(owner SchedulerTransactionToken, candidate StackSummaryCandidate, startByte, endByte uint32, children []SubtreeID, cost ReductionOutputCostFunc) ([]Head, error) {
	return c.recoverAncestorOutputsOwned(owner, candidate, cost, &ancestorRecoveryOpenRegion{startByte: startByte, endByte: endByte, children: children})
}

func (c *Core) recoverAncestorOutputsOwned(owner SchedulerTransactionToken, candidate StackSummaryCandidate, cost ReductionOutputCostFunc, region *ancestorRecoveryOpenRegion) (outputs []Head, err error) {
	err = c.RunSchedulerOwned(owner, func() error {
		if candidate.owner != c || candidate.generation == 0 || candidate.generation != c.classificationPhase ||
			candidate.source == 0 || candidate.linkDepth == 0 || candidate.depth > StackSummaryMaxDepth || cost == nil {
			return errors.New("parser-core phase zero: invalid plural recovery candidate or cost")
		}
		source, err := c.node(candidate.source)
		if err != nil {
			return err
		}
		if region != nil {
			if len(region.children) == 0 || region.endByte < region.startByte || region.startByte < source.byteOffset || uint64(len(region.children)) > uint64(c.limits.MaxChildren) {
				return errors.New("parser-core phase zero: invalid plural recovery region")
			}
			previous := region.startByte
			for _, id := range region.children {
				child, err := c.subtree(id)
				if err != nil {
					return err
				}
				if child.startByte < previous || child.endByte < child.startByte || child.endByte > region.endByte {
					return errors.New("parser-core phase zero: invalid plural recovery child span")
				}
				previous = child.endByte
			}
		}
		paths, err := c.ancestorRecoveryPaths(candidate)
		if err != nil {
			return err
		}
		for _, path := range paths {
			target, err := c.node(path.target)
			if err != nil {
				return err
			}
			entry := candidate
			entry.byteOffset = target.byteOffset
			head, err := c.publishAncestorRecoveryPath(entry, cost, region, path.links, path.target, true)
			if err != nil {
				return err
			}
			outputs = append(outputs, head)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

type ancestorRecoveryPath struct {
	target NodeID
	links  []linkRecord
}

// ancestorRecoveryPaths advances existing iterators once per round. Branches
// start in the next round, matching stack.c rather than depth-first traversal.
func (c *Core) ancestorRecoveryPaths(candidate StackSummaryCandidate) ([]ancestorRecoveryPath, error) {
	type iterator struct {
		node  NodeID
		depth int
		links []linkRecord
	}
	iterators := []iterator{{node: candidate.source}}
	seen := make(map[NodeID]bool)
	var paths []ancestorRecoveryPath
	var popped uint64
	steps := 0
	for len(iterators) != 0 {
		for i, size := 0, len(iterators); i < size; {
			current := iterators[i]
			node, err := c.node(current.node)
			if err != nil {
				return nil, err
			}
			if current.depth == int(candidate.depth) || node.linkCount == 0 {
				if current.depth == int(candidate.depth) {
					popped++
					if popped > c.limits.MaxPopPaths {
						return nil, errors.New("parser-core phase zero: ancestor recovery pop enumeration cap")
					}
					if node.state == candidate.state && !seen[current.node] {
						seen[current.node] = true
						paths = append(paths, ancestorRecoveryPath{target: current.node, links: current.links})
					}
				}
				copy(iterators[i:], iterators[i+1:])
				iterators[len(iterators)-1] = iterator{}
				iterators = iterators[:len(iterators)-1]
				size--
				continue
			}
			var inline [inlineAdjacencyCapacity]linkRecord
			links, err := c.publishedNodeLinksInto(inline[:0], *node)
			if err != nil {
				return nil, err
			}
			for j := 1; j <= len(links); j++ {
				// C preserves link zero in the current slot. It suppresses
				// additional branches before copying their traversal state.
				if j != len(links) && len(iterators) >= ancestorRecoveryMaxIterators {
					continue
				}
				link := links[j%len(links)]
				if err := link.validateShape(); err != nil {
					return nil, err
				}
				if link.prev == 0 || link.prev >= current.node {
					return nil, errors.New("parser-core phase zero: ancestor recovery predecessor does not decrease")
				}
				next := iterator{node: link.prev, depth: current.depth, links: append(append([]linkRecord(nil), current.links...), link)}
				if link.isRecoveryDiscontinuity() {
					next.depth++
				} else {
					payload, err := c.subtree(link.payload)
					if err != nil {
						return nil, err
					}
					if !payload.extra {
						next.depth++
					}
				}
				steps++
				if steps > stackSummaryMaxVisitedNodes*StackSummaryMaxDepth || len(next.links) >= stackSummaryMaxVisitedNodes {
					return nil, errors.New("parser-core phase zero: ancestor recovery traversal cap")
				}
				if j == len(links) {
					iterators[i] = next
				} else {
					iterators = append(iterators, next)
				}
			}
			i++
		}
	}
	return paths, nil
}

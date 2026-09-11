package parsercorephase0

import "errors"

// RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned publishes every pop_all
// slice in C order. The region can be empty before the first absorption.
func (c *Core) RecoverEOFAcceptOutputsWithOpenRegionAndCostOwned(owner SchedulerTransactionToken, head Head, regionStart, regionEnd uint32, children []SubtreeID, cost ReductionOutputCostFunc) (outputs []Head, err error) {
	err = c.RunSchedulerOwned(owner, func() error {
		source, err := c.node(head.Node)
		if err != nil {
			return err
		}
		if cost == nil || source.byteOffset > regionStart || regionStart > regionEnd || len(children) > EOFAdmissionMaxTopPayloads {
			return errors.New("parser-core phase zero: invalid plural EOF region or cost")
		}
		checkpoint, ok := c.nodeScannerCheckpoint(head.Node)
		if !ok {
			return errors.New("parser-core phase zero: plural EOF source checkpoint is unavailable")
		}
		previousRegionEnd := regionStart
		for _, id := range children {
			record, err := c.subtree(id)
			if err != nil {
				return err
			}
			if record.startByte < previousRegionEnd || record.startByte > record.endByte || record.endByte > regionEnd {
				return errors.New("parser-core phase zero: plural EOF region child span is invalid")
			}
			previousRegionEnd = record.endByte
		}
		paths, err := c.recoveryPopAllPaths(head)
		if err != nil {
			return err
		}
		if len(children) != 0 {
			repeat, err := c.RecoveryErrorRepeatOwned(owner, children)
			if err != nil {
				return err
			}
			children = []SubtreeID{repeat}
		}
		for _, path := range paths {
			payloads := make([]SubtreeID, 0, len(path.links)+len(children))
			var score int64
			var order ForkOrder
			for i := len(path.links) - 1; i >= 0; i-- {
				link := path.links[i]
				if link.hasOrder() {
					order = ForkOrder{Present: true, Value: link.order}
				}
				score, err = checkedAddScore(score, link.scoreDelta)
				if err != nil {
					return err
				}
				if !link.isRecoveryDiscontinuity() {
					payloads = append(payloads, link.payload)
				}
			}
			payloads = append(payloads, children...)
			if len(payloads) > EOFAdmissionMaxTopPayloads {
				return errors.New("parser-core phase zero: plural EOF payload cap")
			}
			start := regionStart
			if len(payloads) != 0 {
				first, err := c.subtree(payloads[0])
				if err != nil {
					return err
				}
				start = first.startByte
			}
			previous := start
			for _, id := range payloads {
				record, err := c.subtree(id)
				if err != nil {
					return err
				}
				if record.startByte < previous || record.startByte > record.endByte || record.endByte > regionEnd {
					return errors.New("parser-core phase zero: plural EOF child span is invalid")
				}
				_, exact, err := c.subtreeExternalProvenance(id)
				if err != nil {
					return err
				}
				if !exact {
					return errors.New("parser-core phase zero: plural EOF child scanner provenance is inexact")
				}
				previous = record.endByte
			}
			out, root, err := c.publishRecoverEOFAcceptAt(payloads, start, regionEnd, score, order, cost, checkpoint)
			if err != nil {
				return err
			}
			stored, err := c.RecoveryStoredErrorCost(out)
			if err != nil {
				return err
			}
			if err := c.copyRecoveryDiscontinuityLineage(head.Node, out.Node); err != nil {
				return err
			}
			if err := c.publishInheritedStoredErrorCost(out, stored); err != nil {
				return err
			}
			if _, err := c.MaterializationOrder([]SubtreeID{root}, nil); err != nil {
				return err
			}
			outputs = append(outputs, out)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

// recoveryPopAllPaths uses C's live iterator bound and grouped slice order.
func (c *Core) recoveryPopAllPaths(head Head) ([]ancestorRecoveryPath, error) {
	iterators := []ancestorRecoveryPath{{target: head.Node}}
	var paths []ancestorRecoveryPath
	steps := 0
	for len(iterators) != 0 {
		for i, size := 0, len(iterators); i < size; {
			current := iterators[i]
			node, err := c.node(current.target)
			if err != nil {
				return nil, err
			}
			if node.linkCount == 0 {
				if uint64(len(paths)) >= c.limits.MaxPopPaths {
					return nil, errors.New("parser-core phase zero: plural EOF pop path cap")
				}
				insert := len(paths)
				for j := len(paths) - 1; j >= 0; j-- {
					if paths[j].target == current.target {
						insert = j + 1
						break
					}
				}
				paths = append(paths, ancestorRecoveryPath{})
				copy(paths[insert+1:], paths[insert:])
				paths[insert] = current
				copy(iterators[i:], iterators[i+1:])
				iterators[len(iterators)-1] = ancestorRecoveryPath{}
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
				if j != len(links) && len(iterators) >= ancestorRecoveryMaxIterators {
					continue
				}
				link := links[j%len(links)]
				if err := link.validateShape(); err != nil {
					return nil, err
				}
				if link.prev == 0 || link.prev >= current.target {
					return nil, errors.New("parser-core phase zero: plural EOF predecessor does not decrease")
				}
				next := ancestorRecoveryPath{target: link.prev, links: append(append([]linkRecord(nil), current.links...), link)}
				steps++
				if steps > stackSummaryMaxVisitedNodes*StackSummaryMaxDepth || len(next.links) > EOFAdmissionMaxTopPayloads {
					return nil, errors.New("parser-core phase zero: plural EOF traversal cap")
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

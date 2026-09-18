package parsercorephase0

import "errors"

// RecoveryErrorRepeatSymbol identifies C's hidden recovery history container.
const RecoveryErrorRepeatSymbol Symbol = ErrorRegionSymbol - 1

// PushRecoveryErrorRepeatOwned publishes an open history before a new pause
// episode. The output retains the source scanner checkpoint and graph paths.
func (c *Core) PushRecoveryErrorRepeatOwned(owner SchedulerTransactionToken, head Head, children []SubtreeID, cost ReductionOutputCostFunc) (out Head, err error) {
	err = c.RunSchedulerOwned(owner, func() error {
		node, err := c.node(head.Node)
		if err != nil {
			return err
		}
		checkpoint, ok := c.nodeScannerCheckpoint(head.Node)
		if !ok || node.state != 0 || cost == nil {
			return errors.New("parser-core phase zero: invalid recovery repeat source")
		}
		position := node.byteOffset
		repeat, err := c.RecoveryErrorRepeatOwned(owner, children)
		if err != nil {
			return err
		}
		record, err := c.subtree(repeat)
		if err != nil {
			return err
		}
		if record.startByte < position {
			return errors.New("parser-core phase zero: recovery repeat precedes source")
		}
		stored, err := cost(head.Node, repeat)
		if err != nil {
			return err
		}
		id, err := c.appendAdjacencyNodeAtWithPrecedenceAndStoredErrorCost(0, record.endByte, checkpoint, []linkRecord{{prev: head.Node, payload: repeat}}, precedenceMaximumWitness{}, stored, true)
		if err != nil {
			return err
		}
		out = Head{Node: id}
		if err := c.copyRecoveryDiscontinuityLineage(head.Node, id); err != nil {
			return err
		}
		return c.publishInheritedStoredErrorCost(out, stored)
	})
	if err != nil {
		return Head{}, err
	}
	return out, nil
}

// RecoveryErrorRepeatOwned retains C's hidden absorption history. Each token
// gets a singleton container. Further tokens join the preceding history.
func (c *Core) RecoveryErrorRepeatOwned(owner SchedulerTransactionToken, children []SubtreeID) (root SubtreeID, err error) {
	err = c.RunSchedulerOwned(owner, func() error {
		if len(children) == 0 {
			return errors.New("parser-core phase zero: empty recovery repeat")
		}
		var start, previous uint32
		for i, child := range children {
			record, err := c.subtree(child)
			if err != nil {
				return err
			}
			if record.startByte > record.endByte || (i != 0 && record.startByte < previous) {
				return errors.New("parser-core phase zero: unordered recovery repeat children")
			}
			if _, exact, err := c.subtreeExternalProvenance(child); err != nil {
				return err
			} else if !exact {
				return errors.New("parser-core phase zero: inexact recovery repeat scanner provenance")
			}
			end := record.endByte
			single, err := c.appendSubtree(subtreeRecord{symbol: RecoveryErrorRepeatSymbol, startByte: record.startByte, endByte: end}, []SubtreeID{child}, nil, nil)
			if err != nil {
				return err
			}
			if i == 0 {
				root, start = single, record.startByte
			} else {
				root, err = c.appendSubtree(subtreeRecord{symbol: RecoveryErrorRepeatSymbol, startByte: start, endByte: end}, []SubtreeID{root, single}, nil, nil)
				if err != nil {
					return err
				}
			}
			previous = end
		}
		_, err := c.MaterializationOrder([]SubtreeID{root}, nil)
		return err
	})
	if err != nil {
		return 0, err
	}
	return root, nil
}

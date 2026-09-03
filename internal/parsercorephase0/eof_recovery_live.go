package parsercorephase0

import (
	"errors"
	"fmt"
	"slices"
)

// RecoverEOFAcceptOwned replaces one authenticated, exact singleton stack
// path with the non-extra ERROR root that tree-sitter publishes from
// recover_eof. originalPayloads authenticate the exact path and become the
// children of the new root. The caller must keep the returned head and root
// together.
//
// The mutation runs through the scheduler-owner seam. A caller that needs a
// rollback must place this operation inside ApplySchedulerAtomic; a fresh
// scheduler session rolls back the complete Core when its owner returns an
// error. The root provenance sidecar lets generic materialization authenticate
// this synthetic production without growing subtreeRecord.
func (c *Core) RecoverEOFAcceptOwned(
	owner SchedulerTransactionToken,
	head Head,
	payloads []SubtreeID,
	startByte uint32,
	endByte uint32,
) (out Head, root SubtreeID, err error) {
	if c == nil {
		return Head{}, 0, errors.New("parser-core phase zero: recover_eof accept on nil core")
	}
	err = c.RunSchedulerOwned(owner, func() error {
		var operationErr error
		out, root, operationErr = c.recoverEOFAcceptUncheckpointed(
			head, payloads, startByte, endByte,
		)
		return operationErr
	})
	if err != nil {
		return Head{}, 0, err
	}
	return out, root, nil
}

// RecoverEOFAcceptWithCostOwned publishes the synthetic ERROR root with its
// authenticated cumulative cost. The root replaces the old path, so its cost
// does not inherit the predecessor path cost.
func (c *Core) RecoverEOFAcceptWithCostOwned(
	owner SchedulerTransactionToken,
	head Head,
	payloads []SubtreeID,
	startByte uint32,
	endByte uint32,
	cost ReductionOutputCostFunc,
) (out Head, root SubtreeID, err error) {
	if c == nil {
		return Head{}, 0, errors.New("parser-core phase zero: recover_eof accept on nil core")
	}
	err = c.RunSchedulerOwned(owner, func() error {
		if cost == nil {
			return errors.New("parser-core phase zero: recover_eof cost callback is required")
		}
		var operationErr error
		out, root, operationErr = c.recoverEOFAcceptUncheckpointedWithCost(
			head, payloads, startByte, endByte, cost,
		)
		return operationErr
	})
	if err != nil {
		return Head{}, 0, err
	}
	return out, root, nil
}

func (c *Core) recoverEOFAcceptUncheckpointed(
	head Head,
	payloads []SubtreeID,
	startByte uint32,
	endByte uint32,
) (Head, SubtreeID, error) {
	return c.recoverEOFAcceptUncheckpointedWithCost(head, payloads, startByte, endByte, nil)
}

func (c *Core) recoverEOFAcceptUncheckpointedWithCost(
	head Head,
	payloads []SubtreeID,
	startByte uint32,
	endByte uint32,
	cost ReductionOutputCostFunc,
) (Head, SubtreeID, error) {
	if head.Node == 0 {
		return Head{}, 0, errors.New("parser-core phase zero: recover_eof accept requires a head")
	}
	if len(payloads) == 0 || len(payloads) > EOFAdmissionMaxTopPayloads {
		return Head{}, 0, fmt.Errorf(
			"parser-core phase zero: recover_eof accept payload count %d is outside 1..%d",
			len(payloads), EOFAdmissionMaxTopPayloads,
		)
	}
	if startByte > endByte {
		return Head{}, 0, errors.New("parser-core phase zero: recover_eof accept span is reversed")
	}
	if _, err := c.node(head.Node); err != nil {
		return Head{}, 0, err
	}
	paths, err := c.Derivations(head)
	if err != nil {
		return Head{}, 0, err
	}
	if len(paths) != 1 || !slices.Equal(paths[0].Payloads, payloads) {
		return Head{}, 0, errors.New("parser-core phase zero: recover_eof accept requires one exact singleton path")
	}
	if err := c.validateRecoverEOFAcceptPayloads(payloads, startByte, endByte); err != nil {
		return Head{}, 0, err
	}

	root, err := c.appendSubtree(subtreeRecord{
		symbol:    ErrorRegionSymbol,
		startByte: startByte,
		endByte:   endByte,
	}, payloads, nil, nil)
	if err != nil {
		return Head{}, 0, err
	}
	// Mark the synthetic root only after its record is complete. The marker is
	// length-journaled by Core checkpoints and therefore rolls back with it.
	c.eofRecoveryRoots = append(c.eofRecoveryRoots, root)

	// The recovered root replaces the old stack path. Build a private empty
	// predecessor so the new head has exactly one payload and no old siblings.
	base, err := c.appendNodeAt(nodeRecord{
		state: 1, byteOffset: endByte, pathCount: 1,
	}, c.checkpoint)
	if err != nil {
		return Head{}, 0, err
	}
	var storedErrorCost uint32
	if cost != nil {
		computed, costErr := cost(base, root)
		if costErr != nil {
			return Head{}, 0, costErr
		}
		storedErrorCost, costErr = c.storedErrorCostForLink(linkInput{
			prev: base, payload: root, storedErrorCost: computed, hasStoredErrorCost: true,
		})
		if costErr != nil {
			return Head{}, 0, costErr
		}
	}
	linkID := c.appendGraphLink(linkRecord{prev: base, payload: root})
	c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
	newNode, err := c.appendNodeAt(nodeRecord{
		state: 1, byteOffset: endByte,
		firstLink: uint32(linkID), linkCount: 1, pathCount: 1,
	}, c.checkpoint)
	if err != nil {
		return Head{}, 0, err
	}
	if cost != nil {
		if err := c.publishInheritedStoredErrorCost(Head{Node: newNode}, storedErrorCost); err != nil {
			return Head{}, 0, err
		}
	}
	return Head{Node: newNode}, root, nil
}

func (c *Core) validateRecoverEOFAcceptPayloads(payloads []SubtreeID, startByte, endByte uint32) error {
	previousEnd := startByte
	for index, payload := range payloads {
		record, err := c.subtree(payload)
		if err != nil {
			return err
		}
		if record.extra {
			return fmt.Errorf("parser-core phase zero: recover_eof accept payload %d is trailing extra", index)
		}
		if record.missing {
			return fmt.Errorf("parser-core phase zero: recover_eof accept payload %d is missing", index)
		}
		_, exact, provenanceErr := c.subtreeExternalProvenance(payload)
		if provenanceErr != nil {
			return provenanceErr
		}
		if !exact {
			return fmt.Errorf("parser-core phase zero: recover_eof accept payload %d has inexact external provenance", index)
		}
		if record.startByte > record.endByte || record.startByte < startByte || record.endByte > endByte {
			return fmt.Errorf("parser-core phase zero: recover_eof accept payload %d is outside root span", index)
		}
		if index != 0 && record.startByte < previousEnd {
			return fmt.Errorf("parser-core phase zero: recover_eof accept payload %d overlaps its predecessor", index)
		}
		previousEnd = record.endByte
	}
	return nil
}

func (c *Core) isRecoverEOFAcceptRoot(id SubtreeID) bool {
	for _, root := range c.eofRecoveryRoots {
		if root == id {
			return true
		}
	}
	return false
}

// IsRecoverEOFAcceptRoot reports whether id is the synthetic root published
// by RecoverEOFAcceptOwned. The marker is provenance, not a grammar symbol:
// ordinary ERROR subtrees must retain normal materialization checks.
func (c *Core) IsRecoverEOFAcceptRoot(id SubtreeID) bool {
	return c != nil && c.isRecoverEOFAcceptRoot(id)
}

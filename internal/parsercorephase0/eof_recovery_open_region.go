package parsercorephase0

import "errors"

// RecoverEOFAcceptWithOpenRegionAndCostOwned includes the live error-repeat
// contents in C's final ERROR. It does not publish a nested ERROR region.
// The caller supplies only absorbed children, not independent stack extras.
func (c *Core) RecoverEOFAcceptWithOpenRegionAndCostOwned(
	owner SchedulerTransactionToken,
	head Head,
	regionStart, regionEnd uint32,
	children []SubtreeID,
	cost ReductionOutputCostFunc,
) (out Head, root SubtreeID, err error) {
	if c == nil {
		return Head{}, 0, errors.New("parser-core phase zero: open-region EOF accept on nil core")
	}
	err = c.RunSchedulerOwned(owner, func() error {
		if cost == nil || len(children) == 0 || regionStart >= regionEnd {
			return errors.New("parser-core phase zero: open-region EOF accept requires children, span, and cost")
		}
		paths, pathErr := c.Derivations(head)
		if pathErr != nil {
			return pathErr
		}
		if len(paths) != 1 || len(paths[0].Payloads)+len(children) > EOFAdmissionMaxTopPayloads {
			return errors.New("parser-core phase zero: open-region EOF accept requires one bounded path")
		}
		payloads := make([]SubtreeID, 0, len(paths[0].Payloads)+len(children))
		payloads = append(payloads, paths[0].Payloads...)
		payloads = append(payloads, children...)
		first, viewErr := c.subtree(payloads[0])
		if viewErr != nil {
			return viewErr
		}
		// Keep the initial route exact and scanner-independent. The existing
		// validator rejects missing nodes, extras, and unknown scanner state.
		if err := c.validateRecoverEOFAcceptPayloads(payloads, first.startByte, regionEnd); err != nil {
			return err
		}
		if err := c.validateRecoverEOFAcceptPayloads(children, regionStart, regionEnd); err != nil {
			return err
		}
		_, position, boundaryErr := c.Boundary(head)
		if boundaryErr != nil {
			return boundaryErr
		}
		if position > regionStart {
			return errors.New("parser-core phase zero: open-region EOF contents overlap the stack")
		}
		var publishErr error
		out, root, publishErr = c.publishRecoverEOFAcceptUncheckpointed(payloads, first.startByte, regionEnd, paths[0].Score, cost)
		return publishErr
	})
	if err != nil {
		return Head{}, 0, err
	}
	return out, root, nil
}

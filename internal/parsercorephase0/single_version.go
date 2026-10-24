package parsercorephase0

import "errors"

// SingleVersionRows supplies already-compiled immutable singleton rows. A miss
// hands the unchanged action back to the general scheduler. It must not spend
// logical action-lookup work on a miss, matching corridor probe semantics.
type SingleVersionRows interface {
	SingleVersionRow(StateID, Symbol) (ActionRow, bool)
}

// SingleVersionObserver preserves stop polling and lexical/reuse provenance at
// each operation. It does not own reduction, graph or lineage publication.
type SingleVersionObserver interface {
	BeforeSingleVersion(ActionType) error
	AfterSingleVersion(Head, ActionType, bool) error
}

type SingleVersionResult struct {
	Head    Head
	Steps   uint32
	Shifted bool
	// A failed reduction probe may already have resolved its GOTO. The general
	// reducer consumes this exact classified cell instead of repeating that read.
	ReductionFallback bool
	Boundary          ClassifiedBoundary
}

// RunSingleVersionOwned runs one elected token through deterministic reductions
// and its final shift inside one scheduler operation. It uses the native graph
// throughout: there is no private stack, graph import, replay journal or flush.
// Only new boundaries are published here; ambiguity and historical boundaries
// leave the next action intact for the general scheduler.
func (c *Core) RunSingleVersionOwned(owner SchedulerTransactionToken, head Head, token Token, publication ShiftHeadLineage, maximum uint32, rows SingleVersionRows, observer SingleVersionObserver) (result SingleVersionResult, err error) {
	result.Head = head
	if err = c.beginSchedulerOwned(owner); err != nil {
		return result, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	defer func() { err = c.finishSchedulerOwned(owner, err) }()
	if !c.schedulerFrame.fresh || phase0AEnabled || c.condenseScopeActive || c.popScratch.busy || c.reduceConflictContext || c.reduceNoLookaheadContext {
		return result, nil
	}
	if rows == nil || observer == nil || publication.Owner == 0 {
		return result, errors.New("parser-core phase zero: incomplete single-version operation")
	}
	for result.Steps < maximum {
		state, _, e := c.Boundary(result.Head)
		if e != nil {
			return result, e
		}
		row, ok := rows.SingleVersionRow(state, token.Symbol)
		if !ok || row.Len() != 1 {
			return result, nil
		}
		action := row.At(0)
		if action.Type != ActionReduce && (action.Type != ActionShift || action.Extra) {
			return result, nil
		}
		// An external shift is owned here only with an authenticated election.
		// Leave an unbound token to the existing scheduler/publication path.
		if action.Type == ActionShift && token.External && !c.externalTokenScannerExact {
			return result, nil
		}
		if e = observer.BeforeSingleVersion(action.Type); e != nil {
			return result, e
		}
		if e = c.validateSchedulerTransaction(owner); e != nil {
			return result, e
		}
		var next Head
		switch action.Type {
		case ActionReduce:
			boundary, e := c.ClassifyBoundaryWithRow(result.Head, token.Symbol, row)
			if e != nil {
				return result, e
			}
			next, ok, e = c.reduceDirectActive(owner, &boundary, publication)
			if e != nil {
				return result, e
			}
			if !ok {
				result.ReductionFallback = true
				result.Boundary = boundary
				return result, nil
			}
		case ActionShift:
			// Prove a new boundary before allocating a terminal. A collision keeps
			// all publication and canonicalization with the existing shift path.
			key := c.shiftedBoundaryKey(action.State, token.EndByte)
			if probe, _ := c.boundaries.probe(boundaryIdentityFromKey(key)); probe.found {
				return result, nil
			}
			if e = c.enterLiveCondenseCandidates(nil); e != nil {
				return result, e
			}
			next, e = c.shiftDirectUncheckpointed(result.Head, token.Symbol, action.State, false, token, ForkOrder{})
			c.clearLiveCondenseCandidates()
			if e != nil {
				return result, e
			}
			if !c.lastShiftFresh {
				return result, errors.New("parser-core phase zero: single-version shift lost new boundary")
			}
			if e = c.recordHeadOwner(next, publication.Owner); e != nil {
				return result, e
			}
			if publication.PublishLineage {
				if e = c.publishShiftLineage(next, publication); e != nil {
					return result, e
				}
			}
			result.Shifted = true
		}
		result.Head = next
		result.Steps++
		if publication.PublishLineage {
			publication.PreviousHead = next
			publication.SetDirty = false
		}
		if e = observer.AfterSingleVersion(next, action.Type, result.Shifted); e != nil {
			return result, e
		}
		if result.Shifted {
			return result, nil
		}
	}
	return result, nil
}

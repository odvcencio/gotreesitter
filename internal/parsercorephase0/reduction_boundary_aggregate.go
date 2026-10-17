package parsercorephase0

import "math"

// aggregateReductionBoundary keeps the general provenance workspace off the
// sole fresh-pop path. It reads the same live predecessor/history records and
// authenticates imported certificates before publishing aggregation state.
func (c *Core) aggregateReductionBoundary(
	owner SchedulerTransactionToken,
	boundary *ClassifiedBoundary,
	actionOrdinal int,
	act *Action,
	path *popPath,
	key boundaryKey,
	out Head,
	outcome *condenseOutcome,
) error {
	// expectedHistoricalAction is read only under historical certificate
	// authentication; build it on that branch instead of on every reduce.
	expectedHistoricalAction := func() DropCohortActionIdentity {
		return DropCohortActionIdentity{
			BoundaryState: boundary.state,
			Lookahead:     boundary.lookahead,
			ActionOrdinal: int32(actionOrdinal),
			Action:        *act,
			NoLookahead:   c.reduceNoLookaheadContext,
			Selection:     c.dropCohortSelectionContext,
		}
	}
	scratch := &c.reductionScratch
	boundaryIndex, seen := scratch.boundary(key)
	var previousStore reductionBoundaryOutput
	previous := &previousStore
	if seen {
		previous = &scratch.boundaries[boundaryIndex]
	}
	freshness := previous.freshness
	cleanPathRank := mergeCleanPathRank(previous.cleanPathRank, path.cleanPathRank)
	historicalBoundarySplit := previous.historicalBoundarySplit || outcome.historicalBoundarySplit
	historicalConvergedSplit := previous.historicalConvergedSplit
	historicalForestDeterministic := previous.historicalForestDeterministic
	historicalCleanPathRank := previous.historicalCleanPathRank
	historicalLineage := previous.historicalLineage
	historicalSet := previous.historicalSet
	historicalBlended := previous.historicalBlended
	dropCohortRefs := previous.dropCohortRefs
	if source, refsErr := c.lineageView(path.prev); refsErr == nil {
		if source.dropCohortRefIndex != 0 {
			refs, refsErr := c.nodeLineageRefs(source.dropCohortRefIndex)
			if refsErr != nil {
				return refsErr
			}
			if _, refsErr = c.dropCohortRefUnion(&dropCohortRefs, refs); refsErr != nil {
				return refsErr
			}
		}
	} else {
		return refsErr
	}
	if outcome.historicalBoundarySplit {
		switch {
		case !previous.historicalBoundarySplit:
			historicalConvergedSplit = outcome.historicalConvergedSplit
			historicalForestDeterministic = outcome.historicalForestDeterministic
			historicalCleanPathRank = outcome.historicalCleanPathRank
			historicalLineage = outcome.historicalLineage
		case !historicalConvergedSplit && outcome.historicalConvergedSplit:
			historicalConvergedSplit = true
			historicalCleanPathRank = outcome.historicalCleanPathRank
			historicalLineage = outcome.historicalLineage
		case historicalConvergedSplit && !outcome.historicalConvergedSplit:
			// Keep the exact provenance from the converged historical version.
		case historicalCleanPathRank != outcome.historicalCleanPathRank ||
			historicalLineage != outcome.historicalLineage:
			historicalCleanPathRank = CleanPathRankUnknown
			historicalLineage = 0
		}
		if previous.historicalBoundarySplit {
			historicalForestDeterministic =
				historicalForestDeterministic && outcome.historicalForestDeterministic
		}
		// Membership is never invalidated: where the scalar pair above may
		// poison to Unknown/0 on disagreement between multiple historical
		// versions landing on the same boundary, the alternative set instead
		// unions every converged historical version's recorded set
		// (spec.b4b-alternative-set.v1 section 4, "Dead-node historical
		// import"). The dead node's lineage record persists for the rest of
		// the parse. Fold-class union (spec.b4b-alternative-set.v2 section
		// 3.4): two differing dead sets unioning into one output, or a dead
		// record that was itself blended, mark this boundary's accumulated
		// historicalSet blended -- computed before the union mutates it.
		if outcome.historicalConvergedSplit {
			// Condensation retains the dead node's identity; its lineage is
			// immutable through publication of the replacement. Resolve the
			// reference set here, alongside the alternative set, so fresh
			// boundaries never carry a copy of this cold provenance.
			dead, deadErr := c.lineageView(outcome.historicalNode)
			var deadRefs DropCohortRefSet
			if deadErr == nil {
				deadRefs, deadErr = c.nodeLineageRefs(dead.dropCohortRefIndex)
			}
			if !c.historicalCertificateAuthentication {
				// Preserve the pre-D2 historical union and counter behavior.
				if deadErr == nil {
					incomparable := c.AlternativeSetIncomparable(historicalSet, dead.set)
					unionChanged := c.alternativeSetUnion(&historicalSet, dead.set)
					wasBlended := historicalBlended
					historicalBlended = historicalBlended || dead.isBlended() || incomparable
					if unionChanged || historicalBlended != wasBlended {
						c.addDropCohortProducerWrite(dropCohortProducerDeadHistoryImport)
						if c.dropCohortAuthenticatedHistory != math.MaxUint64 {
							c.dropCohortAuthenticatedHistory++
						}
					}
					if _, refsErr := c.dropCohortRefUnion(&dropCohortRefs, deadRefs); refsErr != nil {
						return refsErr
					}
				}
			} else {
				markUnproved := func() {
					historicalConvergedSplit = false
					historicalForestDeterministic = false
					historicalCleanPathRank = CleanPathRankUnknown
					historicalLineage = 0
					historicalSet = AlternativeSet{}
					historicalBlended = false
					// Do not retain source or historical certificate refs on an
					// unproved result.
					dropCohortRefs = DropCohortRefSet{}
					if c.dropCohortUnprovedHistory != math.MaxUint64 {
						c.dropCohortUnprovedHistory++
					}
				}
				if deadErr != nil || !c.authenticateHistoricalDropCohortImport(
					owner, deadRefs, outcome.historicalNode, expectedHistoricalAction(),
				) {
					markUnproved()
				} else {
					c.addDropCohortProducerWrite(dropCohortProducerDeadHistoryImport)
					if c.dropCohortAuthenticatedHistory != math.MaxUint64 {
						c.dropCohortAuthenticatedHistory++
					}
					incomparable := c.AlternativeSetIncomparable(historicalSet, dead.set)
					historicalBlended = historicalBlended || dead.isBlended() || incomparable
					c.alternativeSetUnion(&historicalSet, dead.set)
					if _, refsErr := c.dropCohortRefUnion(&dropCohortRefs, deadRefs); refsErr != nil {
						return refsErr
					}
				}
			}
		}
	}
	switch outcome.change {
	case condenseUnchanged:
		if !seen {
			freshness = ReductionUnchanged
		}
	case condenseNew:
		freshness = ReductionNew
	case condenseUpdated:
		if freshness != ReductionNew {
			freshness = ReductionUpdated
		}
	}
	linkChain, err := c.reductionLinkChainForHead(out)
	if err != nil {
		return err
	}
	scratch.store(boundaryIndex, seen, &reductionBoundaryOutput{
		key: key, head: out, links: linkChain, freshness: freshness, cleanPathRank: cleanPathRank,
		dropCohortRefs:                dropCohortRefs,
		historicalBoundarySplit:       historicalBoundarySplit,
		historicalConvergedSplit:      historicalConvergedSplit,
		historicalForestDeterministic: historicalForestDeterministic,
		historicalCleanPathRank:       historicalCleanPathRank,
		historicalLineage:             historicalLineage,
		historicalSet:                 historicalSet,
		historicalBlended:             historicalBlended,
	})
	return nil
}

package parsercorephase0

import "errors"

// ---------------------------------------------------------------------------
// recovery_election.go -- campaign v7 tranche B3 stage S4, first coherent
// sub-unit: a single-path strategy-1 election over compact headers.
//
// THE PRODUCTION GO PORT IS THE EXECUTABLE SPECIFICATION for the mechanism
// (spec.compact-recovery-ownership.v1, section 6): parser_recover_c.go's
// cRecoverStrategy1Election, itself a faithful port of tree-sitter C's
// ts_parser__recover strategy-1 summary scan. The pinned C oracle is the
// parity arbiter (decision 0007).
//
// Scope of THIS sub-unit only (design section 4's own escape hatch: "If the
// next non-green stage is too large for one pass ... implement its first
// coherent sub-unit"): the pure ELECTION DECISION -- which ancestor state,
// if any, a depth-major scan would resume to -- over the single-path chain
// a compact head already publishes. It does not fork, does not materialize
// an ERROR wrap, and is not called from any dispatch path. It is dead code
// by construction, exactly like recovery_cost.go's own stage-S2 landing
// (that file's doc comment; design section 9, open question 8's "land
// inert" default, generalized here because the fork/materialization half of
// S4 is not yet built). TestRecoveryElectionNoOutsideCallSitesRatchet
// enforces this the same way TestRecoveryCostNoOutsideCallSitesRatchet does
// for recovery_cost.go.
//
// Two deliberate restrictions keep this sub-unit's proof bounded, both
// documented on ElectRecoveryTarget itself:
//
//  1. Single incoming link only. cRecoverStrategy1Election scans a MERGED
//     C-version summary that can carry more than one path to the same
//     (depth, state) pair; production dedups those at first encounter. A
//     compact node can likewise carry more than one incoming link from
//     shallow path-merging condensation alone (AncestorStateWithActionExists'
//     own doc comment). Resolving which of several incoming paths supplies
//     the correct pop-and-wrap chain for an eventual fork is genuine
//     multi-path reasoning this sub-unit does not attempt -- it declines
//     (ok=false) the instant it meets a node with more than one incoming
//     link, mirroring the single-deterministic-path restriction
//     s3CloseInProgressProductions (parsercore_phase0_driver.go) already
//     applies to S3's own closure walk. A future sub-unit that widens past
//     one path must widen this guard deliberately, not by deleting it.
//
//  2. No cross-head cost competition. cBetterVersionExists compares the
//     candidate's hypothetical cost against every OTHER live GLR version at
//     the same or later position, aborting the whole scan when a cheaper
//     sibling exists. The compact shape this sub-unit targets --
//     dispatchPassActive's "len(s.headers) == 1 && len(noActionIndices) ==
//     1" guard, the same guard s3TryOpenErrorRegion is scoped to -- has no
//     sibling head to compare against by construction, so there is nothing
//     for a cost check to abort in favor of. Computing RecoveryErrorStatus
//     here anyway would be a cost-model invocation with no decision effect,
//     which is exactly what G2/G3 (design section 8) forbid computing on a
//     path that does not need it. A future extension enabling multi-head
//     strategy-1 competition must reintroduce the cost-before-lookahead-
//     validity ordering using RecoveryCompareVersions/RecoveryVersionStatus
//     (recovery_cost.go) at that point, not before.
//
// Depth-major order and (depth, state) dedup (parity obligation 4, design
// section 6) hold trivially in this restricted shape: a single-path walk
// visits exactly one candidate per depth, so there is only ever one state
// to consider at each depth and nothing to dedup against a sibling.
// ---------------------------------------------------------------------------

// ElectedRecoveryTarget names one strategy-1 resume candidate a single-path
// depth-major scan elected: the ancestor's own depth from head (depth 1 is
// head's direct predecessor, matching AncestorStateWithActionExists), its
// state and byte offset, and the ordered chain of already-published subtree
// payloads a fork would need to wrap in one ERROR container to discard
// everything strategy 1 pops between the ancestor and head.
//
// Popped is in walk order, nearest-head-first: Popped[0] is the payload
// attached immediately below head (the most recently attached one), and
// Popped[len(Popped)-1] is the payload attached immediately above the
// elected ancestor (the least recently attached one, closest to it in
// source position). A future fork-building stage that wraps these into one
// ERROR node's children needs them in source-byte order (oldest first),
// which is Popped read BACKWARD -- this type deliberately does not reverse
// it itself, leaving that to the caller that actually builds the ERROR
// container, so a pure election never has an opinion about materialization
// order.
type ElectedRecoveryTarget struct {
	Depth      int
	State      StateID
	ByteOffset uint32
	Ancestor   NodeID
	Popped     []SubtreeID
}

// ElectRecoveryTarget walks head's sole predecessor chain, one link per
// step, up to maxDepth steps back, and returns the shallowest ancestor
// whose state has a genuine table action for lookahead -- the compact
// analogue of cRecoverStrategy1Election's depth-major merged-summary scan,
// restricted to the single-path shape this sub-unit owns (see the file doc
// comment for why). ok is false when maxDepth <= 0 (the loop bound below
// never runs), when the walk reaches a node with zero or more than one
// incoming link before finding an accepting ancestor, or when no ancestor
// within maxDepth accepts lookahead -- every one of those is a decline, not
// a guess: the caller falls back to the existing fail-closed recovery
// boundary unchanged.
func (c *Core) ElectRecoveryTarget(head Head, lookahead Symbol, maxDepth int) (target ElectedRecoveryTarget, ok bool, err error) {
	current, err := c.node(head.Node)
	if err != nil {
		return ElectedRecoveryTarget{}, false, err
	}
	var popped []SubtreeID
	for depth := 1; depth <= maxDepth; depth++ {
		if current.linkCount != 1 {
			// Zero: reached the stack base, nothing left to elect. More
			// than one: a genuine multi-path branch this sub-unit declines
			// rather than guesses at (file doc comment, restriction 1).
			return ElectedRecoveryTarget{}, false, nil
		}
		linkID := current.firstLink
		if linkID == 0 || uint64(linkID) > uint64(len(c.links)) {
			return ElectedRecoveryTarget{}, false, errors.New("parser-core phase zero: recovery election adjacency out of range")
		}
		link := c.links[linkID-1]
		if link.prev == 0 {
			return ElectedRecoveryTarget{}, false, nil
		}
		popped = append(popped, link.payload)
		ancestor, err := c.node(link.prev)
		if err != nil {
			return ElectedRecoveryTarget{}, false, err
		}
		row, err := c.tables.Actions(ancestor.state, lookahead)
		if err != nil {
			return ElectedRecoveryTarget{}, false, err
		}
		if row.Len() > 0 {
			return ElectedRecoveryTarget{
				Depth:      depth,
				State:      ancestor.state,
				ByteOffset: ancestor.byteOffset,
				Ancestor:   link.prev,
				Popped:     popped,
			}, true, nil
		}
		current = ancestor
	}
	return ElectedRecoveryTarget{}, false, nil
}

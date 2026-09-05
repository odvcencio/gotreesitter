package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
)

// StackSummaryMaxDepth matches tree-sitter's MAX_SUMMARY_DEPTH. Compact S4
// crosses at most this many non-extra payloads. Extra payloads do not consume
// summary depth and stay bounded by stackSummaryMaxVisitedNodes.
const StackSummaryMaxDepth = 16

const stackSummaryMaxVisitedNodes = 4096

var (
	errAncestorRecoveryCandidateAmbiguous = errors.New("parser-core phase zero: ancestor recovery candidate has ambiguous pop paths")
	errAncestorRecoveryCandidateNoPath    = errors.New("parser-core phase zero: stack-summary candidate has no exact pop path")
	errAncestorRecoveryCandidateLinkDepth = errors.New("parser-core phase zero: stale stack-summary candidate link depth")
)

// StackSummaryCandidate authenticates one ordered predecessor summary entry.
// Depth counts non-extra payloads, exactly like tree-sitter's stack summary.
// The scheduler performs cost comparison before it looks up an action.
//
// The fields stay private because owner, generation, and source identity are
// one capability. RecoverToAncestorStateOwned rejects copied candidates after
// rollback or reset before it reads a reusable arena ID.
type StackSummaryCandidate struct {
	owner      *Core
	generation uint64
	source     NodeID
	state      StateID
	byteOffset uint32
	linkDepth  uint16
	depth      uint8
}

// Depth returns the C stack-summary depth of this candidate. Extra payloads do
// not increase this value.
func (c StackSummaryCandidate) Depth() int { return int(c.depth) }

// State returns the parser state recorded for this candidate.
func (c StackSummaryCandidate) State() StateID { return c.state }

// ByteOffset returns the source position recorded for this candidate.
func (c StackSummaryCandidate) ByteOffset() uint32 { return c.byteOffset }

type stackSummaryNodeKey struct {
	node  NodeID
	depth uint8
}

type stackSummaryStateKey struct {
	state StateID
	depth uint8
}

type stackSummaryWalkItem struct {
	node      NodeID
	linkDepth uint16
}

// StackSummaryCandidates enumerates every predecessor summary entry in
// depth-major and stable link-insertion order. It increments depth only for a
// non-extra payload and deduplicates metadata by (depth, state), matching C's
// stack summary. It does not look up actions. The caller must apply C's cost
// and live-version guards before it tests the entry's action row.
//
// Path deduplication does not authorize mutation. RecoverToAncestorStateOwned
// re-enumerates paths and requires exactly one match for the elected pair.
func (c *Core) StackSummaryCandidates(head Head, maxDepth int) ([]StackSummaryCandidate, error) {
	if maxDepth <= 0 {
		return nil, nil
	}
	if maxDepth > StackSummaryMaxDepth {
		return nil, fmt.Errorf("parser-core phase zero: stack-summary depth %d exceeds limit %d", maxDepth, StackSummaryMaxDepth)
	}
	if c == nil {
		return nil, errors.New("parser-core phase zero: stack-summary enumeration on nil core")
	}
	if _, err := c.node(head.Node); err != nil {
		return nil, err
	}

	var levels [StackSummaryMaxDepth + 1][]stackSummaryWalkItem
	levels[0] = append(levels[0], stackSummaryWalkItem{node: head.Node})
	visited := map[stackSummaryNodeKey]struct{}{{node: head.Node}: {}}
	seen := make(map[stackSummaryStateKey]struct{})
	var candidates []StackSummaryCandidate
	for depth := 0; depth <= maxDepth; depth++ {
		level := &levels[depth]
		for cursor := 0; cursor < len(*level); cursor++ {
			item := (*level)[cursor]
			node, err := c.node(item.node)
			if err != nil {
				return nil, err
			}
			if item.linkDepth != 0 {
				stateKey := stackSummaryStateKey{state: node.state, depth: uint8(depth)}
				if _, ok := seen[stateKey]; !ok {
					seen[stateKey] = struct{}{}
					candidates = append(candidates, StackSummaryCandidate{
						owner: c, generation: c.classificationPhase, source: head.Node,
						state: node.state, byteOffset: node.byteOffset,
						linkDepth: item.linkDepth, depth: uint8(depth),
					})
				}
			}
			var inline [inlineAdjacencyCapacity]linkRecord
			links, err := c.publishedNodeLinksInto(inline[:0], *node)
			if err != nil {
				return nil, err
			}
			for _, link := range links {
				if err := link.validateShape(); err != nil {
					return nil, err
				}
				if link.prev == 0 || link.prev >= item.node {
					return nil, errors.New("parser-core phase zero: stack-summary predecessor does not decrease")
				}
				nextDepth := depth
				if link.isRecoveryDiscontinuity() {
					if depth == maxDepth {
						continue
					}
					nextDepth++
				} else {
					payload, err := c.subtree(link.payload)
					if err != nil {
						return nil, err
					}
					if !payload.extra {
						if depth == maxDepth {
							continue
						}
						nextDepth++
					}
				}
				if item.linkDepth == math.MaxUint16 {
					return nil, errors.New("parser-core phase zero: stack-summary link depth overflow")
				}
				key := stackSummaryNodeKey{node: link.prev, depth: uint8(nextDepth)}
				if _, ok := visited[key]; ok {
					continue
				}
				visited[key] = struct{}{}
				if len(visited) > stackSummaryMaxVisitedNodes {
					return nil, errors.New("parser-core phase zero: stack-summary visited-node cap")
				}
				levels[nextDepth] = append(levels[nextDepth], stackSummaryWalkItem{
					node: link.prev, linkDepth: item.linkDepth + 1,
				})
			}
		}
	}
	return candidates, nil
}

// AncestorStateWithActionExists preserves the S3 compatibility probe. Keep
// its short-circuit and silent observation-cap behavior separate from the S4
// enumerator. Existing production callers depend on this diagnostic result.
func (c *Core) AncestorStateWithActionExists(head Head, lookahead Symbol, maxDepth int) (bool, error) {
	if maxDepth <= 0 {
		return false, nil
	}
	const maxVisitedNodes = 4096
	frontier := []NodeID{head.Node}
	visited := map[NodeID]bool{head.Node: true}
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		var next []NodeID
		for _, id := range frontier {
			node, err := c.node(id)
			if err != nil {
				return false, err
			}
			count := node.linkCount
			if count == 0 {
				continue
			}
			if uint64(count) > uint64(c.limits.MaxLinks) || uint64(count) > uint64(c.limits.MaxLinksPerBoundary) {
				return false, errors.New("parser-core phase zero: recorded link count exceeds configured limit")
			}
			linkID := LinkID(node.firstLink)
			for remaining := count; remaining > 0; remaining-- {
				if linkID == 0 || uint64(linkID) > uint64(len(c.links)) {
					return false, errors.New("parser-core phase zero: ancestor adjacency out of range")
				}
				link := c.links[linkID-1]
				if err := link.validateShape(); err != nil {
					return false, err
				}
				if link.prev == 0 || link.prev >= id {
					return false, errors.New("parser-core phase zero: ancestor predecessor does not decrease")
				}
				if link.prev != 0 && !visited[link.prev] {
					visited[link.prev] = true
					if len(visited) > maxVisitedNodes {
						return false, nil
					}
					next = append(next, link.prev)
				}
				linkID = link.next
			}
		}
		for _, id := range next {
			ancestor, err := c.node(id)
			if err != nil {
				return false, err
			}
			row, err := c.tables.Actions(ancestor.state, lookahead)
			if err != nil {
				return false, err
			}
			if row.Len() > 0 {
				return true, nil
			}
		}
		frontier = next
	}
	return false, nil
}

// RecoverToAncestorStateOwned performs the S4 stack mutation for one elected
// candidate. The scheduler token owns the surrounding atomic transaction.
// This method poisons that owner on every failure, so ignored errors cannot
// commit partial ERROR, child, link, node, or boundary-index publication.
func (c *Core) RecoverToAncestorStateOwned(owner SchedulerTransactionToken, candidate StackSummaryCandidate) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	out, err = c.recoverToAncestorStateUncheckpointed(candidate, nil)
	return out, c.finishSchedulerOwned(owner, err)
}

// RecoverToAncestorStateWithCostOwned publishes the complete cumulative cost
// of the ERROR link and each trailing extra link before boundary publication.
// The callback must return the authenticated cost for its complete output.
func (c *Core) RecoverToAncestorStateWithCostOwned(
	owner SchedulerTransactionToken,
	candidate StackSummaryCandidate,
	cost ReductionOutputCostFunc,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if cost == nil {
		err = errors.New("parser-core phase zero: ancestor recovery cost callback is required")
		return out, c.finishSchedulerOwned(owner, err)
	}
	out, err = c.recoverToAncestorStateUncheckpointed(candidate, cost)
	return out, c.finishSchedulerOwned(owner, err)
}

// RecoverToAncestorStateWithOpenRegionAndCostOwned folds absorbed region contents
// after the elected path's payloads into one extra ERROR. Children are region
// contents, not separate trailing stack extras. Their extra flags stay unchanged.
// Every failure poisons the surrounding scheduler transaction.
func (c *Core) RecoverToAncestorStateWithOpenRegionAndCostOwned(
	owner SchedulerTransactionToken, candidate StackSummaryCandidate,
	startByte, endByte uint32, children []SubtreeID, cost ReductionOutputCostFunc,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if cost == nil || len(children) == 0 || endByte < startByte {
		err = errors.New("parser-core phase zero: invalid open-region ancestor recovery")
		return out, c.finishSchedulerOwned(owner, err)
	}
	out, err = c.recoverToAncestorStateWithRegionUncheckpointed(candidate, cost, &ancestorRecoveryOpenRegion{
		startByte: startByte, endByte: endByte, children: children,
	})
	return out, c.finishSchedulerOwned(owner, err)
}

type ancestorRecoveryOpenRegion struct {
	startByte, endByte uint32
	children           []SubtreeID
}

// StackSummaryCandidateRecoverable reports whether one authenticated summary
// entry identifies exactly one mutable pop path. Summary deduplication can
// retain an observational entry that is not safe to mutate.
func (c *Core) StackSummaryCandidateRecoverable(candidate StackSummaryCandidate) (bool, error) {
	if c == nil || candidate.owner != c || candidate.generation == 0 ||
		candidate.generation != c.classificationPhase || candidate.source == 0 {
		return false, nil
	}
	_, _, err := c.uniqueAncestorRecoveryPath(candidate)
	if errors.Is(err, errAncestorRecoveryCandidateAmbiguous) ||
		errors.Is(err, errAncestorRecoveryCandidateNoPath) ||
		errors.Is(err, errAncestorRecoveryCandidateLinkDepth) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *Core) recoverToAncestorStateUncheckpointed(candidate StackSummaryCandidate, cost ReductionOutputCostFunc) (Head, error) {
	return c.recoverToAncestorStateWithRegionUncheckpointed(candidate, cost, nil)
}

func (c *Core) recoverToAncestorStateWithRegionUncheckpointed(candidate StackSummaryCandidate, cost ReductionOutputCostFunc, region *ancestorRecoveryOpenRegion) (Head, error) {
	if candidate.owner != c {
		return Head{}, errors.New("parser-core phase zero: stack-summary candidate belongs to a different core")
	}
	if candidate.generation == 0 || candidate.generation != c.classificationPhase {
		return Head{}, errors.New("parser-core phase zero: stale stack-summary candidate")
	}
	if candidate.source == 0 || candidate.linkDepth == 0 || candidate.depth > StackSummaryMaxDepth || candidate.linkDepth >= stackSummaryMaxVisitedNodes {
		return Head{}, errors.New("parser-core phase zero: invalid stack-summary candidate")
	}
	if _, err := c.node(candidate.source); err != nil {
		return Head{}, err
	}
	if region != nil {
		source, _ := c.node(candidate.source)
		if region.startByte < source.byteOffset || uint64(len(region.children)) > uint64(c.limits.MaxChildren) {
			return Head{}, errors.New("parser-core phase zero: open region precedes its source or exceeds child limit")
		}
		previous := region.startByte
		for _, id := range region.children {
			child, err := c.subtree(id)
			if err != nil {
				return Head{}, err
			}
			if child.startByte < previous || child.endByte < child.startByte || child.endByte > region.endByte {
				return Head{}, errors.New("parser-core phase zero: invalid open-region child span")
			}
			previous = child.endByte
		}
	}

	links, target, err := c.uniqueAncestorRecoveryPath(candidate)
	if err != nil {
		return Head{}, err
	}
	var priorChildren []SubtreeID
	var priorStart uint32
	var priorScore int64
	var priorOrder ForkOrder
	if region != nil {
		targetNode, err := c.node(target)
		if err != nil {
			return Head{}, err
		}
		var inline [inlineAdjacencyCapacity]linkRecord
		targetLinks, err := c.publishedNodeLinksInto(inline[:0], *targetNode)
		if err != nil {
			return Head{}, err
		}
		for _, link := range targetLinks {
			if err := link.validateShape(); err != nil {
				return Head{}, err
			}
			if link.isRecoveryDiscontinuity() {
				continue
			}
			prior, err := c.subtree(link.payload)
			if err != nil {
				return Head{}, err
			}
			if prior.symbol != ErrorRegionSymbol {
				continue
			}
			// C pops one immediately preceding ERROR before it constructs the
			// replacement. Ambiguous paths need a separate ownership proof.
			if len(targetLinks) != 1 || prior.missing || prior.childCount == 0 ||
				link.prev == 0 || link.prev >= target || prior.endByte > candidate.byteOffset ||
				prior.startByte > prior.endByte || uint64(prior.firstChild)+uint64(prior.childCount) > uint64(len(c.children)) {
				return Head{}, errors.New("parser-core phase zero: prior ERROR lacks a unique bounded pop proof")
			}
			predecessor, err := c.node(link.prev)
			if err != nil {
				return Head{}, err
			}
			if predecessor.byteOffset > prior.startByte {
				return Head{}, errors.New("parser-core phase zero: prior ERROR overlaps its predecessor")
			}
			priorChildren = c.children[prior.firstChild : prior.firstChild+prior.childCount]
			previous := prior.startByte
			for _, id := range priorChildren {
				child, err := c.subtree(id)
				if err != nil {
					return Head{}, err
				}
				if child.startByte < previous || child.endByte < child.startByte || child.endByte > prior.endByte {
					return Head{}, errors.New("parser-core phase zero: invalid prior ERROR child span")
				}
				previous = child.endByte
			}
			priorStart, priorScore = prior.startByte, link.scoreDelta
			if link.hasOrder() {
				priorOrder = ForkOrder{Present: true, Value: link.order}
			}
			target = link.prev
			break
		}
	}
	depth := len(links)
	trailing := 0
	for region == nil && trailing < depth {
		if links[trailing].isRecoveryDiscontinuity() {
			break
		}
		payload, err := c.subtree(links[trailing].payload)
		if err != nil {
			return Head{}, err
		}
		if !payload.extra {
			break
		}
		trailing++
	}
	if trailing == depth {
		return Head{}, errors.New("parser-core phase zero: ancestor recovery path has only trailing extras")
	}

	children := make([]SubtreeID, 0, depth-trailing)
	score, order := priorScore, priorOrder
	for index := depth - 1; index >= trailing; index-- {
		link := links[index]
		if link.isRecoveryDiscontinuity() {
			continue
		}
		children = append(children, link.payload)
		score, err = checkedAddScore(score, link.scoreDelta)
		if err != nil {
			return Head{}, err
		}
		if link.hasOrder() {
			order = ForkOrder{Present: true, Value: link.order}
		}
	}
	if region != nil {
		previous := candidate.byteOffset
		for _, id := range children {
			child, err := c.subtree(id)
			if err != nil {
				return Head{}, err
			}
			if child.startByte < previous || child.endByte < child.startByte || child.endByte > region.startByte {
				return Head{}, errors.New("parser-core phase zero: invalid popped open-region child span")
			}
			previous = child.endByte
		}
		if uint64(len(c.children))+uint64(len(priorChildren))+uint64(len(children))+uint64(len(region.children)) > uint64(c.limits.MaxChildren) {
			return Head{}, errors.New("parser-core phase zero: open-region recovery child limit")
		}
		if len(priorChildren) != 0 {
			combined := make([]SubtreeID, 0, len(priorChildren)+len(children)+len(region.children))
			combined = append(combined, priorChildren...)
			children = append(combined, children...)
		}
		children = append(children, region.children...)
	}
	if len(children) == 0 {
		return Head{}, errors.New("parser-core phase zero: ancestor recovery path has no ERROR payload")
	}
	first, err := c.subtree(children[0])
	if err != nil {
		return Head{}, err
	}
	last, err := c.subtree(children[len(children)-1])
	if err != nil {
		return Head{}, err
	}
	startByte, endByte := first.startByte, last.endByte
	if region != nil {
		endByte = region.endByte
		if len(priorChildren) != 0 {
			startByte = priorStart
		}
		if region.startByte < startByte {
			startByte = region.startByte
		}
	}
	errorPayload, err := c.appendSubtree(subtreeRecord{
		symbol: ErrorRegionSymbol, startByte: startByte, endByte: endByte, extra: true,
	}, children, nil, nil)
	if err != nil {
		return Head{}, err
	}

	out := Head{Node: target}
	errorLink := linkInput{prev: target, payload: errorPayload, scoreDelta: score, order: order}
	if cost != nil {
		storedErrorCost, costErr := cost(errorLink.prev, errorLink.payload)
		if costErr != nil {
			return Head{}, costErr
		}
		errorLink.storedErrorCost = storedErrorCost
		errorLink.hasStoredErrorCost = true
	}
	if trailing == 0 {
		outcome, err := c.condenseWithOutcomeAtomic(c.shiftedBoundaryKey(candidate.state, endByte), errorLink)
		return outcome.head, err
	}
	out, err = c.appendPrivate(candidate.state, last.endByte, errorLink)
	if err != nil {
		return Head{}, err
	}
	for index := trailing - 1; index >= 0; index-- {
		link := links[index]
		if link.isRecoveryDiscontinuity() {
			continue
		}
		payload, err := c.subtree(link.payload)
		if err != nil {
			return Head{}, err
		}
		input := linkInput{prev: out.Node, payload: link.payload, scoreDelta: link.scoreDelta}
		if link.hasOrder() {
			input.order = ForkOrder{Present: true, Value: link.order}
		}
		if cost != nil {
			storedErrorCost, costErr := cost(input.prev, input.payload)
			if costErr != nil {
				return Head{}, costErr
			}
			input.storedErrorCost = storedErrorCost
			input.hasStoredErrorCost = true
		}
		if index == 0 {
			outcome, err := c.condenseWithOutcomeAtomic(c.shiftedBoundaryKey(candidate.state, payload.endByte), input)
			if err != nil {
				return Head{}, err
			}
			out = outcome.head
			continue
		}
		out, err = c.appendPrivate(candidate.state, payload.endByte, input)
		if err != nil {
			return Head{}, err
		}
	}
	return out, nil
}

func (c *Core) uniqueAncestorRecoveryPath(candidate StackSummaryCandidate) ([]linkRecord, NodeID, error) {
	wantDepth := int(candidate.depth)
	route := make([]linkRecord, 0, int(candidate.linkDepth))
	var selected []linkRecord
	var selectedTarget NodeID
	var completePaths uint64
	matches := 0
	steps := 0
	const maxSteps = stackSummaryMaxVisitedNodes * StackSummaryMaxDepth

	var walk func(NodeID, int) error
	walk = func(id NodeID, depth int) error {
		node, err := c.node(id)
		if err != nil {
			return err
		}
		// C stops each pop path at the first node with the requested subtree
		// depth. Count that path before the goal-state filter.
		if depth == wantDepth {
			completePaths++
			if completePaths > c.limits.MaxPopPaths {
				return errors.New("parser-core phase zero: ancestor recovery pop enumeration cap")
			}
			if len(route) != 0 && node.state == candidate.state {
				matches++
				if matches > 1 {
					return errAncestorRecoveryCandidateAmbiguous
				}
				if node.byteOffset != candidate.byteOffset {
					return errors.New("parser-core phase zero: stale stack-summary candidate position")
				}
				if len(route) == int(candidate.linkDepth) {
					selected = append(selected[:0], route...)
					selectedTarget = id
				}
			}
			return nil
		}
		var inline [inlineAdjacencyCapacity]linkRecord
		links, err := c.publishedNodeLinksInto(inline[:0], *node)
		if err != nil {
			return err
		}
		for _, link := range links {
			if err := link.validateShape(); err != nil {
				return err
			}
			if link.prev == 0 || link.prev >= id {
				return errors.New("parser-core phase zero: ancestor recovery predecessor does not decrease")
			}
			nextDepth := depth
			if link.isRecoveryDiscontinuity() {
				if depth == wantDepth {
					continue
				}
				nextDepth++
			} else {
				payload, err := c.subtree(link.payload)
				if err != nil {
					return err
				}
				if !payload.extra {
					if depth == wantDepth {
						continue
					}
					nextDepth++
				}
			}
			steps++
			if steps > maxSteps {
				return errors.New("parser-core phase zero: ancestor recovery traversal cap")
			}
			route = append(route, link)
			if err := walk(link.prev, nextDepth); err != nil {
				return err
			}
			route = route[:len(route)-1]
		}
		return nil
	}
	if err := walk(candidate.source, 0); err != nil {
		return nil, 0, err
	}
	if matches == 0 {
		return nil, 0, errAncestorRecoveryCandidateNoPath
	}
	if selectedTarget == 0 || len(selected) != int(candidate.linkDepth) {
		return nil, 0, errAncestorRecoveryCandidateLinkDepth
	}
	return selected, selectedTarget, nil
}

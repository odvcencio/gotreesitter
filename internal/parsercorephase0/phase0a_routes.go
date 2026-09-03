//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
	"unsafe"
)

// Phase0APopRouteRecord binds one production pop ordinal to the independently
// re-enumerated physical links that produced it. FirstLink indexes the flat
// Phase0AProofSnapshot.PopRouteLinks slice; linkless zero-child pops are valid.
type Phase0APopRouteRecord struct {
	ID                  uint64
	TransactionID       uint64
	PopOrdinal          uint32
	Head                NodeID
	Predecessor         NodeID
	FirstLink           uint64
	LinkCount           uint32
	RetainedLinkCount   uint32
	TrailingLinkCount   uint32
	Occurrence          ConstructionOccurrenceKey
	Edge                IncomingEdgeKey
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

type Phase0APopRouteSegment uint8

const (
	Phase0APopRouteRetained Phase0APopRouteSegment = iota + 1
	Phase0APopRouteTrailing
)

type Phase0APopRouteLinkRecord struct {
	Route                 uint64
	TransactionID         uint64
	Ordinal               uint32
	Segment               Phase0APopRouteSegment
	SegmentOrdinal        uint32
	Link                  LinkID
	Node                  NodeID
	Predecessor           NodeID
	Payload               SubtreeID
	ScoreDelta            int64
	Order                 uint64
	HasOrder              bool
	RecoveryDiscontinuity bool
	RolledBack            bool
	RollbackTransaction   uint64
	RollbackCause         Phase0ARollbackCause
}

// Phase0AAcceptedSelectionRecord freezes the unique physical root-to-head
// spine after exact acceptance. The linkless graph seed contributes no row.
type Phase0AAcceptedSelectionRecord struct {
	Namespace  CoreRunNamespace
	Generation uint64
	Capability uint64
	Head       NodeID
	FirstLink  uint64
	LinkCount  uint32
}

// Phase0ASelectionCapabilityRecord authenticates one captured current head.
// Serial is monotonic within the run and never rewinds after rollback, so an
// arena-reused NodeID cannot make an old capability current again.
type Phase0ASelectionCapabilityRecord struct {
	Namespace           CoreRunNamespace
	Serial              uint64
	TransactionID       uint64
	Head                NodeID
	RolledBack          bool
	RollbackTransaction uint64
	RollbackCause       Phase0ARollbackCause
}

// Phase0AAcceptedLinkRecord is one selected physical link in root-to-head
// order. BoundExpression is the link binding; ResolvedExpression is the exact
// Direct leaf selected through any nested FactorChoice expressions.
// ResolvedLowerLink is the final source-adjacency lower link after applying
// every selector-route translation; it is zero only at the graph seed.
type Phase0AAcceptedLinkRecord struct {
	Namespace             CoreRunNamespace
	Generation            uint64
	Ordinal               uint32
	Link                  LinkID
	Payload               SubtreeID
	RecoveryDiscontinuity bool
	BoundExpression       Phase0AExpressionID
	ResolvedExpression    Phase0AExpressionID
	ResolvedLowerLink     LinkID
	Occurrence            ConstructionOccurrenceKey
	Edge                  IncomingEdgeKey
}

type phase0ARouteObserver struct {
	popRoutes           []Phase0APopRouteRecord
	popLinks            []Phase0APopRouteLinkRecord
	migrations          []Phase0ATrailingExtraMigrationRecord
	capabilities        []Phase0ASelectionCapabilityRecord
	acceptedSelections  []Phase0AAcceptedSelectionRecord
	acceptedLinks       []Phase0AAcceptedLinkRecord
	selectedTrees       []Phase0ASelectedOccurrenceSnapshot
	selectedOccurrences []Phase0ASelectedOccurrenceRecord
	selectedStates      []phase0ASelectedOccurrenceState
	frames              []phase0ARouteFrame
	nextPopRoute        uint64
	nextCapability      uint64
	nextSelection       uint64
}

type phase0ARouteFrame struct {
	popRouteStart   uint64
	popLinkStart    uint64
	migrationStart  uint64
	capabilityStart uint64
}

type phase0APhysicalPopPath struct {
	prev          NodeID
	children      []SubtreeID
	trailing      []SubtreeID
	links         []LinkID
	linkNodes     []NodeID
	linkPrevs     []NodeID
	linkScores    []int64
	linkOrders    []ForkOrder
	retainedCount uint32
	score         int64
	order         ForkOrder
	startByte     uint32
	structuralEnd uint32
}

type phase0APhysicalDerivation struct {
	links          []LinkID
	payloads       []SubtreeID
	score          int64
	branchOrder    uint64
	hasBranchOrder bool
}

type phase0APhysicalLink struct {
	id     LinkID
	node   NodeID
	record linkRecord
}

const (
	phase0APopRouteBytes               = uint64(unsafe.Sizeof(Phase0APopRouteRecord{}))
	phase0APopRouteLinkBytes           = uint64(unsafe.Sizeof(Phase0APopRouteLinkRecord{}))
	phase0ATrailingExtraMigrationBytes = uint64(unsafe.Sizeof(Phase0ATrailingExtraMigrationRecord{}))
	phase0ASelectionCapabilityBytes    = uint64(unsafe.Sizeof(Phase0ASelectionCapabilityRecord{}))
	phase0AAcceptedSelectionBytes      = uint64(unsafe.Sizeof(Phase0AAcceptedSelectionRecord{}))
	phase0AAcceptedLinkBytes           = uint64(unsafe.Sizeof(Phase0AAcceptedLinkRecord{}))
	phase0ARouteFrameBytes             = uint64(unsafe.Sizeof(phase0ARouteFrame{}))
)

func phase0ARouteMarkLocked(observer *phase0AObserver) {
	observer.route.frames = append(observer.route.frames, phase0ARouteFrame{
		popRouteStart:   uint64(len(observer.route.popRoutes)),
		popLinkStart:    uint64(len(observer.route.popLinks)),
		migrationStart:  uint64(len(observer.route.migrations)),
		capabilityStart: uint64(len(observer.route.capabilities)),
	})
}

func phase0ARouteRollbackLocked(observer *phase0AObserver, transaction uint64, cause Phase0ARollbackCause) {
	if len(observer.route.frames) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "route rollback frame mismatch"})
		return
	}
	frame := observer.route.frames[len(observer.route.frames)-1]
	for index := frame.popRouteStart; index < uint64(len(observer.route.popRoutes)); index++ {
		row := &observer.route.popRoutes[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.popLinkStart; index < uint64(len(observer.route.popLinks)); index++ {
		row := &observer.route.popLinks[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.migrationStart; index < uint64(len(observer.route.migrations)); index++ {
		row := &observer.route.migrations[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	for index := frame.capabilityStart; index < uint64(len(observer.route.capabilities)); index++ {
		row := &observer.route.capabilities[index]
		row.RolledBack, row.RollbackTransaction, row.RollbackCause = true, transaction, cause
	}
	observer.route.frames = observer.route.frames[:len(observer.route.frames)-1]
}

func phase0ARouteCommitLocked(observer *phase0AObserver) {
	if len(observer.route.frames) == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "route commit frame mismatch"})
		return
	}
	observer.route.frames = observer.route.frames[:len(observer.route.frames)-1]
}

func phase0ACheckRouteRowsLocked(observer *phase0AObserver, popRoutes, popLinks, migrations, selections, acceptedLinks uint64) error {
	checks := []struct {
		current uint64
		add     uint64
		limit   uint64
		detail  string
	}{
		{uint64(len(observer.route.popRoutes)), popRoutes, phase0AObservers.limits.MaxPopRoutes, "pop-route row cap"},
		{uint64(len(observer.route.popLinks)), popLinks, phase0AObservers.limits.MaxPopRouteLinks, "pop-route link cap"},
		{uint64(len(observer.route.migrations)), migrations, phase0AObservers.limits.MaxTrailingExtraMigrations, "trailing-extra migration row cap"},
		{uint64(len(observer.route.acceptedSelections)), selections, phase0AObservers.limits.MaxAcceptedSelections, "accepted-selection row cap"},
		{uint64(len(observer.route.acceptedLinks)), acceptedLinks, phase0AObservers.limits.MaxAcceptedLinks, "accepted-link row cap"},
	}
	for _, check := range checks {
		limit := check.limit
		if limit == 0 {
			limit = phase0AObservers.limits.MaxRecords
		}
		if check.current > limit || check.add > limit-check.current {
			return &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: check.detail}
		}
	}
	return nil
}

func phase0AReserveRouteRowsLocked(observer *phase0AObserver, popRoutes, popLinks, migrations, selections, acceptedLinks uint64) error {
	if err := phase0ACheckRouteRowsLocked(observer, popRoutes, popLinks, migrations, selections, acceptedLinks); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	records := popRoutes + popLinks + migrations + selections + acceptedLinks
	bytes := popRoutes*phase0APopRouteBytes + popLinks*phase0APopRouteLinkBytes + migrations*phase0ATrailingExtraMigrationBytes + selections*phase0AAcceptedSelectionBytes + acceptedLinks*phase0AAcceptedLinkBytes
	if err := phase0AChargeManyLocked(records, bytes); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	return nil
}

func phase0AReserveSelectionCapabilityLocked(observer *phase0AObserver) error {
	limit := phase0AObservers.limits.MaxSelectionCapabilities
	if limit == 0 {
		limit = phase0AObservers.limits.MaxRecords
	}
	if uint64(len(observer.route.capabilities)) >= limit {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selection-capability row cap"})
	}
	if err := phase0AChargeManyLocked(1, phase0ASelectionCapabilityBytes); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	return nil
}

func phase0AStablePhysicalLinks(core *Core, id NodeID) ([]phase0APhysicalLink, error) {
	node, err := core.node(id)
	if err != nil {
		return nil, err
	}
	if node.linkCount > core.limits.MaxLinksPerBoundary || uint64(node.linkCount) > uint64(core.limits.MaxLinks) || uint64(node.linkCount) > uint64(len(core.links)) {
		return nil, errors.New("parser-core phase zero A: physical adjacency exceeds cap")
	}
	out := make([]phase0APhysicalLink, node.linkCount)
	next := LinkID(node.firstLink)
	for index := len(out) - 1; index >= 0; index-- {
		if next == 0 || uint64(next) > uint64(len(core.links)) {
			return nil, errors.New("parser-core phase zero A: physical adjacency is unavailable")
		}
		link := core.links[next-1]
		if err := link.validateShape(); err != nil {
			return nil, err
		}
		out[index] = phase0APhysicalLink{id: next, node: id, record: link}
		next = link.next
	}
	if next != 0 {
		return nil, errors.New("parser-core phase zero A: physical adjacency exceeds recorded width")
	}
	return out, nil
}

// phase0AEnumeratePopRoutes deliberately mirrors popPaths without sharing its
// scratch or result. The caller compares every semantic field before binding
// production pop ordinals to physical LinkIDs.
func phase0AEnumeratePopRoutes(core *Core, head NodeID, childCount int) ([]phase0APhysicalPopPath, error) {
	if childCount == 0 {
		node, err := core.node(head)
		if err != nil {
			return nil, err
		}
		return []phase0APhysicalPopPath{{prev: head, startByte: node.byteOffset, structuralEnd: node.byteOffset}}, nil
	}
	var out []phase0APhysicalPopPath
	var revLinks []LinkID
	var revNodes []NodeID
	var revPrevs []NodeID
	var revPayloads []SubtreeID
	var revScores []int64
	var revOrders []ForkOrder
	var trailingLinks []LinkID
	var trailingNodes []NodeID
	var trailingPrevs []NodeID
	var trailingPayloads []SubtreeID
	var trailingScores []int64
	var trailingOrders []ForkOrder
	var walk func(NodeID, int, bool, uint32) error
	walk = func(id NodeID, remaining int, peelingTrailing bool, structuralEnd uint32) error {
		links, err := phase0AStablePhysicalLinks(core, id)
		if err != nil {
			return err
		}
		for _, physical := range links {
			link := physical.record
			if err := link.validateShape(); err != nil {
				return err
			}
			if link.prev == 0 || link.prev >= id {
				return errors.New("parser-core phase zero A: physical pop predecessor does not decrease")
			}
			order := ForkOrder{Value: link.order, Present: link.hasOrder()}
			if link.isRecoveryDiscontinuity() {
				revLinks = append(revLinks, physical.id)
				revNodes = append(revNodes, physical.node)
				revPrevs = append(revPrevs, link.prev)
				revPayloads = append(revPayloads, 0)
				revScores = append(revScores, 0)
				revOrders = append(revOrders, ForkOrder{})
				next := remaining - 1
				if next == 0 {
					if uint64(len(out)) >= core.limits.MaxPopPaths {
						return errors.New("parser-core phase zero A: physical pop enumeration cap")
					}
					path := phase0APhysicalPopPath{prev: link.prev, structuralEnd: structuralEnd}
					haveStartByte := false
					for index := len(revLinks) - 1; index >= 0; index-- {
						path.links = append(path.links, revLinks[index])
						path.linkNodes = append(path.linkNodes, revNodes[index])
						path.linkPrevs = append(path.linkPrevs, revPrevs[index])
						path.linkScores = append(path.linkScores, revScores[index])
						path.linkOrders = append(path.linkOrders, revOrders[index])
						if revPayloads[index] == 0 {
							continue
						}
						path.children = append(path.children, revPayloads[index])
						payloadRecord, payloadErr := core.subtree(revPayloads[index])
						if payloadErr != nil {
							return payloadErr
						}
						if !payloadRecord.extra && !haveStartByte {
							path.startByte, haveStartByte = payloadRecord.startByte, true
							if path.structuralEnd == 0 {
								path.structuralEnd = payloadRecord.endByte
							}
						}
						path.score, err = checkedAddScore(path.score, revScores[index])
						if err != nil {
							return err
						}
						if revOrders[index].Present {
							path.order = revOrders[index]
						}
					}
					if !haveStartByte && path.prev != 0 {
						previous, nodeErr := core.node(path.prev)
						if nodeErr != nil {
							return nodeErr
						}
						path.startByte, path.structuralEnd = previous.byteOffset, previous.byteOffset
					}
					path.retainedCount = uint32(len(path.links))
					for index := len(trailingLinks) - 1; index >= 0; index-- {
						path.links = append(path.links, trailingLinks[index])
						path.linkNodes = append(path.linkNodes, trailingNodes[index])
						path.linkPrevs = append(path.linkPrevs, trailingPrevs[index])
						path.linkScores = append(path.linkScores, trailingScores[index])
						path.linkOrders = append(path.linkOrders, trailingOrders[index])
						path.trailing = append(path.trailing, trailingPayloads[index])
						if trailingOrders[index].Present {
							path.order = trailingOrders[index]
						}
					}
					out = append(out, path)
				} else if err := walk(link.prev, next, false, structuralEnd); err != nil {
					return err
				}
				revLinks = revLinks[:len(revLinks)-1]
				revNodes = revNodes[:len(revNodes)-1]
				revPrevs = revPrevs[:len(revPrevs)-1]
				revPayloads = revPayloads[:len(revPayloads)-1]
				revScores = revScores[:len(revScores)-1]
				revOrders = revOrders[:len(revOrders)-1]
				continue
			}
			payload, err := core.subtree(link.payload)
			if err != nil {
				return err
			}
			if payload.extra && peelingTrailing {
				trailingLinks = append(trailingLinks, physical.id)
				trailingNodes = append(trailingNodes, physical.node)
				trailingPrevs = append(trailingPrevs, link.prev)
				trailingPayloads = append(trailingPayloads, link.payload)
				trailingScores = append(trailingScores, link.scoreDelta)
				trailingOrders = append(trailingOrders, order)
				if err := walk(link.prev, remaining, true, structuralEnd); err != nil {
					return err
				}
				trailingLinks = trailingLinks[:len(trailingLinks)-1]
				trailingNodes = trailingNodes[:len(trailingNodes)-1]
				trailingPrevs = trailingPrevs[:len(trailingPrevs)-1]
				trailingPayloads = trailingPayloads[:len(trailingPayloads)-1]
				trailingScores = trailingScores[:len(trailingScores)-1]
				trailingOrders = trailingOrders[:len(trailingOrders)-1]
				continue
			}
			nextStructuralEnd := structuralEnd
			if !payload.extra && peelingTrailing {
				nextStructuralEnd = payload.endByte
			}
			revLinks = append(revLinks, physical.id)
			revNodes = append(revNodes, physical.node)
			revPrevs = append(revPrevs, link.prev)
			revPayloads = append(revPayloads, link.payload)
			revScores = append(revScores, link.scoreDelta)
			revOrders = append(revOrders, order)
			next := remaining
			if !payload.extra {
				next--
			}
			if next == 0 {
				if uint64(len(out)) >= core.limits.MaxPopPaths {
					return errors.New("parser-core phase zero A: physical pop enumeration cap")
				}
				path := phase0APhysicalPopPath{prev: link.prev, startByte: payload.startByte, structuralEnd: nextStructuralEnd}
				for index := len(revLinks) - 1; index >= 0; index-- {
					path.links = append(path.links, revLinks[index])
					path.linkNodes = append(path.linkNodes, revNodes[index])
					path.linkPrevs = append(path.linkPrevs, revPrevs[index])
					path.linkScores = append(path.linkScores, revScores[index])
					path.linkOrders = append(path.linkOrders, revOrders[index])
					if revPayloads[index] != 0 {
						path.children = append(path.children, revPayloads[index])
					}
					path.score, err = checkedAddScore(path.score, revScores[index])
					if err != nil {
						return err
					}
					if revOrders[index].Present {
						path.order = revOrders[index]
					}
				}
				path.retainedCount = uint32(len(path.links))
				for index := len(trailingLinks) - 1; index >= 0; index-- {
					path.links = append(path.links, trailingLinks[index])
					path.linkNodes = append(path.linkNodes, trailingNodes[index])
					path.linkPrevs = append(path.linkPrevs, trailingPrevs[index])
					path.linkScores = append(path.linkScores, trailingScores[index])
					path.linkOrders = append(path.linkOrders, trailingOrders[index])
					path.trailing = append(path.trailing, trailingPayloads[index])
					if trailingOrders[index].Present {
						path.order = trailingOrders[index]
					}
				}
				out = append(out, path)
			} else if err := walk(link.prev, next, false, nextStructuralEnd); err != nil {
				return err
			}
			revLinks = revLinks[:len(revLinks)-1]
			revNodes = revNodes[:len(revNodes)-1]
			revPrevs = revPrevs[:len(revPrevs)-1]
			revPayloads = revPayloads[:len(revPayloads)-1]
			revScores = revScores[:len(revScores)-1]
			revOrders = revOrders[:len(revOrders)-1]
		}
		return nil
	}
	if err := walk(head, childCount, true, 0); err != nil {
		return nil, err
	}
	return out, nil
}

func phase0ASameSubtrees(left, right []SubtreeID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func phase0AObservePopRoutes(core *Core, head NodeID, childCount int, production []popPath) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	pending := &observer.reduction
	if !pending.active || pending.observed != 0 || phase0ACurrentTransaction(observer) != pending.transaction || uint64(len(production)) != uint64(pending.expected) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "physical pop proof is outside its reduction"})
		return
	}
	routes, err := phase0AEnumeratePopRoutes(core, head, childCount)
	if err != nil {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: err.Error()})
		return
	}
	if len(routes) != len(production) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: fmt.Sprintf("physical pop path count %d differs from production %d", len(routes), len(production))})
		return
	}
	linkCount := uint64(0)
	for index := range routes {
		got, want := routes[index], production[index]
		wantTrailing := make([]SubtreeID, len(want.trailing))
		for trailingIndex := range want.trailing {
			wantTrailing[trailingIndex] = want.trailing[trailingIndex].payload
		}
		if len(got.links) != len(got.linkNodes) || len(got.links) != len(got.linkPrevs) || len(got.links) != len(got.linkScores) || len(got.links) != len(got.linkOrders) || uint64(got.retainedCount) > uint64(len(got.links)) ||
			uint64(len(got.children)) != uint64(len(want.children)) || uint64(len(got.links))-uint64(got.retainedCount) != uint64(len(want.trailing)) ||
			got.prev != want.prev || got.score != want.score || got.order != want.order || got.startByte != want.startByte || got.structuralEnd != want.structuralEnd || !phase0ASameSubtrees(got.children, want.children) || !phase0ASameSubtrees(got.trailing, wantTrailing) {
			phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "physical pop path does not exactly match production ordinal"})
			return
		}
		if linkCount > math.MaxUint64-uint64(len(got.links)) {
			phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "physical pop link count"})
			return
		}
		linkCount += uint64(len(got.links))
	}
	if uint64(len(routes)) > math.MaxUint32 || linkCount > math.MaxUint32 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "physical pop route width"})
		return
	}
	if observer.route.nextPopRoute > math.MaxUint64-uint64(len(routes)) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "physical pop route serial"})
		return
	}
	if err := phase0AReserveRouteRowsLocked(observer, uint64(len(routes)), linkCount, 0, 0, 0); err != nil {
		return
	}
	pending.routeStart = uint64(len(observer.route.popRoutes))
	pending.routeCount = uint32(len(routes))
	pending.routeProof = true
	for index, route := range routes {
		observer.route.nextPopRoute++
		id := observer.route.nextPopRoute
		first := uint64(len(observer.route.popLinks))
		observer.route.popRoutes = append(observer.route.popRoutes, Phase0APopRouteRecord{
			ID: id, TransactionID: pending.transaction, PopOrdinal: uint32(index), Head: head,
			Predecessor: route.prev, FirstLink: first, LinkCount: uint32(len(route.links)),
			RetainedLinkCount: route.retainedCount, TrailingLinkCount: uint32(len(route.links)) - route.retainedCount,
		})
		for ordinal, linkID := range route.links {
			link := core.links[linkID-1]
			segment, segmentOrdinal := Phase0APopRouteRetained, uint32(ordinal)
			if uint32(ordinal) >= route.retainedCount {
				segment, segmentOrdinal = Phase0APopRouteTrailing, uint32(ordinal)-route.retainedCount
			}
			observer.route.popLinks = append(observer.route.popLinks, Phase0APopRouteLinkRecord{
				Route: id, TransactionID: pending.transaction, Ordinal: uint32(ordinal), Segment: segment, SegmentOrdinal: segmentOrdinal,
				Link: linkID, Node: route.linkNodes[ordinal], Predecessor: route.linkPrevs[ordinal], Payload: link.payload,
				ScoreDelta: route.linkScores[ordinal], Order: route.linkOrders[ordinal].Value, HasOrder: route.linkOrders[ordinal].Present,
				RecoveryDiscontinuity: link.payload == 0,
			})
		}
	}
}

func phase0ABindReductionRouteLocked(observer *phase0AObserver, pending *phase0AReductionConstruction, occurrence ConstructionOccurrenceKey, edge IncomingEdgeKey, predecessor NodeID) bool {
	if pending.routeCount != pending.expected || pending.observed >= pending.routeCount {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "reduction occurrence has no physical pop route"})
		return false
	}
	index := pending.routeStart + uint64(pending.observed)
	if index >= uint64(len(observer.route.popRoutes)) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "physical pop route row is unavailable"})
		return false
	}
	route := &observer.route.popRoutes[index]
	if route.RolledBack || route.TransactionID != pending.transaction || route.PopOrdinal != pending.observed || route.Predecessor != predecessor || route.Occurrence != (ConstructionOccurrenceKey{}) || route.Edge != (IncomingEdgeKey{}) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "physical pop route cannot bind this occurrence"})
		return false
	}
	route.Occurrence, route.Edge = occurrence, edge
	return true
}

func phase0AReserveTrailingExtraMigrationLocked(observer *phase0AObserver) error {
	mutationLimit := phase0AObservers.limits.MaxMutations
	if mutationLimit == 0 {
		mutationLimit = math.MaxUint64
	}
	if uint64(len(observer.mutations)) > mutationLimit || 2 > mutationLimit-uint64(len(observer.mutations)) {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorMutationCap, Namespace: observer.run})
	}
	occurrenceLimit := phase0AObservers.limits.MaxOccurrences
	if occurrenceLimit == 0 {
		occurrenceLimit = math.MaxUint64
	}
	if observer.occurrenceCount >= occurrenceLimit {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorOccurrenceCap, Namespace: observer.run})
	}
	occurrenceBytes := phase0AOccurrenceRecordBytes + phase0AEdgeRecordBytes
	occurrenceByteLimit := phase0AObservers.limits.MaxOccurrenceBytes
	if occurrenceByteLimit == 0 {
		occurrenceByteLimit = math.MaxUint64
	}
	if observer.occurrenceBytes > occurrenceByteLimit || occurrenceBytes > occurrenceByteLimit-observer.occurrenceBytes {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorOccurrenceByteCap, Namespace: observer.run})
	}
	if err := phase0ACheckFactorRowsLocked(observer, 1, 0, 0, 0, 0, 0); err != nil {
		return err
	}
	if err := phase0ACheckRouteRowsLocked(observer, 0, 0, 1, 0, 0); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	if err := phase0AChargeManyLocked(4, occurrenceBytes+phase0ACandidateBytes+phase0ATrailingExtraMigrationBytes); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	observer.occurrenceCount++
	observer.occurrenceBytes += occurrenceBytes
	return nil
}

func phase0AExactRouteLinkLocked(observer *phase0AObserver, route Phase0APopRouteRecord, ordinal uint32) (Phase0APopRouteLinkRecord, error) {
	if ordinal >= route.LinkCount || route.FirstLink > uint64(len(observer.route.popLinks)) || uint64(ordinal) >= uint64(len(observer.route.popLinks))-route.FirstLink {
		return Phase0APopRouteLinkRecord{}, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "physical pop route link is unavailable"}
	}
	row := observer.route.popLinks[route.FirstLink+uint64(ordinal)]
	if row.RolledBack || row.Route != route.ID || row.TransactionID != route.TransactionID || row.Ordinal != ordinal {
		return Phase0APopRouteLinkRecord{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "physical pop route link is stale"}
	}
	return row, nil
}

func phase0AValidateCurrentRouteLink(core *Core, observer *phase0AObserver, row Phase0APopRouteLinkRecord) error {
	physical, err := phase0AStablePhysicalLinks(core, row.Node)
	if err != nil {
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "source trailing physical adjacency is unavailable"}
	}
	matches := 0
	for _, candidate := range physical {
		if candidate.id != row.Link {
			continue
		}
		matches++
		link := candidate.record
		if link.prev != row.Predecessor || link.payload != row.Payload || link.scoreDelta != row.ScoreDelta || link.hasOrder() != row.HasOrder || (row.HasOrder && link.order != row.Order) || link.isRecoveryDiscontinuity() != row.RecoveryDiscontinuity {
			return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "source trailing physical link changed identity"}
		}
	}
	if matches != 1 {
		kind := Phase0AErrorMissingReference
		if matches > 1 {
			kind = Phase0AErrorAmbiguousReference
		}
		return &Phase0AError{Kind: kind, Namespace: observer.run, Detail: "source trailing physical link is not unique"}
	}
	return nil
}

func phase0AExactPublishedCandidateLocked(observer *phase0AObserver, occurrence ConstructionOccurrenceKey, edge IncomingEdgeKey) (Phase0ACandidateRecord, error) {
	var found Phase0ACandidateRecord
	live, rolled := 0, 0
	for _, candidate := range observer.factor.candidates {
		if candidate.Occurrence != occurrence || candidate.Edge != edge {
			continue
		}
		if candidate.RolledBack {
			rolled++
			continue
		}
		live++
		found = candidate
	}
	if live > 1 {
		return Phase0ACandidateRecord{}, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "target publication has multiple live candidates"}
	}
	if live == 0 && rolled != 0 {
		return Phase0ACandidateRecord{}, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "target publication candidate is rolled back"}
	}
	if live == 0 {
		return Phase0ACandidateRecord{}, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "target publication candidate is unavailable"}
	}
	if !found.Claimed || !found.Resolved || !phase0ATransactionActiveLocked(observer, found.TransactionID) {
		return Phase0ACandidateRecord{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "target publication candidate is not live and resolved"}
	}
	return found, nil
}

func phase0AExpectedMigrationTargetLocked(observer *phase0AObserver, route Phase0APopRouteRecord, trailingOrdinal uint32) (ConstructionOccurrenceKey, IncomingEdgeKey, Phase0AConstructionKind, error) {
	if trailingOrdinal == 0 {
		return route.Occurrence, route.Edge, Phase0AConstructionReductionParent, nil
	}
	var found Phase0ATrailingExtraMigrationRecord
	live, rolled := 0, 0
	for _, migration := range observer.route.migrations {
		if migration.Route != route.ID || migration.TransactionID != route.TransactionID || migration.TrailingOrdinal != trailingOrdinal-1 {
			continue
		}
		if migration.RolledBack {
			rolled++
			continue
		}
		live++
		found = migration
	}
	if live > 1 {
		return ConstructionOccurrenceKey{}, IncomingEdgeKey{}, 0, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "prior trailing-extra migration has multiple live rows"}
	}
	if live == 0 && rolled != 0 {
		return ConstructionOccurrenceKey{}, IncomingEdgeKey{}, 0, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "prior trailing-extra migration is rolled back"}
	}
	if live == 0 {
		return ConstructionOccurrenceKey{}, IncomingEdgeKey{}, 0, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "prior trailing-extra migration is unavailable"}
	}
	return found.Occurrence, found.Edge, Phase0AConstructionExtraTerminal, nil
}

func phase0AValidateMigrationTargetLocked(core *Core, observer *phase0AObserver, route Phase0APopRouteRecord, trailingOrdinal uint32, predecessor NodeID) (LinkID, ConstructionOccurrenceKey, IncomingEdgeKey, error) {
	node, err := core.node(predecessor)
	if err != nil || node.linkCount != 1 {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, &Phase0AError{Kind: Phase0AErrorInvalidOccurrence, Namespace: observer.run, Detail: "trailing-extra target predecessor is not a live width-one publication"}
	}
	physical, err := phase0AStablePhysicalLinks(core, predecessor)
	if err != nil || len(physical) != 1 {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "trailing-extra target predecessor adjacency is unavailable"}
	}
	expectedOccurrence, expectedEdge, expectedKind, err := phase0AExpectedMigrationTargetLocked(observer, route, trailingOrdinal)
	if err != nil {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, err
	}
	candidate, err := phase0AExactPublishedCandidateLocked(observer, expectedOccurrence, expectedEdge)
	if err != nil {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, err
	}
	link := physical[0]
	if link.node != predecessor || link.record.prev != candidate.Predecessor || link.record.payload != candidate.Payload || link.record.scoreDelta != candidate.ScoreDelta || link.record.hasOrder() != candidate.HasOrder || (candidate.HasOrder && link.record.order != candidate.Order) ||
		node.state != candidate.Boundary.State || node.byteOffset != candidate.Boundary.ByteOffset || candidate.Boundary.Frontier != core.frontier || candidate.Boundary.Checkpoint != core.checkpoint || candidate.Boundary.Shifted {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, &Phase0AError{Kind: Phase0AErrorInvalidOccurrence, Namespace: observer.run, Detail: "trailing-extra target predecessor is not the exact expected publication"}
	}
	expressionID, err := phase0AExactBindingLocked(observer, link.id)
	if err != nil {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, err
	}
	expression, err := phase0AExactExpressionLocked(observer, expressionID)
	if err != nil {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, err
	}
	if expression.Kind != Phase0AExpressionDirect || expression.Occurrence != expectedOccurrence || expression.Edge != expectedEdge {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "trailing-extra target link is not bound to the expected publication"}
	}
	if err := phase0AValidateDirectOccurrenceLocked(observer, expression); err != nil {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, err
	}
	occurrence := observer.mutations[observer.occurrenceIndex[expectedOccurrence]]
	if occurrence.Payload != candidate.Payload || occurrence.Predecessor != candidate.Predecessor || occurrence.ConstructionKind != expectedKind {
		return 0, ConstructionOccurrenceKey{}, IncomingEdgeKey{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "trailing-extra target occurrence identity mismatch"}
	}
	return link.id, expectedOccurrence, expectedEdge, nil
}

// phase0AObserveTrailingExtraMigration authenticates an existing selected
// trailing-extra physical edge and allocates a fresh occurrence/edge under its
// original extra-terminal event before production re-links that suffix.
func phase0AObserveTrailingExtraMigration(core *Core, trailingOrdinal uint32, boundary boundaryKey, in linkInput) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active || observer.failure != nil {
		return
	}
	pending := &observer.reduction
	if !pending.active || !pending.routeProof || pending.observed == 0 || pending.observed > pending.routeCount || phase0ACurrentTransaction(observer) != pending.transaction {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "trailing-extra migration is outside its proved reduction route"})
		return
	}
	routeIndex := pending.routeStart + uint64(pending.observed-1)
	if routeIndex >= uint64(len(observer.route.popRoutes)) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "trailing-extra migration route is unavailable"})
		return
	}
	route := observer.route.popRoutes[routeIndex]
	if route.RolledBack || route.TransactionID != pending.transaction || route.PopOrdinal != pending.observed-1 || route.Occurrence == (ConstructionOccurrenceKey{}) || route.Edge == (IncomingEdgeKey{}) || trailingOrdinal >= route.TrailingLinkCount || route.RetainedLinkCount == 0 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "trailing-extra migration route is not exactly bound"})
		return
	}
	sourceOrdinal := route.RetainedLinkCount + trailingOrdinal
	source, err := phase0AExactRouteLinkLocked(observer, route, sourceOrdinal)
	if err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	lower, err := phase0AExactRouteLinkLocked(observer, route, sourceOrdinal-1)
	if err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	if source.Segment != Phase0APopRouteTrailing || source.SegmentOrdinal != trailingOrdinal || lower.Node != source.Predecessor || lower.Ordinal+1 != source.Ordinal {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "trailing-extra source/lower physical chain mismatch"})
		return
	}
	if err := phase0AValidateCurrentRouteLink(core, observer, source); err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	// The lower identity participates in factor selection. Re-authenticate it
	// independently against the current physical adjacency rather than trusting
	// the frozen route row that supplied SourceLowerLink.
	if err := phase0AValidateCurrentRouteLink(core, observer, lower); err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	bound, err := phase0AExactBindingLocked(observer, source.Link)
	if err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	direct, err := phase0AResolveAcceptedExpressionLocked(observer, bound, lower.Link)
	if err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	if err := phase0AValidateDirectOccurrenceLocked(observer, direct); err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	occurrenceMutation := observer.mutations[observer.occurrenceIndex[direct.Occurrence]]
	eventIndex, ok := observer.eventIndex[direct.Occurrence.Event]
	if !ok || eventIndex >= uint64(len(observer.mutations)) {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "trailing-extra source event is unavailable"})
		return
	}
	eventMutation := observer.mutations[eventIndex]
	if eventMutation.RolledBack || eventMutation.Kind != Phase0AMutationEvent || eventMutation.Event != direct.Occurrence.Event || eventMutation.Payload != source.Payload || eventMutation.ConstructionKind != Phase0AConstructionExtraTerminal || occurrenceMutation.Payload != source.Payload || occurrenceMutation.ConstructionKind != Phase0AConstructionExtraTerminal {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "trailing-extra source is not one live extra-terminal event"})
		return
	}
	subtree, subtreeErr := core.subtree(in.payload)
	if subtreeErr != nil || !subtree.terminal || !subtree.extra || source.Payload != in.payload || source.ScoreDelta != in.scoreDelta || boundary.frontier != core.frontier || boundary.checkpoint != core.checkpoint || boundary.shifted || boundary.state == 0 || boundary.byteOffset != subtree.endByte {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorInvalidOccurrence, Namespace: observer.run, Detail: "trailing-extra migration input or private predecessor mismatch"})
		return
	}
	targetLink, targetOccurrence, targetEdge, err := phase0AValidateMigrationTargetLocked(core, observer, route, trailingOrdinal, in.prev)
	if err != nil {
		phase0AStickyLocked(observer, err.(*Phase0AError))
		return
	}
	nextSlot, ok := observer.nextOccurrenceSlot[direct.Occurrence.Event]
	if !ok || nextSlot < direct.Occurrence.Slot {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "trailing-extra event occurrence slot is unavailable"})
		return
	}
	if nextSlot == math.MaxUint32 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Counter: Phase0ACounterOccurrenceSlot, Namespace: observer.run})
		return
	}
	if observer.nextEdge == math.MaxUint64 {
		phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Counter: Phase0ACounterEdgeSerial, Namespace: observer.run})
		return
	}
	if err := phase0AReserveTrailingExtraMigrationLocked(observer); err != nil {
		return
	}
	nextSlot++
	observer.nextEdge++
	occurrence := ConstructionOccurrenceKey{Event: direct.Occurrence.Event, Slot: nextSlot}
	edge := IncomingEdgeKey{Event: direct.Occurrence.Event, Serial: observer.nextEdge}
	record := Phase0AMutationRecord{TransactionID: pending.transaction, Event: direct.Occurrence.Event, Edge: edge, Occurrence: occurrence, Payload: in.payload, Predecessor: in.prev, ConstructionKind: Phase0AConstructionExtraTerminal}
	record.Kind = Phase0AMutationOccurrence
	observer.occurrenceIndex[occurrence] = uint64(len(observer.mutations))
	observer.mutations = append(observer.mutations, record)
	record.Kind = Phase0AMutationEdge
	observer.edgeIndex[edge] = uint64(len(observer.mutations))
	observer.mutations = append(observer.mutations, record)
	phase0AAppendCandidatePrechargedLocked(observer, occurrence, edge, boundary, in)
	observer.route.migrations = append(observer.route.migrations, Phase0ATrailingExtraMigrationRecord{
		TransactionID: pending.transaction, Route: route.ID, TrailingOrdinal: trailingOrdinal,
		SourceLink: source.Link, SourceLowerLink: lower.Link, SourceExpression: direct.ID,
		SourceOccurrence: direct.Occurrence, SourceEdge: direct.Edge, Occurrence: occurrence, Edge: edge,
		TargetLink: targetLink, TargetOccurrence: targetOccurrence, TargetEdge: targetEdge,
		Boundary: phase0ABoundaryInput(boundary), Payload: in.payload, Predecessor: in.prev,
		ScoreDelta: in.scoreDelta, Order: in.order.Value, HasOrder: in.order.Present,
	})
	observer.nextOccurrenceSlot[direct.Occurrence.Event] = nextSlot
}

func phase0AEnumeratePhysicalDerivations(core *Core, head Head) ([]phase0APhysicalDerivation, error) {
	visiting := make(map[NodeID]bool)
	var walk func(NodeID) ([]phase0APhysicalDerivation, error)
	walk = func(id NodeID) ([]phase0APhysicalDerivation, error) {
		if visiting[id] {
			return nil, errors.New("parser-core phase zero A: accepted physical spine cycle")
		}
		node, err := core.node(id)
		if err != nil {
			return nil, err
		}
		if node.linkCount == 0 {
			if node.pathCount != 1 {
				return nil, errors.New("parser-core phase zero A: accepted physical seed path count")
			}
			return []phase0APhysicalDerivation{{}}, nil
		}
		visiting[id] = true
		defer delete(visiting, id)
		links, err := phase0AStablePhysicalLinks(core, id)
		if err != nil {
			return nil, err
		}
		var out []phase0APhysicalDerivation
		for _, physical := range links {
			if err := physical.record.validateShape(); err != nil {
				return nil, err
			}
			if physical.record.prev == 0 || physical.record.prev >= id {
				return nil, errors.New("parser-core phase zero A: accepted physical predecessor does not decrease")
			}
			prefixes, err := walk(physical.record.prev)
			if err != nil {
				return nil, err
			}
			for _, prefix := range prefixes {
				if uint64(len(out)) >= core.limits.MaxDerivations {
					return nil, ErrDerivationEnumerationCap
				}
				score, err := checkedAddScore(prefix.score, physical.record.scoreDelta)
				if err != nil {
					return nil, err
				}
				path := phase0APhysicalDerivation{score: score, branchOrder: prefix.branchOrder, hasBranchOrder: prefix.hasBranchOrder}
				path.links = append(path.links, prefix.links...)
				path.links = append(path.links, physical.id)
				path.payloads = append(path.payloads, prefix.payloads...)
				if physical.record.payload != 0 {
					path.payloads = append(path.payloads, physical.record.payload)
				}
				if physical.record.hasOrder() {
					path.branchOrder, path.hasBranchOrder = physical.record.order, true
				}
				out = append(out, path)
			}
		}
		if node.pathCount != math.MaxUint64 && uint64(len(out)) != node.pathCount {
			return nil, errors.New("parser-core phase zero A: accepted physical path-count mismatch")
		}
		return out, nil
	}
	return walk(head.Node)
}

func phase0AExactBindingLocked(observer *phase0AObserver, link LinkID) (Phase0AExpressionID, error) {
	var found Phase0AExpressionID
	live, rolled := 0, 0
	for index := range observer.factor.bindings {
		binding := observer.factor.bindings[index]
		if binding.Link != link {
			continue
		}
		if binding.RolledBack {
			rolled++
			continue
		}
		live++
		found = binding.Expression
	}
	if live > 1 {
		return 0, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "accepted link has multiple live bindings"}
	}
	if live == 0 && rolled != 0 {
		return 0, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "accepted link resolves only to rolled-back bindings"}
	}
	if live == 0 {
		return 0, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "accepted link has no binding"}
	}
	return found, nil
}

func phase0AExactExpressionLocked(observer *phase0AObserver, id Phase0AExpressionID) (Phase0AExpressionRecord, error) {
	var found Phase0AExpressionRecord
	live, rolled := 0, 0
	for _, expression := range observer.factor.expressions {
		if expression.ID != id {
			continue
		}
		if expression.RolledBack {
			rolled++
			continue
		}
		live++
		found = expression
	}
	if live > 1 {
		return Phase0AExpressionRecord{}, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "expression ID has multiple live rows"}
	}
	if live == 0 && rolled != 0 {
		return Phase0AExpressionRecord{}, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "expression ID resolves only to a rolled-back row"}
	}
	if live == 0 {
		return Phase0AExpressionRecord{}, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "expression ID is unavailable"}
	}
	return found, nil
}

func phase0ASelectorBranchLocked(observer *phase0AObserver, selector Phase0ASelectorID, selectedLower LinkID) (Phase0ASelectorBranch, LinkID, error) {
	selectorLive, selectorRolled := 0, 0
	for _, row := range observer.factor.selectors {
		if row.ID != selector {
			continue
		}
		if row.RolledBack {
			selectorRolled++
		} else {
			selectorLive++
		}
	}
	if selectorLive > 1 {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "factor selector has multiple live rows"}
	}
	if selectorLive == 0 && selectorRolled != 0 {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "factor selector is rolled back"}
	}
	if selectorLive == 0 {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "factor selector is unavailable"}
	}
	var branch Phase0ASelectorBranch
	var sourceLower LinkID
	routeLive, routeRolled := 0, 0
	for _, route := range observer.factor.selectorRoutes {
		if route.Selector != selector || route.SelectedLowerLink != selectedLower {
			continue
		}
		if route.RolledBack {
			routeRolled++
		} else {
			routeLive++
			branch = route.Branch
			sourceLower = route.SourceLowerLink
		}
	}
	if routeLive > 1 {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "factor selector has multiple routes for selected lower link"}
	}
	if routeLive == 0 && routeRolled != 0 {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "factor route is rolled back"}
	}
	if routeLive == 0 {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "factor route does not contain selected lower link"}
	}
	if (branch != Phase0ASelectorLeft && branch != Phase0ASelectorRight) || sourceLower == 0 {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "factor route has invalid branch or source link"}
	}
	return branch, sourceLower, nil
}

func phase0AResolveAcceptedExpressionAndLowerLocked(observer *phase0AObserver, initial Phase0AExpressionID, selectedLower LinkID) (Phase0AExpressionRecord, LinkID, error) {
	current := initial
	visited := make(map[Phase0AExpressionID]struct{})
	stepCap := len(observer.factor.expressions) + 1
	for steps := 0; steps < stepCap; steps++ {
		if _, duplicate := visited[current]; duplicate {
			return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorCyclicReference, Namespace: observer.run, Detail: "factor expression cycle"}
		}
		visited[current] = struct{}{}
		expression, err := phase0AExactExpressionLocked(observer, current)
		if err != nil {
			return Phase0AExpressionRecord{}, 0, err
		}
		switch expression.Kind {
		case Phase0AExpressionDirect:
			if expression.Occurrence.Event.Attempt.Run != observer.run || expression.Edge.Event.Attempt.Run != observer.run || expression.Occurrence.Event.Attempt.AttemptEpoch != 1 || expression.Edge.Event.Attempt.AttemptEpoch != 1 {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "direct expression belongs to another run"}
			}
			return expression, selectedLower, nil
		case Phase0AExpressionFactorChoice:
			if selectedLower == 0 {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "factor expression has no selected lower link"}
			}
			branch, sourceLower, err := phase0ASelectorBranchLocked(observer, expression.Selector, selectedLower)
			if err != nil {
				return Phase0AExpressionRecord{}, 0, err
			}
			if branch == Phase0ASelectorLeft {
				current = expression.Left
			} else {
				current = expression.Right
			}
			// A selector route translates the selected link in the current
			// merged adjacency back into the source adjacency used to build the
			// chosen expression. Nested FactorChoice resolution must carry that
			// physical identity inward rather than reusing the copied link.
			selectedLower = sourceLower
		default:
			return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "unknown expression kind"}
		}
	}
	return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorCyclicReference, Namespace: observer.run, Detail: "factor expression step cap"}
}

func phase0AResolveAcceptedExpressionLocked(observer *phase0AObserver, initial Phase0AExpressionID, selectedLower LinkID) (Phase0AExpressionRecord, error) {
	direct, _, err := phase0AResolveAcceptedExpressionAndLowerLocked(observer, initial, selectedLower)
	return direct, err
}

func phase0AValidateDirectOccurrenceLocked(observer *phase0AObserver, expression Phase0AExpressionRecord) error {
	occurrenceIndex, ok := observer.occurrenceIndex[expression.Occurrence]
	if !ok || occurrenceIndex >= uint64(len(observer.mutations)) {
		return &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "accepted direct occurrence is not indexed"}
	}
	occurrence := observer.mutations[occurrenceIndex]
	if occurrence.RolledBack {
		return &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "accepted direct occurrence was rolled back"}
	}
	if occurrence.Kind != Phase0AMutationOccurrence || occurrence.Occurrence != expression.Occurrence || occurrence.Edge != expression.Edge {
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "accepted direct occurrence index mismatch"}
	}
	edgeIndex, ok := observer.edgeIndex[expression.Edge]
	if !ok || edgeIndex >= uint64(len(observer.mutations)) {
		return &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "accepted direct edge is not indexed"}
	}
	edge := observer.mutations[edgeIndex]
	if edge.RolledBack {
		return &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "accepted direct edge was rolled back"}
	}
	if edge.Kind != Phase0AMutationEdge || edge.Occurrence != expression.Occurrence || edge.Edge != expression.Edge {
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "accepted direct edge index mismatch"}
	}
	return nil
}

// CapturePhase0ASelectionCapability authenticates one current head with a
// run-scoped serial that does not rewind after rollback. The record follows
// the parser transaction that was active at capture time.
func (core *Core) CapturePhase0ASelectionCapability(head Head) (Phase0AAcceptedSelectionCapability, error) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	if !phase0AObservers.active {
		return Phase0AAcceptedSelectionCapability{head: head}, nil
	}
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active {
		return Phase0AAcceptedSelectionCapability{head: head}, nil
	}
	if observer.failure != nil {
		return Phase0AAcceptedSelectionCapability{}, phase0AExistingFailureLocked(observer)
	}
	if _, err := core.node(head.Node); err != nil {
		return Phase0AAcceptedSelectionCapability{}, phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selection capability head is unavailable"})
	}
	if observer.route.nextCapability == math.MaxUint64 {
		return Phase0AAcceptedSelectionCapability{}, phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Counter: Phase0ACounterSelectionCapability, Namespace: observer.run})
	}
	if err := phase0AReserveSelectionCapabilityLocked(observer); err != nil {
		return Phase0AAcceptedSelectionCapability{}, err
	}
	observer.route.nextCapability++
	capability := Phase0AAcceptedSelectionCapability{
		coreInstance: observer.run.CoreInstance, runGeneration: observer.run.RunGeneration,
		serial: observer.route.nextCapability, head: head,
	}
	observer.route.capabilities = append(observer.route.capabilities, Phase0ASelectionCapabilityRecord{
		Namespace: observer.run, Serial: capability.serial,
		TransactionID: phase0ACurrentTransaction(observer), Head: head.Node,
	})
	return capability, nil
}

func phase0AValidateSelectionCapabilityLocked(observer *phase0AObserver, capability Phase0AAcceptedSelectionCapability) (Head, error) {
	namespace := CoreRunNamespace{CoreInstance: capability.coreInstance, RunGeneration: capability.runGeneration}
	if namespace != observer.run || capability.serial == 0 || capability.serial > uint64(len(observer.route.capabilities)) {
		return Head{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selection capability is not current in this run"}
	}
	record := observer.route.capabilities[capability.serial-1]
	if record.Namespace != observer.run || record.Serial != capability.serial || record.Head != capability.head.Node || record.RolledBack {
		return Head{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selection capability was rolled back or replaced"}
	}
	return capability.head, nil
}

// ObservePhase0AAcceptedSelection freezes one exact accepted physical spine
// and reconstructs its raw compact occurrence tree from construction identity.
// It is called only after the production diagnostic scheduler has proved one
// accepted head and one derivation. Public visibility/folding remains out of
// scope: the raw proof is validated against compact records only after it is
// built from occurrence, edge, route, link, and migration identities.
func (core *Core) ObservePhase0AAcceptedSelection(capability Phase0AAcceptedSelectionCapability) error {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	if !phase0AObservers.active {
		return nil
	}
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active {
		return nil
	}
	if observer.failure != nil {
		return phase0AExistingFailureLocked(observer)
	}
	if len(core.transactions) != 0 || core.schedulerFrame.active || len(observer.frames) != 0 {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorTransactionProof, Namespace: observer.run, Detail: "accepted selection observed inside a transaction"})
	}
	head, err := phase0AValidateSelectionCapabilityLocked(observer, capability)
	if err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	if _, err := core.node(head.Node); err != nil {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selection capability head is unavailable"})
	}
	production, err := core.Derivations(head)
	if err != nil {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: err.Error()})
	}
	physical, err := phase0AEnumeratePhysicalDerivations(core, head)
	if err != nil {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Namespace: observer.run, Detail: err.Error()})
	}
	if len(production) != 1 || len(physical) != 1 {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "accepted selection is not one exact physical derivation"})
	}
	want, got := production[0], physical[0]
	if !phase0ASameSubtrees(want.Payloads, got.payloads) || want.Score != got.score || want.BranchOrder != got.branchOrder || want.HasBranchOrder != got.hasBranchOrder {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "accepted physical spine differs from production derivation"})
	}
	if observer.route.nextSelection == math.MaxUint64 {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Counter: Phase0ACounterAcceptedSelectionGeneration, Namespace: observer.run})
	}
	if uint64(len(got.links)) > math.MaxUint32 {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "accepted physical spine width"})
	}
	selectedCount, selectedDepth, err := phase0ACountSelectedCompactOccurrencesLocked(core, observer, got.payloads)
	if err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	transientBytes, err := phase0APreflightSelectedIndexLocked(core, observer, uint64(len(got.links)), selectedCount, selectedDepth)
	if err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	phase0AObservers.bytes += transientBytes
	defer func() {
		phase0AObservers.bytes -= transientBytes
	}()
	selectedIndex := phase0ABuildSelectedIndex(observer)
	resolved := make([]Phase0AAcceptedLinkRecord, len(got.links))
	payloadIndex := 0
	selectedLower := LinkID(0)
	for index, link := range got.links {
		if core.links[link-1].isRecoveryDiscontinuity() {
			resolved[index] = Phase0AAcceptedLinkRecord{
				Ordinal: uint32(index), Link: link, RecoveryDiscontinuity: true,
				ResolvedLowerLink: selectedLower,
			}
			continue
		}
		if payloadIndex >= len(got.payloads) {
			return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "accepted physical payload census is incomplete"})
		}
		payload := got.payloads[payloadIndex]
		payloadIndex++
		bound, err := phase0ASelectedLookupError(observer, selectedIndex.bindings[link], "accepted root binding is unavailable or non-unique")
		if err != nil {
			return phase0AStickyLocked(observer, err.(*Phase0AError))
		}
		direct, resolvedLower, err := phase0AResolveSelectedExpressionLocked(core, observer, selectedIndex, bound, selectedLower)
		if err != nil {
			return phase0AStickyLocked(observer, err.(*Phase0AError))
		}
		if err := phase0AValidateDirectOccurrenceLocked(observer, direct); err != nil {
			return phase0AStickyLocked(observer, err.(*Phase0AError))
		}
		if err := phase0AValidateResolvedLowerLocked(core, observer, direct, resolvedLower); err != nil {
			return phase0AStickyLocked(observer, err.(*Phase0AError))
		}
		resolved[index] = Phase0AAcceptedLinkRecord{
			Ordinal: uint32(index), Link: link, Payload: payload, BoundExpression: bound,
			ResolvedExpression: direct.ID, ResolvedLowerLink: resolvedLower, Occurrence: direct.Occurrence, Edge: direct.Edge,
		}
		selectedLower = link
	}
	generation := observer.route.nextSelection + 1
	selectedTree, selectedOccurrences, err := phase0ABuildSelectedOccurrenceSnapshotLocked(core, observer, selectedIndex, generation, resolved)
	if err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	if uint64(len(selectedOccurrences)) != selectedCount || uint64(selectedTree.MaxDepth) != selectedDepth {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected identity tree disagrees with compact preflight census"})
	}
	selectedStates, err := phase0ASelectedOccurrenceStatesLocked(core, observer, selectedIndex, selectedOccurrences)
	if err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	if err := phase0AReserveAcceptedSelectionLocked(observer, uint64(len(resolved)), uint64(len(selectedOccurrences))); err != nil {
		return err
	}
	observer.route.nextSelection = generation
	first := uint64(len(observer.route.acceptedLinks))
	selectedTree.FirstOccurrence = uint64(len(observer.route.selectedOccurrences))
	observer.route.acceptedSelections = append(observer.route.acceptedSelections, Phase0AAcceptedSelectionRecord{
		Namespace: observer.run, Generation: generation, Capability: capability.serial,
		Head: head.Node, FirstLink: first, LinkCount: uint32(len(resolved)),
	})
	for index := range resolved {
		resolved[index].Namespace, resolved[index].Generation = observer.run, generation
		observer.route.acceptedLinks = append(observer.route.acceptedLinks, resolved[index])
	}
	observer.route.selectedTrees = append(observer.route.selectedTrees, selectedTree)
	observer.route.selectedOccurrences = append(observer.route.selectedOccurrences, selectedOccurrences...)
	observer.route.selectedStates = append(observer.route.selectedStates, selectedStates...)
	return nil
}

//go:build gts_parsercorephase0 && gts_parsercorephase0a

package parsercorephase0

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"math"
	"math/bits"
	"unsafe"
)

// Phase0ASelectedOccurrenceRecord is one position in the selected raw compact
// occurrence tree. Identity is the construction occurrence and incoming edge;
// Payload is retained only for post-construction compact-record validation.
// Ordinal and Parent are one-based indexes into the containing flat snapshot.
type Phase0ASelectedOccurrenceRecord struct {
	Ordinal          uint64
	Parent           uint64
	ChildOrdinal     uint32
	Depth            uint32
	Occurrence       ConstructionOccurrenceKey
	Edge             IncomingEdgeKey
	Payload          SubtreeID
	ConstructionKind Phase0AConstructionKind
	SourceLink       LinkID
	SelectedLower    LinkID
	Route            uint64
	ChildCount       uint32
	Migrated         bool
}

// Phase0ASelectedOccurrenceSnapshot describes one accepted selection in
// stable root-to-leaf, physical-child order. Digest hashes the ordered compact
// records only after the identity tree has been reconstructed.
type Phase0ASelectedOccurrenceSnapshot struct {
	Namespace       CoreRunNamespace
	Generation      uint64
	FirstOccurrence uint64
	OccurrenceCount uint64
	RootCount       uint32
	TerminalCount   uint64
	ReductionCount  uint64
	MigratedCount   uint64
	MaxDepth        uint32
	Digest          [sha256.Size]byte
}

// Phase0ASelectedOccurrenceCapability is an opaque, run-scoped borrowed-view
// token for one authenticated accepted occurrence tree. Its span is retained
// privately so callers cannot redirect it onto unrelated observer storage.
type Phase0ASelectedOccurrenceCapability struct {
	coreInstance  uint64
	runGeneration uint64
	generation    uint64
	first         uint64
	count         uint64
}

// Count reports the authenticated number of selected occurrences without
// exposing the observer-local backing-window offset.
func (capability Phase0ASelectedOccurrenceCapability) Count() uint64 { return capability.count }

// Phase0ASelectedOccurrenceView is valid only for the duration of its visitor
// callback. Payload identifies the compact record at this occurrence; Parent
// and ChildOrdinal preserve occurrence identity when compact payload IDs are
// repeated. ParseState is the published target boundary and PreGotoState is
// the predecessor/top state authenticated at construction. These states prove
// compact construction provenance only; they do not independently certify
// C-oracle or folded public-tree state metadata. SubtreeCount is the checked
// size of this occurrence's contiguous preorder span, allowing a bounded
// consumer to skip or reverse one occurrence subtree without copying the
// observer proof.
type Phase0ASelectedOccurrenceView struct {
	Ordinal          uint64
	Parent           uint64
	ChildOrdinal     uint32
	Depth            uint32
	Payload          SubtreeID
	ConstructionKind Phase0AConstructionKind
	ChildCount       uint32
	ParseState       StateID
	PreGotoState     StateID
	SubtreeCount     uint64
	Migrated         bool
}

type phase0ASelectedOccurrenceState struct {
	parseState   StateID
	preGotoState StateID
	subtreeCount uint64
}

type phase0ASelectedCountFrame struct {
	id    SubtreeID
	next  uint32
	depth uint32
}

const (
	phase0ASelectedOccurrenceBytes   = uint64(unsafe.Sizeof(Phase0ASelectedOccurrenceRecord{}))
	phase0ASelectedStateBytes        = uint64(unsafe.Sizeof(phase0ASelectedOccurrenceState{}))
	phase0ASelectedTreeBytes         = uint64(unsafe.Sizeof(Phase0ASelectedOccurrenceSnapshot{}))
	phase0ASelectedHardMaxDepth      = uint64(4096)
	phase0ASelectedMapEntryBytes     = uint64(192)
	phase0ASelectedCountScratchBytes = (phase0ASelectedHardMaxDepth + 1) * uint64(unsafe.Sizeof(phase0ASelectedCountFrame{}))
)

// CapturePhase0ASelectedOccurrenceCapability captures the most recent exact
// accepted selection in namespace. It exposes no observer rows and remains
// usable only while that same run generation is active.
func CapturePhase0ASelectedOccurrenceCapability(core *Core, namespace CoreRunNamespace) (Phase0ASelectedOccurrenceCapability, error) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	observer, err := phase0AObserverForNamespaceLocked(core, namespace)
	if err != nil {
		return Phase0ASelectedOccurrenceCapability{}, err
	}
	return capturePhase0ASelectedOccurrenceCapabilityLocked(observer)
}

// CaptureCurrentPhase0ASelectedOccurrenceCapability captures the current
// driver-owned run without exposing its namespace. The returned capability
// still embeds and authenticates the exact run generation.
func CaptureCurrentPhase0ASelectedOccurrenceCapability(core *Core) (Phase0ASelectedOccurrenceCapability, error) {
	phase0AObservers.Lock()
	defer phase0AObservers.Unlock()
	if !phase0AObservers.active {
		return Phase0ASelectedOccurrenceCapability{}, &Phase0AError{Kind: Phase0AErrorSessionInactive}
	}
	observer := phase0AObservers.byCore[core]
	if observer == nil || !observer.active {
		return Phase0ASelectedOccurrenceCapability{}, &Phase0AError{Kind: Phase0AErrorStaleNamespace, Detail: "selected occurrence run is not active"}
	}
	return capturePhase0ASelectedOccurrenceCapabilityLocked(observer)
}

func capturePhase0ASelectedOccurrenceCapabilityLocked(observer *phase0AObserver) (Phase0ASelectedOccurrenceCapability, error) {
	namespace := observer.run
	if observer.failure != nil {
		return Phase0ASelectedOccurrenceCapability{}, phase0AExistingFailureLocked(observer)
	}
	if len(observer.route.selectedTrees) == 0 {
		return Phase0ASelectedOccurrenceCapability{}, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: namespace, Detail: "accepted selection has no selected occurrence tree"}
	}
	tree := observer.route.selectedTrees[len(observer.route.selectedTrees)-1]
	if tree.Namespace != namespace || tree.Generation == 0 || tree.Generation != observer.route.nextSelection {
		return Phase0ASelectedOccurrenceCapability{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected occurrence tree generation is stale"}
	}
	end, carry := bits.Add64(tree.FirstOccurrence, tree.OccurrenceCount, 0)
	if carry != 0 || end > uint64(len(observer.route.selectedOccurrences)) || end > uint64(len(observer.route.selectedStates)) {
		return Phase0ASelectedOccurrenceCapability{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected occurrence span is unavailable"}
	}
	return Phase0ASelectedOccurrenceCapability{
		coreInstance: namespace.CoreInstance, runGeneration: namespace.RunGeneration,
		generation: tree.Generation, first: tree.FirstOccurrence, count: tree.OccurrenceCount,
	}, nil
}

// VisitPhase0ASelectedOccurrences visits one authenticated occurrence preorder
// without copying Phase0AObserverProof or exposing observer-owned slices. A
// bounded run-scoped borrow pins the immutable row/state windows while callbacks
// run without the global observer lock. Lifecycle mutation rejects the active
// borrow; read-only Phase0A APIs may be called from callbacks. Polling occurs
// before the first row, every 256 rows, and before success.
func VisitPhase0ASelectedOccurrences(
	core *Core,
	capability Phase0ASelectedOccurrenceCapability,
	poll func() error,
	visit func(Phase0ASelectedOccurrenceView) error,
) error {
	if visit == nil {
		return &Phase0AError{Kind: Phase0AErrorUnsupportedProof, Detail: "selected occurrence visit requires a callback"}
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	phase0AObservers.Lock()
	namespace := CoreRunNamespace{CoreInstance: capability.coreInstance, RunGeneration: capability.runGeneration}
	observer, err := phase0AObserverForNamespaceLocked(core, namespace)
	if err != nil {
		phase0AObservers.Unlock()
		return err
	}
	if capability.generation == 0 || capability.generation > uint64(len(observer.route.selectedTrees)) {
		phase0AObservers.Unlock()
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected occurrence capability generation is unavailable"}
	}
	tree := observer.route.selectedTrees[capability.generation-1]
	if tree.Namespace != namespace || tree.Generation != capability.generation || tree.FirstOccurrence != capability.first || tree.OccurrenceCount != capability.count {
		phase0AObservers.Unlock()
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected occurrence capability span is stale"}
	}
	limit := phase0ASelectedOccurrenceLimit()
	if limit == 0 || capability.count > limit {
		phase0AObservers.Unlock()
		return &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: namespace, Detail: "selected occurrence capability exceeds row cap"}
	}
	end, carry := bits.Add64(capability.first, capability.count, 0)
	if carry != 0 || end > uint64(len(observer.route.selectedOccurrences)) || end > uint64(len(observer.route.selectedStates)) {
		phase0AObservers.Unlock()
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected occurrence capability span is unavailable"}
	}
	if observer.activeBorrows == math.MaxUint64 {
		phase0AObservers.Unlock()
		return &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: namespace, Detail: "selected occurrence borrow count"}
	}
	rows := observer.route.selectedOccurrences[capability.first:end:end]
	states := observer.route.selectedStates[capability.first:end:end]
	observer.activeBorrows++
	phase0AObservers.Unlock()
	defer func() {
		phase0AObservers.Lock()
		if observer.activeBorrows != 0 {
			observer.activeBorrows--
		}
		phase0AObservers.Unlock()
	}()
	if err := poll(); err != nil {
		return err
	}
	for offset := uint64(0); offset < capability.count; offset++ {
		if offset != 0 && offset&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
		row := rows[offset]
		state := states[offset]
		if row.Ordinal != offset+1 {
			return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected occurrence preorder ordinal is stale"}
		}
		if err := visit(Phase0ASelectedOccurrenceView{
			Ordinal: row.Ordinal, Parent: row.Parent, ChildOrdinal: row.ChildOrdinal, Depth: row.Depth,
			Payload: row.Payload, ConstructionKind: row.ConstructionKind, ChildCount: row.ChildCount,
			ParseState: state.parseState, PreGotoState: state.preGotoState, SubtreeCount: state.subtreeCount, Migrated: row.Migrated,
		}); err != nil {
			return err
		}
	}
	return poll()
}

type phase0AOccurrenceJoin struct {
	occurrence ConstructionOccurrenceKey
	edge       IncomingEdgeKey
}

type phase0ASelectedValue[T any] struct {
	value  T
	live   uint32
	rolled uint32
}

type phase0ASelectedSelectorRoute struct {
	row     Phase0ASelectorRouteRecord
	ordinal uint64
}

type phase0ASelectorRouteKey struct {
	selector Phase0ASelectorID
	lower    LinkID
}

type phase0AMigrationRouteKey struct {
	route   uint64
	ordinal uint32
}

type phase0ASelectedIndex struct {
	routes             map[phase0AOccurrenceJoin]phase0ASelectedValue[Phase0APopRouteRecord]
	routesByID         map[uint64]phase0ASelectedValue[Phase0APopRouteRecord]
	migrations         map[phase0AOccurrenceJoin]phase0ASelectedValue[Phase0ATrailingExtraMigrationRecord]
	migrationsByRoute  map[phase0AMigrationRouteKey]phase0ASelectedValue[Phase0ATrailingExtraMigrationRecord]
	candidates         map[phase0AOccurrenceJoin]phase0ASelectedValue[Phase0ACandidateRecord]
	bindings           map[LinkID]phase0ASelectedValue[Phase0AExpressionID]
	expressions        map[Phase0AExpressionID]phase0ASelectedValue[Phase0AExpressionRecord]
	selectors          map[Phase0ASelectorID]phase0ASelectedValue[Phase0ASelectorRecord]
	selectorRoutes     map[phase0ASelectorRouteKey]phase0ASelectedValue[phase0ASelectedSelectorRoute]
	selectorRouteCount uint64
}

func phase0ASelectedOccurrenceLimit() uint64 {
	limit := phase0AObservers.limits.MaxSelectedOccurrences
	if limit == 0 {
		limit = phase0AObservers.limits.MaxRecords
	}
	return limit
}

func phase0ACountSelectedCompactOccurrencesLocked(core *Core, observer *phase0AObserver, roots []SubtreeID) (uint64, uint64, error) {
	if phase0AObservers.bytes > phase0AObservers.limits.MaxBytes || phase0ASelectedCountScratchBytes > phase0AObservers.limits.MaxBytes-phase0AObservers.bytes {
		return 0, 0, &Phase0AError{Kind: Phase0AErrorByteCap, Namespace: observer.run, Detail: "selected count scratch exceeds remaining observer bytes"}
	}
	var stack [phase0ASelectedHardMaxDepth + 1]phase0ASelectedCountFrame
	limit := phase0ASelectedOccurrenceLimit()
	var count, maxDepth uint64
	for _, root := range roots {
		depth := uint32(0)
		id := root
		stackSize := 0
		for {
			_, children, _, _, err := phase0ASelectedCompactWindows(core, observer.run, id)
			if err != nil {
				return 0, 0, err
			}
			if count == math.MaxUint64 || count >= limit {
				return 0, 0, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected occurrence row cap"}
			}
			count++
			if uint64(depth) > maxDepth {
				maxDepth = uint64(depth)
			}
			if len(children) != 0 {
				if uint64(depth) >= phase0ASelectedHardMaxDepth || stackSize >= len(stack) {
					return 0, 0, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected occurrence depth cap"}
				}
				stack[stackSize] = phase0ASelectedCountFrame{id: id, depth: depth}
				stackSize++
			}
			advanced := false
			for stackSize != 0 {
				top := &stack[stackSize-1]
				_, parentChildren, _, _, err := phase0ASelectedCompactWindows(core, observer.run, top.id)
				if err != nil {
					return 0, 0, err
				}
				if top.next >= uint32(len(parentChildren)) {
					stackSize--
					continue
				}
				child := parentChildren[top.next]
				top.next++
				if child == 0 || child >= top.id {
					return 0, 0, &Phase0AError{Kind: Phase0AErrorCyclicReference, Namespace: observer.run, Detail: "selected compact child violates append-only identity"}
				}
				id, depth, advanced = child, top.depth+1, true
				break
			}
			if !advanced {
				break
			}
		}
	}
	return count, maxDepth, nil
}

// phase0APreflightSelectedIndexLocked conservatively bounds all transient
// maps and the occurrence-record scratch before any of them are allocated.
// It never charges persistent observer accounting, so success and failure
// leave records/bytes unchanged.
func phase0APreflightSelectedIndexLocked(core *Core, observer *phase0AObserver, acceptedLinks, selectedOccurrences, selectedDepth uint64) (uint64, error) {
	ledgerEntries := uint64(0)
	for _, count := range []uint64{
		uint64(len(observer.route.popRoutes)), uint64(len(observer.route.popRoutes)),
		uint64(len(observer.route.migrations)), uint64(len(observer.route.migrations)),
		uint64(len(observer.factor.candidates)), uint64(len(observer.factor.bindings)),
		uint64(len(observer.factor.expressions)), uint64(len(observer.factor.selectors)), uint64(len(observer.factor.selectorRoutes)),
	} {
		var carry uint64
		ledgerEntries, carry = bits.Add64(ledgerEntries, count, 0)
		if carry != 0 {
			return 0, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selected transient ledger entries"}
		}
	}
	occurrenceLimit := phase0ASelectedOccurrenceLimit()
	if occurrenceLimit == 0 || selectedOccurrences > occurrenceLimit {
		return 0, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected occurrence scratch has no bounded row limit"}
	}
	entries, carry := bits.Add64(ledgerEntries, selectedOccurrences, 0)
	entries, carry2 := bits.Add64(entries, selectedOccurrences, 0) // builder active + permanent seen
	entries, carry3 := bits.Add64(entries, selectedOccurrences, 0) // independent digest permanent seen
	if carry != 0 || carry2 != 0 || carry3 != 0 {
		return 0, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selected transient index entries"}
	}
	entryLimit := phase0AObservers.limits.MaxSelectedIndexEntries
	if entryLimit == 0 {
		entryLimit = phase0AObservers.limits.MaxRecords
	}
	if entryLimit == 0 || entries > entryLimit {
		return 0, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected transient index entry cap"}
	}
	mapHi, mapBytes := bits.Mul64(entries, phase0ASelectedMapEntryBytes)
	recordHi, recordBytes := bits.Mul64(selectedOccurrences, phase0ASelectedOccurrenceBytes)
	stateHi, stateBytes := bits.Mul64(selectedOccurrences, phase0ASelectedStateBytes)
	acceptedHi, acceptedBytes := bits.Mul64(acceptedLinks, phase0AAcceptedLinkBytes)
	stackHi, stackBytes := bits.Mul64(selectedDepth+1, 256)
	physicalHi, physicalBytes := bits.Mul64(uint64(core.limits.MaxLinksPerBoundary), uint64(unsafe.Sizeof(phase0APhysicalLink{})))
	transientBytes, byteCarry := bits.Add64(mapBytes, recordBytes, 0)
	transientBytes, byteCarry2 := bits.Add64(transientBytes, stateBytes, 0)
	transientBytes, byteCarry3 := bits.Add64(transientBytes, acceptedBytes, 0)
	transientBytes, byteCarry4 := bits.Add64(transientBytes, stackBytes, 0)
	transientBytes, byteCarry5 := bits.Add64(transientBytes, physicalBytes, 0)
	transientBytes, byteCarry6 := bits.Add64(transientBytes, phase0ASelectedCountScratchBytes, 0)
	if mapHi != 0 || recordHi != 0 || stateHi != 0 || acceptedHi != 0 || stackHi != 0 || physicalHi != 0 || byteCarry != 0 || byteCarry2 != 0 || byteCarry3 != 0 || byteCarry4 != 0 || byteCarry5 != 0 || byteCarry6 != 0 {
		return 0, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selected transient index bytes"}
	}
	byteLimit := phase0AObservers.limits.MaxSelectedIndexBytes
	if byteLimit == 0 {
		byteLimit = phase0AObservers.limits.MaxBytes
	}
	if byteLimit == 0 || transientBytes > byteLimit {
		return 0, &Phase0AError{Kind: Phase0AErrorByteCap, Namespace: observer.run, Detail: "selected transient index byte cap"}
	}
	if phase0AObservers.bytes > phase0AObservers.limits.MaxBytes || transientBytes > phase0AObservers.limits.MaxBytes-phase0AObservers.bytes {
		return 0, &Phase0AError{Kind: Phase0AErrorByteCap, Namespace: observer.run, Detail: "selected transient index exceeds remaining observer bytes"}
	}
	return transientBytes, nil
}

func phase0ABuildSelectedIndex(observer *phase0AObserver) phase0ASelectedIndex {
	index := phase0ASelectedIndex{
		routes:             make(map[phase0AOccurrenceJoin]phase0ASelectedValue[Phase0APopRouteRecord], len(observer.route.popRoutes)),
		routesByID:         make(map[uint64]phase0ASelectedValue[Phase0APopRouteRecord], len(observer.route.popRoutes)),
		migrations:         make(map[phase0AOccurrenceJoin]phase0ASelectedValue[Phase0ATrailingExtraMigrationRecord], len(observer.route.migrations)),
		migrationsByRoute:  make(map[phase0AMigrationRouteKey]phase0ASelectedValue[Phase0ATrailingExtraMigrationRecord], len(observer.route.migrations)),
		candidates:         make(map[phase0AOccurrenceJoin]phase0ASelectedValue[Phase0ACandidateRecord], len(observer.factor.candidates)),
		bindings:           make(map[LinkID]phase0ASelectedValue[Phase0AExpressionID], len(observer.factor.bindings)),
		expressions:        make(map[Phase0AExpressionID]phase0ASelectedValue[Phase0AExpressionRecord], len(observer.factor.expressions)),
		selectors:          make(map[Phase0ASelectorID]phase0ASelectedValue[Phase0ASelectorRecord], len(observer.factor.selectors)),
		selectorRoutes:     make(map[phase0ASelectorRouteKey]phase0ASelectedValue[phase0ASelectedSelectorRoute], len(observer.factor.selectorRoutes)),
		selectorRouteCount: uint64(len(observer.factor.selectorRoutes)),
	}
	for _, row := range observer.route.popRoutes {
		key := phase0AOccurrenceJoin{occurrence: row.Occurrence, edge: row.Edge}
		value := index.routes[key]
		if row.RolledBack {
			value.rolled++
		} else {
			value.live++
			value.value = row
		}
		index.routes[key] = value
		byID := index.routesByID[row.ID]
		if row.RolledBack {
			byID.rolled++
		} else {
			byID.live++
			byID.value = row
		}
		index.routesByID[row.ID] = byID
	}
	for _, row := range observer.route.migrations {
		key := phase0AOccurrenceJoin{occurrence: row.Occurrence, edge: row.Edge}
		value := index.migrations[key]
		if row.RolledBack {
			value.rolled++
		} else {
			value.live++
			value.value = row
		}
		index.migrations[key] = value
		byRouteKey := phase0AMigrationRouteKey{route: row.Route, ordinal: row.TrailingOrdinal}
		byRoute := index.migrationsByRoute[byRouteKey]
		if row.RolledBack {
			byRoute.rolled++
		} else {
			byRoute.live++
			byRoute.value = row
		}
		index.migrationsByRoute[byRouteKey] = byRoute
	}
	for _, row := range observer.factor.candidates {
		key := phase0AOccurrenceJoin{occurrence: row.Occurrence, edge: row.Edge}
		value := index.candidates[key]
		if row.RolledBack {
			value.rolled++
		} else {
			value.live++
			value.value = row
		}
		index.candidates[key] = value
	}
	for _, row := range observer.factor.bindings {
		value := index.bindings[row.Link]
		if row.RolledBack {
			value.rolled++
		} else {
			value.live++
			value.value = row.Expression
		}
		index.bindings[row.Link] = value
	}
	for _, row := range observer.factor.expressions {
		value := index.expressions[row.ID]
		if row.RolledBack {
			value.rolled++
		} else {
			value.live++
			value.value = row
		}
		index.expressions[row.ID] = value
	}
	for _, row := range observer.factor.selectors {
		value := index.selectors[row.ID]
		if row.RolledBack {
			value.rolled++
		} else {
			value.live++
			value.value = row
		}
		index.selectors[row.ID] = value
	}
	for ordinal, row := range observer.factor.selectorRoutes {
		key := phase0ASelectorRouteKey{selector: row.Selector, lower: row.SelectedLowerLink}
		value := index.selectorRoutes[key]
		if row.RolledBack {
			value.rolled++
		} else {
			value.live++
			value.value = phase0ASelectedSelectorRoute{row: row, ordinal: uint64(ordinal)}
		}
		index.selectorRoutes[key] = value
	}
	return index
}

func phase0ASelectedLookupError[T any](observer *phase0AObserver, value phase0ASelectedValue[T], detail string) (T, error) {
	if value.live > 1 {
		var zero T
		return zero, &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: detail}
	}
	if value.live == 0 {
		var zero T
		if value.rolled != 0 {
			return zero, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: detail}
		}
		return zero, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: detail}
	}
	return value.value, nil
}

func phase0AResolveSelectedExpressionLocked(core *Core, observer *phase0AObserver, index phase0ASelectedIndex, initial Phase0AExpressionID, selectedLower LinkID) (Phase0AExpressionRecord, LinkID, error) {
	current := initial
	visited := make(map[Phase0AExpressionID]struct{})
	for steps := 0; steps <= len(index.expressions); steps++ {
		if _, duplicate := visited[current]; duplicate {
			return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorCyclicReference, Namespace: observer.run, Detail: "selected factor expression cycle"}
		}
		visited[current] = struct{}{}
		expression, err := phase0ASelectedLookupError(observer, index.expressions[current], "selected expression is unavailable or non-unique")
		if err != nil {
			return Phase0AExpressionRecord{}, 0, err
		}
		switch expression.Kind {
		case Phase0AExpressionDirect:
			return expression, selectedLower, nil
		case Phase0AExpressionFactorChoice:
			if selectedLower == 0 {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "selected factor expression has no lower link"}
			}
			selector, err := phase0ASelectedLookupError(observer, index.selectors[expression.Selector], "selected factor selector is unavailable or non-unique")
			if err != nil {
				return Phase0AExpressionRecord{}, 0, err
			}
			if selector.ID != expression.Selector || selector.MergedPredecessor == 0 {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected factor selector has invalid merged predecessor"}
			}
			mergedLinks, err := phase0AStablePhysicalLinks(core, selector.MergedPredecessor)
			if err != nil {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected factor merged predecessor is unavailable"}
			}
			containsSelectedLower := false
			for _, link := range mergedLinks {
				if link.id == selectedLower {
					containsSelectedLower = true
					break
				}
			}
			if !containsSelectedLower {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected factor lower link is outside merged predecessor"}
			}
			indexedRoute, err := phase0ASelectedLookupError(observer, index.selectorRoutes[phase0ASelectorRouteKey{selector: expression.Selector, lower: selectedLower}], "selected factor route is unavailable or non-unique")
			if err != nil {
				return Phase0AExpressionRecord{}, 0, err
			}
			windowEnd := selector.FirstRoute + uint64(selector.RouteCount)
			if windowEnd < selector.FirstRoute || windowEnd > index.selectorRouteCount {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected factor selector route window is unavailable"}
			}
			if indexedRoute.ordinal < selector.FirstRoute || indexedRoute.ordinal >= windowEnd || indexedRoute.row.TransactionID != selector.TransactionID || indexedRoute.row.Selector != selector.ID || indexedRoute.row.SelectedLowerLink != selectedLower {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected factor route is outside selector window"}
			}
			route := indexedRoute.row
			if route.SourceLowerLink == 0 || (route.Branch != Phase0ASelectorLeft && route.Branch != Phase0ASelectorRight) {
				return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected factor route has invalid source identity"}
			}
			if route.Branch == Phase0ASelectorLeft {
				current = expression.Left
			} else {
				current = expression.Right
			}
			selectedLower = route.SourceLowerLink
		default:
			return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected expression has unknown kind"}
		}
	}
	return Phase0AExpressionRecord{}, 0, &Phase0AError{Kind: Phase0AErrorCyclicReference, Namespace: observer.run, Detail: "selected factor expression step cap"}
}

func phase0AExactOccurrenceLocked(observer *phase0AObserver, occurrence ConstructionOccurrenceKey, edge IncomingEdgeKey) (Phase0AMutationRecord, Phase0AMutationRecord, error) {
	if occurrence.Event.Attempt.Run != observer.run || edge.Event.Attempt.Run != observer.run ||
		occurrence.Event.Attempt.AttemptEpoch != 1 || edge.Event.Attempt.AttemptEpoch != 1 || edge.Event != occurrence.Event ||
		occurrence.Slot == 0 || occurrence.Slot > observer.nextOccurrenceSlot[occurrence.Event] ||
		occurrence.Event.Serial == 0 || occurrence.Event.Serial > observer.nextEvent || edge.Serial == 0 || edge.Serial > observer.nextEdge {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence namespace or monotonic serial is stale"}
	}
	occurrenceIndex, ok := observer.occurrenceIndex[occurrence]
	if !ok || occurrenceIndex >= uint64(len(observer.mutations)) {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "selected occurrence is not indexed"}
	}
	occurrenceRow := observer.mutations[occurrenceIndex]
	if occurrenceRow.RolledBack {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "selected occurrence is rolled back"}
	}
	if occurrenceRow.Kind != Phase0AMutationOccurrence || occurrenceRow.Occurrence != occurrence || occurrenceRow.Edge != edge {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence index disagrees with its edge"}
	}
	edgeIndex, ok := observer.edgeIndex[edge]
	if !ok || edgeIndex >= uint64(len(observer.mutations)) {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "selected occurrence edge is not indexed"}
	}
	edgeRow := observer.mutations[edgeIndex]
	if edgeRow.RolledBack {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "selected occurrence edge is rolled back"}
	}
	if edgeRow.Kind != Phase0AMutationEdge || edgeRow.Occurrence != occurrence || edgeRow.Edge != edge ||
		edgeRow.Payload != occurrenceRow.Payload || edgeRow.Predecessor != occurrenceRow.Predecessor || edgeRow.ConstructionKind != occurrenceRow.ConstructionKind {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence and edge rows disagree"}
	}
	eventIndex, ok := observer.eventIndex[occurrence.Event]
	if !ok || eventIndex >= uint64(len(observer.mutations)) {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "selected occurrence event is not indexed"}
	}
	event := observer.mutations[eventIndex]
	if event.RolledBack {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "selected occurrence event is rolled back"}
	}
	if event.Kind != Phase0AMutationEvent || event.Event != occurrence.Event || event.Payload != occurrenceRow.Payload || event.ConstructionKind != occurrenceRow.ConstructionKind {
		return Phase0AMutationRecord{}, Phase0AMutationRecord{}, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence event disagrees with its slot"}
	}
	return occurrenceRow, event, nil
}

func phase0AValidateResolvedLowerLocked(core *Core, observer *phase0AObserver, direct Phase0AExpressionRecord, resolvedLower LinkID) error {
	occurrenceIndex, ok := observer.occurrenceIndex[direct.Occurrence]
	if !ok || occurrenceIndex >= uint64(len(observer.mutations)) {
		return &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "resolved direct occurrence is unavailable"}
	}
	predecessor := observer.mutations[occurrenceIndex].Predecessor
	links, err := phase0AStablePhysicalLinks(core, predecessor)
	if err != nil {
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "resolved direct predecessor adjacency is unavailable"}
	}
	if len(links) == 0 {
		if resolvedLower != 0 {
			return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "seed occurrence retained a nonzero resolved lower link"}
		}
		return nil
	}
	if resolvedLower == 0 {
		return &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: observer.run, Detail: "non-seed occurrence lost its resolved lower link"}
	}
	for _, link := range links {
		if link.id == resolvedLower {
			return nil
		}
	}
	return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "resolved lower link belongs to another predecessor"}
}

func phase0AValidateSelectedMigrationLocked(core *Core, observer *phase0AObserver, index phase0ASelectedIndex, migration Phase0ATrailingExtraMigrationRecord) error {
	join := phase0AOccurrenceJoin{occurrence: migration.Occurrence, edge: migration.Edge}
	indexedMigration, err := phase0ASelectedLookupError(observer, index.migrations[join], "selected migration occurrence row is unavailable or non-unique")
	if err != nil {
		return err
	}
	if indexedMigration != migration {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration occurrence index disagrees with its row"}
	}
	indexedRouteMigration, err := phase0ASelectedLookupError(observer, index.migrationsByRoute[phase0AMigrationRouteKey{route: migration.Route, ordinal: migration.TrailingOrdinal}], "selected migration route ordinal is unavailable or non-unique")
	if err != nil {
		return err
	}
	if indexedRouteMigration != migration {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration route index disagrees with its row"}
	}
	route, err := phase0ASelectedLookupError(observer, index.routesByID[migration.Route], "selected migration route is unavailable or non-unique")
	if err != nil {
		return err
	}
	if migration.TransactionID != route.TransactionID || migration.TrailingOrdinal >= route.TrailingLinkCount || route.RetainedLinkCount == 0 {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration route identity is stale"}
	}
	sourceOrdinal := route.RetainedLinkCount + migration.TrailingOrdinal
	source, err := phase0AExactRouteLinkLocked(observer, route, sourceOrdinal)
	if err != nil {
		return err
	}
	lower, err := phase0AExactRouteLinkLocked(observer, route, sourceOrdinal-1)
	if err != nil {
		return err
	}
	if err := phase0AValidateCurrentRouteLink(core, observer, source); err != nil {
		return err
	}
	if err := phase0AValidateCurrentRouteLink(core, observer, lower); err != nil {
		return err
	}
	if source.Segment != Phase0APopRouteTrailing || source.SegmentOrdinal != migration.TrailingOrdinal || lower.Node != source.Predecessor || lower.Ordinal+1 != source.Ordinal ||
		migration.SourceLink != source.Link || migration.SourceLowerLink != lower.Link || migration.Payload != source.Payload {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration source route identity drifted"}
	}
	bound, err := phase0ASelectedLookupError(observer, index.bindings[source.Link], "selected migration source binding is unavailable or non-unique")
	if err != nil {
		return err
	}
	direct, resolvedLower, err := phase0AResolveSelectedExpressionLocked(core, observer, index, bound, lower.Link)
	if err != nil {
		return err
	}
	if direct.ID != migration.SourceExpression || direct.Occurrence != migration.SourceOccurrence || direct.Edge != migration.SourceEdge {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration source expression identity drifted"}
	}
	if err := phase0AValidateDirectOccurrenceLocked(observer, direct); err != nil {
		return err
	}
	if err := phase0AValidateResolvedLowerLocked(core, observer, direct, resolvedLower); err != nil {
		return err
	}
	sourceOccurrence, _, err := phase0AExactOccurrenceLocked(observer, direct.Occurrence, direct.Edge)
	if err != nil {
		return err
	}
	if sourceOccurrence.Payload != source.Payload || sourceOccurrence.ConstructionKind != Phase0AConstructionExtraTerminal {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration source is not an extra-terminal occurrence"}
	}

	expectedOccurrence, expectedEdge := route.Occurrence, route.Edge
	if migration.TrailingOrdinal != 0 {
		prior, err := phase0ASelectedLookupError(observer, index.migrationsByRoute[phase0AMigrationRouteKey{route: route.ID, ordinal: migration.TrailingOrdinal - 1}], "selected prior migration is unavailable or non-unique")
		if err != nil {
			return err
		}
		expectedOccurrence, expectedEdge = prior.Occurrence, prior.Edge
	}
	if migration.TargetOccurrence != expectedOccurrence || migration.TargetEdge != expectedEdge {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration target occurrence identity drifted"}
	}
	targetOccurrence, _, err := phase0AExactOccurrenceLocked(observer, expectedOccurrence, expectedEdge)
	if err != nil {
		return err
	}
	wantTargetKind := Phase0AConstructionReductionParent
	if migration.TrailingOrdinal != 0 {
		wantTargetKind = Phase0AConstructionExtraTerminal
	}
	if targetOccurrence.ConstructionKind != wantTargetKind {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration target construction kind drifted"}
	}
	targetCandidate, err := phase0ASelectedLookupError(observer, index.candidates[phase0AOccurrenceJoin{occurrence: expectedOccurrence, edge: expectedEdge}], "selected migration target candidate is unavailable or non-unique")
	if err != nil {
		return err
	}
	if !targetCandidate.Claimed || !targetCandidate.Resolved || targetCandidate.TransactionID != migration.TransactionID || targetCandidate.TransactionID != route.TransactionID ||
		targetCandidate.Boundary.Frontier != migration.Boundary.Frontier || targetCandidate.Boundary.Checkpoint != migration.Boundary.Checkpoint || targetCandidate.Boundary.Shifted ||
		targetCandidate.Payload != targetOccurrence.Payload || targetCandidate.Predecessor != targetOccurrence.Predecessor {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration target candidate identity drifted"}
	}
	targetLinks, err := phase0AStablePhysicalLinks(core, migration.Predecessor)
	if err != nil || len(targetLinks) != 1 || targetLinks[0].id != migration.TargetLink ||
		targetLinks[0].record.prev != targetCandidate.Predecessor || targetLinks[0].record.payload != targetCandidate.Payload ||
		targetLinks[0].record.scoreDelta != targetCandidate.ScoreDelta || targetLinks[0].record.order != targetCandidate.Order || targetLinks[0].record.hasOrder() != targetCandidate.HasOrder {
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected migration target physical link is stale"}
	}
	targetNode, err := core.node(migration.Predecessor)
	if err != nil || targetNode.state != targetCandidate.Boundary.State || targetNode.byteOffset != targetCandidate.Boundary.ByteOffset {
		return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected migration target node boundary is stale"}
	}
	targetBinding, err := phase0ASelectedLookupError(observer, index.bindings[migration.TargetLink], "selected migration target binding is unavailable or non-unique")
	if err != nil {
		return err
	}
	targetExpression, err := phase0ASelectedLookupError(observer, index.expressions[targetBinding], "selected migration target expression is unavailable or non-unique")
	if err != nil {
		return err
	}
	if targetExpression.Kind != Phase0AExpressionDirect || targetExpression.Occurrence != expectedOccurrence || targetExpression.Edge != expectedEdge ||
		targetExpression.Occurrence != targetCandidate.Occurrence || targetExpression.Edge != targetCandidate.Edge {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration target link identity drifted"}
	}
	if err := phase0AValidateDirectOccurrenceLocked(observer, targetExpression); err != nil {
		return err
	}
	occurrence, _, err := phase0AExactOccurrenceLocked(observer, migration.Occurrence, migration.Edge)
	if err != nil {
		return err
	}
	if occurrence.Payload != migration.Payload || occurrence.Predecessor != migration.Predecessor || occurrence.ConstructionKind != Phase0AConstructionExtraTerminal {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration occurrence identity drifted"}
	}
	candidate, err := phase0ASelectedLookupError(observer, index.candidates[join], "selected migration candidate is unavailable or non-unique")
	if err != nil {
		return err
	}
	if !candidate.Claimed || !candidate.Resolved || candidate.TransactionID != migration.TransactionID || candidate.Boundary != migration.Boundary || candidate.Payload != migration.Payload || candidate.Predecessor != migration.Predecessor ||
		candidate.ScoreDelta != migration.ScoreDelta || candidate.Order != migration.Order || candidate.HasOrder != migration.HasOrder {
		return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected migration publication identity drifted"}
	}
	return nil
}

func phase0AValidateLiveSelectedMigrationsLocked(core *Core, observer *phase0AObserver, index phase0ASelectedIndex) error {
	for _, migration := range observer.route.migrations {
		if migration.RolledBack {
			continue
		}
		if err := phase0AValidateSelectedMigrationLocked(core, observer, index, migration); err != nil {
			return err
		}
	}
	return nil
}

func phase0AResolveSelectedRouteLinkLocked(core *Core, observer *phase0AObserver, index phase0ASelectedIndex, row Phase0APopRouteLinkRecord, selectedLower LinkID) (Phase0AExpressionRecord, LinkID, error) {
	bound, err := phase0ASelectedLookupError(observer, index.bindings[row.Link], "selected route link binding is unavailable or non-unique")
	if err != nil {
		return Phase0AExpressionRecord{}, 0, err
	}
	direct, resolvedLower, err := phase0AResolveSelectedExpressionLocked(core, observer, index, bound, selectedLower)
	if err != nil {
		return Phase0AExpressionRecord{}, 0, err
	}
	if err := phase0AValidateDirectOccurrenceLocked(observer, direct); err != nil {
		return Phase0AExpressionRecord{}, 0, err
	}
	if err := phase0AValidateResolvedLowerLocked(core, observer, direct, resolvedLower); err != nil {
		return Phase0AExpressionRecord{}, 0, err
	}
	return direct, resolvedLower, nil
}

func phase0ABuildSelectedOccurrenceSnapshotLocked(core *Core, observer *phase0AObserver, selectedIndex phase0ASelectedIndex, generation uint64, accepted []Phase0AAcceptedLinkRecord) (Phase0ASelectedOccurrenceSnapshot, []Phase0ASelectedOccurrenceRecord, error) {
	if uint64(len(accepted)) > math.MaxUint32 {
		return Phase0ASelectedOccurrenceSnapshot{}, nil, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "accepted selection root width"}
	}
	occurrenceLimit := phase0ASelectedOccurrenceLimit()
	depthLimit := phase0AObservers.limits.MaxSelectedDepth
	if depthLimit == 0 || depthLimit > phase0ASelectedHardMaxDepth {
		depthLimit = phase0ASelectedHardMaxDepth
	}
	if err := phase0AValidateLiveSelectedMigrationsLocked(core, observer, selectedIndex); err != nil {
		return Phase0ASelectedOccurrenceSnapshot{}, nil, err
	}
	records := make([]Phase0ASelectedOccurrenceRecord, 0, len(accepted))
	active := make(map[phase0AOccurrenceJoin]struct{})
	seen := make(map[phase0AOccurrenceJoin]struct{})
	var snapshot Phase0ASelectedOccurrenceSnapshot
	snapshot.Namespace, snapshot.Generation = observer.run, generation
	for _, root := range accepted {
		if !root.RecoveryDiscontinuity && root.Payload != 0 {
			snapshot.RootCount++
		}
	}
	var visit func(ConstructionOccurrenceKey, IncomingEdgeKey, SubtreeID, LinkID, LinkID, uint64, uint32, uint32) error
	visit = func(occurrence ConstructionOccurrenceKey, edge IncomingEdgeKey, payload SubtreeID, sourceLink, selectedLower LinkID, parent uint64, childOrdinal, depth uint32) error {
		if uint64(depth) > depthLimit {
			return &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected occurrence depth cap"}
		}
		if uint64(len(records)) >= occurrenceLimit || len(records) == math.MaxInt {
			return &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected occurrence row cap"}
		}
		join := phase0AOccurrenceJoin{occurrence: occurrence, edge: edge}
		if _, duplicate := active[join]; duplicate {
			return &Phase0AError{Kind: Phase0AErrorCyclicReference, Namespace: observer.run, Detail: "selected occurrence cycle"}
		}
		if _, duplicate := seen[join]; duplicate {
			return &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "selected occurrence identity appears in multiple positions"}
		}
		mutation, _, err := phase0AExactOccurrenceLocked(observer, occurrence, edge)
		if err != nil {
			return err
		}
		if mutation.Payload != payload {
			return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected physical payload crosses occurrence identity"}
		}
		compact, compactErr := core.subtree(payload)
		if compactErr != nil {
			return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence compact record is unavailable"}
		}
		switch mutation.ConstructionKind {
		case Phase0AConstructionOrdinaryTerminal:
			if !compact.terminal || compact.extra {
				return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "ordinary-terminal occurrence disagrees with compact flags"}
			}
		case Phase0AConstructionExtraTerminal:
			if !compact.terminal || !compact.extra {
				return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "extra-terminal occurrence disagrees with compact flags"}
			}
		case Phase0AConstructionReductionParent:
			if compact.terminal || compact.extra {
				return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "reduction occurrence disagrees with compact flags"}
			}
		default:
			return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence has unknown construction kind"}
		}
		migrationValue := selectedIndex.migrations[join]
		if migrationValue.live > 1 {
			return &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: observer.run, Detail: "selected occurrence has multiple live migration rows"}
		}
		if migrationValue.live == 0 && migrationValue.rolled != 0 {
			return &Phase0AError{Kind: Phase0AErrorRolledBackReference, Namespace: observer.run, Detail: "selected migration occurrence is rolled back"}
		}
		migrated := migrationValue.live == 1
		if migrated {
			if err := phase0AValidateSelectedMigrationLocked(core, observer, selectedIndex, migrationValue.value); err != nil {
				return err
			}
		}
		ordinal := uint64(len(records)) + 1
		records = append(records, Phase0ASelectedOccurrenceRecord{
			Ordinal: ordinal, Parent: parent, ChildOrdinal: childOrdinal, Depth: depth,
			Occurrence: occurrence, Edge: edge, Payload: payload, ConstructionKind: mutation.ConstructionKind,
			SourceLink: sourceLink, SelectedLower: selectedLower, Migrated: migrated,
		})
		if depth > snapshot.MaxDepth {
			snapshot.MaxDepth = depth
		}
		if migrated {
			snapshot.MigratedCount++
		}
		active[join] = struct{}{}
		seen[join] = struct{}{}
		defer delete(active, join)
		switch mutation.ConstructionKind {
		case Phase0AConstructionOrdinaryTerminal, Phase0AConstructionExtraTerminal:
			snapshot.TerminalCount++
			return nil
		case Phase0AConstructionReductionParent:
			snapshot.ReductionCount++
			route, err := phase0ASelectedLookupError(observer, selectedIndex.routes[join], "selected reduction route is unavailable or non-unique")
			if err != nil {
				return err
			}
			if route.Predecessor != mutation.Predecessor || route.LinkCount != route.RetainedLinkCount+route.TrailingLinkCount {
				return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected reduction route crosses its construction boundary"}
			}
			records[ordinal-1].Route = route.ID
			if route.RetainedLinkCount == 0 {
				if route.LinkCount != 0 {
					return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "zero-child reduction retained trailing route links"}
				}
				return nil
			}
			if uint64(depth) >= depthLimit {
				return &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected occurrence depth cap"}
			}
			lower := selectedLower
			var previousNode NodeID
			selectedChildren := uint32(0)
			for index := uint32(0); index < route.LinkCount; index++ {
				row, err := phase0AExactRouteLinkLocked(observer, route, index)
				if err != nil {
					return err
				}
				if err := phase0AValidateCurrentRouteLink(core, observer, row); err != nil {
					return err
				}
				if index == 0 {
					if row.Predecessor != route.Predecessor {
						return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected reduction route starts at another predecessor"}
					}
				} else if row.Predecessor != previousNode {
					return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected reduction route is not one physical chain"}
				}
				previousNode = row.Node
				if index >= route.RetainedLinkCount {
					if row.Segment != Phase0APopRouteTrailing || row.SegmentOrdinal != index-route.RetainedLinkCount {
						return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected trailing route segment identity drifted"}
					}
					continue
				}
				if row.RecoveryDiscontinuity {
					continue
				}
				if row.Segment != Phase0APopRouteRetained || row.SegmentOrdinal != index {
					return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected retained route segment identity drifted"}
				}
				if depth == math.MaxUint32 {
					return &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selected occurrence depth"}
				}
				direct, resolvedLower, err := phase0AResolveSelectedRouteLinkLocked(core, observer, selectedIndex, row, lower)
				if err != nil {
					return err
				}
				if err := visit(direct.Occurrence, direct.Edge, row.Payload, row.Link, resolvedLower, ordinal, index, depth+1); err != nil {
					return err
				}
				selectedChildren++
				lower = row.Link
			}
			if previousNode != route.Head {
				return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected reduction route ends at another head"}
			}
			records[ordinal-1].ChildCount = selectedChildren
			return nil
		default:
			return &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence has unknown construction kind"}
		}
	}

	for index, root := range accepted {
		if root.RecoveryDiscontinuity || root.Payload == 0 {
			continue
		}
		lower := LinkID(0)
		for prior := index - 1; prior >= 0; prior-- {
			if !accepted[prior].RecoveryDiscontinuity && accepted[prior].Payload != 0 {
				lower = accepted[prior].Link
				break
			}
		}
		if root.ResolvedLowerLink != lower && root.BoundExpression == root.ResolvedExpression {
			return Phase0ASelectedOccurrenceSnapshot{}, nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "direct accepted link changed resolved lower identity"}
		}
		if err := visit(root.Occurrence, root.Edge, root.Payload, root.Link, root.ResolvedLowerLink, 0, uint32(index), 0); err != nil {
			return Phase0ASelectedOccurrenceSnapshot{}, nil, err
		}
	}
	snapshot.OccurrenceCount = uint64(len(records))
	if snapshot.TerminalCount+snapshot.ReductionCount != snapshot.OccurrenceCount {
		return Phase0ASelectedOccurrenceSnapshot{}, nil, &Phase0AError{Kind: Phase0AErrorInvalidOccurrence, Namespace: observer.run, Detail: "selected occurrence kind census is incomplete"}
	}
	digest, err := phase0AValidateAndDigestSelectedOccurrences(core, observer.run, records, snapshot.RootCount)
	if err != nil {
		return Phase0ASelectedOccurrenceSnapshot{}, nil, err
	}
	snapshot.Digest = digest
	return snapshot, records, nil
}

func phase0AValidateAndDigestSelectedOccurrences(core *Core, namespace CoreRunNamespace, records []Phase0ASelectedOccurrenceRecord, rootCount uint32) ([sha256.Size]byte, error) {
	h := sha256.New()
	_, _ = h.Write([]byte("gotreesitter-phase0a-selected-occurrences-v1\x00"))
	cursor := 0
	type frame struct {
		ordinal  uint64
		children []SubtreeID
		next     uint32
		depth    uint32
	}
	stack := make([]frame, 0, 32)
	seen := make(map[phase0AOccurrenceJoin]struct{}, len(records))
	enter := func(parent uint64, childOrdinal, depth uint32) error {
		if cursor >= len(records) {
			return &Phase0AError{Kind: Phase0AErrorMissingReference, Namespace: namespace, Detail: "selected preorder ended before its claimed children"}
		}
		row := records[cursor]
		ordinal := uint64(cursor + 1)
		if row.Ordinal != ordinal || row.Parent != parent || row.ChildOrdinal != childOrdinal || row.Depth != depth || row.Parent == ordinal {
			return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: namespace, Detail: "selected preorder parent or child ordinal is unstable"}
		}
		join := phase0AOccurrenceJoin{occurrence: row.Occurrence, edge: row.Edge}
		if _, duplicate := seen[join]; duplicate {
			return &Phase0AError{Kind: Phase0AErrorAmbiguousReference, Namespace: namespace, Detail: "selected occurrence identity appears in multiple positions"}
		}
		seen[join] = struct{}{}
		record, children, fields, aliases, err := phase0ASelectedCompactWindows(core, namespace, row.Payload)
		if err != nil {
			return err
		}
		if record.childCount != row.ChildCount {
			return &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: namespace, Detail: "selected identity children disagree with compact record"}
		}
		phase0AHashCompactRecord(h, record, fields, aliases)
		cursor++
		if len(children) == 0 {
			_, _ = h.Write([]byte{0xff})
			return nil
		}
		if uint64(depth) >= phase0ASelectedHardMaxDepth {
			return &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: namespace, Detail: "selected occurrence depth cap"}
		}
		stack = append(stack, frame{ordinal: ordinal, children: children, depth: depth})
		return nil
	}
	for root := uint32(0); root < rootCount; root++ {
		if err := enter(0, root, 0); err != nil {
			return [sha256.Size]byte{}, err
		}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < uint32(len(top.children)) {
				childOrdinal := top.next
				child := top.children[childOrdinal]
				top.next++
				if child == 0 || child >= records[top.ordinal-1].Payload {
					return [sha256.Size]byte{}, &Phase0AError{Kind: Phase0AErrorCyclicReference, Namespace: namespace, Detail: "selected compact child violates append-only identity"}
				}
				if cursor >= len(records) || child != records[cursor].Payload {
					return [sha256.Size]byte{}, &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: namespace, Detail: "selected physical child order disagrees with compact record"}
				}
				if err := enter(top.ordinal, childOrdinal, top.depth+1); err != nil {
					return [sha256.Size]byte{}, err
				}
				continue
			}
			_, _ = h.Write([]byte{0xff})
			stack = stack[:len(stack)-1]
		}
	}
	if cursor != len(records) {
		return [sha256.Size]byte{}, &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: namespace, Detail: "selected preorder retained unclaimed occurrences"}
	}
	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	return digest, nil
}

func phase0ASelectedOccurrenceStatesLocked(core *Core, observer *phase0AObserver, index phase0ASelectedIndex, records []Phase0ASelectedOccurrenceRecord) ([]phase0ASelectedOccurrenceState, error) {
	states := make([]phase0ASelectedOccurrenceState, len(records))
	for recordIndex, row := range records {
		join := phase0AOccurrenceJoin{occurrence: row.Occurrence, edge: row.Edge}
		candidate, err := phase0ASelectedLookupError(observer, index.candidates[join], "selected occurrence candidate is unavailable or non-unique")
		if err != nil {
			return nil, err
		}
		if candidate.Occurrence != row.Occurrence || candidate.Edge != row.Edge || candidate.Payload != row.Payload || candidate.Predecessor == 0 || !candidate.Claimed || !candidate.Resolved || candidate.RolledBack {
			return nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence candidate state source is stale"}
		}
		predecessor, err := core.node(candidate.Predecessor)
		if err != nil {
			return nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: observer.run, Detail: "selected occurrence predecessor state is unavailable"}
		}
		states[recordIndex] = phase0ASelectedOccurrenceState{
			parseState: candidate.Boundary.State, preGotoState: predecessor.state, subtreeCount: 1,
		}
	}
	for recordIndex := len(records) - 1; recordIndex >= 0; recordIndex-- {
		parent := records[recordIndex].Parent
		if parent == 0 {
			continue
		}
		if parent > uint64(recordIndex) {
			return nil, &Phase0AError{Kind: Phase0AErrorCrossBoundary, Namespace: observer.run, Detail: "selected occurrence parent is outside preorder prefix"}
		}
		parentIndex := parent - 1
		if states[parentIndex].subtreeCount > math.MaxUint64-states[recordIndex].subtreeCount {
			return nil, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selected occurrence subtree count"}
		}
		states[parentIndex].subtreeCount += states[recordIndex].subtreeCount
	}
	return states, nil
}

func phase0ASelectedCompactWindows(core *Core, namespace CoreRunNamespace, id SubtreeID) (subtreeRecord, []SubtreeID, []FieldMapEntry, []Symbol, error) {
	if core == nil || id == 0 || uint64(id) > uint64(len(core.subtrees)) {
		return subtreeRecord{}, nil, nil, nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected compact record is unavailable"}
	}
	record := core.subtrees[id-1]
	if record.externalProvenanceState == subtreeExternalProvenanceReusedOpaque {
		return subtreeRecord{}, nil, nil, nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected occurrence cannot inspect a reused subtree"}
	}
	childEnd := uint64(record.firstChild) + uint64(record.childCount)
	fieldEnd := uint64(record.firstField) + uint64(record.fieldCount)
	aliasEnd := uint64(record.firstAlias) + uint64(record.aliasCount)
	if childEnd > uint64(len(core.children)) {
		return subtreeRecord{}, nil, nil, nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected compact child window is unavailable"}
	}
	if fieldEnd > uint64(len(core.fields)) {
		return subtreeRecord{}, nil, nil, nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected compact field window is unavailable"}
	}
	if aliasEnd > uint64(len(core.aliases)) {
		return subtreeRecord{}, nil, nil, nil, &Phase0AError{Kind: Phase0AErrorStaleReference, Namespace: namespace, Detail: "selected compact alias window is unavailable"}
	}
	return record,
		core.children[record.firstChild:uint32(childEnd)],
		core.fields[record.firstField:uint32(fieldEnd)],
		core.aliases[record.firstAlias:uint32(aliasEnd)], nil
}

func phase0AHashCompactRecord(h hash.Hash, record subtreeRecord, fields []FieldMapEntry, aliases []Symbol) {
	var buf [8]byte
	_, _ = h.Write([]byte{0x01})
	binary.LittleEndian.PutUint16(buf[:2], uint16(record.symbol))
	_, _ = h.Write(buf[:2])
	binary.LittleEndian.PutUint16(buf[:2], record.productionID)
	_, _ = h.Write(buf[:2])
	binary.LittleEndian.PutUint16(buf[:2], uint16(record.dynamicPrecedence))
	_, _ = h.Write(buf[:2])
	binary.LittleEndian.PutUint32(buf[:4], record.startByte)
	_, _ = h.Write(buf[:4])
	binary.LittleEndian.PutUint32(buf[:4], record.endByte)
	_, _ = h.Write(buf[:4])
	binary.LittleEndian.PutUint32(buf[:4], record.childCount)
	_, _ = h.Write(buf[:4])
	binary.LittleEndian.PutUint32(buf[:4], record.fieldCount)
	_, _ = h.Write(buf[:4])
	binary.LittleEndian.PutUint32(buf[:4], record.aliasCount)
	_, _ = h.Write(buf[:4])
	flags := byte(0)
	if record.extra {
		flags |= 1
	}
	if record.external {
		flags |= 2
	}
	if record.terminal {
		flags |= 4
	}
	if record.missing {
		// A new bit, not a new byte: existing records keep their digest.
		flags |= 8
	}
	_, _ = h.Write([]byte{flags})
	for _, field := range fields {
		binary.LittleEndian.PutUint16(buf[:2], uint16(field.FieldID))
		_, _ = h.Write(buf[:2])
		_, _ = h.Write([]byte{field.ChildIndex})
		if field.Inherited {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}
	for _, alias := range aliases {
		binary.LittleEndian.PutUint16(buf[:2], uint16(alias))
		_, _ = h.Write(buf[:2])
	}
}

// phase0ARawSelectedCompactDigest independently walks the selected compact
// roots. It shares only the record encoding with the identity-derived proof;
// traversal and child ordering come directly from the compact arena.
func phase0ARawSelectedCompactDigest(core *Core, roots []SubtreeID) ([sha256.Size]byte, error) {
	if core == nil {
		return [sha256.Size]byte{}, &Phase0AError{Kind: Phase0AErrorInvalidOccurrence, Detail: "raw selected digest requires a core"}
	}
	h := sha256.New()
	_, _ = h.Write([]byte("gotreesitter-phase0a-selected-occurrences-v1\x00"))
	type frame struct {
		id       SubtreeID
		children []SubtreeID
		next     uint32
		depth    uint32
	}
	stack := make([]frame, 0, 32)
	enter := func(id SubtreeID, depth uint32) error {
		record, children, fields, aliases, err := phase0ASelectedCompactWindows(core, CoreRunNamespace{}, id)
		if err != nil {
			return err
		}
		phase0AHashCompactRecord(h, record, fields, aliases)
		if len(children) == 0 {
			_, _ = h.Write([]byte{0xff})
			return nil
		}
		if uint64(depth) >= phase0ASelectedHardMaxDepth {
			return &Phase0AError{Kind: Phase0AErrorRecordCap, Detail: "selected occurrence depth cap"}
		}
		stack = append(stack, frame{id: id, children: children, depth: depth})
		return nil
	}
	for _, root := range roots {
		if err := enter(root, 0); err != nil {
			return [sha256.Size]byte{}, err
		}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < uint32(len(top.children)) {
				child := top.children[top.next]
				top.next++
				if child == 0 || child >= top.id {
					return [sha256.Size]byte{}, &Phase0AError{Kind: Phase0AErrorCyclicReference, Detail: "raw selected compact child violates append-only identity"}
				}
				if err := enter(child, top.depth+1); err != nil {
					return [sha256.Size]byte{}, err
				}
				continue
			}
			_, _ = h.Write([]byte{0xff})
			stack = stack[:len(stack)-1]
		}
	}
	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	return digest, nil
}

func phase0AReserveAcceptedSelectionLocked(observer *phase0AObserver, acceptedLinks, selectedOccurrences uint64) error {
	if err := phase0ACheckRouteRowsLocked(observer, 0, 0, 0, 1, acceptedLinks); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	limit := phase0AObservers.limits.MaxSelectedOccurrences
	if limit == 0 {
		limit = phase0AObservers.limits.MaxRecords
	}
	if limit == 0 {
		limit = math.MaxUint64
	}
	if uint64(len(observer.route.selectedOccurrences)) > limit || selectedOccurrences > limit-uint64(len(observer.route.selectedOccurrences)) {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorRecordCap, Namespace: observer.run, Detail: "selected occurrence row cap"})
	}
	records, carry := bits.Add64(acceptedLinks, selectedOccurrences, 0)
	records, carry2 := bits.Add64(records, selectedOccurrences, 0)
	records, carry3 := bits.Add64(records, 2, 0)
	if carry != 0 || carry2 != 0 || carry3 != 0 {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selected occurrence accounting rows"})
	}
	acceptedHi, acceptedBytes := bits.Mul64(acceptedLinks, phase0AAcceptedLinkBytes)
	selectedHi, selectedBytes := bits.Mul64(selectedOccurrences, phase0ASelectedOccurrenceBytes)
	stateHi, stateBytes := bits.Mul64(selectedOccurrences, phase0ASelectedStateBytes)
	bytes, byteCarry := bits.Add64(phase0AAcceptedSelectionBytes, phase0ASelectedTreeBytes, 0)
	bytes, byteCarry2 := bits.Add64(bytes, acceptedBytes, 0)
	bytes, byteCarry3 := bits.Add64(bytes, selectedBytes, 0)
	bytes, byteCarry4 := bits.Add64(bytes, stateBytes, 0)
	if acceptedHi != 0 || selectedHi != 0 || stateHi != 0 || byteCarry != 0 || byteCarry2 != 0 || byteCarry3 != 0 || byteCarry4 != 0 {
		return phase0AStickyLocked(observer, &Phase0AError{Kind: Phase0AErrorCounterOverflow, Namespace: observer.run, Detail: "selected occurrence accounting bytes"})
	}
	if err := phase0AChargeManyLocked(records, bytes); err != nil {
		return phase0AStickyLocked(observer, err.(*Phase0AError))
	}
	return nil
}

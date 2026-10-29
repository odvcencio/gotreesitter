package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
)

// buildSelectedStoreOnePass traverses accepted compact occurrences in
// iterative postorder and emits their logical result directly into the sealed
// store. The three parallel frame slices are existing Core-owned scratch:
// stack carries next-child/logical-window state, collect carries payload IDs,
// and rawRoots carries packed alias/field provenance.
func (c *Core) buildSelectedStoreOnePass(roots []SubtreeID, policy SelectedStorePolicy, source []byte, poll func() error) (*SelectedStore, error) {
	if c != nil && len(c.reusedSubtrees) != 0 {
		return nil, errors.New("parser-core phase zero: selected store cannot expand reused subtrees")
	}
	if c == nil || len(roots) == 0 {
		return nil, errors.New("parser-core phase zero: selected store requires accepted roots")
	}
	if err := policy.validate(); err != nil {
		return nil, err
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return nil, err
	}

	defer c.finishSelectedBuildScratch()
	var reserve int
	if policy.transientProjection {
		reserve = policy.projectionCapacityHint
		if reserve == 0 {
			// Estimate potential public records from existing raw headers. This
			// avoids a replay walk merely to size the temporary construction plan.
			for index := range c.subtrees.Len() {
				record := c.subtrees.At(index)
				meta, ok := policy.symbol(record.symbol)
				if record.terminal || record.extra || ok && meta.Visible {
					reserve++
				}
				if index&255 == 0 {
					if err := poll(); err != nil {
						return nil, err
					}
				}
			}
		}
		if uint64(reserve) > uint64(c.limits.MaxSelectedOccurrences) {
			reserve = int(c.limits.MaxSelectedOccurrences)
		}
		// This is a capacity hint, not a bound on expanded occurrences. Shared
		// payloads and retained aliases still use the checked growth path.
		if selectedStoreRetainedBytesWithSources(reserve, reserve, reserve) > c.limits.MaxSelectedBytes {
			reserve = 0
		}
	}
	store := c.acquireSelectedStoreWithSources(reserve, reserve, policy.transientProjection)
	sealed := false
	defer func() {
		if !sealed {
			store.Release()
		}
	}()

	precedenceDeltas := c.selectedBuild.precedence[:0]
	logical := c.selectedBuild.logical[:0]
	rootIDs := c.selectedBuild.rootIDs[:0]
	stack := c.selectedBuild.stack[:0]
	if cap(stack) < 64 {
		stack = make([]selectedOccurrenceFrame, 0, 64)
	}
	payloads := c.selectedBuild.collect[:0]
	if cap(payloads) < 64 {
		payloads = make([]uint32, 0, 64)
	}
	edges := c.selectedBuild.rawRoots[:0]
	if cap(edges) < 64 {
		edges = make([]uint32, 0, 64)
	}
	c.selectedBuild.precedence = precedenceDeltas
	c.selectedBuild.logical = logical
	c.selectedBuild.rootIDs = rootIDs
	c.selectedBuild.stack = stack
	c.selectedBuild.collect = payloads
	c.selectedBuild.rawRoots = edges

	var occurrences, rawChildSlots, work uint64
	push := func(payload SubtreeID, alias Symbol, field FieldID) error {
		record, err := c.subtree(payload)
		if err != nil {
			return err
		}
		if record.childCount > math.MaxUint16 || occurrences+1 > uint64(c.limits.MaxSelectedOccurrences) ||
			rawChildSlots+uint64(record.childCount) > uint64(c.limits.MaxSelectedOccurrences) {
			return errors.New("parser-core phase zero: raw selected occurrence cap")
		}
		if occurrences >= math.MaxUint32 || rawChildSlots+uint64(record.childCount) > math.MaxUint32 {
			return errors.New("parser-core phase zero: raw selected occurrence exceeds arena")
		}
		if uint64(len(logical)) > math.MaxUint32 {
			return errors.New("parser-core phase zero: selected logical output exceeds arena")
		}
		occurrences++
		rawChildSlots += uint64(record.childCount)
		stack = append(stack, selectedOccurrenceFrame{occurrence: uint32(len(logical))})
		payloads = append(payloads, uint32(payload))
		edges = append(edges, uint32(alias)|uint32(field)<<16)
		return nil
	}

	for _, root := range roots {
		rootStart := len(logical)
		if err := push(root, 0, 0); err != nil {
			return nil, err
		}
		for len(stack) != 0 {
			work++
			if work&255 == 0 {
				if err := poll(); err != nil {
					return nil, err
				}
			}
			frameIndex := len(stack) - 1
			frame := &stack[frameIndex]
			record, err := c.subtree(SubtreeID(payloads[frameIndex]))
			if err != nil {
				return nil, err
			}
			if frame.next < record.childCount {
				ordinal := frame.next
				frame.next++
				payload := c.children[record.firstChild+ordinal]
				var alias Symbol
				if record.aliasCount != 0 {
					if record.aliasCount != record.childCount {
						return nil, errors.New("parser-core phase zero: selected alias width drifted")
					}
					alias = c.aliases[record.firstAlias+ordinal]
				}
				var field FieldID
				for _, entry := range c.fields[record.firstField : record.firstField+record.fieldCount] {
					if uint32(entry.ChildIndex) != ordinal {
						continue
					}
					if entry.Inherited || field != 0 {
						return nil, errors.New("parser-core phase zero: selected field profile is outside admitted direct single-field scope")
					}
					field = entry.FieldID
				}
				if err := push(payload, alias, field); err != nil {
					return nil, err
				}
				continue
			}

			logicalStart := int(frame.occurrence)
			packedEdge := edges[frameIndex]
			alias := Symbol(packedEdge)
			field := FieldID(packedEdge >> 16)
			retainAliasChild := !record.extra && policy.retainsAliasChild(alias, record.symbol)
			symbol := record.symbol
			if alias != 0 && !retainAliasChild {
				symbol = alias
			}
			meta, ok := policy.symbol(symbol)
			if !ok {
				return nil, fmt.Errorf("parser-core phase zero: selected symbol %d is outside policy", symbol)
			}

			hasError := frame.hasError || record.missing || record.symbol == ErrorRegionSymbol
			folded := false
			if !record.terminal && record.childCount == 1 && record.fieldCount == 0 && record.aliasCount == 0 && len(logical)-logicalStart == 1 {
				childID := logical[logicalStart]
				child := &store.records[childID-1]
				rule := policy.unary(record.symbol, child.Symbol)
				if !child.Extra() && (rule == SelectedUnaryPass || rule == SelectedUnaryRenameLeaf && child.ChildCount == 0) {
					store.setPayloadSource(childID, SubtreeID(payloads[frameIndex]))
					if record.fragile {
						child.flags |= selectedNodeFlagFragile
					}
					if rule == SelectedUnaryPass && record.symbol != child.Symbol {
						child.SupertypeMask |= policy.supertypeMask(record.symbol)
					}
					if hasError {
						child.flags |= selectedNodeFlagHasError
					}
					if rule == SelectedUnaryRenameLeaf {
						child.Symbol = record.symbol
						child.flags = selectedFlags(policy.Symbols[record.symbol], child.Extra(), child.External(), child.Terminal(), child.Missing(), child.HasError()) | child.flags&selectedNodeFlagFragile
					}
					if alias != 0 && !retainAliasChild {
						child.Symbol = alias
						child.flags = selectedFlags(meta, child.Extra(), child.External(), child.Terminal(), child.Missing(), child.HasError()) | child.flags&selectedNodeFlagFragile
					}
					child.ProductionID = record.productionID
					delta, err := checkedSelectedPrecedence(precedenceDeltas[childID-1], int32(record.dynamicPrecedence))
					if err != nil {
						return nil, err
					}
					precedenceDeltas[childID-1] = delta
					if field != 0 && !retainAliasChild {
						child.Field = field
					}
					logical = logical[:logicalStart+1]
					logical[logicalStart] = childID
					folded = true
				}
			}
			if folded && retainAliasChild {
				if err := ensureSelectedOnePassCapacity(store, len(store.records)+1, len(store.children)+1, c.limits.MaxSelectedBytes, poll); err != nil {
					return nil, err
				}
				childID := logical[logicalStart]
				var err error
				childID, precedenceDeltas, err = appendSelectedRetainedAliasWrapper(store, policy, childID, alias, field, record.productionID, precedenceDeltas)
				if err != nil {
					return nil, err
				}
				logical[logicalStart] = childID
			}

			if !folded && !meta.Visible && !record.extra {
				if mask := policy.supertypeMask(record.symbol); mask != 0 {
					for _, id := range logical[logicalStart:] {
						store.records[id-1].SupertypeMask |= mask
					}
				}
			}
			if !folded && (meta.Visible || record.extra) {
				childCount := len(logical) - logicalStart
				if childCount > math.MaxUint16 || uint64(len(store.children))+uint64(childCount) > math.MaxUint32 || uint64(len(store.records)) >= math.MaxUint32 {
					return nil, errors.New("parser-core phase zero: selected store exceeded uint32 arena")
				}
				if err := ensureSelectedOnePassCapacity(store, len(store.records)+1, len(store.children)+childCount, c.limits.MaxSelectedBytes, poll); err != nil {
					return nil, err
				}
				id := SelectedNodeID(uint64(len(store.records)) + 1)
				first := uint32(len(store.children))
				children := logical[logicalStart:]
				store.children = append(store.children, children...)
				flags := selectedFlags(meta, record.extra, record.external, record.terminal, record.missing, hasError)
				if record.fragile {
					flags |= selectedNodeFlagFragile
				}
				startByte, endByte := record.startByte, record.endByte
				if len(children) != 0 {
					firstChild := store.records[children[0]-1]
					lastChild := store.records[children[len(children)-1]-1]
					if firstChild.StartByte < startByte {
						startByte = firstChild.StartByte
					}
					if lastChild.EndByte > endByte {
						endByte = lastChild.EndByte
					}
				}
				incomingField := field
				if retainAliasChild {
					incomingField = 0
				}
				store.records = append(store.records, SelectedNodeRecord{
					FirstChild: first, StartByte: startByte, EndByte: endByte,
					Symbol: symbol, Field: incomingField,
					ProductionID: record.productionID, DynamicPrecedence: int32(record.dynamicPrecedence),
					ChildCount: uint16(len(children)), flags: flags,
				})
				precedenceDeltas = append(precedenceDeltas, int32(record.dynamicPrecedence))
				store.appendPayloadSource(SubtreeID(payloads[frameIndex]))
				for childIndex, childID := range children {
					store.records[childID-1].Parent = id
					store.records[childID-1].ChildIndex = uint16(childIndex)
				}
				logical = logical[:logicalStart]
				logical = append(logical, id)
				if retainAliasChild {
					if err := ensureSelectedOnePassCapacity(store, len(store.records)+1, len(store.children)+1, c.limits.MaxSelectedBytes, poll); err != nil {
						return nil, err
					}
					var err error
					id, precedenceDeltas, err = appendSelectedRetainedAliasWrapper(store, policy, id, alias, field, record.productionID, precedenceDeltas)
					if err != nil {
						return nil, err
					}
					logical[logicalStart] = id
				}
			}
			if field != 0 && !retainAliasChild {
				store.applyDirectField(logical[logicalStart:], field)
			}
			if frameIndex > 0 && hasError {
				stack[frameIndex-1].hasError = true
			}
			stack = stack[:frameIndex]
			payloads = payloads[:frameIndex]
			edges = edges[:frameIndex]
		}
		rootIDs = append(rootIDs, logical[rootStart:]...)
		logical = logical[:rootStart]
	}

	// Drop record reserve before root merging so its retained-cap preflight is
	// charged to logical nodes, not hidden/collapsed raw occurrences. Keep the
	// already-admitted child reserve until the final seal to avoid copying the
	// edge arena twice when compatibility normalization removes children.
	if !policy.transientProjection {
		if err := rightSizeSelectedOnePassStore(store, len(store.records), cap(store.children), c.limits.MaxSelectedBytes, poll); err != nil {
			return nil, err
		}
	}
	if err := store.finishRoots(rootIDs, policy, precedenceDeltas, c.limits.MaxSelectedBytes, poll); err != nil {
		return nil, err
	}
	if err := store.normalizeGoCompatibility(policy, source, poll); err != nil {
		return nil, err
	}
	if err := store.normalizeResultRootSourceStart(policy, source); err != nil {
		return nil, err
	}
	if err := store.recomputePrecedence(precedenceDeltas); err != nil {
		return nil, err
	}
	if !policy.transientProjection {
		if err := rightSizeSelectedOnePassStore(store, len(store.records), len(store.children), c.limits.MaxSelectedBytes, poll); err != nil {
			return nil, err
		}
	}
	if err := poll(); err != nil {
		return nil, err
	}
	c.selectedBuild.precedence = precedenceDeltas
	c.selectedBuild.logical = logical
	c.selectedBuild.rootIDs = rootIDs
	c.selectedBuild.stack = stack
	c.selectedBuild.collect = payloads
	c.selectedBuild.rawRoots = edges
	sealed = true
	return store, nil
}

func ensureSelectedOnePassCapacity(store *SelectedStore, recordNeed, childNeed int, retainedCap uint64, poll func() error) error {
	recordCapacity := selectedOnePassGrowth(cap(store.records), recordNeed)
	childCapacity := selectedOnePassGrowth(cap(store.children), childNeed)
	if store.retainedBytesForCapacity(recordCapacity, childCapacity) > retainedCap {
		// Tight test/application caps may not admit the normal geometric reserve.
		// Fall back to exactly the currently required pair before declining.
		recordCapacity, childCapacity = max(cap(store.records), recordNeed), max(cap(store.children), childNeed)
		if store.retainedBytesForCapacity(recordCapacity, childCapacity) > retainedCap {
			return errors.New("parser-core phase zero: selected store retained-byte cap")
		}
	}
	if recordCapacity != cap(store.records) {
		records := make([]SelectedNodeRecord, len(store.records), recordCapacity)
		if err := copySelectedStoreChunks(records, store.records, poll); err != nil {
			return err
		}
		store.records = records
		if store.payloadSources != nil {
			sources := make([]SubtreeID, len(store.payloadSources), recordCapacity)
			if err := copySelectedStoreChunks(sources, store.payloadSources, poll); err != nil {
				return err
			}
			store.payloadSources = sources
		}
	}
	if childCapacity != cap(store.children) {
		children := make([]SelectedNodeID, len(store.children), childCapacity)
		if err := copySelectedStoreChunks(children, store.children, poll); err != nil {
			return err
		}
		store.children = children
	}
	return nil
}

func selectedOnePassGrowth(capacity, need int) int {
	if capacity >= need {
		return capacity
	}
	next := capacity * 2
	if next < 64 {
		next = 64
	}
	if next < need {
		next = need
	}
	return next
}

func rightSizeSelectedOnePassStore(store *SelectedStore, recordCapacity, childCapacity int, retainedCap uint64, poll func() error) error {
	if recordCapacity < len(store.records) || childCapacity < len(store.children) ||
		store.retainedBytesForCapacity(recordCapacity, childCapacity) > retainedCap {
		return errors.New("parser-core phase zero: selected store retained-byte cap")
	}
	if cap(store.records) != recordCapacity {
		records := make([]SelectedNodeRecord, len(store.records), recordCapacity)
		if err := copySelectedStoreChunks(records, store.records, poll); err != nil {
			return err
		}
		store.records = records
		if store.payloadSources != nil {
			sources := make([]SubtreeID, len(store.payloadSources), recordCapacity)
			if err := copySelectedStoreChunks(sources, store.payloadSources, poll); err != nil {
				return err
			}
			store.payloadSources = sources
		}
	}
	if cap(store.children) != childCapacity {
		children := make([]SelectedNodeID, len(store.children), childCapacity)
		if err := copySelectedStoreChunks(children, store.children, poll); err != nil {
			return err
		}
		store.children = children
	}
	return nil
}

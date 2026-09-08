package parsercorephase0

import "errors"

type materializationPostorderFrame struct {
	id     SubtreeID
	next   uint32
	record *subtreeRecord
	// The replay fields carry the top-down parse-state replay through the
	// same traversal. cursor is the state after the children visited so far,
	// which is the next child's pre-goto state. pre and state are this
	// subtree's own replay results; the Known flags follow the replay rules.
	cursor     StateID
	pre        StateID
	state      StateID
	preKnown   bool
	stateKnown bool
}

// MaterializationReplayTransition computes the parse state reached after one
// subtree is pushed from pre, and reports whether the tables held that
// transition. It mirrors the root package's replay transition, so the fused
// visit assigns the same states as the separate top-down replay.
type MaterializationReplayTransition func(pre StateID, view MaterializationReplayView) (StateID, bool, error)

// MaterializationPostorderScratch retains the transient ownership colors and
// iterative traversal stack for VisitMaterializationPostorderWithScratch.
// Callers must not use one scratch concurrently or reentrantly.
type MaterializationPostorderScratch struct {
	colors []uint8
	frames []materializationPostorderFrame
	inUse  bool
}

// Reset clears traversal state while retaining the backing storage for reuse.
func (scratch *MaterializationPostorderScratch) Reset() {
	if scratch == nil {
		return
	}
	clear(scratch.colors)
	scratch.colors = scratch.colors[:0]
	if cap(scratch.frames) > 0 {
		clear(scratch.frames[:cap(scratch.frames)])
	}
	scratch.frames = scratch.frames[:0]
	scratch.inUse = false
}

func (scratch *MaterializationPostorderScratch) prepare(subtrees int) error {
	if scratch == nil {
		return errors.New("parser-core phase zero: materialization postorder scratch is nil")
	}
	if scratch.inUse {
		return errors.New("parser-core phase zero: materialization postorder scratch is already in use")
	}
	colorCount := subtrees + 1
	if cap(scratch.colors) < colorCount {
		scratch.colors = make([]uint8, colorCount)
	} else {
		scratch.colors = scratch.colors[:colorCount]
	}
	if cap(scratch.frames) < 64 {
		scratch.frames = make([]materializationPostorderFrame, 0, 64)
	} else {
		scratch.frames = scratch.frames[:0]
	}
	scratch.inUse = true
	return nil
}

// VisitMaterializationPostorderWithScratch authenticates roots and visits each
// subtree after its children. It retains caller-owned traversal storage.
func (c *Core) VisitMaterializationPostorderWithScratch(
	roots []SubtreeID,
	poll func() error,
	scratch *MaterializationPostorderScratch,
	visit func(SubtreeID, MaterializationSubtreeView) error,
) error {
	return c.VisitMaterializationPostorderWithReplay(roots, poll, scratch, 0, nil, visit)
}

// VisitMaterializationPostorderWithReplay is VisitMaterializationPostorderWithScratch
// with the top-down parse-state replay fused into the same traversal. When
// transition is non-nil, every root starts from rootPre, each child's pre-goto
// state is the state after its previous sibling, and the visit receives each
// subtree's replay states in the view. A nil transition visits without replay.
func (c *Core) VisitMaterializationPostorderWithReplay(
	roots []SubtreeID,
	poll func() error,
	scratch *MaterializationPostorderScratch,
	rootPre StateID,
	transition MaterializationReplayTransition,
	visit func(SubtreeID, MaterializationSubtreeView) error,
) error {
	if c == nil || len(roots) == 0 {
		return errors.New("parser-core phase zero: materialization requires at least one compact root")
	}
	if visit == nil {
		return errors.New("parser-core phase zero: materialization requires a visitor")
	}
	if scratch == nil {
		return errors.New("parser-core phase zero: materialization postorder scratch is nil")
	}
	if scratch.inUse {
		return errors.New("parser-core phase zero: materialization postorder scratch is already in use")
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return err
	}
	if err := scratch.prepare(len(c.subtrees)); err != nil {
		return err
	}
	defer scratch.Reset()

	colors := scratch.colors
	var reusedOwners map[uint32]SubtreeID
	if len(c.reusedSubtrees) != 0 {
		reusedOwners = make(map[uint32]SubtreeID)
	}
	var visited, work uint64
	for _, root := range roots {
		record, err := c.subtree(root)
		if err != nil {
			return err
		}
		if colors[root] != 0 {
			return errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
		}
		colors[root] = 1
		rootFrame := materializationPostorderFrame{id: root, record: record}
		if transition != nil {
			state, known, err := transition(rootPre, c.materializationReplayViewForRecord(root, record))
			if err != nil {
				return err
			}
			rootFrame.pre, rootFrame.cursor = rootPre, rootPre
			rootFrame.state, rootFrame.stateKnown = state, known
			rootFrame.preKnown = known && !record.extra
		}
		scratch.frames = append(scratch.frames, rootFrame)
		for len(scratch.frames) != 0 {
			work++
			if work&255 == 0 {
				if err := poll(); err != nil {
					return err
				}
			}
			top := &scratch.frames[len(scratch.frames)-1]
			record := top.record
			if top.next < record.childCount {
				child := c.children[record.firstChild+top.next]
				top.next++
				childRecord, err := c.subtree(child)
				if err != nil {
					return err
				}
				switch colors[child] {
				case 0:
					colors[child] = 1
					childFrame := materializationPostorderFrame{id: child, record: childRecord}
					if transition != nil {
						childPre := top.cursor
						state, known, err := transition(childPre, c.materializationReplayViewForRecord(child, childRecord))
						if err != nil {
							return err
						}
						// The replay advances the parent's cursor by the child's
						// result even when the tables held no transition.
						top.cursor = state
						childFrame.pre, childFrame.cursor = childPre, childPre
						childFrame.state, childFrame.stateKnown = state, known
						childFrame.preKnown = known && !childRecord.extra
					}
					scratch.frames = append(scratch.frames, childFrame)
					continue
				case 1:
					return errors.New("parser-core phase zero: compact subtree cycle during materialization")
				default:
					return errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
				}
			}
			if err := c.validateMaterializationMetadata(top.id, record); err != nil {
				return err
			}
			if err := c.claimReusedOwnership(top.id, reusedOwners); err != nil {
				return err
			}
			view := MaterializationSubtreeView{
				Symbol: record.symbol, ProductionID: record.productionID,
				DynamicPrecedence: int32(record.dynamicPrecedence),
				StartByte:         record.startByte, EndByte: record.endByte,
				Children: c.children[record.firstChild : record.firstChild+record.childCount],
				Extra:    record.extra, External: record.external, Terminal: record.terminal,
				Fragile: record.fragile, Missing: record.missing,
			}
			if record.terminal {
				if provenance, ok := c.externalPayloadScannerProvenance(top.id); ok {
					view.ExternalScannerCheckpointStart = provenance.start
					view.ExternalScannerCheckpointEnd = provenance.end
					view.ExternalScannerCheckpointExact = true
				}
			}
			if record.missing {
				view.MissingDependency, view.MissingDependencyExact = c.missingLeafDependency(top.id)
			}
			view.LexerSkippedPrefixStart, view.LexerSkippedPrefix = c.lexerSkippedPrefix(top.id)
			if transition != nil {
				view.ReplayPreGotoState, view.ReplayParseState = top.pre, top.state
				view.ReplayPreGotoKnown, view.ReplayParseStateKnown = top.preKnown, top.stateKnown
			}
			c.applyReusedMaterializationView(top.id, &view)
			if err := visit(top.id, view); err != nil {
				return err
			}
			colors[top.id] = 2
			visited++
			scratch.frames = scratch.frames[:len(scratch.frames)-1]
		}
	}
	if visited > uint64(len(c.subtrees)) {
		return errors.New("parser-core phase zero: materialization exceeded compact subtree arena")
	}
	return poll()
}

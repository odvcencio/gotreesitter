package parsercorephase0

import "errors"

type materializationPostorderFrame struct {
	id     SubtreeID
	next   uint32
	record *subtreeRecord
}

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
		scratch.frames = append(scratch.frames, materializationPostorderFrame{id: root, record: record})
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
					scratch.frames = append(scratch.frames, materializationPostorderFrame{id: child, record: childRecord})
					continue
				case 1:
					return errors.New("parser-core phase zero: compact subtree cycle during materialization")
				default:
					return errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
				}
			}
			if err := c.validateMaterializationMetadata(top.id, *record); err != nil {
				return err
			}
			view := MaterializationSubtreeView{
				Symbol: record.symbol, ProductionID: record.productionID,
				DynamicPrecedence: record.dynamicPrecedence,
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
			view.LexerSkippedPrefixStart, view.LexerSkippedPrefix = c.lexerSkippedPrefix(top.id)
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

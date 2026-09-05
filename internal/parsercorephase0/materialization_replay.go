package parsercorephase0

// MaterializationReplayChildren is a borrowed compact-child view for one
// parse-state replay traversal. It remains valid until the Core is reset.
// Callers must not retain it past that boundary.
type MaterializationReplayChildren struct {
	first uint32
	count uint32
}

// Len returns the number of compact children.
func (c MaterializationReplayChildren) Len() uint32 {
	return c.count
}

// At returns one compact child without copying the child arena.
func (c *Core) MaterializationReplayChild(children MaterializationReplayChildren, index uint32) (SubtreeID, bool) {
	if c == nil || index >= children.count {
		return 0, false
	}
	childIndex := uint64(children.first) + uint64(index)
	if childIndex >= uint64(len(c.children)) {
		return 0, false
	}
	return c.children[childIndex], true
}

// MaterializationReplayView exposes only the compact fields that parse-state
// replay consumes.
type MaterializationReplayView struct {
	ReusedKey                       uint32
	ReusedPreGotoState, ReusedState StateID
	Symbol                          Symbol
	Children                        MaterializationReplayChildren
	Extra                           bool
	Terminal                        bool
}

// MaterializationReplayView returns one borrowed parse-state replay view.
func (c *Core) MaterializationReplayView(id SubtreeID) (MaterializationReplayView, error) {
	record, err := c.subtree(id)
	if err != nil {
		return MaterializationReplayView{}, err
	}
	view := MaterializationReplayView{
		Symbol: record.symbol,
		Children: MaterializationReplayChildren{
			first: record.firstChild,
			count: record.childCount,
		},
		Extra:    record.extra,
		Terminal: record.terminal,
	}
	if reused, ok := c.reusedSubtree(id); ok {
		view.ReusedKey = reused.Key
		view.ReusedPreGotoState, view.ReusedState = reused.PreGotoState, reused.State
	}
	return view, nil
}

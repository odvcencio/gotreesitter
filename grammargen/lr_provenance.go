package grammargen

// mergeOrigin records that a state received merged lookaheads from
// a particular kernel (in the full LR(1) path) or was targeted by
// LALR lookahead propagation.
type mergeOrigin struct {
	kernelHash  uint64 // hash of the incoming kernel items
	sourceState int    // source state index (-1 if unknown/LALR)
}

// mergeProvenance tracks merge history for LALR states.
// This is diagnostic metadata — it does not affect table construction.
type mergeProvenance struct {
	fresh          map[int]bool
	merges         map[int][]mergeOrigin
	laContributors map[int]map[int][]int
}

func newMergeProvenance() *mergeProvenance {
	return &mergeProvenance{
		fresh:  make(map[int]bool),
		merges: make(map[int][]mergeOrigin),
	}
}

func (p *mergeProvenance) recordFresh(stateIdx int) {
	p.fresh[stateIdx] = true
}

func (p *mergeProvenance) recordMerge(stateIdx int, origin mergeOrigin) {
	p.merges[stateIdx] = append(p.merges[stateIdx], origin)
}

func (p *mergeProvenance) isMerged(stateIdx int) bool {
	return len(p.merges[stateIdx]) > 0
}

func (p *mergeProvenance) origins(stateIdx int) []mergeOrigin {
	return p.merges[stateIdx]
}

func (p *mergeProvenance) recordLookaheadContributor(stateIdx, lookahead, ntTransIdx int) {
	if p.laContributors == nil {
		p.laContributors = make(map[int]map[int][]int)
	}
	if p.laContributors[stateIdx] == nil {
		p.laContributors[stateIdx] = make(map[int][]int)
	}
	p.laContributors[stateIdx][lookahead] = append(
		p.laContributors[stateIdx][lookahead], ntTransIdx,
	)
}

func (p *mergeProvenance) lookaheadContributors(stateIdx, lookahead int) []int {
	if m, ok := p.laContributors[stateIdx]; ok {
		return m[lookahead]
	}
	return nil
}

func (p *mergeProvenance) mergedStateCount() int {
	return len(p.merges)
}

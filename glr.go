package gotreesitter

// glrStack is one version of the parse stack in a GLR parser.
// When the parse table has multiple actions for a (state, symbol) pair,
// the parser forks: one glrStack per alternative. Stacks that hit errors
// are dropped; surviving stacks are merged when their top states converge.
type glrStack struct {
	entries []stackEntry
	// score tracks dynamic precedence accumulated through reduce actions.
	// When merging ambiguous stacks, the one with the highest score wins.
	score int
	// dead marks a stack version that encountered an error and should be
	// removed at the next merge point.
	dead bool
	// accepted is set when the stack reaches a ParseActionAccept.
	accepted bool
}

const (
	defaultStackEntrySlabCap = 4 * 1024
	maxRetainedStackEntryCap = 256 * 1024
)

type glrMergeScratch struct {
	best   map[StateID]int
	result []glrStack
}

type glrEntryScratch struct {
	slabs []stackEntrySlab
}

type stackEntrySlab struct {
	data []stackEntry
	used int
}

func newGLRStack(initial StateID) glrStack {
	return glrStack{
		entries: []stackEntry{{state: initial, node: nil}},
	}
}

func newGLRStackWithScratch(initial StateID, scratch *glrEntryScratch) glrStack {
	if scratch == nil {
		return newGLRStack(initial)
	}
	entries := scratch.alloc(1)
	entries[0] = stackEntry{state: initial}
	return glrStack{entries: entries}
}

func (s *glrStack) top() stackEntry {
	return s.entries[len(s.entries)-1]
}

func (s *glrStack) clone() glrStack {
	entries := make([]stackEntry, len(s.entries))
	copy(entries, s.entries)
	return glrStack{entries: entries, score: s.score}
}

func (s *glrStack) cloneWithScratch(scratch *glrEntryScratch) glrStack {
	if scratch == nil {
		return s.clone()
	}
	entries := scratch.clone(s.entries)
	return glrStack{entries: entries, score: s.score}
}

func (s *glrStack) push(state StateID, node *Node, scratch *glrEntryScratch) {
	if scratch == nil {
		s.entries = append(s.entries, stackEntry{state: state, node: node})
		return
	}
	if len(s.entries) == cap(s.entries) {
		s.entries = scratch.grow(s.entries, len(s.entries)+1)
	}
	idx := len(s.entries)
	s.entries = s.entries[:idx+1]
	s.entries[idx] = stackEntry{state: state, node: node}
}

// mergeStacks removes dead stacks and merges stacks with identical top
// states. When two stacks share a top state, the one with the higher
// dynamic precedence score wins. Returns the surviving stacks.
func mergeStacks(stacks []glrStack) []glrStack {
	var scratch glrMergeScratch
	return mergeStacksWithScratch(stacks, &scratch)
}

func mergeStacksWithScratch(stacks []glrStack, scratch *glrMergeScratch) []glrStack {
	// Remove dead stacks.
	alive := stacks[:0]
	for i := range stacks {
		if !stacks[i].dead {
			alive = append(alive, stacks[i])
		}
	}
	if len(alive) <= 1 {
		return alive
	}
	if len(alive) <= 64 {
		result := ensureMergeResultCap(scratch, len(alive))
		for i := range alive {
			key := alive[i].top().state
			merged := false
			for j := range result {
				if result[j].top().state != key {
					continue
				}
				if alive[i].score > result[j].score {
					result[j] = alive[i]
				}
				merged = true
				break
			}
			if !merged {
				result = append(result, alive[i])
			}
		}
		scratch.result = result
		return result
	}

	// Merge stacks with the same top state. Keep the highest-scoring one.
	if scratch.best == nil {
		scratch.best = make(map[StateID]int, len(alive))
	} else {
		clear(scratch.best)
	}
	result := ensureMergeResultCap(scratch, len(alive))
	for i := range alive {
		key := alive[i].top().state
		if idx, ok := scratch.best[key]; ok {
			if alive[i].score > result[idx].score {
				result[idx] = alive[i]
			}
		} else {
			scratch.best[key] = len(result)
			result = append(result, alive[i])
		}
	}
	scratch.result = result
	return result
}

func ensureMergeResultCap(scratch *glrMergeScratch, n int) []glrStack {
	if cap(scratch.result) < n {
		scratch.result = make([]glrStack, 0, n)
	}
	return scratch.result[:0]
}

func (s *glrEntryScratch) alloc(n int) []stackEntry {
	if n <= 0 {
		return nil
	}
	if len(s.slabs) == 0 {
		capacity := defaultStackEntrySlabCap
		if n > capacity {
			capacity = n
		}
		s.slabs = append(s.slabs, stackEntrySlab{data: make([]stackEntry, capacity)})
	}

	last := &s.slabs[len(s.slabs)-1]
	if len(last.data)-last.used < n {
		capacity := len(last.data) * 2
		if capacity < defaultStackEntrySlabCap {
			capacity = defaultStackEntrySlabCap
		}
		if n > capacity {
			capacity = n
		}
		s.slabs = append(s.slabs, stackEntrySlab{data: make([]stackEntry, capacity)})
		last = &s.slabs[len(s.slabs)-1]
	}

	start := last.used
	last.used += n
	return last.data[start:last.used:last.used]
}

func (s *glrEntryScratch) clone(entries []stackEntry) []stackEntry {
	if len(entries) == 0 {
		return nil
	}
	out := s.alloc(len(entries))
	copy(out, entries)
	return out
}

func (s *glrEntryScratch) grow(entries []stackEntry, minCap int) []stackEntry {
	newCap := cap(entries) * 2
	if newCap < 1 {
		newCap = 1
	}
	if newCap < minCap {
		newCap = minCap
	}
	out := s.alloc(newCap)
	copy(out, entries)
	return out[:len(entries)]
}

func (s *glrEntryScratch) reset() {
	if len(s.slabs) == 0 {
		return
	}

	totalCap := 0
	for i := range s.slabs {
		slab := &s.slabs[i]
		for j := 0; j < slab.used; j++ {
			slab.data[j].node = nil
		}
		slab.used = 0
		totalCap += len(slab.data)
	}

	if totalCap <= maxRetainedStackEntryCap {
		return
	}
	if cap(s.slabs[0].data) != defaultStackEntrySlabCap {
		s.slabs[0] = stackEntrySlab{data: make([]stackEntry, defaultStackEntrySlabCap)}
	}
	s.slabs = s.slabs[:1]
}

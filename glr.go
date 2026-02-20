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

func newGLRStack(initial StateID) glrStack {
	return glrStack{
		entries: []stackEntry{{state: initial, node: nil}},
	}
}

func (s *glrStack) top() stackEntry {
	return s.entries[len(s.entries)-1]
}

func (s *glrStack) clone() glrStack {
	entries := make([]stackEntry, len(s.entries))
	copy(entries, s.entries)
	return glrStack{entries: entries, score: s.score}
}

// mergeStacks removes dead stacks and merges stacks with identical top
// states. When two stacks share a top state, the one with the higher
// dynamic precedence score wins. Returns the surviving stacks.
func mergeStacks(stacks []glrStack) []glrStack {
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
	if len(alive) <= 8 {
		result := make([]glrStack, 0, len(alive))
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
		return result
	}

	// Merge stacks with the same top state. Keep the highest-scoring one.
	best := make(map[StateID]int) // top state -> index in result
	var result []glrStack
	for i := range alive {
		key := alive[i].top().state
		if idx, ok := best[key]; ok {
			if alive[i].score > result[idx].score {
				result[idx] = alive[i]
			}
		} else {
			best[key] = len(result)
			result = append(result, alive[i])
		}
	}
	return result
}

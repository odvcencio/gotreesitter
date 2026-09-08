package gotreesitter

type queryAlternationIndex struct {
	bySymbolNamed map[uint64][]int
	byText        map[string][]int
	wildcard      []int
}

func alternationSymbolNamedKey(symbol Symbol, isNamed bool) uint64 {
	key := uint64(symbol) << 1
	if isNamed {
		key |= 1
	}
	return key
}

func (q *Query) buildAlternationIndices() {
	for pi := range q.patterns {
		buildAlternationIndicesForSteps(q.patterns[pi].steps)
	}
}

func buildAlternationIndicesForSteps(steps []QueryStep) {
	for si := range steps {
		step := &steps[si]
		if len(step.alternatives) == 0 {
			continue
		}

		idx := &queryAlternationIndex{}
		hasMissing := false
		for ai := range step.alternatives {
			alt := &step.alternatives[ai]
			if len(alt.steps) > 0 {
				buildAlternationIndicesForSteps(alt.steps)
			}
			if alt.isMissing || alt.supertype != 0 {
				hasMissing = true
			}
			switch {
			case alt.symbol == 0 && alt.textMatch == "":
				idx.wildcard = append(idx.wildcard, ai)
			case alt.textMatch != "":
				if idx.byText == nil {
					idx.byText = make(map[string][]int)
				}
				idx.byText[alt.textMatch] = append(idx.byText[alt.textMatch], ai)
			default:
				if idx.bySymbolNamed == nil {
					idx.bySymbolNamed = make(map[uint64][]int)
				}
				key := alternationSymbolNamedKey(alt.symbol, alt.isNamed)
				idx.bySymbolNamed[key] = append(idx.bySymbolNamed[key], ai)
			}
		}

		// MISSING is a semantic qualifier, not a symbol class. Keep these rare
		// alternations on the exact linear matcher rather than admitting ordinary
		// nodes from a symbol-only index bucket.
		if !hasMissing {
			step.altIndex = idx
		}
	}
}

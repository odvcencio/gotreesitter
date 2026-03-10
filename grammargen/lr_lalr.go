package grammargen

// DeRemer/Pennello LALR(1) lookahead computation.
//
// Instead of building full LR(1) item sets with lookaheads (which is O(n²) for
// large grammars due to iterative merging), this builds:
//   1. An LR(0) automaton (cores only, no lookaheads) — very fast
//   2. Lookaheads for reduce items via READS/INCLUDES/LOOKBACK relations
//      resolved with Tarjan's SCC digraph — near-linear time
//
// References:
//   - DeRemer, Pennello: "Efficient Computation of LALR(1) Look-Ahead Sets" (1982)
//   - Grune, Jacobs: "Parsing Techniques: A Practical Guide", §9.7

// ntTransition identifies a nonterminal transition (p, A) in the LR(0) automaton,
// meaning: in state p, reading nonterminal A, go to some state q.
type ntTransition struct {
	state   int // source state p
	nonterm int // nonterminal symbol A
	target  int // target state q = GOTO(p, A)
}

// buildItemSetsLALR constructs LALR(1) item sets using the DeRemer/Pennello algorithm.
// Returns the item sets with lookaheads attached only to reduce items.
func (ctx *lrContext) buildItemSetsLALR() []lrItemSet {
	// Phase 1: Build LR(0) automaton.
	ctx.buildLR0()

	// Phase 2: Compute LALR(1) lookaheads via DeRemer/Pennello.
	ctx.computeLALRLookaheads()

	return ctx.itemSets
}

// buildLR0 constructs the LR(0) automaton: item sets with cores only, no lookaheads.
// This is much faster than the full LR(1) construction because there's no lookahead
// propagation, merging, or worklist re-processing.
func (ctx *lrContext) buildLR0() {
	ctx.transitions = make(map[int]map[int]int)
	ctx.ensureProvenance()
	ng := ctx.ng
	tokenCount := ctx.tokenCount

	// Hash map for state dedup: coreHash → chain of state indices.
	coreMap := make(map[uint64]*stateHashEntry)

	// Build initial state: closure of [S' → .S]
	initialSet := ctx.lr0Closure([]coreItem{{prodIdx: ng.AugmentProdID, dot: 0}})
	ctx.itemSets = []lrItemSet{initialSet}
	addToHashMap(coreMap, initialSet.coreHash, 0)
	ctx.recordFreshState(0)

	// BFS through states.
	for stateIdx := 0; stateIdx < len(ctx.itemSets); stateIdx++ {
		itemSet := &ctx.itemSets[stateIdx]

		// Collect all symbols after the dot.
		symsSeen := make(map[int]bool)
		var syms []int
		for _, ce := range itemSet.cores {
			prod := &ng.Productions[ce.prodIdx]
			if ce.dot < len(prod.RHS) {
				sym := prod.RHS[ce.dot]
				if !symsSeen[sym] {
					symsSeen[sym] = true
					syms = append(syms, sym)
				}
			}
		}

		for _, sym := range syms {
			// Compute GOTO(state, sym): advance dot past sym, then close.
			var kernel []coreItem
			for _, ce := range itemSet.cores {
				prod := &ng.Productions[ce.prodIdx]
				if ce.dot < len(prod.RHS) && prod.RHS[ce.dot] == sym {
					kernel = append(kernel, coreItem{ce.prodIdx, ce.dot + 1})
				}
			}
			if len(kernel) == 0 {
				continue
			}

			closedSet := ctx.lr0Closure(kernel)

			// Find existing state with same core, or create new.
			targetIdx := -1
			for entry := coreMap[closedSet.coreHash]; entry != nil; entry = entry.next {
				if sameCores(&ctx.itemSets[entry.stateIdx], &closedSet) {
					targetIdx = entry.stateIdx
					ctx.recordMergedState(targetIdx, mergeOrigin{
						kernelHash:  closedSet.coreHash,
						sourceState: stateIdx,
					})
					break
				}
			}
			if targetIdx < 0 {
				targetIdx = len(ctx.itemSets)
				ctx.itemSets = append(ctx.itemSets, closedSet)
				addToHashMap(coreMap, closedSet.coreHash, targetIdx)
				ctx.recordFreshState(targetIdx)
			}

			// Record transition.
			if ctx.transitions[stateIdx] == nil {
				ctx.transitions[stateIdx] = make(map[int]int)
			}
			ctx.transitions[stateIdx][sym] = targetIdx

			// After appending to itemSets, re-read pointer in case of slice realloc.
			itemSet = &ctx.itemSets[stateIdx]
		}

		_ = tokenCount // used implicitly via lr0Closure
	}
}

// lr0Closure computes the LR(0) closure of a set of kernel items.
// No lookaheads are involved — just expands nonterminals to their productions.
func (ctx *lrContext) lr0Closure(kernel []coreItem) lrItemSet {
	ng := ctx.ng
	tokenCount := ctx.tokenCount

	for _, prodIdx := range ctx.dot0Dirty {
		ctx.dot0Index[prodIdx] = -1
	}
	ctx.dot0Dirty = ctx.dot0Dirty[:0]

	kernelSeen := make(map[uint64]int, len(kernel))
	cores := make([]coreEntry, 0, len(kernel)*2)

	// Add kernel items.
	for _, ki := range kernel {
		key := packCoreItemKey(ki.prodIdx, ki.dot)
		if _, ok := kernelSeen[key]; ok {
			continue // deduplicate
		}
		idx := len(cores)
		kernelSeen[key] = idx
		cores = append(cores, coreEntry{
			prodIdx:    ki.prodIdx,
			dot:        ki.dot,
			lookaheads: newBitset(tokenCount), // empty, will be filled in phase 2
		})
		if ki.dot == 0 {
			ctx.dot0Index[ki.prodIdx] = idx
			ctx.dot0Dirty = append(ctx.dot0Dirty, ki.prodIdx)
		}
	}

	// Expand: for each item [A → α.Bβ], add [B → .γ] for all B-productions.
	// Use a worklist but only process each core item once (no re-processing needed
	// since LR(0) closure doesn't change — there are no lookaheads to propagate).
	for i := 0; i < len(cores); i++ {
		ce := &cores[i]
		prod := &ng.Productions[ce.prodIdx]
		if ce.dot >= len(prod.RHS) {
			continue
		}

		nextSym := prod.RHS[ce.dot]
		if nextSym < tokenCount {
			continue
		}

		for _, prodIdx := range ctx.prodsByLHS[nextSym] {
			if ctx.dot0Index[prodIdx] >= 0 {
				continue
			}
			idx := len(cores)
			ctx.dot0Index[prodIdx] = idx
			ctx.dot0Dirty = append(ctx.dot0Dirty, prodIdx)
			cores = append(cores, coreEntry{
				prodIdx:    prodIdx,
				dot:        0,
				lookaheads: newBitset(tokenCount),
			})
		}
	}

	packedCoreIndex := make(map[uint64]int, len(cores))
	for idx, ce := range cores {
		packedCoreIndex[packCoreItemKey(ce.prodIdx, ce.dot)] = idx
	}

	set := lrItemSet{
		cores:           cores,
		packedCoreIndex: packedCoreIndex,
	}
	// Compute only coreHash (fullHash and reduceLAHash will be set after lookaheads).
	var ch uint64
	for _, c := range cores {
		ch += mixCoreItem(c.prodIdx, c.dot)
	}
	set.coreHash = ch
	set.fullHash = ch // temporary, will be recomputed
	set.reduceLAHash = ch

	return set
}

func packCoreItemKey(prodIdx, dot int) uint64 {
	return uint64(uint32(prodIdx))<<32 | uint64(uint32(dot))
}

// computeLALRLookaheads implements the DeRemer/Pennello algorithm to compute
// LALR(1) lookaheads for all reduce items in the LR(0) automaton.
func (ctx *lrContext) computeLALRLookaheads() {
	ng := ctx.ng
	tokenCount := ctx.tokenCount

	// Step 1: Index all nonterminal transitions.
	var ntTrans []ntTransition
	ntTransIndex := make(map[[2]int]int) // (state, nonterm) → index in ntTrans

	for state, trans := range ctx.transitions {
		for sym, target := range trans {
			if sym >= tokenCount {
				idx := len(ntTrans)
				ntTransIndex[[2]int{state, sym}] = idx
				ntTrans = append(ntTrans, ntTransition{
					state:   state,
					nonterm: sym,
					target:  target,
				})
			}
		}
	}

	numTrans := len(ntTrans)
	if numTrans == 0 {
		return
	}

	// Step 2: Compute DR (Directly-Reads) sets.
	// DR(p, A) = { t ∈ Terminals | GOTO(p, A) has a shift on t }
	// i.e., terminals reachable in one step from the target state of (p, A).
	dr := make([]bitset, numTrans)
	for i, nt := range ntTrans {
		dr[i] = newBitset(tokenCount)
		q := nt.target // target state
		if trans, ok := ctx.transitions[q]; ok {
			for sym := range trans {
				if sym < tokenCount {
					dr[i].add(sym)
				}
			}
		}
	}

	// Seed $end into DR(0, start_symbol). The accept state (GOTO(0, start_symbol))
	// doesn't have a transition on $end, but $end is conceptually "readable" there
	// since the augmented production S' → S reduces on $end.
	startSym := ng.Productions[ng.AugmentProdID].RHS[0]
	if idx, ok := ntTransIndex[[2]int{0, startSym}]; ok {
		dr[idx].add(0) // $end = symbol 0
	}

	// Step 3: Compute READS relation.
	// (p, A) reads (q, C) iff GOTO(p, A) = q and C is nullable.
	// This means: from the target state of (p,A), if we can read a nullable
	// nonterminal C, then whatever C reads also contributes to Read(p,A).
	reads := make([][]int, numTrans)
	for i, nt := range ntTrans {
		q := nt.target
		if trans, ok := ctx.transitions[q]; ok {
			for sym := range trans {
				if sym >= tokenCount && ctx.nullables[sym] {
					if j, ok := ntTransIndex[[2]int{q, sym}]; ok {
						reads[i] = append(reads[i], j)
					}
				}
			}
		}
	}

	// Step 4: Compute Read sets = Digraph(DR, READS).
	// Read(p, A) = DR(p, A) ∪ ∪{ Read(q, C) | (p,A) reads (q,C) }
	readSets := digraph(numTrans, dr, reads)

	// Step 5: Compute INCLUDES relation.
	// (p, A) includes (p', B) iff B → βAγ is a production, p' --β--> p,
	// and γ is nullable (γ ⇒* ε).
	//
	// For each production B → X₁X₂...Xₙ and each state p' that has this
	// production in its item set [B → .X₁...Xₙ], trace the path
	// p' → p₁ → p₂ → ... → pₙ through the automaton. For each position k
	// where Xₖ is a nonterminal A and Xₖ₊₁...Xₙ is nullable, add:
	// (pₖ₋₁, A=Xₖ) includes (p', B).
	//
	// At the same time, compute LOOKBACK:
	// (q, A → ω) lookback (p, A) iff p --ω--> q
	// i.e., from state p, reading the entire RHS of production "A → ω" leads to q.
	type lookbackEntry struct {
		stateIdx int // state q where reduce happens
		prodIdx  int // production A → ω
		ntIdx    int // index into ntTrans for (p, A)
	}
	var lookbacks []lookbackEntry

	includes := make([][]int, numTrans)

	for pi := range ng.Productions {
		prod := &ng.Productions[pi]
		lhs := prod.LHS
		rhs := prod.RHS

		// Find all states that have [B → .X₁...Xₙ] (dot=0 for this production).
		// These are all states p' where production pi appears with dot=0.
		for stateIdx := range ctx.itemSets {
			itemSet := &ctx.itemSets[stateIdx]
			if _, ok := itemSet.coreLookup(pi, 0); !ok {
				continue
			}

			// Trace path p' → p₁ → ... → pₙ through the automaton.
			curState := stateIdx
			valid := true
			for dot := 0; dot < len(rhs); dot++ {
				sym := rhs[dot]

				// Before moving past sym, check if this position contributes.
				// If sym is a nonterminal A and the suffix rhs[dot+1:] is nullable,
				// then (curState, A) includes (stateIdx, lhs).
				if sym >= tokenCount {
					suffixNullable := true
					for _, s := range rhs[dot+1:] {
						if s < tokenCount || !ctx.nullables[s] {
							suffixNullable = false
							break
						}
					}
					if suffixNullable {
						srcKey := [2]int{stateIdx, lhs}
						tgtKey := [2]int{curState, sym}
						if srcIdx, ok := ntTransIndex[srcKey]; ok {
							if tgtIdx, ok := ntTransIndex[tgtKey]; ok {
								includes[tgtIdx] = append(includes[tgtIdx], srcIdx)
							}
						}
					}
				}

				// Advance: curState = GOTO(curState, sym)
				if trans, ok := ctx.transitions[curState]; ok {
					if next, ok := trans[sym]; ok {
						curState = next
					} else {
						valid = false
						break
					}
				} else {
					valid = false
					break
				}
			}

			if valid && len(rhs) > 0 {
				// curState is the state q where we can reduce A → ω.
				// lookback: (q, A → ω) lookback (stateIdx, A)
				srcKey := [2]int{stateIdx, lhs}
				if srcIdx, ok := ntTransIndex[srcKey]; ok {
					lookbacks = append(lookbacks, lookbackEntry{
						stateIdx: curState,
						prodIdx:  pi,
						ntIdx:    srcIdx,
					})
				}
			}

			// Also handle epsilon productions: if rhs is empty, the reduce
			// state is stateIdx itself.
			if valid && len(rhs) == 0 {
				srcKey := [2]int{stateIdx, lhs}
				if srcIdx, ok := ntTransIndex[srcKey]; ok {
					lookbacks = append(lookbacks, lookbackEntry{
						stateIdx: stateIdx,
						prodIdx:  pi,
						ntIdx:    srcIdx,
					})
				}
			}
		}
	}

	// Step 6: Compute Follow sets = Digraph(Read, INCLUDES).
	// Follow(p, A) = Read(p, A) ∪ ∪{ Follow(p', B) | (p,A) includes (p',B) }
	followSets := digraph(numTrans, readSets, includes)

	// Step 7: Compute LA (lookahead) sets for reduce items via LOOKBACK.
	// LA(q, A → ω) = ∪{ Follow(p, A) | (q, A → ω) lookback (p, A) }
	//
	// We attach lookaheads to the reduce core entries in each item set.
	for _, lb := range lookbacks {
		itemSet := &ctx.itemSets[lb.stateIdx]
		prod := &ng.Productions[lb.prodIdx]
		if idx, ok := itemSet.coreLookup(lb.prodIdx, len(prod.RHS)); ok {
			itemSet.cores[idx].lookaheads.unionWith(&followSets[lb.ntIdx])
			followSets[lb.ntIdx].forEach(func(la int) {
				ctx.recordLookaheadContributor(lb.stateIdx, la, lb.ntIdx)
			})
		}
	}

	// Step 8: Handle augmented start production: S' → S has lookahead {$end}.
	// The augmented production reduces in the state reached after reading S.
	augProd := &ng.Productions[ng.AugmentProdID]
	if len(augProd.RHS) > 0 {
		// Find the state reached from state 0 via the start symbol.
		if trans, ok := ctx.transitions[0]; ok {
			if targetState, ok := trans[augProd.RHS[0]]; ok {
				augSet := &ctx.itemSets[targetState]
				if idx, ok := augSet.coreLookup(ng.AugmentProdID, len(augProd.RHS)); ok {
					augSet.cores[idx].lookaheads.add(0) // $end
				}
			}
		}
	}

	// Recompute hashes now that lookaheads are populated.
	for i := range ctx.itemSets {
		ctx.itemSets[i].computeHashes(ng.Productions, &ctx.boundaryLookaheads, false)
	}
}

// digraph implements Tarjan's SCC-based algorithm for computing F(x) across a
// relation R, given initial values f(x):
//
//	F(x) = f(x) ∪ ∪{ F(y) | x R y }
//
// This is the core algorithm from DeRemer & Pennello (1982). It visits each
// node at most twice (push + pop), making it near-linear.
//
// n: number of nodes
// f: initial values f(0..n-1), each a bitset
// rel: adjacency list for relation R: rel[x] = list of y such that x R y
// bitcap: capacity for new bitsets
//
// Returns F(0..n-1).
func digraph(n int, f []bitset, rel [][]int) []bitset {
	result := make([]bitset, n)
	for i := 0; i < n; i++ {
		result[i] = f[i].clone()
	}

	// Tarjan's SCC stack and state.
	const infinity = 0x7FFFFFFF
	depth := make([]int, n) // 0 = unvisited, >0 = stack depth, infinity = done
	stack := make([]int, 0, n)
	d := 0 // current depth counter

	var traverse func(x int)
	traverse = func(x int) {
		d++
		depth[x] = d
		stack = append(stack, x)

		for _, y := range rel[x] {
			if depth[y] == 0 {
				traverse(y)
			}
			// If y is still on the stack (not yet assigned to an SCC),
			// propagate its result into x and update x's depth.
			if depth[y] < depth[x] {
				depth[x] = depth[y]
			}
			result[x].unionWith(&result[y])
		}

		// If x is the root of an SCC, pop the SCC and assign the same result.
		if depth[x] == d {
			for {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				depth[top] = infinity
				if top == x {
					break
				}
				// All nodes in this SCC get the same result.
				result[top] = result[x].clone()
			}
		}
		d--
	}

	for i := 0; i < n; i++ {
		if depth[i] == 0 {
			traverse(i)
		}
	}

	return result
}

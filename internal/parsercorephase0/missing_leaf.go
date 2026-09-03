package parsercorephase0

import "errors"

// compactMissingLeafStoredErrorCost is the pinned C cost of one missing
// terminal: one recovery plus one missing-tree term.
const compactMissingLeafStoredErrorCost uint32 = 610

// ---------------------------------------------------------------------------
// missing_leaf.go -- compact support for C's recovery-inserted MISSING
// terminal.
//
// THE PRODUCTION GO PORT IS THE EXECUTABLE SPECIFICATION for the mechanism
// that produces these (cHandleError's missing-token search,
// parser_recover_c.go, itself a port of ts_parser__handle_error step 2,
// parser.c:2154-2230). The pinned C oracle is the parity arbiter
// (decision 0007).
//
// The compact S5 recovery stage calls MissingLeaf after it creates competing
// missing-insertion and error-absorb lineages. Artifact certification gates
// that stage.
// ---------------------------------------------------------------------------

// MissingLeaf publishes one zero-width recovery-inserted terminal.
//
// C constructs the same object with ts_subtree_new_missing_leaf
// (subtree.c:534-546): a leaf whose size is length_zero() and whose
// is_missing bit is set. Two consequences of that construction are
// reproduced here and must not be "simplified" away:
//
//   - The leaf is ZERO WIDTH. atByte is both its start and its end. C
//     positions it at the stack's current byte offset: ts_parser__handle_error
//     resets the lexer to the stack position and immediately marks the end
//     (parser.c), so the computed padding is zero and the leaf carries no
//     source text at all. A caller that passes a span here is describing
//     something that is not a missing token.
//
//   - The leaf is NOT extra and NOT external. C passes false for external
//     tokens and builds the leaf outside the scanner entirely; it is a
//     synthetic terminal the table demanded, not lexed input. Marking it
//     extra would additionally make popPaths skip it when counting a
//     production's structural arity (see ErrorRegionResume, where extra IS
//     correct and load-bearing for the opposite reason), which would silently
//     change every enclosing reduce.
//
// The record's missing bit is what makes the cost model correct later:
// ts_subtree_error_cost (subtree.h:331-337) short-circuits on it and returns
// ERROR_COST_PER_MISSING_TREE + ERROR_COST_PER_RECOVERY (610), ignoring any
// stored cost. Materialization also reads it to set the public node's own
// missing and has-error bits, matching ts_node_has_error, which C defines as
// error_cost > 0 (node.c:520-522) and which is therefore true on the missing
// node itself, not only on its ancestors.
//
// appendSubtree, not appendSubtreeRecord: like ErrorRegionLeaf, a missing
// terminal is published outside the grammar-table-authenticated shift seam,
// so metadataConstructionAuthenticated must clear and the record earns
// materialization-time metadata validation rather than skipping it.
//
// WHAT THAT VALIDATION DOES NOT COVER, so the caller owes it. This function
// rejects only the two RESERVED symbols that can never name a missing token:
// end-of-file and ERROR. It cannot do more, because it takes no StateID and
// so cannot ask whether the grammar actually demands the token here. C can:
// ts_parser__handle_error only ever builds a missing leaf for a symbol the
// current state has a shift entry for (parser.c:2154-2230). An ordinary
// grammar nonterminal, or a symbol past the table's symbol count, is accepted
// here and yields a record the cost model prices as a missing subtree.
// Materialization validates reduction metadata, not this. The scheduler
// caller must authenticate the symbol against its state before publishing.
func (c *Core) MissingLeaf(symbol Symbol, atByte uint32) (id SubtreeID, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	return c.missingLeafUncheckpointed(symbol, atByte)
}

func (c *Core) missingLeafUncheckpointed(symbol Symbol, atByte uint32) (SubtreeID, error) {
	if symbol == 0 {
		return 0, errors.New("parser-core phase zero: missing leaf requires a real terminal symbol")
	}
	if symbol == ErrorRegionSymbol {
		return 0, errors.New("parser-core phase zero: missing leaf cannot carry the ERROR symbol")
	}
	return c.appendSubtree(subtreeRecord{
		symbol:    symbol,
		startByte: atByte,
		endByte:   atByte,
		terminal:  true,
		missing:   true,
	}, nil, nil, nil)
}

// ShiftMissingLeaf publishes one missing terminal and attaches it to head at
// targetState, the compact equivalent of C pushing the missing subtree onto a
// copied stack version (ts_parser__handle_error, parser.c:2154-2230:
// ts_stack_push(stack, version_with_missing_tree, missing_tree, false,
// state_after_missing_symbol)).
//
// The boundary is keyed as a SHIFT boundary (shiftedBoundaryKey), matching
// every other terminal attachment onto a head (shiftClassifiedUncheckpointed)
// rather than a reduction replacing children. The byte offset does not move:
// the leaf is zero width, so the resulting head sits at the same atByte the
// caller's head already occupied, which is exactly what C's own
// length_zero() size produces.
//
// targetState must be the state C's ts_language_next_state returns for
// symbol at the head's own state. This function does not re-derive it,
// because the caller has already read the action row to find it and to run
// C's reduce-action test on the result.
func (c *Core) ShiftMissingLeaf(head Head, targetState StateID, symbol Symbol, atByte uint32) (out Head, err error) {
	if targetState == 0 {
		return Head{}, errors.New("parser-core phase zero: missing leaf shift requires a real target state")
	}
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	return c.shiftMissingLeafUncheckpointed(head, targetState, symbol, atByte)
}

// ShiftMissingLeafOwned attaches one authenticated missing leaf without
// opening a nested ordinary checkpoint. Scheduler recovery uses this seam
// inside a speculative transaction.
func (c *Core) ShiftMissingLeafOwned(owner SchedulerTransactionToken, head Head, targetState StateID, symbol Symbol, atByte uint32) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	out, err = c.shiftMissingLeafUncheckpointed(head, targetState, symbol, atByte)
	return out, c.finishSchedulerOwned(owner, err)
}

func (c *Core) shiftMissingLeafUncheckpointed(head Head, targetState StateID, symbol Symbol, atByte uint32) (out Head, err error) {
	if targetState == 0 {
		return Head{}, errors.New("parser-core phase zero: missing leaf shift requires a real target state")
	}
	if _, err := c.node(head.Node); err != nil {
		return Head{}, err
	}
	payload, err := c.missingLeafUncheckpointed(symbol, atByte)
	if err != nil {
		return Head{}, err
	}
	lineage, err := c.nodeLineage(head.Node)
	if err != nil {
		return Head{}, err
	}
	if ^uint32(0)-lineage.storedErrorCost < compactMissingLeafStoredErrorCost {
		return Head{}, errors.New("parser-core phase zero: missing leaf stored recovery cost overflow")
	}
	storedErrorCost := lineage.storedErrorCost + compactMissingLeafStoredErrorCost
	outcome, err := c.condenseWithOutcomeAtomic(c.shiftedBoundaryKey(targetState, atByte), linkInput{
		prev: head.Node, payload: payload, storedErrorCost: storedErrorCost, hasStoredErrorCost: true,
	})
	return outcome.head, err
}

// UniqueStateSpine returns the state stack below head, from the seed to head.
// It succeeds only when each node has one predecessor.
//
// A compact head can represent many graph-structured stack paths. Recovery
// simulations need one concrete version, so ambiguity and truncation decline.
func (c *Core) UniqueStateSpine(head Head, maxDepth int) ([]StateID, bool, error) {
	if maxDepth <= 0 {
		return nil, false, nil
	}
	reversed := make([]StateID, 0, min(maxDepth, 64))
	id := head.Node
	for depth := 0; depth < maxDepth; depth++ {
		node, err := c.node(id)
		if err != nil {
			return nil, false, err
		}
		reversed = append(reversed, node.state)
		if node.linkCount == 0 {
			for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
				reversed[left], reversed[right] = reversed[right], reversed[left]
			}
			return reversed, true, nil
		}
		if node.linkCount != 1 {
			return nil, false, nil
		}
		linkID := LinkID(node.firstLink)
		if linkID == 0 || uint64(linkID) > uint64(len(c.links)) {
			return nil, false, errors.New("parser-core phase zero: state spine adjacency is out of range")
		}
		link := c.links[linkID-1]
		if err := link.validateShape(); err != nil {
			return nil, false, err
		}
		if link.prev == 0 || link.prev >= id {
			return nil, false, errors.New("parser-core phase zero: state spine link has no predecessor")
		}
		id = link.prev
	}
	return nil, false, nil
}

const (
	canShiftAfterReductionsMaxSteps       = 4096
	canShiftAfterReductionsMaxStackGrowth = 64
)

type recoverySimulationStackNode struct {
	prev  int
	state StateID
	depth int
}

type recoverySimulationStackKey struct {
	prev  int
	state StateID
}

// CanShiftAfterReductions reports whether lookahead can make progress after
// zero or more reductions. It explores each reduction arm.
//
// The simulation interns persistent stack nodes. Thus, its memory grows with
// explored states instead of multiplying each state by the source stack depth.
func (c *Core) CanShiftAfterReductions(spine []StateID, lookahead Symbol) (bool, error) {
	if len(spine) == 0 {
		return false, nil
	}

	nodes := make([]recoverySimulationStackNode, 1, len(spine)+64)
	interned := make(map[recoverySimulationStackKey]int, len(spine)+64)
	top := 0
	for _, state := range spine {
		key := recoverySimulationStackKey{prev: top, state: state}
		id, ok := interned[key]
		if !ok {
			id = len(nodes)
			nodes = append(nodes, recoverySimulationStackNode{
				prev: top, state: state, depth: nodes[top].depth + 1,
			})
			interned[key] = id
		}
		top = id
	}

	frontier := []int{top}
	seen := map[int]struct{}{top: {}}
	steps := 0
	maxDepth := len(spine) + canShiftAfterReductionsMaxStackGrowth
	for len(frontier) != 0 {
		current := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		row, err := c.tables.Actions(nodes[current].state, lookahead)
		if err != nil {
			return false, err
		}
		for ordinal := 0; ordinal < row.Len(); ordinal++ {
			action := row.At(ordinal)
			switch action.Type {
			case ActionShift, ActionRecover:
				if !action.Extra && !action.Repetition {
					return true, nil
				}
			case ActionReduce:
				steps++
				if steps > canShiftAfterReductionsMaxSteps {
					return false, nil
				}
				base := current
				for count := 0; count < int(action.ChildCount) && base != 0; count++ {
					base = nodes[base].prev
				}
				if base == 0 {
					continue
				}
				gotoState, err := c.tables.Goto(nodes[base].state, action.Symbol)
				if err != nil {
					return false, err
				}
				if gotoState == 0 || nodes[base].depth+1 > maxDepth {
					continue
				}
				key := recoverySimulationStackKey{prev: base, state: gotoState}
				next, ok := interned[key]
				if !ok {
					next = len(nodes)
					nodes = append(nodes, recoverySimulationStackNode{
						prev: base, state: gotoState, depth: nodes[base].depth + 1,
					})
					interned[key] = next
				}
				if _, ok := seen[next]; ok {
					continue
				}
				seen[next] = struct{}{}
				frontier = append(frontier, next)
			}
		}
	}
	return false, nil
}

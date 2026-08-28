package parsercorephase0

import "errors"

// ---------------------------------------------------------------------------
// missing_leaf.go -- campaign v7 tranche B3 stage S5 substrate: the compact
// representation of C's recovery-inserted MISSING terminal.
//
// THE PRODUCTION GO PORT IS THE EXECUTABLE SPECIFICATION for the mechanism
// that will produce these (cHandleError's missing-token search,
// parser_recover_c.go, itself a port of ts_parser__handle_error step 2,
// parser.c:2154-2230). The pinned C oracle is the parity arbiter
// (decision 0007).
//
// NOTHING IN THE SHIPPED PARSER CALLS MissingLeaf TODAY. It is inert
// substrate, landed on its own so the storage and materialization change can
// be reviewed apart from the scheduler mechanism that will use it. The
// admission census reports six real-corpus rows whose first decline point is
// owned by this mechanism, and three of those six already carry a production
// tree that is structurally identical to the locked C oracle, so the
// mechanism has a measured target to hit.
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
// so metadataConstructionAuthenticated must clear and the record earns full
// materialization-time metadata validation rather than skipping it.
func (c *Core) MissingLeaf(symbol Symbol, atByte uint32) (id SubtreeID, err error) {
	if symbol == 0 {
		return 0, errors.New("parser-core phase zero: missing leaf requires a real terminal symbol")
	}
	if symbol == ErrorRegionSymbol {
		return 0, errors.New("parser-core phase zero: missing leaf cannot carry the ERROR symbol")
	}
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
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
	if _, err := c.node(head.Node); err != nil {
		return Head{}, err
	}
	payload, err := c.MissingLeaf(symbol, atByte)
	if err != nil {
		return Head{}, err
	}
	return c.condense(c.shiftedBoundaryKey(targetState, atByte), linkInput{
		prev: head.Node, payload: payload,
	})
}

// UniqueStateSpine returns the parser-state stack under head, bottom first and
// head's own state last, but ONLY when that stack is unambiguous: every node
// on the walk must carry exactly one link, so there is exactly one path back
// to the seed. ok is false the moment a node carries more than one link (a
// live ambiguity, where "the" state stack does not exist) or the walk exceeds
// maxDepth.
//
// This exists so a caller can run C's linear stack simulations -- which
// operate on a version's own state array -- against a compact head. A GLR
// head is a DAG in general, and those simulations are not defined on a DAG,
// so the unique-path requirement is not a convenience: it is the condition
// under which the question is meaningful at all. Callers must treat ok=false
// as "decline", never as "assume the first path".
//
// A truncating walk reports ok=false rather than a partial spine. A partial
// spine silently answers a different question -- a deep reduce would find
// fewer states to pop than really exist and report failure -- so returning it
// would trade a fail-closed decline for a wrong answer.
func (c *Core) UniqueStateSpine(head Head, maxDepth int) ([]StateID, bool, error) {
	if maxDepth <= 0 {
		return nil, false, nil
	}
	var reversed []StateID
	id := head.Node
	for depth := 0; depth <= maxDepth; depth++ {
		node, err := c.node(id)
		if err != nil {
			return nil, false, err
		}
		reversed = append(reversed, node.state)
		if node.linkCount == 0 {
			// Reached the seed. Reverse into bottom-first order.
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}
			return reversed, true, nil
		}
		if node.linkCount != 1 {
			return nil, false, nil
		}
		linkID := LinkID(node.firstLink)
		if linkID == 0 || uint64(linkID) > uint64(len(c.links)) {
			return nil, false, errors.New("parser-core phase zero: state spine adjacency out of range")
		}
		link := c.links[linkID-1]
		if link.prev == 0 {
			return nil, false, errors.New("parser-core phase zero: state spine link has no predecessor")
		}
		id = link.prev
	}
	return nil, false, nil
}

// canShiftAfterReductionsMaxSteps bounds the whole search below: total
// (stack, reduce-action) expansions, not depth. C's own exploration is
// bounded by its version-count and candidate-attempt ceilings; this is the
// read-only equivalent.
const canShiftAfterReductionsMaxSteps = 4096

// canShiftAfterReductionsMaxStackGrowth bounds how far above the caller's
// spine a chain of reductions may push before the search gives up. A
// nullable (zero-child) reduce consumes nothing, so without a growth bound a
// grammar with a nullable cycle would expand forever inside the step budget.
const canShiftAfterReductionsMaxStackGrowth = 64

// CanShiftAfterReductions reports whether a version whose state stack is
// spine could act on lookahead after applying zero or more reductions that
// lookahead itself enables.
//
// This is the read-only equivalent of C's
// ts_parser__do_all_potential_reductions confirmation step, which
// ts_parser__handle_error uses to decide whether a missing-token insertion is
// viable at all (parser.c:2154-2230, the
// `if (ts_parser__do_all_potential_reductions(...))` gate). C explores EVERY
// reduce action available for the lookahead, keeping each resulting version,
// and answers yes when ANY of them can then shift. The search here mirrors
// that branching.
//
// It deliberately does NOT mirror the production parser's own
// canShiftAfterReductions helper (parser.go), which follows only the FIRST
// reduce action it finds at each state. That linear walk is a sound
// approximation for the narrower question production asks with it, but it is
// strictly stricter than C here: measured on the shipped corpus, taking only
// the first reduce rejected every candidate on all four php and sql
// missing-token rows, whose real-corpus production trees show C does insert.
// A stricter confirmation is fail-closed rather than wrong, but it forfeits
// exactly the rows this stage exists to graduate.
//
// spine is bottom first, head state last, exactly as UniqueStateSpine returns
// it. It is never modified: every explored stack is a private copy.
//
// Extra and repetition shifts do not count as acting on the lookahead. An
// extra shift is available from nearly every state and a repetition shift is
// a self-loop, so neither proves the version made grammatical progress.
func (c *Core) CanShiftAfterReductions(spine []StateID, lookahead Symbol) (bool, error) {
	if len(spine) == 0 {
		return false, nil
	}
	base := len(spine)
	frontier := [][]StateID{append(make([]StateID, 0, base+8), spine...)}
	// Dedup on (depth, top state). Two stacks that agree on both answer this
	// question identically for every continuation the search can take,
	// because every later step reads only the top state and pops by count.
	type seenKey struct {
		depth int
		state StateID
	}
	seen := map[seenKey]bool{{depth: base, state: spine[base-1]}: true}
	steps := 0
	for len(frontier) > 0 {
		states := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		if len(states) == 0 {
			continue
		}
		row, err := c.tables.Actions(states[len(states)-1], lookahead)
		if err != nil {
			return false, err
		}
		for index := 0; index < row.Len(); index++ {
			act := row.At(index)
			switch act.Type {
			case ActionShift, ActionRecover:
				if !act.Extra && !act.Repetition {
					return true, nil
				}
			case ActionReduce:
				steps++
				if steps > canShiftAfterReductionsMaxSteps {
					return false, nil
				}
				childCount := int(act.ChildCount)
				if childCount > len(states) {
					continue
				}
				popped := len(states) - childCount
				if popped == 0 {
					continue
				}
				gotoState, err := c.tables.Goto(states[popped-1], act.Symbol)
				if err != nil {
					return false, err
				}
				if gotoState == 0 {
					continue
				}
				if popped+1 > base+canShiftAfterReductionsMaxStackGrowth {
					continue
				}
				key := seenKey{depth: popped + 1, state: gotoState}
				if seen[key] {
					continue
				}
				seen[key] = true
				next := make([]StateID, popped, popped+1)
				copy(next, states[:popped])
				frontier = append(frontier, append(next, gotoState))
			}
		}
	}
	return false, nil
}

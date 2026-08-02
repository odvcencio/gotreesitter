package parsercorephase0

import (
	"errors"
	"fmt"
)

// ---------------------------------------------------------------------------
// acceptance_election.go -- the reference runtime's tied-tree selection order,
// ported for the compact core.
//
// THE PINNED C ORACLE IS THE EXECUTABLE SPECIFICATION for this file
// (decision 0007, hypha://m31labs/gotreesitter): tree-sitter v0.25.0's
// ts_parser__select_tree (parser.c:836-879) and ts_subtree_compare
// (subtree.c:591-618). The production Go port of the same order lives in the
// root package -- stackCompareForResultSelectionWithRawShape
// (parser_result.go) at result granularity, and reduceForkWindowPreference /
// compareRawStackEntriesRec (parser_reduce.go) at fork granularity. Every
// rung, constant, and evaluation order below mirrors the C functions; a
// divergence gets a receipt, not a silent "improvement".
//
// ts_parser__select_tree(left, right) answers one question: does right
// replace left? Its rungs, in order:
//
//	1. right error cost  <  left error cost   -> take right
//	2. left  error cost  <  right error cost  -> keep left
//	3. right dyn. prec.  >  left dyn. prec.   -> take right
//	4. left  dyn. prec.  >  right dyn. prec.  -> keep left
//	5. left error cost   >  0                 -> take right
//	6. ts_subtree_compare(left, right) < 0    -> keep left
//	   ts_subtree_compare(left, right) > 0    -> take right
//	   ts_subtree_compare(left, right) == 0   -> keep left
//
// Rung 5 is not a typo and is not dead: when two candidates carry the SAME
// nonzero error cost and the same dynamic precedence, C takes the later
// candidate. Production's port carries the identical rung with the identical
// note (parser_result.go, `else if ac > 0`).
//
// ---------------------------------------------------------------------------
// THIS FILE IS INERT. NOTHING IN THIS PACKAGE, AND NOTHING IN THE ROOT
// PACKAGE, CALLS IT. TestAcceptanceElectionNoOutsideCallSitesRatchet enforces
// that as a compile-time property, in the same style as
// recovery_cost.go's own ratchet.
//
// It is inert BECAUSE IT WAS MEASURED, not because it is unfinished. The
// intended use was to replace the compact acceptance election's fail-closed
// decline on a tied accept (completeAcceptance, parsercore_phase0_driver.go)
// with this order, so a tie would resolve to the tree the reference runtime
// keeps instead of handing the input back to the production parser. Wired
// that way and run against the locked C oracle, the order produced a
// DIFFERENT tree from the oracle on 8 of the 10 tied accepts it newly
// admitted across the apex, perl, ada, and kotlin certification sweeps.
//
// The two witnesses that state the finding most plainly:
//
//   - apex "Object t = RecordPage.class;". The two live derivations are
//     class_literal (symbol 268) and field_access (symbol 270). Rung 6 orders
//     by LOWER symbol id, so it selects class_literal. The C oracle's
//     published tree is field_access, with an "object" field on its first
//     child. The oracle's tree also reads the source text "class" as an
//     identifier token, which the class_literal derivation cannot do: the
//     reference runtime never built that alternative at all.
//   - perl "print $fh, \"a\", \"b\";". The two live derivations nest
//     list_expression (333) and ambiguous_function_call_expression (395) the
//     opposite way round. Rung 6 selects the list_expression nesting; the C
//     oracle publishes the ambiguous_function_call_expression nesting.
//
// In both, the oracle keeps the alternative rung 6 ranks SECOND, so no
// ordering of these candidate sets reproduces the oracle. The defect is not
// in this port: it is that the compact core's live-derivation set at a tied
// accept is not the reference runtime's live-version set. The compact
// scheduler re-lexes and forks per header, and its condense step
// deliberately defers precedence ties instead of collapsing them
// (Core.condense, core.go), so it can carry alternatives the reference
// runtime never creates. Ranking a set that contains such an alternative
// cannot recover the reference answer, whatever the ranking.
//
// So this file lands as a verified library with its own tests, and the
// shipping acceptance election keeps its fail-closed decline unchanged. The
// prerequisite for using it is a live-derivation set proven equal to the
// reference runtime's live-version set; that is a scheduler and lexer
// question, not a selection question, and it is not solved here.
// ---------------------------------------------------------------------------

// ErrAcceptanceElectionEmpty reports that an election ran over no candidates.
// The compact driver never calls the election with an empty candidate set;
// this exists so a caller error surfaces as an error instead of an index
// panic.
var ErrAcceptanceElectionEmpty = errors.New("parser-core phase zero: acceptance election requires at least one derivation")

// AcceptanceElectionCompare is the compact port of ts_subtree_compare
// (subtree.c:591-618). It returns -1 when a sorts before b, 1 when b sorts
// before a, and 0 when the two compare equal on every axis C inspects.
//
// C compares exactly three things, in this order: the raw subtree symbol
// (LOWER symbol id sorts first), the raw child count (FEWER children sorts
// first), and then the children pairwise, left to right, recursively. It
// does NOT compare spans, production ids, field maps, alias sequences,
// extra/external flags, or dynamic precedence -- those are decided by the
// earlier rungs of ts_parser__select_tree, or not at all. Two candidates
// that differ only in a field assignment therefore compare EQUAL here, and
// the election's fold resolves them by C's own "keep left" tie answer.
//
// The C implementation is iterative over an explicit stack whose children
// are pushed last-to-first so the pairs pop left-to-right; this port keeps
// that shape (rather than recursing) so a deep tree cannot grow the Go
// stack, and so the traversal order is the same one the oracle takes.
//
// src supplies the symbol and children of each id. RecoveryCostSource is
// reused rather than introducing a second, near-identical source interface:
// RecoveryCostNode already carries both fields this comparison reads, and
// the election needs the same source for its error-cost rungs anyway.
func AcceptanceElectionCompare(src RecoveryCostSource, a, b SubtreeID) (int, error) {
	if src == nil {
		return 0, errors.New("parser-core phase zero: acceptance election requires a subtree source")
	}
	var scratch acceptanceElectionCompareScratch
	return acceptanceElectionCompare(src, &scratch, a, b)
}

// acceptanceElectionCompareScratch carries the explicit pair stack across one
// comparison so a whole election reuses one allocation instead of one per
// compared pair.
type acceptanceElectionCompareScratch struct {
	pairs []acceptanceElectionPair
}

type acceptanceElectionPair struct {
	left  SubtreeID
	right SubtreeID
}

func acceptanceElectionCompare(src RecoveryCostSource, scratch *acceptanceElectionCompareScratch, a, b SubtreeID) (int, error) {
	scratch.pairs = append(scratch.pairs[:0], acceptanceElectionPair{left: a, right: b})
	for len(scratch.pairs) > 0 {
		pair := scratch.pairs[len(scratch.pairs)-1]
		scratch.pairs = scratch.pairs[:len(scratch.pairs)-1]
		// A zero id is the compact analogue of C's NULL subtree. C never
		// stores one in a children array, so neither side should ever be
		// zero here; order the pair deterministically rather than
		// dereferencing, and keep the walk going.
		if pair.left == 0 || pair.right == 0 {
			if pair.left == pair.right {
				continue
			}
			scratch.pairs = scratch.pairs[:0]
			if pair.left == 0 {
				return -1, nil
			}
			return 1, nil
		}
		left, err := src.RecoveryCostNode(pair.left)
		if err != nil {
			return 0, fmt.Errorf("parser-core phase zero: acceptance election subtree %d: %w", pair.left, err)
		}
		right, err := src.RecoveryCostNode(pair.right)
		if err != nil {
			return 0, fmt.Errorf("parser-core phase zero: acceptance election subtree %d: %w", pair.right, err)
		}
		result := 0
		switch {
		case left.Symbol < right.Symbol:
			result = -1
		case right.Symbol < left.Symbol:
			result = 1
		case len(left.Children) < len(right.Children):
			result = -1
		case len(right.Children) < len(left.Children):
			result = 1
		}
		if result != 0 {
			scratch.pairs = scratch.pairs[:0]
			return result, nil
		}
		for i := len(left.Children); i > 0; i-- {
			scratch.pairs = append(scratch.pairs, acceptanceElectionPair{
				left:  left.Children[i-1],
				right: right.Children[i-1],
			})
		}
	}
	return 0, nil
}

// AcceptanceElectionCompareRoots applies AcceptanceElectionCompare to two
// accepted derivations' payload roots.
//
// One compact derivation is a LIST of accepted payload roots, not a single
// subtree: C's ts_parser__accept pops every tree off the accepted version's
// stack and wraps them in one root node whose children are exactly those
// trees (parser.c). Comparing the two lists the way ts_subtree_compare
// compares one node's children -- count first, then pairwise, left to right
// -- is therefore the same comparison C runs on the roots it would have
// built. The wrapping root's own symbol is the grammar's start symbol on
// both sides (the same accepted state), so the symbol rung is vacuous at
// this level and is skipped rather than fabricated.
//
// Production's port of the same list-level comparison is
// compareRawReduceWindows (parser_reduce.go): length first, then elementwise.
func AcceptanceElectionCompareRoots(src RecoveryCostSource, a, b []SubtreeID) (int, error) {
	if src == nil {
		return 0, errors.New("parser-core phase zero: acceptance election requires a subtree source")
	}
	var scratch acceptanceElectionCompareScratch
	return acceptanceElectionCompareRoots(src, &scratch, a, b)
}

func acceptanceElectionCompareRoots(src RecoveryCostSource, scratch *acceptanceElectionCompareScratch, a, b []SubtreeID) (int, error) {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1, nil
		}
		return 1, nil
	}
	for i := range a {
		cmp, err := acceptanceElectionCompare(src, scratch, a[i], b[i])
		if err != nil {
			return 0, err
		}
		if cmp != 0 {
			return cmp, nil
		}
	}
	return 0, nil
}

// AcceptanceElectionErrorCost is the compact analogue of cStackErrorCost
// (parser_recover_c.go) for one accepted derivation: the sum of every
// payload root's own subtree error cost.
//
// C stores error_cost on each subtree and sums children into the parent
// during ts_subtree_summarize_children (subtree.c), so the root
// ts_parser__accept builds carries exactly this sum. Summing the roots here
// reproduces that number without building the root.
//
// memo may be nil. Passing a shared memo across the whole election makes the
// total work linear in the number of DISTINCT subtrees the candidates
// reference, not in candidates times tree size: tied derivations of one
// accepted head share almost all of their subtrees.
//
// On a clean, non-recovery accept every payload's cost is 0: compact
// publishes an ERROR container only under an admitted native strategy-2
// error region (error_region.go), and it never publishes a MISSING node at
// all. This function is still computed rather than assumed, because an
// admitted error region CAN reach an accept with a nonzero-cost payload, and
// rungs 1, 2, and 5 of the C ladder are only correct when the real number is
// used.
func AcceptanceElectionErrorCost(
	symbols []SelectedSymbolPolicy, src RecoveryCostSource, memo *RecoveryCostMemo, payloads []SubtreeID,
) (uint32, error) {
	if src == nil {
		return 0, errors.New("parser-core phase zero: acceptance election requires a subtree source")
	}
	var total uint32
	for _, id := range payloads {
		if id == 0 {
			continue
		}
		cost, err := recoveryNodeErrorCost(symbols, src, memo, id)
		if err != nil {
			return 0, err
		}
		total += cost
	}
	return total, nil
}

// AcceptanceElectionOutcome reports which candidate the election elected and
// the evidence behind the pick, so callers can log or assert against the C
// oracle without re-running the ladder.
type AcceptanceElectionOutcome struct {
	// Index is the elected candidate's position in the caller's slice.
	Index int
	// ErrorCost is the elected candidate's derivation error cost.
	ErrorCost uint32
	// Rung names the LAST ladder rung that moved the incumbent, or
	// "sole-candidate" when only one candidate existed, or "incumbent" when
	// no challenger ever displaced the first candidate.
	Rung string
	// MaxErrorCost is the highest derivation error cost seen across every
	// candidate. Zero proves the whole election ran on the clean path, where
	// rungs 1, 2, and 5 are vacuous.
	MaxErrorCost uint32
	// ComparatorDecided reports whether rung 6 (ts_subtree_compare) ever
	// returned a nonzero answer during the fold: the election needed the
	// structural comparison, not just error cost and dynamic precedence.
	ComparatorDecided bool
	// StructurallyTied reports whether rung 6 answered 0 for at least one
	// challenger: two candidates that C itself cannot order structurally,
	// resolved by C's own "keep left" answer.
	StructurallyTied bool
}

// Rung names for AcceptanceElectionOutcome.Rung.
const (
	AcceptanceElectionRungSoleCandidate = "sole-candidate"
	AcceptanceElectionRungIncumbent     = "incumbent"
	AcceptanceElectionRungErrorCost     = "error-cost"
	AcceptanceElectionRungDynamicPrec   = "dynamic-precedence"
	AcceptanceElectionRungErrorTakeLate = "error-cost-tie-takes-later"
	AcceptanceElectionRungComparator    = "subtree-compare"
)

// AcceptanceElectionFoldOrder returns the candidate indexes in the order the
// C runtime would have folded them, appended to dst.
//
// THIS ORDER IS LOAD-BEARING, not cosmetic. Rung 6 of the ladder answers 0
// whenever two candidates share every axis ts_subtree_compare reads, and C
// resolves that answer by KEEPING THE INCUMBENT. Whichever candidate the fold
// starts from therefore wins every structurally tied election -- including
// the whole family where two derivations share a symbol and shape and differ
// only in which production built them, and so only in the field map the
// published tree carries.
//
// C's incumbent is the lower stack version index. Version indices are handed
// out in creation order, and ts_parser__advance creates them by walking a
// conflict cell's actions from ordinal 0 upward: ordinal 0's reduction makes
// the first new version, each later ordinal makes a higher-numbered one. When
// two versions later merge, ts_stack_merge(stack, j, i) always keeps the
// lower index j and folds i into it (parser.c ts_parser__condense_stack,
// ts_parser__reduce). So C's fold order is: the primary arm first, then each
// secondary arm in ascending action-ordinal order.
//
// Core.Derivations enumerates in link-insertion order, which is a property of
// how the compact conflict executor and condense step happened to publish
// links, not a statement about C's version numbering. This function does not
// rely on it. The fork stamps the compact core already records rebuild C's
// order directly: a link created by a secondary arm carries that dispatch's
// ForkOrder, a link created by the primary arm carries none, and
// Core.Derivations propagates the latest stamp seen along a path. So:
//
//   - a candidate with no stamp never left the primary arm at any conflict.
//     It is C's original, un-forked version: fold key 0, first.
//   - a candidate with a stamp forked at the dispatch that stamped it. Fork
//     stamps come from one monotone per-parse counter that increments in
//     action-ordinal order within a dispatch and in dispatch order across
//     the parse, so ascending stamp order is ascending version-creation
//     order: fold key stamp+1.
//
// The sort is stable, so two candidates carrying the same key keep their
// enumeration order relative to each other.
func AcceptanceElectionFoldOrder(dst []int, paths []Derivation) []int {
	dst = dst[:0]
	for index := range paths {
		dst = append(dst, index)
	}
	// Insertion sort, stable, over a candidate set bounded by the driver's
	// own election cap. It avoids a sort.Slice closure allocation on a path
	// that runs inside an accept.
	for i := 1; i < len(dst); i++ {
		value := dst[i]
		key := acceptanceElectionFoldKey(paths[value])
		j := i - 1
		for j >= 0 && acceptanceElectionFoldKey(paths[dst[j]]) > key {
			dst[j+1] = dst[j]
			j--
		}
		dst[j+1] = value
	}
	return dst
}

func acceptanceElectionFoldKey(path Derivation) uint64 {
	if !path.HasBranchOrder {
		return 0
	}
	if path.BranchOrder == ^uint64(0) {
		return ^uint64(0)
	}
	return path.BranchOrder + 1
}

// ElectAcceptanceDerivation folds ts_parser__select_tree over paths and
// reports the elected candidate.
//
// The fold is C's own: the first candidate in C's version order is the
// incumbent ("left"), every later candidate challenges it ("right"), and a
// challenger replaces the incumbent exactly when ts_parser__select_tree would
// have returned true. AcceptanceElectionFoldOrder supplies that version
// order; it is NOT the caller's enumeration order, and the difference decides
// every structurally tied election.
//
// symbols is the visible-symbol policy the error-cost rungs read
// (SelectedStorePolicy.Symbols). src resolves every referenced subtree.
// memo may be nil; supplying one makes the error-cost rungs linear in the
// number of distinct subtrees.
//
// The returned outcome always names a candidate when err is nil: the ladder
// is total, so this function has no "cannot order" answer to report. Index is
// a position in the caller's own paths slice, not in the fold order.
func ElectAcceptanceDerivation(
	symbols []SelectedSymbolPolicy, src RecoveryCostSource, memo *RecoveryCostMemo, paths []Derivation,
) (AcceptanceElectionOutcome, error) {
	if len(paths) == 0 {
		return AcceptanceElectionOutcome{}, ErrAcceptanceElectionEmpty
	}
	if src == nil {
		return AcceptanceElectionOutcome{}, errors.New("parser-core phase zero: acceptance election requires a subtree source")
	}
	order := AcceptanceElectionFoldOrder(nil, paths)
	incumbentCost, err := AcceptanceElectionErrorCost(symbols, src, memo, paths[order[0]].Payloads)
	if err != nil {
		return AcceptanceElectionOutcome{}, err
	}
	outcome := AcceptanceElectionOutcome{
		Index:        order[0],
		ErrorCost:    incumbentCost,
		Rung:         AcceptanceElectionRungIncumbent,
		MaxErrorCost: incumbentCost,
	}
	if len(paths) == 1 {
		outcome.Rung = AcceptanceElectionRungSoleCandidate
		return outcome, nil
	}
	var scratch acceptanceElectionCompareScratch
	for _, index := range order[1:] {
		challenger := paths[index]
		challengerCost, err := AcceptanceElectionErrorCost(symbols, src, memo, challenger.Payloads)
		if err != nil {
			return AcceptanceElectionOutcome{}, err
		}
		if challengerCost > outcome.MaxErrorCost {
			outcome.MaxErrorCost = challengerCost
		}
		take, rung, err := acceptanceElectionTakeChallenger(
			src, &scratch, paths[outcome.Index], incumbentCost, challenger, challengerCost, &outcome,
		)
		if err != nil {
			return AcceptanceElectionOutcome{}, err
		}
		if take {
			outcome.Index = index
			outcome.Rung = rung
			incumbentCost = challengerCost
		}
	}
	outcome.ErrorCost = incumbentCost
	return outcome, nil
}

// acceptanceElectionTakeChallenger is one ts_parser__select_tree call:
// reports whether right replaces left, and which rung decided it.
func acceptanceElectionTakeChallenger(
	src RecoveryCostSource, scratch *acceptanceElectionCompareScratch,
	left Derivation, leftCost uint32, right Derivation, rightCost uint32,
	outcome *AcceptanceElectionOutcome,
) (bool, string, error) {
	if rightCost < leftCost {
		return true, AcceptanceElectionRungErrorCost, nil
	}
	if leftCost < rightCost {
		return false, "", nil
	}
	if right.Score > left.Score {
		return true, AcceptanceElectionRungDynamicPrec, nil
	}
	if left.Score > right.Score {
		return false, "", nil
	}
	if leftCost > 0 {
		return true, AcceptanceElectionRungErrorTakeLate, nil
	}
	cmp, err := acceptanceElectionCompareRoots(src, scratch, left.Payloads, right.Payloads)
	if err != nil {
		return false, "", err
	}
	switch {
	case cmp < 0:
		outcome.ComparatorDecided = true
		return false, "", nil
	case cmp > 0:
		outcome.ComparatorDecided = true
		return true, AcceptanceElectionRungComparator, nil
	default:
		outcome.StructurallyTied = true
		return false, "", nil
	}
}

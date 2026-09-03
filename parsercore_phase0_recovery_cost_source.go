//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"
	"sort"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// ---------------------------------------------------------------------------
// parsercore_phase0_recovery_cost_source.go -- campaign v7 tranche B3: the
// bridge that lets the compact scheduler PRICE a recovery lineage with the
// stage S2 cost model (internal/parsercorephase0/recovery_cost.go).
//
// Why this exists. Stage S5 (missing-token insertion) cannot decide anything
// on its own. C does not choose between inserting a missing token and
// absorbing into an ERROR region at the point of failure: ts_parser__handle_error
// creates the missing version as a copy and pushes the error discontinuity
// onto the original anyway, so both versions parse forward, and the winner is
// settled later by error cost (ts_parser__select_tree,
// ts_parser__better_version_exists). Measured on php "<?php namespace ; ?>",
// the missing version costs 610 and the absorbing version 609
// (500 recovery + 9 skipped chars + 100 for one visible child), so C publishes
// an ERROR tree even though its own missing-token search accepted the
// insertion. The margin is ONE point: every term of this model has to be
// present, or the arbitration inverts. Pricing a lineage is therefore the prerequisite for either
// recovery stage to be correct, not an optimization.
//
// Stage S2 deliberately shipped its cost model with no caller
// ("RecoveryCostSource resolves a SubtreeID to its cost-model view"), leaving
// the source implementation to whichever stage first needed it. This file is
// that implementation. It reads only exported Core surface
// (MaterializationView, Derivations), so the compact core needs no change to
// support it.
//
// RATCHET NOTE. internal/parsercorephase0's own
// TestRecoveryCostNoOutsideCallSitesRatchet proves the cost model has no
// caller, but it parses only that package's files. recovery_cost.go's API is
// exported, so a caller in THIS package does not trip it and the ratchet stays
// green while the model is, in fact, now reachable. The ratchet's guarantee is
// therefore narrower than its doc comment claims. Read it as "no caller inside
// the compact core", not "no caller anywhere".
// ---------------------------------------------------------------------------

// diagnosticParserCoreRecoveryCostSource adapts a compact Core to the stage S2
// cost model's RecoveryCostSource interface.
//
// The compact view supplies the missing bit. The compact record does not store
// row spans. This source computes rows from the text only for ERROR nodes.
// It builds the newline index once per parse and only when pricing needs it.
type diagnosticParserCoreRecoveryCostSource struct {
	compact *core.Core
	source  []byte
	// newlines holds the byte offset of every '\n' in source, ascending.
	// newlinesBuilt, not the slice length, records whether the scan has run: a
	// source with no newline leaves the slice empty, which is indistinguishable
	// from "not scanned yet" by length alone.
	//
	// Neither field is reset. Bind one source per cost source and build a new
	// one for the next parse; reassigning `source` in place would keep
	// answering row queries from the previous text.
	newlines      []uint32
	newlinesBuilt bool
}

// newDiagnosticParserCoreRecoveryCostSource binds a cost source to one Core
// and one source text.
//
// It is the only supported way to build one. Constructing the struct by
// literal makes `source` look optional, and an absent or short source is not a
// harmless default: rowAt would answer 0 for every offset, silently dropping
// RecoveryCostPerSkippedLine (30 per line) from every ERROR region. A
// twenty-line region would then be under-priced by 600, which is more than an
// entire missing insertion costs, and the arbitration inverts.
func newDiagnosticParserCoreRecoveryCostSource(compact *core.Core, source []byte) (*diagnosticParserCoreRecoveryCostSource, error) {
	if compact == nil {
		return nil, errors.New("parser-core phase zero: recovery cost source requires a compact core")
	}
	if uint64(len(source)) > uint64(^uint32(0)) {
		return nil, errors.New("parser-core phase zero: recovery cost source exceeds uint32 offsets")
	}
	return &diagnosticParserCoreRecoveryCostSource{compact: compact, source: source}, nil
}

// rowAt returns the zero-based row containing byteOffset: the count of
// newlines strictly before the offset.
//
// It does NOT reuse diagnosticParserCorePointIndex (parsercore_phase0_driver.go),
// which answers the same question with a cancellation poll and a reusable
// buffer. That helper is built per materialization against a runner scratch;
// this source is built per arbitration and may outlive no scratch at all.
// The scan is bounded by the source length and runs once per source.
func (s *diagnosticParserCoreRecoveryCostSource) rowAt(byteOffset uint32) uint32 {
	if !s.newlinesBuilt {
		s.newlines = s.newlines[:0]
		for index, b := range s.source {
			if b == '\n' {
				s.newlines = append(s.newlines, uint32(index))
			}
		}
		s.newlinesBuilt = true
	}
	// sort.Search over an empty slice already returns 0 without calling the
	// predicate, so no length guard is needed here.
	return uint32(sort.Search(len(s.newlines), func(i int) bool {
		return s.newlines[i] >= byteOffset
	}))
}

// RecoveryCostNode implements core.RecoveryCostSource.
//
// Children and aliases are COPIED, not retained. MaterializationView documents
// both slices as borrowed arena storage. The cost model holds them across
// recursive calls that re-enter this method.
func (s *diagnosticParserCoreRecoveryCostSource) RecoveryCostNode(id core.SubtreeID) (core.RecoveryCostNode, error) {
	if s == nil || s.compact == nil {
		return core.RecoveryCostNode{}, errors.New("parser-core phase zero: recovery cost source has no compact core")
	}
	view, err := s.compact.MaterializationView(id)
	if err != nil {
		return core.RecoveryCostNode{}, err
	}
	node := core.RecoveryCostNode{
		Symbol:    view.Symbol,
		Extra:     view.Extra,
		Missing:   view.Missing,
		StartByte: view.StartByte,
		EndByte:   view.EndByte,
	}
	// Only an ERROR node's row extent is ever read (recovery_cost.go), so
	// every other node skips the newline scan entirely.
	if view.Symbol == core.RecoveryErrorSymbol {
		node.StartRow = s.rowAt(view.StartByte)
		node.EndRow = s.rowAt(view.EndByte)
	}
	if len(view.Children) != 0 {
		node.Children = append(make([]core.SubtreeID, 0, len(view.Children)), view.Children...)
	}
	if len(view.Aliases) != 0 {
		node.Aliases = append(make([]core.Symbol, 0, len(view.Aliases)), view.Aliases...)
	}
	return node, nil
}

// diagnosticParserCoreLineageCostUnavailable reports that a head could not be
// priced because it does not carry exactly one derivation. Every recovery
// arbitration this supports is defined on a single lineage; an ambiguous head
// is a shape the caller must decline rather than price arbitrarily.
var diagnosticParserCoreLineageCostUnavailable = errors.New("parser-core phase zero: recovery lineage cost requires exactly one derivation")

// diagnosticParserCoreLineageErrorCost prices one head the way C prices a
// stack version: ts_stack_error_cost sums ts_subtree_error_cost over the
// version's stack nodes (stack.c:482-490, ported as cStackErrorCost in
// parser_recover_c.go). The compact equivalent of "the version's stack nodes"
// is the payload list of the head's single derivation, which is exactly the
// list materialization already walks.
//
// The +500 that C adds for a version parked at ERROR_STATE with no open node
// is NOT added here. That term belongs to the head's live recovery state, not
// to its published subtrees, so it is RecoveryVersionStatus's hasOpenRecovery
// input and stays the caller's to supply -- the same split stage S2 already
// documented when it declined to port cNodeCountSinceError.
func diagnosticParserCoreLineageErrorCost(
	compact *core.Core,
	head core.Head,
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
	memo *core.RecoveryCostMemo,
) (uint32, error) {
	if compact == nil || src == nil {
		return 0, errors.New("parser-core phase zero: recovery lineage cost requires a compact core and source")
	}
	derivations, err := compact.Derivations(head)
	if err != nil {
		return 0, err
	}
	if len(derivations) != 1 {
		return 0, diagnosticParserCoreLineageCostUnavailable
	}
	return diagnosticParserCoreDerivationErrorCost(symbols, src, memo, derivations[0])
}

// diagnosticParserCoreDerivationErrorCost prices one already-resolved
// derivation. It exists so a caller that has to read Derivations anyway --
// pricing needs the payload list, ordering needs the Score -- does not resolve
// the same head twice.
func diagnosticParserCoreDerivationErrorCost(
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
	memo *core.RecoveryCostMemo,
	derivation core.Derivation,
) (uint32, error) {
	var total uint32
	for _, payload := range derivation.Payloads {
		if payload == 0 {
			continue
		}
		cost, err := core.RecoveryNodeErrorCostMemo(symbols, src, memo, payload)
		if err != nil {
			return 0, err
		}
		total += cost
	}
	return total, nil
}

// diagnosticParserCoreLineage is one competing recovery lineage at an
// accepting frontier, together with the price the selection ladder reads.
type diagnosticParserCoreLineage struct {
	Head core.Head
	// Cost is the lineage's total error cost: C's ts_stack_error_cost, which
	// is the sum over published subtrees PLUS the open-recovery term below.
	Cost uint32
	// Score is the summed dynamic precedence of the lineage's sole
	// derivation, C's ts_subtree_dynamic_precedence.
	Score int64
}

// diagnosticParserCoreLineageInput names one candidate head together with the
// live recovery state that pricing cannot read off the published subtrees.
//
// OpenRecoverySegments is the compact analogue of cStackOpenRecoveryCost
// (parser_recover_c.go:2475-2489), which is the half of C's
// ts_stack_error_cost that does NOT come from the arena. C charges
// ERROR_COST_PER_RECOVERY once for a version that is paused or sitting at
// ERROR_STATE with no open error node yet, and once more for each extra
// error_repeat segment an unlexable-run re-pause opened. Count both here.
//
// Omitting this term is not a rounding error. A head paused with an empty
// open region prices at 0 instead of 500, and the arbitration this module
// exists for turns on a margin of one point.
type diagnosticParserCoreLineageInput struct {
	Head                 core.Head
	OpenRecoverySegments int
}

// errDiagnosticParserCoreLineageTie reports that the selection ladder ran out
// of C-defined tiebreaks. C's last resort is ts_subtree_compare, a structural
// comparison it reaches only when BOTH candidates are clean
// (ts_parser__select_tree returns early for a positive error cost). Every
// lineage this selection exists to arbitrate carries recovery content, so
// reaching that clause means the caller handed in a shape this port does not
// model, and the honest answer is to decline rather than pick one.
var errDiagnosticParserCoreLineageTie = errors.New("parser-core phase zero: competing recovery lineages tie beyond the modeled selection ladder")

// diagnosticParserCorePriceLineages prices each candidate head so the
// selection ladder can order them. A physically merged head is one stack
// version, not several recovery competitors. The graph aggregate admits it
// only when every physical path has the same authenticated recovery cost.
func diagnosticParserCorePriceLineages(
	inputs []diagnosticParserCoreLineageInput,
	symbols []core.SelectedSymbolPolicy,
	src *diagnosticParserCoreRecoveryCostSource,
	_ *core.RecoveryCostMemo,
) ([]diagnosticParserCoreLineage, error) {
	if src == nil || src.compact == nil {
		return nil, errors.New("parser-core phase zero: lineage pricing requires a bound cost source")
	}
	out := make([]diagnosticParserCoreLineage, 0, len(inputs))
	for _, in := range inputs {
		aggregate, supported, err := src.compact.RecoveryGraphAggregateForHead(in.Head, symbols, src)
		if err != nil {
			if errors.Is(err, core.RecoveryGraphAggregateLimitError) {
				return nil, diagnosticParserCoreLineageCostUnavailable
			}
			return nil, err
		}
		if !supported {
			return nil, diagnosticParserCoreLineageCostUnavailable
		}
		if in.OpenRecoverySegments < 0 {
			return nil, errors.New("parser-core phase zero: negative open-recovery segment count")
		}
		openCost := uint64(in.OpenRecoverySegments) * uint64(core.RecoveryCostPerRecovery)
		if openCost > math.MaxUint32 || uint64(aggregate.MinimumErrorCost)+openCost > math.MaxUint32 {
			return nil, errors.New("parser-core phase zero: recovery lineage cost overflow")
		}
		cost := aggregate.MinimumErrorCost + uint32(openCost)
		out = append(out, diagnosticParserCoreLineage{
			Head: in.Head, Cost: cost, Score: aggregate.StoredPrecedenceMaximum,
		})
	}
	return out, nil
}

// diagnosticParserCoreSelectRecoveryLineage returns the index of the lineage C
// would publish, porting ts_parser__select_tree's ordering (parser.c:836-878).
//
// C folds every accepted root into one winner pairwise, asking for each new
// candidate whether it should REPLACE the incumbent. Its ladder, in order:
//
//  1. lower error cost wins;
//  2. then higher dynamic precedence wins;
//  3. then, when the incumbent's error cost is positive, the LATER candidate
//     wins (parser.c:864, `if (ts_subtree_error_cost(left) > 0) return true`);
//  4. otherwise C compares the trees structurally (ts_subtree_compare).
//
// Clause 3 is not a coin flip and must not be "simplified" to keeping the
// incumbent: it is what lets a later, equally priced recovery displace an
// earlier one. Clause 4 is unreachable here, because it requires BOTH sides to
// be clean, and a lineage with zero error cost is not a recovery lineage at
// all; this port returns errDiagnosticParserCoreLineageTie instead of guessing.
//
// The order of lineages is therefore load-bearing. Callers must pass them in
// the scheduler's own publication order, which is the compact analogue of C's
// version index order.
func diagnosticParserCoreSelectRecoveryLineage(lineages []diagnosticParserCoreLineage) (int, error) {
	if len(lineages) == 0 {
		return 0, errors.New("parser-core phase zero: no recovery lineage to select")
	}
	// Fold left to right, keeping the incumbent when a pair is unresolvable.
	// An unresolvable PAIR is not an unresolvable RESULT: C folds the same way
	// and a later candidate can dominate both sides of an earlier tie on cost
	// or precedence. Declining at the pair would refuse inputs C decides.
	winner := 0
	for candidate := 1; candidate < len(lineages); candidate++ {
		replace, resolved := diagnosticParserCoreLineageReplaces(lineages[winner], lineages[candidate])
		if resolved && replace {
			winner = candidate
		}
	}
	// Only the FINAL winner has to be decided. If any other lineage still ties
	// with it unresolvably, the answer genuinely depends on the structural
	// comparison this port does not model, so decline rather than guess.
	for other := range lineages {
		if other == winner {
			continue
		}
		if _, resolved := diagnosticParserCoreLineageReplaces(lineages[winner], lineages[other]); !resolved {
			return 0, errDiagnosticParserCoreLineageTie
		}
	}
	return winner, nil
}

// diagnosticParserCoreLineageReplaces reports whether candidate should replace
// incumbent, mirroring ts_parser__select_tree's return value exactly (true
// means "take the right-hand side").
//
// resolved is false only for C's final clause, the structural
// ts_subtree_compare, which this port does not model. That clause is reachable
// only when BOTH sides are clean, because parser.c:864 returns early for a
// positive error cost.
func diagnosticParserCoreLineageReplaces(incumbent, candidate diagnosticParserCoreLineage) (replace, resolved bool) {
	if candidate.Cost < incumbent.Cost {
		return true, true
	}
	if incumbent.Cost < candidate.Cost {
		return false, true
	}
	if candidate.Score > incumbent.Score {
		return true, true
	}
	if incumbent.Score > candidate.Score {
		return false, true
	}
	if incumbent.Cost > 0 {
		// C parser.c:864. Equal cost, equal precedence, and the incumbent
		// carries error content: the later candidate wins.
		return true, true
	}
	// Both sides are clean and tied. C would compare the trees structurally.
	return false, false
}

// diagnosticParserCoreRecoverySymbolPolicy projects the visibility signal the
// cost model reads out of a Language's own symbol metadata.
//
// It reproduces buildParserCoreSelectedStorePolicy's width and default rule
// EXACTLY, and both halves are load-bearing:
//
//   - Width is max(len(SymbolMetadata), len(SymbolNames), SymbolCount), not
//     len(SymbolMetadata). SymbolMetadata can be shorter than the symbol space
//     -- the embedded loader pads it up to len(SymbolNames) precisely because
//     some blobs arrive short.
//   - Symbols past the metadata end default to VISIBLE. RecoverySymbolVisible
//     answers false for an out-of-range index, so a short slice would price
//     every such child invisible.
//
// Getting either wrong drops RecoveryCostPerSkippedTree (100) per absorbed
// visible child. The measured php arbitration turns on a single point, so an
// under-counted child inverts it and the route publishes a tree C would not.
func diagnosticParserCoreRecoverySymbolPolicy(lang *Language) []core.SelectedSymbolPolicy {
	if lang == nil {
		return nil
	}
	width := max(len(lang.SymbolMetadata), len(lang.SymbolNames), int(lang.SymbolCount))
	out := make([]core.SelectedSymbolPolicy, width)
	for index := range out {
		visible, named := true, false
		if index < len(lang.SymbolMetadata) {
			visible = lang.SymbolMetadata[index].Visible
			named = lang.SymbolMetadata[index].Named
		}
		out[index] = core.SelectedSymbolPolicy{Visible: visible, Named: named}
	}
	return out
}

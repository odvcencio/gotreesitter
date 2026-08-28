//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
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
// the missing version costs 610 and the absorbing version about 509, so C
// publishes an ERROR tree even though its own missing-token search accepted
// the insertion. Pricing a lineage is therefore the prerequisite for either
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
// The cost model needs two things a compact subtree record does not store:
// the missing bit (now carried, stage S5 substrate) and a row span. Rows are
// read for ERROR nodes only -- RecoveryCostPerSkippedLine times the node's row
// extent -- so this source computes them from the source text on demand rather
// than widening the record. newlines is built once per parse, lazily, and only
// if some ERROR node actually asks.
type diagnosticParserCoreRecoveryCostSource struct {
	compact *core.Core
	source  []byte
	// newlines holds the byte offset of every '\n' in source, ascending. A
	// nil slice with newlinesBuilt false means "not computed yet"; a source
	// with no newline at all builds an empty non-nil slice, so the flag, not
	// the length, records whether the scan ran.
	newlines      []uint32
	newlinesBuilt bool
}

// rowAt returns the zero-based row containing byteOffset, matching how the
// public tree numbers points: the row is the count of newlines strictly
// before the offset.
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
	if len(s.newlines) == 0 {
		return 0
	}
	return uint32(sort.Search(len(s.newlines), func(i int) bool {
		return s.newlines[i] >= byteOffset
	}))
}

// RecoveryCostNode implements core.RecoveryCostSource.
//
// Children are COPIED, not aliased. MaterializationView documents its
// Children slice as borrowed arena storage that must not be retained, and the
// cost model holds a node's children across recursive calls that re-enter this
// method. Copying is the cheap, obviously-correct response; this path runs at
// a recovery decision point, never on the clean hot path.
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
	var total uint32
	for _, payload := range derivations[0].Payloads {
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

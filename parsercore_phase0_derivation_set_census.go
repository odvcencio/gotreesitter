//go:build !gts_no_parsercorephase0 && gts_derivation_set_census

package gotreesitter

import (
	"fmt"
	"strings"
	"sync"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Derivation-set differential instrument, compact side (stage D0 of
// spec.derivation-set-equivalence.v1).
//
// The lane's prerequisite is a proof that the compact core's live derivation
// set D at an accept equals the reference runtime's live version set V.
// Selection cannot repair a wrong candidate set
// (finding.derivation-set-equivalence-prerequisite), so the set difference
// must first be MEASURED. This file publishes D.
//
// The whole file is behind the gts_derivation_set_census build tag. The
// shipped build compiles
// parsercore_phase0_derivation_set_census_disabled.go instead, whose
// recordDerivationSetCensusAccept is an empty function the inliner removes.
// The default binary therefore contains no census code at all, so the
// instrument cannot cost the shipped path -- a stronger guarantee than an
// env-var read.
//
// The census NEVER influences routing. completeAcceptance calls it after the
// acceptance derivation set is already enumerated and after the materiality
// gate has already decided; the recorded decision is copied, never made
// here. Materializing every candidate is expensive, which is the second
// reason for the build tag.

// DerivationSetCensusCandidate is one live compact derivation at an accept:
// one root-to-head path through the persistent link DAG
// (internal/parsercorephase0/core.go, Core.Derivations).
type DerivationSetCensusCandidate struct {
	// FoldIndex is the candidate's position in the compact fold order the
	// spec defines: the unstamped primary first, then stamped links by
	// ascending fork stamp. Criterion 4 of the equivalence statement
	// compares this position against C's version-index position.
	FoldIndex int
	// Score is the summed dynamic precedence of the path (core.Derivation
	// Score). The spec's criterion 3 compares it with C's dynamic
	// precedence.
	Score int64
	// BranchOrder is the fork stamp, present only when HasBranchOrder is
	// true. An absent stamp marks the primary path.
	BranchOrder    uint64
	HasBranchOrder bool
	// PayloadCount is the number of stack payloads on the path. C's
	// equivalent is the popped slice length at ts_parser__accept.
	PayloadCount int
	// RootSymbol, RootStartByte, RootEndByte and RootChildCount describe the
	// materialized public root, which is what C assembles from its popped
	// slice.
	RootSymbol     string
	RootStartByte  uint32
	RootEndByte    uint32
	RootChildCount int
	// Shape is the canonical deep dump used as this candidate's identity:
	// symbol, span, named/extra/missing flags, field names, and children,
	// recursively. Two candidates with equal Shape publish the same public
	// tree, which is the same predicate the materiality gate
	// (compactAcceptanceDerivationTreesEqual) applies.
	Shape string
	// Nodes is the same materialized tree in pre-order, one entry per node.
	// The differential rebuilds the tree from it to attribute a set
	// difference to a mechanism, which a flat string cannot support.
	Nodes []DerivationSetCensusNode
	// MaterializeError records a failed trial materialization. The instrument
	// reports such a candidate as unclassifiable rather than guessing.
	MaterializeError string
}

// DerivationSetCensusNode is one node of a materialized candidate, recorded
// in pre-order. ChildCount lets a reader rebuild the tree from the flat
// slice without a second pass.
type DerivationSetCensusNode struct {
	Field      string
	Type       string
	StartByte  uint32
	EndByte    uint32
	ChildCount int
	Named      bool
	Extra      bool
	Missing    bool
}

// DerivationSetCensusAccept is the derivation set D recorded at one accept
// event, plus the route's own outcome at that accept.
type DerivationSetCensusAccept struct {
	// ElectionIndex is the scheduler election that reached the accept.
	ElectionIndex int
	// ByteOffset is the accepted head's byte offset.
	ByteOffset uint32
	// Candidates is D in compact fold order.
	Candidates []DerivationSetCensusCandidate
	// SelectedFoldIndex is the derivation selectCompactAcceptanceDerivation
	// picked, or -1 when no derivation was selected.
	SelectedFoldIndex int
	// DeclinedMaterial is true when the R1 materiality gate declined this
	// accept because the tied election was not proven vacuous.
	DeclinedMaterial bool
	// EnumerationTruncated is true when the derivation enumeration cap fired
	// and D is incomplete.
	EnumerationTruncated bool
}

var (
	derivationSetCensusMu      sync.Mutex
	derivationSetCensusAccepts []DerivationSetCensusAccept
)

// DerivationSetCensusBuilt reports whether this binary carries the
// derivation-set census. It is true only under the gts_derivation_set_census
// build tag; the shipped build's stub returns false.
func DerivationSetCensusBuilt() bool { return true }

// DerivationSetCensusReset drops every recorded accept. Call it before each
// parse the instrument measures.
func DerivationSetCensusReset() {
	derivationSetCensusMu.Lock()
	derivationSetCensusAccepts = nil
	derivationSetCensusMu.Unlock()
}

// DerivationSetCensusSnapshot returns a copy of every accept recorded since
// the last reset.
func DerivationSetCensusSnapshot() []DerivationSetCensusAccept {
	derivationSetCensusMu.Lock()
	defer derivationSetCensusMu.Unlock()
	out := make([]DerivationSetCensusAccept, len(derivationSetCensusAccepts))
	copy(out, derivationSetCensusAccepts)
	return out
}

// censusAcceptanceDerivationSet records the compact derivation set D at one
// accept. completeAcceptance calls it after the route has already decided, so
// it can only observe. declinedMaterial reports whether the R1 materiality
// gate refused this accept.
func (s *diagnosticParserCoreGenericScheduler) censusAcceptanceDerivationSet(
	paths []core.Derivation, selected core.Derivation, hasSelected bool, declinedMaterial bool,
) {
	if len(s.headers) == 0 {
		return
	}
	recordDerivationSetCensusAccept(
		s.compact, s.options.materializationParser, s.options.materializationSource,
		s.options.materializationForceReplayParseStates, s.options.materializationContextSet,
		s.headers[0].head, paths, selected, hasSelected, declinedMaterial,
		s.electionIndex, s.token.StartByte,
	)
}

// censusAcceptanceDerivationSetTruncated records an accept whose derivation
// enumeration hit the cap, so D is unknown at this accept.
func (s *diagnosticParserCoreGenericScheduler) censusAcceptanceDerivationSetTruncated() {
	recordDerivationSetCensusTruncated(s.electionIndex, s.token.StartByte)
}

// recordDerivationSetCensusAccept records D at one accept. Every failure
// inside it is recorded as text on the candidate; it never returns an error
// and never changes the caller's control flow.
func recordDerivationSetCensusAccept(
	compact *core.Core, parser *Parser, source []byte,
	forceReplayParseStates bool, contextSet bool,
	head core.Head, paths []core.Derivation, selected core.Derivation,
	hasSelected bool, declinedMaterial bool,
	electionIndex int, byteOffset uint32,
) {
	record := DerivationSetCensusAccept{
		ElectionIndex:     electionIndex,
		ByteOffset:        byteOffset,
		SelectedFoldIndex: -1,
		DeclinedMaterial:  declinedMaterial,
	}
	ordered := derivationSetCensusFoldOrder(paths)
	for foldIndex, index := range ordered {
		path := paths[index]
		candidate := DerivationSetCensusCandidate{
			FoldIndex:      foldIndex,
			Score:          path.Score,
			BranchOrder:    path.BranchOrder,
			HasBranchOrder: path.HasBranchOrder,
			PayloadCount:   len(path.Payloads),
		}
		if hasSelected && derivationSetCensusSamePath(path, selected) {
			record.SelectedFoldIndex = foldIndex
		}
		if !contextSet || compact == nil || parser == nil {
			candidate.MaterializeError = "no materialization context"
			record.Candidates = append(record.Candidates, candidate)
			continue
		}
		tree, err := materializeDiagnosticParserCoreAcceptedSelection(
			compact, head, path.Payloads, parser, source, nil, forceReplayParseStates, false,
		)
		if err != nil {
			candidate.MaterializeError = err.Error()
			record.Candidates = append(record.Candidates, candidate)
			continue
		}
		root := tree.RootNode()
		if root != nil {
			candidate.RootSymbol = root.Type(parser.language)
			candidate.RootStartByte = root.StartByte()
			candidate.RootEndByte = root.EndByte()
			candidate.RootChildCount = root.ChildCount()
			var builder strings.Builder
			derivationSetCensusShape(&builder, parser.language, root, "")
			candidate.Shape = builder.String()
			candidate.Nodes = derivationSetCensusNodes(nil, parser.language, root, "")
		} else {
			candidate.MaterializeError = "materialized tree has no root"
		}
		tree.Release()
		record.Candidates = append(record.Candidates, candidate)
	}

	derivationSetCensusMu.Lock()
	derivationSetCensusAccepts = append(derivationSetCensusAccepts, record)
	derivationSetCensusMu.Unlock()
}

// recordDerivationSetCensusTruncated records an accept whose derivation
// enumeration hit the cap, so D is unknown. The instrument must not treat an
// unknown set as an empty one.
func recordDerivationSetCensusTruncated(electionIndex int, byteOffset uint32) {
	derivationSetCensusMu.Lock()
	derivationSetCensusAccepts = append(derivationSetCensusAccepts, DerivationSetCensusAccept{
		ElectionIndex:        electionIndex,
		ByteOffset:           byteOffset,
		SelectedFoldIndex:    -1,
		EnumerationTruncated: true,
	})
	derivationSetCensusMu.Unlock()
}

// derivationSetCensusFoldOrder returns indices into paths in the compact fold
// order the spec names: the unstamped primary first, then stamped paths by
// ascending fork stamp, ties broken by enumeration position so the order is
// total and reproducible. Core.Derivations already enumerates per link and
// predecessor; this only imposes the stamp order the equivalence criterion
// compares against C's version indices.
func derivationSetCensusFoldOrder(paths []core.Derivation) []int {
	order := make([]int, len(paths))
	for index := range paths {
		order[index] = index
	}
	// Insertion sort keeps the enumeration position as the stable tiebreak
	// and stays allocation-free for the observed candidate counts (at most 8).
	for i := 1; i < len(order); i++ {
		for j := i; j > 0; j-- {
			if !derivationSetCensusFoldLess(paths[order[j]], paths[order[j-1]]) {
				break
			}
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	return order
}

func derivationSetCensusFoldLess(left, right core.Derivation) bool {
	if left.HasBranchOrder != right.HasBranchOrder {
		return !left.HasBranchOrder
	}
	if !left.HasBranchOrder {
		return false
	}
	return left.BranchOrder < right.BranchOrder
}

func derivationSetCensusSamePath(left, right core.Derivation) bool {
	if left.Score != right.Score || left.HasBranchOrder != right.HasBranchOrder || left.BranchOrder != right.BranchOrder {
		return false
	}
	if len(left.Payloads) != len(right.Payloads) {
		return false
	}
	for index := range left.Payloads {
		if left.Payloads[index] != right.Payloads[index] {
			return false
		}
	}
	return true
}

// derivationSetCensusShape writes the canonical deep dump of one materialized
// candidate. It records exactly the axes the materiality gate compares
// (symbol, span, named/extra/missing, per-child field name, children), so a
// shape match and a gate match always agree.
func derivationSetCensusShape(builder *strings.Builder, lang *Language, node *Node, field string) {
	if node == nil {
		builder.WriteString("(nil)")
		return
	}
	builder.WriteString("(")
	if field != "" {
		builder.WriteString(field)
		builder.WriteString(":")
	}
	fmt.Fprintf(builder, "%s[%d-%d]", node.Type(lang), node.StartByte(), node.EndByte())
	if !node.IsNamed() {
		builder.WriteString("!anon")
	}
	if node.IsExtra() {
		builder.WriteString("!extra")
	}
	if node.IsMissing() {
		builder.WriteString("!missing")
	}
	childCount := node.ChildCount()
	for index := 0; index < childCount; index++ {
		builder.WriteString(" ")
		derivationSetCensusShape(builder, lang, node.Child(index), node.FieldNameForChild(index, lang))
	}
	builder.WriteString(")")
}

// derivationSetCensusNodes appends one pre-order entry per node of the
// materialized candidate.
func derivationSetCensusNodes(out []DerivationSetCensusNode, lang *Language, node *Node, field string) []DerivationSetCensusNode {
	if node == nil {
		// Keep one entry per declared child so the flat slice always
		// rebuilds a tree with the recorded child counts.
		return append(out, DerivationSetCensusNode{Field: field, Type: "<nil>"})
	}
	childCount := node.ChildCount()
	out = append(out, DerivationSetCensusNode{
		Field: field, Type: node.Type(lang),
		StartByte: node.StartByte(), EndByte: node.EndByte(),
		ChildCount: childCount,
		Named:      node.IsNamed(), Extra: node.IsExtra(), Missing: node.IsMissing(),
	})
	for index := 0; index < childCount; index++ {
		out = derivationSetCensusNodes(out, lang, node.Child(index), node.FieldNameForChild(index, lang))
	}
	return out
}

//go:build gts_eof_recovery_shadow

package parsercorephase0

import (
	"errors"
	"fmt"
	"sync"
)

const (
	// DiagnosticEOFRecoveryMaxPayloads bounds one private recover_eof fold.
	DiagnosticEOFRecoveryMaxPayloads = 4096
	// DiagnosticEOFRecoveryMaxCloneBytes bounds the copied compact state.
	DiagnosticEOFRecoveryMaxCloneBytes = 16 << 20
)

// DiagnosticEOFRecoveryForkReceipt proves the bounds and isolation for one
// diagnostic recover_eof fold. This type is absent from production builds.
type DiagnosticEOFRecoveryForkReceipt struct {
	Steps                  uint32
	MaxSteps               uint32
	Payloads               uint32
	MaxPayloads            uint32
	SourceFootprintBytes   uint64
	MaxCloneBytes          uint64
	StartByte              uint32
	EndByte                uint32
	SubtreesBefore         uint32
	SubtreesAfter          uint32
	ChildrenBefore         uint32
	ChildrenAfter          uint32
	ExistingArenaPreserved bool
	RootChildrenExact      bool
	MutableStorageDisjoint bool
	WorkBefore             Work
	WorkAfter              Work
}

// ForkDiagnosticEOFRecovery copies the compact state and wraps one exact
// payload sequence in a private ERROR root. It never changes live.
func ForkDiagnosticEOFRecovery(
	live *Core,
	head Head,
	payloads []SubtreeID,
) (*Core, SubtreeID, DiagnosticEOFRecoveryForkReceipt, error) {
	var receipt DiagnosticEOFRecoveryForkReceipt
	receipt.MaxSteps = 1
	receipt.MaxPayloads = DiagnosticEOFRecoveryMaxPayloads
	receipt.MaxCloneBytes = DiagnosticEOFRecoveryMaxCloneBytes
	if live == nil {
		return nil, 0, receipt, errors.New("parser-core phase zero: nil EOF recovery source")
	}
	if len(live.transactions) != 0 {
		return nil, 0, receipt, errors.New("parser-core phase zero: EOF recovery source has an active nested transaction")
	}
	if _, err := live.node(head.Node); err != nil {
		return nil, 0, receipt, err
	}
	if len(payloads) == 0 || len(payloads) > DiagnosticEOFRecoveryMaxPayloads {
		return nil, 0, receipt, fmt.Errorf(
			"parser-core phase zero: EOF recovery payload count %d is outside 1..%d",
			len(payloads), DiagnosticEOFRecoveryMaxPayloads,
		)
	}
	receipt.Payloads = uint32(len(payloads))
	receipt.SourceFootprintBytes = live.FootprintBytes()
	if receipt.SourceFootprintBytes > DiagnosticEOFRecoveryMaxCloneBytes {
		return nil, 0, receipt, fmt.Errorf(
			"parser-core phase zero: EOF recovery clone footprint %d exceeds %d",
			receipt.SourceFootprintBytes, DiagnosticEOFRecoveryMaxCloneBytes,
		)
	}

	shadow := cloneDiagnosticEOFRecoveryCore(live)
	receipt.MutableStorageDisjoint = diagnosticEOFRecoveryStorageDisjoint(live, shadow)
	if !receipt.MutableStorageDisjoint {
		return nil, 0, receipt, errors.New("parser-core phase zero: EOF recovery clone aliases mutable storage")
	}

	first, err := shadow.subtree(payloads[0])
	if err != nil {
		return nil, 0, receipt, err
	}
	last, err := shadow.subtree(payloads[len(payloads)-1])
	if err != nil {
		return nil, 0, receipt, err
	}
	if last.endByte < first.startByte {
		return nil, 0, receipt, errors.New("parser-core phase zero: EOF recovery payload span is reversed")
	}
	receipt.StartByte = first.startByte
	receipt.EndByte = last.endByte
	receipt.SubtreesBefore = uint32(len(shadow.subtrees))
	receipt.ChildrenBefore = uint32(len(shadow.children))
	beforeSubtrees := append([]subtreeRecord(nil), shadow.subtrees...)
	beforeChildren := append([]SubtreeID(nil), shadow.children...)
	receipt.WorkBefore = shadow.Work()
	root, err := shadow.appendSubtree(subtreeRecord{
		symbol: ErrorRegionSymbol, startByte: first.startByte, endByte: last.endByte,
	}, append([]SubtreeID(nil), payloads...), nil, nil)
	if err != nil {
		return nil, 0, receipt, err
	}
	receipt.Steps = 1
	receipt.SubtreesAfter = uint32(len(shadow.subtrees))
	receipt.ChildrenAfter = uint32(len(shadow.children))
	receipt.ExistingArenaPreserved = equalDiagnosticSlice(beforeSubtrees, shadow.subtrees[:len(beforeSubtrees)]) &&
		equalDiagnosticSlice(beforeChildren, shadow.children[:len(beforeChildren)])
	receipt.RootChildrenExact = diagnosticEOFRecoveryRootChildrenEqual(shadow, root, payloads)
	receipt.WorkAfter = shadow.Work()
	return shadow, root, receipt, nil
}

func diagnosticEOFRecoveryRootChildrenEqual(shadow *Core, root SubtreeID, payloads []SubtreeID) bool {
	record, err := shadow.subtree(root)
	if err != nil || uint64(record.firstChild)+uint64(record.childCount) > uint64(len(shadow.children)) {
		return false
	}
	children := shadow.children[record.firstChild : record.firstChild+record.childCount]
	return equalDiagnosticSlice(children, payloads)
}

func cloneDiagnosticEOFRecoveryCore(live *Core) *Core {
	shadow := *live
	shadow.nodes = append([]nodeRecord(nil), live.nodes...)
	shadow.nodeLineages = append([]nodeLineageRecord(nil), live.nodeLineages...)
	shadow.nodeCheckpoints = append([]CheckpointID(nil), live.nodeCheckpoints...)
	shadow.links = append([]linkRecord(nil), live.links...)
	shadow.subtrees = append([]subtreeRecord(nil), live.subtrees...)
	shadow.externalProvenance = append([]externalPayloadProvenance(nil), live.externalProvenance...)
	shadow.children = append([]SubtreeID(nil), live.children...)
	shadow.fields = append([]FieldMapEntry(nil), live.fields...)
	shadow.aliases = append([]Symbol(nil), live.aliases...)
	shadow.checkpoints.records = append([]checkpointRecord(nil), live.checkpoints.records...)
	shadow.checkpoints.bytes = append([]byte(nil), live.checkpoints.bytes...)
	shadow.checkpoints.buckets = make(map[[32]byte]CheckpointID, len(live.checkpoints.buckets))
	for digest, id := range live.checkpoints.buckets {
		shadow.checkpoints.buckets[digest] = id
	}
	shadow.boundaries.slots = append([]boundarySlot(nil), live.boundaries.slots...)
	shadow.alternativeSpillArena = append([]uint32(nil), live.alternativeSpillArena...)
	if live.selectedPolicy != nil {
		policy := cloneDiagnosticEOFRecoverySelectedPolicy(*live.selectedPolicy)
		shadow.selectedPolicy = &policy
	}

	// The shadow does not inherit live journals, capabilities, or scratch.
	shadow.boundaryJournal = nil
	shadow.nodeLineageJournal = nil
	shadow.condenseCandidates = nil
	shadow.condenseNewNode = 0
	shadow.condenseScopeActive = false
	shadow.reductionSourceOwner = 0
	shadow.transactions = nil
	shadow.popScratch = popEnumerationScratch{}
	shadow.reductionScratch = reductionOutputScratch{}
	shadow.historicalNodeScratch = nil
	shadow.cohortHeadScratch = nil
	shadow.factorLinkScratch = nil
	shadow.selectedBuild = selectedStoreBuildScratch{}
	shadow.selectedPoolMu = sync.Mutex{}
	shadow.selectedPool = selectedStoreBacking{}
	shadow.schedulerFrame = schedulerTransactionFrame{}
	return &shadow
}

func cloneDiagnosticEOFRecoverySelectedPolicy(policy SelectedStorePolicy) SelectedStorePolicy {
	policy.Symbols = append([]SelectedSymbolPolicy(nil), policy.Symbols...)
	policy.Unary = append([]SelectedUnaryRule(nil), policy.Unary...)
	policy.RetainedAliasChildren = append([]SelectedAliasChildPair(nil), policy.RetainedAliasChildren...)
	policy.SemicolonContainers = append([]bool(nil), policy.SemicolonContainers...)
	policy.Cases = append([]bool(nil), policy.Cases...)
	policy.StatementLists = append([]bool(nil), policy.StatementLists...)
	return policy
}

func diagnosticEOFRecoveryStorageDisjoint(live, shadow *Core) bool {
	if live == nil || shadow == nil || live == shadow {
		return false
	}
	return disjointDiagnosticSlice(live.nodes, shadow.nodes) &&
		disjointDiagnosticSlice(live.nodeLineages, shadow.nodeLineages) &&
		disjointDiagnosticSlice(live.nodeCheckpoints, shadow.nodeCheckpoints) &&
		disjointDiagnosticSlice(live.links, shadow.links) &&
		disjointDiagnosticSlice(live.subtrees, shadow.subtrees) &&
		disjointDiagnosticSlice(live.externalProvenance, shadow.externalProvenance) &&
		disjointDiagnosticSlice(live.children, shadow.children) &&
		disjointDiagnosticSlice(live.fields, shadow.fields) &&
		disjointDiagnosticSlice(live.aliases, shadow.aliases) &&
		disjointDiagnosticSlice(live.checkpoints.records, shadow.checkpoints.records) &&
		disjointDiagnosticSlice(live.checkpoints.bytes, shadow.checkpoints.bytes) &&
		disjointDiagnosticSlice(live.boundaries.slots, shadow.boundaries.slots) &&
		disjointDiagnosticSlice(live.alternativeSpillArena, shadow.alternativeSpillArena) &&
		diagnosticEOFRecoveryPolicyDisjoint(live.selectedPolicy, shadow.selectedPolicy)
}

func diagnosticEOFRecoveryPolicyDisjoint(live, shadow *SelectedStorePolicy) bool {
	if live == nil || shadow == nil {
		return live == nil && shadow == nil
	}
	if live == shadow {
		return false
	}
	return disjointDiagnosticSlice(live.Symbols, shadow.Symbols) &&
		disjointDiagnosticSlice(live.Unary, shadow.Unary) &&
		disjointDiagnosticSlice(live.RetainedAliasChildren, shadow.RetainedAliasChildren) &&
		disjointDiagnosticSlice(live.SemicolonContainers, shadow.SemicolonContainers) &&
		disjointDiagnosticSlice(live.Cases, shadow.Cases) &&
		disjointDiagnosticSlice(live.StatementLists, shadow.StatementLists)
}

func disjointDiagnosticSlice[T any](left, right []T) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	return &left[0] != &right[0]
}

func equalDiagnosticSlice[T comparable](left, right []T) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

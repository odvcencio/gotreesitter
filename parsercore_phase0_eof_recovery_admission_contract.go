//go:build !gts_no_parsercorephase0 && gts_eof_recovery_admission_contract

package gotreesitter

import "sync"

// EOFRecoveryAdmissionEvent records one authenticated EOF election event.
type EOFRecoveryAdmissionEvent struct {
	Ordinal           int
	Kind              string
	Cost              uint32
	DynamicPrecedence int64
}

// EOFRecoveryAdmissionWork records the bounded producer work.
type EOFRecoveryAdmissionWork struct {
	Polls                      uint64
	SourceChunks               uint64
	ChildGroups                uint64
	PathsVisited               uint64
	LinksVisited               uint64
	PayloadRecordsVisited      uint64
	MaxDepth                   uint64
	BytesInspected             uint64
	MaxSourceChunk             uint64
	MaxChildGroup              uint64
	CheckedArithmetic          uint64
	PublicationAttempts        uint64
	ParserConstructions        uint64
	TreeConstructions          uint64
	SelectedStoreConstructions uint64
	Overflow                   bool
}

// EOFRecoveryAdmissionReceipt records one consumed metadata-only admission.
type EOFRecoveryAdmissionReceipt struct {
	State               string
	CoreGeneration      uint64
	SourceLength        uint32
	NormalPayloads      uint32
	RecoveryPayloads    uint32
	NormalOccurrences   uint32
	RecoveryOccurrences uint32
	NormalFrontier      uint32
	RecoveryFrontier    uint32
	Events              [2]EOFRecoveryAdmissionEvent
	SelectedEvent       int
	MetadataOnly        bool
	ConsumptionCount    uint64
	ConstructionRoute   string
	// Mechanism names the admission route that produced this accept:
	// "scanner-free-eof" for a language with no external scanner, and
	// "eof-scanner-quiescence" for a scanner-owning language admitted by
	// proveCompactEOFScannerQuiescence
	// (parsercore_phase0_eof_scanner_quiescence.go).
	Mechanism string
	// ScannerQuiescenceProved reports a completed quiescence proof.
	ScannerQuiescenceProved bool
	// ScannerQuiescenceStateless reports that the scanner declared itself
	// stateless, so the payload comparison was vacuous by contract.
	ScannerQuiescenceStateless bool
	// ScannerQuiescenceStates counts the head states the proof measured.
	ScannerQuiescenceStates uint8
	// ExternalCount counts the zero-width external tokens both admitted
	// heads carry, and ExternalDigest folds that shared history.
	ExternalCount  uint32
	ExternalDigest [32]byte
	Work           EOFRecoveryAdmissionWork
}

// EOFRecoveryAdmissionMechanism names one admission route for the census.
const (
	EOFRecoveryAdmissionMechanismScannerFree      = "scanner-free-eof"
	EOFRecoveryAdmissionMechanismScannerQuiescent = "eof-scanner-quiescence"
)

var (
	eofRecoveryAdmissionCensusMu       sync.Mutex
	eofRecoveryAdmissionCensusReceipts []EOFRecoveryAdmissionReceipt
)

func init() {
	compactEOFRecoveryAdmissionCensusHook = recordEOFRecoveryAdmissionReceipt
}

// EOFRecoveryAdmissionCensusBuilt reports whether this binary includes the
// tagged EOF recovery admission census.
func EOFRecoveryAdmissionCensusBuilt() bool { return true }

// EOFRecoveryAdmissionCensusReset removes all recorded admission receipts.
func EOFRecoveryAdmissionCensusReset() {
	eofRecoveryAdmissionCensusMu.Lock()
	eofRecoveryAdmissionCensusReceipts = nil
	eofRecoveryAdmissionCensusMu.Unlock()
}

// EOFRecoveryAdmissionCensusSnapshot returns an independent receipt copy.
func EOFRecoveryAdmissionCensusSnapshot() []EOFRecoveryAdmissionReceipt {
	eofRecoveryAdmissionCensusMu.Lock()
	defer eofRecoveryAdmissionCensusMu.Unlock()
	return append([]EOFRecoveryAdmissionReceipt(nil), eofRecoveryAdmissionCensusReceipts...)
}

func recordEOFRecoveryAdmissionReceipt(receipt compactEOFRecoveryAdmissionReceipt) {
	record := EOFRecoveryAdmissionReceipt{
		State:               receipt.state.String(),
		CoreGeneration:      receipt.coreGeneration,
		SourceLength:        receipt.sourceLength,
		NormalPayloads:      receipt.normalPayloads,
		RecoveryPayloads:    receipt.recoveryPayloads,
		NormalOccurrences:   receipt.normalOccurrences,
		RecoveryOccurrences: receipt.recoveryOccurrences,
		NormalFrontier:      receipt.normalFrontier,
		RecoveryFrontier:    receipt.recoveryFrontier,
		SelectedEvent:       receipt.selectedEvent,
		MetadataOnly:        receipt.metadataOnly,
		ConsumptionCount:    receipt.consumptionCount,
		ConstructionRoute:   receipt.constructionRoute.String(),
		Mechanism: func() string {
			if receipt.scannerQuiescence.proved {
				return EOFRecoveryAdmissionMechanismScannerQuiescent
			}
			return EOFRecoveryAdmissionMechanismScannerFree
		}(),
		ScannerQuiescenceProved:    receipt.scannerQuiescence.proved,
		ScannerQuiescenceStateless: receipt.scannerQuiescence.stateless,
		ScannerQuiescenceStates:    receipt.scannerQuiescence.probedStates,
		ExternalCount:              receipt.externalCount,
		ExternalDigest:             receipt.externalDigest,
		Work: EOFRecoveryAdmissionWork{
			Polls:                      receipt.work.polls,
			SourceChunks:               receipt.work.sourceChunks,
			ChildGroups:                receipt.work.childGroups,
			PathsVisited:               receipt.work.pathsVisited,
			LinksVisited:               receipt.work.linksVisited,
			PayloadRecordsVisited:      receipt.work.payloadRecordsVisited,
			MaxDepth:                   receipt.work.maxDepth,
			BytesInspected:             receipt.work.bytesInspected,
			MaxSourceChunk:             receipt.work.maxSourceChunk,
			MaxChildGroup:              receipt.work.maxChildGroup,
			CheckedArithmetic:          receipt.work.checkedArithmetic,
			PublicationAttempts:        receipt.work.publicationAttempts,
			ParserConstructions:        receipt.work.parserConstructions,
			TreeConstructions:          receipt.work.treeConstructions,
			SelectedStoreConstructions: receipt.work.selectedStoreConstructions,
			Overflow:                   receipt.work.overflow,
		},
	}
	for index, event := range receipt.events {
		record.Events[index] = EOFRecoveryAdmissionEvent{
			Ordinal:           event.ordinal,
			Kind:              event.kind.String(),
			Cost:              event.cost,
			DynamicPrecedence: event.dynamicPrecedence,
		}
	}
	eofRecoveryAdmissionCensusMu.Lock()
	eofRecoveryAdmissionCensusReceipts = append(eofRecoveryAdmissionCensusReceipts, record)
	eofRecoveryAdmissionCensusMu.Unlock()
}

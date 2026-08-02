package parsercorephase0

import "unsafe"

// Byte footprints of the compact core's growable record families, computed
// once from each record type's real in-memory layout so StorageBytes stays
// authoritative if a record ever gains or loses a field.
var (
	coreNodeRecordBytes    = uint64(unsafe.Sizeof(nodeRecord{}))
	coreLinkRecordBytes    = uint64(unsafe.Sizeof(linkRecord{}))
	coreSubtreeRecordBytes = uint64(unsafe.Sizeof(subtreeRecord{}))
	coreChildRecordBytes   = uint64(unsafe.Sizeof(SubtreeID(0)))
	coreFieldRecordBytes   = uint64(unsafe.Sizeof(FieldMapEntry{}))
	coreAliasRecordBytes   = uint64(unsafe.Sizeof(Symbol(0)))
)

// StorageBytes returns a cheap, deterministic, O(1) estimate of the compact
// core's current record storage in bytes: every growable record family's
// live slice length times that family's real struct size. It costs six
// multiply-adds over already-tracked slice lengths -- no allocation, no
// traversal, no I/O -- so it is safe to call from a tight scheduler poll
// (spec.campaign.v7 tranche B8).
//
// StorageBytes is a documented LOWER BOUND, not a footprint gauge: it counts
// LENGTH (live records), not CAPACITY (allocated backing-array bytes, which
// amortized growth keeps ahead of length and Reset never shrinks), and it
// omits per-parse scratch, the boundary index, and checkpoint interning
// entirely. A caller that needs a real containment gauge -- one a memory-stop
// poll can actually trust to bound RSS -- must use FootprintBytes instead
// (tranche B9 storage-release/honest-accounting gate: StorageBytes alone let
// a declined parse's own scheduler run overshoot its configured budget by
// more than 11x before the poll noticed, because len()-only accounting is
// blind to retained capacity and to every structure named above). Keeping
// StorageBytes's own meaning unchanged here matters: some callers may still
// want the cheap, non-allocating, purely-length-based reading it always gave
// (for example a caller correlating record counts with Work counters), and
// changing its definition out from under them would be its own silent
// contract break.
//
// It grows monotonically with exactly the counters every mutating Core
// operation already increments, so it is reproducible for a given input and
// Limits: same input, same Limits, same StorageBytes trajectory, on every
// run.
func (c *Core) StorageBytes() uint64 {
	if c == nil {
		return 0
	}
	return uint64(len(c.nodes))*coreNodeRecordBytes +
		uint64(len(c.links))*coreLinkRecordBytes +
		uint64(len(c.subtrees))*coreSubtreeRecordBytes +
		uint64(len(c.children))*coreChildRecordBytes +
		uint64(len(c.fields))*coreFieldRecordBytes +
		uint64(len(c.aliases))*coreAliasRecordBytes
}

// Byte footprints for the additional families FootprintBytes covers that
// StorageBytes does not: the lineage/checkpoint/provenance side records, the
// checkpoint interner, the boundary index, and the two dominant per-parse
// scratch buffers (pop-path enumeration and reduction-output batching).
var (
	coreNodeLineageRecordBytes    = uint64(unsafe.Sizeof(nodeLineageRecord{}))
	coreCheckpointIDBytes         = uint64(unsafe.Sizeof(CheckpointID(0)))
	coreExternalProvenanceBytes   = uint64(unsafe.Sizeof(externalPayloadProvenance{}))
	coreCheckpointRecordBytes     = uint64(unsafe.Sizeof(checkpointRecord{}))
	coreCheckpointBucketBytes     = uint64(unsafe.Sizeof([32]byte{})) + uint64(unsafe.Sizeof(CheckpointID(0)))
	coreBoundarySlotBytes         = uint64(unsafe.Sizeof(boundarySlot{}))
	coreBoundaryMutationBytes     = uint64(unsafe.Sizeof(boundaryMutation{}))
	coreNodeLineageMutationBytes  = uint64(unsafe.Sizeof(nodeLineageMutation{}))
	coreUint32Bytes               = uint64(unsafe.Sizeof(uint32(0)))
	coreCondenseCandidateBytes    = uint64(unsafe.Sizeof(CondenseCandidate{}))
	coreUint64Bytes               = uint64(unsafe.Sizeof(uint64(0)))
	coreNodeIDBytes               = uint64(unsafe.Sizeof(NodeID(0)))
	coreHeadBytes                 = uint64(unsafe.Sizeof(Head{}))
	coreLinkRecordSliceBytes      = uint64(unsafe.Sizeof([]linkRecord(nil)))
	coreSubtreeIDBytes            = uint64(unsafe.Sizeof(SubtreeID(0)))
	coreInt64Bytes                = uint64(unsafe.Sizeof(int64(0)))
	coreForkOrderBytes            = uint64(unsafe.Sizeof(ForkOrder{}))
	corePathPayloadBytes          = uint64(unsafe.Sizeof(pathPayload{}))
	corePopPathBytes              = uint64(unsafe.Sizeof(popPath{}))
	coreReductionBoundaryOutBytes = uint64(unsafe.Sizeof(reductionBoundaryOutput{}))
	coreBoundaryKeyEntryBytes     = uint64(unsafe.Sizeof(boundaryKey{})) + uint64(unsafe.Sizeof(int(0)))
	coreUint16Bytes               = uint64(unsafe.Sizeof(uint16(0)))
)

// FootprintBytes returns a cheap, deterministic, O(1) estimate of the compact
// core's real retained-memory footprint in bytes: every growable record and
// scratch family's CAPACITY (not length) times that family's real struct
// size, covering the record families StorageBytes counts plus the lineage,
// checkpoint, and provenance side tables, the checkpoint interner, the
// boundary index, and the pop-path/reduction-output scratch buffers. Like
// StorageBytes it costs only already-tracked slice/map length and capacity
// reads -- no allocation, no traversal, no I/O -- so it is safe to call from
// a tight scheduler poll.
//
// This is the gauge the scheduler's stop-control memory-budget poll uses
// (pollStopControl in the root package), because capacity, not length, is
// what Reset leaves behind: Reset truncates every tracked slice to length
// zero but retains its backing array for reuse, so a length-based gauge
// reads zero immediately after Reset regardless of how much real memory a
// declined parse's arenas still hold. FootprintBytes reads real allocated
// bytes instead, so the poll can actually trip before a pathological input's
// true footprint clears the configured budget, not merely before its live
// record count would have.
//
// The two map-backed fields (the checkpoint interner's digest-to-ID bucket
// map and the reduction scratch's boundary-key-to-index map) have no cap();
// FootprintBytes estimates their footprint as len() times one bucket's
// key+value size, an underestimate of Go's real hash-table bucket overhead
// but a monotonic, deterministic one that never claims less than the map
// logically holds.
func (c *Core) FootprintBytes() uint64 {
	if c == nil {
		return 0
	}
	total := uint64(cap(c.nodes))*coreNodeRecordBytes +
		uint64(cap(c.links))*coreLinkRecordBytes +
		uint64(cap(c.subtrees))*coreSubtreeRecordBytes +
		uint64(cap(c.children))*coreChildRecordBytes +
		uint64(cap(c.fields))*coreFieldRecordBytes +
		uint64(cap(c.aliases))*coreAliasRecordBytes

	total += uint64(cap(c.nodeLineages)) * coreNodeLineageRecordBytes
	total += uint64(cap(c.nodeCheckpoints)) * coreCheckpointIDBytes
	total += uint64(cap(c.externalProvenance)) * coreExternalProvenanceBytes
	total += uint64(cap(c.boundaryJournal)) * coreBoundaryMutationBytes
	total += uint64(cap(c.nodeLineageJournal)) * coreNodeLineageMutationBytes
	total += uint64(cap(c.alternativeSpillArena)) * coreUint32Bytes
	total += uint64(cap(c.condenseCandidates)) * coreCondenseCandidateBytes
	total += uint64(cap(c.transactions)) * coreUint64Bytes
	total += uint64(cap(c.historicalNodeScratch)) * coreNodeIDBytes
	total += uint64(cap(c.cohortHeadScratch)) * coreHeadBytes
	total += uint64(cap(c.factorLinkScratch)) * coreLinkRecordBytes

	total += c.checkpoints.footprintBytes()
	total += c.boundaries.footprintBytes()
	total += c.popScratch.footprintBytes()
	total += c.reductionScratch.footprintBytes()
	total += selectedStoreRetainedBytes(cap(c.selectedPool.records), cap(c.selectedPool.children))

	return total
}

func (i *checkpointInterner) footprintBytes() uint64 {
	if i == nil {
		return 0
	}
	return uint64(cap(i.records))*coreCheckpointRecordBytes +
		uint64(cap(i.bytes)) +
		uint64(len(i.buckets))*coreCheckpointBucketBytes
}

func (b *boundaryIndex) footprintBytes() uint64 {
	if b == nil {
		return 0
	}
	return uint64(cap(b.slots)) * coreBoundarySlotBytes
}

// footprintBytes covers popEnumerationScratch's per-parse pop-path
// enumeration buffers. linkFrames is a slice of slices; only the outer
// backing array (one slice header per frame) is counted here, matching the
// bounded, per-reduction-fanout sizing the frame pool already caps. paths
// entries (popPath) each embed their own nested children/trailing slices,
// which nextPath reuses by truncating rather than reallocating -- this count
// includes each entry's own struct size (so its two nested slice headers
// count) but not the bytes those nested backing arrays hold beyond the
// header; dropping (nilling) paths still frees them in full, this is an
// accounting-completeness gap, not a release gap.
func (s *popEnumerationScratch) footprintBytes() uint64 {
	if s == nil {
		return 0
	}
	return uint64(cap(s.linkFrames))*coreLinkRecordSliceBytes +
		uint64(cap(s.rev))*coreSubtreeIDBytes +
		uint64(cap(s.revScores))*coreInt64Bytes +
		uint64(cap(s.revOrders))*coreForkOrderBytes +
		uint64(cap(s.trailing))*corePathPayloadBytes +
		uint64(cap(s.external))*coreSubtreeIDBytes +
		uint64(cap(s.paths))*corePopPathBytes
}

func (s *reductionOutputScratch) footprintBytes() uint64 {
	if s == nil {
		return 0
	}
	return uint64(cap(s.boundaries))*coreReductionBoundaryOutBytes +
		uint64(len(s.boundaryByKey))*coreBoundaryKeyEntryBytes +
		uint64(cap(s.batchParents))*coreSubtreeIDBytes +
		uint64(cap(s.structuralPositions))*coreUint16Bytes +
		uint64(cap(s.remappedFields))*coreFieldRecordBytes +
		uint64(cap(s.remappedAliases))*coreAliasRecordBytes
}

// coreRetentionCapBytes bounds how much FootprintBytes capacity Reset keeps
// pooled for reuse across parses on the same cached runner. Measured
// steady-state retention for a clean production-route parse (a reference
// point for "reasonable", not compact's own architecture) sits at roughly
// 12-15MB after release; coreRetentionCapBytes sets the drop threshold at
// noticeably above that, so ordinary variance in legitimate large-file
// workloads does not force reallocation on every parse, while a
// pathological input's post-decline retention (measured before this gate at
// 157-193MB for one large declined parse, tranche B9 retention-cap finding)
// still gets dropped rather than billed to every later unrelated parse.
const coreRetentionCapBytes = 48 << 20 // 48 MiB

// releaseOversizedRetention drops every growable family's backing array when
// the core's total FootprintBytes retained after Reset exceeds
// coreRetentionCapBytes, replacing each with nil so the next user
// reallocates from a clean slate instead of reusing an oversized slab. This
// mirrors the drop-when-alone-exceeds-budget pattern already used for the
// production GLR GSS/entry scratch pools (glr_gss.go, glr.go): a pathological
// parse can grow a single core's backing arrays far past ordinary steady
// state, and pooling that growth unchanged would otherwise bill it, unshrunk,
// to every later parse that reuses this Core regardless of language or file
// size.
//
// Call this only after Reset (it assumes every tracked length is already
// zero) and only from a decline path -- the routine "clear the slate before
// the next attempt" reset every fresh parse performs should keep preserving
// capacity for legitimate reuse across repeated large-file parses. Dropping
// unconditionally on every reset would instead force full reallocation on
// every large-fixture benchmark iteration.
func (c *Core) releaseOversizedRetention() {
	if c == nil || c.FootprintBytes() <= coreRetentionCapBytes {
		return
	}
	c.nodes = nil
	c.nodeLineages = nil
	c.nodeCheckpoints = nil
	c.links = nil
	c.subtrees = nil
	c.externalProvenance = nil
	c.children = nil
	c.fields = nil
	c.aliases = nil
	c.boundaryJournal = nil
	c.nodeLineageJournal = nil
	c.alternativeSpillArena = nil
	c.transactions = nil
	c.historicalNodeScratch = nil
	c.cohortHeadScratch = nil
	c.factorLinkScratch = nil
	c.checkpoints.dropOversized()
	c.boundaries.dropOversized()
	c.popScratch.dropOversized()
	c.reductionScratch.dropOversized()
	c.selectedPool = selectedStoreBacking{}
}

func (i *checkpointInterner) dropOversized() {
	if i == nil {
		return
	}
	i.records = nil
	i.bytes = nil
	i.buckets = nil
}

func (b *boundaryIndex) dropOversized() {
	if b == nil {
		return
	}
	b.slots = nil
}

func (s *popEnumerationScratch) dropOversized() {
	if s == nil {
		return
	}
	s.linkFrames = nil
	s.rev = nil
	s.revScores = nil
	s.revOrders = nil
	s.trailing = nil
	s.external = nil
	s.paths = nil
}

func (s *reductionOutputScratch) dropOversized() {
	if s == nil {
		return
	}
	s.boundaries = nil
	s.boundaryByKey = nil
	s.batchParents = nil
	s.structuralPositions = nil
	s.remappedFields = nil
	s.remappedAliases = nil
}
